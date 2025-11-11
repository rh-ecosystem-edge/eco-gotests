package rds_core_system_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clusteroperator"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscorecommon"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreparams"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe(
	"RDS Core Top Level Suite",
	Ordered,
	ContinueOnFailure,
	Label("rds-core-workflow"), func() {
		Context("Configured Cluster", Label("clean-cluster"), func() {
			It("Verify EgressService with Cluster ExternalTrafficPolicy",
				Label("egress", "egress-etp-cluster"), reportxml.ID("76485"),
				rdscorecommon.VerifyEgressServiceWithClusterETP)

			It("Verify EgressService with Local ExternalTrafficPolicy",
				Label("egress", "egress-etp-local"), reportxml.ID("76484"),
				rdscorecommon.VerifyEgressServiceWithLocalETP)

			It("Verifies workload reachable over BGP route",
				Label("frr"), reportxml.ID("76009"),
				rdscorecommon.ReachURLviaFRRroute)

			It("Verifies KDump service on Control Plane node",
				Label("kdump", "kdump-cp"), reportxml.ID("75620"),
				rdscorecommon.VerifyKDumpOnControlPlane)

			It("Verifies KDump service on Worker node",
				Label("kdump", "kdump-worker"), reportxml.ID("75621"),
				rdscorecommon.VerifyKDumpOnWorkerMCP)

			It("Verifies KDump service on CNF node",
				Label("kdump", "kdump-cnf"), reportxml.ID("75622"),
				rdscorecommon.VerifyKDumpOnCNFMCP)

			It("Verifies mount namespace service on Control Plane node",
				Label("mount-ns", "mount-ns-cp"), reportxml.ID("75048"),
				rdscorecommon.VerifyMountNamespaceOnControlPlane)

			It("Verifies mount namespace service on Worker node",
				Label("mount-ns", "mount-ns-worker"), reportxml.ID("75832"),
				rdscorecommon.VerifyMountNamespaceOnWorkerMCP)

			It("Verifies mount namespace service on CNF node",
				Label("mount-ns", "mount-ns-cnf"), reportxml.ID("75833"),
				rdscorecommon.VerifyMountNamespaceOnCNFMCP)

			It("Verifies SR-IOV workloads on same node and different networks",
				Label("sriov", "sriov-same-node-different-nets"),
				reportxml.ID("72258"), MustPassRepeatedly(3),
				rdscorecommon.VerifySRIOVWorkloadsOnSameNodeDifferentNet)

			It("Verifies SR-IOV workloads on different nodes and different networks",
				Label("sriov", "sriov-different-nodes-different-nets"),
				reportxml.ID("72259"), MustPassRepeatedly(3),
				rdscorecommon.VerifySRIOVWorkloadsOnDifferentNodesDifferentNet)

			It("Verifies NUMA-aware workload is deployable", reportxml.ID("73677"), Label("nrop"),
				rdscorecommon.VerifyNROPWorkload)

			It("Verifies all policies are compliant", reportxml.ID("72354"), Label("validate-policies"),
				rdscorecommon.ValidateAllPoliciesCompliant)

			It("Verify MACVLAN workload on different nodes", Label("macvlan",
				"validate-new-macvlan-different-nodes"), reportxml.ID("72566"),
				rdscorecommon.VerifyMacVlanOnDifferentNodes)

			It("Verify MACVLAN workloads on the same node", Label("macvlan",
				"validate-new-macvlan-same-node"), reportxml.ID("72567"),
				rdscorecommon.VerifyMacVlanOnSameNode)

			It("Verify IPVLAN workload on different nodes", Label("ipvlan",
				"validate-new-ipvlan-different-nodes"), reportxml.ID("75057"),
				rdscorecommon.VerifyIPVlanOnDifferentNodes)

			It("Verify IPVLAN workloads on the same node", Label("ipvlan", "validate-new-ipvlan-same-node"),
				reportxml.ID("75562"), rdscorecommon.VerifyIPVlanOnSameNode)

			It("Verifices SR-IOV workloads on the same node",
				Label("sriov", "sriov-same-node"), reportxml.ID("71949"), MustPassRepeatedly(3),
				rdscorecommon.VerifySRIOVWorkloadsOnSameNode)

			It("Verifices SR-IOV workloads on different nodes",
				Label("sriov", "sriov-different-node"), reportxml.ID("71950"), MustPassRepeatedly(3),
				rdscorecommon.VerifySRIOVWorkloadsOnDifferentNodes)

			It(fmt.Sprintf("Verifies %s namespace exists", RDSCoreConfig.NMStateOperatorNamespace),
				Label("nmstate", "nmstate-ns"),
				rdscorecommon.VerifyNMStateNamespaceExists)

			It("Verifies NMState instance exists",
				Label("nmstate", "nmstate-instance"), reportxml.ID("67027"),
				rdscorecommon.VerifyNMStateInstanceExists)

			It("Verifies all NodeNetworkConfigurationPolicies are Available",
				Label("nmstate", "validate-policies"), reportxml.ID("71846"),
				rdscorecommon.VerifyAllNNCPsAreOK)

			It("Verifies CephFS",
				Label("persistent-storage", "odf-cephfs-pvc"), reportxml.ID("71850"), MustPassRepeatedly(3),
				rdscorecommon.VerifyCephFSPVC)

			It("Verifies CephRBD",
				Label("persistent-storage", "odf-cephrbd-pvc"), reportxml.ID("71989"), MustPassRepeatedly(3),
				rdscorecommon.VerifyCephRBDPVC)

			It("Verify eIPv4 address from the list of defined used for the assigned pods in a single eIP namespace",
				Label("egressip", "egressip-ipv4", "egressip-single-ns"), reportxml.ID("78105"),
				rdscorecommon.VerifyEgressIPOneNamespaceThreeNodesBalancedEIPTrafficIPv4)

			It("Verify eIPv6 address from the list of defined used for the assigned pods in a single eIP namespace",
				Label("egressip", "egressip-ipv6", "egressip-single-ns"), reportxml.ID("78135"),
				rdscorecommon.VerifyEgressIPOneNamespaceThreeNodesBalancedEIPTrafficIPv6)

			It("Verify eIPv4 address from the list of defined used for the assigned pods in two eIP namespaces",
				Label("egressip", "egressip-ipv4", "egressip-two-ns"), reportxml.ID("75060"),
				rdscorecommon.VerifyEgressIPTwoNamespacesThreeNodesIPv4)

			It("Verify eIPv6 address from the list of defined used for the assigned pods in two eIP namespaces",
				Label("egressip", "egressip-ipv6", "egressip-two-ns"), reportxml.ID("78136"),
				rdscorecommon.VerifyEgressIPTwoNamespacesThreeNodesIPv6)

			It("Verify eIP address from the list of defined does not used for the assigned pods in single "+
				"eIP namespace, but with the wrong pod label",
				Label("egressip", "egressip-single-ns"), reportxml.ID("78106"),
				rdscorecommon.VerifyEgressIPOneNamespaceOneNodeWrongPodLabel)

			It("Verify eIP address from the list of defined does not used for the assigned pods in single "+
				"eIP namespace with the wrong label",
				Label("egressip", "egressip-one-ns"), reportxml.ID("78109"),
				rdscorecommon.VerifyEgressIPWrongNsLabel)

			It("Verify eIPv4 address assigned to the next available node after node reboot; fail-over",
				Label("egressip", "egressip-ipv4", "egressip-failover"), reportxml.ID("78280"),
				rdscorecommon.VerifyEgressIPFailOverIPv4)

			It("Verify eIPv6 address assigned to the next available node after node reboot; fail-over",
				Label("egressip", "egressip-ipv6", "egressip-failover"), reportxml.ID("78283"),
				rdscorecommon.VerifyEgressIPFailOverIPv6)

			AfterEach(func(ctx SpecContext) {
				By("Ensure all nodes are Ready and scheduling enabled")
				rdscorecommon.EnsureInNodeReadiness(ctx)
			})
		})

		Context("Ungraceful Cluster Reboot", Label("ungraceful-cluster-reboot"), func() {
			BeforeAll(func(ctx SpecContext) {
				By("Creating EgressService with ETP=Cluster")
				rdscorecommon.VerifyEgressServiceWithClusterETP(ctx)

				By("Creating EgressService with ETP=Local")
				rdscorecommon.VerifyEgressServiceWithLocalETP(ctx)

				By("Creating EgressIP workload config")
				rdscorecommon.CreateEgressIPTestDeployment()

				By("Creating a workload with CephFS PVC")
				rdscorecommon.DeployWorkflowCephFSPVC(ctx)

				By("Creating a workload with CephRBD PVC")
				rdscorecommon.DeployWorkloadCephRBDPVC(ctx)

				By("Creating SR-IOV workloads on the same node")
				rdscorecommon.VerifySRIOVWorkloadsOnSameNode(ctx)

				By("Creating SR-IOV workloads on different nodes")
				rdscorecommon.VerifySRIOVWorkloadsOnDifferentNodes(ctx)

				By("Creating MACVLAN workloads on the same node")
				rdscorecommon.VerifyMacVlanOnSameNode()

				By("Creating MACVLAN workloads on different nodes")
				rdscorecommon.VerifyMacVlanOnDifferentNodes()

				By("Creating IPVLAN workloads on the same node")
				rdscorecommon.VerifyIPVlanOnSameNode()

				By("Creating IPVLAN workloads on different nodes")
				rdscorecommon.VerifyIPVlanOnDifferentNodes()

				By("Creating NUMA aware workload")
				rdscorecommon.VerifyNROPWorkload(ctx)

				By("Creating SR-IOV workload on same node and different SR-IOV networks")
				rdscorecommon.VerifySRIOVWorkloadsOnSameNodeDifferentNet(ctx)

				By("Creating SR-IOV workload on different nodes and different SR-IOV networks")
				rdscorecommon.VerifySRIOVWorkloadsOnDifferentNodesDifferentNet(ctx)
			})

			It("Verifies ungraceful cluster reboot",
				Label("rds-core-hard-reboot"), reportxml.ID("30020"),
				rdscorecommon.VerifyUngracefulReboot)

			It("Verifies all ClusterOperators are Available after ungraceful reboot",
				Label("verify-cos"), reportxml.ID("71868"), func() {
					By("Checking all cluster operators")

					glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Waiting for all ClusterOperators to be Available")
					glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Sleeping for 3 minutes")

					time.Sleep(3 * time.Minute)

					ok, err := clusteroperator.WaitForAllClusteroperatorsAvailable(
						APIClient, 15*time.Minute, metav1.ListOptions{})
					Expect(err).ToNot(HaveOccurred(), "Failed to get cluster operator status")
					Expect(ok).To(BeTrue(), "Some cluster operators not Available")
				})

			It("Removes all pods with UnexpectedAdmissionError", Label("sriov-unexpected-pods"),
				MustPassRepeatedly(3), func(ctx SpecContext) {
					By("Remove any pods in UnexpectedAdmissionError state")

					glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Remove pods with UnexpectedAdmissionError status")

					glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Sleeping for 3 minutes")

					time.Sleep(3 * time.Minute)

					listOptions := metav1.ListOptions{
						FieldSelector: "status.phase=Failed",
					}

					var podsList []*pod.Builder

					var err error

					Eventually(func() bool {
						podsList, err = pod.ListInAllNamespaces(APIClient, listOptions)
						if err != nil {
							glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to list pods: %v", err)

							return false
						}

						glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Found %d pods matching search criteria",
							len(podsList))

						for _, failedPod := range podsList {
							glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod %q in %q ns matches search criteria",
								failedPod.Definition.Name, failedPod.Definition.Namespace)
						}

						return true
					}).WithContext(ctx).WithPolling(5*time.Second).WithTimeout(1*time.Minute).Should(BeTrue(),
						"Failed to search for pods with UnexpectedAdmissionError status")

					for _, failedPod := range podsList {
						if failedPod.Definition.Status.Reason == "UnexpectedAdmissionError" {
							glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deleting pod %q in %q ns",
								failedPod.Definition.Name, failedPod.Definition.Namespace)

							_, err := failedPod.DeleteAndWait(5 * time.Minute)
							Expect(err).ToNot(HaveOccurred(), "could not delete pod in UnexpectedAdmissionError state")
						}
					}
				})

			It("Verifies all deploymentes are available",
				Label("verify-deployments"), reportxml.ID("71872"),
				rdscorecommon.WaitAllDeploymentsAreAvailable)

			It("Verifies all statefulsets are in Ready state after ungraceful reboot",
				Label("statefulset-ready"), reportxml.ID("73972"),
				rdscorecommon.WaitAllStatefulsetsReady)

			It("Verifies all NodeNetworkConfigurationPolicies are Available after ungraceful reboot",
				Label("nmstate", "validate-policies"), reportxml.ID("71848"),
				rdscorecommon.VerifyAllNNCPsAreOK)

			It("Verifies all policies are compliant after hard reboot", reportxml.ID("72355"),
				Label("validate-policies"), rdscorecommon.ValidateAllPoliciesCompliant)

			It("Verify EgressService with Cluster ExternalTrafficPolicy after ungraceful reboot",
				Label("egress-validate-cluster-etp", "egress"), reportxml.ID("76503"),
				rdscorecommon.VerifyEgressServiceConnectivityETPCluster)

			It("Verify EgressService with Local ExternalTrafficPolicy after ungraceful reboot",
				Label("egress-validate-local-etp", "egress"), reportxml.ID("76504"),
				rdscorecommon.VerifyEgressServiceConnectivityETPLocal)

			It("Verify EgressService  ingress with Local ExternalTrafficPolicy after ungraceful reboot",
				Label("egress-validate-local-etp", "egress"), reportxml.ID("76672"),
				rdscorecommon.VerifyEgressServiceETPLocalIngressConnectivity)

			It("Verify EgressIP connectivity over IPv4 address after ungraceful reboot",
				Label("egressip", "egressip-ipv4"), reportxml.ID("75061"),
				rdscorecommon.VerifyEgressIPConnectivityThreeNodesIPv4)

			It("Verify EgressIP connectivity over IPv6 address after ungraceful reboot",
				Label("egressip", "egressip-ipv6"), reportxml.ID("78137"),
				rdscorecommon.VerifyEgressIPConnectivityThreeNodesIPv6)

			It("Verifies NUMA-aware workload is available after ungraceful reboot",
				Label("nrop"), reportxml.ID("73727"),
				rdscorecommon.VerifyNROPWorkloadAvailable)

			It("Verifies CephFS PVC is still accessible",
				Label("persistent-storage", "verify-cephfs"), reportxml.ID("71873"),
				rdscorecommon.VerifyDataOnCephFSPVC)

			It("Verifies CephRBD PVC is still accessible",
				Label("persistent-storage", "verify-cephrbd"), reportxml.ID("71990"),
				rdscorecommon.VerifyDataOnCephRBDPVC)

			It("Verifies CephFS workload is deployable after hard reboot",
				Label("persistent-storage", "deploy-cephfs-pvc"), reportxml.ID("71851"), MustPassRepeatedly(3),
				rdscorecommon.VerifyCephFSPVC)

			It("Verifies CephRBD workload is deployable after hard reboot",
				Label("persistent-storage", "deploy-cephrbd-pvc"), reportxml.ID("71992"), MustPassRepeatedly(3),
				rdscorecommon.VerifyCephRBDPVC)

			It("Verifies SR-IOV workloads on different nodes post reboot",
				Label("sriov", "verify-sriov-different-node"), reportxml.ID("71952"),
				rdscorecommon.VerifySRIOVConnectivityBetweenDifferentNodes)

			It("Verifies SR-IOV workloads on the same node post reboot",
				Label("sriov", "verify-sriov-same-node"), reportxml.ID("71951"),
				rdscorecommon.VerifySRIOVConnectivityOnSameNode)

			It("Verifies SR-IOV workloads on the same node and different SR-IOV nets post reboot",
				Label("sriov", "verify-sriov-same-node-diff-nets"), reportxml.ID("72264"),
				rdscorecommon.VerifySRIOVConnectivityOnSameNodeAndDifferentNets)

			It("Verifies SR-IOV workloads on different nodes and different SR-IOV nets post reboot",
				Label("sriov", "verify-sriov-diff-nodes-diff-nets"), reportxml.ID("72265"),
				rdscorecommon.VerifySRIOVConnectivityOnDifferentNodesAndDifferentNetworks)

			It("Verifies MACVLAN workloads on the same node post hard reboot",
				Label("macvlan", "verify-macvlan-same-node"), reportxml.ID("72569"),
				rdscorecommon.VerifyMACVLANConnectivityOnSameNode)

			It("Verifies MACVLAN workloads on different nodes post hard reboot",
				Label("macvlan", "verify-macvlan-different-nodes"), reportxml.ID("72568"),
				rdscorecommon.VerifyMACVLANConnectivityBetweenDifferentNodes)

			It("Verifies IPVLAN workloads on the same node post hard reboot",
				Label("ipvlan", "verify-ipvlan-same-node"), reportxml.ID("75565"),
				rdscorecommon.VerifyIPVLANConnectivityOnSameNode)

			It("Verifies IPVLAN workloads on different nodes post hard reboot",
				Label("ipvlan", "verify-ipvlan-different-nodes"), reportxml.ID("75059"),
				rdscorecommon.VerifyIPVLANConnectivityBetweenDifferentNodes)

			It("Verifies workload reachable over BGP route post hard reboot",
				Label("frr"), reportxml.ID("76010"),
				rdscorecommon.ReachURLviaFRRroute)
		})

		Context("Graceful Cluster Reboot", Label("graceful-cluster-reboot"), func() {
			BeforeAll(func(ctx SpecContext) {
				By("Creating EgressService with ETP=Cluster")
				rdscorecommon.VerifyEgressServiceWithClusterETP(ctx)

				By("Creating EgressService with ETP=Local")
				rdscorecommon.VerifyEgressServiceWithLocalETP(ctx)

				By("Creating EgressIP workload config")
				rdscorecommon.CreateEgressIPTestDeployment()

				By("Creating a workload with CephFS PVC")
				rdscorecommon.DeployWorkflowCephFSPVC(ctx)

				By("Creating a workload with CephRBD PVC")
				rdscorecommon.DeployWorkloadCephRBDPVC(ctx)

				By("Creating SR-IOV worklods that run on same node")
				rdscorecommon.VerifySRIOVWorkloadsOnSameNode(ctx)

				By("Verifying SR-IOV workloads on different nodes")
				rdscorecommon.VerifySRIOVWorkloadsOnDifferentNodes(ctx)

				By("Creating MACVLAN workloads on the same node")
				rdscorecommon.VerifyMacVlanOnSameNode()

				By("Creating MACVLAN workloads on different nodes")
				rdscorecommon.VerifyMacVlanOnDifferentNodes()

				By("Creating IPVLAN workloads on the same node")
				rdscorecommon.VerifyIPVlanOnSameNode()

				By("Creating IPVLAN workloads on different nodes")
				rdscorecommon.VerifyIPVlanOnDifferentNodes()

				By("Creating NUMA aware workload")
				rdscorecommon.VerifyNROPWorkload(ctx)

				By("Creating SR-IOV workload on same node and different SR-IOV networks")
				rdscorecommon.VerifySRIOVWorkloadsOnSameNodeDifferentNet(ctx)

				By("Creating SR-IOV workload on different nodes and different SR-IOV networks")
				rdscorecommon.VerifySRIOVWorkloadsOnDifferentNodesDifferentNet(ctx)
			})

			It("Verifies graceful cluster reboot",
				Label("rds-core-soft-reboot"), reportxml.ID("30021"), rdscorecommon.VerifySoftReboot)

			It("Verifies all ClusterOperators are Available after ungraceful reboot",
				Label("verify-cos"), reportxml.ID("72040"), func() {
					By("Checking all cluster operators")

					glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Waiting for all ClusterOperators to be Available")
					glog.V(rdscoreparams.RDSCoreLogLevel).Infof("Sleeping for 3 minutes")

					time.Sleep(3 * time.Minute)

					ok, err := clusteroperator.WaitForAllClusteroperatorsAvailable(
						APIClient, 15*time.Minute, metav1.ListOptions{})
					Expect(err).ToNot(HaveOccurred(), "Failed to get cluster operator status")
					Expect(ok).To(BeTrue(), "Some cluster operators not Available")
				})

			It("Verifies all deploymentes are available",
				Label("verify-deployments"), reportxml.ID("72041"),
				rdscorecommon.WaitAllDeploymentsAreAvailable)

			It("Verifies all statefulsets are in Ready state after soft reboot",
				Label("statefulset-ready"), reportxml.ID("73973"),
				rdscorecommon.WaitAllStatefulsetsReady)

			It("Verifies all NodeNetworkConfigurationPolicies are Available after soft reboot",
				Label("nmstate", "validate-policies"), reportxml.ID("71849"),
				rdscorecommon.VerifyAllNNCPsAreOK)

			It("Verifies all policies are compliant after soft reboot", reportxml.ID("72357"),
				Label("validate-policies"), rdscorecommon.ValidateAllPoliciesCompliant)

			It("Verify EgressService with Cluster ExternalTrafficPolicy after graceful reboot",
				Label("egress-validate-cluster-etp", "egress"), reportxml.ID("76505"),
				rdscorecommon.VerifyEgressServiceConnectivityETPCluster)

			It("Verify EgressService with Local ExternalTrafficPolicy after graceful reboot",
				Label("egress-validate-local-etp", "egress"), reportxml.ID("76506"),
				rdscorecommon.VerifyEgressServiceConnectivityETPLocal)

			It("Verify EgressService ingress with Local ExternalTrafficPolicy after graceful reboot",
				Label("egress-validate-local-etp", "egress"), reportxml.ID("76673"),
				rdscorecommon.VerifyEgressServiceETPLocalIngressConnectivity)

			It("Verify EgressIP connectivity over IPv4 address after graceful reboot",
				Label("egressip", "egressip-ipv4"), reportxml.ID("75062"),
				rdscorecommon.VerifyEgressIPConnectivityThreeNodesIPv4)

			It("Verify EgressIP connectivity over IPv6 address after graceful reboot",
				Label("egressip", "egressip-ipv6"), reportxml.ID("78138"),
				rdscorecommon.VerifyEgressIPConnectivityThreeNodesIPv6)

			It("Verifies NUMA-aware workload is available after soft reboot",
				Label("nrop"), reportxml.ID("73726"),
				rdscorecommon.VerifyNROPWorkloadAvailable)

			It("Verifies CephFS PVC is still accessible",
				Label("persistent-storage", "verify-cephfs"), reportxml.ID("72042"),
				rdscorecommon.VerifyDataOnCephFSPVC)

			It("Verifies CephRBD PVC is still accessible",
				Label("persistent-storage", "verify-cephrbd"), reportxml.ID("72044"),
				rdscorecommon.VerifyDataOnCephRBDPVC)

			It("Verifies CephFS workload is deployable after graceful reboot",
				Label("persistent-storage", "deploy-cephfs-pvc"), reportxml.ID("72045"), MustPassRepeatedly(3),
				rdscorecommon.VerifyCephFSPVC)

			It("Verifies CephRBD workload is deployable after graceful reboot",
				Label("persistent-storage", "deploy-cephrbd-pvc"), reportxml.ID("72046"), MustPassRepeatedly(3),
				rdscorecommon.VerifyCephRBDPVC)

			It("Verifies SR-IOV workloads on different nodes post graceful reboot",
				Label("sriov", "verify-sriov-different-node"), reportxml.ID("72039"),
				rdscorecommon.VerifySRIOVConnectivityBetweenDifferentNodes)

			It("Verifies SR-IOV workloads on the same node post graceful reboot",
				Label("sriov", "verify-sriov-same-node"), reportxml.ID("72038"),
				rdscorecommon.VerifySRIOVConnectivityOnSameNode)

			It("Verifies SR-IOV workloads deployable on the same node after graceful reboot",
				Label("sriov", "deploy-sriov-same-node"), reportxml.ID("72048"), MustPassRepeatedly(3),
				rdscorecommon.VerifySRIOVWorkloadsOnSameNode)

			It("Verifies SR-IOV workloads deployable on different nodes after graceful reboot",
				Label("sriov", "deploy-sriov-different-node"), reportxml.ID("72049"), MustPassRepeatedly(3),
				rdscorecommon.VerifySRIOVWorkloadsOnDifferentNodes)

			It("Verifies SR-IOV workloads on the same node and different SR-IOV nets after graceful reboot",
				Label("sriov", "verify-sriov-same-node-diff-nets"), reportxml.ID("72260"),
				rdscorecommon.VerifySRIOVConnectivityOnSameNodeAndDifferentNets)

			It("Verifies SR-IOV workloads on different nodes and different SR-IOV nets after graceful reboot",
				Label("sriov", "verify-sriov-diff-nodes-diff-nets"), reportxml.ID("72261"),
				rdscorecommon.VerifySRIOVConnectivityOnDifferentNodesAndDifferentNetworks)

			It("Verifies MACVLAN workloads on the same node post soft reboot",
				Label("macvlan", "verify-macvlan-same-node"), reportxml.ID("72571"),
				rdscorecommon.VerifyMACVLANConnectivityOnSameNode)

			It("Verifies MACVLAN workloads on different nodes post soft reboot",
				Label("macvlan", "verify-macvlan-different-nodes"), reportxml.ID("72570"),
				rdscorecommon.VerifyMACVLANConnectivityBetweenDifferentNodes)

			It("Verifies IPVLAN workloads on the same node post soft reboot",
				Label("ipvlan", "verify-ipvlan-same-node"), reportxml.ID("75564"),
				rdscorecommon.VerifyIPVLANConnectivityOnSameNode)

			It("Verifies IPVLAN workloads on different nodes post soft reboot",
				Label("ipvlan", "verify-ipvlan-different-nodes"), reportxml.ID("75058"),
				rdscorecommon.VerifyIPVLANConnectivityBetweenDifferentNodes)

			It("Verifies workload reachable over BGP route post soft reboot",
				Label("frr"), reportxml.ID("76011"),
				rdscorecommon.ReachURLviaFRRroute)
		})
	})
