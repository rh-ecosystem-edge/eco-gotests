package operator_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/internal/ztpinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/assisted/ztp/operator/internal/tsparams"
)

var (
	minTimeInSeconds int64 = 600
)
var _ = Describe(
	"AssistedPodsNoRestarts",
	Ordered,
	ContinueOnFailure,
	Label(tsparams.LabelAssistedPodsNoRestartsTestCases), func() {
		When("on MCE 2.0 and above", func() {
			DescribeTable("no restarts for assisted pods",
				func(podName string, getPodName func() *pod.Builder) {
					By("Get the " + podName + " pod")

					podBuilder := getPodName()

					By("Assure at least 10 minutes passed since the " + podName + " pod is UP")

					creationTimeStamp := podBuilder.Definition.GetCreationTimestamp()
					if time.Now().Unix()-creationTimeStamp.Unix() < minTimeInSeconds {
						Skip("Must wait at least " + fmt.Sprintf("%v", minTimeInSeconds) +
							" seconds before running the test")
					}

					By("Assure the " + podName + " pod didn't restart")
					Expect(podBuilder.Object.Status.ContainerStatuses[0].RestartCount).To(Equal(int32(0)),
						"Failed asserting 0 restarts for "+podName+" pod")
				},
				Entry("Assert the assisted-service pod wasn't restarted shortly after creation",
					"assisted-service", ZTPConfig.HubAssistedServicePod, reportxml.ID("56581")),
				Entry("Assert the assisted-image-service pod wasn't restarted shortly after creation",
					"assisted-image-service", ZTPConfig.HubAssistedImageServicePod, reportxml.ID("56582")),
			)

		})
	})
