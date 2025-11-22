package vcorecommon

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/vcore/internal/ocpcli"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/apiobjectshelper"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/servicemesh"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/await"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/vcore/internal/vcoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/vcore/internal/vcoreparams"
)

// VerifyServiceMeshSuite container that contains tests for Service Mesh verification.
func VerifyServiceMeshSuite() {
	Describe(
		"Service Mesh Operator deployment and configuration validation",
		Label(vcoreparams.LabelVCoreOperators), func() {
			It(fmt.Sprintf("Verifies %s namespace exists", vcoreparams.DTPONamespace),
				Label("smo"), VerifyDTPONamespaceExists)

			It(fmt.Sprintf("Verifies %s namespace exists", vcoreparams.KialiNamespace),
				Label("smo"), VerifyKialiNamespaceExists)

			It(fmt.Sprintf("Verifies %s namespace exists", vcoreparams.IstioNamespace),
				Label("smo"), VerifyIstioNamespaceExists)

			It(fmt.Sprintf("Verifies %s namespace exists", vcoreparams.SMONamespace),
				Label("smo"), VerifyServiceMeshNamespaceExists)

			It("Verifies Distributed Tracing Platform Operator deployment succeeded",
				Label("smo"), reportxml.ID("59495"), VerifyDTPODeployment)

			It("Verifies Kiali deployment succeeded",
				Label("smo"), reportxml.ID("59496"), VerifyKialiDeployment)

			It("Verifies Service Mesh deployment succeeded",
				Label("smo"), reportxml.ID("73732"), VerifyServiceMeshDeployment)

			It("Verifies Service Mesh configuration procedure succeeded",
				Label("smo"), reportxml.ID("59502"), VerifyServiceMeshConfig)
		})
}

// VerifyDTPONamespaceExists asserts Distributed Tracing Platform Operator namespace exists.
func VerifyDTPONamespaceExists(ctx SpecContext) {
	err := apiobjectshelper.VerifyNamespaceExists(APIClient, vcoreparams.DTPONamespace, time.Second)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to pull namespace %q; %v",
		vcoreparams.DTPONamespace, err))
} // func VerifyDTPONamespaceExists (ctx SpecContext)

// VerifyKialiNamespaceExists asserts Kiali Operator namespace exists.
func VerifyKialiNamespaceExists(ctx SpecContext) {
	err := apiobjectshelper.VerifyNamespaceExists(APIClient, vcoreparams.KialiNamespace, time.Second)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to pull namespace %q; %v",
		vcoreparams.KialiNamespace, err))
} // func VerifyKialiNamespaceExists (ctx SpecContext)

// VerifyIstioNamespaceExists asserts Istio Operator namespace exists.
func VerifyIstioNamespaceExists(ctx SpecContext) {
	err := apiobjectshelper.VerifyNamespaceExists(APIClient, vcoreparams.IstioNamespace, time.Second)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to pull namespace %q; %v",
		vcoreparams.IstioNamespace, err))
} // func VerifyIstioNamespaceExists (ctx SpecContext)

// VerifyServiceMeshNamespaceExists asserts Kiali Operator namespace exists.
func VerifyServiceMeshNamespaceExists(ctx SpecContext) {
	err := apiobjectshelper.VerifyNamespaceExists(APIClient, vcoreparams.SMONamespace, time.Second)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to pull namespace %q; %v",
		vcoreparams.SMONamespace, err))
} // func VerifyServiceMeshNamespaceExists (ctx SpecContext)

// VerifyDTPODeployment assert Distributed Tracing Platform Operator deployment succeeded.
func VerifyDTPODeployment(ctx SpecContext) {
	klog.V(vcoreparams.VCoreLogLevel).Infof("Verify Distributed Tracing Platform Operator deployment")

	err := apiobjectshelper.VerifyOperatorDeployment(APIClient,
		vcoreparams.DTPOperatorSubscriptionName,
		vcoreparams.DTPOperatorDeploymentName,
		vcoreparams.DTPONamespace,
		time.Minute)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("operator deployment %s failure in the namespace %s; %v",
			vcoreparams.DTPOperatorSubscriptionName, vcoreparams.DTPONamespace, err))
} // func VerifyDTPODeployment (ctx SpecContext)

