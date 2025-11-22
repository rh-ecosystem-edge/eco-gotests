package systemreporter

import (
	"bytes"
	"fmt"
	"os"
	re "regexp"
	"strings"

	"github.com/onsi/ginkgo/v2/types"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	"golang.org/x/crypto/ssh"
	"k8s.io/klog/v2"
)

var (
	// Matches option hypens, spaces, and special characters.
	specialChars = re.MustCompile(`-?\s-?|[/|'"\.\[\]]`)

	// Matches duplicate underscores.
	dupUnderscores = re.MustCompile(`__+`)

	// Matches leading and trailing underscores.
	leadAndTrailUnderscores = re.MustCompile(`^_|_$`)
)

// ReportIfFailedFromClient dumps the requested command output
// from nodes pulled from specified apiClient if test case fails.
func ReportIfFailedFromClient(
	report types.SpecReport, testSuite string, commands []string, apiClient *clients.Settings) {
	if types.SpecStateFailureStates.Is(report.State) {
		if apiClient == nil {
			klog.Errorf("cannot gather system report from nil apiClient")

			return
		}

		dumpDir := GeneralConfig.GetDumpFailedTestReportLocation(testSuite)

		tcReportFolderName := strings.ReplaceAll(report.FullText(), " ", "_")

		systemFolder := fmt.Sprintf("%s/%s/system", dumpDir, tcReportFolderName)

		err := os.MkdirAll(systemFolder, 0755)
		if err != nil {
			klog.Errorf("failed creating dir for system info: %s", err)

			return
		}

		GatherInfoThroughKubeClient(commands, systemFolder, apiClient)
	}
}

// ReportIfFailedFromNodeList dumps the requested command output from specified nodes through SSH if test case fails.
func ReportIfFailedFromNodeList(report types.SpecReport, testSuite string, commands []string, nodes []string) {
	if types.SpecStateFailureStates.Is(report.State) {
		dumpDir := GeneralConfig.GetDumpFailedTestReportLocation(testSuite)

		tcReportFolderName := strings.ReplaceAll(report.FullText(), " ", "_")

		systemFolder := fmt.Sprintf("%s/%s/system", dumpDir, tcReportFolderName)

		err := os.MkdirAll(systemFolder, 0755)
		if err != nil {
			klog.Errorf("failed to create directory for storing system info %s", err)

			return
		}

		GatherInfoThroughSSH(commands, systemFolder, GeneralConfig.SSHKeyPath, nodes)
	}
}

// GatherInfoThroughSSH gathers command output from specified nodes
// and writes output to specified directory.
func GatherInfoThroughSSH(commands []string, outputdir string, sshKeyPath string, nodes []string) {
	if sshKeyPath == "" {
		klog.Errorf("cannot gather system information without providing ssh key path")

		return
	}

	privateKey, err := os.ReadFile(sshKeyPath)
	if err != nil {
		klog.Errorf("failed to read private ssh key from system: %s", err)

		return
	}

	signer, err := ssh.ParsePrivateKey(privateKey)
	if err != nil {
		klog.Errorf("failed to parse private ssh key: %s", err)

		return
	}

	config := ssh.ClientConfig{
		User:            GeneralConfig.SSHUser,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
	}

	for _, node := range nodes {
		client, err := ssh.Dial("tcp", fmt.Sprintf("%s:22", node), &config)
		if err != nil {
			klog.Errorf("failed to establish SSH connection to %s: %s", node, err)

			continue
		}

		defer client.Close()

		for _, command := range commands {
			var output bytes.Buffer

			var stderr bytes.Buffer

			session, err := client.NewSession()
			if err != nil {
				klog.Errorf("failed to create SSH session on %s: %s", node, err)

				break
			}

			defer session.Close()

			session.Stdout = &output
			session.Stderr = &stderr

			err = session.Run(command)
			if err == nil {
				err = os.WriteFile(outputdir+"/"+node+"_"+fileNameFromCommand(command), output.Bytes(), 0650)
				if err != nil {
					klog.Errorf("error writing to file: %s", err)
				}
			} else {
				klog.Errorf("error executing command '%s' on %s: %s", command, node, stderr.String())
			}
		}
	}
}

// GatherInfoThroughKubeClient gathers command output from nodes accessible from APIClient
// and writes output to specified directory.
func GatherInfoThroughKubeClient(commands []string, outputdir string, apiClient *clients.Settings) {
	if apiClient == nil {
		klog.Errorf("cannot gather system information from nil APIClient")

		return
	}

	for _, command := range commands {
		output, err := cluster.ExecCmdWithStdout(apiClient, command)
		if err != nil {
			klog.Errorf("error occurred while executing command: %s", err)

			continue
		}

		for node, results := range output {
			err = os.WriteFile(outputdir+"/"+node+"_"+fileNameFromCommand(command), []byte(results), 0650)
			if err != nil {
				klog.Errorf("error writing to file: %s", err)
			}
		}
	}
}

func fileNameFromCommand(command string) string {
	fileName := command

	// Replace option hypens, spaces, and special characters with underscores everywhere in the command.
	fileName = specialChars.ReplaceAllString(fileName, "_")

	// Remove repeated underscores
	fileName = dupUnderscores.ReplaceAllString(fileName, "_")

	// Remove leading and trailing underscores
	fileName = leadAndTrailUnderscores.ReplaceAllString(fileName, "")

	return fileName
}
