package nfd

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/internal/deploy"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/features/internal/tsparams"
	_ "github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/features/tests"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/internal/nfdconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/internal/nfdhelpers"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/hw-accel/nfd/nfdparams"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/reporter"
	"k8s.io/klog/v2"
)

var _, currentFile, _, _ = runtime.Caller(0)

var NFDInstaller *deploy.OperatorInstaller
var NFDCRUtils *deploy.NFDCRUtils

func TestFeatures(t *testing.T) {
	_, reporterConfig := GinkgoConfiguration()
	reporterConfig.JUnitReport = GeneralConfig.GetJunitReportPath(currentFile)

	RegisterFailHandler(Fail)

	tsparams.Labels = append(tsparams.Labels, "feature")
	RunSpecs(t, "NFD", Label(tsparams.Labels...), reporterConfig)
}

var _ = BeforeSuite(func() {
	nfdConfig := nfdconfig.NewNfdConfig()

	By("Installing NFD operator for all feature tests")
	var options *nfdhelpers.NFDInstallConfigOptions
	if nfdConfig.CatalogSource != "" {
		options = &nfdhelpers.NFDInstallConfigOptions{
			CatalogSource: nfdhelpers.StringPtr(nfdConfig.CatalogSource),
		}
	}

	installConfig := nfdhelpers.GetDefaultNFDInstallConfig(APIClient, options)
	NFDInstaller = deploy.NewOperatorInstaller(installConfig)
	NFDCRUtils = deploy.NewNFDCRUtils(APIClient, installConfig.Namespace, nfdparams.NfdInstance)

	klog.V(nfdparams.LogLevel).Info("Installing NFD operator")
	err := NFDInstaller.Install()
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("error installing NFD operator: %s", err))

	By("Waiting for NFD operator to be ready")
	ready, err := NFDInstaller.IsReady(5 * time.Minute)
	Expect(err).ToNot(HaveOccurred(), "error waiting for NFD operator readiness")
	Expect(ready).To(BeTrue(), "NFD operator not ready")

	By("Creating NFD CR")
	crConfig := deploy.NFDCRConfig{
		Image:          nfdConfig.Image,
		EnableTopology: true,
	}
	err = NFDCRUtils.DeployNFDCR(crConfig)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("error creating NFD CR: %s", err))

	By("Waiting for NFD CR to be ready")
	crReady, err := NFDCRUtils.IsNFDCRReady(5 * time.Minute)
	Expect(err).ToNot(HaveOccurred(), "error waiting for NFD CR")
	Expect(crReady).To(BeTrue(), "NFD CR not ready")

	klog.V(nfdparams.LogLevel).Info("NFD installation complete - all tests can now run")

	// Make NFDCRUtils available to tests that need CR management
	tsparams.SharedNFDCRUtils = NFDCRUtils
})

var _ = AfterSuite(func() {
	if NFDCRUtils != nil {
		By("Deleting NFD CR")
		err := NFDCRUtils.DeleteNFDCR()
		if err != nil {
			klog.Errorf("Failed to delete NFD CR: %v", err)
		}
	}

	if NFDInstaller != nil {
		By("Uninstalling NFD operator")
		uninstallConfig := nfdhelpers.GetDefaultNFDUninstallConfig(
			APIClient,
			"nfd-operator-group",
			"nfd-subscription")
		nfdUninstaller := deploy.NewOperatorUninstaller(uninstallConfig)
		err := nfdUninstaller.Uninstall()
		if err != nil {
			klog.Errorf("Failed to uninstall NFD operator: %v", err)
		}
	}

	klog.V(nfdparams.LogLevel).Info("NFD cleanup complete")
})

var _ = ReportAfterSuite("", func(report Report) {
	reportxml.Create(
		report, GeneralConfig.GetReportPath(), GeneralConfig.TCPrefix)
})

var _ = JustAfterEach(func() {
	reporter.ReportIfFailed(
		CurrentSpecReport(), currentFile, tsparams.ReporterNamespacesToDump, tsparams.ReporterCRDsToDump)
})
