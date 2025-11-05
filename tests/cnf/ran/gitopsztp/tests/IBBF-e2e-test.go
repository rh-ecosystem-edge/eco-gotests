package tests

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/argocd"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/hive"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ocm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	siteconfigv1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/siteconfig/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/siteconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/gitopsztp/internal/gitdetails"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/gitopsztp/internal/tsparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"fmt"
	"time"
)

var _ = Describe(
	"Performing Image-Based Break/Fix Flow",
	Label(tsparams.LabelIBBFe2e), func() {

		var (
			spokeNamespace = RANConfig.Spoke1Name
		)

		AfterEach(func() {

			By("Cleanup test configmap")

			err := configmap.NewBuilder(HubAPIClient, tsparams.TestCMName, spokeNamespace).
				Delete()
			Expect(err).ToNot(HaveOccurred(), "Unable to delete configmap")
		})

		It("tests HW replacement via IBBF", reportxml.ID("78333"), func() {
			By("getting the clusters app")
			clustersApp, err := argocd.PullApplication(
				HubAPIClient, tsparams.ArgoCdClustersAppName, ranparam.OpenshiftGitOpsNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to get the clusters app")

			By("checking if the git path exists")
			if !clustersApp.DoesGitPathExist(tsparams.ZtpTestPathIBBFe2e) {
				Skip(fmt.Sprintf("git path '%s' could not be found", tsparams.ZtpTestPathIBBFe2e))
			}

			By("Enabling cluster reinstallation in SiteconfigOperator")

			scoConfig, err := configmap.Pull(HubAPIClient, "siteconfig-operator-configuration",
				ranparam.AcmOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "error pulling siteconfig-operator-configuration configmap")

			scoConfig.Definition.Data["allowReinstalls"] = "true"
			_, err = scoConfig.Update()
			Expect(err).ToNot(HaveOccurred(), "error updating siteconfig-operator-configuration configmap to allow reinstalls")

			By("Creating test configmap to be preserved")
			configMap := configmap.NewBuilder(
				HubAPIClient, tsparams.TestCMName, spokeNamespace).
				WithData(map[string]string{"testValue": "true"})

			// Apply preservation label to configmap
			configMap.Definition.ObjectMeta.SetLabels(map[string]string{"siteconfig.open-cluster-management.io/preserve": ""})

			// Create configmap
			_, err = configMap.Create()
			Expect(err).ToNot(HaveOccurred(), "error creating configmap")

			By("Getting cluster identity pre-IBBF")
			clusterDeployment, err := hive.PullClusterDeployment(HubAPIClient,
				RANConfig.Spoke1Name,
				spokeNamespace)
			Expect(err).ToNot(HaveOccurred(), "error pulling clusterdeployment")

			originalClusterID := clusterDeployment.Object.Spec.ClusterMetadata.ClusterID
			originalInfraID := clusterDeployment.Object.Spec.ClusterMetadata.InfraID

			// Get clusterinstance.
			clusterInstance, err := siteconfig.PullClusterInstance(
				HubAPIClient, RANConfig.Spoke1Name, spokeNamespace)
			Expect(err).ToNot(HaveOccurred(), "error pulling clusterinstance")

			By("Changing clusters app to point to IBBF test target directory")

			err = gitdetails.UpdateAndWaitForSync(clustersApp, true, tsparams.ZtpTestPathIBBFe2e)
			Expect(err).ToNot(HaveOccurred(), "Failed to update ArgoCD with new git details")

			By("Waiting for clusterinstance re-installation to trigger")

			_, err = clusterInstance.WaitForReinstallCondition(metav1.Condition{
				Type:   string(siteconfigv1alpha1.ReinstallRequestProcessed),
				Reason: string(siteconfigv1alpha1.Completed)}, 10*time.Minute)

			Expect(err).ToNot(HaveOccurred(), "error waiting for clusterinstance to begin re-install")

			By("Waiting for clusterinstance to start provisioning")

			_, err = clusterInstance.WaitForCondition(metav1.Condition{
				Type:   string(siteconfigv1alpha1.ClusterProvisioned),
				Reason: string(siteconfigv1alpha1.InProgress)}, 5*time.Minute)

			Expect(err).ToNot(HaveOccurred(), "error waiting for clusterinstance to begin provisioning")

			By("Waiting for clusterinstance to finish provisioning")

			_, err = clusterInstance.WaitForCondition(metav1.Condition{
				Type:   string(siteconfigv1alpha1.ClusterProvisioned),
				Reason: string(siteconfigv1alpha1.Completed)}, 60*time.Minute)

			Expect(err).ToNot(HaveOccurred(), "error waiting for clusterinstance to complete re-install")

			By("Verifying test configmap was preserved post-IBBF")

			_, err = configmap.Pull(HubAPIClient,
				tsparams.TestCMName, spokeNamespace)
			Expect(err).ToNot(HaveOccurred(), "Preserved configmap is missing after IBBF")

			By("Comparing cluster identity post-IBBF")
			clusterDeploymentPostIBBF, err := hive.PullClusterDeployment(HubAPIClient,
				RANConfig.Spoke1Name,
				spokeNamespace)
			Expect(err).ToNot(HaveOccurred(), "error pulling clusterdeployment")

			Expect(originalClusterID).To(Equal(clusterDeploymentPostIBBF.Object.Spec.ClusterMetadata.ClusterID),
				"error: reinstalled cluster has different ClusterID than original cluster")

			Expect(originalInfraID).To(Equal(clusterDeploymentPostIBBF.Object.Spec.ClusterMetadata.InfraID),
				"error: reinstalled cluster has different InfraID than original cluster")

			By("Verifying managedcluster is available")

			managedCluster, err := ocm.PullManagedCluster(HubAPIClient, RANConfig.Spoke1Name)
			Expect(err).ToNot(HaveOccurred(), "error pulling managedcluster")

			_, err = managedCluster.WaitForCondition(metav1.Condition{
				Type:   "ManagedClusterConditionAvailable",
				Status: metav1.ConditionTrue,
			}, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "error waiting for managedcluster to become available")

		})
	})
