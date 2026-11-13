package ocloudcommon

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	provisioningv1alpha1 "github.com/openshift-kni/oran-o2ims/api/provisioning/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/o-cloud/internal/ocloudinittools"
)

const (
	defaultScaleTimeout       = 90 * time.Minute
	scalePRPollInterval       = 10 * time.Second
	scaleSpokePollInterval    = 15 * time.Second
	masterRoleLabel           = "node-role.kubernetes.io/master"
	controlPlaneRoleLabel     = "node-role.kubernetes.io/control-plane"
	clusterInstanceParamsName = "clusterInstanceParameters"
)

type scaleRequestedNode struct {
	hostname string
	role     string
}

// VerifyScaleOneWorker removes one discovered worker and verifies it is restored.
func VerifyScaleOneWorker(ctx SpecContext) {
	verifyScaleWorkers(ctx, 1)
}

// VerifyScaleTwoWorkers removes two discovered workers and verifies they are restored.
func VerifyScaleTwoWorkers(ctx SpecContext) {
	verifyScaleWorkers(ctx, 2)
}

func verifyScaleWorkers(ctx SpecContext, workersToRemove int) {
	validateScaleConfiguration()

	timeout := scaleOperationTimeout()
	originalTemplateParameters, templateParameters := loadScalePRTemplateParameters()

	requestedNodes, err := parseScaleRequestedNodes(templateParameters)
	if err != nil {
		Skip(fmt.Sprintf("Scale test requires a supported nodes or nodeGroups layout: %v", err))
	}

	spokeNodes := readScaleBaselineNodes(requestedNodes)
	selectedWorkers := selectScaleWorkers(requestedNodes, spokeNodes, workersToRemove)
	selectedSet := scaleHostnameSet(selectedWorkers)

	originalHostnames := scaleRequestedHostnames(requestedNodes)
	retainedHostnames := withoutHostnames(originalHostnames, selectedSet)

	registerScaleRestore(originalTemplateParameters, originalHostnames, timeout)

	By(fmt.Sprintf("Scaling in worker nodes %v", selectedWorkers))

	scaledParameters, err := removeScaleRequestedNodes(templateParameters, selectedSet)
	Expect(err).NotTo(HaveOccurred(), "Failed to remove selected worker nodes from the ProvisioningRequest")

	minimumGeneration, updatedAfter := updateScaleProvisioningRequest(OCloudConfig.ScalePRName, scaledParameters)
	updatedPR := waitForScaleProvisioningRequest(ctx, OCloudConfig.ScalePRName, minimumGeneration, updatedAfter, timeout)
	expectScalePRHostnames(updatedPR, retainedHostnames)
	waitForScaleSpokeNodes(ctx, retainedHostnames, selectedWorkers, timeout)
}

func validateScaleConfiguration() {
	Expect(OCloudConfig.ScalePRName).NotTo(BeEmpty(),
		"ECO_OCLOUD_SCALE_PR_NAME must identify the existing ProvisioningRequest")
	Expect(HubAPIClient).NotTo(BeNil(), "hub API client is not initialized")
	Expect(ScaleSpokeAPIClient).NotTo(BeNil(),
		"ECO_OCLOUD_SCALE_SPOKE_KUBECONFIG must point to the target spoke kubeconfig")
}

func scaleOperationTimeout() time.Duration {
	timeout := OCloudConfig.ScaleTimeout
	if timeout <= 0 {
		return defaultScaleTimeout
	}

	return timeout
}

