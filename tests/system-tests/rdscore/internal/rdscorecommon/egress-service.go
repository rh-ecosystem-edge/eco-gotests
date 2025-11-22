package rdscorecommon

import (
	"fmt"
	"net"
	"net/netip"
	"os/exec"
	"strings"

	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/egressservice"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/service"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreparams"
)

const (
	egressSVCContainer         = "rds-egress-container"
	egressSVCDeploy1Name       = "rds-egress-deploy"
	egressSVC1Name             = "egress-svc-1"
	egressSVC1Labels           = "rds-egress=rds-core"
	egressSVCDeploy2Name       = "rds-egress-deploy2"
	egressSVC2Name             = "egress-svc-2"
	egressSVC2Labels           = "rds-egress=rds-core-2"
	egressSVCDeploy3Name       = "rds-egress-etp-cluster"
	egressSVC3Name             = "egress-svc-etp-cluster"
	egressSVC3Labels           = "rds-egress=rds-core-etp-cluster"
	egressSVCDeploy4Name       = "rds-egress-etp-local"
	egressSVC4Name             = "egress-svc-etp-local"
	egressSVC4Labels           = "rds-egress=rds-core-etp-local"
	servicePort          int32 = 9090
	serviceTargetPort    int32 = 9090
	httpSuccessCode            = "200"
)

func defineEgressSVCContainer(cName, cImage string, cCmd []string) *pod.ContainerBuilder {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Creating container %q", egressSVCContainer)

	deployContainer := pod.NewContainerBuilder(cName, cImage, cCmd)

	cPort := corev1.ContainerPort{
		ContainerPort: servicePort,
		Protocol:      corev1.ProtocolTCP,
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Adding ContainerPort definition:\n%v\n", cPort)
	deployContainer = deployContainer.WithPorts([]corev1.ContainerPort{cPort})

	return deployContainer
}

func defineEgressSVCDeployment(containerConfig *corev1.Container, deployName, deployNs string,
	deployLabels, nodeSelector map[string]string, replicas int32) *deployment.Builder {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Defining deployment %q in %q ns", deployName, deployNs)

	deploy := deployment.NewBuilder(APIClient, deployName, deployNs, deployLabels, *containerConfig)

	By("Setting Replicas count")

	deploy = deploy.WithReplicas(replicas)

	if len(nodeSelector) != 0 {
		By("Adding NodeSelector to the deployment")

		deploy = deploy.WithNodeSelector(nodeSelector)
	}

	return deploy
}

func defineService(svcName, svcNSName string,
	svcSelector map[string]string, svcPort corev1.ServicePort) *service.Builder {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Defining service %q in %q ns", svcName, svcNSName)

	return service.NewBuilder(APIClient, svcName, svcNSName, svcSelector, svcPort)
}

func deleteService(svcName, svcNSName string) {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deleting service %q in %q ns", svcName, svcNSName)

	var ctx SpecContext

	svcBuilder, err := service.Pull(APIClient, svcName, svcNSName)

	switch {
	case svcBuilder == nil && err != nil:
		{
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Service %q not found in %q ns", svcName, svcNSName)
		}
	case svcBuilder != nil && err == nil:
		{
			Eventually(func() bool {
				err := svcBuilder.Delete()
				if err != nil {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Error deleting Service %q in %q namespace: %v",
						svcBuilder.Definition.Name, svcBuilder.Definition.Namespace, err)

					return false
				}

				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deleted Service %q in %q namespace:",
					svcName, svcNSName)

				return true
			}).WithContext(ctx).WithPolling(5*time.Second).WithTimeout(1*time.Minute).Should(
				BeTrue(), fmt.Sprintf("Failed to deleted service %q in %q namespace", svcName, svcNSName))
		}
	default:
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Unhandled error while deleting service %q in %q ns",
			svcName, svcNSName)
	}
}

func deleteEgressService(svcName, svcNSName string) {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deleting EgressService %q in %q ns",
		svcName, svcNSName)

	var ctx SpecContext

	svcBuilder, err := egressservice.Pull(APIClient, svcName, svcNSName)

	switch {
	case svcBuilder == nil && err != nil:
		{
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("EgressService %q not found in %q ns", svcName, svcNSName)
		}
	case svcBuilder != nil && err == nil:
		{
			Eventually(func() bool {
				svcBuilder, err := svcBuilder.Delete()
				if err != nil {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Error deleting EgressService %q in %q namespace: %v",
						svcBuilder.Definition.Name, svcBuilder.Definition.Namespace, err)

					return false
				}

				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deleted EgressService %q in %q namespace:",
					svcBuilder.Definition.Name, svcBuilder.Definition.Namespace)

				return true
			}).WithContext(ctx).WithPolling(5*time.Second).WithTimeout(1*time.Minute).Should(
				BeTrue(), fmt.Sprintf("Failed to deleted EgressService %q in %q namespace", svcName, svcNSName))
		}
	default:
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Unhandled error while deleting EgressService %q in %q ns",
			svcName, svcNSName)
	}
}

func waitForPodsGone(podNS, podSelector string) {
	By(fmt.Sprintf("Ensuring pods matching %q label in %q namespace are gone", podNS, podSelector))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Ensuring pods matching %q label in %q namespace are gone",
		podSelector, podNS)

	var ctx SpecContext

	Eventually(func() bool {
		oldPods, err := pod.List(APIClient, podNS,
			metav1.ListOptions{LabelSelector: podSelector})
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Error listing pods: %v", err)

			return false
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Found %d pods matching label", len(oldPods))

		return len(oldPods) == 0
	}).WithContext(ctx).WithPolling(5*time.Second).WithTimeout(1*time.Minute).Should(BeTrue(),
		"pods matching label() still present")
}

