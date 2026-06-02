// Commatrix host-firewall workflow (ginkgo reportxml 95001–95008): optional generate/apply helpers,
// plus connectivity (95003/95005/95007) and journal (95004/95006/95008) verification against a cluster that already has rules applied.
//
//nolint:varnamelen,lll,wsl_v5 // test helpers follow oc/k8s naming; long oc/shell/klog lines are intentional.
package rdscorecommon

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	mcv1 "github.com/openshift/api/machineconfiguration/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/mco"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	apimachinerytypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/remote"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreparams"
)

const (
	commatrixFormatMCSubdir             = "format-mc"
	commatrixFormatButaneSubdir         = "format-butane"
	commatrixFileExtYAML                = ".yaml"
	commatrixFileExtYML                 = ".yml"
	commatrixNDPNFTFilePath             = "/etc/sysconfig/nftables.conf"
	commatrixNDPNFTUnitName             = "nftables.service"
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
)

// commatrixMCPStableWait is how long apply/revert waits for all MachineConfigPools to stabilize.
const commatrixMCPStableWait = 15 * time.Minute

// journalShortTimePrefixRe parses journalctl short-iso timestamps for firewall log rate-limit checks.
var journalShortTimePrefixRe = regexp.MustCompile(`^([A-Z][a-z]{2}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2})`)

// commatrixJSONPatchOp is one RFC 6902 operation for MachineConfiguration cluster JSON patches.
type commatrixJSONPatchOp struct {
	Op   string `json:"op"`
	Path string `json:"path"`
}

// commatrixBuildJSONPatchRemoveNDPNFT builds a JSON Patch that removes the nftables file and unit from
// spec.nodeDisruptionPolicy by list index (operator.openshift.io MachineConfiguration does not support strategic patch).
func commatrixBuildJSONPatchRemoveNDPNFT(clusterJSON string) ([]byte, error) {
	var root map[string]interface{}

	if err := json.Unmarshal([]byte(clusterJSON), &root); err != nil {
		return nil, fmt.Errorf("parse MachineConfiguration cluster JSON: %w", err)
	}

	spec, hasSpec := root["spec"].(map[string]interface{})
	if !hasSpec {
		return nil, fmt.Errorf("cluster JSON missing spec")
	}

	ndp, hasNDP := spec["nodeDisruptionPolicy"].(map[string]interface{})
	if !hasNDP {
		return []byte("[]"), nil
	}

	var ops []commatrixJSONPatchOp

	if files, hasFiles := ndp["files"].([]interface{}); hasFiles {
		for fileIdx, item := range files {
			fm, isFileMap := item.(map[string]interface{})
			if !isFileMap {
				continue
			}

			path, _ := fm["path"].(string)
			if filepath.Clean(path) == filepath.Clean(commatrixNDPNFTFilePath) {
				ops = append(ops, commatrixJSONPatchOp{
					Op:   "remove",
					Path: fmt.Sprintf("/spec/nodeDisruptionPolicy/files/%d", fileIdx),
				})

				break
			}
		}
	}

	if units, hasUnits := ndp["units"].([]interface{}); hasUnits {
		for unitIdx, item := range units {
			um, isUnitMap := item.(map[string]interface{})
			if !isUnitMap {
				continue
			}

			name, _ := um["name"].(string)
			if name == commatrixNDPNFTUnitName {
				ops = append(ops, commatrixJSONPatchOp{
					Op:   "remove",
					Path: fmt.Sprintf("/spec/nodeDisruptionPolicy/units/%d", unitIdx),
				})

				break
			}
		}
	}

	if len(ops) == 0 {
		return []byte("[]"), nil
	}

	patchBytes, err := json.Marshal(ops)
	if err != nil {
		return nil, err
	}

	return patchBytes, nil
}

// commatrixRunTopology holds node names and probe IPs resolved for connectivity checks.
type commatrixRunTopology struct {
	SecureWorkerName string
	MasterIPs        []string
	SecureWorkerIPs  []string
}

// commatrixWorkflowState carries shared state for the ordered Commatrix spec.
type commatrixWorkflowState struct {
	run                  commatrixRunTopology
	mcApplied            bool
	ndpApplied           bool
	revertNodeNames      []string
	generatedMCPoolNames []string
	appliedMCPoolNames   []string
	appliedMCRels        []string
	nftProbeNodeName     string
}

var commatrixWorkflow commatrixWorkflowState

func commatrixResetWorkflow() {
	commatrixWorkflow = commatrixWorkflowState{}
}

// commatrixPrimeWorkflowForVerification discovers host-firewall pools from commatrix MachineConfigs
// already on the cluster (95003/95004 assume platform prep or 95002 apply completed there).
func commatrixPrimeWorkflowForVerification() error {
	if commatrixWorkflow.mcApplied {
		return nil
	}

	poolNames, err := commatrixHostFirewallPoolNamesFromCluster()
	if err != nil {
		return fmt.Errorf("discover host-firewall MachineConfig pools: %w", err)
	}

	if len(poolNames) == 0 {
		detail := commatrixHostFirewallMCDiscoveryDetail()
		klog.Warningf("Commatrix: host-firewall MC discovery failed: %s", detail)

		return fmt.Errorf(
			"no commatrix host-firewall MachineConfigs on cluster (look for %q in mc names); %s; "+
				"apply rules before connectivity/journal tests (run 95001/95002 or platform prep)",
			commatrixHostFirewallMCNameSubstring, detail)
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Commatrix: discovered host-firewall MC pools on cluster: %v", poolNames)

	commatrixWorkflow.generatedMCPoolNames = append([]string(nil), poolNames...)

	securePool := commatrixInferSecureMCPoolNameFromPools(poolNames)
	if securePool == "" {
		return fmt.Errorf("could not infer secure/firewall MCP from pools %v", poolNames)
	}

	masterPool := commatrixInferMasterMCPoolNameFromPools(poolNames)

	applyPools := []string{securePool}
	if masterPool != "" && masterPool != securePool {
		applyPools = append(applyPools, masterPool)
	}

	commatrixWorkflow.appliedMCPoolNames = append([]string(nil), applyPools...)

	secureNodes, err := commatrixNodesFromMachineConfigPools([]string{securePool})
	if err != nil {
		return fmt.Errorf("list nodes in secure pool %q: %w", securePool, err)
	}

	if len(secureNodes) == 0 {
		return fmt.Errorf("no nodes in secure pool %q for connectivity/journal probes", securePool)
	}

	commatrixWorkflow.nftProbeNodeName = secureNodes[0]
	commatrixWorkflow.mcApplied = true

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
		"Commatrix: primed verification workflow pools=%v secureWorker=%q",
		applyPools, commatrixWorkflow.nftProbeNodeName)

	return nil
}

func commatrixMachineConfigPoolRole(mcObj *mcv1.MachineConfig) string {
	if role := strings.TrimSpace(mcObj.Labels[mcv1.MachineConfigRoleLabelKey]); role != "" {
		return role
	}

	// Commatrix MC names encode the pool (e.g. 98-nftables-commatrix-appworker-mcp-a) even when the role label is absent.
	markerIdx := strings.Index(mcObj.Name, commatrixHostFirewallMCNameSubstring)
	if markerIdx < 0 {
		return ""
	}

	return strings.TrimSpace(strings.TrimPrefix(mcObj.Name[markerIdx+len(commatrixHostFirewallMCNameSubstring):], "-"))
}

