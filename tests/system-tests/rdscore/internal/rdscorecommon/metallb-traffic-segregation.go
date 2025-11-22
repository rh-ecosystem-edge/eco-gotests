package rdscorecommon

import (
	"context"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/await"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"k8s.io/apimachinery/pkg/util/wait"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/remote"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/supporttools"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreparams"
)

const (
	lbDeploymentLabel = "systemtest-test=rds-core-metal-app"
	stNamespace       = "rds-metallb-supporttools-ns"
	stDeploymentLabel = "rds-core=supporttools-deploy"
	codesPattern      = "200 404"
	captureScript     = `#!/bin/bash
tcpdump -vvv -e -nn -i %s | egrep -i 'Host:'
`
)

var (
	bgpOneAppURLIPv4 = fmt.Sprintf("http://%s:%s",
		RDSCoreConfig.MetalLBLoadBalancerOneIPv4, RDSCoreConfig.MetalLBTrafficSegregationTargetPort)
	bgpOneAppURLIPv6 = fmt.Sprintf("http://[%s]:%s",
		RDSCoreConfig.MetalLBLoadBalancerOneIPv6, RDSCoreConfig.MetalLBTrafficSegregationTargetPort)
	bgpTwoAppURLIPv4 = fmt.Sprintf("http://%s:%s",
		RDSCoreConfig.MetalLBLoadBalancerTwoIPv4, RDSCoreConfig.MetalLBTrafficSegregationTargetPort)
	bgpTwoAppURLIPv6 = fmt.Sprintf("http://[%s]:%s",
		RDSCoreConfig.MetalLBLoadBalancerTwoIPv6, RDSCoreConfig.MetalLBTrafficSegregationTargetPort)
	stDeploymentLabelList = metav1.ListOptions{
		LabelSelector: "rds-core=supporttools-deploy",
	}
)

