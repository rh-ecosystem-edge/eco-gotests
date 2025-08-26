package spoke_test

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/assisted"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/network"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/assisted/api/hiveextension/v1beta1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/assisted/models"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/meets"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/ztpinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/spoke/internal/tsparams"
	corev1 "k8s.io/api/core/v1"
)

var _ = Describe(
	"NetworkType",
	Ordered,
	ContinueOnFailure,
	Label(tsparams.LabelNetworkTypeVerificationTestCases), func() {
		var (
			networkTypeACI string
		)

		When("on MCE 2.0 and above", func() {
			BeforeAll(func() {
				By("Get the networktype from the AgentClusterInstall")
				networkTypeACI = ZTPConfig.SpokeAgentClusterInstall.Object.Spec.Networking.NetworkType
				Expect(networkTypeACI).To(Or(Equal(models.ClusterNetworkTypeOVNKubernetes),
					Equal(models.ClusterNetworkTypeOpenShiftSDN), Equal("")))

			})
			It("Assert IPv4 spoke cluster with OVNKubernetes set as NetworkType gets deployed",
				reportxml.ID("44899"), func() {

					By("Check that spoke cluster is IPV4 Single Stack")
					reqMet, msg := meets.SpokeSingleStackIPv4Requirement()
					if !reqMet {
						Skip(msg)
					}

					By("Check that the networktype in AgentClusterInstall is set properly")
					if networkTypeACI != models.ClusterNetworkTypeOVNKubernetes {
						Skip(fmt.Sprintf("the network type in ACI is not set to %s", models.ClusterNetworkTypeOVNKubernetes))
					}

					By("Check that the deployment of the spoke has completed")
					agentClusterInstallCompleted(ZTPConfig.SpokeAgentClusterInstall)

				})
			It("Assert the NetworkType in the IPV4 spoke matches ACI and is set to OVNKubernetes",
				reportxml.ID("44900"), func() {

					By("Check that spoke cluster is IPV4 Single Stack")
					reqMet, msg := meets.SpokeSingleStackIPv4Requirement()
					if !reqMet {
						Skip(msg)
					}

					By("Check that the networktype in AgentClusterInstall is set properly")
					if networkTypeACI != models.ClusterNetworkTypeOVNKubernetes {
						Skip(fmt.Sprintf("the network type in ACI is not set to %s", models.ClusterNetworkTypeOVNKubernetes))
					}

					By("Get the network config from the spoke")
					spokeClusterNetwork, err := network.PullConfig(SpokeAPIClient)
					Expect(err).ToNot(HaveOccurred(),
						"error pulling network configuration from the spoke")

					By("Assure the networktype in AgentClusterInstall matches the networktype in the spoke")
					Expect(models.ClusterNetworkTypeOVNKubernetes).To(Equal(spokeClusterNetwork.Object.Spec.NetworkType),
						"error matching the network type in agentclusterinstall to the network type in the spoke")
				})
			It("Assert IPv6 spoke cluster with OVNKubernetes set as NetworkType gets deployed",
				reportxml.ID("44894"), func() {

					By("Check that spoke cluster is IPV6 Single Stack")
					reqMet, msg := meets.SpokeSingleStackIPv6Requirement()
					if !reqMet {
						Skip(msg)
					}

					By("Check that the networktype in AgentClusterInstall is set properly")
					if networkTypeACI != models.ClusterNetworkTypeOVNKubernetes {
						Skip(fmt.Sprintf("the network type in ACI is not set to %s", models.ClusterNetworkTypeOVNKubernetes))
					}

					By("Check that the deployment of the spoke has completed")
					agentClusterInstallCompleted(ZTPConfig.SpokeAgentClusterInstall)

				})
			It("Assert the NetworkType in the IPV6 spoke matches ACI and is set to OVNKubernetes",
				reportxml.ID("44895"), func() {

					By("Check that spoke cluster is IPV6 Single Stack")
					reqMet, msg := meets.SpokeSingleStackIPv6Requirement()
					if !reqMet {
						Skip(msg)
					}

					By("Check that the networktype in AgentClusterInstall is set properly")
					if networkTypeACI != models.ClusterNetworkTypeOVNKubernetes {
						Skip(fmt.Sprintf("the network type in ACI is not set to %s", models.ClusterNetworkTypeOVNKubernetes))
					}

					By("Get the network config from the spoke")
					spokeClusterNetwork, err := network.PullConfig(SpokeAPIClient)
					Expect(err).ToNot(HaveOccurred(),
						"error pulling network configuration from the spoke")

					By("Assure the networktype in AgentClusterInstall matches the networktype in the spoke")
					Expect(models.ClusterNetworkTypeOVNKubernetes).To(Equal(spokeClusterNetwork.Object.Spec.NetworkType),
						"error matching the network type in agentclusterinstall to the network type in the spoke")
				})
			It("Assert IPv4 spoke cluster with OpenShiftSDN set as NetworkType gets deployed",
				reportxml.ID("44896"), func() {

					By("Check that spoke cluster is IPV4 Single Stack")
					reqMet, msg := meets.SpokeSingleStackIPv4Requirement()
					if !reqMet {
						Skip(msg)
					}

					By("Check that the networktype in AgentClusterInstall is set properly")
					if networkTypeACI != models.ClusterNetworkTypeOpenShiftSDN {
						Skip(fmt.Sprintf("the network type in ACI is not set to %s", models.ClusterNetworkTypeOpenShiftSDN))
					}

					By("Check that the deployment of the spoke has completed")
					agentClusterInstallCompleted(ZTPConfig.SpokeAgentClusterInstall)

				})
			It("Assert the NetworkType in the IPV4 spoke matches ACI and is set to OpenShiftSDN",
				reportxml.ID("44897"), func() {

					By("Check that spoke cluster is IPV4 Single Stack")
					reqMet, msg := meets.SpokeSingleStackIPv4Requirement()
					if !reqMet {
						Skip(msg)
					}

					By("Check that the networktype in AgentClusterInstall is set properly")
					if networkTypeACI != models.ClusterNetworkTypeOpenShiftSDN {
						Skip(fmt.Sprintf("the network type in ACI is not set to %s", models.ClusterNetworkTypeOpenShiftSDN))
					}

					By("Get the network config from the spoke")
					spokeClusterNetwork, err := network.PullConfig(SpokeAPIClient)
					Expect(err).ToNot(HaveOccurred(),
						"error pulling network configuration from the spoke")

					By("Assure the networktype in AgentClusterInstall matches the networktype in the spoke")
					Expect(models.ClusterNetworkTypeOpenShiftSDN).To(Equal(spokeClusterNetwork.Object.Spec.NetworkType),
						"error matching the network type in agentclusterinstall to the network type in the spoke")
				})
			It("Assert the NetworkType if omitted in ACI is set to OVNKubernetes",
				reportxml.ID("49558"), func() {

					By("Check the networktype is not set via install-config-overrides")
					installConfigOverrides := ZTPConfig.SpokeAgentClusterInstall.
						Object.ObjectMeta.Annotations["agent-install.openshift.io/install-config-overrides"]
					if strings.Contains(installConfigOverrides, models.ClusterNetworkTypeOVNKubernetes) {
						Skip("the network type for spoke is set via install-config-overrides")
					}

					By("Check that the networktype is not set in AgentClusterInstall")
					if networkTypeACI != "" {
						Skip("the network type in ACI is not empty")
					}

					By("Get the network config from the spoke")
					spokeClusterNetwork, err := network.PullConfig(SpokeAPIClient)
					Expect(err).ToNot(HaveOccurred(),
						"error pulling network configuration from the spoke")

					By("Assure the networktype in the spoke is set to OVNKubernetes")
					Expect(models.ClusterNetworkTypeOVNKubernetes).To(Equal(spokeClusterNetwork.Object.Spec.NetworkType),
						"error matching the network type in the spoke to %s", models.ClusterNetworkTypeOVNKubernetes)
				})
		})
	})

func agentClusterInstallCompleted(agentClusterInstallBuilder *assisted.AgentClusterInstallBuilder) {
	err := agentClusterInstallBuilder.WaitForConditionStatus(
		v1beta1.ClusterCompletedCondition, corev1.ConditionTrue, time.Second*5)
	Expect(err).ToNot(HaveOccurred(), "error verifying that the completed condition status is True")
	err = agentClusterInstallBuilder.WaitForConditionReason(
		v1beta1.ClusterCompletedCondition, v1beta1.ClusterInstalledReason, time.Second*5)
	Expect(err).ToNot(HaveOccurred(), "error verifying that the complete condition reason is InstallationCompleted")
}