func commatrixHostFirewallPoolNamesFromCluster() ([]string, error) {
	mcList, err := mco.ListMC(APIClient)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	pools := make([]string, 0)

	for _, mcBuilder := range mcList {
		if mcBuilder == nil || mcBuilder.Object == nil {
			continue
		}

		mcObj := mcBuilder.Object
		if !strings.Contains(mcObj.Name, commatrixHostFirewallMCNameSubstring) {
			continue
		}

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

func commatrixMasterLabelSelector() string {
	return labels.Set(inittools.GeneralConfig.ControlPlaneLabelMap).String()
}

// commatrixRunOC runs the oc CLI (required for the commatrix plugin subcommand only).
func commatrixRunOC(ocCmdArgs ...string) (string, error) {
	ocCmd := exec.Command("oc", ocCmdArgs...)
	ocCmd.Env = os.Environ()

	ocCmdOut, ocCmdErr := ocCmd.CombinedOutput()

	return string(ocCmdOut), ocCmdErr
}

func commatrixLoadMachineConfigYAML(path string) (*mcv1.MachineConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var mc mcv1.MachineConfig

	if err := yaml.Unmarshal(raw, &mc); err != nil {
		return nil, fmt.Errorf("parse MachineConfig %s: %w", path, err)
	}

	if strings.TrimSpace(mc.Name) == "" {
		return nil, fmt.Errorf("MachineConfig %s has no metadata.name", path)
	}

	return &mc, nil
}

func commatrixApplyMachineConfigYAML(path string) error {
	mc, err := commatrixLoadMachineConfigYAML(path)
	if err != nil {
		return err
	}

	if err := APIClient.AttachScheme(mcv1.Install); err != nil {
		return err
	}

	existing := &mcv1.MachineConfig{}

	getErr := APIClient.Get(context.TODO(), goclient.ObjectKey{Name: mc.Name}, existing)
	if getErr != nil {
		if !k8serrors.IsNotFound(getErr) {
			return fmt.Errorf("get MachineConfig %q: %w", mc.Name, getErr)
		}

		return APIClient.Create(context.TODO(), mc)
	}

	mc.ResourceVersion = existing.ResourceVersion

	return APIClient.Update(context.TODO(), mc)
}

func commatrixDeleteMachineConfigYAML(path string) error {
	mc, err := commatrixLoadMachineConfigYAML(path)
	if err != nil {
		return err
	}

	mcBuilder := mco.NewMCBuilder(APIClient, mc.Name)
	if mcBuilder == nil {
		return fmt.Errorf("create MachineConfig builder for %q", mc.Name)
	}

	if !mcBuilder.Exists() {
		return nil
	}

	return mcBuilder.Delete()
}

func commatrixPatchMachineConfigurationClusterMerge(patchPath string) error {
	patchBytes, err := os.ReadFile(patchPath)
	if err != nil {
		return err
	}

	patchBytes = bytes.TrimSpace(patchBytes)
	if len(patchBytes) == 0 {
		return fmt.Errorf("NDP patch file %s is empty", patchPath)
	}

	mcCluster := &operatorv1.MachineConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
	}

	return APIClient.Patch(
		context.TODO(),
		mcCluster,
		goclient.RawPatch(apimachinerytypes.MergePatchType, patchBytes),
	)
}

func commatrixGetMachineConfigurationClusterJSON() ([]byte, error) {
	mcCluster := &operatorv1.MachineConfiguration{}

	err := APIClient.Get(context.TODO(), goclient.ObjectKey{Name: "cluster"}, mcCluster)
	if err != nil {
		return nil, fmt.Errorf("get machineconfiguration/cluster: %w", err)
	}

	clusterJSON, err := json.Marshal(mcCluster)
	if err != nil {
		return nil, fmt.Errorf("marshal machineconfiguration/cluster: %w", err)
	}

	return clusterJSON, nil
}

func commatrixPatchMachineConfigurationClusterJSON(patch []byte) error {
	if len(patch) == 0 || string(patch) == "[]" {
		return nil
	}

	mcCluster := &operatorv1.MachineConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
	}

	return APIClient.Patch(
		context.TODO(),
		mcCluster,
		goclient.RawPatch(apimachinerytypes.JSONPatchType, patch),
	)
}

func commatrixFormatMCPStatusSnapshot() (string, error) {
	mcpList, err := mco.ListMCP(APIClient)
	if err != nil {
		return "", err
	}

	var b strings.Builder

	for _, mcpBuilder := range mcpList {
		if mcpBuilder == nil || mcpBuilder.Object == nil {
			continue
		}

		mcpObj := mcpBuilder.Object

		_, _ = fmt.Fprintf(&b, "%s  updated=%d ready=%d machine=%d degraded=%d\n",
			mcpObj.Name,
			mcpObj.Status.UpdatedMachineCount,
			mcpObj.Status.ReadyMachineCount,
			mcpObj.Status.MachineCount,
			mcpObj.Status.DegradedMachineCount,
		)
	}

	return strings.TrimRight(b.String(), "\n"), nil
}

// commatrixRunLocalShell runs shellCmd on the machine executing the test (not via oc debug).
func commatrixRunLocalShell(shellCmd string) error {
	localCmd := exec.Command("bash", "-c", shellCmd)
	localCmd.Env = os.Environ()

	localCmdOut, localCmdErr := localCmd.CombinedOutput()

	if localCmdErr != nil {
		klog.Infof("local: bash -c %q failed: %v\n%s", shellCmd, localCmdErr, string(localCmdOut))
	} else {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("local: bash -c %q\n%s", shellCmd, string(localCmdOut))
	}

	return localCmdErr
}

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
	probeDesc := fmt.Sprintf("dial %s %s", network, addr)

	if expectConnect {
		if err := commatrixDialTCP(host, port); err != nil {
			klog.Infof("probe: %s failed: %v", probeDesc, err)

			return err
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("probe: %s succeeded", probeDesc)

		return nil
	}

	if err := commatrixDialTCP(host, port); err != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("probe: %s failed as expected: %v", probeDesc, err)

		return nil
	}

	klog.Infof("probe: %s unexpectedly connected", probeDesc)

	return fmt.Errorf("expected connection to %s to fail, but dial succeeded", addr)
}

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
		return fmt.Errorf("%s: no internal IP addresses to probe", desc)
	}

	if expectConnect {
		var failures []string

		for _, addr := range addrs {
			label := commatrixProbeLabel(addr)
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("TCP probe %s via %s -> %s:%s (expect connect)",
				desc, label, addr, port)

			if err := commatrixRunTCPProbeFromRunner(true, addr, port); err == nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("TCP probe %s succeeded via %s", desc, label)

				return nil
			} else {
				failures = append(failures, fmt.Sprintf("%s: %v", label, err))
			}
		}

		return fmt.Errorf("%s: none of %v reachable from test runner (%s)",
			desc, addrs, strings.Join(failures, "; "))
	}

	for _, addr := range addrs {
		label := commatrixProbeLabel(addr)
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("TCP probe %s via %s -> %s:%s (expect blocked)",
			desc, label, addr, port)

		if err := commatrixRunTCPProbeFromRunner(false, addr, port); err != nil {
			return fmt.Errorf("%s via %s: %w", desc, label, err)
		}
	}

	return nil
}

// commatrixRunOnNodeHostDebug runs cmd on the node host via the MCO daemon pod (chroot /rootfs).
func commatrixRunOnNodeHostDebug(nodeName string, cmd []string) (string, error) {
	hostDebugCmdOut, hostDebugCmdErr := remote.ExecuteOnNodeWithDebugPod(cmd, nodeName)

	if hostDebugCmdErr != nil {
		klog.Infof("node %s exec %v failed: %v\n%s", nodeName, cmd, hostDebugCmdErr, hostDebugCmdOut)
	} else {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("node %s exec %v\n%s", nodeName, cmd, hostDebugCmdOut)
	}

	return hostDebugCmdOut, hostDebugCmdErr
}

// commatrixButaneBasenameToMC replaces the first case-insensitive "butane" substring in a filename with "mc".
func commatrixButaneBasenameToMC(name string) (string, bool) {
	lower := strings.ToLower(name)
	i := strings.Index(lower, "butane")
	if i < 0 {
		return "", false
	}

	return name[:i] + "mc" + name[i+len("butane"):], true
}

