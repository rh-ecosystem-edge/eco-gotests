package vcorecommon

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/vcore/internal/ocpcli"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/remote"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clusterversion"

	operatorv1 "github.com/openshift/api/operator/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clusteroperator"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/mco"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/networkpolicy"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/scc"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/imageregistryconfig"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/vcore/internal/vcoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/vcore/internal/vcoreparams"
)

// VerifyPostDeploymentConfig container that contains tests for basic post-deployment config verification.
func VerifyPostDeploymentConfig() {
	Describe(
		"Post-deployment config validation",
		Label(vcoreparams.LabelVCoreDeployment), func() {
			It("Verifies Image Registry management state is Enabled",
				Label("day2"), reportxml.ID("72812"), VerifyImageRegistryManagementStateEnablement)

			It("Verifies network policy configuration procedure",
				Label("day2"), reportxml.ID("60086"), VerifyNetworkPolicyConfig)

			It("Verify scc activation succeeded",
				Label("day2"), reportxml.ID("60042"), VerifySCCActivation)

			It("Verifies sctp module activation succeeded",
				Label("day2"), reportxml.ID("60086"), VerifySCTPModuleActivation)

			It("Verifies system reserved memory for masters succeeded",
				Label("day2"), reportxml.ID("60045"), SetSystemReservedMemoryForMasterNodes)
		})
}

// VerifyImageRegistryManagementStateEnablement asserts imageRegistry managementState can be changed to the Managed.
func VerifyImageRegistryManagementStateEnablement(ctx SpecContext) {
	klog.V(vcoreparams.VCoreLogLevel).Infof("Enable local imageregistryconfig; change ManagementState to the Managed")

	err := imageregistryconfig.SetManagementState(APIClient, operatorv1.Managed)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to change imageRegistry state to the Managed; %v", err))

	klog.V(vcoreparams.VCoreLogLevel).Infof("Setup imageRegistry storage")

	err = imageregistryconfig.SetStorageToTheEmptyDir(APIClient)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to setup imageRegistry storage; %v", err))
} // func VerifyImageRegistryManagementStateEnablement (ctx SpecContext)

// VerifyNetworkPolicyConfig assert network policy configuration procedure.
func VerifyNetworkPolicyConfig(ctx SpecContext) {
	klog.V(vcoreparams.VCoreLogLevel).Infof("Verify network policy configuration procedure")

	for _, npNamespace := range vcoreparams.NetworkPoliciesNamespaces {
		nsBuilder := namespace.NewBuilder(APIClient, npNamespace)

		inNamespaceStr := fmt.Sprintf("in namespace %s", npNamespace)

		klog.V(vcoreparams.VCoreLogLevel).Infof("Create %s namespace", npNamespace)

		_, err := nsBuilder.Create()
		Expect(err).ToNot(HaveOccurred(), "failed to create namespace %s",
			npNamespace)

		klog.V(vcoreparams.VCoreLogLevel).Infof("Create networkpolicy %s %s",
			vcoreparams.MonitoringNetworkPolicyName, inNamespaceStr)

		npBuilder, err := networkpolicy.NewNetworkPolicyBuilder(APIClient,
			vcoreparams.MonitoringNetworkPolicyName, npNamespace).WithNamespaceIngressRule(
			vcoreparams.NetworkPolicyMonitoringNamespaceSelectorMatchLabels,
			nil).WithPolicyType(vcoreparams.NetworkPolicyType).Create()
		Expect(err).ToNot(HaveOccurred(), "failed to create networkpolicy %s %s",
			vcoreparams.MonitoringNetworkPolicyName, inNamespaceStr)

		klog.V(vcoreparams.VCoreLogLevel).Infof("Verify networkpolicy %s successfully created %s",
			vcoreparams.MonitoringNetworkPolicyName, inNamespaceStr)
		Expect(npBuilder.Exists()).To(Equal(true), "networkpolicy %s not found %s",
			vcoreparams.MonitoringNetworkPolicyName, inNamespaceStr)

		klog.V(vcoreparams.VCoreLogLevel).Infof("Create networkpolicy %s %s",
			vcoreparams.AllowAllNetworkPolicyName, inNamespaceStr)

		npBuilder, err = networkpolicy.NewNetworkPolicyBuilder(APIClient,
			vcoreparams.AllowAllNetworkPolicyName, npNamespace).
			WithPolicyType(vcoreparams.NetworkPolicyType).Create()
		Expect(err).ToNot(HaveOccurred(), "failed to create networkpolicy %s objects %s",
			vcoreparams.AllowAllNetworkPolicyName, inNamespaceStr)

		klog.V(vcoreparams.VCoreLogLevel).Infof("Verify networkpolicy %s successfully created %s",
			vcoreparams.MonitoringNetworkPolicyName, inNamespaceStr)
		Expect(npBuilder.Exists()).To(Equal(true), "networkpolicy %s not found %s",
			vcoreparams.AllowAllNetworkPolicyName, inNamespaceStr)
	}
} // func VerifyNetworkPolicyConfig (ctx SpecContext)

