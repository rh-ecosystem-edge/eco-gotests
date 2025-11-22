package ocloudcommon

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/o-cloud/internal/ocloudinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/o-cloud/internal/ocloudparams"

	"github.com/google/uuid"
	bmhv1alpha1 "github.com/metal3-io/baremetal-operator/apis/metal3.io/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/bmh"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ocm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/siteconfig"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/shell"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// VerifyAllPodsRunningInNamespace verifies that all the pods in a given namespace are running.
func VerifyAllPodsRunningInNamespace(apiClient *clients.Settings, nsName string) {
	By(fmt.Sprintf("Verifying that pods exist in %s namespace", nsName))

	pods, err := pod.List(apiClient, nsName)
	Expect(err).NotTo(HaveOccurred(),
		fmt.Sprintf("Error while listing pods in %s namespace", nsName))
	Expect(len(pods) > 0).To(BeTrue(),
		fmt.Sprintf("Error: did not find any pods in the %s namespace", nsName))

	By(fmt.Sprintf("Verifying that pods are running correctly in %s namespace", nsName))

	running, err := pod.WaitForAllPodsInNamespaceRunning(apiClient, nsName, time.Minute)
	Expect(err).NotTo(HaveOccurred(),
		fmt.Sprintf("Error occurred while waiting for %s pods to be in Running state", nsName))
	Expect(running).To(BeTrue(),
		fmt.Sprintf("Some %s pods are not in Running state", nsName))

	klog.V(ocloudparams.OCloudLogLevel).Infof("all pods running in %s namespace", nsName)
}

// VerifyNamespaceExists verifies that a specific namespace exists.
func VerifyNamespaceExists(nsName string) *namespace.Builder {
	By(fmt.Sprintf("Verifying that %s namespace exists", nsName))

	namespace, err := namespace.Pull(HubAPIClient, nsName)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull namespace %q; %v", nsName, err)

	klog.V(ocloudparams.OCloudLogLevel).Infof("namespace %s exists", nsName)

	return namespace
}

// VerifyNamespaceDoesNotExist verifies that a given namespace does not exist.
func VerifyNamespaceDoesNotExist(namespace *namespace.Builder, waitGroup *sync.WaitGroup, ctx SpecContext) {
	defer waitGroup.Done()
	defer GinkgoRecover()

	nsName := namespace.Object.Name

	By(fmt.Sprintf("Verifying that namespace %s does not exist", nsName))

	Eventually(func(ctx context.Context) bool {
		return !namespace.Exists()
	}).WithTimeout(30*time.Minute).WithPolling(time.Second).WithContext(ctx).Should(BeTrue(),
		fmt.Sprintf("Namespace %s still exists", nsName))

	klog.V(ocloudparams.OCloudLogLevel).Infof("namespace %s does not exist", nsName)
}

// VerifyProvisionSnoCluster verifies the successful creation or provisioning request and
// that the provisioning request is progressing.
func VerifyProvisionSnoCluster(
	templateName string,
	templateVersion string,
	nodeClusterName string,
	oCloudSiteID string,
	policyTemplateParameters map[string]any,
	clusterInstanceParameters map[string]any) *oran.ProvisioningRequestBuilder {
	prName := uuid.New().String()

	By(fmt.Sprintf("Verifing the successful creation of the %s PR", prName))

	prBuilder := oran.NewPRBuilder(HubAPIClient, prName, templateName, templateVersion).
		WithTemplateParameter("nodeClusterName", nodeClusterName).
		WithTemplateParameter("oCloudSiteId", oCloudSiteID).
		WithTemplateParameter("policyTemplateParameters", policyTemplateParameters).
		WithTemplateParameter("clusterInstanceParameters", clusterInstanceParameters)
	provisioningReq, err := prBuilder.Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create PR %s", prName)

	condition := metav1.Condition{
		Type:   "ClusterInstanceProcessed",
		Reason: "Completed",
	}

	_, err = provisioningReq.WaitForCondition(condition, time.Minute*5)
	Expect(err).ToNot(HaveOccurred(), "PR %s is not getting processing", prName)

	klog.V(ocloudparams.OCloudLogLevel).Infof("provisioning request %s has been created", prName)

	return provisioningReq
}

