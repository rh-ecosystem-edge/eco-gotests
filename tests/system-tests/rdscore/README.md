# RDS Core System Tests

Documentaion of parameters to be set for running this test suite.

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

### _VerifyCommatrixHostFirewallArtifacts_

This test verifies `oc commatrix` host-firewall _MachineConfig_ artifacts: direct `mc` output matches butane-rendered
_MachineConfigs_, and embedded _nftables_ `openshift_filter` rules are present in generated MC payloads.

Test expects `oc commatrix generate` to write under _rdscore_commatrix_output_dir_ (`format-mc` and `format-butane`), the
butane CLI to render `mc-*.yaml` beside `butane-*.yaml`, and decoded MC payloads to contain host-firewall nftables rules.

**Requires `oc commatrix` on the test runner, a writable output directory, and a cluster that will receive MCs in later specs**

| parameter | description | example |
|-----------|-------------|---------|
|rdscore_commatrix_output_dir | Writable directory for `oc commatrix generate` output (_required_) | `/tmp/commatrix-work` |
|rdscore_commatrix_open_pool_node_label | Label selector for open pool nodes (_optional_; inferred from `mc-*mcp-b*.yaml` when empty) | `` |

API/kubelet/closed ports, MCP wait, and journal keyword are fixed in test code (6443, 10250, 9999, 15m, `firewall`).

Run the ordered commatrix spec: `ginkgo --label-filter=commatrix ./tests/system-tests/rdscore`

### _VerifyCommatrixHostFirewallApply_

This test verifies host-firewall _MachineConfiguration_ apply on the cluster: node disruption policy merge patch,
selective apply of secure-pool and master _MachineConfigs_, _MachineConfigPool_ stability, and `openshift_filter` chain
presence on a worker node.

Test expects `format-mc` artifacts from the artifacts spec, including `node-disruption-policy.yaml`, `mc-<secure-pool>.yaml`, and
`mc-master.yaml`. Other generated `mc-<pool>.yaml` files (e.g. open pool) must remain unapplied so open workers stay
without host-firewall rules for the open-pool kubelet connectivity check.

**Requires commatrix artifacts and at least one node in each applied _MachineConfigPool_. Reverts cluster changes in AfterAll after all four specs**

Uses commatrix parameters listed under _VerifyCommatrixHostFirewallArtifacts_.

### _VerifyCommatrixHostFirewallConnectivity_

This test verifies host-firewall TCP reachability from the test runner to node _InternalIP_ addresses: API and kubelet
ports allowed or blocked per pool, and open-pool kubelet reachable when that pool's MC was not applied.

Test expects `nc` on the test runner to reach the control-plane API where allowed, to fail or time out
to secure-pool worker API ports, to reach kubelet on the secure worker, to fail on a closed test port, and to reach
kubelet on an open-pool worker.

**Requires applied host-firewall MCs, resolvable master/secure/open worker internal IPs (secure worker is the first node in the applied firewall MCP from apply; open pool via label or inferred MCP), and optionally a second node in an applied firewall MCP for the peer API probe**

Uses commatrix parameters listed under _VerifyCommatrixHostFirewallArtifacts_.

### _VerifyCommatrixHostFirewallJournal_

This test verifies host-firewall kernel logging: rate-limited `firewall` / `firewall ` log-prefix buckets in the
journal, and a temporary `TCP_TEST` nft log rule with matching journal lines after a probe.

Test expects kernel journal lines on the same secure-worker node as connectivity matching the firewall log keyword, at most five
lines per bucket in each of two consecutive one-minute windows (`2m–1m ago` and `last 1m`), and at least one `TCP_TEST` line referencing
the probed destination port after `nc` from the test runner.

**Requires applied host-firewall MCs, connectivity spec run first (same secure-worker node), and firewall traffic or probes sufficient to produce journal lines**

Uses commatrix parameters listed under _VerifyCommatrixHostFirewallArtifacts_.

### _VerifyNMStateInstanceExists_

Verifies that _NMState_ instance `nmstate` exists
