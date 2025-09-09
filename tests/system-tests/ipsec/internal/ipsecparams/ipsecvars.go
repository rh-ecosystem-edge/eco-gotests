package ipsecparams

import (
	"fmt"
	"slices"
	"strings"

	"github.com/openshift-kni/k8sreporter"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/systemtestsparams"
	corev1 "k8s.io/api/core/v1"
)

var (
	// Labels represents the range of labels that can be used for test cases selection.
	// Used in ipsec_suite_test.go.
	Labels = []string{systemtestsparams.Label, Label}

	// ReporterNamespacesToDump tells the reporter from where to collect logs.
	// Used in ipsec_suite_test.go.
	ReporterNamespacesToDump = map[string]string{
		"test": "ipsec-test-workload",
	}

	// ReporterCRDsToDump tells the reporter what CRs to dump.
	// Used in ipsec_suite_test.go.
	ReporterCRDsToDump = []k8sreporter.CRData{
		{Cr: &corev1.PodList{}},
	}

	// TestNamespaceName is used for defining the namespace name where test resources are created.
	TestNamespaceName = "ipsec-system-tests"

	// ContainerLabelKey is used below in CreateContainerLabelsMap(),
	// CreateContainerLabelsStr() ,and CreateServiceDeploymentName().
	ContainerLabelKey = "ipsec-systemtest"

	// ContainerLabelValPrefix is used below in CreateContainerLabelsMap(),
	// CreateContainerLabelsStr(), and CreateServiceDeploymentName().
	ContainerLabelValPrefix = "iperf3"

	// ContainerCmdBash start a container command with bash.
	ContainerCmdBash = []string{"/bin/bash", "-c"}

	// ContainerCmdChroot start a container command with with chroot for root access.
	ContainerCmdChroot = []string{"chroot", "/rootfs", "/bin/sh", "-c"}

	// ContainerCmdSleep start a container with a sleep, later execute the needed
	// command in the container.
	ContainerCmdSleep = append(slices.Clone(ContainerCmdBash), "sleep infinity")

	// IpsecCmdShow IPSec command string to check for active/open tunnels.
	IpsecCmdShow = append(slices.Clone(ContainerCmdChroot), "ipsec show")

	// IpsecCmdTrafficStatus IPSec command string to check for tunnel packets.
	IpsecCmdTrafficStatus = append(slices.Clone(ContainerCmdChroot), "ipsec trafficstatus")

	// Iperf3OptionBind option to bind to an IP.
	Iperf3OptionBind = "-B"

	// Iperf3OptionBytes option to specify how may bytes to send.
	Iperf3OptionBytes = "-n"

	// Iperf3OptionPort option to use a specific port, intead of the default 5201.
	Iperf3OptionPort = "-p"

	// Iperf3ClientBaseCmd Start an iperf3 client with JSON output,
	// need to append the serverIP, port, and bytes.
	Iperf3ClientBaseCmd = []string{"iperf3", "-J", "-c"}

	// Iperf3ServerBaseCmd Start an iperf3 server that stops after 1 iperf3 client
	// connection completes, need to append the server IP to bind to.
	// If the client does not connect in 180 seconds, the server will be interrupted.
	Iperf3ServerBaseCmd = []string{"timeout", "180", "iperf3", "--one-off", "-J", "-s"}
)

// CreateContainerLabelsMap create a container label map with the index passed in.
func CreateContainerLabelsMap(index int, namePrefix string) map[string]string {
	labelValue := fmt.Sprintf("%s-%s-%d", ContainerLabelValPrefix, namePrefix, index)
	containerLabelsMap := map[string]string{ContainerLabelKey: labelValue}

	return containerLabelsMap
}

// CreateContainerLabelsStr create a container label string with the index passed in.
func CreateContainerLabelsStr(index int, namePrefix string) string {
	return fmt.Sprintf("%s=%s-%s-%d", ContainerLabelKey, ContainerLabelValPrefix, namePrefix, index)
}

// CreateServiceDeploymentName create a deployment name with the index and prefix passed in.
func CreateServiceDeploymentName(index int, namePrefix string) string {
	return fmt.Sprintf("%s-%s-%d", ContainerLabelValPrefix, namePrefix, index)
}

// CreateIperf3ServerOcpIPs convert a sting of comma-separated IPs into a string
// list of IPs.
func CreateIperf3ServerOcpIPs(ipsStr string) ([]string, error) {
	if len(ipsStr) == 0 {
		return nil, fmt.Errorf("error: CreateIperf3ServerOcpIPs variable must be set")
	}

	return strings.Split(ipsStr, ","), nil
}