// VerifyProvisioningRequestIsFulfilled verifies that a given provisioning request is fulfilled.
func VerifyProvisioningRequestIsFulfilled(provisioningReq *oran.ProvisioningRequestBuilder) {
	Expect(provisioningReq).ToNot(BeNil(), "provisioningReq should not be nil")
	Expect(provisioningReq.Object).ToNot(BeNil(), "provisioningReq.Object should not be nil")
	Expect(provisioningReq.Object.Name).ToNot(BeEmpty(), "provisioningReq.Object.Name should not be empty")

	By(fmt.Sprintf("Verifing that PR %s is fulfilled", provisioningReq.Object.Name))

	_, err := provisioningReq.WaitUntilFulfilled(time.Minute * 10)
	Expect(err).ToNot(HaveOccurred(), "PR %s is not fulfilled", provisioningReq.Object.Name)

	klog.V(ocloudparams.OCloudLogLevel).Infof("provisioningrequest %s is fulfilled", provisioningReq.Object.Name)
}

// VerifyProvisioningRequestTimeout verifies that a provisioning request has timed out.
func VerifyProvisioningRequestTimeout(provisioningReq *oran.ProvisioningRequestBuilder) {
	Expect(provisioningReq).ToNot(BeNil(), "provisioningReq should not be nil")
	Expect(provisioningReq.Object).ToNot(BeNil(), "provisioningReq.Object should not be nil")
	Expect(provisioningReq.Object.Name).ToNot(BeEmpty(), "provisioningReq.Object.Name should not be empty")

	By(fmt.Sprintf("Verifing that PR %s has timed out", provisioningReq.Object.Name))

	condition := metav1.Condition{
		Type:   "ConfigurationApplied",
		Reason: "TimedOut",
		Status: "False",
	}
	_, err := provisioningReq.WaitForCondition(condition, time.Minute*100)
	Expect(err).ToNot(HaveOccurred(), "PR %s failed to report timeout", provisioningReq.Object.Name)

	klog.V(ocloudparams.OCloudLogLevel).
		Infof("provisioningrequest %s has failed (timeout)", provisioningReq.Object.Name)
}

// VerifyProvisioningRequestIsDeleted verifies that a given provisioning request is deleted.
func VerifyProvisioningRequestIsDeleted(
	provisioningReq *oran.ProvisioningRequestBuilder,
	waitGroup *sync.WaitGroup,
	ctx SpecContext) {
	defer waitGroup.Done()
	defer GinkgoRecover()

	prName := provisioningReq.Object.Name
	err := provisioningReq.DeleteAndWait(30 * time.Minute)
	Expect(err).ToNot(HaveOccurred(), "Failed to delete PR %s: %v", prName, err)

	klog.V(ocloudparams.OCloudLogLevel).Infof("provisioningrequest %s deleted", prName)
}

