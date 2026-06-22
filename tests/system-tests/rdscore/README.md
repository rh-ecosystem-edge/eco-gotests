# RDS Core System Tests

Documentaion of parameters to be set for running this test suite.

## Environment Variables

All environment variables are defined in
`tests/system-tests/rdscore/internal/rdscoreconfig/config.go`.
See `default.yaml` in the same directory for default values.

| Variable | Description |
|----------|-------------|
| `ECO_RDSCORE_POLICY_NS` | Policy namespace |
| `ECO_RDSCORE_WLKD_SRIOV_ONE_NS` | Workload SR-IOV one namespace |
| `ECO_RDSCORE_WLKD_SRIOV_TWO_NS` | Workload SR-IOV two namespace |
| `ECO_RDSCORE_WLKD_SRIOV_3_NS` | Workload SR-IOV 3 namespace |
| `ECO_RDSCORE_WLKD_SRIOV_4_NS` | Workload SR-IOV 4 namespace |
| `ECO_RDSCORE_WLKD_NROP_ONE_NS` | Workload NROP one namespace |
| `ECO_RDSCORE_WLKD_NROP_TWO_NS` | Workload NROP two namespace |
| `ECO_RDSCORE_MCVLAN_NS_ONE` | MAC VLAN namespace one |
| `ECO_RDSCORE_MCVLAN_NS_TWO` | MAC VLAN namespace two |
| `ECO_SYSTEM_RDSCORE_DEPLOY_IMG_ONE` | MAC VLAN / IP VLAN deploy image one |
| `ECO_SYSTEM_RDSCORE_MCVLAN_NAD_ONE_NAME` | MAC VLAN NAD one name |
| `ECO_RDSCORE_KDUMP_CP_NODE_LABEL` | Kdump control plane node label |
| `ECO_RDSCORE_KDUMP_CNF_NODE_LABEL` | Kdump CNF MCP node label |
| `ECO_RDSCORE_IPVLAN_NS_ONE` | IP VLAN namespace one |
| `ECO_RDSCORE_IPVLAN_NS_TWO` | IP VLAN namespace two |
| `ECO_SYSTEM_RDSCORE_IPVLAN_NAD_ONE_NAME` | IP VLAN NAD one name |
| `ECO_SYSTEM_RDSCORE_IPVLAN_NAD_TWO_NAME` | IP VLAN NAD two name |
| `ECO_SYSTEM_RDSCORE_IPVLAN_NAD_THREE_NAME` | IP VLAN NAD three name |
| `ECO_SYSTEM_RDSCORE_IPVLAN_NAD_FOUR_NAME` | IP VLAN NAD four name |
| `ECO_RDSCORE_GRACEFUL_RESTART_APP_LABEL` | Graceful restart app label |
| `ECO_RDSCORE_KDUMP_WORKER_NODE_LABEL` | Kdump worker MCP node label |
| `ECO_RDS_CORE_PERFORMANCE_PROFILE_HT_NAME` | Performance profile HT name |
| `ECO_RDSCORE_NMI_REDFISH_CP_NODE_LABEL` | NMI Redfish control plane node label |
| `ECO_RDSCORE_NMI_REDFISH_WORKER_NODE_LABEL` | NMI Redfish worker MCP node label |
| `ECO_RDSCORE_NMI_REDFISH_CNF_NODE_LABEL` | NMI Redfish CNF MCP node label |
| `ECO_RDSCORE_TOLERATIONS_LIST` | Workload toleration list |
| `ECO_RDSCORE_NROP_TOLERATIONS_LIST` | Workload NROP toleration list |
| `ECO_SYSTEM_RDSCORE_MCVLAN_CM_DATA_ONE` | MAC VLAN ConfigMap data one |
| `ECO_SYSTEM_RDSCORE_IPVLAN_CM_DATA_ONE` | IP VLAN ConfigMap data one |
| `ECO_RDSCORE_STORAGE_WLKD_IMAGE` | Storage ODF workload image |
| `ECO_RDSCORE_NODES_CREDENTIALS_MAP` | Nodes BMC credentials map |
| `ECO_RDSCORE_WLKD_SRIOV_ONE_IMG` | Workload SR-IOV deploy one image |
| `ECO_RDSCORE_WLKD_SRIOV_TWO_IMG` | Workload SR-IOV deploy two image |
| `ECO_RDSCORE_WLKD_SRIOV_3_IMG` | Workload SR-IOV deploy 3 image |
| `ECO_RDSCORE_WLKD_SRIOV_4_IMG` | Workload SR-IOV deploy 4 image |
| `ECO_RDSCORE_WLKD_NROP_ONE_IMG` | Workload NROP deploy one image |
| `ECO_RDSCORE_WLKD_NROP_TWO_IMG` | Workload NROP deploy two image |
| `ECO_RDSCORE_WLKD_SRIOV_NET_ONE` | Workload SR-IOV network one |
| `ECO_RDSCORE_WLKD_SRIOV_NET_TWO` | Workload SR-IOV network two |
| `ECO_RDSCORE_WLKD_SRIOV_NET_21` | Workload SR-IOV network 21 |
| `ECO_RDSCORE_WLKD_SRIOV_NET_22` | Workload SR-IOV network 22 |
| `ECO_RDSCORE_WLKD_SRIOV_NET_31` | Workload SR-IOV network 31 |
| `ECO_RDSCORE_WLKD_SRIOV_NET_32` | Workload SR-IOV network 32 |
| `ECO_RDSCORE_WLKD_SRIOV_NET_41` | Workload SR-IOV network 41 |
| `ECO_RDSCORE_WLKD_SRIOV_NET_42` | Workload SR-IOV network 42 |
| `ECO_RDSCORE_WLKD_SRIOV_TWO_SA` | Workload SR-IOV two service account |
| `ECO_RDSCORE_NROP_SCHEDULER_NAME` | NROP scheduler name |
| `ECO_RDSCORE_METALLB_FRR_TEST_URL_IPV4` | MetalLB FRR test URL IPv4 |
| `ECO_RDSCORE_METALLB_FRR_NAMESPACE` | MetalLB FRR namespace |
| `ECO_RDSCORE_METALLB_FRR_ONE_IPV4` | MetalLB FRR one IPv4 |
| `ECO_RDSCORE_METALLB_FRR_ONEIPV6` | MetalLB FRR one IPv6 |
| `ECO_RDSCORE_METALLB_FRR_TWO_IPV4` | MetalLB FRR two IPv4 |
| `ECO_RDSCORE_METALLB_FRR_TWO_IPV6` | MetalLB FRR two IPv6 |
| `ECO_RDSCORE_METALLB_FRR_TEST_URL_IPV6` | MetalLB FRR test URL IPv6 |
| `ECO_RDSCORE_METALLB_TRAFFIC_SEGREGATION_TARGET_PORT` | MetalLB traffic segregation target port |
| `ECO_RDSCORE_METALLB_TRAFFIC_SEG_TARGET_ONE_IPV4` | MetalLB traffic segregation target one IPv4 |
| `ECO_RDSCORE_METALLB_TRAFFIC_SEG_TARGET_ONE_IPV6` | MetalLB traffic segregation target one IPv6 |
| `ECO_RDSCORE_METALLB_TRAFFIC_SEG_TARGET_TWO_IPV4` | MetalLB traffic segregation target two IPv4 |
| `ECO_RDSCORE_METALLB_TRAFFIC_SEG_TARGET_TWO_IPV6` | MetalLB traffic segregation target two IPv6 |
| `ECO_RDSCORE_METALLB_LB_ONE_NAMESPACE` | MetalLB load balancer one namespace |
| `ECO_RDSCORE_METALLB_LB_TWO_NAMESPACE` | MetalLB load balancer two namespace |
| `ECO_RDSCORE_METALLB_SUPPORT_TOOLS_IMAGE` | MetalLB support tools image |
| `ECO_RDSCORE_METALLB_TRAFFIC_SEG_TCPDUMP_INT_ONE` | MetalLB traffic segregation tcpdump interface one |
| `ECO_RDSCORE_METALLB_TRAFFIC_SEG_TCPDUMP_INT_TWO` | MetalLB traffic segregation tcpdump interface two |
| `ECO_RDSCORE_METALLB_FRR_CONTAINER_NAME_ONE` | MetalLB FRR container name one |
| `ECO_RDSCORE_METALLB_FRR_CONTAINER_NAME_TWO` | MetalLB FRR container name two |
| `ECO_RDSCORE_METALLB_LB_ONE_IPV4` | MetalLB load balancer one IPv4 |
| `ECO_RDSCORE_METALLB_LB_ONE_IPV6` | MetalLB load balancer one IPv6 |
| `ECO_RDSCORE_METALLB_LB_TWO_IPV4` | MetalLB load balancer two IPv4 |
| `ECO_RDSCORE_METALLB_LB_TWO_IPV6` | MetalLB load balancer two IPv6 |
| `ECO_SYSTEM_TEST_HYPERVISOR_HOST` | Hypervisor host |
| `ECO_SYSTEM_TEST_HYPERVISOR_USER` | Hypervisor user |
| `ECO_SYSTEM_TEST_HYPERVISOR_PASS` | Hypervisor password |
| `ECO_RDSCORE_SRIOV_CM_DATA_ONE` | Workload SR-IOV ConfigMap data one |
| `ECO_RDSCORE_SRIOV_CM_DATA_TWO` | Workload SR-IOV ConfigMap data two |
| `ECO_RDSCORE_SRIOV_CM_DATA_3` | Workload SR-IOV ConfigMap data 3 |
| `ECO_RDSCORE_SRIOV_CM_DATA_4` | Workload SR-IOV ConfigMap data 4 |
| `ECO_RDSCORE_WLKD_ODF_ONE_SELECTOR` | Storage ODF deploy one selector |
| `ECO_RDSCORE_WLKD_ODF_TWO_SELECTOR` | Storage ODF deploy two selector |
| `ECO_RDSCORE_WLKD_NROP_ONE_SELECTOR` | Workload NROP deploy one selector |
| `ECO_RDSCORE_WLKD_SRIOV_ONE_SELECTOR` | Workload SR-IOV deploy one selector |
| `ECO_RDSCORE_WLKD_SRIOV_TWO_SELECTOR` | Workload SR-IOV deploy two selector |
| `ECO_RDSCORE_WLKD_SRIOV_3_0_SELECTOR` | Workload SR-IOV deploy 3 one selector |
| `ECO_RDSCORE_WLKD_SRIOV_4_0_SELECTOR` | Workload SR-IOV deploy 4 one selector |
| `ECO_RDSCORE_WLKD_SRIOV_4_1_SELECTOR` | Workload SR-IOV deploy 4 two selector |
| `ECO_RDSCORE_WLKD_SRIOV_3_1_SELECTOR` | Workload SR-IOV deploy 3 two selector |
| `ECO_RDSCORE_WLKD_NROP_ONE_RES_REQUESTS` | Workload NROP deploy one resource requests |
| `ECO_RDSCORE_WLKD_SRIOV_ONE_RES_REQUESTS` | Workload SR-IOV deploy one resource requests |
| `ECO_RDSCORE_WLKD_SRIOV_TWO_RES_REQUESTS` | Workload SR-IOV deploy two resource requests |
| `ECO_RDSCORE_WLKD_SRIOV_3_0_RES_REQUESTS` | Workload SR-IOV deploy 3 one resource requests |
| `ECO_RDSCORE_WLKD_SRIOV_3_1_RES_REQUESTS` | Workload SR-IOV deploy 3 two resource requests |
| `ECO_RDSCORE_WLKD_SRIOV_4_0_RES_REQUESTS` | Workload SR-IOV deploy 4 one resource requests |
| `ECO_RDSCORE_WLKD_SRIOV_4_1_RES_REQUESTS` | Workload SR-IOV deploy 4 two resource requests |
| `ECO_RDSCORE_WLKD_NROP_ONE_RES_LIMITS` | Workload NROP deploy one resource limits |
| `ECO_RDSCORE_WLKD_SRIOV_ONE_RES_LIMITS` | Workload SR-IOV deploy one resource limits |
| `ECO_RDSCORE_WLKD_SRIOV_TWO_RES_LIMITS` | Workload SR-IOV deploy two resource limits |
| `ECO_RDSCORE_WLKD_SRIOV_3_0_RES_LIMITS` | Workload SR-IOV deploy 3 one resource limits |
| `ECO_RDSCORE_WLKD_SRIOV_3_1_RES_LIMITS` | Workload SR-IOV deploy 3 two resource limits |
| `ECO_RDSCORE_WLKD_SRIOV_4_0_RES_LIMITS` | Workload SR-IOV deploy 4 one resource limits |
| `ECO_RDSCORE_WLKD_SRIOV_4_1_RES_LIMITS` | Workload SR-IOV deploy 4 two resource limits |
| `ECO_RDSCORE_NODE_SELECTOR_HT_NODES` | Node selector for HT nodes |
| `ECO_RDSCORE_SRIOV_WLKD_DEPLOY_ONE_TARGET` | Workload SR-IOV deploy one target address |
| `ECO_RDSCORE_SRIOV_WLKD_DEPLOY_ONE_TARGET_IPV6` | Workload SR-IOV deploy one target address IPv6 |
| `ECO_RDSCORE_SRIOV_WLKD_DEPLOY_3_ONE_TARGET` | Workload SR-IOV deploy 3 one target address |
| `ECO_RDSCORE_SRIOV_WLKD_DEPLOY_3_ONE_TARGET_IPV6` | Workload SR-IOV deploy 3 one target address IPv6 |
| `ECO_RDSCORE_SRIOV_WLKD_DEPLOY_4_ONE_TARGET` | Workload SR-IOV deploy 4 one target address |
| `ECO_RDSCORE_SRIOV_WLKD_DEPLOY_4_ONE_TARGET_IPV6` | Workload SR-IOV deploy 4 one target address IPv6 |
| `ECO_RDSCORE_SRIOV_WLKD_DEPLOY_TWO_TARGET` | Workload SR-IOV deploy two target address |
| `ECO_RDSCORE_SRIOV_WLKD_DEPLOY_TWO_TARGET_IPV6` | Workload SR-IOV deploy two target address IPv6 |
| `ECO_RDSCORE_SRIOV_WLKD_3_DEPLOY_TWO_TARGET` | Workload SR-IOV deploy 3 two target address |
| `ECO_RDSCORE_SRIOV_WLKD_3_DEPLOY_TWO_TARGET_IPV6` | Workload SR-IOV deploy 3 two target address IPv6 |
| `ECO_RDSCORE_SRIOV_WLKD_4_DEPLOY_TWO_TARGET` | Workload SR-IOV deploy 4 two target address |
| `ECO_RDSCORE_SRIOV_WLKD_4_DEPLOY_TWO_TARGET_IPV6` | Workload SR-IOV deploy 4 two target address IPv6 |
| `ECO_RDSCORE_SRIOV_WLKD2_DEPLOY_ONE_TARGET` | Workload SR-IOV deploy 2 one target address |
| `ECO_RDSCORE_SRIOV_WLKD2_DEPLOY_ONE_TARGET_IPV6` | Workload SR-IOV deploy 2 one target address IPv6 |
| `ECO_RDSCORE_SRIOV_WLKD2_DEPLOY_TWO_TARGET` | Workload SR-IOV deploy 2 two target address |
| `ECO_RDSCORE_SRIOV_WLKD2_DEPLOY_TWO_TARGET_IPV6` | Workload SR-IOV deploy 2 two target address IPv6 |
| `ECO_SYSTEM_RDSCORE_MCVLAN_1_NODE_SELECTOR` | MAC VLAN deploy node selector one |
| `ECO_SYSTEM_RDSCORE_MCVLAN_2_NODE_SELECTOR` | MAC VLAN deploy node selector two |
| `ECO_SYSTEM_RDSCORE_MACVLAN_DEPLOY_ONE_TARGET` | MAC VLAN deploy 1 target address |
| `ECO_SYSTEM_RDSCORE_MACVLAN_DEPLOY_ONE_TARGET_IPV6` | MAC VLAN deploy 1 target address IPv6 |
| `ECO_SYSTEM_RDSCORE_MACVLAN_DEPLOY_TWO_TARGET` | MAC VLAN deploy 2 target address |
| `ECO_SYSTEM_RDSCORE_MACVLAN_DEPLOY_TWO_TARGET_IPV6` | MAC VLAN deploy 2 target address IPv6 |
| `ECO_SYSTEM_RDSCORE_MACVLAN_DEPLOY_3_TARGET` | MAC VLAN deploy 3 target address |
| `ECO_SYSTEM_RDSCORE_MACVLAN_DEPLOY_3_TARGET_IPV6` | MAC VLAN deploy 3 target address IPv6 |
| `ECO_SYSTEM_RDSCORE_MACVLAN_DEPLOY_4_TARGET` | MAC VLAN deploy 4 target address |
| `ECO_SYSTEM_RDSCORE_MACVLAN_DEPLOY_4_TARGET_IPV6` | MAC VLAN deploy 4 target address IPv6 |
| `ECO_SYSTEM_RDSCORE_IPVLAN_1_NODE_SELECTOR` | IP VLAN deploy node selector one |
| `ECO_SYSTEM_RDSCORE_IPVLAN_2_NODE_SELECTOR` | IP VLAN deploy node selector two |
| `ECO_SYSTEM_RDSCORE_IPVLAN_DEPLOY_ONE_TARGET` | IP VLAN deploy 1 target address |
| `ECO_SYSTEM_RDSCORE_IPVLAN_DEPLOY_ONE_TARGET_IPV6` | IP VLAN deploy 1 target address IPv6 |
| `ECO_SYSTEM_RDSCORE_IPVLAN_DEPLOY_TWO_TARGET` | IP VLAN deploy 2 target address |
| `ECO_SYSTEM_RDSCORE_IPVLAN_DEPLOY_TWO_TARGET_IPV6` | IP VLAN deploy 2 target address IPv6 |
| `ECO_SYSTEM_RDSCORE_IPVLAN_DEPLOY_3_TARGET` | IP VLAN deploy 3 target address |
| `ECO_SYSTEM_RDSCORE_IPVLAN_DEPLOY_3_TARGET_IPV6` | IP VLAN deploy 3 target address IPv6 |
| `ECO_SYSTEM_RDSCORE_IPVLAN_DEPLOY_4_TARGET` | IP VLAN deploy 4 target address |
| `ECO_SYSTEM_RDSCORE_IPVLAN_DEPLOY_4_TARGET_IPV6` | IP VLAN deploy 4 target address IPv6 |
| `ECO_RDSCORE_WLKD_NROP_ONE_CMD` | Workload NROP deploy one command |
| `ECO_RDSCORE_WLKD_SRIOV_ONE_CMD` | Workload SR-IOV deploy one command |
| `ECO_RDSCORE_WLKD_SRIOV_TWO_CMD` | Workload SR-IOV deploy two command |
| `ECO_RDSCORE_WLKD_SRIOV_2_ONE_CMD` | Workload SR-IOV deploy 2 one command |
| `ECO_RDSCORE_WLKD_SRIOV_2_TWO_CMD` | Workload SR-IOV deploy 2 two command |
| `ECO_RDSCORE_WLKD_SRIOV_3_ONE_CMD` | Workload SR-IOV deploy 3 one command |
| `ECO_RDSCORE_WLKD_SRIOV_3_TWO_CMD` | Workload SR-IOV deploy 3 two command |
| `ECO_RDSCORE_WLKD_SRIOV_4_ONE_CMD` | Workload SR-IOV deploy 4 one command |
| `ECO_RDSCORE_WLKD_SRIOV_4_TWO_CMD` | Workload SR-IOV deploy 4 two command |
| `ECO_SYSTEM_RDSCORE_MCVLAN_DEPLOY_1_CMD` | MAC VLAN deploy one command |
| `ECO_SYSTEM_RDSCORE_MCVLAN_DEPLOY_2_CMD` | MAC VLAN deploy two command |
| `ECO_SYSTEM_RDSCORE_MCVLAN_DEPLOY_3_CMD` | MAC VLAN deploy 3 command |
| `ECO_SYSTEM_RDSCORE_MCVLAN_DEPLOY_4_CMD` | MAC VLAN deploy 4 command |
| `ECO_SYSTEM_RDSCORE_IPVLAN_DEPLOY_1_CMD` | IP VLAN deploy one command |
| `ECO_SYSTEM_RDSCORE_IPVLAN_DEPLOY_2_CMD` | IP VLAN deploy two command |
| `ECO_SYSTEM_RDSCORE_IPVLAN_DEPLOY_3_CMD` | IP VLAN deploy 3 command |
| `ECO_SYSTEM_RDSCORE_IPVLAN_DEPLOY_4_CMD` | IP VLAN deploy 4 command |
| `ECO_RDSCORE_SC_CEPHFS_NAME` | Storage CephFS storage class name |
| `ECO_RDSCORE_SC_CEPHRBD_NAME` | Storage CephRBD storage class name |
| `ECO_RDSCORE_EGRESS_SERVICE_NS` | Egress service namespace |
| `ECO_RDSCORE_GRACEFUL_RESTART_SERVICE_NS` | Graceful restart service namespace |
| `ECO_RDSCORE_GRACEFUL_RESTART_SERVICE_NAME` | Graceful restart service name |
| `ECO_RDSCORE_GRACEFUL_RESTART_SERVICE_PORT` | Graceful restart app service port |
| `ECO_RDSCORE_EGRESS_SERVICE_REMOTE_IP` | Egress service remote IP |
| `ECO_RDSCORE_EGRESS_SERVICE_REMOTE_IPV6` | Egress service remote IPv6 |
| `ECO_RDSCORE_EGRESS_SERVICE_REMOTE_PORT` | Egress service remote port |
| `ECO_RDSCORE_EGRESS_SERVICE_DEPLOY_1_CMD` | Egress service deploy 1 command |
| `ECO_RDSCORE_EGRESS_SVC_DEPLOY_1_IMG` | Egress service deploy 1 image |
| `ECO_RDSCORE_EGRESS_SVC_VRF_1_NET` | Egress service VRF 1 network |
| `ECO_RDSCORE_EGRESS_SVC_DEPLOY_1_IPADDR_POOL` | Egress service deploy 1 IP address pool |
| `ECO_RDSCORE_EGRESS_SVC_1_NODE_SELECTOR` | Egress service deploy 1 node selector |
| `ECO_RDSCORE_EGRESS_SERVICE_DEPLOY_2_CMD` | Egress service deploy 2 command |
| `ECO_RDSCORE_EGRESS_SVC_DEPLOY_2_IMG` | Egress service deploy 2 image |
| `ECO_RDSCORE_EGRESS_SVC_VRF_2_NET` | Egress service VRF 2 network |
| `ECO_RDSCORE_EGRESS_SVC_2_NODE_SELECTOR` | Egress service deploy 2 node selector |
| `ECO_RDSCORE_EGRESS_SVC_DEPLOY_2_IPADDR_POOL` | Egress service deploy 2 IP address pool |
| `ECO_RDSCORE_EGRESS_SERVICE_DEPLOY_3_CMD` | Egress service deploy 3 command |
| `ECO_RDSCORE_EGRESS_SVC_DEPLOY_3_IMG` | Egress service deploy 3 image |
| `ECO_RDSCORE_EGRESS_SVC_VRF_3_NET` | Egress service VRF 3 network |
| `ECO_RDSCORE_EGRESS_SVC_DEPLOY_3_IPADDR_POOL` | Egress service deploy 3 IP address pool |
| `ECO_RDSCORE_EGRESS_SVC_3_NODE_SELECTOR` | Egress service deploy 3 node selector |
| `ECO_RDSCORE_EGRESS_SERVICE_DEPLOY_4_CMD` | Egress service deploy 4 command |
| `ECO_RDSCORE_EGRESS_SVC_DEPLOY_4_IMG` | Egress service deploy 4 image |
| `ECO_RDSCORE_EGRESS_SVC_VRF_4_NET` | Egress service VRF 4 network |
| `ECO_RDSCORE_EGRESS_SVC_DEPLOY_4_IPADDR_POOL` | Egress service deploy 4 IP address pool |
| `ECO_RDSCORE_EGRESS_SVC_4_NODE_SELECTOR` | Egress service deploy 4 node selector |
| `ECO_RDSCORE_EGRESS_SVC_NETWORK_EXPECTED_IPS` | Egress service network expected IPs |
| `ECO_RDSCORE_EGRESSIP_NAME` | Egress IP name |
| `ECO_RDSCORE_EGRESSIP_DEPLOY_IMG` | Egress IP deployment image |
| `ECO_SYSTEM_RDSCORE_EGRESSIP_NODE_ONE` | Egress IP node one |
| `ECO_SYSTEM_RDSCORE_EGRESSIP_NODE_TWO` | Egress IP node two |
| `ECO_SYSTEM_RDSCORE_EGRESSIP_NODE_THREE` | Egress IP node three |
| `ECO_RDSCORE_EGRESSIP_TCP_PORT_NUMBER` | Egress IP TCP port |
| `ECO_SYSTEM_RDSCORE_NON_EGRESSIP_NODE` | Non egress IP node |
| `ECO_RDSCORE_EGRESSIP_NS_LABEL` | Egress IP namespace label |
| `ECO_RDSCORE_EGRESSIP_POD_LABEL` | Egress IP pod label |
| `ECO_RDSCORE_EGRESSIP_NS_ONE` | Egress IP namespace one |
| `ECO_RDSCORE_EGRESSIP_NS_TWO` | Egress IP namespace two |
| `ECO_RDSCORE_EGRESSIP_IPV4` | Egress IPv4 address |
| `ECO_RDSCORE_EGRESSIP_IPV6` | Egress IPv6 address |
| `ECO_RDSCORE_EGRESSIP_REMOTE_IPV4` | Egress IP remote IPv4 |
| `ECO_RDSCORE_EGRESSIP_REMOTE_IPV6` | Egress IP remote IPv6 |
| `ECO_RDSCORE_POD_LEVEL_BOND_PORT` | Pod level bond port |
| `ECO_RDSCORE_POD_LEVEL_BOND_NS` | Pod level bond namespace |
| `ECO_RDSCORE_POD_LEVEL_BOND_IMG` | Pod level bond deploy image |
| `ECO_RDSCORE_POD_LEVEL_BOND_ONE_NAME` | Pod level bond deployment one name |
| `ECO_RDSCORE_POD_LEVEL_BOND_TWO_NAME` | Pod level bond deployment two name |
| `ECO_RDSCORE_POD_LEVEL_BOND_ONE_IPV4` | Pod level bond deployment one IPv4 |
| `ECO_RDSCORE_POD_LEVEL_BOND_ONE_IPV6` | Pod level bond deployment one IPv6 |
| `ECO_RDSCORE_POD_LEVEL_BOND_TWO_IPV4` | Pod level bond deployment two IPv4 |
| `ECO_RDSCORE_POD_LEVEL_BOND_TWO_IPV6` | Pod level bond deployment two IPv6 |
| `ECO_RDSCORE_POD_LEVEL_BOND_POD_ONE_NODE` | Pod level bond pod one schedule on host |
| `ECO_RDSCORE_POD_LEVEL_BOND_POD_TWO_NODE` | Pod level bond pod two schedule on host |
| `ECO_RDSCORE_POD_LEVEL_BOND_SRIOV_NET_ONE` | Pod level bond SR-IOV network one |
| `ECO_RDSCORE_POD_LEVEL_BOND_SRIOV_NET_TWO` | Pod level bond SR-IOV network two |
| `ECO_RDSCORE_POD_LEVEL_BOND_POD_SUBNET_MASK_IPV4` | Pod level bond pod subnet mask IPv4 |
| `ECO_RDSCORE_POD_LEVEL_BOND_POD_SUBNET_MASK_IPV6` | Pod level bond pod subnet mask IPv6 |
| `ECO_RDSCORE_POD_LEVEL_BOND_POD_MAC_ADDR` | Pod level bond pod MAC address |
| `ECO_RDSCORE_FRR_EXPECTED_NODES` | FRR expected nodes |
| `ECO_RDSCORE_ROOTLESS_DPDK_NS` | Rootless DPDK namespace |
| `ECO_RDSCORE_ROOTLESS_DPDK_CLIENT_DEPLOYMENT_NAME` | Rootless DPDK client deployment name |
| `ECO_RDSCORE_ROOTLESS_DPDK_NETWORK_ONE` | Rootless DPDK network one |
| `ECO_RDSCORE_ROOTLESS_DPDK_NETWORK_TWO` | Rootless DPDK network two |
| `ECO_RDSCORE_ROOTLESS_DPDK_VLAN_ID` | Rootless DPDK VLAN ID |
| `ECO_RDSCORE_ROOTLESS_DPDK_DUMMY_VLAN_ID` | Rootless DPDK dummy VLAN ID |
| `ECO_RDSCORE_ROOTLESS_DPDK_NODE_ONE` | Rootless DPDK node one |
| `ECO_RDSCORE_ROOTLESS_DPDK_NODE_TWO` | Rootless DPDK node two |
| `ECO_RDSCORE_ROOTLESS_DPDK_DEPLOYMENT_SA` | Rootless DPDK deployment service account |
| `ECO_RDSCORE_ROOTLESS_DPDK_POLICY_TWO` | Rootless DPDK policy two |
| `ECO_RDSCORE_ROOTLESS_DPDK_CLIENT_VLAN_MAC` | Rootless DPDK client VLAN MAC |
| `ECO_RDSCORE_ROOTLESS_DPDK_CLIENT_MACVLAN_MAC` | Rootless DPDK client MAC VLAN MAC |
| `ECO_RDSCORE_ROOTLESS_DPDK_CLIENT_IPVLAN_MAC` | Rootless DPDK client IP VLAN MAC |
| `ECO_RDSCORE_ROOTLESS_DPDK_CLIENT_IPVLAN_IPV4` | Rootless DPDK client IP VLAN IPv4 |
| `ECO_RDSCORE_ROOTLESS_DPDK_CLIENT_IPVLAN_IPV4_DUMMY` | Rootless DPDK client IP VLAN IPv4 dummy |
| `ECO_RDSCORE_DPDK_TEST_CONTAINER` | DPDK test container |
| `ECO_RDSCORE_KAFKA_LOGS_LABEL` | Kafka logs label |
| `ECO_RDSCORE_WHEREABOUT_NS` | Whereabout namespace |
| `ECO_RDSCORE_WHEREABOUTS_ST_IMAGE_ONE` | Whereabouts statefulset image one |
| `ECO_RDSCORE_WHEREABOUTS_ST_IMAGE_TWO` | Whereabouts statefulset image two |
| `ECO_RDSCORE_WHEREABOUTS_ST_ONE_CMD` | Whereabouts statefulset one command |
| `ECO_RDSCORE_WHEREABOUTS_ST_TWO_CMD` | Whereabouts statefulset two command |
| `ECO_RDSCORE_WHEREABOUTS_ST_ONE_PORT` | Whereabouts statefulset one port |
| `ECO_RDSCORE_WHEREABOUTS_ST_TWO_PORT` | Whereabouts statefulset two port |
| `ECO_RDSCORE_WHEREABOUTS_ST_ONE_NAD` | Whereabouts statefulset one NAD |
| `ECO_RDSCORE_WHEREABOUTS_ST_TWO_NAD` | Whereabouts statefulset two NAD |
| `ECO_RDSCORE_WHEREABOUTS_DEPLOY_IMAGE_ONE` | Whereabouts deploy image one |
| `ECO_RDSCORE_WHEREABOUTS_DEPLOY_IMAGE_TWO` | Whereabouts deploy image two |
| `ECO_RDSCORE_WHEREABOUTS_DEPLOY_ONE_CMD` | Whereabouts deploy one command |
| `ECO_RDSCORE_WHEREABOUTS_DEPLOY_TWO_CMD` | Whereabouts deploy two command |
| `ECO_RDSCORE_WHEREABOUTS_DEPLOY_ONE_PORT` | Whereabouts deploy one port |
| `ECO_RDSCORE_WHEREABOUTS_DEPLOY_TWO_PORT` | Whereabouts deploy two port |
| `ECO_RDSCORE_WHEREABOUTS_DEPLOY_ONE_NAD` | Whereabouts deploy one NAD |
| `ECO_RDSCORE_WHEREABOUTS_DEPLOY_TWO_NAD` | Whereabouts deploy two NAD |
| `ECO_RDSCORE_WHEREABOUTS_DEPLOY_IMAGE_3` | Whereabouts deploy image 3 |
| `ECO_RDSCORE_WHEREABOUTS_DEPLOY_IMAGE_4` | Whereabouts deploy image 4 |
| `ECO_RDSCORE_WHEREABOUTS_DEPLOY_3_CMD` | Whereabouts deploy 3 command |
| `ECO_RDSCORE_WHEREABOUTS_DEPLOY_4_CMD` | Whereabouts deploy 4 command |
| `ECO_RDSCORE_WHEREABOUTS_DEPLOY_3_PORT` | Whereabouts deploy 3 port |
| `ECO_RDSCORE_WHEREABOUTS_DEPLOY_4_PORT` | Whereabouts deploy 4 port |
| `ECO_RDSCORE_WHEREABOUTS_DEPLOY_3_NAD` | Whereabouts deploy 3 NAD |
| `ECO_RDSCORE_WHEREABOUTS_DEPLOY_4_NAD` | Whereabouts deploy 4 NAD |
| `ECO_RDSCORE_PYTHON_HTTP_SERVER_IMAGE` | Python HTTP server image |

