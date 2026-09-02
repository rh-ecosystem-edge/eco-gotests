# OCP SR-IOV Test Suite

## Overview

This test suite validates SR-IOV (Single Root I/O Virtualization) functionality on OpenShift Container Platform (OCP) clusters. The tests cover Virtual Function (VF) configuration, network connectivity, operator lifecycle management, QinQ/VLAN tagging, metrics export, DPDK, RDMA, and parallel draining.

> **Agent reference**: [`agent.md`](agent.md) in this directory captures deep investigation notes, non-obvious skip conditions, infrastructure requirements per test file, and switch configuration details for AI-assisted analysis.

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
| `ECO_OCP_SRIOV_SWITCH_USER` | Switch username for QinQ/metrics tests | -- |
| `ECO_OCP_SRIOV_SWITCH_PASS` | Switch password for QinQ/metrics tests | -- |
| `ECO_OCP_SRIOV_SWITCH_IP` | Switch IP address for QinQ/metrics tests | -- |
| `ECO_OCP_SRIOV_SWITCH_INTERFACES` | Comma-separated Junos port names (e.g. `et-0/0/15,et-0/0/14`) | -- |
| `ECO_OCP_SRIOV_VLAN` | S-VLAN ID for QinQ tests (1–4094) | -- |

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
│   ├── sriovocpenv/            # OCP-specific environment helpers (switch NETCONF)
│   └── tsparams/               # Test suite constants and parameters
│       ├── consts.go           # Labels, timeouts, test constants
│       └── ocpsriovvars.go     # Variables and reporter configuration
├── tests/
│   ├── allmulti.go             # Allmulti/multicast reception tests
│   ├── app-ns-sriovnet.go      # SriovNetwork cross-namespace tests
│   ├── basic.go                # Basic SR-IOV VF tests
│   ├── exposemtu.go            # MTU exposure tests
│   ├── metricsExporter.go      # SR-IOV metrics exporter tests
│   ├── paralleldraining.go     # Parallel drain behavior tests
│   ├── qinq.go                 # QinQ / 802.1AD / 802.1Q / DPDK tests
│   ├── rdmametricsapi.go       # RDMA metrics API tests (Mellanox only)
│   └── reinstallation.go       # SR-IOV operator reinstallation tests
├── agent.md                    # Agent reference: investigation notes, infra requirements
├── README.md
└── sriov_suite_test.go         # Ginkgo test suite entry point
```

## Test Cases

For detailed test ID tables, per-test skip conditions, and infrastructure requirements by test file, see [agent.md](agent.md).

### Basic Tests (`basic.go`) — Label: `basic`

Tests VF creation, network configuration, and connectivity across multiple hardware configurations. Iterates over all configured devices in `ECO_OCP_SRIOV_DEVICES`.

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

### Reinstallation Tests (`reinstallation.go`) — Label: `sriovreinstall`

Tests SR-IOV operator lifecycle: removal, validation, and reinstallation. Requires `ECO_OCP_SRIOV_INTERFACE_LIST` (≥2 interfaces). Tests run in strict order.

| Test ID | Description |
|---------|-------------|
| 46528 | Verify SR-IOV operator control plane is operational before removal |
| 46529 | Verify SR-IOV operator data plane is operational before removal |
| 46530 | Verify all SR-IOV components are deleted when operator is removed |
| 46531 | Validate that SR-IOV resources cannot be deployed without operator |
| 46532 | Validate that re-installed SR-IOV operator control plane is up |
| 46533 | Validate that re-installed SR-IOV operator data plane is up |

### Allmulti Tests (`allmulti.go`) — Labels: `allmulti`, `sriov-hw-enabled`

Tests multicast reception via SR-IOV allmulti mode. Requires 2+ workers. Cross-node tests (67815, 67816) send traffic through the switch but do not require switch credentials.

Test IDs: 67813, 67815, 67816, 67817, 67818, 67819, 67820

### Expose MTU Tests (`exposemtu.go`) — Label: `exposemtu`

Tests MTU exposure for netdev and vfio-pci SR-IOV devices at 1500 and 9000 byte MTU.

Test IDs: 73786, 73787, 73788, 73789, 73790

### Metrics Exporter Tests (`metricsExporter.go`) — Labels: `sriovmetricsexporter`, `sriov-hw-enabled`

Tests the SR-IOV network metrics exporter feature gate. Enables the `metricsExporter` feature in SriovOperatorConfig and validates Prometheus metrics. Vfio-pci contexts require a PerformanceProfile and switch credentials (switch MAC table is cleared in BeforeEach).

Test IDs: 74762, 74797, 74800, 75929, 75930, 75931, 75932, 75933, 75934

### Parallel Draining Tests (`paralleldraining.go`) — Label: `paralleldraining`

Tests SR-IOV node draining behavior with and without `SriovNetworkPoolConfig`. Requires 2+ workers (3+ for some tests).

Test IDs: 68640, 68661, 68662, 68663, 68664

### QinQ Tests (`qinq.go`) — Labels: `qinq`, `sriov-hw-enabled`

Tests double-tagged VLAN (802.1AD, 802.1Q) and DPDK QinQ functionality. Requires 2 workers, switch credentials, and L2 connectivity through a Juniper switch configured via NETCONF. 802.1AD and DPDK tests are Intel E810 only (device IDs `1592`/`1593`).

**Required env vars**: `ECO_OCP_SRIOV_SWITCH_IP`, `ECO_OCP_SRIOV_SWITCH_USER`, `ECO_OCP_SRIOV_SWITCH_PASS`, `ECO_OCP_SRIOV_SWITCH_INTERFACES`, `ECO_OCP_SRIOV_VLAN`

Test IDs: 71676, 71677, 71678, 71679, 71680, 71681, 71682, 71683, 71684, 72636, 72638, 73105

### RDMA Metrics API Tests (`rdmametricsapi.go`) — Labels: `rdmametricsapi`, `sriov-hw-enabled`

Tests RDMA metrics API in exclusive and shared modes via `SriovNetworkPoolConfig`. Requires Mellanox NIC and 2 SR-IOV interfaces. Deploys a PerformanceProfile.

Test IDs: 77649, 77650, 77651, 77653

### Application Namespace SR-IOV Network Tests (`app-ns-sriovnet.go`) — Label: `sriovnetappns`

Tests SriovNetwork scoping and lifecycle across application namespaces, including targetNamespace and resource update behavior.

Test IDs: 83121, 83123, 83124, 83125, 83142

## Test Labels

| Label | Description |
|-------|-------------|
| `ocpsriov` | Suite label — all SR-IOV tests |
| `basic` | Basic VF functionality tests |
| `sriovreinstall` | Operator reinstallation tests |
| `allmulti` | Allmulti/multicast reception tests |
| `exposemtu` | MTU exposure tests |
| `sriovmetricsexporter` | SR-IOV metrics exporter tests |
| `paralleldraining` | Parallel drain behavior tests |
| `qinq` | QinQ / 802.1AD / 802.1Q tests |
| `rdmametricsapi` | RDMA metrics API tests |
| `sriovnetappns` | Application namespace SR-IOV network tests |
| `sriov-hw-enabled` | Hardware-dependent tests (switch, PerformanceProfile, or specific NIC required) |

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

### Run QinQ Tests

```bash
export ECO_TEST_LABELS="qinq"
export ECO_OCP_SRIOV_SWITCH_IP=<switch-ip>
export ECO_OCP_SRIOV_SWITCH_USER=<user>
export ECO_OCP_SRIOV_SWITCH_PASS=<pass>
export ECO_OCP_SRIOV_SWITCH_INTERFACES="et-0/0/15,et-0/0/14,et-0/0/10,et-0/0/11"
export ECO_OCP_SRIOV_VLAN=3010
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

**Notes:**
- DPDK tests skip Broadcom NICs due to driver configuration requirements (OCPBUGS-30909)
- QinQ 802.1AD and DPDK tests require Intel E810 (device IDs `1592` or `1593`)
- RDMA metrics tests require Mellanox NICs

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

### QinQ Switch Configuration

QinQ tests configure a Juniper switch via NETCONF. If switch-related tests fail:
- Verify switch credentials in `ECO_OCP_SRIOV_SWITCH_*` env vars
- Confirm switch port names match physical wiring (`ECO_OCP_SRIOV_SWITCH_INTERFACES`)
- See [agent.md](agent.md) for switch port mapping, NETCONF command details, and known failure analysis
