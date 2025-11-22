package vcorecommon

import (
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/klog/v2"

	nropv1 "github.com/openshift-kni/numaresources-operator/api/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clusteroperator"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/await"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nrop"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/apiobjectshelper"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/vcore/internal/vcoreparams"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/vcore/internal/vcoreinittools"
)

var (
	workerLabel           = VCoreConfig.WorkerLabel
	workerLabelMap        = VCoreConfig.WorkerLabelMap
	workerLabelListOption = VCoreConfig.WorkerLabelListOption
)

// VerifyNROPSuite container that contains tests for Numa Resources Operator verification.
func VerifyNROPSuite() {
	Describe(
		"NROP validation",
		Label(vcoreparams.LabelVCoreOperators), func() {
			It(fmt.Sprintf("Verifies %s namespace exists", vcoreparams.NROPNamespace),
				Label("numa"), VerifyNROPNamespaceExists)

			It("Verify Numa Resources Operator successfully installed",
				Label("numa"), reportxml.ID("66337"), VerifyNROPDeployment)

			It("Verify Numa Resources Operator Custom Resource deployment",
				Label("numa"), reportxml.ID("66343"), VerifyNROPCustomResources)

			It("Verify numa-aware secondary pod scheduler",
				Label("numa"), reportxml.ID("66339"), VerifyNROPAwareSecondaryPodScheduler)

			It("Verify scheduling workloads with the NUMA-aware scheduler",
				Label("numa"), reportxml.ID("66341"), VerifyNROPSchedulingWorkload)

			AfterAll(func() {
				By("Teardown")

				workloadDeployment, err := deployment.Pull(APIClient,
					vcoreparams.NumaWorkloadName,
					vcoreparams.NROPNamespace)
				if err == nil {
					err = workloadDeployment.DeleteAndWait(time.Minute * 2)
					Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to delete deployment due to %v", err))
				}

				nropScheduler := nrop.NewSchedulerBuilder(APIClient,
					vcoreparams.NumaAwareSecondarySchedulerName,
					vcoreparams.NROPNamespace)

				if nropScheduler.Exists() {
					_, err = nropScheduler.Delete()
					Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to delete NUMAResourcesScheduler %s "+
						"in namespace %s due to %v",
						vcoreparams.NumaAwareSecondarySchedulerName, vcoreparams.NROPNamespace, err))
				}
			})
		})
}

// VerifyNROPNamespaceExists asserts namespace for Numa Resources Operator exists.
func VerifyNROPNamespaceExists(ctx SpecContext) {
	err := apiobjectshelper.VerifyNamespaceExists(APIClient, vcoreparams.NROPNamespace, time.Second)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to pull namespace %q; %v",
		vcoreparams.NROPNamespace, err))
} // func VerifyNROPNamespaceExists (ctx SpecContext)

// VerifyNROPDeployment asserts Numa Resources Operator successfully installed.
func VerifyNROPDeployment(ctx SpecContext) {
	err := apiobjectshelper.VerifyOperatorDeployment(APIClient,
		vcoreparams.NROPSubscriptionName,
		vcoreparams.NROPDeploymentName,
		vcoreparams.NROPNamespace,
		time.Minute)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("operator deployment %s failure in the namespace %s; %v",
			vcoreparams.NROPDeploymentName, vcoreparams.NROPNamespace, err))
} // func VerifyNROPDeployment (ctx SpecContext)

