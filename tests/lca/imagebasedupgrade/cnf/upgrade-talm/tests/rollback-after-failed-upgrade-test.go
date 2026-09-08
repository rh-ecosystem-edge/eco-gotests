package upgrade_test

import (
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	ibguv1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/imagebasedgroupupgrades/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/internal/cnfclusterinfo"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/internal/cnfhelper"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/internal/cnfinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/upgrade-talm/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/internal/nodestate"
)

var _ = Describe(
	"Validating rollback stage after a failed upgrade",
	Label(tsparams.LabelRollbackFlow), func() {
		BeforeEach(func() {
			By("Ensuring the spoke IBU is Idle and ready for Prep")

			Expect(cnfhelper.EnsureSpokeReadyForNextIbgu()).To(Succeed(),
				"spoke IBU was not Idle with Prep in validNextStages before auto-rollback")

			By("Saving target sno cluster info before the test")

			err := cnfclusterinfo.PreUpgradeClusterInfo.SaveClusterInfo()
			Expect(err).NotTo(HaveOccurred(), "Failed to collect and save target sno cluster info before the test")

			tsparams.TargetSnoClusterName = cnfclusterinfo.PreUpgradeClusterInfo.Name
		})

		AfterEach(func() {
			deleteIbgusAndRecoverIfFailed()
		})

		// 69054 - Rollback after a failed upgrade
		It("Rollback after a failed upgrade", reportxml.ID("69054"), func() {
			By("Creating Prep->Upgrade->FinalizeUpgrade IBGU with auto-rollback on failure")

			_, err := cnfhelper.NewIbgu(tsparams.IbguName).
				WithAutoRollbackOnFailure(300).
				WithPlan([]string{ibguv1alpha1.Prep}, 20, 20).
				WithPlan([]string{ibguv1alpha1.Upgrade}, 20, 20).
				WithPlan([]string{ibguv1alpha1.FinalizeUpgrade}, 20, 20).
				Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create IBGU")

			By("Get list of node to be upgraded")

			ibuNodes, err := nodes.List(TargetSNOAPIClient)
			Expect(err).NotTo(HaveOccurred(), "error listing node")

			By("Waiting until spoke IBU Prep completes")

			Expect(cnfhelper.WaitForSpokeIBU(cnfhelper.SpokePrepCompleted, 20*time.Minute)).To(Succeed(),
				"Prep did not complete on the spoke IBU")

			waitNodesUnreachable(ibuNodes)
			waitNodesAPIReachable(ibuNodes)
			waitIBUAvailable()
			waitBootedIntoStaterootB()

			By("Simulate a fault to make upgrade fail")

			faultInjectCmd := "echo a > /etc/mco/proxy.env"
			_, err = cluster.ExecCmdWithStdout(TargetSNOAPIClient, faultInjectCmd)
			Expect(err).NotTo(HaveOccurred(), "could not execute fault injection command")

			By("Verifying auto rollback triggered upon upgrade failure")

			waitNodesUnreachable(ibuNodes)
			waitNodesAPIReachable(ibuNodes)
			waitIBUAvailable()

			By("Waiting until spoke IBU reports UpgradeFailed")

			Expect(cnfhelper.WaitForSpokeIBU(cnfhelper.SpokeUpgradeFailed, 30*time.Minute)).To(Succeed(),
				"spoke IBU did not report UpgradeFailed after auto-rollback")

			By("Saving target sno cluster info after the test")

			err = cnfclusterinfo.PostUpgradeClusterInfo.SaveClusterInfo()
			Expect(err).NotTo(HaveOccurred(), "Failed to collect and save target sno cluster info after the test")

			By("Validating target sno cluster version after auto rollback")

			Expect(cnfclusterinfo.PreUpgradeClusterInfo.Version).
				To(Equal(cnfclusterinfo.PostUpgradeClusterInfo.Version),
					"Target sno cluster reports old cluster version")

			By("Creating abort IBGU after auto-rollback")

			abortIbgu, err := cnfhelper.NewIbgu(tsparams.AbortIbguName).
				WithPlan([]string{ibguv1alpha1.Abort}, 5, 10).
				Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create abort Ibgu.")

			By("Waiting until spoke IBU returns to Idle")

			Expect(cnfhelper.WaitForSpokeIBU(cnfhelper.SpokeIdle, 10*time.Minute)).To(Succeed(),
				"spoke IBU did not return to Idle after Abort")

			By("Waiting until abort IBGU completes")

			Expect(cnfhelper.WaitForIbgu(abortIbgu, cnfhelper.IbguCompleted, 10*time.Minute)).To(Succeed(),
				"abort IBGU did not complete")

			By("Waiting until the spoke is healthy after auto-rollback")

			Expect(cnfhelper.WaitUntilHealthy(cnfhelper.DefaultHealthTimeout)).To(Succeed(),
				"spoke cluster was not healthy after auto-rollback")
		})
	})

