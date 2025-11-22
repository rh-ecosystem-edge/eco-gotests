package tests

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	eventptp "github.com/redhat-cne/sdk-go/pkg/event/ptp"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/querier"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/consumer"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/events"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/iface"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/metrics"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/profiles"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	"k8s.io/klog/v2"
)

var _ = Describe("PTP Interfaces", Label(tsparams.LabelInterfaces), func() {
	var prometheusAPI prometheusv1.API

	BeforeEach(func() {
		By("creating a Prometheus API client")
		var err error
		prometheusAPI, err = querier.CreatePrometheusAPIForCluster(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to create Prometheus API client")

		By("ensuring clocks are locked before testing")
		err = metrics.AssertQuery(context.TODO(), prometheusAPI, metrics.ClockStateQuery{}, metrics.ClockStateLocked,
			metrics.AssertWithStableDuration(10*time.Second),
			metrics.AssertWithTimeout(5*time.Minute))
		Expect(err).ToNot(HaveOccurred(), "Failed to assert clock state is locked")
	})

	AfterEach(func() {
		By("ensuring clocks are locked after testing")
		err := metrics.AssertQuery(context.TODO(), prometheusAPI, metrics.ClockStateQuery{}, metrics.ClockStateLocked,
			metrics.AssertWithStableDuration(10*time.Second),
			metrics.AssertWithTimeout(5*time.Minute))
		Expect(err).ToNot(HaveOccurred(), "Failed to assert clock state is locked")
	})

	// 49742 - Validating events when slave interface goes down and up
	It("should generate events when slave interface goes down and up", reportxml.ID("49742"), func() {
		testActuallyRan := false

		By("getting node info map")
		nodeInfoMap, err := profiles.GetNodeInfoMap(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to get node info map")

		for nodeName, nodeInfo := range nodeInfoMap {
			By("getting receiver interfaces for node " + nodeName)
			receiverInterfaces := nodeInfo.GetInterfacesByClockType(profiles.ClockTypeClient)
			if len(receiverInterfaces) == 0 {
				continue
			}

			klog.V(tsparams.LogLevel).Infof("Receiver interfaces for node %s: %v",
				nodeName, profiles.GetInterfacesNames(receiverInterfaces))

			By("getting the egress interface for the node")
			egressInterface, err := iface.GetEgressInterfaceName(RANConfig.Spoke1APIClient, nodeName)
			Expect(err).ToNot(HaveOccurred(), "Failed to get egress interface name for node %s", nodeName)

			By("grouping the receiver interfaces")
			interfaceGroups := iface.GroupInterfacesByNIC(profiles.GetInterfacesNames(receiverInterfaces))

			for nicName, interfaceGroup := range interfaceGroups {
				// Especially for SNO, bringing down the egress interface will break the test, so we skip
				// this NIC.
				if nicName == egressInterface.GetNIC() {
					klog.V(tsparams.LogLevel).Infof("Skipping test for egress interface %s", nicName)

					continue
				}

				testActuallyRan = true

				By("getting the event pod for the node")
				eventPod, err := consumer.GetConsumerPodforNode(RANConfig.Spoke1APIClient, nodeName)
				Expect(err).ToNot(HaveOccurred(), "Failed to get event pod for node %s", nodeName)

				// DeferCleanup will create a pseudo-AfterEach to run after the test completes, even if
				// it fails. This ensures these interfaces are set up even if the test fails.
				DeferCleanup(func() {
					By("ensuring all interfaces are set up even if the test fails")
					var errs []error

					for _, ifaceName := range interfaceGroup {
						err := iface.SetInterfaceStatus(RANConfig.Spoke1APIClient, nodeName, ifaceName, iface.InterfaceStateUp)
						if err != nil {
							klog.V(tsparams.LogLevel).Infof("Failed to set interface %s to up on node %s: %v", ifaceName, nodeName, err)

							errs = append(errs, err)
						}
					}

					Expect(errs).To(BeEmpty(), "Failed to set some interfaces to up on node %s", nodeName)
				})

				startTime := time.Now()

				By("setting all interfaces in the group to be down")
				for _, ifaceName := range interfaceGroup {
					err := iface.SetInterfaceStatus(RANConfig.Spoke1APIClient, nodeName, ifaceName, iface.InterfaceStateDown)
					Expect(err).ToNot(HaveOccurred(), "Failed to set interface %s to down on node %s", ifaceName, nodeName)
				}

				By("waiting for ptp state change HOLDOVER event")
				holdoverFilter := events.All(
					events.IsType(eventptp.PtpStateChange),
					events.HasValue(events.WithSyncState(eventptp.HOLDOVER), events.OnInterface(nicName)),
				)
				err = events.WaitForEvent(eventPod, startTime, 3*time.Minute, holdoverFilter)
				Expect(err).ToNot(HaveOccurred(), "Failed to wait for ptp state change HOLDOVER event")

				By("waiting for ptp state change FREERUN event")
				freerunFilter := events.All(
					events.IsType(eventptp.PtpStateChange),
					events.HasValue(events.WithSyncState(eventptp.FREERUN), events.OnInterface(nicName)),
				)
				err = events.WaitForEvent(eventPod, startTime, 5*time.Minute, freerunFilter)
				Expect(err).ToNot(HaveOccurred(), "Failed to wait for ptp state change FREERUN event")

				By("asserting that interface group on that node has FREERUN metric")
				clockStateQuery := metrics.ClockStateQuery{
					Interface: metrics.Equals(nicName),
					Node:      metrics.Equals(nodeName),
				}
				err = metrics.AssertQuery(context.TODO(), prometheusAPI, clockStateQuery, metrics.ClockStateFreerun,
					metrics.AssertWithTimeout(5*time.Minute),
					metrics.AssertWithStableDuration(10*time.Second))
				Expect(err).ToNot(HaveOccurred(), "Failed to assert that interface group on that node has FREERUN metric")

				By("setting all interfaces in the group up")
				for _, ifaceName := range interfaceGroup {
					err := iface.SetInterfaceStatus(RANConfig.Spoke1APIClient, nodeName, ifaceName, iface.InterfaceStateUp)
					Expect(err).ToNot(HaveOccurred(), "Failed to set interface %s to up on node %s", ifaceName, nodeName)
				}

				By("waiting for ptp state change LOCKED event")
				lockedFilter := events.All(
					events.IsType(eventptp.PtpStateChange),
					events.HasValue(events.WithSyncState(eventptp.LOCKED), events.OnInterface(nicName)),
				)
				err = events.WaitForEvent(eventPod, startTime, 5*time.Minute, lockedFilter)
				Expect(err).ToNot(HaveOccurred(), "Failed to wait for ptp state change LOCKED event")

				By("asserting that all metrics are LOCKED")
				err = metrics.AssertQuery(context.TODO(), prometheusAPI, metrics.ClockStateQuery{}, metrics.ClockStateLocked,
					metrics.AssertWithStableDuration(10*time.Second),
					metrics.AssertWithTimeout(5*time.Minute))
				Expect(err).ToNot(HaveOccurred(), "Failed to assert that all metrics are LOCKED")
			}
		}

		if !testActuallyRan {
			Skip("Could not find any interfaces to test")
		}
	})
})
