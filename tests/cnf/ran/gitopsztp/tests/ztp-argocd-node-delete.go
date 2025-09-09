package tests

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/argocd"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/bmh"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/mco"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/gitopsztp/internal/gitdetails"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/gitopsztp/internal/nodedelete"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/gitopsztp/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/rancluster"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/version"
)

var _ = Describe("ZTP Argo CD Node Deletion Tests", Label(tsparams.LabelArgoCdNodeDeletionTestCases), func() {
	var (
		plusOneNodeName         string
		bmhNamespace            string
		clustersApp             *argocd.ApplicationBuilder
		originalClustersGitPath string
	)

	BeforeEach(func() {
		By("checking the ZTP version")
		versionInRange, err := version.IsVersionStringInRange(RANConfig.ZTPVersion, "4.14", "")
		Expect(err).ToNot(HaveOccurred(), "Failed to check if ZTP version is in range")

		if !versionInRange {
			Skip("ZTP node deletion tests require ZTP version of at least 4.14")
		}

		By("checking that the cluster contains a control-plane and worker node")
		snoPlusOne, err := rancluster.IsSnoPlusOne(Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to check if cluster is SNO+1")

		if !snoPlusOne {
			Skip("Cluster does not contain a single control plane and a single worker node")
		}

		By("checking that the 'worker' mcp is ready")
		mcp, err := mco.Pull(Spoke1APIClient, "worker")
		Expect(err).ToNot(HaveOccurred(), "Failed to pull 'worker' MCP")
		Expect(mcp.Definition.Status.ReadyMachineCount).To(BeNumerically(">", 0), "Node deletion requires ready 'worker' MCP")

		plusOneNodeName, err = rancluster.GetPlusOneWorkerName(Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to get SNO+1 worker name")

		bmhNamespace, err = nodedelete.GetBmhNamespace(HubAPIClient, plusOneNodeName)
		Expect(err).ToNot(HaveOccurred(), "Failed to get BMH namespace")
		Expect(bmhNamespace).ToNot(BeEmpty(), "BMH namespace cannot be empty")

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
		updatedClustersApp, err := clustersApp.Update(true)
		Expect(err).ToNot(HaveOccurred(), "Failed to update the clusters app back to the original settings")

		By("waiting for the clusters app to sync")
		err = updatedClustersApp.WaitForSourceUpdate(true, tsparams.ArgoCdChangeTimeout)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait for the clusters app to sync")

		By("checking that the cluster is back to SNO+1")
		err = rancluster.WaitForNumberOfNodes(Spoke1APIClient, 2, 45*time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait for cluster to return to 2 nodes")

		snoPlusOne, err := rancluster.IsSnoPlusOne(Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to check if cluster is SNO+1")
		Expect(snoPlusOne).To(BeTrue(), "Cluster is no longer SNO+1")

		// This is a quick workaround to wait until we're sure the CSRs for the node we just added back have
		// been approved. It will be replaced by a function to wait for CSRs to be approved.
		By("waiting 5 minutes to ensure the CSRs are approved")
		time.Sleep(5 * time.Minute)
	})

	// 72463 - Delete and re-add a worker node from cluster
	It("should delete a worker node from the cluster", reportxml.ID("72463"), func() {
		By("updating the Argo CD git path to apply crAnnotation")
		if !clustersApp.DoesGitPathExist(tsparams.ZtpTestPathNodeDeleteAddAnnotation) {
			Skip(fmt.Sprintf("git path '%s' could not be found", tsparams.ZtpTestPathNodeDeleteAddAnnotation))
		}

		err := gitdetails.UpdateAndWaitForSync(clustersApp, true, tsparams.ZtpTestPathNodeDeleteAddAnnotation)
		Expect(err).ToNot(HaveOccurred(), "Failed to update Argo CD git path")

		By("waiting for the crAnnotation to be added to the worker node")
		bareMetalHost, err := bmh.Pull(HubAPIClient, plusOneNodeName, bmhNamespace)
		Expect(err).ToNot(HaveOccurred(), "Failed to pull BMH")

		_, err = bareMetalHost.WaitUntilAnnotationExists(tsparams.NodeDeletionCrAnnotation, tsparams.ArgoCdChangeTimeout)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait for BMH annotation")

		// This time the path not being found is an error since if one path for the test is found, the other
		// being missing is an actual issue, not just a skip because the test is not applicable.
		By("updating the Argo CD app to apply the suppression to the spec")
		// Since UpdateAndWaitForSync appends the git path, we need to reset it first to the original path
		// before appending the new path.
		clustersApp.Definition.Spec.Source.Path = originalClustersGitPath
		exists := clustersApp.DoesGitPathExist(tsparams.ZtpTestPathNodeDeleteAddSuppression)
		Expect(exists).To(BeTrue(), "Already applied node delete crAnnotation but cannot find node delete suppression path")

		err = gitdetails.UpdateAndWaitForSync(clustersApp, false, tsparams.ZtpTestPathNodeDeleteAddSuppression)
		Expect(err).ToNot(HaveOccurred(), "Failed to update Argo CD git path")

		By("waiting for the worker node to be removed")
		err = nodedelete.WaitForBMHDeprovisioning(HubAPIClient, plusOneNodeName, bmhNamespace, 60*time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait for worker BMH to be deprovisioned")

		By("checking that the cluster is healthy")
		healthy, err := rancluster.IsClusterStable(Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to check if spoke cluster is healthy")
		Expect(healthy).To(BeTrue(), "Spoke cluster was not healthy")
	})
})
