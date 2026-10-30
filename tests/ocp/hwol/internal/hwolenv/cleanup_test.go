//go:build unit_test

package hwolenv

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	sriovv1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	hwolconfig "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/ocphwolconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/hwol/internal/tsparams"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	runtimeClient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestValidateHwolOperatorConfigDoesNotEnableDisabledPlugins(t *testing.T) {
	spec := sriovv1.SriovOperatorConfigSpec{
		EnableOvsOffload: true,
		FeatureGates:     map[string]bool{tsparams.ManageSoftwareBridgesGate: true},
	}
	require.NoError(t, validateHwolOperatorConfig(spec, "15b3"))
	spec.DisablePlugins = append(spec.DisablePlugins, "mellanox")
	require.ErrorContains(t, validateHwolOperatorConfig(spec, "15b3"), "Mellanox plugin")
	require.NoError(t, validateHwolOperatorConfig(spec, "8086"))
	spec.DisablePlugins = nil
	spec.EnableOvsOffload = false
	require.ErrorContains(t, validateHwolOperatorConfig(spec, "15b3"), "must enable")
}

func TestSwitchdevPreflightRejectsForeignState(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1", Labels: map[string]string{
		"node-role.kubernetes.io/canary": "",
	}}}
	device := hwolconfig.DeviceConfig{InterfaceName: "pf0", Vendor: "15b3", DeviceID: "a2d6"}
	state := &sriovv1.SriovNetworkNodeState{ObjectMeta: metav1.ObjectMeta{Name: node.Name},
		Status: sriovv1.SriovNetworkNodeStateStatus{SyncStatus: "Succeeded", Interfaces: sriovv1.InterfaceExts{{
			Name: "pf0", Vendor: "15b3", DeviceID: "a2d6", PciAddress: "0000:4b:00.0", TotalVfs: 3,
		}}}}
	require.NoError(t, validateNodeFoundationState(node, state, device, 3, nil, nil))
	state.Status.Interfaces[0].NumVfs = 1
	require.ErrorContains(t, validateNodeFoundationState(node, state, device, 3, nil, nil), "unused legacy PF")
	state.Status.Interfaces[0].NumVfs = 0
	state.Status.Interfaces[0].EswitchMode = "switchdev"
	require.ErrorContains(t, validateNodeFoundationState(node, state, device, 3, nil, nil), "unused legacy PF")
	state.Status.Interfaces[0].EswitchMode = "legacy"
	require.ErrorContains(t, validateNodeFoundationState(node, state, device, 4, nil, nil), "capacity")
	state.Spec.Interfaces = sriovv1.Interfaces{{PciAddress: "0000:4b:00.0"}}
	require.ErrorContains(t, validateNodeFoundationState(node, state, device, 3, nil, nil), "pre-existing")
	state.Spec.Interfaces = nil
	policy := sriovv1.SriovNetworkNodePolicy{ObjectMeta: metav1.ObjectMeta{Name: "foreign"}}
	policy.Spec.NodeSelector = map[string]string{"node-role.kubernetes.io/canary": ""}
	require.ErrorContains(t, validateNodeFoundationState(node, state, device, 3, []sriovv1.SriovNetworkNodePolicy{policy}, nil), "policy foreign")
	pool := sriovv1.SriovNetworkPoolConfig{ObjectMeta: metav1.ObjectMeta{Name: "foreign-pool"}}
	pool.Spec.OvsHardwareOffloadConfig.Name = "canary"
	require.ErrorContains(t, validateNodeFoundationState(node, state, device, 3, nil, []sriovv1.SriovNetworkPoolConfig{pool}), "pool config foreign-pool")
}

func TestOvsOffloadBaseline(t *testing.T) {
	for _, tc := range []struct {
		name    string
		current map[string]OvsOffloadBaseline
		wantErr string
	}{
		{"unchanged", map[string]OvsOffloadBaseline{"node1": {NodeUID: "uid", Value: "false"}}, ""},
		{"stale enabled", map[string]OvsOffloadBaseline{"node1": {NodeUID: "uid", Value: "true"}}, "controlled OVS restart"},
		{"absent differs from false", map[string]OvsOffloadBaseline{"node1": {NodeUID: "uid", Value: ""}}, "changed"},
		{"different node", map[string]OvsOffloadBaseline{"node2": {NodeUID: "uid", Value: "false"}}, "no longer selected"},
		{"replacement node", map[string]OvsOffloadBaseline{"node1": {NodeUID: "replacement", Value: "false"}}, "replaced"},
		{"different count", map[string]OvsOffloadBaseline{}, "membership changed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := compareOvsOffloadBaseline(tc.current, map[string]OvsOffloadBaseline{"node1": {NodeUID: "uid", Value: "false"}})
			if tc.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.wantErr)
			}
		})
	}
}

