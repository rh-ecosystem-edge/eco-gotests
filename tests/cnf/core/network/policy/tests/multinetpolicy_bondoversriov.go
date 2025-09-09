package tests

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"

	multinetpolicyapiv1 "github.com/k8snetworkplumbingwg/multi-networkpolicy/pkg/apis/k8s.cni.cncf.io/v1beta1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/networkpolicy"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netenv"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/policy/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/params"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

var _ = Describe("Multi-NetworkPolicy : Bond CNI", Ordered, Label("bondcnioversriov"), ContinueOnFailure, func() {

	var (
		sriovInterfacesUnderTest                         []string
		tNs1, tNs2                                       *namespace.Builder
		testPod1, testPod2, testPod3, testPod4, testPod5 *pod.Builder
		testNAD1, testNAD2                               *nad.Builder
	)

	const (
		nicPf1, nicPf2               = "pf1", "pf2"
		ns1, ns2                     = "ns1", "ns2"
		pod1, pod2, pod3, pod4, pod5 = "pod1", "pod2", "pod3", "pod4", "pod5"
	)

	BeforeAll(func() {
		By("Verifying if Multi-NetPolicy tests can be executed on given cluster")
		err := netenv.DoesClusterHasEnoughNodes(APIClient, NetConfig, 1, 1)
		Expect(err).ToNot(HaveOccurred(),
			"Cluster doesn't support Multi-NetPolicy test cases as it doesn't have enough nodes")

		By("Listing Worker nodes")
		workerNodeList, err := nodes.List(
			APIClient, metav1.ListOptions{LabelSelector: labels.Set(NetConfig.WorkerLabelMap).String()})
		Expect(err).ToNot(HaveOccurred(), "Failed to list worker nodes")

		By("Fetching SR-IOV interfaces from ENV VAR")
		sriovInterfacesUnderTest, err = NetConfig.GetSriovInterfaces(2)
		Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for testing")

		By("Enable MultiNetworkPolicy support")
		enableMultiNetworkPolicy(true)

		By("Deploy Test Resources: 2 Namespaces")
		tNs1, err = namespace.NewBuilder(APIClient, tsparams.MultiNetPolNs1).WithMultipleLabels(params.PrivilegedNSLabels).
			WithLabel("ns", "ns1").Create()
		Expect(err).ToNot(HaveOccurred(), "Failed to create test namespace")
		tNs2, err = namespace.NewBuilder(APIClient, tsparams.MultiNetPolNs2).WithMultipleLabels(params.PrivilegedNSLabels).
			WithLabel("ns", "ns2").Create()
		Expect(err).ToNot(HaveOccurred(), "Failed to create test namespace")

		By("Deploy Test Resources: 2 Sriov Policies")
		_, err = sriov.NewPolicyBuilder(APIClient,
			"nicpf1", NetConfig.SriovOperatorNamespace, nicPf1, 5,
			[]string{sriovInterfacesUnderTest[0]}, NetConfig.WorkerLabelMap).
			WithDevType("netdevice").
			Create()
		Expect(err).ToNot(HaveOccurred(), "Failed to create test policy")

		_, err = sriov.NewPolicyBuilder(APIClient,
			"nicpf2", NetConfig.SriovOperatorNamespace, nicPf2, 5,
			[]string{sriovInterfacesUnderTest[1]}, NetConfig.WorkerLabelMap).
			WithDevType("netdevice").
			Create()
		Expect(err).ToNot(HaveOccurred(), "Failed to create test policy")

		By("Deploy Test Resources: 4 Sriov Networks")
		defineAndCreateSriovNetwork(ns1+nicPf1, nicPf1, tsparams.MultiNetPolNs1)
		defineAndCreateSriovNetwork(ns1+nicPf2, nicPf2, tsparams.MultiNetPolNs1)
		defineAndCreateSriovNetwork(ns2+nicPf1, nicPf1, tsparams.MultiNetPolNs2)
		defineAndCreateSriovNetwork(ns2+nicPf2, nicPf2, tsparams.MultiNetPolNs2)

		err = netenv.WaitForSriovAndMCPStable(APIClient, tsparams.MCOWaitTimeout, 10*time.Second,
			NetConfig.CnfMcpLabel, NetConfig.SriovOperatorNamespace)
		Expect(err).ToNot(HaveOccurred(), "Sriov and MCP are not stable")

		By("Deploy Test Resources: 2 NADs for bond CNI")
		testNAD1 = defineAndCreateBondNAD(tsparams.MultiNetPolNs1)
		testNAD2 = defineAndCreateBondNAD(tsparams.MultiNetPolNs2)

		By("Deploy Test Resources: 5 Pods")
		testPod1 = defineAndCreatePodWithBondIf(pod1, tsparams.MultiNetPolNs1, ns1+nicPf1, ns1+nicPf2,
			workerNodeList[0].Object.Name, tsparams.TestData)
		testPod2 = defineAndCreatePodWithBondIf(pod2, tsparams.MultiNetPolNs1, ns1+nicPf1, ns1+nicPf2,
			workerNodeList[0].Object.Name, tsparams.TestData)
		testPod3 = defineAndCreatePodWithBondIf(pod3, tsparams.MultiNetPolNs1, ns1+nicPf1, ns1+nicPf2,
			workerNodeList[0].Object.Name, tsparams.TestData)
		testPod4 = defineAndCreatePodWithBondIf(pod4, tsparams.MultiNetPolNs2, ns2+nicPf1, ns2+nicPf2,
			workerNodeList[0].Object.Name, tsparams.TestData)
		testPod5 = defineAndCreatePodWithBondIf(pod5, tsparams.MultiNetPolNs2, ns2+nicPf1, ns2+nicPf2,
			workerNodeList[0].Object.Name, tsparams.TestData)

		By("Check egress traffic from pod1 to other 4 pods. All ports should be open")

		verifyPaths(testPod1, testPod2, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod1, testPod3, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod1, testPod4, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod1, testPod5, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)

		By("Check ingress traffic to pod1 from other 4 pods. All ports should be open")

		verifyPaths(testPod2, testPod1, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod3, testPod1, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod4, testPod1, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod5, testPod1, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
	})

	AfterEach(func() {
		err := tNs1.CleanObjects(1*time.Minute, networkpolicy.GetMultiNetworkGVR())
		Expect(err).ToNot(HaveOccurred(), "failed to clean Multi-NetworkPolicies in test namespace")
	})

	AfterAll(func() {
		By("Cleaning up test pods")
		err := tNs1.CleanObjects(2*time.Minute, pod.GetGVR())
		Expect(err).ToNot(HaveOccurred(), "failed to clean test pods in test namespace")
		err = tNs2.CleanObjects(2*time.Minute, pod.GetGVR())
		Expect(err).ToNot(HaveOccurred(), "failed to clean test pods in test namespace")

		By("Cleaning up test NADs")
		err = testNAD1.Delete()
		Expect(err).ToNot(HaveOccurred(), "failed to clean test NADs in test namespace")
		err = testNAD2.Delete()
		Expect(err).ToNot(HaveOccurred(), "failed to clean test NADs in test namespace")

		By("Removing SRIOV configuration and wait for MCP stable")
		err = netenv.RemoveSriovConfigurationAndWaitForSriovAndMCPStable()
		Expect(err).ToNot(HaveOccurred(), "Failed to remove SRIOV configuration and MCP stable")

		By("Delete test namespace")
		err = tNs1.DeleteAndWait(1 * time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Failed to delete test namespace")
		err = tNs2.DeleteAndWait(1 * time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Failed to delete test namespace")
	})

	It("Egress - block all", reportxml.ID("77169"), func() {

		By("Create Multi Network Policy")
		_, err := networkpolicy.NewMultiNetworkPolicyBuilder(APIClient, "egress-deny", tsparams.MultiNetPolNs1).
			WithNetwork(fmt.Sprintf("%s/bond,%s/bond", tsparams.MultiNetPolNs1, tsparams.MultiNetPolNs2)).
			WithPodSelector(metav1.LabelSelector{MatchLabels: map[string]string{"app": "pod1"}}).
			WithPolicyType(multinetpolicyapiv1.PolicyTypeEgress).
			Create()
		Expect(err).ToNot(HaveOccurred(), "Failed to create Multi Network Policy")

		// Wait for 5 seconds for the multi network policy to be configured.
		time.Sleep(5 * time.Second)

		By("Check egress traffic from pod1 to other 4 pods. All ports should be filtered")

		verifyPaths(testPod1, testPod2, tsparams.AllClose, tsparams.AllClose, tsparams.TestData)
		verifyPaths(testPod1, testPod3, tsparams.AllClose, tsparams.AllClose, tsparams.TestData)
		verifyPaths(testPod1, testPod4, tsparams.AllClose, tsparams.AllClose, tsparams.TestData)
		verifyPaths(testPod1, testPod5, tsparams.AllClose, tsparams.AllClose, tsparams.TestData)

		By("Check ingress traffic to pod1 from other 4 pods. All ports should be open")

		verifyPaths(testPod2, testPod1, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod3, testPod1, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod4, testPod1, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod5, testPod1, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
	})

	It("Egress - allow all", reportxml.ID("77201"), func() {

		By("Create Multi Network Policy")
		testEgressRule, err := networkpolicy.NewEgressRuleBuilder().GetEgressRuleCfg()
		Expect(err).ToNot(HaveOccurred(), "egress rule configuration not generated")

		_, err = networkpolicy.NewMultiNetworkPolicyBuilder(APIClient, "egress-allow", tsparams.MultiNetPolNs1).
			WithNetwork(fmt.Sprintf("%s/bond,%s/bond", tsparams.MultiNetPolNs1, tsparams.MultiNetPolNs2)).
			WithPodSelector(metav1.LabelSelector{MatchLabels: map[string]string{"app": "pod1"}}).
			WithPolicyType(multinetpolicyapiv1.PolicyTypeEgress).
			WithEgressRule(*testEgressRule).
			Create()
		Expect(err).ToNot(HaveOccurred(), "Failed to create Multi Network Policy")

		// Wait for 5 seconds for the multi network policy to be configured.
		time.Sleep(5 * time.Second)

		By("Check egress traffic from pod1 to other 4 pods. All ports should be open")

		verifyPaths(testPod1, testPod2, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod1, testPod3, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod1, testPod4, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod1, testPod5, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)

		By("Check ingress traffic to pod1 from other 4 pods. All ports should be open")

		verifyPaths(testPod2, testPod1, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod3, testPod1, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod4, testPod1, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod5, testPod1, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
	})

	It("Egress - podSelector - NonExistent Label", reportxml.ID("77199"), func() {

		By("Create Multi Network Policy")
		testEgressRule, err := networkpolicy.NewEgressRuleBuilder().
			WithPeerPodSelector(metav1.LabelSelector{MatchLabels: map[string]string{"app": "none"}}).
			GetEgressRuleCfg()
		Expect(err).ToNot(HaveOccurred(), "egress rule configuration not generated")

		_, err = networkpolicy.NewMultiNetworkPolicyBuilder(APIClient, "egress-podsel-nonexist", tsparams.MultiNetPolNs1).
			WithNetwork(fmt.Sprintf("%s/bond,%s/bond", tsparams.MultiNetPolNs1, tsparams.MultiNetPolNs2)).
			WithPodSelector(metav1.LabelSelector{MatchLabels: map[string]string{"app": "pod1"}}).
			WithPolicyType(multinetpolicyapiv1.PolicyTypeEgress).
			WithEgressRule(*testEgressRule).
			Create()
		Expect(err).NotTo(HaveOccurred(), "failed to create multi network policy")

		// Wait for 5 seconds for the multi network policy to be configured.
		time.Sleep(5 * time.Second)

		By("Check egress traffic from pod1 to other 4 pods. All ports should be filtered")

		verifyPaths(testPod1, testPod2, tsparams.AllClose, tsparams.AllClose, tsparams.TestData)
		verifyPaths(testPod1, testPod3, tsparams.AllClose, tsparams.AllClose, tsparams.TestData)
		verifyPaths(testPod1, testPod4, tsparams.AllClose, tsparams.AllClose, tsparams.TestData)
		verifyPaths(testPod1, testPod5, tsparams.AllClose, tsparams.AllClose, tsparams.TestData)

		By("Check ingress traffic to pod1 from other 4 pods. All ports should be open")

		verifyPaths(testPod2, testPod1, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod3, testPod1, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod4, testPod1, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod5, testPod1, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
	})

	It("Egress - namespaceSelector - NonExistent Label", reportxml.ID("77197"), func() {

		By("Create Multi Network Policy")
		testEgressRule, err := networkpolicy.NewEgressRuleBuilder().
			WithPeerNamespaceSelector(metav1.LabelSelector{MatchLabels: map[string]string{"ns": "none"}}).
			GetEgressRuleCfg()
		Expect(err).ToNot(HaveOccurred(), "egress rule configuration not generated")

		_, err = networkpolicy.NewMultiNetworkPolicyBuilder(APIClient, "egress-nssel-nonexist", tsparams.MultiNetPolNs1).
			WithNetwork(fmt.Sprintf("%s/bond,%s/bond", tsparams.MultiNetPolNs1, tsparams.MultiNetPolNs2)).
			WithPodSelector(metav1.LabelSelector{MatchLabels: map[string]string{"app": "pod1"}}).
			WithPolicyType(multinetpolicyapiv1.PolicyTypeEgress).
			WithEgressRule(*testEgressRule).
			Create()
		Expect(err).NotTo(HaveOccurred(), "failed to create multi network policy")

		// Wait for 5 seconds for the multi network policy to be configured.
		time.Sleep(5 * time.Second)

		By("Check egress traffic from pod1 to other 4 pods. All ports should be filtered")

		verifyPaths(testPod1, testPod2, tsparams.AllClose, tsparams.AllClose, tsparams.TestData)
		verifyPaths(testPod1, testPod3, tsparams.AllClose, tsparams.AllClose, tsparams.TestData)
		verifyPaths(testPod1, testPod4, tsparams.AllClose, tsparams.AllClose, tsparams.TestData)
		verifyPaths(testPod1, testPod5, tsparams.AllClose, tsparams.AllClose, tsparams.TestData)

		By("Check ingress traffic to pod1 from other 4 pods. All ports should be open")

		verifyPaths(testPod2, testPod1, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod3, testPod1, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod4, testPod1, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod5, testPod1, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
	})

	It("Egress - Pod and/or Namespace Selector", reportxml.ID("77204"), func() {

		By("Create Multi Network Policy")
		testEgressRule, err := networkpolicy.NewEgressRuleBuilder().
			WithPeerPodAndNamespaceSelector(metav1.LabelSelector{MatchLabels: map[string]string{"app": "pod4"}},
				metav1.LabelSelector{MatchLabels: map[string]string{"ns": "ns2"}}).
			WithPeerPodSelector(metav1.LabelSelector{MatchLabels: map[string]string{"app": "pod2"}}).
			GetEgressRuleCfg()
		Expect(err).ToNot(HaveOccurred(), "egress rule configuration not generated")

		_, err = networkpolicy.NewMultiNetworkPolicyBuilder(APIClient, "egress-pod-ns-selector", tsparams.MultiNetPolNs1).
			WithNetwork(fmt.Sprintf("%s/bond,%s/bond", tsparams.MultiNetPolNs1, tsparams.MultiNetPolNs2)).
			WithPodSelector(metav1.LabelSelector{MatchLabels: map[string]string{"app": "pod1"}}).
			WithPolicyType(multinetpolicyapiv1.PolicyTypeEgress).
			WithEgressRule(*testEgressRule).
			Create()
		Expect(err).NotTo(HaveOccurred(), "failed to create multi network policy")

		// Wait for 5 seconds for the multi network policy to be configured.
		time.Sleep(5 * time.Second)

		By("Check egress traffic from pod1 to other 4 pods. Only pod2 and pod4 should be accessible on all ports")

		verifyPaths(testPod1, testPod2, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod1, testPod3, tsparams.AllClose, tsparams.AllClose, tsparams.TestData)
		verifyPaths(testPod1, testPod4, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod1, testPod5, tsparams.AllClose, tsparams.AllClose, tsparams.TestData)

		By("Check ingress traffic to pod1 from other 4 pods. All ports should be open")

		verifyPaths(testPod2, testPod1, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod3, testPod1, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod4, testPod1, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod5, testPod1, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
	})

	It("Egress - IPBlock IPv4 and IPv6 and Ports", reportxml.ID("77202"), func() {

		By("Create Multi Network Policy")
		testEgressRule, err := networkpolicy.NewEgressRuleBuilder().
			WithPortAndProtocol(5001, "TCP").
			WithCIDR("192.168.10.0/24", []string{"192.168.10.12/32"}).
			WithCIDR("2001:0:0:2::/64", []string{"2001:0:0:2::12/128"}).
			GetEgressRuleCfg()
		Expect(err).ToNot(HaveOccurred(), "egress rule configuration not generated")

		_, err = networkpolicy.NewMultiNetworkPolicyBuilder(APIClient, "egress-ipv4v6-port", tsparams.MultiNetPolNs1).
			WithNetwork(fmt.Sprintf("%s/bond,%s/bond", tsparams.MultiNetPolNs1, tsparams.MultiNetPolNs2)).
			WithPodSelector(metav1.LabelSelector{MatchLabels: map[string]string{"app": "pod1"}}).
			WithPolicyType(multinetpolicyapiv1.PolicyTypeEgress).
			WithEgressRule(*testEgressRule).
			Create()
		Expect(err).NotTo(HaveOccurred(), "failed to create multi network policy")

		// Wait for 5 seconds for the multi network policy to be configured.
		time.Sleep(5 * time.Second)

		By("Check egress traffic from pod1 to other 4 pods. " +
			"Pod2 tcp port 5001 should be accessible over IPv4." +
			"Pod4 tcp port 5001 should be accessible over IPv6")

		verifyPaths(testPod1, testPod2, tsparams.P5001Open, tsparams.AllClose, tsparams.TestData)
		verifyPaths(testPod1, testPod3, tsparams.AllClose, tsparams.AllClose, tsparams.TestData)
		verifyPaths(testPod1, testPod4, tsparams.AllClose, tsparams.P5001Open, tsparams.TestData)
		verifyPaths(testPod1, testPod5, tsparams.AllClose, tsparams.AllClose, tsparams.TestData)

		By("Check ingress traffic to pod1 from other 4 pods. All ports should be open")

		verifyPaths(testPod2, testPod1, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod3, testPod1, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod4, testPod1, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod5, testPod1, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
	})

	It("Ingress - block all", reportxml.ID("77237"), func() {

		By("Create Multi Network Policy")
		_, err := networkpolicy.NewMultiNetworkPolicyBuilder(APIClient, "ingress-deny", tsparams.MultiNetPolNs1).
			WithNetwork(fmt.Sprintf("%s/bond,%s/bond", tsparams.MultiNetPolNs1, tsparams.MultiNetPolNs2)).
			WithPodSelector(metav1.LabelSelector{MatchLabels: map[string]string{"app": "pod1"}}).
			WithPolicyType(multinetpolicyapiv1.PolicyTypeIngress).
			Create()
		Expect(err).ToNot(HaveOccurred(), "Failed to create Multi Network Policy")

		// Wait for 5 seconds for the multi network policy to be configured.
		time.Sleep(5 * time.Second)

		By("Check egress traffic from pod1 to other 4 pods. All ports should be open")

		verifyPaths(testPod1, testPod2, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod1, testPod3, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod1, testPod4, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod1, testPod5, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)

		By("Check ingress traffic to pod1 from other 4 pods. All ports should be filtered")

		verifyPaths(testPod2, testPod1, tsparams.AllClose, tsparams.AllClose, tsparams.TestData)
		verifyPaths(testPod3, testPod1, tsparams.AllClose, tsparams.AllClose, tsparams.TestData)
		verifyPaths(testPod4, testPod1, tsparams.AllClose, tsparams.AllClose, tsparams.TestData)
		verifyPaths(testPod5, testPod1, tsparams.AllClose, tsparams.AllClose, tsparams.TestData)
	})

	It("Ingress - allow all", reportxml.ID("77236"), func() {

		By("Create Multi Network Policy")
		testIngressRule, err := networkpolicy.NewIngressRuleBuilder().GetIngressRuleCfg()
		Expect(err).ToNot(HaveOccurred(), "ingress rule configuration not generated")

		_, err = networkpolicy.NewMultiNetworkPolicyBuilder(APIClient, "ingress-allow", tsparams.MultiNetPolNs1).
			WithNetwork(fmt.Sprintf("%s/bond,%s/bond", tsparams.MultiNetPolNs1, tsparams.MultiNetPolNs2)).
			WithPodSelector(metav1.LabelSelector{MatchLabels: map[string]string{"app": "pod1"}}).
			WithPolicyType(multinetpolicyapiv1.PolicyTypeIngress).
			WithIngressRule(*testIngressRule).
			Create()
		Expect(err).ToNot(HaveOccurred(), "Failed to create Multi Network Policy")

		// Wait for 5 seconds for the multi network policy to be configured.
		time.Sleep(5 * time.Second)

		By("Check egress traffic from pod1 to other 4 pods. All ports should be open")

		verifyPaths(testPod1, testPod2, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod1, testPod3, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod1, testPod4, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod1, testPod5, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)

		By("Check ingress traffic to pod1 from other 4 pods. All ports should be open")

		verifyPaths(testPod2, testPod1, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod3, testPod1, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod4, testPod1, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod5, testPod1, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
	})

	It("Ingress - podSelector - NonExistent Label", reportxml.ID("77233"), func() {

		By("Create Multi Network Policy")
		testIngressRule, err := networkpolicy.NewIngressRuleBuilder().
			WithPeerPodSelector(metav1.LabelSelector{MatchLabels: map[string]string{"app": "none"}}).
			GetIngressRuleCfg()
		Expect(err).ToNot(HaveOccurred(), "ingress rule configuration not generated")

		_, err = networkpolicy.NewMultiNetworkPolicyBuilder(APIClient, "ingress-podsel-nonexist", tsparams.MultiNetPolNs1).
			WithNetwork(fmt.Sprintf("%s/bond,%s/bond", tsparams.MultiNetPolNs1, tsparams.MultiNetPolNs2)).
			WithPodSelector(metav1.LabelSelector{MatchLabels: map[string]string{"app": "pod1"}}).
			WithPolicyType(multinetpolicyapiv1.PolicyTypeIngress).
			WithIngressRule(*testIngressRule).
			Create()
		Expect(err).NotTo(HaveOccurred(), "failed to create multi network policy")

		// Wait for 5 seconds for the multi network policy to be configured.
		time.Sleep(5 * time.Second)

		By("Check egress traffic from pod1 to other 4 pods. All ports should be open")

		verifyPaths(testPod1, testPod2, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod1, testPod3, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod1, testPod4, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod1, testPod5, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)

		By("Check ingress traffic to pod1 from other 4 pods. All ports should be filtered")

		verifyPaths(testPod2, testPod1, tsparams.AllClose, tsparams.AllClose, tsparams.TestData)
		verifyPaths(testPod3, testPod1, tsparams.AllClose, tsparams.AllClose, tsparams.TestData)
		verifyPaths(testPod4, testPod1, tsparams.AllClose, tsparams.AllClose, tsparams.TestData)
		verifyPaths(testPod5, testPod1, tsparams.AllClose, tsparams.AllClose, tsparams.TestData)
	})

	It("Ingress - namespaceSelector - NonExistent Label", reportxml.ID("77235"), func() {

		By("Create Multi Network Policy")
		testIngressRule, err := networkpolicy.NewIngressRuleBuilder().
			WithPeerNamespaceSelector(metav1.LabelSelector{MatchLabels: map[string]string{"ns": "none"}}).
			GetIngressRuleCfg()
		Expect(err).ToNot(HaveOccurred(), "ingress rule configuration not generated")

		_, err = networkpolicy.NewMultiNetworkPolicyBuilder(APIClient, "ingress-nssel-nonexist", tsparams.MultiNetPolNs1).
			WithNetwork(fmt.Sprintf("%s/bond,%s/bond", tsparams.MultiNetPolNs1, tsparams.MultiNetPolNs2)).
			WithPodSelector(metav1.LabelSelector{MatchLabels: map[string]string{"app": "pod1"}}).
			WithPolicyType(multinetpolicyapiv1.PolicyTypeIngress).
			WithIngressRule(*testIngressRule).
			Create()
		Expect(err).NotTo(HaveOccurred(), "failed to create multi network policy")

		// Wait for 5 seconds for the multi network policy to be configured.
		time.Sleep(5 * time.Second)

		By("Check egress traffic from pod1 to other 4 pods. All ports should be open")

		verifyPaths(testPod1, testPod2, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod1, testPod3, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod1, testPod4, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod1, testPod5, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)

		By("Check ingress traffic to pod1 from other 4 pods. All ports should be filtered")

		verifyPaths(testPod2, testPod1, tsparams.AllClose, tsparams.AllClose, tsparams.TestData)
		verifyPaths(testPod3, testPod1, tsparams.AllClose, tsparams.AllClose, tsparams.TestData)
		verifyPaths(testPod4, testPod1, tsparams.AllClose, tsparams.AllClose, tsparams.TestData)
		verifyPaths(testPod5, testPod1, tsparams.AllClose, tsparams.AllClose, tsparams.TestData)
	})

	It("Ingress - Pod and/or Namespace Selector", reportxml.ID("77242"), func() {

		By("Create Multi Network Policy")
		testIngressRule, err := networkpolicy.NewIngressRuleBuilder().
			WithPeerPodAndNamespaceSelector(metav1.LabelSelector{MatchLabels: map[string]string{"app": "pod4"}},
				metav1.LabelSelector{MatchLabels: map[string]string{"ns": "ns2"}}).
			WithPeerPodSelector(metav1.LabelSelector{MatchLabels: map[string]string{"app": "pod2"}}).
			GetIngressRuleCfg()
		Expect(err).ToNot(HaveOccurred(), "ingress rule configuration not generated")

		_, err = networkpolicy.NewMultiNetworkPolicyBuilder(APIClient, "ingress-pod-ns-selector", tsparams.MultiNetPolNs1).
			WithNetwork(fmt.Sprintf("%s/bond,%s/bond", tsparams.MultiNetPolNs1, tsparams.MultiNetPolNs2)).
			WithPodSelector(metav1.LabelSelector{MatchLabels: map[string]string{"app": "pod1"}}).
			WithPolicyType(multinetpolicyapiv1.PolicyTypeIngress).
			WithIngressRule(*testIngressRule).
			Create()
		Expect(err).NotTo(HaveOccurred(), "failed to create multi network policy")

		// Wait for 5 seconds for the multi network policy to be configured.
		time.Sleep(5 * time.Second)

		By("Check egress traffic from pod1 to other 4 pods. All ports should be open")

		verifyPaths(testPod1, testPod2, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod1, testPod3, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod1, testPod4, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod1, testPod5, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)

		By("Check ingress traffic to pod1 from other 4 pods. Only pod2 and pod4 can access pod1 on all ports")

		verifyPaths(testPod2, testPod1, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod3, testPod1, tsparams.AllClose, tsparams.AllClose, tsparams.TestData)
		verifyPaths(testPod4, testPod1, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod5, testPod1, tsparams.AllClose, tsparams.AllClose, tsparams.TestData)
	})

	It("Ingress - IPBlock IPv4 and IPv6 and Ports", reportxml.ID("77238"), func() {

		By("Create Multi Network Policy")
		testIngressRule, err := networkpolicy.NewIngressRuleBuilder().
			WithPortAndProtocol(5001, "TCP").
			WithCIDR("192.168.10.0/24", []string{"192.168.10.12/32"}).
			WithCIDR("2001:0:0:2::/64", []string{"2001:0:0:2::12/128"}).
			GetIngressRuleCfg()
		Expect(err).ToNot(HaveOccurred(), "ingress rule configuration not generated")

		_, err = networkpolicy.NewMultiNetworkPolicyBuilder(APIClient, "ingress-ipv4v6-port", tsparams.MultiNetPolNs1).
			WithNetwork(fmt.Sprintf("%s/bond,%s/bond", tsparams.MultiNetPolNs1, tsparams.MultiNetPolNs2)).
			WithPodSelector(metav1.LabelSelector{MatchLabels: map[string]string{"app": "pod1"}}).
			WithPolicyType(multinetpolicyapiv1.PolicyTypeIngress).
			WithIngressRule(*testIngressRule).
			Create()
		Expect(err).NotTo(HaveOccurred(), "failed to create multi network policy")

		// Wait for 5 seconds for the multi network policy to be configured.
		time.Sleep(5 * time.Second)

		By("Check egress traffic from pod1 to other 4 pods. All ports should be open")

		verifyPaths(testPod1, testPod2, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod1, testPod3, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod1, testPod4, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod1, testPod5, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)

		By("Check ingress traffic to pod1 from other 4 pods. " +
			"Pod2 can access tcp port 5001 of pod1 over IPv4." +
			"Pod4 can access tcp port 5001 of pod4 over IPv6")

		verifyPaths(testPod2, testPod1, tsparams.P5001Open, tsparams.AllClose, tsparams.TestData)
		verifyPaths(testPod3, testPod1, tsparams.AllClose, tsparams.AllClose, tsparams.TestData)
		verifyPaths(testPod4, testPod1, tsparams.AllClose, tsparams.P5001Open, tsparams.TestData)
		verifyPaths(testPod5, testPod1, tsparams.AllClose, tsparams.AllClose, tsparams.TestData)
	})

	It("Ingress & Egress - Peer and Ports", reportxml.ID("77469"), func() {

		By("Create Multi Network Policy")
		testEgressRule, err := networkpolicy.NewEgressRuleBuilder().
			WithPeerPodSelector(metav1.LabelSelector{MatchLabels: map[string]string{"app": "pod2"}}).
			WithCIDR("2001:0:0:2::/64", []string{"2001:0:0:2::11/128"}).
			GetEgressRuleCfg()
		Expect(err).ToNot(HaveOccurred(), "egress rule configuration not generated")

		testIngressRule, err := networkpolicy.NewIngressRuleBuilder().
			WithCIDR("192.168.10.0/24", []string{"192.168.10.12/32"}).
			WithPeerPodAndNamespaceSelector(metav1.LabelSelector{MatchLabels: map[string]string{"app": "pod4"}},
				metav1.LabelSelector{MatchLabels: map[string]string{"ns": "ns2"}}).
			WithProtocol("TCP").
			GetIngressRuleCfg()
		Expect(err).ToNot(HaveOccurred(), "ingress rule configuration not generated")

		_, err = networkpolicy.NewMultiNetworkPolicyBuilder(APIClient, "ingress-egress", "policy-ns1").
			WithNetwork(fmt.Sprintf("%s/bond,%s/bond", tsparams.MultiNetPolNs1, tsparams.MultiNetPolNs2)).
			WithPodSelector(metav1.LabelSelector{MatchLabels: map[string]string{"app": "pod1"}}).
			WithPolicyType(multinetpolicyapiv1.PolicyTypeIngress).
			WithPolicyType(multinetpolicyapiv1.PolicyTypeEgress).
			WithIngressRule(*testIngressRule).
			WithEgressRule(*testEgressRule).
			Create()
		Expect(err).NotTo(HaveOccurred(), "failed to create multi network policy")

		// Wait for 5 seconds for the multi network policy to be configured.
		time.Sleep(5 * time.Second)

		By("Check egress traffic from pod1 to other 4 pods. Only Pod5 ports should be accessible over IPv6. " +
			"Pod2 should be accessible over IPv6 and IPv4")

		verifyPaths(testPod1, testPod2, tsparams.AllOpen, tsparams.AllOpen, tsparams.TestData)
		verifyPaths(testPod1, testPod3, tsparams.AllClose, tsparams.AllClose, tsparams.TestData)
		verifyPaths(testPod1, testPod4, tsparams.AllClose, tsparams.AllClose, tsparams.TestData)
		verifyPaths(testPod1, testPod5, tsparams.AllClose, tsparams.AllOpen, tsparams.TestData)

		By("Check ingress traffic to pod1 from other 4 pods. " +
			"Pod2 can access tcp ports 5001 & 5002 of pod1 over IPv4. " +
			"Pod4 can access tcp ports 5001 & 5002 of pod1 over both IPv4 & IPv6")

		verifyPaths(testPod2, testPod1, tsparams.P5001p5002Open, tsparams.AllClose, tsparams.TestData)
		verifyPaths(testPod3, testPod1, tsparams.AllClose, tsparams.AllClose, tsparams.TestData)
		verifyPaths(testPod4, testPod1, tsparams.P5001p5002Open, tsparams.P5001p5002Open, tsparams.TestData)
		verifyPaths(testPod5, testPod1, tsparams.AllClose, tsparams.AllClose, tsparams.TestData)
	})
})

func defineAndCreateSriovNetwork(netName, resName, targetNs string) {
	_, err := sriov.NewNetworkBuilder(
		APIClient, netName, NetConfig.SriovOperatorNamespace, targetNs, resName).
		WithLogLevel(netparam.LogLevelDebug).
		Create()
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to create Sriov Network %s", netName))
}

func defineAndCreateBondNAD(nsName string) *nad.Builder {
	config, err := nad.NewMasterBondPlugin("bond", "active-backup").
		WithFailOverMac(1).
		WithLinksInContainer(true).
		WithMiimon(100).
		WithLinks([]nad.Link{{Name: "net1"}, {Name: "net2"}}).
		WithCapabilities(&nad.Capability{IPs: true}).
		WithIPAM(&nad.IPAM{
			Type:   "static",
			Routes: []nad.Routes{{Dst: "192.168.0.0/16"}, {Dst: "2001::0/62"}},
		}).GetMasterPluginConfig()
	Expect(err).ToNot(HaveOccurred(), "Failed to get master bond plugin config")

	createdNAD, err := nad.NewBuilder(APIClient, "bond", nsName).WithMasterPlugin(config).Create()
	Expect(err).ToNot(HaveOccurred(), "Failed to create net-attach-def")

	return createdNAD
}

func defineAndCreatePodWithBondIf(
	podName, nsName, net1Name, net2Name, nodeName string, testData tsparams.PodsData) *pod.Builder {
	var rootUser int64

	securityContext := corev1.SecurityContext{
		RunAsUser: &rootUser,
		Capabilities: &corev1.Capabilities{
			Add: []corev1.Capability{"IPC_LOCK", "SYS_RESOURCE", "NET_RAW", "NET_ADMIN"},
		},
	}

	netAnnotation := []*types.NetworkSelectionElement{
		{
			Name:             net1Name,
			InterfaceRequest: "net1",
		},
		{
			Name:             net2Name,
			InterfaceRequest: "net2",
		},
		{
			Name:             "bond",
			InterfaceRequest: "bond1",
			IPRequest:        []string{testData[podName].IPv4, testData[podName].IPv6},
		},
	}

	tPodBuilder := pod.NewBuilder(APIClient, podName, nsName, NetConfig.CnfNetTestContainer).
		WithNodeSelector(map[string]string{"kubernetes.io/hostname": nodeName}).
		WithSecondaryNetwork(netAnnotation).
		WithPrivilegedFlag().
		WithLabel("app", podName)

	for index := range len(testData[podName].Protocols) {
		containerBuilder, err := pod.NewContainerBuilder(testData[podName].Protocols[index]+testData[podName].Ports[index],
			NetConfig.CnfNetTestContainer,
			[]string{"/bin/bash", "-c", fmt.Sprintf("testcmd -listen -interface bond1 -protocol %s -port %s",
				testData[podName].Protocols[index], testData[podName].Ports[index])}).
			WithSecurityContext(&securityContext).
			GetContainerCfg()
		Expect(err).ToNot(HaveOccurred(), "Failed to get container config")

		if index == 0 {
			tPodBuilder.RedefineDefaultContainer(*containerBuilder)
		} else {
			tPodBuilder.WithAdditionalContainer(containerBuilder)
		}
	}

	tPod, err := tPodBuilder.CreateAndWaitUntilRunning(1 * time.Minute)
	Expect(err).ToNot(HaveOccurred(), "Failed to create test pod")

	return tPod
}
