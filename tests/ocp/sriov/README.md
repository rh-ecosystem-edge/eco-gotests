# OCP SR-IOV Test Suite

## DRA lifecycle coverage

The separate `tests/ocp/sriov/dra` suite ports the six functional cases from
[operator PR 1151](https://github.com/k8snetworkplumbingwg/sriov-network-operator/pull/1151),
using reference commit `02c9caacf32d0dcae37a7cb5d38f3b2d5a74cc92`.
It verifies provider enablement, the base DeviceClass, policy-generated DRA
resources, advertisement of two VFs, an in-place increase to four VFs, and DRA
disablement with device-plugin restoration. It does not test ResourceClaim
consumption by workloads or network traffic.

The suite changes the operator-wide resource provider and configures a selected
PF. Reserve an exclusive operator test window or use a dedicated cluster. A
selected node/PF and Ginkgo serial execution do not isolate other users of the
operator. Node reconfiguration may drain or reboot nodes depending on the
operator configuration. Do not run alongside other SR-IOV suites or jobs.

### Prerequisites

- A healthy installed operator containing the referenced DRA integration and a
  compatible dra-driver-sriov image. The general suite's OpenShift minimum does
  not establish DRA support; validate the actual operator and platform versions.
- `resource.k8s.io/v1` APIs and the driver's DRA CRDs available as required by
  the deployment. Explicitly enabled runs fail when required APIs cannot be read.
- DRA disabled initially and no conflicting SR-IOV policies or active users of
  the provider. The suite must not remove other users' configuration.
- An explicitly selected managed node and unused PF supporting at least four VFs.
- Credentials permitting reads of nodes, SR-IOV node states, DaemonSets, pods,
  DeviceClasses, ResourceSlices and driver resources; update of operator config;
  and creation, update and deletion of test policies. Existing reporter behavior
  may also require access to logs/events.
- The companion eco-goinfra DRA helpers. This review branch uses an immutable
  fork revision through a Go module replacement, with dependencies vendored.
  Before merge, remove that replacement and pin the merged upstream revision.

### Configuration and execution

For local verification with Go 1.27.3 or newer, run from eco-gotests:

```bash
verify_dir=$(mktemp -d)
KUBECONFIG=/dev/null go test ./tests/ocp/sriov/dra/internal/draenv
go test -c -o "$verify_dir/dra.test" ./tests/ocp/sriov/dra
```

The helper package has no cluster-client initialization. The final command only
compiles; do not execute the resulting integration binary as a local check.

| Variable | Meaning |
| --- | --- |
| `ECO_OCP_SRIOV_DRA_ENABLE` | Explicit opt-in; defaults to false |
| `ECO_OCP_SRIOV_DRA_NODE` | Exact node name; required when enabled |
| `ECO_OCP_SRIOV_DRA_PF` | PF interface name on that node; required when enabled |
| `ECO_OCP_SRIOV_OPERATOR_NAMESPACE` | Existing operator namespace setting |

The following is a user-run command template, not evidence of cluster validation.
Set the placeholders only after coordinating the test window and checking the
prerequisites. Run from the repository root with the vendored dependencies.

```bash
export KUBECONFIG=/path/to/kubeconfig
export ECO_OCP_SRIOV_DRA_ENABLE=true
export ECO_OCP_SRIOV_DRA_NODE=<reserved-node>
export ECO_OCP_SRIOV_DRA_PF=<reserved-pf>
export ECO_TEST_FEATURES=ocp/sriov/dra
export ECO_TEST_LABELS='ocpsriov && dra'
make run-tests
```

Replace angle-bracket placeholders before executing. Select the exact nested
feature path: `sriov` and `all` recursively discover other suites with their own
setup and cleanup. The DRA suite does not need a workload namespace or image
pre-pull. Run the complete ordered container; filtering individual dependent
cases does not establish six-case coverage. Assigned test-management IDs remain
pending and must not be fabricated.

### Results and recovery

Require all six cases to pass without skips and verify cleanup. Retain the JUnit
report, configured failure artifacts, the test/library revisions, OpenShift and
operator/driver versions, hardware selection and test duration. A default-disabled
skip is not a successful DRA validation. RDU2 lab relocation may affect baremetal
execution availability independently of a test failure.

Cleanup removes only test-owned policies, waits for PF reconfiguration and restores
the original DRA flag entry. It must not delete operator-generated resources to
make assertions pass. A killed test process or an API outage can prevent cleanup.
If cleanup fails, stop additional runs and inspect the reported policy names,
selected node state and current operator flag. Remove only positively identified
test policies, wait for node/PF convergence, and only then restore the original
provider setting. Do not use bulk policy or namespace deletion. Review exact
recovery commands against the captured failure state before executing them.

Cluster execution and verification are pending. This suite supplies downstream
functional coverage; security-lane and DRA CI-lane configuration are separate
CORENET-7535 deliverables.

## Overview

This test suite validates SR-IOV (Single Root I/O Virtualization) functionality on OpenShift Container Platform (OCP) clusters. The tests cover Virtual Function (VF) configuration, network connectivity, and operator lifecycle management.

## Prerequisites

### Cluster Requirements

- OCP cluster version >= 4.13
- SR-IOV operator installed and healthy
- At least one worker node with SR-IOV-capable network interfaces
- Access to eco-goinfra packages

### Required Environment Variables

| Variable | Description | Example |
|----------|-------------|---------|
| `KUBECONFIG` | Path to kubeconfig file | `/path/to/kubeconfig` |
| `ECO_OCP_SRIOV_VF_NUM` | Number of Virtual Functions to configure | `5` |

### Device Configuration (Choose One Method)

#### Option 1: ECO_OCP_SRIOV_DEVICES (Recommended)

Full device configuration including deviceID, vendor, and optional minTxRate support.

**Format:** `name:deviceID:vendorID:interfaceName[:supportsMinTxRate]`

```bash
export ECO_OCP_SRIOV_DEVICES="e810xxv:159b:8086:ens2f0,cx7:1021:15b3:ens3f0np0:true"
```

- `name`: Logical name for the device
- `deviceID`: PCI device ID (e.g., `159b` for Intel E810)
- `vendorID`: PCI vendor ID (e.g., `8086` for Intel, `15b3` for Mellanox)
- `interfaceName`: Physical interface name on the node
- `supportsMinTxRate`: Optional, defaults to `true` if omitted

#### Option 2: ECO_OCP_SRIOV_INTERFACE_LIST (Legacy)

Simple comma-separated list of interface names. Used primarily for reinstallation tests.

```bash
export ECO_OCP_SRIOV_INTERFACE_LIST="ens2f0,ens3f0"
```

### Optional Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `ECO_OCP_SRIOV_OPERATOR_NAMESPACE` | SR-IOV operator namespace | `openshift-sriov-network-operator` |
| `ECO_OCP_SRIOV_TEST_CONTAINER` | Container image for test workloads | -- |
| `ECO_OCP_SRIOV_DPDK_TEST_CONTAINER` | Container image for DPDK test workloads | -- |
| `ECO_OCP_SRIOV_PROMETHEUS_OPERATOR_NAMESPACE` | Prometheus operator namespace | -- |
| `ECO_OCP_SRIOV_MCP_LABEL` | Machine config pool label | -- |
| `ECO_OCP_SRIOV_SWITCH_USER` | Switch username for switch-based tests | -- |
| `ECO_OCP_SRIOV_SWITCH_PASS` | Switch password for switch-based tests | -- |
| `ECO_OCP_SRIOV_SWITCH_IP` | Switch IP address for switch-based tests | -- |
| `ECO_OCP_SRIOV_SWITCH_INTERFACES` | Switch interfaces for switch-based tests (comma-separated `name:port` pairs) | -- |
| `ECO_OCP_SRIOV_SWITCH_LAGS` | Switch LAG interface names for switch-based tests (comma-separated pair) | -- |
| `ECO_OCP_SRIOV_VLAN` | VLAN ID for switch-based tests (1-4094) | -- |

## Directory Structure

```text
tests/ocp/sriov/
├── internal/
│   ├── consolebrowser/         # Browser-based console utilities
│   ├── ocpsriovconfig/         # Configuration loading and parsing
│   │   ├── config.go           # Config struct and env var parsing
│   │   └── default.yaml        # Default device configurations
│   ├── ocpsriovinittools/      # Initialization tools (APIClient, SriovOcpConfig)
│   ├── sriovenv/               # SR-IOV network/policy/pod helpers
│   ├── sriovocpenv/            # OCP-specific environment helpers
│   └── tsparams/               # Test suite constants and parameters
│       ├── consts.go           # Labels, timeouts, test constants
│       └── ocpsriovvars.go     # Variables and reporter configuration
├── tests/
│   ├── basic.go                # Basic SR-IOV VF tests
│   └── reinstallation.go       # SR-IOV operator reinstallation tests
├── README.md
└── sriov_suite_test.go         # Ginkgo test suite entry point
```

## Test Cases

### Basic Tests (`basic.go`)

Label: `basic`

Tests VF creation, network configuration, and connectivity across multiple hardware configurations.

| Test ID | Description |
|---------|-------------|
| 25959 | SR-IOV VF with spoof checking enabled |
| 70820 | SR-IOV VF with spoof checking disabled |
| 25960 | SR-IOV VF with trust disabled |
| 70821 | SR-IOV VF with trust enabled |
| 25963 | SR-IOV VF with VLAN and rate limiting configuration (requires minTxRate support) |
| 25961 | SR-IOV VF with auto link state |
| 71006 | SR-IOV VF with enabled link state |
| 69646 | MTU configuration for SR-IOV policy (MTU 9000) |
| 69582 | DPDK SR-IOV VF functionality validation (skips Broadcom NICs) |

**Notes:**
- Tests iterate over all configured devices in `ECO_OCP_SRIOV_DEVICES`
- Tests skip gracefully when interfaces have NO-CARRIER status
- VLAN/rate limiting test (25963) only runs on devices with `supportsMinTxRate: true`
- DPDK test (69582) skips Broadcom NICs due to OCPBUGS-30909

### Reinstallation Tests (`reinstallation.go`)

Label: `sriovreinstall`

Tests SR-IOV operator lifecycle: removal, validation, and reinstallation.

| Test ID | Description |
|---------|-------------|
| 46528 | Verify SR-IOV operator control plane is operational before removal |
| 46529 | Verify SR-IOV operator data plane is operational before removal |
| 46530 | Verify all SR-IOV components are deleted when operator is removed |
| 46531 | Validate that SR-IOV resources cannot be deployed without operator |
| 46532 | Validate that re-installed SR-IOV operator control plane is up |
| 46533 | Validate that re-installed SR-IOV operator data plane is up |

**Notes:**
- Tests run in order (Ordered container)
- Requires at least 2 SR-IOV interfaces (`ECO_OCP_SRIOV_INTERFACE_LIST`)
- May trigger Machine Config Operator (MCO) rolling updates

## Test Labels

| Label | Description |
|-------|-------------|
| `ocpsriov` | Suite label - all SR-IOV tests |
| `basic` | Basic VF functionality tests |
| `sriovreinstall` | Operator reinstallation tests |

## Running Tests

### Run All SR-IOV Tests

```bash
export KUBECONFIG=/path/to/kubeconfig
export ECO_TEST_FEATURES="sriov"
export ECO_TEST_LABELS="ocpsriov"
export ECO_OCP_SRIOV_VF_NUM=5
export ECO_OCP_SRIOV_DEVICES="e810xxv:159b:8086:ens2f0"
make run-tests
```

### Run Basic Tests Only

```bash
export ECO_TEST_LABELS="basic"
make run-tests
```

### Run Reinstallation Tests Only

```bash
export ECO_TEST_LABELS="sriovreinstall"
export ECO_OCP_SRIOV_INTERFACE_LIST="ens2f0,ens3f0"
make run-tests
```

### Run Specific Test by ID

```bash
export ECO_TEST_LABELS="25959"
make run-tests
```

### Exclude Tests

```bash
# Run basic tests but skip DPDK test
export ECO_TEST_LABELS="basic && !69582"
make run-tests
```

## Test Constants

Key constants defined in `tsparams/consts.go`:

| Constant | Value | Description |
|----------|-------|-------------|
| `DefaultTestMTU` | 9000 | MTU value for MTU tests |
| `TestVLAN` | 100 | VLAN ID for VLAN tests |
| `TestVlanQoS` | 2 | QoS value for VLAN tests |
| `TestMinTxRate` | 40 Mbps | Minimum TX rate for rate limiting |
| `TestMaxTxRate` | 100 Mbps | Maximum TX rate for rate limiting |
| `MCOWaitTimeout` | 35 min | Timeout for MCO operations |
| `DefaultTimeout` | 300 sec | Default operation timeout |
| `PodReadyTimeout` | 300 sec | Pod readiness timeout |

## Supported Hardware

Tests are designed to work with common SR-IOV NICs:

| Vendor | Example NICs | Vendor ID |
|--------|--------------|-----------|
| Intel | E810, X710 | `8086` |
| Mellanox/NVIDIA | ConnectX-6, ConnectX-7 | `15b3` |
| Broadcom | BCM57xxx | `14e4` |

**Note:** DPDK tests skip Broadcom NICs due to driver configuration requirements (OCPBUGS-30909).

## Troubleshooting

### NO-CARRIER Errors

If tests skip with "NO-CARRIER" status, verify:
- Physical network cable is connected
- Switch port is enabled and configured
- Interface is administratively up on the node

### VF Initialization Failures

If VF initialization fails:
- Check SR-IOV operator status: `oc get sriovnetworknodestates -n openshift-sriov-network-operator`
- Verify device is SR-IOV capable: `lspci -v | grep -i sriov`
- Check node resources: `oc describe node <node-name> | grep -i sriov`

### MCO Timeout

Reinstallation tests may timeout during MCO rolling updates:
- Increase `MCOWaitTimeout` if needed
- Check MCO status: `oc get mcp`
