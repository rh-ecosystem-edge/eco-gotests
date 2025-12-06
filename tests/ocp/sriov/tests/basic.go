package tests

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/params"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/ocpsriovinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/sriovenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/tsparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// isNoCarrierError checks if an error indicates a NO-CARRIER condition.
func isNoCarrierError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := err.Error()

	return strings.Contains(errMsg, "NO-CARRIER") ||
		strings.Contains(errMsg, "no physical connection")
}

// setupTestNamespace creates a test namespace with required labels and registers cleanup.
func setupTestNamespace(testID string, data tsparams.DeviceConfig) string {
	ns := "e2e-" + testID + data.Name

	By(fmt.Sprintf("Creating test namespace %q", ns))

	nsBuilder := namespace.NewBuilder(APIClient, ns)
	for key, value := range params.PrivilegedNSLabels {
		nsBuilder.WithLabel(key, value)
	}

	_, err := nsBuilder.Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create namespace %q", ns)

	Eventually(func() bool {
		return nsBuilder.Exists()
	}, tsparams.NamespaceTimeout, tsparams.RetryInterval).Should(BeTrue(), "Namespace %q should exist", ns)

	DeferCleanup(func() {
		By(fmt.Sprintf("Cleaning up namespace %q", ns))

		err := nsBuilder.DeleteAndWait(tsparams.CleanupTimeout)
		Expect(err).ToNot(HaveOccurred(), "Failed to delete namespace %q", ns)
	})

	return ns
}

// setupSriovNetwork creates a SRIOV network and registers cleanup.
func setupSriovNetwork(networkName, resourceName, targetNs string, opts ...sriovenv.NetworkOption) {
	By(fmt.Sprintf("Creating SR-IOV network %q", networkName))

	err := sriovenv.CreateSriovNetwork(networkName, resourceName, targetNs, opts...)
	Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV network %q", networkName)

	DeferCleanup(func() {
		By(fmt.Sprintf("Cleaning up SR-IOV network %q", networkName))

		err := sriovenv.RemoveSriovNetwork(networkName, tsparams.DefaultTimeout)
		Expect(err).ToNot(HaveOccurred(), "Failed to remove SR-IOV network %q", networkName)
	})
}