func loadScalePRTemplateParameters() ([]byte, map[string]any) {
	By(fmt.Sprintf("Reading ProvisioningRequest %s and the target spoke node inventory", OCloudConfig.ScalePRName))
	provisioningRequest, err := oran.PullPR(HubAPIClient, OCloudConfig.ScalePRName)
	Expect(err).NotTo(HaveOccurred(), "Failed to pull ProvisioningRequest %s", OCloudConfig.ScalePRName)

	request := provisioningRequest.Object
	Expect(request.Status.ProvisioningStatus.ProvisioningPhase).
		To(Equal(provisioningv1alpha1.StateFulfilled), "ProvisioningRequest must be fulfilled before scaling")
	Expect(request.Status.ObservedGeneration).To(BeNumerically(">=", request.Generation),
		"ProvisioningRequest status has not observed generation %d", request.Generation)

	originalTemplateParameters := bytes.Clone(request.Spec.TemplateParameters.Raw)
	Expect(originalTemplateParameters).NotTo(BeEmpty(), "ProvisioningRequest has empty templateParameters")

	templateParameters, err := provisioningRequest.GetTemplateParameters()
	Expect(err).NotTo(HaveOccurred(), "Failed to read ProvisioningRequest templateParameters")

	return originalTemplateParameters, templateParameters
}

func readScaleBaselineNodes(requestedNodes []scaleRequestedNode) map[string]*nodes.Builder {
	spokeNodes, err := listScaleSpokeNodes()
	Expect(err).NotTo(HaveOccurred(), "Failed to list nodes from the configured spoke")

	for _, requestedNode := range requestedNodes {
		spokeNode, found := spokeNodes[requestedNode.hostname]
		if !found {
			Skip(fmt.Sprintf("PR host %q does not match a node on the configured spoke", requestedNode.hostname))
		}

		if !scaleNodeReady(spokeNode) {
			Skip(fmt.Sprintf("PR host %q is not Ready on the configured spoke", requestedNode.hostname))
		}
	}

	return spokeNodes
}

func selectScaleWorkers(
	requestedNodes []scaleRequestedNode,
	spokeNodes map[string]*nodes.Builder,
	workersToRemove int) []string {
	workerHostnames, err := scaleWorkerHostnames(requestedNodes, spokeNodes)
	if err != nil {
		Skip(fmt.Sprintf("Cannot safely identify worker-only nodes: %v", err))
	}

	minimumWorkers := workersToRemove + 1
	if len(workerHostnames) < minimumWorkers {
		Skip(fmt.Sprintf("This case removes %d worker(s) and requires at least %d worker-only nodes; found %d",
			workersToRemove, minimumWorkers, len(workerHostnames)))
	}

	sort.Strings(workerHostnames)

	return workerHostnames[len(workerHostnames)-workersToRemove:]
}

func scaleHostnameSet(hostnames []string) map[string]struct{} {
	selected := make(map[string]struct{}, len(hostnames))
	for _, hostname := range hostnames {
		selected[hostname] = struct{}{}
	}

	return selected
}

func registerScaleRestore(originalTemplateParameters []byte, originalHostnames []string, timeout time.Duration) {
	DeferCleanup(func(cleanupCtx SpecContext) {
		By("Restoring the original ProvisioningRequest template parameters")
		restoreScaleProvisioningRequest(cleanupCtx, OCloudConfig.ScalePRName,
			originalTemplateParameters, originalHostnames, timeout)
		waitForScaleSpokeNodes(cleanupCtx, originalHostnames, nil, timeout)
	}, NodeTimeout(2*timeout+time.Minute))
}

func expectScalePRHostnames(provisioningRequest *oran.ProvisioningRequestBuilder, expectedHostnames []string) {
	Expect(provisioningRequest).NotTo(BeNil(), "ProvisioningRequest builder is nil")
	templateParameters, err := provisioningRequest.GetTemplateParameters()
	Expect(err).NotTo(HaveOccurred(), "Failed to read updated ProvisioningRequest templateParameters")
	requestedNodes, err := parseScaleRequestedNodes(templateParameters)
	Expect(err).NotTo(HaveOccurred(), "Failed to parse updated ProvisioningRequest node layout")

	actualHostnames := scaleRequestedHostnames(requestedNodes)
	sort.Strings(actualHostnames)

	expected := append([]string(nil), expectedHostnames...)
	sort.Strings(expected)
	Expect(actualHostnames).To(Equal(expected),
		"ProvisioningRequest node list does not match the requested scale topology")
}

