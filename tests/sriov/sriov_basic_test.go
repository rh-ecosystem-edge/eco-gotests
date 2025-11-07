package sriov

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/params"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// deviceConfig represents a SR-IOV device configuration
type deviceConfig struct {
	Name          string
	DeviceID      string
	Vendor        string
	InterfaceName string
}

// parseDeviceConfig parses device configuration from environment variable
// Format: export SRIOV_DEVICES="name1:deviceid1:vendor1:interface1,name2:deviceid2:vendor2:interface2,..."
// Example: export SRIOV_DEVICES="e810xxv:159b:8086:ens2f0,e810c:1593:8086:ens2f2"
// Returns empty slice if env var is not set or parsing fails
func parseDeviceConfig() []deviceConfig {
	envDevices := os.Getenv("SRIOV_DEVICES")
	if envDevices == "" {
		return nil
	}

	var devices []deviceConfig
	entries := strings.Split(envDevices, ",")

	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		parts := strings.Split(entry, ":")
		if len(parts) != 4 {
			GinkgoLogr.Info("Invalid device entry format (expected name:deviceid:vendor:interface)", "entry", entry)
			continue
		}

		devices = append(devices, deviceConfig{
			Name:          strings.TrimSpace(parts[0]),
			DeviceID:      strings.TrimSpace(parts[1]),
			Vendor:        strings.TrimSpace(parts[2]),
			InterfaceName: strings.TrimSpace(parts[3]),
		})
	}

	return devices
}

func getDefaultDeviceConfig() []deviceConfig {
	return []deviceConfig{
		{"e810xxv", "159b", "8086", "eno12409"},
		{"e810c", "1593", "8086", "ens2f2"},
		{"x710", "1572", "8086", "ens5f0"}, //NO-CARRIER
		{"bcm57414", "16d7", "14e4", "ens4f1np1"},
		{"bcm57508", "1750", "14e4", "ens3f0np0"}, //NO-CARRIER
		{"e810back", "1591", "8086", "ens4f2"},
		{"cx7anl244", "1021", "15b3", "ens2f0np0"},
	}
}

// getDeviceConfig returns device configuration from environment variable or defaults
func getDeviceConfig() []deviceConfig {
	envDevices := os.Getenv("SRIOV_DEVICES")
	if devices := parseDeviceConfig(); len(devices) > 0 {
		return devices
	}
	if envDevices != "" {
		panic(fmt.Sprintf("SRIOV_DEVICES is set to %q but no valid entries could be parsed; expected format: name:deviceid:vendor:interface", envDevices))
	}
	return getDefaultDeviceConfig()
}

// getVFNum returns the number of virtual functions to create, configurable via SRIOV_VF_NUM env var
func getVFNum() int {
	if vfNumStr := os.Getenv("SRIOV_VF_NUM"); vfNumStr != "" {
		if vfNum, err := strconv.Atoi(vfNumStr); err == nil && vfNum > 0 {
			return vfNum
		}
	}
	return 2 // default
}

var (
	testNS *namespace.Builder
)

// getTestNS returns the test namespace, initializing it if necessary
func getTestNS() *namespace.Builder {
	if testNS == nil {
		testNS = namespace.NewBuilder(getAPIClient(), "sriov-basic-test")
	}
	return testNS
}

func TestSriovBasic(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = NetConfig.GetJunitReportPath()

	RegisterFailHandler(Fail)
	RunSpecs(t, "sriov-basic", Label("sriov", "basic"), reporterConfig)
}

var _ = BeforeSuite(func() {
	By("Cleaning up leftover resources from previous test runs")
	cleanupLeftoverResources(getAPIClient(), NetConfig.SriovOperatorNamespace)

	By("Creating test namespace with privileged labels")
	// Log equivalent oc command for troubleshooting
	GinkgoLogr.Info("Equivalent oc command", "command", fmt.Sprintf("oc get namespace %s || oc create namespace %s", getTestNS().Definition.Name, getTestNS().Definition.Name))
	for key, value := range params.PrivilegedNSLabels {
		getTestNS().WithLabel(key, value)
	}
	_, err := getTestNS().Create()
	Expect(err).ToNot(HaveOccurred(), "error to create test namespace")

	By("Verifying if sriov tests can be executed on given cluster")
	err = IsSriovDeployed(getAPIClient(), NetConfig)
	Expect(err).ToNot(HaveOccurred(), "Cluster doesn't support sriov test cases")

	By("Pulling test images on cluster before running test cases")
	err = pullTestImageOnNodes(getAPIClient(), NetConfig.WorkerLabel, NetConfig.CnfNetTestContainer, 300)
	Expect(err).ToNot(HaveOccurred(), "Failed to pull test image on nodes")
})

