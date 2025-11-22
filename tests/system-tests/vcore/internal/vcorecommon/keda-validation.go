package vcorecommon

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/vcore/internal/ocpcli"

	monv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/monitoring"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/rbac"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/secret"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/service"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/serviceaccount"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/await"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/klog/v2"

	kedav1alpha1 "github.com/kedacore/keda-olm-operator/api/keda/v1alpha1"
	kedav2v1alpha1 "github.com/kedacore/keda/v2/apis/keda/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/keda"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/apiobjectshelper"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/vcore/internal/vcoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/vcore/internal/vcoreparams"
)

const (
	kedaScaledObjectName      = "prometheus-scaledobject"
	configmapName             = "cluster-monitoring-config"
	configmapNamespace        = "openshift-monitoring"
	testAppServiceMonitorName = "keda-testing-sm"
	serviceAccountName        = "thanos"
	saSecretName              = "thanos-secret"
	triggerAuthName           = "keda-trigger-auth-prometheus"
	metricsReaderName         = "thanos-metrics-reader"

	prometheusOriginMirrorURL = "quay.io/zroubalik"
	prometheusImageName       = "prometheus-app"
	prometheusImageTag        = "latest"

	abOriginMirrorURL = "docker.io/jordi"
	abImageName       = "ab"
	abImageTag        = "latest"
)

// VerifyKedaSuite container that contains tests for Keda verification.
func VerifyKedaSuite() {
	Describe(
		"Keda validation",
		Label(vcoreparams.LabelVCoreOperators), func() {
			It(fmt.Sprintf("Verifies %s namespace exists", vcoreparams.KedaNamespace),
				Label("keda"), VerifyKedaNamespaceExists)

			It("Verifies Keda operator deployment succeeded",
				Label("keda"), reportxml.ID("65001"), VerifyKedaDeployment)

			It("Verifies KedaController instance created successfully",
				Label("keda"), reportxml.ID("65004"), VerifyKedaControllerDeployment)

			It("Verifies ScaleObject instance created successfully",
				Label("keda"), reportxml.ID("65007"), VerifyScaleObjectDeployment)

			AfterAll(func() {
				By("Teardown")

				Expect(ensureNamespaceNotExists(vcoreparams.KedaWatchNamespace)).
					To(Equal(true), fmt.Sprintf("Failed to delete watch namespace %s",
						vcoreparams.KedaWatchNamespace))
			})
		})
}

// VerifyKedaNamespaceExists asserts namespace for NMState operator exists.
func VerifyKedaNamespaceExists(ctx SpecContext) {
	err := apiobjectshelper.VerifyNamespaceExists(APIClient, vcoreparams.KedaNamespace, time.Second)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to pull namespace %q; %v", vcoreparams.KedaNamespace, err))
} // func VerifyKedaNamespaceExists (ctx SpecContext)

// VerifyKedaDeployment assert that Keda operator deployment succeeded.
func VerifyKedaDeployment(ctx SpecContext) {
	err := apiobjectshelper.VerifyOperatorDeployment(APIClient,
		vcoreparams.KedaSubscriptionName,
		vcoreparams.KedaDeploymentName,
		vcoreparams.KedaNamespace,
		time.Minute)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Keda operator deployment %s failure in the namespace %s; %v",
			vcoreparams.KedaDeploymentName, vcoreparams.KedaNamespace, err))
} // func VerifyKedaDeployment (ctx SpecContext)

// VerifyKedaControllerDeployment assert that kedaController instance created successfully.
func VerifyKedaControllerDeployment(ctx SpecContext) {
	klog.V(vcoreparams.VCoreLogLevel).Infof("Verify kedaController instance exists")

	kedaControllerBuilder := keda.NewControllerBuilder(
		APIClient,
		vcoreparams.KedaControllerName,
		vcoreparams.KedaNamespace)

	if !kedaControllerBuilder.Exists() {
		var err error

		admissionWebhooks := kedav1alpha1.KedaAdmissionWebhooksSpec{
			LogLevel:   "info",
			LogEncoder: "console",
		}
		operator := kedav1alpha1.KedaOperatorSpec{
			LogLevel:   "info",
			LogEncoder: "console",
		}
		metricsServer := kedav1alpha1.KedaMetricsServerSpec{
			LogLevel: "0",
		}

		kedaControllerBuilder, err = kedaControllerBuilder.WithAdmissionWebhooks(admissionWebhooks).
			WithOperator(operator).
			WithMetricsServer(metricsServer).
			WithWatchNamespace(vcoreparams.KedaWatchNamespace).
			Create()
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to create kedaController instance %q in namespace %q due to: %v",
				vcoreparams.KedaControllerName, vcoreparams.KedaNamespace, err))
		Expect(kedaControllerBuilder.Exists()).To(Equal(true), fmt.Sprintf(
			"no kedaController instance  %s was found in the namespace %s",
			vcoreparams.KedaControllerName, vcoreparams.KedaNamespace))
	}
} // func VerifyKedaControllerDeployment (ctx SpecContext)