### _VerifySRIOVWorkloadsOnSameNodeDifferentNet_

This test verifies connectivity between pods that use different SR-IOV networks and are scheduled
on the same node.

Test expects `nc` process to listen on IP address(es) on SR-IOV interfaces on both workloads,
for e.g. on `192.168.12.22 1111` on 1st workload and `192.168.12.33 1111` on 2nd workload.

Messages are sent between the workloads and asserted they are present in pods' logs.

**Requires 2 SR-IOV networks that have SR-IOV resources configured on the same node**


| parameter | description | example |
|-----------|-------------|---------|
|rdscore_wlkd_sriov_3_ns | Namespace where to deploy test workload | `my-ns-3` |
|rdscore_wlkd_sriov_cm_data_3 | Content of configMap that is mounted within a pod under `/opt/net/` | |
|rdscore_wlkd_sriov_3_image | Image used by the deployment | `quay.io/myorg/my-sriov-app:1.1` |
|rdscore_wlkd3_sriov_one_cmd | Command executed by 1st container | `["/bin/sh", "-c", "/myapp --run"]` |
|rdscore_wlkd_sriov_3_0_res_requests | Resource requests for 1st container(_Optional_) | `cpu: 1` |
|rdscore_wlkd_sriov_3_0_res_limits | Resource limits for 1st container(_Optional_) | `memory: 100M` |
|rdscore_wlkd3_sriov_two_cmd | Command executed by 2nd container | `["/bin/sh", "-c", "/myapp --run"]` |
|rdscore_wlkd_sriov_3_1_res_requests | Resource requests for 2nd container(_Optional_) | `cpu: 1` |
|rdscore_wlkd_sriov_3_1_res_limits | Resource limits for 2nd container(_Optional_) | `memory: 100M` |
|rdscore_wlkd_sriov_net_one | SR-IOV Network for 1st workload | `sriov-net-one` |
|rdscore_wlkd_sriov_3_0_selector | Node selector for both workloads | `kubernetes.io/hostname: worker-X` |
|rdscore_wlkd_sriov_net_two | SR-IOV Network for 2nd workload | `sriov-net-two` |
|rdscore_wlkd3_sriov_deploy_one_target | IPv4 address and port configured on 2nd workload | `192.168.12.22 1111` |
|rdscore_wlkd3_sriov_deploy_one_target_ipv6 | IPv6 address configured on 2nd workload(_Optional_) | |
|rdscore_wlkd3_sriov_deploy_two_target | IPv4 address and port configured on 1st workload | `192.168.12.12 1111` |
|rdscore_wlkd3_sriov_deploy_two_target_ipv6 | IPv6 address configured on 1st workload(_Optional_) | |


