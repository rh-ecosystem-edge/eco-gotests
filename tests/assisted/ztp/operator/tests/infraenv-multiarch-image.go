package operator_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	goVersion "github.com/hashicorp/go-version"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/assisted"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/hive"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/assisted/api/hiveextension/v1beta1"
	agentInstallV1Beta1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/assisted/api/v1beta1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/assisted/models"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/secret"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/meets"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/ztpinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/ztpparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/operator/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/url"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

const (
	infraenvTestSpoke                string = "infraenv-spoke"
	assistedUnsupportedConfigMapName string = "assisted-unsupported-config-unique"
)

var (
	multiarchAgentServiceConfigBuilder                         *assisted.AgentServiceConfigBuilder
	testClusterImageSetName                                    string
	nsBuilder                                                  *namespace.Builder
	archURL                                                    string
	platformType                                               v1beta1.PlatformType
	originalClusterImagesetBuilder, tempClusterImagesetBuilder *hive.ClusterImageSetBuilder
	mirrorRegistryRef                                          *corev1.LocalObjectReference
	configmapBuilder                                           *configmap.Builder
)
var _ = Describe(
	"MultiArchitectureImage",
	Ordered,
	ContinueOnFailure,
	Label(tsparams.LabelMultiArchitectureImageTestCases), Label("disruptive"), func() {
		When("on MCE 2.2 and above", func() {
			BeforeAll(func() {
				By("Check the hub cluster is connected")
				connectedCluster, _ := meets.HubConnectedRequirement()
				if !connectedCluster {
					Skip("The hub cluster must be connected")
				}

				tsparams.ReporterNamespacesToDump[infraenvTestSpoke] = "infraenv-spoke namespace"

				nsBuilder = namespace.NewBuilder(HubAPIClient, infraenvTestSpoke)

				By("Grab the MirrorRegistryRef value from the spec")
				mirrorRegistryRef = ZTPConfig.HubAgentServiceConfig.Definition.Spec.MirrorRegistryRef

				By("Create configmap for AgentServiceConfig's annotation")
				agentServiceConfigMapData := map[string]string{
					"ISO_IMAGE_TYPE": "full-iso",
				}
				configmapBuilder = configmap.NewBuilder(HubAPIClient, assistedUnsupportedConfigMapName,
					ZTPConfig.HubAssistedServicePod().Object.Namespace)
				_, err = configmapBuilder.WithData(agentServiceConfigMapData).Create()
				Expect(err).ToNot(HaveOccurred(), "error creating configmap for AgentServiceConfig's annotation")

				By("Delete the pre-existing AgentServiceConfig")
				err = ZTPConfig.HubAgentServiceConfig.DeleteAndWait(time.Second * 10)
				Expect(err).ToNot(HaveOccurred(), "error deleting pre-existing agentserviceconfig")

				By("Retrieve ClusterImageSet before all tests if exists")
				testClusterImageSetName = ZTPConfig.HubOCPXYVersion + "-test"
				originalClusterImagesetBuilder, err = hive.PullClusterImageSet(HubAPIClient, testClusterImageSetName)

				By("Delete ClusterImageSet if exists before all tests ")
				if err == nil {
					err = originalClusterImagesetBuilder.Delete()
					Expect(err).ToNot(HaveOccurred(), "error deleting clusterimageset %s", testClusterImageSetName)
					klog.V(ztpparams.ZTPLogLevel).Infof("The ClusterImageSet %s was deleted", testClusterImageSetName)
				} else {
					klog.V(ztpparams.ZTPLogLevel).Infof("The ClusterImageSet %s doesn't exist. No attempt to delete it",
						testClusterImageSetName)
				}

				By("Create AgentServiceConfig with OS images for multiple architectures")
				multiarchAgentServiceConfigBuilder = assisted.NewDefaultAgentServiceConfigBuilder(HubAPIClient)

				// Add annotation to be able to use full ISO with s390x arch
				multiarchAgentServiceConfigBuilder.Definition.Annotations = map[string](string){}
				multiarchAgentServiceConfigBuilder.Definition.
					Annotations["unsupported.agent-install.openshift.io/assisted-service-configmap"] =
					assistedUnsupportedConfigMapName

				hubversion, err := goVersion.NewVersion(ZTPConfig.HubOCPXYVersion)
				Expect(err).ToNot(HaveOccurred(), "error parsing hub version")

				var isoFileSuffix string

				if hubversion.GreaterThanOrEqual(goVersion.Must(goVersion.NewVersion("4.19"))) {
					isoFileSuffix = "rhcos-live-iso"
				} else {
					isoFileSuffix = "rhcos-live"
				}

				if mirrorRegistryRef != nil {
					multiarchAgentServiceConfigBuilder.Definition.Spec.MirrorRegistryRef = mirrorRegistryRef
				}
				_, err = multiarchAgentServiceConfigBuilder.WithOSImage(agentInstallV1Beta1.OSImage{
					OpenshiftVersion: ZTPConfig.HubOCPXYVersion,
					Version:          ZTPConfig.HubOCPXYVersion,
					Url: getArchURL(
						"https://mirror.openshift.com/pub/openshift-v4/amd64/dependencies/rhcos/"+
							ZTPConfig.HubOCPXYVersion+"/latest/"+isoFileSuffix+".x86_64.iso",
						"https://mirror.openshift.com/pub/openshift-v4/amd64/dependencies/rhcos/pre-release/latest-"+
							ZTPConfig.HubOCPXYVersion+"/"+isoFileSuffix+".x86_64.iso"),
					CPUArchitecture: models.ClusterCPUArchitectureX8664}).WithOSImage(agentInstallV1Beta1.OSImage{
					OpenshiftVersion: ZTPConfig.HubOCPXYVersion,
					Version:          ZTPConfig.HubOCPXYVersion,
					Url: getArchURL(
						"https://mirror.openshift.com/pub/openshift-v4/aarch64/dependencies/rhcos/"+
							ZTPConfig.HubOCPXYVersion+"/latest/"+isoFileSuffix+".aarch64.iso",
						"https://mirror.openshift.com/pub/openshift-v4/aarch64/dependencies/rhcos/pre-release/latest-"+
							ZTPConfig.HubOCPXYVersion+"/"+isoFileSuffix+".aarch64.iso"),
					CPUArchitecture: models.ClusterCPUArchitectureArm64}).WithOSImage(agentInstallV1Beta1.OSImage{
					OpenshiftVersion: ZTPConfig.HubOCPXYVersion,
					Version:          ZTPConfig.HubOCPXYVersion,
					Url: getArchURL(
						"https://mirror.openshift.com/pub/openshift-v4/ppc64le/dependencies/rhcos/"+
							ZTPConfig.HubOCPXYVersion+"/latest/"+isoFileSuffix+".ppc64le.iso",
						"https://mirror.openshift.com/pub/openshift-v4/ppc64le/dependencies/rhcos/pre-release/latest-"+
							ZTPConfig.HubOCPXYVersion+"/"+isoFileSuffix+".ppc64le.iso"),
					CPUArchitecture: models.ClusterCPUArchitecturePpc64le}).WithOSImage(agentInstallV1Beta1.OSImage{
					OpenshiftVersion: ZTPConfig.HubOCPXYVersion,
					Version:          ZTPConfig.HubOCPXYVersion,
					Url: getArchURL(
						"https://mirror.openshift.com/pub/openshift-v4/s390x/dependencies/rhcos/"+
							ZTPConfig.HubOCPXYVersion+"/latest/"+isoFileSuffix+".s390x.iso",
						"https://mirror.openshift.com/pub/openshift-v4/s390x/dependencies/rhcos/pre-release/latest-"+
							ZTPConfig.HubOCPXYVersion+"/"+isoFileSuffix+".s390x.iso"),
					CPUArchitecture: models.ClusterCPUArchitectureS390x}).WithUnauthenticatedRegistry("registry.redhat.io").
					Create()
				Expect(err).ToNot(HaveOccurred(),
					"error creating agentserviceconfig with osimages for multiple archs")

				By("Wait until AgentServiceConfig with OSImages for multiple archs is deployed")
				_, err = multiarchAgentServiceConfigBuilder.WaitUntilDeployed(time.Minute * 10)
				Expect(err).ToNot(HaveOccurred(),
					"error waiting until agentserviceconfig with osimages for multiple archs is deployed")

			})
			AfterEach(func() {
				By("Delete the temporary namespace after test")
				if nsBuilder.Exists() {
					err := nsBuilder.DeleteAndWait(time.Second * 300)
					Expect(err).ToNot(HaveOccurred(), "error deleting the temporary namespace after test")
				}

				By("Delete ClusterImageSet after a test")
				err = tempClusterImagesetBuilder.Delete()
				Expect(err).ToNot(HaveOccurred(), "error deleting clusterimageset %s", testClusterImageSetName)
				klog.V(ztpparams.ZTPLogLevel).Infof("The ClusterImageSet %s was deleted", testClusterImageSetName)

			})
			AfterAll(func() {
				By("Re-create the original ClusterImageSet after all test if existed")
				if originalClusterImagesetBuilder != nil {
					_, err := originalClusterImagesetBuilder.Create()
					Expect(err).ToNot(HaveOccurred(), "error re-creating clusterimageset %s",
						testClusterImageSetName)
					klog.V(ztpparams.ZTPLogLevel).Infof("The ClusterImageSet %s was re-created", testClusterImageSetName)
				} else {
					klog.V(ztpparams.ZTPLogLevel).Infof("Skip on re-creating the ClusterImageSet after all tests - didn't exist.")
				}
				By("Delete AgentServiceConfig after test")
				err = multiarchAgentServiceConfigBuilder.DeleteAndWait(time.Second * 10)
				Expect(err).ToNot(HaveOccurred(), "error deleting agentserviceconfig after test")

				By("Delete ConfigMap after test")
				err = configmapBuilder.Delete()
				Expect(err).ToNot(HaveOccurred(), "error deleting configmap after test")

				By("Re-create the original AgentServiceConfig after all tests")
				_, err = ZTPConfig.HubAgentServiceConfig.Create()
				Expect(err).ToNot(HaveOccurred(), "error re-creating the original agentserviceconfig after all tests")

				_, err = ZTPConfig.HubAgentServiceConfig.WaitUntilDeployed(time.Minute * 10)
				Expect(err).ToNot(HaveOccurred(),
					"error waiting until the original agentserviceconfig is deployed")

				reqMet, msg := meets.HubInfrastructureOperandRunningRequirement()
				Expect(reqMet).To(BeTrue(), "error waiting for hub infrastructure operand to start running: %s", msg)
			})
			DescribeTable("Assert valid ISO is created by InfraEnv with different cpuArchitecture",
				func(cpuArchitecture string, payloadURL string, mismatchCPUArchitecture ...string) {

					By("Create ClusterImageSet")
					payloadVersion, payloadImage, err := getLatestReleasePayload(payloadURL)
					Expect(err).ToNot(HaveOccurred(), "error getting latest release payload image")

					klog.V(ztpparams.ZTPLogLevel).Infof("ClusterImageSet %s will use version %s with image %s",
						testClusterImageSetName, payloadVersion, payloadImage)
					tempClusterImagesetBuilder, err = hive.NewClusterImageSetBuilder(
						HubAPIClient, testClusterImageSetName, payloadImage).Create()
					Expect(err).ToNot(HaveOccurred(), "error creating clusterimageset %s", testClusterImageSetName)

					By("Create namespace with pull-secret for the spoke cluster")
					createSpokeClusterNamespace()

					if len(mismatchCPUArchitecture) > 0 {
						createSpokeClusterResources(cpuArchitecture, mismatchCPUArchitecture[0])
					} else {
						createSpokeClusterResources(cpuArchitecture)
					}

				},

				Entry("Assert valid ISO is created by InfraEnv with cpuArchitecture set to x86_64",
					models.ClusterCPUArchitectureX8664,
					"https://openshift-release.apps.ci.l2s4.p1.openshiftapps.com/graph",
					reportxml.ID("56183")),

				Entry("Assert valid ISO is created by InfraEnv with cpuArchitecture set to s390x",
					models.ClusterCPUArchitectureS390x,
					"https://s390x.ocp.releases.ci.openshift.org/graph",
					reportxml.ID("56184")),

				Entry("Assert valid ISO is created by InfraEnv with cpuArchitecture set to ppc64le",
					models.ClusterCPUArchitecturePpc64le,
					"https://ppc64le.ocp.releases.ci.openshift.org/graph",
					reportxml.ID("56185")),

				Entry("Assert valid ISO is created by InfraEnv with cpuArchitecture set to arm64",
					models.ClusterCPUArchitectureArm64,
					"https://arm64.ocp.releases.ci.openshift.org/graph",
					reportxml.ID("56186")),

				Entry("Assert the InfraEnv creation fails when the cpuArchitecture is mismatched with arm64 arch",
					models.ClusterCPUArchitectureX8664,
					"https://openshift-release.apps.ci.l2s4.p1.openshiftapps.com/graph",
					models.ClusterCPUArchitectureArm64,
					reportxml.ID("56190")),

				Entry("Assert the InfraEnv creation fails when the cpuArchitecture is mismatched with ppc64le arch",
					models.ClusterCPUArchitectureX8664,
					"https://openshift-release.apps.ci.l2s4.p1.openshiftapps.com/graph",
					models.ClusterCPUArchitecturePpc64le,
					reportxml.ID("56189")),

				Entry("Assert the InfraEnv creation fails when the cpuArchitecture is mismatched with s390x arch",
					models.ClusterCPUArchitectureX8664,
					"https://openshift-release.apps.ci.l2s4.p1.openshiftapps.com/graph",
					models.ClusterCPUArchitectureS390x,
					reportxml.ID("56188")),

				Entry("Assert the InfraEnv creation fails when the cpuArchitecture is mismatched with x86_64 arch",
					models.ClusterCPUArchitectureArm64,
					"https://arm64.ocp.releases.ci.openshift.org/graph",
					models.ClusterCPUArchitectureX8664,
					reportxml.ID("56187")),
			)
		})

	})

