package tests

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/daemonset"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/webhook"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/sriovoperator"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/ocpsriovinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/sriovenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/sriovocpenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/tsparams"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
)

const (
	sharedVFResourceName         = "sharedvfs"
	sharedVFPolicyName           = "sharedvfs"
	sharedVFNetwork1Name         = "sharednet1"
	sharedVFNetwork2Name         = "sharednet2"
	sharedVFContainer1Name       = "ctn1"
	sharedVFContainer2Name       = "ctn2"
	sharedVFInterface1Name       = "net1"
	sharedVFInterface2Name       = "net2"
	sharedVFPodName              = "sharedvf-pod"
	networkResourcesInjectorDS   = "network-resources-injector"
	networkResourcesInjectorHook = "network-resources-injector-config"
	// vfPairingAttempts is the number of create/delete cycles used to catch an intermittent VF pairing mismatch.
	vfPairingAttempts = 10
	// minSharedVFPoolSize requires a pool larger than the two VFs consumed by the test pod.
	minSharedVFPoolSize      = 3
	injectorReconcileTimeout = 2 * time.Minute
)

var _ = Describe("Multi-container shared SR-IOV resourceName", Ordered,
	Label(tsparams.LabelMultiContainerTestCases, tsparams.LabelSriovHWEnabled),
	ContinueOnFailure, func() {
		var (
			workerNodeList           []*nodes.Builder
			sriovInterfacesUnderTest []string
			vfNum                    int
		)

		BeforeAll(func() {
			By("Verifying if multi-container SR-IOV tests can be executed on given cluster")

			err := sriovocpenv.DoesClusterHaveEnoughNodes(1, 1)
			if err != nil {
				Skip(fmt.Sprintf("Skipping test - cluster doesn't have enough nodes: %v", err))
			}

			vfNum, err = SriovOcpConfig.GetVFNum()
			Expect(err).ToNot(HaveOccurred(), "Failed to get VF number")

			if vfNum < minSharedVFPoolSize {
				Skip(fmt.Sprintf(
					"Skipping test - need at least %d VFs in the shared pool (more than the two VFs the pod uses), "+
						"got %d (ECO_OCP_SRIOV_VF_NUM)",
					minSharedVFPoolSize, vfNum))
			}

			klog.V(90).Infof(
				"Shared VF pool size is %d; a larger pool makes a pairing mismatch more likely", vfNum)

			By("Validating SR-IOV interfaces")

			workerNodeList, err = nodes.List(APIClient,
				metav1.ListOptions{LabelSelector: labels.Set(SriovOcpConfig.WorkerLabelMap).String()})
			Expect(err).ToNot(HaveOccurred(), "Failed to discover worker nodes")

			Expect(sriovocpenv.ValidateSriovInterfaces(workerNodeList, 1)).ToNot(HaveOccurred(),
				"Failed to get required SR-IOV interfaces")

			sriovInterfacesUnderTest, err = SriovOcpConfig.GetSriovInterfaces(1)
			Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for testing")

			By("Disabling the network-resources-injector so each container can request one VF")

			disableNetworkResourcesInjectorIfEnabled()

			By("Creating SriovNetworkNodePolicy with a shared VF pool")

			sriovPolicy := sriov.NewPolicyBuilder(
				APIClient,
				sharedVFPolicyName,
				SriovOcpConfig.OcpSriovOperatorNamespace,
				sharedVFResourceName,
				vfNum,
				[]string{sriovInterfacesUnderTest[0]},
				SriovOcpConfig.WorkerLabelMap).
				WithDevType("netdevice")

			err = sriovoperator.CreateSriovPolicyAndWaitUntilItsApplied(
				APIClient,
				SriovOcpConfig.WorkerLabelEnvVar,
				SriovOcpConfig.OcpSriovOperatorNamespace,
				sriovPolicy,
				tsparams.MCOWaitTimeout,
				tsparams.DefaultStableDuration)
			Expect(err).ToNot(HaveOccurred(), "Failed to configure SR-IOV policy %q", sharedVFPolicyName)

			By("Creating two SriovNetworks backed by the same resourceName")

			createSharedResourceSriovNetwork(sharedVFNetwork1Name)
			createSharedResourceSriovNetwork(sharedVFNetwork2Name)

			By("Verifying both NADs advertise the same resourceName")

			verifyNADSharesResourceName(sharedVFNetwork1Name)
			verifyNADSharesResourceName(sharedVFNetwork2Name)
		})

		AfterEach(func() {
			By("Cleaning test namespace")

			err := namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
				tsparams.DefaultTimeout, pod.GetGVR())
			Expect(err).ToNot(HaveOccurred(), "Failed to clean test namespace")
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
		})

		It("keeps device-plugin PCI and Multus VF pairing across pod restarts", func() {
			nodeName := workerNodeList[0].Object.Name

			for attempt := 1; attempt <= vfPairingAttempts; attempt++ {
				By(fmt.Sprintf("Attempt %d/%d: creating multi-container pod on node %s",
					attempt, vfPairingAttempts, nodeName))

				testPod := createMultiContainerSharedVFPod(nodeName)

				By(fmt.Sprintf("Attempt %d/%d: verifying each container's PCIDEVICE matches Multus network-status",
					attempt, vfPairingAttempts))

				verifySharedVFPairing(testPod, attempt)

				By(fmt.Sprintf("Attempt %d/%d: deleting pod to re-roll VF assignment",
					attempt, vfPairingAttempts))

				_, err := testPod.DeleteAndWait(tsparams.CleanupTimeout)
				Expect(err).ToNot(HaveOccurred(), "Failed to delete pod %q on attempt %d",
					sharedVFPodName, attempt)
			}
		})
	})