// VerifySCCActivation assert successfully scc activation.
func VerifySCCActivation(ctx SpecContext) {
	klog.V(vcoreparams.VCoreLogLevel).Infof("Verify scc activation succeeded")

	nodesList, err := nodes.List(APIClient, VCoreConfig.VCoreCpLabelListOption)
	Expect(err).ToNot(HaveOccurred(), "Failed to get control-plane-worker nodes list; %s", err)
	Expect(len(nodesList)).ToNot(Equal(0), "control-plane-worker nodes list is empty")

	sccBuilder := scc.NewBuilder(APIClient, vcoreparams.SccName, "RunAsAny", "MustRunAs").
		WithHostDirVolumePlugin(true).
		WithHostIPC(false).
		WithHostNetwork(false).
		WithHostPID(false).
		WithHostPorts(false).
		WithPrivilegedEscalation(true).
		WithPrivilegedContainer(true).
		WithAllowCapabilities(vcoreparams.CpSccAllowCapabilities).
		WithFSGroup("MustRunAs").
		WithFSGroupRange(1000, 1000).
		WithGroups(vcoreparams.CpSccGroups).
		WithPriority(nil).
		WithReadOnlyRootFilesystem(false).
		WithDropCapabilities(vcoreparams.CpSccDropCapabilities).
		WithSupplementalGroups("RunAsAny").
		WithVolumes(vcoreparams.CpSccVolumes)

	if !sccBuilder.Exists() {
		klog.V(vcoreparams.VCoreLogLevel).Infof("Create securityContextConstraints instance")

		sccObj, err := sccBuilder.Create()
		Expect(err).ToNot(HaveOccurred(), "Failed to create %s sccObj instance; %s",
			vcoreparams.SccName, err)
		Expect(sccObj.Exists()).To(Equal(true),
			"Failed to create %s SCC", vcoreparams.SccName)
	}
} // func VerifySCCActivation (ctx SpecContext)

// VerifySCTPModuleActivation assert successfully sctp module activation.
func VerifySCTPModuleActivation(ctx SpecContext) {
	klog.V(vcoreparams.VCoreLogLevel).Infof("Verify sctp module activation succeeded")

	nodesList, err := nodes.List(APIClient, VCoreConfig.VCoreCpLabelListOption)
	Expect(err).ToNot(HaveOccurred(), "Failed to get control-plane-worker nodes list; %s", err)
	Expect(len(nodesList)).ToNot(Equal(0), "control-plane-worker nodes list is empty")

	sctpBuilder := mco.NewMCBuilder(APIClient, vcoreparams.SctpModuleName)
	if !sctpBuilder.Exists() {
		klog.V(vcoreparams.VCoreLogLevel).Infof("Apply sctp config using shell method")

		sctpModuleTemplateName := "sctp-module.yaml"
		varsToReplace := make(map[string]interface{})
		varsToReplace["SctpModuleName"] = "load-sctp-module"
		varsToReplace["McNodeRole"] = vcoreparams.CpMCSelector

		workingDir, err := os.Getwd()
		Expect(err).ToNot(HaveOccurred(), err)

		templateDir := filepath.Join(workingDir, vcoreparams.TemplateFilesFolder)

		err = ocpcli.ApplyConfig(
			filepath.Join(templateDir, sctpModuleTemplateName),
			filepath.Join(vcoreparams.ConfigurationFolderPath, sctpModuleTemplateName),
			varsToReplace)
		Expect(err).To(BeNil(), fmt.Sprint(err))
		Expect(sctpBuilder.Exists()).To(Equal(true),
			fmt.Sprintf("Failed to create %s CRD", sctpModuleTemplateName))

		_, err = nodes.WaitForAllNodesToReboot(
			APIClient,
			20*time.Minute,
			VCoreConfig.VCoreCpLabelListOption)
		Expect(err).ToNot(HaveOccurred(), "Nodes failed to reboot after applying %s config; %s",
			sctpModuleTemplateName, err)
	}

	_, err = clusteroperator.WaitForAllClusteroperatorsAvailable(APIClient, 60*time.Second)
	Expect(err).ToNot(HaveOccurred(), "Error waiting for all available clusteroperators: %s", err)

	klog.V(vcoreparams.VCoreLogLevel).Infof("Verify SCTP was activated on each %s node", VCoreConfig.VCoreCpLabel)

	for _, node := range nodesList {
		checkCmd := []string{"chroot", "/rootfs", "/bin/sh", "-c", "lsmod | grep sctp"}

		output, err := remote.ExecuteOnNodeWithDebugPod(checkCmd, node.Object.Name)
		Expect(err).ToNot(HaveOccurred(), "Failed to execute command on node %s; %s",
			node.Object.Name, err)
		Expect(output).To(ContainSubstring("sctp"),
			"Failed to enable SCTP on %s node: %s", node.Object.Name, output)
	}
} // func VerifySCTPModuleActivation (ctx SpecContext)