func verifyPodSourceAddress(clientPods []*pod.Builder, cmdToRun []string, expectedIP string,
	expectedIPsMap map[string]string) {
	By("Validating pods source address")

	for _, clientPod := range clientPods {
		var (
			parsedIP string
			ctx      SpecContext
		)

		Eventually(func() bool {
			result, err := clientPod.ExecCommand(cmdToRun, clientPod.Object.Spec.Containers[0].Name)
			if err != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Error running command from within a pod %q: %v",
					clientPod.Object.Name, err)

				return false
			}

			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Successfully executed command from within a pod %q: %v",
				clientPod.Object.Name, err)
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Command's output:\n\t%v", result.String())

			parsedIP, _, err = net.SplitHostPort(result.String())
			if err != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to parse %q for host/port pair", result.String())

				return false
			}

			return true
		}).WithContext(ctx).WithPolling(5*time.Second).WithTimeout(1*time.Minute).Should(BeTrue(),
			"Failed to run command from within pod")

		var expectedResult []string

		if expectedIP == "" {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Use IP from the ExpectedIPsMap")

			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Get expected IP address for pod %q running on %q",
				clientPod.Object.Name, clientPod.Object.Spec.NodeName)

			var ok bool

			expectedNodeIP, ok := expectedIPsMap[clientPod.Object.Spec.NodeName]

			if !ok {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Cannot find expected IP address for pod %q running on %q",
					clientPod.Object.Name, clientPod.Object.Spec.NodeName)

				Fail(fmt.Sprintf("Cannot find expected IP address for pod %q running on %q",
					clientPod.Object.Name, clientPod.Object.Spec.NodeName))
			}

			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Expected IP address for pod %q running on %q is %q",
				clientPod.Object.Name, clientPod.Object.Spec.NodeName, expectedNodeIP)

			expectedResult = strings.Split(expectedNodeIP, ",")
		} else {
			expectedResult = []string{expectedIP}
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Comparing %q with expected %q", parsedIP, expectedResult)

		Expect(expectedResult).To(ContainElement(parsedIP),
			fmt.Sprintf("Mismatched IP address. Expected %q got %q", expectedResult, parsedIP))
	}
}

// VerifyEgressServiceWithClusterETPLoadbalancer verifies EgressService with externalTrafficPolicy set to Cluster
// and sourceIPBy set to "LoadBalancer".
func VerifyEgressServiceWithClusterETPLoadbalancer(ctx SpecContext) {
	VerifyEgressServiceETPClusterWrapper(
		egressSVCDeploy1Name,
		map[string]string{},
		RDSCoreConfig.EgressServiceNS,
		egressSVC1Name,
		egressSVC1Labels,
		RDSCoreConfig.EgressServiceDeploy1Image,
		RDSCoreConfig.EgressServiceDeploy1CMD,
		RDSCoreConfig.EgressServiceDeploy1IPAddrPool,
		"LoadBalancerIP",
		RDSCoreConfig.EgressServiceDeploy1NodeSelector,
		RDSCoreConfig.EgressServiceVRF1Network,
		RDSCoreConfig.EgressServiceRemoteIP,
		RDSCoreConfig.EgressServiceRemoteIPv6,
		RDSCoreConfig.EgressServiceRemotePort)
}

// VerifyEgressServiceWithClusterETPNetwork verifies EgressService with externalTrafficPolicy set to Cluster
// and sourceIPBy set to "Network".
func VerifyEgressServiceWithClusterETPNetwork(ctx SpecContext) {
	VerifyEgressServiceETPClusterWrapper(
		egressSVCDeploy3Name,
		RDSCoreConfig.EgressServiceDeploy3NodeSelector,
		RDSCoreConfig.EgressServiceNS,
		egressSVC3Name,
		egressSVC3Labels,
		RDSCoreConfig.EgressServiceDeploy3Image,
		RDSCoreConfig.EgressServiceDeploy3CMD,
		RDSCoreConfig.EgressServiceDeploy3IPAddrPool,
		"Network",
		RDSCoreConfig.EgressServiceDeploy3NodeSelector,
		RDSCoreConfig.EgressServiceVRF3Network,
		RDSCoreConfig.EgressServiceRemoteIP,
		RDSCoreConfig.EgressServiceRemoteIPv6,
		RDSCoreConfig.EgressServiceRemotePort)
}

