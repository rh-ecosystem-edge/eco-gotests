/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package utils

import (
	"bytes"
	_ "embed"
	"fmt"
	"net"
	"strings"
	"text/template"
)

// NMStateConfig represents the configuration for generating NMState YAML
type NMStateConfig struct {
	InterfaceName string
	VLAN          bool
	VLANID        int
	IPv4Config    *IPConfig
	IPv6Config    *IPConfig
	DNSServers    []string
}

// IPConfig represents IP configuration for a specific protocol
type IPConfig struct {
	Address        string
	PrefixLen      int
	Enabled        bool
	DHCPEnabled    bool
	DesiredGateway string
	CurrentGateway string
}

// NMStateTemplateData represents the template data for NMState YAML generation
type NMStateTemplateData struct {
	InterfaceName string
	VLANID        int
	IPv4          IPConfig
	IPv6          IPConfig
	DNSServers    []string
}

//go:embed templates/nmstate.yaml.tmpl
var nmstateTemplate string

// GenerateNMStateYAML generates NMState YAML configuration from the provided config
func generateNMStateYAML(config *NMStateConfig) (string, error) {
	if config.InterfaceName == "" {
		return "", fmt.Errorf("interface name is required")
	}

	templateData := NMStateTemplateData{
		InterfaceName: config.InterfaceName,
		VLANID:        config.VLANID,
		DNSServers:    config.DNSServers,
		IPv4: IPConfig{
			Enabled:     false,
			DHCPEnabled: false,
		},
		IPv6: IPConfig{
			Enabled:     false,
			DHCPEnabled: false,
		},
	}

	if config.IPv4Config != nil {
		templateData.IPv4 = *config.IPv4Config
	}

	if config.IPv6Config != nil {
		templateData.IPv6 = *config.IPv6Config
	}

	tmpl, err := template.New("nmstate").Parse(nmstateTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse NMState template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, templateData); err != nil {
		return "", fmt.Errorf("failed to execute NMState template: %w", err)
	}

	return buf.String(), nil
}

// GenerateNMState generates NMState YAML from IPs and their machine network CIDRs.
// It derives the prefix length from the provided CIDRs.
func GenerateNMState(
	interfaceName string,
	ips []string,
	machineNetworks []string,
	desiredGatewayV4 string,
	desiredGatewayV6 string,
	dnsServers []string,
	vlanID int,
	currentGatewayV4 string,
	currentGatewayV6 string,
) (string, error) {
	if len(ips) == 0 {
		return "", fmt.Errorf("at least one IP address is required")
	}
	if len(ips) != len(machineNetworks) {
		return "", fmt.Errorf("ips and machineNetworks must be same length")
	}

	config := &NMStateConfig{
		InterfaceName: interfaceName,
		VLANID:        vlanID,
		DNSServers:    dnsServers,
	}

	for idx, ip := range ips {
		cidr := machineNetworks[idx]
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			return "", fmt.Errorf("invalid machine network CIDR %q: %w", cidr, err)
		}
		ones, _ := ipNet.Mask.Size()

		if strings.Contains(ip, ":") {
			if config.IPv6Config == nil {
				config.IPv6Config = &IPConfig{
					Address:     ip,
					PrefixLen:   ones,
					Enabled:     true,
					DHCPEnabled: false,
				}
			}
		} else {
			if config.IPv4Config == nil {
				config.IPv4Config = &IPConfig{
					Address:     ip,
					PrefixLen:   ones,
					Enabled:     true,
					DHCPEnabled: false,
				}
			}
		}
	}

	if config.IPv4Config != nil {
		config.IPv4Config.DesiredGateway = desiredGatewayV4

		if desiredGatewayV4 != "" && currentGatewayV4 != desiredGatewayV4 {
			config.IPv4Config.CurrentGateway = currentGatewayV4
		}
	}

	if config.IPv6Config != nil {
		config.IPv6Config.DesiredGateway = desiredGatewayV6

		if desiredGatewayV6 != "" && currentGatewayV6 != desiredGatewayV6 {
			config.IPv6Config.CurrentGateway = currentGatewayV6
		}
	}

	return generateNMStateYAML(config)
}