func createSharedResourceSriovNetwork(networkName string) {
	_, err := sriov.NewNetworkBuilder(
		APIClient,
		networkName,
		SriovOcpConfig.OcpSriovOperatorNamespace,
		tsparams.TestNamespaceName,
		sharedVFResourceName).
		WithStaticIpam().
		WithMacAddressSupport().
		WithIPAddressSupport().
		WithLogLevel("debug").
		Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create SriovNetwork %q", networkName)

	err = sriovenv.WaitForNADCreation(networkName, tsparams.TestNamespaceName, tsparams.NADTimeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to wait for NAD %q", networkName)
}

func verifyNADSharesResourceName(networkName string) {
	nadBuilder, err := nad.Pull(APIClient, networkName, tsparams.TestNamespaceName)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull NAD %q", networkName)

	expected := fmt.Sprintf("openshift.io/%s", sharedVFResourceName)
	Expect(nadBuilder.Object.Annotations).To(HaveKeyWithValue(resourceNameAnnotationOfNAD, expected),
		"NAD %q should advertise shared resourceName %q", networkName, expected)
}

func createMultiContainerSharedVFPod(nodeName string) *pod.Builder {
	sriovResource := corev1.ResourceList{
		corev1.ResourceName("openshift.io/" + sharedVFResourceName): resource.MustParse("1"),
	}

	container1 := defineSharedVFContainer(sharedVFContainer1Name, sriovResource)
	container2 := defineSharedVFContainer(sharedVFContainer2Name, sriovResource)

	netAnnotations := []*types.NetworkSelectionElement{
		{
			Name:             sharedVFNetwork1Name,
			InterfaceRequest: sharedVFInterface1Name,
			IPRequest:        []string{tsparams.ClientIPv4IPAddress},
		},
		{
			Name:             sharedVFNetwork2Name,
			InterfaceRequest: sharedVFInterface2Name,
			IPRequest:        []string{tsparams.ClientIPv4IPAddress2},
		},
	}

	testPod, err := pod.NewBuilder(
		APIClient, sharedVFPodName, tsparams.TestNamespaceName, SriovOcpConfig.OcpSriovTestContainer).
		RedefineDefaultContainer(*container1).
		WithAdditionalContainer(container2).
		WithSecondaryNetwork(netAnnotations).
		WithPrivilegedFlag().
		DefineOnNode(nodeName).
		CreateAndWaitUntilRunning(tsparams.PodReadyTimeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to create multi-container SR-IOV pod %q", sharedVFPodName)
	Expect(testPod.Exists()).To(BeTrue(), "Pod %q does not exist after creation", sharedVFPodName)

	assertOneVFPerContainer(testPod)

	return testPod
}

func defineSharedVFContainer(name string, sriovResource corev1.ResourceList) *corev1.Container {
	container, err := pod.NewContainerBuilder(
		name, SriovOcpConfig.OcpSriovTestContainer, []string{"/bin/bash", "-c", "sleep INF"}).
		WithCustomResourcesRequests(sriovResource).
		WithCustomResourcesLimits(sriovResource).
		GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Failed to define container %q", name)

	return container
}

func assertOneVFPerContainer(testPod *pod.Builder) {
	resKey := corev1.ResourceName("openshift.io/" + sharedVFResourceName)

	Expect(testPod.Object.Spec.Containers).To(HaveLen(2),
		"Pod should have two containers requesting the shared SR-IOV resource")

	for _, container := range testPod.Object.Spec.Containers {
		qty, found := container.Resources.Requests[resKey]
		Expect(found).To(BeTrue(),
			"Container %q is missing resource request %q", container.Name, resKey)
		Expect(qty.Cmp(resource.MustParse("1"))).To(Equal(0),
			"Container %q should request exactly 1 VF of %q, got %s",
			container.Name, resKey, qty.String())
	}
}

func verifySharedVFPairing(testPod *pod.Builder, attempt int) {
	refreshedPod, err := pod.Pull(APIClient, testPod.Definition.Name, testPod.Definition.Namespace)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull pod %q", testPod.Definition.Name)

	pairings := []struct {
		containerName string
		ifaceName     string
	}{
		{sharedVFContainer1Name, sharedVFInterface1Name},
		{sharedVFContainer2Name, sharedVFInterface2Name},
	}

	pluginPCIs := make([]string, 0, len(pairings))
	multusPCIs := make([]string, 0, len(pairings))

	for _, pairing := range pairings {
		pluginPCI := containerPCIDEVICE(refreshedPod, pairing.containerName)
		multusPCI := waitForMultusInterfacePCI(refreshedPod, pairing.ifaceName)

		pluginPCIs = append(pluginPCIs, pluginPCI)
		multusPCIs = append(multusPCIs, multusPCI)

		klog.V(90).Infof(
			"Attempt %d container %q: device-plugin PCI=%s Multus %s PCI=%s",
			attempt, pairing.containerName, pluginPCI, pairing.ifaceName, multusPCI)

		Expect(pluginPCI).To(Equal(multusPCI), func() string {
			annotation := refreshedPod.Object.Annotations["k8s.v1.cni.cncf.io/network-status"]

			return fmt.Sprintf(
				"VF pairing mismatch on attempt %d. "+
					"Container %q device-plugin PCIDEVICE=%s, Multus interface %q pci-address=%s. "+
					"network-status=%s",
				attempt, pairing.containerName, pluginPCI, pairing.ifaceName, multusPCI, annotation)
		})
	}

	Expect(pluginPCIs[0]).ToNot(Equal(pluginPCIs[1]),
		"Both containers were granted the same VF PCI address %s", pluginPCIs[0])
	Expect(multusPCIs[0]).ToNot(Equal(multusPCIs[1]),
		"Multus attached the same VF PCI address %s to both interfaces", multusPCIs[0])
}

func containerPCIDEVICE(testPod *pod.Builder, containerName string) string {
	envName := pciDeviceEnvName(sharedVFResourceName)

	output, err := testPod.ExecCommand([]string{"printenv", envName}, containerName)
	Expect(err).ToNot(HaveOccurred(),
		"Failed to read %s from container %q: %s", envName, containerName, output.String())

	pciValues := strings.Split(strings.TrimSpace(output.String()), ",")
	Expect(pciValues).To(HaveLen(1),
		"Container %q should have exactly one PCI in %s, got %q",
		containerName, envName, strings.TrimSpace(output.String()))
	Expect(pciValues[0]).NotTo(BeEmpty(),
		"Container %q has empty %s", containerName, envName)

	return strings.ToLower(pciValues[0])
}

func waitForMultusInterfacePCI(testPod *pod.Builder, ifaceName string) string {
	var pciAddress string

	Eventually(func() error {
		var err error

		pciAddress, err = sriovenv.GetPciAddress(
			testPod.Definition.Namespace, testPod.Definition.Name, ifaceName)

		return err
	}, tsparams.NADTimeout, tsparams.PollingInterval).Should(Succeed(),
		"Failed to get Multus pci-address for interface %q on pod %q",
		ifaceName, testPod.Definition.Name)

	return strings.ToLower(pciAddress)
}

func pciDeviceEnvName(resourceName string) string {
	fullName := strings.ToUpper("openshift.io/" + resourceName)
	replacer := strings.NewReplacer(".", "_", "/", "_", "-", "_")

	return "PCIDEVICE_" + replacer.Replace(fullName)
}

func disableNetworkResourcesInjectorIfEnabled() {
	operatorConfig, err := sriov.PullOperatorConfig(APIClient, SriovOcpConfig.OcpSriovOperatorNamespace)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull SriovOperatorConfig")

	if !operatorConfig.Definition.Spec.EnableInjector {
		return
	}

	// Register restore first so a failure while waiting for the webhook to
	// disappear still puts enableInjector back.
	DeferCleanup(func() {
		By("Restoring the network-resources-injector")

		setNetworkResourcesInjector(true)
	})

	setNetworkResourcesInjector(false)
}

func setNetworkResourcesInjector(enable bool) {
	operatorConfig, err := sriov.PullOperatorConfig(APIClient, SriovOcpConfig.OcpSriovOperatorNamespace)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull SriovOperatorConfig")

	_, err = operatorConfig.WithInjector(enable).Update()
	Expect(err).ToNot(HaveOccurred(), "Failed to set enableInjector=%t", enable)

	if enable {
		waitForNetworkResourcesInjectorReady()

		return
	}

	waitForNetworkResourcesInjectorGone()
}

func waitForNetworkResourcesInjectorReady() {
	Eventually(func() error {
		injectorDS, pullErr := daemonset.Pull(
			APIClient, networkResourcesInjectorDS, SriovOcpConfig.OcpSriovOperatorNamespace)
		if pullErr != nil {
			return pullErr
		}

		if !injectorDS.IsReady(5 * time.Second) {
			return fmt.Errorf("daemonset %s is not ready", networkResourcesInjectorDS)
		}

		return nil
	}, injectorReconcileTimeout, tsparams.RetryInterval).Should(Succeed(),
		"network-resources-injector did not become ready")

	Eventually(func() error {
		_, pullErr := webhook.PullMutatingConfiguration(APIClient, networkResourcesInjectorHook)

		return pullErr
	}, injectorReconcileTimeout, tsparams.RetryInterval).Should(Succeed(),
		"MutatingWebhook %s was not created after enabling the injector", networkResourcesInjectorHook)
}

func waitForNetworkResourcesInjectorGone() {
	Eventually(func() error {
		_, pullErr := webhook.PullMutatingConfiguration(APIClient, networkResourcesInjectorHook)
		if pullErr == nil {
			return fmt.Errorf("mutating webhook %s still exists", networkResourcesInjectorHook)
		}

		if !isNotFoundError(pullErr) {
			return pullErr
		}

		return nil
	}, injectorReconcileTimeout, tsparams.RetryInterval).Should(Succeed(),
		"MutatingWebhook %s was not removed after disabling the injector", networkResourcesInjectorHook)
}

func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	return k8serrors.IsNotFound(err) || strings.Contains(err.Error(), "does not exist")
}
