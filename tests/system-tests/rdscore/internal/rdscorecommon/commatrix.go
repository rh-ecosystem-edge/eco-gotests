// Commatrix host-firewall verification (ginkgo reportxml 95003–95008): TCP connectivity and firewall
// journal checks against a cluster that already has commatrix host-firewall MachineConfigs applied.
//
//nolint:varnamelen,lll,wsl_v5 // test helpers follow oc/k8s naming; long shell/klog lines are intentional.
package rdscorecommon

import (
	"context"
	"fmt"
	"net"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	mcv1 "github.com/openshift/api/machineconfiguration/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/mco"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/remote"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreparams"
)

const (
	commatrixNFTablesOpenshiftChain     = "OPENSHIFT"
	commatrixJournalSinceOneMinute      = "1 minute ago"
	commatrixJournalSinceTwoMinutes     = "2 minutes ago"
	commatrixFirewallLogPrefixNoSpace   = "firewall"  // kernel: firewallIN=...
	commatrixFirewallLogPrefixWithSpace = "firewall " // kernel: firewall IN=...
	commatrixFirewallRateLimitPerMinute = 5
	// Fixed probe targets and waits (not configurable; same across OCP host-firewall test plans).
	commatrixAPIPort                     = 6443
	commatrixKubeletPort                 = 10250
	commatrixClosedTCPPort               = 9999
	commatrixNFTablesLogKeyword          = "firewall"
	commatrixHostFirewallMCNameSubstring = "nftables-commatrix"
	// commatrixTCPProbeTimeout matches legacy nc -w3 probe wait.
	commatrixTCPProbeTimeout = 3 * time.Second
	// commatrixLogMsgPrefix prefixes host-firewall test log lines (matches rdscorecommon action-oriented style).
	commatrixLogMsgPrefix = "Commatrix host-firewall"

	commatrixNodeRoleLabelWorker       = "node-role.kubernetes.io/worker"
	commatrixNodeRoleLabelMaster       = "node-role.kubernetes.io/master"
	commatrixNodeRoleLabelControlPlane = "node-role.kubernetes.io/control-plane"
	commatrixNodeRoleLabelStorage      = "node-role.kubernetes.io/storage"
)

// journalShortTimePrefixRe parses journalctl short-iso timestamps for firewall log rate-limit checks.
var journalShortTimePrefixRe = regexp.MustCompile(`^([A-Z][a-z]{2}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2})`)

// commatrixNFTablesPortTokenRe extracts numeric ports from nftables dport clauses.
var commatrixNFTablesPortTokenRe = regexp.MustCompile(`\b(\d{1,5})\b`)

// commatrixRunTopology holds node names and probe IPs resolved for connectivity checks.
type commatrixRunTopology struct {
	SecureWorkerName  string
	MasterIPs         []string
	SecureWorkerIPs   []string
	SecureOpenPort    int
	SecureBlockedPort int
}

// commatrixWorkflowState carries shared state between connectivity and journal specs in one test run.
type commatrixWorkflowState struct {
	run                 commatrixRunTopology
	primed              bool
	discoveredPoolNames []string
	probePoolNames      []string
	nftProbeNodeName    string
}

var commatrixWorkflow commatrixWorkflowState

// commatrixLogOutputSnippet returns trimmed command output for logs, truncated when very long.
func commatrixLogOutputSnippet(output string, maxLen int) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return "(empty)"
	}

	if maxLen <= 0 || len(output) <= maxLen {
		return output
	}

	return output[:maxLen] + "...(truncated)"
}

// commatrixPrimeWorkflowForVerification discovers host-firewall pools from commatrix MachineConfigs on the cluster.
func commatrixPrimeWorkflowForVerification() error {
	if commatrixWorkflow.primed {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: workflow already primed (nftProbeNode=%q)",
			commatrixLogMsgPrefix, commatrixWorkflow.nftProbeNodeName))

		return nil
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: priming verification workflow (discover commatrix MCs matching %q on cluster)",
		commatrixLogMsgPrefix, commatrixHostFirewallMCNameSubstring))

	poolNames, err := commatrixHostFirewallPoolNamesFromCluster()
	if err != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: list commatrix host-firewall MachineConfigs failed: %v", commatrixLogMsgPrefix, err))

		return fmt.Errorf("discover host-firewall MachineConfig pools: %w", err)
	}

	if len(poolNames) == 0 {
		detail := commatrixHostFirewallMCDiscoveryDetail()
		klog.Warningf("%s: host-firewall MachineConfig discovery failed: %s", commatrixLogMsgPrefix, detail)

		return fmt.Errorf(
			"no commatrix host-firewall MachineConfigs on cluster (look for %q in mc names); %s; "+
				"apply host-firewall rules before connectivity/journal tests",
			commatrixHostFirewallMCNameSubstring, detail)
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
		"%s: discovered MachineConfig pool name(s) from cluster MCs: %v", commatrixLogMsgPrefix, poolNames))

	commatrixWorkflow.discoveredPoolNames = append([]string(nil), poolNames...)

	securePool := commatrixInferSecureMCPoolNameFromPools(poolNames)
	if securePool == "" {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: could not infer secure worker MachineConfigPool from discovered pools %v",
			commatrixLogMsgPrefix, poolNames))

		return fmt.Errorf("could not infer secure/firewall MCP from pools %v", poolNames)
	}

	masterPool := commatrixInferMasterMCPoolNameFromPools(poolNames)

	applyPools := []string{securePool}
	if masterPool != "" && masterPool != securePool {
		applyPools = append(applyPools, masterPool)
	}

	commatrixWorkflow.probePoolNames = append([]string(nil), applyPools...)

	secureNodes, err := commatrixNodesFromMachineConfigPools([]string{securePool})
	if err != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: list nodes in secure pool %q failed: %v", commatrixLogMsgPrefix, securePool, err))

		return fmt.Errorf("list nodes in secure pool %q: %w", securePool, err)
	}

	if len(secureNodes) == 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: secure pool %q matched no nodes for connectivity/journal probes",
			commatrixLogMsgPrefix, securePool))

		return fmt.Errorf("no nodes in secure pool %q for connectivity/journal probes", securePool)
	}

	commatrixWorkflow.nftProbeNodeName = secureNodes[0]
	commatrixWorkflow.primed = true

	klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
		"%s: primed workflow discoveredPools=%v securePool=%q masterPool=%q probePools=%v nftProbeNode=%q securePoolNodes=%v",
		commatrixLogMsgPrefix,
		poolNames, securePool, masterPool, applyPools, commatrixWorkflow.nftProbeNodeName, secureNodes))

	return nil
}

// commatrixMachineConfigPoolRole returns the MachineConfigPool name for a commatrix MC from its role label or name suffix.
func commatrixMachineConfigPoolRole(mcObj *mcv1.MachineConfig) string {
	if mcObj == nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: cannot resolve pool role from nil MachineConfig", commatrixLogMsgPrefix))

		return ""
	}

	if role := strings.TrimSpace(mcObj.Labels[mcv1.MachineConfigRoleLabelKey]); role != "" {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: MachineConfig %q -> pool %q (from %s label)",
			commatrixLogMsgPrefix, mcObj.Name, role, mcv1.MachineConfigRoleLabelKey))

		return role
	}

	// Some clusters omit machineconfiguration.openshift.io/role; commatrix names are
	// <priority>-nftables-commatrix-<pool> and the pool is the suffix after the marker.
	if !strings.Contains(mcObj.Name, commatrixHostFirewallMCNameSubstring) {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: MachineConfig %q has no role label and name lacks %q; cannot resolve pool",
			commatrixLogMsgPrefix, mcObj.Name, commatrixHostFirewallMCNameSubstring))

		return ""
	}

	pool := strings.TrimSpace(strings.TrimPrefix(
		mcObj.Name[strings.Index(mcObj.Name, commatrixHostFirewallMCNameSubstring)+len(commatrixHostFirewallMCNameSubstring):],
		"-",
	))
	if pool == "" {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: MachineConfig %q has no role label and empty pool suffix after %q",
			commatrixLogMsgPrefix, mcObj.Name, commatrixHostFirewallMCNameSubstring))

		return ""
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: MachineConfig %q -> pool %q (inferred from metadata.name suffix)",
		commatrixLogMsgPrefix, mcObj.Name, pool))

	return pool
}