// getArchURL returns the proper ISO image URL.
func getArchURL(archURLReleased string, archURLPrereleased string) string {
	By("Check the rhcos-live ISO exists")

	archURL = archURLReleased

	_, statusCode, err := url.Fetch(archURL, "Head", true)
	if err != nil || statusCode != 200 {
		archURL = archURLPrereleased
		_, statusCode, err = url.Fetch(archURL, "Head", true)
	}

	Expect(statusCode).To(Equal(200), "bad HTTP status from url %s", archURL)
	Expect(err).ToNot(HaveOccurred(), "error reaching %s", archURL)

	klog.V(ztpparams.ZTPLogLevel).Infof("Verified ISO from URL %s exists", archURL)

	return archURL
}

// createSpokeClusterNamespace is a helper function that creates namespace
// for the spoke cluster.
func createSpokeClusterNamespace() {
	By("Create namespace for the test")

	if nsBuilder.Exists() {
		klog.V(ztpparams.ZTPLogLevel).Infof("The namespace '%s' already exists",
			nsBuilder.Object.Name)
	} else {
		klog.V(ztpparams.ZTPLogLevel).Infof("Creating the namespace:  %v", infraenvTestSpoke)

		_, err := nsBuilder.Create()
		Expect(err).ToNot(HaveOccurred(), "error creating namespace '%s' :  %v ",
			nsBuilder.Definition.Name, err)
	}
}