###  _VerifySRIOVWorkloadsOnDifferentNodesDifferentNet_

This test verifies connectivity between pods that use different SR-IOV networks and are scheduled
on different nodes.

Test expects `nc` process to listen on IP address(es) on SR-IOV interfaces on both workloads,
for e.g. on `192.168.12.22 1111` on 1st workload and `192.168.12.33 1111` on 2nd workload.

**Requires 2 nodes and 2 SR-IOV networks that have SR-IOV resources configured on the nodes**

| paremater | description | example |
|-----------|-------------|---------|
|rdscore_wlkd_sriov_4_ns | Namespace where to deploy test workload | `my-ns-4` |
|rdscore_wlkd_sriov_cm_data_4 | Content of configMap that is mounted within pods under `/opt/net/` | |
|rdscore_wlkd_sriov_4_image | Image used by the workloads | `quay.io/myorg/my-sriov-app:1.1` |
|rdscore_wlkd4_sriov_one_cmd | Command executed by 1st container | `["/bin/sh", "-c", "/myapp --run"]` |
|rdscore_wlkd_sriov_4_0_res_requests | Resource requests for 1st container(_Optional_) | `cpu: 1` |
|rdscore_wlkd_sriov_4_0_res_limits | Resource limits for 1st container(_Optional_) | `memory: 100M` |
|rdscore_wlkd4_sriov_two_cmd | Command executed by 2nd container | `["/bin/sh", "-c", "/myapp --run"]` |
|rdscore_wlkd_sriov_4_1_res_requests | Resource requests for 2nd container(_Optional_) | `cpu: 1` |
|rdscore_wlkd_sriov_4_1_res_limits | Resource limits for 2nd container(_Optional_) | `memory: 100M` |
|rdscore_wlkd_sriov_net_one | SR-IOV Network for 1st workload | `sriov-net-one` |
|rdscore_wlkd_sriov_4_0_selector | Node selector for 1st workload | `kubernetes.io/hostname: worker-X` |
|rdscore_wlkd_sriov_net_two | SR-IOV Network for 2nd workload | `sriov-net-two` |
|rdscore_wlkd_sriov_4_1_selector | Node selector for 2nd workload | `kubernetes.io/hostname: worker-Y` |
|rdscore_wlkd4_sriov_deploy_one_target | IPv4 address and port configured on 2nd workload | `192.168.12.22 1111` |
|rdscore_wlkd4_sriov_deploy_one_target_ipv6 | IPv6 address configured on 2nd workload(_Optional_) | |
|rdscore_wlkd4_sriov_deploy_two_target | IPv4 address and port configured on 1st workload | `192.168.12.12 1111` |
|rdscore_wlkd4_sriov_deploy_two_target_ipv6 | IPv6 address configured on 1st workload(_Optional_) | |


