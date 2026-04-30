package tests

import (
	"fmt"
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
)

const (
	bondInterfaceName = "bond0"
	bondSlave1IfName  = "net1"
	bondSlave2IfName  = "net2"

	bondNADName = "sriov-bond-nad"

	// Bond failover updates active_slave asynchronously; poll briefly to avoid flakes on slow nodes.
	bondActiveSlavePollInterval  = 100 * time.Millisecond
	bondActiveSlaveChangeTimeout = 30 * time.Second
)

var _ = Describe("SR-IOV Bond CNI IPv4", Ordered, Label(tsparams.LabelSuite, "bond-mode"), ContinueOnFailure, func() {
	var (
		workerNodeList []*nodes.Builder
		pf1            string
		pf2            string
		err            error
	)

	BeforeAll(func() {
		By("Checking cluster IP family")

		clusterIPFamily, err := netenv.GetClusterIPFamily(APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to detect cluster IP family")

		if !netenv.ClusterSupportsIPv4(clusterIPFamily) {
			Skip("Cluster does not support IPv4 - skipping SR-IOV bond IPv4 tests")
		}

		By("Ensuring sriov-tests namespace can run privileged pods")

		err = ensureSriovTestsNamespaceHasPrivilegedSCC()
		Expect(err).ToNot(HaveOccurred(), "Failed to grant privileged SCC to sriov-tests default serviceaccount")

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

		By("Creating SR-IOV policies and networks for IPv4 bond tests")

		err = sriovenv.CreateAllSriovPolicies(
			pf1, pf2,
			tsparams.BondResourceV4PF1Custom, tsparams.BondResourceV4PF1Jumbo,
			tsparams.BondResourceV4PF2Custom, tsparams.BondResourceV4PF2Jumbo,
			"ipv4", tsparams.MTU500, tsparams.MTU9000)
		Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV policies for IPv4 bond tests")

		err = createBondSriovNetworksIPv4(
			tsparams.BondResourceV4PF1Custom, tsparams.BondResourceV4PF1Jumbo,
			tsparams.BondResourceV4PF2Custom, tsparams.BondResourceV4PF2Jumbo)
		Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV diff-PF networks for IPv4 bond tests")

		err = createBondSamePFSriovNetworksIPv4(tsparams.BondResourceV4PF1Custom, tsparams.BondResourceV4PF1Jumbo)
		Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV same-PF networks for IPv4 bond tests")
	})

	AfterAll(func() {
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
		By("Cleaning pods and NAD after test")

		err = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
			netparam.DefaultTimeout, pod.GetGVR())
		Expect(err).ToNot(HaveOccurred(), "Failed to delete test pods")

		// Best-effort cleanup: tests may create MTU-specific bond NADs.
		_ = deleteBondNADIfExists(bondNADName)
		_ = deleteBondNADIfExists(fmt.Sprintf("%s-mtu%d", bondNADName, tsparams.MTU500))
		_ = deleteBondNADIfExists(fmt.Sprintf("%s-mtu%d", bondNADName, tsparams.MTU9000))
		_ = deleteBondNADIfExists(fmt.Sprintf("%s-mtu%d", bondNADName, tsparams.MTU1280))
	})

	Context("Mode: active-backup", func() {
		It("DiffNodeDiffPF (custom+jumbo MTU)", reportxml.ID("TBD-BOND-V4-ACTIVEBACKUP-DIFFNODE-DIFFPF"), func() {
			runBondScenario(
				bondModeActiveBackup,
				tsparams.MTU500, tsparams.MTU9000,
				tsparams.ServerIPv4IPAddress, tsparams.ClientIPv4IPAddress,
				tsparams.ServerIPv4IPAddress2, tsparams.ClientIPv4IPAddress2,
				workerNodeList[0].Definition.Name, workerNodeList[1].Definition.Name,
				bondNetworksV4DiffPFCustom, bondNetworksV4DiffPFJumbo,
			)
		})

		It("DiffNodeSamePF (custom+jumbo MTU)", reportxml.ID("TBD-BOND-V4-ACTIVEBACKUP-DIFFNODE-SAMEPF"), func() {
			runBondScenario(
				bondModeActiveBackup,
				tsparams.MTU500, tsparams.MTU9000,
				tsparams.ServerIPv4IPAddress, tsparams.ClientIPv4IPAddress,
				tsparams.ServerIPv4IPAddress2, tsparams.ClientIPv4IPAddress2,
				workerNodeList[0].Definition.Name, workerNodeList[1].Definition.Name,
				bondNetworksV4SamePFCustom, bondNetworksV4SamePFJumbo,
			)
		})
	})

	Context("Active-active bond modes", func() {
		It("balance-rr: DiffNodeDiffPF (custom+jumbo MTU)", reportxml.ID("TBD-BOND-V4-BALANCERR-DIFFNODE-DIFFPF"), func() {
			runBondScenario(
				"balance-rr",
				tsparams.MTU500, tsparams.MTU9000,
				tsparams.ServerIPv4IPAddress, tsparams.ClientIPv4IPAddress,
				tsparams.ServerIPv4IPAddress2, tsparams.ClientIPv4IPAddress2,
				workerNodeList[0].Definition.Name, workerNodeList[1].Definition.Name,
				bondNetworksV4DiffPFCustom, bondNetworksV4DiffPFJumbo,
			)
		})

		It("balance-rr: DiffNodeSamePF (custom+jumbo MTU)", reportxml.ID("TBD-BOND-V4-BALANCERR-DIFFNODE-SAMEPF"), func() {
			runBondScenario(
				"balance-rr",
				tsparams.MTU500, tsparams.MTU9000,
				tsparams.ServerIPv4IPAddress, tsparams.ClientIPv4IPAddress,
				tsparams.ServerIPv4IPAddress2, tsparams.ClientIPv4IPAddress2,
				workerNodeList[0].Definition.Name, workerNodeList[1].Definition.Name,
				bondNetworksV4SamePFCustom, bondNetworksV4SamePFJumbo,
			)
		})

		It("balance-xor: DiffNodeDiffPF (custom+jumbo MTU)", reportxml.ID("TBD-BOND-V4-BALANCEXOR-DIFFNODE-DIFFPF"), func() {
			runBondScenario(
				"balance-xor",
				tsparams.MTU500, tsparams.MTU9000,
				tsparams.ServerIPv4IPAddress, tsparams.ClientIPv4IPAddress,
				tsparams.ServerIPv4IPAddress2, tsparams.ClientIPv4IPAddress2,
				workerNodeList[0].Definition.Name, workerNodeList[1].Definition.Name,
				bondNetworksV4DiffPFCustom, bondNetworksV4DiffPFJumbo,
			)
		})

		It("balance-xor: DiffNodeSamePF (custom+jumbo MTU)", reportxml.ID("TBD-BOND-V4-BALANCEXOR-DIFFNODE-SAMEPF"), func() {
			runBondScenario(
				"balance-xor",
				tsparams.MTU500, tsparams.MTU9000,
				tsparams.ServerIPv4IPAddress, tsparams.ClientIPv4IPAddress,
				tsparams.ServerIPv4IPAddress2, tsparams.ClientIPv4IPAddress2,
				workerNodeList[0].Definition.Name, workerNodeList[1].Definition.Name,
				bondNetworksV4SamePFCustom, bondNetworksV4SamePFJumbo,
			)
		})
	})

	Context("Scale: Bond with 16 VFs", func() {
		const (
			scaleBondMode   = bondModeActiveBackup
			scaleBondNAD    = "sriov-bond-scale-nad"
			scaleNetA       = "sriov-bond-scale-a"
			scaleNetB       = "sriov-bond-scale-b"
			scaleResA       = "sriovbondscalea"
			scaleResB       = "sriovbondscaleb"
			scaleTotalVFs   = 16
			scaleSlaveCount = 16
		)

		BeforeAll(func() {
			By("Checking that requested interfaces support at least 16 total VFs")

			nodeName := workerNodeList[0].Definition.Name

			pf1Total, err := sriov.NewNetworkNodeStateBuilder(APIClient, nodeName, NetConfig.SriovOperatorNamespace).
				GetTotalVFs(pf1)
			Expect(err).ToNot(HaveOccurred(), "Failed to get total VFs for PF1")

			pf2Total, err := sriov.NewNetworkNodeStateBuilder(APIClient, nodeName, NetConfig.SriovOperatorNamespace).
				GetTotalVFs(pf2)
			Expect(err).ToNot(HaveOccurred(), "Failed to get total VFs for PF2")

			if pf1Total < scaleTotalVFs || pf2Total < scaleTotalVFs {
				Skip(fmt.Sprintf("Scale test requires >=%d total VFs on each PF; got pf1=%d, pf2=%d",
					scaleTotalVFs, pf1Total, pf2Total))
			}

			By("Creating SR-IOV policies for 16 VFs scale test")

			// Create scale policies with VF range 0-15.
			_, err = sriov.NewPolicyBuilder(
				APIClient,
				"bond-scale-policy-pf1",
				NetConfig.SriovOperatorNamespace,
				scaleResA,
				scaleTotalVFs,
				[]string{pf1},
				NetConfig.WorkerLabelMap).
				WithMTU(tsparams.MTU500).
				WithVFRange(0, scaleTotalVFs-1).
				Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create scale policy for PF1")

			_, err = sriov.NewPolicyBuilder(
				APIClient,
				"bond-scale-policy-pf2",
				NetConfig.SriovOperatorNamespace,
				scaleResB,
				scaleTotalVFs,
				[]string{pf2},
				NetConfig.WorkerLabelMap).
				WithMTU(tsparams.MTU500).
				WithVFRange(0, scaleTotalVFs-1).
				Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create scale policy for PF2")

			err = sriovoperator.WaitForSriovAndMCPStable(
				APIClient, tsparams.MCOWaitTimeout, tsparams.DefaultStableDuration,
				NetConfig.WorkerLabelEnvVar, NetConfig.SriovOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed waiting for SR-IOV and MCP stability for scale policies")

			By("Creating SR-IOV networks for scale test")

			err = sriovenv.CreateSriovNetworkWithWhereaboutsIPAM(
				scaleNetA, scaleResA, tsparams.WhereaboutsIPv4Range, tsparams.WhereaboutsIPv4Gateway, "", "",
			)
			Expect(err).ToNot(HaveOccurred(), "Failed to create scale SR-IOV network A")

			err = sriovenv.CreateSriovNetworkWithWhereaboutsIPAM(
				scaleNetB, scaleResB, tsparams.WhereaboutsIPv4Range, tsparams.WhereaboutsIPv4Gateway, "", "",
			)
			Expect(err).ToNot(HaveOccurred(), "Failed to create scale SR-IOV network B")
		})

		AfterAll(func() {
			_ = deleteBondNADIfExists(scaleBondNAD)
		})

		AfterEach(func() {
			err = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
				netparam.DefaultTimeout, pod.GetGVR())
			Expect(err).ToNot(HaveOccurred(), "Failed to delete test pods")

			_ = deleteBondNADIfExists(scaleBondNAD)
		})

		It("Verify bond with 16 VFs works with ICMP traffic", reportxml.ID("TBD-BOND-V4-SCALE-16VFS"), func() {
			By("Creating bond NAD with 16 slave links")

			_, err := sriovenv.CreateBondNAD(scaleBondNAD, scaleBondMode, tsparams.MTU500, scaleSlaveCount, "static", nil)
			Expect(err).ToNot(HaveOccurred(), "Failed to create scale bond NAD")

			By("Creating slave network list (8 from each SR-IOV network)")

			var slaveNetworks []string

			for i := 0; i < scaleSlaveCount/2; i++ {
				slaveNetworks = append(slaveNetworks, scaleNetA)
			}

			for i := 0; i < scaleSlaveCount/2; i++ {
				slaveNetworks = append(slaveNetworks, scaleNetB)
			}

			serverNode := workerNodeList[0].Definition.Name
			clientNode := workerNodeList[1].Definition.Name

			serverIP := tsparams.ServerIPv4IPAddress
			clientIP := tsparams.ClientIPv4IPAddress

			serverPod, clientPod := createBondedPodsPair(
				scaleBondNAD,
				"bond-scale-server", "bond-scale-client",
				serverNode, clientNode,
				slaveNetworks, serverIP, clientIP, tsparams.MTU500,
			)

			By("Verifying bond interface is up, has correct mode and slave count")
			Expect(verifyBondInterfaceState(clientPod, scaleBondMode, scaleSlaveCount)).To(Succeed(),
				"Bond interface validation failed")

			By("Running ICMP connectivity over the bond")

			serverIPNoPrefix := ipaddr.RemovePrefix(serverIP)
			Expect(cmd.ICMPConnectivityCheck(clientPod, []string{serverIPNoPrefix + "/32"}, bondInterfaceName)).
				To(Succeed(), "ICMP connectivity over bond failed")

			_, err = serverPod.Delete()
			Expect(err).ToNot(HaveOccurred(), "Failed to delete server pod")

			_, err = clientPod.Delete()
			Expect(err).ToNot(HaveOccurred(), "Failed to delete client pod")
		})
	})
})