// VerifyMetallbEgressTrafficSegregation test metallb egress traffic segregation.
//
//nolint:funlen
func VerifyMetallbEgressTrafficSegregation(ctx SpecContext) {
	By("Asserting if test URLs are provided")

	if RDSCoreConfig.MetalLBTrafficSegregationTargetOneIPv4 == "" &&
		RDSCoreConfig.MetalLBTrafficSegregationTargetOneIPv6 == "" {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Test URLs for MetalLB FRR testing not specified or are empty. Skipping...")
		Skip("Test URL for MetalLB FRR testing not specified or are empty")
	}

	if RDSCoreConfig.MetalLBTrafficSegregationTargetTwoIPv4 == "" &&
		RDSCoreConfig.MetalLBTrafficSegregationTargetTwoIPv6 == "" {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Test URLs for the second MetalLB FRR testing not specified or are empty. Skipping...")
		Skip("Test URL for the second MetalLB FRR testing not specified or are empty")
	}

	By("Randomly choosing node for the traceroute deployment")

	segregationTestNode, err := getNodeForTest(RDSCoreConfig.MetalLBFRRNamespace,
		rdscoreparams.MetalLBFRRPodSelector)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to retrieve testing node name: %v", err))

	By("Creating the packet traceroute deployment")

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Create traceroute deployment on the node %s",
		segregationTestNode)

	_, err =
		supporttools.CreateTraceRouteDeployment(
			APIClient,
			stNamespace,
			stDeploymentLabel,
			RDSCoreConfig.MetalLBSupportToolsImage,
			[]string{segregationTestNode})
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create traceroute deployment for node %v in namespace %s from image %s: %v",
			segregationTestNode, stNamespace, RDSCoreConfig.MetalLBSupportToolsImage, err))

	By("Finding traceroute pods")

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Searching for traceroute pods in %q namespace", stNamespace)

	stPodsList := findPodWithSelector(stNamespace, stDeploymentLabel)
	Expect(len(stPodsList)).ToNot(Equal(0), "No traceroute pods found in namespace %s",
		stNamespace)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Found %d 'traceroute' pods", len(stPodsList))

	for _, _pod := range stPodsList {
		By("Verify the correct route was used for the first BGP route")

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Verifying that the correct route was used "+
			"for the first BGP route %s/%s",
			RDSCoreConfig.MetalLBFRROneIPv4, RDSCoreConfig.MetalLBFRROneIPv6)

		for _, targetHostIP := range []string{RDSCoreConfig.MetalLBTrafficSegregationTargetOneIPv4,
			RDSCoreConfig.MetalLBTrafficSegregationTargetOneIPv6} {
			if targetHostIP == "" {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Empty target IP continue")

				continue
			}

			myIP, err := netip.ParseAddr(targetHostIP)
			Expect(err).ToNot(HaveOccurred(),
				fmt.Sprintf("Failed to parse host ip %s", targetHostIP))

			searchString := RDSCoreConfig.MetalLBFRROneIPv4

			if myIP.Is6() {
				searchString = RDSCoreConfig.MetalLBFRROneIPv6
			}

			err = supporttools.SendTrafficFindExpectedString(
				_pod,
				targetHostIP,
				RDSCoreConfig.MetalLBTrafficSegregationTargetPort,
				searchString)
			Expect(err).ToNot(HaveOccurred(),
				fmt.Sprintf("Failed to find required IP address %s in response: %v", searchString, err))
		}

		By("Verify the correct route was used for the second BGP route")

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Verifying that the correct route was used "+
			"for the second BGP route %s/%s", RDSCoreConfig.MetalLBFRRTwoIPv4,
			RDSCoreConfig.MetalLBFRRTwoIPv6)

		for _, targetHostIP := range []string{RDSCoreConfig.MetalLBTrafficSegregationTargetTwoIPv4,
			RDSCoreConfig.MetalLBTrafficSegregationTargetTwoIPv6} {
			if targetHostIP == "" {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Empty target IP continue")

				continue
			}

			myIP, err := netip.ParseAddr(targetHostIP)
			Expect(err).ToNot(HaveOccurred(),
				fmt.Sprintf("Failed to parse host ip %s", targetHostIP))

			searchString := RDSCoreConfig.MetalLBFRRTwoIPv4

			if myIP.Is6() {
				searchString = RDSCoreConfig.MetalLBFRRTwoIPv6
			}

			err = supporttools.SendTrafficFindExpectedString(
				_pod,
				targetHostIP,
				RDSCoreConfig.MetalLBTrafficSegregationTargetPort,
				searchString)
			Expect(err).ToNot(HaveOccurred(),
				fmt.Sprintf("Failed to find required IP address %s in response: %v", searchString, err))
		}
	}
}

