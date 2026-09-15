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
- Host IOMMU enabled (`intel_iommu=on iommu=pt`); Dell/BF often also need
  `pci=realloc` (and sometimes BIOS MMIO High Size) before VF create works
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
| `ECO_WORKER_LABEL` | Worker node label selector | from shared ocpconfig |

Default test image is the OpenShift org Quay `iperf3` image (digest-pinned). Prefer
`openshift/network-tools` once it ships iperf3
([PR #189](https://github.com/openshift/network-tools/pull/189)). Override with
`ECO_OCP_HWOL_TEST_CONTAINER` if the lab cannot pull Quay.

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

The Ordered HWOL spec ensures `SriovOperatorConfig`
(`featureGates.manageSoftwareBridges`, `enableOvsOffload`; mellanox plugin left
enabled so firmware SR-IOV/TotalVfs can be programmed on CX6), creates
`SriovNetworkPoolConfig` (OvsHardwareOffload only — no parallel
nodeSelector/maxUnavailable), applies switchdev policy with `bridge.ovs: {}`,
and waits for MCP/node-state sync. The `switchdev` spec asserts
`eSwitchMode=switchdev` and a managed OVS bridge uplink. Attach table Entries
create the network CR, assert NAD type/resource (and bridge for ovs), and run a
sleep pod. The ovs offload Entry creates two same-node iperf pods, asserts
representors on the managed bridge, runs traffic, and asserts non-empty
`type=offloaded` datapath flows on the node.

`AfterAll` removes only the HWOL-created OVSNetwork/SriovNetwork objects,
`hwol-switchdev` policy, and HWOL pool config — run on a dedicated lab MCP, not
a shared production-like pool. Cleanup waits up to `CleanupWaitTimeout` (15m),
then verifies that the MCP and SR-IOV node state are stable, the policy and pool
config are gone, the target PF has returned to legacy mode, and its managed OVS
bridge has been removed. A cleanup timeout (often switchdev reset stuck with
`device or resource busy`) fails the suite; reboot the MCP-labeled HWOL node
before re-running.

## Current coverage

| Spec | What it checks |
|------|----------------|
| HWOL setup smoke | SR-IOV operator deployed, `ECO_OCP_HWOL_DEVICES` parses, ≥1 worker |
| HWOL switchdev | Foundation + OVS bridge status on MCP nodes |
| HWOL attach (ovs / sriov) | NAD `type` + resource (+ ovs bridge) + pod secondary network |
| HWOL offload (ovs) | Representors on managed bridge + iperf3 + `type=offloaded` flows |
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
