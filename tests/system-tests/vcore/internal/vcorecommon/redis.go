package vcorecommon

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/remote"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/platform"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/statefulset"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/secret"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/template"
	corev1 "k8s.io/api/core/v1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/vcore/internal/vcoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/vcore/internal/vcoreparams"
)

// VerifyRedisSuite container that contains tests for the Redis deployment verification.
func VerifyRedisSuite() {
	Describe(
		"Redis validation",
		Label(vcoreparams.LabelVCoreOperators), func() {
			It("Verify Redis deployment procedure",
				Label("redis"), reportxml.ID("59503"), VerifyRedisDeploymentProcedure)
		})
}

// VerifyRedisDeploymentProcedure asserts Redis deployment procedure.
//
//nolint:funlen
func VerifyRedisDeploymentProcedure(ctx SpecContext) {
	klog.V(vcoreparams.VCoreLogLevel).Infof("Verify Redis can be installed and works correctly")

	redisAppName := "redis-ha"
	redisNamespace := "redis-ha"
	redisSecretName := "redis-secret"
	redisCustomValuesTemplate := "redis-custom-values.yaml"
	redisStatefulsetName := "redis-ha-server"

	klog.V(vcoreparams.VCoreLogLevel).Info("Check if redis already installed")

	redisStatefulset, err := statefulset.Pull(APIClient, redisStatefulsetName, redisNamespace)

	if err == nil && redisStatefulset.IsReady(time.Second) {
		klog.V(vcoreparams.VCoreLogLevel).Infof("redis statefulset %s in namespace %s exists and ready",
			redisStatefulsetName, redisNamespace)
	} else {
		redisConfigFilePath := filepath.Join(vcoreparams.ConfigurationFolderPath, redisCustomValuesTemplate)

		redisImageRepository := "quay.io/cloud-bulldozer"
		redisImageName := "redis"
		redisImageTag := "latest"

		klog.V(vcoreparams.VCoreLogLevel).Info("Check that cluster pull-secret can be retrieved")

		clusterPullSecret, err := cluster.GetOCPPullSecret(APIClient)
		Expect(err).ToNot(HaveOccurred(), "error occurred when retrieving cluster pull-secret")

		imageURL := fmt.Sprintf("%s/%s", redisImageRepository, redisImageName)

		isDisconnected, err := platform.IsDisconnectedDeployment(APIClient)
		Expect(err).ToNot(HaveOccurred(), "failed to detect a deployment type")

		if !isDisconnected {
			klog.V(vcoreparams.VCoreLogLevel).Info("The connected deployment type was detected, " +
				"the images mirroring is not required")
		} else {
			_, err = getImageURL(redisImageRepository, redisImageName, redisImageTag)
			Expect(err).ToNot(HaveOccurred(),
				fmt.Sprintf("Failed to mirror image for %s/%s:%s due to: %v",
					redisImageRepository, redisImageName, redisImageTag, err))
		}

		klog.V(vcoreparams.VCoreLogLevel).Infof("Install redis")

		klog.V(100).Infof("Ensure local directory %s exists", vcoreparams.ConfigurationFolderPath)

		err = os.Mkdir(vcoreparams.ConfigurationFolderPath, 0755)
		if err != nil {
			klog.V(100).Infof("Failed to create directory %s, it is already exists",
				vcoreparams.ConfigurationFolderPath)
		}

		installRedisCmd := []string{
			"helm repo add dandydev https://dandydeveloper.github.io/charts",
			"helm repo update",
			fmt.Sprintf("helm fetch dandydev/redis-ha --version 4.12.9 -d %s/.",
				vcoreparams.ConfigurationFolderPath),
			fmt.Sprintf("tar xvfz %s/redis-ha-4.12.9.tgz --directory=%s/.",
				vcoreparams.ConfigurationFolderPath, vcoreparams.ConfigurationFolderPath)}
		for _, cmd := range installRedisCmd {
			_, err = remote.ExecCmdOnHost(VCoreConfig.Host, VCoreConfig.User, VCoreConfig.Pass, cmd)
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to execute %s command due to %v", cmd, err))
		}

		klog.V(vcoreparams.VCoreLogLevel).Info("Create redis namespace")

		redisNamespaceBuilder := namespace.NewBuilder(APIClient, redisNamespace)

		if !redisNamespaceBuilder.Exists() {
			_, err = redisNamespaceBuilder.Create()
			Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to create namespace %s due to %v",
				redisNamespace, err))
			Expect(redisNamespaceBuilder.Exists()).To(Equal(true),
				fmt.Sprintf("namespace %s not found", redisNamespace))
		}

		klog.V(vcoreparams.VCoreLogLevel).Infof("Create redis secret %s in namespace %s",
			redisSecretName, redisNamespace)

		redisSecretBuilder := secret.NewBuilder(
			APIClient,
			redisSecretName,
			redisNamespace,
			corev1.SecretTypeDockerConfigJson)

		if redisSecretBuilder.Exists() {
			err = redisSecretBuilder.Delete()
			Expect(err).ToNot(HaveOccurred(), "failed to delete redis secret %s in namespace %s due to %w",
				redisSecretName, redisNamespace, err)
		}

		redisSecretBuilder, err = redisSecretBuilder.WithData(clusterPullSecret.Object.Data).Create()
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to create redis secret %s in namespace %s due %v",
			redisSecretName, redisNamespace, err))
		Expect(redisSecretBuilder.Exists()).To(Equal(true),
			fmt.Sprintf("Failed to create redis secret %s in namespace %s", redisSecretName, redisNamespace))

		klog.V(vcoreparams.VCoreLogLevel).Info("Get runAsUser and fsGroup values")

		fsGroupFull := ""
		_ = wait.PollUntilContextTimeout(
			context.TODO(), 3*time.Second, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
				fsGroupFull = redisNamespaceBuilder.Object.Annotations["openshift.io/sa.scc.supplemental-groups"]
				if fsGroupFull == "" {
					klog.V(90).Infof("no fsGroup was defined yet, retry")

					return false, nil
				}

				return true, nil
			})

		Expect(fsGroupFull).ToNot(Equal(""),
			fmt.Sprintf("failed to get fsGroup value for the namespase %s; fsGroup is %s; "+
				"redis ns: %s; Definition %s",
				redisNamespace, fsGroupFull, redisNamespaceBuilder.Object, redisNamespaceBuilder.Definition))

		fsGroup := strings.Split(fsGroupFull, "/")[0]

		runAsUserFull := redisNamespaceBuilder.Object.Annotations["openshift.io/sa.scc.uid-range"]
		Expect(runAsUserFull).ToNot(Equal(""), fmt.Sprintf("failed to get runAsUser value for the namespase %s",
			redisNamespace))

		runAsUser := strings.Split(runAsUserFull, "/")[0]

		klog.V(vcoreparams.VCoreLogLevel).Info("Redis custom config")

		varsToReplace := make(map[string]interface{})
		varsToReplace["ImageRepository"] = imageURL
		varsToReplace["ImageTag"] = redisImageTag
		varsToReplace["RedisSecret"] = redisSecretName
		varsToReplace["StorageClass"] = vcoreparams.StorageClassName
		varsToReplace["RunAsUser"] = runAsUser
		varsToReplace["FsGroup"] = fsGroup

		workingDir, err := os.Getwd()
		Expect(err).ToNot(HaveOccurred(), err)

		templateDir := filepath.Join(workingDir, vcoreparams.TemplateFilesFolder)
		cfgFilePath := filepath.Join(vcoreparams.ConfigurationFolderPath, redisCustomValuesTemplate)

		err = template.SaveTemplate(
			filepath.Join(templateDir, redisCustomValuesTemplate),
			cfgFilePath,
			varsToReplace)
		Expect(err).ToNot(HaveOccurred(), "failed to create config file %s at folder %s due to %w",
			redisCustomValuesTemplate, vcoreparams.ConfigurationFolderPath, err)

		err = remote.ScpFileTo(cfgFilePath, cfgFilePath, VCoreConfig.Host, VCoreConfig.User, VCoreConfig.Pass)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to transfer file %s to the %s/%s due to: %v",
			cfgFilePath, VCoreConfig.Host, cfgFilePath, err))

		customConfigCmd := fmt.Sprintf("helm upgrade --install %s -n %s %s/%s -f %s --kubeconfig=%s",
			redisAppName, redisNamespace, vcoreparams.ConfigurationFolderPath, redisAppName,
			redisConfigFilePath, VCoreConfig.KubeconfigPath)
		klog.V(vcoreparams.VCoreLogLevel).Infof("Execute command %s", customConfigCmd)

		result, err := remote.ExecCmdOnHost(VCoreConfig.Host, VCoreConfig.User, VCoreConfig.Pass, customConfigCmd)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to config redis due to %v", err))
		Expect(strings.Contains(result, "STATUS: deployed")).To(Equal(true),
			fmt.Sprintf("redis is not properly configured: %s", result))
	}

	klog.V(vcoreparams.VCoreLogLevel).Info("Wait for the statefulset ready")

	redisStatefulset, err = statefulset.Pull(APIClient, redisStatefulsetName, redisNamespace)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("statefulset %s not found in namespace %s; %v",
		redisStatefulsetName, redisNamespace, err))
	Expect(redisStatefulset.IsReady(5*time.Minute)).To(Equal(true),
		fmt.Sprintf("statefulset %s in namespace %s is not ready after 5 minutes",
			redisStatefulsetName, redisNamespace))

	klog.V(vcoreparams.VCoreLogLevel).Info("Verify redis server pods count")

	podsList, err := pod.ListByNamePattern(APIClient, redisAppName, redisNamespace)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("pods %s not found in namespace %s; %v",
		redisAppName, redisNamespace, err))
	Expect(len(podsList)).To(Equal(3), fmt.Sprintf("not all redis servers pods %s found in namespace %s;"+
		"expected: 3, found: %d", redisAppName, redisNamespace, len(podsList)))
} // func VerifyRedisDeploymentProcedure (ctx SpecContext)
