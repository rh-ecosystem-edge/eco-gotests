# OCP HWOL Test Suite

Validates hardware offload (HWOL / switchdev) on OpenShift using the SR-IOV
Network Operator. This is a standalone suite under `tests/ocp/hwol/` — select it
locally with `ECO_TEST_FEATURES=hwol` (`make run-tests`).

## Prerequisites

- OCP cluster with the SR-IOV Network Operator installed and healthy
- At least one worker with an mlx5-capable NIC in a mode that supports host
  eswitch switchdev (BF-3 NIC mode / CX-6 Dx; BF-2 DPU mode is **not** enough)
- MCP matching `ECO_OCP_HWOL_MCP_LABEL` (default `sriov`) with workers labeled
  `node-role.kubernetes.io/<mcp_label>=""`
- Confirm IOMMU and PCI resource requirements for the selected device, driver
  and platform. Do not change kernel or BIOS settings without validating that
  they are required for the deployment.
- `KUBECONFIG` pointing at the cluster
- For the **ovs-network** path: Subscription must set `OVS_CNI_IMAGE` to a
  pullable ovs-cni image, with registry credentials configured on the cluster
  as needed
- For the **offload** path: test image must include `iperf3` (default does);
  `ECO_OCP_HWOL_VF_NUM` ≥ 3 (VF0 reserved + server/client VFs)

## Required environment variables

| Variable | Description | Example |
|----------|-------------|---------|
| `KUBECONFIG` | Path to kubeconfig | `/path/to/kubeconfig` |
| `ECO_OCP_HWOL_DEVICES` | Device list (`name:deviceID:vendor:interface`, comma-separated) | `bf2:a2d6:15b3:ens7f0np0` |

Devices are **not** configured in YAML. `ECO_OCP_HWOL_DEVICES` is required.

## Optional environment variables

| Variable | Description | Default |
|----------|-------------|---------|
| `ECO_OCP_HWOL_OPERATOR_NAMESPACE` | SR-IOV operator namespace | `openshift-sriov-network-operator` |
| `ECO_OCP_HWOL_TEST_CONTAINER` | Workload / traffic image (needs `iperf3` for offload) | `quay.io/openshifttest/iperf3@sha256:440c5925…` |
| `ECO_OCP_HWOL_MCP_LABEL` | Machine config pool name / role label | `sriov` |
| `ECO_OCP_HWOL_VF_NUM` | Number of VFs (≥3 recommended; VF0 reserved) | `3` |
| `ECO_OCP_HWOL_OVS_DOCA` | Verify OVS-DOCA and use the DOCA bridge and PF/VF representors | `false` |
| `ECO_HWOL_RECOVER_ON_CLEANUP_FAILURE` | Dedicated labs only: reboot HWOL MCP nodes after failed cleanup; the run still fails | `false` |
| `ECO_WORKER_LABEL` | Worker node label selector | from shared ocpconfig |

