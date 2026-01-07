package tests

import (
	"context"
	"fmt"
	"slices"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	eventptp "github.com/redhat-cne/sdk-go/pkg/event/ptp"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ptp"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/internal/nicinfo"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/querier"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/consumer"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/events"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/iface"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/metrics"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/profiles"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/ptpdaemon"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	"k8s.io/klog/v2"
)

var _ = Describe("PTP Interfaces", Label(tsparams.LabelInterfaces), func() {
	var (
		prometheusAPI   prometheusv1.API
		savedPtpConfigs []*ptp.PtpConfigBuilder
	)

	BeforeEach(func() {
		var err error

		By("creating a Prometheus API client")
		prometheusAPI, err = querier.CreatePrometheusAPIForCluster(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to create Prometheus API client")

		By("ensuring clocks are locked before testing")
		err = metrics.EnsureClocksAreLocked(prometheusAPI)
		Expect(err).ToNot(HaveOccurred(), "Failed to assert clock state is locked")

		By("saving PtpConfigs before testing")
		savedPtpConfigs, err = profiles.SavePtpConfigs(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to save PtpConfigs")
	})

	AfterEach(func() {
		By("restoring PtpConfigs after testing")
		startTime := time.Now()
		changedProfiles, err := profiles.RestorePtpConfigs(RANConfig.Spoke1APIClient, savedPtpConfigs)
		Expect(err).ToNot(HaveOccurred(), "Failed to restore PtpConfigs")

		if len(changedProfiles) > 0 {
			By("waiting for profile load on nodes")
			err := ptpdaemon.WaitForProfileLoadOnPTPNodes(RANConfig.Spoke1APIClient,
				ptpdaemon.WithStartTime(startTime),
				ptpdaemon.WithTimeout(5*time.Minute))
			if err != nil {
				// Timeouts may occur if the profiles changed do not apply to all PTP nodes, so we make
				// this non-fatal. This only happens in certain scenarios in MNO clusters.
				klog.V(tsparams.LogLevel).Infof("Failed to wait for profile load on PTP nodes: %v", err)
			}
		}

		By("ensuring clocks are locked after testing")
		err = metrics.EnsureClocksAreLocked(prometheusAPI)
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

				// Include all interfaces in the interface group in the interface information report for this suite.
				nicinfo.Node(nodeName).MarkTested(iface.NamesToStrings(interfaceGroup)...)

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
				err = metrics.EnsureClocksAreLocked(prometheusAPI)
				Expect(err).ToNot(HaveOccurred(), "Failed to assert that all metrics are LOCKED")
			}
		}

		if !testActuallyRan {
			Skip("Could not find any interfaces to test")
		}
	})

	// 49734 - Validating there is no effect when Boundary Clock master interface goes down and up
	It("should have no effect when Boundary Clock master interface goes down and up", reportxml.ID("49734"), func() {
		testActuallyRan := false

		By("getting node info map")
		nodeInfoMap, err := profiles.GetNodeInfoMap(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to get node info map")

		for nodeName, nodeInfo := range nodeInfoMap {
			By("checking if node has Boundary Clock configuration")
			if nodeInfo.Counts[profiles.ProfileTypeBC] == 0 {
				klog.V(tsparams.LogLevel).Infof("Node %s has no BC configuration, skipping", nodeName)

				continue
			}

			testActuallyRan = true

			By("getting the event pod for the node")
			eventPod, err := consumer.GetConsumerPodforNode(RANConfig.Spoke1APIClient, nodeName)
			Expect(err).ToNot(HaveOccurred(), "Failed to get event pod for node %s", nodeName)

			By("getting the boundary clock master interfaces")
			masterInterfaces := nodeInfo.GetInterfacesByClockType(profiles.ClockTypeServer)
			Expect(masterInterfaces).ToNot(BeEmpty(), "Failed to get Boundary Clock master interfaces for node %s", nodeName)

			masterInterfaceGroups := iface.GroupInterfacesByNIC(profiles.GetInterfacesNames(masterInterfaces))

			DeferCleanup(func() {
				if !CurrentSpecReport().Failed() {
					return
				}
				By("setting the boundary clock master interfaces up")
				for _, masterInterface := range masterInterfaces {
					By(fmt.Sprintf("setting the Boundary Clock master interface %s up", masterInterface.Name))
					err := iface.SetInterfaceStatus(RANConfig.Spoke1APIClient, nodeName, masterInterface.Name, iface.InterfaceStateUp)
					Expect(err).ToNot(HaveOccurred(), "Failed to set interface %s to up on node %s", masterInterface.Name, nodeName)
				}
			})

			startTime := time.Now()
			By("setting the boundary clock master interfaces down")
			for _, masterInterface := range masterInterfaces {
				By(fmt.Sprintf("setting the Boundary Clock master interface %s down", masterInterface.Name))
				err := iface.SetInterfaceStatus(RANConfig.Spoke1APIClient, nodeName, masterInterface.Name, iface.InterfaceStateDown)
				Expect(err).ToNot(HaveOccurred(), "Failed to set interface %s to down on node %s", masterInterface.Name, nodeName)
			}

			By("validating that the ptp metric stays in locked state")
			err = metrics.AssertQuery(context.TODO(), prometheusAPI, metrics.ClockStateQuery{}, metrics.ClockStateLocked,
				metrics.AssertWithStableDuration(30*time.Second),
				metrics.AssertWithTimeout(45*time.Second))
			Expect(err).ToNot(HaveOccurred(), "Failed to assert that the PTP metric stays in locked state")

			By("validating that no holdover event is generated")
			for nicName := range masterInterfaceGroups {
				By(fmt.Sprintf("validating that no holdover event is generated for interface %s", nicName))
				holdoverFilter := events.All(
					events.IsType(eventptp.PtpStateChange),
					events.HasValue(events.WithSyncState(eventptp.HOLDOVER), events.OnInterface(nicName)),
				)
				err = events.WaitForEvent(eventPod, startTime, 1*time.Minute, holdoverFilter)
				Expect(err).To(HaveOccurred(), "Unexpected HOLDOVER event detected for interface %s", nicName)
			}

			By("setting the boundary clock master interfaces up")
			for _, masterInterface := range masterInterfaces {
				By(fmt.Sprintf("setting the Boundary Clock master interface %s up", masterInterface.Name))
				err := iface.SetInterfaceStatus(RANConfig.Spoke1APIClient, nodeName, masterInterface.Name, iface.InterfaceStateUp)
				Expect(err).ToNot(HaveOccurred(), "Failed to set interface %s to up on node %s", masterInterface.Name, nodeName)
			}

			By("validating that the ptp metric stays in locked state")
			err = metrics.AssertQuery(context.TODO(), prometheusAPI, metrics.ClockStateQuery{}, metrics.ClockStateLocked,
				metrics.AssertWithStableDuration(30*time.Second),
				metrics.AssertWithTimeout(45*time.Second))
			Expect(err).ToNot(HaveOccurred(), "Failed to assert that the PTP metric stays in locked state")
		}

		if !testActuallyRan {
			Skip("Could not find any boundary clock to test")
		}
	})

	// 73093 - Validating HA failover when active interface goes down
	It("should change high availability active profile when other nic interface is down", reportxml.ID("73093"), func() {
		// Setup
		testActuallyRan := false

		By("getting node info map")
		nodeInfoMap, err := profiles.GetNodeInfoMap(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to get node info map")

		for nodeName, nodeInfo := range nodeInfoMap {
			By("checking if node has HA configuration")
			if nodeInfo.Counts[profiles.ProfileTypeHA] == 0 {
				klog.V(tsparams.LogLevel).Infof("Node %s has no HA configuration, skipping", nodeName)

				continue
			}

			By("getting the active and inactive HA profiles")
			activeProfiles, err := profiles.GetHAProfiles(context.TODO(), prometheusAPI, nodeName,
				metrics.HAProfileStatusActive)
			Expect(err).ToNot(HaveOccurred(), "Failed to get active HA profiles")
			Expect(len(activeProfiles)).To(Equal(1), "Expected exactly one active HA profile")

			inactiveProfiles, err := profiles.GetHAProfiles(context.TODO(), prometheusAPI, nodeName,
				metrics.HAProfileStatusInactive)
			Expect(err).ToNot(HaveOccurred(), "Failed to get inactive HA profiles")
			Expect(len(inactiveProfiles)).To(BeNumerically(">=", 1), "Expected at least one inactive HA profile")

			activeProfile := activeProfiles[0]

			// Client interfaces are the interfaces that are used to synchronize the clock.
			By("getting active profile client interface")
			activeProfileClientInterface := getHAProfileClientInterface(nodeInfo, activeProfile)

			inactiveProfileClientInterfaces := make([]iface.Name, 0, 1)
			for _, inactiveProfile := range inactiveProfiles {
				inactiveProfileClientInterface := getHAProfileClientInterface(nodeInfo, inactiveProfile)
				inactiveProfileClientInterfaces = append(inactiveProfileClientInterfaces, inactiveProfileClientInterface)
			}

			By("checking if active profile client interface is the egress interface")
			egressInterface, err := iface.GetEgressInterfaceName(RANConfig.Spoke1APIClient, nodeName)
			Expect(err).ToNot(HaveOccurred(), "Failed to get egress interface")

			if activeProfileClientInterface == egressInterface {
				klog.V(tsparams.LogLevel).Infof("Skipping test for egress interface %s", activeProfileClientInterface.GetNIC())

				continue
			}

			// Test
			testActuallyRan = true
			startTime := time.Now()

			By(fmt.Sprintf("bringing down the active HA's interface %s", activeProfileClientInterface))

			DeferCleanup(func() {
				By("restoring original active HA's interface")
				err := iface.SetInterfaceStatus(RANConfig.Spoke1APIClient,
					nodeName, activeProfileClientInterface, iface.InterfaceStateUp)
				Expect(err).ToNot(HaveOccurred(), "Failed to restore original active HA's interface")
			})

			err = iface.SetInterfaceStatus(RANConfig.Spoke1APIClient,
				nodeName, activeProfileClientInterface, iface.InterfaceStateDown)
			Expect(err).ToNot(HaveOccurred(), "Failed to set the active HA's interface down")

			By("validating the active HA profile changed")
			newActiveProfile := waitForActiveHAProfileChange(prometheusAPI, nodeName, activeProfile, 2*time.Minute)
			Expect(newActiveProfile).NotTo(Equal(activeProfile), "Active profile should have changed: %s", newActiveProfile)

			By("validating the original active interface is in FREERUN state")
			activeNIC := activeProfileClientInterface.GetNIC()
			clockStateQuery := metrics.ClockStateQuery{
				Interface: metrics.Equals(activeNIC),
				Node:      metrics.Equals(nodeName),
			}
			err = metrics.AssertQuery(context.TODO(), prometheusAPI, clockStateQuery, metrics.ClockStateFreerun,
				metrics.AssertWithTimeout(5*time.Minute),
				metrics.AssertWithStableDuration(10*time.Second))
			Expect(err).ToNot(HaveOccurred(), "Failed to assert original active interface is in FREERUN")

			By("validating that all inactive HA profile client interfaces are in LOCKED state")
			inactiveProfileClientNICs := make([]iface.NICName, 0, len(inactiveProfileClientInterfaces))
			for _, inactiveProfileClientInterface := range inactiveProfileClientInterfaces {
				inactiveProfileClientNICs = append(inactiveProfileClientNICs, inactiveProfileClientInterface.GetNIC())
			}

			inactiveIfacesClockStateQuery := metrics.ClockStateQuery{
				Interface: metrics.Includes(inactiveProfileClientNICs...),
				Node:      metrics.Equals(nodeName),
			}
			err = metrics.AssertQuery(context.TODO(), prometheusAPI, inactiveIfacesClockStateQuery, metrics.ClockStateLocked,
				metrics.AssertWithTimeout(5*time.Minute),
				metrics.AssertWithStableDuration(10*time.Second))
			Expect(err).ToNot(HaveOccurred(), "Failed to assert all inactive interfaces are in LOCKED state")

			By("validating no HOLDOVER event for original inactive interfaces")
			eventPod, err := consumer.GetConsumerPodforNode(RANConfig.Spoke1APIClient, nodeName)
			Expect(err).ToNot(HaveOccurred(), "Failed to get event pod")

			for _, inactiveProfileClientInterface := range inactiveProfileClientInterfaces {
				inactiveProfileClientNIC := inactiveProfileClientInterface.GetNIC()
				holdoverFilter := events.All(
					events.IsType(eventptp.PtpStateChange),
					events.HasValue(events.WithSyncState(eventptp.HOLDOVER), events.OnInterface(inactiveProfileClientNIC)),
				)
				err = events.WaitForEvent(eventPod, startTime, 10*time.Second, holdoverFilter)
				Expect(err).To(HaveOccurred(), "Unexpected HOLDOVER event on original inactive interface %s",
					inactiveProfileClientNIC)
			}

			By("validating CLOCK_REALTIME is in LOCKED state")
			clockRealtimeQuery := metrics.ClockStateQuery{
				Interface: metrics.Equals(iface.ClockRealtime),
				Node:      metrics.Equals(nodeName),
			}
			err = metrics.AssertQuery(context.TODO(), prometheusAPI, clockRealtimeQuery, metrics.ClockStateLocked,
				metrics.AssertWithTimeout(5*time.Minute),
				metrics.AssertWithStableDuration(10*time.Second))
			Expect(err).ToNot(HaveOccurred(), "Failed to assert CLOCK_REALTIME is LOCKED")

			// Cleanup
			By("restoring original active HA's interface")
			err = iface.SetInterfaceStatus(RANConfig.Spoke1APIClient,
				nodeName, activeProfileClientInterface, iface.InterfaceStateUp)
			Expect(err).ToNot(HaveOccurred(), "Failed to restore original active HA's interface")

			By("validating restored active profile client interface returns to LOCKED state")
			activeNIC = activeProfileClientInterface.GetNIC()
			clockStateQuery = metrics.ClockStateQuery{
				Interface: metrics.Equals(activeNIC),
				Node:      metrics.Equals(nodeName),
			}
			err = metrics.AssertQuery(context.TODO(), prometheusAPI, clockStateQuery, metrics.ClockStateLocked,
				metrics.AssertWithTimeout(3*time.Minute),
				metrics.AssertWithStableDuration(10*time.Second))
			Expect(err).ToNot(HaveOccurred(), "Failed to assert restored active profile client interface is LOCKED")

			initialTotalProfiles := len(activeProfiles) + len(inactiveProfiles)
			By("validating HA system returns to healthy state")
			waitForHAHealthy(prometheusAPI, nodeName, initialTotalProfiles, 2*time.Minute)

			By("validating CLOCK_REALTIME remains LOCKED")
			clockRealtimeQuery = metrics.ClockStateQuery{
				Interface: metrics.Equals(iface.ClockRealtime),
				Node:      metrics.Equals(nodeName),
			}
			err = metrics.AssertQuery(context.TODO(), prometheusAPI, clockRealtimeQuery, metrics.ClockStateLocked,
				metrics.AssertWithTimeout(1*time.Minute),
				metrics.AssertWithStableDuration(10*time.Second))
			Expect(err).ToNot(HaveOccurred(), "Failed to assert CLOCK_REALTIME is LOCKED")

			break
		}

		if !testActuallyRan {
			Skip("Could not find any HA configuration to test")
		}
	})

	// 73094 - Validating complete HA failure when both active and inactive interfaces go down
	It("should move to FREERUN state when active and inactive interfaces are down", reportxml.ID("73094"), func() {
		testActuallyRan := false

		By("getting node info map")
		nodeInfoMap, err := profiles.GetNodeInfoMap(RANConfig.Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to get node info map")

		for nodeName, nodeInfo := range nodeInfoMap {
			By("checking if node has HA configuration")
			if nodeInfo.Counts[profiles.ProfileTypeHA] == 0 {
				klog.V(tsparams.LogLevel).Infof("Node %s has no HA configuration, skipping", nodeName)

				continue
			}

			By("getting the active and inactive HA profiles")
			activeProfiles, err := profiles.GetHAProfiles(context.TODO(), prometheusAPI, nodeName, metrics.HAProfileStatusActive)
			Expect(err).ToNot(HaveOccurred(), "Failed to get active HA profiles")
			Expect(len(activeProfiles)).To(Equal(1), "Expected exactly one active HA profile")

			activeProfileInfo := nodeInfo.GetProfileByName(activeProfiles[0])
			Expect(activeProfileInfo).ToNot(BeNil(), "Failed to find active profile in node info")

			activeClientInterfaces := activeProfileInfo.GetInterfacesByClockType(profiles.ClockTypeClient)
			Expect(len(activeClientInterfaces)).To(Equal(1), "Expected exactly one client interface for HA profile")

			inactiveProfiles, err := profiles.GetHAProfiles(context.TODO(),
				prometheusAPI, nodeName, metrics.HAProfileStatusInactive)
			Expect(err).ToNot(HaveOccurred(), "Failed to get inactive HA profiles")
			Expect(len(inactiveProfiles)).To(BeNumerically(">=", 1), "Expected at least one inactive HA profile")

			haInterfaces := make([]iface.Name, 0, len(activeProfiles)+len(inactiveProfiles))
			haInterfaces = append(haInterfaces, activeClientInterfaces[0].Name)
			for _, inactiveProfile := range inactiveProfiles {
				inactiveProfileInfo := nodeInfo.GetProfileByName(inactiveProfile)
				Expect(inactiveProfileInfo).ToNot(BeNil(), "Failed to find inactive profile in node info")

				inactiveClientInterfaces := inactiveProfileInfo.GetInterfacesByClockType(profiles.ClockTypeClient)
				Expect(len(inactiveClientInterfaces)).To(Equal(1), "Expected exactly one client interface for HA profile")
				haInterfaces = append(haInterfaces, inactiveClientInterfaces[0].Name)
			}

			By("checking if any interface is the egress interface")
			egressInterface, err := iface.GetEgressInterfaceName(RANConfig.Spoke1APIClient, nodeName)
			Expect(err).ToNot(HaveOccurred(), "Failed to get egress interface")

			if slices.Contains(haInterfaces, egressInterface) {
				klog.V(tsparams.LogLevel).Infof("Skipping test - HA interface is egress interface")

				continue
			}

			testActuallyRan = true

			By("bringing down all HA interfaces")
			DeferCleanup(func() {
				By("restoring all HA interfaces")
				for _, haInterface := range haInterfaces {
					err := iface.SetInterfaceStatus(RANConfig.Spoke1APIClient, nodeName, haInterface, iface.InterfaceStateUp)
					Expect(err).ToNot(HaveOccurred(), "Failed to restore HA interface %s", haInterface)
				}
			})

			for _, haInterface := range haInterfaces {
				err = iface.SetInterfaceStatus(RANConfig.Spoke1APIClient, nodeName, haInterface, iface.InterfaceStateDown)
				Expect(err).ToNot(HaveOccurred(), "Failed to set HA interface %s down", haInterface)
			}

			By("validating all HA Clock States are in FREERUN state")
			haNICs := make([]iface.NICName, 0, len(haInterfaces))
			for _, haInterface := range haInterfaces {
				haNICs = append(haNICs, haInterface.GetNIC())
			}

			haIfacesClockStateQuery := metrics.ClockStateQuery{
				Interface: metrics.Includes(haNICs...),
				Node:      metrics.Equals(nodeName),
			}

			err = metrics.AssertQuery(context.TODO(), prometheusAPI, haIfacesClockStateQuery, metrics.ClockStateFreerun,
				metrics.AssertWithTimeout(1*time.Minute),
				metrics.AssertWithStableDuration(10*time.Second))
			Expect(err).ToNot(HaveOccurred(), "Failed to assert all HA interfaces are in FREERUN state")

			By("validating CLOCK_REALTIME is in FREERUN state")
			clockRealtimeQuery := metrics.ClockStateQuery{
				Interface: metrics.Equals(iface.ClockRealtime),
				Node:      metrics.Equals(nodeName),
			}
			err = metrics.AssertQuery(context.TODO(), prometheusAPI, clockRealtimeQuery, metrics.ClockStateFreerun,
				metrics.AssertWithTimeout(1*time.Minute),
				metrics.AssertWithStableDuration(10*time.Second))
			Expect(err).ToNot(HaveOccurred(), "Failed to assert CLOCK_REALTIME is in FREERUN")

			By("restoring HA interfaces")
			for _, haInterface := range haInterfaces {
				err = iface.SetInterfaceStatus(RANConfig.Spoke1APIClient, nodeName, haInterface, iface.InterfaceStateUp)
				Expect(err).ToNot(HaveOccurred(), "Failed to restore HA interface %s", haInterface)
			}

			By("validating all Clock State metrics return to LOCKED state")
			haNICs = make([]iface.NICName, 0, len(haInterfaces))
			for _, haInterface := range haInterfaces {
				haNICs = append(haNICs, haInterface.GetNIC())
			}

			haIfacesClockStateQuery = metrics.ClockStateQuery{
				Interface: metrics.Includes(haNICs...),
				Node:      metrics.Equals(nodeName),
			}

			err = metrics.AssertQuery(context.TODO(), prometheusAPI, haIfacesClockStateQuery, metrics.ClockStateLocked,
				metrics.AssertWithTimeout(3*time.Minute),
				metrics.AssertWithStableDuration(10*time.Second))
			Expect(err).ToNot(HaveOccurred(), "Failed to assert all HA interfaces are in LOCKED state")

			By("validating HA system returns to healthy state")
			waitForHAHealthy(prometheusAPI, nodeName, len(haInterfaces), 2*time.Minute)

			By("validating CLOCK_REALTIME returns to LOCKED state")
			clockRealtimeQuery = metrics.ClockStateQuery{
				Interface: metrics.Equals(iface.ClockRealtime),
				Node:      metrics.Equals(nodeName),
			}
			err = metrics.AssertQuery(context.TODO(), prometheusAPI, clockRealtimeQuery, metrics.ClockStateLocked,
				metrics.AssertWithTimeout(3*time.Minute),
				metrics.AssertWithStableDuration(10*time.Second))
			Expect(err).ToNot(HaveOccurred(), "Failed to assert CLOCK_REALTIME is LOCKED")

			break
		}

		if !testActuallyRan {
			Skip("Could not find any HA configuration to test")
		}
	})

	Context("HA profile configuration deletion", func() {
		// 73095 - Validating HA failover when active profile configuration is deleted
		It("should change high availability active profile when active profile is deleted",
			reportxml.ID("73095"), func() {
				testActuallyRan := false

				By("getting node info map")
				nodeInfoMap, err := profiles.GetNodeInfoMap(RANConfig.Spoke1APIClient)
				Expect(err).ToNot(HaveOccurred(), "Failed to get node info map")

				for nodeName, nodeInfo := range nodeInfoMap {
					By("checking if node has HA configuration")
					if nodeInfo.Counts[profiles.ProfileTypeHA] == 0 {
						klog.V(tsparams.LogLevel).Infof("Node %s has no HA configuration, skipping", nodeName)

						continue
					}

					testActuallyRan = true

					totalHAProfiles := 0

					By("getting the active HA profile & total number of HA profiles")
					activeProfiles, err := profiles.GetHAProfiles(context.TODO(),
						prometheusAPI, nodeName, metrics.HAProfileStatusActive)
					Expect(err).ToNot(HaveOccurred(), "Failed to get active HA profiles")
					Expect(len(activeProfiles)).To(Equal(1), "Expected exactly one active HA profile")

					activeProfileName := activeProfiles[0]
					totalHAProfiles++

					By(fmt.Sprintf("getting the PtpConfig for active profile %s", activeProfileName))
					activeProfileInfo := nodeInfo.GetProfileByName(activeProfileName)
					Expect(activeProfileInfo).ToNot(BeNil(), "Failed to find profile info for active profile %s", activeProfileName)

					By(fmt.Sprintf("pulling PtpConfig %s", activeProfileInfo.Reference.ConfigReference.Name))
					activeProfilePtpConfig, err := activeProfileInfo.Reference.PullPtpConfig(RANConfig.Spoke1APIClient)
					Expect(err).ToNot(HaveOccurred(), "Failed to pull PtpConfig")

					inactiveProfiles, err := profiles.GetHAProfiles(context.TODO(),
						prometheusAPI, nodeName, metrics.HAProfileStatusInactive)
					Expect(err).ToNot(HaveOccurred(), "Failed to get inactive HA profiles")
					Expect(len(inactiveProfiles)).To(BeNumerically(">=", 1), "Expected at least one inactive HA profile")
					totalHAProfiles += len(inactiveProfiles)

					DeferCleanup(func() {
						By("validating HA is healthy after restoration")
						waitForHAHealthy(prometheusAPI, nodeName, totalHAProfiles, 5*time.Minute)

						By("validating all clocks are locked after restoration")
						err = metrics.EnsureClocksAreLocked(prometheusAPI)
						Expect(err).ToNot(HaveOccurred(), "Failed to assert all clocks are locked after restoration")
					})

					By(fmt.Sprintf("deleting PtpProfile %s from PtpConfig %s",
						activeProfileName, activeProfilePtpConfig.Definition.Name))
					err = profiles.RemoveProfileFromConfig(RANConfig.Spoke1APIClient, activeProfileInfo.Reference)
					Expect(err).ToNot(HaveOccurred(),
						"Failed to delete PtpProfile %s from PtpConfig %s", activeProfileName, activeProfilePtpConfig.Definition.Name)

					By("validating active profile has changed")
					newActiveProfile := waitForActiveHAProfileChange(prometheusAPI, nodeName, activeProfileName, 2*time.Minute)

					By("validating new active interface is in LOCKED state")
					newActiveInterface := getHAProfileClientInterface(nodeInfo, newActiveProfile)
					nicName := newActiveInterface.GetNIC()
					clockStateQuery := metrics.ClockStateQuery{
						Interface: metrics.Equals(nicName),
						Node:      metrics.Equals(nodeName),
					}
					err = metrics.AssertQuery(context.TODO(), prometheusAPI, clockStateQuery, metrics.ClockStateLocked,
						metrics.AssertWithTimeout(3*time.Minute),
						metrics.AssertWithStableDuration(10*time.Second))
					Expect(err).ToNot(HaveOccurred(), "Failed to assert new active interface is LOCKED")

					// Test only on first HA node found
					break
				}

				if !testActuallyRan {
					Skip("Could not find any HA configuration to test")
				}
			})
	})
})

// waitForActiveHAProfileChange waits for the active HA profile to change away from the specified profile.
// It returns the new active profile name.
func waitForActiveHAProfileChange(
	prometheusAPI prometheusv1.API,
	nodeName string,
	oldProfileName string,
	timeout time.Duration,
) string {
	GinkgoHelper()

	var newActiveProfile string

	Eventually(func() (bool, error) {
		activeProfiles, err := profiles.GetHAProfiles(context.TODO(), prometheusAPI, nodeName,
			metrics.HAProfileStatusActive)
		if err != nil {
			klog.V(tsparams.LogLevel).Infof("Failed to get active HA profiles: %v", err)

			return false, err
		}

		// Must have exactly one active profile
		if len(activeProfiles) != 1 {
			klog.V(tsparams.LogLevel).Infof("Expected 1 active profile, got %d", len(activeProfiles))

			return false, fmt.Errorf("expected 1 active profile, got %d", len(activeProfiles))
		}

		// Active profile must be different from the old one
		if activeProfiles[0] == oldProfileName {
			return false, nil
		}

		newActiveProfile = activeProfiles[0]

		return true, nil
	}, timeout, 5*time.Second).Should(BeTrue(),
		"Active HA profile should have changed from %s on node %s", oldProfileName, nodeName)

	return newActiveProfile
}

// getHAProfileClientInterface returns the client interface for a given HA profile name.
// This is a convenience helper to reduce boilerplate in tests. It expects the profile to have exactly one client.
func getHAProfileClientInterface(nodeInfo *profiles.NodeInfo, profileName string) iface.Name {
	GinkgoHelper()

	profileInfo := nodeInfo.GetProfileByName(profileName)
	Expect(profileInfo).ToNot(BeNil(), "Failed to find profile %s in node info", profileName)

	clientInterfaces := profileInfo.GetInterfacesByClockType(profiles.ClockTypeClient)
	Expect(len(clientInterfaces)).To(Equal(1),
		"Expected exactly one client interface for HA profile %s", profileName)

	return clientInterfaces[0].Name
}

// waitForHAHealthy waits for the HA to return to a healthy state with the expected
// number of profiles. This is used for cleanup/restoration validation.
func waitForHAHealthy(
	prometheusAPI prometheusv1.API,
	nodeName string,
	expectedTotalProfiles int,
	timeout time.Duration,
) {
	GinkgoHelper()
	Eventually(func() (bool, error) {
		activeProfiles, activeErr := profiles.GetHAProfiles(context.TODO(), prometheusAPI, nodeName,
			metrics.HAProfileStatusActive)
		if activeErr != nil {
			klog.V(tsparams.LogLevel).Infof("Failed to get active HA profiles: %v",
				activeErr)

			return false, activeErr
		}

		inactiveProfiles, inactiveErr := profiles.GetHAProfiles(context.TODO(), prometheusAPI, nodeName,
			metrics.HAProfileStatusInactive)
		if inactiveErr != nil {
			klog.V(tsparams.LogLevel).Infof("Failed to get inactive HA profiles: %v",
				inactiveErr)

			return false, inactiveErr
		}

		// Should have exactly 1 active profile
		if len(activeProfiles) != 1 {
			klog.V(tsparams.LogLevel).Infof("Expected 1 active profile, got %d", len(activeProfiles))

			return false, nil
		}

		// Total profiles should match expected
		totalProfiles := len(activeProfiles) + len(inactiveProfiles)
		if totalProfiles != expectedTotalProfiles {
			klog.V(tsparams.LogLevel).Infof("Expected %d total profiles, got %d (active=%d, inactive=%d)",
				expectedTotalProfiles, totalProfiles, len(activeProfiles), len(inactiveProfiles))

			return false, nil
		}

		return true, nil
	}, timeout, 5*time.Second).Should(BeTrue(),
		"HA system should be healthy with %d total profiles on node %s", expectedTotalProfiles, nodeName)
}