// VerifyKialiDeployment assert Kiali Operator deployment succeeded.
func VerifyKialiDeployment(ctx SpecContext) {
	klog.V(vcoreparams.VCoreLogLevel).Infof("Verify Kiali Operator deployment")

	err := apiobjectshelper.VerifyOperatorDeployment(APIClient,
		vcoreparams.KialiOperatorSubscriptionName,
		vcoreparams.KialiOperatorDeploymentName,
		vcoreparams.KialiNamespace,
		time.Minute)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("operator deployment %s failure in the namespace %s; %v",
			vcoreparams.KialiOperatorSubscriptionName, vcoreparams.KialiNamespace, err))
} // func VerifyKialiDeployment (ctx SpecContext)

// VerifyServiceMeshDeployment assert Service Mesh Operator deployment succeeded.
func VerifyServiceMeshDeployment(ctx SpecContext) {
	klog.V(vcoreparams.VCoreLogLevel).Infof("Verify Service Mesh Operator deployment")

	err := apiobjectshelper.VerifyOperatorDeployment(APIClient,
		vcoreparams.SMOSubscriptionName,
		vcoreparams.SMODeploymentName,
		vcoreparams.SMONamespace,
		time.Minute)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("operator deployment %s failure in the namespace %s; %v",
			vcoreparams.SMOSubscriptionName, vcoreparams.SMONamespace, err))
} // func VerifyServiceMeshDeployment (ctx SpecContext)

