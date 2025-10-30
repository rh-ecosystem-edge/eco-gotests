package tests

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	eventptp "github.com/redhat-cne/sdk-go/pkg/event/ptp"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/querier"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/rancluster"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/consumer"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/events"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/iface"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/metrics"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
)

// This test must be ordered so that we only need to reboot the node once. It may continue on failure since the test
// cases are not necessarily dependent on each other.
var _ = Describe("PTP Node Reboot", Ordered, ContinueOnFailure, Label(tsparams.LabelNodeReboot), func() {
	var (
		nodeName   string
		rebootTime time.Time
	)

	BeforeAll(func() {
		By("checking if the cluster is SNO")
		isSNO, err := rancluster.IsSNO(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to check if the cluster is SNO")

		By("selecting a node to reboot")
		// list all the ptp daemon set pods, select the first, then use spec.nodeName to get the node name
		ptpDaemonPods, err := pod.List(RANConfig.Spoke1APIClient, ranparam.PtpOperatorNamespace, metav1.ListOptions{
			LabelSelector: ranparam.PtpDaemonsetLabelSelector,
		})
		Expect(err).ToNot(HaveOccurred(), "Failed to list PTP daemon set pods")
		Expect(ptpDaemonPods).ToNot(BeEmpty(), "No PTP daemon set pods found")

		nodeName = ptpDaemonPods[0].Definition.Spec.NodeName
		rebootTime = time.Now()

		By("soft rebooting the node")
		// Even though we do not care about the output, we need to use ExecCmdWithStdoutWithRetries to get
		// access to the node list options directly.
		_, err = cluster.ExecCmdWithStdoutWithRetries(
			RANConfig.Spoke1APIClient, 3, 10*time.Second, "sudo systemctl reboot",
			metav1.ListOptions{FieldSelector: fields.OneTermEqualSelector("metadata.name", nodeName).String()},
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to soft reboot the node")

		if isSNO {
			By("waiting for the SNO node to recover")
			err = cluster.WaitForRecover(RANConfig.Spoke1APIClient, []string{}, 45*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for the node to recover")
		} else {
			By("waiting for the node to recover")
			// If the cluster is not SNO, the trick of waiting for the cluster to be reachable will not
			// work, so we instead wait for the node to transition to not ready and then back to ready.
			rebootedNode, err := nodes.Pull(RANConfig.Spoke1APIClient, nodeName)
			Expect(err).ToNot(HaveOccurred(), "Failed to pull the node")

			err = rebootedNode.WaitUntilNotReady(15 * time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for the node to become not ready")

			err = rebootedNode.WaitUntilReady(30 * time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for the node to become ready")

			By("waiting for all pods on rebooted node to be healthy")
			err = pod.WaitForPodsInNamespacesHealthy(RANConfig.Spoke1APIClient, nil, 10*time.Minute, metav1.ListOptions{
				FieldSelector: fields.OneTermEqualSelector("spec.nodeName", nodeName).String(),
			})
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for all pods on rebooted node to be healthy")
		}
	})

	// 59858 - verify the system returns to stability after reboot node
	It("should return to same stable status after ptp node soft reboot", reportxml.ID("59858"), func() {
		By("waiting for all clocks to be locked")
		prometheusAPI, err := querier.CreatePrometheusAPIForCluster(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to create Prometheus API client")

		query := metrics.ClockStateQuery{Node: metrics.Equals(nodeName)}
		err = metrics.AssertQuery(context.TODO(), prometheusAPI, query, metrics.ClockStateLocked,
			metrics.AssertWithStableDuration(10*time.Second),
			metrics.AssertWithTimeout(10*time.Minute))
		Expect(err).ToNot(HaveOccurred(), "Failed to assert clock state is locked after 5 minutes")
	})

	// 59995 - Validates PTP consumer events after ptp node reboot
	It("validates PTP consumer events after ptp node reboot", reportxml.ID("59995"), func() {
		By("getting the event pod for the node " + nodeName)
		eventPod, err := consumer.GetConsumerPodforNode(RANConfig.Spoke1APIClient, nodeName)
		Expect(err).ToNot(HaveOccurred(), "Failed to get event pod for node %s", nodeName)

		By("waiting for the LOCKED event to be reported")
		filter := events.All(
			events.IsType(eventptp.PtpStateChange),
			events.HasValue(events.WithSyncState(eventptp.LOCKED), events.ContainingResource(string(iface.Master))),
		)
		err = events.WaitForEvent(eventPod, rebootTime, 5*time.Minute, filter)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait for locked event on node %s", nodeName)
	})
})