type commatrixButaneRenderJob struct {
	srcRel string
	outRel string
}

func commatrixDiscoverButaneRenderJobs(buRawRoot string) ([]commatrixButaneRenderJob, error) {
	seenOut := make(map[string]string)

	var jobs []commatrixButaneRenderJob

	err := filepath.WalkDir(buRawRoot, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(buRawRoot, path)
		if err != nil {
			return err
		}

		base := filepath.Base(rel)
		ext := strings.ToLower(filepath.Ext(base))

		var outRel string

		switch {
		case strings.Contains(strings.ToLower(base), "butane") && (ext == commatrixFileExtYAML || ext == commatrixFileExtYML):
			newBase, ok := commatrixButaneBasenameToMC(base)
			if !ok {
				return nil
			}

			outRel = filepath.Join(filepath.Dir(rel), newBase)
		case ext == ".bu":
			outRel = filepath.Join(filepath.Dir(rel), strings.TrimSuffix(base, filepath.Ext(base))+commatrixFileExtYAML)
		default:
			return nil
		}

		outRel = filepath.Clean(outRel)

		if prev, ok := seenOut[outRel]; ok {
			return fmt.Errorf("duplicate rendered MC path %q from sources %q and %q", outRel, prev, rel)
		}

		seenOut[outRel] = rel
		jobs = append(jobs, commatrixButaneRenderJob{srcRel: rel, outRel: outRel})

		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].srcRel < jobs[j].srcRel
	})

	return jobs, nil
}

func commatrixYAMLEquivalent(leftYAML, rightYAML []byte) error {
	leftJSON, err := yaml.YAMLToJSON(bytes.TrimSpace(leftYAML))
	if err != nil {
		return fmt.Errorf("left yaml-to-json: %w", err)
	}

	rightJSON, err := yaml.YAMLToJSON(bytes.TrimSpace(rightYAML))
	if err != nil {
		return fmt.Errorf("right yaml-to-json: %w", err)
	}

	if !bytes.Equal(leftJSON, rightJSON) {
		return fmt.Errorf(
			"MachineConfig YAML differs after YAML→JSON normalization (left %d B, right %d B)",
			len(leftJSON), len(rightJSON))
	}

	return nil
}

func commatrixListMachineConfigYAMLRels(root string) ([]string, error) {
	var rels []string

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if d.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext != commatrixFileExtYAML && ext != commatrixFileExtYML {
			return nil
		}

		baseName := strings.ToLower(filepath.Base(path))
		if baseName == "node-disruption-policy.yaml" || baseName == "node-disruption-policy.yml" {
			return nil
		}

		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		if !bytes.Contains(raw, []byte("kind: MachineConfig")) && !bytes.Contains(raw, []byte("kind: \"MachineConfig\"")) {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		rels = append(rels, filepath.ToSlash(rel))

		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(rels)

	return rels, nil
}

// commatrixFindNodeDisruptionPolicyPatch returns the absolute path to a single node-disruption-policy.yaml
// under root (e.g. format-mc). MachineConfig apply skips that file; merged via API patch on MachineConfiguration cluster instead.
// If none are present, returns ("", nil). More than one match is an error.
func commatrixFindNodeDisruptionPolicyPatch(root string) (string, error) {
	var matches []string

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if d.IsDir() {
			return nil
		}

		if strings.ToLower(filepath.Base(path)) == "node-disruption-policy.yaml" {
			rel, errRel := filepath.Rel(root, path)
			if errRel != nil {
				return errRel
			}

			matches = append(matches, filepath.ToSlash(rel))
		}

		return nil
	})
	if err != nil {
		return "", err
	}

	sort.Strings(matches)

	switch len(matches) {
	case 0:
		return "", nil
	case 1:
		abs := filepath.Join(root, filepath.FromSlash(matches[0]))

		return filepath.Clean(abs), nil
	default:
		return "", fmt.Errorf("multiple node-disruption-policy.yaml files under %s: %v", root, matches)
	}
}

func commatrixCompareMCDirectVsRendered(mcRoot, renderRoot string) {
	mcRels, err := commatrixListMachineConfigYAMLRels(mcRoot)
	Expect(err).NotTo(HaveOccurred())
	Expect(mcRels).NotTo(BeEmpty(), "no MachineConfig YAML under direct mc output %s", mcRoot)

	renderedRels, err := commatrixListMachineConfigYAMLRels(renderRoot)
	Expect(err).NotTo(HaveOccurred())

	Expect(renderedRels).To(Equal(mcRels),
		"MachineConfig relative paths under %s and %s should match exactly", mcRoot, renderRoot)

	for _, rel := range mcRels {
		relOS := filepath.FromSlash(rel)

		left, err := os.ReadFile(filepath.Join(mcRoot, relOS))
		Expect(err).NotTo(HaveOccurred(), "read direct mc %s", rel)

		right, err := os.ReadFile(filepath.Join(renderRoot, relOS))
		Expect(err).NotTo(HaveOccurred(), "read butane-rendered mc %s", rel)

		errEq := commatrixYAMLEquivalent(left, right)
		Expect(errEq).NotTo(HaveOccurred(), "MachineConfig %q: direct mc vs butane-rendered differ: %v", rel, errEq)
	}
}

// commatrixDecodeDataURLLinePayload decodes ignition data: URL payloads from generated MCs:
// awk -F ',' '/source: data:/ {print $NF}' ... | base64 -d | gunzip
// Ignition often uses a single line `source: data:...;base64,<payload>`.
func commatrixDecodeDataURLLinePayload(line string) ([]byte, bool) {
	lineLower := strings.ToLower(line)
	if !strings.Contains(lineLower, "source:") || !strings.Contains(lineLower, "data:") {
		return nil, false
	}

	idx := strings.Index(lineLower, "base64,")
	if idx < 0 {
		return nil, false
	}

	b64 := strings.TrimSpace(line[idx+len("base64,"):])
	b64 = strings.Trim(b64, `"'`)

	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, false
	}

	if len(raw) >= 2 && raw[0] == 0x1f && raw[1] == 0x8b {
		zr, errZ := gzip.NewReader(bytes.NewReader(raw))
		if errZ != nil {
			return nil, false
		}

		out, errR := io.ReadAll(zr)
		_ = zr.Close()
		if errR != nil {
			return nil, false
		}

		return out, true
	}

	return raw, true
}

func commatrixCollectDecodedDataURLPayloads(mcYAML []byte) [][]byte {
	var out [][]byte

	for _, line := range strings.Split(string(mcYAML), "\n") {
		if p, ok := commatrixDecodeDataURLLinePayload(line); ok {
			out = append(out, p)
		}
	}

	return out
}

func commatrixExpectDecodedHostFirewallNFTables(decoded []byte, mcRel string) {
	s := strings.ToLower(string(decoded))

	Expect(s).To(ContainSubstring("table inet openshift_filter"),
		"mc %s: decoded payload should define table inet openshift_filter", mcRel)
	Expect(s).To(ContainSubstring("delete table inet openshift_filter"),
		"mc %s: decoded payload should delete table inet openshift_filter before recreate", mcRel)
	Expect(s).To(ContainSubstring("type filter hook input"),
		"mc %s: decoded payload should use an input filter hook (host firewall)", mcRel)
	Expect(s).To(ContainSubstring("tcp dport"),
		"mc %s: decoded payload should open tcp destination ports", mcRel)
	Expect(s).To(ContainSubstring("udp dport"),
		"mc %s: decoded payload should open udp destination ports", mcRel)
	Expect(s).To(ContainSubstring("accept"),
		"mc %s: decoded payload should contain accept rules", mcRel)
	Expect(s).To(ContainSubstring("log"),
		"mc %s: decoded payload should contain log rules (rate-limited firewall logging)", mcRel)
	Expect(s).To(ContainSubstring("chain openshift"),
		"mc %s: decoded payload should define openshift chain (e.g. OPENSHIFT)", mcRel)
}

