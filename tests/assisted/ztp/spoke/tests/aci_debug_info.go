package spoke_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/assisted/models"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/ztpinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/spoke/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/url"
)

var _ = Describe(
	"ACIDebugInfo",
	Ordered,
	ContinueOnFailure,
	Label(tsparams.LabelDebugInfoVerificationTestCases), func() {

		DescribeTable("aci debug info url checks", func(urlToCheck func() string) {
			Expect(urlToCheck()).Should(ContainSubstring("https://"),
				"error verifying that the debug info url in agent cluster install is set")
			_, statusCode, err := url.Fetch(urlToCheck(), "GET", true)
			Expect(err).ToNot(HaveOccurred(), "error retrieving content from url: %s", err)
			Expect(statusCode).To(Equal(200), "error verifying http return code is 200")
		},
			Entry("Logs URL", func() string { return ZTPConfig.SpokeAgentClusterInstall.Object.Status.DebugInfo.LogsURL },
				reportxml.ID("42811")),
			Entry("Events URL", func() string { return ZTPConfig.SpokeAgentClusterInstall.Object.Status.DebugInfo.EventsURL },
				reportxml.ID("42812")),
		)

		It("Assert agent cluster install state and stateInfo params are valid",
			reportxml.ID("42813"), func() {

				By("Check that the DebugInfo state in AgentClusterInstall shows cluster is installed")
				Expect(ZTPConfig.SpokeAgentClusterInstall.Object.Status.DebugInfo.State).
					Should(Equal(models.ClusterStatusAddingHosts),
						"error verifying that DebugInfo state shows that the cluster is installed")

				By("Check that the DebugInfo stateInfo in AgentClusterInstall shows cluster is installed")
				Expect(ZTPConfig.SpokeAgentClusterInstall.Object.Status.DebugInfo.StateInfo).Should(Equal("Cluster is installed"),
					"error verifying that DebugInfo stateInfo shows that the cluster is installed")
			})
	})