// VerifyEgressServiceETPClusterWrapper verifies EgressService with externalTrafficPolicy set to Cluster.
// sourceIPBy is customizable and accepts next values: "LoadBalancer" and/or "Network"
//
//nolint:funlen
func VerifyEgressServiceETPClusterWrapper(
	deployName string, // egressSVCDeploy1Name
	deployNodeSelector map[string]string, //
	testNamespace string, // RDSCoreConfig.EgressServiceNS
	svcName string, // egressSVC1Name
	svcSelector string, // egressSVC1Labels
	containerImage string, // RDSCoreConfig.EgressServiceDeploy1Image
	containerCommand []string, // RDSCoreConfig.EgressServiceDeploy1CMD
	ipAddrPoolName string, // RDSCoreConfig.EgressServiceDeploy1IPAddrPool
	sourceIPBy string, // "LoadBalancer", "Network"
	egrSvcNodeSelector map[string]string, // RDSCoreConfig.EgressServiceDeploy1NodeSelector
	vrfNetwork string, // RDSCoreConfig.EgressServiceVRF1Network
	remoteTargetIP string, // RDSCoreConfig.EgressServiceRemoteIP
	remoteTargetIPv6 string, // RDSCoreConfig.EgressServiceRemoteIPv6
	remoteTargetPort string) {
	if strings.EqualFold(sourceIPBy, "Network") && len(RDSCoreConfig.EgressServiceNetworkExpectedIPs) == 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("EgressServiceNetworkExpectedIPs is not defined. Skipping")

		Skip("EgressServiceNetworkExpectedIPs is not defined. Skipping")
	}

	deleteDeployments(deployName, testNamespace)
	deleteService(svcName, testNamespace)
	deleteEgressService(svcName, testNamespace)

	waitForPodsGone(testNamespace, svcSelector)

	podContainer := defineEgressSVCContainer(egressSVCContainer,
		containerImage, containerCommand)

	By("Reseting SecurityContext")

	podContainer = podContainer.WithSecurityContext(&corev1.SecurityContext{RunAsGroup: nil, RunAsUser: nil})

	cfgContainer, err := podContainer.GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to get container definition: %v", err))

	deployLabels := map[string]string{
		strings.Split(svcSelector, "=")[0]: strings.Split(svcSelector, "=")[1],
	}

	deploy := defineEgressSVCDeployment(cfgContainer, deployName,
		testNamespace, deployLabels, deployNodeSelector, int32(2))

	Expect(deploy).ToNot(BeNil(), "Failed to create deployment")

	deploy, err = deploy.CreateAndWaitUntilReady(5 * time.Minute)

	Expect(err).ToNot(HaveOccurred(), "Deployment hasn't reached Ready status")

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deployment %q in %q namespace is Ready",
		deploy.Object.Name, deploy.Object.Namespace)

	By("Defining a ServicePort")

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Defining ServicePort object")

	svcPort, err := service.DefineServicePort(servicePort, serviceTargetPort, corev1.ProtocolTCP)

	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to define TargetPort: %v", err))

	By("Defining Service")

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Defining Service object")

	svcLabels := map[string]string{
		strings.Split(svcSelector, "=")[0]: strings.Split(svcSelector, "=")[1],
	}

	svcBuilder := defineService(svcName, testNamespace, svcLabels, *svcPort)

	Expect(svcBuilder).ToNot(BeNil(), "Failed to defined service")

	By("Setting type to LoadBalancer")

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Setting Service's type to 'LoadBalancer'")

	svcBuilder.Definition.Spec.Type = "LoadBalancer"

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Service Definition:\n%#v\n", svcBuilder.Definition)

	By("Setting AddressPool annotation")

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Setting MetalLB address-pool annotation")

	svcBuilder = svcBuilder.WithAnnotation(map[string]string{
		"metallb.universe.tf/address-pool": ipAddrPoolName})

	By("Setting ipFamilyPolicy to 'RequireDualStack'")

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Setting ipFamilyPolicy to 'RequireDualStack'")

	svcBuilder = svcBuilder.WithIPFamily([]corev1.IPFamily{"IPv4", "IPv6"},
		corev1.IPFamilyPolicyRequireDualStack)

	By("Creating a service")

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Creating Service object")

	svcBuilder, err = svcBuilder.Create()
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to create Service: %v", err))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Created Service: %#v", svcBuilder.Object)

	By("Defining EgressService")

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Defining EgressService %q in %q namespace",
		svcName, testNamespace)

	egrSVCBuilder := egressservice.NewEgressServiceBuilder(APIClient, svcName,
		testNamespace, sourceIPBy)

	if len(egrSvcNodeSelector) != 0 {
		By("Setting nodeSelector for EgressService")

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Configuring nodeSelector on EgressService %q in %q namespace",
			egrSVCBuilder.Definition.Name, egrSVCBuilder.Definition.Namespace)

		egrSVCBuilder = egrSVCBuilder.WithNodeLabelSelector(egrSvcNodeSelector)
	}

	if vrfNetwork != "" {
		By("Setting VRF network for EgressService")

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Configuring VRF network on EgressService %q in %q namespace",
			egrSVCBuilder.Definition.Name, egrSVCBuilder.Definition.Namespace)

		egrSVCBuilder = egrSVCBuilder.WithVRFNetwork(vrfNetwork)
	}

	By("Creating EgressService")

	egrSVCBuilder, err = egrSVCBuilder.Create()

	Expect(err).ToNot(HaveOccurred(), "Failed to create EgressService")

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Created EgressService %q in %q namespace",
		egrSVCBuilder.Object.Name, egrSVCBuilder.Object.Namespace)

	By("Getting status of EgressService")

	var ctx SpecContext

	Eventually(func() bool {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Check EgressService %q in %q namespace has host assigned",
			egrSVCBuilder.Definition.Name, egrSVCBuilder.Definition.Namespace)

		refreshEgressSVC, err := egrSVCBuilder.Get()
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to refresh egress service status")

			return false
		}

		if refreshEgressSVC.Status.Host == "" {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Host is not assigned to EgressService")

			return false
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("EgressService %q in %q namespace is assigned to %q",
			refreshEgressSVC.Name, refreshEgressSVC.Namespace, refreshEgressSVC.Status.Host)

		return true
	}).WithContext(ctx).WithPolling(15*time.Second).WithTimeout(3*time.Minute).Should(BeTrue(),
		"EgressService does not have node assigned")

	By("Finding pod from app deployment")

	clientPods := findPodWithSelector(testNamespace, svcSelector)

	Expect(clientPods).ToNot(BeNil(),
		fmt.Sprintf("Application pods matching %q label not found in %q namespace",
			testNamespace, svcSelector))

	By("Getting status of service")

	Eventually(func() bool {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Check service %q in %q namespace has LoadBalancer IP",
			svcBuilder.Definition.Name, svcBuilder.Definition.Namespace)

		refreshSVC := svcBuilder.Exists()

		if !refreshSVC {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to refresh service status")

			return false
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Service has %d IP addresses",
			len(svcBuilder.Object.Status.LoadBalancer.Ingress))

		return len(svcBuilder.Object.Status.LoadBalancer.Ingress) != 0
	}).WithContext(ctx).WithPolling(15*time.Second).WithTimeout(3*time.Minute).Should(BeTrue(),
		"Service does not have LoadBalancer IP address")

	for _, vip := range svcBuilder.Object.Status.LoadBalancer.Ingress {
		loadBalancerIP := vip.IP

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("LoadBalancer IP address: %q", loadBalancerIP)

		var (
			cmdToRun []string
		)

		myIP, err := netip.ParseAddr(loadBalancerIP)

		Expect(err).ToNot(HaveOccurred(), "Failed to parse IP address")

		if myIP.Is4() {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Processing IPv4 address")

			cmdToRun = []string{"/bin/bash", "-c",
				fmt.Sprintf("curl --connect-timeout 3 -Ls http://%s:%s/clientip",
					remoteTargetIP, remoteTargetPort)}
		} else {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Processing IPv6 address")

			cmdToRun = []string{"/bin/bash", "-c",
				fmt.Sprintf("curl --connect-timeout 3 -Ls http://[%s]:%s/clientip",
					remoteTargetIPv6, remoteTargetPort)}
		}

		if strings.EqualFold(strings.TrimSpace(string(egrSVCBuilder.Object.Spec.SourceIPBy)), "Network") {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Processing EgressService with sourceIPBy=%s",
				string(egrSVCBuilder.Object.Spec.SourceIPBy))

			verifyPodSourceAddress(clientPods, cmdToRun, "", RDSCoreConfig.EgressServiceNetworkExpectedIPs)
		} else {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Processing EgressService with sourceIPBy=%s",
				string(egrSVCBuilder.Object.Spec.SourceIPBy))

			verifyPodSourceAddress(clientPods, cmdToRun, loadBalancerIP, map[string]string{})
		}

		By(fmt.Sprintf("Accessing workload via LoadBalancer's IP %s", loadBalancerIP))

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Accessing  workload via LoadBalancer's IP %s", loadBalancerIP)

		verifyIngressIP(loadBalancerIP, "200", servicePort, true)
	}
}

