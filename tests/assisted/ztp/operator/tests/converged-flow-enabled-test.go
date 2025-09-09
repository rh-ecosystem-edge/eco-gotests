package operator_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/ztpinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/operator/internal/tsparams"
)

const (
	assistedContainer = "assisted-service"
)

var (
	convergedFlowLog string
)

var _ = Describe(
	"ConvergedFlowEnabled",
	Ordered,
	ContinueOnFailure,
	Label(tsparams.LabelConvergedFlowEnabled), func() {
		BeforeAll(func() {

			command := []string{"printenv", "ALLOW_CONVERGED_FLOW"}
			convergedFlowVariable, err := ZTPConfig.HubAssistedServicePod().ExecCommand(command, assistedContainer)
			Expect(err.Error()).To(Or(BeEmpty(), Equal("command terminated with exit code 1")),
				"error msg is not as expected")
			if convergedFlowVariable.Len() != 0 {
				Skip("environment variable set not by default")
			}

			By("Registering converged flow status")
			convergedFlowLog, err = ZTPConfig.HubAssistedServicePod().GetFullLog(assistedContainer)
			Expect(err).ToNot(HaveOccurred(), "error occurred when getting log")
		})

		It("Validates that converged flow is enabled by default", reportxml.ID("62628"), func() {

			enabledInLog := strings.Contains(convergedFlowLog, "Converged flow enabled: true")
			Expect(enabledInLog).To(BeTrue(), "environment variable not defined or not in log.")

		})

	})
