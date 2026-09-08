package upgrade_test

import (
	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/internal/cnfhelper"
	cnfibuvalidations "github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/internal/validations"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/upgrade-talm/internal/tsparams"
)

// setupNodeTypes are Ginkgo nodes that run before the It body. A failure there
// means the spec never entered the test, so AfterEach Recover cannot fix the
// setup failure and only delays the suite.
var setupNodeTypes = types.NodeTypeBeforeEach |
	types.NodeTypeJustBeforeEach |
	types.NodeTypeBeforeAll |
	types.NodeTypeBeforeSuite |
	types.NodeTypeSynchronizedBeforeSuite

// deleteIbgusAndRecoverIfFailed deletes leftover hub IBGUs after every spec.
// Recover runs only after a failed spec that entered the It body, so a
// successful upgrade is left for the Ordered rollback test and setup failures
// (BeforeEach/BeforeAll) fail fast instead of hanging in Recover.
func deleteIbgusAndRecoverIfFailed() {
	By("Deleting leftover IBGUs on the hub")

	deleteErr := cnfhelper.DeleteIbgus(tsparams.IbguName, tsparams.AbortIbguName, cnfhelper.RecoverIbguName)

	if shouldRecoverAfterFailedSpec() {
		By("Recovering spoke cluster health after failed spec")

		Expect(cnfhelper.Recover(leaveSuccessfulUpgrade())).To(Succeed(),
			"spoke cluster was not recovered after the failed test")
	}

	// Delayed failure check so Recover is still attempted after an It-body
	// failure even when leftover IBGU delete fails.
	Expect(deleteErr).To(Succeed(), "failed to delete leftover IBGUs")
}

// shouldRecoverAfterFailedSpec reports whether AfterEach should run Recover.
// Recover is skipped when the spec passed, or when it failed in setup before
// the It body ran.
func shouldRecoverAfterFailedSpec() bool {
	report := CurrentSpecReport()
	if !report.Failed() {
		return false
	}

	if report.Failure.FailureNodeType.Is(setupNodeTypes) {
		By("Skipping Recover because the spec failed in " +
			report.Failure.FailureNodeType.String() + " before entering the It body")

		return false
	}

	return true
}

// leaveSuccessfulUpgrade reports whether Recover must not roll back a completed
// Upgrade. That is only true after 68954 passed and before Ordered test 69058
// runs; a failed 69058 or any independent spec must still recover to Idle.
func leaveSuccessfulUpgrade() bool {
	if !cnfibuvalidations.UpgradeSucceeded {
		return false
	}

	return CurrentSpecReport().LeafNodeText != tsparams.SpecRollbackSuccessfulUpgrade
}
