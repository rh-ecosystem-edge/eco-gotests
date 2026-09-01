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
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/sriovoperator"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/ocpsriovinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/sriovenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/sriovocpenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/tsparams"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
	admv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

var _ = Describe("webhook-resource-injector", Ordered, Label(tsparams.LabelWebhookInjector),
	ContinueOnFailure, func() {
		var workerNodeList []*nodes.Builder

		BeforeAll(func() {
			By("Verifying if tests can be executed on given cluster")

			err := sriovocpenv.DoesClusterHaveEnoughNodes(1, 1)
			if err != nil {
				Skip(fmt.Sprintf("Skipping test - cluster doesn't have enough nodes: %v", err))
			}

			By("Validating SR-IOV interfaces")

			workerNodeList, err = nodes.List(APIClient,
				metav1.ListOptions{LabelSelector: labels.Set(SriovOcpConfig.WorkerLabelMap).String()})
			Expect(err).ToNot(HaveOccurred(), "Failed to discover worker nodes")

			Expect(sriovocpenv.ValidateSriovInterfaces(workerNodeList, 2)).ToNot(HaveOccurred(),
				"Failed to get required SR-IOV interfaces")

			sriovInterfacesUnderTest, err := SriovOcpConfig.GetSriovInterfaces(2)
			Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for testing")

			vfNum, err := SriovOcpConfig.GetVFNum()
			Expect(err).ToNot(HaveOccurred(), "Failed to get VF number")

			By("Creating SriovNetworkNodePolicy and SriovNetwork")

			err = sriovoperator.CreateSriovPolicyAndWaitUntilItsApplied(
				APIClient,
				SriovOcpConfig.WorkerLabelEnvVar,
				SriovOcpConfig.OcpSriovOperatorNamespace,
				defineWebhookPolicy(
					"client", "netdevice", sriovInterfacesUnderTest[0], vfNum, 0),
				tsparams.MCOWaitTimeout,
				tsparams.DefaultStableDuration)
			Expect(err).ToNot(HaveOccurred(), "Failed to create SriovNetworkNodePolicy")

			sriovNetworkBuilder := defineWebhookNetwork("client", "netdevice")
			_, err = sriovNetworkBuilder.Create()
			Expect(err).ToNot(HaveOccurred(),
				"Failed to create Sriov Network %s with error %v",
				sriovNetworkBuilder.Definition.Name, err)

			err = sriovenv.WaitForNADCreation(
				sriovNetworkBuilder.Definition.Name, tsparams.TestNamespaceName, tsparams.NADWaitTimeout)
			Expect(err).ToNot(HaveOccurred(),
				"Failed to create and wait for NAD creation for Sriov Network %s with error %v",
				sriovNetworkBuilder.Definition.Name, err)
		})

		AfterAll(func() {
			By("Removing SR-IOV configuration")

			err := sriovoperator.RemoveSriovConfigurationAndWaitForSriovAndMCPStable(
				APIClient,
				SriovOcpConfig.WorkerLabelEnvVar,
				SriovOcpConfig.OcpSriovOperatorNamespace,
				tsparams.MCOWaitTimeout,
				tsparams.DefaultTimeout)
			Expect(err).ToNot(HaveOccurred(), "Failed to remove SR-IOV configuration")

			By("Cleaning test namespace")

			err = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
				tsparams.DefaultTimeout, pod.GetGVR())
			Expect(err).ToNot(HaveOccurred(), "Failed to clean test namespace")

			By("Removing feature Gate resourceInjectorMatchCondition")

			sriovConfig, err := sriov.PullOperatorConfig(APIClient, SriovOcpConfig.OcpSriovOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to pull SriovOperatorConfig")

			delete(sriovConfig.Definition.Spec.FeatureGates, "resourceInjectorMatchCondition")
			_, err = sriovConfig.Update()
			Expect(err).ToNot(HaveOccurred(), "Failed to update SriovOperatorConfig")
		})

		AfterEach(func() {
			By("Delete network-resources-injector daemonset")
			deleteDaemonSetAndWaitForNewDaemonSet(
				tsparams.OperatorResourceInjector, SriovOcpConfig.OcpSriovOperatorNamespace)
		})

		It("resourceInjectorMatchCondition set to True", reportxml.ID("80110"), func() {
			runInjectorTests(true, workerNodeList[0].Object.Name)
		})

		It("resourceInjectorMatchCondition set to False", reportxml.ID("80109"), func() {
			runInjectorTests(false, workerNodeList[0].Object.Name)
		})
	})