func TestOvsRecoveryRefusesClaimedOrReplacedNode(t *testing.T) {
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1", UID: "uid", Labels: MCPNodeSelector("canary")},
		Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}}
	require.NoError(t, validateOvsRecoveryNode(node, "canary", "uid", nil, nil))
	require.ErrorContains(t, validateOvsRecoveryNode(node, "canary", "replacement", nil, nil), "identity")
	policy := sriovv1.SriovNetworkNodePolicy{ObjectMeta: metav1.ObjectMeta{Name: "foreign"}}
	policy.Spec.NodeSelector = MCPNodeSelector("canary")
	policy.Spec.EswitchMode = "legacy"
	require.ErrorContains(t, validateOvsRecoveryNode(node, "canary", "uid", []sriovv1.SriovNetworkNodePolicy{policy}, nil), "policy foreign")
	pool := sriovv1.SriovNetworkPoolConfig{ObjectMeta: metav1.ObjectMeta{Name: "foreign-pool"}}
	pool.Spec.NodeSelector = &metav1.LabelSelector{MatchLabels: MCPNodeSelector("canary")}
	require.ErrorContains(t, validateOvsRecoveryNode(node, "canary", "uid", nil, []sriovv1.SriovNetworkPoolConfig{pool}), "pool config foreign-pool")
	pool.Spec.NodeSelector = nil
	require.ErrorContains(t, validateOvsRecoveryNode(node, "canary", "uid", nil, []sriovv1.SriovNetworkPoolConfig{pool}), "no identifiable target")
}

func TestDeleteOwnedTestResource(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, sriovv1.AddToScheme(scheme))
	obj := &sriovv1.SriovNetworkNodePolicy{ObjectMeta: metav1.ObjectMeta{
		Name: "policy", Namespace: "operator", UID: types.UID("original-uid"),
		Labels: map[string]string{tsparams.TestRunLabelKey: "other-run"},
	}}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(obj).Build()

	err := deleteOwnedTestResource(client, "operator", "policy", "this-run", &sriovv1.SriovNetworkNodePolicy{})
	require.ErrorContains(t, err, "not owned")
	require.NoError(t, client.Get(context.Background(), runtimeClient.ObjectKeyFromObject(obj), &sriovv1.SriovNetworkNodePolicy{}))

	err = deleteOwnedTestResource(client, "operator", "policy", "other-run", &sriovv1.SriovNetworkNodePolicy{})
	require.ErrorContains(t, err, "original created UID is missing")
	key := resourceKey(obj, "other-run")
	createdResourceUIDs.Store(key, types.UID("different-uid"))
	t.Cleanup(func() { createdResourceUIDs.Delete(key) })
	err = deleteOwnedTestResource(client, "operator", "policy", "other-run", &sriovv1.SriovNetworkNodePolicy{})
	require.ErrorContains(t, err, "original created UID is missing or changed")
	createdResourceUIDs.Store(key, obj.UID)
	err = deleteOwnedTestResource(client, "operator", "policy", "other-run", &sriovv1.SriovNetworkNodePolicy{})
	require.NoError(t, err)
	require.Error(t, client.Get(context.Background(), runtimeClient.ObjectKeyFromObject(obj), &sriovv1.SriovNetworkNodePolicy{}))
}

