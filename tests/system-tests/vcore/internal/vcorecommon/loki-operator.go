package vcorecommon

import (
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	lokiv1 "github.com/grafana/loki/operator/apis/loki/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clusterlogging"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/console"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/mco"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/secret"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/storage"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/await"
	corev1 "k8s.io/api/core/v1"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/apiobjectshelper"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/vcore/internal/vcoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/vcore/internal/vcoreparams"
)

// VerifyLokiSuite container that contains tests for LokiStack and ClusterLogging verification.
func VerifyLokiSuite() {
	Describe(
		"LokiStack and Cluster Logging validation",
		Label(vcoreparams.LabelVCoreODF), func() {
			It(fmt.Sprintf("Verifies %s namespace exists", vcoreparams.CLONamespace),
				Label("loki"), VerifyCLONamespaceExists)

			It(fmt.Sprintf("Verifies %s namespace exists", vcoreparams.LokiNamespace),
				Label("loki"), VerifyLokiNamespaceExists)

			It("Verify Loki Operator successfully installed",
				Label("loki"), reportxml.ID("74913"), VerifyLokiDeployment)

			It("Verify ClusterLogging Operator successfully installed",
				Label("loki"), reportxml.ID("73678"), VerifyCLODeployment)

			It("Create ObjectBucketClaim config",
				Label("loki"), reportxml.ID("74914"), CreateObjectBucketClaim)

			It("Create LokiStack instance",
				Label("loki"), reportxml.ID("74915"), CreateLokiStackInstance)

			It(fmt.Sprintf("Verify Cluster Logging instance %s is running in namespace %s",
				vcoreparams.CLOInstanceName, vcoreparams.CLONamespace),
				Label("loki"), reportxml.ID("59494"), CreateClusterLogForwarderInstance)
		})
}

// VerifyCLONamespaceExists asserts namespace for ClusterLogging Operator exists.
func VerifyCLONamespaceExists(ctx SpecContext) {
	err := apiobjectshelper.VerifyNamespaceExists(APIClient, vcoreparams.CLONamespace, time.Second)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to pull namespace %q; %v",
		vcoreparams.CLONamespace, err))
} // func VerifyCLONamespaceExists (ctx SpecContext)

// VerifyLokiNamespaceExists asserts namespace for ElasticSearch Operator exists.
func VerifyLokiNamespaceExists(ctx SpecContext) {
	err := apiobjectshelper.VerifyNamespaceExists(APIClient, vcoreparams.LokiNamespace, time.Second)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to pull namespace %q; %v",
		vcoreparams.LokiNamespace, err))
} // func VerifyLokiNamespaceExists (ctx SpecContext)

// VerifyLokiDeployment asserts ElasticSearch Operator successfully installed.
func VerifyLokiDeployment(ctx SpecContext) {
	err := apiobjectshelper.VerifyOperatorDeployment(APIClient,
		vcoreparams.LokiOperatorSubscriptionName,
		vcoreparams.LokiOperatorDeploymentName,
		vcoreparams.LokiNamespace,
		time.Minute)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Loki operator deployment %s failure in the namespace %s; %v",
			vcoreparams.LokiOperatorDeploymentName, vcoreparams.LokiNamespace, err))
} // func VerifyLokiDeployment (ctx SpecContext)

// VerifyCLODeployment asserts ClusterLogging Operator successfully installed.
func VerifyCLODeployment(ctx SpecContext) {
	err := apiobjectshelper.VerifyOperatorDeployment(APIClient,
		vcoreparams.CLOName,
		vcoreparams.CLODeploymentName,
		vcoreparams.CLONamespace,
		time.Minute)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("operator deployment %s failure in the namespace %s; %v",
			vcoreparams.CLOName, vcoreparams.CLONamespace, err))
} // func VerifyCLODeployment (ctx SpecContext)

