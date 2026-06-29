package tests

import (
	"fmt"
	"net"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/cmd"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/ipaddr"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netenv"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/sriovenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/sriovoperator"
	multus "gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
)

const (
	bondNADName = "sriov-bond-nad"

	mtu500  = 500
	mtu1280 = 1280
	mtu9000 = 9000

	// SR-IOV policy resourceNames for bond tests (PF1/PF2 × small/jumbo MTU × IP family).
	bondResourceV4PF1Small = "sriovbondpf1mtu500"
	bondResourceV4PF1Jumbo = "sriovbondpf1mtu9000"
	bondResourceV4PF2Small = "sriovbondpf2mtu500"
	bondResourceV4PF2Jumbo = "sriovbondpf2mtu9000"
	bondResourceV6PF1Small = "sriovbondpf1mtu1280v6"
	bondResourceV6PF1Jumbo = "sriovbondpf1mtu9000v6"
	bondResourceV6PF2Small = "sriovbondpf2mtu1280v6"
	bondResourceV6PF2Jumbo = "sriovbondpf2mtu9000v6"

	// bondMinSwitchInterfaces is the number of physical switch ports required for static LAG setup (2 LAGs × 2 members).
	bondMinSwitchInterfaces = 4

	// Placeholder Polarion IDs; update when assigned.
	polarionBondIPAMDiffNodeDiffPF = "TBD"
	polarionBondIPAMDiffNodeSamePF = "TBD"
)

// Lab switch state for balance-rr/xor LAG setup (active-backup tests leave the switch untouched).
var (
	bondSwitchCredentials  *sriovenv.SwitchCredentials
	bondSwitchInterfaces   []string
	bondSwitchLagNames     []string
	bondSwitchSavedConfigs []string
)

