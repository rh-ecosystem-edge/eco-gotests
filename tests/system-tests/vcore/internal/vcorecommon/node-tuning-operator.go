package vcorecommon

import (
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/remote"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"

	"k8s.io/apimachinery/pkg/api/resource"

	tunedv1 "github.com/openshift/cluster-node-tuning-operator/pkg/apis/tuned/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clusteroperator"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/mco"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nto"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/service"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/apiobjectshelper"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/vcore/internal/vcoreparams"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	kubeletconfigv1beta1 "k8s.io/kubelet/config/v1beta1"

	v2 "github.com/openshift/cluster-node-tuning-operator/pkg/apis/performanceprofile/v2"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/vcore/internal/vcoreinittools"
)

var (
	workerLabelList = []string{VCoreConfig.VCorePpMCPName, VCoreConfig.VCoreCpMCPName}
)

// VerifyNTOSuite container that contains tests for Node Tuning Operator verification.
func VerifyNTOSuite() {
	Describe(
		"NTO validation",
		Label(vcoreparams.LabelVCoreOperators), func() {
			It(fmt.Sprintf("Verifies %s namespace exists", vcoreparams.NTONamespace),
				Label("nto"), VerifyNTONamespaceExists)

			It("Verify Node Tuning Operator successfully installed",
				Label("nto"), reportxml.ID("63656"), VerifyNTODeployment)

			It("Create new performanceprofile",
				Label("nto"), reportxml.ID("63741"), CreatePerformanceProfile)

			It("Create new nodes tuning",
				Label("nto"), reportxml.ID("63740"), CreateNodesTuning)

			It("Verify CPU Manager config",
				Label("nto"), reportxml.ID("63809"), VerifyCPUManagerConfig)

			It("Verify Node Tuning Operator Huge Pages configuration",
				Label("nto"), reportxml.ID("60062"), VerifyHugePagesConfig)

			It("Verify System Reserved memory for user-plane-worker nodes configuration",
				Label("nto"),
				reportxml.ID("60047"), SetSystemReservedMemoryForWorkers)
		})
}

// VerifyNTONamespaceExists asserts namespace for Node Tuning Operator exists.
func VerifyNTONamespaceExists(ctx SpecContext) {
	err := apiobjectshelper.VerifyNamespaceExists(APIClient, vcoreparams.NTONamespace, time.Second)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to pull namespace %q; %v",
		vcoreparams.NTONamespace, err))
} // func VerifyNTONamespaceExists (ctx SpecContext)