// CreateObjectBucketClaim asserts the ObjectBucketClaim created.
func CreateObjectBucketClaim(ctx SpecContext) {
	klog.V(vcoreparams.VCoreLogLevel).Infof("Create an objectBucketClaim for openshift-storage.noobaa.io")

	var err error

	objectBucketClaimObj := storage.NewObjectBucketClaimBuilder(APIClient,
		vcoreparams.ObjectBucketClaimName,
		vcoreparams.CLONamespace)

	if objectBucketClaimObj.Exists() {
		err := objectBucketClaimObj.Delete()
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("failed to delete objectBucketClaim %s from namespace %s; %v",
				vcoreparams.ObjectBucketClaimName, vcoreparams.CLONamespace, err))
	}

	_, err = objectBucketClaimObj.
		WithGenerateBucketName(vcoreparams.ObjectBucketClaimName).
		WithStorageClassName("openshift-storage.noobaa.io").Create()
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("failed to create objectBucketClaim %s in namespace %s due to %v",
			vcoreparams.ObjectBucketClaimName, vcoreparams.CLONamespace, err))

	klog.V(vcoreparams.VCoreLogLevel).Infof("Wait for the PVCs are created")

	err = await.WaitUntilPersistentVolumeClaimCreated(APIClient,
		vcoreparams.ODFNamespace,
		4,
		5*time.Minute,
		metav1.ListOptions{})
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf(
		"failed to create persistentVolumeClaims in namespace %s due to %v",
		vcoreparams.ODFNamespace, err))

	klog.V(vcoreparams.VCoreLogLevel).Infof("Verify storageClass %s created", vcoreparams.StorageClassName)

	_, err = storage.PullClass(APIClient, vcoreparams.StorageClassName)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("storageClass %s not found; %v",
		vcoreparams.StorageClassName, err))

	klog.V(vcoreparams.VCoreLogLevel).Infof("Wait until configmap %s created", vcoreparams.ObjectBucketClaimName)

	err = await.WaitUntilConfigMapCreated(APIClient,
		vcoreparams.ObjectBucketClaimName,
		vcoreparams.CLONamespace,
		5*time.Minute)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("configmap %s not found in namespace %s; %v",
		vcoreparams.ObjectBucketClaimName, vcoreparams.CLONamespace, err))
} // func CreateObjectBucketClaim (ctx SpecContext)

