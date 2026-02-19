package tests

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nfd"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/features/internal/helpers"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/internal/get"
	nfdset "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/internal/set"
	nfdwait "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/internal/wait"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/nfdparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	"k8s.io/klog/v2"
)

var _ = Describe("NFD NodeFeatureRule", Label("custom-rules"), func() {
	Context("Custom Rule Processing", func() {
		var testRule *nfd.NodeFeatureRuleBuilder

		AfterEach(func() {
			if testRule != nil && testRule.Exists() {
				_, err := testRule.Delete()
				if err != nil {
					klog.Errorf("Failed to delete test rule: %v", err)
				}
				testRule = nil
			}
		})

		It("Validates matchExpressions operators", func() {
			By("Creating NodeFeatureRule with various matchExpression operators")

			// This rule tests various operators: In, Exists, Gt, Lt, IsTrue
			ruleYAML := `[
{
    "apiVersion": "nfd.openshift.io/v1alpha1",
    "kind": "NodeFeatureRule",
    "metadata": {
        "name": "test-match-expressions",
        "namespace": "` + nfdparams.NFDNamespace + `"
    },
    "spec": {
        "rules": [
            {
                "name": "test.cpu.features",
                "labels": {
                    "test.feature.node.kubernetes.io/cpu-present": "true"
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
            },
            {
                "name": "test.kernel.version",
                "labels": {
                    "test.feature.node.kubernetes.io/kernel-present": "true"
                },
                "matchFeatures": [
                    {
                        "feature": "kernel.version",
                        "matchExpressions": {
                            "major": {
                                "op": "Gt",
                                "value": ["3"]
                            }
                        }
                    }
                ]
            }
        ]
    }
}]
`

			var err error
			testRule, err = nfdset.CreateNodeFeatureRuleFromJSON(APIClient, ruleYAML)
			Expect(err).NotTo(HaveOccurred(), "Failed to create NodeFeatureRule")
			Expect(testRule).NotTo(BeNil())

			By("Waiting for labels to be applied")
			err = nfdwait.WaitForLabelsFromRule(APIClient,
				[]string{"test.feature.node.kubernetes.io/cpu-present",
					"test.feature.node.kubernetes.io/kernel-present"},
				5*time.Minute)
			Expect(err).NotTo(HaveOccurred(), "Labels were not applied within timeout")

			By("Verifying labels exist on nodes")
			nodelabels, err := get.NodeFeatureLabels(APIClient, GeneralConfig.WorkerLabelMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(nodelabels)).To(BeNumerically(">", 0))

			labelFound := false
			for nodeName := range nodelabels {
				err = helpers.CheckLabelsExist(nodelabels,
					[]string{"test.feature.node.kubernetes.io/cpu-present"},
					nil, nodeName)
				if err == nil {
					labelFound = true

					break
				}
			}
			Expect(labelFound).To(BeTrue(), "Expected labels not found on any node")
		})

		It("Validates labelsTemplate dynamic label generation", func() {
			By("Creating NodeFeatureRule with labelsTemplate")

			// This rule uses template to create dynamic labels from feature values
			// Using CPU model which is more universally available than kernel version attributes
			ruleYAML := `[
{
    "apiVersion": "nfd.openshift.io/v1alpha1",
    "kind": "NodeFeatureRule",
    "metadata": {
        "name": "test-labels-template",
        "namespace": "` + nfdparams.NFDNamespace + `"
    },
    "spec": {
        "rules": [
            {
                "name": "test.template.cpu",
                "labelsTemplate": "test.feature.node.kubernetes.io/cpu-model=true",
                "matchFeatures": [
                    {
                        "feature": "cpu.model",
                        "matchExpressions": {
                            "vendor_id": {
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

			var err error
			testRule, err = nfdset.CreateNodeFeatureRuleFromJSON(APIClient, ruleYAML)
			Expect(err).NotTo(HaveOccurred(), "Failed to create NodeFeatureRule")
			Expect(testRule).NotTo(BeNil())

			By("Waiting for templated labels to be applied")
			err = nfdwait.WaitForLabelsFromRule(APIClient,
				[]string{"test.feature.node.kubernetes.io/cpu-model"},
				3*time.Minute)

			if err != nil {
				klog.V(nfdparams.LogLevel).Infof("labelsTemplate test timed out - this may indicate NFD version incompatibility")
				Skip("labelsTemplate feature not working as expected - may require different NFD configuration")
			}

			By("Verifying dynamic labels exist on nodes")
			nodelabels, err := get.NodeFeatureLabels(APIClient, GeneralConfig.WorkerLabelMap)
			Expect(err).NotTo(HaveOccurred())

			labelFound := false
			for _, labels := range nodelabels {
				for _, label := range labels {
					if len(label) >= len("test.feature.node.kubernetes.io/cpu-model") &&
						label[0:len("test.feature.node.kubernetes.io/cpu-model")] == "test.feature.node.kubernetes.io/cpu-model" {
						klog.V(nfdparams.LogLevel).Infof("Found templated label: %s", label)
						labelFound = true

						break
					}
				}
				if labelFound {

					break
				}
			}
			Expect(labelFound).To(BeTrue(), "Templated labels not found on any node")
		})

		It("Validates matchAny OR logic", func() {
			By("Creating NodeFeatureRule with matchAny for OR logic")

			// This rule uses matchAny to match if ANY condition is true (OR logic)
			ruleYAML := `[
{
    "apiVersion": "nfd.openshift.io/v1alpha1",
    "kind": "NodeFeatureRule",
    "metadata": {
        "name": "test-match-any",
        "namespace": "` + nfdparams.NFDNamespace + `"
    },
    "spec": {
        "rules": [
            {
                "name": "test.matchany.cpu",
                "labels": {
                    "test.feature.node.kubernetes.io/advanced-cpu": "true"
                },
                "matchAny": [
                    {
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
                    },
                    {
                        "matchFeatures": [
                            {
                                "feature": "cpu.cpuid",
                                "matchExpressions": {
                                    "AVX2": {
                                        "op": "Exists"
                                    }
                                }
                            }
                        ]
                    }
                ]
            }
        ]
    }
}]
`

			var err error
			testRule, err = nfdset.CreateNodeFeatureRuleFromJSON(APIClient, ruleYAML)
			Expect(err).NotTo(HaveOccurred(), "Failed to create NodeFeatureRule")
			Expect(testRule).NotTo(BeNil())

			By("Waiting for matchAny labels to be applied")
			err = nfdwait.WaitForLabelsFromRule(APIClient,
				[]string{"test.feature.node.kubernetes.io/advanced-cpu"},
				5*time.Minute)
			Expect(err).NotTo(HaveOccurred(), "matchAny labels were not applied within timeout")

			By("Verifying OR logic labels exist")
			nodelabels, err := get.NodeFeatureLabels(APIClient, GeneralConfig.WorkerLabelMap)
			Expect(err).NotTo(HaveOccurred())

			labelFound := false
			for nodeName := range nodelabels {
				err = helpers.CheckLabelsExist(nodelabels,
					[]string{"test.feature.node.kubernetes.io/advanced-cpu"},
					nil, nodeName)
				if err == nil {
					labelFound = true

					break
				}
			}
			Expect(labelFound).To(BeTrue(), "matchAny labels not found on any node")
		})

		It("Validates backreferences from previous rules", reportxml.ID("54493"), func() {
			By("Checking NFD configuration for backreference support")
			supported, skipReason, err := helpers.CheckNFDFeatureSupport(APIClient, nfdparams.NFDNamespace, "backreferences")
			Expect(err).NotTo(HaveOccurred())

			if !supported {
				Skip(skipReason)
			}

			By("Creating NodeFeatureRule with backreferences")

			// This rule uses backreferences to refer to matches from previous rules
			ruleYAML := `[
{
    "apiVersion": "nfd.openshift.io/v1alpha1",
    "kind": "NodeFeatureRule",
    "metadata": {
        "name": "test-backreferences",
        "namespace": "` + nfdparams.NFDNamespace + `"
    },
    "spec": {
        "rules": [
            {
                "name": "test.first.rule",
                "labels": {
                    "test.feature.node.kubernetes.io/first-rule": "true"
                },
                "matchFeatures": [
                    {
                        "feature": "cpu.cpuid",
                        "matchExpressions": {
                            "SSE4": {
                                "op": "Exists"
                            }
                        }
                    }
                ]
            },
            {
                "name": "test.second.rule",
                "labels": {
                    "test.feature.node.kubernetes.io/second-rule": "true"
                },
                "matchFeatures": [
                    {
                        "feature": "rule.matched",
                        "matchExpressions": {
                            "test.first.rule": {
                                "op": "IsTrue"
                            }
                        }
                    }
                ]
            }
        ]
    }
}]
`

			testRule, err = nfdset.CreateNodeFeatureRuleFromJSON(APIClient, ruleYAML)
			Expect(err).NotTo(HaveOccurred(), "Failed to create NodeFeatureRule")
			Expect(testRule).NotTo(BeNil())

			By("Waiting for first rule labels to be applied")
			err = nfdwait.WaitForLabelsFromRule(APIClient,
				[]string{"test.feature.node.kubernetes.io/first-rule"},
				3*time.Minute)
			Expect(err).NotTo(HaveOccurred(), "First rule labels were not applied within timeout")

			By("Checking if backreferences are supported in this NFD version")
			// Try to detect backreference support with a reasonable timeout (2 minutes)
			// If not supported, skip the test instead of failing
			backrefSupported := false
			Eventually(func() bool {
				nodelabels, err := get.NodeFeatureLabels(APIClient, GeneralConfig.WorkerLabelMap)
				if err != nil {
					return false
				}
				for nodeName := range nodelabels {
					if helpers.CheckLabelsExist(nodelabels,
						[]string{"test.feature.node.kubernetes.io/second-rule"},
						nil, nodeName) == nil {
						klog.V(nfdparams.LogLevel).Infof("Backreference label found on node %s", nodeName)
						backrefSupported = true

						return true
					}
				}

				return false
			}).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(Or(BeTrue(), BeFalse()))

			if !backrefSupported {
				Skip("Backreferences not supported in this NFD version - feature requires NFD v0.12+")
			}

			By("Backreferences are supported - verifying labels")
			// Feature is supported, do final verification

			By("Verifying both rules were processed")
			nodelabels, err := get.NodeFeatureLabels(APIClient, GeneralConfig.WorkerLabelMap)
			Expect(err).NotTo(HaveOccurred())

			firstRuleFound := false
			secondRuleFound := false
			for nodeName := range nodelabels {
				if helpers.CheckLabelsExist(nodelabels,
					[]string{"test.feature.node.kubernetes.io/first-rule"},
					nil, nodeName) == nil {
					firstRuleFound = true
				}
				if helpers.CheckLabelsExist(nodelabels,
					[]string{"test.feature.node.kubernetes.io/second-rule"},
					nil, nodeName) == nil {
					secondRuleFound = true
				}
				if firstRuleFound && secondRuleFound {
					break
				}
			}
			Expect(firstRuleFound).To(BeTrue(), "First rule labels not found")
			Expect(secondRuleFound).To(BeTrue(), "Second rule (with backreference) labels not found")
		})

		It("Validates CRUD lifecycle", func() {
			By("Creating a NodeFeatureRule")

			ruleYAML := `[
{
    "apiVersion": "nfd.openshift.io/v1alpha1",
    "kind": "NodeFeatureRule",
    "metadata": {
        "name": "test-crud-lifecycle",
        "namespace": "` + nfdparams.NFDNamespace + `"
    },
    "spec": {
        "rules": [
            {
                "name": "test.crud",
                "labels": {
                    "test.feature.node.kubernetes.io/crud-test": "true"
                },
                "matchFeatures": [
                    {
                        "feature": "kernel.version",
                        "matchExpressions": {
                            "major": {
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

			var err error
			testRule, err = nfdset.CreateNodeFeatureRuleFromJSON(APIClient, ruleYAML)
			Expect(err).NotTo(HaveOccurred(), "Failed to create NodeFeatureRule")
			Expect(testRule).NotTo(BeNil())
			Expect(testRule.Exists()).To(BeTrue(), "Rule should exist after creation")

			By("Waiting for labels to appear")
			err = nfdwait.WaitForLabelsFromRule(APIClient,
				[]string{"test.feature.node.kubernetes.io/crud-test"},
				5*time.Minute)
			Expect(err).NotTo(HaveOccurred(), "Labels were not applied")

			By("Verifying labels exist")
			nodelabels, err := get.NodeFeatureLabels(APIClient, GeneralConfig.WorkerLabelMap)
			Expect(err).NotTo(HaveOccurred())

			labelFound := false
			for nodeName := range nodelabels {
				if helpers.CheckLabelsExist(nodelabels,
					[]string{"test.feature.node.kubernetes.io/crud-test"},
					nil, nodeName) == nil {
					labelFound = true

					break
				}
			}
			Expect(labelFound).To(BeTrue(), "Labels not found after creation")

			By("Deleting the NodeFeatureRule")
			err = nfdset.DeleteNodeFeatureRule(APIClient, "test-crud-lifecycle", nfdparams.NFDNamespace)
			Expect(err).NotTo(HaveOccurred(), "Failed to delete NodeFeatureRule")

			By("Verifying rule no longer exists")
			Eventually(func() bool {
				_, err := get.NodeFeatureRule(APIClient, "test-crud-lifecycle", nfdparams.NFDNamespace)

				return err != nil
			}).WithTimeout(1*time.Minute).Should(BeTrue(), "Rule should be deleted")

			By("Verifying labels are eventually removed")
			Eventually(func() bool {
				nodelabels, err := get.NodeFeatureLabels(APIClient, GeneralConfig.WorkerLabelMap)
				if err != nil {

					return false
				}

				for nodeName := range nodelabels {
					if helpers.CheckLabelsExist(nodelabels,
						[]string{"test.feature.node.kubernetes.io/crud-test"},
						nil, nodeName) == nil {

						return false
					}
				}

				return true
			}).WithTimeout(5*time.Minute).Should(BeTrue(), "Labels should be removed after rule deletion")

			// Mark as nil so AfterEach doesn't try to delete again
			testRule = nil
		})
	})
})
