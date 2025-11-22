package vcorecommon

import (
	"fmt"
	"time"

	lsov1 "github.com/openshift/local-storage-operator/api/v1"
	lsov1alpha1 "github.com/openshift/local-storage-operator/api/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/lso"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/await"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/apiobjectshelper"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/vcore/internal/vcoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/vcore/internal/vcoreparams"
)

// VerifyLSOSuite container that contains tests for LSO verification.
func VerifyLSOSuite() {
	Describe(
		"LSO validation",
		Label(vcoreparams.LabelVCoreLSO), func() {
			It(fmt.Sprintf("Verifies %s namespace exists", vcoreparams.LSONamespace),
				Label("lso"), VerifyLSONamespaceExists)

			It("Verify Local Storage Operator successfully installed",
				Label("lso"), reportxml.ID("59491"), VerifyLSODeployment)

			It("Verify localvolumeset instance exists",
				Label("lso"), reportxml.ID("74918"), VerifyLocalVolumeSet)
		})
}

// VerifyLSONamespaceExists asserts namespace for Local Storage Operator exists.
func VerifyLSONamespaceExists(ctx SpecContext) {
	err := apiobjectshelper.VerifyNamespaceExists(APIClient, vcoreparams.LSONamespace, time.Second)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to pull namespace %q; %v", vcoreparams.LSONamespace, err))

	klog.V(vcoreparams.VCoreLogLevel).Infof("Short sleep to stabilize nodes to avoid pv creation failure")

	time.Sleep(15 * time.Minute)
} // func VerifyLSONamespaceExists (ctx SpecContext)

// VerifyLSODeployment asserts Local Storage Operator successfully installed.
func VerifyLSODeployment(ctx SpecContext) {
	err := apiobjectshelper.VerifyOperatorDeployment(APIClient,
		vcoreparams.LSOName,
		vcoreparams.LSOName,
		vcoreparams.LSONamespace,
		time.Minute)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("operator deployment %s failure in the namespace %s; %v",
			vcoreparams.LSOName, vcoreparams.LSONamespace, err))

	klog.V(vcoreparams.VCoreLogLevel).Infof("Confirm that LSO %s pod was deployed and running in %s namespace",
		vcoreparams.LSOName, vcoreparams.LSONamespace)

	lsoPods, err := pod.ListByNamePattern(APIClient, vcoreparams.LSOName, vcoreparams.LSONamespace)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("No %s pods were found in %s namespace due to %v",
		vcoreparams.LSOName, vcoreparams.LSONamespace, err))
	Expect(len(lsoPods)).ToNot(Equal(0), fmt.Sprintf("The list of pods %s found in namespace %s is empty",
		vcoreparams.LSOName, vcoreparams.LSONamespace))

	lsoPod := lsoPods[0]
	lsoPodName := lsoPod.Object.Name

	time.Sleep(10 * time.Minute)

	err = lsoPod.WaitUntilReady(time.Second)
	if err != nil {
		lsoPodLog, _ := lsoPod.GetLog(600*time.Second, vcoreparams.LSOName)
		klog.Fatalf("%s pod in %s namespace in a bad state: %s",
			lsoPodName, vcoreparams.LSONamespace, lsoPodLog)
	}
} // func VerifyLSODeployment (ctx SpecContext)

// VerifyLocalVolumeSet asserts localvolumeset instance exists.
func VerifyLocalVolumeSet(ctx SpecContext) {
	klog.V(vcoreparams.VCoreLogLevel).Infof("Create localvolumeset instance %s in namespace %s if not found",
		vcoreparams.ODFLocalVolumeSetName, vcoreparams.LSONamespace)

	var err error

	localVolumeSetObj := lso.NewLocalVolumeSetBuilder(APIClient,
		vcoreparams.ODFLocalVolumeSetName,
		vcoreparams.LSONamespace)

	nodeSelector := corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{
		MatchExpressions: []corev1.NodeSelectorRequirement{{
			Key:      "kubernetes.io/hostname",
			Operator: "In",
			Values:   odfNodesList,
		}}},
	}}

	deviceInclusionSpec := lsov1alpha1.DeviceInclusionSpec{
		DeviceTypes:                []lsov1alpha1.DeviceType{lsov1alpha1.RawDisk},
		DeviceMechanicalProperties: []lsov1alpha1.DeviceMechanicalProperty{lsov1alpha1.NonRotational},
	}

	tolerations := []corev1.Toleration{{
		Key:      "node.ocs.openshift.io/storage",
		Operator: "Equal",
		Value:    "true",
		Effect:   "NoSchedule",
	}}

	_, err = localVolumeSetObj.WithNodeSelector(nodeSelector).
		WithStorageClassName(vcoreparams.StorageClassName).
		WithVolumeMode(lsov1.PersistentVolumeBlock).
		WithFSType("ext4").
		WithMaxDeviceCount(int32(42)).
		WithDeviceInclusionSpec(deviceInclusionSpec).
		WithTolerations(tolerations).Create()
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to create localvolumeset %s in namespace %s "+
		"due to %v", vcoreparams.ODFLocalVolumeSetName, vcoreparams.LSONamespace, err))

	pvLabel := fmt.Sprintf("storage.openshift.com/owner-name=%s", vcoreparams.ODFLocalVolumeSetName)

	err = await.WaitUntilPersistentVolumeCreated(APIClient,
		3,
		15*time.Minute,
		metav1.ListOptions{LabelSelector: pvLabel})
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to create persistentVolumes due to %v", err))
} // func VerifyLocalVolumeSet (ctx SpecContext)