// VerifyNTODeployment asserts Node Tuning Operator successfully installed.
func VerifyNTODeployment(ctx SpecContext) {
	ntoServiceName := "node-tuning-operator"
	paoServiceName := "performance-addon-operator-service"

	err := apiobjectshelper.VerifyOperatorDeployment(APIClient,
		"",
		vcoreparams.NTODeploymentName,
		vcoreparams.NTONamespace,
		time.Minute)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("operator deployment %s failure in the namespace %s; %v",
			vcoreparams.NTODeploymentName, vcoreparams.NTONamespace, err))

	klog.V(vcoreparams.VCoreLogLevel).Infof(
		"Confirm that NTO %s pod was deployed and running in %s namespace",
		vcoreparams.NTODeploymentName, vcoreparams.NTONamespace)

	ntoPods, err := pod.ListByNamePattern(APIClient, vcoreparams.NTODeploymentName, vcoreparams.NTONamespace)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("No %s pods were found in %s namespace due to %v",
		vcoreparams.NTODeploymentName, vcoreparams.NTONamespace, err))
	Expect(len(ntoPods)).ToNot(Equal(0), fmt.Sprintf("The list of pods %s found in namespace %s is empty",
		vcoreparams.NTODeploymentName, vcoreparams.NTONamespace))

	ntoPod := ntoPods[0]
	ntoPodName := ntoPod.Object.Name

	err = ntoPod.WaitUntilReady(time.Second)
	if err != nil {
		ntoPodLog, _ := ntoPod.GetLog(3*time.Second, vcoreparams.NTODeploymentName)
		klog.Fatalf("%s pod in %s namespace in a bad state: %s",
			ntoPodName, vcoreparams.NTONamespace, ntoPodLog)
	}

	klog.V(vcoreparams.VCoreLogLevel).Infof("Verify NTO service %s created in the namespace %s",
		ntoServiceName, vcoreparams.NTONamespace)

	ntoService, err := service.Pull(APIClient, ntoServiceName, vcoreparams.NTONamespace)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to get service %s from the namespace %s due to %v",
		ntoServiceName, vcoreparams.NTONamespace, err))
	Expect(ntoService.Exists()).To(Equal(true), fmt.Sprintf("no service %s was found in the namespace %s",
		ntoServiceName, vcoreparams.NTONamespace))

	klog.V(vcoreparams.VCoreLogLevel).Infof("Verify NTO service %s created in the namespace %s",
		ntoServiceName, vcoreparams.NTONamespace)

	paoService, err := service.Pull(APIClient, paoServiceName, vcoreparams.NTONamespace)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to get service %s from the namespace %s due to %v",
		paoServiceName, vcoreparams.NTONamespace, err))
	Expect(paoService.Exists()).To(Equal(true), fmt.Sprintf("no service %s was found in the namespace %s",
		paoServiceName, vcoreparams.NTONamespace))
} // func VerifyNTODeployment (ctx SpecContext)

// CreatePerformanceProfile asserts performanceprofile can be created and successfully applied.
func CreatePerformanceProfile(ctx SpecContext) {
	for _, workerLabel := range workerLabelList {
		klog.V(vcoreparams.VCoreLogLevel).Infof("Verify performanceprofile %s exists", workerLabel)

		hugePagesNodeOne := int32(0)
		hugePagesNodeTwo := int32(1)
		hugePages := []v2.HugePage{
			{
				Size:  vcoreparams.HugePagesSize,
				Count: 32768,
				Node:  &hugePagesNodeOne,
			},
			{
				Size:  vcoreparams.HugePagesSize,
				Count: 32768,
				Node:  &hugePagesNodeTwo,
			},
		}
		performanceKubeletConfigName := fmt.Sprintf("performance-%s", workerLabel)
		// ppAnnotations := map[string]string{"performance.openshift.io/ignore-cgroups-version": "true",
		//	"kubeletconfig.experimental": fmt.Sprintf("{\"systemReserved\":{\"cpu\":\"%s\",\"memory\":\"%s\"}}",
		//		vcoreparams.SystemReservedCPU, vcoreparams.SystemReservedMemory)}
		ppAnnotations := map[string]string{
			"kubeletconfig.experimental": fmt.Sprintf("{\"systemReserved\":{\"cpu\":\"%s\",\"memory\":\"%s\"}}",
				vcoreparams.SystemReservedCPU, vcoreparams.SystemReservedMemory)}
		netInterfaceName := "ens2f(0|1)"
		netDevices := []v2.Device{{
			InterfaceName: &netInterfaceName,
		}}

		var err error

		nodeLabel := fmt.Sprintf("node-role.kubernetes.io/%s", workerLabel)
		nodeLabelMap := map[string]string{nodeLabel: ""}
		nodeLabelListOption := metav1.ListOptions{LabelSelector: nodeLabel}

		ppObj := nto.NewBuilder(APIClient,
			workerLabel,
			VCoreConfig.CPUIsolated,
			VCoreConfig.CPUReserved,
			nodeLabelMap).
			WithAnnotations(ppAnnotations).
			WithAdditionalKernelArgs([]string{fmt.Sprintf("nohz_full=%s", VCoreConfig.CPUIsolated)}).
			WithNet(true, netDevices).
			WithGloballyDisableIrqLoadBalancing().
			WithHugePages(vcoreparams.HugePagesSize, hugePages).
			WithNumaTopology(vcoreparams.TopologyConfig).
			WithWorkloadHints(false, true, false)

		if !ppObj.Exists() {
			klog.V(vcoreparams.VCoreLogLevel).Infof("Create new performanceprofile %s", workerLabel)

			_, err = ppObj.Create()
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to create performanceprofile %s; %v",
				workerLabel, err))

			klog.V(vcoreparams.VCoreLogLevel).Infof("Wait for all nodes rebooting after applying performanceprofile %s",
				workerLabel)

			_, err = nodes.WaitForAllNodesToReboot(
				APIClient,
				40*time.Minute,
				nodeLabelListOption)
			Expect(err).ToNot(HaveOccurred(),
				fmt.Sprintf("Nodes failed to reboot after applying performanceprofile %s config; %v",
					workerLabel, err))

			klog.V(vcoreparams.VCoreLogLevel).Info("Wait for all clusteroperators availability after nodes reboot")

			_, err = clusteroperator.WaitForAllClusteroperatorsAvailable(APIClient, 60*time.Second)
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error waiting for all available clusteroperators: %v",
				err))
		}

		klog.V(vcoreparams.VCoreLogLevel).Info("Verify NUMA Topology Manager")

		performanceKubeletConfigObj, err := mco.PullKubeletConfig(APIClient, performanceKubeletConfigName)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to get kubeletconfigs %s due to %v",
			performanceKubeletConfigName, err))

		kubeletConfig := performanceKubeletConfigObj.Object.Spec.KubeletConfig.Raw

		currentTopologyManagerPolicy :=
			unmarshalRaw[kubeletconfigv1beta1.KubeletConfiguration](kubeletConfig).TopologyManagerPolicy
		Expect(currentTopologyManagerPolicy).To(Equal(vcoreparams.TopologyConfig),
			fmt.Sprintf("incorrect topology manager policy found; expected: %s, found: %s",
				vcoreparams.TopologyConfig, currentTopologyManagerPolicy))
	}
} // func CreatePerformanceProfile (ctx SpecContext)

