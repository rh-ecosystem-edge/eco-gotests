package rdscorecommon

import (
	"fmt"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreparams"
)

const (
	// imageSignatureContainerName is the container name used by the image-signature workloads.
	imageSignatureContainerName = "sleep-container"
	// imageSignatureFailureMarker is the substring reported by CRI-O when signature validation fails.
	imageSignatureFailureMarker = "SignatureValidationFailed"
	// imageSignatureReadyTimeout is the time to wait for a signed workload to become ready.
	imageSignatureReadyTimeout = 5 * time.Minute
	// imageSignatureFailTimeout is the time to wait for an unsigned workload to be rejected.
	imageSignatureFailTimeout = 3 * time.Minute
	// imageSignatureCMPullTimeout is the time to wait for the image-signature ConfigMap to be available.
	imageSignatureCMPullTimeout = 1 * time.Minute
)

// Image-signature ConfigMap keys. These must match the keys written into the ConfigMap referenced by
// RDSCoreConfig.ImageSignatureConfigMapName by the out-of-band image-provisioning script.
const (
	imageSignatureKeyPublicKeyRegularSigned     = "rds_pubkey_regular_signed"
	imageSignatureKeyPublicKeyRegularUnsigned   = "rds_pubkey_regular_unsigned"
	imageSignatureKeyPublicKeyMultiArchSigned   = "rds_pubkey_multiarch_signed"
	imageSignatureKeyPublicKeyMultiArchUnsigned = "rds_pubkey_multiarch_unsigned"
	imageSignatureKeyCertRegularSigned          = "rds_cert_regular_signed"
	imageSignatureKeyCertRegularUnsigned        = "rds_cert_regular_unsigned"
	imageSignatureKeyCertMultiArchSigned        = "rds_cert_multiarch_signed"
	imageSignatureKeyCertMultiArchUnsigned      = "rds_cert_multiarch_unsigned"
)

// imageSignatureDefaultCmd is the command used to keep the workload container running.
var imageSignatureDefaultCmd = []string{"sleep", "infinity"}

// imageSignatureCase describes a single ClusterImagePolicy validation scenario.
type imageSignatureCase struct {
	// deployName is the unique deployment/workload name for the scenario.
	deployName string
	// imageFn resolves the scenario image at run time from the image-signature ConfigMap. It is a
	// function because DescribeTable entries are evaluated at spec-tree build time, before the
	// configuration is loaded and before the cluster is reachable, so the image must not be read
	// until the spec actually runs.
	imageFn func(ctx SpecContext) string
	// shouldSucceed indicates whether the deployment is expected to become ready (true) or to be
	// rejected due to failed image signature validation (false).
	shouldSucceed bool
}

// imageSignatureCases enumerates the ClusterImagePolicy validation scenarios, keyed by a stable
// case key referenced from the DescribeTable entries. The permutations cover the signature
// validation method (publicKey, Certificate), the image type (regular, multi-arch) and the
// expected outcome (pass, fail).
var imageSignatureCases = map[string]imageSignatureCase{
	"publickey-regular-signed": {
		deployName: "rds-crio-pubkey-reg-signed",
		imageFn: func(ctx SpecContext) string {
			return imageSignatureImage(ctx, imageSignatureKeyPublicKeyRegularSigned)
		},
		shouldSucceed: true,
	},
	"publickey-regular-unsigned": {
		deployName: "rds-crio-pubkey-reg-unsigned",
		imageFn: func(ctx SpecContext) string {
			return imageSignatureImage(ctx, imageSignatureKeyPublicKeyRegularUnsigned)
		},
		shouldSucceed: false,
	},
	"publickey-multiarch-signed": {
		deployName: "rds-crio-pubkey-multi-signed",
		imageFn: func(ctx SpecContext) string {
			return imageSignatureImage(ctx, imageSignatureKeyPublicKeyMultiArchSigned)
		},
		shouldSucceed: true,
	},
	"publickey-multiarch-unsigned": {
		deployName: "rds-crio-pubkey-multi-unsigned",
		imageFn: func(ctx SpecContext) string {
			return imageSignatureImage(ctx, imageSignatureKeyPublicKeyMultiArchUnsigned)
		},
		shouldSucceed: false,
	},
	"certificate-regular-signed": {
		deployName: "rds-crio-cert-reg-signed",
		imageFn: func(ctx SpecContext) string {
			return imageSignatureImage(ctx, imageSignatureKeyCertRegularSigned)
		},
		shouldSucceed: true,
	},
	"certificate-regular-unsigned": {
		deployName: "rds-crio-cert-reg-unsigned",
		imageFn: func(ctx SpecContext) string {
			return imageSignatureImage(ctx, imageSignatureKeyCertRegularUnsigned)
		},
		shouldSucceed: false,
	},
	"certificate-multiarch-signed": {
		deployName: "rds-crio-cert-multi-signed",
		imageFn: func(ctx SpecContext) string {
			return imageSignatureImage(ctx, imageSignatureKeyCertMultiArchSigned)
		},
		shouldSucceed: true,
	},
	"certificate-multiarch-unsigned": {
		deployName: "rds-crio-cert-multi-unsigned",
		imageFn: func(ctx SpecContext) string {
			return imageSignatureImage(ctx, imageSignatureKeyCertMultiArchUnsigned)
		},
		shouldSucceed: false,
	},
}