// createSpokeClusterResources is a helper function that creates
// spoke cluster resources required for the test.
//
//nolint:funlen
func createSpokeClusterResources(cpuArch string, mismatchCPUArchitecture ...string) {
	By("Create pull-secret in the new namespace")

	testSecret := secret.NewBuilder(
		HubAPIClient,
		fmt.Sprintf("%s-pull-secret", infraenvTestSpoke),
		infraenvTestSpoke,
		corev1.SecretTypeDockerConfigJson)

	if ZTPConfig.HubPullSecretOverridePath != "" {
		testSecret.WithData(ZTPConfig.HubPullSecretOverride)
	} else {
		testSecret.WithData(ZTPConfig.HubPullSecret.Object.Data)
	}

	_, err := testSecret.Create()
	Expect(err).ToNot(HaveOccurred(), "error occurred when creating pull-secret")

	By("Create clusterdeployment in the new namespace")

	testClusterDeployment, err := hive.NewABMClusterDeploymentBuilder(
		HubAPIClient,
		infraenvTestSpoke,
		infraenvTestSpoke,
		infraenvTestSpoke,
		"assisted.test.com",
		infraenvTestSpoke,
		metav1.LabelSelector{
			MatchLabels: map[string]string{
				"dummy": "label",
			},
		}).WithPullSecret(testSecret.Definition.Name).Create()
	Expect(err).ToNot(HaveOccurred(), "error occurred when creating clusterdeployment")

	By("Create agentclusterinstall in the new namespace")

	_, err = assisted.NewAgentClusterInstallBuilder(
		HubAPIClient,
		infraenvTestSpoke,
		infraenvTestSpoke,
		testClusterDeployment.Object.Name,
		3,
		2,
		v1beta1.Networking{
			ClusterNetwork: []v1beta1.ClusterNetworkEntry{{
				CIDR:       "10.128.0.0/14",
				HostPrefix: 23,
			}},
			ServiceNetwork: []string{"172.30.0.0/16"},
		}).WithImageSet(testClusterImageSetName).
		WithPlatformType(platformType).
		WithUserManagedNetworking(true).Create()
	Expect(err).ToNot(HaveOccurred(), "error creating agentclusterinstall")

	By("Create infraenv in the new namespace")

	cpuArchTemp := cpuArch

	if len(mismatchCPUArchitecture) > 0 {
		cpuArchTemp = mismatchCPUArchitecture[0]
	}

	infraEnvBuilder, err := assisted.NewInfraEnvBuilder(
		HubAPIClient,
		infraenvTestSpoke,
		infraenvTestSpoke,
		testSecret.Definition.Name).WithCPUType(cpuArchTemp).WithClusterRef(infraenvTestSpoke, infraenvTestSpoke).Create()

	Expect(err).ToNot(HaveOccurred(), "error creating infraenv with cpu architecture %s", cpuArchTemp)

	if len(mismatchCPUArchitecture) == 0 {
		By("Wait until the discovery iso is created for the infraenv")

		Eventually(func() (string, error) {
			infraEnvBuilder.Object, err = infraEnvBuilder.Get()
			if err != nil {
				return "", err
			}

			return infraEnvBuilder.Object.Status.ISODownloadURL, nil
		}).WithTimeout(time.Minute*3).ProbeEvery(time.Second*3).
			Should(Not(BeEmpty()), "error waiting for download url to be created")
	} else {
		By("Wait until infraenv shows an error for the mismatching image architecture")

		err = wait.PollUntilContextTimeout(
			context.TODO(), 5*time.Second, time.Minute*2, true, func(ctx context.Context) (bool, error) {
				infraEnvBuilder.Object, _ = infraEnvBuilder.Get()
				if len(infraEnvBuilder.Object.Status.Conditions) > 0 {
					for _, condition := range infraEnvBuilder.Object.Status.Conditions {
						if condition.Type == agentInstallV1Beta1.ImageCreatedCondition {
							if condition.Message ==
								"Failed to create image: Specified CPU architecture ("+
									mismatchCPUArchitecture[0]+
									") doesn't match the cluster ("+cpuArch+")" {
								return true, nil
							}
						}
					}
				}

				return false, nil
			})
		Expect(err).ToNot(HaveOccurred(), "didn't get the message about mismatching CPU architecture")
	}
}

