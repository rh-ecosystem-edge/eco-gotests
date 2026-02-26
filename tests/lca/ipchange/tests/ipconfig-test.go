package ipchange_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/lca"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	lcaipcv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ipchange/api/ipconfig/v1"

	//nolint:staticcheck
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/lca/ipchange/internal/ipcinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/ipchange/internal/tsparams"
)

var _ = Describe(
	"IPConfig validation",
	Ordered,
	Label(tsparams.LabelSuite),
	func() {
		BeforeAll(func() {
			if APIClient == nil {
				Skip("APIClient is nil")
			}

			ipcBuilder, err := lca.PullIPConfig(APIClient)
			Expect(err).NotTo(HaveOccurred(), "failed to pull IPConfig")
			Expect(ipcBuilder).NotTo(BeNil(), "ipconfig builder is nil")

			if ipcBuilder.Definition.Spec.Stage != "Idle" {
				Skip("Stage is not Idle")
			}
		})

		DescribeTable("validates IPConfig spec fields",
			func(expectedSubstr string, updateFunc func(*lca.IPConfigBuilder) error) {
				ipcBuilder, err := lca.PullIPConfig(APIClient)
				Expect(err).NotTo(HaveOccurred(), "failed to pull IPConfig")
				Expect(ipcBuilder).NotTo(BeNil(), "IPConfig builder is nil")

				updateErr := updateFunc(ipcBuilder)
				Expect(updateErr).To(HaveOccurred())
				Expect(updateErr.Error()).To(ContainSubstring(expectedSubstr))
			},
			Entry("Validates input for the Stage field in the IPConfig spec", reportxml.ID("88085"),
				"spec.stage: Unsupported value: \"Config2\": "+
					"supported values: \"Idle\", \"Config\", \"Rollback\"",
				func(builder *lca.IPConfigBuilder) error {
					_, err := builder.WithStage("Config2").Update()

					return err
				}),
			Entry("Validates input for the vlanID field in the IPConfig spec", reportxml.ID("88088"),
				"spec.vlanID: Invalid value: 5700: spec.vlanID in body should be less than or equal to 4095",
				func(builder *lca.IPConfigBuilder) error {
					_, err := builder.WithVlanID(5700).Update()

					return err
				}),
			Entry("Validates input for the autoRollbackOnFailure field in the IPConfig spec", reportxml.ID("88090"),
				"Invalid value: -1: spec.autoRollbackOnFailure.initMonitorTimeoutSeconds in "+
					"body should be greater than or equal to 0",
				func(builder *lca.IPConfigBuilder) error {
					_, err := builder.WithAutoRollbackOnFailure(-1).Update()

					return err
				}),
			Entry("Validates input for the ipv4.address field in the IPConfig spec", reportxml.ID("88108"),
				fmt.Sprintf("spec.ipv4.address: Invalid value: \"%s\": spec.ipv4.address "+
					"in body must be of type ipv4: \"%s\"", tsparams.BadIPv4Address, tsparams.BadIPv4Address),
				func(builder *lca.IPConfigBuilder) error {
					if builder.Definition.Spec.IPv4 == nil {
						builder.Definition.Spec.IPv4 = &lcaipcv1.IPv4Config{}
					}

					builder.Definition.Spec.IPv4.Address = tsparams.BadIPv4Address
					_, err := builder.Update()

					return err
				}),
		)
	})