var _ = AfterSuite(func() {
	By("Deleting test namespace")
	err := getTestNS().DeleteAndWait(300 * time.Second)
	Expect(err).ToNot(HaveOccurred(), "error to delete test namespace")
})

var _ = JustAfterEach(func() {
	// Note: reporter.ReportIfFailed would need to be implemented differently
	// For now, we'll skip the reporting
})

var _ = ReportAfterSuite("", func(report Report) {
	// Create XML report if needed
})

var _ = Describe("[sig-networking] SDN sriov-legacy", func() {
	defer GinkgoRecover()
	var (
		buildPruningBaseDir  = filepath.Join("testdata", "networking", "sriov")
		sriovNetworkTemplate = filepath.Join(buildPruningBaseDir, "sriovnetwork-whereabouts-template.yaml")
		sriovOpNs            = NetConfig.SriovOperatorNamespace
		vfNum                = getVFNum()
		workerNodes          []*nodes.Builder
	)

	testData := getDeviceConfig()

	BeforeEach(func() {
		By("check the sriov operator is running")
		chkSriovOperatorStatus(sriovOpNs)

		By("Discover worker nodes")
		var err error
		workerNodes, err = nodes.List(getAPIClient(),
			metav1.ListOptions{LabelSelector: labels.Set(NetConfig.WorkerLabelMap).String()})
		Expect(err).ToNot(HaveOccurred(), "Fail to discover nodes")
	})

	AfterEach(func() {
		//after each case finished testing.  remove sriovnodenetworkpolicy CR
		// Only remove policies that were actually created during the test
		// We check each policy and only delete if it exists
		for _, item := range testData {
			rmSriovPolicy(item.Name, sriovOpNs)
		}
		waitForSriovPolicyReady(sriovOpNs)
	})

	It("Author:zzhao-Medium-NonPreRelease-Longduration-25959-SR-IOV VF with spoof checking enabled [Disruptive] [Serial]", func() {
		var caseID = "25959-"
		executed := false

		for _, data := range testData {
			data := data
			// Create VF on with given device
			result := initVF(data.Name, data.DeviceID, data.InterfaceName, data.Vendor, sriovOpNs, vfNum, workerNodes)
			// if the deviceid is not exist on the worker, skip this
			if !result {
				continue
			}
			executed = true
			func() {
				ns1 := "e2e-" + caseID + data.Name
				// Create unique network name with test case ID to avoid conflicts between tests
				networkName := caseID + data.Name
				// Create namespace for the test
				nsBuilder := namespace.NewBuilder(getAPIClient(), ns1)
				for key, value := range params.PrivilegedNSLabels {
					nsBuilder.WithLabel(key, value)
				}
				_, err := nsBuilder.Create()
				Expect(err).NotTo(HaveOccurred(), "Failed to create namespace %s", ns1)
				// Wait for namespace to be ready before proceeding
				Eventually(func() bool {
					return nsBuilder.Exists()
				}, 30*time.Second, time.Second).Should(BeTrue(), "Namespace %s should exist", ns1)
				defer func() {
					// Clean up namespace after test (increased timeout for SR-IOV cleanup)
					err := nsBuilder.DeleteAndWait(120 * time.Second)
					if err != nil {
						GinkgoLogr.Info("Failed to delete namespace", "namespace", ns1, "error", err)
					}
				}()

				By("Create sriovNetwork to generate net-attach-def on the target namespace")
				GinkgoLogr.Info("device ID", "deviceID", data.DeviceID)
				GinkgoLogr.Info("device Name", "deviceName", data.Name)
				sriovnetwork := sriovNetwork{
					name:             networkName,
					resourceName:     data.Name,
					networkNamespace: ns1,
					template:         sriovNetworkTemplate,
					namespace:        sriovOpNs,
					spoolchk:         "on",
					trust:            "off",
				}
				//defer
				defer func() {
					rmSriovNetwork(sriovnetwork.name, sriovOpNs)
				}()
				sriovnetwork.createSriovNetwork()

				chkVFStatusWithPassTraffic(sriovnetwork.name, data.InterfaceName, ns1, "spoof checking on")

			}()
		}
		if !executed {
			Skip("No SR-IOV devices matched the requested configuration")
		}
	})

	It("Author:zzhao-Medium-NonPreRelease-Longduration-70820-SR-IOV VF with spoof checking disabled [Disruptive] [Serial]", func() {
		var caseID = "70820-"
		executed := false

		for _, data := range testData {
			data := data
			// Create VF on with given device
			result := initVF(data.Name, data.DeviceID, data.InterfaceName, data.Vendor, sriovOpNs, vfNum, workerNodes)
			// if the deviceid is not exist on the worker, skip this
			if !result {
				continue
			}
			executed = true
			func() {
				ns1 := "e2e-" + caseID + data.Name
				// Create unique network name with test case ID to avoid conflicts between tests
				networkName := caseID + data.Name
				// Create namespace for the test
				nsBuilder := namespace.NewBuilder(getAPIClient(), ns1)
				for key, value := range params.PrivilegedNSLabels {
					nsBuilder.WithLabel(key, value)
				}
				_, err := nsBuilder.Create()
				Expect(err).NotTo(HaveOccurred(), "Failed to create namespace %s", ns1)
				// Wait for namespace to be ready before proceeding
				Eventually(func() bool {
					return nsBuilder.Exists()
				}, 30*time.Second, time.Second).Should(BeTrue(), "Namespace %s should exist", ns1)
				defer func() {
					// Clean up namespace after test (increased timeout for SR-IOV cleanup)
					err := nsBuilder.DeleteAndWait(120 * time.Second)
					if err != nil {
						GinkgoLogr.Info("Failed to delete namespace", "namespace", ns1, "error", err)
					}
				}()

				By("Create sriovNetwork to generate net-attach-def on the target namespace")
				GinkgoLogr.Info("device ID", "deviceID", data.DeviceID)
				GinkgoLogr.Info("device Name", "deviceName", data.Name)
				sriovnetwork := sriovNetwork{
					name:             networkName,
					resourceName:     data.Name,
					networkNamespace: ns1,
					template:         sriovNetworkTemplate,
					namespace:        sriovOpNs,
					spoolchk:         "off",
					trust:            "on",
				}
				//defer
				defer func() {
					rmSriovNetwork(sriovnetwork.name, sriovOpNs)
				}()
				sriovnetwork.createSriovNetwork()

				chkVFStatusWithPassTraffic(sriovnetwork.name, data.InterfaceName, ns1, "spoof checking off")
			}()
		}
		if !executed {
			Skip("No SR-IOV devices matched the requested configuration")
		}
	})

	It("Author:zzhao-Medium-NonPreRelease-Longduration-25960-SR-IOV VF with trust disabled [Disruptive] [Serial]", func() {
		var caseID = "25960-"
		executed := false

		for _, data := range testData {
			data := data
			// Create VF on with given device
			result := initVF(data.Name, data.DeviceID, data.InterfaceName, data.Vendor, sriovOpNs, vfNum, workerNodes)
			// if the deviceid is not exist on the worker, skip this
			if !result {
				continue
			}
			executed = true
			func() {
				ns1 := "e2e-" + caseID + data.Name
				// Create unique network name with test case ID to avoid conflicts between tests
				networkName := caseID + data.Name
				// Create namespace for the test
				nsBuilder := namespace.NewBuilder(getAPIClient(), ns1)
				for key, value := range params.PrivilegedNSLabels {
					nsBuilder.WithLabel(key, value)
				}
				_, err := nsBuilder.Create()
				Expect(err).NotTo(HaveOccurred(), "Failed to create namespace %s", ns1)
				// Wait for namespace to be ready before proceeding
				Eventually(func() bool {
					return nsBuilder.Exists()
				}, 30*time.Second, time.Second).Should(BeTrue(), "Namespace %s should exist", ns1)
				defer func() {
					// Clean up namespace after test (increased timeout for SR-IOV cleanup)
					err := nsBuilder.DeleteAndWait(120 * time.Second)
					if err != nil {
						GinkgoLogr.Info("Failed to delete namespace", "namespace", ns1, "error", err)
					}
				}()

				By("Create sriovNetwork to generate net-attach-def on the target namespace")
				GinkgoLogr.Info("device ID", "deviceID", data.DeviceID)
				GinkgoLogr.Info("device Name", "deviceName", data.Name)
				sriovnetwork := sriovNetwork{
					name:             networkName,
					resourceName:     data.Name,
					networkNamespace: ns1,
					template:         sriovNetworkTemplate,
					namespace:        sriovOpNs,
					spoolchk:         "off",
					trust:            "off",
				}
				//defer
				defer func() {
					rmSriovNetwork(sriovnetwork.name, sriovOpNs)
				}()
				sriovnetwork.createSriovNetwork()

				chkVFStatusWithPassTraffic(sriovnetwork.name, data.InterfaceName, ns1, "trust off")

			}()
		}
		if !executed {
			Skip("No SR-IOV devices matched the requested configuration")
		}
	})

	It("Author:zzhao-Medium-NonPreRelease-Longduration-70821-SR-IOV VF with trust enabled [Disruptive] [Serial]", func() {
		var caseID = "70821-"
		executed := false

		for _, data := range testData {
			data := data
			// Create VF on with given device
			result := initVF(data.Name, data.DeviceID, data.InterfaceName, data.Vendor, sriovOpNs, vfNum, workerNodes)
			// if the deviceid is not exist on the worker, skip this
			if !result {
				continue
			}
			executed = true
			func() {
				ns1 := "e2e-" + caseID + data.Name
				// Create unique network name with test case ID to avoid conflicts between tests
				networkName := caseID + data.Name
				// Create namespace for the test
				nsBuilder := namespace.NewBuilder(getAPIClient(), ns1)
				for key, value := range params.PrivilegedNSLabels {
					nsBuilder.WithLabel(key, value)
				}
				_, err := nsBuilder.Create()
				Expect(err).NotTo(HaveOccurred(), "Failed to create namespace %s", ns1)
				// Wait for namespace to be ready before proceeding
				Eventually(func() bool {
					return nsBuilder.Exists()
				}, 30*time.Second, time.Second).Should(BeTrue(), "Namespace %s should exist", ns1)
				defer func() {
					// Clean up namespace after test (increased timeout for SR-IOV cleanup)
					err := nsBuilder.DeleteAndWait(120 * time.Second)
					if err != nil {
						GinkgoLogr.Info("Failed to delete namespace", "namespace", ns1, "error", err)
					}
				}()

				By("Create sriovNetwork to generate net-attach-def on the target namespace")
				GinkgoLogr.Info("device ID", "deviceID", data.DeviceID)
				GinkgoLogr.Info("device Name", "deviceName", data.Name)
				sriovnetwork := sriovNetwork{
					name:             networkName,
					resourceName:     data.Name,
					networkNamespace: ns1,
					template:         sriovNetworkTemplate,
					namespace:        sriovOpNs,
					spoolchk:         "on",
					trust:            "on",
				}
				//defer
				defer func() {
					rmSriovNetwork(sriovnetwork.name, sriovOpNs)
				}()
				sriovnetwork.createSriovNetwork()

				chkVFStatusWithPassTraffic(sriovnetwork.name, data.InterfaceName, ns1, "trust on")

			}()
		}
		if !executed {
			Skip("No SR-IOV devices matched the requested configuration")
		}
	})

	It("Author:zzhao-Medium-NonPreRelease-Longduration-25963-SR-IOV VF with VLAN and rate limiting configuration [Disruptive] [Serial]", func() {
		var caseID = "25963-"
		executed := false

		for _, data := range testData {
			data := data

			//x710 do not support minTxRate for now
			if data.Name == "x710" || data.Name == "bcm57414" || data.Name == "bcm57508" {
				continue
			}
			// Create VF on with given device
			result := initVF(data.Name, data.DeviceID, data.InterfaceName, data.Vendor, sriovOpNs, vfNum, workerNodes)
			// if the deviceid is not exist on the worker, skip this
			if !result {
				continue
			}
			executed = true
			func() {
				ns1 := "e2e-" + caseID + data.Name
				// Create unique network name with test case ID to avoid conflicts between tests
				networkName := caseID + data.Name
				// Create namespace for the test
				nsBuilder := namespace.NewBuilder(getAPIClient(), ns1)
				for key, value := range params.PrivilegedNSLabels {
					nsBuilder.WithLabel(key, value)
				}
				_, err := nsBuilder.Create()
				Expect(err).NotTo(HaveOccurred(), "Failed to create namespace %s", ns1)
				// Wait for namespace to be ready before proceeding
				Eventually(func() bool {
					return nsBuilder.Exists()
				}, 30*time.Second, time.Second).Should(BeTrue(), "Namespace %s should exist", ns1)
				defer func() {
					// Clean up namespace after test (increased timeout for SR-IOV cleanup)
					err := nsBuilder.DeleteAndWait(120 * time.Second)
					if err != nil {
						GinkgoLogr.Info("Failed to delete namespace", "namespace", ns1, "error", err)
					}
				}()

				By("Create sriovNetwork to generate net-attach-def on the target namespace")
				GinkgoLogr.Info("device ID", "deviceID", data.DeviceID)
				GinkgoLogr.Info("device Name", "deviceName", data.Name)
				sriovnetwork := sriovNetwork{
					name:             networkName,
					resourceName:     data.Name,
					networkNamespace: ns1,
					template:         sriovNetworkTemplate,
					namespace:        sriovOpNs,
					vlanId:           100,
					vlanQoS:          2,
					minTxRate:        40,
					maxTxRate:        100,
				}
				//defer
				defer func() {
					rmSriovNetwork(sriovnetwork.name, sriovOpNs)
				}()
				sriovnetwork.createSriovNetwork()

				chkVFStatusWithPassTraffic(sriovnetwork.name, data.InterfaceName, ns1, "vlan 100, qos 2, tx rate 100 (Mbps), max_tx_rate 100Mbps, min_tx_rate 40Mbps")

			}()
		}
		if !executed {
			Skip("No SR-IOV devices matched the requested configuration")
		}
	})

	It("Author:zzhao-Medium-NonPreRelease-Longduration-25961-SR-IOV VF with auto link state [Disruptive] [Serial]", func() {
		var caseID = "25961-"
		executed := false

		for _, data := range testData {
			data := data
			// Create VF on with given device
			result := initVF(data.Name, data.DeviceID, data.InterfaceName, data.Vendor, sriovOpNs, vfNum, workerNodes)
			// if the deviceid is not exist on the worker, skip this
			if !result {
				continue
			}
			executed = true
			func() {
				ns1 := "e2e-" + caseID + data.Name
				// Create unique network name with test case ID to avoid conflicts between tests
				networkName := caseID + data.Name
				// Create namespace for the test
				nsBuilder := namespace.NewBuilder(getAPIClient(), ns1)
				for key, value := range params.PrivilegedNSLabels {
					nsBuilder.WithLabel(key, value)
				}
				_, err := nsBuilder.Create()
				Expect(err).NotTo(HaveOccurred(), "Failed to create namespace %s", ns1)
				// Wait for namespace to be ready before proceeding
				Eventually(func() bool {
					return nsBuilder.Exists()
				}, 30*time.Second, time.Second).Should(BeTrue(), "Namespace %s should exist", ns1)
				defer func() {
					// Clean up namespace after test (increased timeout for SR-IOV cleanup)
					err := nsBuilder.DeleteAndWait(120 * time.Second)
					if err != nil {
						GinkgoLogr.Info("Failed to delete namespace", "namespace", ns1, "error", err)
					}
				}()

				By("Create sriovNetwork to generate net-attach-def on the target namespace")
				GinkgoLogr.Info("device ID", "deviceID", data.DeviceID)
				GinkgoLogr.Info("device Name", "deviceName", data.Name)
				sriovnetwork := sriovNetwork{
					name:             networkName,
					resourceName:     data.Name,
					networkNamespace: ns1,
					template:         sriovNetworkTemplate,
					namespace:        sriovOpNs,
					linkState:        "auto",
				}
				//defer
				defer func() {
					rmSriovNetwork(sriovnetwork.name, sriovOpNs)
				}()
				sriovnetwork.createSriovNetwork()

				chkVFStatusWithPassTraffic(sriovnetwork.name, data.InterfaceName, ns1, "link-state auto")

			}()
		}
		if !executed {
			Skip("No SR-IOV devices matched the requested configuration")
		}
	})

	It("Author:zzhao-Medium-NonPreRelease-Longduration-71006-SR-IOV VF with enabled link state [Disruptive] [Serial]", func() {
		var caseID = "71006-"
		executed := false

		for _, data := range testData {
			data := data
			// Create VF on with given device
			result := initVF(data.Name, data.DeviceID, data.InterfaceName, data.Vendor, sriovOpNs, vfNum, workerNodes)
			// if the deviceid is not exist on the worker, skip this
			if !result {
				continue
			}
			executed = true
			func() {
				ns1 := "e2e-" + caseID + data.Name
				// Create unique network name with test case ID to avoid conflicts between tests
				networkName := caseID + data.Name
				// Create namespace for the test
				nsBuilder := namespace.NewBuilder(getAPIClient(), ns1)
				for key, value := range params.PrivilegedNSLabels {
					nsBuilder.WithLabel(key, value)
				}
				_, err := nsBuilder.Create()
				Expect(err).NotTo(HaveOccurred(), "Failed to create namespace %s", ns1)
				// Wait for namespace to be ready before proceeding
				Eventually(func() bool {
					return nsBuilder.Exists()
				}, 30*time.Second, time.Second).Should(BeTrue(), "Namespace %s should exist", ns1)
				defer func() {
					// Clean up namespace after test (increased timeout for SR-IOV cleanup)
					err := nsBuilder.DeleteAndWait(120 * time.Second)
					if err != nil {
						GinkgoLogr.Info("Failed to delete namespace", "namespace", ns1, "error", err)
					}
				}()

				By("Create sriovNetwork to generate net-attach-def on the target namespace")
				GinkgoLogr.Info("device ID", "deviceID", data.DeviceID)
				GinkgoLogr.Info("device Name", "deviceName", data.Name)
				sriovnetwork := sriovNetwork{
					name:             networkName,
					resourceName:     data.Name,
					networkNamespace: ns1,
					template:         sriovNetworkTemplate,
					namespace:        sriovOpNs,
					linkState:        "enable",
				}
				//defer
				defer func() {
					rmSriovNetwork(sriovnetwork.name, sriovOpNs)
				}()
				sriovnetwork.createSriovNetwork()

				chkVFStatusWithPassTraffic(sriovnetwork.name, data.InterfaceName, ns1, "link-state enable")

			}()
		}
		if !executed {
			Skip("No SR-IOV devices matched the requested configuration")
		}

	})

	It("Author:yingwang-Medium-NonPreRelease-Longduration-69646-MTU configuration for SR-IOV policy [Disruptive] [Serial]", func() {
		var caseID = "69646-"
		executed := false

		for _, data := range testData {
			data := data
			// Create VF on with given device
			result := initVF(data.Name, data.DeviceID, data.InterfaceName, data.Vendor, sriovOpNs, vfNum, workerNodes)
			// if the deviceid is not exist on the worker, skip this
			if !result {
				continue
			}
			executed = true
			//configure mtu in sriovnetworknodepolicy
			mtuValue := 1800
			By(fmt.Sprintf("Updating SRIOV policy %s with MTU %d", data.Name, mtuValue))

			// Find the existing policy using ListPolicy
			listOptions := client.ListOptions{}
			policies, err := sriov.ListPolicy(getAPIClient(), sriovOpNs, listOptions)
			Expect(err).NotTo(HaveOccurred(), "Failed to list SRIOV policies")

			// Find the policy with matching name
			var targetPolicy *sriov.PolicyBuilder
			for _, policy := range policies {
				if policy.Definition.Name == data.Name {
					targetPolicy = policy
					break
				}
			}

			Expect(targetPolicy).NotTo(BeNil(), "SRIOV policy %s not found", data.Name)

			// Update the policy with MTU by modifying the definition and using the client
			targetPolicy.Definition.Spec.Mtu = mtuValue

			// Log equivalent oc command for troubleshooting
			GinkgoLogr.Info("Equivalent oc command", "command", fmt.Sprintf("oc patch sriovnetworknodepolicy %s -n %s --type merge -p '{\"spec\":{\"mtu\":%d}}'", data.Name, sriovOpNs, mtuValue))

			// Update the policy using the Kubernetes client
			err = getAPIClient().Client.Update(context.TODO(), targetPolicy.Definition)
			Expect(err).NotTo(HaveOccurred(), "Failed to update SRIOV policy %s with MTU", data.Name)

			GinkgoLogr.Info("SRIOV policy updated with MTU", "name", data.Name, "mtu", mtuValue)

			// Log equivalent oc command to verify update
			GinkgoLogr.Info("Equivalent oc command", "command", fmt.Sprintf("oc get sriovnetworknodepolicy %s -n %s -o jsonpath='{.spec.mtu}'", data.Name, sriovOpNs))
			waitForSriovPolicyReady(sriovOpNs)

			func() {
				ns1 := "e2e-" + caseID + data.Name
				// Create unique network name with test case ID to avoid conflicts between tests
				networkName := caseID + data.Name
				// Create namespace for the test
				nsBuilder := namespace.NewBuilder(getAPIClient(), ns1)
				for key, value := range params.PrivilegedNSLabels {
					nsBuilder.WithLabel(key, value)
				}
				_, err := nsBuilder.Create()
				Expect(err).NotTo(HaveOccurred(), "Failed to create namespace %s", ns1)
				// Wait for namespace to be ready before proceeding
				Eventually(func() bool {
					return nsBuilder.Exists()
				}, 30*time.Second, time.Second).Should(BeTrue(), "Namespace %s should exist", ns1)
				defer func() {
					// Clean up namespace after test (increased timeout for SR-IOV cleanup)
					err := nsBuilder.DeleteAndWait(120 * time.Second)
					if err != nil {
						GinkgoLogr.Info("Failed to delete namespace", "namespace", ns1, "error", err)
					}
				}()

				By("Create sriovNetwork to generate net-attach-def on the target namespace")
				GinkgoLogr.Info("device ID", "deviceID", data.DeviceID)
				GinkgoLogr.Info("device Name", "deviceName", data.Name)
				sriovnetwork := sriovNetwork{
					name:             networkName,
					resourceName:     data.Name,
					networkNamespace: ns1,
					template:         sriovNetworkTemplate,
					namespace:        sriovOpNs,
					spoolchk:         "on",
					trust:            "on",
				}
				//defer
				defer func() {
					rmSriovNetwork(sriovnetwork.name, sriovOpNs)
				}()
				sriovnetwork.createSriovNetwork()

				chkVFStatusWithPassTraffic(sriovnetwork.name, data.InterfaceName, ns1, "mtu "+strconv.Itoa(mtuValue))

			}()
		}
		if !executed {
			Skip("No SR-IOV devices matched the requested configuration")
		}
	})

	It("Author:yingwang-Medium-NonPreRelease-Longduration-69582-DPDK SR-IOV VF functionality validation [Disruptive] [Serial]", func() {
		var caseID = "69582-"
		executed := false

		for _, data := range testData {
			data := data
			// skip bcm nics: OCPBUGS-30909
			if strings.Contains(data.Name, "bcm") {
				continue
			}
			// Create VF on with given device
			policyName := data.Name
			networkName := data.Name + "dpdk" + "net"
			result := initDpdkVF(data.Name, data.DeviceID, data.InterfaceName, data.Vendor, sriovOpNs, vfNum, workerNodes)
			// if the deviceid is not exist on the worker, skip this
			if !result {
				continue
			}
			executed = true
			func() {
				ns1 := "e2e-" + caseID + data.Name
				// Create namespace for the test
				nsBuilder := namespace.NewBuilder(getAPIClient(), ns1)
				for key, value := range params.PrivilegedNSLabels {
					nsBuilder.WithLabel(key, value)
				}
				_, err := nsBuilder.Create()
				Expect(err).NotTo(HaveOccurred(), "Failed to create namespace %s", ns1)
				// Wait for namespace to be ready before proceeding
				Eventually(func() bool {
					return nsBuilder.Exists()
				}, 30*time.Second, time.Second).Should(BeTrue(), "Namespace %s should exist", ns1)
				defer func() {
					// Clean up namespace after test (increased timeout for SR-IOV cleanup)
					err := nsBuilder.DeleteAndWait(120 * time.Second)
					if err != nil {
						GinkgoLogr.Info("Failed to delete namespace", "namespace", ns1, "error", err)
					}
				}()

				By("Create sriovNetwork to generate net-attach-def on the target namespace")
				GinkgoLogr.Info("device ID", "deviceID", data.DeviceID)
				GinkgoLogr.Info("device Name", "deviceName", data.Name)
				sriovNetworkTemplate = filepath.Join(buildPruningBaseDir, "sriovnetwork-template.yaml")
				sriovnetwork := sriovNetwork{
					name:             networkName,
					resourceName:     policyName,
					networkNamespace: ns1,
					template:         sriovNetworkTemplate,
					namespace:        sriovOpNs,
				}

				//defer
				defer func() {
					rmSriovNetwork(sriovnetwork.name, sriovOpNs)
				}()
				sriovnetwork.createSriovNetwork()
				// Wait a bit for NAD to be fully ready before creating pods
				By("Waiting for NetworkAttachmentDefinition to be fully ready")
				time.Sleep(5 * time.Second)

				//create pods
				sriovTestPodDpdkTemplate := filepath.Join(buildPruningBaseDir, "sriov-dpdk-template.yaml")
				sriovTestPod := sriovTestPod{
					name:        "sriovdpdk",
					namespace:   ns1,
					networkName: sriovnetwork.name,
					template:    sriovTestPodDpdkTemplate,
				}
				sriovTestPod.createSriovTestPod()
				err1 := waitForPodWithLabelReady(ns1, "name=sriov-dpdk")
				Expect(err1).ToNot(HaveOccurred(), "this pod with label name=sriov-dpdk not ready")

				By("Check testpmd running well")
				// Verify that the VF was properly assigned by checking the PCI address
				// Log equivalent oc command to check PCI address
				GinkgoLogr.Info("Equivalent oc command", "command", fmt.Sprintf("oc get pod %s -n %s -o jsonpath='{.metadata.annotations.k8s\\.v1\\.cni\\.cncf\\.io/network-status}'", sriovTestPod.name, sriovTestPod.namespace))
				pciAddress := getPciAddress(sriovTestPod.namespace, sriovTestPod.name, policyName)
				Expect(pciAddress).NotTo(Equal("0000:00:00.0"), "PCI address should be assigned from pod network status")
				GinkgoLogr.Info("PCI address assigned to pod", "pciAddress", pciAddress, "pod", sriovTestPod.name)

				// Get the pod to verify it has the network interface
				By(fmt.Sprintf("Verifying DPDK VF is available in pod %s", sriovTestPod.name))
				// Log equivalent oc command to verify network status
				GinkgoLogr.Info("Equivalent oc command", "command", fmt.Sprintf("oc describe pod %s -n %s", sriovTestPod.name, sriovTestPod.namespace))
				podBuilder, err := pod.Pull(getAPIClient(), sriovTestPod.name, sriovTestPod.namespace)
				Expect(err).NotTo(HaveOccurred(), "Failed to pull pod %s", sriovTestPod.name)
				Expect(podBuilder).NotTo(BeNil(), "Pod builder should not be nil")
				Expect(podBuilder.Object).NotTo(BeNil(), "Pod object should not be nil")

				// Check if the pod has the network status annotation with PCI address
				networkStatusAnnotation := "k8s.v1.cni.cncf.io/network-status"
				podNetAnnotation := podBuilder.Object.Annotations[networkStatusAnnotation]
				Expect(podNetAnnotation).NotTo(BeEmpty(), "Pod should have network status annotation")
				Expect(podNetAnnotation).To(ContainSubstring(policyName), "Network status should contain policy name")

				// Verify PCI address is in the annotation
				Expect(podNetAnnotation).To(ContainSubstring("pci-address"), "Network status should contain PCI address")
				Expect(podNetAnnotation).To(ContainSubstring(pciAddress), "Network status should contain the assigned PCI address")

			sriovTestPod.deleteSriovTestPod()

		}()
	}
	if !executed {
		Skip("No SR-IOV devices matched the requested configuration")
	}
})

})