// commatrixHostFirewallPoolNamesFromCluster lists unique MCP names from commatrix host-firewall MachineConfigs on the cluster.
func commatrixHostFirewallPoolNamesFromCluster() ([]string, error) {
	mcList, err := mco.ListMC(APIClient)
	if err != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: mco.ListMC failed while discovering host-firewall pools: %v", commatrixLogMsgPrefix, err))

		return nil, err
	}

	seen := make(map[string]struct{})
	pools := make([]string, 0)
	var matchedMCNames []string

	for _, mcBuilder := range mcList {
		if mcBuilder == nil || mcBuilder.Object == nil {
			continue
		}

		mcObj := mcBuilder.Object
		if !strings.Contains(mcObj.Name, commatrixHostFirewallMCNameSubstring) {
			continue
		}

		matchedMCNames = append(matchedMCNames, mcObj.Name)

		role := commatrixMachineConfigPoolRole(mcObj)
		if role == "" {
			continue
		}

		if _, ok := seen[role]; ok {
			continue
		}

		seen[role] = struct{}{}

		pools = append(pools, role)
	}

	sort.Strings(pools)

	klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
		"%s: listed %d commatrix host-firewall MachineConfig(s) %v -> pool name(s) %v",
		commatrixLogMsgPrefix, len(matchedMCNames), matchedMCNames, pools))

	return pools, nil
}

// commatrixHostFirewallMCDiscoveryDetail summarizes ListMC results to explain empty pool discovery.
func commatrixHostFirewallMCDiscoveryDetail() string {
	mcList, err := mco.ListMC(APIClient)
	if err != nil {
		return fmt.Sprintf("list MachineConfigs failed: %v", err)
	}

	var commatrixNames []string

	for _, mcBuilder := range mcList {
		if mcBuilder == nil || mcBuilder.Object == nil {
			continue
		}

		if strings.Contains(mcBuilder.Object.Name, commatrixHostFirewallMCNameSubstring) {
			commatrixNames = append(commatrixNames, mcBuilder.Object.Name)
		}
	}

	sort.Strings(commatrixNames)

	var b strings.Builder

	_, _ = fmt.Fprintf(&b, "listed %d MachineConfig(s)", len(mcList))

	if len(commatrixNames) == 0 {
		_, _ = fmt.Fprintf(&b, ", none matching %q in metadata.name", commatrixHostFirewallMCNameSubstring)
	} else {
		_, _ = fmt.Fprintf(&b, ", %d matching %q but no pool role resolved: %v",
			len(commatrixNames), commatrixHostFirewallMCNameSubstring, commatrixNames)
	}

	if APIClient != nil && APIClient.Config != nil {
		_, _ = fmt.Fprintf(&b, "; API server %s", APIClient.Config.Host)
	}

	if kubeconfig := strings.TrimSpace(os.Getenv("KUBECONFIG")); kubeconfig != "" {
		_, _ = fmt.Fprintf(&b, "; KUBECONFIG=%s", kubeconfig)
	} else {
		b.WriteString("; KUBECONFIG unset (using default kubeconfig or in-cluster config)")
	}

	return b.String()
}

// commatrixMasterLabelSelector returns the label selector string for control-plane nodes from GeneralConfig.
func commatrixMasterLabelSelector() string {
	return labels.Set(inittools.GeneralConfig.ControlPlaneLabelMap).String()
}

// commatrixTCPProbeNetwork returns the net.Dial network name (tcp4/tcp6/tcp) for a probe target host address.
func commatrixTCPProbeNetwork(host string) string {
	ip := net.ParseIP(host)
	if ip == nil {
		return "tcp"
	}

	if ip.To4() != nil {
		return "tcp4"
	}

	return "tcp6"
}

// commatrixDialTCP opens a short-lived TCP connection to host:port from the test runner.
func commatrixDialTCP(host, port string) error {
	conn, err := net.DialTimeout(commatrixTCPProbeNetwork(host), net.JoinHostPort(host, port), commatrixTCPProbeTimeout)
	if err != nil {
		return err
	}

	return conn.Close()
}

// commatrixRunTCPProbeFromRunner TCP-dials host:port from the test runner (no external nc binary required).
func commatrixRunTCPProbeFromRunner(expectConnect bool, host, port string) error {
	network := commatrixTCPProbeNetwork(host)
	addr := net.JoinHostPort(host, port)
	expectLabel := "connect"
	if !expectConnect {
		expectLabel = "blocked"
	}

	dialErr := commatrixDialTCP(host, port)
	connected := dialErr == nil

	if expectConnect == connected {
		if expectConnect {
			klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
				"%s: TCP probe to %s %s succeeded (expected %s)", commatrixLogMsgPrefix, network, addr, expectLabel))
		} else {
			klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
				"%s: TCP probe to %s %s failed as expected: %v", commatrixLogMsgPrefix, network, addr, dialErr))
		}

		return nil
	}

	if expectConnect {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: TCP probe to %s %s failed (expected %s): %v",
			commatrixLogMsgPrefix, network, addr, expectLabel, dialErr))

		return dialErr
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: TCP probe to %s %s unexpectedly connected (expected %s)",
		commatrixLogMsgPrefix, network, addr, expectLabel))

	return fmt.Errorf("expected connection to %s to fail, but dial succeeded", addr)
}

// commatrixProbeLabel returns a short IPv4/IPv6 label for probe log messages.
func commatrixProbeLabel(addr string) string {
	if ip := net.ParseIP(addr); ip != nil && ip.To4() == nil {
		return "IPv6 " + addr
	}

	return "IPv4 " + addr
}

// commatrixTryTCPProbesFromRunner probes host:port on each address. When expectConnect is true, at least one
// address must succeed; when false, every address must fail to connect.
func commatrixTryTCPProbesFromRunner(desc string, addrs []string, port string, expectConnect bool) error {
	if len(addrs) == 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: TCP probe %q skipped: no addresses to probe", commatrixLogMsgPrefix, desc))

		return fmt.Errorf("%s: no internal IP addresses to probe", desc)
	}

	if expectConnect {
		var failures []string

		for _, addr := range addrs {
			label := commatrixProbeLabel(addr)
			klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
				"%s: TCP probe %q via %s -> %s:%s (expect at least one connect)",
				commatrixLogMsgPrefix, desc, label, addr, port))

			err := commatrixRunTCPProbeFromRunner(true, addr, port)
			if err == nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
					"%s: TCP probe %q succeeded via %s", commatrixLogMsgPrefix, desc, label))

				return nil
			}

			failures = append(failures, fmt.Sprintf("%s: %v", label, err))
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: TCP probe %q failed on all addresses %v: %s",
			commatrixLogMsgPrefix, desc, addrs, strings.Join(failures, "; ")))

		return fmt.Errorf("%s: none of %v reachable from test runner (%s)",
			desc, addrs, strings.Join(failures, "; "))
	}

	for _, addr := range addrs {
		label := commatrixProbeLabel(addr)
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
			"%s: TCP probe %q via %s -> %s:%s (expect blocked on every address)",
			commatrixLogMsgPrefix, desc, label, addr, port))

		if err := commatrixRunTCPProbeFromRunner(false, addr, port); err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: TCP probe %q via %s unexpectedly connected (expected blocked): %v",
				commatrixLogMsgPrefix, desc, label, err))

			return fmt.Errorf("%s via %s: %w", desc, label, err)
		}
	}

	return nil
}

