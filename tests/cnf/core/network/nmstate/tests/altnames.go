package tests

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nmstate"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/define"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netenv"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netnmstate"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/nmstate/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/sriovoperator"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

var _ = Describe("NMState Altnames:", Ordered, Label(tsparams.LabelAltnames), ContinueOnFailure, func() {
	var (
		workerNodes     []*nodes.Builder
		sriovInterfaces []string
		sriovIf0        string
		sriovIf1        string
		worker0LabelMap map[string]string
	)

	BeforeAll(func() {
		By("Discover worker nodes")

		var err error

		workerNodes, err = nodes.List(APIClient,
			metav1.ListOptions{LabelSelector: labels.Set(NetConfig.WorkerLabelMap).String()})
		Expect(err).ToNot(HaveOccurred(), "Fail to discover nodes")
		Expect(len(workerNodes)).To(BeNumerically(">=", 1), "Cluster must have at least 1 worker node")

		worker0LabelMap = map[string]string{corev1.LabelHostname: workerNodes[0].Object.Name}

		By("Collecting SR-IOV interfaces for altnames testing")

		sriovInterfaces, err = NetConfig.GetSriovInterfaces(2)
		Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for testing")

		sriovIf0 = sriovInterfaces[0]
		sriovIf1 = sriovInterfaces[1]
	})

	AfterEach(func() {
		By("Cleaning all NMState policies after each test")

		err := nmstate.CleanAllNMStatePolicies(APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to remove all NMState policies")
	})

	Context("configure altnames", func() {
		It("add and remove altnames without mac/pci identifiers", reportxml.ID("86293"), func() {
			policyName := "nncp-86293-altnames"
			removalPolicyName := "nncp-86293-remove-altnames"
			primaryIfaceAltnames := []string{"t86293-iface0-alt-a", "t86293-iface0-alt-b"}
			secondaryIfaceAltnames := []string{"t86293-iface1-alt-a", "t86293-iface1-alt-b"}

			By("Creating NMState policy for altnames without identifiers")

			nncp := nmstate.NewPolicyBuilder(APIClient, policyName, worker0LabelMap).
				WithInterfaceAltnames(sriovIf0, primaryIfaceAltnames).
				WithInterfaceAltnames(sriovIf1, secondaryIfaceAltnames)

			err := netnmstate.CreatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, nncp)
			Expect(err).ToNot(HaveOccurred(), "Failed to create NMState policy")

			By("Verifying altnames without identifiers")

			nnstates, err := getNodeNetworkStates(worker0LabelMap)
			Expect(err).ToNot(HaveOccurred(), "Failed to get NodeNetwork states")

			for _, nnstate := range nnstates {
				validateAltnamesFromNodeNetworkState(nnstate, sriovIf0, primaryIfaceAltnames, false)(Default)
				validateAltnamesFromNodeNetworkState(nnstate, sriovIf1, secondaryIfaceAltnames, false)(Default)
			}

			By("Delete Existing NNCP and Create a new one with altnames removed")

			deletePriorPoliciesApplyRemovalPolicyAndVerify(
				removalPolicyName,
				worker0LabelMap,
				[]*nmstate.PolicyBuilder{nncp},
				[]string{sriovIf0, sriovIf1},
				[][]string{primaryIfaceAltnames, secondaryIfaceAltnames},
			)
		})

		It("add and remove altnames with identifiers mac-address and pci-address", reportxml.ID("86294"), func() {
			policyName := "nncp-86294-altnames"
			removalPolicyName := "nncp-86294-remove-altnames"
			altnames1 := []string{"t86294-iface0-alt-a", "t86294-iface0-alt-b"}
			altnames2 := []string{"t86294-iface1-alt-a", "t86294-iface1-alt-b"}

			By("Getting MAC addresses and PCI addresses of SR-IOV interfaces")

			if0MacAddress, _, err := getInterfaceIdentifiersOfNode(workerNodes[0].Object.Name, sriovIf0)
			Expect(err).ToNot(HaveOccurred(), "Failed to get MAC and PCI addresses of SR-IOV interface")
			_, if1PciAddress, err := getInterfaceIdentifiersOfNode(workerNodes[0].Object.Name, sriovIf1)
			Expect(err).ToNot(HaveOccurred(), "Failed to get MAC and PCI addresses of SR-IOV interface")

			nncp := nmstate.NewPolicyBuilder(APIClient, policyName, worker0LabelMap).
				WithMACAddressAltnames("dummy0", if0MacAddress, altnames1).
				WithPCIAddressAltnames("dummy1", if1PciAddress, altnames2)

			err = netnmstate.CreatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, nncp)
			Expect(err).ToNot(HaveOccurred(), "Failed to create NMState policy")

			nnstate, err := nmstate.PullNodeNetworkState(APIClient, workerNodes[0].Object.Name)
			Expect(err).ToNot(HaveOccurred(), "Failed to get NodeNetwork state")

			validateAltnamesFromNodeNetworkState(nnstate, sriovIf0, altnames1, false)(Default)
			validateAltnamesFromNodeNetworkState(nnstate, sriovIf1, altnames2, false)(Default)

			By("Delete Existing NNCP and Create a new one with altnames removed")

			deletePriorPoliciesApplyRemovalPolicyAndVerify(
				removalPolicyName,
				worker0LabelMap,
				[]*nmstate.PolicyBuilder{nncp},
				[]string{sriovIf0, sriovIf1},
				[][]string{altnames1, altnames2},
			)
		})

		It("delete altname of an interface that is manually configured", reportxml.ID("86299"), func() {
			policyName := "nncp-86299-remove-altnames"
			altnames1 := []string{"t86299-manual-alt-a"}

			_, err := cluster.ExecCmdWithStdout(
				APIClient,
				fmt.Sprintf("ip link property add altname %s dev %s", altnames1[0], sriovIf0),
				metav1.ListOptions{LabelSelector: labels.Set(worker0LabelMap).String()})
			Expect(err).ToNot(HaveOccurred(),
				"Failed to add altname to interface (see ip-link altname support for %s)", sriovIf0)

			DeferCleanup(func() {
				By("Removing manually-added altname from interface if still present")

				// Best-effort: the altname may already have been removed by the NNCP policy.
				_, _ = cluster.ExecCmdWithStdout(
					APIClient,
					fmt.Sprintf("ip link property del altname %s dev %s", altnames1[0], sriovIf0),
					metav1.ListOptions{LabelSelector: labels.Set(worker0LabelMap).String()})
			})

			Eventually(validateAltnamesOnNode(
				workerNodes[0].Object.Name, sriovIf0, altnames1, false)).
				WithTimeout(netparam.DefaultTimeout).WithPolling(5 * time.Second).Should(Succeed())

			nncp := nmstate.NewPolicyBuilder(APIClient, policyName, worker0LabelMap).
				RemoveInterfaceAltname(sriovIf0, altnames1)

			err = netnmstate.CreatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, nncp)
			Expect(err).ToNot(HaveOccurred(), "Failed to update NMState policy")

			Eventually(validateAltnamesOnNode(
				workerNodes[0].Object.Name, sriovIf0, altnames1, true)).
				WithTimeout(netparam.DefaultTimeout).WithPolling(5 * time.Second).Should(Succeed())

			_, err = nncp.Delete()
			Expect(err).ToNot(HaveOccurred(), "Failed to delete NMState policy")
		})

		It("remove physical interface that has altnames to verify altnames are removed", reportxml.ID("86298"), func() {
			configurePolicyName := "nncp-86298-altnames"
			absentPolicyName := "nncp-86298-absent"
			restorePolicyName := "nncp-86298-restore"
			altnames1 := []string{"t86298-iface0-alt-a", "t86298-iface0-alt-b"}

			By("Creating NMState policy for altnames without identifiers")

			nncp := nmstate.NewPolicyBuilder(APIClient, configurePolicyName, worker0LabelMap).
				WithInterfaceAltnames(sriovIf0, altnames1)

			err := netnmstate.CreatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, nncp)
			Expect(err).ToNot(HaveOccurred(), "Failed to create NMState policy")

			By("Verifying altnames without identifiers")

			nnstates, err := getNodeNetworkStates(worker0LabelMap)
			Expect(err).ToNot(HaveOccurred(), "Failed to get NodeNetwork states")

			for _, nnstate := range nnstates {
				validateAltnamesFromNodeNetworkState(nnstate, sriovIf0, altnames1, false)(Default)
			}

			By("Delete Existing NNCP and Create a new one with physical interface removed")

			_, err = nncp.Delete()
			Expect(err).ToNot(HaveOccurred(), "Failed to delete NMState policy")

			nncp = nmstate.NewPolicyBuilder(APIClient, absentPolicyName, worker0LabelMap).
				WithAbsentInterface(sriovIf0)

			err = netnmstate.CreatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, nncp)
			Expect(err).ToNot(HaveOccurred(), "Failed to create NMState policy")

			By("Verifying physical interface removed")

			Eventually(validateAltnamesOnNode(
				workerNodes[0].Object.Name, sriovIf0, altnames1, true)).
				WithTimeout(netparam.DefaultTimeout).WithPolling(5 * time.Second).Should(Succeed())

			By("Restoring the physical interface to up state")

			_, err = nncp.Delete()
			Expect(err).ToNot(HaveOccurred(), "Failed to delete absent NMState policy")

			nncp = nmstate.NewPolicyBuilder(APIClient, restorePolicyName, worker0LabelMap).
				WithInterfaceUp(sriovIf0)

			err = netnmstate.CreatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, nncp)
			Expect(err).ToNot(HaveOccurred(), "Failed to create NMState policy to restore interface")
		})

		It("add altnames for SRIOV VFs that are configured via NMState", reportxml.ID("86297"), func() {
			configureVFsPolicyName := "nncp-86297-configure-sriov-vfs"
			configureAltnamesPolicyName := "nncp-86297-configure-vf-altnames"
			removeAltnamesPolicyName := "nncp-86297-remove-vf-altnames"
			disableVFsPolicyName := "nncp-86297-disable-sriov-vfs"
			altnames1 := []string{"t86297-vf0-alt-a", "t86297-vf0-alt-b"}

			By("Configuring Mellanox firmware on the worker node")

			isMellanox, err := netenv.IsMellanoxDevice(
				APIClient, NetConfig.SriovOperatorNamespace, sriovIf0, workerNodes[0].Object.Name)
			Expect(err).ToNot(HaveOccurred(), "Failed to check if interface is a Mellanox device")

			if isMellanox {
				err := netenv.ConfigureSriovMlnxFirmwareOnWorkersAndWaitMCP(
					APIClient,
					netparam.MCOWaitTimeout,
					time.Minute,
					NetConfig.CnfMcpLabel,
					NetConfig.SriovOperatorNamespace,
					[]*nodes.Builder{workerNodes[0]},
					sriovIf0,
					true,
					5,
				)
				Expect(err).ToNot(HaveOccurred(), "Failed to configure Mellanox firmware")
			}

			By("Creating SR-IOV VFs via NMState")

			err = netnmstate.ConfigureVFsAndWaitUntilItsConfigured(
				configureVFsPolicyName,
				sriovIf0,
				worker0LabelMap,
				5,
				netparam.DefaultTimeout)
			Expect(err).ToNot(HaveOccurred(), "Failed to create VFs via NMState")

			DeferCleanup(func() {
				By("Disabling SR-IOV VFs created for test 86297")

				_, _ = nmstate.NewPolicyBuilder(APIClient, configureVFsPolicyName, worker0LabelMap).Delete()

				cleanupNNCP := nmstate.NewPolicyBuilder(APIClient, disableVFsPolicyName, worker0LabelMap).
					WithInterfaceAndVFs(sriovIf0, 0)
				cleanupErr := netnmstate.CreatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, cleanupNNCP)
				Expect(cleanupErr).ToNot(HaveOccurred(), "Failed to disable SR-IOV during cleanup")

				_, cleanupErr = cleanupNNCP.Delete()
				Expect(cleanupErr).ToNot(HaveOccurred(), "Failed to delete cleanup NMState policy")
			})

			err = netenv.WaitUntilVfsCreated(
				APIClient, NetConfig.SriovOperatorNamespace, []*nodes.Builder{workerNodes[0]}, sriovIf0, 5, netparam.DefaultTimeout)
			Expect(err).ToNot(HaveOccurred(), "Expected number of VFs are not created")

			nnstateWorker0, err := nmstate.PullNodeNetworkState(APIClient, workerNodes[0].Object.Name)
			Expect(err).ToNot(HaveOccurred(), "Failed to pull NodeNetworkState for VF0 name resolution")

			firstVFNetdev, err := netnmstate.SrIovVfNetdevFromNodeNetworkState(
				nnstateWorker0, sriovIf0, 0)
			Expect(err).ToNot(HaveOccurred(), "Failed to resolve VF0 netdev from NodeNetworkState (MAC match)")

			By("Adding altnames for SR-IOV VFs that are configured via NMState")

			nncp := nmstate.NewPolicyBuilder(APIClient, configureAltnamesPolicyName, worker0LabelMap).
				WithInterfaceAltnames(firstVFNetdev, altnames1)

			err = netnmstate.CreatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, nncp)
			Expect(err).ToNot(HaveOccurred(), "Failed to create NMState policy")

			By("Verifying altnames for SR-IOV VFs that are configured via NMState")

			Eventually(validateAltnamesOnNode(
				workerNodes[0].Object.Name, firstVFNetdev, altnames1, false)).
				WithTimeout(netparam.DefaultTimeout).WithPolling(5 * time.Second).Should(Succeed())

			By("Delete Existing NNCP and Create a new one with altnames removed")

			_, err = nncp.Delete()
			Expect(err).ToNot(HaveOccurred(), "Failed to delete NMState policy")

			nncp = nmstate.NewPolicyBuilder(APIClient, removeAltnamesPolicyName, worker0LabelMap).
				RemoveInterfaceAltname(firstVFNetdev, altnames1)

			err = netnmstate.CreatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, nncp)
			Expect(err).ToNot(HaveOccurred(), "Failed to create NMState policy")

			By("Verifying altnames removed")

			Eventually(validateAltnamesOnNode(
				workerNodes[0].Object.Name, firstVFNetdev, altnames1, true)).
				WithTimeout(netparam.DefaultTimeout).WithPolling(5 * time.Second).Should(Succeed())
		})
	})

	Context("negative tests", func() {
		It("NNCP should be Degraded when duplicate altnames are added", reportxml.ID("86296"), func() {
			degradedPolicyName := "nncp-86296-altnames"
			removalPolicyName := "nncp-86296-remove-altnames"
			altnames1 := []string{"t86296-dup-alt-a", "t86296-dup-alt-a"}
			altnames2 := []string{"t86296-dup-alt-a"}

			By("Creating NMState policy with duplicate altnames to trigger Degraded state")

			nncp := nmstate.NewPolicyBuilder(APIClient, degradedPolicyName, worker0LabelMap).
				WithInterfaceAltnames(sriovIf0, altnames1)

			err := netnmstate.CreatePolicyAndWaitUntilItsDegraded(netparam.DefaultTimeout, nncp)
			Expect(err).ToNot(HaveOccurred(), "Failed to create NMState policy")

			By("Deleting the NMState policy")

			_, err = nncp.Delete()
			Expect(err).ToNot(HaveOccurred(), "Failed to delete NMState policy")

			deletePriorPoliciesApplyRemovalPolicyAndVerify(
				removalPolicyName,
				worker0LabelMap,
				nil,
				[]string{sriovIf0},
				[][]string{altnames2},
			)
		})

		It("NNCP should be Degraded when using non-existent mac-address or pci-address", reportxml.ID("86295"), func() {
			policyName := "nncp-86295-altnames"
			altnames1 := []string{"t86295-invalid-id-alt-a", "t86295-invalid-id-alt-b"}

			By("Creating NMState policy with non-existent MAC and PCI identifiers to trigger Degraded state")

			nncp := nmstate.NewPolicyBuilder(APIClient, policyName, worker0LabelMap).
				WithMACAddressAltnames("dummy-mac-address", "00:00:00:00:00:00", altnames1).
				WithPCIAddressAltnames("dummy-pci-address", "0000:00:00.0", altnames1)

			err := netnmstate.CreatePolicyAndWaitUntilItsDegraded(netparam.DefaultTimeout, nncp)
			Expect(err).ToNot(HaveOccurred(), "Failed to create NMState policy")

			By("Deleting the NMState policy")

			_, err = nncp.Delete()
			Expect(err).ToNot(HaveOccurred(), "Failed to delete NMState policy")
		})
	})

	Context("refer interface with altname", func() {
		It("add and remove altnames for interface using its altname", reportxml.ID("88006"), func() {
			policyName1 := "nncp-88006-altnames"
			policyName2 := "nncp-88006-altnames-ref"
			removalPolicyName := "nncp-88006-remove-altnames"
			altnames1 := []string{"t88006-base-alt-a"}
			altnames2 := []string{"t88006-ref-alt-b"}
			combinedAltnames := []string{"t88006-base-alt-a", "t88006-ref-alt-b"}

			By("Creating NMState policy for altnames without identifiers")

			nncp1 := nmstate.NewPolicyBuilder(APIClient, policyName1, worker0LabelMap).
				WithInterfaceAltnames(sriovIf0, altnames1)

			err := netnmstate.CreatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, nncp1)
			Expect(err).ToNot(HaveOccurred(), "Failed to create NMState policy")

			By("Verifying the altname is added")

			nnstates, err := getNodeNetworkStates(worker0LabelMap)
			Expect(err).ToNot(HaveOccurred(), "Failed to get NodeNetwork states")

			for _, nnstate := range nnstates {
				validateAltnamesFromNodeNetworkState(nnstate, sriovIf0, altnames1, false)(Default)
			}

			nncp2 := nmstate.NewPolicyBuilder(APIClient, policyName2, worker0LabelMap).
				WithInterfaceAltnames(altnames1[0], altnames2)

			err = netnmstate.CreatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, nncp2)
			Expect(err).ToNot(HaveOccurred(), "Failed to create NMState policy")

			By("Verifying the altname is added")

			nnstates, err = getNodeNetworkStates(worker0LabelMap)
			Expect(err).ToNot(HaveOccurred(), "Failed to get NodeNetwork states")

			for _, nnstate := range nnstates {
				validateAltnamesFromNodeNetworkState(nnstate, sriovIf0, combinedAltnames, false)(Default)
			}

			By("Deleting the NMState policy")
			deletePriorPoliciesApplyRemovalPolicyAndVerify(
				removalPolicyName,
				worker0LabelMap,
				[]*nmstate.PolicyBuilder{nncp1, nncp2},
				[]string{sriovIf0},
				[][]string{combinedAltnames},
			)
		})

		It("use altname for network-attachment-definition of type host-device", reportxml.ID("88008"), func() {
			policyName := "nncp-88008-altnames"
			removalPolicyName := "nncp-88008-remove-altnames"
			altnames1 := []string{"t88008-hostdev-alt-a"}

			By("Creating NMState policy to create altnames for interface")

			nncp := nmstate.NewPolicyBuilder(APIClient, policyName, worker0LabelMap).
				WithInterfaceAltnames(sriovIf0, altnames1)

			err := netnmstate.CreatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, nncp)
			Expect(err).ToNot(HaveOccurred(), "Failed to create NMState policy")

			By("Verifying the altname is added")

			nnstates, err := getNodeNetworkStates(worker0LabelMap)
			Expect(err).ToNot(HaveOccurred(), "Failed to get NodeNetwork states")

			for _, nnstate := range nnstates {
				validateAltnamesFromNodeNetworkState(nnstate, sriovIf0, altnames1, false)(Default)
			}

			By("Creating NetworkAttachmentDefinition of type host-device with altname as interface name")

			_, err = define.HostDeviceNad(
				APIClient,
				"host-device-with-altname",
				altnames1[0],
				tsparams.TestNamespaceName,
			)
			Expect(err).ToNot(HaveOccurred(), "Failed to create a network-attachment-definition")

			By("creating pod with network-attachment-definition of type host-device with altname as interface name")

			testPod, err := pod.NewBuilder(APIClient, "testpod", tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
				DefineOnNode(workerNodes[0].Object.Name).
				WithSecondaryNetwork([]*types.NetworkSelectionElement{
					{
						Name: "host-device-with-altname",
					},
				}).
				CreateAndWaitUntilRunning(netparam.DefaultTimeout)
			Expect(err).ToNot(HaveOccurred(), "Failed to create pod")

			By("verifying the pod's secondary interface is available")

			_, err = testPod.ExecCommand([]string{"ip", "addr", "show", "net1"})
			Expect(err).ToNot(HaveOccurred(), "Failed to execute ip addr show command")

			By("clean test resources")

			testNs, err := namespace.Pull(APIClient, tsparams.TestNamespaceName)
			Expect(err).ToNot(HaveOccurred(), "Failed to pull namespace")
			err = testNs.CleanObjects(netparam.DefaultTimeout, nad.GetGVR(), pod.GetGVR())
			Expect(err).ToNot(HaveOccurred(), "Failed to clean test resources")

			By("Deleting the NMState policy")
			deletePriorPoliciesApplyRemovalPolicyAndVerify(
				removalPolicyName,
				worker0LabelMap,
				[]*nmstate.PolicyBuilder{nncp},
				[]string{sriovIf0},
				[][]string{altnames1},
			)
		})

		It("create sriov policy with altname as interface name", reportxml.ID("88007"), func() {
			policyName := "nncp-88007-altnames"
			removalPolicyName := "nncp-88007-remove-altnames"
			sriovPolicyName := "sriov-policy-88007-altname"
			altnames1 := []string{"t88007-sriov-alt-a"}

			By("Creating NMState policy to create altnames for interface")

			nncp := nmstate.NewPolicyBuilder(APIClient, policyName, worker0LabelMap).
				WithInterfaceAltnames(sriovIf0, altnames1)

			err := netnmstate.CreatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, nncp)
			Expect(err).ToNot(HaveOccurred(), "Failed to create NMState policy")

			DeferCleanup(func() {
				deletePriorPoliciesApplyRemovalPolicyAndVerify(
					removalPolicyName,
					worker0LabelMap,
					[]*nmstate.PolicyBuilder{nncp},
					[]string{sriovIf0},
					[][]string{altnames1},
				)
			})

			By("Verifying the altname is added")

			nnstates, err := getNodeNetworkStates(worker0LabelMap)
			Expect(err).ToNot(HaveOccurred(), "Failed to get NodeNetwork states")

			for _, nnstate := range nnstates {
				validateAltnamesFromNodeNetworkState(nnstate, sriovIf0, altnames1, false)(Default)
			}

			By("Waiting for SriovNetworkNodeState to reflect the new altname")

			Eventually(func() error {
				nodeState := sriov.NewNetworkNodeStateBuilder(
					APIClient, workerNodes[0].Object.Name, NetConfig.SriovOperatorNamespace)

				nics, err := nodeState.GetNICs()
				if err != nil {
					return err
				}

				for _, nic := range nics {
					if nic.Name != sriovIf0 {
						continue
					}

					for _, altname := range nic.AltNames {
						if altname == altnames1[0] {
							return nil
						}
					}
				}

				return fmt.Errorf("altname %s not yet found in SriovNetworkNodeState interfaces", altnames1[0])
			}).WithTimeout(30*time.Second).WithPolling(5*time.Second).Should(Succeed(),
				"SriovNetworkNodeState did not reflect the new altname in time")

			By("Creating SriovNetworkNodePolicy with altname as interface name")

			sriovPolicy, err := sriov.NewPolicyBuilder(APIClient,
				sriovPolicyName, NetConfig.SriovOperatorNamespace, "sriovpf1", 6,
				altnames1, worker0LabelMap).
				WithDevType("netdevice").
				Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create SriovNetworkNodePolicy")

			DeferCleanup(func() {
				By("Deleting the SriovNetworkNodePolicy")

				err := sriovPolicy.Delete()
				Expect(err).ToNot(HaveOccurred(), "Failed to delete SriovNetworkNodePolicy")

				err = sriovoperator.WaitForSriovAndMCPStable(APIClient, netparam.MCOWaitTimeout, 1*time.Minute,
					NetConfig.CnfMcpLabel, NetConfig.SriovOperatorNamespace)
				Expect(err).ToNot(HaveOccurred(), "Failed cluster is not stable before creating test resources")
			})

			err = sriovoperator.WaitForSriovAndMCPStable(APIClient, netparam.MCOWaitTimeout, 1*time.Minute,
				NetConfig.CnfMcpLabel, NetConfig.SriovOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed cluster is not stable before creating test resources")

			By("Verifying the sriov resource is created")

			workerNode, err := nodes.Pull(APIClient, workerNodes[0].Object.Name)
			Expect(err).ToNot(HaveOccurred(), "Failed to get worker node0")

			resource, ok := workerNode.Object.Status.Allocatable[corev1.ResourceName("openshift.io/sriovpf1")]
			Expect(ok).To(BeTrue(), "SriovResource is not present in the worker node")

			resourceCount, canConvert := resource.AsInt64()
			Expect(canConvert).To(BeTrue(), "Failed to parse SriovResource quantity as int64")
			Expect(resourceCount).To(Equal(int64(6)), "SriovResource is not equal to 6 in the worker node")
		})
	})
})

