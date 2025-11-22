package vcorecommon

import (
	"fmt"
	apiUrl "net/url"
	"os"
	"regexp"
	"time"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/remote"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/bmh"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clusteroperator"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clusterversion"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/mco"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/vcore/internal/vcoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/vcore/internal/vcoreparams"
)

// VerifyInitialDeploymentConfig container that contains tests for initial cluster deployment verification.
func VerifyInitialDeploymentConfig() {
	Describe(
		"Initial deployment config validation",
		Label(vcoreparams.LabelVCoreDeployment), func() {
			It("Verifies healthy cluster status",
				Label("healthy-cluster"), reportxml.ID("59441"), VerifyHealthyClusterStatus)

			It("Asserts time sync was successfully applied for master nodes",
				Label("initial"), reportxml.ID("60028"), VerifyEtcChronyMasters)

			It("Asserts time sync was successfully applied for workers nodes",
				Label("initial"), reportxml.ID("60029"), VerifyEtcChronyWorkers)

			It("Verifies odf MCP was deployed",
				Label("initial"), reportxml.ID("73673"), VerifyODFMCPAvailability)

			It("Verifies full set of ODF nodes was deployed",
				Label("initial"), reportxml.ID("59442"), VerifyODFNodesAvailability)

			It("Verifies control-plane-worker MCP was deployed",
				Label("initial"), reportxml.ID("60049"), VerifyControlPlaneWorkerMCPAvailability)

			It("Verifies control-plane-worker nodes availability",
				Label("initial"), reportxml.ID("59505"), VerifyControlPlaneWorkerNodesAvailability)

			It("Verifies user-plane-worker MCP was deployed",
				Label("initial"), reportxml.ID("60050"), VerifyUserPlaneWorkerMCPAvailability)

			It("Verifies user-plane-worker nodes availability",
				Label("initial"), reportxml.ID("59506"), VerifyUserPlaneWorkerNodesAvailability)
		})
}

// VerifyHealthyClusterStatus asserts healthy cluster status.
func VerifyHealthyClusterStatus(ctx SpecContext) {
	klog.V(vcoreparams.VCoreLogLevel).Infof("Verify healthy cluster status")

	kubeConfigURL := os.Getenv("KUBECONFIG")

	klog.V(vcoreparams.VCoreLogLevel).Infof("Checking if API URL available")

	_, err := apiUrl.Parse(kubeConfigURL)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error getting API URL: %v", err))

	klog.V(vcoreparams.VCoreLogLevel).Infof("Checking if all BareMetalHosts in good OperationalState")

	var bmhList []*bmh.BmhBuilder

	bmhList, err = bmh.List(APIClient, vcoreparams.OpenshiftMachineAPINamespace)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error getting BareMetaHosts list: %v", err))
	Expect(len(bmhList)).ToNot(Equal(0), "Empty bareMetalHosts list received")

	_, err = bmh.WaitForAllBareMetalHostsInGoodOperationalState(APIClient,
		vcoreparams.OpenshiftMachineAPINamespace,
		5*time.Second)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Error waiting for all BareMetalHosts in good OperationalState: %v", err))

	klog.V(vcoreparams.VCoreLogLevel).Infof("Checking available control-plane nodes count")

	var nodesList []*nodes.Builder

	nodesList, err = nodes.List(APIClient, VCoreConfig.ControlPlaneLabelListOption)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to get master nodes list; %v", err))

	masterNodesCount := len(nodesList)
	Expect(masterNodesCount).To(Equal(3),
		fmt.Sprintf("Error in master nodes count; found master nodes count is %d", masterNodesCount))

	klog.V(vcoreparams.VCoreLogLevel).Infof("Checking all master nodes are Ready")

	var isReady bool

	isReady, err = nodes.WaitForAllNodesAreReady(
		APIClient,
		5*time.Second,
		VCoreConfig.ControlPlaneLabelListOption)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error getting master nodes list: %v", err))
	Expect(isReady).To(Equal(true),
		fmt.Sprintf("Failed master nodes status, not all Master node are Ready; %v", isReady))

	klog.V(vcoreparams.VCoreLogLevel).Infof("Checking that the clusterversion is available")

	_, err = clusterversion.Pull(APIClient)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error accessing csv: %v", err))

	klog.V(vcoreparams.VCoreLogLevel).Infof("Asserting clusteroperators availability")

	var coBuilder []*clusteroperator.Builder

	coBuilder, err = clusteroperator.List(APIClient)
	Expect(err).To(BeNil(), fmt.Sprintf("ClusterOperator List not found: %v", err))
	Expect(len(coBuilder)).ToNot(Equal(0), "Empty clusterOperators list received")

	_, err = clusteroperator.WaitForAllClusteroperatorsAvailable(APIClient, 60*time.Second)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Error waiting for all available clusteroperators: %v", err))
} // func VerifyHealthyClusterStatus (ctx SpecContext)