// VerifyServiceMeshConfig assert Service Mesh Operator configuration procedure.
//
//nolint:funlen
func VerifyServiceMeshConfig(ctx SpecContext) {
	klog.V(vcoreparams.VCoreLogLevel).Infof("Verify Service Mesh Operator configuration procedure")

	istioBasicPodLabel := "app=istiod"
	istioIgwPodLabel := "app=istio-ingressgateway"
	wasmBasicPodLabel := "app=wasm-cacher"
	smoCpTemplateName := "smo-controlplane.yaml"
	memberRollName := "default"
	membersList := []string{"amfmme1", "csdb1", "nrf1", "nssf1", "smf1", "upf1"}
	servicemeshControlplaneDeployments := []string{"istio-ingressgateway", "istiod-basic", "jaeger"}

	klog.V(vcoreparams.VCoreLogLevel).Info("Create namespaces according to the members list")

	for _, member := range membersList {
		nsBuilder, err := namespace.NewBuilder(APIClient, member).Create()
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to create namespace %s due to %v",
			member, err))
		Expect(nsBuilder.Exists()).To(Equal(true), fmt.Sprintf("namespace %s not found", member))
	}

	klog.V(vcoreparams.VCoreLogLevel).Info("Start to configure Service-Mesh Operator")

	klog.V(vcoreparams.VCoreLogLevel).Info("Create service-mesh memberRoll")

	memberRollObj := servicemesh.NewMemberRollBuilder(APIClient, memberRollName, vcoreparams.IstioNamespace)

	if !memberRollObj.Exists() {
		memberRoleObj, err := memberRollObj.WithMembersList(membersList).Create()
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to create memberRoll %s in namespace %s due to %v",
			memberRollName, vcoreparams.IstioNamespace, err))
		Expect(memberRoleObj.Exists()).To(Equal(true),
			fmt.Sprintf("memberRoll %s not found in namespace %s", memberRollName, vcoreparams.IstioNamespace))
	}

	klog.V(vcoreparams.VCoreLogLevel).Info("Create service-mesh control plane")

	controlPlaneBuilder := servicemesh.NewControlPlaneBuilder(APIClient, "basic", vcoreparams.IstioNamespace)

	if !controlPlaneBuilder.Exists() {
		workingDir, err := os.Getwd()
		Expect(err).ToNot(HaveOccurred(), err)

		templateDir := filepath.Join(workingDir, vcoreparams.TemplateFilesFolder)

		varsToReplace := make(map[string]interface{})
		varsToReplace["ControlPlaneName"] = "basic"
		varsToReplace["ControlPlaneNamespace"] = vcoreparams.IstioNamespace

		err = ocpcli.ApplyConfig(
			filepath.Join(templateDir, smoCpTemplateName),
			filepath.Join(vcoreparams.ConfigurationFolderPath, smoCpTemplateName),
			varsToReplace)
		Expect(err).To(BeNil(), fmt.Sprint(err))
		Expect(controlPlaneBuilder.Exists()).To(Equal(true),
			fmt.Sprintf("failed to create service-mesh control plane %s in namespace %s",
				"basic", vcoreparams.IstioNamespace))
	}

	memeberRollReady, err := memberRollObj.IsReady(2 * time.Second)
	Expect(err).To(BeNil(), fmt.Sprint(err))
	Expect(memeberRollReady).To(Equal(true),
		fmt.Sprintf("memberroll %s not found in namespace %s", memberRollName, vcoreparams.IstioNamespace))

	klog.V(vcoreparams.VCoreLogLevel).Infof("Confirm that all servicemesh controlplane deployments %v "+
		"are running in %s namespace",
		servicemeshControlplaneDeployments, vcoreparams.IstioNamespace)

	for _, smodeployment := range servicemeshControlplaneDeployments {
		err = await.WaitUntilDeploymentReady(APIClient,
			smodeployment,
			vcoreparams.IstioNamespace,
			2*time.Minute)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("smo deployment %s not found in %s namespace; %v",
			smodeployment, vcoreparams.IstioNamespace, err))
	}

	klog.V(vcoreparams.VCoreLogLevel).Info("Check the service-mesh control plane pods are creating on the " +
		"istio-system project")
	klog.V(vcoreparams.VCoreLogLevel).Infof("Confirm that %s pod was deployed and running in %s namespace",
		istioBasicPodLabel, vcoreparams.IstioNamespace)

	_, err = await.WaitForThePodReplicasCountInNamespace(
		APIClient,
		vcoreparams.IstioNamespace,
		metav1.ListOptions{LabelSelector: istioBasicPodLabel},
		3,
		5*time.Second)
	Expect(err).ToNot(HaveOccurred(), "No %s labeled pods were found in %s namespace; %w",
		istioBasicPodLabel, vcoreparams.IstioNamespace, err)

	_, err = pod.WaitForAllPodsInNamespaceRunning(
		APIClient,
		vcoreparams.IstioNamespace,
		5*time.Second,
		metav1.ListOptions{LabelSelector: istioBasicPodLabel})
	Expect(err).ToNot(HaveOccurred(), "No %s labeled pods were found in %s namespace; %w",
		istioBasicPodLabel, vcoreparams.IstioNamespace, err)

	klog.V(vcoreparams.VCoreLogLevel).Infof("Confirm that %s pod was deployed and running in %s namespace",
		istioIgwPodLabel, vcoreparams.IstioNamespace)

	_, err = pod.WaitForAllPodsInNamespaceRunning(
		APIClient,
		vcoreparams.IstioNamespace,
		5*time.Second,
		metav1.ListOptions{LabelSelector: istioIgwPodLabel})
	Expect(err).ToNot(HaveOccurred(), "No %s labeled pods were found in %s namespace; %w",
		istioIgwPodLabel, vcoreparams.IstioNamespace, err)

	klog.V(vcoreparams.VCoreLogLevel).Infof("Confirm that %s pod was deployed and running in %s namespace",
		wasmBasicPodLabel, vcoreparams.IstioNamespace)

	_, err = pod.WaitForAllPodsInNamespaceRunning(
		APIClient,
		vcoreparams.IstioNamespace,
		5*time.Second,
		metav1.ListOptions{LabelSelector: wasmBasicPodLabel})
	Expect(err).ToNot(HaveOccurred(), "No %s labeled pods were found in %s namespace; %w",
		wasmBasicPodLabel, vcoreparams.IstioNamespace, err)
} // func VerifyServiceMeshConfig (ctx SpecContext)
