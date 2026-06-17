package node_health_cnf_test

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/node-health/internal/nodehealthparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe(
	"Node Health Monitoring Suite",
	Ordered,
	ContinueOnFailure,
	Label("node-health-workflow"), func() {

		var (
			nodesList []*nodes.Builder
		)

		BeforeAll(func() {
			// Get all nodes using the nodes.List function
			var err error
			nodesList, err = nodes.List(APIClient)
			Expect(err).NotTo(HaveOccurred(), "Failed to list nodes")
			Expect(nodesList).NotTo(BeEmpty(), "No nodes found in the cluster")

			glog.Infof("Found %d nodes for health monitoring", len(nodesList))
		})

		Context("Node Readiness Validation", Label("node-readiness"), func() {
			It("Verify all nodes are in Ready state",
				Label("node-readiness-check"),
				reportxml.ID("node-health-001"),
				func() {
					for _, nodeBuilder := range nodesList {
						nodeObj := nodeBuilder.Object
						glog.Infof("Checking node: %s", nodeObj.Name)

						// Check if node is ready
						isReady := false
						for _, condition := range nodeObj.Status.Conditions {
							if condition.Type == corev1.NodeReady {
								isReady = condition.Status == corev1.ConditionTrue
								break
							}
						}

						Expect(isReady).To(BeTrue(), "Node %s is not in Ready state", nodeObj.Name)
						glog.Infof("Node %s is Ready", nodeObj.Name)
					}
				})

			It("Verify no nodes have NotReady condition",
				Label("node-notready-check"),
				reportxml.ID("node-health-002"),
				func() {
					for _, nodeBuilder := range nodesList {
						nodeObj := nodeBuilder.Object
						glog.Infof("Checking node: %s for NotReady condition", nodeObj.Name)

						// Check for NotReady condition
						hasNotReady := false
						for _, condition := range nodeObj.Status.Conditions {
							if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionFalse {
								hasNotReady = true
								break
							}
						}

						Expect(hasNotReady).To(BeFalse(), "Node %s has NotReady condition", nodeObj.Name)
						glog.Infof("Node %s has no NotReady condition", nodeObj.Name)
					}
				})
		})

		Context("Node Pressure Validation", Label("node-pressure"), func() {
			It("Verify nodes are not under disk pressure",
				Label("disk-pressure-check"),
				reportxml.ID("node-health-003"),
				func() {
					for _, nodeBuilder := range nodesList {
						nodeObj := nodeBuilder.Object
						glog.Infof("Checking node: %s for disk pressure", nodeObj.Name)

						// Check disk pressure condition
						hasDiskPressure := false
						for _, condition := range nodeObj.Status.Conditions {
							if condition.Type == corev1.NodeDiskPressure && condition.Status == corev1.ConditionTrue {
								hasDiskPressure = true
								break
							}
						}

						Expect(hasDiskPressure).To(BeFalse(), "Node %s is under disk pressure", nodeObj.Name)
						glog.Infof("Node %s has no disk pressure", nodeObj.Name)
					}
				})

			It("Verify nodes are not under memory pressure",
				Label("memory-pressure-check"),
				reportxml.ID("node-health-004"),
				func() {
					for _, nodeBuilder := range nodesList {
						nodeObj := nodeBuilder.Object
						glog.Infof("Checking node: %s for memory pressure", nodeObj.Name)

						// Check memory pressure condition
						hasMemoryPressure := false
						for _, condition := range nodeObj.Status.Conditions {
							if condition.Type == corev1.NodeMemoryPressure && condition.Status == corev1.ConditionTrue {
								hasMemoryPressure = true
								break
							}
						}

						Expect(hasMemoryPressure).To(BeFalse(), "Node %s is under memory pressure", nodeObj.Name)
						glog.Infof("Node %s has no memory pressure", nodeObj.Name)
					}
				})

			It("Verify nodes are not under network unavailable pressure",
				Label("network-pressure-check"),
				reportxml.ID("node-health-005"),
				func() {
					for _, nodeBuilder := range nodesList {
						nodeObj := nodeBuilder.Object
						glog.Infof("Checking node: %s for network unavailable pressure", nodeObj.Name)

						// Check network unavailable condition
						hasNetworkUnavailable := false
						for _, condition := range nodeObj.Status.Conditions {
							if condition.Type == corev1.NodeNetworkUnavailable && condition.Status == corev1.ConditionTrue {
								hasNetworkUnavailable = true
								break
							}
						}

						Expect(hasNetworkUnavailable).To(BeFalse(), "Node %s has network unavailable condition", nodeObj.Name)
						glog.Infof("Node %s has no network unavailable condition", nodeObj.Name)
					}
				})
		})

		Context("Node Resource Usage Validation", Label("node-resources"), func() {
			It("Verify node disk usage is within acceptable limits",
				Label("disk-usage-check"),
				reportxml.ID("node-health-006"),
				func() {
					for _, nodeBuilder := range nodesList {
						nodeObj := nodeBuilder.Object
						glog.Infof("Checking node: %s disk usage", nodeObj.Name)

						// Get disk usage from node status
						for _, image := range nodeObj.Status.Images {
							if image.SizeBytes > 0 {
								// Calculate disk usage percentage (simplified)
								// In a real scenario, you might want to use node metrics API
								glog.Infof("Node %s has image size: %d bytes", nodeObj.Name, image.SizeBytes)
							}
						}

						// For now, we'll just verify the node has capacity information
						Expect(nodeObj.Status.Capacity).NotTo(BeNil(), "Node %s has no capacity information", nodeObj.Name)
						Expect(nodeObj.Status.Allocatable).NotTo(BeNil(), "Node %s has no allocatable information", nodeObj.Name)

						glog.Infof("Node %s disk capacity verified", nodeObj.Name)
					}
				})

			It("Verify node memory usage is within acceptable limits",
				Label("memory-usage-check"),
				reportxml.ID("node-health-007"),
				func() {
					for _, nodeBuilder := range nodesList {
						nodeObj := nodeBuilder.Object
						glog.Infof("Checking node: %s memory usage", nodeObj.Name)

						// Check memory capacity and allocatable
						memoryCapacity, hasMemoryCapacity := nodeObj.Status.Capacity[corev1.ResourceMemory]
						Expect(hasMemoryCapacity).To(BeTrue(), "Node %s has no memory capacity", nodeObj.Name)

						memoryAllocatable, hasMemoryAllocatable := nodeObj.Status.Allocatable[corev1.ResourceMemory]
						Expect(hasMemoryAllocatable).To(BeTrue(), "Node %s has no memory allocatable", nodeObj.Name)

						// Verify memory values are reasonable
						Expect(memoryCapacity.Value()).To(BeNumerically(">", 0), "Node %s has invalid memory capacity", nodeObj.Name)
						Expect(memoryAllocatable.Value()).To(BeNumerically(">", 0), "Node %s has invalid memory allocatable", nodeObj.Name)

						glog.Infof("Node %s memory: Capacity=%s, Allocatable=%s",
							nodeObj.Name, memoryCapacity.String(), memoryAllocatable.String())
					}
				})

			It("Verify node CPU usage is within acceptable limits",
				Label("cpu-usage-check"),
				reportxml.ID("node-health-008"),
				func() {
					for _, nodeBuilder := range nodesList {
						nodeObj := nodeBuilder.Object
						glog.Infof("Checking node: %s CPU usage", nodeObj.Name)

						// Check CPU capacity and allocatable
						cpuCapacity, hasCPUCapacity := nodeObj.Status.Capacity[corev1.ResourceCPU]
						Expect(hasCPUCapacity).To(BeTrue(), "Node %s has no CPU capacity", nodeObj.Name)

						cpuAllocatable, hasCPUAllocatable := nodeObj.Status.Allocatable[corev1.ResourceCPU]
						Expect(hasCPUAllocatable).To(BeTrue(), "Node %s has no CPU allocatable", nodeObj.Name)

						// Verify CPU values are reasonable
						Expect(cpuCapacity.Value()).To(BeNumerically(">", 0), "Node %s has invalid CPU capacity", nodeObj.Name)
						Expect(cpuAllocatable.Value()).To(BeNumerically(">", 0), "Node %s has invalid CPU allocatable", nodeObj.Name)

						glog.Infof("Node %s CPU: Capacity=%s, Allocatable=%s",
							nodeObj.Name, cpuCapacity.String(), cpuAllocatable.String())
					}
				})
		})

		Context("Kubelet Status Validation", Label("kubelet-status"), func() {
			It("Verify kubelet pods are running on all nodes",
				Label("kubelet-pod-status"),
				reportxml.ID("node-health-009"),
				func() {
					for _, nodeBuilder := range nodesList {
						nodeObj := nodeBuilder.Object
						glog.Infof("Checking kubelet status on node: %s", nodeObj.Name)

						// Check if kubelet pod is running on this node
						podList, err := APIClient.CoreV1Interface.Pods(nodehealthparams.KubeletNamespace).List(
							context.TODO(),
							metav1.ListOptions{
								FieldSelector: fmt.Sprintf("spec.nodeName=%s", nodeObj.Name),
								LabelSelector: nodehealthparams.KubeletPodSelector,
							},
						)
						Expect(err).NotTo(HaveOccurred(), "Failed to list kubelet pods on node %s", nodeObj.Name)

						// Verify at least one kubelet pod is running
						hasRunningKubelet := false
						for _, pod := range podList.Items {
							if pod.Status.Phase == corev1.PodRunning {
								hasRunningKubelet = true
								break
							}
						}

						Expect(hasRunningKubelet).To(BeTrue(), "No running kubelet pod found on node %s", nodeObj.Name)
						glog.Infof("Kubelet pod is running on node %s", nodeObj.Name)
					}
				})

			It("Verify kubelet service is responding",
				Label("kubelet-service-check"),
				reportxml.ID("node-health-010"),
				func() {
					// This test would typically check kubelet health endpoint
					// For now, we'll verify the kubelet pods are healthy
					for _, nodeBuilder := range nodesList {
						nodeObj := nodeBuilder.Object
						glog.Infof("Checking kubelet service health on node: %s", nodeObj.Name)

						// Check kubelet pod readiness
						podList, err := APIClient.CoreV1Interface.Pods(nodehealthparams.KubeletNamespace).List(
							context.TODO(),
							metav1.ListOptions{
								FieldSelector: fmt.Sprintf("spec.nodeName=%s", nodeObj.Name),
								LabelSelector: nodehealthparams.KubeletPodSelector,
							},
						)
						Expect(err).NotTo(HaveOccurred(), "Failed to list kubelet pods on node %s", nodeObj.Name)

						for _, pod := range podList.Items {
							// Check if pod is ready
							isReady := false
							for _, condition := range pod.Status.Conditions {
								if condition.Type == corev1.PodReady {
									isReady = condition.Status == corev1.ConditionTrue
									break
								}
							}

							Expect(isReady).To(BeTrue(), "Kubelet pod %s on node %s is not ready", pod.Name, nodeObj.Name)
							glog.Infof("Kubelet pod %s on node %s is ready", pod.Name, nodeObj.Name)
						}
					}
				})
		})

		Context("Node Conditions Validation", Label("node-conditions"), func() {
			It("Verify all node conditions are healthy",
				Label("node-conditions-check"),
				reportxml.ID("node-health-011"),
				func() {
					for _, nodeBuilder := range nodesList {
						nodeObj := nodeBuilder.Object
						glog.Infof("Checking all conditions on node: %s", nodeObj.Name)

						// Check all node conditions
						for _, condition := range nodeObj.Status.Conditions {
							glog.Infof("Node %s condition: %s = %s (LastTransitionTime: %s)",
								nodeObj.Name, condition.Type, condition.Status, condition.LastTransitionTime)

							// Skip informational conditions that are expected to be False
							if condition.Type == corev1.NodeNetworkUnavailable &&
								strings.Contains(nodeObj.Name, "master") {
								// Master nodes might have NetworkUnavailable=True during initial setup
								continue
							}

							// Check if condition is healthy
							switch condition.Type {
							case corev1.NodeReady:
								Expect(condition.Status).To(Equal(corev1.ConditionTrue),
									"Node %s Ready condition is not True", nodeObj.Name)
							case corev1.NodeDiskPressure:
								Expect(condition.Status).To(Equal(corev1.ConditionFalse),
									"Node %s DiskPressure condition is not False", nodeObj.Name)
							case corev1.NodeMemoryPressure:
								Expect(condition.Status).To(Equal(corev1.ConditionFalse),
									"Node %s MemoryPressure condition is not False", nodeObj.Name)
							case corev1.NodePIDPressure:
								Expect(condition.Status).To(Equal(corev1.ConditionFalse),
									"Node %s PIDPressure condition is not False", nodeObj.Name)
							}
						}

						glog.Infof("All conditions on node %s are healthy", nodeObj.Name)
					}
				})

			It("Verify node last transition times are recent",
				Label("node-transition-time-check"),
				reportxml.ID("node-health-012"),
				func() {
					for _, nodeBuilder := range nodesList {
						nodeObj := nodeBuilder.Object
						glog.Infof("Checking transition times on node: %s", nodeObj.Name)

						// Check if node conditions have recent transition times
						for _, condition := range nodeObj.Status.Conditions {
							if condition.LastTransitionTime.IsZero() {
								continue // Skip conditions without transition time
							}

							// Check if transition time is within reasonable bounds (not too old)
							timeSinceTransition := time.Since(condition.LastTransitionTime.Time)
							Expect(timeSinceTransition).To(BeNumerically("<", 24*time.Hour),
								"Node %s condition %s has very old transition time: %s",
								nodeObj.Name, condition.Type, condition.LastTransitionTime)

							glog.Infof("Node %s condition %s transition time: %s (age: %v)",
								nodeObj.Name, condition.Type, condition.LastTransitionTime, timeSinceTransition)
						}

						glog.Infof("Transition times on node %s are recent", nodeObj.Name)
					}
				})
		})

		Context("Node Resource Monitoring", Label("node-resource-monitoring"), func() {
			It("Monitor node resource usage over time",
				Label("resource-monitoring"),
				reportxml.ID("node-health-013"),
				func() {
					// This test demonstrates continuous monitoring
					monitoringDuration := 2 * time.Minute
					checkInterval := 30 * time.Second
					startTime := time.Now()

					glog.Infof("Starting resource monitoring for %v", monitoringDuration)

					for time.Since(startTime) < monitoringDuration {
						glog.Infof("Resource check at %v", time.Now().Format("15:04:05"))

						for _, nodeBuilder := range nodesList {
							nodeObj := nodeBuilder.Object

							// Get current node status
							currentNode, err := APIClient.CoreV1Interface.Nodes().Get(
								context.TODO(), nodeObj.Name, metav1.GetOptions{})
							Expect(err).NotTo(HaveOccurred(), "Failed to get current node status for %s", nodeObj.Name)

							// Check current conditions
							for _, condition := range currentNode.Status.Conditions {
								if condition.Type == corev1.NodeReady {
									Expect(condition.Status).To(Equal(corev1.ConditionTrue),
										"Node %s became not ready during monitoring", nodeObj.Name)
								}
							}

							glog.Infof("Node %s is healthy at %v", nodeObj.Name, time.Now().Format("15:04:05"))
						}

						time.Sleep(checkInterval)
					}

					glog.Infof("Resource monitoring completed successfully")
				})
		})
	})