var _ = Describe(
	"SRIOV Bond CNI",
	Ordered,
	Label(tsparams.LabelSuite, tsparams.LabelBondModeTestCases),
	ContinueOnFailure,
	func() {
		var (
			workerNodeList  []*nodes.Builder
			pf1             string
			pf2             string
			clusterIPFamily string
			err             error
		)

		BeforeAll(func() {
			By("Checking cluster IP family")

			clusterIPFamily, err = netenv.GetClusterIPFamily(APIClient)
			Expect(err).ToNot(HaveOccurred(), "Failed to detect cluster IP family")

			if !netenv.ClusterSupportsIPv4(clusterIPFamily) && !netenv.ClusterSupportsIPv6(clusterIPFamily) {
				Skip("Cluster does not support IPv4 or IPv6 - skipping SR-IOV bond tests")
			}

			By("Discover and list worker nodes")

			workerNodeList, err = nodes.List(
				APIClient, metav1.ListOptions{LabelSelector: labels.Set(NetConfig.WorkerLabelMap).String()})
			Expect(err).ToNot(HaveOccurred(), "Failed to list worker nodes")

			if len(workerNodeList) < 2 {
				Skip("Cluster needs at least 2 worker nodes for SR-IOV bond tests")
			}

			By("Validating SR-IOV interfaces exist on nodes")
			Expect(sriovenv.ValidateSriovInterfaces(workerNodeList, 2)).ToNot(HaveOccurred(),
				"Failed to get required SR-IOV interfaces")

			sriovInterfaces, err := NetConfig.GetSriovInterfaces(2)
			Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for testing")

			pf1 = sriovInterfaces[0]
			pf2 = sriovInterfaces[1]

			sriovenv.ActivateSCTPModuleOnWorkerNodes()

			By("Verifying SCTP kernel module is loaded on worker nodes")

			sctpOutput, err := cluster.ExecCmdWithStdout(APIClient, "lsmod | grep -q sctp && echo loaded",
				metav1.ListOptions{LabelSelector: labels.Set(NetConfig.WorkerLabelMap).String()})
			Expect(err).ToNot(HaveOccurred(), "Failed to check SCTP kernel module on worker nodes")
			Expect(sctpOutput).NotTo(BeEmpty(),
				"SCTP kernel module must be loaded on workers for this suite (traffic tests require SCTP); "+
					"configure SCTP per tests/cnf/core/network/README prerequisites (e.g. MachineConfig)")

			supportsIPv4 := netenv.ClusterSupportsIPv4(clusterIPFamily)
			supportsIPv6 := netenv.ClusterSupportsIPv6(clusterIPFamily)

			// Default VF layout (single-stack): 0-4 small MTU tier, 5-9 large MTU tier.
			vf4SmallStart, vf4SmallEnd := 0, 4
			vf4LargeStart, vf4LargeEnd := 5, 9
			vf6SmallStart, vf6SmallEnd := 0, 4
			vf6LargeStart, vf6LargeEnd := 5, 9

			const bondMinVFsDualStack = 6

			bondPolicyNumVFs := 10

			if supportsIPv4 && supportsIPv6 {
				By("Selecting non-overlapping VF ranges for dual-stack bond policies")

				pf1Total, vfErr := sriovenv.GetMinTotalVFsAcrossWorkers(workerNodeList, pf1)
				Expect(vfErr).ToNot(HaveOccurred(), "Failed to get minimum total VFs for PF1 across workers")

				pf2Total, vfErr := sriovenv.GetMinTotalVFsAcrossWorkers(workerNodeList, pf2)
				Expect(vfErr).ToNot(HaveOccurred(), "Failed to get minimum total VFs for PF2 across workers")

				if pf1Total < bondMinVFsDualStack || pf2Total < bondMinVFsDualStack {
					Skip(fmt.Sprintf(
						"Dual-stack bond tests require >=%d VFs per PF on every worker; min across workers: pf1=%d, pf2=%d",
						bondMinVFsDualStack, pf1Total, pf2Total))
				}

				if pf1Total >= 20 && pf2Total >= 20 {
					// Expanded layout: 20+ hardware VFs, but policies still provision 10 VFs (indices 0-9).
					// Split 3+2 VFs per MTU tier per IP family within that range.
					vf4SmallStart, vf4SmallEnd = 0, 2
					vf4LargeStart, vf4LargeEnd = 3, 4
					vf6SmallStart, vf6SmallEnd = 5, 7
					vf6LargeStart, vf6LargeEnd = 8, 9
				} else if pf1Total >= 10 && pf2Total >= 10 {
					// Compact layout for 10-VF PFs: IPv4 VFs 0-4, IPv6 VFs 5-9 (split 2+3 per MTU tier).
					vf4SmallStart, vf4SmallEnd = 0, 1
					vf4LargeStart, vf4LargeEnd = 2, 4
					vf6SmallStart, vf6SmallEnd = 5, 6
					vf6LargeStart, vf6LargeEnd = 7, 9
				} else {
					// Minimal layout for 6-VF PFs: IPv4 VFs 0-2, IPv6 VFs 3-5 (split 1+2 per MTU tier).
					vf4SmallStart, vf4SmallEnd = 0, 0
					vf4LargeStart, vf4LargeEnd = 1, 2
					vf6SmallStart, vf6SmallEnd = 3, 3
					vf6LargeStart, vf6LargeEnd = 4, 5
					bondPolicyNumVFs = 6
				}
			}

			if supportsIPv4 {
				setupBondStackForFamily(bondStackFamilyParams{
					familyLabel:  "IPv4",
					policyPrefix: "ipv4",
					dualStack:    supportsIPv6,
					policyNumVFs: bondPolicyNumVFs,
					pf1:          pf1,
					pf2:          pf2,
					mtuSmall:     mtu500,
					mtuLarge:     mtu9000,
					vfSmallStart: vf4SmallStart,
					vfSmallEnd:   vf4SmallEnd,
					vfLargeStart: vf4LargeStart,
					vfLargeEnd:   vf4LargeEnd,
					resPF1Small:  bondResourceV4PF1Small,
					resPF1Large:  bondResourceV4PF1Jumbo,
					resPF2Small:  bondResourceV4PF2Small,
					resPF2Large:  bondResourceV4PF2Jumbo,
					networks: bondWhereaboutsNetworkConfigs(
						bondNetworksV4DiffPFSmall, bondNetworksV4DiffPFJumbo,
						bondNetworksV4SamePFSmall, bondNetworksV4SamePFJumbo,
						bondResourceV4PF1Small, bondResourceV4PF1Jumbo,
						bondResourceV4PF2Small, bondResourceV4PF2Jumbo,
						tsparams.WhereaboutsIPv4Range, tsparams.WhereaboutsIPv4Gateway,
						tsparams.WhereaboutsIPv4Range2, tsparams.WhereaboutsIPv4Gateway2,
					),
				})
			}

			if supportsIPv6 {
				setupBondStackForFamily(bondStackFamilyParams{
					familyLabel:  "IPv6",
					policyPrefix: "ipv6",
					dualStack:    supportsIPv4,
					policyNumVFs: bondPolicyNumVFs,
					pf1:          pf1,
					pf2:          pf2,
					mtuSmall:     mtu1280,
					mtuLarge:     mtu9000,
					vfSmallStart: vf6SmallStart,
					vfSmallEnd:   vf6SmallEnd,
					vfLargeStart: vf6LargeStart,
					vfLargeEnd:   vf6LargeEnd,
					resPF1Small:  bondResourceV6PF1Small,
					resPF1Large:  bondResourceV6PF1Jumbo,
					resPF2Small:  bondResourceV6PF2Small,
					resPF2Large:  bondResourceV6PF2Jumbo,
					networks: bondWhereaboutsNetworkConfigs(
						bondNetworksV6DiffPFSmall, bondNetworksV6DiffPFJumbo,
						bondNetworksV6SamePFSmall, bondNetworksV6SamePFJumbo,
						bondResourceV6PF1Small, bondResourceV6PF1Jumbo,
						bondResourceV6PF2Small, bondResourceV6PF2Jumbo,
						tsparams.WhereaboutsIPv6Range, tsparams.WhereaboutsIPv6Gateway,
						tsparams.WhereaboutsIPv6Range2, tsparams.WhereaboutsIPv6Gateway2,
					),
				})
			}
		})

		AfterAll(func() {
			if len(bondSwitchSavedConfigs) > 0 && bondSwitchCredentials != nil {
				By("Restoring lab switch configuration after bond tests")

				err = restoreBondSwitchLAG(
					bondSwitchCredentials, bondSwitchInterfaces, bondSwitchLagNames, bondSwitchSavedConfigs)
				Expect(err).ToNot(HaveOccurred(), "Failed to restore lab switch configuration")
			}

			By("Removing SR-IOV configuration")

			err = sriovoperator.RemoveSriovConfigurationAndWaitForSriovAndMCPStable(
				APIClient,
				NetConfig.WorkerLabelEnvVar,
				NetConfig.SriovOperatorNamespace,
				tsparams.MCOWaitTimeout,
				tsparams.DefaultTimeout)
			Expect(err).ToNot(HaveOccurred(), "Failed to remove SR-IOV configuration")
		})

		AfterEach(func() {
			By("Deleting test pods")

			err = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
				netparam.DefaultTimeout, pod.GetGVR())
			Expect(err).ToNot(HaveOccurred(), "Failed to delete test pods")

			// Best-effort cleanup: tests may create mode- and MTU-specific bond NADs.
			for _, nadName := range bondNADNamesForCleanup() {
				_ = deleteBondNADIfExists(nadName)
			}
		})

		Context("Mode: active-backup", func() {
			It("DiffNodeDiffPF IPv4", reportxml.ID("89050"), func() {
				if !netenv.ClusterSupportsIPv4(clusterIPFamily) {
					Skip("Cluster does not support IPv4 - skipping SR-IOV bond IPv4 tests")
				}

				runBondScenario(
					sriovenv.BondModeActiveBackup,
					mtu500, mtu9000,
					tsparams.ServerIPv4IPAddress, tsparams.ClientIPv4IPAddress,
					tsparams.ServerIPv4IPAddress2, tsparams.ClientIPv4IPAddress2,
					workerNodeList[0].Definition.Name, workerNodeList[1].Definition.Name,
					bondNetworksV4DiffPFSmall, bondNetworksV4DiffPFJumbo,
				)
			})

			It("DiffNodeSamePF IPv4", reportxml.ID("89051"), func() {
				if !netenv.ClusterSupportsIPv4(clusterIPFamily) {
					Skip("Cluster does not support IPv4 - skipping SR-IOV bond IPv4 tests")
				}

				runBondScenario(
					sriovenv.BondModeActiveBackup,
					mtu500, mtu9000,
					tsparams.ServerIPv4IPAddress, tsparams.ClientIPv4IPAddress,
					tsparams.ServerIPv4IPAddress2, tsparams.ClientIPv4IPAddress2,
					workerNodeList[0].Definition.Name, workerNodeList[1].Definition.Name,
					bondNetworksV4SamePFSmall, bondNetworksV4SamePFJumbo,
				)
			})

			It("DiffNodeDiffPF IPv6", reportxml.ID("89057"), func() {
				if !netenv.ClusterSupportsIPv6(clusterIPFamily) {
					Skip("Cluster does not support IPv6 - skipping SR-IOV bond IPv6 tests")
				}

				runBondScenario(
					sriovenv.BondModeActiveBackup,
					mtu1280, mtu9000,
					tsparams.ServerIPv6IPAddress, tsparams.ClientIPv6IPAddress,
					tsparams.ServerIPv6IPAddress2, tsparams.ClientIPv6IPAddress2,
					workerNodeList[0].Definition.Name, workerNodeList[1].Definition.Name,
					bondNetworksV6DiffPFSmall, bondNetworksV6DiffPFJumbo,
				)
			})

			It("DiffNodeSamePF IPv6", reportxml.ID("89058"), func() {
				if !netenv.ClusterSupportsIPv6(clusterIPFamily) {
					Skip("Cluster does not support IPv6 - skipping SR-IOV bond IPv6 tests")
				}

				runBondScenario(
					sriovenv.BondModeActiveBackup,
					mtu1280, mtu9000,
					tsparams.ServerIPv6IPAddress, tsparams.ClientIPv6IPAddress,
					tsparams.ServerIPv6IPAddress2, tsparams.ClientIPv6IPAddress2,
					workerNodeList[0].Definition.Name, workerNodeList[1].Definition.Name,
					bondNetworksV6SamePFSmall, bondNetworksV6SamePFJumbo,
				)
			})

			It("DiffNodeDiffPF IPv6 Whereabouts IPAM", reportxml.ID(polarionBondIPAMDiffNodeDiffPF), func() {
				if !netenv.ClusterSupportsIPv6(clusterIPFamily) {
					Skip("Cluster does not support IPv6 - skipping SR-IOV bond whereabouts IPAM tests")
				}

				runBondWhereaboutsScenario(
					sriovenv.BondModeActiveBackup,
					mtu1280, mtu9000,
					workerNodeList[0].Definition.Name, workerNodeList[1].Definition.Name,
					bondNetworksV6DiffPFSmall, bondNetworksV6DiffPFJumbo,
				)
			})

			It("DiffNodeSamePF IPv6 Whereabouts IPAM", reportxml.ID(polarionBondIPAMDiffNodeSamePF), func() {
				if !netenv.ClusterSupportsIPv6(clusterIPFamily) {
					Skip("Cluster does not support IPv6 - skipping SR-IOV bond whereabouts IPAM tests")
				}

				runBondWhereaboutsScenario(
					sriovenv.BondModeActiveBackup,
					mtu1280, mtu9000,
					workerNodeList[0].Definition.Name, workerNodeList[1].Definition.Name,
					bondNetworksV6SamePFSmall, bondNetworksV6SamePFJumbo,
				)
			})
		})

		Context("Mode: active-active", func() {
			// cnf-gotests TestActiveActiveBondScenario: switch LAG + DiffNodeDiffPF only (PF1+PF2 slaves).
			BeforeEach(func() {
				By("Configure static LAGs on lab switch for active-active bond tests")

				err = setupBondSwitchLAGForActiveActiveTests()
				Expect(err).ToNot(HaveOccurred(), "Failed to configure static LAGs on lab switch")
			})

			AfterEach(func() {
				restoreBondSwitchLAGAfterActiveActiveTest()
			})

			It("balance-rr DiffNodeDiffPF IPv4", reportxml.ID("89052"), func() {
				if !netenv.ClusterSupportsIPv4(clusterIPFamily) {
					Skip("Cluster does not support IPv4 - skipping SR-IOV bond IPv4 tests")
				}

				runBondScenario(
					sriovenv.BondModeBalanceRR,
					mtu500, mtu9000,
					tsparams.ServerIPv4IPAddress, tsparams.ClientIPv4IPAddress,
					tsparams.ServerIPv4IPAddress2, tsparams.ClientIPv4IPAddress2,
					workerNodeList[0].Definition.Name, workerNodeList[1].Definition.Name,
					bondNetworksV4DiffPFSmall, bondNetworksV4DiffPFJumbo,
				)
			})

			It("balance-xor: DiffNodeDiffPF IPv4", reportxml.ID("89054"), func() {
				if !netenv.ClusterSupportsIPv4(clusterIPFamily) {
					Skip("Cluster does not support IPv4 - skipping SR-IOV bond IPv4 tests")
				}

				runBondScenario(
					sriovenv.BondModeBalanceXOR,
					mtu500, mtu9000,
					tsparams.ServerIPv4IPAddress, tsparams.ClientIPv4IPAddress,
					tsparams.ServerIPv4IPAddress2, tsparams.ClientIPv4IPAddress2,
					workerNodeList[0].Definition.Name, workerNodeList[1].Definition.Name,
					bondNetworksV4DiffPFSmall, bondNetworksV4DiffPFJumbo,
				)
			})

			It("balance-rr: DiffNodeDiffPF IPv6", reportxml.ID("89059"), func() {
				if !netenv.ClusterSupportsIPv6(clusterIPFamily) {
					Skip("Cluster does not support IPv6 - skipping SR-IOV bond IPv6 tests")
				}

				runBondScenario(
					sriovenv.BondModeBalanceRR,
					mtu1280, mtu9000,
					tsparams.ServerIPv6IPAddress, tsparams.ClientIPv6IPAddress,
					tsparams.ServerIPv6IPAddress2, tsparams.ClientIPv6IPAddress2,
					workerNodeList[0].Definition.Name, workerNodeList[1].Definition.Name,
					bondNetworksV6DiffPFSmall, bondNetworksV6DiffPFJumbo,
				)
			})

			It("balance-xor: DiffNodeDiffPF IPv6", reportxml.ID("89061"), func() {
				if !netenv.ClusterSupportsIPv6(clusterIPFamily) {
					Skip("Cluster does not support IPv6 - skipping SR-IOV bond IPv6 tests")
				}

				runBondScenario(
					sriovenv.BondModeBalanceXOR,
					mtu1280, mtu9000,
					tsparams.ServerIPv6IPAddress, tsparams.ClientIPv6IPAddress,
					tsparams.ServerIPv6IPAddress2, tsparams.ClientIPv6IPAddress2,
					workerNodeList[0].Definition.Name, workerNodeList[1].Definition.Name,
					bondNetworksV6DiffPFSmall, bondNetworksV6DiffPFJumbo,
				)
			})
		})

		Context("Scale: Bond with 16 VFs", func() {
			const (
				scaleBondMode     = sriovenv.BondModeActiveBackup
				scaleBondNADIPv4  = "sriov-bond-scale-nad"
				scaleBondNADIPv6  = "sriov-bond-scale-nad-ipv6"
				scaleNetAIPv4     = "sriov-bond-scale-a"
				scaleNetBIPv4     = "sriov-bond-scale-b"
				scaleResAIPv4     = "sriovbondscalea"
				scaleResBIPv4     = "sriovbondscaleb"
				scaleNetAIPv6     = "sriov-bond-scale-a-ipv6"
				scaleNetBIPv6     = "sriov-bond-scale-b-ipv6"
				scaleResAIPv6     = "sriovbondscaleaipv6"
				scaleResBIPv6     = "sriovbondscalebipv6"
				scaleTotalVFs     = 16
				scaleSlaveCount   = 16
				scaleVFRangeStart = 16
			)

			createBondScalePolicies := func(
				policyPF1, policyPF2, resourceA, resourceB string,
				mtu, vfStart, numVFs int,
			) {
				vfEnd := vfStart + scaleTotalVFs - 1

				_, err := sriov.NewPolicyBuilder(
					APIClient,
					policyPF1,
					NetConfig.SriovOperatorNamespace,
					resourceA,
					numVFs,
					[]string{pf1},
					NetConfig.WorkerLabelMap).
					WithMTU(mtu).
					WithVFRange(vfStart, vfEnd).
					Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create scale policy for PF1")

				_, err = sriov.NewPolicyBuilder(
					APIClient,
					policyPF2,
					NetConfig.SriovOperatorNamespace,
					resourceB,
					numVFs,
					[]string{pf2},
					NetConfig.WorkerLabelMap).
					WithMTU(mtu).
					WithVFRange(vfStart, vfEnd).
					Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create scale policy for PF2")
			}

			BeforeAll(func() {
				supportsIPv4 := netenv.ClusterSupportsIPv4(clusterIPFamily)
				supportsIPv6 := netenv.ClusterSupportsIPv6(clusterIPFamily)

				if !supportsIPv4 && !supportsIPv6 {
					Skip("Cluster does not support IPv4 or IPv6 - skipping SR-IOV bond scale tests")
				}

				By("Checking that requested interfaces support enough total VFs for scale tests")

				pf1Total, err := sriovenv.GetMinTotalVFsAcrossWorkers(workerNodeList, pf1)
				Expect(err).ToNot(HaveOccurred(), "Failed to get minimum total VFs for PF1 across workers")

				pf2Total, err := sriovenv.GetMinTotalVFsAcrossWorkers(workerNodeList, pf2)
				Expect(err).ToNot(HaveOccurred(), "Failed to get minimum total VFs for PF2 across workers")

				minVFsPerPF := scaleTotalVFs
				if supportsIPv4 && supportsIPv6 {
					minVFsPerPF = scaleTotalVFs * 2
				}

				if pf1Total < minVFsPerPF || pf2Total < minVFsPerPF {
					Skip(fmt.Sprintf(
						"Scale test requires >=%d total VFs on each PF on every worker; min across workers: pf1=%d, pf2=%d",
						minVFsPerPF, pf1Total, pf2Total))
				}

				if supportsIPv4 {
					By("Creating SR-IOV policies and networks for IPv4 16-VF scale test")

					createBondScalePolicies(
						"bond-scale-policy-pf1", "bond-scale-policy-pf2",
						scaleResAIPv4, scaleResBIPv4,
						mtu500, 0, scaleTotalVFs)

					err = sriovenv.CreateSriovBondNetworkWithWhereaboutsIPAM(
						scaleNetAIPv4, scaleResAIPv4,
						tsparams.WhereaboutsIPv4Range, tsparams.WhereaboutsIPv4Gateway, "", "",
					)
					Expect(err).ToNot(HaveOccurred(), "Failed to create IPv4 scale SR-IOV network A")

					err = sriovenv.CreateSriovBondNetworkWithWhereaboutsIPAM(
						scaleNetBIPv4, scaleResBIPv4,
						tsparams.WhereaboutsIPv4Range, tsparams.WhereaboutsIPv4Gateway, "", "",
					)
					Expect(err).ToNot(HaveOccurred(), "Failed to create IPv4 scale SR-IOV network B")
				}

				if supportsIPv6 {
					By("Creating SR-IOV policies and networks for IPv6 16-VF scale test")

					vfStart := 0
					numVFs := scaleTotalVFs

					if supportsIPv4 {
						vfStart = scaleVFRangeStart
						numVFs = scaleTotalVFs * 2
					}

					createBondScalePolicies(
						"bond-scale-policy-pf1-ipv6", "bond-scale-policy-pf2-ipv6",
						scaleResAIPv6, scaleResBIPv6,
						mtu1280, vfStart, numVFs)

					err = sriovenv.CreateSriovBondNetworkWithWhereaboutsIPAM(
						scaleNetAIPv6, scaleResAIPv6,
						tsparams.WhereaboutsIPv6Range, tsparams.WhereaboutsIPv6Gateway, "", "",
					)
					Expect(err).ToNot(HaveOccurred(), "Failed to create IPv6 scale SR-IOV network A")

					err = sriovenv.CreateSriovBondNetworkWithWhereaboutsIPAM(
						scaleNetBIPv6, scaleResBIPv6,
						tsparams.WhereaboutsIPv6Range, tsparams.WhereaboutsIPv6Gateway, "", "",
					)
					Expect(err).ToNot(HaveOccurred(), "Failed to create IPv6 scale SR-IOV network B")
				}

				err = sriovoperator.WaitForSriovAndMCPStable(
					APIClient, tsparams.MCOWaitTimeout, tsparams.DefaultStableDuration,
					NetConfig.CnfMcpLabel, NetConfig.SriovOperatorNamespace)
				Expect(err).ToNot(HaveOccurred(), "Failed waiting for SR-IOV and MCP stability for scale policies")
			})

			AfterAll(func() {
				_ = deleteBondNADIfExists(scaleBondNADIPv4)
				_ = deleteBondNADIfExists(scaleBondNADIPv6)
			})

			AfterEach(func() {
				_ = deleteBondNADIfExists(scaleBondNADIPv4)
				_ = deleteBondNADIfExists(scaleBondNADIPv6)
			})

			runBondScaleICMPTest := func(
				bondNAD, netA, netB string,
				mtu int,
				serverIP, clientIP, icmpPrefix string,
			) {
				By("Creating bond NAD with 16 slave links")

				bondBuilder, err := sriovenv.CreateBondNAD(bondNAD, scaleBondMode, "static", mtu, scaleSlaveCount)
				Expect(err).ToNot(HaveOccurred(), "Failed to build scale bond NAD")

				_, err = bondBuilder.Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create scale bond NAD")

				By("Creating slave network list (8 from each SR-IOV network)")

				var slaveNetworks []string

				for idx := 0; idx < scaleSlaveCount/2; idx++ {
					slaveNetworks = append(slaveNetworks, netA)
				}

				for idx := 0; idx < scaleSlaveCount/2; idx++ {
					slaveNetworks = append(slaveNetworks, netB)
				}

				serverNode := workerNodeList[0].Definition.Name
				clientNode := workerNodeList[1].Definition.Name

				_, clientPod := createBondedPodsPair(
					bondNAD,
					"bond-scale-server", "bond-scale-client",
					serverNode, clientNode,
					slaveNetworks, serverIP, clientIP, mtu,
				)

				By("Verifying bond interface is up, has correct mode and slave count")
				Expect(sriovenv.VerifyBondInterfaceState(
					clientPod, sriovenv.BondInterfaceName, scaleBondMode, scaleSlaveCount)).
					To(Succeed(), "Bond interface validation failed")

				By("Running ICMP connectivity over the bond")

				serverIPNoPrefix := ipaddr.RemovePrefix(serverIP)
				Expect(cmd.ICMPConnectivityCheck(clientPod, []string{serverIPNoPrefix + icmpPrefix}, sriovenv.BondInterfaceName)).
					To(Succeed(), "ICMP connectivity over bond failed")
			}

			It("Verify bond with 16 VFs works with ICMP traffic IPv4", reportxml.ID("89056"), func() {
				if !netenv.ClusterSupportsIPv4(clusterIPFamily) {
					Skip("Cluster does not support IPv4 - skipping SR-IOV bond IPv4 scale test")
				}

				runBondScaleICMPTest(
					scaleBondNADIPv4, scaleNetAIPv4, scaleNetBIPv4,
					mtu500,
					tsparams.ServerIPv4IPAddress, tsparams.ClientIPv4IPAddress, "/32",
				)
			})

			It("Verify bond with 16 VFs works with ICMP traffic IPv6", reportxml.ID("89068"), func() {
				if !netenv.ClusterSupportsIPv6(clusterIPFamily) {
					Skip("Cluster does not support IPv6 - skipping SR-IOV bond IPv6 scale test")
				}

				runBondScaleICMPTest(
					scaleBondNADIPv6, scaleNetAIPv6, scaleNetBIPv6,
					mtu1280,
					tsparams.ServerIPv6IPAddress, tsparams.ClientIPv6IPAddress, "/128",
				)
			})
		})
	})

