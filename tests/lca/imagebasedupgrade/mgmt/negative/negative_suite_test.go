package negative_test

import (
	"runtime"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/lca"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/reporter"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/mgmt/internal/mgmtinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/mgmt/negative/internal/tsparams"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/internal/safeapirequest"
	_ "github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/mgmt/negative/tests"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/internal/seedimage"
)

var _, currentFile, _, _ = runtime.Caller(0)

func TestNegative(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = MGMTConfig.GetJunitReportPath(currentFile)

	RegisterFailHandler(Fail)
	RunSpecs(t, "Negative Suite", Label(tsparams.Labels...), reporterConfig)
}

var _ = BeforeSuite(func() {
	var err error
	seedClusterInfo, err := seedimage.GetContent(APIClient, MGMTConfig.SeedImage)
	Expect(err).NotTo(HaveOccurred(), "error getting seed image info")

	MGMTConfig.SeedClusterInfo = seedClusterInfo
})

var _ = AfterEach(func() {
	By("Pull the imagebasedupgrade from the cluster")
	ibu, err := lca.PullImageBasedUpgrade(APIClient)
	Expect(err).NotTo(HaveOccurred(), "error pulling imagebasedupgrade resource")

	if ibu.Object.Spec.Stage != "Idle" {
		err = safeapirequest.Do(func() error {
			ibu, err = lca.PullImageBasedUpgrade(APIClient)
			if err != nil {
				return err
			}

			_, err = ibu.WithStage("Idle").Update()
			if err != nil {
				return err
			}

			return nil
		})

		Expect(err).NotTo(HaveOccurred(), "error setting ibu to idle stage")

		By("Wait until IBU has become Idle")
		_, err = ibu.WaitUntilStageComplete("Idle")
		Expect(err).NotTo(HaveOccurred(), "error waiting for idle stage to complete")
	}

	Eventually(func() (bool, error) {
		ibu.Object, err = ibu.Get()
		if err != nil {
			return false, err
		}

		return len(ibu.Object.Status.Conditions) == 1 &&
			ibu.Object.Status.Conditions[0].Type == "Idle" &&
			ibu.Object.Status.Conditions[0].Status == "True", nil
	}).WithTimeout(time.Second*60).WithPolling(time.Second*2).Should(
		BeTrue(), "error waiting for image based upgrade to become idle")

	Expect(string(ibu.Object.Spec.Stage)).To(Equal("Idle"), "error: ibu resource contains unexpected state")
})

var _ = ReportAfterSuite("", func(report Report) {
	reportxml.Create(
		report, MGMTConfig.GetReportPath(), MGMTConfig.TCPrefix)
})

var _ = JustAfterEach(func() {
	reporter.ReportIfFailed(
		CurrentSpecReport(),
		currentFile,
		tsparams.ReporterNamespacesToDump,
		tsparams.ReporterCRDsToDump)
})
