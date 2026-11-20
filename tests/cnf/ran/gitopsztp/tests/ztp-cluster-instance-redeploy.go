package tests

import (
	"context"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/argocd"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/bmh"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/hive"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ocm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	argocdtypes "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/argocd/argocdtypes/v1alpha1"
	siteconfigv1alpha1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/siteconfig/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/siteconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/gitopsztp/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/alerter"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/querier"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/rancluster"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/version"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
)

const clusterRedeployTimeout = 90 * time.Minute

// CNF-24973 verifies the lifecycle of a running bare-metal MNO spoke managed through ClusterInstance.
var _ = Describe("ZTP ClusterInstance redeploy", Label(tsparams.LabelClusterInstanceDeleteTestCases), func() {
	var (
		clustersApp            *argocd.ApplicationBuilder
		originalSyncPolicy     *argocdtypes.SyncPolicy
		autoSyncPaused         bool
		clusterInstanceDeleted bool
		probeRule              *monitoringv1.PrometheusRule
		cleanupQuerierAccess   bool
	)

	BeforeEach(func() {
		spokeVersion, err := version.GetOCPVersion(Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to get spoke OpenShift version")

		supportedVersion, err := version.IsVersionStringInRange(spokeVersion, "4.22.0-0", "")
		Expect(err).ToNot(HaveOccurred(), "Failed to check spoke OpenShift version")
		if !supportedVersion {
			Skip("CNF-24973 requires OpenShift 4.22 or later")
		}

		clustersApp, err = argocd.PullApplication(
			HubAPIClient, redeployClustersApplicationName(), ranparam.OpenshiftGitOpsNamespace)
		Expect(err).ToNot(HaveOccurred(), "Failed to get clusters Argo CD application")
		// The clusters app may legitimately start OutOfSync: an aborted reinstall can leave an
		// immutable spec.reinstall on the live ClusterInstance that GitOps cannot prune. This test
		// deletes and recreates the ClusterInstance, which clears such drift, so it requires the
		// app to be Healthy but not necessarily Synced. Log the sync status for context.
		if clustersApp.Object.Status.Sync.Status != argocdtypes.SyncStatusCodeSynced {
			GinkgoWriter.Printf(
				"Clusters Argo CD application is not Synced at test start (status=%s); "+
					"recreating the ClusterInstance should resolve this.\n",
				clustersApp.Object.Status.Sync.Status)
		}
		Expect(string(clustersApp.Object.Status.Health.Status)).To(Equal("Healthy"),
			"Clusters Argo CD application must be Healthy before this test")

		if clustersApp.Definition.Spec.SyncPolicy != nil {
			originalSyncPolicy = clustersApp.Definition.Spec.SyncPolicy.DeepCopy()
		}
	})

	It("detaches and redeploys the running MNO spoke", reportxml.ID("24973"), func() {
		// Register recovery in dependency order. Ginkgo executes these cleanups in reverse order.
		DeferCleanup(func() {
			if cleanupQuerierAccess {
				Expect(querier.CleanupQuerierResources(HubAPIClient)).
					To(Succeed(), "Failed to remove temporary ACM metrics access")
			}
		})
		DeferCleanup(func() {
			if probeRule != nil {
				err := Spoke1APIClient.Delete(context.Background(), probeRule)
				Expect(err == nil || apierrors.IsNotFound(err)).To(BeTrue(),
					"Failed to delete temporary spoke alert rule: %v", err)
			}
		})
		DeferCleanup(func() {
			if autoSyncPaused {
				Expect(setClustersSyncPolicy(originalSyncPolicy)).To(Succeed(),
					"Failed to restore clusters app sync policy")
			}
		})
		DeferCleanup(func() {
			if clusterInstanceDeleted {
				Expect(syncClustersApplication(clusterRedeployTimeout)).To(Succeed(),
					"Failed to restore the deleted ClusterInstance")
				Expect(refreshSpokeAPIClientFromClusterDeployment()).To(Succeed(),
					"Failed to refresh spoke API client after restoring the ClusterInstance")
			}
		})

		By("checking the spoke is a multi-node cluster")
		controlPlaneNodes, err := rancluster.ListNodesByLabel(Spoke1APIClient, RANConfig.ControlPlaneLabelMap)
		Expect(err).ToNot(HaveOccurred(), "Failed to list spoke control-plane nodes")
		if len(controlPlaneNodes) < 2 {
			Skip("CNF-24973 applies only to a multi-node spoke")
		}

		By("checking the spoke is operational and recording its version and node count")
		originalVersion, err := version.GetOCPVersion(Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Spoke API is unavailable")
		originalNodeCount, err := readySpokeNodeCount()
		Expect(err).ToNot(HaveOccurred(), "Spoke nodes are not all Ready")
		Expect(originalNodeCount).To(BeNumerically(">", 0), "Spoke has no nodes")

		By("checking the ClusterInstance is provisioned on the hub")
		clusterInstance, err := siteconfig.PullClusterInstance(HubAPIClient, RANConfig.Spoke1Name, RANConfig.Spoke1Name)
		Expect(err).ToNot(HaveOccurred(), "Failed to find spoke ClusterInstance")
		_, err = clusterInstance.WaitForCondition(metav1.Condition{
			Type: string(siteconfigv1alpha1.ClusterProvisioned), Reason: string(siteconfigv1alpha1.Completed),
		}, 5*time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Spoke ClusterInstance is not provisioned")

		By("checking the spoke uses bare-metal hosts")
		hosts, err := bmh.List(HubAPIClient, RANConfig.Spoke1Name)
		Expect(err).ToNot(HaveOccurred(), "Failed to list spoke BareMetalHosts")
		Expect(hosts).ToNot(BeEmpty(), "CNF-24973 requires bare-metal hosts")

		clusterDeployment, err := hive.PullClusterDeployment(HubAPIClient, RANConfig.Spoke1Name, RANConfig.Spoke1Name)
		Expect(err).ToNot(HaveOccurred(), "Failed to find spoke ClusterDeployment")
		originalClusterID := clusterDeployment.Object.Spec.ClusterMetadata.ClusterID
		originalInfraID := clusterDeployment.Object.Spec.ClusterMetadata.InfraID
		Expect(originalClusterID).ToNot(BeEmpty(), "ClusterDeployment has no cluster ID")
		Expect(originalInfraID).ToNot(BeEmpty(), "ClusterDeployment has no infra ID")
		managedCluster, err := ocm.PullManagedCluster(HubAPIClient, RANConfig.Spoke1Name)
		Expect(err).ToNot(HaveOccurred(), "Failed to find the spoke ManagedCluster")
		_, err = managedCluster.WaitForCondition(metav1.Condition{
			Type: "ManagedClusterConditionAvailable", Status: metav1.ConditionTrue,
		}, 5*time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Spoke ManagedCluster is not Available before detachment")

		By("connecting to ACM Observability and recording the spoke metrics and alerts")
		cleanupQuerierAccess, err = querierResourcesNeedCleanup()
		Expect(err).ToNot(HaveOccurred(), "Failed to check ACM metrics access resources")
		metricsAPI, err := newACMMetricsClient()
		Expect(err).ToNot(HaveOccurred(), "Failed to create ACM metrics client")
		alertsAPI, err := alerter.CreateAlerterClientForCluster(HubAPIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to create ACM Alertmanager client")

		testID := uuid.NewString()
		probeRule, err = createOrUpdateRedeployProbeRule(testID, "Temporary alert for CNF-24973")
		Expect(err).ToNot(HaveOccurred(), "Failed to create temporary spoke alert rule")

		Eventually(func() (bool, error) {
			return redeployACMSpokeMetricExists(metricsAPI, RANConfig.Spoke1Name)
		}).WithTimeout(12*time.Minute).WithPolling(30*time.Second).Should(BeTrue(),
			"ACM did not receive spoke metrics before detachment")
		Eventually(func() (bool, error) {
			return redeployACMSpokeAlertExists(alertsAPI, testID)
		}).WithTimeout(5*time.Minute).WithPolling(10*time.Second).Should(BeTrue(),
			"ACM did not receive the probe alert before detachment")

		By("pausing automated sync so Argo CD does not immediately recreate the deleted object")
		if clustersApp.Definition.Spec.SyncPolicy != nil &&
			clustersApp.Definition.Spec.SyncPolicy.Automated != nil {
			pausedPolicy := clustersApp.Definition.Spec.SyncPolicy.DeepCopy()
			pausedPolicy.Automated = nil
			autoSyncPaused = true
			Expect(setClustersSyncPolicy(pausedPolicy)).To(Succeed(), "Failed to pause clusters app auto-sync")
		}

		By("deleting the ClusterInstance to detach the spoke")
		clusterInstanceDeleted = true
		Expect(clusterInstance.Delete()).To(Succeed(), "Failed to delete spoke ClusterInstance")
		Expect(waitForClusterInstance(RANConfig.Spoke1Name, RANConfig.Spoke1Name, false, 30*time.Minute)).To(Succeed(),
			"ClusterInstance was not removed from the hub")
		Expect(waitForManagedCluster(RANConfig.Spoke1Name, false, 15*time.Minute)).To(Succeed(),
			"ManagedCluster was not detached from ACM")
		By("checking the spoke remains operational while detached")
		detachedVersion, err := version.GetOCPVersion(Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Spoke API is unavailable after detachment")
		Expect(detachedVersion).To(Equal(originalVersion), "Spoke version changed while detached")
		detachedNodeCount, err := readySpokeNodeCount()
		Expect(err).ToNot(HaveOccurred(), "Spoke nodes are not Ready while detached")
		Expect(detachedNodeCount).To(Equal(originalNodeCount), "Spoke node count changed while detached")

		By("checking ACM no longer receives spoke metrics or new alert updates")
		detachedAlertSummary := "CNF-24973 detached probe " + testID
		Expect(updateRedeployProbeRuleSummary(probeRule, detachedAlertSummary)).To(Succeed(),
			"Failed to update the spoke alert probe while detached")
		Eventually(func() (bool, error) {
			return redeployACMSpokeMetricExists(metricsAPI, RANConfig.Spoke1Name)
		}).WithTimeout(15*time.Minute).WithPolling(30*time.Second).Should(BeFalse(),
			"ACM continued to return spoke metrics after detachment")
		// The changed annotation gives the test a direct marker for any new alert payload.
		var detachedAlertSummaries []string
		Consistently(func() (bool, error) {
			updated, summaries, err := redeployACMSpokeAlertHasSummary(alertsAPI, testID, detachedAlertSummary)
			detachedAlertSummaries = summaries

			return updated, err
		}).WithTimeout(10*time.Minute).WithPolling(15*time.Second).Should(BeFalse(),
			"ACM received the detached spoke alert update; observed summaries: %v", detachedAlertSummaries)

		By("recreating the ClusterInstance from the clusters Argo CD application")
		Expect(syncClustersApplication(clusterRedeployTimeout)).To(Succeed(),
			"Failed to redeploy the spoke from the clusters app")
		Expect(refreshSpokeAPIClientFromClusterDeployment()).To(Succeed(),
			"Failed to refresh spoke API client after redeployment")
		clusterInstanceDeleted = false
		redeployedAlertSummary := "CNF-24973 redeployed probe " + testID
		probeRule, err = createOrUpdateRedeployProbeRule(testID, redeployedAlertSummary)
		Expect(err).ToNot(HaveOccurred(), "Failed to create or update the spoke alert probe after redeployment")

		By("checking the spoke reattached and hub management resumed")
		Eventually(func() (bool, error) {
			return redeployACMSpokeMetricExists(metricsAPI, RANConfig.Spoke1Name)
		}).WithTimeout(15*time.Minute).WithPolling(30*time.Second).Should(BeTrue(),
			"ACM did not resume receiving spoke metrics")
		var redeployedAlertSummaries []string
		Eventually(func() (bool, error) {
			updated, summaries, err := redeployACMSpokeAlertHasSummary(alertsAPI, testID, redeployedAlertSummary)
			redeployedAlertSummaries = summaries

			return updated, err
		}).WithTimeout(10*time.Minute).WithPolling(15*time.Second).Should(BeTrue(),
			"ACM did not receive the spoke alert update after redeployment; probe test ID %s, expected summary %q, observed summaries: %v",
			testID, redeployedAlertSummary, redeployedAlertSummaries)

		redeployedVersion, err := version.GetOCPVersion(Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Spoke API is unavailable after redeployment")
		Expect(redeployedVersion).To(Equal(originalVersion), "Spoke version changed after redeployment")
		redeployedNodeCount, err := readySpokeNodeCount()
		Expect(err).ToNot(HaveOccurred(), "Spoke nodes are not Ready after redeployment")
		Expect(redeployedNodeCount).To(Equal(originalNodeCount), "Spoke node count changed after redeployment")

		clusterDeployment, err = hive.PullClusterDeployment(HubAPIClient, RANConfig.Spoke1Name, RANConfig.Spoke1Name)
		Expect(err).ToNot(HaveOccurred(), "Failed to find ClusterDeployment after redeployment")
		redeployedClusterID := clusterDeployment.Object.Spec.ClusterMetadata.ClusterID
		redeployedInfraID := clusterDeployment.Object.Spec.ClusterMetadata.InfraID
		Expect(redeployedClusterID).ToNot(BeEmpty(), "ClusterDeployment has no cluster ID after redeployment")
		Expect(redeployedInfraID).ToNot(BeEmpty(), "ClusterDeployment has no infra ID after redeployment")
		GinkgoWriter.Printf(
			"CNF-24973 ClusterDeployment identity before/after redeployment: ClusterID %s -> %s, InfraID %s -> %s\n",
			originalClusterID, redeployedClusterID, originalInfraID, redeployedInfraID)

		By("confirming the redeploy produced a freshly installed cluster")
		Expect(redeployedClusterID).ToNot(Equal(originalClusterID),
			"ClusterDeployment cluster ID is unchanged; the spoke was not reinstalled")
		Expect(redeployedInfraID).ToNot(Equal(originalInfraID),
			"ClusterDeployment infra ID is unchanged; the spoke was not reinstalled")

		By("restoring the clusters app automated sync policy")
		if autoSyncPaused {
			Expect(setClustersSyncPolicy(originalSyncPolicy)).To(Succeed(),
				"Failed to restore clusters app auto-sync")
			autoSyncPaused = false
		}
	})
})
