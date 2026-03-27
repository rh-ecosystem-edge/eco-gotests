package ptp

import (
	"fmt"
	"sort"
	"strings"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
)

// ShellQuoteForNsenter escapes single quotes for use inside sh -c '...' on the node.
func ShellQuoteForNsenter(shellCmd string) string {
	return strings.ReplaceAll(shellCmd, `'`, `'\''`)
}

// ShellQuoteArg wraps s in single quotes for safe embedding in POSIX shell words (escapes ' as '\'').
func ShellQuoteArg(s string) string {
	return `'` + strings.ReplaceAll(s, `'`, `'\''`) + `'`
}

// ExecCmdOnNodeHost runs hostShellCmd in the host mount namespace via the machine-config-daemon pod.
// It reuses cluster.ExecCmdWithStdout and expects exactly one node to match metadata.name == nodeName.
func ExecCmdOnNodeHost(apiClient *clients.Settings, nodeName, hostShellCmd string) (string, error) {
	escaped := ShellQuoteForNsenter(hostShellCmd)

	outMap, err := cluster.ExecCmdWithStdout(
		apiClient,
		escaped,
		metav1.ListOptions{
			FieldSelector: fields.OneTermEqualSelector("metadata.name", nodeName).String(),
		},
	)
	if err != nil {
		return "", err
	}

	if len(outMap) != 1 {
		keys := make([]string, 0, len(outMap))
		for k := range outMap {
			keys = append(keys, k)
		}

		sort.Strings(keys)

		return "", fmt.Errorf(
			"ExecCmdOnNodeHost: expected exactly 1 stdout entry for node %s, got %d hostnames: %v",
			nodeName, len(outMap), keys,
		)
	}

	var combined string

	for _, v := range outMap {
		combined = v

		break
	}

	return combined, nil
}