### _VerifySRIOVWorkloadsOnSameNode_

This test verifies connectivity between pods that use same SR-IOV networks and are scheduled
on the same node.

Test expects `nc` process to listen on IP address(es) on SR-IOV interfaces on both workloads,
for e.g. on `192.168.12.22 1111` on 1st workload and `192.168.12.33 1111` on 2nd workload.

**Requires 1 node and 1 SR-IOV network that has SR-IOV resources configured on the node**

| paremater | description | example |
|-----------|-------------|---------|
|rdscore_wlkd_sriov_one_ns | Namespace where to deploy test workload | `my-ns-1` |
|rdscore_wlkd_sriov_cm_data_one | Content of configMap that is mounted within pods under `/opt/net/` | |
|rdscore_wlkd_sriov_one_image | Image used by the 1st workload | `quay.io/myorg/my-sriov-app:1.1` |
|rdscore_wlkd_sriov_one_cmd | Command executed by 1st container | `["/bin/sh", "-c", "/myapp --run"]` |
|rdscore_wlkd_sriov_one_res_requests | Resource requests for 1st container(_Optional_) | `cpu: 1` |
|rdscore_wlkd_sriov_one_res_limits | Resource limits for 1st container(_Optional_) | `memory: 100M` |
|rdscore_wlkd_sriov_two_image | Image used by the 2nd workload | `quay.io/myorg/my-sriov-app:1.1` |
|rdscore_wlkd_sriov_two_cmd | Command executed by 2nd container | `["/bin/sh", "-c", "/myapp --run"]` |
|rdscore_wlkd_sriov_two_res_requests | Resource requests for 2nd container(_Optional_) | `cpu: 1` |
|rdscore_wlkd_sriov_two_res_limits | Resource limits for 2nd container(_Optional_) | `memory: 100M` |
|rdscore_wlkd_sriov_one_selector | Node selector for both workloads | `kubernetes.io/hostname: worker-X` |
|rdscore_wlkd_sriov_deploy_one_target | IPv4 address and port configured on 2nd workload | `192.168.12.22 1111` |
|rdscore_wlkd_sriov_deploy_one_target_ipv6 | IPv6 address configured on 2nd workload(_Optional_) | |
|rdscore_wlkd_sriov_deploy_two_target | IPv4 address and port configured on 1st workload | `192.168.12.12 1111` |
|rdscore_wlkd_sriov_deploy_two_target_ipv6 | IPv6 address configured on 1st workload(_Optional_) | |