// VerifyEgressServiceWithLocalETP verifies EgressService with externalTrafficPolicy set to Local.
func VerifyEgressServiceWithLocalETP(ctx SpecContext) {
	VerifyEgressServiceWithLocalETPWrapper(
		egressSVCDeploy2Name,
		RDSCoreConfig.EgressServiceDeploy2NodeSelector,
		RDSCoreConfig.EgressServiceNS,
		egressSVC2Name,
		egressSVC2Labels,
		RDSCoreConfig.EgressServiceDeploy2Image,
		RDSCoreConfig.EgressServiceDeploy2CMD,
		RDSCoreConfig.EgressServiceDeploy2IPAddrPool,
		"LoadBalancerIP",
		RDSCoreConfig.EgressServiceDeploy2NodeSelector,
		RDSCoreConfig.EgressServiceVRF2Network,
		RDSCoreConfig.EgressServiceRemoteIP,
		RDSCoreConfig.EgressServiceRemoteIPv6,
		RDSCoreConfig.EgressServiceRemotePort)
}

// VerifyEgressServiceWithLocalETPSourceIPByNetwork verifies EgressService with externalTrafficPolicy set to Local.
// sourceIPBy is set to "Network".
func VerifyEgressServiceWithLocalETPSourceIPByNetwork(ctx SpecContext) {
	VerifyEgressServiceWithLocalETPWrapper(
		egressSVCDeploy4Name,
		RDSCoreConfig.EgressServiceDeploy4NodeSelector,
		RDSCoreConfig.EgressServiceNS,
		egressSVC4Name,
		egressSVC4Labels,
		RDSCoreConfig.EgressServiceDeploy4Image,
		RDSCoreConfig.EgressServiceDeploy4CMD,
		RDSCoreConfig.EgressServiceDeploy4IPAddrPool,
		"Network",
		RDSCoreConfig.EgressServiceDeploy4NodeSelector,
		RDSCoreConfig.EgressServiceVRF4Network,
		RDSCoreConfig.EgressServiceRemoteIP,
		RDSCoreConfig.EgressServiceRemoteIPv6,
		RDSCoreConfig.EgressServiceRemotePort)
}

