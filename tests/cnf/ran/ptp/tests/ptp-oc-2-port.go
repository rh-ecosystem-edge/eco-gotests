package tests

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	eventptp "github.com/redhat-cne/sdk-go/pkg/event/ptp"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/querier"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/version"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/consumer"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/events"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/iface"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/metrics"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/profiles"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	"k8s.io/klog/v2"
)

var _ = Describe("PTP OC 2-port", Label(tsparams.LabelOC2Port, tsparams.LabelInterfaces), func() {
	var prometheusAPI prometheusv1.API

	BeforeEach(func() {
		var err error

		By("creating a Prometheus API client")
		prometheusAPI, err = querier.CreatePrometheusAPIForCluster(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to create Prometheus API client")

		By("checking if PTP operator version supports OC 2-port tests")
		inRange, err := version.IsVersionStringInRange(
			RANConfig.Spoke1OperatorVersions[ranparam.PTP], "4.18", "")
		Expect(err).ToNot(HaveOccurred(), "Failed to parse PTP operator version")
		if !inRange {
			Skip("Test is valid from version 4.18")
		}
	})

	// 80963 - Verifies 2-port OC HA failover when active port goes down
	It("verifies 2-port oc ha failover when active port goes down", reportxml.ID("80963"), func() {
		testActuallyRan := false

		By("getting node info map")
		nodeInfoMap, err := profiles.GetNodeInfoMap(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to get node info map")

		for nodeName, nodeInfo := range nodeInfoMap {
			By("checking if node has OC 2-port configuration")
			oc2PortProfiles := nodeInfo.GetProfilesByTypes(profiles.ProfileTypeTwoPortOC)
			if len(oc2PortProfiles) == 0 {
				klog.V(tsparams.LogLevel).Infof("Node %s has no OC 2-port configuration, skipping",
					nodeName)

				continue
			}

			testActuallyRan = true

			oc2PortInfo := getOc2PortInfo(nodeName, oc2PortProfiles, prometheusAPI)

			DeferCleanup(func() {
				if !CurrentSpecReport().Failed() {
					return
				}

				By("Restoring OC 2-port interfaces")
				restoreOc2PortAndValidate(context.TODO(), prometheusAPI, nodeName, oc2PortInfo.Interfaces)

			})
			By("getting event consumer pod for the node")
			eventPod, err := consumer.GetConsumerPodforNode(RANConfig.Spoke1APIClient, nodeName)
			Expect(err).ToNot(HaveOccurred(), "Failed to get event consumer pod for node %s", nodeName)

			startTime := time.Now()

			By("bringing down the active interface to cause a failover")
			err = iface.SetInterfaceStatus(
				RANConfig.Spoke1APIClient, nodeName, oc2PortInfo.ActiveInterface, iface.InterfaceStateDown)
			Expect(err).ToNot(HaveOccurred(),
				"Failed to set interface %s to down on node %s", oc2PortInfo.ActiveInterface, nodeName)

			By("validating PTP clock class metric remains 6 after failover")
			clockClassQuery := metrics.ClockClassQuery{
				Node:    metrics.Equals(nodeName),
				Process: metrics.Equals(metrics.ProcessPTP4L),
			}
			err = metrics.AssertQuery(context.TODO(), prometheusAPI, clockClassQuery, metrics.ClockClass6,
				metrics.AssertWithStableDuration(10*time.Second),
				metrics.AssertWithTimeout(45*time.Second))
			Expect(err).ToNot(HaveOccurred(),
				"Failed to assert that the PTP clock class metric remains 6 after failover")

			By("validating PTP clock state metric remains LOCKED after failover")
			clockStateQuery := metrics.ClockStateQuery{
				Node:    metrics.Equals(nodeName),
				Process: metrics.Includes(metrics.ProcessPTP4L, metrics.ProcessPHC2SYS),
			}
			err = metrics.AssertQuery(context.TODO(), prometheusAPI, clockStateQuery, metrics.ClockStateLocked,
				metrics.AssertWithStableDuration(10*time.Second),
				metrics.AssertWithTimeout(45*time.Second))
			Expect(err).ToNot(HaveOccurred(),
				"Failed to assert that the PTP process metric stays in LOCKED state after failover")

			By("validating PTP initial active interface role metric change to FAULTY after failover")
			interfaceRoleQuery := metrics.InterfaceRoleQuery{
				Interface: metrics.Equals(oc2PortInfo.ActiveInterface),
				Node:      metrics.Equals(nodeName),
				Process:   metrics.Equals(metrics.ProcessPTP4L),
			}
			err = metrics.AssertQuery(context.TODO(), prometheusAPI, interfaceRoleQuery, metrics.InterfaceRoleFaulty,
				metrics.AssertWithTimeout(45*time.Second))
			Expect(err).ToNot(HaveOccurred(),
				"Failed to assert that the PTP active interface role metric changed to FAULTY after failover")

			By("validating PTP passive interface role metric changed to SLAVE after failover")
			interfaceRoleQuery = metrics.InterfaceRoleQuery{
				Interface: metrics.Equals(oc2PortInfo.PassiveInterface),
				Node:      metrics.Equals(nodeName),
				Process:   metrics.Equals(metrics.ProcessPTP4L),
			}
			err = metrics.AssertQuery(context.TODO(), prometheusAPI, interfaceRoleQuery, metrics.InterfaceRoleFollower,
				metrics.AssertWithTimeout(45*time.Second))
			Expect(err).ToNot(HaveOccurred(),
				"Failed to assert that the PTP passive interface role metric changed to SLAVE after failover")

			By("validating no FREERUN event is generated after failover")
			freerunFilter := events.All(
				events.IsType(eventptp.PtpStateChange),
				events.HasValue(events.WithSyncState(eventptp.FREERUN), events.OnInterface(oc2PortInfo.IfaceGroup)),
			)
			err = events.WaitForEvent(eventPod, startTime, 1*time.Minute, freerunFilter)
			Expect(err).To(HaveOccurred(),
				"Unexpected FREERUN event detected for interface %s", oc2PortInfo.ActiveInterface)

			By("restoring OC 2-port interfaces before test completion")
			restoreOc2PortAndValidate(context.TODO(), prometheusAPI, nodeName, oc2PortInfo.Interfaces)
		}

		if !testActuallyRan {
			Skip("Could not find any OC 2-port configuration to test")
		}
	})

	// 80964 - Verifies 2-port OC HA holdover & freerun when both ports go down
	It("verifies 2-port oc ha holdover & freerun when both ports go down", reportxml.ID("80964"), func() {
		testActuallyRan := false

		By("getting node info map")
		nodeInfoMap, err := profiles.GetNodeInfoMap(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to get node info map")

		for nodeName, nodeInfo := range nodeInfoMap {
			By("checking if node has OC 2-port configuration")
			oc2PortProfiles := nodeInfo.GetProfilesByTypes(profiles.ProfileTypeTwoPortOC)
			if len(oc2PortProfiles) == 0 {
				klog.V(tsparams.LogLevel).Infof("Node %s has no OC 2-port configuration, skipping",
					nodeName)

				continue
			}

			testActuallyRan = true

			oc2PortInfo := getOc2PortInfo(nodeName, oc2PortProfiles, prometheusAPI)

			DeferCleanup(func() {
				if !CurrentSpecReport().Failed() {
					return
				}

				By("restoring OC 2-port interfaces")
				restoreOc2PortAndValidate(context.TODO(), prometheusAPI, nodeName, oc2PortInfo.Interfaces)
			})

			By("getting event consumer pod for the node")
			eventPod, err := consumer.GetConsumerPodforNode(RANConfig.Spoke1APIClient, nodeName)
			Expect(err).ToNot(HaveOccurred(), "Failed to get event consumer pod for node %s", nodeName)

			startTime := time.Now()

			By("bringing down both interfaces")
			for _, ifaceName := range []iface.Name{
				oc2PortInfo.ActiveInterface,
				oc2PortInfo.PassiveInterface,
			} {
				err = iface.SetInterfaceStatus(
					RANConfig.Spoke1APIClient, nodeName, ifaceName, iface.InterfaceStateDown)
				Expect(err).ToNot(HaveOccurred(),
					"Failed to set interface %s to down on node %s", ifaceName, nodeName)
			}

			By("validating both interfaces are FAULTY")
			for _, ifaceName := range []iface.Name{
				oc2PortInfo.ActiveInterface,
				oc2PortInfo.PassiveInterface,
			} {
				interfaceRoleQuery := metrics.InterfaceRoleQuery{
					Interface: metrics.Equals(ifaceName),
					Node:      metrics.Equals(nodeName),
					Process:   metrics.Equals(metrics.ProcessPTP4L),
				}
				err = metrics.AssertQuery(context.TODO(), prometheusAPI, interfaceRoleQuery, metrics.InterfaceRoleFaulty,
					metrics.AssertWithStableDuration(10*time.Second),
					metrics.AssertWithTimeout(1*time.Minute))
				Expect(err).ToNot(HaveOccurred(), "Failed to assert interface %s is FAULTY", ifaceName)
			}

			By("validating clock states transition to FREERUN")
			clockStateQuery := metrics.ClockStateQuery{
				Node:    metrics.Equals(nodeName),
				Process: metrics.Includes(metrics.ProcessPTP4L, metrics.ProcessPHC2SYS),
			}
			err = metrics.AssertQuery(context.TODO(), prometheusAPI, clockStateQuery, metrics.ClockStateFreerun,
				metrics.AssertWithStableDuration(10*time.Second),
				metrics.AssertWithTimeout(5*time.Minute))
			Expect(err).ToNot(HaveOccurred(), "Failed to assert clock state is FREERUN")

			By("validating HOLDOVER event is generated")
			holdoverFilter := events.All(
				events.IsType(eventptp.PtpStateChange),
				events.HasValue(events.WithSyncState(eventptp.HOLDOVER), events.OnInterface(oc2PortInfo.IfaceGroup)),
			)
			err = events.WaitForEvent(eventPod, startTime, 1*time.Minute, holdoverFilter)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for HOLDOVER event")

			By("validating FREERUN event is generated")
			freerunFilter := events.All(
				events.IsType(eventptp.PtpStateChange),
				events.HasValue(events.WithSyncState(eventptp.FREERUN), events.OnInterface(oc2PortInfo.IfaceGroup)),
			)
			err = events.WaitForEvent(eventPod, startTime, 1*time.Minute, freerunFilter)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for FREERUN event")

			By("restoring OC 2-port interfaces before test completion")
			restoreOc2PortAndValidate(context.TODO(), prometheusAPI, nodeName, oc2PortInfo.Interfaces)
		}

		if !testActuallyRan {
			Skip("Could not find any OC 2-port configuration to test")
		}
	})

	// 82012 - Verifies 2-port OC HA passive interface recovery when passive port goes down
	It("verifies 2-port oc ha passive interface recovery", reportxml.ID("82012"), func() {
		testActuallyRan := false

		By("getting node info map")
		nodeInfoMap, err := profiles.GetNodeInfoMap(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to get node info map")

		for nodeName, nodeInfo := range nodeInfoMap {
			By("checking if node has OC 2-port configuration")
			oc2PortProfiles := nodeInfo.GetProfilesByTypes(profiles.ProfileTypeTwoPortOC)
			if len(oc2PortProfiles) == 0 {
				klog.V(tsparams.LogLevel).Infof("Node %s has no OC 2-port configuration, skipping",
					nodeName)

				continue
			}

			testActuallyRan = true

			oc2PortInfo := getOc2PortInfo(nodeName, oc2PortProfiles, prometheusAPI)

			DeferCleanup(func() {
				if !CurrentSpecReport().Failed() {
					return
				}

				By("restoring OC 2-port interfaces")
				restoreOc2PortAndValidate(context.TODO(), prometheusAPI, nodeName, oc2PortInfo.Interfaces)
			})

			By("getting event consumer pod for the node")
			eventPod, err := consumer.GetConsumerPodforNode(RANConfig.Spoke1APIClient, nodeName)
			Expect(err).ToNot(HaveOccurred(), "Failed to get event consumer pod for node %s", nodeName)

			startTime := time.Now()

			By("bringing down the passive interface")
			err = iface.SetInterfaceStatus(
				RANConfig.Spoke1APIClient, nodeName, oc2PortInfo.PassiveInterface, iface.InterfaceStateDown)
			Expect(err).ToNot(HaveOccurred(),
				"Failed to set interface %s to down on node %s",
				oc2PortInfo.PassiveInterface, nodeName)

			By("validating clock states remain LOCKED")
			clockStateQuery := metrics.ClockStateQuery{
				Node:    metrics.Equals(nodeName),
				Process: metrics.Includes(metrics.ProcessPTP4L, metrics.ProcessPHC2SYS),
			}
			err = metrics.AssertQuery(context.TODO(), prometheusAPI, clockStateQuery, metrics.ClockStateLocked,
				metrics.AssertWithStableDuration(10*time.Second),
				metrics.AssertWithTimeout(1*time.Minute))
			Expect(err).ToNot(HaveOccurred(), "Failed to assert clock state remains LOCKED")

			By("validating active interface remains FOLLOWER")
			interfaceRoleQuery := metrics.InterfaceRoleQuery{
				Interface: metrics.Equals(oc2PortInfo.ActiveInterface),
				Node:      metrics.Equals(nodeName),
				Process:   metrics.Equals(metrics.ProcessPTP4L),
			}
			err = metrics.AssertQuery(context.TODO(), prometheusAPI, interfaceRoleQuery, metrics.InterfaceRoleFollower,
				metrics.AssertWithTimeout(1*time.Minute))
			Expect(err).ToNot(HaveOccurred(), "Failed to assert active interface remains FOLLOWER")

			By("validating passive interface is FAULTY")
			interfaceRoleQuery = metrics.InterfaceRoleQuery{
				Interface: metrics.Equals(oc2PortInfo.PassiveInterface),
				Node:      metrics.Equals(nodeName),
				Process:   metrics.Equals(metrics.ProcessPTP4L),
			}
			err = metrics.AssertQuery(context.TODO(), prometheusAPI, interfaceRoleQuery, metrics.InterfaceRoleFaulty,
				metrics.AssertWithTimeout(1*time.Minute))
			Expect(err).ToNot(HaveOccurred(), "Failed to assert passive interface is FAULTY")

			By("validating no HOLDOVER event is generated")
			holdoverFilter := events.All(
				events.IsType(eventptp.PtpStateChange),
				events.HasValue(events.WithSyncState(eventptp.HOLDOVER), events.OnInterface(oc2PortInfo.IfaceGroup)),
			)
			err = events.WaitForEvent(eventPod, startTime, 1*time.Minute, holdoverFilter)
			Expect(err).To(HaveOccurred(), "Unexpected HOLDOVER event detected")

			By("restoring OC 2-port interfaces before test completion")
			restoreOc2PortAndValidate(context.TODO(), prometheusAPI, nodeName, oc2PortInfo.Interfaces)
		}

		if !testActuallyRan {
			Skip("Could not find any OC 2-port configuration to test")
		}
	})
})

// getOc2PortInfo validates OC 2-port profile data and derives runtime roles.
func getOc2PortInfo(
	nodeName string,
	oc2PortProfiles []*profiles.ProfileInfo,
	prometheusAPI prometheusv1.API,
) profiles.Oc2PortInfo {
	GinkgoHelper()

	oc2PortProfile := oc2PortProfiles[0]
	By(fmt.Sprintf("Using OC 2-port profile %s on node %s",
		oc2PortProfile.Reference.ProfileName, nodeName))

	oc2PortInterfaces := oc2PortProfile.GetInterfacesByClockType(profiles.ClockTypeClient)
	Expect(len(oc2PortInterfaces)).To(Equal(2),
		"Expected exactly 2 interfaces for OC 2-port profile %s on node %s",
		oc2PortProfile.Reference.ProfileName, nodeName)

	interfaceGroups := iface.GroupInterfacesByNIC(profiles.GetInterfacesNames(oc2PortInterfaces))
	Expect(len(interfaceGroups)).To(Equal(1),
		"Expected to find one interface group for OC 2-port profile %s on node %s",
		oc2PortProfile.Reference.ProfileName, nodeName)

	var oc2PortIfaceGroup iface.NICName

	for nicName := range interfaceGroups {
		oc2PortIfaceGroup = nicName

		break
	}

	var err error

	activeInterface, passiveInterface, err := profiles.Oc2PortDetermineActivePassiveInterfaces(
		context.TODO(), prometheusAPI, nodeName, oc2PortInterfaces)
	Expect(err).ToNot(HaveOccurred(), "Failed to determine active/passive interfaces")

	By(fmt.Sprintf("identified active interface: %s, passive interface: %s",
		activeInterface, passiveInterface))

	return profiles.Oc2PortInfo{
		Interfaces:       oc2PortInterfaces,
		IfaceGroup:       oc2PortIfaceGroup,
		ActiveInterface:  activeInterface,
		PassiveInterface: passiveInterface,
	}
}

// restoreOc2PortAndValidate brings OC 2-port interfaces up and validates clock state and roles.
func restoreOc2PortAndValidate(
	ctx context.Context,
	prometheusAPI prometheusv1.API,
	nodeName string,
	oc2PortInterfaces []*profiles.InterfaceInfo,
) {
	GinkgoHelper()

	By("bringing OC 2-port interfaces back up")

	for _, oc2PortInterface := range oc2PortInterfaces {
		err := iface.SetInterfaceStatus(RANConfig.Spoke1APIClient, nodeName,
			oc2PortInterface.Name, iface.InterfaceStateUp)
		Expect(err).ToNot(HaveOccurred(), "Failed to set interface %s to up on node %s",
			oc2PortInterface.Name, nodeName)
	}

	By("validating OC 2-port clock state returns to LOCKED")

	clockStateQuery := metrics.ClockStateQuery{
		Node:    metrics.Equals(nodeName),
		Process: metrics.Includes(metrics.ProcessPTP4L, metrics.ProcessPHC2SYS),
	}
	err := metrics.AssertQuery(ctx, prometheusAPI, clockStateQuery, metrics.ClockStateLocked,
		metrics.AssertWithStableDuration(10*time.Second),
		metrics.AssertWithTimeout(3*time.Minute))
	Expect(err).ToNot(HaveOccurred(), "Failed to assert clock state is LOCKED after restoration")

	By("validating OC 2-port active/passive roles after restoration")

	waitForOc2PortActivePassive(ctx, prometheusAPI, nodeName, oc2PortInterfaces, 30*time.Second)
}

// waitForOc2PortActivePassive waits for OC 2-port roles to stabilize.
func waitForOc2PortActivePassive(
	ctx context.Context,
	prometheusAPI prometheusv1.API,
	nodeName string,
	oc2PortInterfaces []*profiles.InterfaceInfo,
	timeout time.Duration,
) {
	GinkgoHelper()

	Eventually(func() error {
		_, _, err := profiles.Oc2PortDetermineActivePassiveInterfaces(
			ctx, prometheusAPI, nodeName, oc2PortInterfaces)

		return err
	}, timeout, 5*time.Second).Should(Succeed(),
		"OC 2-port roles should stabilize on node %s", nodeName)
}
