package bmc

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/rbac"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/serviceaccount"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/bmc/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/define"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/internal/kmmparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/reporter"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"

	_ "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/kmm/bmc/tests"
)

var (
	prereqName = "kmm-bmc-tests-executor"
)

var _, currentFile, _, _ = runtime.Caller(0)

func TestBMC(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = GeneralConfig.GetJunitReportPath(currentFile)

	RegisterFailHandler(Fail)
	RunSpecs(t, "KMM-BMC", Label(tsparams.Labels...), reporterConfig)
}

var _ = BeforeSuite(func() {
	By("Prepare environment for BMC tests execution")

	By("Create helper ServiceAccount and ClusterRoleBindings")

	svcAccount, err := serviceaccount.
		NewBuilder(APIClient, prereqName, kmmparams.KmmOperatorNamespace).Create()
	Expect(err).ToNot(HaveOccurred(), "error creating serviceaccount")

	// Create CRB for privileged SCC (to run privileged pods)
	crbSCC := define.ModuleCRB(*svcAccount, prereqName)
	_, err = crbSCC.Create()
	Expect(err).ToNot(HaveOccurred(), "error creating privileged SCC clusterrolebinding")

	// Create CRB for pods/exec and other operational permissions
	crbEdit := rbac.NewClusterRoleBindingBuilder(APIClient, prereqName+"-exec-binding",
		"edit",
		rbacv1.Subject{
			Kind:      "ServiceAccount",
			Name:      prereqName,
			Namespace: kmmparams.KmmOperatorNamespace,
		})
	_, err = crbEdit.Create()
	Expect(err).ToNot(HaveOccurred(), "error creating edit clusterrolebinding")

	By("Create helper Deployments on worker nodes")

	nodeList, err := nodes.List(
		APIClient, metav1.ListOptions{LabelSelector: labels.Set(GeneralConfig.WorkerLabelMap).String()})
	if err != nil {
		Skip(fmt.Sprintf("Error listing worker nodes. Got error: '%v'", err))
	}

	for _, node := range nodeList {
		deploymentName := fmt.Sprintf("%s-%s", kmmparams.KmmTestHelperLabelName, node.Object.Name)

		containerCfg, err := pod.NewContainerBuilder("test", kmmparams.DTKImage,
			[]string{"/bin/bash", "-c", "sleep INF"}).
			WithSecurityContext(kmmparams.PrivilegedSC).
			WithVolumeMount(corev1.VolumeMount{Name: "host", MountPath: "/host", ReadOnly: false}).
			GetContainerCfg()
		if err != nil {
			Fail(fmt.Sprintf("Failed to create container config for node %s: %v", node.Object.Name, err))
		}

		deploymentCfg := deployment.NewBuilder(APIClient, deploymentName, kmmparams.KmmOperatorNamespace,
			map[string]string{kmmparams.KmmTestHelperLabelName: ""}, *containerCfg)
		deploymentCfg.WithVolume(corev1.Volume{
			Name: "host",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{Path: "/"},
			},
		})
		deploymentCfg.WithToleration(kmmparams.TolerationNoExecuteK8sUnreachable)
		deploymentCfg.WithToleration(kmmparams.TolerationNoScheduleK8sUnreachable)
		deploymentCfg.WithToleration(kmmparams.TolerationNoScheduleK8sUnschedulable)
		deploymentCfg.WithToleration(kmmparams.TolerationNoScheduleK8sDiskPressure)
		deploymentCfg.WithToleration(kmmparams.TolerationNoExecuteKeyValue)
		deploymentCfg.WithToleration(kmmparams.TolerationNoScheduleKeyValue)

		deploymentCfg.WithLabel(kmmparams.KmmTestHelperLabelName, "").
			WithNodeSelector(map[string]string{"kubernetes.io/hostname": node.Object.Name}).
			WithServiceAccountName(prereqName)

		_, err = deploymentCfg.CreateAndWaitUntilReady(10 * time.Minute)
		if err != nil {
			Skip(fmt.Sprintf("Could not create deploymentCfg on %s. Got error : %v", node.Object.Name, err))
		}
	}
})

var _ = AfterSuite(func() {
	By("Cleanup environment after BMC tests execution")

	By("Delete helper deployments")

	testDeployments, err := deployment.List(APIClient, kmmparams.KmmOperatorNamespace, metav1.ListOptions{})
	if err != nil {
		Fail(fmt.Sprintf("Error cleaning up environment. Got error: %v", err))
	}

	for _, deploymentObj := range testDeployments {
		if strings.Contains(deploymentObj.Object.Name, kmmparams.KmmTestHelperLabelName) {
			err = deploymentObj.DeleteAndWait(time.Minute)

			Expect(err).ToNot(HaveOccurred(), "error deleting helper deployment")
		}
	}

	By("Delete helper ServiceAccount and ClusterRoleBindings")

	svcAccount := serviceaccount.NewBuilder(APIClient, prereqName, kmmparams.KmmOperatorNamespace)

	if svcAccount.Exists() {
		// Delete CRB for privileged SCC
		crbSCC := define.ModuleCRB(*svcAccount, prereqName)
		err = crbSCC.Delete()
		Expect(err).ToNot(HaveOccurred(), "error deleting privileged SCC clusterrolebinding")

		// Delete CRB for edit permissions
		crbEdit := rbac.NewClusterRoleBindingBuilder(APIClient, prereqName+"-exec-binding",
			"edit",
			rbacv1.Subject{
				Kind:      "ServiceAccount",
				Name:      prereqName,
				Namespace: kmmparams.KmmOperatorNamespace,
			})
		err = crbEdit.Delete()
		Expect(err).ToNot(HaveOccurred(), "error deleting edit clusterrolebinding")

		// Delete ServiceAccount
		err = svcAccount.Delete()
		Expect(err).ToNot(HaveOccurred(), "error deleting helper serviceaccount")
	} else {
		klog.V(kmmparams.KmmLogLevel).Infof(
			"Service account %s does not exist, skipping cleanup", prereqName)
	}
})

var _ = ReportAfterSuite("", func(report Report) {
	reportxml.Create(
		report, GeneralConfig.GetReportPath(), GeneralConfig.TCPrefix)
})

var _ = JustAfterEach(func() {
	reporter.ReportIfFailed(
		CurrentSpecReport(), currentFile, tsparams.ReporterNamespacesToDump, tsparams.ReporterCRDsToDump)
})