// VerifyClusterInstanceCompleted verifies that a cluster instance exists, that it is provisioned and
// that it is associated to a given provisioning request.
func VerifyClusterInstanceCompleted(
	provisioningReq *oran.ProvisioningRequestBuilder, ctx SpecContext) *siteconfig.CIBuilder {
	name := provisioningReq.Object.Status.Extensions.ClusterDetails.Name
	nsname := provisioningReq.Object.Status.Extensions.ClusterDetails.Name
	prName := provisioningReq.Object.Name

	By(fmt.Sprintf("Verifying that %s PR has a Cluster Instance CR associated", provisioningReq.Object.Name))

	clusterInstance, err := siteconfig.PullClusterInstance(HubAPIClient, name, nsname)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull Cluster Instance %q; %v", name, err)

	found := false

	for _, value := range clusterInstance.Object.ObjectMeta.Labels {
		if value == prName {
			found = true

			break
		}
	}

	Expect(found).To(BeTrue(),
		fmt.Sprintf("Failed to verify that Cluster Instance %s is associated to PR %s", name, prName))

	condition := metav1.Condition{
		Type:   "Provisioned",
		Status: "True",
	}

	clusterInstance, err = clusterInstance.WaitForCondition(condition, 80*time.Minute)
	Expect(err).ToNot(HaveOccurred(), "Clusterinstance is not provisioned %s: %v", name, err)

	klog.V(ocloudparams.OCloudLogLevel).Infof("clusterinstance %s is completed", name)

	return clusterInstance
}

// VerifyClusterInstanceDoesNotExist verifies that a given cluster instance does not exist.
func VerifyClusterInstanceDoesNotExist(
	clusterInstance *siteconfig.CIBuilder, waitGroup *sync.WaitGroup, ctx SpecContext) {
	defer waitGroup.Done()
	defer GinkgoRecover()

	ciName := clusterInstance.Object.Name
	By(fmt.Sprintf("Verifying that clusterinstance %s does not exist", ciName))
	Eventually(func(ctx context.Context) bool {
		return !clusterInstance.Exists()
	}).WithTimeout(30*time.Minute).WithPolling(time.Second).WithContext(ctx).Should(BeTrue(),
		fmt.Sprintf("ClusterInstance %s still exists", ciName))

	klog.V(ocloudparams.OCloudLogLevel).Infof("clusterinstance %s does not exist", ciName)
}

// VerifyAllPoliciesInNamespaceAreCompliant verifies that all the policies in a given namespace
// report compliant.
func VerifyAllPoliciesInNamespaceAreCompliant(
	nsName string, ctx SpecContext, waitGroup *sync.WaitGroup, mutex *sync.Mutex) {
	if waitGroup != nil {
		defer waitGroup.Done()
		defer GinkgoRecover()
	}

	By(fmt.Sprintf("Verifying that all the policies in namespace %s are Compliant", nsName))

	Eventually(func(ctx context.Context) bool {
		if mutex != nil {
			mutex.Lock()
		}

		policies, err := ocm.ListPoliciesInAllNamespaces(
			HubAPIClient, runtimeclient.ListOptions{Namespace: nsName})
		Expect(err).ToNot(HaveOccurred(), "Failed to pull policies from namespaces %s: %v", nsName, err)

		if mutex != nil {
			mutex.Unlock()
		}

		for _, policy := range policies {
			if policy.Object.Status.ComplianceState != "Compliant" {
				return false
			}
		}

		return true
	}).WithTimeout(100*time.Minute).WithPolling(30*time.Second).WithContext(ctx).Should(BeTrue(),
		fmt.Sprintf("Failed to verify that all the policies in namespace %s are Compliant", nsName))

	klog.V(ocloudparams.OCloudLogLevel).Infof("all the policies in namespace %s are compliant", nsName)
}

// VerifyPoliciesAreNotCompliant verifies that not all the policies in a given namespace
// report compliant.
func VerifyPoliciesAreNotCompliant(
	nsName string,
	ctx SpecContext,
	waitGroup *sync.WaitGroup,
	mutex *sync.Mutex) {
	if waitGroup != nil {
		defer waitGroup.Done()
		defer GinkgoRecover()
	}

	By(fmt.Sprintf("Verifying that not all the policies in namespace %s are Compliant", nsName))

	Eventually(func(ctx context.Context) bool {
		if mutex != nil {
			mutex.Lock()
		}

		policies, err := ocm.ListPoliciesInAllNamespaces(
			HubAPIClient, runtimeclient.ListOptions{Namespace: nsName})
		Expect(err).ToNot(HaveOccurred(), "Failed to pull policies from namespace %s: %v", nsName, err)

		if mutex != nil {
			mutex.Unlock()
		}

		for _, policy := range policies {
			if policy.Object.Status.ComplianceState != "Compliant" {
				return true
			}
		}

		return false
	}).WithTimeout(30*time.Minute).WithPolling(3*time.Second).WithContext(ctx).Should(BeTrue(),
		fmt.Sprintf("Failed to verify that not all the policies in namespace %s are Compliant", nsName))

	klog.V(ocloudparams.OCloudLogLevel).Infof("all the policies in namespace %s are not compliant", nsName)
}

