package spoke_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/assisted/models"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/ztpinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/spoke/internal/tsparams"
)

var _ = Describe(
	"AssistedPodsNoRestarts",
	Ordered,
	ContinueOnFailure,
	Label(tsparams.LabelAssistedPodsNoRestartsTestCases), func() {
		When("on MCE 2.0 and above", func() {

			BeforeAll(func() {
				if ZTPConfig.SpokeAgentClusterInstall.Object.Status.DebugInfo.State != models.ClusterStatusAddingHosts {
					Skip("spoke cluster has not been installed")
				}

			})

			DescribeTable("no restarts for assisted pods",
				func(podName string, getPodName func() *pod.Builder) {

					By("Get the " + podName + " pod")
					podBuilder := getPodName()

					By("Assure the " + podName + " pod didn't restart")
					Expect(podBuilder.Object.Status.ContainerStatuses[0].RestartCount).To(Equal(int32(0)),
						"failed asserting 0 restarts for "+podName+" pod")
				},
				Entry("Assert the assisted-service pod wasn't restarted after creation",
					"assisted-service", ZTPConfig.HubAssistedServicePod, reportxml.ID("56584")),
				Entry("Assert the assisted-image-service pod wasn't restarted after creation",
					"assisted-image-service", ZTPConfig.HubAssistedImageServicePod, reportxml.ID("56583")),
			)

		})
	})