// VerifyEgressServiceWithLocalETPWrapper verifies EgressService with externalTrafficPolicy set to Local.
// sourceIPBy is customizable and accepts next values: "LoadBalancerIP" or "Network"
//
//nolint:funlen
func VerifyEgressServiceWithLocalETPWrapper(
	deployName string, // egressSVCDeploy2Name
	deployNodeSelector map[string]string, // RDSCoreConfig.EgressServiceDeploy2NodeSelector
	testNamespace string, // RDSCoreConfig.EgressServiceNS
	svcName string, // egressSVC2Name
	svcSelector string, // egressSVC2Labels
	containerImage string, // RDSCoreConfig.EgressServiceDeploy2Image
	containerCommand []string, // RDSCoreConfig.EgressServiceDeploy2CMD
	ipAddrPoolName string, // RDSCoreConfig.EgressServiceDeploy2IPAddrPool
	sourceIPBy string, // "LoadBalancer", "Network"
	egrSvcNodeSelector map[string]string, // RDSCoreConfig.EgressServiceDeploy2NodeSelector
	vrfNetwork string, // RDSCoreConfig.EgressServiceVRF2Network
	remoteTargetIP string, // RDSCoreConfig.EgressServiceRemoteIP
	remoteTargetIPv6 string, // RDSCoreConfig.EgressServiceRemoteIPv6
	remoteTargetPort string) {
	if strings.EqualFold(sourceIPBy, "Network") && len(RDSCoreConfig.EgressServiceNetworkExpectedIPs) == 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("EgressServiceNetworkExpectedIPs is not defined. Skipping")

		Skip("EgressServiceNetworkExpectedIPs is not defined. Skipping")
	}

	deleteDeployments(deployName, testNamespace)
	deleteService(svcName, testNamespace)
	deleteEgressService(svcName, testNamespace)

	waitForPodsGone(testNamespace, svcSelector)

	const (
		servicePort       int32 = 9090
		serviceTargetPort int32 = 9090
	)

	var ctx SpecContext

	podContainer := defineEgressSVCContainer(egressSVCContainer,
		containerImage, containerCommand)

	By("Reseting SecurityContext")

	podContainer = podContainer.WithSecurityContext(&corev1.SecurityContext{RunAsGroup: nil, RunAsUser: nil})

	cfgContainer, err := podContainer.GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to get container definition: %v", err))

	deployLabels := map[string]string{
		strings.Split(svcSelector, "=")[0]: strings.Split(svcSelector, "=")[1],
	}

	deploy := defineEgressSVCDeployment(cfgContainer, deployName,
		testNamespace, deployLabels, egrSvcNodeSelector, int32(1))

	Expect(deploy).ToNot(BeNil(), "Failed to create deployment")

	deploy, err = deploy.CreateAndWaitUntilReady(5 * time.Minute)

	Expect(err).ToNot(HaveOccurred(), "Deployment hasn't reached Ready status")

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deployment %q in %q namespace is Ready",
		deploy.Object.Name, deploy.Object.Namespace)

	By("Defining a ServicePort")

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Defining ServicePort object")

	svcPort, err := service.DefineServicePort(servicePort, serviceTargetPort, corev1.ProtocolTCP)

	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to define TargetPort: %v", err))

	By("Defining Service")

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Defining Service object")

	svcLabels := map[string]string{
		strings.Split(svcSelector, "=")[0]: strings.Split(svcSelector, "=")[1],
	}

	svcBuilder := defineService(svcName, testNamespace, svcLabels, *svcPort)

	Expect(svcBuilder).ToNot(BeNil(), "Failed to defined service")

	By("Setting ExternalTrafficPolicy to 'Local'")

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Setting Service's External Traffic Policy to 'Local'")

	svcBuilder.WithExternalTrafficPolicy(corev1.ServiceExternalTrafficPolicy(corev1.ServiceInternalTrafficPolicyLocal))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Service Definition:\n%#v\n", svcBuilder.Definition)

	By("Setting AddressPool annotation")

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Setting MetalLB address-pool annotation")

	svcBuilder = svcBuilder.WithAnnotation(map[string]string{
		"metallb.universe.tf/address-pool": ipAddrPoolName})

	By("Setting ipFamilyPolicy to 'RequireDualStack'")

	svcBuilder = svcBuilder.WithIPFamily([]corev1.IPFamily{"IPv4", "IPv6"},
		corev1.IPFamilyPolicyRequireDualStack)

	By("Creating a service")
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Creating Service object")

	svcBuilder, err = svcBuilder.Create()
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to create Service: %v", err))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Created Service: %#v", svcBuilder.Object)

	By("Defining EgressService")

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Defining EgressService %q in %q namespace",
		svcName, testNamespace)

	egrSVCBuilder := egressservice.NewEgressServiceBuilder(APIClient, svcName,
		testNamespace, sourceIPBy)

	if len(egrSvcNodeSelector) != 0 {
		By("Setting nodeSelector for EgressService")

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Configuring nodeSelector on EgressService %q in %q namespace",
			egrSVCBuilder.Definition.Name, egrSVCBuilder.Definition.Namespace)

		egrSVCBuilder = egrSVCBuilder.WithNodeLabelSelector(egrSvcNodeSelector)
	}

	if vrfNetwork != "" {
		By("Setting VRF network for EgressService")

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Configuring VRF network on EgressService %q in %q namespace",
			egrSVCBuilder.Definition.Name, egrSVCBuilder.Definition.Namespace)

		egrSVCBuilder = egrSVCBuilder.WithVRFNetwork(vrfNetwork)
	}

	By("Creating EgressService")

	egrSVCBuilder, err = egrSVCBuilder.Create()

	Expect(err).ToNot(HaveOccurred(), "Failed to create EgressService")

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Created EgressService %q in %q namespace",
		egrSVCBuilder.Object.Name, egrSVCBuilder.Object.Namespace)

	By("Getting status of EgressService")

	Eventually(func() bool {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Check EgressService %q in %q namespace has host assigned",
			egrSVCBuilder.Definition.Name, egrSVCBuilder.Definition.Namespace)

		refreshEgressSVC, err := egrSVCBuilder.Get()
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to refresh egress service status")

			return false
		}

		if refreshEgressSVC.Status.Host == "" {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Host is not assigned to EgressService")

			return false
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("EgressService %q in %q namespace is assigned to %q",
			refreshEgressSVC.Name, refreshEgressSVC.Namespace, refreshEgressSVC.Status.Host)

		return true
	}).WithContext(ctx).WithPolling(15*time.Second).WithTimeout(3*time.Minute).Should(BeTrue(),
		"EgressService does not have node assigned")

	By("Getting status of service")

	Eventually(func() bool {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Check service %q in %q namespace has LoadBalancer IP",
			svcBuilder.Definition.Name, svcBuilder.Definition.Namespace)

		refreshSVC := svcBuilder.Exists()

		if !refreshSVC {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to refresh service status")

			return false
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Service has %d IP addresses",
			len(svcBuilder.Object.Status.LoadBalancer.Ingress))

		return len(svcBuilder.Object.Status.LoadBalancer.Ingress) != 0
	}).WithContext(ctx).WithPolling(15*time.Second).WithTimeout(3*time.Minute).Should(BeTrue(),
		"Service does not have LoadBalancer IP address")

	By("Finding pod from app deployment")

	clientPods := findPodWithSelector(testNamespace, svcSelector)

	Expect(clientPods).ToNot(BeNil(),
		fmt.Sprintf("Application pods matching %q label not found in %q namespace",
			testNamespace, svcSelector))

	for _, vip := range svcBuilder.Object.Status.LoadBalancer.Ingress {
		loadBalancerIP := vip.IP

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("LoadBalancer IP address: %q", loadBalancerIP)

		var (
			cmdToRun   []string
			expectedIP string
		)

		myIP, err := netip.ParseAddr(loadBalancerIP)

		Expect(err).ToNot(HaveOccurred(), "Failed to parse IP address")

		if myIP.Is4() {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Processing IPv4 address")

			cmdToRun = []string{"/bin/bash", "-c",
				fmt.Sprintf("curl --connect-timeout 3 -Ls http://%s:%s/clientip",
					remoteTargetIP, remoteTargetPort)}

			expectedIP = remoteTargetIP
		} else {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Processing IPv6 address")

			cmdToRun = []string{"/bin/bash", "-c",
				fmt.Sprintf("curl --connect-timeout 3 -Ls http://[%s]:%s/clientip",
					remoteTargetIPv6, remoteTargetPort)}

			expectedIP = remoteTargetIPv6
		}

		if sourceIPBy == "LoadBalancerIP" {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Processing sourceIPBy %q", sourceIPBy)

			verifyPodSourceAddress(clientPods, cmdToRun, loadBalancerIP, map[string]string{})
		} else {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Processing sourceIPBy %q", sourceIPBy)

			verifyPodSourceAddress(clientPods, cmdToRun, "", RDSCoreConfig.EgressServiceNetworkExpectedIPs)
		}

		By(fmt.Sprintf("Accessing workload via LoadBalancer's IP %s", loadBalancerIP))

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Accessing  workload via LoadBalancer's IP %s", loadBalancerIP)

		verifyIngressIP(loadBalancerIP, expectedIP, servicePort, false)
	}
}