Default test image is the OpenShift org Quay `iperf3` image (digest-pinned). Prefer
`openshift/network-tools` once it ships iperf3
([PR #189](https://github.com/openshift/network-tools/pull/189)). Override with
`ECO_OCP_HWOL_TEST_CONTAINER` if the lab cannot pull Quay.
Images are pulled by the test-owned pods on their target node; the suite does
not create shared pre-pull pods in the `default` namespace.

Set `ECO_OCP_HWOL_OVS_DOCA=true` only for a CI lane that has already provisioned
OVS-DOCA on its dedicated MCP. The suite does not install OVS-DOCA; it verifies
`doca-init=true`, `hw-offload=true`, and DOCA datapath availability, then uses
a DOCA bridge, PF uplink and `OVSNetwork` representors. It checks the bridge,
interface types, port health, fresh `doca_flow_api` assignments for both
workload representors, and workload-specific offloaded counters. Provider
verification fails closed if the OVS process or log changes, the log is rotated
or truncated, or assignment evidence is missing. This mode is experimental and
requires a compatible OVS-DOCA/CNI deployment. The
default `false` leaves the existing kernel/TC path selected.

## Directory structure

```text
tests/ocp/hwol/
├── hwol_suite_test.go              # Ginkgo suite entrypoint
├── README.md
├── tests/
│   ├── setup.go                    # Precondition smoke
│   └── hwol.go                     # Ordered: foundation, switchdev, attach, offload
└── internal/
    ├── hwolenv/                    # Foundation, OVSNetwork, offload helpers
    ├── ocphwolconfig/              # ECO_OCP_HWOL_* config + default.yaml
    ├── ocphwolinittools/           # APIClient + HwolOcpConfig globals
    └── tsparams/                   # Labels, timeouts, reporter dumps
```

Shared NAD wait/assert helpers live in `tests/internal/sriovoperator/nad.go`.

## Test labels

| Label | Description |
|-------|-------------|
| `ocphwol` | Suite label — all HWOL tests |
| `setup` | Precondition smoke (operator, devices, workers) |
| `switchdev` | Switchdev mode + managed OVS bridge assert |
| `ovs-network` | Attach / offload via `OVSNetwork` (ovs CNI) |
| `sriov-network` | Attach / offload via `SriovNetwork` (sriov CNI) |
| `offload` | Same-node iperf + `ovs-appctl dpctl/dump-flows type=offloaded` |

Foundation (operator config, pool, switchdev policy, MCP wait) runs once in the
Ordered `BeforeAll` for the HWOL Describe. Filtering with `ECO_TEST_LABELS`
still runs that foundation when an attach, offload, or switchdev spec is selected.

## Running tests

### Compile / dry-run (no cluster)

```bash
go test -c ./tests/ocp/hwol
ginkgo --dry-run ./tests/ocp/hwol
```

### Live smoke (local make runner)

```bash
export KUBECONFIG=/path/to/kubeconfig
export ECO_TEST_FEATURES=hwol
export ECO_TEST_LABELS=setup
export ECO_OCP_HWOL_DEVICES="bf2:a2d6:15b3:ens7f0np0"
make run-tests
```

### switchdev bridge assert

```bash
export ECO_TEST_FEATURES=hwol
export ECO_TEST_LABELS=switchdev
export ECO_OCP_HWOL_DEVICES="bf2:a2d6:15b3:ens7f0np0"
make run-tests
```

### ovs CNI attach (requires OVS_CNI_IMAGE on the Subscription)

```bash
export ECO_TEST_FEATURES=hwol
export ECO_TEST_LABELS=ovs-network
export ECO_OCP_HWOL_DEVICES="bf2:a2d6:15b3:ens7f0np0"
make run-tests
```

### sriov CNI attach

```bash
export ECO_TEST_FEATURES=hwol
export ECO_TEST_LABELS=sriov-network
export ECO_OCP_HWOL_DEVICES="bf2:a2d6:15b3:ens7f0np0"
make run-tests
```

### offload verify (iperf + dpctl; OVSNetwork path)

Hardware-offload traffic verification is the **OVSNetwork / ovs CNI** path:
NAD config must name the managed bridge, both pod VF representors must be
`ovs-vsctl` ports on that bridge, then same-node iperf and
`dpctl/dump-flows type=offloaded`. The **sriov-network offload** Entry is
**Pending** until sriov CNI places VF representors on the managed bridge;
sriov **attach** still runs. Prefer CI labels such as
`ECO_TEST_LABELS=ovs-network` (attach + offload ovs) or
`ECO_TEST_LABELS='offload && ovs-network'`.

```bash
export ECO_TEST_FEATURES=hwol
export ECO_TEST_LABELS='offload && ovs-network'
export ECO_OCP_HWOL_DEVICES="bf2:a2d6:15b3:ens7f0np0"
export ECO_OCP_HWOL_VF_NUM=3
make run-tests
```

The Ordered HWOL spec requires `SriovOperatorConfig`
(`featureGates.manageSoftwareBridges`, `enableOvsOffload`; mellanox plugin left
enabled so firmware SR-IOV/TotalVfs can be programmed on CX6). It checks these
prerequisites without changing the existing operator configuration, then creates
`SriovNetworkPoolConfig` (OvsHardwareOffload only — no parallel
nodeSelector/maxUnavailable), applies switchdev policy with `bridge.ovs: {}`,
and waits for MCP/node-state sync. The `switchdev` spec asserts
`eSwitchMode=switchdev` and a managed OVS bridge uplink. Attach table Entries
create the network CR, assert NAD type/resource (and bridge for ovs), and run a
sleep pod. The ovs offload Entry creates two same-node iperf pods, asserts
representors on the managed bridge, records offloaded IPv4 counters for the
workload MAC pair, runs traffic, and requires those counters to increase.

Before creating a pool or policy, the suite requires every selected node to
have no other selecting SR-IOV policy or pool config and an unused legacy PF
with capacity for the requested VFs. This intentionally rejects shared nodes,
including nodes with unrelated SR-IOV policies, rather than altering their state.

Per-spec `AfterEach` removes workload pods before their network. If that
cleanup fails, the suite retains the foundation rather than resetting a PF with
potentially active workloads. `AfterAll` then removes only the HWOL-created OVSNetwork/SriovNetwork objects,
`hwol-switchdev` policy, and HWOL pool config — run on a dedicated lab MCP, not
a shared production-like pool. Cleanup waits up to `CleanupWaitTimeout` (15m),
then verifies that the MCP and SR-IOV node state are stable, the policy and pool
config are gone, the target PF has returned to legacy mode, and its managed OVS
bridge has been removed. The suite refuses to adopt pre-existing test-named
networks, policies or pool configs. Cleanup uses the shared SR-IOV/MCP stability
helper. A convergence failure fails the suite. The existing
`ECO_HWOL_RECOVER_ON_CLEANUP_FAILURE=true` option can reboot the selected MCP
nodes and verify recovery, while preserving the original failure. Keep it
disabled on ANL; investigate pending firmware settings before a manual reboot.
Operator-managed changes may still reboot the node during setup or cleanup.

On ANL246, deleting the test policy
queued `SRIOV_EN=False` and `NUM_OF_VFS=0` for the next boot, while the running
settings remained enabled. The suite does not inspect or restore firmware
settings; the lab and operator own them. Check the next-boot configuration
before a later reboot if VF capacity must be preserved.

The pool controller creates/removes a HWOL MachineConfig, which can trigger MCD
reboots. A full ANL246 run starting with `False`/`0` pending passed all five
runnable specs: the operator restored VF capacity during setup without manual
firmware restoration. Cleanup queued `False`/`0` again. This validates that lab
sequence, not arbitrary reboot/recovery scenarios.

Each suite run atomically creates a uniquely named `hwol-tests-<run-id>`
namespace and retains it after execution. Namespace ownership alone cannot
establish ownership of every resource inside it: review all namespaced resources
and the reported UID before manually deleting it. Subsequent runs create new
namespaces rather than adopting or deleting retained ones; they still refuse
pre-existing HWOL policies, networks and pool configs. Run with one Ginkgo
process because those operator resources have shared names.

## Current coverage

| Spec | What it checks |
|------|----------------|
| HWOL setup smoke | SR-IOV operator deployed, `ECO_OCP_HWOL_DEVICES` parses, ≥1 worker |
| HWOL switchdev | Foundation + OVS bridge status on MCP nodes |
| HWOL attach (ovs / sriov) | NAD `type` + resource (+ ovs bridge) + pod secondary network |
| HWOL offload (ovs) | Representors on managed bridge + iperf3 + increasing `type=offloaded` counters for the workload MAC pair |
| HWOL offload (sriov) | Pending — VF representors not on managed OVS bridge |

Attach networks default to host-local IPAM (configurable on the create helpers);
NV-IPAM is not a suite prerequisite.

The offload path requires the HWOL PF to have **link/carrier** (BF/CX data
ports cabled to the lab switch). Without carrier, switchdev + NAD attach can
still pass while same-node VF↔VF iperf fails (`Host is unreachable`) because the
mlx5 eSwitch does not deliver traffic to OVS representors.

Representor tcpdump heuristics and full
[kubernetes-traffic-flow-tests](https://github.com/ovn-kubernetes/kubernetes-traffic-flow-tests)
(`validate_offload`) integration are follow-on work.
