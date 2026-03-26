package ptp

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var ensIfacePattern = regexp.MustCompile(`\b(ens[a-z0-9]+)\b`)

// InferSyncInterfacesFromLogs finds sorted unique ens* names on lines that also mention s2 (sync subscribed).
func InferSyncInterfacesFromLogs(logText string) []string {
	seen := make(map[string]struct{})

	for _, line := range strings.Split(logText, "\n") {
		if !strings.Contains(line, "s2") {
			continue
		}

		for _, m := range ensIfacePattern.FindAllStringSubmatch(line, -1) {
			seen[m[1]] = struct{}{}
		}
	}

	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}

	sort.Strings(out)

	return out
}

// ParseCommaSeparatedStrings splits a comma-separated list, trims spaces, drops empties.
func ParseCommaSeparatedStrings(csv string) []string {
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))

	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}

	return out
}

// WPCInterfacePlan holds PHC interface names for multi-card WPC checks.
type WPCInterfacePlan struct {
	Primary string
	SyncAll []string
}

// ResolveWPCInterfaces builds the interface list from config or linuxptp logs.
// If configSyncCSV is non-empty it defines SyncAll order; otherwise names are inferred from logText.
// Primary: configPrimary if set, otherwise the last entry in SyncAll (typical third NIC in 3-card setups).
func ResolveWPCInterfaces(configSyncCSV, configPrimary, logText string) (WPCInterfacePlan, error) {
	var plan WPCInterfacePlan

	switch {
	case strings.TrimSpace(configSyncCSV) != "":
		plan.SyncAll = ParseCommaSeparatedStrings(configSyncCSV)
	default:
		plan.SyncAll = InferSyncInterfacesFromLogs(logText)
	}

	if len(plan.SyncAll) == 0 {
		return plan, fmt.Errorf(
			"no PTP sync interfaces resolved: set ptp_wpc_sync_interfaces or ensure logs contain ens* and s2",
		)
	}

	switch {
	case strings.TrimSpace(configPrimary) != "":
		plan.Primary = strings.TrimSpace(configPrimary)
	default:
		plan.Primary = plan.SyncAll[len(plan.SyncAll)-1]
	}

	return plan, nil
}

// IfaceSyncLineRegexp matches "interfacename ... s2" for any of the given interface names (order-independent).
func IfaceSyncLineRegexp(ifaces []string) (*regexp.Regexp, error) {
	if len(ifaces) == 0 {

		return nil, fmt.Errorf("IfaceSyncLineRegexp: empty interface list")
	}

	quoted := make([]string, len(ifaces))
	for i, name := range ifaces {
		quoted[i] = regexp.QuoteMeta(name)
	}

	pattern := fmt.Sprintf(`(%s).*s2`, strings.Join(quoted, "|"))

	return regexp.MustCompile(pattern), nil
}