//nolint:funlen
func verifySourceIP(svcName, svcNS, podLabels string, cmdToRun []string, useIPv6 bool,
	expectedIPsMap map[string]string) {
	By(fmt.Sprintf("Pulling %q service configuration", svcName))

	var (
		svcBuilder    *service.Builder
		egrSVCBuilder *egressservice.EgressServiceBuilder
		err           error
		ctx           SpecContext
	)

	Eventually(func() bool {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pulling %q service from %q namespace",
			svcName, svcNS)

		svcBuilder, err = service.Pull(APIClient, svcName, svcNS)
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Error pulling %q service from %q namespace: %v",
				svcName, svcNS, err)

			return false
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Successfully pulled %q service from %q namespace",
			svcBuilder.Definition.Name, svcBuilder.Definition.Namespace)

		return true
	}).WithContext(ctx).WithPolling(5*time.Second).WithTimeout(1*time.Minute).Should(BeTrue(),
		fmt.Sprintf("Error obtaining service %q configuration", svcName))

	By(fmt.Sprintf("Asserting service %q has LoadBalancer IP address", svcName))

	Eventually(func() bool {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Check service %q in %q namespace has LoadBalancer IP",
			svcBuilder.Definition.Name, svcBuilder.Definition.Namespace)

		refreshSVC := svcBuilder.Exists()

		if !refreshSVC {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to refresh service status")

			return false
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Service has %d IP addresses",
			len(svcBuilder.Object.Status.LoadBalancer.Ingress))

		return len(svcBuilder.Object.Status.LoadBalancer.Ingress) != 0
	}).WithContext(ctx).WithPolling(15*time.Second).WithTimeout(3*time.Minute).Should(BeTrue(),
		"Service does not have LoadBalancer IP address")

	Eventually(func() bool {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pulling %q EgressService from %q namespace",
			svcName, svcNS)

		egrSVCBuilder, err = egressservice.Pull(APIClient, svcName, svcNS)
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Error pulling EgressService %q from %q namespace: %v",
				svcName, svcNS, err)

			return false
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Successfully pulled EgressService %q from %q namespace",
			egrSVCBuilder.Definition.Name, egrSVCBuilder.Definition.Namespace)

		return true
	}).WithContext(ctx).WithPolling(5*time.Second).WithTimeout(1*time.Minute).Should(BeTrue(),
		fmt.Sprintf("Error obtaining EgressService %q configuration", svcName))

	By("Finding pod from app deployment")

	clientPods := findPodWithSelector(svcNS, podLabels)

	Expect(clientPods).ToNot(BeNil(),
		fmt.Sprintf("Application pods matching %q label not found in %q namespace",
			svcName, svcNS))

	if strings.EqualFold(strings.TrimSpace(string(egrSVCBuilder.Object.Spec.SourceIPBy)), "LoadBalancerIP") {
		var trafficValidated bool

		By("Processing all LoadBalancer IP addresses")

		for _, vip := range svcBuilder.Object.Status.LoadBalancer.Ingress {
			loadBalancerIP := vip.IP

			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Processing %q address", loadBalancerIP)

			myIP, err := netip.ParseAddr(loadBalancerIP)

			Expect(err).ToNot(HaveOccurred(), "Failed to parse IP address")

			if myIP.Is4() && useIPv6 {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Skipping %q address as IPv6 is required",
					loadBalancerIP)

				continue
			}

			if myIP.Is6() && !useIPv6 {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Skipping %q address as IPv4 is required",
					loadBalancerIP)

				continue
			}

			trafficValidated = true

			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("LoadBalancer IP address: %q", loadBalancerIP)

			verifyPodSourceAddress(clientPods, cmdToRun, loadBalancerIP, map[string]string{})
		}

		Expect(trafficValidated).To(BeTrue(), "Traffic wasn't validated")
	} else {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Processing EgressService with sourceIPBy=Network")

		verifyPodSourceAddress(clientPods, cmdToRun, "", expectedIPsMap)
	}
}