// VerifyNROPCustomResources asserts Numa Resources Operator custom resource deployment.
func VerifyNROPCustomResources(ctx SpecContext) {
	klog.V(vcoreparams.VCoreLogLevel).Infof("Verify NUMAResourcesOperator %s configuration",
		vcoreparams.NROPInstanceName)

	nropCustomResource := nrop.NewBuilder(APIClient, vcoreparams.NROPInstanceName)

	if !nropCustomResource.Exists() {
		klog.V(vcoreparams.VCoreLogLevel).Infof("Create new NUMAResourcesOperator %s",
			vcoreparams.NROPInstanceName)

		var err error

		mcpSelectorExpression := metav1.LabelSelectorRequirement{
			Key:      "machineconfiguration.openshift.io/role",
			Operator: "In",
			Values:   []string{VCoreConfig.VCorePpMCPName, VCoreConfig.VCoreCpMCPName},
		}
		mcpSelector := metav1.LabelSelector{MatchExpressions: []metav1.LabelSelectorRequirement{mcpSelectorExpression}}
		infoRefreshPeriodic := nropv1.InfoRefreshPeriodic
		nodeGroupConfig := nropv1.NodeGroupConfig{InfoRefreshMode: &infoRefreshPeriodic}

		_, err = nropCustomResource.WithMCPSelector(nodeGroupConfig, mcpSelector).Create()
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to create NUMAResourcesOperator %s; %v",
			vcoreparams.NROPInstanceName, err))

		klog.V(vcoreparams.VCoreLogLevel).Infof(
			"Wait for all nodes rebooting after applying NUMAResourcesOperator %s",
			vcoreparams.NROPInstanceName)

		_, err = nodes.WaitForAllNodesToReboot(
			APIClient,
			40*time.Minute,
			workerLabelListOption)
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Nodes failed to reboot after applying NUMAResourcesOperator %s config; %v",
				vcoreparams.NROPInstanceName, err))

		klog.V(vcoreparams.VCoreLogLevel).Info("Wait for all clusteroperators availability after nodes reboot")

		_, err = clusteroperator.WaitForAllClusteroperatorsAvailable(APIClient, 60*time.Second)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error waiting for all available clusteroperators: %v",
			err))
	}

	klog.V(vcoreparams.VCoreLogLevel).Info("Verify all NUMAResourcesOperator pods was created")

	nropDaemonsetCPName := fmt.Sprintf("%s-%s", vcoreparams.NROPInstanceName, VCoreConfig.VCoreCpMCPName)
	nropDaemonsetPPName := fmt.Sprintf("%s-%s", vcoreparams.NROPInstanceName, VCoreConfig.VCorePpMCPName)

	err := await.WaitUntilDaemonSetIsRunning(APIClient, nropDaemonsetCPName, vcoreparams.NROPNamespace, time.Minute*2)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf(
		"Failed to create NUMAResourcesOperator %s pods in namespace %s due to %v",
		nropDaemonsetCPName, vcoreparams.NROPNamespace, err))

	err = await.WaitUntilDaemonSetIsRunning(APIClient, nropDaemonsetCPName, vcoreparams.NROPNamespace, time.Minute*2)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf(
		"Failed to create NUMAResourcesOperator %s pods in namespace %s due to %v",
		nropDaemonsetPPName, vcoreparams.NROPNamespace, err))

	workerNodesList, err := nodes.List(APIClient, workerLabelListOption)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf(
		"Failed to get nodes %s list due to %v", workerLabel, err))

	replicasCnt := len(workerNodesList)

	isReady, err := await.WaitForThePodReplicasCountInNamespace(APIClient, vcoreparams.NROPNamespace, metav1.ListOptions{
		LabelSelector: "name=resource-topology",
	}, replicasCnt, time.Minute*2)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf(
		"Failed to create %d NUMAResourcesOperator %s and %s pods in namespace %s due to %v",
		replicasCnt, nropDaemonsetCPName, nropDaemonsetPPName, vcoreparams.NROPNamespace, err))
	Expect(isReady).To(Equal(true), fmt.Sprintf(
		"Failed to create %d NUMAResourcesOperator %s and %s pods in namespace %s",
		replicasCnt, nropDaemonsetCPName, nropDaemonsetPPName, vcoreparams.NROPNamespace))
} // func VerifyNROPCustomResources (ctx SpecContext)

