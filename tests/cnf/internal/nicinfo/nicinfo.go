package nicinfo

import (
	"encoding/json"
	"fmt"
	"iter"
	"regexp"
	"sync"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/cluster"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/klog/v2"
)

const logLevel klog.Level = 90

// NodeNICInfo contains information about the tested interfaces on a node.
type NodeNICInfo struct {
	name       string
	interfaces *sync.Map
}

func newNodeNICInfo(nodeName string) *NodeNICInfo {
	return &NodeNICInfo{
		name:       nodeName,
		interfaces: &sync.Map{},
	}
}

// MarkTested marks interfaces as tested for this node. Tested interfaces will show up in the final report.
func (n *NodeNICInfo) MarkTested(interfaceNames ...string) {
	for _, interfaceName := range interfaceNames {
		klog.V(logLevel).Infof("Marking interface %s as tested for node %s", interfaceName, n.name)

		n.interfaces.Store(interfaceName, struct{}{})
	}
}

// MarkSeqTested marks a sequence of interface names as tested for this node. Tested interfaces will show up in the
// final report.
//
// This function is equivalent to:
//
//	MarkTested(slices.Collect(interfaceNames))
func (n *NodeNICInfo) MarkSeqTested(interfaceNames iter.Seq[string]) {
	for interfaceName := range interfaceNames {
		klog.V(logLevel).Infof("Marking interface %s as tested for node %s", interfaceName, n.name)

		n.interfaces.Store(interfaceName, struct{}{})
	}
}

// clusterNodesNICInfo is a map of node names to NodeNICInfo instances. It is global and meant to be used when testing a
// single cluster.
var clusterNodesNICInfo = &sync.Map{}

// Node returns the NodeNICInfo for a given node name. If the NodeNICInfo is not found, a new one is created.
func Node(nodeName string) *NodeNICInfo {
	nodeNICInfoUntyped, _ := clusterNodesNICInfo.LoadOrStore(nodeName, newNodeNICInfo(nodeName))

	nodeNICInfo, ok := nodeNICInfoUntyped.(*NodeNICInfo)
	if !ok {
		klog.V(logLevel).Infof("Expected NodeNICInfo for node %s, but got %T", nodeName, nodeNICInfoUntyped)

		nodeNICInfo = newNodeNICInfo(nodeName)
		clusterNodesNICInfo.Store(nodeName, nodeNICInfo)
	}

	return nodeNICInfo
}

// nicInfo contains information about a network interface.
type nicInfo struct {
	Name             string `json:"name"`
	Driver           string `json:"driver"`
	Version          string `json:"version"`
	FirmwareVersion  string `json:"firmware_version"`
	PTPHardwareClock string `json:"ptp_hardware_clock"`
}

// nodeInfo contains information about a node and its network interfaces. Only tested interfaces will be included in the
// interface list.
type nodeInfo struct {
	NodeName   string    `json:"node_name"`
	Interfaces []nicInfo `json:"interfaces"`
}

// GenerateReport generates a JSON report of the network interface information for all nodes in the cluster.
func GenerateReport(client *clients.Settings) (string, error) {
	nodeNICInfos := getStoredNodeNICInfos()

	var nodeInfos []nodeInfo

	for _, nodeNICInfo := range nodeNICInfos {
		klog.V(logLevel).Infof("Generating report for node %s", nodeNICInfo.name)

		nodeInfo := nodeInfo{
			NodeName:   nodeNICInfo.name,
			Interfaces: []nicInfo{},
		}

		var interfaceNames []string

		nodeNICInfo.interfaces.Range(func(interfaceNameUntyped, _ any) bool {
			interfaceName, ok := interfaceNameUntyped.(string)
			if !ok {
				return true
			}

			interfaceNames = append(interfaceNames, interfaceName)

			return true
		})

		for _, interfaceName := range interfaceNames {
			nicInfo, err := getInterfaceInfo(client, nodeNICInfo.name, interfaceName)
			if err != nil {
				return "", fmt.Errorf("failed to get info for interface %s on node %s: %w", interfaceName, nodeNICInfo.name, err)
			}

			nodeInfo.Interfaces = append(nodeInfo.Interfaces, nicInfo)
		}

		nodeInfos = append(nodeInfos, nodeInfo)
	}

	report, err := json.Marshal(nodeInfos)
	if err != nil {
		return "", fmt.Errorf("failed to marshal node infos: %w", err)
	}

	return string(report), nil
}

