package utils

import (
	"fmt"
	"net"
	"strings"

	"github.com/openshift-kni/lifecycle-agent/internal/common"
)

// ValidateIPFamilyConfig performs family-specific validation for addr, CIDR, gateway and DNS.
func ValidateIPFamilyConfig(
	family string,
	addr string,
	networkCIDR string,
	gateway string,
) error {
	if addr != "" {
		if err := validateIPAddress(family, addr); err != nil {
			return fmt.Errorf("invalid %s address: %w", strings.ToUpper(family), err)
		}
	}

	if networkCIDR != "" {
		if err := validateNetworkCIDR(family, networkCIDR); err != nil {
			return fmt.Errorf("invalid %s machine network CIDR: %w", strings.ToLower(family), err)
		}
	}

	if gateway != "" {
		if err := validateIPAddress(family, gateway); err != nil {
			return fmt.Errorf("invalid %s gateway: %w", strings.ToUpper(family), err)
		}
	}

	// Validate that the gateway is within the machine network CIDR for IPv4.
	// IPv6 gateways may not be within the machine network CIDR since they can be the link-local address.
	if gateway != "" && networkCIDR != "" && family == common.IPv4FamilyName {
		if err := validateGatewayInNetworkCIDR(family, gateway, networkCIDR); err != nil {
			return fmt.Errorf("invalid %s gateway: %w", strings.ToUpper(family), err)
		}
	}

	return nil
}

func validateGatewayInNetworkCIDR(family, gateway, networkCIDR string) error {
	_, ipNet, err := net.ParseCIDR(networkCIDR)
	if err != nil {
		return fmt.Errorf(
			"invalid %s machine network CIDR: %s: %w",
			strings.ToLower(family), networkCIDR, err,
		)
	}

	gatewayIP := net.ParseIP(gateway)
	if gatewayIP == nil {
		return fmt.Errorf(
			"invalid %s gateway: %s: %w", strings.ToUpper(family), gateway, err,
		)
	}

	if !ipNet.Contains(gatewayIP) {
		return fmt.Errorf("%s gateway %s is not within machine network %s",
			strings.ToUpper(family), gateway, networkCIDR,
		)
	}

	return nil
}

func validateNetworkCIDR(family, networkCIDR string) error {
	_, ipNet, err := net.ParseCIDR(networkCIDR)
	if err != nil {
		return fmt.Errorf("invalid %s machine network CIDR: %s", strings.ToLower(family), networkCIDR)
	}

	isIPv4 := family == common.IPv4FamilyName
	isIPv6 := family == common.IPv6FamilyName

	if !isIPv4 && !isIPv6 {
		return fmt.Errorf("unsupported IP family: %s", family)
	}

	if isIPv4 {
		if ipNet.IP.To4() == nil {
			return fmt.Errorf("invalid %s machine network CIDR: %s", strings.ToLower(family), networkCIDR)
		}
		return nil
	}

	if isIPv6 {
		// Note: ip.To16() is non-nil for IPv4 addresses too; ensure we only treat
		// it as IPv6 when it's not IPv4.
		if ipNet.IP.To4() != nil || ipNet.IP.To16() == nil {
			return fmt.Errorf("invalid %s machine network CIDR: %s", strings.ToLower(family), networkCIDR)
		}
	}

	return nil
}

func validateIPAddress(family, ipStr string) error {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return fmt.Errorf("%s is not parsable as a %s address", ipStr, strings.ToUpper(family))
	}

	isIPv4 := family == common.IPv4FamilyName
	isIPv6 := family == common.IPv6FamilyName

	if !isIPv4 && !isIPv6 {
		return fmt.Errorf("unsupported IP family: %s", family)
	}

	if isIPv4 {
		if ip.To4() == nil {
			return fmt.Errorf("%s is not a valid %s address", ipStr, strings.ToUpper(family))
		}
		return nil
	}

	// Note: ip.To16() is non-nil for IPv4 addresses too; ensure we only treat
	// it as IPv6 when it's not IPv4.
	if ip.To4() != nil || ip.To16() == nil {
		return fmt.Errorf("%s is not a valid %s address", ipStr, strings.ToUpper(family))
	}

	return nil
}
