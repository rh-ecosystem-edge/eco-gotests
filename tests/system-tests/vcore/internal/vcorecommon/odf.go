package vcorecommon

import (
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/storage"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/remote"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/vcore/internal/ocpcli"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/console"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	ocsoperatorv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ocs/operatorv1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/klog/v2"
	"k8s.io/utils/strings/slices"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/apiobjectshelper"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/vcore/internal/vcoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/vcore/internal/vcoreparams"
)

var (
	odfNodesList           = []string{"worker-0", "worker-1", "worker-2"}
	odfConsolePlugin       = "odf-console"
	odfOperatorDeployments = []string{"csi-addons-controller-manager", "noobaa-operator",
		"ocs-operator", "odf-console", "odf-operator-controller-manager", "rook-ceph-operator"}

	odfStorageClassName = vcoreparams.StorageClassName
	volumeMode          = corev1.PersistentVolumeBlock
)

// VerifyODFSuite container that contains tests for ODF verification.
func VerifyODFSuite() {
	Describe(
		"ODF validation",
		Label(vcoreparams.LabelVCoreODF), func() {
			It(fmt.Sprintf("Verifies %s namespace exists", vcoreparams.ODFNamespace),
				Label("odf"), VerifyODFNamespaceExists)

			It("Verify ODF successfully installed",
				Label("odf"), reportxml.ID("63844"), VerifyODFOperatorDeployment)

			It("Verify ODF console enabled",
				Label("odf"), reportxml.ID("74917"), VerifyODFConsoleConfig)

			It("Apply taints to the ODF nodes",
				Label("odf"), reportxml.ID("74916"), VerifyODFTaints)

			It("Verify ODF operator StorageSystem configuration procedure",
				Label("odf"), reportxml.ID("59487"), VerifyODFStorageSystemConfig)

			It("Apply operators config for the ODF nodes",
				Label("odf"), reportxml.ID("74919"), VerifyOperatorsConfigForODFNodes)
		})
}

// VerifyODFNamespaceExists asserts namespace for ODF exists.
func VerifyODFNamespaceExists(ctx SpecContext) {
	err := apiobjectshelper.VerifyNamespaceExists(APIClient, vcoreparams.ODFNamespace, time.Second)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to pull namespace %q; %v",
		vcoreparams.ODFNamespace, err))
} // func VerifyODFNamespaceExists (ctx SpecContext)

// VerifyODFOperatorDeployment asserts ODF successfully installed.
func VerifyODFOperatorDeployment(ctx SpecContext) {
	for _, operatorPod := range odfOperatorDeployments {
		klog.V(vcoreparams.VCoreLogLevel).Infof("Confirm that odf %s pod was deployed and running in %s namespace",
			operatorPod, vcoreparams.ODFNamespace)

		odfPods, err := pod.ListByNamePattern(APIClient, operatorPod, vcoreparams.ODFNamespace)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("No %s pods were found in %s namespace; %v",
			operatorPod, vcoreparams.ODFNamespace, err))
		Expect(len(odfPods)).ToNot(Equal(0), fmt.Sprintf("No %s pods were found in %s namespace; %v",
			operatorPod, vcoreparams.ODFNamespace, err))

		odfPod := odfPods[0]
		odfPodName := odfPod.Object.Name

		err = odfPod.WaitUntilReady(30 * time.Second)
		if err != nil {
			odfPodLog, _ := odfPod.GetLog(600*time.Second, operatorPod)
			klog.Fatalf("%s pod in %s namespace in a bad state: %s",
				odfPodName, vcoreparams.ODFNamespace, odfPodLog)
		}
	}

	for _, operatorDeployment := range odfOperatorDeployments {
		klog.V(vcoreparams.VCoreLogLevel).Infof("Confirm that %s deployment is running in %s namespace",
			operatorDeployment, vcoreparams.ODFNamespace)

		odfDeployment, err := deployment.Pull(APIClient, operatorDeployment, vcoreparams.ODFNamespace)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("%s deployment not found in %s namespace; %v",
			operatorDeployment, vcoreparams.ODFNamespace, err))
		Expect(odfDeployment.IsReady(5*time.Second)).To(Equal(true),
			fmt.Sprintf("Bad state for %s deployment in %s namespace",
				operatorDeployment, vcoreparams.ODFNamespace))
	}
} // func VerifyODFOperatorDeployment (ctx SpecContext)

// VerifyODFConsoleConfig asserts ODF console enabled.
func VerifyODFConsoleConfig(ctx SpecContext) {
	klog.V(vcoreparams.VCoreLogLevel).Infof("Enable odf-console")

	consoleOperatorObj, err := console.PullConsoleOperator(APIClient, "cluster")
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("consoleOperator 'cluster' not found; %v", err))

	consoleOperatorObj, err = consoleOperatorObj.WithPlugins([]string{odfConsolePlugin}, false).Update()
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to add plugins %v to the consoleOperator 'cluster' "+
		"due to: %v", odfConsolePlugin, err))

	newPluginsList, err := consoleOperatorObj.GetPlugins()
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to get plugins value from the consoleOperator "+
		"'cluster'; %v", err))
	Expect(slices.Contains(*newPluginsList, odfConsolePlugin),
		fmt.Sprintf("Failed to add new plugin %s to the consoleOperator plugins list: %v",
			odfConsolePlugin, newPluginsList))

	klog.V(vcoreparams.VCoreLogLevel).Infof("Wait for the console enablement")
	time.Sleep(7 * time.Minute)
} // func VerifyODFConsoleConfig (ctx SpecContext)