// commatrixRunOnNodeHostDebug runs cmd on the node host via the MCO daemon pod (chroot /rootfs).
func commatrixRunOnNodeHostDebug(nodeName string, cmd []string) (string, error) {
	cmdStr := strings.Join(cmd, " ")
	hostDebugCmdOut, hostDebugCmdErr := remote.ExecuteOnNodeWithDebugPod(cmd, nodeName)

	if hostDebugCmdErr != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: node %q debug-pod exec failed: cmd=%q err=%v output=%q",
			commatrixLogMsgPrefix, nodeName, cmdStr, hostDebugCmdErr,
			commatrixLogOutputSnippet(hostDebugCmdOut, 500)))
	} else {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: node %q debug-pod exec succeeded: cmd=%q output=%q",
			commatrixLogMsgPrefix, nodeName, cmdStr, commatrixLogOutputSnippet(hostDebugCmdOut, 500)))
	}

	return hostDebugCmdOut, hostDebugCmdErr
}

// commatrixExtractOpenshiftFilterRuleLines returns non-empty nft rule lines from a decoded openshift_filter payload.
func commatrixExtractOpenshiftFilterRuleLines(nftText string) []string {
	lines := strings.Split(nftText, "\n")
	klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
		"%s: parsing openshift_filter nft text (%d line(s), %d byte(s))",
		commatrixLogMsgPrefix, len(lines), len(nftText)))

	var rules []string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "table ") || strings.HasPrefix(lower, "chain ") ||
			strings.HasPrefix(lower, "type ") || strings.HasPrefix(lower, "delete ") {
			continue
		}

		if strings.Contains(lower, " dport ") || strings.Contains(lower, " sport ") ||
			strings.Contains(lower, " accept") || strings.Contains(lower, " drop") ||
			strings.Contains(lower, " reject") || strings.Contains(lower, " log") ||
			strings.HasPrefix(lower, "tcp ") || strings.HasPrefix(lower, "udp ") ||
			strings.HasPrefix(lower, "ip ") || strings.HasPrefix(lower, "meta ") {
			rules = append(rules, line)
		}
	}

	if len(rules) == 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
			"%s: no openshift_filter rule lines matched filter heuristics; nft text: %q",
			commatrixLogMsgPrefix, commatrixLogOutputSnippet(nftText, 1000)))

		return rules
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
		"%s: extracted %d openshift_filter rule line(s): %v",
		commatrixLogMsgPrefix, len(rules), rules))

	return rules
}

// commatrixExtractPortNumbersAfterDport parses TCP/UDP destination port numbers from one nftables rule line.
func commatrixExtractPortNumbersAfterDport(ruleLine string) []int {
	lower := strings.ToLower(ruleLine)
	dportIdx := strings.Index(lower, " dport ")
	if dportIdx < 0 {
		return nil
	}

	rest := strings.TrimSpace(ruleLine[dportIdx+len(" dport "):])
	for _, stopWord := range []string{" accept", " drop", " reject", " log", " counter", " limit"} {
		if stopIdx := strings.Index(strings.ToLower(rest), stopWord); stopIdx >= 0 {
			rest = strings.TrimSpace(rest[:stopIdx])
		}
	}

	rest = strings.Trim(rest, "{}")

	var ports []int

	for _, token := range strings.Split(rest, ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}

		if dashIdx := strings.Index(token, "-"); dashIdx > 0 {
			startStr := strings.TrimSpace(token[:dashIdx])
			endStr := strings.TrimSpace(token[dashIdx+1:])

			startPort, errStart := strconv.Atoi(startStr)
			endPort, errEnd := strconv.Atoi(endStr)
			if errStart == nil && errEnd == nil && startPort > 0 && endPort >= startPort && endPort <= 65535 {
				for p := startPort; p <= endPort && p <= startPort+32; p++ {
					ports = append(ports, p)
				}
			}

			continue
		}

		for _, match := range commatrixNFTablesPortTokenRe.FindAllString(token, -1) {
			portNum, errAtoi := strconv.Atoi(match)
			if errAtoi == nil && portNum > 0 && portNum <= 65535 {
				ports = append(ports, portNum)
			}
		}
	}

	return ports
}

// commatrixParseAcceptedTCPDPortsFromNFTables builds the set of TCP dports with accept rules in openshift_filter text.
func commatrixParseAcceptedTCPDPortsFromNFTables(nftText string) map[int]struct{} {
	allowed := make(map[int]struct{})

	for _, ruleLine := range commatrixExtractOpenshiftFilterRuleLines(nftText) {
		lower := strings.ToLower(ruleLine)
		if !strings.Contains(lower, "tcp") || !strings.Contains(lower, " dport ") {
			continue
		}

		if !strings.Contains(lower, "accept") {
			continue
		}

		for _, portNum := range commatrixExtractPortNumbersAfterDport(ruleLine) {
			allowed[portNum] = struct{}{}
		}
	}

	return allowed
}

// commatrixFormatPortSet returns sorted port numbers from a port set for logging and selection.
func commatrixFormatPortSet(portSet map[int]struct{}) []int {
	ports := make([]int, 0, len(portSet))
	for portNum := range portSet {
		ports = append(ports, portNum)
	}

	sort.Ints(ports)

	return ports
}

// commatrixPickBlockedTCPPort chooses a TCP port not present in the nft accept set for blocked-connectivity probes.
func commatrixPickBlockedTCPPort(allowed map[int]struct{}) int {
	candidates := []int{
		commatrixAPIPort,
		commatrixClosedTCPPort,
		443,
		8080,
		8443,
		9090,
	}

	for _, candidate := range candidates {
		if _, open := allowed[candidate]; !open {
			klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
				"%s: selected blocked probe port %d (not in nft accept set %v)",
				commatrixLogMsgPrefix, candidate, commatrixFormatPortSet(allowed)))

			return candidate
		}
	}

	klog.Warningf(
		"%s: all blocked-port candidates %v are in nft accept rules %v; falling back to default closed port %d",
		commatrixLogMsgPrefix, candidates, commatrixFormatPortSet(allowed), commatrixClosedTCPPort)

	return commatrixClosedTCPPort
}

// commatrixSelectProbePorts picks distinct open (accepted) and blocked probe ports from an nft accept port set.
func commatrixSelectProbePorts(allowed map[int]struct{}) (openPort, blockedPort int, err error) {
	if len(allowed) == 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: cannot select probe ports: no accepted tcp dport rules in openshift_filter",
			commatrixLogMsgPrefix))

		return 0, 0, fmt.Errorf("no accepted tcp dport rules found in openshift_filter")
	}

	if _, ok := allowed[commatrixKubeletPort]; ok {
		openPort = commatrixKubeletPort
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
			"%s: selected open probe port %d (kubelet port present in nft accept rules)",
			commatrixLogMsgPrefix, openPort))
	} else {
		openPort = commatrixFormatPortSet(allowed)[0]
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
			"%s: selected open probe port %d (lowest port in nft accept rules %v)",
			commatrixLogMsgPrefix, openPort, commatrixFormatPortSet(allowed)))
	}

	blockedPort = commatrixPickBlockedTCPPort(allowed)
	if blockedPort == openPort {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: cannot select probe ports: open port %d equals blocked port from allowed set %v",
			commatrixLogMsgPrefix, openPort, commatrixFormatPortSet(allowed)))

		return 0, 0, fmt.Errorf("could not pick distinct open/blocked probe ports from allowed set %v",
			commatrixFormatPortSet(allowed))
	}

	return openPort, blockedPort, nil
}

