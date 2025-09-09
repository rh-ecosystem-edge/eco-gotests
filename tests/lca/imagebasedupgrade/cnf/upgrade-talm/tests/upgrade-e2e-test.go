package upgrade_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ibgu"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/lca"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/internal/cnfcluster"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/internal/cnfclusterinfo"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/internal/cnfhelper"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/internal/cnfinittools"
	cnfibuvalidations "github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/internal/validations"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/upgrade-talm/internal/tsparams"
)

var _ = Describe(
	"Performing happy path image based upgrade",
	Ordered,
	ContinueOnFailure,
	Label(tsparams.LabelEndToEndUpgrade), func() {

		var (
			clusterList       []*clients.Settings
			upgradeSuccessful = true
		)

		BeforeAll(func() {
			// Initialize cluster list.
			clusterList = cnfhelper.GetAllTestClients()

			// Check that the required clusters are present.
			err := cnfcluster.CheckClustersPresent(clusterList)
			if err != nil {
				Skip(fmt.Sprintf("error occurred validating required clusters are present: %s", err.Error()))
			}

			By("Saving target sno cluster info before upgrade", func() {
				err := cnfclusterinfo.PreUpgradeClusterInfo.SaveClusterInfo()
				Expect(err).ToNot(HaveOccurred(), "Failed to collect and save target sno cluster info before upgrade")

				tsparams.TargetSnoClusterName = cnfclusterinfo.PreUpgradeClusterInfo.Name

				ibu, err = lca.PullImageBasedUpgrade(cnfinittools.TargetSNOAPIClient)
				Expect(err).NotTo(HaveOccurred(), "error pulling ibu resource from cluster")
			})
		})

		AfterEach(func() {
			By("Deleting IBGUs on target hub cluster", func() {
				_, err := ibgu.NewIbguBuilder(cnfinittools.TargetHubAPIClient,
					tsparams.IbguName, tsparams.IbguNamespace).DeleteAndWait(1 * time.Minute)

				Expect(err).ToNot(HaveOccurred(), "Failed to delete ibgu on target hub cluster")

			})

			// Sleep for 10 seconds to allow talm to reconcile state.
			// Sometimes if the next test re-creates the IBGUs too quickly,
			// the policies compliance status is not updated correctly.
			time.Sleep(10 * time.Second)
		})

		It("Upgrade end to end", reportxml.ID("68954"), func() {
			upgradeSuccessful = false

			By("Create Upgrade IBGU", func() {
				newIbguBuilder := ibgu.NewIbguBuilder(cnfinittools.TargetHubAPIClient,
					tsparams.IbguName, tsparams.IbguNamespace).
					WithClusterLabelSelectors(tsparams.ClusterLabelSelector).
					WithSeedImageRef(cnfinittools.CNFConfig.IbguSeedImage, cnfinittools.CNFConfig.IbguSeedImageVersion).
					WithOadpContent(cnfinittools.CNFConfig.IbguOadpCmName, cnfinittools.CNFConfig.IbguOadpCmNamespace).
					WithPlan([]string{"Prep", "Upgrade"}, 5, 30)

				newIbguBuilder, err := newIbguBuilder.Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create upgrade Ibgu.")

				_, err = newIbguBuilder.WaitUntilComplete(30 * time.Minute)
				Expect(err).NotTo(HaveOccurred(), "Prep and Upgrade IBGU did not complete in time.")

			})

			By("Saving target sno cluster info after upgrade", func() {
				err := cnfclusterinfo.PostUpgradeClusterInfo.SaveClusterInfo()
				Expect(err).ToNot(HaveOccurred(), "Failed to collect and save target sno cluster info after upgrade")
			})

			upgradeSuccessful = true

		})

		if upgradeSuccessful {
			cnfibuvalidations.PostUpgradeValidations()
		}

		It("Rollback successful upgrade", reportxml.ID("69058"), func() {
			if !upgradeSuccessful {
				Skip("Skipping rollback test due to upgrade failure.")
			}

			By("Creating an IBGU to rollback upgrade", func() {
				rollbackIbguBuilder := ibgu.NewIbguBuilder(cnfinittools.TargetHubAPIClient,
					tsparams.IbguName, tsparams.IbguNamespace)
				rollbackIbguBuilder = rollbackIbguBuilder.WithClusterLabelSelectors(tsparams.ClusterLabelSelector)
				rollbackIbguBuilder = rollbackIbguBuilder.WithSeedImageRef(
					cnfinittools.CNFConfig.IbguSeedImage,
					cnfinittools.CNFConfig.IbguSeedImageVersion)
				rollbackIbguBuilder = rollbackIbguBuilder.WithOadpContent(
					cnfinittools.CNFConfig.IbguOadpCmName,
					cnfinittools.CNFConfig.IbguOadpCmNamespace)
				rollbackIbguBuilder = rollbackIbguBuilder.WithPlan([]string{"Rollback", "FinalizeRollback"}, 5, 30)

				rollbackIbguBuilder, err = rollbackIbguBuilder.Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create rollback Ibgu.")

				_, err = rollbackIbguBuilder.WaitUntilComplete(30 * time.Minute)
				Expect(err).NotTo(HaveOccurred(), "Rollback IBGU did not complete in time.")

			})

		})

	})
