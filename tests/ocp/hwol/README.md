# OCP HWOL Test Suite

Validates hardware offload (HWOL / switchdev) on OpenShift using the SR-IOV
Network Operator. This is a standalone suite under `tests/ocp/hwol/` — select it
locally with `ECO_TEST_FEATURES=hwol` (`make run-tests`).

## Prerequisites

- `SriovOperatorConfig` must already enable OVS hardware offload — the suite is
  read-only here and fails preflight otherwise:
  `oc patch sriovoperatorconfig default -n openshift-sriov-network-operator --type=merge -p '{"spec":{"enableOvsOffload":true,"featureGates":{"manageSoftwareBridges":true}}}'`
  `mellanox` must also not be listed in `spec.disablePlugins`.
- `SriovOperatorConfig/default` must already set
  `spec.enableOvsOffload: true` and
  `spec.featureGates.manageSoftwareBridges: true`, and `mellanox` must not be
  listed in `spec.disablePlugins`. The suite checks these settings without
  changing cluster-wide operator configuration; prepare a dedicated HWOL MCP
  before running it.
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
| `ECO_HWOL_RECOVER_ON_CLEANUP_FAILURE` | On a dedicated CI lab only, reboot HWOL MCP nodes after failed cleanup; a verified recovery of the post-switchdev cleanup timeout counts as a pass | `false` |
| `ECO_HWOL_ALLOW_PARALLEL` | Allow parallel Ginkgo workers for HWOL (dedicated debugging only) | `false` |
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

Before changing any HWOL-owned resource, the Ordered HWOL spec verifies the
operator configuration prerequisite above and that no non-HWOL policy selects the
requested PF. A shared or externally OVS-managed PF is rejected before the
suite changes cluster state. If this read-only preflight fails, the suite does
not create a namespace, pull images, or perform cleanup against the PF.

After this preflight, the Ordered HWOL spec creates
`SriovNetworkPoolConfig` (OvsHardwareOffload only — no parallel
nodeSelector/maxUnavailable). The SR-IOV Operator renders the corresponding
`00-<MCP>-ovs-hw-offload` MachineConfig; it is owned indirectly by the suite's
PoolConfig, not by a hand-written MachineConfig. Cleanup deletes the workload
networks, deletes that PoolConfig, waits for the operator to remove its
MachineConfig and for the MCP to disable hardware offload, and only then
removes the switchdev policy. This avoids asking mlx5 to change eSwitch mode
while OVN's offloaded TC rules still hold eSwitch references (`E-BUSY`). A
different PoolConfig already enabling OVS offload on the same MCP is rejected
during preflight because it would share that MachineConfig. The suite then
applies switchdev policy with `bridge.ovs: {}`, and waits for MCP/node-state
sync. The `switchdev` spec asserts
`eSwitchMode=switchdev` and a managed OVS bridge uplink. Attach table Entries
create the network CR, assert NAD type/resource (and bridge for ovs), and run a
sleep pod. The ovs offload Entry creates two same-node iperf pods, asserts
representors on the managed bridge, runs traffic, and asserts non-empty
`type=offloaded` datapath flows on the node.

`AfterAll` first deletes the HWOL test namespace and waits for its pods to
release their VFs and OVS representors. Ginkgo runs `AfterAll` before the last
Ordered spec's `DeferCleanup` callbacks, so those callbacks alone are not a
sufficient barrier. It then removes only the HWOL-created
OVSNetwork/SriovNetwork objects, `hwol-switchdev` policy, and HWOL pool config.
Every one of these resources
carries `eco-gotests.openshift.io/test=ocp-hwol`; a matching unlabelled object
is treated as externally owned and is rejected rather than changed or deleted.
Run on a dedicated lab MCP, not a shared production-like pool. Cleanup waits
up to `CleanupWaitTimeout` (15m),
then verifies that the MCP and SR-IOV node state are stable, the policy and pool
config are gone, the target PF has returned to legacy mode, and its managed OVS
bridge has been removed. By default, a cleanup timeout (often switchdev reset
stuck with `device or resource busy`) fails the suite. On a dedicated CI lab,
`ECO_HWOL_RECOVER_ON_CLEANUP_FAILURE=true` reboots the HWOL MCP nodes one at a
time, waits for recovery, and verifies cleanup. The original cleanup failure is
recorded in the report. Only the timeout while waiting for SR-IOV/MCP to
stabilize after switchdev-policy deletion becomes a pass when reboot recovery
completes and verifies that all test-owned resources are gone, the PFs are in
legacy mode, and the MCP and SR-IOV node states are stable. Any other cleanup
error, failed reboot, or failed verification still fails the run. The console
prints `PASS after verified reboot recovery` when this fallback succeeds.

The suite also repeats this fixed-name resource cleanup once in its synchronized
after-suite hook. This covers a process interruption or setup failure that
prevents the ordered `AfterAll` from running. It reruns the stability wait and
cleanup verification even when no resources remain, so it adds runtime.

`scripts/test-runner.sh` forces HWOL to one Ginkgo process by default, then
runs an independent post-run cleanup command after Ginkgo exits. The command
deletes only suite-owned resources and fails unless every matching PF returns
to `legacy` (or the operator's empty/default representation), with no managed
HWOL bridge or rendered offload MachineConfig remaining. Configure the BOS2
Jenkins job to run the same command from an unconditional post-build step;
that post step is needed if the runner itself is interrupted before its normal
post-run guard can execute:

```bash
go run ./tests/ocp/hwol/cmd/cleanup
```

The cleanup command revalidates PF/MCP ownership before it changes anything.
If the PF has become shared or an unowned fixed-name resource exists, it exits
unsuccessfully and leaves that resource untouched.

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
