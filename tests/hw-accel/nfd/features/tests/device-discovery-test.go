package tests

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/features/internal/helpers"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/internal/get"
	nfdset "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/internal/set"
	nfdwait "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/internal/wait"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/nfdparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	"k8s.io/klog/v2"
)

var _ = Describe("NFD Device Discovery", Label("device-discovery"), func() {
	Context("Hardware Device Detection", func() {

		It("Discovers PCI devices", func() {
			By("Creating NodeFeatureRule for PCI device discovery")

			// PCI device discovery rule - detects all PCI devices
			pciRuleYAML := `
[{
    "apiVersion": "nfd.openshift.io/v1alpha1",
    "kind": "NodeFeatureRule",
    "metadata": {
        "name": "test-pci-discovery",
        "namespace": "` + nfdparams.NFDNamespace + `"
    },
    "spec": {
        "rules": [
            {
                "name": "test.pci.device",
                "labels": {
                    "test.feature.node.kubernetes.io/pci-present": "true"
                },
                "matchFeatures": [
                    {
                        "feature": "pci.device",
                        "matchExpressions": {
                            "vendor": {
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

			testRule, err := nfdset.CreateNodeFeatureRuleFromJSON(APIClient, pciRuleYAML)
			Expect(err).NotTo(HaveOccurred(), "Failed to create PCI discovery rule")
			Expect(testRule).NotTo(BeNil())

			DeferCleanup(func() {
				if testRule != nil && testRule.Exists() {
					_, _ = testRule.Delete()
				}
			})

			By("Waiting for PCI device labels to appear")
			err = nfdwait.WaitForLabelsFromRule(APIClient,
				[]string{"test.feature.node.kubernetes.io/pci-present"},
				5*time.Minute)
			Expect(err).NotTo(HaveOccurred(), "PCI device labels not found")

			By("Verifying PCI device labels exist")
			nodelabels, err := get.NodeFeatureLabels(APIClient, GeneralConfig.WorkerLabelMap)
			Expect(err).NotTo(HaveOccurred())

			labelFound := false
			for nodeName := range nodelabels {
				if helpers.CheckLabelsExist(nodelabels,
					[]string{"test.feature.node.kubernetes.io/pci-present"},
					nil, nodeName) == nil {
					klog.V(nfdparams.LogLevel).Infof("Node %s has PCI device labels", nodeName)
					labelFound = true

					break
				}
			}
			Expect(labelFound).To(BeTrue(), "PCI devices should be detected on at least one node")
		})

		It("Discovers network device features", func() {
			By("Creating NodeFeatureRule for network device detection")

			networkRuleYAML := `
[{
    "apiVersion": "nfd.openshift.io/v1alpha1",
    "kind": "NodeFeatureRule",
    "metadata": {
        "name": "test-network-discovery",
        "namespace": "` + nfdparams.NFDNamespace + `"
    },
    "spec": {
        "rules": [
            {
                "name": "test.network.device",
                "labels": {
                    "test.feature.node.kubernetes.io/network-present": "true"
                },
                "matchFeatures": [
                    {
                        "feature": "network.device",
                        "matchExpressions": {
                            "name": {
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

			testRule, err := nfdset.CreateNodeFeatureRuleFromJSON(APIClient, networkRuleYAML)
			Expect(err).NotTo(HaveOccurred(), "Failed to create network discovery rule")
			Expect(testRule).NotTo(BeNil())

			DeferCleanup(func() {
				if testRule != nil && testRule.Exists() {
					_, _ = testRule.Delete()
				}
			})

			By("Waiting for network device labels")
			err = nfdwait.WaitForLabelsFromRule(APIClient,
				[]string{"test.feature.node.kubernetes.io/network-present"},
				3*time.Minute)
			Expect(err).NotTo(HaveOccurred(), "Network device labels should be present")
		})

		It("Discovers system features (OS, kernel)", func() {
			By("Creating NodeFeatureRule for system feature detection")

			systemRuleYAML := `
[{
    "apiVersion": "nfd.openshift.io/v1alpha1",
    "kind": "NodeFeatureRule",
    "metadata": {
        "name": "test-system-discovery",
        "namespace": "` + nfdparams.NFDNamespace + `"
    },
    "spec": {
        "rules": [
            {
                "name": "test.system.os",
                "labels": {
                    "test.feature.node.kubernetes.io/os-linux": "true"
                },
                "matchFeatures": [
                    {
                        "feature": "system.osRelease",
                        "matchExpressions": {
                            "ID": {
                                "op": "Exists"
                            }
                        }
                    }
                ]
            },
            {
                "name": "test.system.kernel",
                "labels": {
                    "test.feature.node.kubernetes.io/kernel-detected": "true"
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

			testRule, err := nfdset.CreateNodeFeatureRuleFromJSON(APIClient, systemRuleYAML)
			Expect(err).NotTo(HaveOccurred(), "Failed to create system discovery rule")
			Expect(testRule).NotTo(BeNil())

			DeferCleanup(func() {
				if testRule != nil && testRule.Exists() {
					_, _ = testRule.Delete()
				}
			})

			By("Waiting for system feature labels")
			err = nfdwait.WaitForLabelsFromRule(APIClient,
				[]string{"test.feature.node.kubernetes.io/os-linux",
					"test.feature.node.kubernetes.io/kernel-detected"},
				5*time.Minute)
			Expect(err).NotTo(HaveOccurred(), "System feature labels should always be present")

			By("Verifying system labels exist on all nodes")
			nodelabels, err := get.NodeFeatureLabels(APIClient, GeneralConfig.WorkerLabelMap)
			Expect(err).NotTo(HaveOccurred())

			for nodeName := range nodelabels {
				err = helpers.CheckLabelsExist(nodelabels,
					[]string{"test.feature.node.kubernetes.io/os-linux"},
					nil, nodeName)
				Expect(err).NotTo(HaveOccurred(), "OS labels should exist on all nodes")
			}
		})
	})
})