// commatrixAllowedTCPDPortsFromNode lists accepted TCP dports from live openshift_filter rules on a node.
func commatrixAllowedTCPDPortsFromNode(nodeName string) (map[int]struct{}, error) {
	nftListShellCmd := "nft list table inet openshift_filter 2>/dev/null || nft list ruleset 2>/dev/null || true"

	klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
		"%s: reading live openshift_filter nftables accept rules on node %q", commatrixLogMsgPrefix, nodeName))

	nftOutput, err := commatrixRunOnNodeHostShell(nodeName, "list openshift_filter nftables rules", nftListShellCmd)
	if err != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: node %q openshift_filter nft list failed: %v", commatrixLogMsgPrefix, nodeName, err))

		return nil, fmt.Errorf("list openshift_filter nftables on %q: %w", nodeName, err)
	}

	allowed := commatrixParseAcceptedTCPDPortsFromNFTables(nftOutput)
	if len(allowed) == 0 {
		klog.Warningf("%s: node %q has no accepted tcp dport rules in openshift_filter; nft output: %q",
			commatrixLogMsgPrefix, nodeName, commatrixLogOutputSnippet(nftOutput, 1000))

		return nil, fmt.Errorf(
			"no accepted tcp dport rules in openshift_filter on %q; nft output:\n%s",
			nodeName, strings.TrimSpace(nftOutput))
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: node %q nft accept tcp dports: %v",
		commatrixLogMsgPrefix, nodeName, commatrixFormatPortSet(allowed)))

	return allowed, nil
}

// commatrixResolveSecureProbePorts reads nft rules on nodeName and stores open/blocked ports in workflow state.
func commatrixResolveSecureProbePorts(nodeName string) error {
	allowed, err := commatrixAllowedTCPDPortsFromNode(nodeName)
	if err != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: node %q probe port resolution failed reading nft accept rules: %v",
			commatrixLogMsgPrefix, nodeName, err))

		return err
	}

	openPort, blockedPort, errPick := commatrixSelectProbePorts(allowed)
	if errPick != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: node %q probe port selection failed: %v", commatrixLogMsgPrefix, nodeName, errPick))

		return errPick
	}

	commatrixWorkflow.run.SecureOpenPort = openPort
	commatrixWorkflow.run.SecureBlockedPort = blockedPort

	klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: secure worker %q probe ports open=%d blocked=%d (from live nft accept rules)",
		commatrixLogMsgPrefix, nodeName, openPort, blockedPort))

	return nil
}

// commatrixFilterPoolsOnCluster keeps only pool names that exist as MachineConfigPool objects on the cluster.
func commatrixFilterPoolsOnCluster(pools []string) []string {
	filtered := make([]string, 0, len(pools))

	for _, poolName := range pools {
		mcpB, errPull := mco.Pull(APIClient, poolName)
		if errPull != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: skip pool %q (MachineConfigPool not found on cluster): %v",
				commatrixLogMsgPrefix, poolName, errPull))

			continue
		}

		if _, errGet := mcpB.Get(); errGet != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: skip pool %q (get MachineConfigPool failed): %v",
				commatrixLogMsgPrefix, poolName, errGet))

			continue
		}

		filtered = append(filtered, poolName)
	}

	sort.Strings(filtered)

	return filtered
}

// commatrixNodeBuilderName returns a node name for logging, or empty when the builder is nil.
func commatrixNodeBuilderName(nb *nodes.Builder) string {
	if nb == nil || nb.Object == nil {
		return ""
	}

	return nb.Object.Name
}

// commatrixNodeHasRoleLabel reports whether a node has any of the given node-role.kubernetes.io label keys.
func commatrixNodeHasRoleLabel(nb *nodes.Builder, roleLabels ...string) bool {
	nodeName := commatrixNodeBuilderName(nb)
	roleList := strings.Join(roleLabels, ",")

	if nb == nil || nb.Object == nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
			"%s: node %q role label(s) %q -> false (nil node builder)",
			commatrixLogMsgPrefix, nodeName, roleList))

		return false
	}

	nodeLabels := nb.Object.Labels
	if nodeLabels == nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
			"%s: node %q role label(s) %q -> false (no labels)",
			commatrixLogMsgPrefix, nodeName, roleList))

		return false
	}

	for _, roleLabel := range roleLabels {
		if _, ok := nodeLabels[roleLabel]; ok {
			klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
				"%s: node %q role label(s) %q -> true (matched %q)",
				commatrixLogMsgPrefix, nodeName, roleList, roleLabel))

			return true
		}
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
		"%s: node %q role label(s) %q -> false",
		commatrixLogMsgPrefix, nodeName, roleList))

	return false
}

// commatrixPoolNodeBuilders returns node builders for all nodes matched by a MachineConfigPool nodeSelector.
func commatrixPoolNodeBuilders(poolName string) ([]*nodes.Builder, error) {
	nodeNames, err := commatrixNodesFromMachineConfigPools([]string{poolName})
	if err != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: list nodes for pool %q failed: %v", commatrixLogMsgPrefix, poolName, err))

		return nil, err
	}

	builders := make([]*nodes.Builder, 0, len(nodeNames))

	for _, nodeName := range nodeNames {
		nb, errPull := nodes.Pull(APIClient, nodeName)
		if errPull != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: pull node %q in pool %q failed: %v", commatrixLogMsgPrefix, nodeName, poolName, errPull))

			return nil, fmt.Errorf("pull node %q in pool %q: %w", nodeName, poolName, errPull)
		}

		builders = append(builders, nb)
	}

	return builders, nil
}

// commatrixPoolIsControlPlaneOnly reports whether every node in the MCP is a control-plane node.
func commatrixPoolIsControlPlaneOnly(poolName string) bool {
	nodeBuilders, err := commatrixPoolNodeBuilders(poolName)
	if err != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
			"%s: pool %q control-plane-only -> false (list nodes failed: %v)",
			commatrixLogMsgPrefix, poolName, err))

		return false
	}

	if len(nodeBuilders) == 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
			"%s: pool %q control-plane-only -> false (no nodes)",
			commatrixLogMsgPrefix, poolName))

		return false
	}

	for _, nb := range nodeBuilders {
		if !commatrixNodeHasRoleLabel(nb, commatrixNodeRoleLabelMaster, commatrixNodeRoleLabelControlPlane) {
			klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
				"%s: pool %q control-plane-only -> false (node %q is not control-plane)",
				commatrixLogMsgPrefix, poolName, commatrixNodeBuilderName(nb)))

			return false
		}
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
		"%s: pool %q control-plane-only -> true (%d node(s))",
		commatrixLogMsgPrefix, poolName, len(nodeBuilders)))

	return true
}

// commatrixPoolIsStorageOnly reports whether every node in the MCP is storage-role without worker or control-plane roles.
func commatrixPoolIsStorageOnly(poolName string) bool {
	nodeBuilders, err := commatrixPoolNodeBuilders(poolName)
	if err != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
			"%s: pool %q storage-only -> false (list nodes failed: %v)",
			commatrixLogMsgPrefix, poolName, err))

		return false
	}

	if len(nodeBuilders) == 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
			"%s: pool %q storage-only -> false (no nodes)",
			commatrixLogMsgPrefix, poolName))

		return false
	}

	for _, nb := range nodeBuilders {
		if nb == nil || nb.Object == nil || nb.Object.Labels == nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
				"%s: pool %q storage-only -> false (node %q has nil builder or labels)",
				commatrixLogMsgPrefix, poolName, commatrixNodeBuilderName(nb)))

			return false
		}

		if !commatrixNodeHasRoleLabel(nb, commatrixNodeRoleLabelStorage) {
			klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
				"%s: pool %q storage-only -> false (node %q lacks storage role)",
				commatrixLogMsgPrefix, poolName, commatrixNodeBuilderName(nb)))

			return false
		}

		if commatrixNodeHasRoleLabel(nb, commatrixNodeRoleLabelWorker) {
			klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
				"%s: pool %q storage-only -> false (node %q has worker role)",
				commatrixLogMsgPrefix, poolName, commatrixNodeBuilderName(nb)))

			return false
		}

		if commatrixNodeHasRoleLabel(nb, commatrixNodeRoleLabelMaster, commatrixNodeRoleLabelControlPlane) {
			klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
				"%s: pool %q storage-only -> false (node %q has control-plane role)",
				commatrixLogMsgPrefix, poolName, commatrixNodeBuilderName(nb)))

			return false
		}
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
		"%s: pool %q storage-only -> true (%d node(s))",
		commatrixLogMsgPrefix, poolName, len(nodeBuilders)))

	return true
}