// waitNodesUnreachable waits until kube-apiserver and SSH on each node are down.
func waitNodesUnreachable(nodeList []*nodes.Builder) {
	By("Wait for node to become unreachable")

	for _, node := range nodeList {
		kubeAPIUnreachable, err := nodestate.WaitForNodeToBeUnreachable(node.Object.Name, "6443", time.Minute*10)
		Expect(err).To(BeNil(), "error waiting for %s kube-api to become unreachable", node.Object.Name)
		Expect(kubeAPIUnreachable).To(BeTrue(), "error: kube-api %s is still reachable", node.Object.Name)

		sshUnreachable, err := nodestate.WaitForNodeToBeUnreachable(node.Object.Name, "22", time.Minute*10)
		Expect(err).To(BeNil(), "error waiting for %s node to shutdown", node.Object.Name)
		Expect(sshUnreachable).To(BeTrue(), "error: node %s is still reachable", node.Object.Name)
	}
}

// waitNodesAPIReachable waits until kube-apiserver on each node is reachable.
func waitNodesAPIReachable(nodeList []*nodes.Builder) {
	By("Wait for node to become reachable")

	for _, node := range nodeList {
		reachable, err := nodestate.WaitForNodeToBeReachable(node.Object.Name, "6443", time.Minute*30)
		Expect(err).To(BeNil(), "error waiting for %s node to become reachable", node.Object.Name)
		Expect(reachable).To(BeTrue(), "error: node %s is still unreachable", node.Object.Name)
	}
}

// waitIBUAvailable waits until the spoke ImageBasedUpgrade CR exists after an ostree pivot.
func waitIBUAvailable() {
	By("Wait for IBU resource to be available")

	Expect(cnfhelper.WaitUntilIBUAvailable(cnfhelper.IBUAvailableTimeout)).To(Succeed(),
		"error waiting for ibu resource to become available")
}

// waitBootedIntoStaterootB waits until rpm-ostree reports the seed-image stateroot.
func waitBootedIntoStaterootB() {
	By("Waiting until the node has booted into stateroot B")

	seedImageVersion := CNFConfig.IbguSeedImageVersion
	Expect(seedImageVersion).NotTo(BeEmpty(), "seed image version is empty")

	Eventually(func() bool {
		stdoutByNode, execErr := cluster.ExecCmdWithStdout(TargetSNOAPIClient,
			"rpm-ostree status --json | jq '.deployments[0].osname'")
		if execErr != nil {
			return false
		}

		for _, stdout := range stdoutByNode {
			bootedStateroot := strings.ReplaceAll(stdout, "_", "-")
			if strings.Contains(bootedStateroot, seedImageVersion) {
				return true
			}
		}

		return false
	}, 2*time.Minute, 5*time.Second).Should(BeTrue(),
		"Target cluster node has not booted into stateroot B (%s)", seedImageVersion)
}