func parseScaleRequestedNodes(templateParameters map[string]any) ([]scaleRequestedNode, error) {
	clusterInstanceParametersRaw, found := templateParameters[clusterInstanceParamsName]
	if !found {
		return nil, fmt.Errorf("templateParameters has no %q", clusterInstanceParamsName)
	}

	clusterInstanceParameters, ok := clusterInstanceParametersRaw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%q is not an object", clusterInstanceParamsName)
	}

	flatNodes, hasFlatNodes := clusterInstanceParameters["nodes"]

	nodeGroups, hasNodeGroups := clusterInstanceParameters["nodeGroups"]
	if hasFlatNodes == hasNodeGroups {
		return nil, fmt.Errorf("expected exactly one of nodes or nodeGroups")
	}

	var requestedNodes []scaleRequestedNode
	if hasFlatNodes {
		if err := appendScaleRequestedNodes(&requestedNodes, flatNodes, ""); err != nil {
			return nil, err
		}
	} else {
		var err error

		requestedNodes, err = parseScaleNodeGroups(nodeGroups)
		if err != nil {
			return nil, err
		}
	}

	if len(requestedNodes) == 0 {
		return nil, fmt.Errorf("node layout contains no nodes")
	}

	if err := validateUniqueScaleHostnames(requestedNodes); err != nil {
		return nil, err
	}

	return requestedNodes, nil
}

func parseScaleNodeGroups(nodeGroupsRaw any) ([]scaleRequestedNode, error) {
	groups, ok := nodeGroupsRaw.([]any)
	if !ok {
		return nil, fmt.Errorf("nodeGroups is not a list")
	}

	var requestedNodes []scaleRequestedNode

	for groupIndex, groupRaw := range groups {
		group, ok := groupRaw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("nodeGroups[%d] is not an object", groupIndex)
		}

		groupRole := normalizeScaleRole(group["role"])
		if groupRole == "" {
			groupRole = normalizeScaleRole(group["name"])
		}

		if groupRole != "master" && groupRole != "worker" && groupRole != "control-plane" {
			groupRole = ""
		}

		groupNodes, found := group["nodes"]
		if !found {
			return nil, fmt.Errorf("nodeGroups[%d] has no nodes list", groupIndex)
		}

		if err := appendScaleRequestedNodes(&requestedNodes, groupNodes, groupRole); err != nil {
			return nil, fmt.Errorf("nodeGroups[%d]: %w", groupIndex, err)
		}
	}

	return requestedNodes, nil
}

func validateUniqueScaleHostnames(requestedNodes []scaleRequestedNode) error {
	seenHostnames := make(map[string]struct{}, len(requestedNodes))
	for _, requestedNode := range requestedNodes {
		if _, found := seenHostnames[requestedNode.hostname]; found {
			return fmt.Errorf("duplicate requested hostname %q", requestedNode.hostname)
		}

		seenHostnames[requestedNode.hostname] = struct{}{}
	}

	return nil
}

func appendScaleRequestedNodes(
	requestedNodes *[]scaleRequestedNode,
	nodesRaw any,
	inheritedRole string) error {
	nodeList, ok := nodesRaw.([]any)
	if !ok {
		return fmt.Errorf("nodes is not a list")
	}

	for nodeIndex, nodeRaw := range nodeList {
		node, isObject := nodeRaw.(map[string]any)
		if !isObject {
			return fmt.Errorf("nodes[%d] is not an object", nodeIndex)
		}

		hostname, ok := node["hostName"].(string)
		if !ok || strings.TrimSpace(hostname) == "" {
			return fmt.Errorf("nodes[%d] has no hostName", nodeIndex)
		}

		nodeRole := normalizeScaleRole(node["role"])

		role := inheritedRole
		if nodeRole == "master" || nodeRole == "control-plane" {
			role = nodeRole
		} else if role != "master" && role != "control-plane" && nodeRole != "" {
			role = nodeRole
		}

		*requestedNodes = append(*requestedNodes, scaleRequestedNode{hostname: hostname, role: role})
	}

	return nil
}

