package tests

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/querier"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/metrics"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/profiles"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	"k8s.io/apimachinery/pkg/util/wait"
)

var _ = Describe("PTP Leap File", Label(tsparams.LabelLeapFile), func() {
	var prometheusAPI prometheusv1.API
	var leapConfigMap *configmap.Builder
	var err error

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
		By("restoring the original leap configmap")
		leapConfigMap, err = configmap.Pull(
			RANConfig.Spoke1APIClient, tsparams.LeapConfigmapName, ranparam.PtpOperatorNamespace)
		Expect(err).ToNot(HaveOccurred(), "Failed to pull original leap configmap")

		leapConfigMap.Definition.Data = map[string]string{}
		_, err = leapConfigMap.Update()
		Expect(err).ToNot(HaveOccurred(), "Failed to update original leap configmap")

		listPtpDaemonsetPods, err := pod.List(RANConfig.Spoke1APIClient, ranparam.PtpOperatorNamespace)
		Expect(err).ToNot(HaveOccurred(), "Failed to list PTP daemon set pods")
		for _, pod := range listPtpDaemonsetPods {
			_, err = pod.DeleteAndWait(5 * time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to delete PTP daemon set pod")
		}

		prometheusAPI, err = querier.CreatePrometheusAPIForCluster(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to create Prometheus API client")

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
				By(fmt.Sprintf("removing the last leap announcement from the leap configmap for node %s", nodeInfo.Name))
				withoutLastLeapAnnouncementData := removeLastLeapAnnouncement(leapConfigMap.Object.Data[nodeInfo.Name])
				_, err := leapConfigMap.WithData(
					map[string]string{nodeInfo.Name: withoutLastLeapAnnouncementData}).Update()
				Expect(err).ToNot(HaveOccurred(), "Failed to update original leap configmap")
			}

			By("deleting all linuxptp-daemon pods")
			listPtpDaemonsetPods, err := pod.List(RANConfig.Spoke1APIClient, ranparam.PtpOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to list PTP daemon set pods")
			for _, pod := range listPtpDaemonsetPods {
				_, err = pod.Delete()
				Expect(err).ToNot(HaveOccurred(), "Failed to delete PTP daemon set pod")
			}

			prometheusAPI, err = querier.CreatePrometheusAPIForCluster(RANConfig.Spoke1APIClient)
			Expect(err).ToNot(HaveOccurred(), "Failed to create Prometheus API client")

			By("waiting for all linuxptp-daemon pods on nodes to be healthy")
			err = pod.WaitForPodsInNamespacesHealthy(
				RANConfig.Spoke1APIClient, []string{ranparam.PtpOperatorNamespace}, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for all PTP daemon set pods to be healthy")

			leapConfigMap, err := configmap.Pull(
				RANConfig.Spoke1APIClient, tsparams.LeapConfigmapName, ranparam.PtpOperatorNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to pull leap configmap")

			By("waiting for configmap to be updated with today's date leap announcement")
			newLeapConfigMap, err := waitForConfigmapToBeUpdated(leapConfigMap)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for configmap to be updated")

			By("ensuring new last announcement is different from the original last announcement")
			for _, nodeInfo := range nodeInfoMap {
				lastAnnouncement, err := getLastAnnouncement(leapConfigMap.Object.Data[nodeInfo.Name])
				Expect(err).ToNot(HaveOccurred(), "Failed to get last announcement")
				newLastAnnouncement, err := getLastAnnouncement(newLeapConfigMap.Object.Data[nodeInfo.Name])
				Expect(err).ToNot(HaveOccurred(), "Failed to get last announcement")
				Expect(newLastAnnouncement).NotTo(Equal(lastAnnouncement), "Last announcement should be different")
			}
		})
})

// RemoveLastLeapAnnouncement removes the last "leap announcement" line,
// i.e., the last line that looks like: "<seconds> <offset> # <date>".
func removeLastLeapAnnouncement(s string) string {
	lines := strings.Split(s, "\n")
	leapLine := regexp.MustCompile(`^\s*\d+\s+\d+\s+#`)

	for i := len(lines) - 1; i >= 0; i-- {
		if leapLine.MatchString(lines[i]) {
			// Remove the matched line
			lines = append(lines[:i], lines[i+1:]...)

			break
		}
	}

	return strings.Join(lines, "\n")
}

// waitForConfigmapToBeUpdated waits until the configmap is updated with the last leap announcement line
// that matches today's date in UTC, formatted "d Mon yyyy".
func waitForConfigmapToBeUpdated(leapConfigMap *configmap.Builder) (*configmap.Builder, error) {
	var err error

	interval := 5 * time.Second
	timeout := 10 * time.Minute

	return leapConfigMap, wait.PollUntilContextTimeout(
		context.TODO(), interval, timeout, true, func(ctx context.Context) (bool, error) {
			today := time.Now().UTC().Format("2 Jan 2006")
			leapConfigMap, err = configmap.Pull(
				RANConfig.Spoke1APIClient, tsparams.LeapConfigmapName, ranparam.PtpOperatorNamespace)

			if err != nil {
				return false, nil
			}

			for _, leapConfigmapData := range leapConfigMap.Object.Data {
				if strings.Contains(leapConfigmapData, today) {
					return true, nil
				}
			}

			return false, nil
		})
}

// getLastAnnouncement returns the last leap event announcement from a leap-configmap Data.
func getLastAnnouncement(leapConfigMapData string) (string, error) {
	if len(leapConfigMapData) == 0 {
		return leapConfigMapData, nil
	}

	announcementPattern := regexp.MustCompile(`\n(\d+\s+\d+\s+#\s\d+\s[a-zA-Z]+\s\d{4})\n\n`)
	announcementSlice := announcementPattern.FindStringSubmatch(leapConfigMapData)

	if len(announcementSlice) < 2 {
		return "", fmt.Errorf("error finding the last announcement")
	}

	return announcementSlice[1], nil
}
