package tests

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/argocd"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/assisted"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ocm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/gitopsztp/internal/gitdetails"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/gitopsztp/internal/tsparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/version"
)

var _ = Describe("ZTP Argo CD Clusters Tests", Label(tsparams.LabelArgoCdClustersAppTestCases), func() {
	var (
		clustersApp             *argocd.ApplicationBuilder
		originalClustersGitPath string
	)

	BeforeEach(func() {
		By("verifying that ZTP meets the minimum version")
		versionInRange, err := version.IsVersionStringInRange(RANConfig.ZTPVersion, "4.11", "")
		Expect(err).ToNot(HaveOccurred(), "Failed to compare ZTP version string")

		if !versionInRange {
			Skip("ZTP Argo CD clusters app tests require ZTP 4.11 or later")
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
		clustersApp, err := clustersApp.Update(true)
		Expect(err).ToNot(HaveOccurred(), "Failed to update the clusters app with the original git path")

		By("waiting for the clusters app to sync")
		err = clustersApp.WaitForSourceUpdate(true, tsparams.ArgoCdChangeTimeout)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait for the clusters app to sync")
	})

	// 54238 - User modification of klustletaddonconfig via gitops
	It("should override the KlusterletAddonConfiguration and verify the change", reportxml.ID("54238"), func() {
		By("checking if the ztp test path exists")
		if !clustersApp.DoesGitPathExist(tsparams.ZtpTestPathClustersApp) {
			Skip(fmt.Sprintf("git path '%s' could not be found", tsparams.ZtpTestPathClustersApp))
		}

		By("updating the clusters app git path")
		err := gitdetails.UpdateAndWaitForSync(clustersApp, true, tsparams.ZtpTestPathClustersApp)
		Expect(err).ToNot(HaveOccurred(), "Failed to update the clusters app git path")

		By("validating the klusterlet addon change occurred")
		kac, err := ocm.PullKAC(HubAPIClient, RANConfig.Spoke1Name, RANConfig.Spoke1Name)
		Expect(err).ToNot(HaveOccurred(), "Failed to pull klusterlet addon config")

		_, err = kac.WaitUntilSearchCollectorEnabled(tsparams.ArgoCdChangeTimeout)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait for klusterlet addon config to have search collector enabled")
	})

	// 60619 - Image creation fails when NMstateConfig CR is empty
	It("should not have NMStateConfig CR when nodeNetwork section not in siteConfig", reportxml.ID("60619"), func() {
		By("checking if the ztp test path exists")
		if !clustersApp.DoesGitPathExist(tsparams.ZtpTestPathRemoveNmState) {
			Skip(fmt.Sprintf("git path '%s' could not be found", tsparams.ZtpTestPathRemoveNmState))
		}

		By("checking if the NM state config exists on hub")
		nmStateConfigList, err := assisted.ListNmStateConfigsInAllNamespaces(HubAPIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to list NM state configs")
		Expect(nmStateConfigList).ToNot(BeEmpty(), "Failed to find NM state config")

		By("updating the clusters app git path")
		err = gitdetails.UpdateAndWaitForSync(clustersApp, true, tsparams.ZtpTestPathRemoveNmState)
		Expect(err).ToNot(HaveOccurred(), "Failed to update the clusters app git path")

		By("validate the NM state config is gone on hub")
		nmStateConfigList, err = assisted.ListNmStateConfigsInAllNamespaces(HubAPIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to list NM state configs")
		Expect(nmStateConfigList).To(BeEmpty(), "Found NM state config when it should be gone")
	})
})