func normalizeScaleRole(raw any) string {
	role, ok := raw.(string)
	if !ok {
		return ""
	}

	return strings.ToLower(strings.TrimSpace(role))
}

func listScaleSpokeNodes() (map[string]*nodes.Builder, error) {
	nodeBuilders, err := nodes.List(ScaleSpokeAPIClient)
	if err != nil {
		return nil, err
	}

	spokeNodes := make(map[string]*nodes.Builder, len(nodeBuilders))
	for _, nodeBuilder := range nodeBuilders {
		if nodeBuilder == nil || nodeBuilder.Object == nil {
			continue
		}

		spokeNodes[nodeBuilder.Object.Name] = nodeBuilder
	}

	return spokeNodes, nil
}

func scaleNodeReady(node *nodes.Builder) bool {
	if node == nil || node.Object == nil {
		return false
	}

	for _, condition := range node.Object.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}

	return false
}

func scaleWorkerHostnames(
	requestedNodes []scaleRequestedNode,
	spokeNodes map[string]*nodes.Builder) ([]string, error) {
	var workers []string

	for _, requestedNode := range requestedNodes {
		spokeNode, found := spokeNodes[requestedNode.hostname]
		if !found || spokeNode == nil || spokeNode.Object == nil {
			return nil, fmt.Errorf("requested host %q is missing from the spoke", requestedNode.hostname)
		}

		// Master/control-plane identity takes precedence when a node carries both labels.
		if scaleControlPlaneNode(requestedNode, spokeNode.Object.Labels) {
			continue
		}

		_, liveWorker := spokeNode.Object.Labels[inittools.GeneralConfig.WorkerLabel]
		if requestedNode.role == "worker" && !liveWorker {
			return nil, fmt.Errorf("PR marks host %q as worker but the spoke has no worker role label",
				requestedNode.hostname)
		}

		if requestedNode.role != "" && requestedNode.role != "worker" {
			return nil, fmt.Errorf("PR role %q for host %q conflicts with the spoke worker role label",
				requestedNode.role, requestedNode.hostname)
		}

		if liveWorker {
			workers = append(workers, requestedNode.hostname)

			continue
		}

		return nil, fmt.Errorf("role for host %q cannot be determined from the PR or spoke node labels",
			requestedNode.hostname)
	}

	sort.Strings(workers)

	return workers, nil
}

func scaleControlPlaneNode(requestedNode scaleRequestedNode, nodeLabels map[string]string) bool {
	if requestedNode.role == "master" || requestedNode.role == "control-plane" {
		return true
	}

	config := inittools.GeneralConfig
	for _, label := range []string{
		masterRoleLabel,
		controlPlaneRoleLabel,
		config.ControlPlaneLabel,
		config.KubernetesRolePrefix + "/master",
		config.KubernetesRolePrefix + "/control-plane",
	} {
		if _, found := nodeLabels[label]; found {
			return true
		}
	}

	return false
}

func scaleRequestedHostnames(requestedNodes []scaleRequestedNode) []string {
	hostnames := make([]string, 0, len(requestedNodes))
	for _, requestedNode := range requestedNodes {
		hostnames = append(hostnames, requestedNode.hostname)
	}

	return hostnames
}

func withoutHostnames(hostnames []string, removed map[string]struct{}) []string {
	retained := make([]string, 0, len(hostnames)-len(removed))
	for _, hostname := range hostnames {
		if _, found := removed[hostname]; !found {
			retained = append(retained, hostname)
		}
	}

	sort.Strings(retained)

	return retained
}

