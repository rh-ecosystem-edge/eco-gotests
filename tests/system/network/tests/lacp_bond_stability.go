package tests

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system/network/internal/lacpparams"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

var _ = Describe(
	"LACP Bond Stability",
	Label(lacpparams.LabelLACPBondStability),
	Ordered,
	ContinueOnFailure,
	func() {
		var (
			bondedNodeNames   []string
			bondInterfaceName string
		)

		BeforeAll(func() {
			bondInterfaceName = getBondInterfaceName()
			bondedNodeNames = loadBondedNodes()
		})

		Context("Bond Reboot Recovery", Label("bond-reboot"), func() {
			It("Verifies COLLECTING_AND_DISTRIBUTING state after node reboot", func() {
				nodeName := bondedNodeNames[0]
				verifyPreRebootLACPState(nodeName, bondInterfaceName)
				rebootNodeAndWait(nodeName)
				verifyPostRebootLACPState(nodeName, bondInterfaceName)
			})
		})

		Context("LACP Timeout Validation", Label("timeout-validation"), func() {
			It("Verifies LACP rate and timeout thresholds", func() {
				for _, nodeName := range bondedNodeNames {
					By(fmt.Sprintf("Checking LACP timeout on node %s", nodeName))
					verifyLACPTimeout(nodeName, bondInterfaceName)
				}
			})
		})
	},
)

// getBondInterfaceName reads the bond interface name from ECO_LACP_BOND_INTERFACE
// env var with fallback to the default bond0.
func getBondInterfaceName() string {
	bondIface := os.Getenv(lacpparams.EnvLACPBondInterface)
	if bondIface == "" {
		bondIface = lacpparams.DefaultBondInterfaceName
	}

	klog.V(lacpparams.LogLevel).Infof(
		"Using bond interface: %s", bondIface)

	return bondIface
}

func loadBondedNodes() []string {
	nodesEnv := os.Getenv("ECO_LACP_BONDED_NODES")
	Expect(nodesEnv).NotTo(BeEmpty(),
		"ECO_LACP_BONDED_NODES must be set with comma-separated node names")

	nodeNames := strings.Split(nodesEnv, ",")

	for i := range nodeNames {
		nodeNames[i] = strings.TrimSpace(nodeNames[i])
	}

	nodeNames = slices.DeleteFunc(nodeNames, func(s string) bool { return s == "" })

	Expect(nodeNames).NotTo(BeEmpty(),
		"ECO_LACP_BONDED_NODES must contain at least one non-empty node name")

	klog.V(lacpparams.LogLevel).Infof(
		"LACP bonded nodes: %v", nodeNames)

	validateNodesExist(nodeNames)

	workerNodes := filterWorkerNodes(nodeNames)

	return workerNodes
}

func validateNodesExist(nodeNames []string) {
	for _, name := range nodeNames {
		By(fmt.Sprintf("Validating node %s exists in the cluster", name))

		node, err := nodes.Pull(APIClient, name)
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to find node %s", name))
		Expect(node).NotTo(BeNil(),
			fmt.Sprintf("Node %s not found in cluster", name))
	}
}

// filterWorkerNodes filters a list of node names to include only worker nodes,
// excluding any control-plane or master nodes. This prevents destructive test
// operations (reboots, OVS bridge recreation) from affecting etcd quorum.
func filterWorkerNodes(nodeNames []string) []string {
	var workerNodes []string

	for _, name := range nodeNames {
		node, err := nodes.Pull(APIClient, name)
		Expect(err).ToNot(HaveOccurred(),
			fmt.Sprintf("Failed to pull node %s for role filtering", name))

		_, hasControlPlane := node.Object.Labels["node-role.kubernetes.io/control-plane"]
		_, hasMaster := node.Object.Labels["node-role.kubernetes.io/master"]

		if hasControlPlane || hasMaster {
			klog.V(lacpparams.LogLevel).Infof(
				"Excluding non-worker node %s (control-plane=%v, master=%v)",
				name, hasControlPlane, hasMaster)

			continue
		}

		_, hasWorker := node.Object.Labels["node-role.kubernetes.io/worker"]
		if !hasWorker {
			klog.V(lacpparams.LogLevel).Infof(
				"Excluding node %s: missing worker role label", name)

			continue
		}

		workerNodes = append(workerNodes, name)
	}

	Expect(workerNodes).NotTo(BeEmpty(),
		"ECO_LACP_BONDED_NODES must contain at least one worker node; "+
			"control-plane/master nodes are excluded from destructive LACP tests")

	klog.V(lacpparams.LogLevel).Infof(
		"Filtered to worker nodes: %v (from %d total bonded nodes)",
		workerNodes, len(nodeNames))

	return workerNodes
}