// CreateNodesTuning creates new Node Tuning configuration.
func CreateNodesTuning(ctx SpecContext) {
	for _, workerLabel := range workerLabelList {
		tunedInstanceName := fmt.Sprintf("configuration-nic-%s", workerLabel)
		tunedProfileData := fmt.Sprintf(
			"[main]\n"+
				"summary=Configuration changes profile inherited from performance created tuned\n\n"+
				"include=openshift-node-performance-%s\n"+
				"\n"+
				"[net]\n"+
				"type=net\n"+
				"devices_udev_regex=^INTERFACE=ens2f(0|1)\n"+
				"channels=combined 32",
			workerLabel)
		tunedProfile := tunedv1.TunedProfile{
			Name: &tunedInstanceName,
			Data: &tunedProfileData,
		}
		recommendPriority := uint64(19)
		recommendLabel := workerLabel
		tunedRecommend := tunedv1.TunedRecommend{
			Profile:  &tunedInstanceName,
			Priority: &recommendPriority,
			Match: []tunedv1.TunedMatch{{
				Label: &recommendLabel,
			}},
		}

		klog.V(vcoreparams.VCoreLogLevel).Infof("Verify nodes tuning %s already exists in namespace %s",
			workerLabel, vcoreparams.NTONamespace)

		var err error

		nodeLabel := fmt.Sprintf("node-role.kubernetes.io/%s", workerLabel)
		nodeLabelListOption := metav1.ListOptions{LabelSelector: nodeLabel}

		ntoObj := nto.NewTunedBuilder(APIClient,
			tunedInstanceName,
			vcoreparams.NTONamespace).
			WithProfile(tunedProfile).
			WithRecommend(tunedRecommend)

		if !ntoObj.Exists() {
			klog.V(vcoreparams.VCoreLogLevel).Infof("Create new node tuning profile %s in namespace %s",
				tunedInstanceName, vcoreparams.NTONamespace)

			ntoObj, err = ntoObj.Create()
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to create tuned %s in namespace %s; %v",
				tunedInstanceName, vcoreparams.NTONamespace, err))
			Expect(ntoObj.Exists()).To(Equal(true),
				fmt.Sprintf("tuned %s not found in namespace %s", tunedInstanceName, vcoreparams.NTONamespace))

			klog.V(vcoreparams.VCoreLogLevel).Info("The short sleep to update new values before the following change")

			time.Sleep(10 * time.Second)
		}

		nodesList, err := nodes.List(APIClient, nodeLabelListOption)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to get nodes list with the label %v; %v",
			nodeLabel, err))

		for _, node := range nodesList {
			klog.V(vcoreparams.VCoreLogLevel).Infof("Check nohz_full configured on the node %s",
				node.Definition.Name)

			var output string

			nohzFullCmd := []string{"chroot", "/rootfs", "/bin/sh", "-c", "cat /proc/cmdline"}

			output, err = remote.ExecuteOnNodeWithDebugPod(nohzFullCmd, node.Object.Name)
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to execute %s cmd on the node %s due to %v",
				nohzFullCmd, node.Object.Name, err))
			Expect(output).To(ContainSubstring("nohz_full"),
				fmt.Sprintf("nohz_full not found configured on the node %s; %s", node.Definition.Name, output))
		}
	}
} // func CreateNodesTuning (ctx SpecContext)