var _ = Describe("SR-IOV Bond CNI IPv6", Ordered, Label(tsparams.LabelSuite, "bond-mode"), ContinueOnFailure, func() {
	var (
		workerNodeList []*nodes.Builder
		pf1            string
		pf2            string
		err            error
	)

	BeforeAll(func() {
		By("Checking cluster IP family")

		clusterIPFamily, err := netenv.GetClusterIPFamily(APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to detect cluster IP family")

		if !netenv.ClusterSupportsIPv6(clusterIPFamily) {
			Skip("Cluster does not support IPv6 - skipping SR-IOV bond IPv6 tests")
		}

		By("Ensuring sriov-tests namespace can run privileged pods")

		err = ensureSriovTestsNamespaceHasPrivilegedSCC()
		Expect(err).ToNot(HaveOccurred(), "Failed to grant privileged SCC to sriov-tests default serviceaccount")

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

		By("Creating SR-IOV policies and networks for IPv6 bond tests")

		err = sriovenv.CreateAllSriovPolicies(
			pf1, pf2,
			tsparams.BondResourceV6PF1Custom, tsparams.BondResourceV6PF1Jumbo,
			tsparams.BondResourceV6PF2Custom, tsparams.BondResourceV6PF2Jumbo,
			"ipv6", tsparams.MTU1280, tsparams.MTU9000)
		Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV policies for IPv6 bond tests")

		err = createBondSriovNetworksIPv6(
			tsparams.BondResourceV6PF1Custom, tsparams.BondResourceV6PF1Jumbo,
			tsparams.BondResourceV6PF2Custom, tsparams.BondResourceV6PF2Jumbo)
		Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV diff-PF networks for IPv6 bond tests")

		err = createBondSamePFSriovNetworksIPv6(tsparams.BondResourceV6PF1Custom, tsparams.BondResourceV6PF1Jumbo)
		Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV same-PF networks for IPv6 bond tests")
	})

	AfterAll(func() {
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
		By("Cleaning pods and NAD after test")

		err = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName).CleanObjects(
			netparam.DefaultTimeout, pod.GetGVR())
		Expect(err).ToNot(HaveOccurred(), "Failed to delete test pods")

		_ = deleteBondNADIfExists(bondNADName)
	})

	Context("Mode: active-backup", func() {
		It("DiffNodeDiffPF (custom+jumbo MTU)", reportxml.ID("TBD-BOND-V6-ACTIVEBACKUP-DIFFNODE-DIFFPF"), func() {
			runBondScenario(
				bondModeActiveBackup,
				tsparams.MTU1280, tsparams.MTU9000,
				tsparams.ServerIPv6IPAddress, tsparams.ClientIPv6IPAddress,
				tsparams.ServerIPv6IPAddress2, tsparams.ClientIPv6IPAddress2,
				workerNodeList[0].Definition.Name, workerNodeList[1].Definition.Name,
				bondNetworksV6DiffPFCustom, bondNetworksV6DiffPFJumbo,
			)
		})

		It("DiffNodeSamePF (custom+jumbo MTU)", reportxml.ID("TBD-BOND-V6-ACTIVEBACKUP-DIFFNODE-SAMEPF"), func() {
			runBondScenario(
				bondModeActiveBackup,
				tsparams.MTU1280, tsparams.MTU9000,
				tsparams.ServerIPv6IPAddress, tsparams.ClientIPv6IPAddress,
				tsparams.ServerIPv6IPAddress2, tsparams.ClientIPv6IPAddress2,
				workerNodeList[0].Definition.Name, workerNodeList[1].Definition.Name,
				bondNetworksV6SamePFCustom, bondNetworksV6SamePFJumbo,
			)
		})
	})

	Context("Active-active bond modes", func() {
		It("balance-rr: DiffNodeDiffPF (custom+jumbo MTU)", reportxml.ID("TBD-BOND-V6-BALANCERR-DIFFNODE-DIFFPF"), func() {
			runBondScenario(
				"balance-rr",
				tsparams.MTU1280, tsparams.MTU9000,
				tsparams.ServerIPv6IPAddress, tsparams.ClientIPv6IPAddress,
				tsparams.ServerIPv6IPAddress2, tsparams.ClientIPv6IPAddress2,
				workerNodeList[0].Definition.Name, workerNodeList[1].Definition.Name,
				bondNetworksV6DiffPFCustom, bondNetworksV6DiffPFJumbo,
			)
		})

		It("balance-rr: DiffNodeSamePF (custom+jumbo MTU)", reportxml.ID("TBD-BOND-V6-BALANCERR-DIFFNODE-SAMEPF"), func() {
			runBondScenario(
				"balance-rr",
				tsparams.MTU1280, tsparams.MTU9000,
				tsparams.ServerIPv6IPAddress, tsparams.ClientIPv6IPAddress,
				tsparams.ServerIPv6IPAddress2, tsparams.ClientIPv6IPAddress2,
				workerNodeList[0].Definition.Name, workerNodeList[1].Definition.Name,
				bondNetworksV6SamePFCustom, bondNetworksV6SamePFJumbo,
			)
		})

		It("balance-xor: DiffNodeDiffPF (custom+jumbo MTU)", reportxml.ID("TBD-BOND-V6-BALANCEXOR-DIFFNODE-DIFFPF"), func() {
			runBondScenario(
				"balance-xor",
				tsparams.MTU1280, tsparams.MTU9000,
				tsparams.ServerIPv6IPAddress, tsparams.ClientIPv6IPAddress,
				tsparams.ServerIPv6IPAddress2, tsparams.ClientIPv6IPAddress2,
				workerNodeList[0].Definition.Name, workerNodeList[1].Definition.Name,
				bondNetworksV6DiffPFCustom, bondNetworksV6DiffPFJumbo,
			)
		})

		It("balance-xor: DiffNodeSamePF (custom+jumbo MTU)", reportxml.ID("TBD-BOND-V6-BALANCEXOR-DIFFNODE-SAMEPF"), func() {
			runBondScenario(
				"balance-xor",
				tsparams.MTU1280, tsparams.MTU9000,
				tsparams.ServerIPv6IPAddress, tsparams.ClientIPv6IPAddress,
				tsparams.ServerIPv6IPAddress2, tsparams.ClientIPv6IPAddress2,
				workerNodeList[0].Definition.Name, workerNodeList[1].Definition.Name,
				bondNetworksV6SamePFCustom, bondNetworksV6SamePFJumbo,
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

func createBondSriovNetworksIPv4(pf1SmallRes, pf1LargeRes, pf2SmallRes, pf2LargeRes string) error {
	if err := sriovenv.CreateSriovNetworkWithWhereaboutsIPAM(
		bondNetworksV4DiffPFCustom[0], pf1SmallRes,
		tsparams.WhereaboutsIPv4Range, tsparams.WhereaboutsIPv4Gateway, "", "",
	); err != nil {
		return err
	}

	if err := sriovenv.CreateSriovNetworkWithWhereaboutsIPAM(
		bondNetworksV4DiffPFJumbo[0], pf1LargeRes,
		tsparams.WhereaboutsIPv4Range2, tsparams.WhereaboutsIPv4Gateway2, "", "",
	); err != nil {
		return err
	}

	if err := sriovenv.CreateSriovNetworkWithWhereaboutsIPAM(
		bondNetworksV4DiffPFCustom[1], pf2SmallRes,
		tsparams.WhereaboutsIPv4Range, tsparams.WhereaboutsIPv4Gateway, "", "",
	); err != nil {
		return err
	}

	return sriovenv.CreateSriovNetworkWithWhereaboutsIPAM(
		bondNetworksV4DiffPFJumbo[1], pf2LargeRes,
		tsparams.WhereaboutsIPv4Range2, tsparams.WhereaboutsIPv4Gateway2, "", "",
	)
}

func createBondSriovNetworksIPv6(pf1SmallRes, pf1LargeRes, pf2SmallRes, pf2LargeRes string) error {
	if err := sriovenv.CreateSriovNetworkWithWhereaboutsIPAM(
		bondNetworksV6DiffPFCustom[0], pf1SmallRes,
		tsparams.WhereaboutsIPv6Range, tsparams.WhereaboutsIPv6Gateway, "", "",
	); err != nil {
		return err
	}

	if err := sriovenv.CreateSriovNetworkWithWhereaboutsIPAM(
		bondNetworksV6DiffPFJumbo[0], pf1LargeRes,
		tsparams.WhereaboutsIPv6Range2, tsparams.WhereaboutsIPv6Gateway2, "", "",
	); err != nil {
		return err
	}

	if err := sriovenv.CreateSriovNetworkWithWhereaboutsIPAM(
		bondNetworksV6DiffPFCustom[1], pf2SmallRes,
		tsparams.WhereaboutsIPv6Range, tsparams.WhereaboutsIPv6Gateway, "", "",
	); err != nil {
		return err
	}

	return sriovenv.CreateSriovNetworkWithWhereaboutsIPAM(
		bondNetworksV6DiffPFJumbo[1], pf2LargeRes,
		tsparams.WhereaboutsIPv6Range2, tsparams.WhereaboutsIPv6Gateway2, "", "",
	)
}

func createBondSamePFSriovNetworksIPv4(pf1CustomRes, pf1JumboRes string) error {
	if err := createTwoSriovNetworksWhereaboutsIPv4(
		bondNetworksV4SamePFCustom[0], bondNetworksV4SamePFCustom[1],
		pf1CustomRes, tsparams.WhereaboutsIPv4Range, tsparams.WhereaboutsIPv4Gateway); err != nil {
		return err
	}

	return createTwoSriovNetworksWhereaboutsIPv4(
		bondNetworksV4SamePFJumbo[0], bondNetworksV4SamePFJumbo[1],
		pf1JumboRes, tsparams.WhereaboutsIPv4Range2, tsparams.WhereaboutsIPv4Gateway2)
}

func createBondSamePFSriovNetworksIPv6(pf1CustomRes, pf1JumboRes string) error {
	if err := createTwoSriovNetworksWhereaboutsIPv6(
		bondNetworksV6SamePFCustom[0], bondNetworksV6SamePFCustom[1],
		pf1CustomRes, tsparams.WhereaboutsIPv6Range, tsparams.WhereaboutsIPv6Gateway); err != nil {
		return err
	}

	return createTwoSriovNetworksWhereaboutsIPv6(
		bondNetworksV6SamePFJumbo[0], bondNetworksV6SamePFJumbo[1],
		pf1JumboRes, tsparams.WhereaboutsIPv6Range2, tsparams.WhereaboutsIPv6Gateway2)
}

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

	_, err := sriovenv.CreateBondNAD(nadSmall, bondMode, mtuSmall, 2, "static", nil)
	Expect(err).ToNot(HaveOccurred(), "Failed to create small MTU bond NAD")

	defer func() { _ = deleteBondNADIfExists(nadSmall) }()

	_, err = sriovenv.CreateBondNAD(nadLarge, bondMode, mtuLarge, 2, "static", nil)
	Expect(err).ToNot(HaveOccurred(), "Failed to create large MTU bond NAD")

	defer func() { _ = deleteBondNADIfExists(nadLarge) }()

	modeSuffix := strings.ReplaceAll(bondMode, "balance-", "")
	serverSmallName := fmt.Sprintf("bond-server-%s-mtu%d", modeSuffix, mtuSmall)
	clientSmallName := fmt.Sprintf("bond-client-%s-mtu%d", modeSuffix, mtuSmall)
	serverLargeName := fmt.Sprintf("bond-server-%s-mtu%d", modeSuffix, mtuLarge)
	clientLargeName := fmt.Sprintf("bond-client-%s-mtu%d", modeSuffix, mtuLarge)

	By("Creating server and client pods for both MTUs (4 pods total)")

	serverSmall, clientSmall := createBondedPodsPair(
		nadSmall,
		serverSmallName, clientSmallName,
		serverNode, clientNode,
		slaveNetworksSmall, serverIPSmall, clientIPSmall, mtuSmall,
	)
	serverLarge, clientLarge := createBondedPodsPair(
		nadLarge,
		serverLargeName, clientLargeName,
		serverNode, clientNode,
		slaveNetworksLarge, serverIPLarge, clientIPLarge, mtuLarge,
	)

	By("Verifying traffic on bond interface for both MTUs")
	Expect(runTrafficTestOnInterface(clientSmall, serverIPSmall, mtuSmall, bondInterfaceName)).
		To(Succeed(), "Traffic tests failed on bond interface (small MTU)")
	Expect(runTrafficTestOnInterface(clientLarge, serverIPLarge, mtuLarge, bondInterfaceName)).
		To(Succeed(), "Traffic tests failed on bond interface (large MTU)")

	By("Triggering link failure and verifying traffic still works for both MTUs")
	Expect(triggerBondLinkFailureAndVerify(clientSmall, bondMode, serverIPSmall, mtuSmall)).
		To(Succeed(), "Bond link failure verification failed (small MTU)")
	Expect(triggerBondLinkFailureAndVerify(clientLarge, bondMode, serverIPLarge, mtuLarge)).
		To(Succeed(), "Bond link failure verification failed (large MTU)")

	By("Deleting pods for both MTUs")

	_, err = serverSmall.Delete()
	Expect(err).ToNot(HaveOccurred(), "Failed to delete small MTU server pod")

	_, err = clientSmall.Delete()
	Expect(err).ToNot(HaveOccurred(), "Failed to delete small MTU client pod")

	_, err = serverLarge.Delete()
	Expect(err).ToNot(HaveOccurred(), "Failed to delete large MTU server pod")

	_, err = clientLarge.Delete()
	Expect(err).ToNot(HaveOccurred(), "Failed to delete large MTU client pod")
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
		bondInterfaceName,
		slaveNetworks,
		[]string{serverIPWithCIDR},
	)
	Expect(annotationServer).NotTo(BeNil(), "Failed to create bond annotation for server pod")

	annotationClient := pod.StaticIPBondAnnotationWithInterface(
		nadName,
		bondInterfaceName,
		slaveNetworks,
		[]string{clientIPWithCIDR},
	)
	Expect(annotationClient).NotTo(BeNil(), "Failed to create bond annotation for client pod")

	serverBindIP := ipaddr.RemovePrefix(serverIPWithCIDR)
	serverCmd := sriovenv.BuildServerCommand(serverBindIP, bondInterfaceName, mtu)

	serverContainer, err := pod.NewContainerBuilder("server", NetConfig.CnfNetTestContainer, serverCmd).GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Failed to build server container config")

	serverPod, err := pod.NewBuilder(APIClient, serverName, tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
		DefineOnNode(serverNode).
		RedefineDefaultContainer(*serverContainer).
		WithPrivilegedFlag().
		WithSecondaryNetwork(annotationServer).
		CreateAndWaitUntilRunning(netparam.DefaultTimeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to create server pod")

	Expect(sriovenv.WaitForServerReady(serverPod, tsparams.WaitTimeout)).
		To(Succeed(), "Server pod testcmd listeners not ready")

	clientPod, err := pod.NewBuilder(APIClient, clientName, tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
		DefineOnNode(clientNode).
		WithPrivilegedFlag().
		WithSecondaryNetwork(annotationClient).
		CreateAndWaitUntilRunning(netparam.DefaultTimeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to create client pod")

	return serverPod, clientPod
}

//nolint:unparam // interfaceName is always bond0 in this suite (kept for reuse/readability).
func runTrafficTestOnInterface(clientPod *pod.Builder, serverIP string, mtu int, interfaceName string) error {
	serverIPAddress := ipaddr.RemovePrefix(serverIP)

	packetSize := mtu - 100

	serverIPWithPrefix := serverIPAddress + "/32"
	if strings.Contains(serverIPAddress, ":") {
		serverIPWithPrefix = serverIPAddress + "/128"
	}

	var failed []string

	if err := cmd.ICMPConnectivityCheck(clientPod, []string{serverIPWithPrefix}, interfaceName); err != nil {
		failed = append(failed, fmt.Sprintf("ICMP: %v", err))
	}

	if err := sriovenv.RunProtocolTest(clientPod, "TCP",
		fmt.Sprintf("testcmd -protocol tcp -port 5001 -interface %s -server %s -mtu %d",
			interfaceName, serverIPAddress, packetSize)); err != nil {
		failed = append(failed, fmt.Sprintf("TCP: %v", err))
	}

	if err := sriovenv.RunProtocolTest(clientPod, "UDP",
		fmt.Sprintf("testcmd -protocol udp -port 5002 -interface %s -server %s -mtu %d",
			interfaceName, serverIPAddress, packetSize)); err != nil {
		failed = append(failed, fmt.Sprintf("UDP: %v", err))
	}

	if err := sriovenv.RunProtocolTest(clientPod, "SCTP",
		fmt.Sprintf("testcmd -protocol sctp -port 5003 -interface %s -server %s -mtu %d",
			interfaceName, serverIPAddress, packetSize)); err != nil {
		failed = append(failed, fmt.Sprintf("SCTP: %v", err))
	}

	multicastGroup := tsparams.MulticastIPv4Group
	if strings.Contains(serverIPAddress, ":") {
		multicastGroup = tsparams.MulticastIPv6Group
	} else if mtu == 9000 {
		multicastGroup = tsparams.MulticastIPv4GroupLargeMTU
	}

	if err := sriovenv.RunProtocolTest(clientPod, "multicast",
		fmt.Sprintf("testcmd -multicast -protocol udp -port 5004 -interface %s -server %s -mtu %d",
			interfaceName, multicastGroup, packetSize)); err != nil {
		failed = append(failed, fmt.Sprintf("multicast: %v", err))
	}

	if len(failed) > 0 {
		return fmt.Errorf("traffic tests failed: %s", strings.Join(failed, "; "))
	}

	return nil
}

func triggerBondLinkFailureAndVerify(clientPod *pod.Builder, bondMode, serverIP string, mtu int) error {
	// active-backup exposes active_slave and should switch deterministically.
	if bondMode == bondModeActiveBackup {
		active, err := getBondActiveSlave(clientPod, bondInterfaceName)
		if err != nil {
			return err
		}

		if err := setLinkStatus(clientPod, active, "down"); err != nil {
			return fmt.Errorf("failed to bring active slave %s down: %w", active, err)
		}

		if _, err := waitForBondActiveSlaveChange(clientPod, bondInterfaceName, active); err != nil {
			return err
		}

		if err := runTrafficTestOnInterface(clientPod, serverIP, mtu, bondInterfaceName); err != nil {
			return fmt.Errorf("traffic failed after failover: %w", err)
		}

		_ = setLinkStatus(clientPod, active, "up")

		return nil
	}

	// For balance-rr / balance-xor, prefer switching off a physical port on the lab switch
	// (matches upstream behavior) when switch credentials/interfaces are available.
	if err := toggleSwitchPortsAndVerifyTraffic(clientPod, serverIP, mtu); err == nil {
		return nil
	}

	// Fallback: validate resilience by toggling the slave links in the pod.
	if err := setLinkStatus(clientPod, bondSlave1IfName, "down"); err != nil {
		return fmt.Errorf("failed to bring %s down: %w", bondSlave1IfName, err)
	}

	if err := runTrafficTestOnInterface(clientPod, serverIP, mtu, bondInterfaceName); err != nil {
		return fmt.Errorf("traffic failed with %s down: %w", bondSlave1IfName, err)
	}

	_ = setLinkStatus(clientPod, bondSlave1IfName, "up")

	if err := setLinkStatus(clientPod, bondSlave2IfName, "down"); err != nil {
		return fmt.Errorf("failed to bring %s down: %w", bondSlave2IfName, err)
	}

	if err := runTrafficTestOnInterface(clientPod, serverIP, mtu, bondInterfaceName); err != nil {
		return fmt.Errorf("traffic failed with %s down: %w", bondSlave2IfName, err)
	}

	_ = setLinkStatus(clientPod, bondSlave2IfName, "up")

	return nil
}

func toggleSwitchPortsAndVerifyTraffic(clientPod *pod.Builder, serverIP string, mtu int) error {
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

		trafficErr := runTrafficTestOnInterface(clientPod, serverIP, mtu, bondInterfaceName)

		if err := enable(iface); err != nil {
			if trafficErr != nil {
				return fmt.Errorf("failed to re-enable switch interface %s: %w (traffic test error: %w)", iface, err, trafficErr)
			}

			return fmt.Errorf("failed to re-enable switch interface %s: %w", iface, err)
		}

		if trafficErr != nil {
			return fmt.Errorf("traffic failed with switch interface %s disabled: %w", iface, trafficErr)
		}
	}

	return nil
}

func getBondActiveSlave(clientPod *pod.Builder, bondName string) (string, error) {
	out, err := clientPod.ExecCommand([]string{"bash", "-c",
		fmt.Sprintf("cat /sys/class/net/%s/bonding/active_slave", bondName)})
	if err != nil {
		return "", fmt.Errorf("failed to read bond active_slave: %w (out=%s)", err, out.String())
	}

	return strings.TrimSpace(out.String()), nil
}

// waitForBondActiveSlaveChange polls active_slave until it differs from previousSlave or times out.
func waitForBondActiveSlaveChange(clientPod *pod.Builder, bondName, previousSlave string) (string, error) {
	deadline := time.Now().Add(bondActiveSlaveChangeTimeout)

	var last string

	for {
		slave, err := getBondActiveSlave(clientPod, bondName)
		if err != nil {
			return "", err
		}

		last = slave

		if slave != "" && slave != previousSlave {
			return slave, nil
		}

		if time.Now().After(deadline) {
			return last, fmt.Errorf(
				"bond did not switch active slave from %q within %v (last active_slave=%q)",
				previousSlave, bondActiveSlaveChangeTimeout, last)
		}

		time.Sleep(bondActiveSlavePollInterval)
	}
}

func setLinkStatus(podBuilder *pod.Builder, nic string, status string) error {
	out, err := podBuilder.ExecCommand([]string{"bash", "-c", fmt.Sprintf("ip link set dev %s %s", nic, status)})
	if err != nil {
		return fmt.Errorf("failed to set interface %s %s: %w (out=%s)", nic, status, err, out.String())
	}

	return nil
}

func verifyBondInterfaceState(podBuilder *pod.Builder, expectedMode string, expectedSlaveCount int) error {
	// Up?
	out, err := podBuilder.ExecCommand([]string{"bash", "-c",
		fmt.Sprintf("cat /sys/class/net/%s/operstate", bondInterfaceName)})
	if err != nil {
		return fmt.Errorf("failed to read bond operstate: %w (out=%s)", err, out.String())
	}

	if strings.TrimSpace(out.String()) != "up" {
		return fmt.Errorf("bond interface %s is not up (operstate=%q)", bondInterfaceName, strings.TrimSpace(out.String()))
	}

	// Mode?
	out, err = podBuilder.ExecCommand([]string{"bash", "-c",
		fmt.Sprintf("cat /sys/class/net/%s/bonding/mode", bondInterfaceName)})
	if err != nil {
		return fmt.Errorf("failed to read bond mode: %w (out=%s)", err, out.String())
	}

	if !strings.Contains(out.String(), expectedMode) {
		return fmt.Errorf("bond mode mismatch: expected %q, got %q", expectedMode, strings.TrimSpace(out.String()))
	}

	// Slave count?
	out, err = podBuilder.ExecCommand([]string{"bash", "-c",
		fmt.Sprintf("cat /sys/class/net/%s/bonding/slaves | wc -w", bondInterfaceName)})
	if err != nil {
		return fmt.Errorf("failed to read bond slaves: %w (out=%s)", err, out.String())
	}

	got := strings.TrimSpace(out.String())
	if got != fmt.Sprintf("%d", expectedSlaveCount) {
		return fmt.Errorf("bond slave count mismatch: expected %d, got %s", expectedSlaveCount, got)
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

func createTwoSriovNetworksWhereaboutsIPv4(netA, netB, resource, ipRange, gateway string) error {
	if err := sriovenv.CreateSriovNetworkWithWhereaboutsIPAM(
		netA, resource, ipRange, gateway, "", "",
	); err != nil {
		return err
	}

	return sriovenv.CreateSriovNetworkWithWhereaboutsIPAM(
		netB, resource, ipRange, gateway, "", "",
	)
}

func createTwoSriovNetworksWhereaboutsIPv6(netA, netB, resource, ipv6Range, gateway string) error {
	if err := sriovenv.CreateSriovNetworkWithWhereaboutsIPAM(
		netA, resource, ipv6Range, gateway, "", "",
	); err != nil {
		return err
	}

	return sriovenv.CreateSriovNetworkWithWhereaboutsIPAM(
		netB, resource, ipv6Range, gateway, "", "",
	)
}

func ensureSriovTestsNamespaceHasPrivilegedSCC() error {
	// Bond CNI needs permissions to enslave links to bond0; in OCP this is controlled by SCC.
	// We grant privileged SCC to the sriov-tests serviceaccounts group to avoid SCC selection
	// landing on a restrictive SCC (e.g. insights-runtime-extractor-scc).
	bind := func(nameSuffix string, subj rbacv1.Subject) error {
		crbName := fmt.Sprintf("%s-privileged-scc-%s", tsparams.TestNamespaceName, nameSuffix)

		crb := rbac.NewClusterRoleBindingBuilder(
			APIClient,
			crbName,
			"system:openshift:scc:privileged",
			subj,
		)

		_, err := crb.Create()
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return err
		}

		return nil
	}

	// Grant to all serviceaccounts in the namespace (covers default and any custom SAs).
	if err := bind("serviceaccounts", rbacv1.Subject{
		Kind: "Group",
		Name: fmt.Sprintf("system:serviceaccounts:%s", tsparams.TestNamespaceName),
	}); err != nil {
		return err
	}

	// Also grant explicitly to default SA (some clusters/SCC selection paths prefer direct SA subjects).
	return bind("default", rbacv1.Subject{
		Kind:      "ServiceAccount",
		Name:      "default",
		Namespace: tsparams.TestNamespaceName,
	})
}