// SetSystemReservedMemoryForMasterNodes assert system reserved memory for masters succeeded.
func SetSystemReservedMemoryForMasterNodes(ctx SpecContext) {
	klog.V(vcoreparams.VCoreLogLevel).Infof("Verify system reserved memory config for masters succeeded")

	kubeletConfigName := "set-sysreserved-master"
	systemReservedBuilder := mco.NewKubeletConfigBuilder(APIClient, kubeletConfigName).
		WithMCPoolSelector("pools.operator.machineconfiguration.openshift.io/master", "").
		WithSystemReserved(vcoreparams.SystemReservedCPU, vcoreparams.SystemReservedMemory)

	if !systemReservedBuilder.Exists() {
		klog.V(vcoreparams.VCoreLogLevel).Infof("Create system-reserved configuration")

		systemReserved, err := systemReservedBuilder.Create()
		Expect(err).ToNot(HaveOccurred(), "Failed to create %s kubeletConfig objects "+
			"with system-reserved definition", kubeletConfigName)

		_, err = nodes.WaitForAllNodesToReboot(
			APIClient,
			40*time.Minute,
			VCoreConfig.ControlPlaneLabelListOption)
		Expect(err).ToNot(HaveOccurred(), "Nodes failed to reboot after applying %s config; %s",
			kubeletConfigName, err)

		Expect(systemReserved.Exists()).To(Equal(true),
			"Failed to setup master system reserved memory, %s kubeletConfig not found; %s",
			kubeletConfigName, err)

		klog.V(vcoreparams.VCoreLogLevel).Infof("Checking all master nodes are Ready")

		var isReady bool

		isReady, err = nodes.WaitForAllNodesAreReady(
			APIClient,
			30*time.Second,
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
	}

	klog.V(vcoreparams.VCoreLogLevel).Infof("Verify system reserved data updated for all %s nodes",
		VCoreConfig.ControlPlaneLabel)

	nodesList, err := nodes.List(APIClient, VCoreConfig.ControlPlaneLabelListOption)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to get %s nodes list; %v",
		VCoreConfig.ControlPlaneLabel, err))

	systemReservedDataCmd := []string{"chroot", "/rootfs", "/bin/sh", "-c", "cat /etc/node-sizing.env"}
	for _, node := range nodesList {
		output, err := remote.ExecuteOnNodeWithDebugPod(systemReservedDataCmd, node.Object.Name)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to execute %v cmd on the %s node due to %v",
			systemReservedDataCmd, VCoreConfig.ControlPlaneLabel, err))
		Expect(output).To(ContainSubstring(fmt.Sprintf("SYSTEM_RESERVED_CPU=%s", vcoreparams.SystemReservedCPU)),
			fmt.Sprintf("reserved CPU configuration did not changed for the node %s; expected value: %s, "+
				"currently configured: %v", node.Definition.Name, vcoreparams.SystemReservedCPU, output))
		Expect(output).To(ContainSubstring(fmt.Sprintf("SYSTEM_RESERVED_MEMORY=%s", vcoreparams.SystemReservedMemory)),
			fmt.Sprintf("reserved memory configuration did not changed for the node %s; expected value: %s, "+
				"currently configured: %v", node.Definition.Name, vcoreparams.SystemReservedMemory, output))
	}
} // func SetSystemReservedMemoryForMasterNodes (ctx SpecContext)
