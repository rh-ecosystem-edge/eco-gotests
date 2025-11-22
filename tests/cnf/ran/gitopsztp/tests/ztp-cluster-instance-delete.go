package tests

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	. "github.com/onsi/gomega"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/argocd"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/assisted"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/bmh"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/hive"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ocm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/siteconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/gitopsztp/internal/gitdetails"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/gitopsztp/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/rancluster"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranhelper"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/version"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
)

var _ = Describe("ZTP Siteconfig Operator's Cluster Instance Delete Tests",
	Label(tsparams.LabelClusterInstanceDeleteTestCases), func() {
		var (
			clustersApp             *argocd.ApplicationBuilder
			originalClustersGitPath string
		)

		BeforeEach(func() {
			By("verifying that ZTP meets the minimum version")
			versionInRange, err := version.IsVersionStringInRange(RANConfig.ZTPVersion, "4.17", "")
			Expect(err).ToNot(HaveOccurred(), "Failed to compare ZTP version string")

			if !versionInRange {
				Skip("ZTP Siteconfig operator tests require ZTP 4.17 or later")
			}

			By("saving the original clusters app source")
			clustersApp, err = argocd.PullApplication(
				HubAPIClient, tsparams.ArgoCdClustersAppName, ranparam.OpenshiftGitOpsNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to get the original clusters app")

			originalClustersGitPath, err = gitdetails.GetGitPath(clustersApp)
			Expect(err).ToNot(HaveOccurred(), "Failed to get the original clusters app git path")
		})

		AfterEach(func() {
			if CurrentSpecReport().State.Is(types.SpecStateSkipped) {
				return
			}

			By("resetting the clusters app back to the original settings")
			clustersApp.Definition.Spec.Source.Path = originalClustersGitPath
			updatedApp, err := clustersApp.Update(true)
			Expect(err).ToNot(HaveOccurred(), "Failed to update clusters app back to the original settings")

			By("waiting for the clusters app to sync")
			err = updatedApp.WaitForSourceUpdate(true, tsparams.ArgoCdChangeTimeout)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for clusters app to sync")

			By("checking the infra env manifests exists on hub")
			_, err = assisted.PullInfraEnvInstall(HubAPIClient, RANConfig.Spoke1Name, RANConfig.Spoke1Name)
			Expect(err).ToNot(HaveOccurred(), "Failed to find spoke infra env manifests")

			By("checking the bare metal host manifests exists on hub")
			_, err = bmh.Pull(HubAPIClient, RANConfig.Spoke1Name, RANConfig.Spoke1Name)
			Expect(err).ToNot(HaveOccurred(), "Failed to find spoke bmh manifests")

			By("checking the cluster deployment manifests exists on hub")
			_, err = hive.PullClusterDeployment(HubAPIClient, RANConfig.Spoke1Name, RANConfig.Spoke1Name)
			Expect(err).ToNot(HaveOccurred(), "Failed to find spoke cluster deployment manifests")

			By("checking the NM state config manifests exists on hub")
			nmStateConfigList, err := assisted.ListNmStateConfigs(HubAPIClient, RANConfig.Spoke1Name)
			Expect(err).ToNot(HaveOccurred(), "Failed to list NM state config manifests")
			Expect(nmStateConfigList).ToNot(BeEmpty(), "Failed to find NM state config manifests")

			By("checking the klusterlet addon config manifests exists on hub")
			_, err = ocm.PullKAC(HubAPIClient, RANConfig.Spoke1Name, RANConfig.Spoke1Name)
			Expect(err).ToNot(HaveOccurred(), "Failed to find spoke kac manifests")

			By("checking the agent cluster install manifests exists on hub")
			_, err = assisted.PullAgentClusterInstall(HubAPIClient, RANConfig.Spoke1Name, RANConfig.Spoke1Name)
			Expect(err).ToNot(HaveOccurred(), "Failed to find spoke agent cluster install manifests")
		})

		// 75374 - Detaching the AI multi-node openshift (MNO) spoke cluster.
		It("Validate detaching the AI multi-node openshift spoke cluster", reportxml.ID("75374"), func() {
			By("checking spoke cluster type")
			spokeClusterType, err := rancluster.CheckSpokeClusterType(RANConfig.Spoke1APIClient)
			Expect(err).ToNot(HaveOccurred(), "Failed to fetch spoke cluster type")

			if spokeClusterType == ranparam.SNOCluster {
				Skip("This test only applies to standard or multi-node openshift spoke cluster")
			}

			// The clusters app is updated later on, but skip early if the git path does not exist to avoid
			// extra cleanup.
			By("checking if the ztp test path exists")
			if !clustersApp.DoesGitPathExist(tsparams.ZtpTestPathDetachAIMNO) {
				Skip(fmt.Sprintf("git path '%s' could not be found", tsparams.ZtpTestPathDetachAIMNO))
			}

			By("deleting default assisted installer template reference ConfigMap custom resources")

			By("deleting default assisted installer cluster level templates ConfigMap CR")
			clusterTemplateConfigMap, err := configmap.Pull(HubAPIClient, tsparams.DefaultAIClusterTemplatesConfigMapName,
				ranparam.AcmOperatorNamespace)
			if err == nil {
				err = clusterTemplateConfigMap.Delete()
				Expect(err).ToNot(HaveOccurred(), "Failed to delete AI cluster level templates config map")
			}

			By("deleting default assisted installer node level templates ConfigMap CR")
			nodeTemplateConfigMap, err := configmap.Pull(HubAPIClient, tsparams.DefaultAINodeTemplatesConfigMapName,
				ranparam.AcmOperatorNamespace)
			if err == nil {
				err = nodeTemplateConfigMap.Delete()
				Expect(err).ToNot(HaveOccurred(), "Failed to delete AI node level templates config map")
			}

			By("verifying installed spoke cluster should still be functional")
			_, err = version.GetOCPVersion(Spoke1APIClient)
			Expect(err).ToNot(HaveOccurred(), "Failed to get OCP version from spoke and verify spoke cluster access")

			By("updating the clusters app git path")
			err = gitdetails.UpdateAndWaitForSync(clustersApp, true, tsparams.ZtpTestPathDetachAIMNO)
			Expect(err).ToNot(HaveOccurred(), "Failed to update the clusters app git path")

			validateAISpokeClusterInstallCRsRemoved()

			By("deleting siteconfig operator pod running under rhacm namespace on hub cluster")

			By("Get the siteconfig operator pod name with label " + tsparams.SiteconfigOperatorPodLabel)
			desiredPodName, err := ranhelper.GetPodNameWithLabel(HubAPIClient, ranparam.AcmOperatorNamespace,
				tsparams.SiteconfigOperatorPodLabel)
			Expect(err).ToNot(HaveOccurred(), "Failed to get siteconfig operator pod name with label "+
				tsparams.SiteconfigOperatorPodLabel+" from "+ranparam.AcmOperatorNamespace+" namespace")

			By("deleting the siteconfig operator pod name from namespace " + ranparam.AcmOperatorNamespace)
			siteconfigOperatorPodName, err := pod.Pull(HubAPIClient, desiredPodName, ranparam.AcmOperatorNamespace)
			if err == nil {
				_, err = siteconfigOperatorPodName.DeleteAndWait(3 * time.Minute)
				Expect(err).ToNot(HaveOccurred(), "Failed to delete siteconfig operator pod")
			}

			// Teardown test expected results validation.
			// The default assisted installer template reference ConfigMap custom resource should be recreated successfully.
			By("checking the default assisted installer template reference ConfigMap CRs recreated successfully")
			// Wait for 10 seconds to allow siteconfig operator to reconcile state after restarting controller pod.
			// before checking cluster-level and node-level templates configmap CR recreated on rhacm namespace.
			time.Sleep(10 * time.Second)

			By("checking default assisted installer cluster level templates ConfigMap CR exists")
			_, err = configmap.Pull(HubAPIClient, tsparams.DefaultAIClusterTemplatesConfigMapName,
				ranparam.AcmOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to find default AI cluster level templates config map")

			By("checking default assisted installer node level templates ConfigMap CR exists")
			_, err = configmap.Pull(HubAPIClient, tsparams.DefaultAINodeTemplatesConfigMapName,
				ranparam.AcmOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to find default AI node level templates config map")

			By("verifying installed spoke cluster should still be functional")
			_, err = version.GetOCPVersion(Spoke1APIClient)
			Expect(err).ToNot(HaveOccurred(), "Failed to get OCP version from spoke and verify spoke cluster access")

			By("verifying spoke cluster namespace CR exists on hub after siteconfig operator's pod restart")
			_, err = namespace.Pull(HubAPIClient, RANConfig.Spoke1Name)
			Expect(err).ToNot(HaveOccurred(), "Failed to find spoke cluster namespace CR")

			By("verifying cluster instance CR exists on hub after siteconfig operator's pod restart")
			_, err = siteconfig.PullClusterInstance(HubAPIClient, RANConfig.Spoke1Name, RANConfig.Spoke1Name)
			Expect(err).ToNot(HaveOccurred(), "Failed to find cluster instance custom resource")
		})

		// 75376 - Detaching the AI single-node openshift (SNO) spoke cluster.
		It("Validate detaching the AI single-node openshift spoke cluster", reportxml.ID("75376"), func() {
			By("checking spoke cluster type")
			spokeClusterType, err := rancluster.CheckSpokeClusterType(RANConfig.Spoke1APIClient)
			Expect(err).ToNot(HaveOccurred(), "Failed to fetch spoke cluster type")

			if spokeClusterType == ranparam.HighlyAvailableCluster {
				Skip("This test only applies to single-node openshift spoke cluster")
			}

			By("checking if the ztp test path exists")
			if !clustersApp.DoesGitPathExist(tsparams.ZtpTestPathDetachAISNO) {
				Skip(fmt.Sprintf("git path '%s' could not be found", tsparams.ZtpTestPathDetachAISNO))
			}

			By("updating the clusters app git path")
			err = gitdetails.UpdateAndWaitForSync(clustersApp, true, tsparams.ZtpTestPathDetachAISNO)
			Expect(err).ToNot(HaveOccurred(), "Failed to update the clusters app git path")

			validateAISpokeClusterInstallCRsRemoved()
		})
	})

// validateAISpokeClusterInstallCRsRemoved verifies AI spoke cluster install CRs removed and spoke cluster accessible.
//
//nolint:wsl
func validateAISpokeClusterInstallCRsRemoved() {
	By("checking the infra env manifests removed on hub")

	_, err := assisted.PullInfraEnvInstall(HubAPIClient, RANConfig.Spoke1Name, RANConfig.Spoke1Name)
	Expect(err).To(HaveOccurred(), "Found spoke infra env manifests but expected to be removed")

	By("checking the bare metal host manifests removed on hub")

	_, err = bmh.Pull(HubAPIClient, RANConfig.Spoke1Name, RANConfig.Spoke1Name)
	Expect(err).To(HaveOccurred(), "Found spoke bmh manifests but expected to be removed")

	By("checking the cluster deployment manifests removed on hub")

	_, err = hive.PullClusterDeployment(HubAPIClient, RANConfig.Spoke1Name, RANConfig.Spoke1Name)
	Expect(err).To(HaveOccurred(), "Found spoke cluster deployment manifests but expected to be removed")

	By("checking the NM state config manifests removed on hub")

	nmStateConfigList, err := assisted.ListNmStateConfigs(HubAPIClient, RANConfig.Spoke1Name)
	Expect(err).ToNot(HaveOccurred(), "Failed to list NM state config manifests")
	Expect(nmStateConfigList).To(BeEmpty(), "Found spoke NM state config manifests but expected to be removed")

	By("checking the klusterlet addon config manifests removed on hub")

	_, err = ocm.PullKAC(HubAPIClient, RANConfig.Spoke1Name, RANConfig.Spoke1Name)
	Expect(err).To(HaveOccurred(), "Found spoke kac manifests but expected to be removed")

	By("checking the agent cluster install manifests removed on hub")

	_, err = assisted.PullAgentClusterInstall(HubAPIClient, RANConfig.Spoke1Name, RANConfig.Spoke1Name)
	Expect(err).To(HaveOccurred(), "Found spoke ACI manifests but expected to be removed")

	By("verifying installed spoke cluster should still be functional")

	_, err = version.GetOCPVersion(Spoke1APIClient)
	Expect(err).ToNot(HaveOccurred(), "Failed to get OCP version from spoke and verify spoke cluster access")
}