// VerifyMetallbIngressTrafficSegregation test ingress connectivity to the workloads running on the cluster exposed
// via service(of type loadbalancer) with traffic segregation.
//
//nolint:funlen
func VerifyMetallbIngressTrafficSegregation(ctx SpecContext) {
	By("Asserting if test URLs are provided")

	var err error

	if RDSCoreConfig.MetalLBFRROneIPv4 == "" && RDSCoreConfig.MetalLBFRROneIPv6 == "" {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"First MetalLB FRR IP not specified or are empty. Skipping...")
		Skip("First MetalLB FRR IP not specified or are empty")
	}

	if RDSCoreConfig.MetalLBFRRTwoIPv4 == "" && RDSCoreConfig.MetalLBFRRTwoIPv6 == "" {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Second MetalLB FRR IP not specified or are empty. Skipping...")
		Skip("Second MetalLB FRR IP not specified or are empty")
	}

	if RDSCoreConfig.MetalLBTrafficSegregationTCPDumpIntOne == "" &&
		RDSCoreConfig.MetalLBTrafficSegregationTCPDumpIntTwo == "" {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"One of the capture interfaces not specified or are empty. Skipping...")
		Skip("One of the capture interfaces not specified or are empty")
	}

	By("Ensure LB application deployment created for the first BGP route")

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Searching for the metallb app pods in %q namespace with label %q",
		RDSCoreConfig.MetalLBLoadBalancerOneNamespace, lbDeploymentLabel)

	lbOneNodeName, err := getNodeForTest(RDSCoreConfig.MetalLBLoadBalancerOneNamespace, lbDeploymentLabel)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to retrieve testing LB one node name: %v", err))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("debug for the node selector: %s", lbOneNodeName)

	By("Ensure LB application deployment created for the second bgp route")

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Searching for the metallb app pods in %q namespace with label %q",
		RDSCoreConfig.MetalLBLoadBalancerTwoNamespace, lbDeploymentLabel)

	lbTwoNodeName, err := getNodeForTest(RDSCoreConfig.MetalLBLoadBalancerTwoNamespace, lbDeploymentLabel)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to retrieve testing LB two node name: %v", err))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("debug for the node selector: %s", lbTwoNodeName)

	By("Make sure that 1st external FRR container has learnt route to the 1st deployment")

	ipRouteList := []string{}

	if RDSCoreConfig.MetalLBLoadBalancerOneIPv4 != "" {
		ipRouteList = append(ipRouteList, fmt.Sprintf("%s/32", RDSCoreConfig.MetalLBLoadBalancerOneIPv4))
	}

	if RDSCoreConfig.MetalLBLoadBalancerOneIPv6 != "" {
		ipRouteList = append(ipRouteList, fmt.Sprintf("%s/128", RDSCoreConfig.MetalLBLoadBalancerOneIPv6))
	}

	for _, ipRoute := range ipRouteList {
		err := verifyIPRouteBGP(
			RDSCoreConfig.HypervisorHost,
			RDSCoreConfig.HypervisorUser,
			RDSCoreConfig.HypervisorPass,
			RDSCoreConfig.MetalLBFRRContainerNameOne,
			ipRoute)
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("first external FRR container %s route check for the %s failed: %v",
				RDSCoreConfig.MetalLBFRRContainerNameOne, ipRoute, err))
	}

	By("Make sure that 2nd external FRR container has learnt route to the 2nd deployment")

	ipRouteList = []string{}

	if RDSCoreConfig.MetalLBLoadBalancerTwoIPv4 != "" {
		ipRouteList = append(ipRouteList, fmt.Sprintf("%s/32", RDSCoreConfig.MetalLBLoadBalancerTwoIPv4))
	}

	if RDSCoreConfig.MetalLBLoadBalancerTwoIPv6 != "" {
		ipRouteList = append(ipRouteList, fmt.Sprintf("%s/128", RDSCoreConfig.MetalLBLoadBalancerTwoIPv6))
	}

	for _, ipRoute := range ipRouteList {
		err := verifyIPRouteBGP(
			RDSCoreConfig.HypervisorHost,
			RDSCoreConfig.HypervisorUser,
			RDSCoreConfig.HypervisorPass,
			RDSCoreConfig.MetalLBFRRContainerNameTwo,
			ipRoute)
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("second external FRR container %s route check for the %s failed: %v",
				RDSCoreConfig.MetalLBFRRContainerNameTwo, ipRoute, err))
	}

	By("Creating the tcpdump deployment for the first FRR packets capturing")

	var workerNodesList []string

	if len(RDSCoreConfig.FRRExpectedNodes) > 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Using expected nodes list %v", RDSCoreConfig.FRRExpectedNodes)
		workerNodesList = RDSCoreConfig.FRRExpectedNodes
	} else {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Trying to retrieve worker nodes names list from the label %v",
			RDSCoreConfig.WorkerLabelListOption)

		workerNodesList, err = getNodesNamesList(APIClient, RDSCoreConfig.WorkerLabelListOption)
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to retrieve worker nodes names list due to %v", err))

		Expect(len(workerNodesList)).ToNot(Equal(0), "No worker nodes found matching the label %v",
			RDSCoreConfig.WorkerLabelListOption)
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Create tcpdump deployment for the nodes %v", workerNodesList)

	_, err =
		supporttools.CreateTCPDumpDeployment(
			APIClient,
			stNamespace,
			stDeploymentLabel,
			RDSCoreConfig.MetalLBSupportToolsImage,
			RDSCoreConfig.MetalLBTrafficSegregationTCPDumpIntOne,
			captureScript,
			workerNodesList)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create tcpdump deployment for the node %v in namespace %s from image %s: %v",
			lbOneNodeName, stNamespace, RDSCoreConfig.MetalLBSupportToolsImage, err))

	By("Finding support-tools pods")

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Searching for tcpdump pods in %q namespace", stNamespace)

	stPodsFound, err := await.WaitForThePodReplicasCountInNamespace(
		APIClient,
		stNamespace,
		stDeploymentLabelList,
		len(workerNodesList),
		time.Second*30)
	Expect(err).ToNot(HaveOccurred(), "Failed to retrieve all tcpdump pods in namespace %s due to: %v",
		stNamespace, err)
	Expect(stPodsFound).To(Equal(true), "Not all tcpdump pods found in namespace %s", stNamespace)

	stPodsList := findPodWithSelector(stNamespace, stDeploymentLabel)
	Expect(len(stPodsList)).To(Equal(len(workerNodesList)), "Not all tcpdump pods found in namespace "+
		"%s: %v", stNamespace, stPodsList)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Found %d 'tcpdump' pods", len(stPodsList))

	By("Assert that 1st mockup app is reachable from the node with 1st FRR container")

	timeStart := time.Now()

	time.Sleep(time.Second * 5)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("start run traffic at %v", timeStart)

	appURLList := []string{}

	if RDSCoreConfig.MetalLBLoadBalancerOneIPv4 != "" {
		appURLList = append(appURLList, bgpOneAppURLIPv4)
	}

	if RDSCoreConfig.MetalLBLoadBalancerOneIPv6 != "" {
		appURLList = append(appURLList, bgpOneAppURLIPv6)
	}

	if len(appURLList) == 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"No app URLs to check. Skipping...")
		Skip("No app URLs to check")
	}

	for _, appURL := range appURLList {
		err := verifyAppIsReachableFromFRRContainer(
			RDSCoreConfig.HypervisorHost,
			RDSCoreConfig.HypervisorUser,
			RDSCoreConfig.HypervisorPass,
			RDSCoreConfig.MetalLBFRRContainerNameOne,
			appURL)
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("first mockup app %s is not reachable from the container %s: %v",
				appURL, RDSCoreConfig.MetalLBFRRContainerNameOne, err))
	}

	By("Check the traffic flows for the first FRR through the expected interface")

	var searchStringList []string

	if RDSCoreConfig.MetalLBLoadBalancerOneIPv4 != "" {
		searchStringList = append(searchStringList, RDSCoreConfig.MetalLBLoadBalancerOneIPv4)
	}

	if RDSCoreConfig.MetalLBLoadBalancerOneIPv6 != "" {
		searchStringList = append(searchStringList, RDSCoreConfig.MetalLBLoadBalancerOneIPv6)
	}

	for _, searchString := range searchStringList {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Making sure that the traffic flows from the first FRR "+
			"through the %s interface", RDSCoreConfig.MetalLBTrafficSegregationTCPDumpIntOne)

		err = scanTroughPodsList(stPodsList, searchString, timeStart)
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("expected string %s not found in tcpdump log: %v", searchString, err))
	}

	By("Creating the tcpdump deployment for the second FRR packets capturing")

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Create tcpdump deployment for the nodes %v", workerNodesList)

	_, err =
		supporttools.CreateTCPDumpDeployment(
			APIClient,
			stNamespace,
			stDeploymentLabel,
			RDSCoreConfig.MetalLBSupportToolsImage,
			RDSCoreConfig.MetalLBTrafficSegregationTCPDumpIntTwo,
			captureScript,
			workerNodesList)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create tcpdump deployment for the node %v in namespace %s from image %s: %v",
			lbTwoNodeName, stNamespace, RDSCoreConfig.MetalLBSupportToolsImage, err))

	By("Finding support-tools pods")

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Searching for tcpdump pods in %q namespace", stNamespace)

	stPodsFound, err = await.WaitForThePodReplicasCountInNamespace(
		APIClient,
		stNamespace,
		stDeploymentLabelList,
		len(workerNodesList),
		time.Second*30)
	Expect(err).ToNot(HaveOccurred(), "Failed to retrieve all tcpdump pods in namespace %s due to: %v",
		stNamespace, err)
	Expect(stPodsFound).To(Equal(true), "Not all tcpdump pods found in namespace %s", stNamespace)

	stPodsList = findPodWithSelector(stNamespace, stDeploymentLabel)
	Expect(len(stPodsList)).ToNot(Equal(0), "No tcpdump pods found in namespace %s",
		stNamespace)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Found %d 'tcpdump' pods", len(stPodsList))

	timeStart = time.Now()

	time.Sleep(time.Second * 5)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("start run traffic at %v", timeStart)

	By("Assert that 2nd mockup app is reachable from the node with 2nd FRR container")

	appURLList = []string{}

	if RDSCoreConfig.MetalLBLoadBalancerTwoIPv4 != "" {
		appURLList = append(appURLList, bgpTwoAppURLIPv4)
	}

	if RDSCoreConfig.MetalLBLoadBalancerTwoIPv6 != "" {
		appURLList = append(appURLList, bgpTwoAppURLIPv6)
	}

	if len(appURLList) == 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"No app URLs to check. Skipping...")
		Skip("No app URLs to check")
	}

	for _, appURL := range appURLList {
		err := verifyAppIsReachableFromFRRContainer(
			RDSCoreConfig.HypervisorHost,
			RDSCoreConfig.HypervisorUser,
			RDSCoreConfig.HypervisorPass,
			RDSCoreConfig.MetalLBFRRContainerNameTwo,
			appURL)
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("second mockup app %s is not reachable from the container %s: %v",
				appURL, RDSCoreConfig.MetalLBFRRContainerNameTwo, err))
	}

	By("Check the traffic flows for the second FRR through the expected interface")

	searchStringList = []string{}

	if RDSCoreConfig.MetalLBLoadBalancerTwoIPv4 != "" {
		searchStringList = append(searchStringList, RDSCoreConfig.MetalLBLoadBalancerTwoIPv4)
	}

	if RDSCoreConfig.MetalLBLoadBalancerTwoIPv6 != "" {
		searchStringList = append(searchStringList, RDSCoreConfig.MetalLBLoadBalancerTwoIPv6)
	}

	for _, searchString := range searchStringList {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Making sure that the traffic flows from the second FRR "+
			"through the %s interface", RDSCoreConfig.MetalLBTrafficSegregationTCPDumpIntTwo)

		err = scanTroughPodsList(stPodsList, searchString, timeStart)
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("expected string %s not found in tcpdump log: %v", searchString, err))
	}
}

