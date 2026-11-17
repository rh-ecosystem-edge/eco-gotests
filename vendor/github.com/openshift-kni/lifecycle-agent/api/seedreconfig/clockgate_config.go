package seedreconfig

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

// This file holds the pure validation/interpretation helpers for the two
// SeedReconfiguration fields that drive the post-pivot clock gate: MaxSeedAge and
// ChronyConfig. They live here, next to the field definitions, so the build-time
// create-usb validation (lca-cli/createusb) and the runtime clock gate
// (lca-cli/postpivot) share one implementation and cannot drift. Both packages already
// import api/seedreconfig, and this package imports nothing internal, so this is the only
// place both can reach without an import cycle.

const (
	// DefaultMaxSeedAge is the default plausibility ceiling for the system clock relative to
	// the seed's creation time: a conservative warehouse->ship->field shelf life that catches
	// gross RTC errors (an unset/battery-dead clock landing in 1970/2000, or a wildly future
	// date) without tripping on realistic logistics. It is configurable per deployment via
	// SeedReconfiguration.MaxSeedAge. 180 days was confirmed as a reasonable starting default.
	DefaultMaxSeedAge = 180 * 24 * time.Hour

	// MaxAllowedSeedAge caps a configured MaxSeedAge so an absurd or overflowing value is
	// rejected with a clear config error rather than silently widening the window to meaningless.
	MaxAllowedSeedAge = 10 * 365 * 24 * time.Hour
)

