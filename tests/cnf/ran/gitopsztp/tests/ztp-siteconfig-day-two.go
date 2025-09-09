package tests

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/argocd"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/gitopsztp/internal/gitdetails"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/gitopsztp/internal/helper"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/gitopsztp/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/rancluster"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/version"
)

var _ = Describe("ZTP Siteconfig Operator's Day 2 configuration Test",
	Label(tsparams.LabelSiteconfigDayTwoConfigTestCase), func() {
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

			// Remove newly added custom label from the ClusterInstance CR underneath “extraLabels” field
			// after the spoke cluster deployed using git flow.
			By("resetting the clusters app back to the original settings")
			clustersApp.Definition.Spec.Source.Path = originalClustersGitPath
			clustersApp, err := clustersApp.Update(true)
			Expect(err).ToNot(HaveOccurred(), "Failed to update the clusters app git path")

			By("waiting for the clusters app to sync")
			err = clustersApp.WaitForSourceUpdate(true, tsparams.ArgoCdChangeTimeout)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for the clusters app to sync")

			// Make sure the newly added cluster label removed from ClusterInstance CR on hub cluster.
			// $ oc get clusterinstance <spoke cluster name> -n <spoke namespace>
			// -o jsonpath='{.spec.extraLabels.ManagedCluster}' | jq
			By("checking newly added cluster label removed from cluster instance CR")
			extraLabelPresent, err := helper.DoesCIExtraLabelsExists(HubAPIClient, RANConfig.Spoke1Name, RANConfig.Spoke1Name,
				tsparams.CIExtraLabelsKey, tsparams.TestLabelKey)
			Expect(err).ToNot(HaveOccurred(), "Failed to check newly added cluster label "+
				"removed from cluster instance CR")
			Expect(extraLabelPresent).To(BeFalse(), "Day-2 cluster label was present "+
				"on cluster instance CR")

			// New siteconfig operator should honor day2 cluster label remove event and newly added cluster label.
			// removed from ManagedCluster CR on hub cluster  using the command.
			// $ oc get managedcluster <spoke cluster name> -o jsonpath='{.metadata.labels}' | jq.
			By("checking newly added spoke cluster label removed from managed cluster CR")
			labelPresent, err := rancluster.DoesClusterLabelExist(HubAPIClient, RANConfig.Spoke1Name,
				tsparams.TestLabelKey)
			Expect(err).ToNot(HaveOccurred(), "Failed to check newly added spoke cluster label "+
				"removed from managed cluster CR")
			Expect(labelPresent).To(BeFalse(), "Day-2 cluster label was present on spoke")
		})

		// 75342 - Verify modification of cluster labels in ClusterInstance CR using git flows after installation.
		It("Verify modification of cluster labels in ClusterInstance CR using git flows after installation",
			reportxml.ID("75342"), func() {
				By("checking if ztp test path exists")
				if !clustersApp.DoesGitPathExist(tsparams.ZtpTestPathNewClusterLabel) {
					Skip(fmt.Sprintf("git path '%s' could not be found", tsparams.ZtpTestPathNewClusterLabel))
				}

				// Add a new custom label to the ClusterInstance CR underneath “extraLabels” field.
				// after the spoke cluster deployed using git flow.
				// Test step 1-Update the ztp-test git path to reference a new custom label addition.
				// in clusterinstance.yaml as day-2 configuration.
				By("updating the Argo CD clusters app with the new custom label reference git path")
				err := gitdetails.UpdateAndWaitForSync(clustersApp, true, tsparams.ZtpTestPathNewClusterLabel)
				Expect(err).ToNot(HaveOccurred(), "Failed to update Argo CD clusters app with new git path")

				// Make sure the ClusterInstance CR on hub cluster updated with newly added cluster label.
				// $ oc get clusterinstance <spoke cluster name> -n <spoke namespace>
				// -o jsonpath='{.spec.extraLabels.ManagedCluster}' | jq
				// Test step 1.a expected result validation.
				By("checking cluster instance CR updated with newly added cluster label on hub")
				extraLabelPresent, err := helper.DoesCIExtraLabelsExists(HubAPIClient, RANConfig.Spoke1Name, RANConfig.Spoke1Name,
					tsparams.CIExtraLabelsKey, tsparams.TestLabelKey)
				Expect(err).ToNot(HaveOccurred(), "Failed to check if cluster instance "+
					"has newly added cluster label")
				Expect(extraLabelPresent).To(BeTrue(), "Day-2 cluster label was not present "+
					"on cluster instance CR")

				// New siteconfig operator should honor day2 cluster label add event and cluster label added
				// to ManagedCluster CR on hub cluster using below command,
				// $ oc get managedcluster <spoke cluster name> -o jsonpath='{.metadata.labels}' | jq
				// Test step 1.b expected result validation.
				By("checking managed cluster CR updated with newly added spoke cluster label on hub")
				labelPresent, err := rancluster.DoesClusterLabelExist(HubAPIClient, RANConfig.Spoke1Name,
					tsparams.TestLabelKey)
				Expect(err).ToNot(HaveOccurred(), "Failed to check if spoke has newly added cluster label")
				Expect(labelPresent).To(BeTrue(), "Day-2 cluster label was not present on spoke")
			})
	})
