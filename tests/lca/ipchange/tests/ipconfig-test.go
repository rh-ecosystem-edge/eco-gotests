package ipchange_test

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/lca"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/network"
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
			Entry("Validates input for the ipv4.gateway field in the IPConfig spec", reportxml.ID("88107"),
				fmt.Sprintf("spec.ipv4.gateway: Invalid value: \"%s\": spec.ipv4.gateway "+
					"in body must be of type ipv4: \"%s\"", tsparams.BadIPv4Address, tsparams.BadIPv4Address),
				func(builder *lca.IPConfigBuilder) error {
					if builder.Definition.Spec.IPv4 == nil {
						builder.Definition.Spec.IPv4 = &lcaipcv1.IPv4Config{}
					}

					builder.Definition.Spec.IPv4.Gateway = tsparams.BadIPv4Address
					_, err := builder.Update()

					return err
				}),
			Entry("Validates input for the ipv4.machineNetwork field in the IPConfig spec", reportxml.ID("88089"),
				fmt.Sprintf("spec.ipv4.machineNetwork: Invalid value: \"%s\": spec.ipv4.machineNetwork "+
					"in body must be of type cidr: \"%s\"", tsparams.BadIPv4Address, tsparams.BadIPv4Address),
				func(builder *lca.IPConfigBuilder) error {
					if builder.Definition.Spec.IPv4 == nil {
						builder.Definition.Spec.IPv4 = &lcaipcv1.IPv4Config{}
					}

					builder.Definition.Spec.IPv4.MachineNetwork = tsparams.BadIPv4Address
					_, err := builder.Update()

					return err
				}),
			Entry("Validates input for dnsServers list entry in the IPConfig spec", reportxml.ID("88106"),
				"spec.dnsServers[0]: Invalid value: \""+string(tsparams.BadIPv4Address)+"\": must be a valid IP address",
				func(builder *lca.IPConfigBuilder) error {
					builder.Definition.Spec.DNSServers = []lcaipcv1.IPAddress{tsparams.BadIPv4Address}

					_, err := builder.Update()

					return err
				}),
		)

		It("Validates AutoRollbackOnFailure works as expected", reportxml.ID("88105"), func() {
			By("Pulling IPConfig")

			builder, err := lca.PullIPConfig(APIClient)
			Expect(err).NotTo(HaveOccurred(), "failed to pull IPConfig")
			Expect(builder).NotTo(BeNil(), "IPConfig builder is nil")

			By("Updating AutoRollbackOnFailure")

			builder.Definition.Spec.AutoRollbackOnFailure = &lcaipcv1.AutoRollbackOnFailure{
				InitMonitorTimeoutSeconds: 9,
			}

			By("Updating IP address fields with arbitrary values to ensure the spec is updated")

			if clusterHasIPv4Network() {
				builder.WithIPv4Address("192.168.250.250")
				builder.WithIPv4Gateway("192.168.250.254")
				builder.WithIPv4MachineNetwork("192.168.250.0/24")
			} else {
				builder.WithIPv6Address("2001:db8::2")
				builder.WithIPv6Gateway("2001:db8::1")
				builder.WithIPv6MachineNetwork("2001:db8::/64")
			}

			By("Setting the stage to Config")

			builder.WithStage(string(lcaipcv1.IPStages.Config))

			By("Updating IPConfig")

			_, err = builder.Update()
			Expect(err).NotTo(HaveOccurred(), "failed to update AutoRollbackOnFailure")

			By("Waiting for IPConfig to fail")

			builder, err = builder.WaitUntilFailed(time.Minute * 15)
			Expect(err).NotTo(HaveOccurred(), "failed to wait for IPConfig to rollback")

			By("Checking the IPConfig status to contain the rollback message")

			Expect(builder.Object.Status.Conditions).To(ContainElement(
				HaveField("Message", ContainSubstring("Rollback due to LCA Init Monitor timeout")),
			))

			By("Waiting for IPConfig to allow Idle stage")

			Eventually(func() (bool, error) {
				builder, err = lca.PullIPConfig(APIClient)
				if err != nil {
					return false, err
				}

				return builder.Object.Status.ValidNextStages[0] == "Idle", nil
			}).WithTimeout(time.Minute*3).WithPolling(time.Second*10).Should(
				BeTrue(), "error waiting for ipconfig to allow Idle stage")

			By("Move the stage back to Idle")

			builder.WithStage(string(lcaipcv1.IPStages.Idle))

			By("Updating IPConfig")

			_, err = builder.Update()
			Expect(err).NotTo(HaveOccurred(), "failed to set stage to Idle for IPConfig")

			By("Waiting for IPConfig to become Idle")

			_, err = builder.WaitUntilIdle(time.Minute * 5)
			Expect(err).NotTo(HaveOccurred(), "failed to wait for IPConfig to become Idle")
		})
	})

// clusterHasIPv4Network returns true if the cluster has at least one IPv4 cluster network CIDR
// configured in the cluster-level Network config (config.openshift.io/v1).
func clusterHasIPv4Network() bool {
	netConfig, err := network.PullConfig(APIClient)
	Expect(err).NotTo(HaveOccurred(), "failed to pull cluster network config")

	for _, clusterNet := range netConfig.Object.Spec.ClusterNetwork {
		if !strings.Contains(clusterNet.CIDR, ":") {
			return true
		}
	}

	return false
}
