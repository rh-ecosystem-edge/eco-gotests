package cmd

import (
	"bytes"
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netparam"
)

// DefineRouteAndSleep creates route and appends sleep CMD.
func DefineRouteAndSleep(dstNet, nextHop string) []string {
	cmd := DefineRoute(dstNet, nextHop)
	cmd[2] += ` && (echo 1 > /proc/sys/net/ipv4/ip_forward 2>/dev/null || true); \
trap : TERM INT; sleep infinity & wait`

	return cmd
}

// DefineRoute creates route.
func DefineRoute(dstNet, nextHop string) []string {
	return []string{"/bin/bash", "-c", fmt.Sprintf("ip route add %s via %s", dstNet, nextHop)}
}

// SetRouteOnPod adds route to the given pod.
func SetRouteOnPod(client *pod.Builder, dstNet, nextHop string) (bytes.Buffer, error) {
	cmd := DefineRoute(dstNet, nextHop)

	return client.ExecCommand(cmd)
}

// Curl exec curl cmd inside given pod.
func Curl(client *pod.Builder, sourceIPAddr, destIPAddr, ipFamily string, containerName ...string) (string, error) {
	command := fmt.Sprintf("curl --interface %s %s --max-time 10", sourceIPAddr, destIPAddr)

	if ipFamily == netparam.IPV6Family {
		command = fmt.Sprint("curl --interface ", sourceIPAddr, " [", destIPAddr, "] ", "--max-time 5")
	}

	var (
		curlStatus bytes.Buffer
		err        error
	)

	if len(containerName) > 0 {
		curlStatus, err = client.ExecCommand([]string{"bash", "-c", command}, containerName[0])
	} else {
		curlStatus, err = client.ExecCommand([]string{"bash", "-c", command})
	}

	if err != nil {
		return curlStatus.String(), fmt.Errorf("curl command failed - %w", err)
	}

	return curlStatus.String(), nil
}

// Arping exec arping cmd inside given pod.
func Arping(client *pod.Builder, destIPAddr string) (string, error) {
	arpStatus, err := client.ExecCommand([]string{"bash", "-c", fmt.Sprint("arping -I net1 ",
		destIPAddr, " -c3")})
	if err != nil {
		return arpStatus.String(), fmt.Errorf("arping command failed - %w", err)
	}

	return arpStatus.String(), nil
}
