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
	var prometheusAPI prometheusv1.API
	var leapConfigMap *configmap.Builder
	var err error
	var nodeName string
	var testRanAtLeastOnce = false

	BeforeEach(func() {
		By("creating a Prometheus API client")
		prometheusAPI, err = querier.CreatePrometheusAPIForCluster(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to create Prometheus API client")

		By("ensuring clocks are locked before testing")
		err = metrics.AssertQuery(context.TODO(), prometheusAPI, metrics.ClockStateQuery{}, metrics.ClockStateLocked,
			metrics.AssertWithStableDuration(10*time.Second),
			metrics.AssertWithTimeout(5*time.Minute))
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
				if len(nodeInfo.GetProfilesByType(profiles.ProfileTypeMultiNICGM)) == 0 &&
					len(nodeInfo.GetProfilesByType(profiles.ProfileTypeGM)) == 0 {
					continue
				}
				testRanAtLeastOnce = true
				nodeName = nodeInfo.Name
				originalLeapConfigMapData := leapConfigMap.Object.Data[nodeName]

				By(fmt.Sprintf("removing the last leap announcement from the leap configmap for node %s", nodeName))
				withoutLastLeapAnnouncementData := ptpleap.RemoveLastLeapAnnouncement(leapConfigMap.Object.Data[nodeName])
				leapConfigMap.Definition.Data[nodeName] = withoutLastLeapAnnouncementData
				_, err := leapConfigMap.Update()
				Expect(err).ToNot(HaveOccurred(), "Failed to update original leap configmap")

				By("deleting the PTP daemon pod for node " + nodeName)
				ptpDaemonPod, err := ptpdaemon.GetPtpDaemonPodOnNode(RANConfig.Spoke1APIClient, nodeName)
				Expect(err).ToNot(HaveOccurred(), "Failed to get PTP daemon pod for node %s", nodeName)
				_, err = ptpDaemonPod.Delete()
				Expect(err).ToNot(HaveOccurred(), "Failed to delete PTP daemon pod for node %s", nodeName)

				By("validating the PTP daemon pod is running on node " + nodeName)
				err = ptpdaemon.ValidatePtpDaemonPodRunning(RANConfig.Spoke1APIClient, nodeName)
				Expect(err).ToNot(HaveOccurred(), "Failed to validate PTP daemon pod running on node %s", nodeName)

				By("waiting for configmap to be updated with today's date leap announcement")
				err = ptpleap.WaitForConfigmapToBeUpdated(5*time.Second, 10*time.Minute)
				Expect(err).ToNot(HaveOccurred(), "Failed to wait for configmap to be updated")

				By("ensuring new last announcement is different from the original last announcement")
				originalLastAnnouncement, err := ptpleap.GetLastAnnouncement(originalLeapConfigMapData)
				Expect(err).ToNot(HaveOccurred(), "Failed to get last announcement")
				newLeapConfigMap, err := configmap.Pull(
					RANConfig.Spoke1APIClient, tsparams.LeapConfigmapName, ranparam.PtpOperatorNamespace)
				Expect(err).ToNot(HaveOccurred(), "Failed to pull leap configmap")

				newLastAnnouncement, err := ptpleap.GetLastAnnouncement(newLeapConfigMap.Object.Data[nodeName])
				Expect(err).ToNot(HaveOccurred(), "Failed to get last announcement")
				Expect(newLastAnnouncement).NotTo(Equal(originalLastAnnouncement), "Last announcement should be different")

			}

			if !testRanAtLeastOnce {
				Skip("Could not find any node to run the test on")
			}
		})
})