// VerifyCPUManagerConfig verifies CPU Manager configuration.
func VerifyCPUManagerConfig(ctx SpecContext) {
	for _, workerLabel := range workerLabelList {
		nodeLabel := fmt.Sprintf("node-role.kubernetes.io/%s", workerLabel)
		nodeLabelListOption := metav1.ListOptions{LabelSelector: nodeLabel}

		nodesList, err := nodes.List(APIClient, nodeLabelListOption)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to get nodes list with the label %v; %v",
			nodeLabel, err))

		klog.V(vcoreparams.VCoreLogLevel).Info("Verify CPU Manager configuration")

		cpuManagerCmd := []string{"chroot", "/rootfs", "/bin/sh", "-c",
			"sudo grep cpuManager /etc/kubernetes/kubelet.conf"}

		for _, node := range nodesList {
			klog.V(vcoreparams.VCoreLogLevel).Infof("Check CPU Manager activated on the node %s",
				node.Definition.Name)

			output, err := remote.ExecuteOnNodeWithDebugPod(cpuManagerCmd, node.Object.Name)
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to execute %s cmd on the node %s due to %v",
				cpuManagerCmd, node.Object.Name, err))
			Expect(output).To(ContainSubstring("cpuManagerPolicy"),
				fmt.Sprintf("failed to activate CPU Manager on the node %s; %v", node.Definition.Name, output))
		}
	}
} // func VerifyCPUManagerConfig (ctx SpecContext)