// CreateLokiStackInstance asserts the LokiStack instance created.
//
//nolint:funlen
func CreateLokiStackInstance(ctx SpecContext) {
	klog.V(vcoreparams.VCoreLogLevel).Infof("Create a LokiStack instance")

	time.Sleep(2 * time.Minute)

	var err error

	lokiSecretObj := secret.NewBuilder(APIClient,
		vcoreparams.LokiSecretName,
		vcoreparams.CLONamespace,
		corev1.SecretTypeOpaque)

	if lokiSecretObj.Exists() {
		err := lokiSecretObj.Delete()
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("failed to delete loki secret %s from namespace %s; %v",
				vcoreparams.LokiSecretName, vcoreparams.CLONamespace, err))
	}

	klog.V(vcoreparams.VCoreLogLevel).Infof("Create loki secret %s in namespace %s",
		vcoreparams.LokiSecretName, vcoreparams.CLONamespace)

	err = await.WaitUntilConfigMapCreated(APIClient,
		vcoreparams.ObjectBucketClaimName,
		vcoreparams.CLONamespace, time.Minute*15)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("configMap %s was not created in namespace %s; check objectBucketClaim %s in namespace %s "+
			"configuration due to %v",
			vcoreparams.ObjectBucketClaimName, vcoreparams.CLONamespace,
			vcoreparams.ObjectBucketClaimName, vcoreparams.CLONamespace, err))

	objectBucketClaimConfigMap, err := configmap.Pull(APIClient,
		vcoreparams.ObjectBucketClaimName, vcoreparams.CLONamespace)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("configMap %s not found in namespace %s due to %v",
			vcoreparams.ObjectBucketClaimName, vcoreparams.CLONamespace, err))

	cmData := objectBucketClaimConfigMap.Object.Data
	bucketHost := cmData["BUCKET_HOST"]
	bucketName := cmData["BUCKET_NAME"]
	bucketPort := cmData["BUCKET_PORT"]

	objectBucketClaimSecret, err := secret.Pull(APIClient,
		vcoreparams.ObjectBucketClaimName, vcoreparams.CLONamespace)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("secret %s not found in namespace %s; "+
			"check objectBucketClaim %s in namespace %s configuration; %v",
			vcoreparams.ObjectBucketClaimName, vcoreparams.CLONamespace,
			vcoreparams.ObjectBucketClaimName, vcoreparams.CLONamespace, err))

	secretData := objectBucketClaimSecret.Object.Data
	accessKeyID := string(secretData["AWS_ACCESS_KEY_ID"])
	secretAccessKey := string(secretData["AWS_SECRET_ACCESS_KEY"])

	stringData := map[string]string{
		"bucketnames":       bucketName,
		"endpoint":          fmt.Sprintf("https://%s:%s", bucketHost, bucketPort),
		"region":            "",
		"access_key_id":     accessKeyID,
		"access_key_secret": secretAccessKey,
	}

	_, err = lokiSecretObj.WithStringData(stringData).Create()
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to create secret %s in namespace %s due to: %v",
			vcoreparams.LokiSecretName, vcoreparams.CLONamespace, err))

	klog.V(vcoreparams.VCoreLogLevel).Infof("Create a LokiStack instance %s in namespace %s",
		vcoreparams.LokiStackName, vcoreparams.CLONamespace)

	lokiStackObj := clusterlogging.NewLokiStackBuilder(APIClient,
		vcoreparams.LokiStackName,
		vcoreparams.CLONamespace)

	if lokiStackObj.Exists() {
		_, err = lokiStackObj.Delete()
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("failed to delete lokiStack %s from namespace %s; %v",
				vcoreparams.LokiStackName, vcoreparams.CLONamespace, err))
	}

	lokiStackStorage := lokiv1.ObjectStorageSpec{
		Schemas: []lokiv1.ObjectStorageSchema{{
			EffectiveDate: lokiv1.StorageSchemaEffectiveDateFormat,
			Version:       lokiv1.ObjectStorageSchemaV13,
		}},
		Secret: lokiv1.ObjectStorageSecretSpec{
			Type: "s3",
			Name: vcoreparams.LokiSecretName,
		},
		TLS: &lokiv1.ObjectStorageTLSSpec{CASpec: lokiv1.CASpec{
			CA: "openshift-service-ca.crt",
		}},
	}

	lokiComponent := lokiv1.LokiComponentSpec{
		NodeSelector: map[string]string{"node-role.kubernetes.io/infra": ""},
		Tolerations: []corev1.Toleration{{
			Key:      "node-role.kubernetes.io/infra",
			Operator: "Exists",
		}},
	}

	template := lokiv1.LokiTemplateSpec{
		Distributor:   &lokiComponent,
		Querier:       &lokiComponent,
		QueryFrontend: &lokiComponent,
		Gateway:       &lokiComponent,
		IndexGateway:  &lokiComponent,
	}

	lokiStackObj, err = lokiStackObj.
		WithSize(lokiv1.SizeOneXSmall).
		WithManagementState(lokiv1.ManagementStateManaged).
		WithStorage(lokiStackStorage).
		WithStorageClassName(vcoreparams.StorageClassName).
		WithTenants(lokiv1.TenantsSpec{Mode: lokiv1.OpenshiftLogging}).
		WithTemplate(template).
		WithLimits(lokiv1.LimitsSpec{
			Global: &lokiv1.LimitsTemplateSpec{
				Retention: &lokiv1.RetentionLimitSpec{
					Days: 7,
				},
			},
		}).Create()
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("failed to create lokiStack instance %s in namespace %s due to %v",
			vcoreparams.LokiStackName, vcoreparams.CLONamespace, err))
	Expect(lokiStackObj.IsReady(5*time.Minute)).To(Equal(true),
		fmt.Sprintf("lokiStack instance %s in namespace %s failed to reach Ready state after 5 mins",
			vcoreparams.LokiStackName, vcoreparams.CLONamespace))

	time.Sleep(3 * time.Minute)
} // func CreateLokiStackInstance (ctx SpecContext)