// commatrixPoolHasWorkerNodes reports whether the MCP contains at least one worker or non-control-plane node.
func commatrixPoolHasWorkerNodes(poolName string) bool {
	nodeBuilders, err := commatrixPoolNodeBuilders(poolName)
	if err != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
			"%s: pool %q has-worker-nodes -> false (list nodes failed: %v)",
			commatrixLogMsgPrefix, poolName, err))

		return false
	}

	if len(nodeBuilders) == 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
			"%s: pool %q has-worker-nodes -> false (no nodes)",
			commatrixLogMsgPrefix, poolName))

		return false
	}

	for _, nb := range nodeBuilders {
		if commatrixNodeHasRoleLabel(nb, commatrixNodeRoleLabelWorker) {
			klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
				"%s: pool %q has-worker-nodes -> true (node %q has worker role)",
				commatrixLogMsgPrefix, poolName, commatrixNodeBuilderName(nb)))

			return true
		}

		if !commatrixNodeHasRoleLabel(nb, commatrixNodeRoleLabelMaster, commatrixNodeRoleLabelControlPlane) {
			klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
				"%s: pool %q has-worker-nodes -> true (node %q has no control-plane role)",
				commatrixLogMsgPrefix, poolName, commatrixNodeBuilderName(nb)))

			return true
		}
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
		"%s: pool %q has-worker-nodes -> false (all %d node(s) are control-plane only)",
		commatrixLogMsgPrefix, poolName, len(nodeBuilders)))

	return false
}

// commatrixInferMasterMCPoolNameFromPools selects the control-plane MCP from commatrix-discovered pool names.
func commatrixInferMasterMCPoolNameFromPools(pools []string) string {
	onCluster := commatrixFilterPoolsOnCluster(pools)

	klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: infer master MachineConfigPool from on-cluster pools: %v", commatrixLogMsgPrefix, onCluster))

	for _, poolName := range onCluster {
		if strings.EqualFold(poolName, "master") {
			klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: selected master MachineConfigPool %q (standard pool name)", commatrixLogMsgPrefix, poolName))

			return poolName
		}
	}

	for _, poolName := range onCluster {
		if commatrixPoolIsControlPlaneOnly(poolName) {
			klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: selected master MachineConfigPool %q (control-plane nodes only)", commatrixLogMsgPrefix, poolName))

			return poolName
		}
	}

	for _, poolName := range onCluster {
		lower := strings.ToLower(poolName)
		if strings.Contains(lower, "master") && !strings.Contains(lower, "storage") {
			klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: selected master MachineConfigPool %q (name contains master)", commatrixLogMsgPrefix, poolName))

			return poolName
		}
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: could not infer master MachineConfigPool from pools %v", commatrixLogMsgPrefix, onCluster))

	return ""
}

// commatrixInferSecureMCPoolNameFromPools selects the worker MCP used for host-firewall connectivity probes.
func commatrixInferSecureMCPoolNameFromPools(pools []string) string {
	onCluster := commatrixFilterPoolsOnCluster(pools)
	masterPool := commatrixInferMasterMCPoolNameFromPools(onCluster)

	klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: infer secure worker MachineConfigPool from on-cluster pools=%v masterPool=%q",
		commatrixLogMsgPrefix, onCluster, masterPool))

	candidates := make([]string, 0, len(onCluster))

	for _, poolName := range onCluster {
		if poolName == masterPool {
			continue
		}

		if commatrixPoolIsControlPlaneOnly(poolName) {
			klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
				"%s: skip pool %q for secure worker MCP (control-plane only)", commatrixLogMsgPrefix, poolName))

			continue
		}

		if commatrixPoolIsStorageOnly(poolName) {
			klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
				"%s: skip pool %q for secure worker MCP (storage-only nodes)", commatrixLogMsgPrefix, poolName))

			continue
		}

		if !commatrixPoolHasWorkerNodes(poolName) {
			klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
				"%s: skip pool %q for secure worker MCP (no worker nodes)", commatrixLogMsgPrefix, poolName))

			continue
		}

		candidates = append(candidates, poolName)
	}

	if len(candidates) == 0 {
		for _, poolName := range onCluster {
			if poolName != masterPool {
				candidates = append(candidates, poolName)
			}
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: no worker MachineConfigPool matched heuristics; fallback non-master candidates: %v",
			commatrixLogMsgPrefix, candidates))
	}

	if len(candidates) == 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: could not infer secure worker MachineConfigPool from pools %v", commatrixLogMsgPrefix, onCluster))

		return ""
	}

	sort.Strings(candidates)

	if len(candidates) > 1 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: multiple secure worker MachineConfigPool candidates %v; using %q",
			commatrixLogMsgPrefix, candidates, candidates[0]))
	} else {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: selected secure worker MachineConfigPool %q", commatrixLogMsgPrefix, candidates[0]))
	}

	return candidates[0]
}

// commatrixSetWorkerFromNodeName records the secure-worker node name and external probe IPs in workflow state.
func commatrixSetWorkerFromNodeName(nodeName string) error {
	nb, errPull := nodes.Pull(APIClient, nodeName)
	if errPull != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: pull secure worker node %q failed: %v", commatrixLogMsgPrefix, nodeName, errPull))

		return fmt.Errorf("pull node %q: %w", nodeName, errPull)
	}

	ips, errIPs := commatrixNodeProbeIPs(nb)
	if errIPs != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: resolve probe IPs for secure worker node %q failed: %v",
			commatrixLogMsgPrefix, nodeName, errIPs))

		return errIPs
	}

	commatrixWorkflow.run.SecureWorkerName = nodeName
	commatrixWorkflow.run.SecureWorkerIPs = ips

	klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: recorded secure worker node %q probe IPs %v", commatrixLogMsgPrefix, nodeName, ips))

	return nil
}

// commatrixResolveConnectivityTopology fills master and secure-worker names and probe IPs for connectivity and journal specs.
func commatrixResolveConnectivityTopology() error {
	commatrixWorkflow.run.MasterIPs = nil
	commatrixWorkflow.run.SecureWorkerName = ""
	commatrixWorkflow.run.SecureWorkerIPs = nil
	commatrixWorkflow.run.SecureOpenPort = 0
	commatrixWorkflow.run.SecureBlockedPort = 0

	masters, err := nodes.List(APIClient, metav1.ListOptions{LabelSelector: commatrixMasterLabelSelector()})
	if err != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: list control-plane nodes (selector %q) failed: %v",
			commatrixLogMsgPrefix, commatrixMasterLabelSelector(), err))

		return fmt.Errorf("list master nodes: %w", err)
	}

	if len(masters) == 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: no nodes matched control-plane label selector %q",
			commatrixLogMsgPrefix, commatrixMasterLabelSelector()))

		return fmt.Errorf("no nodes matched master label %q", commatrixMasterLabelSelector())
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: resolving connectivity topology (master label selector %q, %d control-plane node(s))",
		commatrixLogMsgPrefix, commatrixMasterLabelSelector(), len(masters)))

	for i, master := range masters {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: control-plane candidate[%d]=%q",
			commatrixLogMsgPrefix, i, master.Definition.Name))
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: using control-plane node %q for master API probe IPs",
		commatrixLogMsgPrefix, masters[0].Definition.Name))

	masterIPs, err := commatrixNodeProbeIPs(masters[0])
	if err != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: resolve probe IPs for control-plane node %q failed: %v",
			commatrixLogMsgPrefix, masters[0].Definition.Name, err))

		return err
	}

	commatrixWorkflow.run.MasterIPs = masterIPs

	nftNode := strings.TrimSpace(commatrixWorkflow.nftProbeNodeName)
	if nftNode == "" {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: nft probe node not set (call commatrixPrimeWorkflowForVerification first)",
			commatrixLogMsgPrefix))

		return fmt.Errorf("secure worker: nft probe node not recorded (prime verification workflow first)")
	}

	if err := commatrixSetWorkerFromNodeName(nftNode); err != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: record secure worker from nft probe node %q failed: %v",
			commatrixLogMsgPrefix, nftNode, err))

		return fmt.Errorf("secure worker %q: %w", nftNode, err)
	}

	if err := commatrixResolveSecureProbePorts(nftNode); err != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: resolve secure worker probe ports on %q failed: %v",
			commatrixLogMsgPrefix, nftNode, err))

		return fmt.Errorf("resolve secure worker probe ports on %q: %w", nftNode, err)
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
		"%s: connectivity topology masterIPs=%v secureWorker=%q secureWorkerIPs=%v openPort=%d blockedPort=%d nftProbeNode=%q",
		commatrixLogMsgPrefix,
		commatrixWorkflow.run.MasterIPs,
		commatrixWorkflow.run.SecureWorkerName,
		commatrixWorkflow.run.SecureWorkerIPs,
		commatrixWorkflow.run.SecureOpenPort,
		commatrixWorkflow.run.SecureBlockedPort,
		nftNode))

	return nil
}