func getStoredNodeNICInfos() []*NodeNICInfo {
	var nodeNICInfos []*NodeNICInfo

	clusterNodesNICInfo.Range(func(nodeName, nodeNICInfoUntyped any) bool {
		nodeNICInfo, ok := nodeNICInfoUntyped.(*NodeNICInfo)
		if !ok {
			return true
		}

		nodeNICInfos = append(nodeNICInfos, nodeNICInfo)

		return true
	})

	return nodeNICInfos
}

// These regexes are used to parse the output of the ethtool -i and ethtool -T commands.
var (
	driverRegex           = regexp.MustCompile(`(?m)^driver: (.+)$`)
	versionRegex          = regexp.MustCompile(`(?m)^version: (.+)$`)
	firmwareVersionRegex  = regexp.MustCompile(`(?m)^firmware-version: (.+)$`)
	ptpHardwareClockRegex = regexp.MustCompile(`(?m)^PTP Hardware Clock: (\d+)$`)
)

// getInterfaceInfo gets the information for a given interface on a given node by running ethtool commands on the node
// and parsing the output. Commands are retried up to 3 times with a 20 second delay between retries.
func getInterfaceInfo(client *clients.Settings, nodeName string, interfaceName string) (nicInfo, error) {
	nodeSelector := metav1.ListOptions{
		FieldSelector: fields.SelectorFromSet(fields.Set{"metadata.name": nodeName}).String(),
	}

	driverInfoCommand := fmt.Sprintf("ethtool -i %s", interfaceName)

	driverInfoOutput, err := cluster.ExecCmdWithStdoutWithRetries(
		client, 3, 20*time.Second, driverInfoCommand, nodeSelector)
	if err != nil {
		return nicInfo{}, fmt.Errorf(
			"failed to get driver info for interface %s on node %s: %w", interfaceName, nodeName, err)
	}

	driver := driverRegex.FindStringSubmatch(driverInfoOutput[nodeName])
	if len(driver) == 0 {
		return nicInfo{}, fmt.Errorf("failed to get driver for interface %s on node %s", interfaceName, nodeName)
	}

	version := versionRegex.FindStringSubmatch(driverInfoOutput[nodeName])
	if len(version) == 0 {
		return nicInfo{}, fmt.Errorf("failed to get version for interface %s on node %s", interfaceName, nodeName)
	}

	firmwareVersion := firmwareVersionRegex.FindStringSubmatch(driverInfoOutput[nodeName])
	if len(firmwareVersion) == 0 {
		return nicInfo{}, fmt.Errorf("failed to get firmware version for interface %s on node %s", interfaceName, nodeName)
	}

	ptpHardwareClockCommand := fmt.Sprintf("ethtool -T %s", interfaceName)

	ptpHardwareClockOutput, err := cluster.ExecCmdWithStdoutWithRetries(
		client, 3, 20*time.Second, ptpHardwareClockCommand, nodeSelector)
	if err != nil {
		return nicInfo{}, fmt.Errorf(
			"failed to get PTP hardware clock for interface %s on node %s: %w", interfaceName, nodeName, err)
	}

	ptpHardwareClock := ptpHardwareClockRegex.FindStringSubmatch(ptpHardwareClockOutput[nodeName])
	if len(ptpHardwareClock) == 0 {
		return nicInfo{}, fmt.Errorf("failed to get PTP hardware clock for interface %s on node %s", interfaceName, nodeName)
	}

	return nicInfo{
		Name:             interfaceName,
		Driver:           driver[1],
		Version:          version[1],
		FirmwareVersion:  firmwareVersion[1],
		PTPHardwareClock: ptpHardwareClock[1],
	}, nil
}
