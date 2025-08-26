package tests

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/openshift-kni/cluster-group-upgrades-operator/pkg/api/clustergroupupgrades/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/cgu"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/version"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/talm/internal/helper"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/talm/internal/setup"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/talm/internal/tsparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

const (
	blockingA = "-blocking-a"
	blockingB = "-blocking-b"
)

var _ = Describe("TALM Blocking CRs Tests", Label(tsparams.LabelBlockingCRTestCases), func() {
	var err error

	BeforeEach(func() {
		By("ensuring TALM is at least version 4.12")
		versionInRange, err := version.IsVersionStringInRange(RANConfig.HubOperatorVersions[ranparam.TALM], "4.11", "")
		Expect(err).ToNot(HaveOccurred(), "Failed to compare TALM version string")

		if !versionInRange {
			Skip("TALM blocking CR tests require version 4.12 or higher")
		}
	})

	AfterEach(func() {
		By("Cleaning up test resources on hub")
		errList := setup.CleanupTestResourcesOnHub(HubAPIClient, tsparams.TestNamespace, blockingA)
		Expect(errList).To(BeEmpty(), "Failed to cleanup resources for blocking A on hub")

		errList = setup.CleanupTestResourcesOnHub(HubAPIClient, tsparams.TestNamespace, blockingB)
		Expect(errList).To(BeEmpty(), "Failed to cleanup resources for blocking B on hub")

		By("Deleting test namespaces on spoke 1")
		err := namespace.NewBuilder(Spoke1APIClient, tsparams.TemporaryNamespace+blockingA).
			DeleteAndWait(5 * time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Failed to delete namespace for blocking A on spoke 1")

		err = namespace.NewBuilder(Spoke1APIClient, tsparams.TemporaryNamespace+blockingB).
			DeleteAndWait(5 * time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Failed to delete namespace for blocking B on spoke 1")
	})

	When("a blocking CR passes", func() {
		// 47948 - Tests multiple CGUs can be enabled in parallel with blocking CR.
		It("verifies CGU succeeded with blocking CR", reportxml.ID("47948"), func() {
			By("creating two sets of CRs where B will be blocked until A is done")
			cguA := getBlockingCGU(blockingA, 10)
			cguB := getBlockingCGU(blockingB, 15)
			cguB.Definition.Spec.BlockingCRs = []v1alpha1.BlockingCR{{
				Name:      tsparams.CguName + blockingA,
				Namespace: tsparams.TestNamespace,
			}}

			cguA, err = helper.SetupCguWithNamespace(cguA, blockingA)
			Expect(err).ToNot(HaveOccurred(), "Failed to setup CGU A")

			cguB, err = helper.SetupCguWithNamespace(cguB, blockingB)
			Expect(err).ToNot(HaveOccurred(), "Failed to setup CGU B")

			cguA, cguB = waitToEnableCgus(cguA, cguB)

			By("Waiting to verify if CGU B is blocked by A")
			blockedMessage := fmt.Sprintf(tsparams.TalmBlockedMessage, tsparams.CguName+blockingA)
			err = helper.WaitForCguBlocked(cguB, blockedMessage)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for CGU B to be blocked")

			By("Waiting for CGU A to succeed")
			_, err = cguA.WaitForCondition(tsparams.CguSuccessfulFinishCondition, 12*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for CGU A to succeed")

			By("Waiting for CGU B to succeed")
			_, err = cguB.WaitForCondition(tsparams.CguSuccessfulFinishCondition, 17*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for CGU B to succeed")
		})
	})

	When("a blocking CR fails", func() {
		// 74768 - Tests multiple CGUs can be enabled in parallel with blocking CR.
		It("verifies CGU fails with blocking CR", reportxml.ID("74768"), func() {
			By("creating two sets of CRs where B will be blocked until A is done")
			cguA := getBlockingCGU(blockingA, 2)
			cguB := getBlockingCGU(blockingB, 1)
			cguB.Definition.Spec.BlockingCRs = []v1alpha1.BlockingCR{{
				Name:      tsparams.CguName + blockingA,
				Namespace: tsparams.TestNamespace,
			}}

			By("Setting up CGU A with a faulty namespace")
			tempNs := namespace.NewBuilder(HubAPIClient, tsparams.TemporaryNamespace+blockingA)
			tempNs.Definition.Kind = "faulty namespace"

			_, err := helper.CreatePolicy(HubAPIClient, tempNs.Definition, blockingA)
			Expect(err).ToNot(HaveOccurred(), "Failed to create policy in testing namespace")

			err = helper.CreatePolicyComponents(
				HubAPIClient, blockingA, cguA.Definition.Spec.Clusters, metav1.LabelSelector{})
			Expect(err).ToNot(HaveOccurred(), "Failed to create policy components in testing namespace")

			cguA, err = cguA.Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create CGU")

			By("Setting up CGU B correctly")
			cguB, err = helper.SetupCguWithNamespace(cguB, blockingB)
			Expect(err).ToNot(HaveOccurred(), "Failed to setup CGU B")

			cguA, cguB = waitToEnableCgus(cguA, cguB)

			blockedMessage := fmt.Sprintf(tsparams.TalmBlockedMessage, tsparams.CguName+blockingA)

			By("Waiting to verify if CGU B is blocked by A")
			err = helper.WaitForCguBlocked(cguB, blockedMessage)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for CGU B to be blocked")

			By("Waiting for CGU A to fail because of timeout")
			_, err = cguA.WaitForCondition(tsparams.CguTimeoutMessageCondition, 7*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for CGU A to fail")

			By("Verifiying that CGU B is still blocked")
			err = helper.WaitForCguBlocked(cguB, blockedMessage)
			Expect(err).ToNot(HaveOccurred(), "Failed to verify that CGU B is still blocked")
		})
	})

	When("a blocking CR is missing", func() {
		// 47956 - Tests multiple CGUs can be enabled in parallel with missing blocking CR.
		It("verifies CGU is blocked until blocking CR created and succeeded", reportxml.ID("47956"), func() {
			By("creating two sets of CRs where B will be blocked until A is done")
			cguA := getBlockingCGU(blockingA, 10)
			cguB := getBlockingCGU(blockingB, 15)
			cguB.Definition.Spec.BlockingCRs = []v1alpha1.BlockingCR{{
				Name:      tsparams.CguName + blockingA,
				Namespace: tsparams.TestNamespace,
			}}

			By("Setting up CGU B")
			cguB, err = helper.SetupCguWithNamespace(cguB, blockingB)
			Expect(err).ToNot(HaveOccurred(), "Failed to setup CGU B")

			By("Waiting for the system to settle")
			time.Sleep(tsparams.TalmSystemStablizationTime)

			By("Enabling CGU B")
			cguB.Definition.Spec.Enable = ptr.To(true)
			cguB, err = cguB.Update(true)
			Expect(err).ToNot(HaveOccurred(), "Failed to enable CGU B")

			By("Waiting to verify if CGU B is blocked by A because it's missing")
			blockedMessage := fmt.Sprintf(tsparams.TalmMissingCRMessage, tsparams.CguName+blockingA)
			err = helper.WaitForCguBlocked(cguB, blockedMessage)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for CGU B to be blocked")

			By("Setting up CGU A")
			cguA, err = helper.SetupCguWithNamespace(cguA, blockingA)
			Expect(err).ToNot(HaveOccurred(), "Failed to setup CGU A")

			By("Enabling CGU A")
			cguA.Definition.Spec.Enable = ptr.To(true)
			cguA, err = cguA.Update(true)
			Expect(err).ToNot(HaveOccurred(), "Failed to enable CGU A")

			By("Waiting for CGU A to succeed")
			_, err = cguA.WaitForCondition(tsparams.CguSucceededCondition, 12*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for CGU A to succeed")

			By("Waiting for CGU B to succeed")
			_, err = cguB.WaitForCondition(tsparams.CguSucceededCondition, 17*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for CGU B to succeed")
		})
	})
})

func getBlockingCGU(suffix string, timeout int) *cgu.CguBuilder {
	cguBuilder := cgu.NewCguBuilder(HubAPIClient, tsparams.CguName+suffix, tsparams.TestNamespace, 1).
		WithCluster(RANConfig.Spoke1Name).
		WithManagedPolicy(tsparams.PolicyName + suffix)
	cguBuilder.Definition.Spec.RemediationStrategy.Timeout = timeout
	cguBuilder.Definition.Spec.Enable = ptr.To(false)

	return cguBuilder
}

func waitToEnableCgus(cguA *cgu.CguBuilder, cguB *cgu.CguBuilder) (*cgu.CguBuilder, *cgu.CguBuilder) {
	var err error

	By("Waiting for the system to settle")
	time.Sleep(tsparams.TalmSystemStablizationTime)

	By("Enabling CGU A and CGU B")

	cguA.Definition.Spec.Enable = ptr.To(true)
	cguA, err = cguA.Update(true)
	Expect(err).ToNot(HaveOccurred(), "Failed to enable CGU A")

	cguB.Definition.Spec.Enable = ptr.To(true)
	cguB, err = cguB.Update(true)
	Expect(err).ToNot(HaveOccurred(), "Failed to enable CGU B")

	return cguA, cguB
}