// VerifyImageSignaturePolicy validates a single ClusterImagePolicy scenario identified by caseKey.
// A positive scenario expects the workload to become ready; a negative scenario expects the workload
// to be rejected because image signature validation failed.
func VerifyImageSignaturePolicy(ctx SpecContext, caseKey string) {
	testCase, found := imageSignatureCases[caseKey]
	Expect(found).To(BeTrue(), fmt.Sprintf("Unknown image signature case %q", caseKey))

	// Skip rather than fail when the image-signature inputs are not configured, so the scenario
	// is only exercised on clusters where the test namespace and image ConfigMap are provisioned.
	if RDSCoreConfig.ImageSignatureTestNS == "" ||
		RDSCoreConfig.ImageSignatureConfigMapName == "" ||
		RDSCoreConfig.ImageSignatureConfigMapNamespace == "" {
		Skip("Image signature test namespace and/or image ConfigMap are not configured")
	}

	verifyImageSignatureDeployment(ctx, testCase.deployName,
		RDSCoreConfig.ImageSignatureTestNS, testCase.imageFn(ctx), testCase.shouldSucceed)
}

// imageSignatureImage resolves an image reference for the given key from the image-signature
// ConfigMap. Because the image digests are dynamic, they are stored in a ConfigMap (populated by
// the out-of-band image-provisioning script) and read at run time rather than from static config.
func imageSignatureImage(ctx SpecContext, key string) string {
	cmName := RDSCoreConfig.ImageSignatureConfigMapName
	cmNs := RDSCoreConfig.ImageSignatureConfigMapNamespace

	By(fmt.Sprintf("Resolving image for key %q from ConfigMap %q in %q ns", key, cmName, cmNs))

	Expect(cmName).ToNot(BeEmpty(), "Image signature ConfigMap name is not configured")
	Expect(cmNs).ToNot(BeEmpty(), "Image signature ConfigMap namespace is not configured")

	var cmBuilder *configmap.Builder

	Eventually(func() error {
		var err error

		cmBuilder, err = configmap.Pull(APIClient, cmName, cmNs)
		if err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"Error pulling ConfigMap %q from %q ns: %v", cmName, cmNs, err)
		}

		return err
	}).WithContext(ctx).WithPolling(5*time.Second).WithTimeout(imageSignatureCMPullTimeout).
		Should(Succeed(), fmt.Sprintf("Failed to pull ConfigMap %q from %q ns", cmName, cmNs))

	image, keyExists := lookupImageSignatureKey(cmBuilder.Object.Data, key)
	Expect(keyExists).To(BeTrue(),
		fmt.Sprintf("ConfigMap %q in %q ns does not contain key %q", cmName, cmNs, key))
	Expect(image).ToNot(BeEmpty(),
		fmt.Sprintf("Image for key %q in ConfigMap %q in %q ns is empty", key, cmName, cmNs))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Resolved key %q to image %q", key, image)

	return image
}

// lookupImageSignatureKey returns the value for key from data. It first attempts an exact match and
// falls back to a case-insensitive match, tolerating inconsistent key casing in the ConfigMap.
func lookupImageSignatureKey(data map[string]string, key string) (string, bool) {
	if value, ok := data[key]; ok {
		return value, true
	}

	for dataKey, value := range data {
		if strings.EqualFold(dataKey, key) {
			return value, true
		}
	}

	return "", false
}

