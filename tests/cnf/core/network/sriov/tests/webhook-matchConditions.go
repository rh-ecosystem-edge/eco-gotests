package tests

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/daemonset"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/webhook"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netenv"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/sriovenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	admv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

var _ = Describe("webhook-resource-injector", Ordered, Label(tsparams.LabelWebhookInjector),
	ContinueOnFailure, func() {
		var workerNodeList []*nodes.Builder

		BeforeAll(func() {
			By("Verifying if tests can be executed on given cluster")
			err := netenv.DoesClusterHasEnoughNodes(APIClient, NetConfig, 1, 1)
			Expect(err).ToNot(HaveOccurred(),
				"Cluster doesn't support webhook-resource-injector test cases as it doesn't have enough nodes")

			By("Validating SR-IOV interfaces")
			workerNodeList, err = nodes.List(APIClient,
				metav1.ListOptions{LabelSelector: labels.Set(NetConfig.WorkerLabelMap).String()})
			Expect(err).ToNot(HaveOccurred(), "Failed to discover worker nodes")

			Expect(sriovenv.ValidateSriovInterfaces(workerNodeList, 2)).ToNot(HaveOccurred(),
				"Failed to get required SR-IOV interfaces")

			sriovInterfacesUnderTest, err := NetConfig.GetSriovInterfaces(2)
			Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for testing")

			By("Creating SriovNetworkNodePolicy and SriovNetwork")

			err = sriovenv.CreateSriovPolicyAndWaitUntilItsApplied(definePolicy(
				"client", "netdevice", "", sriovInterfacesUnderTest[0], 0), tsparams.MCOWaitTimeout)
			Expect(err).ToNot(HaveOccurred(), "Failed to create SriovNetworkNodePolicy")

			err = sriovenv.CreateSriovNetworkAndWaitForNADCreation(defineNetwork("client", "netdevice"), 5*time.Second)
			Expect(err).ToNot(HaveOccurred(), "Failed to create SriovNetwork")
		})

		AfterAll(func() {
			By("Removing SR-IOV configuration")
			err := netenv.RemoveSriovConfigurationAndWaitForSriovAndMCPStable()
			Expect(err).ToNot(HaveOccurred(), "Failed to remove SR-IOV configuration")

			By("Cleaning test namespace")
			err = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
				netparam.DefaultTimeout, pod.GetGVR())
			Expect(err).ToNot(HaveOccurred(), "Failed to clean test namespace")

			By("Removing feature Gate resourceInjectorMatchCondition")

			sriovConfig, err := sriov.PullOperatorConfig(APIClient, NetConfig.SriovOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to pull SriovOperatorConfig")

			delete(sriovConfig.Definition.Spec.FeatureGates, "resourceInjectorMatchCondition")
			_, err = sriovConfig.Update()
			Expect(err).ToNot(HaveOccurred(), "Failed to update SriovOperatorConfig")
		})

		AfterEach(func() {
			By("Delete network-resources-injector daemonset")
			deleteDaemonSetAndWaitForNewDaemonSet(netparam.OperatorResourceInjector, NetConfig.SriovOperatorNamespace)
		})

		It("resourceInjectorMatchCondition set to True", reportxml.ID("80110"), func() {
			runInjectorTests(true, workerNodeList[0].Object.Name)
		})

		It("resourceInjectorMatchCondition set to False", reportxml.ID("80109"), func() {
			runInjectorTests(false, workerNodeList[0].Object.Name)
		})
	})

func setResourceInjectorMatchCondition(flag bool) {
	defaultOperatorConfig, err := sriov.PullOperatorConfig(APIClient, NetConfig.SriovOperatorNamespace)
	Expect(err).ToNot(HaveOccurred(), "Failed to fetch default Sriov Operator Config")

	if defaultOperatorConfig.Definition.Spec.FeatureGates == nil {
		defaultOperatorConfig.Definition.Spec.FeatureGates = map[string]bool{"resourceInjectorMatchCondition": flag}
	} else {
		defaultOperatorConfig.Definition.Spec.FeatureGates["resourceInjectorMatchCondition"] = flag
	}

	_, err = defaultOperatorConfig.Update()
	Expect(err).ToNot(HaveOccurred(),
		"Failed to update resourceInjectorMatchCondition flag in default Sriov Operator Config")
}