### _VerifySRIOVWorkloadsOnDifferentNodes_

This test verifies connectivity between pods that use same SR-IOV networks and are scheduled
on the the different nodes.

Test expects `nc` process to listen on IP address(es) on SR-IOV interfaces on both workloads,
for e.g. on `192.168.12.22 1111` on 1st workload and `192.168.12.33 1111` on 2nd workload.

**Requires 2 node and 1 SR-IOV network that has SR-IOV resources configured on the nodes**

| paremater | description | example |
|-----------|-------------|---------|
|rdscore_wlkd_sriov_one_ns | Namespace where to deploy test workload | `my-ns-1` |
|rdscore_wlkd_sriov_cm_data_one | Content of configMap that is mounted within pods under `/opt/net/` | |
|rdscore_wlkd_sriov_one_image | Image used by the 1st workload | `quay.io/myorg/my-sriov-app:1.1` |
|rdscore_wlkd2_sriov_one_cmd | Command executed by 1st container | `["/bin/sh", "-c", "/myapp --run"]` |
|rdscore_wlkd_sriov_one_res_requests | Resource requests for both containers(_Optional_) | `cpu: 1` |
|rdscore_wlkd_sriov_one_res_limits | Resource limits for both containers(_Optional_) | `memory: 100M` |
|rdscore_wlkd_sriov_two_image | Image used by the 2nd workload | `quay.io/myorg/my-sriov-app:1.1` |
|rdscore_wlkd2_sriov_two_cmd | Command executed by 2nd container | `["/bin/sh", "-c", "/myapp --run"]` |
|rdscore_wlkd_sriov_one_selector | Node selector for 1st workload | `kubernetes.io/hostname: worker-X` |
|rdscore_wlkd_sriov_two_selector | Node selector for 2nd workload | `kubernetes.io/hostname: worker-Y` |
|rdscore_wlkd2_sriov_deploy_one_target | IPv4 address and port configured on 2nd workload | `192.168.12.22 1111` |
|rdscore_wlkd2_sriov_deploy_one_target_ipv6 | IPv6 address configured on 2nd workload(_Optional_) | |
|rdscore_wlkd2_sriov_deploy_two_target | IPv4 address and port configured on 1st workload | `192.168.12.12 1111` |
|rdscore_wlkd2_sriov_deploy_two_target_ipv6 | IPv6 address configured on 1st workload(_Optional_) | |

