package ipsec_system_test

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/sshcommand"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/ipsec/internal/iperf3workload"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/ipsec/internal/ipsecinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/ipsec/internal/ipsecparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/ipsec/internal/ipsectunnel"
	"k8s.io/klog/v2"
)

// The iperf3 client and server should be started as follows for this test case:
//
// SNO cluster pod (iperf3 client):  iperf3 -c 172.16.123.10 -p 30000   -n 500M
//                                             ^^^^^^^^^^^^^   ^^^^^^^
//                                             SecGW VPN IP   Service NodePort
//
// SecurityGateway  (iperf3 server): iperf3 -s -B 172.16.123.10 -p 30000  -n 500M
//                                                ^^^^^^^^^^^^^   ^^^^^^^
//                                                SecGW VPN IP   Service NodePort

const (
	serviceDeploymentEgressPrefixName = "egress"
)

var _ = Describe(
	"IpsecPacketsSnoEgress",
	Label("IpsecPacketsSnoEgress"),
	Ordered,
	ContinueOnFailure,
	func() {
		BeforeAll(func() {
			klog.V(ipsecparams.IpsecLogLevel).Infof("BeforeAll ipsec_packets_ocp_egress")

			nodePort, err := strconv.Atoi(IpsecTestConfig.NodePort)
			Expect(err).ToNot(HaveOccurred(), "Error converting IpsecTestConfig.NodePort")

			nodePortIncrement, err1 := strconv.Atoi(IpsecTestConfig.NodePortIncrement)
			Expect(err1).ToNot(HaveOccurred(), "Error converting IpsecTestConfig.NodePortIncrement")

			nodeNames, err2 := GetNodeNames()
			Expect(err2).ToNot(HaveOccurred(), "Error getting NodeNames BeforeAll ipsec_packets_ocp_egress")

			// Create a service and workload per node in the cluster
			for index, nodeName := range nodeNames {
				containerLabelsMap := ipsecparams.CreateContainerLabelsMap(index, serviceDeploymentIngressPrefixName)
				srvDeplName := ipsecparams.CreateServiceDeploymentName(index, serviceDeploymentEgressPrefixName)

				klog.V(ipsecparams.IpsecLogLevel).Infof(
					"BeforeAll ipsec_packets_ocp_egress iter node %s, nodePort %d",
					nodeName,
					nodePort)

				// Create the service, using containerLabelsMap to associate it with the deployment
				_, err = iperf3workload.CreateService(APIClient,
					srvDeplName,
					containerLabelsMap,
					int32(nodePort))
				Expect(err).ToNot(HaveOccurred(), "Error while creating iperf3 service")

				// Create the deployment
				_, err = iperf3workload.CreateWorkload(APIClient,
					srvDeplName,
					nodeName,
					containerLabelsMap,
					IpsecTestConfig.Iperf3ToolImage)
				Expect(err).ToNot(HaveOccurred(), "Error while deploying iperf3 workload")

				// Need a unique nodePort per node in cluster
				// For SNO = [30000], For MNO [30000, 31000, 32000]
				nodePort += nodePortIncrement
			}
		})

		It("Asserts packets originating from the SNO cluster are sent via IPSec", func() {
			klog.V(ipsecparams.IpsecLogLevel).Infof("Check egress packets on each cluster node are sent via IPSec")

			nodePortStr := IpsecTestConfig.NodePort

			nodePortIncrement, err1 := strconv.Atoi(IpsecTestConfig.NodePortIncrement)
			Expect(err1).ToNot(HaveOccurred(), "Error converting IpsecTestConfig.NodePortIncrement")

			nodeNames, err2 := GetNodeNames()
			Expect(err2).ToNot(HaveOccurred(), "Error getting NodeNames BeforeAll ipsec_packets_ocp_egress")

			for index, nodeName := range nodeNames {
				klog.V(ipsecparams.IpsecLogLevel).Infof(
					"Checking egress packets on cluster node: %s, index %d",
					nodeName,
					index)
				srvDeplName := ipsecparams.CreateServiceDeploymentName(index, serviceDeploymentEgressPrefixName)

				// Channels for asynchronous calls below
				sshChannel := make(chan *sshcommand.SSHCommandResult)
				iperf3ClientChannel := make(chan bool)

				// Asynchronously start the iperf3 server on the SecGW via SSH
				go func(channel chan *sshcommand.SSHCommandResult) {
					// The iperf3 server-mode command
					iperf3ServerCmd := append(slices.Clone(ipsecparams.Iperf3ServerBaseCmd),
						ipsecparams.Iperf3OptionBind,
						IpsecTestConfig.SecGwServerIP,
						ipsecparams.Iperf3OptionPort,
						nodePortStr)
					sshAddrStr := fmt.Sprintf("%s:%s",
						IpsecTestConfig.SecGwHostIP,
						IpsecTestConfig.SSHPort)
					// For this ssh to work, the ssh privateKey has to have already been
					// added to the SecGW server ~/.ssh/authorized_keys file
					sshResult := sshcommand.SSHCommand(strings.Join(iperf3ServerCmd, " "),
						sshAddrStr,
						IpsecTestConfig.SSHUser,
						IpsecTestConfig.SSHPrivateKey)
					channel <- sshResult
				}(sshChannel)

				// Sleep to let the ssh and iperf3 server get started before
				// trying to start the iperf3 client
				time.Sleep(10 * time.Second)

				packetsBefore := ipsectunnel.TunnelPackets(nodeName)

				// Start the iperf3 client Asynchronously
				go func(channel chan bool) {
					// The iperf3 client-mode command
					iperf3ClientCmd := append(slices.Clone(ipsecparams.Iperf3ClientBaseCmd),
						IpsecTestConfig.SecGwServerIP,
						ipsecparams.Iperf3OptionPort,
						nodePortStr,
						ipsecparams.Iperf3OptionBytes,
						IpsecTestConfig.Iperf3ClientTxBytes)
					containerLabel := ipsecparams.CreateContainerLabelsStr(index, serviceDeploymentIngressPrefixName)
					channel <- iperf3workload.LaunchIperf3Command(APIClient,
						srvDeplName,
						iperf3ClientCmd,
						containerLabel)
				}(iperf3ClientChannel)

				// Get the client via the channel
				clientOutput := <-iperf3ClientChannel
				Expect(clientOutput).To(BeTrue(), "Error in iperf3 client execution.")
				packetsAfter := ipsectunnel.TunnelPackets(nodeName)

				// Get the server results via the channel
				serverOutput := <-sshChannel
				Expect(serverOutput.Err == nil).To(BeTrue(), "Error in iperf3 server execution: %v, %v",
					serverOutput.Err, serverOutput.SSHOutput)

				klog.V(ipsecparams.IpsecLogLevel).Infof("Egress Packets Before in/outBytes %d/%d, After in/outBytes %d/%d",
					packetsBefore.InBytes, packetsBefore.OutBytes,
					packetsAfter.InBytes, packetsAfter.OutBytes)

				// Need to verify the packet counts better
				Expect(packetsAfter.OutBytes-packetsBefore.OutBytes).To(BeNumerically(">", 0), "Invalid number of OutBytes")

				np, _ := strconv.Atoi(nodePortStr)
				nodePortStr = strconv.Itoa(np + nodePortIncrement)
			}
		})

		AfterAll(func() {
			klog.V(ipsecparams.IpsecLogLevel).Infof("AfterAll ipsec_packets_ocp_egress")

			nodeNames, err := GetNodeNames()
			Expect(err).ToNot(HaveOccurred(), "Error getting NodeNames AfterAll ipsec_packets_ocp_egress")

			// Delete the service and workload per node in the cluster
			for index, nodeName := range nodeNames {
				containerLabels := ipsecparams.CreateContainerLabelsStr(index, serviceDeploymentIngressPrefixName)
				srvDeplName := ipsecparams.CreateServiceDeploymentName(index, serviceDeploymentEgressPrefixName)

				klog.V(ipsecparams.IpsecLogLevel).Infof(
					"AfterAll ipsec_packets_ocp_egress iter node %s, labels %s",
					nodeName,
					containerLabels)

				// Stop and delete the workload and deployment
				err = iperf3workload.DeleteWorkload(APIClient, srvDeplName, containerLabels)
				Expect(err).ToNot(HaveOccurred(), "Error in DeleteWorkload")

				err = iperf3workload.DeleteService(APIClient, srvDeplName)
				Expect(err).ToNot(HaveOccurred(), "Error in DeleteService")
			}
		})
	},
)