func TestCreateOwnedTestResourceNeverAdoptsExisting(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, sriovv1.AddToScheme(scheme))
	foreign := &sriovv1.SriovNetworkNodePolicy{ObjectMeta: metav1.ObjectMeta{
		Name: "policy", Namespace: "operator", UID: types.UID("foreign"),
		Labels: map[string]string{tsparams.TestRunLabelKey: "other-run"},
	}}
	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(foreign).Build()
	wanted := &sriovv1.SriovNetworkNodePolicy{ObjectMeta: metav1.ObjectMeta{
		Name: "policy", Namespace: "operator", Labels: map[string]string{tsparams.TestRunLabelKey: "this-run"},
	}}
	err := createOwnedTestResource(client, wanted, "this-run")
	require.True(t, k8serrors.IsAlreadyExists(err), "got %v", err)
	current := &sriovv1.SriovNetworkNodePolicy{}
	require.NoError(t, client.Get(context.Background(), runtimeClient.ObjectKeyFromObject(foreign), current))
	require.Equal(t, foreign.UID, current.UID)
	require.Equal(t, "other-run", current.Labels[tsparams.TestRunLabelKey])
	require.Error(t, createOwnedTestResource(client, wanted, "different-run"))
}

func TestOwnedResourceRejectsReplacementWithCopiedLabel(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, sriovv1.AddToScheme(scheme))
	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	created := &sriovv1.SriovNetworkNodePolicy{ObjectMeta: metav1.ObjectMeta{
		Name: "policy", Namespace: "operator", UID: types.UID("original"),
		Labels: map[string]string{tsparams.TestRunLabelKey: "run-replacement-test"},
	}}
	key := resourceKey(created, "run-replacement-test")
	t.Cleanup(func() { createdResourceUIDs.Delete(key) })
	require.NoError(t, createOwnedTestResource(client, created, "run-replacement-test"))
	require.NoError(t, client.Delete(context.Background(), created))
	replacement := created.DeepCopy()
	replacement.UID = types.UID("replacement")
	replacement.ResourceVersion = ""
	require.NoError(t, client.Create(context.Background(), replacement))
	err := deleteOwnedTestResource(client, "operator", "policy", "run-replacement-test", &sriovv1.SriovNetworkNodePolicy{})
	require.ErrorContains(t, err, "original created UID is missing or changed")
	current := &sriovv1.SriovNetworkNodePolicy{}
	require.NoError(t, client.Get(context.Background(), runtimeClient.ObjectKeyFromObject(replacement), current))
	require.Equal(t, types.UID("replacement"), current.UID)
}

func TestParseOvsOffloadValue(t *testing.T) {
	for _, tc := range []struct{ input, expected string }{
		{"", ""}, {"\"true\"\n", "true"}, {"\"false\"\n", "false"},
	} {
		value, err := parseOvsOffloadValue(tc.input)
		require.NoError(t, err)
		require.Equal(t, tc.expected, value)
	}

	_, err := parseOvsOffloadValue("unexpected")
	require.Error(t, err)
}

func TestOvsRestartRejectsConflictingStartupOrChangedState(t *testing.T) {
	require.NoError(t, validateOvsRestartConfig("[Service]\nExecStart=/usr/bin/ovs-vswitchd\n", "\"true\"\n"))
	require.ErrorContains(t, validateOvsRestartConfig("", "true"), "startup configuration")
	require.ErrorContains(t, validateOvsRestartConfig("ExecStartPre=ovs-vsctl set other_config:hw-offload=true", "true"), "startup configuration")
	require.ErrorContains(t, validateOvsRestartConfig("ExecStart=hw-\x1b[0moffload", "true"), "startup configuration")
	require.ErrorContains(t, validateOvsRestartConfig("[Service]", "false"), "expected stale true")
	require.ErrorContains(t, validateOvsRestartConfig("[Service]", "invalid"), "expected stale true")
}

