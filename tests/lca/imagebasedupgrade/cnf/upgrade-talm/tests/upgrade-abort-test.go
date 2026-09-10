package upgrade_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	ibguv1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/imagebasedgroupupgrades/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/internal/cnfclusterinfo"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/internal/cnfhelper"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/upgrade-talm/internal/tsparams"
)

var _ = Describe(
	"Validating abort at IBU upgrade stage",
	Label(tsparams.LabelUpgradeAbortFlow), func() {
		BeforeEach(func() {
			By("Ensuring the spoke IBU is Idle and ready for Prep")

			Expect(cnfhelper.EnsureSpokeReadyForNextIbgu()).To(Succeed(),
				"spoke IBU was not Idle with Prep in validNextStages before upgrade abort")

			By("Fetching target sno cluster name")

			err := cnfclusterinfo.PreUpgradeClusterInfo.SaveClusterInfo()
			Expect(err).ToNot(HaveOccurred(), "Failed to extract target sno cluster name")

			tsparams.TargetSnoClusterName = cnfclusterinfo.PreUpgradeClusterInfo.Name
		})

		AfterEach(func() {
			deleteIbgusAndRecoverIfFailed()
		})

		// 69055 - Aborts an upgrade at IBU upgrade stage, before pivot.
		// A second Abort IBGU is required: the CRD does not allow appending
		// Abort after Prep+Upgrade. Delete the original IBGU after Abort
		// completes so leftover TALM ManifestWorks cannot re-apply Prep
		// during WaitUntilHealthy.
		It("Aborts an upgrade at IBU upgrade stage", reportxml.ID("69055"), func() {
			By("Creating Prep+Upgrade IBGU")

			_, err := cnfhelper.NewIbgu(tsparams.IbguName).
				WithPlan([]string{ibguv1alpha1.Prep}, 20, 20).
				WithPlan([]string{ibguv1alpha1.Upgrade}, 20, 20).
				Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create IBGU")

			By("Waiting until spoke IBU Prep completes")

			Expect(cnfhelper.WaitForSpokeIBU(cnfhelper.SpokePrepCompleted, 20*time.Minute)).To(Succeed(),
				"Prep did not complete on the spoke IBU")

			By("Waiting until spoke IBU Upgrade starts (pre-pivot)")

			Expect(cnfhelper.WaitForSpokeIBU(cnfhelper.SpokeUpgradeInProgress, 10*time.Minute)).To(Succeed(),
				"Upgrade did not start on the spoke IBU before pivot")

			By("Creating abort IBGU")

			abortIbgu, err := cnfhelper.NewIbgu(tsparams.AbortIbguName).
				WithPlan([]string{ibguv1alpha1.Abort}, 20, 20).
				Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create abort IBGU")

			By("Waiting until spoke IBU returns to Idle")

			Expect(cnfhelper.WaitForSpokeIBU(cnfhelper.SpokeIdle, 10*time.Minute)).To(Succeed(),
				"spoke IBU did not return to Idle after Abort")

			By("Waiting until abort IBGU completes")

			Expect(cnfhelper.WaitForIbgu(abortIbgu, cnfhelper.IbguCompleted, 10*time.Minute)).To(Succeed(),
				"Abort IBGU did not complete")

			By("Deleting original Prep+Upgrade IBGU")

			Expect(cnfhelper.DeleteIbgus(tsparams.IbguName)).To(Succeed(),
				"failed to delete original IBGU after Abort")

			By("Waiting until the spoke is healthy after upgrade abort")

			Expect(cnfhelper.WaitUntilHealthy(cnfhelper.DefaultHealthTimeout)).To(Succeed(),
				"spoke cluster was not healthy after upgrade abort")
		})
	})
