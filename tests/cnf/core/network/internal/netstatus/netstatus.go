package netstatus

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/ipaddr"
	"k8s.io/klog/v2"
)

// AnnotationKey is the Multus network-status pod annotation.
const AnnotationKey = "k8s.v1.cni.cncf.io/network-status"

// Entry is a minimal Multus network-status object.
type Entry struct {
	Name      string   `json:"name"`
	Interface string   `json:"interface"`
	IPs       []string `json:"ips"`
	Mac       string   `json:"mac"`
}

// ParseByInterface pulls the pod and returns network-status entries keyed by interface name
// along with the raw annotation string.
func ParseByInterface(
	apiClient *clients.Settings, podBuilder *pod.Builder) (map[string]Entry, string, error) {
	pulled, err := pod.Pull(apiClient, podBuilder.Definition.Name, podBuilder.Definition.Namespace)
	if err != nil {
		return nil, "", fmt.Errorf("failed to pull pod %s: %w", podBuilder.Definition.Name, err)
	}

	annotation := pulled.Object.Annotations[AnnotationKey]
	if annotation == "" {
		return nil, "", fmt.Errorf("no network-status annotation on pod %s", pulled.Definition.Name)
	}

	var statuses []Entry
	if err := json.Unmarshal([]byte(annotation), &statuses); err != nil {
		return nil, annotation, fmt.Errorf(
			"failed to parse network-status annotation for pod %s: %w", pulled.Definition.Name, err)
	}

	ifaces := make(map[string]Entry, len(statuses))
	for _, status := range statuses {
		if status.Interface != "" {
			ifaces[status.Interface] = status
		}
	}

	return ifaces, annotation, nil
}

// InterfaceIPs returns all IPs assigned to an interface from the pod's network-status annotation.
func InterfaceIPs(
	apiClient *clients.Settings, podBuilder *pod.Builder, interfaceName string) ([]string, error) {
	ifaces, _, err := ParseByInterface(apiClient, podBuilder)
	if err != nil {
		return nil, err
	}

	status, ok := ifaces[interfaceName]
	if !ok {
		return nil, fmt.Errorf("interface %s not found in network-status annotation", interfaceName)
	}

	return status.IPs, nil
}

// PodIPFromInterface retrieves an IP address of a specific interface from a pod's network-status
// annotation. ipFamily should be "ipv4" or "ipv6". For dual-stack, call twice with each family.
func PodIPFromInterface(
	apiClient *clients.Settings, podBuilder *pod.Builder, interfaceName, ipFamily string) (string, error) {
	klog.V(90).Infof("Getting %s from interface %s on pod %s",
		ipFamily, interfaceName, podBuilder.Definition.Name)

	ips, err := InterfaceIPs(apiClient, podBuilder, interfaceName)
	if err != nil {
		return "", err
	}

	for _, ip := range ips {
		ipClean := ipaddr.RemovePrefix(ip)
		isIPv6 := strings.Contains(ipClean, ":")

		if ipFamily == "ipv4" && !isIPv6 {
			return ipClean, nil
		}

		// Skip link-local addresses (fe80::) for IPv6 - return only global/ULA addresses.
		if ipFamily == "ipv6" && isIPv6 && !strings.HasPrefix(strings.ToLower(ipClean), "fe80") {
			return ipClean, nil
		}
	}

	return "", fmt.Errorf("no %s found for interface %s in network-status annotation", ipFamily, interfaceName)
}