// commatrixVerifyHostFirewallNFTablesInMCRoot walks MachineConfig YAML under root, decodes any
// ignition `source: data:...;base64,...` lines, and asserts at least one payload is the openshift_filter ruleset.
func commatrixVerifyHostFirewallNFTablesInMCRoot(mcRoot string) {
	rels, err := commatrixListMachineConfigYAMLRels(mcRoot)
	Expect(err).NotTo(HaveOccurred(), "list MachineConfigs under %s", mcRoot)

	var verifiedRel string

	for _, rel := range rels {
		raw, errRead := os.ReadFile(filepath.Join(mcRoot, filepath.FromSlash(rel)))
		Expect(errRead).NotTo(HaveOccurred(), "read MachineConfig %s", rel)

		for _, payload := range commatrixCollectDecodedDataURLPayloads(raw) {
			if !strings.Contains(strings.ToLower(string(payload)), "table inet openshift_filter") {
				continue
			}

			commatrixExpectDecodedHostFirewallNFTables(payload, rel)
			verifiedRel = rel

			break
		}

		if verifiedRel != "" {
			break
		}
	}

	Expect(verifiedRel).NotTo(BeEmpty(),
		"no MachineConfig under %s contained a data: URL payload decoding to openshift_filter nftables", mcRoot)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("validated host-firewall nftables in %s under %s", verifiedRel, mcRoot)
}

// commatrixExtractOpenshiftFilterRuleLines returns non-empty nft rule lines from a decoded openshift_filter payload.
func commatrixExtractOpenshiftFilterRuleLines(nftText string) []string {
	var rules []string

	for _, line := range strings.Split(nftText, "\n") {
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

	return rules
}

// commatrixLogOpenshiftFilterRulesByPool logs openshift_filter rules from each mc-<pool>.yaml
// and, when the cluster API is available, the node names that match each pool's MachineConfigPool nodeSelector.
func commatrixLogOpenshiftFilterRulesByPool(mcRoot string) {
	rels, err := commatrixListMachineConfigYAMLRels(mcRoot)
	if err != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Commatrix artifacts: openshift_filter rule dump skipped: list MachineConfigs under %s: %v", mcRoot, err)

		return
	}

	sort.Strings(rels)

	for _, rel := range rels {
		raw, errRead := os.ReadFile(filepath.Join(mcRoot, filepath.FromSlash(rel)))
		if errRead != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Commatrix artifacts: read MachineConfig %s: %v", rel, errRead)

			continue
		}

		pool, hasPool := commatrixMachineConfigPoolNameFromMCRel(rel)
		if !hasPool {
			pool = "(unknown pool — expected mc-<pool>.yaml)"
		}

		var logged bool

		for _, payload := range commatrixCollectDecodedDataURLPayloads(raw) {
			if !strings.Contains(strings.ToLower(string(payload)), "table inet openshift_filter") {
				continue
			}

			logged = true

			targetNodes := "(cluster API unavailable)"
			if APIClient != nil && hasPool {
				if nodeNames, errNodes := commatrixNodesFromMachineConfigPools([]string{pool}); errNodes != nil {
					targetNodes = fmt.Sprintf("(list nodes for pool %q: %v)", pool, errNodes)
				} else if len(nodeNames) == 0 {
					targetNodes = "(no nodes matched pool nodeSelector yet)"
				} else {
					targetNodes = strings.Join(nodeNames, ", ")
				}
			}

			ruleLines := commatrixExtractOpenshiftFilterRuleLines(string(payload))
			if len(ruleLines) == 0 {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Commatrix artifacts: MachineConfig %s pool=%q target node(s): %s\nopenshift_filter payload (no discrete rule lines parsed):\n%s",
					rel, pool, targetNodes, strings.TrimSpace(string(payload)))

				continue
			}

			var b strings.Builder
			_, _ = fmt.Fprintf(&b, "Commatrix artifacts: MachineConfig %s pool=%q target node(s): %s\nopenshift_filter rules (%d):",
				rel, pool, targetNodes, len(ruleLines))

			for i, rule := range ruleLines {
				_, _ = fmt.Fprintf(&b, "\n  [%d] %s", i+1, rule)
			}

			klog.Info(b.String())
		}

		if !logged {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Commatrix artifacts: MachineConfig %s pool=%q: no openshift_filter data: URL payload", rel, pool)
		}
	}
}

func commatrixRenderButaneRawToRenderedMC(buRawRoot, renderRoot string) {
	jobs, err := commatrixDiscoverButaneRenderJobs(buRawRoot)
	Expect(err).NotTo(HaveOccurred())
	Expect(jobs).NotTo(BeEmpty(), "no butane inputs (*butane*.yaml/.yml or *.bu) found under %s", buRawRoot)

	for _, j := range jobs {
		absIn := filepath.Join(buRawRoot, j.srcRel)
		absOut := filepath.Join(renderRoot, j.outRel)

		errMk := os.MkdirAll(filepath.Dir(absOut), 0o750)
		Expect(errMk).NotTo(HaveOccurred(), "mkdir for butane output %s", absOut)

		filesDir := filepath.Join(buRawRoot, filepath.Dir(j.srcRel))

		butaneCmd := exec.Command("butane", "--strict", "--files-dir", filesDir, absIn, "-o", absOut)
		butaneCmd.Env = os.Environ()

		butaneCmdOut, butaneCmdErr := butaneCmd.CombinedOutput()
		Expect(butaneCmdErr).NotTo(HaveOccurred(), "butane %q -> %q: %s", absIn, absOut, strings.TrimSpace(string(butaneCmdOut)))
	}
}

// commatrixVerifyDestDirAfterGenerate asserts destDir contains at least one file (recursive) and prints every file (path, mode, size).
func commatrixVerifyDestDirAfterGenerate(destDir string) {
	var lines []string

	n := 0

	err := filepath.WalkDir(destDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		if d.IsDir() {
			return nil
		}

		n++

		rel, relErr := filepath.Rel(destDir, path)
		if relErr != nil {
			rel = path
		}

		fi, errFI := d.Info()
		if errFI != nil {
			lines = append(lines, fmt.Sprintf("%s\t?\t?", filepath.ToSlash(rel)))

			return nil
		}

		lines = append(lines, fmt.Sprintf("%s\t%s\t%d", filepath.ToSlash(rel), fi.Mode().String(), fi.Size()))

		return nil
	})
	Expect(err).NotTo(HaveOccurred(), "walk commatrix destDir %q", destDir)
	Expect(n).To(BeNumerically(">", 0), "expected at least one file under %s", destDir)

	sort.Strings(lines)

	listing := strings.Join(lines, "\n")

	_, _ = fmt.Fprintf(GinkgoWriter, "\n=== commatrix destDir %q (%d files) ===\n%s\n\n", destDir, n, listing)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("commatrix destDir %q: %d files\n%s", destDir, n, listing)

	By(fmt.Sprintf("Verifying commatrix output directory (%d files)", n))
}

func commatrixPickMCPoolName(candidates []string, match func(string) bool) string {
	for _, p := range candidates {
		if match(p) {
			return p
		}
	}

	return ""
}

func commatrixInferSecureMCPoolNameFromPools(pools []string) string {
	if p := commatrixPickMCPoolName(pools, func(name string) bool {
		lower := strings.ToLower(name)

		return strings.Contains(lower, "appworker") && strings.Contains(lower, "mcp-a")
	}); p != "" {
		return p
	}

	if p := commatrixPickMCPoolName(pools, func(name string) bool {
		return strings.Contains(strings.ToLower(name), "appworker")
	}); p != "" {
		return p
	}

	return commatrixPickMCPoolName(pools, func(name string) bool {
		lower := strings.ToLower(name)

		return !strings.Contains(lower, "master") && !strings.Contains(lower, "storage")
	})
}