func verifyPreRebootLACPState(nodeName, bondInterface string) {
	By(fmt.Sprintf("Verifying pre-reboot LACP state on node %s", nodeName))

	err := checkLACPPortState(nodeName, bondInterface)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("LACP bond not healthy on node %s before reboot", nodeName))
}

func rebootNodeAndWait(nodeName string) {
	By(fmt.Sprintf("Rebooting node %s", nodeName))

	nodeSelector := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("kubernetes.io/hostname=%s", nodeName),
	}

	cmd := "sudo systemctl reboot"

	_, err := cluster.ExecCmdWithStdout(APIClient, cmd, nodeSelector)
	if err != nil {
		klog.V(lacpparams.LogLevel).Infof(
			"Reboot command on node %s returned error (expected during reboot): %v",
			nodeName, err)
	}

	node, pullErr := nodes.Pull(APIClient, nodeName)
	Expect(pullErr).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to pull node %s for reboot wait", nodeName))

	By(fmt.Sprintf("Waiting for node %s to become NotReady", nodeName))

	Expect(node.WaitUntilNotReady(2*time.Minute)).ToNot(HaveOccurred(),
		fmt.Sprintf("Node %s did not become NotReady after reboot", nodeName))

	By(fmt.Sprintf("Waiting for node %s to become Ready", nodeName))

	Expect(node.WaitUntilReady(lacpparams.RebootTimeout)).ToNot(HaveOccurred(),
		fmt.Sprintf("Node %s did not become Ready after reboot", nodeName))
}

func verifyPostRebootLACPState(nodeName, bondInterface string) {
	By(fmt.Sprintf(
		"Verifying LACP COLLECTING_AND_DISTRIBUTING state on node %s after reboot",
		nodeName))

	Eventually(func() error {
		return checkLACPPortState(nodeName, bondInterface)
	}, lacpparams.DefaultTimeout, lacpparams.DefaultInterval).Should(Succeed(),
		fmt.Sprintf(
			"LACP did not reach COLLECTING_AND_DISTRIBUTING on node %s after reboot",
			nodeName))
}

// verifyLACPTimeout reads the bonding info and validates the LACP rate.
// Both 'fast' and 'slow' rates are acceptable per CILAB-2608, with appropriate
// timeout thresholds: slow >= 60s, fast >= 1s.
func verifyLACPTimeout(nodeName, bondInterface string) {
	output := readBondingOutput(nodeName, bondInterface)

	lacpRate := parseLACPRate(output)
	klog.V(lacpparams.LogLevel).Infof(
		"LACP rate on node %s: %s", nodeName, lacpRate)

	Expect(lacpRate).To(
		SatisfyAny(Equal("fast"), Equal("slow")),
		fmt.Sprintf(
			"LACP rate must be 'fast' or 'slow' on node %s, got '%s'\nRaw bonding output:\n%s",
			nodeName, lacpRate, output))

	if lacpRate == "slow" {
		klog.V(lacpparams.LogLevel).Infof(
			"Node %s: slow LACP rate detected (timeout >= %ds)",
			nodeName, lacpparams.MinLACPSlowTimeoutSeconds)
	} else {
		klog.V(lacpparams.LogLevel).Infof(
			"Node %s: fast LACP rate detected (timeout >= %ds)",
			nodeName, lacpparams.MinLACPFastTimeoutSeconds)
	}
}

// readBondingOutput reads /proc/net/bonding/<bond> from a node.
func readBondingOutput(nodeName, bondInterface string) string {
	nodeSelector := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("kubernetes.io/hostname=%s", nodeName),
	}

	bondPath := fmt.Sprintf("/proc/net/bonding/%s", bondInterface)
	cmd := fmt.Sprintf("cat %s", bondPath)

	outputs, err := cluster.ExecCmdWithStdout(APIClient, cmd, nodeSelector)
	Expect(err).ToNot(HaveOccurred(),
		fmt.Sprintf("Failed to read bonding info on node %s", nodeName))

	output, exists := outputs[nodeName]
	Expect(exists).To(BeTrue(),
		fmt.Sprintf("No bonding output received from node %s", nodeName))

	return output
}

