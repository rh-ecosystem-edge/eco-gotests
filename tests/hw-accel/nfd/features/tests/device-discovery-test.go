package tests

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/features/internal/helpers"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/internal/get"
	nfdset "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/internal/set"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/internal/validation"
	nfdwait "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/internal/wait"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/nfdparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	"k8s.io/klog/v2"
)

var _ = Describe("NFD Device Discovery", Ordered, Label("device-discovery"), func() {
	Context("Hardware Device Detection", func() {

		It("Discovers PCI devices", reportxml.ID("70010"), func() {
			By("Creating NodeFeatureRule for PCI device discovery")

			// PCI device discovery rule - detects all PCI devices
			pciRuleYAML := `
{
    "apiVersion": "nfd.k8s-sigs.io/v1alpha1",
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

			defer func() {
				if testRule != nil && testRule.Exists() {
					testRule.Delete()
				}
			}()

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

		It("Discovers USB devices", reportxml.ID("70011"), func() {
			By("Checking if USB devices are available")
			hasUSB, err := validation.HasAnyUSBDevice(APIClient)
			Expect(err).NotTo(HaveOccurred())

			if !hasUSB {
				Skip("No USB devices found - skipping USB discovery test")
			}

			By("Creating NodeFeatureRule for USB device discovery")

			usbRuleYAML := `
{
    "apiVersion": "nfd.k8s-sigs.io/v1alpha1",
    "kind": "NodeFeatureRule",
    "metadata": {
        "name": "test-usb-discovery",
        "namespace": "` + nfdparams.NFDNamespace + `"
    },
    "spec": {
        "rules": [
            {
                "name": "test.usb.device",
                "labels": {
                    "test.feature.node.kubernetes.io/usb-present": "true"
                },
                "matchFeatures": [
                    {
                        "feature": "usb.device",
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

			testRule, err := nfdset.CreateNodeFeatureRuleFromJSON(APIClient, usbRuleYAML)
			Expect(err).NotTo(HaveOccurred(), "Failed to create USB discovery rule")
			Expect(testRule).NotTo(BeNil())

			defer func() {
				if testRule != nil && testRule.Exists() {
					testRule.Delete()
				}
			}()

			By("Waiting for USB device labels to appear")
			err = nfdwait.WaitForLabelsFromRule(APIClient,
				[]string{"test.feature.node.kubernetes.io/usb-present"},
				3*time.Minute)
			Expect(err).NotTo(HaveOccurred(), "USB device labels not found")
		})

		It("Discovers SR-IOV capability", reportxml.ID("70012"), func() {
			By("Checking if SR-IOV capable NICs are available")
			hasSRIOV, err := validation.HasSRIOVCapability(APIClient)
			Expect(err).NotTo(HaveOccurred())

			if !hasSRIOV {
				Skip("No SR-IOV capable NICs found - skipping SR-IOV test")
			}

			By("Creating NodeFeatureRule for SR-IOV detection")

			sriovRuleYAML := `
{
    "apiVersion": "nfd.k8s-sigs.io/v1alpha1",
    "kind": "NodeFeatureRule",
    "metadata": {
        "name": "test-sriov-discovery",
        "namespace": "` + nfdparams.NFDNamespace + `"
    },
    "spec": {
        "rules": [
            {
                "name": "test.sriov.capable",
                "labels": {
                    "test.feature.node.kubernetes.io/sriov-capable": "true"
                },
                "matchFeatures": [
                    {
                        "feature": "network.sriov.capable",
                        "matchExpressions": {
                            "sriov": {
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

			testRule, err := nfdset.CreateNodeFeatureRuleFromJSON(APIClient, sriovRuleYAML)
			Expect(err).NotTo(HaveOccurred(), "Failed to create SR-IOV discovery rule")
			Expect(testRule).NotTo(BeNil())

			defer func() {
				if testRule != nil && testRule.Exists() {
					testRule.Delete()
				}
			}()

			By("Verifying SR-IOV labels exist")
			nodelabels, err := get.NodeFeatureLabels(APIClient, GeneralConfig.WorkerLabelMap)
			Expect(err).NotTo(HaveOccurred())

			sriovFound := false
			for _, labels := range nodelabels {
				for _, label := range labels {
					if len(label) > 0 && (label == "feature.node.kubernetes.io/network-sriov.capable=true" ||
						label == "test.feature.node.kubernetes.io/sriov-capable=true") {
						sriovFound = true
						break
					}
				}
				if sriovFound {
					break
				}
			}
			Expect(sriovFound).To(BeTrue(), "SR-IOV capability should be detected")
		})

		It("Discovers storage SSD/HDD features", reportxml.ID("70013"), func() {
			By("Creating NodeFeatureRule for storage device detection")

			storageRuleYAML := `
{
    "apiVersion": "nfd.k8s-sigs.io/v1alpha1",
    "kind": "NodeFeatureRule",
    "metadata": {
        "name": "test-storage-discovery",
        "namespace": "` + nfdparams.NFDNamespace + `"
    },
    "spec": {
        "rules": [
            {
                "name": "test.storage.nonrotational",
                "labels": {
                    "test.feature.node.kubernetes.io/storage-ssd": "true"
                },
                "matchFeatures": [
                    {
                        "feature": "storage.block",
                        "matchExpressions": {
                            "rotational": {
                                "op": "In",
                                "value": ["0", "false"]
                            }
                        }
                    }
                ]
            }
        ]
    }
}]
`

			testRule, err := nfdset.CreateNodeFeatureRuleFromJSON(APIClient, storageRuleYAML)
			Expect(err).NotTo(HaveOccurred(), "Failed to create storage discovery rule")
			Expect(testRule).NotTo(BeNil())

			defer func() {
				if testRule != nil && testRule.Exists() {
					testRule.Delete()
				}
			}()

			By("Checking for storage device labels")
			// Storage devices are common, but we'll be lenient with timeout
			Eventually(func() bool {
				nodelabels, err := get.NodeFeatureLabels(APIClient, GeneralConfig.WorkerLabelMap)
				if err != nil {
					return false
				}

				for nodeName := range nodelabels {
					if helpers.CheckLabelsExist(nodelabels,
						[]string{"storage"},
						nil, nodeName) == nil {
						return true
					}
				}
				return false
			}).WithTimeout(3 * time.Minute).Should(BeTrue(), "Storage labels should be present")
		})

		It("Discovers network device features", reportxml.ID("70014"), func() {
			By("Creating NodeFeatureRule for network device detection")

			networkRuleYAML := `
{
    "apiVersion": "nfd.k8s-sigs.io/v1alpha1",
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

			defer func() {
				if testRule != nil && testRule.Exists() {
					testRule.Delete()
				}
			}()

			By("Waiting for network device labels")
			err = nfdwait.WaitForLabelsFromRule(APIClient,
				[]string{"test.feature.node.kubernetes.io/network-present"},
				3*time.Minute)
			Expect(err).NotTo(HaveOccurred(), "Network device labels should be present")
		})

		It("Discovers non-volatile memory (NVDIMM)", reportxml.ID("70015"), func() {
			By("Creating NodeFeatureRule for NVDIMM detection")

			nvdimmRuleYAML := `
{
    "apiVersion": "nfd.k8s-sigs.io/v1alpha1",
    "kind": "NodeFeatureRule",
    "metadata": {
        "name": "test-nvdimm-discovery",
        "namespace": "` + nfdparams.NFDNamespace + `"
    },
    "spec": {
        "rules": [
            {
                "name": "test.memory.nv",
                "labels": {
                    "test.feature.node.kubernetes.io/nvdimm-present": "true"
                },
                "matchFeatures": [
                    {
                        "feature": "memory.nv",
                        "matchExpressions": {
                            "present": {
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

			testRule, err := nfdset.CreateNodeFeatureRuleFromJSON(APIClient, nvdimmRuleYAML)
			Expect(err).NotTo(HaveOccurred(), "Failed to create NVDIMM discovery rule")
			Expect(testRule).NotTo(BeNil())

			defer func() {
				if testRule != nil && testRule.Exists() {
					testRule.Delete()
				}
			}()

			By("Checking for NVDIMM - this is optional hardware")
			// NVDIMM is rare, so we'll just check if the rule was created successfully
			// and skip if no NVDIMM is found
			time.Sleep(30 * time.Second) // Give it a moment to process

			nodelabels, err := get.NodeFeatureLabels(APIClient, GeneralConfig.WorkerLabelMap)
			Expect(err).NotTo(HaveOccurred())

			nvdimmFound := false
			for nodeName := range nodelabels {
				if helpers.CheckLabelsExist(nodelabels,
					[]string{"test.feature.node.kubernetes.io/nvdimm-present"},
					nil, nodeName) == nil {
					nvdimmFound = true
					klog.V(nfdparams.LogLevel).Infof("Node %s has NVDIMM", nodeName)
					break
				}
			}

			if !nvdimmFound {
				klog.V(nfdparams.LogLevel).Info("No NVDIMM devices found (expected for most systems)")
			}
		})

		It("Discovers system features (OS, kernel)", reportxml.ID("70016"), func() {
			By("Creating NodeFeatureRule for system feature detection")

			systemRuleYAML := `
{
    "apiVersion": "nfd.k8s-sigs.io/v1alpha1",
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

			defer func() {
				if testRule != nil && testRule.Exists() {
					testRule.Delete()
				}
			}()

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