// commatrixProbeHostAddr returns the host portion of an OVN/node address (strips /prefix when present).
func commatrixProbeHostAddr(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("empty address")
	}

	if strings.Contains(raw, "/") {
		ip, _, err := net.ParseCIDR(raw)
		if err != nil {
			return "", fmt.Errorf("parse address %q: %w", raw, err)
		}

		return ip.String(), nil
	}

	ip := net.ParseIP(raw)
	if ip == nil {
		return "", fmt.Errorf("invalid address %q", raw)
	}

	return ip.String(), nil
}

// commatrixNodeProbeIPs returns IPv4 then IPv6 node addresses for TCP probes via eco-goinfra nodes helpers.
func commatrixNodeProbeIPs(nb *nodes.Builder) ([]string, error) {
	var ips []string

	if ipv4, err := nb.ExternalIPv4Network(); err == nil {
		if addr, errHost := commatrixProbeHostAddr(ipv4); errHost == nil {
			ips = append(ips, addr)
		}
	}

	if ipv6, err := nb.ExternalIPv6Network(); err == nil {
		if addr, errHost := commatrixProbeHostAddr(ipv6); errHost == nil {
			ips = append(ips, addr)
		}
	}

	if len(ips) == 0 {
		nodeName := ""
		if nb != nil {
			nodeName = nb.Definition.Name
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: node %q has no external IPv4/IPv6 address for TCP probes",
			commatrixLogMsgPrefix, nodeName))

		return nil, fmt.Errorf("no external IPv4/IPv6 address on node %q", nodeName)
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: node %q probe IPs: %v", commatrixLogMsgPrefix, nb.Definition.Name, ips))

	return ips, nil
}

// commatrixNodesFromMachineConfigPools returns sorted unique node names matching each pool's
// MachineConfigPool.spec.nodeSelector on the live cluster.
func commatrixNodesFromMachineConfigPools(poolNames []string) ([]string, error) {
	seen := make(map[string]struct{})

	var out []string

	for _, poolName := range poolNames {
		mcpB, errPull := mco.Pull(APIClient, poolName)
		if errPull != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: pull MachineConfigPool %q failed: %v", commatrixLogMsgPrefix, poolName, errPull))

			return nil, fmt.Errorf("MachineConfigPool %q: %w", poolName, errPull)
		}

		mcpObj, errGet := mcpB.Get()
		if errGet != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: get MachineConfigPool %q failed: %v", commatrixLogMsgPrefix, poolName, errGet))

			return nil, fmt.Errorf("get MachineConfigPool %q: %w", poolName, errGet)
		}

		sel := mcpObj.Spec.NodeSelector
		if sel == nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
				"MachineConfigPool %q has nil nodeSelector; skipping node enumeration for this pool", poolName))

			continue
		}

		nodeLabelSel, errLS := metav1.LabelSelectorAsSelector(sel)
		if errLS != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: MachineConfigPool %q nodeSelector invalid: %v", commatrixLogMsgPrefix, poolName, errLS))

			return nil, fmt.Errorf("MachineConfigPool %q nodeSelector: %w", poolName, errLS)
		}

		labelStr := nodeLabelSel.String()
		if labelStr == "" {
			continue
		}

		nodeList, errList := nodes.List(APIClient, metav1.ListOptions{LabelSelector: labelStr})
		if errList != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: list nodes for pool %q (selector %q) failed: %v",
				commatrixLogMsgPrefix, poolName, labelStr, errList))

			return nil, fmt.Errorf("list nodes for pool %q (%s): %w", poolName, labelStr, errList)
		}

		for _, nb := range nodeList {
			if nb == nil || nb.Object == nil {
				continue
			}

			name := nb.Object.Name
			if _, ok := seen[name]; !ok {
				seen[name] = struct{}{}
				out = append(out, name)
			}
		}
	}

	sort.Strings(out)

	klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: pools %v -> node name(s) %v", commatrixLogMsgPrefix, poolNames, out))

	return out, nil
}

// commatrixRunOnNodeHostShell runs shellCmd on the node host via the MCO daemon pod (chroot /rootfs).
func commatrixRunOnNodeHostShell(nodeName, purpose, shellCmd string) (string, error) {
	klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
		"%s: on node %q (%s), shell: %s", commatrixLogMsgPrefix, nodeName, purpose, shellCmd))

	return commatrixRunOnNodeHostDebug(nodeName, []string{"chroot", "/rootfs", "sh", "-c", shellCmd})
}

// commatrixVerifyHostFirewallConnectivity probes TCP reachability from the test runner using ports from openshift_filter rules.
//
//nolint:funlen // connectivity: topology summary plus master/secure/peer probe cases with logging.
func commatrixVerifyHostFirewallConnectivity(_ SpecContext) {
	By("Resolving cluster topology for connectivity probes")

	Expect(commatrixResolveConnectivityTopology()).NotTo(HaveOccurred())

	openPort := commatrixWorkflow.run.SecureOpenPort
	blockedPort := commatrixWorkflow.run.SecureBlockedPort
	Expect(openPort).To(BeNumerically(">", 0), "secure worker open probe port must be resolved from nft rules")
	Expect(blockedPort).To(BeNumerically(">", 0), "secure worker blocked probe port must be resolved from nft rules")

	openPortStr := strconv.Itoa(openPort)
	blockedPortStr := strconv.Itoa(blockedPort)
	apiPortStr := strconv.Itoa(commatrixAPIPort)

	klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
		"%s: starting connectivity probes from test runner masterIPs=%v secureWorker=%q secureWorkerIPs=%v apiPort=%s openPort=%s blockedPort=%s",
		commatrixLogMsgPrefix,
		commatrixWorkflow.run.MasterIPs,
		commatrixWorkflow.run.SecureWorkerName,
		commatrixWorkflow.run.SecureWorkerIPs,
		apiPortStr,
		openPortStr,
		blockedPortStr))

	tryProbe := func(desc string, addrs []string, port string, expectConnect bool) {
		expectLabel := "connect"
		if !expectConnect {
			expectLabel = "blocked"
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: connectivity probe %q -> addrs=%v port=%s expect=%s",
			commatrixLogMsgPrefix, desc, addrs, port, expectLabel))
		By(desc)

		Expect(commatrixTryTCPProbesFromRunner(desc, addrs, port, expectConnect)).NotTo(HaveOccurred(), "%s", desc)
	}

	tryProbe("Master API reachable from test runner", commatrixWorkflow.run.MasterIPs, apiPortStr, true)

	tryProbe(
		fmt.Sprintf("Secure-pool worker blocked tcp/%s from test runner (not in nft accept rules)", blockedPortStr),
		commatrixWorkflow.run.SecureWorkerIPs, blockedPortStr, false)

	tryProbe(
		fmt.Sprintf("Secure-pool worker open tcp/%s from test runner (from nft accept rules)", openPortStr),
		commatrixWorkflow.run.SecureWorkerIPs, openPortStr, true)

	securePool := commatrixInferSecureMCPoolNameFromPools(commatrixWorkflow.probePoolNames)
	if securePool == "" {
		securePool = commatrixInferSecureMCPoolNameFromPools(commatrixWorkflow.discoveredPoolNames)
	}

	var securePoolPeerNames []string

	if securePool != "" {
		var errPeers error

		securePoolPeerNames, errPeers = commatrixNodesFromMachineConfigPools([]string{securePool})
		Expect(errPeers).NotTo(HaveOccurred())
	}

	if len(securePoolPeerNames) >= 2 {
		var peerName string

		for _, name := range securePoolPeerNames {
			if name != commatrixWorkflow.run.SecureWorkerName {
				peerName = name

				break
			}
		}

		Expect(peerName).NotTo(BeEmpty(), "need a second node in secure pool besides %s", commatrixWorkflow.run.SecureWorkerName)

		peerNB, errPull := nodes.Pull(APIClient, peerName)
		Expect(errPull).NotTo(HaveOccurred())

		peerIPs, errIP := commatrixNodeProbeIPs(peerNB)
		Expect(errIP).NotTo(HaveOccurred())

		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: peer secure-pool worker probe on node %q addrs=%v port=%s expect=blocked",
			commatrixLogMsgPrefix, peerName, peerIPs, blockedPortStr))

		tryProbe(
			fmt.Sprintf("Peer secure-pool worker blocked tcp/%s from test runner", blockedPortStr),
			peerIPs, blockedPortStr, false)
	} else {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: skipping peer secure-pool worker probe (securePool=%q nodeCount=%d need >=2)",
			commatrixLogMsgPrefix, securePool, len(securePoolPeerNames)))
	}
}