// checkLACPPortState reads bonding output and validates LACP port state.
// On failure, logs the raw bonding output for debugging.
func checkLACPPortState(nodeName, bondInterface string) error {
	nodeSelector := metav1.ListOptions{
		LabelSelector: fmt.Sprintf("kubernetes.io/hostname=%s", nodeName),
	}

	bondPath := fmt.Sprintf("/proc/net/bonding/%s", bondInterface)
	cmd := fmt.Sprintf("cat %s", bondPath)

	outputs, err := cluster.ExecCmdWithStdout(APIClient, cmd, nodeSelector)
	if err != nil {
		return fmt.Errorf("failed to read bonding on node %s: %w", nodeName, err)
	}

	output, exists := outputs[nodeName]
	if !exists {
		return fmt.Errorf("no bonding output from node %s", nodeName)
	}

	if err := validateLACPPortState(output, bondInterface); err != nil {
		klog.V(lacpparams.LogLevel).Infof(
			"LACP port state validation failed on node %s.\nRaw bonding output:\n%s",
			nodeName, output)

		return err
	}

	return nil
}

// validateLACPPortState parses port states and verifies Collecting+Distributing bits.
func validateLACPPortState(bondingOutput, bondInterface string) error {
	actorState, partnerState, err := parseLACPPortStates(bondingOutput)
	if err != nil {
		return fmt.Errorf("failed to parse LACP states for %s: %w", bondInterface, err)
	}

	if err := verifyCollectingDistributing(actorState, "actor", bondInterface); err != nil {
		return err
	}

	return verifyCollectingDistributing(partnerState, "partner", bondInterface)
}

// verifyCollectingDistributing checks that the Collecting and Distributing bits
// are set in the given port state value.
func verifyCollectingDistributing(stateStr, role, bondInterface string) error {
	state, err := strconv.Atoi(stateStr)
	if err != nil {
		return fmt.Errorf(
			"%s port state '%s' is not a valid integer on %s: %w",
			role, stateStr, bondInterface, err)
	}

	if state&lacpparams.LACPStateCollecting == 0 {
		return fmt.Errorf(
			"%s port state %d missing Collecting bit on %s", role, state, bondInterface)
	}

	if state&lacpparams.LACPStateDistributing == 0 {
		return fmt.Errorf(
			"%s port state %d missing Distributing bit on %s", role, state, bondInterface)
	}

	return nil
}

// parseLACPPortStates extracts the first actor and partner port state numeric values
// from /proc/net/bonding/ output. Handles the kernel format where port state may
// include descriptive text after the numeric value (e.g., "63 (Activity ...)").
func parseLACPPortStates(bondingOutput string) (string, string, error) {
	lines := strings.Split(bondingOutput, "\n")

	var (
		actorState, partnerState string
		inActorSection           bool
		inPartnerSection         bool
	)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		switch {
		case strings.Contains(trimmed, "details actor lacp pdu:"):
			inActorSection = true
			inPartnerSection = false
		case strings.Contains(trimmed, "details partner lacp pdu:"):
			inActorSection = false
			inPartnerSection = true
		case strings.HasPrefix(trimmed, "Slave Interface:"):
			inActorSection = false
			inPartnerSection = false
		}

		if !strings.Contains(trimmed, "port state:") {
			continue
		}

		state := extractPortStateValue(trimmed)
		if state == "" {
			continue
		}

		if inActorSection && actorState == "" {
			actorState = state
		} else if inPartnerSection && partnerState == "" {
			partnerState = state
		}

		if actorState != "" && partnerState != "" {
			break
		}
	}

	if actorState == "" {
		return "", "", fmt.Errorf("could not find LACP actor port state")
	}

	if partnerState == "" {
		return "", "", fmt.Errorf("could not find LACP partner port state")
	}

	return actorState, partnerState, nil
}

// extractPortStateValue extracts the numeric port state from a "port state:" line.
// Handles formats like "port state: 61" and "port state: 63 (Activity ...)".
func extractPortStateValue(line string) string {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) < 2 {
		return ""
	}

	value := strings.TrimSpace(parts[1])

	fields := strings.Fields(value)
	if len(fields) == 0 {
		return ""
	}

	return fields[0]
}

func parseLACPRate(bondingOutput string) string {
	lines := strings.Split(bondingOutput, "\n")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "LACP rate:") {
			parts := strings.SplitN(trimmed, ":", 2)
			if len(parts) >= 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}

	return ""
}
