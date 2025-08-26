package tests

import (
	"encoding/xml"
	"fmt"
	"net"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/policy/internal/tsparams"
)

func verifyPaths(
	sPod, dPod *pod.Builder,
	ipv4ExpectedResult, ipv6ExpectedResult map[string]string,
	testData tsparams.PodsData,
) {
	By("Deriving applicable paths between given source and destination pods")
	runNmapAndValidateResults(sPod, testData[sPod.Object.Name].IPv4,
		testData[dPod.Object.Name].Protocols, testData[dPod.Object.Name].Ports,
		strings.Split(testData[dPod.Object.Name].IPv4, "/")[0], ipv4ExpectedResult)
	runNmapAndValidateResults(sPod, testData[sPod.Object.Name].IPv6,
		testData[dPod.Object.Name].Protocols, testData[dPod.Object.Name].Ports,
		strings.Split(testData[dPod.Object.Name].IPv6, "/")[0], ipv6ExpectedResult)
}

func runNmapAndValidateResults(
	sPod *pod.Builder,
	sourceIP string,
	protocols []string,
	ports []string,
	targetIP string,
	expectedResult map[string]string) {
	// NmapXML defines the structure nmap command output in xml.
	type NmapXML struct {
		XMLName xml.Name `xml:"nmaprun"`
		Text    string   `xml:",chardata"`
		Host    struct {
			Text   string `xml:",chardata"`
			Status struct {
				Text  string `xml:",chardata"`
				State string `xml:"state,attr"`
			} `xml:"status"`
			Address []struct {
				Text     string `xml:",chardata"`
				Addr     string `xml:"addr,attr"`
				Addrtype string `xml:"addrtype,attr"`
			} `xml:"address"`
			Ports struct {
				Text string `xml:",chardata"`
				Port []struct {
					Text     string `xml:",chardata"`
					Protocol string `xml:"protocol,attr"`
					Portid   string `xml:"portid,attr"`
					State    struct {
						Text  string `xml:",chardata"`
						State string `xml:"state,attr"`
					} `xml:"state"`
				} `xml:"port"`
			} `xml:"ports"`
		} `xml:"host"`
	}

	By("Running nmap command in source pod")

	var nmapOutput NmapXML

	nmapCmd := fmt.Sprintf("nmap -v -oX - -sT -sU -p T:5001,T:5002,U:5003 %s", targetIP)

	if net.ParseIP(targetIP).To4() == nil {
		nmapCmd += " -6"
	}

	output, err := sPod.ExecCommand([]string{"/bin/bash", "-c", nmapCmd})
	Expect(err).NotTo(HaveOccurred(), "Failed to execute nmap command in source pod")

	err = xml.Unmarshal(output.Bytes(), &nmapOutput)
	Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to unmarshal nmap output: %s", output.String()))

	By("Verifying nmap output is matching with expected results")
	Expect(len(nmapOutput.Host.Ports.Port)).To(Equal(len(ports)),
		fmt.Sprintf("number of ports in nmap output as expected. Nmap XML output: %v", nmapOutput.Host.Ports.Port))

	for index := range len(nmapOutput.Host.Ports.Port) {
		if expectedResult[nmapOutput.Host.Ports.Port[index].Portid] == "pass" {
			By(fmt.Sprintf("Path %s/%s =====> %s:%s:%s Expected to Pass\n",
				sPod.Object.Name, sourceIP, targetIP, protocols[index], ports[index]))
			Expect(nmapOutput.Host.Ports.Port[index].State.State).To(Equal("open"),
				fmt.Sprintf("Port is not open as expected. Output: %v", nmapOutput.Host.Ports.Port[index]))
		} else {
			By(fmt.Sprintf("Path %s/%s =====> %s:%s:%s Expected to Fail\n",
				sPod.Object.Name, sourceIP, targetIP, protocols[index], ports[index]))
			Expect(nmapOutput.Host.Ports.Port[index].State.State).To(SatisfyAny(Equal("open|filtered"), Equal("filtered")),
				fmt.Sprintf("Port is not filtered as expected. Output: %v", nmapOutput.Host.Ports.Port[index]))
		}
	}
}