var (
	// SR-IOV networks used as bond slaves per family/mtu/connectivity.
	// DiffPF = one slave backed by PF1 resource and one by PF2 resource.
	// SamePF = both slaves backed by PF1 resource (two distinct networks).
	// Small = MTU 500 (IPv4) or 1280 (IPv6); Jumbo = MTU 9000.
	bondNetworksV4DiffPFSmall = []string{"sriov-bond-v4-diffpf-small-pf1", "sriov-bond-v4-diffpf-small-pf2"}
	bondNetworksV4DiffPFJumbo = []string{"sriov-bond-v4-diffpf-jumbo-pf1", "sriov-bond-v4-diffpf-jumbo-pf2"}
	bondNetworksV4SamePFSmall = []string{"sriov-bond-v4-samepf-small-a", "sriov-bond-v4-samepf-small-b"}
	bondNetworksV4SamePFJumbo = []string{"sriov-bond-v4-samepf-jumbo-a", "sriov-bond-v4-samepf-jumbo-b"}

	bondNetworksV6DiffPFSmall = []string{"sriov-bond-v6-diffpf-small-pf1", "sriov-bond-v6-diffpf-small-pf2"}
	bondNetworksV6DiffPFJumbo = []string{"sriov-bond-v6-diffpf-jumbo-pf1", "sriov-bond-v6-diffpf-jumbo-pf2"}
	bondNetworksV6SamePFSmall = []string{"sriov-bond-v6-samepf-small-a", "sriov-bond-v6-samepf-small-b"}
	bondNetworksV6SamePFJumbo = []string{"sriov-bond-v6-samepf-jumbo-a", "sriov-bond-v6-samepf-jumbo-b"}
)