func commatrixInferSecureMCPoolName() string {
	pools := commatrixWorkflow.appliedMCPoolNames
	if len(pools) == 0 {
		pools = commatrixWorkflow.generatedMCPoolNames
	}

	return commatrixInferSecureMCPoolNameFromPools(pools)
}

func commatrixInferMasterMCPoolNameFromPools(pools []string) string {
	for _, p := range pools {
		if strings.EqualFold(p, "master") {
			return p
		}
	}

	return commatrixPickMCPoolName(pools, func(name string) bool {
		lower := strings.ToLower(name)

		return strings.Contains(lower, "master") && !strings.Contains(lower, "storage") &&
			!strings.Contains(lower, "appworker") && !strings.Contains(lower, "gateway")
	})
}

func commatrixSetWorkerFromNodeName(nodeName string) error {
	nb, errPull := nodes.Pull(APIClient, nodeName)
	if errPull != nil {
		return fmt.Errorf("pull node %q: %w", nodeName, errPull)
	}

	ips, errIPs := commatrixNodeProbeIPs(nb)
	if errIPs != nil {
		return errIPs
	}

	commatrixWorkflow.run.SecureWorkerName = nodeName
	commatrixWorkflow.run.SecureWorkerIPs = ips

	return nil
}

// commatrixResolveConnectivityTopology fills master and secure-worker names and probe IPs for connectivity and journal specs.
func commatrixResolveConnectivityTopology() error {
	commatrixWorkflow.run.MasterIPs = nil
	commatrixWorkflow.run.SecureWorkerName = ""
	commatrixWorkflow.run.SecureWorkerIPs = nil

	masters, err := nodes.List(APIClient, metav1.ListOptions{LabelSelector: commatrixMasterLabelSelector()})
	if err != nil {
		return fmt.Errorf("list master nodes: %w", err)
	}

	if len(masters) == 0 {
		return fmt.Errorf("no nodes matched master label %q", commatrixMasterLabelSelector())
	}

	masterIPs, err := commatrixNodeProbeIPs(masters[0])
	if err != nil {
		return err
	}

	commatrixWorkflow.run.MasterIPs = masterIPs

	nftNode := strings.TrimSpace(commatrixWorkflow.nftProbeNodeName)
	if nftNode == "" {
		return fmt.Errorf("secure worker: nft probe node not recorded (prime verification workflow first)")
	}

	if err := commatrixSetWorkerFromNodeName(nftNode); err != nil {
		return fmt.Errorf("secure worker %q: %w", nftNode, err)
	}

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

// commatrixNodeProbeIPs returns IPv4 then IPv6 node addresses for nc probes via eco-goinfra nodes helpers.
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
		return nil, fmt.Errorf("no external IPv4/IPv6 address on node %q", nb.Definition.Name)
	}

	return ips, nil
}

// commatrixFormatMCDir returns the direct oc commatrix mc output directory (<output_dir>/format-mc).
func commatrixFormatMCDir() (string, error) {
	dest := strings.TrimSpace(RDSCoreConfig.CommatrixOutputDir)
	if dest == "" {
		return "", fmt.Errorf("CommatrixOutputDir is empty")
	}

	return filepath.Join(filepath.Clean(dest), commatrixFormatMCSubdir), nil
}

func commatrixWaitAllMachineConfigPoolsStable(stepLabel string) {
	By(fmt.Sprintf("%s — wait for all MachineConfigPools to stabilize (ready == updated == machine count; no degraded)", stepLabel))

	err := mco.ListMCPWaitToBeStableFor(APIClient, 90*time.Second, commatrixMCPStableWait)
	Expect(err).NotTo(HaveOccurred(), "%s: all MCPs did not stabilize within timeout", stepLabel)

	mcpSnapshot, mcpSnapshotErr := commatrixFormatMCPStatusSnapshot()
	Expect(mcpSnapshotErr).NotTo(HaveOccurred(), "%s: list MachineConfigPools", stepLabel)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("%s: MCP snapshot after stable wait:\n%s", stepLabel, mcpSnapshot)
}

// commatrixMachineConfigPoolNameFromMCRel extracts the MachineConfigPool name from a commatrix MachineConfig path.
// This workflow requires basenames mc-<pool>.yaml or mc-<pool>.yml (e.g. mc-appworker-mcp-a.yaml → appworker-mcp-a).
func commatrixMachineConfigPoolNameFromMCRel(rel string) (string, bool) {
	base := filepath.Base(rel)
	ext := strings.ToLower(filepath.Ext(base))
	if ext != ".yaml" && ext != ".yml" {
		return "", false
	}

	stem := strings.TrimSuffix(base, filepath.Ext(base))
	if len(stem) < 4 || !strings.HasPrefix(strings.ToLower(stem), "mc-") {
		return "", false
	}

	pool := strings.TrimSpace(stem[3:])
	if pool == "" {
		return "", false
	}

	return pool, true
}

// commatrixMachineConfigPoolNamesFromMCRels returns sorted unique pool names from mc-<pool>.yaml paths (each rel must already match that layout).
func commatrixMachineConfigPoolNamesFromMCRels(mcRels []string) []string {
	seen := make(map[string]struct{})

	var pools []string

	for _, rel := range mcRels {
		if p, ok := commatrixMachineConfigPoolNameFromMCRel(rel); ok {
			if _, dup := seen[p]; !dup {
				seen[p] = struct{}{}
				pools = append(pools, p)
			}
		}
	}

	sort.Strings(pools)

	return pools
}

// commatrixMCRelsForPools returns mc-<pool>.yaml paths whose pool name is in poolNames.
func commatrixMCRelsForPools(mcRels []string, poolNames []string) []string {
	want := make(map[string]struct{}, len(poolNames))
	for _, p := range poolNames {
		want[p] = struct{}{}
	}

	var out []string

	for _, rel := range mcRels {
		pool, ok := commatrixMachineConfigPoolNameFromMCRel(rel)
		if !ok {
			continue
		}

		if _, match := want[pool]; match {
			out = append(out, rel)
		}
	}

	sort.Strings(out)

	return out
}

// commatrixNodesFromMachineConfigPools returns sorted unique node names matching each pool's
// MachineConfigPool.spec.nodeSelector on the live cluster.
func commatrixNodesFromMachineConfigPools(poolNames []string) ([]string, error) {
	seen := make(map[string]struct{})

	var out []string

	for _, poolName := range poolNames {
		mcpB, errPull := mco.Pull(APIClient, poolName)
		if errPull != nil {
			return nil, fmt.Errorf("MachineConfigPool %q: %w", poolName, errPull)
		}

		mcpObj, errGet := mcpB.Get()
		if errGet != nil {
			return nil, fmt.Errorf("get MachineConfigPool %q: %w", poolName, errGet)
		}

		sel := mcpObj.Spec.NodeSelector
		if sel == nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
				"MachineConfigPool %q has nil nodeSelector; skipping node enumeration for this pool", poolName)

			continue
		}

		nodeLabelSel, errLS := metav1.LabelSelectorAsSelector(sel)
		if errLS != nil {
			return nil, fmt.Errorf("MachineConfigPool %q nodeSelector: %w", poolName, errLS)
		}

		labelStr := nodeLabelSel.String()
		if labelStr == "" {
			continue
		}

		nodeList, errList := nodes.List(APIClient, metav1.ListOptions{LabelSelector: labelStr})
		if errList != nil {
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

	return out, nil
}

