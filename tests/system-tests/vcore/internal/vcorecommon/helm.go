package vcorecommon

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/remote"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/vcore/internal/vcoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/vcore/internal/vcoreparams"
)

// VerifyHelmSuite container that contains tests for the Helm verification.
func VerifyHelmSuite() {
	Describe(
		"Helm validation",
		Label(vcoreparams.LabelVCoreOperators), func() {
			It("Verify Helm deployment procedure",
				Label("helm"), reportxml.ID("60085"), VerifyHelmDeploymentProcedure)
		})
}

// VerifyHelmDeploymentProcedure asserts Helm deployment procedure.
func VerifyHelmDeploymentProcedure(ctx SpecContext) {
	klog.V(vcoreparams.VCoreLogLevel).Infof("Verify Helm could be installed and works correctly")

	helmScriptURL := "https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3"
	helmScriptName := "get-helm-3"
	helmScriptPath := filepath.Join(vcoreparams.ConfigurationFolderPath, helmScriptName)

	klog.V(vcoreparams.VCoreLogLevel).Infof("Download %s script", helmScriptName)

	downloadCmd := fmt.Sprintf("wget %s -P %s", helmScriptURL, vcoreparams.ConfigurationFolderPath)
	_, err := remote.ExecCmdOnHost(VCoreConfig.Host, VCoreConfig.User, VCoreConfig.Pass, downloadCmd)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to download %s file to the hypervisor from the %s; %v",
		helmScriptName, helmScriptURL, err))

	klog.V(vcoreparams.VCoreLogLevel).Infof("Make %s script executable", helmScriptName)

	chmodCmd := fmt.Sprintf("chmod 700 %s", helmScriptPath)
	_, err = remote.ExecCmdOnHost(VCoreConfig.Host, VCoreConfig.User, VCoreConfig.Pass, chmodCmd)
	Expect(err).ToNot(HaveOccurred(), "failed to make %s script executable due to %w",
		helmScriptPath, err)

	klog.V(vcoreparams.VCoreLogLevel).Info("Install Helm")

	_, err = remote.ExecCmdOnHost(VCoreConfig.Host, VCoreConfig.User, VCoreConfig.Pass, helmScriptPath)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to execute %s script due to %v",
		helmScriptPath, err))

	klog.V(vcoreparams.VCoreLogLevel).Info("Check HELM working properly")

	cmd := "helm version"
	result, err := remote.ExecCmdOnHost(VCoreConfig.Host, VCoreConfig.User, VCoreConfig.Pass, cmd)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("failed to check helm version due %v", err))
	Expect(strings.Contains(result, "version.BuildInfo")).To(Equal(true),
		fmt.Sprintf("Helm was not installed properly; %s", result))
} // func VerifyHelmDeploymentProcedure (ctx SpecContext)
