package tests

import (
	"context"
	"fmt"
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

	When("OC 2-port configuration exists", func() {
		Context("when active port fails", func() {
			// Test 80963
			It("verifies failover to passive port", reportxml.ID("80963"), func() {
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

					oc2PortProfile := oc2PortProfiles[0]
					By(fmt.Sprintf("Using OC 2-port profile %s on node %s",
						oc2PortProfile.Reference.ProfileName, nodeName))

					oc2PortInterfaces := oc2PortProfile.GetInterfacesByClockType(profiles.ClockTypeClient)
					Expect(len(oc2PortInterfaces)).To(BeNumerically(">", 1), "Expected more than 1 OC 2-port interface for node %s", nodeName)

					activeInterface, passiveInterface, err := profiles.Oc2PortDetermineActivePassiveInterfaces(
						context.TODO(), prometheusAPI, nodeName, oc2PortInterfaces)
					Expect(err).ToNot(HaveOccurred(), "Failed to determine active/passive interfaces")

					By(fmt.Sprintf("identified active interface: %s, passive interface: %s",
						activeInterface, passiveInterface))

					DeferCleanup(func() {
						By("Restoring OC 2-port interfaces")
						for _, ifaceName := range []iface.Name{activeInterface, passiveInterface} {
							err := iface.SetInterfaceStatus(RANConfig.Spoke1APIClient, nodeName,
								ifaceName, iface.InterfaceStateUp)
							Expect(err).ToNot(HaveOccurred(), "Failed to set interface %s to up on node %s", ifaceName, nodeName)
						}

					})
					By("getting event consumer pod for the node")
					eventPod, err := consumer.GetConsumerPodforNode(RANConfig.Spoke1APIClient, nodeName)
					Expect(err).ToNot(HaveOccurred(), "Failed to get event consumer pod for node %s", nodeName)

					startTime := time.Now()

					By("bringing down the active interface to cause a failover")
					err = iface.SetInterfaceStatus(
						RANConfig.Spoke1APIClient, nodeName, activeInterface, iface.InterfaceStateDown)
					Expect(err).ToNot(HaveOccurred(),
						"Failed to set interface %s to down on node %s", activeInterface, nodeName)

					By("validating PTP clock class metric remains 6 after failover")
					err = metrics.AssertQuery(context.TODO(), prometheusAPI, metrics.ClockClassQuery{}, 6,
						metrics.AssertWithStableDuration(10*time.Second),
						metrics.AssertWithTimeout(45*time.Second))
					Expect(err).ToNot(HaveOccurred(), "Failed to assert that the PTP clock class metric remains 6 after failover")

					By("validating PTP clock state metric remains LOCKED after failover")
					err = metrics.AssertQuery(context.TODO(), prometheusAPI, metrics.ClockStateQuery{}, metrics.ClockStateLocked,
						metrics.AssertWithStableDuration(10*time.Second),
						metrics.AssertWithTimeout(45*time.Second))
					Expect(err).ToNot(HaveOccurred(), "Failed to assert that the PTP process metric stays in LOCKED state after failover")

					By("validating PTP inital active interface role metric change to FAULTY after failover")
					err = metrics.AssertQuery(context.TODO(), prometheusAPI, metrics.InterfaceRoleQuery{}, metrics.InterfaceRoleFaulty,
						metrics.AssertWithTimeout(45*time.Second))
					Expect(err).ToNot(HaveOccurred(), "Failed to assert that the PTP process metric stays in LOCKED state after failover")

					By("validating PTP passive interface role metric changed to SLAVE after failover")
					err = metrics.AssertQuery(context.TODO(), prometheusAPI, metrics.InterfaceRoleQuery{}, metrics.InterfaceRoleFollower,
						metrics.AssertWithTimeout(45*time.Second))
					Expect(err).ToNot(HaveOccurred(), "Failed to assert that the PTP process metric stays in LOCKED state after failover")

					By("validating no FREERUN event is generated after failover")
					freerunFilter := events.All(
						events.IsType(eventptp.PtpStateChange),
						events.HasValue(events.WithSyncState(eventptp.FREERUN), events.OnInterface(iface.NICName(activeInterface.))),
					)
					err = events.WaitForEvent(eventPod, startTime, 1*time.Minute, freerunFilter)
					Expect(err).To(HaveOccurred(), "Unexpected FREERUN event detected for interface %s", activeInterface)
				}

				if !testActuallyRan {
					Skip("Could not find any OC 2-port configuration to test")
				}
			})

			Context("when both ports fail", func() {
				// Test 80964
				It("verifies holdover then freerun transition", reportxml.ID("80964"), func() {
					DeferCleanup(func() {
						By("restoring OC 2-port interfaces")
					})

					By("bringing down both interfaces")

					By("validating both interfaces are FAULTY")

					By("validating clock states transition to FREERUN")

					By("validating HOLDOVER event is generated")

					By("validating FREERUN event is generated")
				})
			})

			Context("when passive port fails", func() {
				// Test 82012
				It("verifies active port continues operation", reportxml.ID("82012"), func() {
					DeferCleanup(func() {
						By("restoring OC 2-port interfaces")
					})

					By("bringing down the passive interface")

					By("validating clock states remain LOCKED")

					By("validating active interface remains FOLLOWER")

					By("validating passive interface is FAULTY")

					By("validating no HOLDOVER event is generated")
				})
			})
		})
	})
})