// VerifyMetallbMockupAppNotReachableFromOtherFRR test mockup app is not reachable from the not correct FRR container.
//
//nolint:funlen
func VerifyMetallbMockupAppNotReachableFromOtherFRR(ctx SpecContext) {
	By("Asserting if test URLs are provided")

	if RDSCoreConfig.MetalLBFRROneIPv4 == "" && RDSCoreConfig.MetalLBFRROneIPv6 == "" {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"First MetalLB FRR IP not specified or are empty. Skipping...")
		Skip("First MetalLB FRR IP not specified or are empty")
	}

	if RDSCoreConfig.MetalLBFRRTwoIPv4 == "" && RDSCoreConfig.MetalLBFRRTwoIPv6 == "" {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Second MetalLB FRR IP not specified or are empty. Skipping...")
		Skip("Second MetalLB FRR IP not specified or are empty")
	}

	if RDSCoreConfig.MetalLBTrafficSegregationTCPDumpIntOne == "" &&
		RDSCoreConfig.MetalLBTrafficSegregationTCPDumpIntTwo == "" {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"One of the capture interfaces not specified or are empty. Skipping...")
		Skip("One of the capture interfaces not specified or are empty")
	}

	By("Make sure that 1st external FRR container has learnt route to the 1st deployment")

	ipRouteList := []string{}

	if RDSCoreConfig.MetalLBLoadBalancerOneIPv4 != "" {
		ipRouteList = append(ipRouteList, fmt.Sprintf("%s/32", RDSCoreConfig.MetalLBLoadBalancerOneIPv4))
	}

	if RDSCoreConfig.MetalLBLoadBalancerOneIPv6 != "" {
		ipRouteList = append(ipRouteList, fmt.Sprintf("%s/128", RDSCoreConfig.MetalLBLoadBalancerOneIPv6))
	}

	if len(ipRouteList) == 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"No IP routes to check. Skipping...")
		Skip("No IP routes to check")
	}

	for _, ipRoute := range ipRouteList {
		err := verifyIPRouteBGP(
			RDSCoreConfig.HypervisorHost,
			RDSCoreConfig.HypervisorUser,
			RDSCoreConfig.HypervisorPass,
			RDSCoreConfig.MetalLBFRRContainerNameOne,
			ipRoute)
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("first external FRR container %s route check for the %s failed: %v",
				RDSCoreConfig.MetalLBFRRContainerNameOne, ipRoute, err))
	}

	By("Make sure that 2nd external FRR container has learnt route to the 2nd deployment")

	ipRouteList = []string{}

	if RDSCoreConfig.MetalLBLoadBalancerTwoIPv4 != "" {
		ipRouteList = append(ipRouteList, fmt.Sprintf("%s/32", RDSCoreConfig.MetalLBLoadBalancerTwoIPv4))
	}

	if RDSCoreConfig.MetalLBLoadBalancerTwoIPv6 != "" {
		ipRouteList = append(ipRouteList, fmt.Sprintf("%s/128", RDSCoreConfig.MetalLBLoadBalancerTwoIPv6))
	}

	if len(ipRouteList) == 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"No IP routes to check. Skipping...")
		Skip("No IP routes to check")
	}

	for _, ipRoute := range ipRouteList {
		err := verifyIPRouteBGP(
			RDSCoreConfig.HypervisorHost,
			RDSCoreConfig.HypervisorUser,
			RDSCoreConfig.HypervisorPass,
			RDSCoreConfig.MetalLBFRRContainerNameTwo,
			ipRoute)
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("second external FRR container %s route check for the %s failed: %v",
				RDSCoreConfig.MetalLBFRRContainerNameTwo, ipRoute, err))
	}

	By("Assert that 1st mockup app is not reachable from the node with 2st FRR container")

	timeStart := time.Now()

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("start run traffic at %v", timeStart)

	appURLList := []string{}

	if RDSCoreConfig.MetalLBLoadBalancerOneIPv4 != "" {
		appURLList = append(appURLList, bgpOneAppURLIPv4)
	}

	if RDSCoreConfig.MetalLBLoadBalancerOneIPv6 != "" {
		appURLList = append(appURLList, bgpOneAppURLIPv6)
	}

	if len(appURLList) == 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"No app URLs to check. Skipping...")
		Skip("No app URLs to check")
	}

	for _, appURL := range appURLList {
		err := verifyAppIsReachableFromFRRContainer(
			RDSCoreConfig.HypervisorHost,
			RDSCoreConfig.HypervisorUser,
			RDSCoreConfig.HypervisorPass,
			RDSCoreConfig.MetalLBFRRContainerNameTwo,
			appURL)
		Expect(err).To(HaveOccurred(),
			fmt.Sprintf("first mockup app %s is reachable from the container %s: %v",
				appURL, RDSCoreConfig.MetalLBFRRContainerNameTwo, err))
	}

	By("Assert that 2nd mockup app is reachable from the node with 2nd FRR container")

	for _, appURL := range []string{bgpTwoAppURLIPv4, bgpTwoAppURLIPv6} {
		err := verifyAppIsReachableFromFRRContainer(
			RDSCoreConfig.HypervisorHost,
			RDSCoreConfig.HypervisorUser,
			RDSCoreConfig.HypervisorPass,
			RDSCoreConfig.MetalLBFRRContainerNameOne,
			appURL)
		Expect(err).To(HaveOccurred(),
			fmt.Sprintf("second mockup app %s is not reachable from the container %s: %v",
				appURL, RDSCoreConfig.MetalLBFRRContainerNameOne, err))
	}
}

