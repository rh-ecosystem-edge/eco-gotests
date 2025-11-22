package tests

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift-kni/cluster-group-upgrades-operator/pkg/api/clustergroupupgrades/v1alpha1"
	subscriptionsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/cgu"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ocm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/rancluster"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranhelper"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/version"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/talm/internal/helper"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/talm/internal/setup"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/talm/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	configurationPolicyv1 "open-cluster-management.io/config-policy-controller/api/v1"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
)

var _ = Describe("TALM precache", Label(tsparams.LabelPreCacheTestCases), func() {
	When("there is a single spoke", func() {
		Context("precache operator", func() {
			var (
				policies []string
				suffixes []string
			)

			BeforeEach(func() {
				By("verifying TalmPrecachePolicies from config are available on hub")
				preCachePolicies, exist := checkPoliciesExist(
					HubAPIClient, RANConfig.TalmPreCachePolicies)
				if !exist {
					Skip("could not find all policies in TalmPreCachePolicies in config on hub")
				}

				policies, suffixes = copyPoliciesWithSubscription(preCachePolicies)

				for _, suffix := range suffixes {
					policies = append(policies, tsparams.PolicyName+suffix)
				}
			})

			AfterEach(func() {
				for _, suffix := range suffixes {
					errorList := setup.CleanupTestResourcesOnHub(HubAPIClient, tsparams.TestNamespace, suffix)
					Expect(errorList).To(BeEmpty(), "Failed to clean up resources on hub for suffix %s", suffix)
				}

				// Cleanup test resources on hub does not account for the CGU, which does not have a
				// suffix. Therefore, it must be deleted separately to ensure proper cleanup.
				cguBuilder, err := cgu.Pull(HubAPIClient, tsparams.CguName, tsparams.TestNamespace)
				if err == nil {
					_, err = cguBuilder.DeleteAndWait(5 * time.Minute)
					Expect(err).ToNot(HaveOccurred(), "Failed to delete the test CGU")
				}
			})

			// 48902 Tests image precaching - operators
			It("tests for precache operator with multiple sources", reportxml.ID("48902"), func() {
				By("creating CGU with created operator upgrade policy")
				cguBuilder := getPrecacheCGU(policies, []string{RANConfig.Spoke1Name})
				_, err := cguBuilder.Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create CGU")

				By("waiting until CGU pre cache Succeeded")
				assertPrecacheStatus(RANConfig.Spoke1Name, "Succeeded")

				By("verifying image precache pod succeeded on spoke")
				err = checkPrecachePodLog(Spoke1APIClient)
				Expect(err).ToNot(HaveOccurred(), "Failed to check the precache pod log")
			})
		})

		Context("precache OCP with version", func() {
			AfterEach(func() {
				By("cleaning up resources on hub")
				errorList := setup.CleanupTestResourcesOnHub(HubAPIClient, tsparams.TestNamespace, "")
				Expect(errorList).To(BeEmpty(), "Failed to clean up test resources on hub")
			})

			// 47950 Tests ocp upgrade with image precaching enabled
			It("tests for ocp cache with version", reportxml.ID("47950"), func() {
				By("creating and applying policy with clusterversion CR that defines the upgrade graph, channel, and version")
				cguBuilder := getPrecacheCGU([]string{tsparams.PolicyName}, []string{RANConfig.Spoke1Name})

				clusterVersion, err := helper.GetClusterVersionDefinition(Spoke1APIClient, "Version")
				Expect(err).ToNot(HaveOccurred(), "Failed to get cluster version definition")

				_, err = helper.SetupCguWithClusterVersion(cguBuilder, clusterVersion)
				Expect(err).ToNot(HaveOccurred(), "Failed to setup cgu with cluster version")

				By("waiting until CGU Succeeded")
				assertPrecacheStatus(RANConfig.Spoke1Name, "Succeeded")

				By("waiting until new precache pod in spoke1 succeeded and log reports done")
				err = checkPrecachePodLog(Spoke1APIClient)
				Expect(err).ToNot(HaveOccurred(), "Failed to check the precache pod log")
			})
		})

		Context("precache OCP with image", func() {
			var (
				excludedPreCacheImage string
				imageListCommand      string
				imageDeleteCommand    string
			)

			BeforeEach(func() {
				By("finding image to exclude")
				prometheusPod, err := pod.Pull(
					Spoke1APIClient, tsparams.PrometheusPodName, tsparams.PrometheusNamespace)
				Expect(err).ToNot(HaveOccurred(), "Failed to pull prometheus pod")

				getImageNameCommand := fmt.Sprintf(
					tsparams.SpokeImageGetNameCommand, prometheusPod.Definition.Spec.Containers[0].Image)
				excludedPreCacheImages, err := cluster.ExecCmdWithStdoutWithRetries(
					Spoke1APIClient,
					ranparam.RetryCount, ranparam.RetryInterval,
					getImageNameCommand,
					metav1.ListOptions{LabelSelector: tsparams.MasterNodeSelector})
				Expect(err).ToNot(HaveOccurred(), "Failed to get name of prometheus pod image")
				Expect(excludedPreCacheImages).ToNot(BeEmpty(), "Failed to get name of prometheus pod image on any nodes")

				for _, image := range excludedPreCacheImages {
					excludedPreCacheImage = strings.TrimSpace(image)
					imageListCommand = fmt.Sprintf(tsparams.SpokeImageListCommand, excludedPreCacheImage)
					imageDeleteCommand = fmt.Sprintf(tsparams.SpokeImageDeleteCommand, excludedPreCacheImage)

					break
				}

				if excludedPreCacheImage != "" {
					By("wiping any existing images from spoke 1 master")
					_ = cluster.ExecCmdWithRetries(Spoke1APIClient, ranparam.RetryCount, ranparam.RetryInterval,
						tsparams.MasterNodeSelector, imageDeleteCommand)
				}
			})

			AfterEach(func() {
				err := cgu.NewPreCachingConfigBuilder(
					HubAPIClient, tsparams.PreCachingConfigName, tsparams.TestNamespace).Delete()
				Expect(err).ToNot(HaveOccurred(), "Failed to delete PreCachingConfig on hub")

				errList := setup.CleanupTestResourcesOnHub(HubAPIClient, tsparams.TestNamespace, "")
				Expect(errList).To(BeEmpty(), "Failed to clean up test resources on hub")
			})

			// 48903 Upgrade image precaching - OCP image with explicit image url
			It("tests for ocp cache with image", reportxml.ID("48903"), func() {
				By("creating and applying policy with clusterversion that defines the upgrade graph, channel, and version")
				cguBuilder := getPrecacheCGU([]string{tsparams.PolicyName}, []string{RANConfig.Spoke1Name})

				clusterVersion, err := helper.GetClusterVersionDefinition(Spoke1APIClient, "Image")
				Expect(err).ToNot(HaveOccurred(), "Failed to get cluster version definition")

				_, err = helper.SetupCguWithClusterVersion(cguBuilder, clusterVersion)
				Expect(err).ToNot(HaveOccurred(), "Failed to setup cgu with cluster version")

				By("waiting until CGU Succeeded")
				assertPrecacheStatus(RANConfig.Spoke1Name, "Succeeded")

				By("waiting until new precache pod in spoke1 succeeded and log reports done")
				err = checkPrecachePodLog(Spoke1APIClient)
				Expect(err).ToNot(HaveOccurred(), "Failed to check the precache pod log")

				By("generating list of precached images on spoke 1")
				preCachedImages, err := cluster.ExecCmdWithStdoutWithRetries(
					Spoke1APIClient,
					ranparam.RetryCount, ranparam.RetryInterval,
					imageListCommand,
					metav1.ListOptions{LabelSelector: tsparams.MasterNodeSelector})
				Expect(err).ToNot(HaveOccurred(), "Failed to generate list of precached images on spoke 1")
				Expect(preCachedImages).ToNot(BeEmpty(), "Failed to find a master node for spoke 1")

				By("checking excluded image is present")
				for nodeName, nodePreCachedImages := range preCachedImages {
					Expect(nodePreCachedImages).ToNot(BeEmpty(), "Failed to check excluded image is present on node %s", nodeName)

					break
				}
			})

			// 59948 - Configurable filters for precache images.
			It("tests precache image filtering", reportxml.ID("59948"), func() {
				versionInRange, err := version.IsVersionStringInRange(RANConfig.HubOperatorVersions[ranparam.TALM], "4.13", "")
				Expect(err).ToNot(HaveOccurred(), "Failed to compare TALM version string")

				if !versionInRange {
					Skip("Skipping PreCache filtering if TALM is older than 4.13")
				}

				By("creating a configmap on hub to exclude images from precaching")
				_, err = configmap.NewBuilder(HubAPIClient, tsparams.PreCacheOverrideName, tsparams.TestNamespace).
					WithData(map[string]string{"excludePrecachePatterns": "prometheus"}).
					Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create a configmap on hub for excluding images")

				By("creating a cgu and setting it up with an image filter")
				cguBuilder := getPrecacheCGU([]string{tsparams.PolicyName}, []string{RANConfig.Spoke1Name})

				clusterVersion, err := helper.GetClusterVersionDefinition(Spoke1APIClient, "Image")
				Expect(err).ToNot(HaveOccurred(), "Failed to get cluster version definition")

				_, err = helper.SetupCguWithClusterVersion(cguBuilder, clusterVersion)
				Expect(err).ToNot(HaveOccurred(), "Failed to setup cgu with cluster version")

				By("waiting until CGU Succeeded")
				assertPrecacheStatus(RANConfig.Spoke1Name, "Succeeded")

				By("generating list of precached images on spoke 1")
				preCachedImages, err := cluster.ExecCmdWithStdoutWithRetries(
					Spoke1APIClient,
					ranparam.RetryCount, ranparam.RetryInterval,
					imageListCommand,
					metav1.ListOptions{LabelSelector: tsparams.MasterNodeSelector})
				Expect(err).ToNot(HaveOccurred(), "Failed to generate list of precached images on spoke 1")
				Expect(preCachedImages).ToNot(BeEmpty(), "Failed to find a master node for spoke 1")

				By("checking excluded image is not present")
				for nodeName, nodePreCachedImages := range preCachedImages {
					Expect(nodePreCachedImages).To(BeEmpty(), "Failed to check excluded image is not present on node %s", nodeName)

					break
				}
			})

			// 64746 - Precache User-Specified Image
			It("tests custom image precaching using a PreCachingConfig CR", reportxml.ID("64746"), func() {
				versionInRange, err := version.IsVersionStringInRange(RANConfig.HubOperatorVersions[ranparam.TALM], "4.14", "")
				Expect(err).ToNot(HaveOccurred(), "Failed to compare TALM version string")

				if !versionInRange {
					Skip("Skipping custom image pre caching if TALM is older than 4.14")
				}

				By("getting PTP image used by spoke 1")
				ptpDaemonPods, err := pod.List(
					Spoke1APIClient,
					RANConfig.PtpOperatorNamespace,
					metav1.ListOptions{LabelSelector: ranparam.PtpDaemonsetLabelSelector})
				Expect(err).ToNot(HaveOccurred(), "Failed to list PTP daemon pods on spoke 1")
				Expect(ptpDaemonPods).ToNot(BeEmpty(), "Failed to find any PTP daemon pods on spoke 1")

				var targetPrecacheImage string
				for _, container := range ptpDaemonPods[0].Object.Spec.Containers {
					if container.Name == ranparam.PtpContainerName {
						targetPrecacheImage = container.Image

						break
					}
				}
				Expect(targetPrecacheImage).ToNot(BeEmpty())

				By("deleting PTP image used by spoke 1")
				ptpImageDeleteCmd := fmt.Sprintf("podman rmi %s", targetPrecacheImage)
				_ = cluster.ExecCmdWithRetries(Spoke1APIClient, ranparam.RetryCount, ranparam.RetryInterval,
					tsparams.MasterNodeSelector, ptpImageDeleteCmd)

				By("creating a PreCachingConfig on hub")
				preCachingConfig := cgu.NewPreCachingConfigBuilder(
					HubAPIClient, tsparams.PreCachingConfigName, tsparams.TestNamespace)
				preCachingConfig.Definition.Spec.SpaceRequired = "10 GiB"
				preCachingConfig.Definition.Spec.ExcludePrecachePatterns = []string{""}
				preCachingConfig.Definition.Spec.AdditionalImages = []string{targetPrecacheImage}

				_, err = preCachingConfig.Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create PreCachingConfig on hub")

				By("defining a CGU with a PreCachingConfig specified")
				cguBuilder := getPrecacheCGU([]string{tsparams.PolicyName}, []string{RANConfig.Spoke1Name})
				cguBuilder.Definition.Spec.PreCachingConfigRef = v1alpha1.PreCachingConfigCR{
					Name:      tsparams.PreCachingConfigName,
					Namespace: tsparams.TestNamespace,
				}

				By("setting up a CGU with an image cluster version")
				clusterVersion, err := helper.GetClusterVersionDefinition(Spoke1APIClient, "Image")
				Expect(err).ToNot(HaveOccurred(), "Failed to get cluster version definition")

				_, err = helper.SetupCguWithClusterVersion(cguBuilder, clusterVersion)
				Expect(err).ToNot(HaveOccurred(), "Failed to setup cgu with cluster version")

				By("waiting until CGU Succeeded")
				assertPrecacheStatus(RANConfig.Spoke1Name, "Succeeded")

				spokeImageListCmd := fmt.Sprintf(`podman images  --noheading --filter reference=%s`, targetPrecacheImage)
				By("checking images list on spoke for targetImage")
				preCachedImages, err := cluster.ExecCmdWithStdoutWithRetries(
					Spoke1APIClient, ranparam.RetryCount, ranparam.RetryInterval,
					spokeImageListCmd, metav1.ListOptions{LabelSelector: tsparams.MasterNodeSelector})
				Expect(err).ToNot(HaveOccurred(), "Failed to generate list of precached images on spoke 1")
				Expect(preCachedImages).ToNot(BeEmpty(), "Failed to find a master node for spoke 1")

				By("checking target image is present")
				for nodeName, nodePreCachedImages := range preCachedImages {
					Expect(nodePreCachedImages).ToNot(BeEmpty(), "Failed to check excluded image is present on node %s", nodeName)

					break
				}
			})

			// 64747 Precache Invalid User-Specified Image
			It("tests custom image precaching using an invalid image", reportxml.ID("64747"), func() {
				versionInRange, err := version.IsVersionStringInRange(RANConfig.HubOperatorVersions[ranparam.TALM], "4.14", "")
				Expect(err).ToNot(HaveOccurred(), "Failed to compare TALM version string")

				if !versionInRange {
					Skip("Skipping custom image pre caching if TALM is older than 4.14")
				}

				By("creating a PreCachingConfig on hub")
				preCachingConfig := cgu.NewPreCachingConfigBuilder(
					HubAPIClient, tsparams.PreCachingConfigName, tsparams.TestNamespace)
				preCachingConfig.Definition.Spec.SpaceRequired = "10 GiB"
				preCachingConfig.Definition.Spec.ExcludePrecachePatterns = []string{""}
				preCachingConfig.Definition.Spec.AdditionalImages = []string{tsparams.PreCacheInvalidImage}

				_, err = preCachingConfig.Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create PreCachingConfig on hub")

				By("defining a CGU with a PreCachingConfig specified")
				cguBuilder := getPrecacheCGU([]string{tsparams.PolicyName}, []string{RANConfig.Spoke1Name})
				cguBuilder.Definition.Spec.PreCachingConfigRef = v1alpha1.PreCachingConfigCR{
					Name:      tsparams.PreCachingConfigName,
					Namespace: tsparams.TestNamespace,
				}

				By("setting up a CGU with an image cluster version")
				clusterVersion, err := helper.GetClusterVersionDefinition(Spoke1APIClient, "Image")
				Expect(err).ToNot(HaveOccurred(), "Failed to get cluster version definition")

				_, err = helper.SetupCguWithClusterVersion(cguBuilder, clusterVersion)
				Expect(err).ToNot(HaveOccurred(), "Failed to setup cgu with cluster version")

				By("waiting until CGU pre cache failed with UnrecoverableError")
				assertPrecacheStatus(RANConfig.Spoke1Name, "UnrecoverableError")
			})

			// 64751 - Precache with Large Disk
			It("tests precaching disk space checks using preCachingConfig", reportxml.ID("64751"), func() {
				versionInRange, err := version.IsVersionStringInRange(RANConfig.HubOperatorVersions[ranparam.TALM], "4.14", "")
				Expect(err).ToNot(HaveOccurred(), "Failed to compare TALM version string")

				if !versionInRange {
					Skip("Skipping custom image pre caching if TALM is older than 4.14")
				}

				By("creating a PreCachingConfig on hub with large spaceRequired")
				preCachingConfig := cgu.NewPreCachingConfigBuilder(
					HubAPIClient, tsparams.PreCachingConfigName, tsparams.TestNamespace)
				preCachingConfig.Definition.Spec.SpaceRequired = "9000 GiB"
				preCachingConfig.Definition.Spec.ExcludePrecachePatterns = []string{""}
				preCachingConfig.Definition.Spec.AdditionalImages = []string{""}

				_, err = preCachingConfig.Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create PreCachingConfig on hub")

				By("defining a CGU with a PreCachingConfig specified")
				cguBuilder := getPrecacheCGU([]string{tsparams.PolicyName}, []string{RANConfig.Spoke1Name})
				cguBuilder.Definition.Spec.PreCachingConfigRef = v1alpha1.PreCachingConfigCR{
					Name:      tsparams.PreCachingConfigName,
					Namespace: tsparams.TestNamespace,
				}

				By("setting up a CGU with an image cluster version")
				clusterVersion, err := helper.GetClusterVersionDefinition(Spoke1APIClient, "Image")
				Expect(err).ToNot(HaveOccurred(), "Failed to get cluster version definition")

				_, err = helper.SetupCguWithClusterVersion(cguBuilder, clusterVersion)
				Expect(err).ToNot(HaveOccurred(), "Failed to setup CGU with cluster version")

				By("waiting until CGU pre cache failed with UnrecoverableError")
				assertPrecacheStatus(RANConfig.Spoke1Name, "UnrecoverableError")
			})
		})
	})

	When("there are multiple spokes and one turns off", Ordered, ContinueOnFailure, func() {
		var (
			talmCompleteLabel = "talmcomplete"
		)

		BeforeAll(func() {
			clusters := []*clients.Settings{HubAPIClient, Spoke1APIClient, Spoke2APIClient}
			for index, cluster := range clusters {
				if cluster == nil {
					klog.V(tsparams.LogLevel).Infof("cluster #%d is nil", index)
					Skip("Precaching with multiple spokes requires all clients to not be nil")
				}
			}

			if BMCClient == nil {
				Skip("Tests where one spoke is powered off require the BMC configuration be set.")
			}

			By("powering off spoke 1")
			err := rancluster.PowerOffWithRetries(BMCClient, 3)
			Expect(err).ToNot(HaveOccurred(), "Failed to power off spoke 1")
		})

		AfterAll(func() {
			By("powering on spoke 1")
			err := rancluster.PowerOnWithRetries(BMCClient, 3)
			Expect(err).ToNot(HaveOccurred(), "Failed to power on spoke 1")

			By("waiting until all spoke 1 pods are ready")
			err = cluster.WaitForRecover(Spoke1APIClient, []string{}, 45*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for all spoke 1 pods to be ready")
		})

		Context("precaching with one managed cluster powered off and unavailable", func() {
			AfterEach(func() {
				By("cleaning up resources on hub")
				errorList := setup.CleanupTestResourcesOnHub(HubAPIClient, tsparams.TestNamespace, "")
				Expect(errorList).To(BeEmpty(), "Failed to clean up test resources on hub")
			})

			// 54286 - Unblock Batch OCP Upgrade
			It("verifies precaching fails for one spoke and succeeds for the other", reportxml.ID("54286"), func() {
				By("creating and setting up CGU")
				cguBuilder := getPrecacheCGU([]string{tsparams.PolicyName}, []string{RANConfig.Spoke1Name, RANConfig.Spoke2Name})

				clusterVersion, err := helper.GetClusterVersionDefinition(Spoke2APIClient, "Both")
				Expect(err).ToNot(HaveOccurred(), "Failed to get cluster version definition")

				cguBuilder, err = helper.SetupCguWithClusterVersion(cguBuilder, clusterVersion)
				Expect(err).ToNot(HaveOccurred(), "Failed to setup CGU with cluster version")

				By("waiting for pre cache to confirm it is valid")
				cguBuilder, err = cguBuilder.WaitForCondition(tsparams.CguPreCacheValidCondition, 5*time.Minute)
				Expect(err).ToNot(HaveOccurred(), "Failed to wait for pre cache to be valid")

				By("waiting until CGU Succeeded")
				assertPrecacheStatus(RANConfig.Spoke2Name, "Succeeded")

				By("enabling CGU")
				cguBuilder.Definition.Spec.Enable = ptr.To(true)
				cguBuilder, err = cguBuilder.Update(true)
				Expect(err).ToNot(HaveOccurred(), "Failed to enable CGU")

				By("waiting until CGU reports one spoke failed precaching")
				_, err = cguBuilder.WaitForCondition(tsparams.CguPreCachePartialCondition, 5*time.Minute)
				Expect(err).ToNot(HaveOccurred(), "Failed to wait for CGU to report one spoke failed precaching")

				By("checking CGU reports spoke 1 failed with UnrecoverableError")
				assertPrecacheStatus(RANConfig.Spoke1Name, "UnrecoverableError")
			})
		})

		Context("batching with one managed cluster powered off and unavailable", Ordered, func() {
			var cguBuilder *cgu.CguBuilder

			BeforeAll(func() {
				By("creating and setting up CGU with two spokes, one unavailable")
				cguBuilder = cgu.NewCguBuilder(HubAPIClient, tsparams.CguName, tsparams.TestNamespace, 1).
					WithCluster(RANConfig.Spoke1Name).
					WithCluster(RANConfig.Spoke2Name).
					WithManagedPolicy(tsparams.PolicyName)
				cguBuilder.Definition.Spec.RemediationStrategy.Timeout = 17

				var err error
				cguBuilder, err = helper.SetupCguWithNamespace(cguBuilder, "")
				Expect(err).ToNot(HaveOccurred(), "Failed to setup CGU with temporary namespace")

				By("updating CGU to add afterCompletion action")
				cguBuilder.Definition.Spec.Actions = v1alpha1.Actions{
					AfterCompletion: &v1alpha1.AfterCompletion{
						AddClusterLabels: map[string]string{talmCompleteLabel: ""},
					},
				}

				cguBuilder, err = cguBuilder.Update(true)
				Expect(err).ToNot(HaveOccurred(), "Failed to update CGU with afterCompletion action")
			})

			AfterAll(func() {
				By("cleaning up resources on spoke 2")
				errorList := setup.CleanupTestResourcesOnSpokes([]*clients.Settings{Spoke2APIClient}, "")
				Expect(errorList).To(BeEmpty(), "Failed to clean up resources on spoke 2")

				By("cleaning up resources on hub")
				errorList = setup.CleanupTestResourcesOnHub(HubAPIClient, tsparams.TestNamespace, "")
				Expect(errorList).To(BeEmpty(), "Failed to clean up test resources on hub")

				By("deleting label from managed cluster")
				err := helper.DeleteClusterLabel(HubAPIClient, RANConfig.Spoke2Name, talmCompleteLabel)
				Expect(err).ToNot(HaveOccurred(), "Failed to delete label from managed cluster")
			})

			// 54854 - CGU is Unblocked when an Unavailable Cluster is Encountered in a Target Cluster List
			It("verifies CGU fails on 'down' spoke in first batch and succeeds for 'up' spoke in second batch",
				reportxml.ID("54854"), func() {
					By("waiting for spoke 2 to complete successfully")
					cguBuilder, err := cguBuilder.WaitUntilClusterComplete(RANConfig.Spoke2Name, 22*time.Minute)
					Expect(err).ToNot(HaveOccurred(), "Failed to wait for spoke 2 batch remediation progress to complete")

					By("waiting for the CGU to timeout")
					_, err = cguBuilder.WaitForCondition(tsparams.CguTimeoutReasonCondition, 22*time.Minute)
					Expect(err).ToNot(HaveOccurred(), "Failed to wait for CGU to timeout")
				})

			// 59946 - Post completion action on a per cluster basis
			It("verifies CGU afterCompletion action executes on spoke2 when spoke1 is offline", reportxml.ID("59946"), func() {
				By("checking spoke 2 for post-action label present")
				labelPresent, err := helper.DoesClusterLabelExist(HubAPIClient, RANConfig.Spoke2Name, talmCompleteLabel)
				Expect(err).ToNot(HaveOccurred(), "Failed to check if spoke 2 has post-action label")
				Expect(labelPresent).To(BeTrue(), "Cluster post-action label was not present on spoke 2")

				By("checking spoke 1 for post-action label not present")
				labelPresent, err = helper.DoesClusterLabelExist(HubAPIClient, RANConfig.Spoke1Name, talmCompleteLabel)
				Expect(err).ToNot(HaveOccurred(), "Failed to check if cluster post-action label exists on spoke 1")
				Expect(labelPresent).To(BeFalse(), "Cluster post-action label was present on spoke 1")
			})
		})
	})
})

// getPrecacheCGU returns a CguBuilder given the policies and spokes.
func getPrecacheCGU(policyNames, spokes []string) *cgu.CguBuilder {
	cguBuilder := cgu.NewCguBuilder(HubAPIClient, tsparams.CguName, tsparams.TestNamespace, 2)
	cguBuilder.Definition.Spec.Enable = ptr.To(false)
	cguBuilder.Definition.Spec.PreCaching = true

	for _, policyName := range policyNames {
		cguBuilder = cguBuilder.WithManagedPolicy(policyName)
	}

	for _, spoke := range spokes {
		cguBuilder = cguBuilder.WithCluster(spoke)
	}

	return cguBuilder
}

// assertPrecacheStatus asserts status of backup struct.
func assertPrecacheStatus(spokeName, expected string) {
	Eventually(func() string {
		cguBuilder, err := cgu.Pull(HubAPIClient, tsparams.CguName, tsparams.TestNamespace)
		Expect(err).ToNot(HaveOccurred(),
			"Failed to pull CGU %s in namespace %s", tsparams.CguName, tsparams.TestNamespace)

		if cguBuilder.Object.Status.Precaching == nil {
			klog.V(tsparams.LogLevel).Info("precache struct not ready yet")

			return ""
		}

		_, ok := cguBuilder.Object.Status.Precaching.Status[spokeName]
		if !ok {
			klog.V(tsparams.LogLevel).Info("cluster name as key did not appear yet")

			return ""
		}

		klog.V(tsparams.LogLevel).Infof("[%s] %s precache status: %s\n",
			cguBuilder.Object.Name, spokeName, cguBuilder.Object.Status.Precaching.Status[spokeName])

		return cguBuilder.Object.Status.Precaching.Status[spokeName]
	}, 20*time.Minute, 15*time.Second).Should(Equal(expected))
}

// checkPrecachePodLog checks that the pre cache pod has a log that says the pre cache is done.
func checkPrecachePodLog(client *clients.Settings) error {
	var plog string

	err := wait.PollUntilContextTimeout(
		context.TODO(), 5*time.Second, 1*time.Minute, true, func(ctx context.Context) (bool, error) {
			podList, err := pod.List(client, tsparams.PreCacheSpokeNS, metav1.ListOptions{
				LabelSelector: tsparams.PreCachePodLabel,
			})
			if err != nil {
				return false, nil
			}

			if len(podList) == 0 {
				klog.V(tsparams.LogLevel).Info("precache pod does not exist on spoke - skip pod log check.")

				return true, nil
			}

			plog, err = podList[0].GetLog(1*time.Hour, tsparams.PreCacheContainerName)
			if err != nil {
				return false, nil
			}

			if strings.Contains(plog, "Image pre-cache done") {
				return true, nil
			}

			return false, nil
		})

	if err != nil && plog != "" {
		klog.V(tsparams.LogLevel).Infof("generated pod logs: %s", plog)
	}

	return err
}

// checkPoliciesExist returns the PolicyBuilder for all the provided policyNames, regardless of namespace, and whether
// all policyNames could be found on the hub. It takes the policyNames as valid regular expressions and uses a match to
// determine if a policy exists.
func checkPoliciesExist(client *clients.Settings, policyNames []string) ([]*ocm.PolicyBuilder, bool) {
	var policyRegexps []*regexp.Regexp

	for _, policyName := range policyNames {
		policyRegexp, err := regexp.Compile(policyName)
		Expect(err).ToNot(HaveOccurred(), "Failed to compile policy name regex %s", policyName)

		policyRegexps = append(policyRegexps, policyRegexp)
	}

	allPolicies, err := ocm.ListPoliciesInAllNamespaces(client)
	Expect(err).ToNot(HaveOccurred(), "Failed to list policies in all namespaces")

	var expectedPolicies []*ocm.PolicyBuilder

	for _, policyRegexp := range policyRegexps {
		for _, policy := range allPolicies {
			if policyRegexp.MatchString(policy.Object.Name) {
				expectedPolicies = append(expectedPolicies, policy)

				break
			}
		}
	}

	return expectedPolicies, len(expectedPolicies) == len(policyNames)
}

// copyPoliciesWithSubscription copies the policies that have a subscription and makes them NonCompliant. Policies
// without a subscription have their names returned first and then the second return is the suffixes for policies that
// were copied.
func copyPoliciesWithSubscription(policies []*ocm.PolicyBuilder) ([]string, []string) {
	var (
		originals []string
		suffixes  []string
	)

	for index, policy := range policies {
		klog.V(tsparams.LogLevel).Infof(
			"checking for subscriptions on policy %s in namespace %s", policy.Definition.Name, policy.Definition.Namespace)

		template := policy.Object.Spec.PolicyTemplates[0]
		configPolicy, err := ranhelper.UnmarshalRaw[configurationPolicyv1.ConfigurationPolicy](template.ObjectDefinition.Raw)
		Expect(err).ToNot(HaveOccurred(), "Failed to unmarshal config policy")

		hadSubscription := false

		for _, objectTemplate := range configPolicy.Spec.ObjectTemplates {
			untyped := &unstructured.Unstructured{}
			err := untyped.UnmarshalJSON(objectTemplate.ObjectDefinition.Raw)
			Expect(err).ToNot(HaveOccurred(), "Failed to unmarshal object template into unstructured")

			// first check that the object template is a subscription
			if untyped.GetObjectKind().GroupVersionKind().Kind != "Subscription" {
				continue
			}

			hadSubscription = true

			// if the current policy has a subscription then copy the policy and force it to be non-compliant
			suffix := fmt.Sprintf("-with-subscription-%d", index)
			suffixes = append(suffixes, suffix)

			klog.V(tsparams.LogLevel).Infof(
				"Copying policy %s and generating a new one with suffix %s", policy.Object.Name, suffix)

			copiedConfigPolicy := configPolicy.DeepCopy()

			for _, config := range copiedConfigPolicy.Spec.ObjectTemplates {
				typed, err := ranhelper.UnmarshalRaw[subscriptionsv1alpha1.Subscription](config.ObjectDefinition.Raw)
				Expect(err).ToNot(HaveOccurred(), "Failed to unmarshal subscription")

				config.ObjectDefinition.Raw = nil
				config.ObjectDefinition.Object = typed
			}

			// this will never get created so the name is just a placeholder
			tempNs := namespace.NewBuilder(HubAPIClient, "make-it-non-compliant").Definition

			copiedConfigPolicy.Name = policy.Definition.Name + suffix
			copiedConfigPolicy.Spec.ObjectTemplates = append(
				copiedConfigPolicy.Spec.ObjectTemplates, &configurationPolicyv1.ObjectTemplate{
					ObjectDefinition: runtime.RawExtension{Object: tempNs},
					ComplianceType:   configurationPolicyv1.MustHave,
				})
			policyTemplate := &policiesv1.PolicyTemplate{
				ObjectDefinition: runtime.RawExtension{Object: copiedConfigPolicy},
			}

			policyBuilder := ocm.NewPolicyBuilder(
				HubAPIClient, tsparams.PolicyName+suffix, tsparams.TestNamespace, policyTemplate).
				WithRemediationAction(policiesv1.Inform)
			policyBuilder, err = policyBuilder.Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create policy with suffix %s", suffix)

			err = helper.CreatePolicyComponents(
				HubAPIClient, suffix, []string{RANConfig.Spoke1Name}, metav1.LabelSelector{})
			Expect(err).ToNot(HaveOccurred(), "Failed to create policy components with suffix %s", suffix)

			err = policyBuilder.WaitUntilComplianceState(policiesv1.NonCompliant, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for policy with suffix %s to be non-compliant", suffix)

			break
		}

		if !hadSubscription {
			originals = append(originals, policy.Definition.Name)
		}
	}

	return originals, suffixes
}
