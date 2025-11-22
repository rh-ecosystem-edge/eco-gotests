package tests

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/mco"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/gitopsztp/internal/tsparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"k8s.io/klog/v2"
)

var _ = Describe("ZTP Machine Config Tests", Label(tsparams.LabelMachineConfigTestCases), func() {
	// 54239 - Annotation on generated CRs for traceability
	It("should find the ztp annotation present in the machine configs", reportxml.ID("54239"), func() {
		machineConfigsToCheck := []string{
			"container-mount-namespace-and-kubelet-conf-master",
			"container-mount-namespace-and-kubelet-conf-worker",
		}

		By("checking all machine configs for ones deployed by ztp")
		machineConfigs, err := mco.ListMC(Spoke1APIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to list machine configs")

		for _, machineConfigToCheck := range machineConfigsToCheck {
			checked := false

			for _, machineConfig := range machineConfigs {
				if !strings.Contains(machineConfig.Object.Name, machineConfigToCheck) {
					continue
				}

				klog.V(tsparams.LogLevel).Infof(
					"Checking mc '%s' for annotation '%s'", machineConfig.Object.Name, tsparams.ZtpGeneratedAnnotation)

				annotation, ok := machineConfig.Object.Annotations[tsparams.ZtpGeneratedAnnotation]
				Expect(ok).To(BeTrue(), "Failed to find ZTP generated annotation")
				Expect(annotation).To(Equal("{}"), "ZTP generated annotation had the wrong val")

				checked = true
			}

			Expect(checked).To(BeTrue(), "Unable to find machine config containing %s", machineConfigToCheck)
		}
	})
})