func verifyIPRouteBGP(host, user, pass, containerName, lbIP string) error {
	var result string

	var err error

	klog.V(100).Infof("Verify IP version type")

	parsedIP := strings.Split(lbIP, "/")[0]

	klog.V(100).Infof("IP: %s", parsedIP)

	myIP, err := netip.ParseAddr(parsedIP)
	if err != nil {
		klog.V(100).Infof("Failed to parse provided loadbalancer ip address %q; %v", parsedIP, err)

		return fmt.Errorf("failed to parse provided loadbalancer ip address %q; %w", parsedIP, err)
	}

	showIPRouteBGPOnFRRContainerCmd := fmt.Sprintf("sudo podman exec -it %s bash "+
		"-c 'vtysh -c \"show ip route bgp\"'", containerName)

	if myIP.Is6() {
		showIPRouteBGPOnFRRContainerCmd = fmt.Sprintf("sudo podman exec -it %s bash "+
			"-c 'vtysh -c \"show ipv6 route bgp\"'", containerName)
	}

	klog.V(100).Infof("Running command %q from within container %s",
		showIPRouteBGPOnFRRContainerCmd, containerName)

	err = wait.PollUntilContextTimeout(
		context.TODO(),
		time.Second,
		time.Second*30,
		true,
		func(ctx context.Context) (bool, error) {
			result, err = remote.ExecCmdOnHost(host, user, pass, showIPRouteBGPOnFRRContainerCmd)
			if err != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to run command due to: %v", err)

				return false, nil
			}

			klog.V(100).Infof("BGP routes were found in the FRR container %s\\n: %s",
				containerName, result)

			return true, nil
		})
	if err != nil {
		klog.V(100).Infof("Failed to verify BGP routes in the FRR container %s: %v", containerName, err)

		return fmt.Errorf("failed to verify BGP routes in the FRR container %s: %w", containerName, err)
	}

	if !strings.Contains(result, lbIP) {
		klog.V(100).Infof("No BGP route %s in the FRR container %s was found", lbIP, containerName)

		return fmt.Errorf("no BGP route %s in the FRR container %s was found", lbIP, containerName)
	}

	return nil
}