// commatrixRunJournalKernelGrep runs journalctl -k on a node host and greps kernel output for filter.
func commatrixRunJournalKernelGrep(nodeName, since, until, grepFilter string) (string, error) {
	shellCmd := fmt.Sprintf(`journalctl -k --since %q`, since)
	if strings.TrimSpace(until) != "" {
		shellCmd += fmt.Sprintf(` --until %q`, until)
	}

	shellCmd += fmt.Sprintf(` 2>/dev/null | grep -F %q || true`, grepFilter)

	purpose := fmt.Sprintf("kernel journal grep %q since %q", grepFilter, since)
	if strings.TrimSpace(until) != "" {
		purpose = fmt.Sprintf("kernel journal grep %q since %q until %q", grepFilter, since, until)
	}

	return commatrixRunOnNodeHostShell(nodeName, purpose, shellCmd)
}

// commatrixWaitForJournalKernelGrep runs journalctl -k --since on a node (via MCO daemon pod), greps for filter,
// and polls until at least minLines kernel lines are present.
func commatrixWaitForJournalKernelGrep(
	nodeName, since, grepFilter string,
	minLines int,
	interval, timeout time.Duration,
) (lines []string, raw string, err error) {
	err = wait.PollUntilContextTimeout(context.TODO(), interval, timeout, true,
		func(context.Context) (bool, error) {
			var pollErr error

			raw, pollErr = commatrixRunJournalKernelGrep(nodeName, since, "", grepFilter)
			if pollErr != nil {
				return false, pollErr
			}

			lines = commatrixParseJournalKernelLines(raw)

			klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
				"%s: waiting for kernel journal lines on node %q filter=%q since=%q (have %d, need %d)",
				commatrixLogMsgPrefix, nodeName, grepFilter, since, len(lines), minLines))

			return len(lines) >= minLines, nil
		})

	return lines, raw, err
}

// commatrixParseJournalKernelLines filters journalctl output down to plausible kernel firewall log lines.
func commatrixParseJournalKernelLines(raw string) []string {
	var out []string

	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" ||
			strings.HasPrefix(line, "Starting pod/") ||
			strings.HasPrefix(line, "To use host binaries") ||
			strings.HasPrefix(line, "Removing debug pod") ||
			strings.HasPrefix(line, "error: non-zero exit code") {
			continue
		}

		if strings.Contains(line, "kernel:") || journalShortTimePrefixRe.MatchString(line) {
			out = append(out, line)
		}
	}

	return out
}

// commatrixExpectFirewallLogRateLimitsInWindow counts firewall / firewall-space log lines in one journal window (≤5 per bucket).
func commatrixExpectFirewallLogRateLimitsInWindow(lines []string, windowLabel string) {
	counts := map[string]int{
		commatrixFirewallLogPrefixNoSpace:   0,
		commatrixFirewallLogPrefixWithSpace: 0,
	}

	for _, line := range lines {
		prefix, ok := commatrixFirewallLogBucket(line)
		if !ok {
			continue
		}

		counts[prefix]++
	}

	for _, prefix := range []string{commatrixFirewallLogPrefixNoSpace, commatrixFirewallLogPrefixWithSpace} {
		n := counts[prefix]
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
			"%s: firewall journal window %q bucket %q: %d line(s) (max %d/min)",
			commatrixLogMsgPrefix, windowLabel, prefix, n, commatrixFirewallRateLimitPerMinute))

		Expect(n).To(BeNumerically("<=", commatrixFirewallRateLimitPerMinute),
			"firewall journal %s: bucket %q: %d lines (max %d per minute)", windowLabel, prefix, n, commatrixFirewallRateLimitPerMinute)
	}
}

// commatrixFirewallLogBucket classifies a journal line into a firewall log-prefix bucket for rate-limit checks.
func commatrixFirewallLogBucket(line string) (string, bool) {
	if strings.Contains(line, "TCP_TEST") {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
			"%s: firewall journal line -> false (TCP_TEST probe line)",
			commatrixLogMsgPrefix))

		return "", false
	}

	const tag = "kernel: "

	i := strings.Index(line, tag)
	if i < 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
			"%s: firewall journal line -> false (no %q prefix): %q",
			commatrixLogMsgPrefix, tag, commatrixLogOutputSnippet(line, 200)))

		return "", false
	}

	switch msg := line[i+len(tag):]; {
	case strings.HasPrefix(msg, "firewall IN="):
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
			"%s: firewall journal line -> true (bucket %q)",
			commatrixLogMsgPrefix, commatrixFirewallLogPrefixWithSpace))

		return commatrixFirewallLogPrefixWithSpace, true
	case strings.HasPrefix(msg, "firewallIN="):
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
			"%s: firewall journal line -> true (bucket %q)",
			commatrixLogMsgPrefix, commatrixFirewallLogPrefixNoSpace))

		return commatrixFirewallLogPrefixNoSpace, true
	default:
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
			"%s: firewall journal line -> false (kernel message not firewall log): %q",
			commatrixLogMsgPrefix, commatrixLogOutputSnippet(msg, 200)))

		return "", false
	}
}

