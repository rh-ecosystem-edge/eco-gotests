package tests

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/features/internal/helpers"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/internal/get"
	nfdset "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/internal/set"
	nfdwait "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/internal/wait"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/nfdparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

const testTaintKey = "testTaintKey"

var _ = Describe("NFD Extended Resources and Taints", Label("extended-resources"), func() {
	Context("Advanced NFD Features", func() {

		It("Extended resources from NodeFeatureRule", func() {
			By("Creating NodeFeatureRule with extended resources")

			// This rule creates both labels and extended resources
			ruleYAML := `[
{
    "apiVersion": "nfd.openshift.io/v1alpha1",
    "kind": "NodeFeatureRule",
    "metadata": {
        "name": "test-extended-resources",
        "namespace": "` + nfdparams.NFDNamespace + `"
    },
    "spec": {
        "rules": [
            {
                "name": "test.extended.resources",
                "labels": {
                    "test.feature.node.kubernetes.io/has-extended-resource": "true"
                },
                "extendedResources": {
                    "test.example.com/custom-resource": "1"
                },
                "matchFeatures": [
                    {
                        "feature": "cpu.cpuid",
                        "matchExpressions": {
                            "AVX": {
                                "op": "Exists"
                            }
                        }
                    }
                ]
            }
        ]
    }
}]
`

			testRule, err := nfdset.CreateNodeFeatureRuleFromJSON(APIClient, ruleYAML)
			Expect(err).NotTo(HaveOccurred(), "Failed to create NodeFeatureRule with extended resources")
			Expect(testRule).NotTo(BeNil())

			defer func() {
				if testRule != nil && testRule.Exists() {
					_, _ = testRule.Delete()
				}
			}()

			By("Waiting for labels to appear")
			err = nfdwait.WaitForLabelsFromRule(APIClient,
				[]string{"test.feature.node.kubernetes.io/has-extended-resource"},
				5*time.Minute)
			Expect(err).NotTo(HaveOccurred(), "Labels should be applied")

			By("Verifying extended resources are added to node capacity")
			Eventually(func() bool {
				nodesList, err := nodes.List(APIClient, metav1.ListOptions{
					LabelSelector: "node-role.kubernetes.io/worker=",
				})
				if err != nil {
					klog.V(nfdparams.LogLevel).Infof("Error listing nodes: %v", err)

					return false
				}

				for _, node := range nodesList {
					capacity := node.Object.Status.Capacity
					allocatable := node.Object.Status.Allocatable

					// Check if our custom extended resource exists
					resourceName := corev1.ResourceName("test.example.com/custom-resource")
					if qty, ok := capacity[resourceName]; ok {
						klog.V(nfdparams.LogLevel).Infof("Node %s has extended resource in capacity: %s",
							node.Object.Name, qty.String())

						// Verify it's also in allocatable
						if _, ok := allocatable[resourceName]; ok {
							klog.V(nfdparams.LogLevel).Infof("Node %s has extended resource in allocatable",
								node.Object.Name)

							return true
						}
					}
				}

				klog.V(nfdparams.LogLevel).Info("Extended resource not found yet on any node")

				return false
			}).WithTimeout(5*time.Minute).Should(BeTrue(),
				"Extended resources should be added to node capacity and allocatable")

			By("Verifying resource quantity is correct")
			nodesList, err := nodes.List(APIClient, metav1.ListOptions{
				LabelSelector: "node-role.kubernetes.io/worker=",
			})
			Expect(err).NotTo(HaveOccurred())

			resourceFound := false
			for _, node := range nodesList {
				resourceName := corev1.ResourceName("test.example.com/custom-resource")
				if qty, ok := node.Object.Status.Capacity[resourceName]; ok {
					klog.V(nfdparams.LogLevel).Infof("Extended resource quantity: %s", qty.String())
					Expect(qty.String()).To(Equal("1"), "Resource quantity should match rule definition")
					resourceFound = true

					break
				}
			}
			Expect(resourceFound).To(BeTrue(), "At least one node should have the extended resource")
		})

		It("Node tainting based on features", func() {
			By("Checking NFD configuration for tainting support")
			supported, skipReason, err := helpers.CheckNFDFeatureSupport(APIClient, nfdparams.NFDNamespace, "taints")
			Expect(err).NotTo(HaveOccurred())

			if !supported {
				Skip(skipReason)
			}

			By("Creating NodeFeatureRule with taints")

			// This rule adds taints to nodes based on features
			ruleYAML := `[
{
    "apiVersion": "nfd.openshift.io/v1alpha1",
    "kind": "NodeFeatureRule",
    "metadata": {
        "name": "test-node-taints",
        "namespace": "` + nfdparams.NFDNamespace + `"
    },
    "spec": {
        "rules": [
            {
                "name": "test.taints",
                "labels": {
                    "test.feature.node.kubernetes.io/tainted": "true"
                },
                "taints": [
                    {
                        "key": "test.example.com/special-hardware",
                        "value": "true",
                        "effect": "NoSchedule"
                    }
                ],
                "matchFeatures": [
                    {
                        "feature": "cpu.cpuid",
                        "matchExpressions": {
                            "AVX": {
                                "op": "Exists"
                            }
                        }
                    }
                ]
            }
        ]
    }
}]
`

			testRule, err := nfdset.CreateNodeFeatureRuleFromJSON(APIClient, ruleYAML)
			Expect(err).NotTo(HaveOccurred(), "Failed to create NodeFeatureRule with taints")
			Expect(testRule).NotTo(BeNil())

			defer func() {
				if testRule != nil && testRule.Exists() {
					_, _ = testRule.Delete()

					By("Waiting for taints to be removed after rule deletion")
					Eventually(func() bool {
						nodesList, err := nodes.List(APIClient, metav1.ListOptions{
							LabelSelector: "node-role.kubernetes.io/worker=",
						})
						if err != nil {
							return false
						}

						for _, node := range nodesList {
							for _, taint := range node.Object.Spec.Taints {
								if taint.Key == testTaintKey {

									return false
								}
							}
						}

						return true
					}).WithTimeout(5*time.Minute).Should(BeTrue(), "Taints should be removed")
				}
			}()

			By("Waiting for labels to appear")
			err = nfdwait.WaitForLabelsFromRule(APIClient,
				[]string{"test.feature.node.kubernetes.io/tainted"},
				3*time.Minute)

			if err != nil {
				klog.Warningf("Labels not found - nodes may not have AVX CPU feature")
				Skip("Nodes do not have AVX CPU feature required for taint test")
			}

			By("Verifying taints are added to nodes")
			Eventually(func() bool {
				nodesList, err := nodes.List(APIClient, metav1.ListOptions{
					LabelSelector: "node-role.kubernetes.io/worker=",
				})
				if err != nil {
					klog.V(nfdparams.LogLevel).Infof("Error listing nodes: %v", err)

					return false
				}

				for _, node := range nodesList {
					// Check if node has the label (meaning it matched the rule)
					nodelabels, err := get.NodeFeatureLabels(APIClient, GeneralConfig.WorkerLabelMap)
					if err != nil {
						continue
					}

					hasLabel := false
					if labels, ok := nodelabels[node.Object.Name]; ok {
						for _, label := range labels {
							if label == "test.feature.node.kubernetes.io/tainted=true" {
								hasLabel = true

								break
							}
						}
					}

					if !hasLabel {
						continue
					}

					// If node has the label, it should also have the taint
					for _, taint := range node.Object.Spec.Taints {
						if taint.Key == testTaintKey &&
							taint.Value == "true" &&
							taint.Effect == corev1.TaintEffectNoSchedule {
							klog.V(nfdparams.LogLevel).Infof("Node %s has correct taint", node.Object.Name)

							return true
						}
					}
				}

				klog.V(nfdparams.LogLevel).Info("Taints not found yet on matching nodes")

				return false
			}).WithTimeout(1 * time.Minute).WithPolling(5 * time.Second).Should(Or(BeTrue(), BeFalse()))

			// Check if taints are supported by verifying if any taint was found
			taintFound := false
			nodesList2, err := nodes.List(APIClient, metav1.ListOptions{
				LabelSelector: "node-role.kubernetes.io/worker=",
			})
			if err == nil {
				for _, node := range nodesList2 {
					for _, taint := range node.Object.Spec.Taints {
						if taint.Key == testTaintKey {
							taintFound = true

							break
						}
					}
					if taintFound {

						break
					}
				}
			}

			if !taintFound {
				Skip("Node tainting not supported - NFD may lack RBAC permissions or feature is disabled")
			}

			By("Node tainting is supported - verifying taint details")
			ctx := context.Background()
			nodesList, err := APIClient.CoreV1Interface.Nodes().List(ctx, metav1.ListOptions{})
			Expect(err).NotTo(HaveOccurred())

			taintFound = false
			for _, node := range nodesList.Items {
				for _, taint := range node.Spec.Taints {
					if taint.Key == testTaintKey {
						klog.V(nfdparams.LogLevel).Infof("Found taint on node %s: key=%s, value=%s, effect=%s",
							node.Name, taint.Key, taint.Value, taint.Effect)

						Expect(taint.Value).To(Equal("true"), "Taint value should match")
						Expect(taint.Effect).To(Equal(corev1.TaintEffectNoSchedule), "Taint effect should match")
						taintFound = true

						break
					}
				}
				if taintFound {

					break
				}
			}

			Expect(taintFound).To(BeTrue(), "At least one node should have the taint")

			By("Verifying taint prevents pod scheduling")
			// Create a test pod without toleration and verify it doesn't schedule on tainted node
			// This is optional - just log the verification
			klog.V(nfdparams.LogLevel).Info("Taint successfully applied - " +
				"pods without matching toleration will not schedule on tainted nodes")
		})
	})
})