func TestOvsRestartRequiresNewNonzeroPID(t *testing.T) {
	dir := t.TempDir()
	systemctl := `#!/bin/sh
case "$2" in
  is-active) exit 0 ;;
  show) printf '%s\n' "$HWOL_TEST_PID" ;;
  *) exit 1 ;;
esac
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "systemctl"), []byte(systemctl), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ovs-vsctl"), []byte("#!/bin/sh\nprintf 'false\\n'\n"), 0700))
	for _, tc := range []struct {
		pid  string
		pass bool
	}{{"0", false}, {"123", false}, {"invalid", false}, {"124", true}} {
		t.Run(tc.pid, func(t *testing.T) {
			cmd := exec.Command("sh", "-c", ovsRestartReadyScript, "sh", "123")
			cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "HWOL_TEST_PID="+tc.pid)
			out, err := cmd.CombinedOutput()
			if tc.pass {
				require.NoError(t, err, string(out))
				require.Equal(t, "false\n", string(out))
			} else {
				require.Error(t, err, "PID %q must not pass restart verification", tc.pid)
			}
		})
	}
}

func TestOvsRestartCommandsAndTimeouts(t *testing.T) {
	for _, want := range []string{"false", ""} {
		t.Run("baseline="+want, func(t *testing.T) {
			var commands [][]string
			outputs := []string{"[Service]\nExecStart=/usr/bin/ovs-vswitchd", "true", "123", "", ""}
			pid, err := startOvsBaselineRestart("node", want, func(cmd []string, timeout time.Duration) (bytes.Buffer, error) {
				require.Equal(t, 30*time.Second, timeout)
				commands = append(commands, append([]string(nil), cmd...))
				return *bytes.NewBufferString(outputs[len(commands)-1]), nil
			}, func() error { return nil })
			require.NoError(t, err)
			require.Equal(t, "123", pid)
			require.Len(t, commands, 5)
			require.Contains(t, commands[0], "--no-pager")
			require.Contains(t, commands[0], "SYSTEMD_COLORS=0")
			require.Contains(t, commands[1], "--timeout=10")
			require.Contains(t, commands[2], "--no-pager")
			require.Contains(t, commands[3], "--timeout=10")
			if want == "false" {
				require.Contains(t, commands[3], "other_config:hw-offload=false")
			} else {
				require.Contains(t, commands[3], "remove")
			}
			require.Contains(t, commands[4], "--no-block")
			require.Contains(t, commands[4], "restart")
		})
	}
}

func TestOvsRestartStopsOnPreflightOrWriteFailure(t *testing.T) {
	for _, failedStep := range []int{0, 1, 2, 3} {
		t.Run(fmt.Sprintf("step-%d", failedStep), func(t *testing.T) {
			calls := 0
			outputs := []string{"[Service]", "true", "123", "", ""}
			_, err := startOvsBaselineRestart("node", "false", func(cmd []string, timeout time.Duration) (bytes.Buffer, error) {
				step := calls
				calls++
				if step == failedStep {
					return bytes.Buffer{}, context.DeadlineExceeded
				}
				return *bytes.NewBufferString(outputs[step]), nil
			}, func() error { return nil })
			require.Error(t, err)
			require.Equal(t, failedStep+1, calls, "restart must not be requested after a failed preflight or write")
			if failedStep == 3 {
				require.ErrorContains(t, err, "state may have changed")
			}
		})
	}
	_, err := startOvsBaselineRestart("node", "true", func([]string, time.Duration) (bytes.Buffer, error) {
		t.Fatal("unsupported baseline must not execute commands")
		return bytes.Buffer{}, nil
	}, func() error { return nil })
	require.ErrorContains(t, err, "unsupported")
}

func TestOvsRestartRechecksOwnershipBeforeMutations(t *testing.T) {
	for _, failCheck := range []int{1, 2} {
		t.Run(fmt.Sprintf("check-%d", failCheck), func(t *testing.T) {
			calls, checks := 0, 0
			outputs := []string{"[Service]", "true", "123", ""}
			_, err := startOvsBaselineRestart("node", "false", func([]string, time.Duration) (bytes.Buffer, error) {
				out := outputs[calls]
				calls++
				return *bytes.NewBufferString(out), nil
			}, func() error {
				checks++
				if checks == failCheck {
					return fmt.Errorf("foreign ownership")
				}
				return nil
			})
			require.ErrorContains(t, err, "foreign ownership")
			require.Equal(t, 2+failCheck, calls, "must not issue a restart after safety check failure")
			if failCheck == 2 {
				require.ErrorContains(t, err, "baseline changed")
			}
		})
	}
}

func TestOvsRestartReportsUncertainRequestWithoutRetry(t *testing.T) {
	calls := 0
	outputs := []string{"[Service]", "true", "123", ""}
	_, err := startOvsBaselineRestart("node", "false", func([]string, time.Duration) (bytes.Buffer, error) {
		step := calls
		calls++
		if step == 4 {
			return bytes.Buffer{}, context.DeadlineExceeded
		}
		return *bytes.NewBufferString(outputs[step]), nil
	}, func() error { return nil })
	require.ErrorContains(t, err, "restart request on node failed or is uncertain")
	require.Equal(t, 5, calls, "uncertain restart must not trigger another request")
}