// VerifyEgressServiceConnectivityETPCluster verifies source IP address when external traffic policy
// is set to Cluster.
func VerifyEgressServiceConnectivityETPCluster() {
	cmdToRun := []string{"/bin/bash", "-c",
		fmt.Sprintf("curl --connect-timeout 3 -Ls http://%s:%s/clientip",
			RDSCoreConfig.EgressServiceRemoteIP, RDSCoreConfig.EgressServiceRemotePort)}

	verifySourceIP(egressSVC1Name, RDSCoreConfig.EgressServiceNS, egressSVC1Labels, cmdToRun, false,
		RDSCoreConfig.EgressServiceNetworkExpectedIPs)

	cmdToRun = []string{"/bin/bash", "-c",
		fmt.Sprintf("curl --connect-timeout 3 -Ls http://[%s]:%s/clientip",
			RDSCoreConfig.EgressServiceRemoteIPv6, RDSCoreConfig.EgressServiceRemotePort)}

	verifySourceIP(egressSVC1Name, RDSCoreConfig.EgressServiceNS, egressSVC1Labels, cmdToRun, true,
		RDSCoreConfig.EgressServiceNetworkExpectedIPs)
}

// VerifyEgressServiceConnectivityETPClusterSourceIPByNetwork verifies source IP address when external traffic policy
// is set to Cluster.
func VerifyEgressServiceConnectivityETPClusterSourceIPByNetwork() {
	cmdToRun := []string{"/bin/bash", "-c",
		fmt.Sprintf("curl --connect-timeout 3 -Ls http://%s:%s/clientip",
			RDSCoreConfig.EgressServiceRemoteIP, RDSCoreConfig.EgressServiceRemotePort)}

	verifySourceIP(egressSVC3Name, RDSCoreConfig.EgressServiceNS, egressSVC3Labels, cmdToRun, false,
		RDSCoreConfig.EgressServiceNetworkExpectedIPs)

	cmdToRun = []string{"/bin/bash", "-c",
		fmt.Sprintf("curl --connect-timeout 3 -Ls http://[%s]:%s/clientip",
			RDSCoreConfig.EgressServiceRemoteIPv6, RDSCoreConfig.EgressServiceRemotePort)}

	verifySourceIP(egressSVC3Name, RDSCoreConfig.EgressServiceNS, egressSVC3Labels, cmdToRun, true,
		RDSCoreConfig.EgressServiceNetworkExpectedIPs)
}

// VerifyEgressServiceConnectivityETPLocal verifies source IP address when external traffic policy
// is set to Local.
func VerifyEgressServiceConnectivityETPLocal() {
	cmdToRun := []string{"/bin/bash", "-c",
		fmt.Sprintf("curl --connect-timeout 3 -Ls http://%s:%s/clientip",
			RDSCoreConfig.EgressServiceRemoteIP, RDSCoreConfig.EgressServiceRemotePort)}

	verifySourceIP(egressSVC2Name, RDSCoreConfig.EgressServiceNS, egressSVC2Labels, cmdToRun, false,
		RDSCoreConfig.EgressServiceNetworkExpectedIPs)

	cmdToRun = []string{"/bin/bash", "-c",
		fmt.Sprintf("curl --connect-timeout 3 -Ls http://[%s]:%s/clientip",
			RDSCoreConfig.EgressServiceRemoteIPv6, RDSCoreConfig.EgressServiceRemotePort)}

	verifySourceIP(egressSVC2Name, RDSCoreConfig.EgressServiceNS, egressSVC2Labels, cmdToRun, true,
		RDSCoreConfig.EgressServiceNetworkExpectedIPs)
}

// VerifyEgressServiceConnectivityETPLocalSourceIPByNetwork verifies source IP address when external traffic policy
// is set to Local and sourceIPBy=Network.
func VerifyEgressServiceConnectivityETPLocalSourceIPByNetwork() {
	cmdToRun := []string{"/bin/bash", "-c",
		fmt.Sprintf("curl --connect-timeout 3 -Ls http://%s:%s/clientip",
			RDSCoreConfig.EgressServiceRemoteIP, RDSCoreConfig.EgressServiceRemotePort)}

	verifySourceIP(egressSVC4Name, RDSCoreConfig.EgressServiceNS, egressSVC4Labels, cmdToRun, false,
		RDSCoreConfig.EgressServiceNetworkExpectedIPs)

	cmdToRun = []string{"/bin/bash", "-c",
		fmt.Sprintf("curl --connect-timeout 3 -Ls http://[%s]:%s/clientip",
			RDSCoreConfig.EgressServiceRemoteIPv6, RDSCoreConfig.EgressServiceRemotePort)}

	verifySourceIP(egressSVC4Name, RDSCoreConfig.EgressServiceNS, egressSVC4Labels, cmdToRun, true,
		RDSCoreConfig.EgressServiceNetworkExpectedIPs)
}

// VerifyEgressServiceETPLocalIngressConnectivity verifies ingress IP address while accessing backend pods
// via loadbalancer with ETP=Local.
func VerifyEgressServiceETPLocalIngressConnectivity() {
	verifyEgressServiceIngressConnectivit(egressSVC2Name, false)
}

// VerifyEgressServiceETPLocalSourceIPByNetworkIngressConnectivity verifies ingress IP address
// while accessing backend pods via loadbalancer with ETP=Local and sourceIPBy=Network.
func VerifyEgressServiceETPLocalSourceIPByNetworkIngressConnectivity() {
	verifyEgressServiceIngressConnectivit(egressSVC4Name, false)
}