// VerifyODFTaints asserts ODF nodes taints configuration.
func VerifyODFTaints(ctx SpecContext) {
	klog.V(vcoreparams.VCoreLogLevel).Infof("Apply taints to the ODF nodes")

	for _, odfNodeName := range odfNodesList {
		odfNode, err := nodes.Pull(APIClient, odfNodeName)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to retrieve node %s object due to %v",
			odfNodeName, err))

		klog.V(vcoreparams.VCoreLogLevel).Infof("Ensure taints applyed to the %s node", odfNode.Definition.Name)
		applyTaintsCmd := fmt.Sprintf(
			"oc adm taint node %s node.ocs.openshift.io/storage=true:NoSchedule --overwrite=true --kubeconfig=%s",
			odfNode.Definition.Name, VCoreConfig.KubeconfigPath)
		_, err = remote.ExecCmdOnHost(VCoreConfig.Host, VCoreConfig.User, VCoreConfig.Pass, applyTaintsCmd)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to execute %s script due to %v",
			applyTaintsCmd, err))
	}

	time.Sleep(3 * time.Minute)
} // func VerifyODFTaints (ctx SpecContext)

// VerifyODFStorageSystemConfig asserts ODF storage cluster system successfully configured.
func VerifyODFStorageSystemConfig(ctx SpecContext) {
	klog.V(vcoreparams.VCoreLogLevel).Infof("Cleanup StorageSystem and StorageCluster config")

	storageSystemObj := storage.NewSystemODFBuilder(APIClient,
		vcoreparams.ODFStorageSystemName, vcoreparams.ODFNamespace)

	if storageSystemObj.Exists() {
		err := storageSystemObj.Delete()
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("failed to delete ODF StorageSystem %s from namespace %s; %v",
				vcoreparams.ODFStorageSystemName, vcoreparams.ODFNamespace, err))
	}

	storageclusterObj := storage.NewStorageClusterBuilder(APIClient,
		vcoreparams.StorageClusterName, vcoreparams.ODFNamespace)

	if storageclusterObj.Exists() {
		err := storageclusterObj.Delete()
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("failed to delete ODF StorageCluster %s from namespace %s; %v",
				vcoreparams.StorageClusterName, vcoreparams.ODFNamespace, err))
	}

	klog.V(vcoreparams.VCoreLogLevel).Infof("Start to configure ODF StorageSystem")

	_, err := storageSystemObj.WithSpec("storagecluster.ocs.openshift.io/v1",
		vcoreparams.StorageClusterName, vcoreparams.ODFNamespace).Create()
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to create storageSystem %s instance in %s namespace; "+
		"%v", vcoreparams.ODFStorageSystemName, vcoreparams.ODFNamespace, err))

	klog.V(vcoreparams.VCoreLogLevel).Infof("Start to configure ODF StorageCluster")

	annotations := map[string]string{
		"cluster.ocs.openshift.io/local-devices":    "true",
		"uninstall.ocs.openshift.io/cleanup-policy": "delete",
		"uninstall.ocs.openshift.io/mode":           "graceful",
	}

	managedResources := ocsoperatorv1.ManagedResourcesSpec{
		CephBlockPools: ocsoperatorv1.ManageCephBlockPools{
			ReconcileStrategy: "managed",
		},
		CephFilesystems: ocsoperatorv1.ManageCephFilesystems{
			ReconcileStrategy: "managed",
		},
		CephObjectStoreUsers: ocsoperatorv1.ManageCephObjectStoreUsers{
			ReconcileStrategy: "managed",
		},
		CephObjectStores: ocsoperatorv1.ManageCephObjectStores{
			ReconcileStrategy: "managed",
		},
	}

	resourceListMap := make(map[corev1.ResourceName]resource.Quantity)
	resourceListMap[corev1.ResourceStorage] = resource.MustParse("1")

	storageDeviceSet := ocsoperatorv1.StorageDeviceSet{
		Count:    3,
		Replica:  1,
		Portable: false,
		Name:     vcoreparams.ODFLocalVolumeSetName,
		DataPVCTemplate: corev1.PersistentVolumeClaim{
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.VolumeResourceRequirements{
					Requests: resourceListMap,
				},
				StorageClassName: &odfStorageClassName,
				VolumeMode:       &volumeMode,
			},
		},
	}

	storageclusterObj, err = storageclusterObj.
		WithAnnotations(annotations).
		WithManagedResources(managedResources).
		WithMonDataDirHostPath("/var/lib/rook").
		WithStorageDeviceSet(storageDeviceSet).Create()
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to create storageCluster %s instance in namespace %s; "+
		"%v", vcoreparams.StorageClusterName, vcoreparams.ODFNamespace, err))
	Expect(storageclusterObj.Exists()).To(Equal(true),
		fmt.Sprintf("Failed to createstorageCluster %s instance in namespace %s due",
			vcoreparams.StorageClusterName, vcoreparams.ODFNamespace))

	time.Sleep(5 * time.Minute)
} // func VerifyODFStorageSystemConfig (ctx SpecContext)