// VerifyAllocatedNodesExist verifies that AllocatedNodes associated with a NodeAllocationRequest exist.
func VerifyAllocatedNodesExist(nodeAllocationRequest *oran.NARBuilder) []*oran.AllocatedNodeBuilder {
	Expect(nodeAllocationRequest.Object).ToNot(BeNil(), "nodeAllocationRequest.Object should not be nil")
	Expect(nodeAllocationRequest.Object.Status.Properties.NodeNames).ToNot(BeNil(),
		"nodeAllocationRequest.Object.Status.Properties.NodeNames should not be nil")

	allocatedNodeNames := nodeAllocationRequest.Object.Status.Properties.NodeNames
	Expect(len(allocatedNodeNames) > 0).To(BeTrue(),
		fmt.Sprintf("Failed to verify that AllocatedNodes exist for NodeAllocationRequest %s",
			nodeAllocationRequest.Object.Name))

	allocatedNodes := make([]*oran.AllocatedNodeBuilder, 0)

	for _, allocatedNodeName := range allocatedNodeNames {
		By(fmt.Sprintf("Verifying that AllocatedNode %s exists", allocatedNodeName))
		allocatedNode, err := oran.PullAllocatedNode(HubAPIClient, allocatedNodeName, ocloudparams.OranO2ImsNamespace)
		Expect(err).ToNot(HaveOccurred(), "Failed to pull AllocatedNode %s: %v", allocatedNodeName, err)

		allocatedNodes = append(allocatedNodes, allocatedNode)

		klog.V(ocloudparams.OCloudLogLevel).Infof("allocated node %s exists", allocatedNodeName)
	}

	return allocatedNodes
}

// VerifyAllocatedNodesDoNotExist verifies that a given AllocatedNode does not exist.
func VerifyAllocatedNodesDoNotExist(
	allocatedNodes []*oran.AllocatedNodeBuilder, waitGroup *sync.WaitGroup, ctx SpecContext) {
	defer waitGroup.Done()
	defer GinkgoRecover()

	for _, allocatedNode := range allocatedNodes {
		Expect(allocatedNode).ToNot(BeNil(), "allocatedNode should not be nil")
		Expect(allocatedNode.Object).ToNot(BeNil(), "allocatedNode.Object should not be nil")
		Expect(allocatedNode.Object.Name).ToNot(BeEmpty(), "allocatedNode.Object.Name should not be empty")

		nodeName := allocatedNode.Object.Name

		By(fmt.Sprintf("Verifying that allocated node %s does not exist", nodeName))

		Eventually(func(ctx context.Context) bool {
			return !allocatedNode.Exists()
		}).WithTimeout(30*time.Minute).WithPolling(time.Second).WithContext(ctx).Should(BeTrue(),
			fmt.Sprintf("Allocated node %s still exists", nodeName))

		klog.V(ocloudparams.OCloudLogLevel).Infof("allocated node %s does not exists", nodeName)
	}
}

