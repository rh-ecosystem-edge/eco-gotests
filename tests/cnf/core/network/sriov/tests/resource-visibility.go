package tests

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/daemonset"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netenv"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/sriovenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/sriovoperator"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

const (
	// Number of policy pairs to create (one per PF per iteration).
	numPolicyPairsResourceVisibility = 6
	mockDaemonSetName                = "sriov-device-plugin-mock"
)

var _ = Describe("ResourceVisibility", Ordered, Label(tsparams.LabelResourceVisibilityTestCases),
	ContinueOnFailure, func() {
		var (
			sriovInterfacesUnderTest []string
			workerNodeList           []*nodes.Builder
			err                      error
		)

		BeforeAll(func() {
			By("Validating SR-IOV interfaces on 2 PFs")

			workerNodeList, err = nodes.List(APIClient,
				metav1.ListOptions{LabelSelector: labels.Set(NetConfig.WorkerLabelMap).String()})
			Expect(err).ToNot(HaveOccurred(), "Failed to discover worker nodes")
			Expect(sriovenv.ValidateSriovInterfaces(workerNodeList, 2)).ToNot(HaveOccurred(),
				"Failed to get required SR-IOV interfaces")

			sriovInterfacesUnderTest, err = NetConfig.GetSriovInterfaces(2)
			Expect(err).ToNot(HaveOccurred(), "Failed to retrieve 2 SR-IOV interfaces for testing")

			By("Verifying if resource visibility tests can be executed on given cluster")

			err = netenv.DoesClusterHasEnoughNodes(APIClient, NetConfig, 1, 1)
			if err != nil {
				Skip(fmt.Sprintf("Skipping test - cluster doesn't have enough nodes: %v", err))
			}

			By("Verifying enough VFs are available on both PFs")

			for _, pfName := range sriovInterfacesUnderTest {
				minVFs, vfErr := sriovenv.GetMinTotalVFsAcrossWorkers(workerNodeList, pfName)
				Expect(vfErr).ToNot(HaveOccurred(),
					fmt.Sprintf("Failed to get minimum VFs for PF %s", pfName))

				if minVFs < numPolicyPairsResourceVisibility {
					Skip(fmt.Sprintf("Not enough VFs on PF %s: %d available, %d required",
						pfName, minVFs, numPolicyPairsResourceVisibility))
				}
			}
		})

		AfterAll(func() {
			By("Removing mock DaemonSet if it exists")

			mockDS, dsErr := daemonset.Pull(APIClient, mockDaemonSetName, NetConfig.SriovOperatorNamespace)
			if dsErr == nil && mockDS != nil {
				err := mockDS.Delete()
				Expect(err).ToNot(HaveOccurred(), "Failed to delete mock DaemonSet")
			}

			By("Removing SR-IOV configuration")

			err := sriovoperator.RemoveSriovConfigurationAndWaitForSriovAndMCPStable(
				APIClient,
				NetConfig.WorkerLabelEnvVar,
				NetConfig.SriovOperatorNamespace,
				tsparams.MCOWaitTimeout,
				tsparams.DefaultTimeout)
			Expect(err).ToNot(HaveOccurred(), "Failed to remove SR-IOV configuration")
		})

		// When a mock sriov-device-plugin DaemonSet is present and many
		// SriovNetworkNodePolicies are created incrementally (pairs across two PFs),
		// the device plugin may fail to advertise some resources in node allocatable.
		It("Verifies all SR-IOV resources appear in node allocatable after incremental policy creation",
			reportxml.ID("76401"), func() {
				pf1 := sriovInterfacesUnderTest[0]
				pf2 := sriovInterfacesUnderTest[1]

				By("Creating mock sriov-device-plugin DaemonSet")

				mockLabels := map[string]string{
					"mock": mockDaemonSetName,
					"app":  "sriov-device-plugin",
				}
				mockContainer := corev1.Container{
					Name:    "mock-sleep",
					Image:   NetConfig.CnfNetTestContainer,
					Command: []string{"sleep", "3600"},
				}

				_, err = daemonset.NewBuilder(
					APIClient, mockDaemonSetName, NetConfig.SriovOperatorNamespace,
					mockLabels, mockContainer).CreateAndWaitUntilReady(tsparams.DefaultTimeout)
				Expect(err).ToNot(HaveOccurred(), "Failed to create mock sriov-device-plugin DaemonSet")

				var allResourceNames []string

				By("Creating SriovNetworkNodePolicy pairs incrementally across two PFs")

				for policyIdx := 0; policyIdx < numPolicyPairsResourceVisibility; policyIdx++ {
					policyNameA := fmt.Sprintf("resvis-pf1-%d", policyIdx)
					resourceNameA := fmt.Sprintf("resvisibilitya%d", policyIdx)

					_, err = sriov.NewPolicyBuilder(
						APIClient,
						policyNameA,
						NetConfig.SriovOperatorNamespace,
						resourceNameA,
						numPolicyPairsResourceVisibility,
						[]string{pf1},
						NetConfig.WorkerLabelMap).
						WithDevType("netdevice").
						WithVFRange(policyIdx, policyIdx).
						Create()
					Expect(err).ToNot(HaveOccurred(),
						fmt.Sprintf("Failed to create policy %s", policyNameA))

					time.Sleep(time.Duration(policyIdx) * time.Second)

					policyNameB := fmt.Sprintf("resvis-pf2-%d", policyIdx)
					resourceNameB := fmt.Sprintf("resvisibilityb%d", policyIdx)

					_, err = sriov.NewPolicyBuilder(
						APIClient,
						policyNameB,
						NetConfig.SriovOperatorNamespace,
						resourceNameB,
						numPolicyPairsResourceVisibility,
						[]string{pf2},
						NetConfig.WorkerLabelMap).
						WithDevType("netdevice").
						WithVFRange(policyIdx, policyIdx).
						Create()
					Expect(err).ToNot(HaveOccurred(),
						fmt.Sprintf("Failed to create policy %s", policyNameB))

					allResourceNames = append(allResourceNames, resourceNameA, resourceNameB)

					By(fmt.Sprintf("Waiting for resources %s and %s to appear in allocatable (iteration %d)",
						resourceNameA, resourceNameB, policyIdx))

					for _, worker := range workerNodeList {
						verifyResourcesAllocatable(worker.Object.Name, []string{resourceNameA, resourceNameB})
					}
				}

				By("Waiting for SR-IOV operator and MCP to stabilize")

				err = sriovoperator.WaitForSriovAndMCPStable(
					APIClient, tsparams.MCOWaitTimeout, tsparams.DefaultStableDuration,
					NetConfig.CnfMcpLabel, NetConfig.SriovOperatorNamespace)
				Expect(err).ToNot(HaveOccurred(), "Failed to wait for SR-IOV and MCP stability")

				By("Final verification: all resources present in allocatable on every worker")

				for _, worker := range workerNodeList {
					verifyResourcesAllocatable(worker.Object.Name, allResourceNames)
				}
			})
	})

// verifyResourcesAllocatable waits for all given resources to appear in node allocatable
// with an expected count of exactly 1 (each policy carves a single VF).
func verifyResourcesAllocatable(nodeName string, resourceNames []string) {
	Eventually(func(gomega Gomega) {
		workerNode, pullErr := nodes.Pull(APIClient, nodeName)
		gomega.Expect(pullErr).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to pull worker node %s", nodeName))

		for _, resName := range resourceNames {
			fullResName := corev1.ResourceName("openshift.io/" + resName)
			quantity, exists := workerNode.Object.Status.Allocatable[fullResName]

			gomega.Expect(exists).To(BeTrue(),
				fmt.Sprintf("Resource %s not found in allocatable", fullResName))

			num, _ := quantity.AsInt64()
			gomega.Expect(num).To(BeNumerically("==", 1),
				fmt.Sprintf("Resource %s expected 1 allocatable, got %d", fullResName, num))
		}
	}, 5*time.Minute, tsparams.RetryInterval).Should(Succeed(),
		fmt.Sprintf("Resources missing from allocatable on node %s", nodeName))
}