func removeScaleRequestedNodes(
	templateParameters map[string]any,
	removedHostnames map[string]struct{}) (map[string]any, error) {
	clusterInstanceParameters, ok := templateParameters[clusterInstanceParamsName].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%q is not an object", clusterInstanceParamsName)
	}

	flatNodes, hasFlatNodes := clusterInstanceParameters["nodes"]

	nodeGroups, hasNodeGroups := clusterInstanceParameters["nodeGroups"]
	if hasFlatNodes == hasNodeGroups {
		return nil, fmt.Errorf("expected exactly one of nodes or nodeGroups")
	}

	var (
		removedCount int
		err          error
	)
	if hasFlatNodes {
		removedCount, err = removeScaleFlatNodes(clusterInstanceParameters, flatNodes, removedHostnames)
	} else {
		removedCount, err = removeScaleNodeGroups(clusterInstanceParameters, nodeGroups, removedHostnames)
	}

	if err != nil {
		return nil, err
	}

	if removedCount != len(removedHostnames) {
		return nil, fmt.Errorf("removed %d of %d requested hostnames", removedCount, len(removedHostnames))
	}

	return templateParameters, nil
}

func removeScaleFlatNodes(
	clusterInstanceParameters map[string]any,
	nodesRaw any,
	removedHostnames map[string]struct{}) (int, error) {
	nodeList, ok := nodesRaw.([]any)
	if !ok {
		return 0, fmt.Errorf("nodes is not a list")
	}

	retainedNodes, removedCount, err := filterScaleNodeList(nodeList, removedHostnames)
	if err != nil {
		return 0, err
	}

	clusterInstanceParameters["nodes"] = retainedNodes

	return removedCount, nil
}

func removeScaleNodeGroups(
	clusterInstanceParameters map[string]any,
	nodeGroupsRaw any,
	removedHostnames map[string]struct{}) (int, error) {
	groups, ok := nodeGroupsRaw.([]any)
	if !ok {
		return 0, fmt.Errorf("nodeGroups is not a list")
	}

	var removedCount int

	retainedGroups := make([]any, 0, len(groups))
	for groupIndex, groupRaw := range groups {
		group, isObject := groupRaw.(map[string]any)
		if !isObject {
			return 0, fmt.Errorf("nodeGroups[%d] is not an object", groupIndex)
		}

		groupNodes, ok := group["nodes"].([]any)
		if !ok {
			return 0, fmt.Errorf("nodeGroups[%d].nodes is not a list", groupIndex)
		}

		retainedNodes, count, err := filterScaleNodeList(groupNodes, removedHostnames)
		if err != nil {
			return 0, fmt.Errorf("nodeGroups[%d]: %w", groupIndex, err)
		}

		removedCount += count

		if len(retainedNodes) == 0 {
			continue
		}

		group["nodes"] = retainedNodes

		retainedGroups = append(retainedGroups, groupRaw)
	}

	clusterInstanceParameters["nodeGroups"] = retainedGroups

	return removedCount, nil
}

func filterScaleNodeList(
	nodeList []any,
	removedHostnames map[string]struct{}) ([]any, int, error) {
	retainedNodes := make([]any, 0, len(nodeList))

	var removedCount int

	for _, nodeRaw := range nodeList {
		node, isObject := nodeRaw.(map[string]any)
		if !isObject {
			return nil, 0, fmt.Errorf("node entry is not an object")
		}

		hostname, _ := node["hostName"].(string)
		if _, remove := removedHostnames[hostname]; remove {
			removedCount++

			continue
		}

		retainedNodes = append(retainedNodes, nodeRaw)
	}

	return retainedNodes, removedCount, nil
}

func updateScaleProvisioningRequest(prName string, templateParameters map[string]any) (int64, time.Time) {
	provisioningRequest, err := oran.PullPR(HubAPIClient, prName)
	Expect(err).NotTo(HaveOccurred(), "Failed to refresh ProvisioningRequest %s before update", prName)

	previousGeneration := provisioningRequest.Object.Generation

	provisioningRequest.Definition = provisioningRequest.Object.DeepCopy()
	provisioningRequest = provisioningRequest.WithTemplateParameters(templateParameters)
	// Match the MNO upgrade timestamp allowance for API precision and small clock differences.
	updatedAfter := time.Now().Add(-5 * time.Second).Truncate(time.Second)
	_, err = provisioningRequest.Update()
	Expect(err).NotTo(HaveOccurred(), "Failed to update ProvisioningRequest %s", prName)

	return previousGeneration + 1, updatedAfter
}

