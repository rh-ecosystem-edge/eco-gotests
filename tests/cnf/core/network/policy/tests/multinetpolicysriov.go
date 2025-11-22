package tests

import (
	"fmt"
	"strings"
	"time"

	multinetpolicyapiv1 "github.com/k8snetworkplumbingwg/multi-networkpolicy/pkg/apis/k8s.cni.cncf.io/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/daemonset"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/networkpolicy"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/ipaddr"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netenv"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/policy/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

var (
	tcpProtocol            = "tcp"
	sctpProtocol           = "sctp"
	serverPodIP            = "192.168.0.1/24"
	serverPodIPv6          = "2001:1db8:85a3::1/64"
	firstClientPodIP       = "192.168.0.2/24"
	firstClientPodIPv6     = "2001:1db8:85a3::2/64"
	secondClientPodIP      = "192.168.0.3/24"
	secondClientPodIPv6    = "2001:1db8:85a3::3/64"
	labelServerPod         = "pod1"
	labelFirstClientPod    = "pod2"
	labelSecondClientPod   = "pod3"
	multiNetworkPolicyName = "verificationpolicy"
	port5001               = 5001
	port5003               = 5003
)

var _ = Describe("SRIOV", Ordered, Label("multinetworkpolicy"), ContinueOnFailure, func() {

	var (
		workerNodeList                             []*nodes.Builder
		serverPod, firstClientPod, secondClientPod *pod.Builder
		srIovNet                                   *sriov.NetworkBuilder
	)

	BeforeAll(func() {
		By("Enabling MultiNetworkPolicy feature")
		enableMultiNetworkPolicy(true)

		By("Listing worker nodes")
		var err error
		workerNodeList, err = nodes.List(
			APIClient, metav1.ListOptions{LabelSelector: labels.Set(NetConfig.WorkerLabelMap).String()})
		Expect(err).ToNot(HaveOccurred(), "Failed to list worker nodes")
		Expect(len(workerNodeList)).To(BeNumerically(">", 1),
			"Failed cluster doesn't have enough nodes")

		By("Collecting SR-IOV interface for multinetwork policy tests")
		srIovInterfacesUnderTest, err := NetConfig.GetSriovInterfaces(1)
		Expect(err).ToNot(HaveOccurred(), "Failed to retrieve SR-IOV interfaces for testing")

		By("Configuring SR-IOV policy")
		sriovPolicy, err := sriov.NewPolicyBuilder(
			APIClient,
			"policysriov",
			NetConfig.SriovOperatorNamespace,
			"sriovpolicy",
			6,
			srIovInterfacesUnderTest,
			NetConfig.WorkerLabelMap).Create()
		Expect(err).ToNot(HaveOccurred(), "Failed to create SR-IOV policy")

		By("Creating sr-iov network with ipam static config")
		srIovNet, err = sriov.NewNetworkBuilder(
			APIClient,
			"sriovnetpolicy",
			NetConfig.SriovOperatorNamespace,
			tsparams.TestNamespaceName,
			sriovPolicy.Definition.Spec.ResourceName).
			WithStaticIpam().WithLogLevel(netparam.LogLevelDebug).Create()
		Expect(err).ToNot(HaveOccurred(), "Failed to configure sr-iov network")

		By("Waiting until cluster MCP and SR-IOV are stable")
		err = netenv.WaitForSriovAndMCPStable(
			APIClient, tsparams.MCOWaitTimeout, time.Minute, NetConfig.CnfMcpLabel, NetConfig.SriovOperatorNamespace)
		Expect(err).ToNot(HaveOccurred(), "Failed cluster is not stable")
	})
	Context("ipv4", func() {
		BeforeEach(func() {
			By("Creating first client pod")
			firstClientPod = createClientPod(
				"client1",
				srIovNet.Definition.Name,
				workerNodeList[1].Definition.Name,
				tcpProtocol,
				labelFirstClientPod,
				[]string{firstClientPodIP})

			By("Creating second client pod")
			secondClientPod = createClientPod(
				"client2",
				srIovNet.Definition.Name,
				workerNodeList[1].Definition.Name,
				tcpProtocol,
				labelSecondClientPod,
				[]string{secondClientPodIP})

			By("Creating server pod")
			serverPod = createServerPod(
				srIovNet.Definition.Name,
				workerNodeList[0].Definition.Name,
				tcpProtocol,
				[]string{serverPodIP},
				[]string{firstClientPodIP},
				[]string{secondClientPodIP})
		})

		It("Ingress Default rule without PolicyType deny all", reportxml.ID("53901"), func() {
			_, err := networkpolicy.NewMultiNetworkPolicyBuilder(
				APIClient, multiNetworkPolicyName, tsparams.TestNamespaceName).
				WithNetwork(srIovNet.Definition.Name).
				WithEmptyIngress().
				WithPodSelector(metav1.LabelSelector{MatchLabels: map[string]string{"pod": labelServerPod}}).
				Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create multiNetworkPolicy")

			By("Traffic verification")
			// All traffic should be blocked to the serverPod
			Eventually(func() error {
				return runTraffic(firstClientPod, removePrefixFromIP(serverPodIP), tcpProtocol, port5001)
			}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).Should(HaveOccurred(),
				fmt.Sprintf("Unexpectedly pod %s can reach %s with port %d",
					firstClientPod.Definition.Name, serverPod.Definition.Name, port5001))

			Eventually(func() error {
				return runTraffic(secondClientPod, removePrefixFromIP(serverPodIP), tcpProtocol, port5003)
			}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).Should(HaveOccurred(),
				fmt.Sprintf("Unexpectedly pod %s can reach %s with port %d",
					secondClientPod.Definition.Name, serverPod.Definition.Name, port5003))

			// Traffic between firstClientPod and secondClientPod should not be affected (not blocked)
			Eventually(func() error {
				return runTraffic(secondClientPod, removePrefixFromIP(firstClientPodIP), tcpProtocol, port5001)
			}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).ShouldNot(HaveOccurred(),
				fmt.Sprintf("Pod %s can NOT reach %s with port %d",
					secondClientPod.Definition.Name, firstClientPod.Definition.Name, port5001))
		})

		It("Ingress Default rule without PolicyType allow all", reportxml.ID("53899"), func() {
			By("Apply MultiNetworkPolicy with ingress rule allow all without PolicyType field")

			_, err := networkpolicy.NewMultiNetworkPolicyBuilder(
				APIClient, multiNetworkPolicyName, tsparams.TestNamespaceName).
				WithNetwork(srIovNet.Definition.Name).
				WithIngressRule(multinetpolicyapiv1.MultiNetworkPolicyIngressRule{}).
				WithPodSelector(metav1.LabelSelector{MatchLabels: map[string]string{}}).Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create multiNetworkPolicy")

			// All traffic is accepted
			By("Traffic verification")
			Eventually(func() error {
				return runTraffic(firstClientPod, removePrefixFromIP(serverPodIP), tcpProtocol, port5001)
			}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).ShouldNot(HaveOccurred(),
				fmt.Sprintf("Unexpectedly pod %s can NOT reach %s with port %d",
					firstClientPod.Definition.Name, serverPod.Definition.Name, port5001))

			Eventually(func() error {
				return runTraffic(secondClientPod, removePrefixFromIP(serverPodIP), tcpProtocol, port5001)
			}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).ShouldNot(HaveOccurred(),
				fmt.Sprintf("Unexpectedly pod %s can NOT reach %s with port %d",
					secondClientPod.Definition.Name, serverPod.Definition.Name, port5001))

			// Traffic between firstClientPod and secondClientPod should not be affected (not blocked)
			Eventually(func() error {
				return runTraffic(firstClientPod, removePrefixFromIP(secondClientPodIP), tcpProtocol, port5001)
			}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).ShouldNot(HaveOccurred(),
				fmt.Sprintf("pod %s can NOT reach %s with port %d",
					secondClientPod.Definition.Name, firstClientPod.Definition.Name, port5001))
		})

		It("Egress TCP endPort allow specific pod", reportxml.ID("53900"), func() {
			By("Apply MultiNetworkPolicy with egress rule allow ports in range 5000-5002")

			egressRule, err := networkpolicy.NewEgressRuleBuilder().WithPortAndProtocol(uint16(port5001), "TCP").
				WithPeerPodSelector(metav1.LabelSelector{MatchLabels: map[string]string{"pod": labelServerPod}}).
				GetEgressRuleCfg()
			Expect(err).ToNot(HaveOccurred(), "Failed to build egress rule")

			_, err = networkpolicy.NewMultiNetworkPolicyBuilder(
				APIClient, multiNetworkPolicyName, tsparams.TestNamespaceName).
				WithNetwork(srIovNet.Definition.Name).
				WithPodSelector(metav1.LabelSelector{MatchLabels: map[string]string{"pod": labelFirstClientPod}}).
				WithPolicyType(multinetpolicyapiv1.PolicyTypeEgress).
				WithEgressRule(*egressRule).Create()

			Expect(err).ToNot(HaveOccurred(), "Failed to create multiNetworkPolicy")

			By("Traffic verification")
			// Traffic from firstClientPod to serverPod with port range 5000-5002 should pass.
			Eventually(func() error {
				return runTraffic(firstClientPod, ipaddr.RemovePrefix(serverPodIP), tcpProtocol, port5001)
			}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).ShouldNot(HaveOccurred(),
				fmt.Sprintf("pod %s can NOT reach %s with port %d",
					firstClientPod.Definition.Name, serverPod.Definition.Name, port5001))

			// Port 5003 is out of the accepted port range. Traffic should be dropped.
			Eventually(func() error {
				return runTraffic(firstClientPod, ipaddr.RemovePrefix(serverPodIP), tcpProtocol, port5003)
			}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).Should(HaveOccurred(),
				fmt.Sprintf("unexpectedly pod %s can reach %s with port %d",
					firstClientPod.Definition.Name, serverPod.Definition.Name, port5003))

			// Traffic between firstClientPod and secondClientPod is not allowed
			Eventually(func() error {
				return runTraffic(firstClientPod, ipaddr.RemovePrefix(secondClientPodIP), tcpProtocol, port5001)
			}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).Should(HaveOccurred(),
				fmt.Sprintf("unexpectedly pod %s can reach %s with port %d",
					firstClientPod.Definition.Name, secondClientPod.Definition.Name, port5001))

			// Traffic between secondClientPod and serverPod is not affected by rule.
			Eventually(func() error {
				return runTraffic(secondClientPod, ipaddr.RemovePrefix(serverPodIP), tcpProtocol, port5001)
			}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).ShouldNot(HaveOccurred(),
				fmt.Sprintf("pod %s can NOT reach %s with port %d",
					secondClientPod.Definition.Name, serverPod.Definition.Name, port5001))

			Eventually(func() error {
				return runTraffic(secondClientPod, ipaddr.RemovePrefix(serverPodIP), tcpProtocol, port5003)
			}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).ShouldNot(HaveOccurred(),
				fmt.Sprintf("pod %s can NOT reach %s with port %d",
					secondClientPod.Definition.Name, serverPod.Definition.Name, port5003))
		})

		It("Ingress and Egress allow IPv4 address", reportxml.ID("53898"), func() {
			By("Apply MultiNetworkPolicy with ingress and egress rules allow specific IPv4 addresses")
			egressRule, err := networkpolicy.NewEgressRuleBuilder().WithPeerPodSelectorAndCIDR(
				metav1.LabelSelector{MatchLabels: map[string]string{"pod": labelSecondClientPod}},
				ipaddr.RemovePrefix(secondClientPodIP)+"/"+"32").
				GetEgressRuleCfg()
			Expect(err).ToNot(HaveOccurred(), "Failed to build egress rule")

			ingressRule, err := networkpolicy.NewIngressRuleBuilder().
				WithCIDR(ipaddr.RemovePrefix(firstClientPodIP) + "/" + "32").
				GetIngressRuleCfg()
			Expect(err).ToNot(HaveOccurred(), "Failed to build ingress rule")

			multiNetPolicy := networkpolicy.NewMultiNetworkPolicyBuilder(
				APIClient, multiNetworkPolicyName, tsparams.TestNamespaceName).
				WithNetwork(srIovNet.Definition.Name).
				WithPodSelector(metav1.LabelSelector{MatchLabels: map[string]string{"pod": labelServerPod}}).
				WithPolicyType(multinetpolicyapiv1.PolicyTypeEgress).
				WithPolicyType(multinetpolicyapiv1.PolicyTypeIngress).
				WithIngressRule(*ingressRule).WithEgressRule(*egressRule)

			_, err = multiNetPolicy.Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create multiNetworkPolicy")

			By("Traffic verification")
			// Traffic from firstClientPod to serverPod with source IP netpolicyparameters.Pod2IPAddress should pass
			Eventually(func() error {
				return runTraffic(firstClientPod, ipaddr.RemovePrefix(serverPodIP), tcpProtocol, port5001)
			}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).ShouldNot(HaveOccurred(),
				fmt.Sprintf("pod %s can NOT reach %s with port %d",
					firstClientPod.Definition.Name, serverPod.Definition.Name, port5001))

			// Traffic from serverPod to secondClientPod with destination IP netpolicyparameters.Pod3IPAddress should pass
			Eventually(func() error {
				return runTraffic(serverPod, ipaddr.RemovePrefix(secondClientPodIP), tcpProtocol, port5001)
			}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).ShouldNot(HaveOccurred(),
				fmt.Sprintf("pod %s can NOT reach %s with port %d",
					serverPod.Definition.Name, secondClientPod.Definition.Name, port5001))

			// All other traffic should be dropped
			Eventually(func() error {
				return runTraffic(secondClientPod, ipaddr.RemovePrefix(serverPodIP), tcpProtocol, port5001)
			}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).Should(HaveOccurred(),
				fmt.Sprintf("unexpectedly pod %s can reach %s with port %d",
					secondClientPod.Definition.Name, serverPod.Definition.Name, port5001))

			Eventually(func() error {
				return runTraffic(serverPod, ipaddr.RemovePrefix(firstClientPodIP), tcpProtocol, port5001)
			}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).Should(HaveOccurred(),
				fmt.Sprintf("unexpectedly pod %s can reach %s with port %d",
					serverPod.Definition.Name, firstClientPod.Definition.Name, port5001))
		})

		// 55990
		It("Disable multi-network policy", reportxml.ID("55990"), func() {
			By("Apply MultiNetworkPolicy with ingress rule deny all")
			_, err := networkpolicy.NewMultiNetworkPolicyBuilder(
				APIClient, multiNetworkPolicyName, tsparams.TestNamespaceName).
				WithNetwork(srIovNet.Definition.Name).WithPolicyType(multinetpolicyapiv1.PolicyTypeIngress).
				WithEmptyIngress().
				WithPodSelector(metav1.LabelSelector{MatchLabels: map[string]string{"pod": labelServerPod}}).
				Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create multiNetworkPolicy")

			By("Traffic verification")
			// All traffic should be blocked to the serverPod
			Eventually(func() error {
				return runTraffic(firstClientPod, ipaddr.RemovePrefix(serverPodIP), tcpProtocol, port5001)
			}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).Should(HaveOccurred(),
				fmt.Sprintf("Unexpectedly pod %s can reach %s with port %d",
					firstClientPod.Definition.Name, serverPod.Definition.Name, port5001))

			Eventually(func() error {
				return runTraffic(secondClientPod, ipaddr.RemovePrefix(serverPodIP), tcpProtocol, port5003)
			}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).Should(HaveOccurred(),
				fmt.Sprintf("Unexpectedly pod %s can reach %s with port %d",
					secondClientPod.Definition.Name, serverPod.Definition.Name, port5003))

			// Traffic between firstClientPod and secondClientPod should not be affected (not blocked)
			Eventually(func() error {
				return runTraffic(secondClientPod, ipaddr.RemovePrefix(firstClientPodIP), tcpProtocol, port5001)
			}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).ShouldNot(HaveOccurred(),
				fmt.Sprintf("Pod %s can NOT reach %s with port %d",
					secondClientPod.Definition.Name, firstClientPod.Definition.Name, port5001))

			By("Disable MultiNetworkPolicy feature")
			enableMultiNetworkPolicy(false)

			By("Traffic verification with MultiNetworkPolicy disabled")
			// All traffic is accepted and there is no any policy because feature is off
			Eventually(func() error {
				return runTraffic(firstClientPod, ipaddr.RemovePrefix(serverPodIP), tcpProtocol, port5001)
			}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).ShouldNot(HaveOccurred(),
				fmt.Sprintf("pod %s can NOT reach %s with port %d",
					firstClientPod.Definition.Name, serverPod.Definition.Name, port5001))

			Eventually(func() error {
				return runTraffic(secondClientPod, ipaddr.RemovePrefix(serverPodIP), tcpProtocol, port5003)
			}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).ShouldNot(HaveOccurred(),
				fmt.Sprintf("pod %s can NOT reach %s with port %d",
					secondClientPod.Definition.Name, serverPod.Definition.Name, port5003))

			By("Applying MultiNetworkPolicy should fail")
			_, err = networkpolicy.NewMultiNetworkPolicyBuilder(
				APIClient, multiNetworkPolicyName, tsparams.TestNamespaceName).
				WithNetwork(srIovNet.Definition.Name).WithPolicyType(multinetpolicyapiv1.PolicyTypeIngress).Create()
			Expect(err).To(HaveOccurred(), "Failed. Policy created with disabled multinetworkpolicy")

			By("Enable MultiNetworkPolicy feature")
			enableMultiNetworkPolicy(true)
		})
	})

	Context("ipv6 SCTP", func() {
		BeforeEach(func() {
			By("Creating first client pod")
			firstClientPod = createClientPod(
				"client1",
				srIovNet.Definition.Name,
				workerNodeList[1].Definition.Name,
				sctpProtocol,
				labelFirstClientPod,
				[]string{firstClientPodIPv6})

			By("Creating second client pod")
			secondClientPod = createClientPod(
				"client2",
				srIovNet.Definition.Name,
				workerNodeList[1].Definition.Name,
				sctpProtocol,
				labelSecondClientPod,
				[]string{secondClientPodIPv6})

			By("Creating server pod")
			serverPod = createServerPod(
				srIovNet.Definition.Name,
				workerNodeList[0].Definition.Name,
				sctpProtocol,
				[]string{serverPodIPv6},
				[]string{firstClientPodIPv6},
				[]string{secondClientPodIPv6})

			Consistently(func() bool {
				return firstClientPod.WaitUntilRunning(10*time.Second) == nil &&
					secondClientPod.WaitUntilRunning(10*time.Second) == nil &&
					serverPod.WaitUntilRunning(10*time.Second) == nil
			}, 1*time.Minute, 3*time.Second).Should(BeTrue(), "Failed not all pods are in running state")

			// Connectivity works without policy
			By("Testing connectivity without multiNetworkPolicy applied")
			Eventually(func() error {
				return runTraffic(firstClientPod, removePrefixFromIP(serverPodIPv6), sctpProtocol, port5001)
			}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).ShouldNot(HaveOccurred(),
				fmt.Sprintf("pod %s can NOT reach %s with port %d",
					firstClientPod.Definition.Name, serverPod.Definition.Name, port5001))

			Eventually(func() error {
				return runTraffic(secondClientPod, removePrefixFromIP(serverPodIPv6), sctpProtocol, port5003)
			}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).ShouldNot(HaveOccurred(),
				fmt.Sprintf("pod %s can NOT reach %s with port %d",
					secondClientPod.Definition.Name, serverPod.Definition.Name, port5003))

			Eventually(func() error {
				return runTraffic(secondClientPod, removePrefixFromIP(firstClientPodIPv6), sctpProtocol, port5001)
			}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).ShouldNot(HaveOccurred(),
				fmt.Sprintf("pod %s can NOT reach %s with port %d",
					secondClientPod.Definition.Name, firstClientPod.Definition.Name, port5001))
		})

		It("Ingress/Egress Allow access only to a specific port/protocol", reportxml.ID("70040"), func() {
			ingressRule, err := networkpolicy.NewIngressRuleBuilder().WithPortAndProtocol(uint16(port5001), "SCTP").
				WithPeerPodSelector(metav1.LabelSelector{MatchLabels: map[string]string{"pod": labelFirstClientPod}}).
				GetIngressRuleCfg()
			Expect(err).ToNot(HaveOccurred(), "Failed to build ingress rule")

			egressRule, err := networkpolicy.NewEgressRuleBuilder().WithPortAndProtocol(uint16(port5001), "SCTP").
				WithPeerPodSelectorAndCIDR(metav1.LabelSelector{MatchLabels: map[string]string{"pod": labelSecondClientPod}},
					ipaddr.RemovePrefix(secondClientPodIPv6)+"/"+"128").GetEgressRuleCfg()
			Expect(err).ToNot(HaveOccurred(), "Failed to build egress rule")
			_, err = networkpolicy.NewMultiNetworkPolicyBuilder(
				APIClient, multiNetworkPolicyName, tsparams.TestNamespaceName).
				WithNetwork(srIovNet.Definition.Name).
				WithPodSelector(metav1.LabelSelector{MatchLabels: map[string]string{"pod": labelServerPod}}).
				WithPolicyType(multinetpolicyapiv1.PolicyTypeIngress).WithPolicyType(multinetpolicyapiv1.PolicyTypeEgress).
				WithIngressRule(*ingressRule).WithEgressRule(*egressRule).Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create multiNetworkPolicy")

			By("Testing connectivity with multiNetworkPolicy applied")
			// Allowed port works as expected.
			Eventually(func() error {
				return runTraffic(firstClientPod, removePrefixFromIP(serverPodIPv6), sctpProtocol, port5001)
			}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).ShouldNot(HaveOccurred(),
				fmt.Sprintf("pod %s can NOT reach %s with port %d",
					firstClientPod.Definition.Name, serverPod.Definition.Name, port5001))
			// Same client but port is not allowed.
			Eventually(func() error {
				return runTraffic(firstClientPod, removePrefixFromIP(serverPodIPv6), sctpProtocol, port5003)
			}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).Should(HaveOccurred(),
				fmt.Sprintf("pod %s CAN reach %s with port %d",
					firstClientPod.Definition.Name, serverPod.Definition.Name, port5003))
			// Traffic from different pod denied.
			Eventually(func() error {
				return runTraffic(secondClientPod, removePrefixFromIP(serverPodIPv6), sctpProtocol, port5001)
			}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).Should(HaveOccurred(),
				fmt.Sprintf("pod %s CAN reach %s with port %d",
					secondClientPod.Definition.Name, serverPod.Definition.Name, port5001))
			// Egress policy works as expected.
			Eventually(func() error {
				return runTraffic(serverPod, removePrefixFromIP(firstClientPodIPv6), sctpProtocol, port5001)
			}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).Should(HaveOccurred(),
				fmt.Sprintf("pod %s CAN reach %s with port %d",
					serverPod.Definition.Name, firstClientPod.Definition.Name, port5001))

			Eventually(func() error {
				return runTraffic(serverPod, removePrefixFromIP(secondClientPodIPv6), sctpProtocol, port5001)
			}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).ShouldNot(HaveOccurred(),
				fmt.Sprintf("pod %s can NOT reach %s with port %d",
					serverPod.Definition.Name, secondClientPod.Definition.Name, port5001))
		})

		It("Ingress/Egress Allow access only to a specific subnet", reportxml.ID("70041"), func() {
			ingressRule, err := networkpolicy.NewIngressRuleBuilder().
				WithCIDR(ipaddr.RemovePrefix(secondClientPodIPv6) + "/" + "128").
				GetIngressRuleCfg()
			Expect(err).ToNot(HaveOccurred(), "Failed to build ingress rule")

			policy, err := networkpolicy.NewMultiNetworkPolicyBuilder(
				APIClient, multiNetworkPolicyName, tsparams.TestNamespaceName).
				WithNetwork(srIovNet.Definition.Name).
				WithPodSelector(metav1.LabelSelector{MatchLabels: map[string]string{"pod": labelServerPod}}).
				WithPolicyType(multinetpolicyapiv1.PolicyTypeIngress).
				WithIngressRule(*ingressRule).Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create multiNetworkPolicy")

			By("Testing connectivity with multiNetworkPolicy applied")
			Eventually(func() error {
				return runTraffic(secondClientPod, removePrefixFromIP(serverPodIPv6), sctpProtocol, port5001)
			}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).ShouldNot(HaveOccurred(),
				fmt.Sprintf("pod %s can NOT reach %s with port %d",
					secondClientPod.Definition.Name, serverPod.Definition.Name, port5001))

			Eventually(func() error {
				return runTraffic(secondClientPod, removePrefixFromIP(serverPodIPv6), sctpProtocol, port5003)
			}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).ShouldNot(HaveOccurred(),
				fmt.Sprintf("pod %s can NOT reach %s with port %d",
					secondClientPod.Definition.Name, serverPod.Definition.Name, port5003))

			Eventually(func() error {
				return runTraffic(firstClientPod, removePrefixFromIP(serverPodIPv6), sctpProtocol, port5001)
			}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).Should(HaveOccurred(),
				fmt.Sprintf("pod %s CAN reach %s with port %d",
					firstClientPod.Definition.Name, serverPod.Definition.Name, port5001))

			Eventually(func() error {
				return runTraffic(firstClientPod, removePrefixFromIP(serverPodIPv6), sctpProtocol, port5003)
			}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).Should(HaveOccurred(),
				fmt.Sprintf("pod %s CAN reach %s with port %d",
					firstClientPod.Definition.Name, serverPod.Definition.Name, port5003))

			err = policy.Delete()
			Expect(err).ToNot(HaveOccurred(), "Failed to delete multinetworkpolicy object")

			egressRule, err := networkpolicy.NewEgressRuleBuilder().
				WithPeerPodSelectorAndCIDR(metav1.LabelSelector{MatchLabels: map[string]string{"pod": labelSecondClientPod}},
					ipaddr.RemovePrefix(secondClientPodIPv6)+"/"+"128").GetEgressRuleCfg()
			Expect(err).ToNot(HaveOccurred(), "Failed to build egressRule")

			_, err = networkpolicy.NewMultiNetworkPolicyBuilder(
				APIClient, multiNetworkPolicyName, tsparams.TestNamespaceName).
				WithNetwork(srIovNet.Definition.Name).
				WithPodSelector(metav1.LabelSelector{MatchLabels: map[string]string{"pod": labelServerPod}}).
				WithPolicyType(multinetpolicyapiv1.PolicyTypeEgress).
				WithEgressRule(*egressRule).Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create multinetworkpolicy object")

			// Egress policy works as expected.
			Eventually(func() error {
				return runTraffic(serverPod, removePrefixFromIP(firstClientPodIPv6), sctpProtocol, port5001)
			}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).Should(HaveOccurred(),
				fmt.Sprintf("pod %s CAN reach %s with port %d",
					serverPod.Definition.Name, firstClientPod.Definition.Name, port5001))

			Eventually(func() error {
				return runTraffic(serverPod, removePrefixFromIP(secondClientPodIPv6), sctpProtocol, port5001)
			}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).ShouldNot(HaveOccurred(),
				fmt.Sprintf("pod %s can NOT reach %s with port %d",
					serverPod.Definition.Name, secondClientPod.Definition.Name, port5001))
		})
	})

	Context("dual-stack", func() {
		BeforeEach(func() {
			By("Creating first client pod")
			firstClientPod = createClientPod(
				"client1",
				srIovNet.Definition.Name,
				workerNodeList[1].Definition.Name,
				sctpProtocol,
				labelFirstClientPod,
				[]string{firstClientPodIPv6, firstClientPodIP})

			By("Creating second client pod")
			secondClientPod = createClientPod(
				"client2",
				srIovNet.Definition.Name,
				workerNodeList[1].Definition.Name,
				sctpProtocol,
				labelSecondClientPod,
				[]string{secondClientPodIPv6, secondClientPodIP})

			By("Creating server pod")
			serverPod = createServerPod(
				srIovNet.Definition.Name,
				workerNodeList[0].Definition.Name,
				sctpProtocol,
				[]string{serverPodIPv6, serverPodIP},
				[]string{firstClientPodIPv6, firstClientPodIP},
				[]string{secondClientPodIPv6, secondClientPodIP})

			Consistently(func() bool {
				return firstClientPod.WaitUntilRunning(10*time.Second) == nil &&
					secondClientPod.WaitUntilRunning(10*time.Second) == nil &&
					serverPod.WaitUntilRunning(10*time.Second) == nil
			}, 1*time.Minute, 3*time.Second).Should(BeTrue(), "Failed not all pods are in running state")

			By("Testing connectivity without multiNetworkPolicy applied")
			// Connectivity works without policy ipv6
			testSCTPConnectivityWithoutPolicy(firstClientPod, secondClientPod, serverPod, serverPodIPv6, firstClientPodIPv6)

			// Connectivity works without policy ipv4
			testSCTPConnectivityWithoutPolicy(firstClientPod, secondClientPod, serverPod, serverPodIP, firstClientPodIP)
		})

		It("Ingress/Egress allow dual-stack subnet sctp", reportxml.ID("70042"), func() {
			ingressRule, err := networkpolicy.NewIngressRuleBuilder().
				WithCIDR(ipaddr.RemovePrefix(secondClientPodIPv6) + "/128").
				WithCIDR(ipaddr.RemovePrefix(secondClientPodIP) + "/32").
				GetIngressRuleCfg()
			Expect(err).ToNot(HaveOccurred(), "Failed to build ingress rule")

			policy, err := networkpolicy.NewMultiNetworkPolicyBuilder(
				APIClient, multiNetworkPolicyName, tsparams.TestNamespaceName).
				WithNetwork(srIovNet.Definition.Name).
				WithPodSelector(metav1.LabelSelector{MatchLabels: map[string]string{"pod": labelServerPod}}).
				WithPolicyType(multinetpolicyapiv1.PolicyTypeIngress).
				WithIngressRule(*ingressRule).Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create multiNetworkPolicy")

			// Test ingress rule ipv6
			for _, serverIP := range []string{serverPodIPv6, serverPodIP} {
				testIngressSCTPPolicy(firstClientPod, secondClientPod, serverPod, serverIP)
			}

			err = policy.Delete()
			Expect(err).ToNot(HaveOccurred(), "Failed to delete multinetworkpolicy object")

			egressRule, err := networkpolicy.NewEgressRuleBuilder().
				WithPeerPodSelectorAndCIDR(metav1.LabelSelector{MatchLabels: map[string]string{"pod": labelSecondClientPod}},
					ipaddr.RemovePrefix(secondClientPodIPv6)+"/"+"128").
				WithPeerPodSelectorAndCIDR(metav1.LabelSelector{MatchLabels: map[string]string{"pod": labelSecondClientPod}},
					ipaddr.RemovePrefix(secondClientPodIP)+"/"+"32").
				GetEgressRuleCfg()
			Expect(err).ToNot(HaveOccurred(), "Failed to build egressRule")

			_, err = networkpolicy.NewMultiNetworkPolicyBuilder(
				APIClient, multiNetworkPolicyName, tsparams.TestNamespaceName).
				WithNetwork(srIovNet.Definition.Name).
				WithPodSelector(metav1.LabelSelector{MatchLabels: map[string]string{"pod": labelServerPod}}).
				WithPolicyType(multinetpolicyapiv1.PolicyTypeEgress).
				WithEgressRule(*egressRule).Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create multinetworkpolicy object")

			// Egress policy works as expected ipv4/ipv6.
			testEgressSCTPPolicy(firstClientPod, secondClientPod, serverPod, firstClientPodIPv6, secondClientPodIPv6)
			testEgressSCTPPolicy(firstClientPod, secondClientPod, serverPod, firstClientPodIP, secondClientPodIP)
		})
	})

	AfterEach(func() {
		testNameSpace, err := namespace.Pull(APIClient, tsparams.TestNamespaceName)
		Expect(err).ToNot(HaveOccurred(), "Failed to pull namespace")
		err = testNameSpace.CleanObjects(
			10*time.Minute,
			networkpolicy.GetMultiNetworkGVR())
		Expect(err).ToNot(HaveOccurred(), "Failed to remove multiNetworkPolicy object from namespace")

		protocol := tcpProtocol
		serverIP := serverPodIP
		// Pull the latest version of firstClientPod in order to get an updated network Annotations from the cluster.
		Expect(firstClientPod.Exists()).To(BeTrue(), "Client pod doesn't exist")
		if strings.Contains(firstClientPod.Object.Annotations["k8s.v1.cni.cncf.io/network-status"],
			removePrefixFromIP(firstClientPodIPv6)) {
			serverIP = serverPodIPv6
		}

		if strings.Contains(strings.Join(firstClientPod.Definition.Spec.Containers[0].Command, " "),
			fmt.Sprintf("protocol=%s", sctpProtocol)) {
			protocol = sctpProtocol
		}

		// All traffic is accepted
		Eventually(func() error {
			return runTraffic(firstClientPod, removePrefixFromIP(serverIP), protocol, port5001)
		}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).ShouldNot(HaveOccurred(),
			fmt.Sprintf("pod %s can NOT reach %s with port %d",
				firstClientPod.Definition.Name, serverPod.Definition.Name, port5001))

		err = testNameSpace.CleanObjects(
			10*time.Minute,
			pod.GetGVR())
		Expect(err).ToNot(HaveOccurred(), "Failed to remove pod objects from namespace")
	})

	AfterAll(func() {
		By("Removing all SR-IOV Policies")
		err := sriov.CleanAllNetworkNodePolicies(APIClient, NetConfig.SriovOperatorNamespace)
		Expect(err).ToNot(HaveOccurred(), "Fail to clean srIovPolicy")

		By("Removing all srIovNetworks")
		err = sriov.CleanAllNetworksByTargetNamespace(
			APIClient, NetConfig.SriovOperatorNamespace, tsparams.TestNamespaceName)
		Expect(err).ToNot(HaveOccurred(), "Fail to clean sriov networks")

		By("Waiting until cluster MCP and SR-IOV are stable")
		err = netenv.WaitForSriovAndMCPStable(
			APIClient, tsparams.MCOWaitTimeout, time.Minute, NetConfig.CnfMcpLabel, NetConfig.SriovOperatorNamespace)
		Expect(err).ToNot(HaveOccurred(), "Fail to wait until cluster is stable")
	})
})

// runTraffic sends traffic from clientPod to serverIP.
func runTraffic(clientPod *pod.Builder, serverIP, protocol string, port int) error {
	buffer, err := clientPod.ExecCommand(
		[]string{"testcmd", fmt.Sprintf("-port=%d", port), "-interface=net1",
			fmt.Sprintf("-server=%s", serverIP), fmt.Sprintf("-protocol=%s", protocol), "-mtu=1200"},
	)
	if err != nil {
		return fmt.Errorf("%w: %s", err, buffer.String())
	}

	return nil
}

func enableMultiNetworkPolicy(status bool) {
	By(fmt.Sprintf("Configuring MultiNetworkPolicy mode %v", status))

	clusterNetwork, err := cluster.GetOCPNetworkOperatorConfig(APIClient)
	Expect(err).ToNot(HaveOccurred(), "Failed to collect network.operator object")

	clusterNetwork, err = clusterNetwork.SetMultiNetworkPolicy(status, 20*time.Minute)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to set MultiNetworkPolicy mode %v", status))

	network, err := clusterNetwork.Get()
	Expect(err).ToNot(HaveOccurred(), "Failed to collect network.operator object")
	Expect(network.Spec.UseMultiNetworkPolicy).To(BeEquivalentTo(&status),
		"Failed network.operator UseMultiNetworkPolicy flag is not in expected state")

	if status {
		Eventually(func() error {
			multusDs, err := daemonset.Pull(APIClient, tsparams.MultiNetworkPolicyDSName, NetConfig.MultusNamesapce)
			if err != nil {
				return err
			}

			if multusDs.IsReady(10 * time.Second) {
				return nil
			}

			return fmt.Errorf("DS is not ready")
		}, tsparams.WaitTimeout, tsparams.RetryInterval).ShouldNot(HaveOccurred(),
			"Failed MultiNetworkPolicy daemonSet is not ready")
	}
}

func defineClientCMD(protocol, serverIP string) []string {
	cmd := fmt.Sprintf("testcmd --listen --protocol=%s -port=%d -interface=net1 ", protocol, port5001)
	if protocol == sctpProtocol {
		cmd += fmt.Sprintf("-server=%s", serverIP)
	}

	return []string{"/bin/bash", "-c", cmd}
}

func createClientPod(podName, srIovNetwork, nodeName, protocol, label string, ipaddress []string) *pod.Builder {
	staticAnnotation := pod.StaticIPAnnotation(srIovNetwork, ipaddress)

	var containers []*pod.ContainerBuilder
	for idx, ipAddr := range ipaddress {
		containers = append(containers,
			pod.NewContainerBuilder(
				fmt.Sprintf("test%d", idx), NetConfig.CnfNetTestContainer, defineClientCMD(protocol, removePrefixFromIP(ipAddr))))
	}

	clientPod := pod.NewBuilder(APIClient, podName, tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).
		WithSecondaryNetwork(staticAnnotation).
		DefineOnNode(nodeName).WithLabel("pod", label).WithPrivilegedFlag()

	for idx, container := range containers {
		containerCfg, err := container.GetContainerCfg()
		Expect(err).ToNot(HaveOccurred(), "Failed to collect container configuration")

		if idx == 0 {
			clientPod.RedefineDefaultContainer(*containerCfg)
		} else {
			clientPod.WithAdditionalContainer(containerCfg)
		}
	}

	clientPod, err := clientPod.CreateAndWaitUntilRunning(tsparams.WaitTimeout)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to define pod annotation for clientPod with IPAddress: %s", ipaddress))

	return clientPod
}

func createServerPod(
	srIovNetworkName, nodeName, protocol string, serverPodIP, firstClientPodIP, secondClientPodIP []string) *pod.Builder {
	By("Creating server pod")

	var (
		initContainers      []*corev1.Container
		serverPodContainers []*corev1.Container
	)

	for idx, serverIP := range serverPodIP {
		initCommand := []string{"bash", "-c",
			fmt.Sprintf("ping %s -c 3 -w 90 && ping %s -c 3 -w 90",
				removePrefixFromIP(firstClientPodIP[idx]), removePrefixFromIP(secondClientPodIP[idx]))}
		initContainer, err := pod.NewContainerBuilder(
			fmt.Sprintf("init%d", idx), NetConfig.CnfNetTestContainer, initCommand).GetContainerCfg()
		Expect(err).ToNot(HaveOccurred(), "Failed to define init container")

		initContainers = append(initContainers, initContainer)

		serverPod5003Container, err := pod.NewContainerBuilder(
			fmt.Sprintf("testcmd5003%d", idx), NetConfig.CnfNetTestContainer,
			setTestCmdServer(protocol, removePrefixFromIP(serverIP), port5003)).
			GetContainerCfg()
		Expect(err).ToNot(HaveOccurred(), "Failed to define server 5003 pod container")

		serverPodContainers = append(serverPodContainers, serverPod5003Container)

		serverPod5001Container, err := pod.NewContainerBuilder(
			fmt.Sprintf("testcmd5001%d", idx), NetConfig.CnfNetTestContainer,
			setTestCmdServer(protocol, removePrefixFromIP(serverIP), port5001)).
			GetContainerCfg()
		Expect(err).ToNot(HaveOccurred(), "Failed to define server 5001 pod container")

		serverPodContainers = append(serverPodContainers, serverPod5001Container)
	}

	serverPod := pod.NewBuilder(
		APIClient, "server", tsparams.TestNamespaceName, NetConfig.CnfNetTestContainer).DefineOnNode(nodeName).
		WithPrivilegedFlag().
		WithLabel("pod", labelServerPod).
		WithSecondaryNetwork(pod.StaticIPAnnotation(srIovNetworkName, serverPodIP))

	for _, initContainer := range initContainers {
		serverPod.WithAdditionalInitContainer(initContainer)
	}

	for idx, container := range serverPodContainers {
		if idx == 0 {
			serverPod.RedefineDefaultContainer(*container)
		} else {
			serverPod.WithAdditionalContainer(container)
		}
	}

	serverPod, err := serverPod.CreateAndWaitUntilRunning(tsparams.WaitTimeout)
	Expect(err).ToNot(HaveOccurred(), "Failed to create server pod")

	return serverPod
}

func setTestCmdServer(protocol, serverIP string, port int) []string {
	cmd := fmt.Sprintf("testcmd --listen -interface=net1 --protocol=%s --mtu=1500 -port=%d", protocol, port)
	if protocol == sctpProtocol {
		cmd += fmt.Sprintf(" -server=%s", serverIP)
	}

	return []string{"bash", "-c", cmd}
}

func removePrefixFromIP(ipAddr string) string {
	return strings.Split(ipAddr, "/")[0]
}

func testSCTPConnectivityWithoutPolicy(
	firstClientPod, secondClientPod, serverPod *pod.Builder, serverPodIP, clientPodIP string) {
	By("Testing SCTP connectivity without multiNetworkPolicy applied")
	Eventually(func() error {
		return runTraffic(firstClientPod, removePrefixFromIP(serverPodIP), sctpProtocol, port5001)
	}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).ShouldNot(HaveOccurred(),
		fmt.Sprintf("pod %s can NOT reach %s dst ip %s with port %d",
			firstClientPod.Definition.Name, serverPod.Definition.Name, serverPodIP, port5001))

	Eventually(func() error {
		return runTraffic(secondClientPod, removePrefixFromIP(serverPodIP), sctpProtocol, port5003)
	}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).ShouldNot(HaveOccurred(),
		fmt.Sprintf("pod %s can NOT reach %s dst ip %s with port %d",
			secondClientPod.Definition.Name, serverPod.Definition.Name, serverPodIP, port5003))

	Eventually(func() error {
		return runTraffic(secondClientPod, removePrefixFromIP(clientPodIP), sctpProtocol, port5001)
	}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).ShouldNot(HaveOccurred(),
		fmt.Sprintf("pod %s can NOT reach %s dst ip %s with port %d",
			secondClientPod.Definition.Name, firstClientPod.Definition.Name, serverPodIP, port5001))
}

func testIngressSCTPPolicy(firstClientPod, secondClientPod, serverPod *pod.Builder, serverPodIP string) {
	By("Testing SCTP connectivity with ingress multiNetworkPolicy applied")

	Eventually(func() error {
		return runTraffic(secondClientPod, removePrefixFromIP(serverPodIP), sctpProtocol, port5001)
	}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).ShouldNot(HaveOccurred(),
		fmt.Sprintf("pod %s can NOT reach %s dest ip %s with port %d",
			secondClientPod.Definition.Name, serverPod.Definition.Name, serverPodIP, port5001))

	Eventually(func() error {
		return runTraffic(secondClientPod, removePrefixFromIP(serverPodIP), sctpProtocol, port5003)
	}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).ShouldNot(HaveOccurred(),
		fmt.Sprintf("pod %s can NOT reach %s dest ip %s with port %d",
			secondClientPod.Definition.Name, serverPod.Definition.Name, serverPodIP, port5001))

	Eventually(func() error {
		return runTraffic(firstClientPod, removePrefixFromIP(serverPodIP), sctpProtocol, port5001)
	}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).Should(HaveOccurred(),
		fmt.Sprintf("pod %s CAN reach %s dst ip %s with port %d",
			firstClientPod.Definition.Name, serverPod.Definition.Name, serverPodIP, port5001))

	Eventually(func() error {
		return runTraffic(firstClientPod, removePrefixFromIP(serverPodIP), sctpProtocol, port5003)
	}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).Should(HaveOccurred(),
		fmt.Sprintf("pod %s CAN reach %s dst ip %s with port %d",
			firstClientPod.Definition.Name, serverPod.Definition.Name, serverPodIP, port5003))
}

func testEgressSCTPPolicy(
	firstClientPod, secondClientPod, serverPod *pod.Builder, firstClientPodIP, secondClientPodIP string) {
	By("Testing SCTP connectivity with egress multiNetworkPolicy applied")
	Eventually(func() error {
		return runTraffic(serverPod, removePrefixFromIP(firstClientPodIP), sctpProtocol, port5001)
	}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).Should(HaveOccurred(),
		fmt.Sprintf("pod %s CAN reach %s dst ip %s with port %d",
			serverPod.Definition.Name, firstClientPod.Definition.Name, firstClientPodIP, port5001))

	Eventually(func() error {
		return runTraffic(serverPod, removePrefixFromIP(secondClientPodIP), sctpProtocol, port5001)
	}, tsparams.WaitTrafficTimeout, tsparams.RetryTrafficInterval).ShouldNot(HaveOccurred(),
		fmt.Sprintf("pod %s can NOT reach %s dst ip %s with port %d",
			serverPod.Definition.Name, secondClientPod.Definition.Name, secondClientPodIP, port5001))
}