// commatrixStopNFTOnNode stops nftables on one node (best-effort during suite cleanup).
func commatrixStopNFTOnNode(nodeName string) {
	stopNftablesCmdOut, stopNftablesCmdErr := commatrixRunOnNodeHostDebug(nodeName,
		[]string{"chroot", "/rootfs", "sh", "-c", "systemctl stop nftables || true"})
	if stopNftablesCmdErr != nil {
		klog.Errorf("stop nftables on %s: %v\n%s", nodeName, stopNftablesCmdErr, stopNftablesCmdOut)

		return
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("stop nftables on %s: %s", nodeName, strings.TrimSpace(stopNftablesCmdOut))
}

// commatrixRunOnNodeHostShell runs shellCmd on the node host via the MCO daemon pod (chroot /rootfs).
func commatrixRunOnNodeHostShell(nodeName, shellCmd string) (string, error) {
	return commatrixRunOnNodeHostDebug(nodeName, []string{"chroot", "/rootfs", "sh", "-c", shellCmd})
}

// commatrixGenerateArtifacts runs oc commatrix generate for mc and butane, runs the butane CLI in the same
// directory as the generated butane YAML (mc-*.yaml next to butane-*.yaml), then asserts parity with direct mc output.
func commatrixGenerateArtifacts(_ SpecContext) {
	By("Generating commatrix MachineConfig artifacts")

	Expect(strings.TrimSpace(RDSCoreConfig.CommatrixOutputDir)).NotTo(BeEmpty(),
		"rdscore_commatrix_output_dir is required for oc commatrix generate")

	destDir := filepath.Clean(strings.TrimSpace(RDSCoreConfig.CommatrixOutputDir))
	mcDir := filepath.Join(destDir, commatrixFormatMCSubdir)
	buDir := filepath.Join(destDir, commatrixFormatButaneSubdir)

	for _, d := range []string{mcDir, buDir} {
		_ = os.RemoveAll(d)
	}

	for _, d := range []string{destDir, mcDir, buDir} {
		err := os.MkdirAll(d, 0o750)
		Expect(err).NotTo(HaveOccurred(), "mkdir %s", d)
	}

	ocCommatrixMcGenCmdOut, ocCommatrixMcGenCmdErr := commatrixRunOC("commatrix", "generate", "--format", "mc", "--host-open-ports", "--destDir", mcDir)
	Expect(ocCommatrixMcGenCmdErr).NotTo(HaveOccurred(), "oc commatrix generate (mc): %s", ocCommatrixMcGenCmdOut)
	commatrixVerifyDestDirAfterGenerate(mcDir)

	ocCommatrixButaneGenCmdOut, ocCommatrixButaneGenCmdErr := commatrixRunOC("commatrix", "generate", "--format", "butane", "--host-open-ports", "--destDir", buDir)
	Expect(ocCommatrixButaneGenCmdErr).NotTo(HaveOccurred(), "oc commatrix generate (butane): %s", ocCommatrixButaneGenCmdOut)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("oc commatrix generate (butane): %s", strings.TrimSpace(ocCommatrixButaneGenCmdOut))

	commatrixVerifyDestDirAfterGenerate(buDir)

	By("Rendering butane to MachineConfig")
	commatrixRenderButaneRawToRenderedMC(buDir, buDir)

	commatrixVerifyDestDirAfterGenerate(buDir)

	By("Comparing direct mc vs butane-rendered MachineConfigs")
	commatrixCompareMCDirectVsRendered(mcDir, buDir)

	By("Validating host-firewall nftables in generated MachineConfigs")
	commatrixVerifyHostFirewallNFTablesInMCRoot(mcDir)
	commatrixVerifyHostFirewallNFTablesInMCRoot(buDir)

	commatrixLogOpenshiftFilterRulesByPool(mcDir)
}

// commatrixApplyHostFirewallMC patches NDP, applies host-firewall MachineConfigs, waits for MCP stable, verifies nft chain.
//
//nolint:funlen
func commatrixApplyHostFirewallMC(_ SpecContext) {
	mcDir, err := commatrixFormatMCDir()
	if err != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Apply skipped: %v", err)

		return
	}

	if _, statErr := os.Stat(mcDir); statErr != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Apply skipped: format-mc missing (%s): %v", mcDir, statErr)

		return
	}

	mcRels, err := commatrixListMachineConfigYAMLRels(mcDir)
	Expect(err).NotTo(HaveOccurred(), "list MachineConfigs under %s", mcDir)

	if len(mcRels) == 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Apply skipped: no MachineConfig YAML under %s", mcDir)

		return
	}

	for _, rel := range mcRels {
		_, ok := commatrixMachineConfigPoolNameFromMCRel(rel)
		Expect(ok).To(BeTrue(),
			"format-mc MachineConfigs must be named mc-<pool>.yaml (pool = MachineConfigPool name); invalid path %q", rel)
	}

	ndp, errNdp := commatrixFindNodeDisruptionPolicyPatch(mcDir)
	Expect(errNdp).NotTo(HaveOccurred(), "find node-disruption-policy.yaml under format-mc")
	Expect(ndp).NotTo(BeEmpty(),
		"node-disruption-policy.yaml required under format-mc (oc commatrix generate output)")

	By("Patching MachineConfiguration cluster node disruption policy")

	Expect(commatrixPatchMachineConfigurationClusterMerge(ndp)).NotTo(HaveOccurred(), "cluster NDP patch failed")

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("patched NDP from %s", ndp)
	commatrixWorkflow.ndpApplied = true

	commatrixWorkflow.generatedMCPoolNames = commatrixMachineConfigPoolNamesFromMCRels(mcRels)
	Expect(commatrixWorkflow.generatedMCPoolNames).NotTo(BeEmpty(), "internal: expected pool name(s) from mc-<pool>.yaml paths")

	securePool := commatrixInferSecureMCPoolNameFromPools(commatrixWorkflow.generatedMCPoolNames)
	Expect(securePool).NotTo(BeEmpty(),
		"could not infer secure/firewall MCP from generated MCs; set rdscore_commatrix_secure_mcp_name (e.g. appworker-mcp-a)")

	masterPool := commatrixInferMasterMCPoolNameFromPools(commatrixWorkflow.generatedMCPoolNames)
	Expect(masterPool).NotTo(BeEmpty(),
		"could not infer master MCP from generated MCs (expected mc-master.yaml)")

	applyPools := []string{securePool, masterPool}
	applyRels := commatrixMCRelsForPools(mcRels, applyPools)
	Expect(applyRels).NotTo(BeEmpty(),
		"no mc-%s.yaml / mc-%s.yaml under %s", securePool, masterPool, mcDir)
	Expect(commatrixMCRelsForPools(mcRels, []string{masterPool})).NotTo(BeEmpty(),
		"no mc-%s.yaml under %s; commatrix generate must emit mc-master.yaml", masterPool, mcDir)

	applyRelSet := make(map[string]struct{}, len(applyRels))
	for _, rel := range applyRels {
		applyRelSet[rel] = struct{}{}
	}

	var skippedRels []string

	for _, rel := range mcRels {
		if _, applied := applyRelSet[rel]; !applied {
			skippedRels = append(skippedRels, rel)
		}
	}

	By("Applying secure-pool and master MachineConfigs")

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("apply: pools=%v files=%v skipped=%v", applyPools, applyRels, skippedRels)

	for _, rel := range applyRels {
		abs := filepath.Join(mcDir, filepath.FromSlash(rel))

		Expect(commatrixApplyMachineConfigYAML(abs)).NotTo(HaveOccurred(), "apply MachineConfig %q", rel)

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("apply: applied %q", rel)
	}

	commatrixWaitAllMachineConfigPoolsStable("apply host firewall MCs")

	var errRec error

	commatrixWorkflow.revertNodeNames, errRec = commatrixNodesFromMachineConfigPools(applyPools)
	Expect(errRec).NotTo(HaveOccurred(), "resolve nodes for applied pool(s) %v", applyPools)
	Expect(commatrixWorkflow.revertNodeNames).NotTo(BeEmpty(),
		"no nodes matched MachineConfigPool nodeSelector(s) for applied pools %v (check pool membership on the cluster)", applyPools)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("apply: revert nodes (pools %v): %v",
		applyPools, commatrixWorkflow.revertNodeNames)

	commatrixWorkflow.appliedMCPoolNames = append([]string(nil), applyPools...)
	commatrixWorkflow.appliedMCRels = append([]string(nil), applyRels...)

	commatrixWorkflow.mcApplied = true

	securePoolNodes, errSecureNodes := commatrixNodesFromMachineConfigPools([]string{securePool})
	Expect(errSecureNodes).NotTo(HaveOccurred(), "list nodes in secure pool %q", securePool)
	Expect(securePoolNodes).NotTo(BeEmpty(), "no nodes in secure pool %q for nft probe", securePool)

	nftNode := securePoolNodes[0]
	commatrixWorkflow.nftProbeNodeName = nftNode

	By(fmt.Sprintf("Verifying openshift_filter chain on node %s", nftNode))

	nftOpenshiftChainShellCmd := "nft list chain inet openshift_filter OPENSHIFT 2>/dev/null || " +
		"nft list chain inet openshift_filter OPENCHAIN 2>/dev/null || true"

	var nftOpenshiftChainCmdOut string

	err = wait.PollUntilContextTimeout(context.TODO(), 10*time.Second, 3*time.Minute, true,
		func(context.Context) (bool, error) {
			var hostDebugCmdErr error

			nftOpenshiftChainCmdOut, hostDebugCmdErr = commatrixRunOnNodeHostDebug(nftNode,
				[]string{"chroot", "/rootfs", "sh", "-c", nftOpenshiftChainShellCmd})
			if hostDebugCmdErr != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("nft list (retry) on %s: %v", nftNode, hostDebugCmdErr)

				return false, nil
			}

			return strings.TrimSpace(nftOpenshiftChainCmdOut) != "", nil
		})
	Expect(err).NotTo(HaveOccurred(), "nft openshift_filter/OPENSHIFT chain not visible on %s", nftNode)

	Expect(strings.ToLower(nftOpenshiftChainCmdOut)).To(ContainSubstring("openshift_filter"))
}