func verifyAppIsReachableFromFRRContainer(host, user, pass, containerName, appURL string) error {
	var netNs, output string

	var err error

	getContainerNetNsCmd :=
		fmt.Sprintf("sudo podman container inspect --format \"{{.NetworkSettings.SandboxKey}}\" %s | xargs basename",
			containerName)

	klog.V(100).Infof("Running command %q from within container %s", getContainerNetNsCmd, containerName)

	err = wait.PollUntilContextTimeout(
		context.TODO(),
		time.Second,
		time.Second*3,
		true,
		func(ctx context.Context) (bool, error) {
			netNs, err = remote.ExecCmdOnHost(host, user, pass, getContainerNetNsCmd)
			if err != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to run command due to: %v", err)

				return false, nil
			}

			netNs = strings.TrimSpace(netNs)
			netNs = strings.Trim(netNs, "\n")

			klog.V(100).Infof("netns for the container %s successfully retrieved: %s",
				containerName, netNs)

			return true, nil
		})
	if err != nil {
		klog.V(100).Infof("Failed to retrieve netns for the FRR container %s: %v", containerName, err)

		return fmt.Errorf("failed to retrieve netns for the FRR container %s: %w", containerName, err)
	}

	if netNs == "" {
		klog.V(100).Infof("netns for the FRR container %s not found", containerName)

		return fmt.Errorf("netns for the FRR container %s not found", containerName)
	}

	verifyAppIsReachableCmd := fmt.Sprintf("sudo ip netns exec %s curl -Ls --max-time 5 -o /dev/null "+
		"-w '%%{http_code}' %s", netNs, appURL)

	klog.V(100).Infof("Running command %q from within netns %s of the container %s",
		verifyAppIsReachableCmd, netNs, containerName)

	err = wait.PollUntilContextTimeout(
		context.TODO(),
		time.Second,
		time.Second*3,
		true,
		func(ctx context.Context) (bool, error) {
			output, err = remote.ExecCmdOnHost(host, user, pass, verifyAppIsReachableCmd)
			if err != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to run command due to: %v", err)

				return false, nil
			}

			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Command's Output:\n%v\n", output)

			return true, nil
		})
	if err != nil {
		klog.V(100).Infof("URL %s is not reachable from the netns %s of the container %s; %v",
			appURL, netNs, containerName, err)

		return fmt.Errorf("the URL %s is not reachable from the netns %s of the container %s; %w",
			appURL, netNs, containerName, err)
	}

	if !strings.Contains(codesPattern, strings.Trim(output, "'")) {
		klog.V(100).Infof("received HTTP response not as expected; expected: %s, received: %s",
			codesPattern, output)

		return fmt.Errorf("received HTTP response not as expected; expected: %s, received: %s",
			codesPattern, output)
	}

	klog.V(100).Infof("URL %s is reachable from the netns %s of the container %s",
		appURL, netNs, containerName)

	return nil
}