### _ValidateAllPoliciesCompliant_

Checks that all governance policies are Complaint

| paremater | description | example |
|-----------|-------------|---------|
|rdscore_policy_ns | Namespace where policies are created. If empty(_default_) check in all namespaces | `` |

### _VerifyNROPWorkload_

Test deploys a pod with NROP scheduler

| paremater | description | example |
|-----------|-------------|---------|
|rdscore_wlkd_nrop_one_ns | Namespace where to deploy test workload | `my-nrop-1` |
|rdscore_wlkd_nrop_one_image | Image used by the test workload | `quay.io/myorg/my-nrop-app:1.1` |
|rdscore_wlkd_nrop_one_cmd | Command executed by the container | `["/bin/sh", "-c", "/myapp --run"]` |
|rdscore_wlkd_nrop_one_res_requests | Resource requests for test container(_Optional_) | `cpu: 1` |
|rdscore_wlkd_nrop_one_res_limits | Resource limits for test container(_Optional_) | `memory: 100M` |
|rdscore_nrop_scheduler_name | Name of the NROP scheduler | `topo-aware-scheduler` |
|rdscore_wlkd_nrop_one_selector | Node selector for the test workload | `kubernetes.io/hostname: worker-X` |

### _VerifyMacVlanOnDifferentNodes_

