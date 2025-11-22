package ipsectunnel

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/remote"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/ipsec/internal/ipsecparams"
	"k8s.io/klog/v2"
)

// IpsecTunnelPackets IPSec Tunnel packet counters.
type IpsecTunnelPackets struct {
	InBytes  int `default:"0"`
	OutBytes int `default:"0"`
}

// TunnelConnected Check if the IPSec tunnel is connected.
// Return nil if its connected, otherwise return an error.
func TunnelConnected(nodeName string) error {
	// The ipsec show command should list 1 line per connection
	klog.V(ipsecparams.IpsecLogLevel).Infof("Checking IPSec tunnel connection status. Exec cmd: %v",
		ipsecparams.IpsecCmdShow)

	ipsecShowStr, err := remote.ExecuteOnNodeWithDebugPod(ipsecparams.IpsecCmdShow, nodeName)
	if err != nil {
		klog.V(ipsecparams.IpsecLogLevel).Infof("error could not execute command: %s", err)

		return err
	}

	// Example output string:
	//   (output will be empty if there are no tunnels connected)
	// [core@sno ~]$ sudo ipsec show
	// 10.1.232.10/32 <=> 172.16.123.0/24 using reqid 16389

	if len(ipsecShowStr) < 1 {
		return fmt.Errorf("error: IPSec tunnel is not up")
	}

	klog.V(ipsecparams.IpsecLogLevel).Infof("IPSec tunnel is connected: %s",
		ipsecShowStr)

	return nil
}

// TunnelPackets Return the tunnel ingress and egress packets.
func TunnelPackets(nodeName string) *IpsecTunnelPackets {
	klog.V(ipsecparams.IpsecLogLevel).Infof("Checking IPSec tunnel traffic status. Exec cmd: %v",
		ipsecparams.IpsecCmdTrafficStatus)

	ipsecOutput, err := remote.ExecuteOnNodeWithDebugPod(ipsecparams.IpsecCmdTrafficStatus, nodeName)
	if err != nil {
		klog.V(ipsecparams.IpsecLogLevel).Infof("error could not execute command: %s", err)

		return nil
	}

	klog.V(ipsecparams.IpsecLogLevel).Infof("IPSec packets: %s", ipsecOutput)

	// Example output string:
	//   (output will be empty if there are no tunnels connected)
	// [core@sno ~]$ sudo ipsec trafficstatus
	// 006 #12: "21939ab9-6546-4652-8eaf-1be04415ac24", type=ESP, add_time=1714739598, \
	//		inBytes=0, outBytes=0, maxBytes=2^63B, id='CN=north'
	if len(ipsecOutput) < 1 {
		klog.V(ipsecparams.IpsecLogLevel).Infof("Error IPSec tunnel is not up for traffic status")

		return nil
	}

	//
	// Get the inBytes
	//
	commaStr := ","
	inBytesStr := "inBytes="

	startIndex := strings.Index(ipsecOutput, inBytesStr)
	if startIndex < 1 {
		klog.V(ipsecparams.IpsecLogLevel).Infof("Error Cannot parse IPSec traffic status inBytes: %v", ipsecOutput)

		return nil
	}

	endIndex := strings.Index(ipsecOutput[startIndex:], commaStr)
	if endIndex < 1 {
		klog.V(ipsecparams.IpsecLogLevel).Infof("Error Cannot parse IPSec traffic status inBytes ending: %v", ipsecOutput)

		return nil
	}

	tunnelPackets := &IpsecTunnelPackets{}

	tunnelPackets.InBytes, err = strconv.Atoi(ipsecOutput[startIndex+len(inBytesStr) : startIndex+endIndex])
	if err != nil {
		klog.V(ipsecparams.IpsecLogLevel).Infof("Error Cannot parse IPSec traffic status inBytes value: %v", ipsecOutput)

		return nil
	}

	//
	// Get the outBytes
	//
	outBytesStr := "outBytes="

	startIndex = strings.Index(ipsecOutput, outBytesStr)
	if startIndex < 1 {
		klog.V(ipsecparams.IpsecLogLevel).Infof("Error Cannot parse IPSec traffic status outBytes: %v", ipsecOutput)

		return nil
	}

	endIndex = strings.Index(ipsecOutput[startIndex:], commaStr)
	if endIndex < 1 {
		klog.V(ipsecparams.IpsecLogLevel).Infof("Error Cannot parse IPSec traffic status outBytes ending: %v", ipsecOutput)

		return nil
	}

	tunnelPackets.OutBytes, err = strconv.Atoi(ipsecOutput[startIndex+len(outBytesStr) : startIndex+endIndex])
	if err != nil {
		klog.V(ipsecparams.IpsecLogLevel).Infof("Error Cannot parse IPSec traffic status outBytes value: %v", ipsecOutput)

		return nil
	}

	return tunnelPackets
}
