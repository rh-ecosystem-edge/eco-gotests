// Package tests contains the SR-IOV basic test cases for OCP.
// These tests validate VF creation, network configuration, and connectivity.
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
	sriovconfig "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/ocpsriovconfig"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/ocpsriovinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/sriovenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/tsparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

var _ = Describe(
	"SR-IOV Basic Tests",
	Ordered,
	Label(tsparams.LabelBasic),
	ContinueOnFailure,
	func() {
		var (
			vfNum       int
			workerNodes []*nodes.Builder
			testData    []sriovconfig.DeviceConfig
		)

		BeforeAll(func() {
			By("Checking the SR-IOV operator is running")

			err := sriovenv.CheckSriovOperatorStatus()
			Expect(err).ToNot(HaveOccurred(), "SR-IOV operator is not running")

			By("Loading VF configuration")

			vfNum, err = SriovOcpConfig.GetVFNum()
			Expect(err).ToNot(HaveOccurred(), "Failed to get VF number")

			By("Loading SR-IOV device configuration")

			testData, err = SriovOcpConfig.GetSriovDevices()
			Expect(err).ToNot(HaveOccurred(), "Failed to get SR-IOV devices")
			Expect(len(testData)).To(BeNumerically(">", 0), "No SR-IOV devices configured")

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
				testNamespace := setupTestNamespace(caseID+"-", data)
				networkName := caseID + "-" + data.Name
				setupSriovNetwork(networkName, data.Name, testNamespace,
					sriovenv.WithSpoof(true), sriovenv.WithTrust(false))

				By("Verifying VF status with pass traffic")

				err = sriovenv.CheckVFStatusWithPassTraffic(networkName, data.InterfaceName,
					testNamespace, "spoof checking on", tsparams.PodReadyTimeout)
				if isNoCarrierError(err) {
					By(fmt.Sprintf("Skipping device %q - NO-CARRIER status", data.Name))

					continue
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
				testNamespace := setupTestNamespace(caseID+"-", data)
				networkName := caseID + "-" + data.Name
				setupSriovNetwork(networkName, data.Name, testNamespace,
					sriovenv.WithSpoof(false), sriovenv.WithTrust(true))

				err = sriovenv.CheckVFStatusWithPassTraffic(networkName, data.InterfaceName,
					testNamespace, "spoof checking off", tsparams.PodReadyTimeout)
				if isNoCarrierError(err) {
					By(fmt.Sprintf("Skipping device %q - NO-CARRIER status", data.Name))

					continue
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
				testNamespace := setupTestNamespace(caseID+"-", data)
				networkName := caseID + "-" + data.Name
				setupSriovNetwork(networkName, data.Name, testNamespace,
					sriovenv.WithSpoof(false), sriovenv.WithTrust(false))

				err = sriovenv.CheckVFStatusWithPassTraffic(networkName, data.InterfaceName,
					testNamespace, "trust off", tsparams.PodReadyTimeout)
				if isNoCarrierError(err) {
					By(fmt.Sprintf("Skipping device %q - NO-CARRIER status", data.Name))

					continue
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
				testNamespace := setupTestNamespace(caseID+"-", data)
				networkName := caseID + "-" + data.Name
				setupSriovNetwork(networkName, data.Name, testNamespace,
					sriovenv.WithSpoof(true), sriovenv.WithTrust(true))

				err = sriovenv.CheckVFStatusWithPassTraffic(networkName, data.InterfaceName,
					testNamespace, "trust on", tsparams.PodReadyTimeout)
				if isNoCarrierError(err) {
					By(fmt.Sprintf("Skipping device %q - NO-CARRIER status", data.Name))

					continue
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
				testNamespace := setupTestNamespace(caseID+"-", data)
				networkName := caseID + "-" + data.Name
				setupSriovNetwork(networkName, data.Name, testNamespace,
					sriovenv.WithVLAN(tsparams.TestVLAN),
					sriovenv.WithVlanQoS(tsparams.TestVlanQoS),
					sriovenv.WithMinTxRate(tsparams.TestMinTxRate),
					sriovenv.WithMaxTxRate(tsparams.TestMaxTxRate))

				err = sriovenv.CheckVFStatusWithPassTraffic(networkName, data.InterfaceName,
					testNamespace, fmt.Sprintf("vlan %d, qos %d", tsparams.TestVLAN, tsparams.TestVlanQoS),
					tsparams.PodReadyTimeout)
				if isNoCarrierError(err) {
					By(fmt.Sprintf("Skipping device %q - NO-CARRIER status", data.Name))

					continue
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
				testNamespace := setupTestNamespace(caseID+"-", data)
				networkName := caseID + "-" + data.Name
				setupSriovNetwork(networkName, data.Name, testNamespace,
					sriovenv.WithLinkState("auto"))

				err = sriovenv.CheckVFStatusWithPassTraffic(networkName, data.InterfaceName,
					testNamespace, "link-state auto", tsparams.PodReadyTimeout)
				if isNoCarrierError(err) {
					By(fmt.Sprintf("Skipping device %q - NO-CARRIER status", data.Name))

					continue
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
				testNamespace := setupTestNamespace(caseID+"-", data)
				networkName := caseID + "-" + data.Name
				setupSriovNetwork(networkName, data.Name, testNamespace,
					sriovenv.WithLinkState("enable"))

				// Part 1: Verify link state configuration
				By("Verifying link state configuration is applied")

				hasCarrier, err := sriovenv.VerifyLinkStateConfiguration(networkName, testNamespace,
					"link-state enable", tsparams.PodReadyTimeout)
				Expect(err).ToNot(HaveOccurred(), "Failed to verify link state configuration")

				if !hasCarrier {
					By(fmt.Sprintf("Skipping device %q - NO-CARRIER status", data.Name))

					continue
				}

				// Part 2: Test connectivity
				By("Testing connectivity")

				err = sriovenv.CheckVFStatusWithPassTraffic(networkName, data.InterfaceName,
					testNamespace, "link-state enable", tsparams.PodReadyTimeout)
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

				testNamespace := setupTestNamespace(caseID+"-", data)
				networkName := caseID + "-" + data.Name
				setupSriovNetwork(networkName, data.Name, testNamespace,
					sriovenv.WithSpoof(true), sriovenv.WithTrust(true))

				err = sriovenv.CheckVFStatusWithPassTraffic(networkName, data.InterfaceName,
					testNamespace, fmt.Sprintf("mtu %d", tsparams.DefaultTestMTU), tsparams.PodReadyTimeout)
				if isNoCarrierError(err) {
					By(fmt.Sprintf("Skipping device %q - NO-CARRIER status", data.Name))

					continue
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

				// Skip BCM NICs: OCPBUGS-30909 - BCM NICs require different driver configuration for DPDK
				if isBCMDevice(data) {
					By(fmt.Sprintf("Skipping device %q - BCM NICs not supported for DPDK (OCPBUGS-30909)",
						data.Name))

					continue
				}

				result, err := sriovenv.InitDpdkVF(data.Name, data.DeviceID, data.InterfaceName,
					data.Vendor, vfNum, workerNodes)
				Expect(err).ToNot(HaveOccurred(), "Failed to initialize DPDK VF")

				if !result {
					continue
				}

				executed = true
				testNamespace := setupTestNamespace(caseID+"-", data)
				networkName := caseID + "-" + data.Name + "-dpdk"
				setupSriovNetwork(networkName, data.Name, testNamespace)

				// Wait for NAD to be ready
				By("Waiting for NetworkAttachmentDefinition to be fully ready")
				Eventually(func() error {
					_, err := nad.Pull(APIClient, networkName, testNamespace)

					return err
				}, tsparams.NADTimeout, tsparams.PollingInterval).ShouldNot(HaveOccurred(),
					"NAD %q should be ready", networkName)

				// Create DPDK test pod
				By("Creating DPDK test pod")

				_, err = sriovenv.CreateDpdkTestPod("sriovdpdk", testNamespace, networkName)
				Expect(err).ToNot(HaveOccurred(), "Failed to create DPDK test pod")

				DeferCleanup(func() {
					By("Cleaning up DPDK test pod")

					err := sriovenv.DeleteDpdkTestPod("sriovdpdk", testNamespace, tsparams.NamespaceTimeout)
					Expect(err).ToNot(HaveOccurred(), "Failed to delete DPDK test pod")
				})

				// Wait for pod to be ready
				By("Waiting for DPDK test pod to be ready")

				err = sriovenv.WaitForPodWithLabelReady(testNamespace, "name=sriov-dpdk", tsparams.PodReadyTimeout)
				Expect(err).ToNot(HaveOccurred(), "DPDK test pod not ready")

				// Verify PCI address is assigned
				By("Verifying PCI address is assigned to DPDK pod")

				pciAddress, err := sriovenv.GetPciAddress(testNamespace, "sriovdpdk", "net1")
				Expect(err).ToNot(HaveOccurred(), "Failed to get PCI address for DPDK pod")
				Expect(pciAddress).NotTo(BeEmpty(), "PCI address should be assigned")

				// Verify DPDK VF is available in pod
				By("Verifying DPDK VF is available in pod")

				podBuilder, err := pod.Pull(APIClient, "sriovdpdk", testNamespace)
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

// isNoCarrierError checks if an error indicates a NO-CARRIER condition.
// This typically happens when there is no physical network connection on the interface.
func isNoCarrierError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := err.Error()

	return strings.Contains(errMsg, "NO-CARRIER") ||
		strings.Contains(errMsg, "no physical connection")
}

// isBCMDevice checks if the device is a Broadcom NIC by name or vendor ID.
// BCM NICs require special handling for DPDK tests (OCPBUGS-30909).
func isBCMDevice(data sriovconfig.DeviceConfig) bool {
	return strings.Contains(strings.ToLower(data.Name), "bcm") ||
		data.Vendor == tsparams.BCMVendorID
}

// setupTestNamespace creates a test namespace with required labels and registers cleanup.
// The namespace name follows the pattern: e2e-{testID}{deviceName}.
func setupTestNamespace(testID string, data sriovconfig.DeviceConfig) string {
	testNamespace := "e2e-" + testID + data.Name

	By(fmt.Sprintf("Creating test namespace %q", testNamespace))

	nsBuilder := namespace.NewBuilder(APIClient, testNamespace).WithMultipleLabels(params.PrivilegedNSLabels)

	_, err := nsBuilder.Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create namespace %q", testNamespace)

	Eventually(func() bool {
		return nsBuilder.Exists()
	}, tsparams.NamespaceTimeout, tsparams.RetryInterval).Should(BeTrue(), "Namespace %q should exist", testNamespace)

	DeferCleanup(func() {
		By(fmt.Sprintf("Cleaning up namespace %q", testNamespace))

		err := nsBuilder.DeleteAndWait(tsparams.CleanupTimeout)
		Expect(err).ToNot(HaveOccurred(), "Failed to delete namespace %q", testNamespace)
	})

	return testNamespace
}

// setupSriovNetwork creates a SRIOV network and registers cleanup.
// The network is created in the SR-IOV operator namespace and targets the specified namespace.
func setupSriovNetwork(networkName, resourceName, targetNs string, opts ...sriovenv.NetworkOption) {
	By(fmt.Sprintf("Creating SR-IOV network %q", networkName))

	err := sriovenv.CreateSriovNetwork(networkName, resourceName, targetNs, opts...)
	Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV network %q", networkName)

	By(fmt.Sprintf("Waiting for NAD %q to be ready", networkName))

	Eventually(func() error {
		_, err := nad.Pull(APIClient, networkName, targetNs)

		return err
	}, tsparams.NADTimeout, tsparams.PollingInterval).ShouldNot(HaveOccurred(),
		"NAD %q should be ready in namespace %q", networkName, targetNs)

	DeferCleanup(func() {
		By(fmt.Sprintf("Cleaning up SR-IOV network %q", networkName))

		err := sriovenv.RemoveSriovNetwork(networkName, tsparams.DefaultTimeout)
		Expect(err).ToNot(HaveOccurred(), "Failed to remove SR-IOV network %q", networkName)
	})
}