Verifies connectivity between test workloads that use same MACVLAN definition and are scheduled on different nodes.

Test expects `nc` process to listen on IP address(es) on SR-IOV interfaces on both workloads,
for e.g. on `192.168.12.22 1111` on 1st workload and `192.168.12.33 1111` on 2nd workload.

**Requires 2 nodes where the same MACVLAN network is configured**

| paremater | description | example |
|-----------|-------------|---------|
|rdscore_mcvlan_ns_one | Namespace for the test workload | `my-mc-1` |
|rdscore_mcvlan_cm_data_one | Content of configMap that is mounted within pods | |
|rdscore_mcvlan_deploy_img_one | Image used by the test workload | `quay.io/myorg/my-mc-app:1.1` |
|rdscore_mcvlan_deploy_1_cmd | Command executed by 1st container | `["/bin/sh", "-c", "/myapp --run"]` |
|rdscore_mcvlan_deploy_2_cmd | Command executed by 2nd container | `["/bin/sh", "-c", "/myapp --run"]` |
|rdscore_mcvlan_nad_one_name | Name of the MacVlan network | `mcvlan-one` |
|rdscore_mcvlan_1_node_selector | Node selector for the 1st workload | `kubernetes.io/hostname: worker-X` |
|rdscore_mcvlan_2_node_selector | Node selector for the 1st workload | `kubernetes.io/hostname: worker-Y` |
|rdscore_macvlan_deploy_1_target | IPv4 address and port configured on 2nd workload | `192.168.12.22 1111` |
|rdscore_macvlan_deploy_1_target_ipv6 | IPv6 address configured on 2nd workload | |
|rdscore_macvlan_deploy_2_target | IPv4 address and port configured on 1st workload | `192.168.12.12 1111` |
|rdscore_macvlan_deploy_2_target_ipv6 | IPv6 address configured on 1st workload | |

### _VerifyMacVlanOnSameNode_

Verifies connectivity between test workloads that use same MACVLAN definition and are scheduled on the same node.

Test expects `nc` process to listen on IP address(es) on SR-IOV interfaces on both workloads,
for e.g. on `192.168.12.22 1111` on 1st workload and `192.168.12.33 1111` on 2nd workload.

**Requires 1 node where MACVLAN network is configured**

| paremater | description | example |
|-----------|-------------|---------|
|rdscore_mcvlan_ns_one | Namespace for the test workload | `my-mc-1` |
|rdscore_mcvlan_cm_data_one | Content of configMap that is mounted within pods | |
|rdscore_mcvlan_deploy_img_one | Image used by test workloads | `quay.io/myorg/my-mc-app:1.1` |
|rdscore_mcvlan_deploy_3_cmd | Command executed by 1st container | `["/bin/sh", "-c", "/myapp --run"]` |
|rdscore_mcvlan_deploy_4_cmd | Command executed by 2nd container | `["/bin/sh", "-c", "/myapp --run"]` |
|rdscore_mcvlan_nad_one_name | Name of the MacVlan network | `mcvlan-one` |
|rdscore_mcvlan_1_node_selector | Node selector for the 1st workload | `kubernetes.io/hostname: worker-X` |
|rdscore_macvlan_deploy_3_target | IPv4 address and port configured on 2nd workload | `192.168.12.22 1111` |
|rdscore_macvlan_deploy_3_target_ipv6 | IPv6 address configured on 2nd workload | |
|rdscore_macvlan_deploy_4_target | IPv4 address and port configured on 1st workload | `192.168.12.12 1111` |
|rdscore_macvlan_deploy_4_target_ipv6 | IPv6 address configured on 1st workload | |

### _VerifyIpVlanOnDifferentNodes_

Verifies connectivity between test workloads that use same IPVLAN definition and are scheduled on different nodes.

Test expects `nc` process to listen on IP address(es) on IPVLAN interfaces on both workloads,
for e.g. on `192.168.12.22 1111` on 1st workload and `192.168.12.33 1111` on 2nd workload.

**Requires 2 nodes where the same IPVLAN network is configured**