//nolint:unparam // mtu values are fixed by the suite's MTU matrix.
func runBondScenario(
	bondMode string,
	mtuSmall int,
	mtuLarge int,
	serverIPSmall string,
	clientIPSmall string,
	serverIPLarge string,
	clientIPLarge string,
	serverNode string,
	clientNode string,
	slaveNetworksSmall []string,
	slaveNetworksLarge []string,
) {
	nadSmall := bondNADNameFor(bondMode, mtuSmall)
	nadLarge := bondNADNameFor(bondMode, mtuLarge)

	By("Creating bond NADs for both MTUs")

	_ = deleteBondNADIfExists(nadSmall)
	_ = deleteBondNADIfExists(nadLarge)

	bondSmall, err := sriovenv.CreateBondNAD(nadSmall, bondMode, "static", mtuSmall, 2)
	Expect(err).ToNot(HaveOccurred(), "Failed to build small MTU bond NAD")

	_, err = bondSmall.Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create small MTU bond NAD")

	defer func() { _ = deleteBondNADIfExists(nadSmall) }()

	bondLarge, err := sriovenv.CreateBondNAD(nadLarge, bondMode, "static", mtuLarge, 2)
	Expect(err).ToNot(HaveOccurred(), "Failed to build large MTU bond NAD")

	_, err = bondLarge.Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create large MTU bond NAD")

	defer func() { _ = deleteBondNADIfExists(nadLarge) }()

	modeSuffix := strings.ReplaceAll(bondMode, "balance-", "")
	serverSmallName := fmt.Sprintf("bond-server-%s-mtu%d", modeSuffix, mtuSmall)
	clientSmallName := fmt.Sprintf("bond-client-%s-mtu%d", modeSuffix, mtuSmall)
	serverLargeName := fmt.Sprintf("bond-server-%s-mtu%d", modeSuffix, mtuLarge)
	clientLargeName := fmt.Sprintf("bond-client-%s-mtu%d", modeSuffix, mtuLarge)

	By("Creating server and client pods for both MTUs (4 pods total)")

	_, clientSmall := createBondedPodsPair(
		nadSmall,
		serverSmallName, clientSmallName,
		serverNode, clientNode,
		slaveNetworksSmall, serverIPSmall, clientIPSmall, mtuSmall,
	)
	_, clientLarge := createBondedPodsPair(
		nadLarge,
		serverLargeName, clientLargeName,
		serverNode, clientNode,
		slaveNetworksLarge, serverIPLarge, clientIPLarge, mtuLarge,
	)

	By("Verifying traffic on bond interface for both MTUs")
	Expect(verifyInitialBondTraffic(bondMode, clientSmall, serverIPSmall, mtuSmall, "small MTU", false)).
		To(Succeed(), "Bond initial traffic failed (small MTU)")
	Expect(verifyInitialBondTraffic(bondMode, clientLarge, serverIPLarge, mtuLarge, "large MTU", false)).
		To(Succeed(), "Bond initial traffic failed (large MTU)")

	By("Triggering link failure and verifying traffic still works for both MTUs")
	Expect(triggerBondLinkFailureAndVerify(clientSmall, bondMode, serverIPSmall, mtuSmall)).
		To(Succeed(), "Bond link failure verification failed (small MTU)")
	Expect(triggerBondLinkFailureAndVerify(clientLarge, bondMode, serverIPLarge, mtuLarge)).
		To(Succeed(), "Bond link failure verification failed (large MTU)")
}

func verifyInitialBondTraffic(
	bondMode string,
	clientPod *pod.Builder,
	serverIP string,
	mtu int,
	desc string,
	afterFailover bool,
) error {
	timeout := tsparams.WaitTimeout
	pollInterval := tsparams.NADWaitTimeout

	check := func() error {
		return sriovenv.RunTrafficTest(clientPod, serverIP, mtu, sriovenv.BondInterfaceName)
	}

	if afterFailover {
		timeout = sriovenv.BondActiveSlaveChangeTimeout
		pollInterval = time.Second
		check = func() error {
			return runBondConnectivityAfterFailover(clientPod, serverIP, mtu, bondMode)
		}

		if bondMode == sriovenv.BondModeBalanceXOR {
			timeout = 2 * time.Minute
			pollInterval = tsparams.NADWaitTimeout
		}
	}

	deadline := time.Now().Add(timeout)

	var lastErr error

	for {
		lastErr = check()
		if lastErr == nil {
			return nil
		}

		if time.Now().After(deadline) {
			if afterFailover {
				return fmt.Errorf("bond traffic failed after failover (%s) within %v: %w", desc, timeout, lastErr)
			}

			return fmt.Errorf("traffic tests failed on bond interface (%s, mode %s) within %v: %w",
				desc, bondMode, timeout, lastErr)
		}

		time.Sleep(pollInterval)
	}
}