func getNodeForTest(namespace, deploymentLabel string) (string, error) {
	klog.V(100).Infof("Searching for pods in %q namespace with label %q",
		namespace, deploymentLabel)

	podsList := findPodWithSelector(namespace, deploymentLabel)

	if len(podsList) == 0 {
		klog.V(100).Infof(
			"no pods with the label %s found deployed in namespace %s", deploymentLabel, namespace)

		return "", fmt.Errorf("no pods with the label %s found deployed in namespace %s",
			deploymentLabel, namespace)
	}

	if len(podsList) > 1 && deploymentLabel != rdscoreparams.MetalLBFRRPodSelector {
		klog.V(100).Infof("More than one app pod with label %s was found deployed: %d",
			deploymentLabel, len(podsList))

		return "", fmt.Errorf("more than one app pod with label %s was found deployed: %d",
			deploymentLabel, len(podsList))
	}

	klog.V(100).Infof("Getting node name for the app pod with label %s in namespace %s",
		deploymentLabel, namespace)

	for _, _pod := range podsList {
		if len(RDSCoreConfig.FRRExpectedNodes) != 0 {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("User specified list of FRR nodes present: %q",
				RDSCoreConfig.FRRExpectedNodes)

			if !slices.Contains(RDSCoreConfig.FRRExpectedNodes, _pod.Definition.Spec.NodeName) {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod %q(%q) runs on not user expected node %q. Skipping",
					_pod.Definition.Name, _pod.Definition.Namespace, _pod.Definition.Spec.NodeName)

				continue
			}

			return _pod.Definition.Spec.NodeName, nil
		}
	}

	return "", fmt.Errorf("no pods with the label %s found in namespace %s", deploymentLabel, namespace)
}

