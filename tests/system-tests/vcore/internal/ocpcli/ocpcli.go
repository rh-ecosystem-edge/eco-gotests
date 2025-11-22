package ocpcli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/remote"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/files"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/platform"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/shell"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/template"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/vcore/internal/vcoreinittools"
	"github.com/walle/targz"
)

// DownloadAndExtractOcBinaryArchive downloads and extracts oc binary archive.
func DownloadAndExtractOcBinaryArchive(apiClient *clients.Settings) error {
	ocBinaryMirror := "https://mirror.openshift.com/pub/openshift-v4/x86_64/clients/ocp"
	localTarArchiveName := "openshift-client-linux.tar.gz"
	tempDir := os.TempDir()

	ocPath := filepath.Join(tempDir, "oc")

	klog.V(100).Info("Check if oc binary already downloaded locally")

	if stat, err := os.Stat(ocPath); err == nil && stat.Name() != "" {
		klog.V(100).Info("oc binary was found, need to be deleted")

		err = os.Remove(ocPath)
		if err != nil {
			return fmt.Errorf("failed to remove %s", ocPath)
		}
	}

	klog.V(100).Info("install oc binary")

	clusterVersion, err := platform.GetOCPVersion(apiClient)
	if err != nil {
		return err
	}

	ocBinaryURL := fmt.Sprintf("%s/%s/openshift-client-linux-%s.tar.gz",
		ocBinaryMirror, clusterVersion, clusterVersion)

	err = files.DownloadFile(ocBinaryURL, localTarArchiveName, tempDir)
	if err != nil {
		return err
	}

	tarArchiveLocation := filepath.Join(tempDir, localTarArchiveName)

	err = targz.Extract(tarArchiveLocation, tempDir)
	if err != nil {
		return err
	}

	execCmd := fmt.Sprintf("sudo -u root -i cp %s /usr/local/bin/oc", ocPath)

	output, err := shell.ExecuteCmd(execCmd)
	if err != nil {
		return fmt.Errorf("failed to execute %s command due to: %w. \noutput: %v", execCmd, err, output)
	}

	chmodCmd := "sudo -u root -i chmod 755 /usr/local/bin/oc"

	_, err = shell.ExecuteCmd(chmodCmd)
	if err != nil {
		return fmt.Errorf("failed to execute %s command due to: %w", chmodCmd, err)
	}

	return nil
}

// ApplyConfig applies config using shell method.
func ApplyConfig(
	pathToTemplate,
	pathToConfigFile string,
	variablesToReplace map[string]interface{}) error {
	err := template.SaveTemplate(pathToTemplate, pathToConfigFile, variablesToReplace)
	if err != nil {
		return err
	}

	err = remote.ScpFileTo(pathToConfigFile, pathToConfigFile, VCoreConfig.Host, VCoreConfig.User, VCoreConfig.Pass)
	if err != nil {
		return fmt.Errorf("failed to transfer file %s to the %s/%s due to: %w",
			pathToConfigFile, VCoreConfig.Host, pathToConfigFile, err)
	}

	applyCmd := fmt.Sprintf("oc apply -f %s --kubeconfig=%s",
		pathToConfigFile, VCoreConfig.KubeconfigPath)

	_, err = remote.ExecCmdOnHost(VCoreConfig.Host, VCoreConfig.User, VCoreConfig.Pass, applyCmd)
	if err != nil {
		return fmt.Errorf("failed to execute %s command due to: %w", applyCmd, err)
	}

	return nil
}

// CreateConfig creates config using shell method.
func CreateConfig(
	pathToTemplate,
	pathToConfigFile string,
	variablesToReplace map[string]interface{}) error {
	err := template.SaveTemplate(pathToTemplate, pathToConfigFile, variablesToReplace)
	if err != nil {
		return err
	}

	err = remote.ScpFileTo(pathToConfigFile, pathToConfigFile, VCoreConfig.Host, VCoreConfig.User, VCoreConfig.Pass)
	if err != nil {
		return fmt.Errorf("failed to transfer file %s to the %s/%s due to: %w",
			pathToConfigFile, VCoreConfig.Host, pathToConfigFile, err)
	}

	createCmd := fmt.Sprintf("oc create -f %s --kubeconfig=%s", pathToConfigFile, VCoreConfig.KubeconfigPath)

	_, err = remote.ExecCmdOnHost(VCoreConfig.Host, VCoreConfig.User, VCoreConfig.Pass, createCmd)
	if err != nil {
		return fmt.Errorf("failed to execute %s command due to: %w", createCmd, err)
	}

	return nil
}

// PatchAPIObject patches the resource using the given patch type, object belongs to the specific namespace
// The following patches are exactly the same patch but using different types, 'merge' and 'json'
// --type merge -p '{"spec": {"selector": {"app": "frommergepatch"}}}'
// --type json  -p '[{ "op": "replace", "path": "/spec/selector/app", "value": "fromjsonpatch"}]'.
func PatchAPIObject(objName, objNamespace, objKind, patchType, patchStr string) error {
	patchCmd := fmt.Sprintf("oc patch %s/%s --type %s -p '%v'",
		objKind, objName, patchType, patchStr)

	if objNamespace != "" {
		patchCmd = fmt.Sprintf("oc patch %s/%s -n %s --type %s -p '%v' --kubeconfig=%s",
			objKind, objName, objNamespace, patchType, patchStr, VCoreConfig.KubeconfigPath)
	}

	_, err := remote.ExecCmdOnHost(VCoreConfig.Host, VCoreConfig.User, VCoreConfig.Pass, patchCmd)
	if err != nil {
		return fmt.Errorf("failed to execute %s command due to: %w", patchCmd, err)
	}

	return nil
}

// ExecuteViaDebugPodOnNode creates debug pod and executes provided command.
func ExecuteViaDebugPodOnNode(
	nodeName string,
	cmd string) (string, error) {
	execCmd := fmt.Sprintf("oc debug nodes/%s -- bash -c \"chroot /host %s\" "+
		"--insecure-skip-tls-verify --kubeconfig=%s", nodeName, cmd, VCoreConfig.KubeconfigPath)
	klog.V(100).Infof("Execute command %s", execCmd)

	output, err := remote.ExecCmdOnHost(VCoreConfig.Host, VCoreConfig.User, VCoreConfig.Pass, execCmd)
	if err != nil {
		return "", fmt.Errorf("failed to execute %s command due to: %w", execCmd, err)
	}

	return output, nil
}
