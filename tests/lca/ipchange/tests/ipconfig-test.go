package ipchange_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/lca"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	lcaipcv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ipchange/api/ipconfig/v1"
	ipcinittools "github.com/rh-ecosystem-edge/eco-gotests/tests/lca/ipchange/internal/ipcinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/ipchange/internal/tsparams"
)

var _ = Describe(
	"IPConfig validation",
	Ordered,
	Label(tsparams.LabelSuite),
	func() {
		BeforeAll(func() {
			if ipcinittools.IPCConfig == nil {
				Skip("IPCConfig is nil")
			}

			if ipcinittools.TargetSNOAPIClient == nil {
				Skip("TargetSNOAPIClient is nil")
			}

			ipcBuilder, err := lca.PullIPConfig(ipcinittools.TargetSNOAPIClient)
			Expect(err).NotTo(HaveOccurred(), "failed to pull IPConfig")
			Expect(ipcBuilder).NotTo(BeNil(), "IPConfig builder is nil")

			if ipcBuilder.Definition.Spec.Stage != "Idle" {
				Skip("Stage is not Idle")
			}

			if ipcinittools.IPCConfig.ExpectedDNSServers != "" {
				dnsEntries := splitCSV(ipcinittools.IPCConfig.ExpectedDNSServers)
				if len(dnsEntries) > 2 {
					Fail("Expected DNS servers must contain at most 2 entries")
				}
			}
		})

		It("Validates input for the Stage field in the IPConfig spec", reportxml.ID("88085"), func() {
			By("Pulling existing IPConfig from the cluster")

			ipcBuilder, err := lca.PullIPConfig(ipcinittools.TargetSNOAPIClient)
			Expect(err).NotTo(HaveOccurred(), "failed to pull IPConfig")
			Expect(ipcBuilder).NotTo(BeNil(), "IPConfig builder is nil")

			By("Setting the Stage field to a bad value")

			_, err = ipcBuilder.WithStage("Config2").Update()
			Expect(err.Error()).To(ContainSubstring(
				"spec.stage: Unsupported value: \"Config2\": "+
					"supported values: \"Idle\", \"Config\", \"Rollback\""),
				"error: Stage field updated with a bad value")
		})

		It("Validates input for the vlanID field in the IPConfig spec", reportxml.ID("88088"), func() {
			By("Pulling existing IPConfig from the cluster")

			ipcBuilder, err := lca.PullIPConfig(ipcinittools.TargetSNOAPIClient)
			Expect(err).NotTo(HaveOccurred(), "failed to pull IPConfig")
			Expect(ipcBuilder).NotTo(BeNil(), "IPConfig builder is nil")

			By("Setting the vlanID field to a bad value")

			_, err = ipcBuilder.WithVlanID(5700).Update()
			Expect(err.Error()).To(ContainSubstring(
				"spec.vlanID: Invalid value: 5700: spec.vlanID in body should be less than or equal to 4095"),
				"error: vlanID field updated with a bad value")
		})

		It("Validates input for the autoRollbackOnFailure field in the IPConfig spec", reportxml.ID("88090"), func() {
			By("Pulling existing IPConfig from the cluster")

			ipcBuilder, err := lca.PullIPConfig(ipcinittools.TargetSNOAPIClient)
			Expect(err).NotTo(HaveOccurred(), "failed to pull IPConfig")
			Expect(ipcBuilder).NotTo(BeNil(), "IPConfig builder is nil")

			By("Setting the autoRollbackOnFailure field to a bad value")

			_, err = ipcBuilder.WithAutoRollbackOnFailure(-1).Update()
			Expect(err.Error()).To(ContainSubstring(
				"Invalid value: -1: spec.autoRollbackOnFailure.initMonitorTimeoutSeconds in "+
					"body should be greater than or equal to 0"),
				"error: autoRollbackOnFailure field updated with a bad value")
		})

		It("Validates input for the ipv4.address field in the IPConfig spec", reportxml.ID("88108"), func() {
			By("Pulling existing IPConfig from the cluster")

			ipcBuilder, err := lca.PullIPConfig(ipcinittools.TargetSNOAPIClient)
			Expect(err).NotTo(HaveOccurred(), "failed to pull IPConfig")
			Expect(ipcBuilder).NotTo(BeNil(), "IPConfig builder is nil")

			By("Ensuring Spec.IPv4 exists (may be nil after Pull)")

			if ipcBuilder.Definition.Spec.IPv4 == nil {
				ipcBuilder.Definition.Spec.IPv4 = &lcaipcv1.IPv4Config{}
			}

			By("Setting the ipv4.address field to a bad value")

			ipcBuilder.Definition.Spec.IPv4.Address = "192.168.130.261"

			By("Update the builder with the invalid IPv4 - should fail")

			_, err = ipcBuilder.Update()
			Expect(err).To(HaveOccurred(), "Update with invalid IPv4 should fail")
			Expect(err.Error()).To(ContainSubstring(
				"spec.ipv4.address: Invalid value: \"192.168.130.261\": spec.ipv4.address "+
					"in body must be of type ipv4: \"192.168.130.261\""),
				"error: Update with invalid IPv4 fails with a wrong error")
		})
	})

func splitCSV(value string) []string {
	if value == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return result
}