// commatrixVerifyFirewallJournal checks firewall journal rate limits and TCP_TEST log-drop probe.
// Uses the same secure worker node as connectivity; run connectivity before this spec when possible.
//
//nolint:funlen // journal: two 1-minute windows, TCP_TEST rule inject/probe, and journal assertions.
func commatrixVerifyFirewallJournal(_ SpecContext) {
	journalNode := commatrixWorkflow.run.SecureWorkerName
	Expect(journalNode).NotTo(BeEmpty(), "journal node: run connectivity spec first to resolve secure worker")

	journalNB, errPull := nodes.Pull(APIClient, journalNode)
	Expect(errPull).NotTo(HaveOccurred(), "pull journal node %q", journalNode)

	journalProbeIPs, errIP := commatrixNodeProbeIPs(journalNB)
	Expect(errIP).NotTo(HaveOccurred(), "probe IP(s) for journal node %s", journalNode)

	keyword := commatrixNFTablesLogKeyword

	klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: starting firewall journal checks on node %q probeIPs=%v keyword=%q",
		commatrixLogMsgPrefix, journalNode, journalProbeIPs, keyword))

	By(fmt.Sprintf("Verifying firewall log rate limits on node %s (two 1-minute windows)", journalNode))

	window1Label := fmt.Sprintf("%s to %s", commatrixJournalSinceTwoMinutes, commatrixJournalSinceOneMinute)
	klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: reading firewall journal window %q on node %q", commatrixLogMsgPrefix, window1Label, journalNode))

	window1Raw, errWin1 := commatrixRunJournalKernelGrep(
		journalNode, commatrixJournalSinceTwoMinutes, commatrixJournalSinceOneMinute, keyword)
	Expect(errWin1).NotTo(HaveOccurred(), "firewall journal window 1 on %s: %s", journalNode, window1Raw)

	window1Lines := commatrixParseJournalKernelLines(window1Raw)
	klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: firewall journal window %q on node %q: %d kernel line(s) matching %q",
		commatrixLogMsgPrefix, window1Label, journalNode, len(window1Lines), keyword))
	commatrixExpectFirewallLogRateLimitsInWindow(window1Lines, window1Label)

	klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: reading firewall journal window %q on node %q",
		commatrixLogMsgPrefix, commatrixJournalSinceOneMinute, journalNode))

	window2Raw, errWin2 := commatrixRunJournalKernelGrep(journalNode, commatrixJournalSinceOneMinute, "", keyword)
	Expect(errWin2).NotTo(HaveOccurred(), "firewall journal window 2 on %s: %s", journalNode, window2Raw)

	window2Lines := commatrixParseJournalKernelLines(window2Raw)
	if len(window2Lines) == 0 {
		warnMsg := fmt.Sprintf(
			"%s: firewall journal window %q on node %q: no kernel lines matching %q; "+
				"skipping window-2 rate-limit checks (traffic may be quiet). Last output: %q",
			commatrixLogMsgPrefix, commatrixJournalSinceOneMinute, journalNode, keyword,
			commatrixLogOutputSnippet(window2Raw, 500))
		klog.Warning(warnMsg)
		_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: %s\n", warnMsg)
	} else {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: firewall journal window %q on node %q: %d kernel line(s) matching %q",
			commatrixLogMsgPrefix, commatrixJournalSinceOneMinute, journalNode, len(window2Lines), keyword))

		commatrixExpectFirewallLogRateLimitsInWindow(window2Lines, commatrixJournalSinceOneMinute)
	}

	testPort := commatrixWorkflow.run.SecureBlockedPort
	if testPort <= 0 {
		Expect(commatrixResolveSecureProbePorts(journalNode)).NotTo(HaveOccurred(),
			"resolve blocked probe port on journal node %q", journalNode)

		testPort = commatrixWorkflow.run.SecureBlockedPort
	}

	Expect(testPort).To(BeNumerically(">", 0), "journal TCP_TEST probe port must be resolved from nft rules")

	klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: journal TCP_TEST setup on node %q probeIPs=%v blockedPort=%d chain=%s",
		commatrixLogMsgPrefix, journalNode, journalProbeIPs, testPort, commatrixNFTablesOpenshiftChain))

	By(fmt.Sprintf("Injecting TCP_TEST log rule on node %s for tcp/%d", journalNode, testPort))

	nftInsertHostShellCmd := fmt.Sprintf(
		`set -e; nft insert rule inet openshift_filter %s tcp dport %d log prefix \"TCP_TEST \" drop`,
		commatrixNFTablesOpenshiftChain, testPort)

	nftInsertCmdOut, nftInsertCmdErr := commatrixRunOnNodeHostShell(
		journalNode, fmt.Sprintf("insert TCP_TEST nft drop rule for tcp/%d", testPort), nftInsertHostShellCmd)
	Expect(nftInsertCmdErr).NotTo(HaveOccurred(), "nft insert TCP_TEST rule on %s: %s", journalNode, nftInsertCmdOut)

	klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: inserted TCP_TEST rule on node %q: %q",
		commatrixLogMsgPrefix, journalNode, commatrixLogOutputSnippet(nftInsertCmdOut, 200)))

	defer func() {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: removing TCP_TEST rule on node %q (best-effort)",
			commatrixLogMsgPrefix, journalNode))

		nftDeleteLogRuleShellCmd := fmt.Sprintf(
			`HANDLE=$(nft -a list chain inet openshift_filter %s 2>/dev/null | grep TCP_TEST | tail -1 | sed -E "s/.*handle ([0-9]+).*/\\1/"); [ -n "$HANDLE" ] && nft delete rule inet openshift_filter %s handle "$HANDLE" || true`,
			commatrixNFTablesOpenshiftChain, commatrixNFTablesOpenshiftChain)

		_, _ = commatrixRunOnNodeHostShell(journalNode, "remove TCP_TEST nft rule", nftDeleteLogRuleShellCmd)
	}()

	portStr := strconv.Itoa(testPort)

	By(fmt.Sprintf("Probing %v:%d from test runner (TCP_TEST drop expected)", journalProbeIPs, testPort))

	for _, probeTargetIP := range journalProbeIPs {
		klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
			"%s: journal TCP_TEST probe from test runner -> %s:%d (expect drop)",
			commatrixLogMsgPrefix, probeTargetIP, testPort))

		if err := commatrixRunTCPProbeFromRunner(false, probeTargetIP, portStr); err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf(
				"%s: journal TCP_TEST probe %s:%d failed as expected: %v",
				commatrixLogMsgPrefix, probeTargetIP, testPort, err))
		}
	}

	By("Verifying TCP_TEST in kernel journal")

	klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: waiting for TCP_TEST kernel journal lines on node %q (min 1, timeout 90s)",
		commatrixLogMsgPrefix, journalNode))

	tcpTestLines, tcpTestRaw, err := commatrixWaitForJournalKernelGrep(
		journalNode, commatrixJournalSinceOneMinute, "TCP_TEST", 1, 3*time.Second, 90*time.Second)
	Expect(err).NotTo(HaveOccurred(),
		"TCP_TEST journal: expected at least one TCP_TEST kernel log line after probe to %v:%d on %s (got %d); last output:\n%s",
		journalProbeIPs, testPort, journalNode, len(tcpTestLines), tcpTestRaw)

	Expect(tcpTestLines).NotTo(BeEmpty(), "TCP_TEST journal: expected ≥1 TCP_TEST log line on %s", journalNode)

	klog.V(rdscoreparams.RDSCoreLogLevel).Info(fmt.Sprintf("%s: found %d TCP_TEST kernel journal line(s) on node %q: %v",
		commatrixLogMsgPrefix, len(tcpTestLines), journalNode, tcpTestLines))

	dptNeedle := fmt.Sprintf("DPT=%d", testPort)

	journalJoined := strings.ToUpper(strings.Join(tcpTestLines, "\n"))

	Expect(journalJoined).To(ContainSubstring("TCP_TEST"),
		"TCP_TEST journal: journal lines should include TCP_TEST log prefix from injected rule")
	Expect(journalJoined).To(ContainSubstring(strings.ToUpper(dptNeedle)),
		"TCP_TEST journal: journal lines should reference probed destination port %s", dptNeedle)
}

// VerifyCommatrixHostFirewallConnectivity (reportxml 95003/95005/95007) verifies TCP connectivity from the test runner.
func VerifyCommatrixHostFirewallConnectivity(ctx SpecContext) {
	Expect(commatrixPrimeWorkflowForVerification()).NotTo(HaveOccurred(),
		"host-firewall rules must be applied on the cluster before connectivity checks")

	commatrixVerifyHostFirewallConnectivity(ctx)
}

// VerifyCommatrixHostFirewallJournal (reportxml 95004/95006/95008) verifies firewall journal rate limits and TCP_TEST logging.
func VerifyCommatrixHostFirewallJournal(ctx SpecContext) {
	Expect(commatrixPrimeWorkflowForVerification()).NotTo(HaveOccurred(),
		"host-firewall rules must be applied on the cluster before journal checks")

	if strings.TrimSpace(commatrixWorkflow.run.SecureWorkerName) == "" {
		Expect(commatrixResolveConnectivityTopology()).NotTo(HaveOccurred(),
			"resolve connectivity topology before journal checks")
	}

	commatrixVerifyFirewallJournal(ctx)
}