func fetchPIDOfContainer(pod *pod.Builder, containerName string) string {
	var containerID string

	Expect(len(pod.Object.Status.ContainerStatuses)).To(BeNumerically(">", 0), "Container Status field is empty")

	for _, containerStatus := range pod.Object.Status.ContainerStatuses {
		if containerStatus.Name == containerName {
			Expect(containerStatus.ContainerID).NotTo(BeEmpty(), "Container ID is empty")
			containerID = strings.TrimPrefix(containerStatus.ContainerID, "cri-o://")
		}
	}

	Expect(containerID).To(Not(BeEmpty()), "Container ID should not be empty")

	output, err := cluster.ExecCmdWithStdout(APIClient, fmt.Sprintf("crictl inspect %s | jq .info.pid", containerID),
		metav1.ListOptions{LabelSelector: fmt.Sprintf("kubernetes.io/hostname=%s", pod.Object.Spec.NodeName)})
	Expect(err).ToNot(HaveOccurred(), "Failed to fetch container PID")
	Expect(output[pod.Object.Spec.NodeName]).ToNot(BeEmpty(), "Output should not be empty")

	// raw output contains ANSI code and \n apart from the PID number.
	// Using regex to remove ANSI code and replace \n with empty.
	result := strings.Replace(
		regexp.MustCompile(`\x1B\[[0-9;]*[mK]`).ReplaceAllString(output[pod.Object.Spec.NodeName], ""),
		"\n", "", 1)

	// check if the resulted PID can be converted to int in order to make sure it has only numerics.
	_, err = strconv.Atoi(result)
	Expect(err).ToNot(HaveOccurred(), "Failed to parse PID as integer")

	return result
}