func getNodeNetworkStates(label map[string]string) ([]*nmstate.StateBuilder, error) {
	nodesList, err := nodes.List(APIClient, metav1.ListOptions{LabelSelector: labels.Set(label).String()})
	if err != nil {
		return nil, err
	}

	var nnstates []*nmstate.StateBuilder

	for _, node := range nodesList {
		nnstate, err := nmstate.PullNodeNetworkState(APIClient, node.Object.Name)
		if err != nil {
			return nil, err
		}

		nnstates = append(nnstates, nnstate)
	}

	return nnstates, nil
}

// validateAltnamesOnNode returns a Gomega assertion function that pulls fresh NodeNetworkState on each poll,
// for use with Eventually(...).Should(Succeed()).
func validateAltnamesOnNode(nodeName, interfaceName string, altnames []string, absent bool) func(Gomega) {
	return func(g Gomega) {
		nnstate, err := nmstate.PullNodeNetworkState(APIClient, nodeName)
		g.Expect(err).ToNot(HaveOccurred(), "Failed to get NodeNetwork state for node %q", nodeName)
		validateAltnamesFromNodeNetworkState(nnstate, interfaceName, altnames, absent)(g)
	}
}

// validateAltnamesFromNodeNetworkState returns a Gomega assertion function for use with
// Eventually(fn).Should(Succeed())
// or a single synchronous check via validate...(Default).
func validateAltnamesFromNodeNetworkState(
	nnstate *nmstate.StateBuilder, interfaceName string, altnames []string, absent bool) func(Gomega) {
	return func(gomega Gomega) {
		altnamesFromNodeNetworkState, err := nnstate.GetInterfaceAltnames(interfaceName)
		gomega.Expect(err).ToNot(HaveOccurred(),
			"get interface altnames for %q on NodeNetworkState %q", interfaceName, nnstate.Object.Name)

		if absent {
			gomega.Expect(altnamesFromNodeNetworkState).Should(Not(ContainElements(altnames)),
				"interface %q on NodeNetworkState %q should not list altnames %v; actual altnames: %v",
				interfaceName, nnstate.Object.Name, altnames, altnamesFromNodeNetworkState)
		} else {
			gomega.Expect(altnamesFromNodeNetworkState).To(ContainElements(altnames),
				"interface %q on NodeNetworkState %q should list altnames %v; actual altnames: %v",
				interfaceName, nnstate.Object.Name, altnames, altnamesFromNodeNetworkState)
		}
	}
}