// VerifyNROPAwareSecondaryPodScheduler asserts Numa Resources Operator custom resource deployment.
func VerifyNROPAwareSecondaryPodScheduler(ctx SpecContext) {
	klog.V(vcoreparams.VCoreLogLevel).Infof("Verify NUMAResourcesScheduler %s configuration in namespace %s",
		vcoreparams.NumaAwareSecondarySchedulerName, vcoreparams.NROPNamespace)

	schedulerDeploymentName := "secondary-scheduler"

	nropScheduler := nrop.NewSchedulerBuilder(APIClient,
		vcoreparams.NumaAwareSecondarySchedulerName,
		vcoreparams.NROPNamespace)

	if !nropScheduler.Exists() {
		klog.V(vcoreparams.VCoreLogLevel).Infof("Create new NUMAResourcesScheduler %s in namespace %s",
			vcoreparams.NumaAwareSecondarySchedulerName, vcoreparams.NROPNamespace)

		var err error

		nrtOriginMirrorURL := "quay.io/openshift-kni"
		nrtImageName := "scheduler-plugins"
		nrtImageTag := "4.15-snapshot"

		nrtURL, err := getImageURL(nrtOriginMirrorURL, nrtImageName, nrtImageTag)
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to generate noderesourcetopology image URL for %s/%s:%s due to %v",
				nrtOriginMirrorURL, nrtImageName, nrtImageTag, err))

		_, err = nropScheduler.WithImageSpec(nrtURL).WithSchedulerName(vcoreparams.NumaAwareSchedulerName).Create()
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf(
			"failed to create NUMAResourcesScheduler %s in namespace %s; %v",
			vcoreparams.NumaAwareSecondarySchedulerName, vcoreparams.NROPNamespace, err))

		klog.V(vcoreparams.VCoreLogLevel).Info("Wait for all clusteroperators availability")

		_, err = clusteroperator.WaitForAllClusteroperatorsAvailable(APIClient, 60*time.Second)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Error waiting for all available clusteroperators: %v",
			err))
	}

	klog.V(vcoreparams.VCoreLogLevel).Info("Verify all NUMAResourcesScheduler deployment succeeded")

	err := await.WaitUntilDeploymentReady(APIClient, schedulerDeploymentName, vcoreparams.NROPNamespace, time.Minute*2)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf(
		"Deployment failed for the NUMAResourcesScheduler %s in namespace %s due to %v",
		schedulerDeploymentName, vcoreparams.NROPNamespace, err))

	isReady, err := await.WaitForThePodReplicasCountInNamespace(APIClient, vcoreparams.NROPNamespace, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app=%s", schedulerDeploymentName),
	}, 1, time.Minute*2)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf(
		"Failed to create NUMAResourcesScheduler %s pod in namespace %s due to %v",
		schedulerDeploymentName, vcoreparams.NROPNamespace, err))
	Expect(isReady).To(Equal(true), fmt.Sprintf("Failed to create NUMAResourcesScheduler %s pod "+
		"in namespace %s", schedulerDeploymentName, vcoreparams.NROPNamespace))
} // func VerifyNROPAwareSecondaryPodScheduler (ctx SpecContext)

// VerifyNROPSchedulingWorkload asserts namespace for Numa Resources Operator exists.
func VerifyNROPSchedulingWorkload(ctx SpecContext) {
	klog.V(vcoreparams.VCoreLogLevel).Info("Deploy Numa-aware scheduler workload")

	wkldImageOriginURL := "quay.io/openshifttest"
	wkldImageName := "hello-openshift"
	wkldImageTag := "openshift"

	workloadImage, err := getImageURL(wkldImageOriginURL, wkldImageName, wkldImageTag)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to generate numa workload image URL for %s/%s:%s due to %v",
			wkldImageOriginURL, wkldImageName, wkldImageTag, err))

	workloadSelector := map[string]string{"app": "test"}

	workloadDeployment := deployment.NewBuilder(APIClient,
		vcoreparams.NumaWorkloadName,
		vcoreparams.NROPNamespace,
		workloadSelector,
		corev1.Container{
			Name:            "ctnr",
			Image:           workloadImage,
			ImagePullPolicy: "IfNotPresent",
			Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("100Mi"),
					corev1.ResourceCPU:    resource.MustParse("10"),
				},
				Requests: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("100Mi"),
					corev1.ResourceCPU:    resource.MustParse("10"),
				},
			},
		})

	if workloadDeployment.Exists() {
		err = workloadDeployment.DeleteAndWait(time.Minute)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to delete deployment %s in namespace %s due to %v",
			vcoreparams.NumaWorkloadName, vcoreparams.NROPNamespace, err))
	}

	_, err = workloadDeployment.WithNodeSelector(workerLabelMap).
		WithSchedulerName(vcoreparams.NumaAwareSchedulerName).CreateAndWaitUntilReady(time.Minute)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to create deployment %s in namespace %s due to %v",
		vcoreparams.NumaWorkloadName, vcoreparams.NROPNamespace, err))

	klog.V(vcoreparams.VCoreLogLevel).Infof("Verify that the %s is scheduling the deployed pod",
		vcoreparams.NumaAwareSchedulerName)

	isReady, err := await.WaitForThePodReplicasCountInNamespace(APIClient, vcoreparams.NROPNamespace, metav1.ListOptions{
		LabelSelector: "app=test",
	}, 1, time.Second*10)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf(
		"Failed to create numa workload %s pod in namespace %s due to %v",
		vcoreparams.NumaWorkloadName, vcoreparams.NROPNamespace, err))
	Expect(isReady).To(Equal(true), fmt.Sprintf(
		"numa workload %s pod in namespace %s not ready",
		vcoreparams.NumaWorkloadName, vcoreparams.NROPNamespace))
} // func VerifyNROPSchedulingWorkload (ctx SpecContext)