// VerifyScaleObjectDeployment assert that scaleObject instance created successfully.
//
//nolint:funlen
func VerifyScaleObjectDeployment(ctx SpecContext) {
	klog.V(vcoreparams.VCoreLogLevel).Info("Verify monitoring status")

	var err error

	configMapBuilder := configmap.NewBuilder(APIClient, configmapName, configmapNamespace)

	if !configMapBuilder.Exists() {
		configMapBuilder, err = configMapBuilder.
			WithData(map[string]string{"config.yaml": "enableUserWorkload: true"}).Create()
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to create configmap %q in namespace %q due to: %v",
				configmapName, configmapNamespace, err))
		Expect(configMapBuilder.Exists()).To(Equal(true), fmt.Sprintf(
			"no configmap %s was found in the namespace %s", configmapName, configmapNamespace))
	}

	klog.V(vcoreparams.VCoreLogLevel).Info("Deploy application that exposes Prometheus metrics")

	Expect(ensureNamespaceNotExists(vcoreparams.KedaWatchNamespace)).
		To(Equal(true), fmt.Sprintf("Failed to delete watch namespace %s",
			vcoreparams.KedaWatchNamespace))

	Expect(ensureNamespaceExists(vcoreparams.KedaWatchNamespace)).To(Equal(true),
		fmt.Sprintf("failed to create namespace %s", vcoreparams.KedaWatchNamespace))

	klog.V(vcoreparams.VCoreLogLevel).Infof("Create test application deployment %s in namespace %s",
		vcoreparams.KedaWatchAppName, vcoreparams.KedaWatchNamespace)

	prometheusImageURL, err := getImageURL(prometheusOriginMirrorURL, prometheusImageName, prometheusImageTag)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to generate prometheus image URL for %s/%s:%s due to: %v",
			prometheusOriginMirrorURL, prometheusImageName, prometheusImageTag, err))

	falseVar := false
	trueVar := true
	securityContext := corev1.SecurityContext{
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
		RunAsNonRoot:             &trueVar,
		AllowPrivilegeEscalation: &falseVar,
		SeccompProfile: &corev1.SeccompProfile{
			Type: "RuntimeDefault",
		},
	}

	appContainer := corev1.Container{
		Name:            "prom-test-app",
		Image:           prometheusImageURL,
		ImagePullPolicy: "IfNotPresent",
		SecurityContext: &securityContext,
	}

	_, err = deployment.NewBuilder(APIClient,
		vcoreparams.KedaWatchAppName,
		vcoreparams.KedaWatchNamespace,
		map[string]string{"app": vcoreparams.KedaWatchAppName},
		appContainer,
	).WithLabel("type", "keda-testing").WithReplicas(int32(1)).Create()
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create test application %s in namespace %s due to: %v",
			vcoreparams.KedaWatchAppName, vcoreparams.KedaWatchNamespace, err))

	klog.V(vcoreparams.VCoreLogLevel).Infof("Create test application Service %s in namespace %s",
		vcoreparams.KedaWatchAppName, vcoreparams.KedaWatchNamespace)

	_, err = service.NewBuilder(APIClient,
		vcoreparams.KedaWatchAppName,
		vcoreparams.KedaWatchNamespace,
		map[string]string{"type": "keda-testing"},
		corev1.ServicePort{
			Name:     "http",
			Protocol: "TCP",
			Port:     80,
			TargetPort: intstr.IntOrString{
				Type:   intstr.Int,
				IntVal: int32(8080),
			},
		},
	).WithAnnotation(map[string]string{"prometheus.io/scrape": "true"}).
		WithLabels(map[string]string{"app": vcoreparams.KedaWatchAppName}).Create()
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create service %s in namespace %s due to: %v",
			vcoreparams.KedaWatchAppName, vcoreparams.KedaWatchNamespace, err))

	klog.V(vcoreparams.VCoreLogLevel).Infof("Create ServiceMonitor %s in namespace %s",
		testAppServiceMonitorName, vcoreparams.KedaWatchNamespace)

	endpoints := []monv1.Endpoint{{
		Port:   "http",
		Scheme: "http",
	}}

	_, err = monitoring.NewBuilder(APIClient,
		testAppServiceMonitorName,
		vcoreparams.KedaWatchNamespace).WithEndpoints(endpoints).
		WithSelector(map[string]string{"app": vcoreparams.KedaWatchAppName}).Create()
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create serviceMonitor %s in namespace %s due to: %v",
			testAppServiceMonitorName, vcoreparams.KedaWatchNamespace, err))

	klog.V(vcoreparams.VCoreLogLevel).Infof("Create test application ServiceAccount %s in namespace %s "+
		"and locate assigned token",
		vcoreparams.KedaWatchAppName, vcoreparams.KedaWatchNamespace)

	_, err = serviceaccount.NewBuilder(APIClient,
		serviceAccountName, vcoreparams.KedaWatchNamespace).Create()
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create serviceAccount %s in namespace %s due to: %v",
			serviceAccountName, vcoreparams.KedaWatchNamespace, err))

	_, err = secret.NewBuilder(APIClient,
		saSecretName,
		vcoreparams.KedaWatchNamespace,
		corev1.SecretTypeServiceAccountToken).
		WithAnnotations(map[string]string{"kubernetes.io/service-account.name": serviceAccountName}).Create()
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create token assigned to the serviceAccount %s in namespace %s due to: %v",
			serviceAccountName, vcoreparams.KedaWatchNamespace, err))

	klog.V(vcoreparams.VCoreLogLevel).
		Infof("Define TriggerAuthentication %s with the Service Account's token %s in namespace %s",
			triggerAuthName, vcoreparams.KedaWatchNamespace, saSecretName)

	secretTargetRef := []kedav2v1alpha1.AuthSecretTargetRef{{
		Parameter: "bearerToken",
		Name:      saSecretName,
		Key:       "token",
	}, {
		Parameter: "ca",
		Name:      saSecretName,
		Key:       "ca.crt",
	}}

	_, err = keda.NewTriggerAuthenticationBuilder(APIClient,
		triggerAuthName,
		vcoreparams.KedaWatchNamespace).WithSecretTargetRef(secretTargetRef).Create()
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create TriggerAuthentication %s with the Service Account's token %s "+
			"in namespace %s due to: %v",
			triggerAuthName, vcoreparams.KedaWatchNamespace, saSecretName, err))

	klog.V(vcoreparams.VCoreLogLevel).Infof("Create a role %s for reading metric from Thanos in namespace %s",
		metricsReaderName, vcoreparams.KedaWatchNamespace)

	roleRule1 := rbacv1.PolicyRule{
		APIGroups: []string{""},
		Resources: []string{"pods"},
		Verbs:     []string{"get"},
	}
	roleRule2 := rbacv1.PolicyRule{
		APIGroups: []string{"metrics.k8s.io"},
		Resources: []string{"pods", "nodes"},
		Verbs:     []string{"get", "list", "watch"},
	}

	_, err = rbac.NewRoleBuilder(APIClient, metricsReaderName, vcoreparams.KedaWatchNamespace, roleRule1).
		WithRules([]rbacv1.PolicyRule{roleRule2}).Create()
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create a role %s for reading metric from Thanos in namespace %s due to: %v",
			metricsReaderName, vcoreparams.KedaWatchNamespace, err))

	klog.V(vcoreparams.VCoreLogLevel).Infof("Create a roleBinding %s for serviceaccount %s in namespace %s",
		metricsReaderName, serviceAccountName, vcoreparams.KedaWatchNamespace)

	kedaRoleBindingTemplateName := "keda-rolebinding.yaml"
	varsToReplace := make(map[string]interface{})
	varsToReplace["RoleBindingName"] = metricsReaderName
	varsToReplace["RoleBindingNamespace"] = vcoreparams.KedaWatchNamespace
	varsToReplace["ServiceAccountName"] = serviceAccountName
	varsToReplace["RoleName"] = metricsReaderName

	workingDir, err := os.Getwd()
	Expect(err).ToNot(HaveOccurred(), err)

	templateDir := filepath.Join(workingDir, vcoreparams.TemplateFilesFolder)

	err = ocpcli.CreateConfig(
		filepath.Join(templateDir, kedaRoleBindingTemplateName),
		filepath.Join(vcoreparams.ConfigurationFolderPath, kedaRoleBindingTemplateName),
		varsToReplace)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to create load job due to %v", err))

	klog.V(vcoreparams.VCoreLogLevel).Infof("Define scaledObject instance %s in namespace %s",
		kedaScaledObjectName, vcoreparams.KedaWatchNamespace)

	scaledObjectBuilder := keda.NewScaledObjectBuilder(APIClient, kedaScaledObjectName, vcoreparams.KedaWatchNamespace)

	scaleTargetRef := kedav2v1alpha1.ScaleTarget{
		Name: vcoreparams.KedaWatchAppName,
	}

	scaleTriggers := []kedav2v1alpha1.ScaleTriggers{{
		Type: "prometheus",
		Metadata: map[string]string{
			"serverAddress": "https://thanos-querier.openshift-monitoring.svc.cluster.local:9092",
			"namespace":     vcoreparams.KedaWatchNamespace,
			"metricName":    "http_requests_total",
			"threshold":     "5",
			"query":         "sum(rate(http_requests_total{job=\"test-app\"}[1m]))",
			"authModes":     "bearer",
		},
		AuthenticationRef: &kedav2v1alpha1.AuthenticationRef{
			Name: triggerAuthName,
			Kind: "TriggerAuthentication",
		},
	}}

	_, err = scaledObjectBuilder.WithScaleTargetRef(scaleTargetRef).
		WithMinReplicaCount(int32(1)).
		WithMaxReplicaCount(int32(8)).
		WithPollingInterval(int32(5)).
		WithCooldownPeriod(int32(10)).
		WithTriggers(scaleTriggers).Create()
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create scaledObject instance %s in namespace %s due to %v",
			kedaScaledObjectName, vcoreparams.KedaWatchNamespace, err))

	klog.V(vcoreparams.VCoreLogLevel).Info("Generate requests to test the application autoscaling")

	abImageURL, err := getImageURL(abOriginMirrorURL, abImageName, abImageTag)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to generate prometheus image URL for %s/%s:%s due to %v",
			prometheusOriginMirrorURL, prometheusImageName, prometheusImageTag, err))

	appLoadJobTemplateName := "keda-test-app-load-job.yaml"
	varsToReplace["KedaWatchNamespace"] = vcoreparams.KedaWatchNamespace
	varsToReplace["TestNamespace"] = vcoreparams.KedaWatchNamespace
	varsToReplace["AbImageURL"] = abImageURL

	err = ocpcli.CreateConfig(
		filepath.Join(templateDir, appLoadJobTemplateName),
		filepath.Join(vcoreparams.ConfigurationFolderPath, appLoadJobTemplateName),
		varsToReplace)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to create load job due to %v", err))

	klog.V(vcoreparams.VCoreLogLevel).Info("Wait until pods replicas count reach 8")

	isCntReached, err := await.WaitForThePodReplicasCountInNamespace(APIClient,
		vcoreparams.KedaWatchNamespace, metav1.ListOptions{
			LabelSelector: "app=test-app",
		}, 8, time.Minute*5)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to scale %s pods in namespace %s due to %v",
		vcoreparams.KedaWatchAppName, vcoreparams.KedaWatchNamespace, err))
	Expect(isCntReached).To(Equal(true), fmt.Sprintf("failed to scale %s pods in namespace %s after %v",
		vcoreparams.KedaWatchAppName, vcoreparams.KedaWatchNamespace, time.Minute*5))
} // func VerifyKedaControllerDeployment (ctx SpecContext)