func createBondedPodsPair(
	nadName string,
	serverName, clientName,
	serverNode, clientNode string,
	slaveNetworks []string,
	serverIPWithCIDR, clientIPWithCIDR string,
	mtu int,
) (*pod.Builder, *pod.Builder) {
	annotationServer := pod.StaticIPBondAnnotationWithInterface(
		nadName,
		sriovenv.BondInterfaceName,
		slaveNetworks,
		[]string{serverIPWithCIDR},
	)
	Expect(annotationServer).NotTo(BeNil(), "Failed to create bond annotation for server pod")

	annotationClient := pod.StaticIPBondAnnotationWithInterface(
		nadName,
		sriovenv.BondInterfaceName,
		slaveNetworks,
		[]string{clientIPWithCIDR},
	)
	Expect(annotationClient).NotTo(BeNil(), "Failed to create bond annotation for client pod")

	serverBindIP := ipaddr.RemovePrefix(serverIPWithCIDR)
	serverCmd := sriovenv.BuildServerCommand(serverBindIP, sriovenv.BondInterfaceName, mtu)

	serverContainer, err := pod.NewContainerBuilder("server", NetConfig.CnfNetTestContainer, serverCmd).GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Failed to build server container config")

	serverPodBuilder := pod.NewBuilder(APIClient, serverName, tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer)

	serverPod, err := serverPodBuilder.
		DefineOnNode(serverNode).
		RedefineDefaultContainer(*serverContainer).
		WithPrivilegedFlag().
		WithSecondaryNetwork(annotationServer).
		CreateAndWaitUntilRunning(netparam.DefaultTimeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to create server pod")

	Expect(sriovenv.WaitForServerReady(serverPod, tsparams.WaitTimeout)).
		To(Succeed(), "Server pod testcmd listeners not ready")

	clientPodBuilder := pod.NewBuilder(APIClient, clientName, tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer)

	clientPod, err := clientPodBuilder.
		DefineOnNode(clientNode).
		WithPrivilegedFlag().
		WithSecondaryNetwork(annotationClient).
		CreateAndWaitUntilRunning(netparam.DefaultTimeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to create client pod")

	Expect(sriovenv.WaitForBondSlavesMIIUp(serverPod, sriovenv.BondInterfaceName)).
		To(Succeed(), "Server bond slaves not MII up in pod %s", serverName)
	Expect(sriovenv.WaitForBondSlavesMIIUp(clientPod, sriovenv.BondInterfaceName)).
		To(Succeed(), "Client bond slaves not MII up in pod %s", clientName)

	return serverPod, clientPod
}

// bondNetworkConfig describes a single SR-IOV network for bond slave creation.
type bondNetworkConfig struct {
	name,
	resource,
	ipRange,
	gateway string
}

type bondStackFamilyParams struct {
	familyLabel  string
	policyPrefix string
	dualStack    bool
	policyNumVFs int
	pf1,
	pf2 string
	mtuSmall,
	mtuLarge int
	vfSmallStart,
	vfSmallEnd,
	vfLargeStart,
	vfLargeEnd int
	resPF1Small,
	resPF1Large,
	resPF2Small,
	resPF2Large string
	networks []bondNetworkConfig
}

func setupBondStackForFamily(params bondStackFamilyParams) {
	By(fmt.Sprintf("Creating SR-IOV policies and networks for %s bond tests", params.familyLabel))

	if params.dualStack {
		policies := []struct {
			pfSuffix string
			resource string
			pf       string
			mtu      int
			vfStart  int
			vfEnd    int
		}{
			{"pf1", params.resPF1Small, params.pf1, params.mtuSmall, params.vfSmallStart, params.vfSmallEnd},
			{"pf1", params.resPF1Large, params.pf1, params.mtuLarge, params.vfLargeStart, params.vfLargeEnd},
			{"pf2", params.resPF2Small, params.pf2, params.mtuSmall, params.vfSmallStart, params.vfSmallEnd},
			{"pf2", params.resPF2Large, params.pf2, params.mtuLarge, params.vfLargeStart, params.vfLargeEnd},
		}

		for _, policy := range policies {
			policyName := fmt.Sprintf("%s-policy-%s-mtu%d", params.policyPrefix, policy.pfSuffix, policy.mtu)

			Expect(sriovenv.CreateSriovPolicy(
				policyName, policy.resource, policy.pf, policy.mtu, policy.vfStart, policy.vfEnd,
				params.policyNumVFs,
			)).To(Succeed(), "Failed to create %s %s MTU%d policy",
				params.familyLabel, policy.pfSuffix, policy.mtu)
		}

		Expect(sriovoperator.WaitForSriovAndMCPStable(
			APIClient, tsparams.MCOWaitTimeout, tsparams.DefaultStableDuration,
			NetConfig.CnfMcpLabel, NetConfig.SriovOperatorNamespace)).
			To(Succeed(), "Failed to wait for SR-IOV and MCP stability after %s bond policies", params.familyLabel)
	} else {
		err := sriovenv.CreateAllSriovPolicies(
			params.pf1, params.pf2,
			params.resPF1Small, params.resPF1Large,
			params.resPF2Small, params.resPF2Large,
			params.policyPrefix, params.mtuSmall, params.mtuLarge)
		Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV policies for %s bond tests", params.familyLabel)
	}

	Expect(createBondWhereaboutsNetworks(params.networks)).
		To(Succeed(), "Failed to create %s bond networks", params.familyLabel)
}

func bondWhereaboutsNetworkConfigs(
	diffPFSmall,
	diffPFJumbo,
	samePFSmall,
	samePFJumbo []string,
	resPF1Small,
	resPF1Jumbo,
	resPF2Small,
	resPF2Jumbo,
	ipRange,
	gateway,
	ipRangeLarge,
	gatewayLarge string,
) []bondNetworkConfig {
	return []bondNetworkConfig{
		{name: diffPFSmall[0], resource: resPF1Small, ipRange: ipRange, gateway: gateway},
		{name: diffPFJumbo[0], resource: resPF1Jumbo, ipRange: ipRangeLarge, gateway: gatewayLarge},
		{name: diffPFSmall[1], resource: resPF2Small, ipRange: ipRange, gateway: gateway},
		{name: diffPFJumbo[1], resource: resPF2Jumbo, ipRange: ipRangeLarge, gateway: gatewayLarge},
		{name: samePFSmall[0], resource: resPF1Small, ipRange: ipRange, gateway: gateway},
		{name: samePFSmall[1], resource: resPF1Small, ipRange: ipRange, gateway: gateway},
		{name: samePFJumbo[0], resource: resPF1Jumbo, ipRange: ipRangeLarge, gateway: gatewayLarge},
		{name: samePFJumbo[1], resource: resPF1Jumbo, ipRange: ipRangeLarge, gateway: gatewayLarge},
	}
}

// createBondWhereaboutsNetworks creates bond slave SriovNetworks (Trust on, SpoofChk off, LinkState auto).
func createBondWhereaboutsNetworks(configs []bondNetworkConfig) error {
	for _, cfg := range configs {
		if err := sriovenv.CreateSriovBondNetworkWithWhereaboutsIPAM(
			cfg.name, cfg.resource, cfg.ipRange, cfg.gateway, "", ""); err != nil {
			return fmt.Errorf("failed to create network %s: %w", cfg.name, err)
		}
	}

	return nil
}

func bondICMPDestination(serverIPWithCIDR string) string {
	ipNoPrefix := ipaddr.RemovePrefix(serverIPWithCIDR)
	if net.ParseIP(ipNoPrefix).To4() == nil {
		return ipNoPrefix + "/128"
	}

	return ipNoPrefix + "/32"
}

// runBondConnectivityAfterFailover validates reachability after a slave/link failure.
// Balance modes use ICMP only (TCP can flake during rebalance); active-backup keeps full traffic tests.
func runBondConnectivityAfterFailover(clientPod *pod.Builder, serverIP string, mtu int, bondMode string) error {
	if bondMode == sriovenv.BondModeActiveBackup {
		return sriovenv.RunTrafficTest(clientPod, serverIP, mtu, sriovenv.BondInterfaceName)
	}

	return cmd.ICMPConnectivityCheck(clientPod, []string{bondICMPDestination(serverIP)}, sriovenv.BondInterfaceName)
}

func triggerBondLinkFailureAndVerify(clientPod *pod.Builder, bondMode, serverIP string, mtu int) error {
	// active-backup exposes active_slave and should switch deterministically.
	if bondMode == sriovenv.BondModeActiveBackup {
		active, err := sriovenv.GetBondActiveSlave(clientPod, sriovenv.BondInterfaceName)
		if err != nil {
			return err
		}

		if err := sriovenv.SetLinkStatus(clientPod, active, "down"); err != nil {
			return fmt.Errorf("failed to bring active slave %s down: %w", active, err)
		}

		if _, err := sriovenv.WaitForBondActiveSlaveChange(clientPod, sriovenv.BondInterfaceName, active); err != nil {
			return err
		}

		if err := verifyInitialBondTraffic(bondMode, clientPod, serverIP, mtu, "after failover", true); err != nil {
			return fmt.Errorf("traffic failed after failover: %w", err)
		}

		if err := sriovenv.SetLinkStatus(clientPod, active, "up"); err != nil {
			return fmt.Errorf("failed to bring active slave %s up: %w", active, err)
		}

		if err := sriovenv.WaitForBondSlavesMIIUp(clientPod, sriovenv.BondInterfaceName); err != nil {
			return fmt.Errorf("bond did not recover after bringing %s up: %w", active, err)
		}

		return nil
	}

	// For balance-rr / balance-xor, prefer switching off a physical port on the lab switch
	// (matches upstream behavior) when switch credentials/interfaces are available.
	switchErr := toggleSwitchPortsAndVerifyTraffic(clientPod, serverIP, mtu, bondMode)
	if switchErr == nil {
		return nil
	}

	klog.V(90).Infof("Switch-based link failure check failed (%v), falling back to pod slave link toggle", switchErr)

	// Fallback: validate resilience by toggling the slave links in the pod.
	if err := sriovenv.SetLinkStatus(clientPod, sriovenv.BondSlave1IfName, "down"); err != nil {
		return fmt.Errorf("failed to bring %s down: %w", sriovenv.BondSlave1IfName, err)
	}

	if err := sriovenv.WaitForBondSlaveMIIDown(
		clientPod, sriovenv.BondInterfaceName, sriovenv.BondSlave1IfName); err != nil {
		return err
	}

	if err := verifyInitialBondTraffic(bondMode, clientPod, serverIP, mtu, "after failover", true); err != nil {
		return err
	}

	if err := sriovenv.SetLinkStatus(clientPod, sriovenv.BondSlave1IfName, "up"); err != nil {
		return fmt.Errorf("failed to bring %s up: %w", sriovenv.BondSlave1IfName, err)
	}

	if err := sriovenv.WaitForBondSlavesMIIUp(clientPod, sriovenv.BondInterfaceName); err != nil {
		return fmt.Errorf("bond did not recover after bringing %s up: %w", sriovenv.BondSlave1IfName, err)
	}

	if err := sriovenv.SetLinkStatus(clientPod, sriovenv.BondSlave2IfName, "down"); err != nil {
		return fmt.Errorf("failed to bring %s down: %w", sriovenv.BondSlave2IfName, err)
	}

	if err := sriovenv.WaitForBondSlaveMIIDown(
		clientPod, sriovenv.BondInterfaceName, sriovenv.BondSlave2IfName); err != nil {
		return err
	}

	if err := verifyInitialBondTraffic(bondMode, clientPod, serverIP, mtu, "after failover", true); err != nil {
		return err
	}

	if err := sriovenv.SetLinkStatus(clientPod, sriovenv.BondSlave2IfName, "up"); err != nil {
		return fmt.Errorf("failed to bring %s up: %w", sriovenv.BondSlave2IfName, err)
	}

	if err := sriovenv.WaitForBondSlavesMIIUp(clientPod, sriovenv.BondInterfaceName); err != nil {
		return fmt.Errorf("bond did not recover after bringing %s up: %w", sriovenv.BondSlave2IfName, err)
	}

	return nil
}

func waitForSwitchInterfaceUp(jnpr *cmd.Junos, switchInterface string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for {
		up, err := jnpr.IsSwitchInterfaceUp(switchInterface)
		if err == nil && up {
			return nil
		}

		if time.Now().After(deadline) {
			if err != nil {
				return fmt.Errorf("switch interface %s not up within %v: %w", switchInterface, timeout, err)
			}

			return fmt.Errorf("switch interface %s not up within %v", switchInterface, timeout)
		}

		time.Sleep(tsparams.NADWaitTimeout)
	}
}

// toggleSwitchPortsAndVerifyTraffic mirrors cnf-gotests TestActiveActiveBondScenario switch failover:
// disable first LAG member, verify traffic, re-enable it, disable second member, wait for ae up, verify again.
func toggleSwitchPortsAndVerifyTraffic(clientPod *pod.Builder, serverIP string, mtu int, bondMode string) error {
	if bondSwitchCredentials == nil || len(bondSwitchInterfaces) < 2 || len(bondSwitchLagNames) < 1 {
		return fmt.Errorf("switch LAG not configured (need credentials, 2+ interfaces, LAG name)")
	}

	jnpr, err := cmd.NewSession(
		bondSwitchCredentials.SwitchIP, bondSwitchCredentials.User, bondSwitchCredentials.Password)
	if err != nil {
		return err
	}
	defer jnpr.Close()

	disable := func(iface string) error {
		return jnpr.Config([]string{fmt.Sprintf("set interfaces %s disable", iface)})
	}
	enable := func(iface string) error {
		return jnpr.Config([]string{fmt.Sprintf("delete interfaces %s disable", iface)})
	}

	firstPort := bondSwitchInterfaces[0]
	secondPort := bondSwitchInterfaces[1]
	lagName := bondSwitchLagNames[0]

	err = disable(firstPort)
	if err != nil {
		return fmt.Errorf("failed to disable switch interface %s: %w", firstPort, err)
	}

	defer func() {
		_ = enable(firstPort)
	}()

	if err = verifyInitialBondTraffic(bondMode, clientPod, serverIP, mtu, "after failover", true); err != nil {
		return fmt.Errorf("traffic failed after disabling switch port %s: %w", firstPort, err)
	}

	if err = enable(firstPort); err != nil {
		return fmt.Errorf("failed to re-enable switch interface %s: %w", firstPort, err)
	}

	err = disable(secondPort)
	if err != nil {
		return fmt.Errorf("failed to disable switch interface %s: %w", secondPort, err)
	}

	defer func() {
		_ = enable(secondPort)
	}()

	if err = waitForSwitchInterfaceUp(jnpr, lagName, time.Minute); err != nil {
		return err
	}

	if err = verifyInitialBondTraffic(bondMode, clientPod, serverIP, mtu, "after failover", true); err != nil {
		return fmt.Errorf("traffic failed after disabling switch port %s: %w", secondPort, err)
	}

	if err = enable(secondPort); err != nil {
		return fmt.Errorf("failed to re-enable switch interface %s: %w", secondPort, err)
	}

	if err = sriovenv.WaitForBondSlavesMIIUp(clientPod, sriovenv.BondInterfaceName); err != nil {
		return fmt.Errorf("bond did not recover after switch failover: %w", err)
	}

	return nil
}

func bondNADNameFor(bondMode string, mtu int) string {
	return fmt.Sprintf("%s-%s-mtu%d", bondNADName, bondMode, mtu)
}

func bondNADNamesForCleanup() []string {
	modes := []string{
		sriovenv.BondModeActiveBackup,
		sriovenv.BondModeBalanceRR,
		sriovenv.BondModeBalanceXOR,
	}
	mtus := []int{mtu500, mtu1280, mtu9000}

	seen := make(map[string]struct{})

	var names []string

	add := func(name string) {
		if _, exists := seen[name]; exists {
			return
		}

		seen[name] = struct{}{}
		names = append(names, name)
	}

	for _, mode := range modes {
		for _, mtu := range mtus {
			add(bondNADNameFor(mode, mtu))
		}
	}

	// Legacy MTU-only NAD names from earlier revisions.
	add(bondNADName)

	for _, mtu := range mtus {
		add(fmt.Sprintf("%s-mtu%d", bondNADName, mtu))
	}

	return names
}

func setupBondSwitchLAGForActiveActiveTests() error {
	var err error

	if bondSwitchCredentials == nil {
		bondSwitchCredentials, err = sriovenv.NewSwitchCredentials()
		if err != nil {
			return fmt.Errorf("switch credentials: %w", err)
		}
	}

	bondSwitchInterfaces, err = NetConfig.GetSwitchInterfaces()
	if err != nil {
		return fmt.Errorf("switch interfaces: %w", err)
	}

	if len(bondSwitchInterfaces) != bondMinSwitchInterfaces {
		return fmt.Errorf("need %d switch interfaces (ECO_CNF_CORE_NET_SWITCH_INTERFACES), got %d",
			bondMinSwitchInterfaces, len(bondSwitchInterfaces))
	}

	bondSwitchLagNames, err = NetConfig.GetSwitchLagNames()
	if err != nil {
		return fmt.Errorf("switch LAG names: %w", err)
	}

	switchClusterVLAN, err := NetConfig.GetBondSwitchVLANID()
	if err != nil {
		return fmt.Errorf("cluster VLAN: %w", err)
	}

	switchTrunkVLANs, err := NetConfig.GetSwitchTrunkVLANIDs()
	if err != nil {
		return fmt.Errorf("trunk VLANs: %w", err)
	}

	klog.Infof(
		"Bond switch LAG vlan%d, trunk VLANs %v (ECO_CNF_CORE_NET_VLAN=%q, ECO_CNF_CORE_NET_SWITCH_TRUNK_VLANS=%q)",
		switchClusterVLAN, switchTrunkVLANs, NetConfig.VLAN, NetConfig.SwitchTrunkVLANs)

	bondSwitchSavedConfigs, err = configureStaticLAGsOnSwitch(
		bondSwitchCredentials, bondSwitchInterfaces, bondSwitchLagNames)
	if err != nil {
		if len(bondSwitchSavedConfigs) > 0 {
			if restoreErr := restoreBondSwitchLAG(
				bondSwitchCredentials, bondSwitchInterfaces, bondSwitchLagNames, bondSwitchSavedConfigs,
			); restoreErr != nil {
				return fmt.Errorf("configure static LAGs: %w (restore failed: %w)", err, restoreErr)
			}

			bondSwitchSavedConfigs = nil
		}

		return fmt.Errorf("configure static LAGs: %w", err)
	}

	klog.Infof("Configured static switch LAGs %v on ports %v", bondSwitchLagNames, bondSwitchInterfaces)

	return nil
}

func restoreBondSwitchLAGAfterActiveActiveTest() {
	if len(bondSwitchSavedConfigs) == 0 || bondSwitchCredentials == nil {
		return
	}

	By("Restoring lab switch configuration after active-active bond test")

	err := restoreBondSwitchLAG(
		bondSwitchCredentials, bondSwitchInterfaces, bondSwitchLagNames, bondSwitchSavedConfigs)
	Expect(err).ToNot(HaveOccurred(), "Failed to restore lab switch configuration after active-active bond test")

	bondSwitchSavedConfigs = nil
}

func bondStaticLAGCleanupCommands(physicalInterfaces, lagInterfaces []string) []string {
	var cleanupCommands []string

	for _, physicalInterface := range physicalInterfaces {
		cleanupCommands = append(cleanupCommands,
			fmt.Sprintf("delete interfaces %s ether-options 802.3ad", physicalInterface))
	}

	for _, lagInterface := range lagInterfaces {
		cleanupCommands = append(cleanupCommands, fmt.Sprintf("delete interfaces %s", lagInterface))
	}

	for _, physicalInterface := range physicalInterfaces {
		cleanupCommands = append(cleanupCommands, fmt.Sprintf("delete interfaces %s", physicalInterface))
	}

	return cleanupCommands
}

// configureStaticLAGsOnSwitch mirrors cnf-gotests configureLAGsOnSwitch: wipe the four physical
// ports, create two static (non-LACP) 802.3ad LAGs, and trunk lab VLANs on each ae.
func configureStaticLAGsOnSwitch(
	credentials *sriovenv.SwitchCredentials,
	physicalInterfaces, lagInterfaces []string,
) ([]string, error) {
	if len(physicalInterfaces) != bondMinSwitchInterfaces {
		return nil, fmt.Errorf("need %d switch interfaces, got %d", bondMinSwitchInterfaces, len(physicalInterfaces))
	}

	if len(lagInterfaces) != 2 {
		return nil, fmt.Errorf("need 2 switch LAG names, got %d", len(lagInterfaces))
	}

	jnpr, err := cmd.NewSession(credentials.SwitchIP, credentials.User, credentials.Password)
	if err != nil {
		return nil, err
	}
	defer jnpr.Close()

	savedConfigs, err := jnpr.SaveInterfaceConfigs(physicalInterfaces)
	if err != nil {
		return nil, fmt.Errorf("save switch interface configs: %w", err)
	}

	cleanupCommands := bondStaticLAGCleanupCommands(physicalInterfaces, lagInterfaces)

	if len(cleanupCommands) > 0 {
		if err := jnpr.Config(cleanupCommands); err != nil {
			return savedConfigs, fmt.Errorf("clean switch interfaces before LAG setup: %w", err)
		}
	}

	vlan, err := NetConfig.GetBondSwitchVLANID()
	if err != nil {
		return rollbackBondSwitchLAGSetup(
			credentials, physicalInterfaces, lagInterfaces, savedConfigs,
			fmt.Errorf("cluster VLAN: %w", err))
	}

	trunkVLANs, err := NetConfig.GetSwitchTrunkVLANIDs()
	if err != nil {
		return rollbackBondSwitchLAGSetup(
			credentials, physicalInterfaces, lagInterfaces, savedConfigs,
			fmt.Errorf("trunk VLANs: %w", err))
	}

	lagMembers := [][2]string{
		{physicalInterfaces[0], physicalInterfaces[1]},
		{physicalInterfaces[2], physicalInterfaces[3]},
	}

	var configureCommands []string

	for idx, lagInterface := range lagInterfaces {
		for _, physicalInterface := range lagMembers[idx] {
			configureCommands = append(configureCommands,
				fmt.Sprintf("set interfaces %s ether-options 802.3ad %s", physicalInterface, lagInterface))
		}

		configureCommands = append(configureCommands,
			fmt.Sprintf("set interfaces %s aggregated-ether-options lacp disable", lagInterface),
			fmt.Sprintf("set interfaces %s unit 0 family ethernet-switching interface-mode trunk", lagInterface),
			fmt.Sprintf("set interfaces %s native-vlan-id %d", lagInterface, vlan),
			fmt.Sprintf("set interfaces %s mtu 9216", lagInterface),
		)

		for _, trunkVLAN := range trunkVLANs {
			configureCommands = append(configureCommands,
				fmt.Sprintf("set interfaces %s unit 0 family ethernet-switching interface-mode trunk vlan "+
					"members vlan%d", lagInterface, trunkVLAN))
		}
	}

	if err := jnpr.Config(configureCommands); err != nil {
		return rollbackBondSwitchLAGSetup(
			credentials, physicalInterfaces, lagInterfaces, savedConfigs,
			fmt.Errorf("configure static LAGs: %w", err))
	}

	return savedConfigs, nil
}

func rollbackBondSwitchLAGSetup(
	credentials *sriovenv.SwitchCredentials,
	physicalInterfaces, lagInterfaces, savedConfigs []string,
	setupErr error,
) ([]string, error) {
	klog.Warningf("Bond switch LAG setup failed, restoring saved interface configs: %v", setupErr)

	if restoreErr := restoreBondSwitchLAG(
		credentials, physicalInterfaces, lagInterfaces, savedConfigs,
	); restoreErr != nil {
		return savedConfigs, fmt.Errorf("%w (failed to restore switch interfaces: %w)", setupErr, restoreErr)
	}

	return savedConfigs, setupErr
}

func restoreBondSwitchLAG(
	credentials *sriovenv.SwitchCredentials,
	physicalInterfaces, lagInterfaces, savedConfigs []string,
) error {
	if credentials == nil || len(savedConfigs) == 0 {
		return nil
	}

	jnpr, err := cmd.NewSession(credentials.SwitchIP, credentials.User, credentials.Password)
	if err != nil {
		return err
	}
	defer jnpr.Close()

	if len(lagInterfaces) > 0 && len(physicalInterfaces) > 0 {
		if err := jnpr.DisableLACP(lagInterfaces, physicalInterfaces); err != nil {
			klog.V(90).Infof("Failed to remove static LAG configuration from switch: %v", err)
		}
	}

	return jnpr.RestoreInterfaceConfigs(savedConfigs)
}

func deleteBondNADIfExists(name string) error {
	nadBuilder := nad.NewBuilder(APIClient, name, tsparams.TestNamespaceName)

	_, err := nadBuilder.Get()
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}

		return err
	}

	return nadBuilder.Delete()
}