// VerifyHugePagesConfig verifies correctness of the Huge Pages configuration.
func VerifyHugePagesConfig(ctx SpecContext) {
	for _, workerLabel := range workerLabelList {
		nodeLabel := fmt.Sprintf("node-role.kubernetes.io/%s", workerLabel)
		nodeLabelListOption := metav1.ListOptions{LabelSelector: nodeLabel}

		nodesList, err := nodes.List(APIClient, nodeLabelListOption)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to get nodes list with the label %v; %v",
			nodeLabel, err))

		klog.V(vcoreparams.VCoreLogLevel).Info("Verify Node Tuning instance hugepages configuration")

		for _, node := range nodesList {
			klog.V(vcoreparams.VCoreLogLevel).Infof("Check hugepages config for the node %s",
				node.Definition.Name)

			allocatableHPResources := node.Object.Status.Allocatable
			capacityHPResources := node.Object.Status.Capacity

			for hpResource, value := range allocatableHPResources {
				if hpResource == "hugepages-2Mi" {
					Expect(value).To(Equal(resource.MustParse(vcoreparams.ExpectedHugePagesResource)),
						fmt.Sprintf("allocatable resource config on node %s not as expected; expected: %s, current: %v",
							node.Definition.Name, vcoreparams.ExpectedHugePagesResource, value))
				}
			}

			for hpResource, value := range capacityHPResources {
				if hpResource == "hugepages-2Mi" {
					Expect(value).To(Equal(resource.MustParse(vcoreparams.ExpectedHugePagesResource)),
						fmt.Sprintf("capacity resource config on node %s not as expected; expected: %s, current: %v",
							node.Definition.Name, vcoreparams.ExpectedHugePagesResource, value))
				}
			}
		}
	}
} // func VerifyHugePagesConfig (ctx SpecContext)

// SetSystemReservedMemoryForWorkers assert system reserved memory for user-plane-worker nodes succeeded.
func SetSystemReservedMemoryForWorkers(ctx SpecContext) {
	for _, workerLabel := range workerLabelList {
		nodeLabel := fmt.Sprintf("node-role.kubernetes.io/%s", workerLabel)
		nodeLabelListOption := metav1.ListOptions{LabelSelector: nodeLabel}

		klog.V(vcoreparams.VCoreLogLevel).Infof("Verify system reserved memory config for %s nodes succeeded",
			nodeLabel)

		kubeletConfigName := fmt.Sprintf("performance-%s", workerLabel)

		_, err := mco.PullKubeletConfig(APIClient, kubeletConfigName)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to get kubeletconfigs %s due to %v",
			kubeletConfigName, err))

		klog.V(vcoreparams.VCoreLogLevel).Infof("Verify system reserved data updated for all %s nodes",
			workerLabel)

		nodesList, err := nodes.List(APIClient, nodeLabelListOption)
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to get %v nodes list; %v", nodeLabel, err))

		systemReservedDataCmd := []string{"chroot", "/rootfs", "/bin/sh", "-c", "cat /etc/node-sizing.env"}
		for _, node := range nodesList {
			output, err := remote.ExecuteOnNodeWithDebugPod(systemReservedDataCmd, node.Object.Name)
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to execute %v cmd on the %s node due to %v",
				systemReservedDataCmd, workerLabel, err))
			Expect(output).To(ContainSubstring(fmt.Sprintf("SYSTEM_RESERVED_CPU=%s", vcoreparams.SystemReservedCPU)),
				fmt.Sprintf("reserved CPU configuration did not changed for the node %s; expected value: %s, "+
					"found: %v", node.Definition.Name, vcoreparams.SystemReservedCPU, output))
			Expect(output).To(ContainSubstring(fmt.Sprintf("SYSTEM_RESERVED_MEMORY=%s", vcoreparams.SystemReservedMemory)),
				fmt.Sprintf("reserved memory configuration did not changed for the node %s; expected value: %s, "+
					"found: %v", node.Definition.Name, vcoreparams.SystemReservedMemory, output))
		}
	}
} // func SetSystemReservedMemoryForWorkers (ctx SpecContext)

// unmarshalRaw converts raw bytes for a K8s CR into the actual type.
func unmarshalRaw[T any](raw []byte) T {
	untyped := &unstructured.Unstructured{}
	err := untyped.UnmarshalJSON(raw)
	Expect(err).ToNot(HaveOccurred(), "Failed to unmarshal JSON into unstructured")

	var typed T

	err = runtime.DefaultUnstructuredConverter.FromUnstructured(untyped.UnstructuredContent(), &typed)
	Expect(err).ToNot(HaveOccurred(), "Failed to convert unstructed to structured")

	return typed
}