func getInterfaceIdentifiersOfNode(name string, interfaceName string) (string, string, error) {
	nnstate, err := nmstate.PullNodeNetworkState(APIClient, name)
	if err != nil {
		return "", "", err
	}

	macAddress, err := nnstate.GetInterfaceMACAddress(interfaceName)
	if err != nil {
		return "", "", err
	}

	pciAddress, err := nnstate.GetInterfacePCIAddress(interfaceName)
	if err != nil {
		return "", "", err
	}

	return macAddress, pciAddress, nil
}

// deletePriorPoliciesApplyRemovalPolicyAndVerify deletes any prior NMState policies, creates removalPolicyName with
// RemoveInterfaceAltname calls derived from interfaceNames/altnames, waits until available, verifies each pair
// shows altnames absent, then deletes the removal policy. Callers should issue any desired By() (e.g. before
// deletes) before invoking this helper.
func deletePriorPoliciesApplyRemovalPolicyAndVerify(
	removalPolicyName string,
	nodeLabel map[string]string,
	priorPolicies []*nmstate.PolicyBuilder,
	interfaceNames []string,
	altnames [][]string,
) {
	Expect(len(interfaceNames)).To(Equal(len(altnames)),
		"interfaceNames and altnames should have the same number of entries")

	for _, p := range priorPolicies {
		_, err := p.Delete()
		Expect(err).ToNot(HaveOccurred(), "Failed to delete NMState policy")
	}

	By("Removing the altname")

	nncp := nmstate.NewPolicyBuilder(APIClient, removalPolicyName, nodeLabel)
	for i, iface := range interfaceNames {
		nncp = nncp.RemoveInterfaceAltname(iface, altnames[i])
	}

	err := netnmstate.CreatePolicyAndWaitUntilItsAvailable(netparam.DefaultTimeout, nncp)
	Expect(err).ToNot(HaveOccurred(), "Failed to create NMState policy")

	By("Verifying altnames removed")

	nnstates, err := getNodeNetworkStates(nodeLabel)
	Expect(err).ToNot(HaveOccurred(), "Failed to get NodeNetwork states")

	for _, nnstate := range nnstates {
		for i, iface := range interfaceNames {
			validateAltnamesFromNodeNetworkState(nnstate, iface, altnames[i], true)(Default)
		}
	}

	By("Deleting the NMState policy")

	_, err = nncp.Delete()
	Expect(err).ToNot(HaveOccurred(), "Failed to delete NMState policy")
}
