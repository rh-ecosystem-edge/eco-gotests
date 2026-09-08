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
	"Performing upgrade prep abort flow",
	Label(tsparams.LabelPrepAbortFlow), func() {
		BeforeEach(func() {
			By("Ensuring the spoke IBU is Idle and ready for Prep")

			Expect(cnfhelper.EnsureSpokeReadyForNextIbgu()).To(Succeed(),
				"spoke IBU was not Idle with Prep in validNextStages before prep abort")

			By("Fetching target sno cluster name")

			err := cnfclusterinfo.PreUpgradeClusterInfo.SaveClusterInfo()
			Expect(err).ToNot(HaveOccurred(), "Failed to extract target sno cluster name")

			tsparams.TargetSnoClusterName = cnfclusterinfo.PreUpgradeClusterInfo.Name
		})

		AfterEach(func() {
			deleteIbgusAndRecoverIfFailed()
		})

		// 68956 - Upgrade prep abort flow
		It("Upgrade prep abort flow", reportxml.ID("68956"), func() {
			By("Creating Prep+Abort IBGU")

			prepAbortIbgu, err := cnfhelper.NewIbgu(tsparams.IbguName).
				WithPlan([]string{ibguv1alpha1.Prep}, 20, 20).
				WithPlan([]string{ibguv1alpha1.Abort}, 20, 20).
				Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create IBGU")

			By("Waiting until spoke IBU Prep completes")

			Expect(cnfhelper.WaitForSpokeIBU(cnfhelper.SpokePrepCompleted, 20*time.Minute)).To(Succeed(),
				"Prep did not complete on the spoke IBU")

			By("Waiting until spoke IBU returns to Idle")

			Expect(cnfhelper.WaitForSpokeIBU(cnfhelper.SpokeIdle, 10*time.Minute)).To(Succeed(),
				"spoke IBU did not return to Idle after Abort")

			By("Waiting until Prep+Abort IBGU completes")

			Expect(cnfhelper.WaitForIbgu(prepAbortIbgu, cnfhelper.IbguCompleted, 10*time.Minute)).To(Succeed(),
				"Prep+Abort IBGU did not complete")

			By("Waiting until the spoke is healthy after prep abort")

			Expect(cnfhelper.WaitUntilHealthy(cnfhelper.DefaultHealthTimeout)).To(Succeed(),
				"spoke cluster was not healthy after prep abort")
		})
	})
