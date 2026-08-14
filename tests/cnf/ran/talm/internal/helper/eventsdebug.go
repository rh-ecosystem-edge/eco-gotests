package helper

import (
	"os"
	"os/exec"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/talm/internal/tsparams"
	"k8s.io/klog/v2"
)

// fallbackOcPaths are checked, in order, if "oc" cannot be resolved from PATH. Different environments (bare host
// vs. running inside a pod) have mounted the oc binary at different locations during debugging.
var fallbackOcPaths = []string{"/clusterconfigs/oc", "/usr/local/bin/oc", "/usr/bin/oc"}

// ClearCGUEventsViaOc is a temporary debugging helper that deletes all ClusterGroupUpgrade events in the
// tsparams.TestNamespace namespace on the hub cluster by shelling out to the oc binary. It is retained as a
// fallback/reference implementation; the active BeforeEach/AfterEach/checkpoint calls use the eco-goinfra
// events.ListEventV1s-based implementation in cguevents.go instead, since it matches the API the real event
// assertions will use and avoids the oc/KUBECONFIG path-resolution issues this shell-based version was prone to.
func ClearCGUEventsViaOc() {
	output, err := runOcCommand("delete", "event.v1.events.k8s.io",
		"-n", tsparams.TestNamespace,
		"--field-selector", "regarding.kind==ClusterGroupUpgrade",
		"--ignore-not-found")
	if err != nil {
		klog.V(tsparams.LogLevel).Infof(
			"Failed to clear CGU events in the %s namespace: %v\noutput: %s", tsparams.TestNamespace, err, output)
	} else {
		klog.V(tsparams.LogLevel).Infof("Cleared CGU events in the %s namespace", tsparams.TestNamespace)
	}
}

// PrintCGUEventsViaOc is a temporary debugging helper that prints all ClusterGroupUpgrade events in the
// tsparams.TestNamespace namespace on the hub cluster by shelling out to the oc binary. It is retained as a
// fallback/reference implementation; the active BeforeEach/AfterEach/checkpoint calls use the eco-goinfra
// events.ListEventV1s-based implementation in cguevents.go instead, since it matches the API the real event
// assertions will use and avoids the oc/KUBECONFIG path-resolution issues this shell-based version was prone to.
func PrintCGUEventsViaOc() {
	output, err := runOcCommand("get", "event.v1.events.k8s.io",
		"-n", tsparams.TestNamespace,
		"--field-selector", "regarding.kind==ClusterGroupUpgrade",
		"--sort-by", "{.metadata.creationTimestamp}")
	if err != nil {
		klog.V(tsparams.LogLevel).Infof(
			"Failed to get CGU events in the %s namespace: %v\noutput: %s", tsparams.TestNamespace, err, output)
	} else {
		klog.V(tsparams.LogLevel).Infof("CGU events in the %s namespace:\n%s", tsparams.TestNamespace, output)
	}
}

// runOcCommand runs the oc binary against the hub cluster with the given arguments, returning the combined
// stdout/stderr output for diagnostics.
func runOcCommand(args ...string) ([]byte, error) {
	ocPath := resolveOcPath()

	hubKubeconfig := RANConfig.HubKubeconfig
	if _, statErr := os.Stat(hubKubeconfig); statErr != nil {
		klog.V(tsparams.LogLevel).Infof("Hub kubeconfig %q not found or inaccessible: %v", hubKubeconfig, statErr)
	}

	cmd := exec.Command(ocPath, args...)

	cmd.Env = append(os.Environ(), "KUBECONFIG="+hubKubeconfig)

	return cmd.CombinedOutput()
}

// resolveOcPath finds the oc binary via PATH, falling back to a list of known mount locations seen across
// different debugging environments (bare host vs. running inside a pod).
func resolveOcPath() string {
	if pathFromLookup, lookErr := exec.LookPath("oc"); lookErr == nil {
		return pathFromLookup
	}

	for _, candidate := range fallbackOcPaths {
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate
		}
	}

	klog.V(tsparams.LogLevel).Infof(
		"Could not resolve oc from PATH or fallback locations %v, defaulting to bare 'oc'", fallbackOcPaths)

	return "oc"
}
