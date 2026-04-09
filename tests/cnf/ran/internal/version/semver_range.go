package version

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// IsVersionStringInRange reports whether version satisfies minimum <= version < maximumUpper using SemVer 2.0
// (github.com/Masterminds/semver/v3).
//
// minimum: empty means no lower bound. For a lower bound that must include pre-releases of X.Y.0 (e.g. OCP
// 4.20.0-20251212...), pass X.Y.0-0 — the lowest pre-release of X.Y.0. Plain X.Y.0 excludes pre-releases of X.Y.0.
//
// maximum: empty means no upper bound. A two-segment bound "X.Y" means the whole minor line: any version strictly
// less than X.(Y+1).0 (e.g. "4.15" allows 4.15.z and their pre-releases, but not 4.16.0).
func IsVersionStringInRange(version, minimum, maximum string) (bool, error) {
	minV, err := parseMinimumBound(minimum)
	if err != nil {
		return false, err
	}

	maxExclusive, err := parseMaximumExclusiveUpper(maximum)
	if err != nil {
		return false, err
	}

	v, err := semver.NewVersion(trimSemverVPrefix(version))
	if err != nil {
		return maximum == "", nil
	}

	if minV != nil && v.LessThan(minV) {
		return false, nil
	}

	if maxExclusive != nil && !v.LessThan(maxExclusive) {
		return false, nil
	}

	return true, nil
}

func trimSemverVPrefix(s string) string {
	return strings.TrimPrefix(strings.TrimSpace(s), "v")
}

func parseMinimumBound(minimum string) (*semver.Version, error) {
	if minimum == "" {
		return nil, nil
	}

	s := trimSemverVPrefix(minimum)
	coerced, err := coerceSemverCore(s)
	if err != nil {
		return nil, fmt.Errorf("invalid minimum provided: '%s'", minimum)
	}

	parsed, err := semver.NewVersion(coerced)
	if err != nil {
		return nil, fmt.Errorf("invalid minimum provided: '%s'", minimum)
	}

	return parsed, nil
}

// parseMaximumExclusiveUpper returns the first version not allowed: valid versions satisfy v < upperExclusive.
func parseMaximumExclusiveUpper(maximum string) (*semver.Version, error) {
	if maximum == "" {
		return nil, nil
	}

	s := trimSemverVPrefix(maximum)
	core, tail := splitSemverCore(s)
	parts := strings.Split(core, ".")
	for _, p := range parts {
		if _, err := strconv.ParseUint(p, 10, 64); err != nil {
			return nil, fmt.Errorf("invalid maximum provided: '%s'", maximum)
		}
	}

	switch len(parts) {
	case 0, 1:
		return nil, fmt.Errorf("invalid maximum provided: '%s'", maximum)
	case 2:
		maj, _ := strconv.ParseUint(parts[0], 10, 64)
		min, _ := strconv.ParseUint(parts[1], 10, 64)

		return semver.NewVersion(fmt.Sprintf("%d.%d.0-0", maj, min+1))
	default:
		full := core
		if tail != "" {
			full = core + tail
		}

		ceil, err := semver.NewVersion(full)
		if err != nil {
			return nil, fmt.Errorf("invalid maximum provided: '%s'", maximum)
		}

		next := ceil.IncPatch()

		return &next, nil
	}
}

func splitSemverCore(s string) (core, tail string) {
	for i, r := range s {
		if r == '-' || r == '+' {
			return s[:i], s[i:]
		}
	}

	return s, ""
}

func coerceSemverCore(s string) (string, error) {
	core, tail := splitSemverCore(s)
	parts := strings.Split(core, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("need at least major.minor")
	}

	for _, p := range parts {
		if _, err := strconv.ParseUint(p, 10, 64); err != nil {
			return "", err
		}
	}

	for len(parts) < 3 {
		parts = append(parts, "0")
	}

	out := strings.Join(parts, ".")
	if tail != "" {
		out += tail
	}

	return out, nil
}