| paremater                           | description                                      | example                             |
|-------------------------------------|--------------------------------------------------|-------------------------------------|
| rdscore_ipvlan_ns_one               | Namespace for the test workload                  | `my-mc-1`                           |
| rdscore_ipvlan_cm_data_one          | Content of configMap that is mounted within pods |                                     |
| rdscore_ipvlan_deploy_img_one       | Image used by the test workload                  | `quay.io/myorg/my-mc-app:1.1`       |
| rdscore_ipvlan_deploy_1_cmd         | Command executed by 1st container                | `["/bin/sh", "-c", "/myapp --run"]` |
| rdscore_ipvlan_deploy_2_cmd         | Command executed by 2nd container                | `["/bin/sh", "-c", "/myapp --run"]` |
| rdscore_ipvlan_nad_one_name         | Name of the IpVlan network                       | `ip-vlan`                           |
| rdscore_ipvlan_1_node_selector      | Node selector for the 1st workload               | `kubernetes.io/hostname: worker-X`  |
| rdscore_ipvlan_2_node_selector      | Node selector for the 1st workload               | `kubernetes.io/hostname: worker-Y`  |
| rdscore_ipvlan_deploy_1_target      | IPv4 address and port configured on 2nd workload | `192.168.12.22 1111`                |
| rdscore_ipvlan_deploy_1_target_ipv6 | IPv6 address configured on 2nd workload          |                                     |
| rdscore_ipvlan_deploy_2_target      | IPv4 address and port configured on 1st workload | `192.168.12.12 1111`                |
| rdscore_ipvlan_deploy_2_target_ipv6 | IPv6 address configured on 1st workload          |                                     |

### _VerifyIpVlanOnSameNode_

Verifies connectivity between test workloads that use same IPVLAN definition and are scheduled on the same node.

Test expects `nc` process to listen on IP address(es) on IPVLAN interfaces on both workloads,
for e.g. on `192.168.12.22 1111` on 1st workload and `192.168.12.33 1111` on 2nd workload.

**Requires 1 node where IPVLAN network is configured**

| paremater                           | description                                      | example                             |
|-------------------------------------|--------------------------------------------------|-------------------------------------|
| rdscore_ipvlan_ns_one               | Namespace for the test workload                  | `my-mc-1`                           |
| rdscore_ipvlan_cm_data_one          | Content of configMap that is mounted within pods |                                     |
| rdscore_ipvlan_deploy_img_one       | Image used by test workloads                     | `quay.io/myorg/my-mc-app:1.1`       |
| rdscore_ipvlan_deploy_3_cmd         | Command executed by 1st container                | `["/bin/sh", "-c", "/myapp --run"]` |
| rdscore_ipvlan_deploy_4_cmd         | Command executed by 2nd container                | `["/bin/sh", "-c", "/myapp --run"]` |
| rdscore_ipvlan_nad_one_name         | Name of the IpVlan network                       | `ip-vlan`                           |
| rdscore_ipvlan_1_node_selector      | Node selector for the 1st workload               | `kubernetes.io/hostname: worker-X`  |
| rdscore_ipvlan_deploy_3_target      | IPv4 address and port configured on 2nd workload | `192.168.12.22 1111`                |
| rdscore_ipvlan_deploy_3_target_ipv6 | IPv6 address configured on 2nd workload          |                                     |
| rdscore_ipvlan_deploy_4_target      | IPv4 address and port configured on 1st workload | `192.168.12.12 1111`                |
| rdscore_ipvlan_deploy_4_target_ipv6 | IPv6 address configured on 1st workload          |                                     |

### _VerifyNMStateNamespaceExists_

Verifies namespace for _NMState_ operator exists

| paremater | description | example |
|-----------|-------------|---------|
|nmstate_operator_namespace | Namespace where NMState operator is installed | `openshift-nmstate` |

### _VerifyAllNNCPsAreOK_

Test assert all available NNCPs are Available, not progressing and not degraded.

_No extra parameters are required_

### _VerifyCephFSPVC_

Create a workload that requests PVC backed by _CephFS_ volume. Deployment is created on the node specified by *rdscore_wlkd_odf_one_selector*
parameter.

After data is stored in a a volume backed by the PVC deployment is scaled down and redeployed to the node specified by *rdscore_wlkd_odf_two_selector* parameter

**Requires 2 nodes**

| paremater | description | example |
|-----------|-------------|---------|
|rdscore_sc_cephfs_name | storageClass name that provides _CephFS_ volumes | `my-cephfs-sc` |
|rdscore_storage_storage_wlkd_image | Image used by the test workload | `quay.io/myorg/my-app:v1.1` |
|rdscore_wlkd_odf_one_selector | Node selector for 1st node | `kubernetes.io/hostname: worker-X` |
|rdscore_wlkd_odf_two_selector | Node selector for 2nd node | `kubernetes.io/hostname: worker-Y` |

### _VerifyCephRBDPVC_

Create a workload that requests PVC backed by _CephRBD_ volume. Deployment is created on the node specified by *rdscore_wlkd_odf_one_selector*
parameter.

After data is stored in a a volume backed by the PVC deployment is scaled down and redeployed to the node specified by *rdscore_wlkd_odf_two_selector* parameter

**Requires 2 nodes**

| paremater | description | example |
|-----------|-------------|---------|
|rdscore_sc_cephrbd_name | storageClass name that provides _CephRBD_ volumes | `my-cephrbd-sc` |
|rdscore_storage_storage_wlkd_image | Image used by the test workload | `quay.io/myorg/my-app:v1.1` |
|rdscore_wlkd_odf_one_selector | Node selector for 1st node | `kubernetes.io/hostname: worker-X` |
|rdscore_wlkd_odf_two_selector | Node selector for 2nd node | `kubernetes.io/hostname: worker-Y` |

### _VerifyCommatrixHostFirewallConnectivity_

This test verifies host-firewall TCP reachability from the test runner to node external IPs: API and dynamically selected
open/blocked ports on master and secure-pool workers (ports are read from live `openshift_filter` nftables rules).

Test expects TCP dials from the test runner to reach the control-plane API where allowed, to fail on blocked ports,
to reach an accepted port on the secure worker, and optionally to fail on a second secure-pool worker for peer probes.

**Requires commatrix host-firewall MachineConfigs already applied on the cluster, resolvable master/secure worker IPs
(secure worker is the first node in the inferred secure firewall MCP), and optionally a second node in that MCP for the peer probe**

API, kubelet fallback, and closed-port candidates are fixed in test code (6443, 10250, 9999).

Run commatrix specs: `ginkgo --label-filter=commatrix ./tests/system-tests/rdscore`

### _VerifyCommatrixHostFirewallJournal_

This test verifies host-firewall kernel logging: rate-limited `firewall` / `firewall ` log-prefix buckets in the
journal, and a temporary `TCP_TEST` nft log rule with matching journal lines after a probe.

Test expects kernel journal lines on the same secure-worker node as connectivity matching the firewall log keyword, at most five
lines per bucket in each of two consecutive one-minute windows (`2m–1m ago` and `last 1m`), and at least one `TCP_TEST` line referencing
the probed destination port after a TCP probe from the test runner.

**Requires commatrix host-firewall MCs on the cluster, connectivity spec run first when possible (same secure-worker node),
and firewall traffic or probes sufficient to produce journal lines**

### _VerifyNMStateInstanceExists_

Verifies that _NMState_ instance `nmstate` exists