// CreateClusterLogForwarderInstance asserts ClusterLogging instance can be created and running.
func CreateClusterLogForwarderInstance(ctx SpecContext) {
	klog.V(vcoreparams.VCoreLogLevel).Infof("Verify clusterLogging instance %s is running in namespace %s",
		vcoreparams.CLOInstanceName, vcoreparams.CLONamespace)

	var err error

	clusterLoggingObj := clusterlogging.NewClusterLogForwarderBuilder(
		APIClient, vcoreparams.CLOInstanceName, vcoreparams.CLONamespace)

	if clusterLoggingObj.Exists() {
		_ = clusterLoggingObj.Delete()
	}

	klog.V(vcoreparams.VCoreLogLevel).Infof("Create new Cluster Logging instance %s in namespace %s",
		vcoreparams.CLOInstanceName, vcoreparams.CLONamespace)

	//nolint:godox
	// ToDo need to be replaced with the ClusterLogForwarding object.
	// _, err = clusterLoggingObj.
	//	WithManagementState(clov1.ManagementStateManaged).
	//	WithLogStore(clov1.LogStoreSpec{
	//		Type:      "lokistack",
	//		LokiStack: clov1.LokiStackStoreSpec{Name: "logging-loki"},
	//	}).
	//	WithCollection(clov1.CollectionSpec{
	//		Type: "vector",
	//		CollectorSpec: clov1.CollectorSpec{
	//			Tolerations: []corev1.Toleration{{
	//				Key:      "node-role.kubernetes.io/infra",
	//				Operator: "Exists",
	//			}, {
	//				Key:      "node.ocs.openshift.io/storage",
	//				Operator: "Equal",
	//				Value:    "true",
	//				Effect:   "NoSchedule",
	//			}},
	//		}}).
	//	WithVisualization(clov1.VisualizationSpec{
	//		Type:         "ocp-console",
	//		NodeSelector: map[string]string{"node-role.kubernetes.io/infra": ""},
	//		Tolerations: []corev1.Toleration{{
	//			Key:      "node-role.kubernetes.io/infra",
	//			Operator: "Exists",
	//		}}}).Create()
	// Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to create clusterLogging instance %s in namespace %s; %v",
	//	vcoreparams.CLOInstanceName, vcoreparams.CLONamespace, err))

	klog.V(90).Infof("Check clusterLogging instance deployment")

	klog.V(90).Infof("Check %s deployment", vcoreparams.CLODeploymentName)

	err = await.WaitUntilDeploymentReady(APIClient, vcoreparams.CLODeploymentName,
		vcoreparams.CLONamespace, time.Minute)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Deployment %s in namespace %s failed due to %v",
		vcoreparams.CLODeploymentName, vcoreparams.CLONamespace, err))

	isReady, err := await.WaitForThePodReplicasCountInNamespace(APIClient, vcoreparams.CLONamespace, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/component=distributor",
	}, 2, time.Minute)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf(
		"Failed to create pods for the deployment %s in namespace %s due to %v",
		vcoreparams.CLODeploymentName, vcoreparams.CLONamespace, err))
	Expect(isReady).To(Equal(true),
		fmt.Sprintf("Failed to create pods for the  deployment %s in namespace %s",
			vcoreparams.CLODeploymentName, vcoreparams.CLONamespace))

	odfMcp := mco.NewMCPBuilder(APIClient, VCoreConfig.OdfMCPName)
	if odfMcp.IsInCondition("Updated") {
		deploymentsList := []string{"logging-loki-distributor", "logging-loki-gateway",
			"logging-loki-querier", "logging-loki-query-frontend"}

		for _, deploymentName := range deploymentsList {
			klog.V(90).Infof("Check %s deployment", deploymentName)

			err = await.WaitUntilDeploymentReady(APIClient, deploymentName, vcoreparams.CLONamespace, time.Minute)
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Deployment %s in namespace %s failed due to %v",
				deploymentName, vcoreparams.CLONamespace, err))

			isReady, err := await.WaitForThePodReplicasCountInNamespace(APIClient, vcoreparams.CLONamespace, metav1.ListOptions{
				LabelSelector: "app.kubernetes.io/component=distributor",
			}, 2, time.Minute)
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf(
				"Failed to create pods for the deployment %s in namespace %s due to %v",
				deploymentName, vcoreparams.CLONamespace, err))
			Expect(isReady).To(Equal(true), fmt.Sprintf("Failed to create pods for the  deployment %s"+
				"in namespace %s", deploymentName, vcoreparams.CLONamespace))
		}
	}

	klog.V(vcoreparams.VCoreLogLevel).Infof("Enable logging-view-plugin")

	consoleoperatorObj, err := console.PullConsoleOperator(APIClient, "cluster")
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("consoleoperator is unavailable: %v", err))

	_, err = consoleoperatorObj.WithPlugins([]string{"logging-view-plugin"}, false).Update()
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to enable logging-view-pluggin due to %v", err))
} // func CreateClusterLogForwarderInstance (ctx SpecContext)