func setResourceInjectorMatchCondition(flag bool) {
	defaultOperatorConfig, err := sriov.PullOperatorConfig(APIClient, SriovOcpConfig.OcpSriovOperatorNamespace)
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

func fetchPIDOfContainer(testPod *pod.Builder, containerName string) string {
	var containerID string

	Expect(len(testPod.Object.Status.ContainerStatuses)).To(BeNumerically(">", 0), "Container Status field is empty")

	for _, containerStatus := range testPod.Object.Status.ContainerStatuses {
		if containerStatus.Name == containerName {
			Expect(containerStatus.ContainerID).NotTo(BeEmpty(), "Container ID is empty")
			containerID = strings.TrimPrefix(containerStatus.ContainerID, "cri-o://")
		}
	}

	Expect(containerID).To(Not(BeEmpty()), "Container ID should not be empty")

	output, err := cluster.ExecCmdWithStdout(APIClient, fmt.Sprintf("crictl inspect %s | jq .info.pid", containerID),
		metav1.ListOptions{LabelSelector: fmt.Sprintf("kubernetes.io/hostname=%s", testPod.Object.Spec.NodeName)})
	Expect(err).ToNot(HaveOccurred(), "Failed to fetch container PID")
	Expect(output[testPod.Object.Spec.NodeName]).ToNot(BeEmpty(), "Output should not be empty")

	// raw output contains ANSI code and \n apart from the PID number.
	// Using regex to remove ANSI code and replace \n with empty.
	result := strings.Replace(
		regexp.MustCompile(`\x1B\[[0-9;]*[mK]`).ReplaceAllString(output[testPod.Object.Spec.NodeName], ""),
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

	sriovPod, err := defineWebhookPod("sriovpod", "client", "clientnetdevice", workerNode, true).
		CreateAndWaitUntilRunning(1 * time.Minute)
	Expect(err).ToNot(HaveOccurred(), "Failed to create Sriov Pod")

	_, err = sriovPod.DeleteAndWait(1 * time.Minute)
	Expect(err).ToNot(HaveOccurred(), "Failed to delete Sriov Pod")

	By("Blocking incoming requests to network-resources-injector pods")

	injectorPods, err := pod.List(APIClient, SriovOcpConfig.OcpSriovOperatorNamespace,
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

	sriovPod, err = defineWebhookPod("sriovpod", "client", "clientnetdevice", workerNode, true).
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

	nonSriovPod, err := defineWebhookPod("nonsriovpod", "client", "clientnetdevice", workerNode, false).
		CreateAndWaitUntilRunning(1 * time.Minute)
	Expect(err).ToNot(HaveOccurred(), "Failed to create non-sriov Pod")

	_, err = nonSriovPod.DeleteAndWait(1 * time.Minute)
	Expect(err).ToNot(HaveOccurred(), "Failed to delete non-sriov Pod")
}

func deleteDaemonSetAndWaitForNewDaemonSet(dsName, nsName string) {
	By(fmt.Sprintf("Deleting DaemonSet %s in namespace %s", dsName, nsName))
	pulledDs, err := daemonset.Pull(APIClient, dsName, nsName)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull daemonset")

	err = pulledDs.Delete()
	Expect(err).ToNot(HaveOccurred(), "Failed to delete daemonset")

	Eventually(func() error {
		pulledDs, err = daemonset.Pull(APIClient, dsName, nsName)
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

func defineWebhookPolicy(role, devType, pfName string, numVfs, vfRange int) *sriov.PolicyBuilder {
	return sriov.NewPolicyBuilder(APIClient,
		role+devType, SriovOcpConfig.OcpSriovOperatorNamespace, role+devType, numVfs, []string{pfName},
		SriovOcpConfig.WorkerLabelMap).
		WithDevType("netdevice").
		WithVFRange(vfRange, vfRange)
}

func defineWebhookNetwork(role, devType string) *sriov.NetworkBuilder {
	return sriov.NewNetworkBuilder(
		APIClient, role+devType, SriovOcpConfig.OcpSriovOperatorNamespace, tsparams.TestNamespaceName, role+devType).
		WithMacAddressSupport().
		WithIPAddressSupport().
		WithStaticIpam().
		WithLogLevel("debug")
}

func defineWebhookPod(name, role, ifName, worker string, secondaryInterface bool) *pod.Builder {
	podbuild := pod.NewBuilder(APIClient, name, tsparams.TestNamespaceName, SriovOcpConfig.OcpSriovTestContainer).
		WithNodeSelector(map[string]string{corev1.LabelHostname: worker}).
		WithPrivilegedFlag()

	if secondaryInterface {
		var netAnnotation []*types.NetworkSelectionElement

		if role == "server" {
			netAnnotation = []*types.NetworkSelectionElement{
				{
					Name:       ifName,
					MacRequest: tsparams.ServerMacAddress,
					IPRequest:  []string{tsparams.ServerIPv4IPAddress},
				},
			}
		} else {
			netAnnotation = []*types.NetworkSelectionElement{
				{
					Name:       ifName,
					MacRequest: tsparams.ClientMacAddress,
					IPRequest:  []string{tsparams.ClientIPv4IPAddress},
				},
			}
		}

		podbuild.WithSecondaryNetwork(netAnnotation)
	}

	return podbuild
}
