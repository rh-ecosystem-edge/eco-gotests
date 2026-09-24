package tests

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/querier"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/metrics"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/profiles"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/ptpdaemon"
	ptpleap "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/ptpleap"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
)

var _ = Describe("PTP Leap File", Label(tsparams.LabelLeapFile), func() {
	var (
		prometheusAPI      prometheusv1.API
		leapConfigMap      *configmap.Builder
		err                error
		nodeName           string
		testRanAtLeastOnce = false
	)

	BeforeEach(func() {
		By("creating a Prometheus API client")

		prometheusAPI, err = querier.CreatePrometheusAPIForCluster(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to create Prometheus API client")

		By("ensuring clocks are locked before testing")

		err = metrics.EnsureClocksAreLocked(prometheusAPI)
		Expect(err).ToNot(HaveOccurred(), "Failed to assert clock state is locked")
	})

	AfterEach(func() {
		if !testRanAtLeastOnce {
			Skip("Test did not run at least once. Skipping cleanup.")
		}

		By("restoring the original leap configmap")

		leapConfigMap, err = configmap.Pull(
			RANConfig.Spoke1APIClient, tsparams.LeapConfigmapName, ranparam.PtpOperatorNamespace)
		Expect(err).ToNot(HaveOccurred(), "Failed to pull original leap configmap")

		leapConfigMap.Definition.Data = map[string]string{}
		_, err = leapConfigMap.Update()
		Expect(err).ToNot(HaveOccurred(), "Failed to update original leap configmap")

		ptpDaemonPod, err := ptpdaemon.GetPtpDaemonPodOnNode(RANConfig.Spoke1APIClient, nodeName)
		Expect(err).ToNot(HaveOccurred(), "Failed to list PTP daemon set pods")
		_, err = ptpDaemonPod.DeleteAndWait(5 * time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Failed to delete PTP daemon set pod")

		By("ensuring clocks are locked after testing")

		err = metrics.AssertQuery(context.TODO(), prometheusAPI, metrics.ClockStateQuery{}, metrics.ClockStateLocked,
			metrics.AssertWithStableDuration(10*time.Second),
			metrics.AssertWithTimeout(5*time.Minute))
		Expect(err).ToNot(HaveOccurred(), "Failed to assert clock state is locked")
	})

	It("should add leap event announcement in leap configmap when removing the last announcement",
		reportxml.ID("75325"), func() {
			By("pulling leap configmap")

			leapConfigMap, err = configmap.Pull(
				RANConfig.Spoke1APIClient, tsparams.LeapConfigmapName, ranparam.PtpOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to pull leap configmap")

			nodeInfoMap, err := profiles.GetNodeInfoMap(RANConfig.Spoke1APIClient)
			Expect(err).ToNot(HaveOccurred(), "Failed to get node info map")

			for _, nodeInfo := range nodeInfoMap {
				if nodeInfo.Counts[profiles.ProfileTypeMultiNICGM] == 0 &&
					nodeInfo.Counts[profiles.ProfileTypeGM] == 0 {
					continue
				}

				testRanAtLeastOnce = true
				nodeName = nodeInfo.Name

				By(fmt.Sprintf("removing the last leap announcement from the leap configmap for node %s", nodeName))
				withoutLastLeapAnnouncementData := ptpleap.RemoveLastLeapAnnouncement(leapConfigMap.Object.Data[nodeName])
				strippedLastAnnouncement, err := ptpleap.GetLastAnnouncement(withoutLastLeapAnnouncementData)
				Expect(err).ToNot(HaveOccurred(), "Failed to get last announcement after strip")

				leapConfigMap.Definition.Data[nodeName] = withoutLastLeapAnnouncementData
				_, err = leapConfigMap.Update()
				Expect(err).ToNot(HaveOccurred(), "Failed to update original leap configmap")

				By("deleting the PTP daemon pod for node " + nodeName)
				ptpDaemonPod, err := ptpdaemon.GetPtpDaemonPodOnNode(RANConfig.Spoke1APIClient, nodeName)
				Expect(err).ToNot(HaveOccurred(), "Failed to get PTP daemon pod for node %s", nodeName)
				_, err = ptpDaemonPod.Delete()
				Expect(err).ToNot(HaveOccurred(), "Failed to delete PTP daemon pod for node %s", nodeName)

				By("validating the PTP daemon pod is running on node " + nodeName)
				err = ptpdaemon.ValidatePtpDaemonPodRunning(RANConfig.Spoke1APIClient, nodeName)
				Expect(err).ToNot(HaveOccurred(), "Failed to validate PTP daemon pod running on node %s", nodeName)

				By("waiting for configmap to be updated after PTP daemon restart")

				err = ptpleap.WaitForConfigmapToBeUpdated(nodeName, withoutLastLeapAnnouncementData, 5*time.Second, 10*time.Minute)
				Expect(err).ToNot(HaveOccurred(), "Failed to wait for configmap to be updated")

				By("validating the new last announcement date is newer than after strip and not in the future")

				newLeapConfigMap, err := configmap.Pull(
					RANConfig.Spoke1APIClient, tsparams.LeapConfigmapName, ranparam.PtpOperatorNamespace)
				Expect(err).ToNot(HaveOccurred(), "Failed to pull leap configmap")

				newLastAnnouncement, err := ptpleap.GetLastAnnouncement(newLeapConfigMap.Object.Data[nodeName])
				Expect(err).ToNot(HaveOccurred(), "Failed to get last announcement")

				newAnnouncementDate, err := ptpleap.ParseAnnouncementDate(newLastAnnouncement)
				Expect(err).ToNot(HaveOccurred(), "Failed to parse new last announcement date")

				strippedAnnouncementDate, err := ptpleap.ParseAnnouncementDate(strippedLastAnnouncement)
				Expect(err).ToNot(HaveOccurred(), "Failed to parse stripped last announcement date")

				todayUTC := time.Now().UTC().Truncate(24 * time.Hour)
				Expect(newAnnouncementDate.After(strippedAnnouncementDate)).To(BeTrue(),
					"New last announcement %q should be after stripped last announcement %q",
					newLastAnnouncement, strippedLastAnnouncement)
				Expect(newAnnouncementDate.After(todayUTC)).To(BeFalse(),
					"New last announcement date should not be in the future")
			}

			if !testRanAtLeastOnce {
				Skip("Could not find any node to run the test on")
			}
		})
})
