package tests

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/argocd"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/siteconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/gitopsztp/internal/gitdetails"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/gitopsztp/internal/tsparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/version"
)

var _ = Describe("ZTP Siteconfig Operator's Failover Tests",
	Label(tsparams.LabelSiteconfigFailoverTestCases), func() {
		var (
			clustersApp             *argocd.ApplicationBuilder
			originalClustersGitPath string
		)

		// These tests use the hub and spoke architecture.
		BeforeEach(func() {
			By("verifying that ZTP meets the minimum version")
			versionInRange, err := version.IsVersionStringInRange(RANConfig.ZTPVersion, "4.17", "")
			Expect(err).ToNot(HaveOccurred(), "Failed to compare ZTP version string")

			if !versionInRange {
				Skip("ZTP Siteconfig operator tests require ZTP 4.17 or later")
			}

			By("getting the clusters app")
			clustersApp, err = argocd.PullApplication(
				HubAPIClient, tsparams.ArgoCdClustersAppName, ranparam.OpenshiftGitOpsNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to get the clusters app")

			originalClustersGitPath, err = gitdetails.GetGitPath(clustersApp)
			Expect(err).ToNot(HaveOccurred(), "Failed to get the original clusters app git path")
		})

		AfterEach(func() {
			if CurrentSpecReport().State.Is(types.SpecStateSkipped) {
				return
			}

			By("resetting the clusters app back to the original settings")
			clustersApp.Definition.Spec.Source.Path = originalClustersGitPath
			clustersApp, err := clustersApp.Update(true)
			Expect(err).ToNot(HaveOccurred(), "Failed to reset clusters app git details")

			err = clustersApp.WaitForSourceUpdate(true, tsparams.ArgoCdChangeTimeout)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for the clusters app to sync")
		})

		// 75382 - Validate recovery mechanism by referencing non-existent cluster template configmap custom resource.
		It("Verify siteconfig operator's recovery mechanism by referencing non-existent cluster template configmap CR",
			reportxml.ID("75382"), func() {

				// Test step 1-Update the ztp-test git path to reference non-existent cluster template configmap.
				// in clusterinstance.yaml.
				By("checking if the non-existent cluster template configmap reference git path exists")
				if !clustersApp.DoesGitPathExist(tsparams.ZtpTestPathNoClusterTemplateCm) {
					Skip(fmt.Sprintf("git path '%s' could not be found", tsparams.ZtpTestPathNoClusterTemplateCm))
				}

				By("updating the Argo CD clusters app with the non-existent cluster template configmap reference git path")
				err := gitdetails.UpdateAndWaitForSync(clustersApp, true, tsparams.ZtpTestPathNoClusterTemplateCm)
				Expect(err).ToNot(HaveOccurred(), "Failed to update Argo CD clusters app with new git path")

				// Test step 1 expected result validation.
				By("checking cluster instance CR reporting validation failed with correct error message")
				clusterInstance, err := siteconfig.PullClusterInstance(HubAPIClient, RANConfig.Spoke1Name, RANConfig.Spoke1Name)
				Expect(err).ToNot(HaveOccurred(), "Failed to find cluster instance custom resource on hub")

				// Test step 1.b expected result validation.
				// i.e. to check the proper message: 'Validation failed: .
				// failed to validate cluster-level TemplateRef'.
				_, err = clusterInstance.WaitForCondition(tsparams.CINonExistentClusterTemplateConfigMapCondition,
					tsparams.SiteconfigOperatorDefaultReconcileTime)
				Expect(err).ToNot(HaveOccurred(), "Cluster instance failed to report an expected error message")
			})

		// 75383 - Validate recovery mechanism by referencing non-existent extra manifests configmap custom resource.
		It("Verify siteconfig operator's recovery mechanism by referencing non-existent extra manifests configmap CR",
			reportxml.ID("75383"), func() {

				// Test step 1-Update the ztp-test git path to reference non-existent extra manifests configmap.
				// in clusterinstance.yaml.
				By("checking if the non-existent extra manifests configmap reference git path exists")
				if !clustersApp.DoesGitPathExist(tsparams.ZtpTestPathNoExtraManifestsCm) {
					Skip(fmt.Sprintf("git path '%s' could not be found", tsparams.ZtpTestPathNoExtraManifestsCm))
				}

				By("updating the Argo CD clusters app with the non-existent extra manifests configmap reference git path")
				err := gitdetails.UpdateAndWaitForSync(clustersApp, true, tsparams.ZtpTestPathNoExtraManifestsCm)
				Expect(err).ToNot(HaveOccurred(), "Failed to update Argo CD clusters app with new git path")

				// Test step 1 expected result validation.
				By("checking cluster instance CR reporting validation failed with correct error message")
				clusterInstance, err := siteconfig.PullClusterInstance(HubAPIClient, RANConfig.Spoke1Name, RANConfig.Spoke1Name)
				Expect(err).ToNot(HaveOccurred(), "Failed to find cluster instance custom resource on hub")

				// Test step 1.b expected result validation.
				// i.e. to check the proper message: 'Validation failed: failed to retrieve ExtraManifest'.
				_, err = clusterInstance.WaitForCondition(tsparams.CINonExistentExtraManifestConfigMapCondition,
					tsparams.SiteconfigOperatorDefaultReconcileTime)
				Expect(err).ToNot(HaveOccurred(), "Cluster instance failed to report an expected error message")
			})
	})
