package vcore_system_test

import (
	"fmt"
	"os"
	"runtime"
	"testing"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/remote"
	"k8s.io/klog/v2"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/reporter"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/vcore/internal/vcoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/vcore/internal/vcoreparams"
	_ "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/vcore/tests"
)

var _, currentFile, _, _ = runtime.Caller(0)

func TestVCore(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = VCoreConfig.GetJunitReportPath(currentFile)

	RegisterFailHandler(Fail)
	RunSpecs(t, "vCore SystemTests Suite", Label(vcoreparams.Labels...), reporterConfig)
}

var _ = BeforeSuite(func() {
	By(fmt.Sprintf("Create the folder %s for eco-gotests container", vcoreparams.ConfigurationFolderPath))

	if err := os.Mkdir(vcoreparams.ConfigurationFolderPath, 0755); os.IsExist(err) {
		klog.V(vcoreparams.VCoreLogLevel).Infof("%s folder already exists", vcoreparams.ConfigurationFolderPath)
	}

	By(fmt.Sprintf("Asserting the folder %s exists on host %s",
		vcoreparams.ConfigurationFolderPath, VCoreConfig.Host))

	execCmd := fmt.Sprintf("mkdir %s", vcoreparams.ConfigurationFolderPath)
	_, err := remote.ExecCmdOnHost(VCoreConfig.Host, VCoreConfig.User, VCoreConfig.Pass, execCmd)

	if err != nil {
		klog.V(vcoreparams.VCoreLogLevel).Infof("folder %s already exists",
			vcoreparams.ConfigurationFolderPath)
	}

	execCmd = fmt.Sprintf("chmod 755 %s", vcoreparams.ConfigurationFolderPath)
	_, err = remote.ExecCmdOnHost(VCoreConfig.Host, VCoreConfig.User, VCoreConfig.Pass, execCmd)

	if err != nil {
		klog.V(vcoreparams.VCoreLogLevel).Infof("failed to change permitions for the folder %s",
			vcoreparams.ConfigurationFolderPath)
	}
})

var _ = AfterSuite(func() {
	By(fmt.Sprintf("Deleting the folder %s", vcoreparams.ConfigurationFolderPath))

	execCmd := fmt.Sprintf("rm -rf %s", vcoreparams.ConfigurationFolderPath)
	_, err := remote.ExecCmdOnHost(VCoreConfig.Host, VCoreConfig.User, VCoreConfig.Pass, execCmd)

	if err != nil {
		klog.V(vcoreparams.VCoreLogLevel).Infof("folder %s already removed",
			vcoreparams.ConfigurationFolderPath)
	}
})

var _ = JustAfterEach(func() {
	reporter.ReportIfFailed(
		CurrentSpecReport(), currentFile, vcoreparams.ReporterNamespacesToDump, vcoreparams.ReporterCRDsToDump)
})

var _ = ReportAfterSuite("", func(report Report) {
	reportxml.Create(
		report, VCoreConfig.GetReportPath(), VCoreConfig.TCPrefix)
})