var _ = Describe(
	"SR-IOV Basic Tests",
	Ordered,
	Label(tsparams.LabelBasic),
	ContinueOnFailure,
	func() {
		var (
			vfNum       = tsparams.GetVFNum()
			workerNodes []*nodes.Builder
			testData    = tsparams.GetDeviceConfig()
		)

		BeforeAll(func() {
			By("Checking the SR-IOV operator is running")
			err := sriovenv.CheckSriovOperatorStatus()
			Expect(err).ToNot(HaveOccurred(), "SR-IOV operator is not running")

			By("Discovering worker nodes")
			workerNodes, err = nodes.List(APIClient,
				metav1.ListOptions{LabelSelector: SriovOcpConfig.WorkerLabel})
			Expect(err).ToNot(HaveOccurred(), "Failed to discover nodes")
			Expect(len(workerNodes)).To(BeNumerically(">", 0), "No worker nodes found")
		})

		AfterAll(func() {
			By("Cleaning up SR-IOV policies after all tests")

			var cleanupErrors []string
			for _, item := range testData {
				err := sriovenv.RemoveSriovPolicy(item.Name, tsparams.DefaultTimeout)
				if err != nil {
					cleanupErrors = append(cleanupErrors, fmt.Sprintf("policy %q: %v", item.Name, err))
				}
			}

			if len(cleanupErrors) > 0 {
				klog.Warningf("Some policies failed to clean up: %v", cleanupErrors)
			}

			By("Waiting for post-cleanup cluster stability")
			err := sriovenv.WaitForSriovPolicyReady(tsparams.DefaultTimeout)
			Expect(err).ToNot(HaveOccurred(), "Cluster did not stabilize after cleanup")
		})

		It("SR-IOV VF with spoof checking enabled", reportxml.ID("25959"), func() {
			caseID := "25959"
			executed := false

			for _, data := range testData {
				By(fmt.Sprintf("Testing device: %s", data.Name))

				result, err := sriovenv.InitVF(data.Name, data.DeviceID, data.InterfaceName,
					data.Vendor, vfNum, workerNodes)
				Expect(err).ToNot(HaveOccurred(), "Failed to initialize VF for device %q", data.Name)

				if !result {
					By(fmt.Sprintf("Skipping device %q - VF init failed", data.Name))

					continue
				}

				executed = true
				ns := setupTestNamespace(caseID+"-", data)
				networkName := caseID + "-" + data.Name
				setupSriovNetwork(networkName, data.Name, ns,
					sriovenv.WithSpoof(true), sriovenv.WithTrust(false))

				By("Verifying VF status with pass traffic")
				err = sriovenv.CheckVFStatusWithPassTraffic(networkName, data.InterfaceName,
					ns, "spoof checking on", tsparams.PodReadyTimeout)
				if isNoCarrierError(err) {
					Skip("Interface has NO-CARRIER status")
				}

				Expect(err).ToNot(HaveOccurred(), "Test verification failed")
			}

			if !executed {
				Skip("No SR-IOV devices matched the requested configuration")
			}
		})

		It("SR-IOV VF with spoof checking disabled", reportxml.ID("70820"), func() {
			caseID := "70820"
			executed := false

			for _, data := range testData {
				By(fmt.Sprintf("Testing device: %s", data.Name))

				result, err := sriovenv.InitVF(data.Name, data.DeviceID, data.InterfaceName,
					data.Vendor, vfNum, workerNodes)
				Expect(err).ToNot(HaveOccurred(), "Failed to initialize VF")

				if !result {
					continue
				}

				executed = true
				ns := setupTestNamespace(caseID+"-", data)
				networkName := caseID + "-" + data.Name
				setupSriovNetwork(networkName, data.Name, ns,
					sriovenv.WithSpoof(false), sriovenv.WithTrust(true))

				err = sriovenv.CheckVFStatusWithPassTraffic(networkName, data.InterfaceName,
					ns, "spoof checking off", tsparams.PodReadyTimeout)
				if isNoCarrierError(err) {
					Skip("Interface has NO-CARRIER status")
				}

				Expect(err).ToNot(HaveOccurred(), "Test verification failed")
			}

			if !executed {
				Skip("No SR-IOV devices matched the requested configuration")
			}
		})

		It("SR-IOV VF with trust disabled", reportxml.ID("25960"), func() {
			caseID := "25960"
			executed := false

			for _, data := range testData {
				By(fmt.Sprintf("Testing device: %s", data.Name))

				result, err := sriovenv.InitVF(data.Name, data.DeviceID, data.InterfaceName,
					data.Vendor, vfNum, workerNodes)
				Expect(err).ToNot(HaveOccurred(), "Failed to initialize VF")

				if !result {
					continue
				}

				executed = true
				ns := setupTestNamespace(caseID+"-", data)
				networkName := caseID + "-" + data.Name
				setupSriovNetwork(networkName, data.Name, ns,
					sriovenv.WithSpoof(false), sriovenv.WithTrust(false))

				err = sriovenv.CheckVFStatusWithPassTraffic(networkName, data.InterfaceName,
					ns, "trust off", tsparams.PodReadyTimeout)
				if isNoCarrierError(err) {
					Skip("Interface has NO-CARRIER status")
				}

				Expect(err).ToNot(HaveOccurred(), "Test verification failed")
			}

			if !executed {
				Skip("No SR-IOV devices matched the requested configuration")
			}
		})

		It("SR-IOV VF with trust enabled", reportxml.ID("70821"), func() {
			caseID := "70821"
			executed := false

			for _, data := range testData {
				By(fmt.Sprintf("Testing device: %s", data.Name))

				result, err := sriovenv.InitVF(data.Name, data.DeviceID, data.InterfaceName,
					data.Vendor, vfNum, workerNodes)
				Expect(err).ToNot(HaveOccurred(), "Failed to initialize VF")

				if !result {
					continue
				}

				executed = true
				ns := setupTestNamespace(caseID+"-", data)
				networkName := caseID + "-" + data.Name
				setupSriovNetwork(networkName, data.Name, ns,
					sriovenv.WithSpoof(true), sriovenv.WithTrust(true))

				err = sriovenv.CheckVFStatusWithPassTraffic(networkName, data.InterfaceName,
					ns, "trust on", tsparams.PodReadyTimeout)
				if isNoCarrierError(err) {
					Skip("Interface has NO-CARRIER status")
				}

				Expect(err).ToNot(HaveOccurred(), "Test verification failed")
			}

			if !executed {
				Skip("No SR-IOV devices matched the requested configuration")
			}
		})

		It("SR-IOV VF with VLAN and rate limiting configuration", reportxml.ID("25963"), func() {
			caseID := "25963"
			executed := false

			for _, data := range testData {
				By(fmt.Sprintf("Testing device: %s", data.Name))

				if !data.SupportsMinTxRate {
					By(fmt.Sprintf("Skipping device %q - does not support minTxRate", data.Name))

					continue
				}

				result, err := sriovenv.InitVF(data.Name, data.DeviceID, data.InterfaceName,
					data.Vendor, vfNum, workerNodes)
				Expect(err).ToNot(HaveOccurred(), "Failed to initialize VF")

				if !result {
					continue
				}

				executed = true
				ns := setupTestNamespace(caseID+"-", data)
				networkName := caseID + "-" + data.Name
				setupSriovNetwork(networkName, data.Name, ns,
					sriovenv.WithVLAN(100),
					sriovenv.WithVlanQoS(2),
					sriovenv.WithMinTxRate(40),
					sriovenv.WithMaxTxRate(100))

				err = sriovenv.CheckVFStatusWithPassTraffic(networkName, data.InterfaceName,
					ns, "vlan 100, qos 2", tsparams.PodReadyTimeout)
				if isNoCarrierError(err) {
					Skip("Interface has NO-CARRIER status")
				}

				Expect(err).ToNot(HaveOccurred(), "Test verification failed")
			}

			if !executed {
				Skip("No SR-IOV devices matched the requested configuration")
			}
		})

		It("SR-IOV VF with auto link state", reportxml.ID("25961"), func() {
			caseID := "25961"
			executed := false

			for _, data := range testData {
				By(fmt.Sprintf("Testing device: %s", data.Name))

				result, err := sriovenv.InitVF(data.Name, data.DeviceID, data.InterfaceName,
					data.Vendor, vfNum, workerNodes)
				Expect(err).ToNot(HaveOccurred(), "Failed to initialize VF")

				if !result {
					continue
				}

				executed = true
				ns := setupTestNamespace(caseID+"-", data)
				networkName := caseID + "-" + data.Name
				setupSriovNetwork(networkName, data.Name, ns,
					sriovenv.WithLinkState("auto"))

				err = sriovenv.CheckVFStatusWithPassTraffic(networkName, data.InterfaceName,
					ns, "link-state auto", tsparams.PodReadyTimeout)
				if isNoCarrierError(err) {
					Skip("Interface has NO-CARRIER status")
				}

				Expect(err).ToNot(HaveOccurred(), "Test verification failed")
			}

			if !executed {
				Skip("No SR-IOV devices matched the requested configuration")
			}
		})

		It("SR-IOV VF with enabled link state", reportxml.ID("71006"), func() {
			caseID := "71006"
			executed := false

			for _, data := range testData {
				By(fmt.Sprintf("Testing device: %s", data.Name))

				result, err := sriovenv.InitVF(data.Name, data.DeviceID, data.InterfaceName,
					data.Vendor, vfNum, workerNodes)
				Expect(err).ToNot(HaveOccurred(), "Failed to initialize VF")

				if !result {
					continue
				}

				executed = true
				ns := setupTestNamespace(caseID+"-", data)
				networkName := caseID + "-" + data.Name
				setupSriovNetwork(networkName, data.Name, ns,
					sriovenv.WithLinkState("enable"))

				// Part 1: Verify link state configuration
				By("Verifying link state configuration is applied")
				hasCarrier, err := sriovenv.VerifyLinkStateConfiguration(networkName, ns,
					"link-state enable", tsparams.PodReadyTimeout)
				Expect(err).ToNot(HaveOccurred(), "Failed to verify link state configuration")

				if !hasCarrier {
					Skip("NO-CARRIER status - link state valid but no physical connection")
				}

				// Part 2: Test connectivity
				By("Testing connectivity")
				err = sriovenv.CheckVFStatusWithPassTraffic(networkName, data.InterfaceName,
					ns, "link-state enable", tsparams.PodReadyTimeout)
				Expect(err).ToNot(HaveOccurred(), "VF connectivity test failed")
			}

			if !executed {
				Skip("No SR-IOV devices matched the requested configuration")
			}
		})

		It("MTU configuration for SR-IOV policy", reportxml.ID("69646"), func() {
			caseID := "69646"
			executed := false

			for _, data := range testData {
				By(fmt.Sprintf("Testing device: %s", data.Name))

				result, err := sriovenv.InitVF(data.Name, data.DeviceID, data.InterfaceName,
					data.Vendor, vfNum, workerNodes)
				Expect(err).ToNot(HaveOccurred(), "Failed to initialize VF")

				if !result {
					continue
				}

				executed = true

				// Configure MTU in SR-IOV policy
				By(fmt.Sprintf("Updating SR-IOV policy %q with MTU %d", data.Name, tsparams.DefaultTestMTU))
				err = sriovenv.UpdateSriovPolicyMTU(data.Name, tsparams.DefaultTestMTU)
				Expect(err).ToNot(HaveOccurred(), "Failed to update SR-IOV policy with MTU")

				By("Waiting for SR-IOV policy to be ready after MTU update")
				err = sriovenv.WaitForSriovPolicyReady(tsparams.DefaultTimeout)
				Expect(err).ToNot(HaveOccurred(), "Policy not ready after MTU update")

				ns := setupTestNamespace(caseID+"-", data)
				networkName := caseID + "-" + data.Name
				setupSriovNetwork(networkName, data.Name, ns,
					sriovenv.WithSpoof(true), sriovenv.WithTrust(true))

				err = sriovenv.CheckVFStatusWithPassTraffic(networkName, data.InterfaceName,
					ns, fmt.Sprintf("mtu %d", tsparams.DefaultTestMTU), tsparams.PodReadyTimeout)
				if isNoCarrierError(err) {
					Skip("Interface has NO-CARRIER status")
				}

				Expect(err).ToNot(HaveOccurred(), "Test verification failed")
			}

			if !executed {
				Skip("No SR-IOV devices matched the requested configuration")
			}
		})

		It("DPDK SR-IOV VF functionality validation", reportxml.ID("69582"), func() {
			caseID := "69582"
			executed := false

			for _, data := range testData {
				By(fmt.Sprintf("Testing device: %s", data.Name))

				// Skip BCM NICs: OCPBUGS-30909
				if strings.Contains(data.Name, "bcm") {
					By(fmt.Sprintf("Skipping device %q - BCM NICs not supported for DPDK", data.Name))

					continue
				}

				result, err := sriovenv.InitDpdkVF(data.Name, data.DeviceID, data.InterfaceName,
					data.Vendor, vfNum, workerNodes)
				Expect(err).ToNot(HaveOccurred(), "Failed to initialize DPDK VF")

				if !result {
					continue
				}

				executed = true
				ns := setupTestNamespace(caseID+"-", data)
				networkName := data.Name + "dpdknet"
				setupSriovNetwork(networkName, data.Name, ns)

				// Wait for NAD to be ready
				By("Waiting for NetworkAttachmentDefinition to be fully ready")
				Eventually(func() error {
					_, err := nad.Pull(APIClient, networkName, ns)

					return err
				}, tsparams.NADTimeout, tsparams.PollingInterval).ShouldNot(HaveOccurred(),
					"NAD %q should be ready", networkName)

				// Create DPDK test pod
				By("Creating DPDK test pod")
				_, err = sriovenv.CreateDpdkTestPod("sriovdpdk", ns, networkName)
				Expect(err).ToNot(HaveOccurred(), "Failed to create DPDK test pod")

				DeferCleanup(func() {
					By("Cleaning up DPDK test pod")
					err := sriovenv.DeleteDpdkTestPod("sriovdpdk", ns, tsparams.NamespaceTimeout)
					Expect(err).ToNot(HaveOccurred(), "Failed to delete DPDK test pod")
				})

				// Wait for pod to be ready
				By("Waiting for DPDK test pod to be ready")
				err = sriovenv.WaitForPodWithLabelReady(ns, "name=sriov-dpdk", tsparams.PodReadyTimeout)
				Expect(err).ToNot(HaveOccurred(), "DPDK test pod not ready")

				// Verify PCI address is assigned
				By("Verifying PCI address is assigned to DPDK pod")
				pciAddress, err := sriovenv.GetPciAddress(ns, "sriovdpdk", networkName)
				Expect(err).ToNot(HaveOccurred(), "Failed to get PCI address for DPDK pod")
				Expect(pciAddress).NotTo(BeEmpty(), "PCI address should be assigned")

				// Verify DPDK VF is available in pod
				By("Verifying DPDK VF is available in pod")
				podBuilder, err := pod.Pull(APIClient, "sriovdpdk", ns)
				Expect(err).ToNot(HaveOccurred(), "Failed to pull DPDK pod")
				Expect(podBuilder).NotTo(BeNil(), "Pod builder should not be nil")

				// Check network status annotation
				networkStatusAnnotation := "k8s.v1.cni.cncf.io/network-status"
				podNetAnnotation := podBuilder.Object.Annotations[networkStatusAnnotation]
				Expect(podNetAnnotation).NotTo(BeEmpty(), "Pod should have network status annotation")
				Expect(podNetAnnotation).To(ContainSubstring("pci-address"),
					"Network status should contain PCI address")
				Expect(podNetAnnotation).To(ContainSubstring(pciAddress),
					"Network status should contain the assigned PCI address")
			}

			if !executed {
				Skip("No SR-IOV devices matched the requested configuration")
			}
		})
	})