// VerifyEgressServiceETPClusterIngressConnectivity verifies ingress IP address while accessing backend pods
// via loadbalancer with ETP=Cluster.
func VerifyEgressServiceETPClusterIngressConnectivity() {
	verifyEgressServiceIngressConnectivit(egressSVC1Name, true)
}

// VerifyEgressServiceETPClusterSourceIPByNetworkIngressConnectivity verifies ingress IP address
// while accessing backend pods via loadbalancer with ETP=Cluster and sourceIPBy=Network.
func VerifyEgressServiceETPClusterSourceIPByNetworkIngressConnectivity() {
	verifyEgressServiceIngressConnectivit(egressSVC3Name, true)
}

// verifyEgressServiceIngressConnectivit shared function to verify backend pods' availability via
// loadbalancer's IP address(es).
func verifyEgressServiceIngressConnectivit(svcName string, validateCode bool) {
	By(fmt.Sprintf("Pulling %q service configuration", svcName))

	var (
		svcBuilder *service.Builder
		err        error
		ctx        SpecContext
	)

	Eventually(func() bool {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pulling %q service from %q namespace",
			svcName, RDSCoreConfig.EgressServiceNS)

		svcBuilder, err = service.Pull(APIClient, svcName, RDSCoreConfig.EgressServiceNS)
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Error pulling %q service from %q namespace: %v",
				svcName, RDSCoreConfig.EgressServiceNS, err)

			return false
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Successfully pulled %q service from %q namespace",
			svcBuilder.Definition.Name, svcBuilder.Definition.Namespace)

		return true
	}).WithContext(ctx).WithPolling(5*time.Second).WithTimeout(1*time.Minute).Should(BeTrue(),
		fmt.Sprintf("Error obtaining service %q configuration", svcName))

	By(fmt.Sprintf("Asserting service %q has LoadBalancer IP address", svcBuilder.Definition.Name))

	Eventually(func() bool {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Check service %q in %q namespace has LoadBalancer IP",
			svcBuilder.Definition.Name, svcBuilder.Definition.Namespace)

		refreshSVC := svcBuilder.Exists()

		if !refreshSVC {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to refresh service status")

			return false
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Service has %d IP addresses",
			len(svcBuilder.Object.Status.LoadBalancer.Ingress))

		return len(svcBuilder.Object.Status.LoadBalancer.Ingress) != 0
	}).WithContext(ctx).WithPolling(15*time.Second).WithTimeout(3*time.Minute).Should(BeTrue(),
		"Service does not have LoadBalancer IP address")

	for _, vip := range svcBuilder.Object.Status.LoadBalancer.Ingress {
		loadBalancerIP := vip.IP

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Accessing  workload via LoadBalancer's IP %s", loadBalancerIP)

		myIP, err := netip.ParseAddr(loadBalancerIP)

		Expect(err).ToNot(HaveOccurred(), "Failed to parse IP address")

		var expectedResult string

		if myIP.Is4() {
			if validateCode {
				expectedResult = httpSuccessCode
			} else {
				expectedResult = RDSCoreConfig.EgressServiceRemoteIP
			}

			verifyIngressIP(loadBalancerIP, expectedResult, servicePort, validateCode)
		}

		if myIP.Is6() {
			if validateCode {
				expectedResult = httpSuccessCode
			} else {
				expectedResult = RDSCoreConfig.EgressServiceRemoteIPv6
			}

			verifyIngressIP(loadBalancerIP, expectedResult, servicePort, validateCode)
		}
	}
}

//nolint:unparam
func verifyIngressIP(loadBalancerIP, expectedIP string, servicePort int32, validateCode bool) {
	var (
		cmdResult []byte
		err       error
		ctx       SpecContext
	)

	By(fmt.Sprintf("Accessing backend pods via %s IP", loadBalancerIP))

	Eventually(func() bool {
		myIP, err := netip.ParseAddr(loadBalancerIP)

		Expect(err).ToNot(HaveOccurred(), "Failed to parse IP address")

		var noramlizedIP string

		if myIP.Is4() {
			noramlizedIP = loadBalancerIP
		}

		if myIP.Is6() {
			noramlizedIP = fmt.Sprintf("[%s]", loadBalancerIP)
		}

		var cmdExternal *exec.Cmd

		if validateCode {
			cmdExternal = exec.Command("curl", "--connect-timeout", "3", "-s",
				"-o", "/dev/null", "-w", "%{http_code}",
				fmt.Sprintf("http://%s:%d/clientip", noramlizedIP, servicePort))
		} else {
			cmdExternal = exec.Command("curl", "--connect-timeout", "3", "-s",
				fmt.Sprintf("http://%s:%d/clientip", noramlizedIP, servicePort))
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Running command: %q", cmdExternal.String())

		cmdResult, err = cmdExternal.CombinedOutput()
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Error running command: %v", err)

			return false
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Successfully executed command: %q",
			cmdExternal.String())

		return true
	}).WithContext(ctx).WithPolling(5*time.Second).WithTimeout(1*time.Minute).Should(
		BeTrue(), "Failed to executed command")

	By("Parsing command's output")

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Parsing command's output:\n\t%v(%v)\n",
		string(cmdResult), err)

	switch validateCode {
	case true:
		By("Comparing response code")

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Comparing response code %q with expected %q",
			string(cmdResult), expectedIP)

		Expect(string(cmdResult)).To(BeEquivalentTo(expectedIP),
			fmt.Sprintf("Wrong response code. Received %q, expected %q", string(cmdResult), expectedIP))
	case false:
		addr, _, err := net.SplitHostPort(string(cmdResult))

		By("Comparing ingress IP address")

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Comparing IP address %q with expected %q", addr, expectedIP)

		Expect(err).ToNot(HaveOccurred(), "Failed to parse Host/Port pairs from command's output")

		Expect(addr).To(BeEquivalentTo(expectedIP), "Wrong IP address used")
	}
}