// getLatestReleasePayload is a helper function that returns
// the latest release payload image for the version.
func getLatestReleasePayload(url string) (string, string, error) {
	response, err := http.Get(url)
	if err != nil {
		return "", "", err
	}
	defer response.Body.Close()

	urlContent, err := io.ReadAll(response.Body)
	if err != nil {
		return "", "", err
	}

	releaseImages := make(map[string]string)

	images, err := readJSON(urlContent)
	if err != nil {
		return "", "", err
	}

	for _, i := range images.Nodes {
		if len(strings.Split(i.Version, ".")) >= 2 {
			if strings.Split(i.Version, ".")[0]+"."+strings.Split(i.Version, ".")[1] == ZTPConfig.HubOCPXYVersion &&
				!strings.Contains(i.Version, "nightly") &&
				!strings.Contains(i.Payload, "dev-preview") {
				releaseImages[i.Version] = i.Payload
			}
		}
	}

	releaseImageVersions := make([]string, 0)

	for version := range releaseImages {
		releaseImageVersions = append(releaseImageVersions, version)
	}

	sort.Sort(sort.Reverse(sort.StringSlice(releaseImageVersions)))

	return releaseImageVersions[0], releaseImages[releaseImageVersions[0]], nil
}

// Images is a helper struct to encode OSImages correctly.
type Images struct {
	Nodes []struct {
		Version, Payload string
	} `json:"nodes"`
}

// readJSON is a helper function that reads a json from URL
// and returns the payload release version and image location.
func readJSON(urlContent []byte) (Images, error) {
	var images Images

	err := json.Unmarshal(urlContent, &images)
	if err != nil {
		return images, err
	}

	return images, nil
}