// ParseMaxSeedAge validates and resolves a configured MaxSeedAge value. An empty value uses
// the default (180 days); a non-empty value must be a positive Go duration string no larger
// than MaxAllowedSeedAge, otherwise a config error is returned so callers fail closed rather
// than silently accepting an implausible window. It is used both at build time (create-usb
// manifest validation) and at runtime (the clock gate).
func ParseMaxSeedAge(configured string) (time.Duration, error) {
	if strings.TrimSpace(configured) == "" {
		return DefaultMaxSeedAge, nil
	}
	d, err := time.ParseDuration(configured)
	if err != nil {
		return 0, fmt.Errorf("maxSeedAge %q is not a valid duration: %w", configured, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("maxSeedAge %q must be positive", configured)
	}
	if d > MaxAllowedSeedAge {
		return 0, fmt.Errorf("maxSeedAge %q exceeds the maximum allowed (%s)", configured, MaxAllowedSeedAge)
	}
	return d, nil
}

// ChronyTrustedSource is one source from a chrony configuration that the clock gate is willing
// to treat as authoritative. Kind is one of server, pool, peer, or refclock. Value is the
// configured address for network sources. For a refclock, it is the custom refid when one is
// specified, or otherwise the driver name. ExplicitRefID is true only when a refclock value
// came from an explicit refid option; default driver IDs may carry a numeric unit suffix.
type ChronyTrustedSource struct {
	Kind          string
	Value         string
	ExplicitRefID bool
}

// ChronyTrustedSources returns every source that can independently establish full time before
// recert: a server, pool, or peer pointing at a real address, or a non-RTC, non-PPS hardware
// reference clock. PPS supplies timing pulses but not full time, so it cannot by itself make
// the clock authoritative. The function also excludes sources backed by the same unreliable
// RTC the clock gate exists to distrust: loopback addresses, the NTP local-clock pseudo-address
// family, the `local stratum` fallback directive, and the RTC refclock driver.
func ChronyTrustedSources(chronyConfig string) []ChronyTrustedSource {
	var sources []ChronyTrustedSource
	refclockIndex := 0
	unexpandedIncludes := hasUnexpandedChronyConfigurationIncludes(chronyConfig)
	for _, raw := range strings.Split(chronyConfig, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || isChronyCommentLine(line) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		switch strings.ToLower(fields[0]) {
		case "server", "pool", "peer":
			if len(fields) < 2 || isLocalClockAddress(fields[1]) {
				continue
			}
			sources = append(sources, ChronyTrustedSource{
				Kind:  strings.ToLower(fields[0]),
				Value: strings.TrimSpace(fields[1]),
			})
		case "refclock":
			// A non-RTC, non-PPS hardware reference clock (for example PHC or GPS) can
			// establish full time. PPS provides only timing pulses and requires another source
			// to complete its samples, so the gate cannot infer full-time trust from it. An
			// unlocked SOCK source may be NMEA-only and its runtime samples do not expose whether
			// they contain full time, so it is also excluded. A locked SOCK is eligible only when
			// its lock target resolves to a trusted full-time refclock. The RTC driver is backed by
			// the same unreliable RTC the gate exists to distrust.
			// A valid refclock directive needs a driver and driver-specific parameter.
			// Chrony assigns default refids by the refclock's global index, including
			// refclocks excluded below, so advance the index before filtering.
			if len(fields) < 3 {
				continue
			}
			refclockValue, explicitRefID := chronyRefclockRuntimeValue(fields, refclockIndex)
			refclockIndex++
			if strings.EqualFold(fields[1], "RTC") || isPPSModeRefclock(fields) ||
				hasChronyRefclockOption(fields, "local") ||
				!isTrustedChronySOCKRefclock(fields, chronyConfig, unexpandedIncludes) {
				continue
			}
			if unexpandedIncludes && !explicitRefID {
				continue
			}
			sources = append(sources, ChronyTrustedSource{
				Kind:          "refclock",
				Value:         refclockValue,
				ExplicitRefID: explicitRefID,
			})
		}
		// `local stratum N`, `rtcsync`, `rtcfile`, and other non-source directives are ignored.
	}
	return sources
}

// isPPSModeRefclock reports whether a refclock directive describes a pulse-only source. This
// covers the PPS driver, the generic pps option, and PHC's extpps driver parameter.
func isPPSModeRefclock(fields []string) bool {
	if len(fields) < 3 {
		return false
	}
	if strings.EqualFold(fields[1], "PPS") {
		return true
	}
	for _, parameter := range strings.Split(fields[2], ":") {
		if strings.EqualFold(parameter, "extpps") {
			return true
		}
	}
	for _, option := range fields[3:] {
		if strings.EqualFold(option, "pps") {
			return true
		}
	}
	return false
}

// hasChronyRefclockOption reports whether a refclock directive includes a boolean option.
func hasChronyRefclockOption(fields []string, option string) bool {
	for _, field := range fields[3:] {
		if strings.EqualFold(field, option) {
			return true
		}
	}
	return false
}

// isTrustedChronySOCKRefclock reports whether a SOCK refclock is locked to a refclock that can
// supply full time. The SOCK protocol marks individual samples as pulse-only or full-time, but
// that distinction is not visible in chrony.conf. An unlocked SOCK is therefore untrusted, and
// a locked SOCK is trusted only when its lock target resolves to a trusted non-SOCK refclock.
func isTrustedChronySOCKRefclock(fields []string, chronyConfig string, unexpandedIncludes bool) bool {
	if len(fields) < 3 || !strings.EqualFold(fields[1], "SOCK") {
		return true
	}
	lockTarget, ok := chronyRefclockLockTarget(fields)
	if !ok {
		return false
	}
	return chronyRefclockIDIsTrusted(lockTarget, chronyConfig, unexpandedIncludes)
}

// chronyRefclockLockTarget returns the refid named by a refclock's lock option.
func chronyRefclockLockTarget(fields []string) (string, bool) {
	for i := 3; i < len(fields)-1; i++ {
		if strings.EqualFold(fields[i], "lock") {
			return fields[i+1], true
		}
	}
	return "", false
}

// chronyRefclockIDIsTrusted resolves a refid to a refclock in the supplied configuration and
// reports whether that refclock can supply full time. Default IDs are considered only when no
// unexpanded include or confdir directive can shift Chrony's global refclock numbering.
func chronyRefclockIDIsTrusted(refID, chronyConfig string, unexpandedIncludes bool) bool {
	refclockIndex := 0
	for _, raw := range strings.Split(chronyConfig, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || isChronyCommentLine(line) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 || !strings.EqualFold(fields[0], "refclock") {
			continue
		}
		value, explicitRefID := chronyRefclockRuntimeValue(fields, refclockIndex)
		refclockIndex++
		if value != refID || (!explicitRefID && unexpandedIncludes) {
			continue
		}
		return isTrustedChronyRefclockForLock(fields)
	}
	return false
}

// isTrustedChronyRefclockForLock reports whether a refclock used as a SOCK lock target can
// supply full time. PPS, RTC, local-standard, and SOCK sources cannot provide the reliable
// configuration-level confirmation required for another SOCK's lock target.
func isTrustedChronyRefclockForLock(fields []string) bool {
	return len(fields) >= 3 &&
		!strings.EqualFold(fields[1], "RTC") &&
		!strings.EqualFold(fields[1], "SOCK") &&
		!isPPSModeRefclock(fields) &&
		!hasChronyRefclockOption(fields, "local")
}

// hasUnexpandedChronyConfigurationIncludes reports whether the configuration can include
// additional configuration files whose refclocks are not present in the supplied string.
// Default refclock IDs depend on the global refclock order, so they cannot be reconstructed
// safely when such files may contribute refclocks.
func hasUnexpandedChronyConfigurationIncludes(chronyConfig string) bool {
	for _, raw := range strings.Split(chronyConfig, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || isChronyCommentLine(line) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		switch strings.ToLower(fields[0]) {
		case "include", "confdir":
			return true
		}
	}
	return false
}

// ChronyConfigHasTrustedSource reports whether a chrony configuration declares at least one
// genuine external or on-link time source.
func ChronyConfigHasTrustedSource(chronyConfig string) bool {
	return len(ChronyTrustedSources(chronyConfig)) > 0
}

// ChronyRefclockRefID identifies a configured refclock in the form chronyc reports it.
// ExplicitRefID records whether the value came from an explicit refid option.
type ChronyRefclockRefID struct {
	Value         string
	ExplicitRefID bool
}

// ChronyRTCRefclockRefIDs returns the identifiers reported by RTC refclocks. The clock gate
// uses them to reject an RTC selection even when its explicit refid collides with a trusted
// non-RTC refclock identifier.
func ChronyRTCRefclockRefIDs(chronyConfig string) []ChronyRefclockRefID {
	var refIDs []ChronyRefclockRefID
	refclockIndex := 0
	unexpandedIncludes := hasUnexpandedChronyConfigurationIncludes(chronyConfig)
	for _, raw := range strings.Split(chronyConfig, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || isChronyCommentLine(line) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 || !strings.EqualFold(fields[0], "refclock") {
			continue
		}
		value, explicitRefID := chronyRefclockRuntimeValue(fields, refclockIndex)
		refclockIndex++
		if !strings.EqualFold(fields[1], "RTC") {
			continue
		}
		if unexpandedIncludes && !explicitRefID {
			continue
		}
		refIDs = append(refIDs, ChronyRefclockRefID{Value: value, ExplicitRefID: explicitRefID})
	}
	return refIDs
}

// chronyRefclockRuntimeValue returns the identifier chronyc reports for a refclock. A custom
// refid, when configured, is reported verbatim. Otherwise chrony uses the first three
// characters of the driver name followed by the refclock's global index. The index is based
// on every configured refclock, including sources excluded by the clock gate.
func chronyRefclockRuntimeValue(fields []string, index int) (string, bool) {
	for i := 1; i < len(fields)-1; i++ {
		if strings.EqualFold(fields[i], "refid") {
			return strings.TrimSpace(fields[i+1]), true
		}
	}
	driver := strings.TrimSpace(fields[1])
	if len(driver) > 3 {
		driver = driver[:3]
	}
	if index >= 10 && len(driver) >= 3 {
		// Chrony stores the tens digit in the identifier's third character.
		driver = driver[:2] + strconv.Itoa((index/10)%10)
	}
	return driver + strconv.Itoa(index%10), false
}

// isChronyCommentLine reports whether a trimmed chrony config line is a comment. chrony treats
// lines beginning with '#', '%', '!', or ';' as comments.
func isChronyCommentLine(line string) bool {
	if line == "" {
		return false
	}
	switch line[0] {
	case '#', '%', '!', ';':
		return true
	}
	return false
}

// isLocalClockAddress reports whether addr is a local or RTC-backed source the gate must not
// trust. It rejects every IPv4 loopback address (including chrony's 127.127.x.x reference-clock
// pseudo-addresses), IPv6 loopback, and the conventional localhost hostname.
func isLocalClockAddress(addr string) bool {
	a := strings.ToLower(strings.TrimSpace(addr))
	if a == "localhost" {
		return true
	}
	if ip := net.ParseIP(a); ip != nil {
		return ip.IsLoopback()
	}
	return false
}
