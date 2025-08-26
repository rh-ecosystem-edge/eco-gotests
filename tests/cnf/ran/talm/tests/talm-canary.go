package tests

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/cgu"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/rancluster"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/talm/internal/helper"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/talm/internal/setup"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/talm/internal/tsparams"
	"k8s.io/utils/ptr"
)

var _ = Describe("TALM Canary Tests", Label(tsparams.LabelCanaryTestCases), func() {
	var err error

	BeforeEach(func() {
		By("checking that hub and two spokes are present")
		Expect(rancluster.AreClustersPresent([]*clients.Settings{HubAPIClient, Spoke1APIClient, Spoke2APIClient})).
			To(BeTrue(), "Failed due to missing API client")
	})

	AfterEach(func() {
		By("cleaning up resources on hub")
		errorList := setup.CleanupTestResourcesOnHub(HubAPIClient, tsparams.TestNamespace, "")
		Expect(errorList).To(BeEmpty(), "Failed to clean up test resources on hub")

		By("cleaning up resources on spokes")
		errorList = setup.CleanupTestResourcesOnSpokes(
			[]*clients.Settings{Spoke1APIClient, Spoke2APIClient}, "")
		Expect(errorList).To(BeEmpty(), "Failed to clean up test resources on spokes")
	})

	// 47954 - Tests upgrade aborted due to short timeout.
	It("should stop the CGU where first canary fails", reportxml.ID("47954"), func() {
		var err error

		By("verifying the temporary namespace does not exist on spoke 1 and 2")
		tempExistsOnSpoke1 := namespace.NewBuilder(Spoke1APIClient, tsparams.TemporaryNamespace).Exists()
		Expect(tempExistsOnSpoke1).To(BeFalse(), "Temporary namespace already exists on spoke 1")

		tempExistsOnSpoke2 := namespace.NewBuilder(Spoke2APIClient, tsparams.TemporaryNamespace).Exists()
		Expect(tempExistsOnSpoke2).To(BeFalse(), "Temporary namespace already exists on spoke 2")

		By("creating the CGU and associated resources")
		cguBuilder := cgu.NewCguBuilder(HubAPIClient, tsparams.CguName, tsparams.TestNamespace, 1).
			WithCluster(RANConfig.Spoke1Name).
			WithCluster(RANConfig.Spoke2Name).
			WithCanary(RANConfig.Spoke2Name).
			WithManagedPolicy(tsparams.PolicyName)
		cguBuilder.Definition.Spec.Enable = ptr.To(false)
		cguBuilder.Definition.Spec.RemediationStrategy.Timeout = 9

		cguBuilder, err = helper.SetupCguWithCatSrc(cguBuilder)
		Expect(err).ToNot(HaveOccurred(), "Failed to setup CGU")

		By("Waiting for the system to settle")
		time.Sleep(tsparams.TalmSystemStablizationTime)

		By("enabling the CGU")
		cguBuilder.Definition.Spec.Enable = ptr.To(true)
		cguBuilder, err = cguBuilder.Update(true)
		Expect(err).ToNot(HaveOccurred(), "Failed to enable CGU")

		By("making sure the canary cluster (spoke 2) starts first")
		cguBuilder, err = cguBuilder.WaitUntilClusterInProgress(RANConfig.Spoke2Name, 3*tsparams.TalmDefaultReconcileTime)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait for batch remediation for spoke 2 to be in progress")

		By("Making sure the non-canary cluster (spoke 1) has not started yet")
		progress, ok := cguBuilder.Object.Status.Status.CurrentBatchRemediationProgress[RANConfig.Spoke1Name]
		if ok {
			Expect(progress.State).ToNot(Equal("InProgress"), "Batch remediation for non-canary cluster has already started")
		}

		By("Validating that the timeout was due to canary failure")
		_, err = cguBuilder.WaitForCondition(tsparams.CguTimeoutCanaryCondition, 11*time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait for timeout due to canary failure")
	})

	// 47947 - Tests successful ocp and operator upgrade with canaries and multiple batches.
	It("should complete the CGU where all canaries are successful", reportxml.ID("47947"), func() {
		By("creating the CGU and associated resources")
		cguBuilder := cgu.NewCguBuilder(HubAPIClient, tsparams.CguName, tsparams.TestNamespace, 1).
			WithCluster(RANConfig.Spoke1Name).
			WithCluster(RANConfig.Spoke2Name).
			WithCanary(RANConfig.Spoke2Name).
			WithManagedPolicy(tsparams.PolicyName)
		cguBuilder.Definition.Spec.RemediationStrategy.Timeout = 9
		cguBuilder, err = helper.SetupCguWithNamespace(cguBuilder, "")
		Expect(err).ToNot(HaveOccurred(), "Failed to setup CGU")

		By("making sure the canary cluster (spoke 2) starts first")
		cguBuilder, err = cguBuilder.WaitUntilClusterInProgress(RANConfig.Spoke2Name, 3*tsparams.TalmDefaultReconcileTime)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait for batch remediation for spoke 2 to be in progress")

		By("Making sure the non-canary cluster (spoke 1) has not started yet")
		progress, ok := cguBuilder.Object.Status.Status.CurrentBatchRemediationProgress[RANConfig.Spoke1Name]
		if ok {
			Expect(progress.State).ToNot(Equal("InProgress"), "Batch remediation for non-canary cluster has already started")
		}

		By("waiting for the CGU to finish successfully")
		_, err = cguBuilder.WaitForCondition(tsparams.CguSuccessfulFinishCondition, 10*time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait for CGU to finish successfully")
	})
})
