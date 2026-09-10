package upgrade_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	ibguv1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/imagebasedgroupupgrades/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/internal/cnfcluster"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/internal/cnfclusterinfo"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/internal/cnfhelper"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/internal/cnfparams"
	cnfibuvalidations "github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/internal/validations"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/upgrade-talm/internal/tsparams"
	"k8s.io/klog/v2"
)

var _ = Describe(
	"Performing happy path image based upgrade",
	Ordered,
	ContinueOnFailure,
	Label(tsparams.LabelEndToEndUpgrade), func() {
		var clusterList []*clients.Settings

		BeforeAll(func() {
			clusterList = cnfhelper.GetAllTestClients()

			err := cnfcluster.CheckClustersPresent(clusterList)
			if err != nil {
				Skip(fmt.Sprintf("error occurred validating required clusters are present: %s", err.Error()))
			}

			By("Ensuring the spoke IBU is Idle and ready for Prep, or already upgraded")

			// 69058 shares this Ordered BeforeAll. A later job that runs only
			// 69058 finds Upgrade completed with Rollback valid, not Idle/Prep.
			upgraded, upgradedErr := cnfhelper.HasSuccessfulUpgrade()
			if upgradedErr == nil && upgraded {
				cnfibuvalidations.UpgradeSucceeded = true
			} else {
				Expect(cnfhelper.EnsureSpokeReadyForNextIbgu()).To(Succeed(),
					"spoke IBU was not Idle with Prep in validNextStages before upgrade")
			}

			By("Saving target sno cluster info before upgrade")

			err = cnfclusterinfo.PreUpgradeClusterInfo.SaveClusterInfo()
			Expect(err).ToNot(HaveOccurred(), "Failed to collect and save target sno cluster info before upgrade")

			tsparams.TargetSnoClusterName = cnfclusterinfo.PreUpgradeClusterInfo.Name
		})

		AfterEach(func() {
			deleteIbgusAndRecoverIfFailed()
		})

		AfterAll(func() {
			cnfibuvalidations.UpgradeSucceeded = false
		})

		// 68954 - Upgrade end to end
		It("Upgrade end to end", reportxml.ID("68954"), func() {
			By("Creating Prep+Upgrade IBGU")

			upgradeStart := time.Now()

			upgradeIbgu, err := cnfhelper.NewIbgu(tsparams.IbguName).
				WithPlan([]string{ibguv1alpha1.Prep, ibguv1alpha1.Upgrade}, 5, 30).
				Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create upgrade Ibgu.")

			By("Waiting until spoke IBU Prep completes")

			Expect(cnfhelper.WaitForSpokeIBU(cnfhelper.SpokePrepCompleted, 20*time.Minute)).To(Succeed(),
				"Prep did not complete on the spoke IBU")

			By("Waiting until spoke IBU Upgrade completes")

			Expect(cnfhelper.WaitForSpokeIBU(cnfhelper.SpokeUpgradeCompleted, 30*time.Minute)).To(Succeed(),
				"Upgrade did not complete on the spoke IBU")

			By("Waiting until Prep+Upgrade IBGU completes")

			Expect(cnfhelper.WaitForIbgu(upgradeIbgu, cnfhelper.IbguCompleted, 10*time.Minute)).To(Succeed(),
				"Prep and Upgrade IBGU did not complete")

			upgradeDuration := time.Since(upgradeStart)
			By(fmt.Sprintf("Upgrade (Prep+Upgrade) completed in %v", upgradeDuration))
			klog.V(cnfparams.CNFLogLevel).Infof("IBU upgrade duration (Prep+Upgrade): %v", upgradeDuration)

			By("Saving target sno cluster info after upgrade")

			err = cnfclusterinfo.PostUpgradeClusterInfo.SaveClusterInfo()
			Expect(err).ToNot(HaveOccurred(), "Failed to collect and save target sno cluster info after upgrade")

			By("Waiting until the spoke is healthy after upgrade")

			Expect(cnfhelper.WaitUntilHealthy(cnfhelper.DefaultHealthTimeout)).To(Succeed(),
				"spoke cluster was not healthy after upgrade")

			cnfibuvalidations.UpgradeSucceeded = true
		})

		cnfibuvalidations.PostUpgradeValidations()

		// 69058 - Rollback successful upgrade
		It(tsparams.SpecRollbackSuccessfulUpgrade, reportxml.ID("69058"), func() {
			if !cnfibuvalidations.UpgradeSucceeded {
				Skip("Rollback test 69058 requires a successful 68954 upgrade")
			}

			By("Creating Rollback+FinalizeRollback IBGU")

			rollbackStart := time.Now()

			rollbackIbgu, err := cnfhelper.NewIbgu(tsparams.IbguName).
				WithPlan([]string{ibguv1alpha1.Rollback, ibguv1alpha1.FinalizeRollback}, 5, 30).
				Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create rollback Ibgu.")

			By("Waiting until spoke IBU Rollback completes")

			Expect(cnfhelper.WaitForSpokeIBU(cnfhelper.SpokeRollbackCompleted, 30*time.Minute)).To(Succeed(),
				"Rollback did not complete on the spoke IBU")

			By("Waiting until spoke IBU returns to Idle")

			Expect(cnfhelper.WaitForSpokeIBU(cnfhelper.SpokeIdle, 10*time.Minute)).To(Succeed(),
				"spoke IBU did not return to Idle after FinalizeRollback")

			By("Waiting until Rollback IBGU completes")

			Expect(cnfhelper.WaitForIbgu(rollbackIbgu, cnfhelper.IbguCompleted, 10*time.Minute)).To(Succeed(),
				"Rollback IBGU did not complete")

			rollbackDuration := time.Since(rollbackStart)
			By(fmt.Sprintf("Rollback (Rollback+FinalizeRollback) completed in %v", rollbackDuration))
			klog.V(cnfparams.CNFLogLevel).Infof("IBU rollback duration (Rollback+FinalizeRollback): %v", rollbackDuration)

			By("Waiting until the spoke is healthy after rollback")

			Expect(cnfhelper.WaitUntilHealthy(cnfhelper.DefaultHealthTimeout)).To(Succeed(),
				"spoke cluster was not healthy after rollback")
		})
	})