// verifyEtcChronyRun assert that time sync config was successfully applied and running.
func verifyEtcChronyRun(nodesRole string, nodeLabelOption metav1.ListOptions) {
	klog.V(vcoreparams.VCoreLogLevel).Infof("Asserts that time sync config was successfully applied "+
		"and running on %s nodes", nodesRole)

	isChronyApplied := false

	chronyConfigNameRegex := fmt.Sprintf("\\d+-%s\\w*-*\\w*-chrony-conf\\w*", nodesRole)
	mcp, err := mco.Pull(APIClient, nodesRole)
	Expect(err).To(BeNil(), fmt.Sprintf("%q MCP was not found", nodesRole))

	for _, source := range mcp.Object.Status.Configuration.Source {
		reg, _ := regexp.Compile(chronyConfigNameRegex)

		if reg.MatchString(source.Name) {
			isChronyApplied = true

			break
		}
	}

	Expect(isChronyApplied).To(BeTrue(), fmt.Sprintf("Error assert time sync was applied for %s nodes", nodesRole))

	klog.V(vcoreparams.VCoreLogLevel).Infof("Verify the chronyd status for %s nodes", nodesRole)

	chronydStatusCmd := []string{"chroot", "/rootfs", "/bin/sh", "-c", "sudo systemctl status chronyd | grep Active"}

	nodesList, err := nodes.List(APIClient, nodeLabelOption)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to get %s nodes list; %v", nodesRole, err))

	for _, node := range nodesList {
		output, err := remote.ExecuteOnNodeWithDebugPod(chronydStatusCmd, node.Object.Name)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to execute %v cmd on the %s node due to %v",
			chronydStatusCmd, nodesRole, err))
		Expect(output).To(ContainSubstring("running"),
			fmt.Sprintf("chronyd failed to start on the node %s, current status: %v",
				node.Definition.Name, output))
	}
}

// VerifyEtcChronyMasters assert that time sync config was successfully applied for master nodes.
func VerifyEtcChronyMasters(ctx SpecContext) {
	verifyEtcChronyRun(vcoreparams.MasterNodeRole, VCoreConfig.ControlPlaneLabelListOption)
} // func VerifyEtcChronyMasters (ctx SpecContext)

// VerifyEtcChronyWorkers assert that time sync config was successfully applied for workers nodes.
func VerifyEtcChronyWorkers(ctx SpecContext) {
	verifyEtcChronyRun(vcoreparams.WorkerNodeRole, metav1.ListOptions{LabelSelector: vcoreparams.WorkerNodeRole})
} // func VerifyEtcChronyWorkers (ctx SpecContext)

// verifyNodesAvailability assert MCP nodes availability.
func verifyNodesAvailability(nodesRole string, nodeLabelOption metav1.ListOptions) {
	klog.V(vcoreparams.VCoreLogLevel).Infof("Verify %s nodes availability", nodesRole)

	nodesList, err := nodes.List(APIClient, nodeLabelOption)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to get %s nodes list; %v", nodesRole, err))
	Expect(len(nodesList)).ToNot(Equal(0), fmt.Sprintf("%s nodes list is empty", nodesRole))
}

// verifyNodesInMCP assert MCP was deployed.
func verifyNodesInMCP(nodesRole string) {
	klog.V(vcoreparams.VCoreLogLevel).Infof("Verify %s MCP was deployed", nodesRole)

	mcp, err := mco.Pull(APIClient, nodesRole)
	Expect(err).To(BeNil(), fmt.Sprintf("%q MCP was not found", nodesRole))

	klog.V(vcoreparams.VCoreLogLevel).Infof("Checking %s MCP condition state", nodesRole)
	Expect(mcp.IsInCondition("Updated")).To(BeTrue(),
		fmt.Sprintf("%s MCP failed to update", nodesRole))
}

// VerifyODFMCPAvailability assert odf MCP was deployed.
func VerifyODFMCPAvailability(ctx SpecContext) {
	verifyNodesInMCP(VCoreConfig.OdfMCPName)
} // func VerifyODFMCPAvailability (ctx SpecContext)

// VerifyODFNodesAvailability assert full set of ODF nodes was deployed.
func VerifyODFNodesAvailability(ctx SpecContext) {
	verifyNodesAvailability(VCoreConfig.OdfMCPName, VCoreConfig.OdfLabelListOption)
} // func VerifyODFNodesAvailability (ctx SpecContext)

// VerifyControlPlaneWorkerMCPAvailability assert control-plane-worker MCP was deployed.
func VerifyControlPlaneWorkerMCPAvailability(ctx SpecContext) {
	verifyNodesInMCP(VCoreConfig.VCoreCpMCPName)
} // func VerifyControlPlaneWorkerMCPAvailability (ctx SpecContext)

// VerifyControlPlaneWorkerNodesAvailability assert control-plane-worker nodes availability.
func VerifyControlPlaneWorkerNodesAvailability(ctx SpecContext) {
	verifyNodesAvailability(VCoreConfig.VCoreCpMCPName, VCoreConfig.VCoreCpLabelListOption)
} // func VerifyControlPlaneWorkerNodesAvailability (ctx SpecContext)

// VerifyUserPlaneWorkerMCPAvailability assert user-plane-worker MCP was deployed.
func VerifyUserPlaneWorkerMCPAvailability(ctx SpecContext) {
	verifyNodesInMCP(VCoreConfig.VCorePpMCPName)
} // func VerifyUserPlaneWorkerMCPAvailability (ctx SpecContext)

// VerifyUserPlaneWorkerNodesAvailability assert user-plane-worker nodes availability.
func VerifyUserPlaneWorkerNodesAvailability(ctx SpecContext) {
	verifyNodesAvailability(VCoreConfig.VCorePpMCPName, VCoreConfig.VCorePpLabelListOption)
} // func VerifyUserPlaneWorkerNodesAvailability (ctx SpecContext)