func waitForScaleProvisioningRequest(
	ctx SpecContext,
	prName string,
	minimumGeneration int64,
	updatedAfter time.Time,
	timeout time.Duration) *oran.ProvisioningRequestBuilder {
	var fulfilledPR *oran.ProvisioningRequestBuilder

	Eventually(func(ctx context.Context) bool {
		currentPR, err := oran.PullPR(HubAPIClient, prName)
		if err != nil || currentPR == nil || currentPR.Object == nil {
			return false
		}

		fulfilledPR = currentPR

		return scalePRFulfilledAfter(currentPR.Object, minimumGeneration, updatedAfter)
	}).WithTimeout(timeout).WithPolling(scalePRPollInterval).WithContext(ctx).Should(BeTrue(),
		"ProvisioningRequest %s did not observe generation %d and return to Fulfilled after %s within %s",
		prName, minimumGeneration, updatedAfter, timeout)

	return fulfilledPR
}

func scalePRFulfilledAfter(
	request *provisioningv1alpha1.ProvisioningRequest, minimumGeneration int64, updatedAfter time.Time) bool {
	status := request.Status

	return request.Generation >= minimumGeneration &&
		status.ObservedGeneration >= request.Generation &&
		status.ProvisioningStatus.ProvisioningPhase == provisioningv1alpha1.StateFulfilled &&
		(updatedAfter.IsZero() || status.ProvisioningStatus.UpdateTime.After(updatedAfter))
}

func waitForScaleSpokeNodes(
	ctx SpecContext,
	expectedHostnames []string,
	absentHostnames []string,
	timeout time.Duration) {
	expected := append([]string(nil), expectedHostnames...)
	absent := append([]string(nil), absentHostnames...)
	Eventually(func(ctx context.Context) bool {
		spokeNodes, err := listScaleSpokeNodes()
		if err != nil {
			return false
		}

		for _, hostname := range expected {
			if !scaleNodeReady(spokeNodes[hostname]) {
				return false
			}
		}

		for _, hostname := range absent {
			if _, found := spokeNodes[hostname]; found {
				return false
			}
		}

		return true
	}).WithTimeout(timeout).WithPolling(scaleSpokePollInterval).WithContext(ctx).Should(BeTrue(),
		"Spoke did not reach expected node state within %s; expected Ready nodes %v and absent nodes %v",
		timeout, expected, absent)
}

func restoreScaleProvisioningRequest(
	ctx SpecContext,
	prName string,
	originalTemplateParameters []byte,
	originalHostnames []string,
	timeout time.Duration) {
	provisioningRequest, err := oran.PullPR(HubAPIClient, prName)
	Expect(err).NotTo(HaveOccurred(), "Failed to pull ProvisioningRequest %s for restoration", prName)

	currentTemplateParameters := provisioningRequest.Object.Spec.TemplateParameters.Raw
	minimumGeneration := provisioningRequest.Object.Generation

	var updatedAfter time.Time

	if !bytes.Equal(currentTemplateParameters, originalTemplateParameters) {
		provisioningRequest.Definition = provisioningRequest.Object.DeepCopy()
		provisioningRequest.Definition.Spec.TemplateParameters.Raw = bytes.Clone(originalTemplateParameters)
		updatedAfter = time.Now().Add(-5 * time.Second).Truncate(time.Second)
		_, err = provisioningRequest.Update()
		Expect(err).NotTo(HaveOccurred(), "Failed to restore ProvisioningRequest %s", prName)

		minimumGeneration++
	}

	restoredPR := waitForScaleProvisioningRequest(ctx, prName, minimumGeneration, updatedAfter, timeout)
	expectScalePRHostnames(restoredPR, originalHostnames)
}