func bondWhereaboutsIPAMForMTU(mtu int) (ipRange, gateway string) {
	ipRange = tsparams.WhereaboutsIPv6Range
	gateway = tsparams.WhereaboutsIPv6Gateway

	if mtu >= mtu9000 {
		ipRange = tsparams.WhereaboutsIPv6Range2
		gateway = tsparams.WhereaboutsIPv6Gateway2
	}

	return ipRange, gateway
}

func runBondWhereaboutsScenario(
	bondMode string,
	mtuSmall, mtuLarge int,
	serverNode, clientNode string,
	slaveNetworksSmall, slaveNetworksLarge []string,
) {
	nadSmall := bondNADNameFor(bondMode, mtuSmall)
	nadLarge := bondNADNameFor(bondMode, mtuLarge)

	By("Creating bond NADs with whereabouts IPAM for both MTUs")

	_ = deleteBondNADIfExists(nadSmall)
	_ = deleteBondNADIfExists(nadLarge)

	ipRangeSmall, gatewaySmall := bondWhereaboutsIPAMForMTU(mtuSmall)

	bondSmall, err := sriovenv.CreateBondNADWithWhereabouts(
		nadSmall, bondMode, mtuSmall, 2, ipRangeSmall, gatewaySmall)
	Expect(err).ToNot(HaveOccurred(), "Failed to build small MTU whereabouts bond NAD")

	_, err = bondSmall.Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create small MTU whereabouts bond NAD")

	defer func() { _ = deleteBondNADIfExists(nadSmall) }()

	ipRangeLarge, gatewayLarge := bondWhereaboutsIPAMForMTU(mtuLarge)

	bondLarge, err := sriovenv.CreateBondNADWithWhereabouts(
		nadLarge, bondMode, mtuLarge, 2, ipRangeLarge, gatewayLarge)
	Expect(err).ToNot(HaveOccurred(), "Failed to build large MTU whereabouts bond NAD")

	_, err = bondLarge.Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create large MTU whereabouts bond NAD")

	defer func() { _ = deleteBondNADIfExists(nadLarge) }()

	modeSuffix := strings.ReplaceAll(bondMode, "balance-", "")
	serverSmallName := fmt.Sprintf("bond-wb-server-%s-mtu%d", modeSuffix, mtuSmall)
	clientSmallName := fmt.Sprintf("bond-wb-client-%s-mtu%d", modeSuffix, mtuSmall)
	serverLargeName := fmt.Sprintf("bond-wb-server-%s-mtu%d", modeSuffix, mtuLarge)
	clientLargeName := fmt.Sprintf("bond-wb-client-%s-mtu%d", modeSuffix, mtuLarge)

	By("Creating server and client pods for both MTUs (4 pods total)")

	serverSmall, clientSmall := createBondWhereaboutsPodsPair(
		nadSmall,
		serverSmallName, clientSmallName,
		serverNode, clientNode,
		slaveNetworksSmall, mtuSmall,
	)
	serverLarge, clientLarge := createBondWhereaboutsPodsPair(
		nadLarge,
		serverLargeName, clientLargeName,
		serverNode, clientNode,
		slaveNetworksLarge, mtuLarge,
	)

	By("Discovering server bond IPs assigned by whereabouts")

	serverIPSmall, err := sriovenv.GetPodIPFromInterface(serverSmall, sriovenv.BondInterfaceName, "ipv6")
	Expect(err).ToNot(HaveOccurred(), "Failed to get server bond IP for small MTU")

	serverIPLarge, err := sriovenv.GetPodIPFromInterface(serverLarge, sriovenv.BondInterfaceName, "ipv6")
	Expect(err).ToNot(HaveOccurred(), "Failed to get server bond IP for large MTU")

	By("Verifying traffic on bond interface for both MTUs")
	Expect(verifyInitialBondTraffic(bondMode, clientSmall, serverIPSmall, mtuSmall, "small MTU", false)).
		To(Succeed(), "Bond initial traffic failed (small MTU)")
	Expect(verifyInitialBondTraffic(bondMode, clientLarge, serverIPLarge, mtuLarge, "large MTU", false)).
		To(Succeed(), "Bond initial traffic failed (large MTU)")

	By("Triggering link failure and verifying traffic still works for both MTUs")
	Expect(triggerBondLinkFailureAndVerify(clientSmall, bondMode, serverIPSmall, mtuSmall)).
		To(Succeed(), "Bond link failure verification failed (small MTU)")
	Expect(triggerBondLinkFailureAndVerify(clientLarge, bondMode, serverIPLarge, mtuLarge)).
		To(Succeed(), "Bond link failure verification failed (large MTU)")
}