// VerifyNodeAllocationRequestExists verifies that a given NodeAllocationRequest exists.
func VerifyNodeAllocationRequestExists(name string) *oran.NARBuilder {
	By(fmt.Sprintf("Verifying that NodeAllocationRequest %s exists", name))

	nodeAllocationRequest, err := oran.PullNodeAllocationRequest(
		HubAPIClient, name, ocloudparams.OranO2ImsNamespace)
	Expect(err).ToNot(HaveOccurred(),
		"Failed to pull node allocation request %s: %v", name, err)

	klog.V(ocloudparams.OCloudLogLevel).Infof("node allocation request %s exists", name)

	return nodeAllocationRequest
}

// VerifyNodeAllocationRequestDoesNotExist verifies that a given NodeAllocationRequest does not exist.
func VerifyNodeAllocationRequestDoesNotExist(
	nodeAllocationRequest *oran.NARBuilder, waitGroup *sync.WaitGroup, ctx SpecContext) {
	defer waitGroup.Done()
	defer GinkgoRecover()

	Expect(nodeAllocationRequest).ToNot(BeNil(), "nodeAllocationRequest should not be nil")
	Expect(nodeAllocationRequest.Object).ToNot(BeNil(), "nodeAllocationRequest.Object should not be nil")
	Expect(nodeAllocationRequest.Object.Name).ToNot(BeEmpty(), "nodeAllocationRequest.Object.Name should not be empty")

	name := nodeAllocationRequest.Object.Name

	By(fmt.Sprintf("Verifying that NodeAllocationRequest %s does not exist", name))

	Eventually(func(ctx context.Context) bool {
		return !nodeAllocationRequest.Exists()
	}).WithTimeout(30*time.Minute).WithPolling(time.Second).WithContext(ctx).Should(BeTrue(),
		fmt.Sprintf("NodeAllocationRequest %s still exists", name))

	klog.V(ocloudparams.OCloudLogLevel).Infof("node allocation request %s does not exist", name)
}

// CreateSnoAPIClient creates a new client for the given node.
func CreateSnoAPIClient(nodeName string) *clients.Settings {
	path := fmt.Sprintf("tmp/%s/auth", nodeName)
	err := os.MkdirAll(path, 0750)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Error creating directory %s", path))

	createSnoKubeconfig := fmt.Sprintf(ocloudparams.SnoKubeconfigCreate, nodeName, nodeName, nodeName)
	_, err = shell.ExecuteCmd(createSnoKubeconfig)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Error creating %s kubeconfig", nodeName))

	snoKubeconfigPath := fmt.Sprintf("tmp/%s/auth/kubeconfig", nodeName)
	snoAPIClient := clients.New(snoKubeconfigPath)

	return snoAPIClient
}

// VerifyOcloudCRsExist verifies that a given ProvisioningRequest has generated a Namespace,
// a NodeAllocationRequest and a AllocatedNodes.
func VerifyOcloudCRsExist(provisioningReq *oran.ProvisioningRequestBuilder) (
	*oran.NARBuilder, []*oran.AllocatedNodeBuilder, *namespace.Builder) {
	Expect(provisioningReq.Object).ToNot(BeNil(), "provisioningReq.Object should not be nil")
	Expect(provisioningReq.Object.Status.Extensions.ClusterDetails).ToNot(BeNil(),
		"provisioningReq.Object.Status.Extensions.ClusterDetails should not be nil")
	Expect(provisioningReq.Object.Status.Extensions.NodeAllocationRequestRef).ToNot(BeNil(),
		"provisioningReq.Object.Status.Extensions.NodeAllocationRequestRef should not be nil")

	nodeAllocationRequest := VerifyNodeAllocationRequestExists(
		provisioningReq.Object.Status.Extensions.NodeAllocationRequestRef.NodeAllocationRequestID)
	allocatedNodes := VerifyAllocatedNodesExist(nodeAllocationRequest)
	namespace := VerifyNamespaceExists(provisioningReq.Object.Status.Extensions.ClusterDetails.Name)

	return nodeAllocationRequest, allocatedNodes, namespace
}