// VerifyOperatorsConfigForODFNodes asserts operators configuration for ODF nodes.
func VerifyOperatorsConfigForODFNodes(ctx SpecContext) {
	klog.V(vcoreparams.VCoreLogLevel).Infof("Configuring operators for ODF nodes")

	klog.V(vcoreparams.VCoreLogLevel).Infof("DNS operator")

	patchStr :=
		"spec:\n" +
			"  nodePlacement:\n" +
			"    tolerations:\n" +
			"    - operator: Exists\n"

	// patchStr := `{"spec": {"nodePlacement": {"tolerations": ["operator": "Exists"]}}}`

	err := ocpcli.PatchAPIObject("default", "openshift-dns",
		"dns.operator", "merge", patchStr)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to patch dns.operator default in namespace openshift-dns"+
		" due to %v", err))

	klog.V(vcoreparams.VCoreLogLevel).Infof("Configuring selectors and tolerations for ODF components")

	rookCephConfigMap, err := configmap.Pull(APIClient, vcoreparams.RookCephConfigMapName, vcoreparams.ODFNamespace)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to retrieve configmap %s from "+
		"namespace %s due to %v", vcoreparams.RookCephConfigMapName, vcoreparams.ODFNamespace, err))

	rookData := map[string]string{
		"CSI_PLUGIN_TOLERATIONS": "" +
			"- operator: \"Exists\"\n" +
			"  key: node-role.kubernetes.io/infra\n" +
			"- key: node.ocs.openshift.io/storage\n" +
			"  operator: Equal\n" +
			"  value: \"true\"\n" +
			"  effect: NoSchedule",
		"CSI_PROVISIONER_TOLERATIONS": "" +
			"- key: node.ocs.openshift.io/storage\n" +
			"  operator: Equal\n" +
			"  value: \"true\"\n" +
			"  effect: NoSchedule\n",
		"CSI_PROVISIONER_NODE_AFFINITY": "" +
			"requiredDuringSchedulingIgnoredDuringExecution:\n" +
			"  nodeSelectorTerms:\n" +
			"  - matchExpressions:\n" +
			"    - key: cluster.ocs.openshift.io/openshift-storage\n" +
			"      operator: Exists",
		"CSI_CEPHFS_PROVISIONER_NODE_AFFINITY": "" +
			"requiredDuringSchedulingIgnoredDuringExecution:\n" +
			"  nodeSelectorTerms:\n" +
			"  - matchExpressions:\n" +
			"    - key: cluster.ocs.openshift.io/openshift-storage\n" +
			"      operator: Exists",
		"CSI_NFS_PROVISIONER_NODE_AFFINITY": "" +
			"requiredDuringSchedulingIgnoredDuringExecution:\n" +
			"  nodeSelectorTerms:\n" +
			"  - matchExpressions:\n" +
			"    - key: cluster.ocs.openshift.io/openshift-storage\n" +
			"      operator: Exists",
		"CSI_RBD_PROVISIONER_NODE_AFFINITY": "" +
			"requiredDuringSchedulingIgnoredDuringExecution:\n" +
			"  nodeSelectorTerms:\n" +
			"  - matchExpressions:\n" +
			"    - key: cluster.ocs.openshift.io/openshift-storage\n" +
			"      operator: Exists",
	}

	_, err = rookCephConfigMap.WithData(rookData).Update()
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to update configMap %s in namespace %s due to %v",
		vcoreparams.RookCephConfigMapName, vcoreparams.ODFNamespace, err))

	klog.V(vcoreparams.VCoreLogLevel).Info("Restarting storage pods")

	restartStoragePodsCmd := fmt.Sprintf("oc delete pods -n %s --all --kubeconfig=%s",
		vcoreparams.ODFNamespace, VCoreConfig.KubeconfigPath)
	_, err = remote.ExecCmdOnHost(VCoreConfig.Host, VCoreConfig.User, VCoreConfig.Pass, restartStoragePodsCmd)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to execute %s script due to %v",
		restartStoragePodsCmd, err))

	_, err = pod.WaitForAllPodsInNamespaceRunning(APIClient, vcoreparams.ODFNamespace, 15*time.Minute)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("not all storage pods in namespace %s succeeded to "+
		"recover after deletion due to %v", vcoreparams.ODFNamespace, err))

	time.Sleep(10 * time.Minute)
} // func VerifyOperatorsConfigForODFNodes (ctx SpecContext)