func runInjectorTests(matchCondition bool, workerNode string) {
	nftCommands := []string{"nft add table inet custom_table",
		"nft add chain inet custom_table custom_chain_INPUT { type filter hook input priority 1 \\; policy accept \\; }",
		"nft add chain inet custom_table custom_chain_OUTPUT { type filter hook output priority 1 \\; policy accept \\; }",
		"nft add rule inet custom_table custom_chain_INPUT tcp dport 6443 log drop",
		"nft add rule inet custom_table custom_chain_OUTPUT tcp dport 6443 log drop"}

	By(fmt.Sprintf("Setting resourceInjectorMatchCondition=%t in SriovOperatorConfig/default ", matchCondition))
	setResourceInjectorMatchCondition(matchCondition)

	By("Fetching MutatingWebhookConfigurations/network-resources-injector-config and " +
		"Verify FailurePolicy and MatchConditions")

	Eventually(func() error {
		resourceInjectorWebhook, err := webhook.PullMutatingConfiguration(APIClient, "network-resources-injector-config")
		if err != nil {
			return err
		}

		for _, injectorWebhook := range resourceInjectorWebhook.Object.Webhooks {
			if injectorWebhook.Name == "network-resources-injector-config.k8s.io" {
				if matchCondition {
					if *injectorWebhook.FailurePolicy == admv1.Fail && len(injectorWebhook.MatchConditions) > 0 {
						return nil
					}
				} else {
					if *injectorWebhook.FailurePolicy == admv1.Ignore && len(injectorWebhook.MatchConditions) == 0 {
						return nil
					}
				}
			}
		}

		return errors.New("network-resources-injector-config.k8s.io webhook not found")
	}, time.Minute, 2*time.Second).Should(BeNil(), "MutatingWebhookConfiguration validation failed")

	By("Creating Sriov Pod and Delete it after it is in Running state")

	sriovPod, err := sriovenv.DefinePod("sriovpod", "client", "clientnetdevice", workerNode, true).
		CreateAndWaitUntilRunning(1 * time.Minute)
	Expect(err).ToNot(HaveOccurred(), "Failed to create Sriov Pod")

	_, err = sriovPod.DeleteAndWait(1 * time.Minute)
	Expect(err).ToNot(HaveOccurred(), "Failed to delete Sriov Pod")

	By("Blocking incoming requests to network-resources-injector pods")

	injectorPods, err := pod.List(APIClient, NetConfig.SriovOperatorNamespace,
		metav1.ListOptions{LabelSelector: "app=network-resources-injector"})
	Expect(err).ToNot(HaveOccurred(), "Failed to list network-resources-injector pods")

	for _, injectorPod := range injectorPods {
		containerPID := fetchPIDOfContainer(injectorPod, "webhook-server")
		for _, nftCommand := range nftCommands {
			nsenterCmd := fmt.Sprintf("nsenter -t %s -n ", containerPID)
			_, err := cluster.ExecCmdWithStdout(APIClient, nsenterCmd+nftCommand,
				metav1.ListOptions{LabelSelector: fmt.Sprintf("kubernetes.io/hostname=%s", injectorPod.Object.Spec.NodeName)})
			Expect(err).ToNot(HaveOccurred(), "Failed to execute nsenter command")
		}
	}

	By("Verifying that sriov pod creation fails")

	sriovPod, err = sriovenv.DefinePod("sriovpod", "client", "clientnetdevice", workerNode, true).
		CreateAndWaitUntilRunning(1 * time.Minute)
	Expect(err).To(HaveOccurred(), "Expected Sriov Pod creation to fail")

	if matchCondition {
		// Pod Creation is rejected by API with "failed calling webhook" error.
		Expect(err.Error()).To(ContainSubstring("failed calling webhook"), "Error should be: failed calling webhook")
	} else {
		// Pod Creation is accepted but the Pod will be stuck in ContainerCreating state. Kube API does not return an error.
		// In such case CreateAndWaitUntilRunning returns context deadline exceed error.
		Expect(err.Error()).ToNot(ContainSubstring("failed calling webhook"), "Error should not be: failed calling webhook")
	}

	_, err = sriovPod.DeleteAndWait(1 * time.Minute)
	Expect(err).ToNot(HaveOccurred(), "Failed to delete Sriov Pod")

	By("Verifying that non-sriov pod creation succeeds")

	nonSriovPod, err := sriovenv.DefinePod("nonsriovpod", "client", "clientnetdevice", workerNode, false).
		CreateAndWaitUntilRunning(1 * time.Minute)
	Expect(err).ToNot(HaveOccurred(), "Failed to create non-sriov Pod")

	_, err = nonSriovPod.DeleteAndWait(1 * time.Minute)
	Expect(err).ToNot(HaveOccurred(), "Failed to delete non-sriov Pod")
}

func deleteDaemonSetAndWaitForNewDaemonSet(dsName, namespace string) {
	By(fmt.Sprintf("Deleting DaemonSet %s in namespace %s", dsName, namespace))
	pulledDs, err := daemonset.Pull(APIClient, dsName, namespace)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull daemonset")

	err = pulledDs.Delete()
	Expect(err).ToNot(HaveOccurred(), "Failed to delete daemonset")

	Eventually(func() error {
		pulledDs, err = daemonset.Pull(APIClient, dsName, namespace)
		if err != nil {
			return err
		}

		ready := pulledDs.IsReady(5 * time.Second)
		if !ready {
			return errors.New("DaemonSet not yet ready")
		}

		return nil
	}, 60*time.Second, 1*time.Second).Should(BeNil(), fmt.Sprintf("DaemonSet %s is not yet ready", dsName))
}