func createBondWhereaboutsPodsPair(
	nadName string,
	serverName, clientName,
	serverNode, clientNode string,
	slaveNetworks []string,
	mtu int,
) (*pod.Builder, *pod.Builder) {
	annotation := bondWhereaboutsPodAnnotation(nadName, slaveNetworks)
	Expect(annotation).NotTo(BeEmpty(), "Failed to create whereabouts bond pod annotation")

	baseCmd := sriovenv.BuildServerCommand("", sriovenv.BondInterfaceName, mtu)
	serverCmd := bondWhereaboutsServerCommand(baseCmd[2])

	serverContainer, err := pod.NewContainerBuilder("server", NetConfig.CnfNetTestContainer, serverCmd).GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Failed to build server container config")

	serverPod, err := pod.NewBuilder(APIClient, serverName, tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
		DefineOnNode(serverNode).
		RedefineDefaultContainer(*serverContainer).
		WithPrivilegedFlag().
		WithSecondaryNetwork(annotation).
		CreateAndWaitUntilRunning(netparam.DefaultTimeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to create server pod")

	Expect(sriovenv.WaitForBondSlavesMIIUp(serverPod, sriovenv.BondInterfaceName)).
		To(Succeed(), "Server bond slaves not MII up in pod %s", serverName)
	Expect(sriovenv.WaitForServerReady(serverPod, tsparams.WaitTimeout)).
		To(Succeed(), "Server pod testcmd listeners not ready")

	clientPod, err := pod.NewBuilder(APIClient, clientName, tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
		DefineOnNode(clientNode).
		WithPrivilegedFlag().
		WithSecondaryNetwork(annotation).
		CreateAndWaitUntilRunning(netparam.DefaultTimeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to create client pod")

	Expect(sriovenv.WaitForBondSlavesMIIUp(clientPod, sriovenv.BondInterfaceName)).
		To(Succeed(), "Client bond slaves not MII up in pod %s", clientName)

	return serverPod, clientPod
}

// bondWhereaboutsServerCommand wraps the dynamic whereabouts server script with a bond-ready
// wait. Bond slaves and the whereabouts IP take longer to settle than a single SR-IOV net1.
func bondWhereaboutsServerCommand(baseScript string) []string {
	bondName := sriovenv.BondInterfaceName
	bondWait := fmt.Sprintf(
		"for _ in $(seq 1 60); do "+
			"[ \"$(cat /sys/class/net/%s/operstate 2>/dev/null)\" = up ] || { sleep 2; continue; }; "+
			"BOND=/proc/net/bonding/%s; [ -r \"$BOND\" ] || { sleep 2; continue; }; "+
			"SLAVE_UP=$(grep -c 'MII Status: up' \"$BOND\" 2>/dev/null || true); "+
			"SLAVE_DOWN=$(grep -c 'MII Status: down' \"$BOND\" 2>/dev/null || true); "+
			"[ \"${SLAVE_UP:-0}\" -ge 2 ] && [ \"${SLAVE_DOWN:-0}\" -eq 0 ] || { sleep 2; continue; }; "+
			"ip -6 -o addr show %s 2>/dev/null | grep -v fe80 | grep -q . && break; "+
			"sleep 2; done; sleep 5; ",
		bondName, bondName, bondName)

	return []string{"bash", "-c", bondWait + baseScript}
}

func bondWhereaboutsPodAnnotation(nadName string, slaveNetworks []string) []*multus.NetworkSelectionElement {
	var annotation []*multus.NetworkSelectionElement

	for _, slaveNetwork := range slaveNetworks {
		slave := pod.StaticAnnotation(slaveNetwork)
		Expect(slave).NotTo(BeNil(), "Failed to build slave network annotation for %s", slaveNetwork)
		annotation = append(annotation, slave)
	}

	annotation = append(annotation, &multus.NetworkSelectionElement{
		Name:             nadName,
		Namespace:        tsparams.TestNamespaceName,
		InterfaceRequest: sriovenv.BondInterfaceName,
	})

	return annotation
}