// verifyImageSignatureDeployment creates a deployment from the given image and, based on shouldSucceed,
// asserts the workload either becomes ready (signed image) or is rejected because of failed image
// signature validation (unsigned image).
func verifyImageSignatureDeployment(ctx SpecContext, deployName, deployNs, image string, shouldSucceed bool) {
	By(fmt.Sprintf("Validating image signature deployment %q in %q ns (shouldSucceed=%t)",
		deployName, deployNs, shouldSucceed))

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Image signature validation: deployment %q, namespace %q, image %q, shouldSucceed=%t",
		deployName, deployNs, image, shouldSucceed)

	Expect(deployNs).ToNot(BeEmpty(), "Image signature test namespace is not configured")
	Expect(image).ToNot(BeEmpty(), "Image signature test image is not configured")

	ensureImageSignatureNamespace(deployNs)

	By(fmt.Sprintf("Removing stale deployment %q from %q ns", deployName, deployNs))

	deleteDeployments(deployName, deployNs)

	By(fmt.Sprintf("Defining container %q with image %q", imageSignatureContainerName, image))

	deployContainer := pod.NewContainerBuilder(imageSignatureContainerName, image, imageSignatureDefaultCmd)

	// Clear the builder's hardcoded RunAsUser/RunAsGroup so the namespace's SCC assigns a UID/GID
	// from its allocated range; a hardcoded UID is rejected by the restricted-v2 SCC.
	deployContainer = deployContainer.WithSecurityContext(&corev1.SecurityContext{RunAsGroup: nil, RunAsUser: nil})

	deployContainerCfg, err := deployContainer.GetContainerCfg()
	Expect(err).ToNot(HaveOccurred(), "Failed to get container definition")

	deployLabels := map[string]string{"app": deployName}

	deploy := defineBaseDeployment(deployContainerCfg, deployName, deployNs, deployLabels, 1)
	Expect(deploy).ToNot(BeNil(), fmt.Sprintf("Failed to define deployment %q", deployName))

	if shouldSucceed {
		By(fmt.Sprintf("Creating deployment %q and waiting until it is ready", deployName))

		_, err = deploy.CreateAndWaitUntilReady(imageSignatureReadyTimeout)
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Signed image deployment %q in %q ns failed to become ready", deployName, deployNs))

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Signed image deployment %q in %q ns is ready as expected", deployName, deployNs)
	} else {
		By(fmt.Sprintf("Creating deployment %q that is expected to fail signature validation", deployName))

		_, err = deploy.Create()
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to create deployment %q in %q ns", deployName, deployNs))

		waitForImageSignatureRejection(ctx, deployNs, fmt.Sprintf("app=%s", deployName))

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Unsigned image deployment %q in %q ns was rejected as expected", deployName, deployNs)
	}
}

// ensureImageSignatureNamespace creates the workload namespace if it does not already exist.
func ensureImageSignatureNamespace(nsName string) {
	By(fmt.Sprintf("Ensuring namespace %q exists", nsName))

	nsBuilder := namespace.NewBuilder(APIClient, nsName)
	if !nsBuilder.Exists() {
		_, err := nsBuilder.Create()
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to create namespace %q", nsName))
	}
}

// waitForImageSignatureRejection asserts that pods matching podLabel report a failed image signature
// validation in their container status.
func waitForImageSignatureRejection(ctx SpecContext, deployNs, podLabel string) {
	By("Verifying pods are rejected due to failed image signature validation")

	Eventually(func() (string, error) {
		pods, err := pod.List(APIClient, deployNs, metav1.ListOptions{LabelSelector: podLabel})
		if err != nil {
			return "", err
		}

		if len(pods) == 0 {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"No pods found yet for label %q in %q ns", podLabel, deployNs)

			return "", fmt.Errorf("no pods found for label %q in %q ns", podLabel, deployNs)
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Found %d pod(s) for label %q in %q ns", len(pods), podLabel, deployNs)

		for _, podObj := range pods {
			for _, containerStatus := range podObj.Object.Status.ContainerStatuses {
				if containerStatus.State.Waiting != nil {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod %q container %q waiting: %s - %s",
						podObj.Object.Name, containerStatus.Name,
						containerStatus.State.Waiting.Reason, containerStatus.State.Waiting.Message)

					return containerStatus.State.Waiting.Message, nil
				}
			}
		}

		return "", fmt.Errorf("no waiting container status found yet for label %q in %q ns", podLabel, deployNs)
	}).WithContext(ctx).WithPolling(10*time.Second).WithTimeout(imageSignatureFailTimeout).
		Should(ContainSubstring(imageSignatureFailureMarker),
			fmt.Sprintf("Expected pods in %q ns to fail with %q", deployNs, imageSignatureFailureMarker))
}
