package tests

import (
	"fmt"
	"path/filepath"
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
	sriovenv "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/sriovenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/tsparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe(
	"SR-IOV Basic Tests",
	Ordered,
	Label(tsparams.LabelSuite, tsparams.LabelBasic),
	ContinueOnFailure,
	func() {
		var (
			buildPruningBaseDir  = filepath.Join("testdata", "networking", "sriov")
			sriovNetworkTemplate = filepath.Join(buildPruningBaseDir, "sriovnetwork-whereabouts-template.yaml")
			sriovOpNs            = SriovOcpConfig.OcpSriovOperatorNamespace
			vfNum                = tsparams.GetVFNum()
			workerNodes          []*nodes.Builder
			testData             = tsparams.GetDeviceConfig()
		)

		// Suppress unused variable warnings for variables that will be used in future test cases
		_ = sriovNetworkTemplate

		BeforeAll(func() {
			By("Checking the SR-IOV operator is running")
			// Note: CheckSriovOperatorStatus will be updated in Phase 5 to use SriovOcpConfig
			// For now, we'll create a temporary NetworkConfig adapter or update the function
			err := sriovenv.CheckSriovOperatorStatus(APIClient, SriovOcpConfig)
			Expect(err).ToNot(HaveOccurred(), "SR-IOV operator is not running")

		By("Discovering worker nodes")
		// Normalize OcpWorkerLabel to ensure it's a valid label selector
		// Unlike sriovenv.go which expects "key=" format, we normalize here as a workaround
		// to handle cases where the label might be provided without the "=" suffix
		workerLabelSelector := SriovOcpConfig.OcpWorkerLabel
		// If it's just a label name without "=", construct the full selector
		if workerLabelSelector != "" && !strings.Contains(workerLabelSelector, "=") {
			// Construct the full label selector format
			workerLabelSelector = fmt.Sprintf("%s=", workerLabelSelector)
		}
			// Default to standard worker label if not set
			if workerLabelSelector == "" {
				workerLabelSelector = "node-role.kubernetes.io/worker="
			}
			workerNodes, err = nodes.List(APIClient,
				metav1.ListOptions{LabelSelector: workerLabelSelector})
			Expect(err).ToNot(HaveOccurred(), "Failed to discover nodes")
			Expect(len(workerNodes)).To(BeNumerically(">", 0), "No worker nodes found")
		})

		AfterAll(func() {
			By("Cleaning up SR-IOV policies after all tests")
			// Clean up all policies that were created during tests
			for _, item := range testData {
				err := sriovenv.RemoveSriovPolicy(APIClient, item.Name, sriovOpNs, tsparams.DefaultTimeout)
				if err != nil {
					// Log error but don't fail - cleanup is best effort
					// Error is already logged in RemoveSriovPolicy function
				}
			}
			By("Waiting for SR-IOV policies to be ready after cleanup")
			err := sriovenv.WaitForSriovPolicyReady(APIClient, SriovOcpConfig, tsparams.DefaultTimeout)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for SR-IOV policies to be ready")
		})

		It("SR-IOV VF with spoof checking enabled", reportxml.ID("25959"), func() {
			var caseID = "25959-"
			executed := false

			for _, data := range testData {
				data := data
				By(fmt.Sprintf("Testing device: %s (DeviceID: %s, Interface: %s)", data.Name, data.DeviceID, data.InterfaceName))

				// Create VF on with given device
				result, err := sriovenv.InitVF(APIClient, SriovOcpConfig, data.Name, data.DeviceID, data.InterfaceName, data.Vendor, sriovOpNs, vfNum, workerNodes)
				Expect(err).ToNot(HaveOccurred(), "Failed to initialize VF for device %q", data.Name)
				if !result {
					By(fmt.Sprintf("Skipping device %q - VF initialization failed or device not found on any node", data.Name))
					continue
				}

				executed = true
				func() {
					ns1 := "e2e-" + caseID + data.Name
					// Create unique network name with test case ID to avoid conflicts between tests
					networkName := caseID + data.Name

					By(fmt.Sprintf("Creating test namespace %q", ns1))
					// Create namespace for the test
					nsBuilder := namespace.NewBuilder(APIClient, ns1)
					for key, value := range params.PrivilegedNSLabels {
						nsBuilder.WithLabel(key, value)
					}
					_, err := nsBuilder.Create()
					Expect(err).ToNot(HaveOccurred(), "Failed to create namespace %q", ns1)

					// Wait for namespace to be ready before proceeding
					Eventually(func() bool {
						return nsBuilder.Exists()
					}, tsparams.NamespaceTimeout, tsparams.RetryInterval).Should(BeTrue(), "Namespace %q should exist", ns1)

					DeferCleanup(func() {
						By(fmt.Sprintf("Cleaning up namespace %q", ns1))
						err := nsBuilder.DeleteAndWait(tsparams.CleanupTimeout)
						Expect(err).ToNot(HaveOccurred(), "Failed to delete namespace %q", ns1)
					})

					By("Creating SR-IOV network to generate net-attach-def on the target namespace")
					networkConfig := &sriovenv.SriovNetworkConfig{
						Name:             networkName,
						ResourceName:     data.Name,
						NetworkNamespace: ns1,
						Namespace:        sriovOpNs,
						SpoofCheck:       "on",
						Trust:            "off",
					}

					err = sriovenv.CreateSriovNetwork(APIClient, networkConfig, tsparams.WaitTimeout)
					Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV network %q", networkName)

					DeferCleanup(func() {
						By(fmt.Sprintf("Cleaning up SR-IOV network %q", networkName))
						err := sriovenv.RemoveSriovNetwork(APIClient, networkName, sriovOpNs, tsparams.DefaultTimeout)
						Expect(err).ToNot(HaveOccurred(), "Failed to remove SR-IOV network %q", networkName)
					})

					By("Verifying VF status with pass traffic")
					err = sriovenv.CheckVFStatusWithPassTraffic(APIClient, SriovOcpConfig, networkName, data.InterfaceName, ns1, "spoof checking on", tsparams.PodReadyTimeout)
					// Handle NO-CARRIER status as a skip condition
					if err != nil && strings.Contains(err.Error(), "NO-CARRIER") {
						Skip("Interface has NO-CARRIER status - skipping connectivity test for interface without physical connection")
					}
					Expect(err).ToNot(HaveOccurred(), "VF status verification failed")
				}()
			}

			if !executed {
				Skip("No SR-IOV devices matched the requested configuration")
			}
		})

		It("SR-IOV VF with spoof checking disabled", reportxml.ID("70820"), func() {
			var caseID = "70820-"
			executed := false

			for _, data := range testData {
				data := data
				By(fmt.Sprintf("Testing device: %s (DeviceID: %s, Interface: %s)", data.Name, data.DeviceID, data.InterfaceName))

				// Create VF on with given device
				result, err := sriovenv.InitVF(APIClient, SriovOcpConfig, data.Name, data.DeviceID, data.InterfaceName, data.Vendor, sriovOpNs, vfNum, workerNodes)
				Expect(err).ToNot(HaveOccurred(), "Failed to initialize VF for device %q", data.Name)
				if !result {
					By(fmt.Sprintf("Skipping device %q - VF initialization failed or device not found on any node", data.Name))
					continue
				}

				executed = true
				func() {
					ns1 := "e2e-" + caseID + data.Name
					networkName := caseID + data.Name

					By(fmt.Sprintf("Creating test namespace %q", ns1))
					nsBuilder := namespace.NewBuilder(APIClient, ns1)
					for key, value := range params.PrivilegedNSLabels {
						nsBuilder.WithLabel(key, value)
					}
					_, err := nsBuilder.Create()
					Expect(err).ToNot(HaveOccurred(), "Failed to create namespace %q", ns1)

					Eventually(func() bool {
						return nsBuilder.Exists()
					}, tsparams.NamespaceTimeout, tsparams.RetryInterval).Should(BeTrue(), "Namespace %q should exist", ns1)

					DeferCleanup(func() {
						By(fmt.Sprintf("Cleaning up namespace %q", ns1))
						err := nsBuilder.DeleteAndWait(tsparams.CleanupTimeout)
						Expect(err).ToNot(HaveOccurred(), "Failed to delete namespace %q", ns1)
					})

					By("Creating SR-IOV network with spoof checking disabled")
					networkConfig := &sriovenv.SriovNetworkConfig{
						Name:             networkName,
						ResourceName:     data.Name,
						NetworkNamespace: ns1,
						Namespace:        sriovOpNs,
						SpoofCheck:       "off",
						Trust:            "on",
					}

					err = sriovenv.CreateSriovNetwork(APIClient, networkConfig, tsparams.WaitTimeout)
					Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV network %q", networkName)

					DeferCleanup(func() {
						By(fmt.Sprintf("Cleaning up SR-IOV network %q", networkName))
						err := sriovenv.RemoveSriovNetwork(APIClient, networkName, sriovOpNs, tsparams.DefaultTimeout)
						Expect(err).ToNot(HaveOccurred(), "Failed to remove SR-IOV network %q", networkName)
					})

					By("Verifying VF status with pass traffic")
					err = sriovenv.CheckVFStatusWithPassTraffic(APIClient, SriovOcpConfig, networkName, data.InterfaceName, ns1, "spoof checking off", tsparams.PodReadyTimeout)
					if err != nil && strings.Contains(err.Error(), "NO-CARRIER") {
						Skip("Interface has NO-CARRIER status - skipping connectivity test for interface without physical connection")
					}
					Expect(err).ToNot(HaveOccurred(), "VF status verification failed")
				}()
			}

			if !executed {
				Skip("No SR-IOV devices matched the requested configuration")
			}
		})

		It("SR-IOV VF with trust disabled", reportxml.ID("25960"), func() {
			var caseID = "25960-"
			executed := false

			for _, data := range testData {
				data := data
				By(fmt.Sprintf("Testing device: %s (DeviceID: %s, Interface: %s)", data.Name, data.DeviceID, data.InterfaceName))

				result, err := sriovenv.InitVF(APIClient, SriovOcpConfig, data.Name, data.DeviceID, data.InterfaceName, data.Vendor, sriovOpNs, vfNum, workerNodes)
				Expect(err).ToNot(HaveOccurred(), "Failed to initialize VF for device %q", data.Name)
				if !result {
					By(fmt.Sprintf("Skipping device %q - VF initialization failed or device not found on any node", data.Name))
					continue
				}

				executed = true
				func() {
					ns1 := "e2e-" + caseID + data.Name
					networkName := caseID + data.Name

					By(fmt.Sprintf("Creating test namespace %q", ns1))
					nsBuilder := namespace.NewBuilder(APIClient, ns1)
					for key, value := range params.PrivilegedNSLabels {
						nsBuilder.WithLabel(key, value)
					}
					_, err := nsBuilder.Create()
					Expect(err).ToNot(HaveOccurred(), "Failed to create namespace %q", ns1)

					Eventually(func() bool {
						return nsBuilder.Exists()
					}, tsparams.NamespaceTimeout, tsparams.RetryInterval).Should(BeTrue(), "Namespace %q should exist", ns1)

					DeferCleanup(func() {
						By(fmt.Sprintf("Cleaning up namespace %q", ns1))
						err := nsBuilder.DeleteAndWait(tsparams.CleanupTimeout)
						Expect(err).ToNot(HaveOccurred(), "Failed to delete namespace %q", ns1)
					})

					By("Creating SR-IOV network with trust disabled")
					networkConfig := &sriovenv.SriovNetworkConfig{
						Name:             networkName,
						ResourceName:     data.Name,
						NetworkNamespace: ns1,
						Namespace:        sriovOpNs,
						SpoofCheck:       "off",
						Trust:            "off",
					}

					err = sriovenv.CreateSriovNetwork(APIClient, networkConfig, tsparams.WaitTimeout)
					Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV network %q", networkName)

					DeferCleanup(func() {
						By(fmt.Sprintf("Cleaning up SR-IOV network %q", networkName))
						err := sriovenv.RemoveSriovNetwork(APIClient, networkName, sriovOpNs, tsparams.DefaultTimeout)
						Expect(err).ToNot(HaveOccurred(), "Failed to remove SR-IOV network %q", networkName)
					})

					By("Verifying VF status with pass traffic")
					err = sriovenv.CheckVFStatusWithPassTraffic(APIClient, SriovOcpConfig, networkName, data.InterfaceName, ns1, "trust off", tsparams.PodReadyTimeout)
					if err != nil && strings.Contains(err.Error(), "NO-CARRIER") {
						Skip("Interface has NO-CARRIER status - skipping connectivity test for interface without physical connection")
					}
					Expect(err).ToNot(HaveOccurred(), "VF status verification failed")
				}()
			}

			if !executed {
				Skip("No SR-IOV devices matched the requested configuration")
			}
		})

		It("SR-IOV VF with trust enabled", reportxml.ID("70821"), func() {
			var caseID = "70821-"
			executed := false

			for _, data := range testData {
				data := data
				By(fmt.Sprintf("Testing device: %s (DeviceID: %s, Interface: %s)", data.Name, data.DeviceID, data.InterfaceName))

				result, err := sriovenv.InitVF(APIClient, SriovOcpConfig, data.Name, data.DeviceID, data.InterfaceName, data.Vendor, sriovOpNs, vfNum, workerNodes)
				Expect(err).ToNot(HaveOccurred(), "Failed to initialize VF for device %q", data.Name)
				if !result {
					By(fmt.Sprintf("Skipping device %q - VF initialization failed or device not found on any node", data.Name))
					continue
				}

				executed = true
				func() {
					ns1 := "e2e-" + caseID + data.Name
					networkName := caseID + data.Name

					By(fmt.Sprintf("Creating test namespace %q", ns1))
					nsBuilder := namespace.NewBuilder(APIClient, ns1)
					for key, value := range params.PrivilegedNSLabels {
						nsBuilder.WithLabel(key, value)
					}
					_, err := nsBuilder.Create()
					Expect(err).ToNot(HaveOccurred(), "Failed to create namespace %q", ns1)

					Eventually(func() bool {
						return nsBuilder.Exists()
					}, tsparams.NamespaceTimeout, tsparams.RetryInterval).Should(BeTrue(), "Namespace %q should exist", ns1)

					DeferCleanup(func() {
						By(fmt.Sprintf("Cleaning up namespace %q", ns1))
						err := nsBuilder.DeleteAndWait(tsparams.CleanupTimeout)
						Expect(err).ToNot(HaveOccurred(), "Failed to delete namespace %q", ns1)
					})

					By("Creating SR-IOV network with trust enabled")
					networkConfig := &sriovenv.SriovNetworkConfig{
						Name:             networkName,
						ResourceName:     data.Name,
						NetworkNamespace: ns1,
						Namespace:        sriovOpNs,
						SpoofCheck:       "on",
						Trust:            "on",
					}

					err = sriovenv.CreateSriovNetwork(APIClient, networkConfig, tsparams.WaitTimeout)
					Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV network %q", networkName)

					DeferCleanup(func() {
						By(fmt.Sprintf("Cleaning up SR-IOV network %q", networkName))
						err := sriovenv.RemoveSriovNetwork(APIClient, networkName, sriovOpNs, tsparams.DefaultTimeout)
						Expect(err).ToNot(HaveOccurred(), "Failed to remove SR-IOV network %q", networkName)
					})

					By("Verifying VF status with pass traffic")
					err = sriovenv.CheckVFStatusWithPassTraffic(APIClient, SriovOcpConfig, networkName, data.InterfaceName, ns1, "trust on", tsparams.PodReadyTimeout)
					if err != nil && strings.Contains(err.Error(), "NO-CARRIER") {
						Skip("Interface has NO-CARRIER status - skipping connectivity test for interface without physical connection")
					}
					Expect(err).ToNot(HaveOccurred(), "VF status verification failed")
				}()
			}

			if !executed {
				Skip("No SR-IOV devices matched the requested configuration")
			}
		})

		It("SR-IOV VF with VLAN and rate limiting configuration", reportxml.ID("25963"), func() {
			var caseID = "25963-"
			executed := false

			for _, data := range testData {
				data := data
				By(fmt.Sprintf("Testing device: %s (DeviceID: %s, Interface: %s)", data.Name, data.DeviceID, data.InterfaceName))

				// x710, bcm57414, and bcm57508 do not support minTxRate for now
				if data.Name == "x710" || data.Name == "bcm57414" || data.Name == "bcm57508" {
					By(fmt.Sprintf("Skipping device %q - does not support minTxRate", data.Name))
					continue
				}

				result, err := sriovenv.InitVF(APIClient, SriovOcpConfig, data.Name, data.DeviceID, data.InterfaceName, data.Vendor, sriovOpNs, vfNum, workerNodes)
				Expect(err).ToNot(HaveOccurred(), "Failed to initialize VF for device %q", data.Name)
				if !result {
					By(fmt.Sprintf("Skipping device %q - VF initialization failed or device not found on any node", data.Name))
					continue
				}

				executed = true
				func() {
					ns1 := "e2e-" + caseID + data.Name
					networkName := caseID + data.Name

					By(fmt.Sprintf("Creating test namespace %q", ns1))
					nsBuilder := namespace.NewBuilder(APIClient, ns1)
					for key, value := range params.PrivilegedNSLabels {
						nsBuilder.WithLabel(key, value)
					}
					_, err := nsBuilder.Create()
					Expect(err).ToNot(HaveOccurred(), "Failed to create namespace %q", ns1)

					Eventually(func() bool {
						return nsBuilder.Exists()
					}, tsparams.NamespaceTimeout, tsparams.RetryInterval).Should(BeTrue(), "Namespace %q should exist", ns1)

					DeferCleanup(func() {
						By(fmt.Sprintf("Cleaning up namespace %q", ns1))
						err := nsBuilder.DeleteAndWait(tsparams.CleanupTimeout)
						Expect(err).ToNot(HaveOccurred(), "Failed to delete namespace %q", ns1)
					})

					By("Creating SR-IOV network with VLAN and rate limiting")
					networkConfig := &sriovenv.SriovNetworkConfig{
						Name:             networkName,
						ResourceName:     data.Name,
						NetworkNamespace: ns1,
						Namespace:        sriovOpNs,
						VlanID:           100,
						VlanQoS:          2,
						MinTxRate:        40,
						MaxTxRate:        100,
					}

					err = sriovenv.CreateSriovNetwork(APIClient, networkConfig, tsparams.WaitTimeout)
					Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV network %q", networkName)

					DeferCleanup(func() {
						By(fmt.Sprintf("Cleaning up SR-IOV network %q", networkName))
						err := sriovenv.RemoveSriovNetwork(APIClient, networkName, sriovOpNs, tsparams.DefaultTimeout)
						Expect(err).ToNot(HaveOccurred(), "Failed to remove SR-IOV network %q", networkName)
					})

					By("Verifying VF status with pass traffic")
					err = sriovenv.CheckVFStatusWithPassTraffic(APIClient, SriovOcpConfig, networkName, data.InterfaceName, ns1, "vlan 100, qos 2, tx rate 100 (Mbps), max_tx_rate 100Mbps, min_tx_rate 40Mbps", tsparams.PodReadyTimeout)
					if err != nil && strings.Contains(err.Error(), "NO-CARRIER") {
						Skip("Interface has NO-CARRIER status - skipping connectivity test for interface without physical connection")
					}
					Expect(err).ToNot(HaveOccurred(), "VF status verification failed")
				}()
			}

			if !executed {
				Skip("No SR-IOV devices matched the requested configuration")
			}
		})

		It("SR-IOV VF with auto link state", reportxml.ID("25961"), func() {
			var caseID = "25961-"
			executed := false

			for _, data := range testData {
				data := data
				By(fmt.Sprintf("Testing device: %s (DeviceID: %s, Interface: %s)", data.Name, data.DeviceID, data.InterfaceName))

				result, err := sriovenv.InitVF(APIClient, SriovOcpConfig, data.Name, data.DeviceID, data.InterfaceName, data.Vendor, sriovOpNs, vfNum, workerNodes)
				Expect(err).ToNot(HaveOccurred(), "Failed to initialize VF for device %q", data.Name)
				if !result {
					By(fmt.Sprintf("Skipping device %q - VF initialization failed or device not found on any node", data.Name))
					continue
				}

				executed = true
				func() {
					ns1 := "e2e-" + caseID + data.Name
					networkName := caseID + data.Name

					By(fmt.Sprintf("Creating test namespace %q", ns1))
					nsBuilder := namespace.NewBuilder(APIClient, ns1)
					for key, value := range params.PrivilegedNSLabels {
						nsBuilder.WithLabel(key, value)
					}
					_, err := nsBuilder.Create()
					Expect(err).ToNot(HaveOccurred(), "Failed to create namespace %q", ns1)

					Eventually(func() bool {
						return nsBuilder.Exists()
					}, tsparams.NamespaceTimeout, tsparams.RetryInterval).Should(BeTrue(), "Namespace %q should exist", ns1)

					DeferCleanup(func() {
						By(fmt.Sprintf("Cleaning up namespace %q", ns1))
						err := nsBuilder.DeleteAndWait(tsparams.CleanupTimeout)
						Expect(err).ToNot(HaveOccurred(), "Failed to delete namespace %q", ns1)
					})

					By("Creating SR-IOV network with auto link state")
					networkConfig := &sriovenv.SriovNetworkConfig{
						Name:             networkName,
						ResourceName:     data.Name,
						NetworkNamespace: ns1,
						Namespace:        sriovOpNs,
						LinkState:        "auto",
					}

					err = sriovenv.CreateSriovNetwork(APIClient, networkConfig, tsparams.WaitTimeout)
					Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV network %q", networkName)

					DeferCleanup(func() {
						By(fmt.Sprintf("Cleaning up SR-IOV network %q", networkName))
						err := sriovenv.RemoveSriovNetwork(APIClient, networkName, sriovOpNs, tsparams.DefaultTimeout)
						Expect(err).ToNot(HaveOccurred(), "Failed to remove SR-IOV network %q", networkName)
					})

					By("Verifying VF status with pass traffic")
					err = sriovenv.CheckVFStatusWithPassTraffic(APIClient, SriovOcpConfig, networkName, data.InterfaceName, ns1, "link-state auto", tsparams.PodReadyTimeout)
					if err != nil && strings.Contains(err.Error(), "NO-CARRIER") {
						Skip("Interface has NO-CARRIER status - skipping connectivity test for interface without physical connection")
					}
					Expect(err).ToNot(HaveOccurred(), "VF status verification failed")
				}()
			}

			if !executed {
				Skip("No SR-IOV devices matched the requested configuration")
			}
		})

		It("SR-IOV VF with enabled link state", reportxml.ID("71006"), func() {
			var caseID = "71006-"
			executed := false

			for _, data := range testData {
				data := data
				By(fmt.Sprintf("Testing device: %s (DeviceID: %s, Interface: %s)", data.Name, data.DeviceID, data.InterfaceName))

				result, err := sriovenv.InitVF(APIClient, SriovOcpConfig, data.Name, data.DeviceID, data.InterfaceName, data.Vendor, sriovOpNs, vfNum, workerNodes)
				Expect(err).ToNot(HaveOccurred(), "Failed to initialize VF for device %q", data.Name)
				if !result {
					By(fmt.Sprintf("Skipping device %q - VF initialization failed or device not found on any node", data.Name))
					continue
				}

				executed = true
				func() {
					ns1 := "e2e-" + caseID + data.Name
					networkName := caseID + data.Name

					By(fmt.Sprintf("Creating test namespace %q", ns1))
					nsBuilder := namespace.NewBuilder(APIClient, ns1)
					for key, value := range params.PrivilegedNSLabels {
						nsBuilder.WithLabel(key, value)
					}
					_, err := nsBuilder.Create()
					Expect(err).ToNot(HaveOccurred(), "Failed to create namespace %q", ns1)

					Eventually(func() bool {
						return nsBuilder.Exists()
					}, tsparams.NamespaceTimeout, tsparams.RetryInterval).Should(BeTrue(), "Namespace %q should exist", ns1)

					DeferCleanup(func() {
						By(fmt.Sprintf("Cleaning up namespace %q", ns1))
						err := nsBuilder.DeleteAndWait(tsparams.CleanupTimeout)
						Expect(err).ToNot(HaveOccurred(), "Failed to delete namespace %q", ns1)
					})

					By("Creating SR-IOV network with enabled link state")
					networkConfig := &sriovenv.SriovNetworkConfig{
						Name:             networkName,
						ResourceName:     data.Name,
						NetworkNamespace: ns1,
						Namespace:        sriovOpNs,
						LinkState:        "enable",
					}

					err = sriovenv.CreateSriovNetwork(APIClient, networkConfig, tsparams.WaitTimeout)
					Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV network %q", networkName)

					DeferCleanup(func() {
						By(fmt.Sprintf("Cleaning up SR-IOV network %q", networkName))
						err := sriovenv.RemoveSriovNetwork(APIClient, networkName, sriovOpNs, tsparams.DefaultTimeout)
						Expect(err).ToNot(HaveOccurred(), "Failed to remove SR-IOV network %q", networkName)
					})

					By("Verifying VF status with pass traffic")
					err = sriovenv.CheckVFStatusWithPassTraffic(APIClient, SriovOcpConfig, networkName, data.InterfaceName, ns1, "link-state enable", tsparams.PodReadyTimeout)
					if err != nil && strings.Contains(err.Error(), "NO-CARRIER") {
						Skip("Interface has NO-CARRIER status - skipping connectivity test for interface without physical connection")
					}
					Expect(err).ToNot(HaveOccurred(), "VF status verification failed")
				}()
			}

			if !executed {
				Skip("No SR-IOV devices matched the requested configuration")
			}
		})

		It("MTU configuration for SR-IOV policy", reportxml.ID("69646"), func() {
			var caseID = "69646-"
			executed := false

			for _, data := range testData {
				data := data
				By(fmt.Sprintf("Testing device: %s (DeviceID: %s, Interface: %s)", data.Name, data.DeviceID, data.InterfaceName))

				// Create VF on with given device
				result, err := sriovenv.InitVF(APIClient, SriovOcpConfig, data.Name, data.DeviceID, data.InterfaceName, data.Vendor, sriovOpNs, vfNum, workerNodes)
				Expect(err).ToNot(HaveOccurred(), "Failed to initialize VF for device %q", data.Name)
				if !result {
					By(fmt.Sprintf("Skipping device %q - VF initialization failed or device not found on any node", data.Name))
					continue
				}

				executed = true

				// Configure MTU in SR-IOV policy
				mtuValue := 1800
				By(fmt.Sprintf("Updating SR-IOV policy %q with MTU %d", data.Name, mtuValue))
				err = sriovenv.UpdateSriovPolicyMTU(APIClient, data.Name, sriovOpNs, mtuValue)
				Expect(err).ToNot(HaveOccurred(), "Failed to update SR-IOV policy %q with MTU", data.Name)

				By("Waiting for SR-IOV policy to be ready after MTU update")
				err = sriovenv.WaitForSriovPolicyReady(APIClient, SriovOcpConfig, tsparams.DefaultTimeout)
				Expect(err).ToNot(HaveOccurred(), "Failed to wait for SR-IOV policy to be ready after MTU update")

				func() {
					ns1 := "e2e-" + caseID + data.Name
					networkName := caseID + data.Name

					By(fmt.Sprintf("Creating test namespace %q", ns1))
					nsBuilder := namespace.NewBuilder(APIClient, ns1)
					for key, value := range params.PrivilegedNSLabels {
						nsBuilder.WithLabel(key, value)
					}
					_, err := nsBuilder.Create()
					Expect(err).ToNot(HaveOccurred(), "Failed to create namespace %q", ns1)

					Eventually(func() bool {
						return nsBuilder.Exists()
					}, tsparams.NamespaceTimeout, tsparams.RetryInterval).Should(BeTrue(), "Namespace %q should exist", ns1)

					DeferCleanup(func() {
						By(fmt.Sprintf("Cleaning up namespace %q", ns1))
						err := nsBuilder.DeleteAndWait(tsparams.CleanupTimeout)
						Expect(err).ToNot(HaveOccurred(), "Failed to delete namespace %q", ns1)
					})

					By("Creating SR-IOV network with MTU configuration")
					networkConfig := &sriovenv.SriovNetworkConfig{
						Name:             networkName,
						ResourceName:     data.Name,
						NetworkNamespace: ns1,
						Namespace:        sriovOpNs,
						SpoofCheck:       "on",
						Trust:            "on",
					}

					err = sriovenv.CreateSriovNetwork(APIClient, networkConfig, tsparams.WaitTimeout)
					Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV network %q", networkName)

					DeferCleanup(func() {
						By(fmt.Sprintf("Cleaning up SR-IOV network %q", networkName))
						err := sriovenv.RemoveSriovNetwork(APIClient, networkName, sriovOpNs, tsparams.DefaultTimeout)
						Expect(err).ToNot(HaveOccurred(), "Failed to remove SR-IOV network %q", networkName)
					})

					By("Verifying VF status with pass traffic")
					err = sriovenv.CheckVFStatusWithPassTraffic(APIClient, SriovOcpConfig, networkName, data.InterfaceName, ns1, fmt.Sprintf("mtu %d", mtuValue), tsparams.PodReadyTimeout)
					if err != nil && strings.Contains(err.Error(), "NO-CARRIER") {
						Skip("Interface has NO-CARRIER status - skipping connectivity test for interface without physical connection")
					}
					Expect(err).ToNot(HaveOccurred(), "VF status verification failed")
				}()
			}

			if !executed {
				Skip("No SR-IOV devices matched the requested configuration")
			}
		})

		It("DPDK SR-IOV VF functionality validation", reportxml.ID("69582"), func() {
			var caseID = "69582-"
			executed := false

			for _, data := range testData {
				data := data
				By(fmt.Sprintf("Testing device: %s (DeviceID: %s, Interface: %s)", data.Name, data.DeviceID, data.InterfaceName))

				// Skip BCM NICs: OCPBUGS-30909
				if strings.Contains(data.Name, "bcm") {
					By(fmt.Sprintf("Skipping device %q - BCM NICs not supported for DPDK", data.Name))
					continue
				}

				// Create DPDK VF on with given device
				policyName := data.Name
				networkName := data.Name + "dpdk" + "net"
				result, err := sriovenv.InitDpdkVF(APIClient, SriovOcpConfig, data.Name, data.DeviceID, data.InterfaceName, data.Vendor, sriovOpNs, vfNum, workerNodes)
				Expect(err).ToNot(HaveOccurred(), "Failed to initialize DPDK VF for device %q", data.Name)
				if !result {
					By(fmt.Sprintf("Skipping device %q - DPDK VF initialization failed or device not found on any node", data.Name))
					continue
				}

				executed = true
				func() {
					ns1 := "e2e-" + caseID + data.Name

					By(fmt.Sprintf("Creating test namespace %q", ns1))
					nsBuilder := namespace.NewBuilder(APIClient, ns1)
					for key, value := range params.PrivilegedNSLabels {
						nsBuilder.WithLabel(key, value)
					}
					_, err := nsBuilder.Create()
					Expect(err).ToNot(HaveOccurred(), "Failed to create namespace %q", ns1)

					Eventually(func() bool {
						return nsBuilder.Exists()
					}, tsparams.NamespaceTimeout, tsparams.RetryInterval).Should(BeTrue(), "Namespace %q should exist", ns1)

					DeferCleanup(func() {
						By(fmt.Sprintf("Cleaning up namespace %q", ns1))
						err := nsBuilder.DeleteAndWait(tsparams.CleanupTimeout)
						Expect(err).ToNot(HaveOccurred(), "Failed to delete namespace %q", ns1)
					})

					By("Creating SR-IOV network for DPDK")
					networkConfig := &sriovenv.SriovNetworkConfig{
						Name:             networkName,
						ResourceName:     policyName,
						NetworkNamespace: ns1,
						Namespace:        sriovOpNs,
					}

					err = sriovenv.CreateSriovNetwork(APIClient, networkConfig, tsparams.WaitTimeout)
					Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV network %q", networkName)

					DeferCleanup(func() {
						By(fmt.Sprintf("Cleaning up SR-IOV network %q", networkName))
						err := sriovenv.RemoveSriovNetwork(APIClient, networkName, sriovOpNs, tsparams.DefaultTimeout)
						Expect(err).ToNot(HaveOccurred(), "Failed to remove SR-IOV network %q", networkName)
					})

					// Wait for NAD to be fully ready before creating pods
					By("Waiting for NetworkAttachmentDefinition to be fully ready")
					Eventually(func() error {
						_, err := nad.Pull(APIClient, networkName, ns1)
						return err
					}, tsparams.NamespaceTimeout, tsparams.RetryInterval).ShouldNot(HaveOccurred(), "NAD %q should be ready in namespace %q", networkName, ns1)

					// Create DPDK test pod
					By("Creating DPDK test pod")
					_, err = sriovenv.CreateDpdkTestPod(APIClient, SriovOcpConfig, "sriovdpdk", ns1, networkName)
					Expect(err).ToNot(HaveOccurred(), "Failed to create DPDK test pod")

					DeferCleanup(func() {
						By("Cleaning up DPDK test pod")
						err := sriovenv.DeleteDpdkTestPod(APIClient, "sriovdpdk", ns1, tsparams.NamespaceTimeout)
						Expect(err).ToNot(HaveOccurred(), "Failed to delete DPDK test pod")
					})

					// Wait for pod to be ready
					By("Waiting for DPDK test pod to be ready")
					err = sriovenv.WaitForPodWithLabelReady(APIClient, ns1, "name=sriov-dpdk", tsparams.PodReadyTimeout)
					Expect(err).ToNot(HaveOccurred(), "DPDK test pod not ready")

					// Verify PCI address is assigned
					By("Verifying PCI address is assigned to DPDK pod")
					pciAddress, err := sriovenv.GetPciAddress(APIClient, SriovOcpConfig, ns1, "sriovdpdk", policyName)
					Expect(err).ToNot(HaveOccurred(), "Failed to get PCI address for DPDK pod")
					Expect(pciAddress).NotTo(Equal("0000:00:00.0"), "PCI address should be assigned from pod network status")

					// Verify DPDK VF is available in pod
					By("Verifying DPDK VF is available in pod")
					podBuilder, err := pod.Pull(APIClient, "sriovdpdk", ns1)
					Expect(err).ToNot(HaveOccurred(), "Failed to pull DPDK pod")
					Expect(podBuilder).NotTo(BeNil(), "Pod builder should not be nil")
					Expect(podBuilder.Object).NotTo(BeNil(), "Pod object should not be nil")

					// Check if the pod has the network status annotation with PCI address
					networkStatusAnnotation := "k8s.v1.cni.cncf.io/network-status"
					podNetAnnotation := podBuilder.Object.Annotations[networkStatusAnnotation]
					Expect(podNetAnnotation).NotTo(BeEmpty(), "Pod should have network status annotation")
					Expect(podNetAnnotation).To(ContainSubstring(policyName), "Network status should contain policy name")
					Expect(podNetAnnotation).To(ContainSubstring("pci-address"), "Network status should contain PCI address")
					Expect(podNetAnnotation).To(ContainSubstring(pciAddress), "Network status should contain the assigned PCI address")
				}()
			}

			if !executed {
				Skip("No SR-IOV devices matched the requested configuration")
			}
		})
	})