// commatrixVerifyHostFirewallConnectivity probes API/kubelet TCP reachability from the test runner.
func commatrixVerifyHostFirewallConnectivity(_ SpecContext) {
	By("Resolving cluster topology for connectivity probes")

	Expect(commatrixResolveConnectivityTopology()).NotTo(HaveOccurred())

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Commatrix connectivity probes: masterIPs=%v secure=%s/%v",
		commatrixWorkflow.run.MasterIPs, commatrixWorkflow.run.SecureWorkerName, commatrixWorkflow.run.SecureWorkerIPs)

	apiPort := strconv.Itoa(commatrixAPIPort)
	kubeletPort := strconv.Itoa(commatrixKubeletPort)
	closedPort := strconv.Itoa(commatrixClosedTCPPort)

	tryProbe := func(desc string, addrs []string, port string, expectConnect bool) {
		By(desc)

		Expect(commatrixTryTCPProbesFromRunner(desc, addrs, port, expectConnect)).NotTo(HaveOccurred(), "%s", desc)
	}

	tryProbe("Master API reachable from test runner", commatrixWorkflow.run.MasterIPs, apiPort, true)

	tryProbe("Secure-pool worker API blocked from test runner", commatrixWorkflow.run.SecureWorkerIPs, apiPort, false)

	var securePoolPeerNames []string

	if len(commatrixWorkflow.appliedMCPoolNames) > 0 {
		var errPeers error

		securePoolPeerNames, errPeers = commatrixNodesFromMachineConfigPools(commatrixWorkflow.appliedMCPoolNames)
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

		tryProbe("Peer secure-pool worker API blocked from test runner", peerIPs, apiPort, false)
	} else {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Skipping peer secure-pool worker API probe: fewer than two secure-pool nodes")
	}

	tryProbe("Secure-pool worker kubelet reachable from test runner", commatrixWorkflow.run.SecureWorkerIPs, kubeletPort, true)

	tryProbe("Closed test port blocked on secure-pool worker", commatrixWorkflow.run.SecureWorkerIPs, closedPort, false)
}

func commatrixRunJournalKernelGrep(nodeName, since, until, grepFilter string) (string, error) {
	shellCmd := fmt.Sprintf(`journalctl -k --since %q`, since)
	if strings.TrimSpace(until) != "" {
		shellCmd += fmt.Sprintf(` --until %q`, until)
	}

	shellCmd += fmt.Sprintf(` 2>/dev/null | grep -F %q || true`, grepFilter)

	return commatrixRunOnNodeHostShell(nodeName, shellCmd)
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
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("journal on %s: %v", nodeName, pollErr)

				return false, nil
			}

			lines = commatrixParseJournalKernelLines(raw)
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("journal on %s since %q filter %q: %d line(s)",
				nodeName, since, grepFilter, len(lines))

			return len(lines) >= minLines, nil
		})

	return lines, raw, err
}

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
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("firewall journal %s: bucket %q: %d line(s)", windowLabel, prefix, n)

		Expect(n).To(BeNumerically("<=", commatrixFirewallRateLimitPerMinute),
			"firewall journal %s: bucket %q: %d lines (max %d per minute)", windowLabel, prefix, n, commatrixFirewallRateLimitPerMinute)
	}
}

func commatrixFirewallLogBucket(line string) (string, bool) {
	if strings.Contains(line, "TCP_TEST") {
		return "", false
	}

	const tag = "kernel: "

	i := strings.Index(line, tag)
	if i < 0 {
		return "", false
	}

	switch msg := line[i+len(tag):]; {
	case strings.HasPrefix(msg, "firewall IN="):
		return commatrixFirewallLogPrefixWithSpace, true
	case strings.HasPrefix(msg, "firewallIN="):
		return commatrixFirewallLogPrefixNoSpace, true
	default:
		return "", false
	}
}

