package tests

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/cgu"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ocm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/olm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/version"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/talm/internal/helper"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/talm/internal/setup"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/talm/internal/tsparams"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
)

var _ = Describe("TALM Batching Tests", Label(tsparams.LabelBatchingTestCases), func() {
	var err error

	BeforeEach(func() {
		By("checking that hub and two spokes are present")
		Expect([]*clients.Settings{HubAPIClient, Spoke1APIClient, Spoke2APIClient}).
			ToNot(ContainElement(BeNil()), "Failed due to missing API client")

		By("ensuring TALM is at least version 4.12")
		versionInRange, err := version.IsVersionStringInRange(RANConfig.HubOperatorVersions[ranparam.TALM], "4.11", "")
		Expect(err).ToNot(HaveOccurred(), "Failed to compare TALM version string")

		if !versionInRange {
			Skip("TALM batching tests require version 4.12 or higher")
		}
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

	When("a single spoke is missing", Label(tsparams.LabelMissingSpokeTestCases), func() {
		// 47949 - Tests selected clusters must be non-compliant AND included in CGU.
		It("should report a missing spoke", reportxml.ID("47949"), func() {
			By("creating the CGU with non-existent cluster and policy")
			cguBuilder := cgu.NewCguBuilder(HubAPIClient, tsparams.CguName, tsparams.TestNamespace, 1).
				WithCluster(tsparams.NonExistentClusterName).
				WithManagedPolicy(tsparams.NonExistentPolicyName)
			cguBuilder.Definition.Spec.RemediationStrategy.Timeout = 1

			cguBuilder, err = cguBuilder.Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create CGU")

			By("waiting for the error condition to match")
			_, err = cguBuilder.WaitForCondition(tsparams.CguNonExistentClusterCondition, 3*tsparams.TalmDefaultReconcileTime)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for CGU to have matching condition")
		})
	})

	When("a policy is missing", Label(tsparams.LabelMissingPolicyTestCases), func() {
		// 47955 - Tests upgrade rejected due to specified managed policies missing
		It("should report the missing policy", reportxml.ID("47955"), func() {
			By("create and enable a CGU with a managed policy that does not exist")
			cguBuilder := cgu.NewCguBuilder(HubAPIClient, tsparams.CguName, tsparams.TestNamespace, 1).
				WithCluster(RANConfig.Spoke1Name).
				WithManagedPolicy("non-existent-policy")
			cguBuilder.Definition.Spec.RemediationStrategy.Timeout = 1

			cguBuilder, err = cguBuilder.Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create CGU")

			By("waiting for the CGU status to report the missing policy")
			// This should immediately error out so we don't need a long timeout
			_, err = cguBuilder.WaitForCondition(tsparams.CguNonExistentPolicyCondition, 2*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for CGU to have matching condition")
		})
	})

	When("there is a catalog source", Label(tsparams.LabelCatalogSourceTestCases), func() {
		// 47952 - Tests upgrade failure of one cluster would not affect other clusters
		It("should abort CGU when the first batch fails with the Abort batch timeout action", reportxml.ID("47952"), func() {
			By("verifying the temporary namespace does not exist on spoke1")
			tempExistsOnSpoke1 := namespace.NewBuilder(Spoke1APIClient, tsparams.TemporaryNamespace).Exists()
			Expect(tempExistsOnSpoke1).To(BeFalse(), "Temporary namespace already exists on spoke 1")

			By("creating the temporary namespace on spoke2 only")
			_, err = namespace.NewBuilder(Spoke2APIClient, tsparams.TemporaryNamespace).Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create temporary namespace on spoke 2")

			By("creating the CGU and associated resources")
			// Use a max concurrency of 1 so we can verify the CGU aborts after the first batch fails
			cguBuilder := cgu.NewCguBuilder(HubAPIClient, tsparams.CguName, tsparams.TestNamespace, 1).
				WithCluster(RANConfig.Spoke1Name).
				WithCluster(RANConfig.Spoke2Name).
				WithManagedPolicy(tsparams.PolicyName)
			cguBuilder.Definition.Spec.RemediationStrategy.Timeout = 9
			cguBuilder.Definition.Spec.Enable = ptr.To(false)
			cguBuilder.Definition.Spec.BatchTimeoutAction = "Abort"

			cguBuilder, err = helper.SetupCguWithCatSrc(cguBuilder)
			Expect(err).ToNot(HaveOccurred(), "Failed to setup CGU")

			By("waiting to enable the CGU")
			cguBuilder, err = helper.WaitToEnableCgu(cguBuilder)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait and enable the CGU")

			By("waiting for the CGU to timeout")
			cguBuilder, err = cguBuilder.WaitForCondition(tsparams.CguTimeoutReasonCondition, 11*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for CGU to timeout")

			By("validating that the policy failed on spoke1")
			catSrcExistsOnSpoke1 := olm.NewCatalogSourceBuilder(
				Spoke1APIClient, tsparams.CatalogSourceName, tsparams.TemporaryNamespace).Exists()
			Expect(catSrcExistsOnSpoke1).To(BeFalse(), "Catalog source exists on spoke 1")

			By("validating that the policy failed on spoke2")
			catSrcExistsOnSpoke2 := olm.NewCatalogSourceBuilder(
				Spoke2APIClient, tsparams.CatalogSourceName, tsparams.TemporaryNamespace).Exists()
			Expect(catSrcExistsOnSpoke2).To(BeFalse(), "Catalog source exists on spoke 2")

			By("validating that the timeout should have occurred after just the first reconcile")
			startTime := cguBuilder.Object.Status.Status.StartedAt.Time

			// endTime may be zero even after timeout so just use now instead.
			endTime := cguBuilder.Object.Status.Status.CompletedAt.Time
			if endTime.IsZero() {
				endTime = time.Now()
			}

			elapsed := endTime.Sub(startTime)
			klog.V(tsparams.LogLevel).Infof("start time: %v, end time: %v, elapsed: %v", startTime, endTime, elapsed)

			// We expect that the total runtime should be about equal to the expected timeout. In
			// particular, we expect it to be just about one reconcile loop for this test.
			Expect(elapsed).To(BeNumerically("~", tsparams.TalmDefaultReconcileTime, 10*time.Second))

			By("validating that the timeout message matched the abort message")
			_, err = cguBuilder.WaitForCondition(tsparams.CguTimeoutMessageCondition, time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for CGU to have matching condition")
		})

		// 47952 - Tests upgrade failure of one cluster would not affect other clusters
		It("should report the failed spoke when one spoke in a batch times out", reportxml.ID("47952"), func() {
			By("verifying the temporary namespace does not exist on spoke2")
			tempExistsOnSpoke2 := namespace.NewBuilder(Spoke2APIClient, tsparams.TemporaryNamespace).Exists()
			Expect(tempExistsOnSpoke2).To(BeFalse(), "Temporary namespace already exists on spoke 2")

			By("creating the temporary namespace on spoke1 only")
			_, err = namespace.NewBuilder(Spoke1APIClient, tsparams.TemporaryNamespace).Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create temporary namespace on spoke 1")

			By("creating the CGU and associated resources")
			// This test uses a max concurrency of 2 so both spokes are in the same batch.
			cguBuilder := cgu.NewCguBuilder(HubAPIClient, tsparams.CguName, tsparams.TestNamespace, 2).
				WithCluster(RANConfig.Spoke1Name).
				WithCluster(RANConfig.Spoke2Name).
				WithManagedPolicy(tsparams.PolicyName)
			cguBuilder.Definition.Spec.RemediationStrategy.Timeout = 9
			cguBuilder.Definition.Spec.Enable = ptr.To(false)

			cguBuilder, err = helper.SetupCguWithCatSrc(cguBuilder)
			Expect(err).ToNot(HaveOccurred(), "Failed to setup CGU")

			By("waiting to enable the CGU")
			cguBuilder, err = helper.WaitToEnableCgu(cguBuilder)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait and enable the CGU")

			By("waiting for the CGU to timeout")
			_, err = cguBuilder.WaitForCondition(tsparams.CguTimeoutReasonCondition, 16*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for CGU to timeout")

			By("validating that the policy succeeded on spoke1")
			catSrcExistsOnSpoke1 := olm.NewCatalogSourceBuilder(
				Spoke1APIClient, tsparams.CatalogSourceName, tsparams.TemporaryNamespace).Exists()
			Expect(catSrcExistsOnSpoke1).To(BeTrue(), "Catalog source does not exist on spoke 1")

			By("validating that the policy failed on spoke2")
			catSrcExistsOnSpoke2 := olm.NewCatalogSourceBuilder(
				Spoke2APIClient, tsparams.CatalogSourceName, tsparams.TemporaryNamespace).Exists()
			Expect(catSrcExistsOnSpoke2).To(BeFalse(), "Catalog source exists on spoke 2")
		})

		// 74753 upgrade failure of first batch would not affect second batch
		It("should continue the CGU when the first batch fails with the Continue batch timeout"+
			"action", reportxml.ID("74753"), func() {
			By("verifying the temporary namespace does not exist on spoke1")
			tempExistsOnSpoke1 := namespace.NewBuilder(Spoke1APIClient, tsparams.TemporaryNamespace).Exists()
			Expect(tempExistsOnSpoke1).To(BeFalse(), "Temporary namespace already exists on spoke 1")

			By("creating the temporary namespace on spoke2 only")
			_, err = namespace.NewBuilder(Spoke2APIClient, tsparams.TemporaryNamespace).Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create temporary namespace on spoke 2")

			By("creating the CGU and associated resources")
			// Max concurrency of one to ensure two batches are used.
			cguBuilder := cgu.NewCguBuilder(HubAPIClient, tsparams.CguName, tsparams.TestNamespace, 1).
				WithCluster(RANConfig.Spoke1Name).
				WithCluster(RANConfig.Spoke2Name).
				WithManagedPolicy(tsparams.PolicyName)
			cguBuilder.Definition.Spec.RemediationStrategy.Timeout = 9
			cguBuilder.Definition.Spec.Enable = ptr.To(false)

			cguBuilder, err = helper.SetupCguWithCatSrc(cguBuilder)
			Expect(err).ToNot(HaveOccurred(), "Failed to setup CGU")

			By("waiting to enable the CGU")
			cguBuilder, err = helper.WaitToEnableCgu(cguBuilder)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait and enable the CGU")

			By("waiting for the CGU to timeout")
			_, err = cguBuilder.WaitForCondition(tsparams.CguTimeoutReasonCondition, 16*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for CGU to timeout")

			By("validating that the policy succeeded on spoke2")
			catSrcExistsOnSpoke2 := olm.NewCatalogSourceBuilder(
				Spoke2APIClient, tsparams.CatalogSourceName, tsparams.TemporaryNamespace).Exists()
			Expect(catSrcExistsOnSpoke2).To(BeTrue(), "Catalog source doesn't exist on spoke 2")

			By("validating that the policy failed on spoke1")
			catSrcExistsOnSpoke1 := olm.NewCatalogSourceBuilder(
				Spoke1APIClient, tsparams.CatalogSourceName, tsparams.TemporaryNamespace).Exists()
			Expect(catSrcExistsOnSpoke1).To(BeFalse(), "Catalog source exists on spoke 1")
		})

		// 54296 - Batch Timeout Calculation
		It("should continue the CGU when the second batch fails with the Continue batch timeout action",
			reportxml.ID("54296"), func() {
				By("verifying the temporary namespace does not exist on spoke2")
				tempExistsOnSpoke2 := namespace.NewBuilder(Spoke2APIClient, tsparams.TemporaryNamespace).Exists()
				Expect(tempExistsOnSpoke2).To(BeFalse(), "Temporary namespace already exists on spoke 2")

				By("creating the temporary namespace on spoke1 only")
				_, err = namespace.NewBuilder(Spoke1APIClient, tsparams.TemporaryNamespace).Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create temporary namespace on spoke 1")

				expectedTimeout := 16

				By("creating the CGU and associated resources")
				// Max concurrency of one to ensure two batches are used.
				cguBuilder := cgu.NewCguBuilder(HubAPIClient, tsparams.CguName, tsparams.TestNamespace, 1).
					WithCluster(RANConfig.Spoke1Name).
					WithCluster(RANConfig.Spoke2Name).
					WithManagedPolicy(tsparams.PolicyName)
				cguBuilder.Definition.Spec.RemediationStrategy.Timeout = expectedTimeout
				cguBuilder.Definition.Spec.Enable = ptr.To(false)

				cguBuilder, err = helper.SetupCguWithCatSrc(cguBuilder)
				Expect(err).ToNot(HaveOccurred(), "Failed to setup CGU")

				By("waiting to enable the CGU")
				cguBuilder, err = helper.WaitToEnableCgu(cguBuilder)
				Expect(err).ToNot(HaveOccurred(), "Failed to wait and enable the CGU")

				By("waiting for the CGU to timeout")
				cguBuilder, err = cguBuilder.WaitForCondition(tsparams.CguTimeoutReasonCondition, 21*time.Minute)
				Expect(err).ToNot(HaveOccurred(), "Failed to wait for CGU to timeout")

				By("validating that the policy succeeded on spoke1")
				catSrcExistsOnSpoke1 := olm.NewCatalogSourceBuilder(
					Spoke1APIClient, tsparams.CatalogSourceName, tsparams.TemporaryNamespace).Exists()
				Expect(catSrcExistsOnSpoke1).To(BeTrue(), "Catalog source doesn't exist on spoke 1")

				By("validating that the policy failed on spoke2")
				catSrcExistsOnSpoke2 := olm.NewCatalogSourceBuilder(
					Spoke2APIClient, tsparams.CatalogSourceName, tsparams.TemporaryNamespace).Exists()
				Expect(catSrcExistsOnSpoke2).To(BeFalse(), "Catalog source exists on spoke 2")

				By("validating that CGU timeout is recalculated for later batches after earlier batches complete")
				startTime := cguBuilder.Object.Status.Status.StartedAt.Time

				// endTime may be zero even after timeout so just use now instead.
				endTime := cguBuilder.Object.Status.Status.CompletedAt.Time
				if endTime.IsZero() {
					endTime = time.Now()
				}

				elapsed := endTime.Sub(startTime)
				klog.V(tsparams.LogLevel).Infof("start time: %v, end time: %v, elapsed: %v", startTime, endTime, elapsed)
				// We expect that the total runtime should be about equal to the expected timeout. In
				// particular, we expect it to be +/- one reconcile loop time (5 minutes). The first
				// batch will complete successfully, so the second should use the entire remaining
				// expected timout.
				Expect(elapsed).To(BeNumerically("~", expectedTimeout*int(time.Minute), tsparams.TalmDefaultReconcileTime))
			})
	})

	When("there is a temporary namespace", Label(tsparams.LabelTempNamespaceTestCases), func() {
		// 47954 - Tests upgrade aborted due to short timeout.
		It("should report the timeout value when one cluster is in a batch and it times out", reportxml.ID("47954"), func() {
			By("verifying the temporary namespace does not exist on spoke1")
			tempExistsOnSpoke1 := namespace.NewBuilder(Spoke1APIClient, tsparams.TemporaryNamespace).Exists()
			Expect(tempExistsOnSpoke1).To(BeFalse(), "Temporary namespace already exists on spoke 1")

			expectedTimeout := 8

			By("creating the CGU and associated resources")
			cguBuilder := cgu.NewCguBuilder(HubAPIClient, tsparams.CguName, tsparams.TestNamespace, 1).
				WithCluster(RANConfig.Spoke1Name).
				WithManagedPolicy(tsparams.PolicyName)
			cguBuilder.Definition.Spec.RemediationStrategy.Timeout = expectedTimeout
			cguBuilder.Definition.Spec.Enable = ptr.To(false)

			cguBuilder, err = helper.SetupCguWithCatSrc(cguBuilder)
			Expect(err).ToNot(HaveOccurred(), "Failed to setup CGU")

			By("waiting to enable the CGU")
			cguBuilder, err = helper.WaitToEnableCgu(cguBuilder)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait and enable the CGU")

			By("waiting for the CGU to timeout")
			cguBuilder, err = cguBuilder.WaitForCondition(tsparams.CguTimeoutReasonCondition, 11*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for CGU to timeout")

			By("validating that the timeout should have occurred after just the first reconcile")
			startTime := cguBuilder.Object.Status.Status.StartedAt.Time

			// endTime may be zero even after timeout so just use now instead.
			endTime := cguBuilder.Object.Status.Status.CompletedAt.Time
			if endTime.IsZero() {
				endTime = time.Now()
			}

			elapsed := endTime.Sub(startTime)
			klog.V(tsparams.LogLevel).Infof("start time: %v, end time: %v, elapsed: %v", startTime, endTime, elapsed)
			// We expect that the total runtime should be about equal to the expected timeout. In
			// particular, we expect it to be just about one reconcile loop for this test
			Expect(elapsed).To(BeNumerically("~", expectedTimeout*int(time.Minute), tsparams.TalmDefaultReconcileTime))

			By("verifying the test policy was deleted upon CGU expiration")
			talmPolicyPrefix := fmt.Sprintf("%s-%s", tsparams.CguName, tsparams.PolicyName)
			talmGeneratedPolicyName, err := helper.GetPolicyNameWithPrefix(
				HubAPIClient, talmPolicyPrefix, tsparams.TestNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to get policy name with the prefix %s", talmPolicyPrefix)

			if talmGeneratedPolicyName != "" {
				By("waiting for the test policy to be deleted")
				policyBuilder, err := ocm.PullPolicy(HubAPIClient, talmGeneratedPolicyName, tsparams.TestNamespace)
				if err == nil {
					err = policyBuilder.WaitUntilDeleted(5 * time.Minute)
					Expect(err).ToNot(HaveOccurred(), "Failed to wait for the test policy to be deleted")
				}
			}
		})

		// 47947 - Tests successful ocp and operator upgrade with canaries and multiple batches.
		It("should complete the CGU when two clusters are successful in a single batch", reportxml.ID("47947"), func() {
			By("creating the CGU and associated resources")
			cguBuilder := cgu.NewCguBuilder(HubAPIClient, tsparams.CguName, tsparams.TestNamespace, 1).
				WithManagedPolicy(tsparams.PolicyName)
			cguBuilder.Definition.Spec.RemediationStrategy.Timeout = 15
			cguBuilder.Definition.Spec.Enable = ptr.To(false)

			By(fmt.Sprintf(
				"using MatchLabels with name %s and MatchExpressions with name %s", RANConfig.Spoke1Name, RANConfig.Spoke2Name))
			policyLabelSelector := metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{{
					Key:      "common",
					Operator: "In",
					Values:   []string{"true"},
				}},
			}

			cguBuilder.Definition.Spec.ClusterLabelSelectors = []metav1.LabelSelector{
				{MatchLabels: map[string]string{"name": RANConfig.Spoke1Name}},
				{MatchExpressions: []metav1.LabelSelectorRequirement{{
					Key:      "name",
					Operator: "In",
					Values:   []string{RANConfig.Spoke2Name},
				}}},
			}

			tempNs := namespace.NewBuilder(HubAPIClient, tsparams.TemporaryNamespace)
			tempNs.Definition.Kind = "Namespace"
			tempNs.Definition.APIVersion = corev1.SchemeGroupVersion.Version

			_, err = helper.CreatePolicy(HubAPIClient, tempNs.Definition, "")
			Expect(err).ToNot(HaveOccurred(), "Failed to create policy in testing namespace")

			err = helper.CreatePolicyComponents(
				HubAPIClient, "", cguBuilder.Definition.Spec.Clusters, policyLabelSelector)
			Expect(err).ToNot(HaveOccurred(), "Failed to create policy components in testing namespace")

			cguBuilder, err = cguBuilder.Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create CGU")

			By("waiting to enable the CGU")
			cguBuilder, err = helper.WaitToEnableCgu(cguBuilder)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait and enable the CGU")

			By("waiting for the CGU to finish successfully")
			_, err = cguBuilder.WaitForCondition(tsparams.CguSuccessfulFinishCondition, 21*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for the CGU to finish successfully")

			By("verifying the test policy was deleted upon CGU expiration")
			talmPolicyPrefix := fmt.Sprintf("%s-%s", tsparams.CguName, tsparams.PolicyName)
			talmGeneratedPolicyName, err := helper.GetPolicyNameWithPrefix(
				HubAPIClient, talmPolicyPrefix, tsparams.TestNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to get policy name with the prefix %s", talmPolicyPrefix)

			if talmGeneratedPolicyName != "" {
				By("waiting for the test policy to be deleted")
				policyBuilder, err := ocm.PullPolicy(HubAPIClient, talmGeneratedPolicyName, tsparams.TestNamespace)
				if err == nil {
					err = policyBuilder.WaitUntilDeleted(5 * time.Minute)
					Expect(err).ToNot(HaveOccurred(), "Failed to wait for the test policy to be deleted")
				}
			}
		})
	})
})