func getNodesNamesList(apiClient *clients.Settings, options metav1.ListOptions) ([]string, error) {
	klog.V(100).Infof("Building node names list for the nodes with the label %q", options.String())

	var nodeNamesList []string

	nodesList, err := nodes.List(apiClient, options)
	if err != nil {
		klog.V(100).Infof("Failed to retrieve %q nodes list; %v", options.String(), err)

		return nil, fmt.Errorf("failed to retrieve %q nodes list; %w", options.String(), err)
	}

	if len(nodesList) == 0 {
		klog.V(100).Infof("The %q nodes list is empty", options.String())

		return nil, fmt.Errorf("%q nodes list is empty", options.String())
	}

	for _, _node := range nodesList {
		nodeNamesList = append(nodeNamesList, _node.Definition.Name)
	}

	return nodeNamesList, nil
}

func scanTroughPodsList(podsList []*pod.Builder, searchString string, timeStart time.Time) error {
	var errorMsg error

	for _, _pod := range podsList {
		numberFound, logs, err := supporttools.ScanTCPDumpPodLogs(
			APIClient,
			_pod,
			stNamespace,
			stDeploymentLabel,
			searchString,
			timeStart)
		if err != nil {
			errorMsg = err
		}

		if numberFound != 0 {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("The searching string %s was found in log: %s",
				searchString, logs)

			return nil
		}
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to find string %s in tcpdump log results for the pods in "+
		"namespace %s: %v", searchString, stNamespace, errorMsg)

	return fmt.Errorf("failed to find string %s in tcpdump log results for the pods in namespace %s: %w",
		searchString, stNamespace, errorMsg)
}