// commatrixVerifyFirewallJournal checks firewall journal rate limits and TCP_TEST log-drop probe.
// Uses the same secure worker node as connectivity (nft apply probe, nc probes); run connectivity before this spec.
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

	By(fmt.Sprintf("Verifying firewall log rate limits on node %s (two 1-minute windows)", journalNode))

	window1Raw, errWin1 := commatrixRunJournalKernelGrep(
		journalNode, commatrixJournalSinceTwoMinutes, commatrixJournalSinceOneMinute, keyword)
	Expect(errWin1).NotTo(HaveOccurred(), "firewall journal window 1 on %s: %s", journalNode, window1Raw)

	window1Lines := commatrixParseJournalKernelLines(window1Raw)
	commatrixExpectFirewallLogRateLimitsInWindow(window1Lines,
		fmt.Sprintf("%s to %s", commatrixJournalSinceTwoMinutes, commatrixJournalSinceOneMinute))

	window2Raw, errWin2 := commatrixRunJournalKernelGrep(journalNode, commatrixJournalSinceOneMinute, "", keyword)
	Expect(errWin2).NotTo(HaveOccurred(), "firewall journal window 2 on %s: %s", journalNode, window2Raw)

	window2Lines := commatrixParseJournalKernelLines(window2Raw)
	if len(window2Lines) == 0 {
		warnMsg := fmt.Sprintf(
			"firewall journal: no kernel log lines matching %q in the last minute on %s; "+
				"skipping window-2 rate-limit checks (traffic may be quiet). Last output:\n%s",
			keyword, journalNode, window2Raw)
		klog.Warning(warnMsg)
		_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: %s\n", warnMsg)
	} else {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("firewall journal: %d line(s) matching %q on %s in the last minute",
			len(window2Lines), keyword, journalNode)

		commatrixExpectFirewallLogRateLimitsInWindow(window2Lines, commatrixJournalSinceOneMinute)
	}

	apiPort := commatrixAPIPort

	By(fmt.Sprintf("Injecting TCP_TEST log rule on node %s", journalNode))

	nftInsertHostShellCmd := fmt.Sprintf(
		`set -e; nft insert rule inet openshift_filter %s tcp dport %d log prefix \"TCP_TEST \" drop`,
		commatrixNFTablesOpenshiftChain, apiPort)

	nftInsertCmdOut, nftInsertCmdErr := commatrixRunOnNodeHostShell(journalNode, nftInsertHostShellCmd)
	Expect(nftInsertCmdErr).NotTo(HaveOccurred(), "nft insert TCP_TEST rule on %s: %s", journalNode, nftInsertCmdOut)

	defer func() {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("removing TCP_TEST rule on %s (best-effort)", journalNode)

		nftDeleteLogRuleShellCmd := fmt.Sprintf(
			`HANDLE=$(nft -a list chain inet openshift_filter %s 2>/dev/null | grep TCP_TEST | tail -1 | sed -E "s/.*handle ([0-9]+).*/\\1/"); [ -n "$HANDLE" ] && nft delete rule inet openshift_filter %s handle "$HANDLE" || true`,
			commatrixNFTablesOpenshiftChain, commatrixNFTablesOpenshiftChain)

		_, _ = commatrixRunOnNodeHostShell(journalNode, nftDeleteLogRuleShellCmd)
	}()

	portStr := strconv.Itoa(apiPort)

	By(fmt.Sprintf("Probing %v:%d from test runner (TCP_TEST drop expected)", journalProbeIPs, apiPort))

	for _, probeTargetIP := range journalProbeIPs {
		if err := commatrixRunTCPProbeFromRunner(false, probeTargetIP, portStr); err != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("local nc to %s:%d (drop expected): %v", probeTargetIP, apiPort, err)
		}
	}

	By("Verifying TCP_TEST in kernel journal")

	tcpTestLines, tcpTestRaw, err := commatrixWaitForJournalKernelGrep(
		journalNode, commatrixJournalSinceOneMinute, "TCP_TEST", 1, 3*time.Second, 90*time.Second)
	Expect(err).NotTo(HaveOccurred(),
		"TCP_TEST journal: expected at least one TCP_TEST kernel log line after nc to %v:%d on %s (got %d); last output:\n%s",
		journalProbeIPs, apiPort, journalNode, len(tcpTestLines), tcpTestRaw)

	Expect(tcpTestLines).NotTo(BeEmpty(), "TCP_TEST journal: expected ≥1 TCP_TEST log line on %s", journalNode)

	dptNeedle := fmt.Sprintf("DPT=%d", apiPort)

	journalJoined := strings.ToUpper(strings.Join(tcpTestLines, "\n"))

	Expect(journalJoined).To(ContainSubstring("TCP_TEST"),
		"TCP_TEST journal: journal lines should include TCP_TEST log prefix from injected rule")
	Expect(journalJoined).To(ContainSubstring(strings.ToUpper(dptNeedle)),
		"TCP_TEST journal: journal lines should reference probed destination port %s", dptNeedle)
}

func commatrixRevertNDP() {
	clusterJSON, errGet := commatrixGetMachineConfigurationClusterJSON()
	if errGet != nil {
		klog.Errorf("revert NDP: %v", errGet)

		return
	}

	patch, errBuild := commatrixBuildJSONPatchRemoveNDPNFT(string(clusterJSON))
	if errBuild != nil {
		klog.Errorf("revert NDP: build JSON patch: %v", errBuild)

		return
	}

	if len(patch) == 0 || string(patch) == "[]" {
		commatrixWorkflow.ndpApplied = false

		return
	}

	if errPatch := commatrixPatchMachineConfigurationClusterJSON(patch); errPatch != nil {
		klog.Errorf("revert NDP: json patch failed: %v", errPatch)

		return
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("revert NDP: patched machineconfiguration/cluster")
	commatrixWorkflow.ndpApplied = false
}

func commatrixRevertHostFirewall() {
	if !commatrixWorkflow.mcApplied {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("revert skipped: no MachineConfig was applied")

		return
	}

	mcDir, errMcDir := commatrixFormatMCDir()
	if errMcDir != nil {
		klog.Errorf("revert: resolve format-mc dir: %v", errMcDir)

		return
	}

	for _, nodeName := range commatrixWorkflow.revertNodeNames {
		commatrixStopNFTOnNode(nodeName)
	}

	for _, rel := range commatrixWorkflow.appliedMCRels {
		abs := filepath.Join(mcDir, filepath.FromSlash(rel))

		if errDelete := commatrixDeleteMachineConfigYAML(abs); errDelete != nil {
			klog.Errorf("revert: delete MachineConfig %q: %v", rel, errDelete)

			continue
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("revert: deleted %q", rel)
	}

	if errWait := mco.ListMCPWaitToBeStableFor(APIClient, 90*time.Second, commatrixMCPStableWait); errWait != nil {
		klog.Errorf("revert: MCP stable wait: %v", errWait)
	}

	commatrixWorkflow.mcApplied = false

	if commatrixWorkflow.ndpApplied {
		commatrixRevertNDP()
	}

	destDir := filepath.Clean(strings.TrimSpace(RDSCoreConfig.CommatrixOutputDir))
	if destDir != "" && destDir != "." && destDir != string(filepath.Separator) {
		if errRm := os.RemoveAll(destDir); errRm != nil {
			klog.Errorf("revert: remove output dir %s: %v", destDir, errRm)
		} else {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("revert: removed output directory %s", destDir)
		}
	}
}

// CommatrixRevertAfterSpec undoes apply changes after the ordered commatrix Describe finishes.
// Errors are logged only so a failed connectivity/journal spec still attempts cleanup.
func CommatrixRevertAfterSpec() {
	By("Reverting commatrix cluster changes")
	commatrixRevertHostFirewall()
}

// VerifyCommatrixHostFirewallArtifacts (reportxml 95001) generates commatrix artifacts and validates embedded openshift_filter nftables.
func VerifyCommatrixHostFirewallArtifacts(ctx SpecContext) {
	commatrixResetWorkflow()
	commatrixGenerateArtifacts(ctx)
}

// VerifyCommatrixHostFirewallApply (reportxml 95002) applies host-firewall MachineConfigs and verifies nftables on a node.
func VerifyCommatrixHostFirewallApply(ctx SpecContext) {
	commatrixApplyHostFirewallMC(ctx)
}

// VerifyCommatrixHostFirewallConnectivity verifies TCP connectivity from the test runner.
func VerifyCommatrixHostFirewallConnectivity(ctx SpecContext) {
	Expect(commatrixPrimeWorkflowForVerification()).NotTo(HaveOccurred(),
		"host-firewall rules must be applied on the cluster before connectivity checks")

	commatrixVerifyHostFirewallConnectivity(ctx)
}

// VerifyCommatrixHostFirewallJournal verifies firewall journal rate limits and TCP_TEST logging.
func VerifyCommatrixHostFirewallJournal(ctx SpecContext) {
	Expect(commatrixPrimeWorkflowForVerification()).NotTo(HaveOccurred(),
		"host-firewall rules must be applied on the cluster before journal checks")

	if strings.TrimSpace(commatrixWorkflow.run.SecureWorkerName) == "" {
		Expect(commatrixResolveConnectivityTopology()).NotTo(HaveOccurred(),
			"resolve connectivity topology before journal checks")
	}

	commatrixVerifyFirewallJournal(ctx)
}
