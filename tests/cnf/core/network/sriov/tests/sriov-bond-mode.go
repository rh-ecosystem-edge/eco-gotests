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
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/rbac"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/serviceaccount"
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
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
)

const (
	bondNADName = "sriov-bond-nad"

	bondTestServiceAccountName = "sriov-bond-test"
	bondTestPrivilegedCRBName  = "sriov-tests-privileged-scc-bond-test"
	bondTestPodLabelKey        = "eco-gotests/sriov-bond-mode"
	bondTestPodLabelValue      = "true"

	mtu500  = 500
	mtu1280 = 1280
	mtu9000 = 9000

	// SR-IOV policy resourceNames for bond tests (PF1/PF2 × custom/jumbo MTU × IP family).
	bondResourceV4PF1Custom = "sriovbondpf1mtu500"
	bondResourceV4PF1Jumbo  = "sriovbondpf1mtu9000"
	bondResourceV4PF2Custom = "sriovbondpf2mtu500"
	bondResourceV4PF2Jumbo  = "sriovbondpf2mtu9000"
	bondResourceV6PF1Custom = "sriovbondpf1mtu1280v6"
	bondResourceV6PF1Jumbo  = "sriovbondpf1mtu9000v6"
	bondResourceV6PF2Custom = "sriovbondpf2mtu1280v6"
	bondResourceV6PF2Jumbo  = "sriovbondpf2mtu9000v6"

	// Post-failover traffic may need multiple attempts while the bond recovers forwarding.
	bondFailoverTrafficTimeout      = 30 * time.Second
	bondFailoverTrafficPollInterval = 1 * time.Second
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

			By("Ensuring bond test pods can use privileged SCC")

			err = setupBondTestPrivilegedSCC()
			Expect(err).ToNot(HaveOccurred(), "Failed to grant privileged SCC to bond test service account")

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

			const bondMinVFsDualStack = 10

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
					// Expanded layout: IPv4 VFs 0-9, IPv6 VFs 10-19.
					vf4SmallStart, vf4SmallEnd = 0, 4
					vf4LargeStart, vf4LargeEnd = 5, 9
					vf6SmallStart, vf6SmallEnd = 10, 14
					vf6LargeStart, vf6LargeEnd = 15, 19
				} else {
					// Compact layout for 10-VF PFs: IPv4 VFs 0-4, IPv6 VFs 5-9 (split 2+3 per MTU tier).
					vf4SmallStart, vf4SmallEnd = 0, 1
					vf4LargeStart, vf4LargeEnd = 2, 4
					vf6SmallStart, vf6SmallEnd = 5, 6
					vf6LargeStart, vf6LargeEnd = 7, 9
				}
			}

			if supportsIPv4 {
				By("Creating SR-IOV policies and networks for IPv4 bond tests")

				if supportsIPv6 {
					Expect(sriovenv.CreateSriovPolicy(
						"ipv4-policy-pf1-mtu500", bondResourceV4PF1Custom, pf1, mtu500,
						vf4SmallStart, vf4SmallEnd)).To(Succeed(), "Failed to create IPv4 PF1 custom policy")
					Expect(sriovenv.CreateSriovPolicy(
						"ipv4-policy-pf1-mtu9000", bondResourceV4PF1Jumbo, pf1, mtu9000,
						vf4LargeStart, vf4LargeEnd)).To(Succeed(), "Failed to create IPv4 PF1 jumbo policy")
					Expect(sriovenv.CreateSriovPolicy(
						"ipv4-policy-pf2-mtu500", bondResourceV4PF2Custom, pf2, mtu500,
						vf4SmallStart, vf4SmallEnd)).To(Succeed(), "Failed to create IPv4 PF2 custom policy")
					Expect(sriovenv.CreateSriovPolicy(
						"ipv4-policy-pf2-mtu9000", bondResourceV4PF2Jumbo, pf2, mtu9000,
						vf4LargeStart, vf4LargeEnd)).To(Succeed(), "Failed to create IPv4 PF2 jumbo policy")
					Expect(sriovoperator.WaitForSriovAndMCPStable(
						APIClient, tsparams.MCOWaitTimeout, tsparams.DefaultStableDuration,
						NetConfig.CnfMcpLabel, NetConfig.SriovOperatorNamespace)).
						To(Succeed(), "Failed to wait for SR-IOV and MCP stability after IPv4 bond policies")
				} else {
					err = sriovenv.CreateAllSriovPolicies(
						pf1, pf2,
						bondResourceV4PF1Custom, bondResourceV4PF1Jumbo,
						bondResourceV4PF2Custom, bondResourceV4PF2Jumbo,
						"ipv4", mtu500, mtu9000)
					Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV policies for IPv4 bond tests")
				}

				Expect(createBondWhereaboutsNetworks([]bondNetworkConfig{
					{
						name: bondNetworksV4DiffPFCustom[0], resource: bondResourceV4PF1Custom,
						ipRange: tsparams.WhereaboutsIPv4Range, gateway: tsparams.WhereaboutsIPv4Gateway,
					},
					{
						name: bondNetworksV4DiffPFJumbo[0], resource: bondResourceV4PF1Jumbo,
						ipRange: tsparams.WhereaboutsIPv4Range2, gateway: tsparams.WhereaboutsIPv4Gateway2,
					},
					{
						name: bondNetworksV4DiffPFCustom[1], resource: bondResourceV4PF2Custom,
						ipRange: tsparams.WhereaboutsIPv4Range, gateway: tsparams.WhereaboutsIPv4Gateway,
					},
					{
						name: bondNetworksV4DiffPFJumbo[1], resource: bondResourceV4PF2Jumbo,
						ipRange: tsparams.WhereaboutsIPv4Range2, gateway: tsparams.WhereaboutsIPv4Gateway2,
					},
					{
						name: bondNetworksV4SamePFCustom[0], resource: bondResourceV4PF1Custom,
						ipRange: tsparams.WhereaboutsIPv4Range, gateway: tsparams.WhereaboutsIPv4Gateway,
					},
					{
						name: bondNetworksV4SamePFCustom[1], resource: bondResourceV4PF1Custom,
						ipRange: tsparams.WhereaboutsIPv4Range, gateway: tsparams.WhereaboutsIPv4Gateway,
					},
					{
						name: bondNetworksV4SamePFJumbo[0], resource: bondResourceV4PF1Jumbo,
						ipRange: tsparams.WhereaboutsIPv4Range2, gateway: tsparams.WhereaboutsIPv4Gateway2,
					},
					{
						name: bondNetworksV4SamePFJumbo[1], resource: bondResourceV4PF1Jumbo,
						ipRange: tsparams.WhereaboutsIPv4Range2, gateway: tsparams.WhereaboutsIPv4Gateway2,
					},
				})).To(Succeed(), "Failed to create IPv4 bond networks")
			}

			if supportsIPv6 {
				By("Creating SR-IOV policies and networks for IPv6 bond tests")

				if supportsIPv4 {
					Expect(sriovenv.CreateSriovPolicy(
						"ipv6-policy-pf1-mtu1280", bondResourceV6PF1Custom, pf1, mtu1280,
						vf6SmallStart, vf6SmallEnd)).To(Succeed(), "Failed to create IPv6 PF1 custom policy")
					Expect(sriovenv.CreateSriovPolicy(
						"ipv6-policy-pf1-mtu9000", bondResourceV6PF1Jumbo, pf1, mtu9000,
						vf6LargeStart, vf6LargeEnd)).To(Succeed(), "Failed to create IPv6 PF1 jumbo policy")
					Expect(sriovenv.CreateSriovPolicy(
						"ipv6-policy-pf2-mtu1280", bondResourceV6PF2Custom, pf2, mtu1280,
						vf6SmallStart, vf6SmallEnd)).To(Succeed(), "Failed to create IPv6 PF2 custom policy")
					Expect(sriovenv.CreateSriovPolicy(
						"ipv6-policy-pf2-mtu9000", bondResourceV6PF2Jumbo, pf2, mtu9000,
						vf6LargeStart, vf6LargeEnd)).To(Succeed(), "Failed to create IPv6 PF2 jumbo policy")
					Expect(sriovoperator.WaitForSriovAndMCPStable(
						APIClient, tsparams.MCOWaitTimeout, tsparams.DefaultStableDuration,
						NetConfig.CnfMcpLabel, NetConfig.SriovOperatorNamespace)).
						To(Succeed(), "Failed to wait for SR-IOV and MCP stability after IPv6 bond policies")
				} else {
					err = sriovenv.CreateAllSriovPolicies(
						pf1, pf2,
						bondResourceV6PF1Custom, bondResourceV6PF1Jumbo,
						bondResourceV6PF2Custom, bondResourceV6PF2Jumbo,
						"ipv6", mtu1280, mtu9000)
					Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV policies for IPv6 bond tests")
				}

				Expect(createBondWhereaboutsNetworks([]bondNetworkConfig{
					{
						name: bondNetworksV6DiffPFCustom[0], resource: bondResourceV6PF1Custom,
						ipRange: tsparams.WhereaboutsIPv6Range, gateway: tsparams.WhereaboutsIPv6Gateway,
					},
					{
						name: bondNetworksV6DiffPFJumbo[0], resource: bondResourceV6PF1Jumbo,
						ipRange: tsparams.WhereaboutsIPv6Range2, gateway: tsparams.WhereaboutsIPv6Gateway2,
					},
					{
						name: bondNetworksV6DiffPFCustom[1], resource: bondResourceV6PF2Custom,
						ipRange: tsparams.WhereaboutsIPv6Range, gateway: tsparams.WhereaboutsIPv6Gateway,
					},
					{
						name: bondNetworksV6DiffPFJumbo[1], resource: bondResourceV6PF2Jumbo,
						ipRange: tsparams.WhereaboutsIPv6Range2, gateway: tsparams.WhereaboutsIPv6Gateway2,
					},
					{
						name: bondNetworksV6SamePFCustom[0], resource: bondResourceV6PF1Custom,
						ipRange: tsparams.WhereaboutsIPv6Range, gateway: tsparams.WhereaboutsIPv6Gateway,
					},
					{
						name: bondNetworksV6SamePFCustom[1], resource: bondResourceV6PF1Custom,
						ipRange: tsparams.WhereaboutsIPv6Range, gateway: tsparams.WhereaboutsIPv6Gateway,
					},
					{
						name: bondNetworksV6SamePFJumbo[0], resource: bondResourceV6PF1Jumbo,
						ipRange: tsparams.WhereaboutsIPv6Range2, gateway: tsparams.WhereaboutsIPv6Gateway2,
					},
					{
						name: bondNetworksV6SamePFJumbo[1], resource: bondResourceV6PF1Jumbo,
						ipRange: tsparams.WhereaboutsIPv6Range2, gateway: tsparams.WhereaboutsIPv6Gateway2,
					},
				})).To(Succeed(), "Failed to create IPv6 bond networks")
			}
		})

		AfterAll(func() {
			By("Removing bond test privileged SCC binding")

			err = removeBondTestPrivilegedSCC()
			Expect(err).ToNot(HaveOccurred(), "Failed to remove bond test privileged SCC binding")

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

			// Best-effort cleanup: tests may create MTU-specific bond NADs.
			for _, mtu := range []int{0, mtu500, mtu1280, mtu9000} {
				nadName := bondNADName
				if mtu > 0 {
					nadName = fmt.Sprintf("%s-mtu%d", bondNADName, mtu)
				}

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
					bondNetworksV4DiffPFCustom, bondNetworksV4DiffPFJumbo,
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
					bondNetworksV4SamePFCustom, bondNetworksV4SamePFJumbo,
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
					bondNetworksV6DiffPFCustom, bondNetworksV6DiffPFJumbo,
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
					bondNetworksV6SamePFCustom, bondNetworksV6SamePFJumbo,
				)
			})
		})

		Context("Mode: active-active", func() {
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
					bondNetworksV4DiffPFCustom, bondNetworksV4DiffPFJumbo,
				)
			})

			It("balance-rr: DiffNodeSamePF IPv4", reportxml.ID("89053"), func() {
				if !netenv.ClusterSupportsIPv4(clusterIPFamily) {
					Skip("Cluster does not support IPv4 - skipping SR-IOV bond IPv4 tests")
				}

				runBondScenario(
					sriovenv.BondModeBalanceRR,
					mtu500, mtu9000,
					tsparams.ServerIPv4IPAddress, tsparams.ClientIPv4IPAddress,
					tsparams.ServerIPv4IPAddress2, tsparams.ClientIPv4IPAddress2,
					workerNodeList[0].Definition.Name, workerNodeList[1].Definition.Name,
					bondNetworksV4SamePFCustom, bondNetworksV4SamePFJumbo,
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
					bondNetworksV4DiffPFCustom, bondNetworksV4DiffPFJumbo,
				)
			})

			It("balance-xor: DiffNodeSamePF IPv4", reportxml.ID("89055"), func() {
				if !netenv.ClusterSupportsIPv4(clusterIPFamily) {
					Skip("Cluster does not support IPv4 - skipping SR-IOV bond IPv4 tests")
				}

				runBondScenario(
					sriovenv.BondModeBalanceXOR,
					mtu500, mtu9000,
					tsparams.ServerIPv4IPAddress, tsparams.ClientIPv4IPAddress,
					tsparams.ServerIPv4IPAddress2, tsparams.ClientIPv4IPAddress2,
					workerNodeList[0].Definition.Name, workerNodeList[1].Definition.Name,
					bondNetworksV4SamePFCustom, bondNetworksV4SamePFJumbo,
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
					bondNetworksV6DiffPFCustom, bondNetworksV6DiffPFJumbo,
				)
			})

			It("balance-rr: DiffNodeSamePF IPv6", reportxml.ID("89060"), func() {
				if !netenv.ClusterSupportsIPv6(clusterIPFamily) {
					Skip("Cluster does not support IPv6 - skipping SR-IOV bond IPv6 tests")
				}

				runBondScenario(
					sriovenv.BondModeBalanceRR,
					mtu1280, mtu9000,
					tsparams.ServerIPv6IPAddress, tsparams.ClientIPv6IPAddress,
					tsparams.ServerIPv6IPAddress2, tsparams.ClientIPv6IPAddress2,
					workerNodeList[0].Definition.Name, workerNodeList[1].Definition.Name,
					bondNetworksV6SamePFCustom, bondNetworksV6SamePFJumbo,
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
					bondNetworksV6DiffPFCustom, bondNetworksV6DiffPFJumbo,
				)
			})

			It("balance-xor: DiffNodeSamePF IPv6", reportxml.ID("89067"), func() {
				if !netenv.ClusterSupportsIPv6(clusterIPFamily) {
					Skip("Cluster does not support IPv6 - skipping SR-IOV bond IPv6 tests")
				}

				runBondScenario(
					sriovenv.BondModeBalanceXOR,
					mtu1280, mtu9000,
					tsparams.ServerIPv6IPAddress, tsparams.ClientIPv6IPAddress,
					tsparams.ServerIPv6IPAddress2, tsparams.ClientIPv6IPAddress2,
					workerNodeList[0].Definition.Name, workerNodeList[1].Definition.Name,
					bondNetworksV6SamePFCustom, bondNetworksV6SamePFJumbo,
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

					err = sriovenv.CreateSriovNetworkWithWhereaboutsIPAM(
						scaleNetAIPv4, scaleResAIPv4,
						tsparams.WhereaboutsIPv4Range, tsparams.WhereaboutsIPv4Gateway, "", "",
					)
					Expect(err).ToNot(HaveOccurred(), "Failed to create IPv4 scale SR-IOV network A")

					err = sriovenv.CreateSriovNetworkWithWhereaboutsIPAM(
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

					err = sriovenv.CreateSriovNetworkWithWhereaboutsIPAM(
						scaleNetAIPv6, scaleResAIPv6,
						tsparams.WhereaboutsIPv6Range, tsparams.WhereaboutsIPv6Gateway, "", "",
					)
					Expect(err).ToNot(HaveOccurred(), "Failed to create IPv6 scale SR-IOV network A")

					err = sriovenv.CreateSriovNetworkWithWhereaboutsIPAM(
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
	bondNetworksV4DiffPFCustom = []string{"sriov-bond-v4-diffpf-custom-pf1", "sriov-bond-v4-diffpf-custom-pf2"}
	bondNetworksV4DiffPFJumbo  = []string{"sriov-bond-v4-diffpf-jumbo-pf1", "sriov-bond-v4-diffpf-jumbo-pf2"}
	bondNetworksV4SamePFCustom = []string{"sriov-bond-v4-samepf-custom-a", "sriov-bond-v4-samepf-custom-b"}
	bondNetworksV4SamePFJumbo  = []string{"sriov-bond-v4-samepf-jumbo-a", "sriov-bond-v4-samepf-jumbo-b"}

	bondNetworksV6DiffPFCustom = []string{"sriov-bond-v6-diffpf-custom-pf1", "sriov-bond-v6-diffpf-custom-pf2"}
	bondNetworksV6DiffPFJumbo  = []string{"sriov-bond-v6-diffpf-jumbo-pf1", "sriov-bond-v6-diffpf-jumbo-pf2"}
	bondNetworksV6SamePFCustom = []string{"sriov-bond-v6-samepf-custom-a", "sriov-bond-v6-samepf-custom-b"}
	bondNetworksV6SamePFJumbo  = []string{"sriov-bond-v6-samepf-jumbo-a", "sriov-bond-v6-samepf-jumbo-b"}
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
	nadSmall := fmt.Sprintf("%s-mtu%d", bondNADName, mtuSmall)
	nadLarge := fmt.Sprintf("%s-mtu%d", bondNADName, mtuLarge)

	By("Creating bond NADs for both MTUs")

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
	verifyInitialBondTraffic(bondMode, clientSmall, serverIPSmall, mtuSmall, "small MTU")
	verifyInitialBondTraffic(bondMode, clientLarge, serverIPLarge, mtuLarge, "large MTU")

	By("Triggering link failure and verifying traffic still works for both MTUs")
	Expect(triggerBondLinkFailureAndVerify(clientSmall, bondMode, serverIPSmall, mtuSmall)).
		To(Succeed(), "Bond link failure verification failed (small MTU)")
	Expect(triggerBondLinkFailureAndVerify(clientLarge, bondMode, serverIPLarge, mtuLarge)).
		To(Succeed(), "Bond link failure verification failed (large MTU)")
}

func verifyInitialBondTraffic(bondMode string, clientPod *pod.Builder, serverIP string, mtu int, mtuDesc string) {
	runTraffic := func() error {
		return sriovenv.RunTrafficTest(clientPod, serverIP, mtu, sriovenv.BondInterfaceName)
	}

	if bondMode == sriovenv.BondModeActiveBackup {
		Expect(runTraffic()).To(Succeed(), "Traffic tests failed on bond interface (%s)", mtuDesc)

		return
	}

	Eventually(runTraffic, bondFailoverTrafficTimeout, bondFailoverTrafficPollInterval).
		Should(Succeed(), "Traffic tests failed on bond interface (%s)", mtuDesc)
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
	serverPodBuilder.Definition.Spec.ServiceAccountName = bondTestServiceAccountName

	serverPod, err := serverPodBuilder.
		DefineOnNode(serverNode).
		RedefineDefaultContainer(*serverContainer).
		WithPrivilegedFlag().
		WithLabel(bondTestPodLabelKey, bondTestPodLabelValue).
		WithSecondaryNetwork(annotationServer).
		CreateAndWaitUntilRunning(netparam.DefaultTimeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to create server pod")

	Expect(sriovenv.WaitForServerReady(serverPod, tsparams.WaitTimeout)).
		To(Succeed(), "Server pod testcmd listeners not ready")

	clientPodBuilder := pod.NewBuilder(APIClient, clientName, tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer)
	clientPodBuilder.Definition.Spec.ServiceAccountName = bondTestServiceAccountName

	clientPod, err := clientPodBuilder.
		DefineOnNode(clientNode).
		WithPrivilegedFlag().
		WithLabel(bondTestPodLabelKey, bondTestPodLabelValue).
		WithSecondaryNetwork(annotationClient).
		CreateAndWaitUntilRunning(netparam.DefaultTimeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to create client pod")

	return serverPod, clientPod
}

// bondNetworkConfig describes a single SR-IOV network for bond slave creation.
type bondNetworkConfig struct {
	name,
	resource,
	ipRange,
	gateway string
}

// createBondWhereaboutsNetworks creates multiple SR-IOV networks with Whereabouts IPAM in one call.
func createBondWhereaboutsNetworks(configs []bondNetworkConfig) error {
	for _, cfg := range configs {
		if err := sriovenv.CreateSriovNetworkWithWhereaboutsIPAM(
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

		if err := runBondTrafficAfterFailoverWithRetry(clientPod, serverIP, mtu, bondMode); err != nil {
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

	if err := runBondTrafficAfterFailoverWithRetry(clientPod, serverIP, mtu, bondMode); err != nil {
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

	if err := runBondTrafficAfterFailoverWithRetry(clientPod, serverIP, mtu, bondMode); err != nil {
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

func runBondTrafficAfterFailoverWithRetry(clientPod *pod.Builder, serverIP string, mtu int, bondMode string) error {
	deadline := time.Now().Add(bondFailoverTrafficTimeout)

	var lastErr error

	for {
		lastErr = runBondConnectivityAfterFailover(clientPod, serverIP, mtu, bondMode)
		if lastErr == nil {
			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("bond traffic failed after failover within %v: %w", bondFailoverTrafficTimeout, lastErr)
		}

		time.Sleep(bondFailoverTrafficPollInterval)
	}
}

func verifyBondTrafficAfterSwitchPortDown(
	clientPod *pod.Builder,
	serverIP string,
	mtu int,
	bondMode string,
	iface string,
	enable func(string) error,
) error {
	if err := sriovenv.WaitForBondDegradedState(clientPod, sriovenv.BondInterfaceName); err != nil {
		if enErr := enable(iface); enErr != nil {
			return fmt.Errorf(
				"bond did not stabilize after disabling switch interface %s: %w; re-enable also failed: %w",
				iface, err, enErr)
		}

		return fmt.Errorf("bond did not stabilize after disabling switch interface %s: %w", iface, err)
	}

	trafficErr := runBondTrafficAfterFailoverWithRetry(clientPod, serverIP, mtu, bondMode)

	if err := enable(iface); err != nil {
		if trafficErr != nil {
			return fmt.Errorf("failed to re-enable switch interface %s: %w (traffic test error: %w)", iface, err, trafficErr)
		}

		return fmt.Errorf("failed to re-enable switch interface %s: %w", iface, err)
	}

	if err := sriovenv.WaitForBondSlavesMIIUp(clientPod, sriovenv.BondInterfaceName); err != nil {
		if trafficErr != nil {
			return fmt.Errorf(
				"bond did not recover after re-enabling switch interface %s: %w (traffic test error: %w)",
				iface, err, trafficErr)
		}

		return fmt.Errorf("bond did not recover after re-enabling switch interface %s: %w", iface, err)
	}

	if trafficErr != nil {
		return fmt.Errorf("traffic failed with switch interface %s disabled: %w", iface, trafficErr)
	}

	return nil
}

func toggleSwitchPortsAndVerifyTraffic(clientPod *pod.Builder, serverIP string, mtu int, bondMode string) error {
	credentials, err := sriovenv.NewSwitchCredentials()
	if err != nil {
		return err
	}

	switchIfaces, err := NetConfig.GetSwitchInterfaces()
	if err != nil {
		return err
	}

	if len(switchIfaces) < 2 {
		return fmt.Errorf("need at least 2 switch interfaces, got %d", len(switchIfaces))
	}

	jnpr, err := cmd.NewSession(credentials.SwitchIP, credentials.User, credentials.Password)
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

	for _, iface := range switchIfaces[:2] {
		if err := disable(iface); err != nil {
			if enErr := enable(iface); enErr != nil {
				return fmt.Errorf("failed to disable switch interface %s: %w; re-enable also failed: %w", iface, err, enErr)
			}

			return fmt.Errorf("failed to disable switch interface %s: %w", iface, err)
		}

		if err := verifyBondTrafficAfterSwitchPortDown(clientPod, serverIP, mtu, bondMode, iface, enable); err != nil {
			return err
		}
	}

	return nil
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

func setupBondTestPrivilegedSCC() error {
	// Bond CNI needs permissions to enslave links to bond0; in OCP this is controlled by SCC.
	if _, err := serviceaccount.NewBuilder(
		APIClient, bondTestServiceAccountName, tsparams.TestNamespaceName,
	).Create(); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}

	_, err := rbac.NewClusterRoleBindingBuilder(
		APIClient,
		bondTestPrivilegedCRBName,
		"system:openshift:scc:privileged",
		rbacv1.Subject{
			Kind:      "ServiceAccount",
			Name:      bondTestServiceAccountName,
			Namespace: tsparams.TestNamespaceName,
		},
	).Create()
	if err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}

	return nil
}

func removeBondTestPrivilegedSCC() error {
	if err := rbac.NewClusterRoleBindingBuilder(
		APIClient,
		bondTestPrivilegedCRBName,
		"system:openshift:scc:privileged",
		rbacv1.Subject{
			Kind:      "ServiceAccount",
			Name:      bondTestServiceAccountName,
			Namespace: tsparams.TestNamespaceName,
		},
	).Delete(); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	if err := serviceaccount.NewBuilder(
		APIClient, bondTestServiceAccountName, tsparams.TestNamespaceName,
	).Delete(); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	return nil
}