// DeprovisionAiSnoCluster deprovisions a SNO cluster.
func DeprovisionAiSnoCluster(
	provisioningReq *oran.ProvisioningRequestBuilder,
	clusterInstance *siteconfig.CIBuilder,
	ctx SpecContext,
	waitGroup *sync.WaitGroup) {
	if waitGroup != nil {
		defer waitGroup.Done()
		defer GinkgoRecover()
	}

	Expect(provisioningReq).ToNot(BeNil(), "provisioningReq should not be nil")
	Expect(provisioningReq.Object).ToNot(BeNil(), "provisioningReq.Object should not be nil")
	Expect(provisioningReq.Object.Name).ToNot(BeEmpty(), "provisioningReq.Object.Name should not be empty")

	prName := provisioningReq.Object.Name
	By(fmt.Sprintf("Tearing down PR %s", prName))

	nodeAllocationRequest, allocatedNodes, namespace := VerifyOcloudCRsExist(provisioningReq)
	bmhs := GetBMHsFromAllocatedNodes(allocatedNodes)

	var tearDownWg sync.WaitGroup

	tearDownWg.Add(5)

	go VerifyProvisioningRequestIsDeleted(provisioningReq, &tearDownWg, ctx)
	go VerifyNamespaceDoesNotExist(namespace, &tearDownWg, ctx)
	go VerifyClusterInstanceDoesNotExist(clusterInstance, &tearDownWg, ctx)
	go VerifyAllocatedNodesDoNotExist(allocatedNodes, &tearDownWg, ctx)
	go VerifyNodeAllocationRequestDoesNotExist(nodeAllocationRequest, &tearDownWg, ctx)

	tearDownWg.Wait()

	klog.V(ocloudparams.OCloudLogLevel).Infof("Provisioning request %s has been removed", prName)

	// Verify the BMHs are available after the SNO cluster is deprovisioned.
	for _, bmh := range bmhs {
		VerifyBmhIsAvailable(bmh, OCloudConfig.InventoryPoolNamespace)
	}
}

// VerifyBmhIsAvailable verifies that a given BMH is available.
func VerifyBmhIsAvailable(hostName string, nsName string) {
	bareMetalHost, err := bmh.Pull(HubAPIClient, hostName, nsName)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull BMH %s from namespace %s: %v", hostName, nsName, err)

	err = bareMetalHost.WaitUntilInStatus(bmhv1alpha1.StateAvailable, ocloudparams.BMHAvailabilityTimeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to verify that BMH %s is available", hostName)
}

// GetBMHsFromAllocatedNodes returns the BMHs from a given AllocatedNodes.
func GetBMHsFromAllocatedNodes(allocatedNodes []*oran.AllocatedNodeBuilder) []string {
	Expect(allocatedNodes).ToNot(BeNil(), "allocatedNodes should not be nil")

	bmhs := make([]string, 0)

	for _, allocatedNode := range allocatedNodes {
		bmh := allocatedNode.Object.Spec.HwMgrNodeId
		bmhs = append(bmhs, bmh)
	}

	return bmhs
}

// GetProvisioningRequestName returns the name of the ProvisioningRequest for a given ClusterInstance.
func GetProvisioningRequestName(clusterID string) string {
	clusterInstance, err := siteconfig.PullClusterInstance(HubAPIClient, clusterID, clusterID)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull Cluster Instance %q; %v", clusterID, err)

	Expect(clusterInstance.Object).ToNot(BeNil(), "clusterInstance.Object should not be nil")
	Expect(clusterInstance.Object.ObjectMeta.Labels).ToNot(BeNil(),
		"clusterInstance.Object.ObjectMeta.Labels should not be nil")

	labels := clusterInstance.Object.ObjectMeta.Labels
	provisioningRequestName, exists := labels["provisioningrequest.clcm.openshift.io/name"]

	Expect(exists).To(BeTrue(), "provisioning request name is missing from cluster instance labels")

	return provisioningRequestName
}
