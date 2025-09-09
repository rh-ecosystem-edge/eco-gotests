package operator_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/proxy"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/meets"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/ztpinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/operator/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
)

var (
	operatorDeployProxy *proxy.Builder
)

var _ = Describe(
	"Infrastructure operator deployment with proxy enabled",
	Ordered,
	ContinueOnFailure,
	Label(tsparams.LabelInfraOperatorProxyDeploy), func() {
		When("on MCE 2.0 and above", func() {
			BeforeAll(func() {
				By("Check that hub cluster was deployed with a proxy")
				if reqMet, msg := meets.HubProxyConfiguredRequirement(); !reqMet {
					Skip(msg)
				}

				By("Get hub OCP proxy")
				operatorDeployProxy, err = cluster.GetOCPProxy(HubAPIClient)
				Expect(err).NotTo(HaveOccurred(), "error pulling hub ocp proxy")
			})

			DescribeTable("succeeds", func(requirement func() (bool, string)) {
				if reqMet, msg := requirement(); !reqMet {
					Skip(msg)
				}

				operandRunning, msg := meets.HubInfrastructureOperandRunningRequirement()
				Expect(operandRunning).To(BeTrue(), msg)

				if operatorDeployProxy.Object.Status.HTTPProxy != "" {
					validateProxyVar(operatorDeployProxy.Object.Status.HTTPProxy, "HTTP_PROXY")
				}

				if operatorDeployProxy.Object.Status.HTTPSProxy != "" {
					validateProxyVar(operatorDeployProxy.Object.Status.HTTPSProxy, "HTTPS_PROXY")
				}

				if operatorDeployProxy.Object.Status.NoProxy != "" {
					validateProxyVar(operatorDeployProxy.Object.Status.NoProxy, "NO_PROXY")
				}
			},
				Entry("in IPv4 environments", meets.HubSingleStackIPv4Requirement, reportxml.ID("49223")),
				Entry("in IPv6 environments", meets.HubSingleStackIPv6Requirement, reportxml.ID("49226")),
			)
		})
	})

// validateProxyVar checks that clusterProxyVar matches the envVar
// returned from the assisted-service and assisted-image-service pods.
func validateProxyVar(clusterProxyVar, envVar string) {
	By("Check that assisted-service " + envVar + " environment variable matches value set by cluster proxy")
	output, err := ZTPConfig.HubAssistedServicePod().ExecCommand([]string{"printenv", envVar})
	Expect(err).NotTo(HaveOccurred(), "error checking "+envVar+" environment variable")
	Expect(strings.Contains(output.String(), clusterProxyVar)).To(BeTrue(),
		"received incorrect "+envVar+" variable inside assisted-service pod")

	By("Check that assisted-image-service " + envVar + " environment variable matches value set by cluster proxy")
	output, err = ZTPConfig.HubAssistedImageServicePod().ExecCommand([]string{"printenv", envVar})
	Expect(err).NotTo(HaveOccurred(), "error checking "+envVar+" environment variable")
	Expect(strings.Contains(output.String(), clusterProxyVar)).To(BeTrue(),
		"received incorrect "+envVar+" variable inside assisted-image-service pod")
}
