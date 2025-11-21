# SR-IOV Basic Tests

This document describes the SR-IOV basic test suite located in `tests/ocp/sriov/tests/basic.go`.

## Overview

The SR-IOV basic tests validate fundamental SR-IOV functionality on OpenShift Container Platform (OCP) clusters. These tests verify Virtual Function (VF) configuration, network attachment, and various SR-IOV features including spoof checking, trust settings, VLAN configuration, rate limiting, link state management, MTU configuration, and DPDK support.

## Test Suite Information

- **Test File**: `tests/ocp/sriov/tests/basic.go`
- **Test Suite Label**: `ocpsriov` and `basic`
- **Test Namespace**: `sriov-tests`
- **Test Container**: Configurable via `ECO_OCP_SRIOV_TEST_CONTAINER` (default: `quay.io/ocp-edge-qe/eco-gotests-sriov-client:v4.15.2`)

## Prerequisites

### Cluster Requirements

- OCP cluster version >= 4.13
- SR-IOV operator installed and healthy
- Worker nodes with SR-IOV-capable network interfaces
- Appropriate hardware or virtualized SR-IOV interfaces available

### Environment Variables

#### Mandatory

- `KUBECONFIG` - Path to kubeconfig file (required for all tests)

#### Optional

- `ECO_OCP_SRIOV_TEST_CONTAINER` - Test container image (default: `quay.io/ocp-edge-qe/eco-gotests-sriov-client:v4.15.2`)
- `ECO_SRIOV_TEST_CONTAINER` - Alternative test container image (fallback if `ECO_OCP_SRIOV_TEST_CONTAINER` is not set)
- `ECO_OCP_SRIOV_VF_NUM` - Number of virtual functions to create (default: 2)
- `SRIOV_VF_NUM` - Alternative VF number environment variable (fallback if `ECO_OCP_SRIOV_VF_NUM` is not set)
- `SRIOV_DEVICES` - Custom device configuration (format: `name1:deviceid1:vendor1:interface1,name2:deviceid2:vendor2:interface2,...`)
- `ECO_REPORTS_DUMP_DIR` - Directory for test reports and JUnit XML files (default: `/tmp/reports`). **Note**: If not set to an absolute path, JUnit reports may be written to the test execution directory instead of the reports directory.

#### Device Configuration

If `SRIOV_DEVICES` is not set, the test suite uses default device configurations:

- `e810xxv` (DeviceID: 159b, Vendor: 8086, Interface: eno12409)
- `e810c` (DeviceID: 1593, Vendor: 8086, Interface: ens2f2)
- `x710` (DeviceID: 1572, Vendor: 8086, Interface: ens5f0) - NO-CARRIER
- `bcm57414` (DeviceID: 16d7, Vendor: 14e4, Interface: ens4f1np1)
- `bcm57508` (DeviceID: 1750, Vendor: 14e4, Interface: ens3f0np0) - NO-CARRIER
- `e810back` (DeviceID: 1591, Vendor: 8086, Interface: ens4f2)
- `cx7anl244` (DeviceID: 1021, Vendor: 15b3, Interface: ens2f0np0)

**Note**: Devices marked with "NO-CARRIER" may be skipped if the interface has no carrier signal.

## Running the Tests

### Running Only OCP SR-IOV Tests (Recommended)

To run only the OCP SR-IOV tests (excluding other SR-IOV test suites), you can use either `go test` or direct `ginkgo` execution:

#### Using `go test` (Recommended)

**Recommended filter: `basic`** (excludes reinstallation test)

```bash
export KUBECONFIG=/path/to/kubeconfig
export SRIOV_DEVICES="e810xxv231:159b:8086:eno12399,cx5ex231:1019:15b3:ens6f0np0"  # Optional: custom device config
export GOSUMDB=sum.golang.org
export GOTOOLCHAIN=auto
go test ./tests/ocp/sriov/... -v -ginkgo.v -ginkgo.label-filter="basic" -timeout 60m
```

**Alternative filter: `ocpsriov && basic`** (may still include reinstallation test)

```bash
go test ./tests/ocp/sriov/... -v -ginkgo.v -ginkgo.label-filter="ocpsriov && basic" -timeout 60m
```

#### Using `ginkgo` directly

**Recommended filter: `basic`** (excludes reinstallation test)

```bash
export KUBECONFIG=/path/to/kubeconfig
export SRIOV_DEVICES="e810xxv231:159b:8086:eno12399,cx5ex231:1019:15b3:ens6f0np0"  # Optional: custom device config
export GOSUMDB=sum.golang.org
export GOTOOLCHAIN=auto
export ECO_TEST_LABELS="basic"
cd tests/ocp/sriov
ginkgo -timeout=60m --keep-going --require-suite --label-filter="$ECO_TEST_LABELS" -v .
```

**Alternative filter: `ocpsriov && basic`** (may still include reinstallation test)

```bash
export ECO_TEST_LABELS="ocpsriov && basic"
cd tests/ocp/sriov
ginkgo -timeout=60m --keep-going --require-suite --label-filter="$ECO_TEST_LABELS" -v .
```

**Note on label usage**: 
- **Suite-level labels**: The test suite applies only the `ocpsriov` label at the suite level (via `RunSpecs`)
- **Test-specific labels**: Individual test files add their own labels:
  - `basic.go`: Adds the `basic` label to all 9 basic test cases
  - `reinstallation.go`: Has only the `ocpsriov` label (no `basic` label)
- **Filter behavior**: 
  - `--label-filter="basic"` - Runs only the 9 basic tests (recommended)
  - `--label-filter="ocpsriov && basic"` - Also runs only the 9 basic tests (equivalent)
  - `--label-filter="ocpsriov"` - Runs ALL tests including reinstallation

**Note**: The 60-minute timeout provides sufficient time for all 9 test cases (which typically complete in ~35 minutes) while allowing buffer for slower environments or network delays.

**Note**: Using `go test` or direct Ginkgo execution ensures only the `ocp/sriov` tests are run, avoiding conflicts with other SR-IOV test suites in the repository.

### Using the Test Runner Script

**Warning**: The test runner script will discover all directories named "sriov", which may include other SR-IOV test suites. To run only OCP SR-IOV tests, use the direct Ginkgo execution method above.

**Recommended filter: `basic`** (excludes reinstallation test)

```bash
export KUBECONFIG=/path/to/kubeconfig
export ECO_TEST_FEATURES="sriov"
export ECO_TEST_LABELS="basic"
make run-tests
```

**Alternative filter: `ocpsriov && basic`** (may still include reinstallation test)

```bash
export ECO_TEST_LABELS="ocpsriov && basic"
make run-tests
```

### Running Specific Test Cases

#### By Test ID

**Using `go test`:**
```bash
export KUBECONFIG=/path/to/kubeconfig
go test ./tests/ocp/sriov/... -v -ginkgo.v -ginkgo.label-filter="25959" -timeout 60m
```

**Using `ginkgo`:**
```bash
export KUBECONFIG=/path/to/kubeconfig
export ECO_TEST_LABELS="25959"
cd tests/ocp/sriov
ginkgo -timeout=60m --keep-going --require-suite --label-filter="$ECO_TEST_LABELS" -v .
```

#### By Multiple Test IDs

**Using `go test`:**
```bash
go test ./tests/ocp/sriov/... -v -ginkgo.v -ginkgo.label-filter="25959 || 70820 || 25960" -timeout 60m
```

**Using `ginkgo`:**
```bash
export ECO_TEST_LABELS="25959 || 70820 || 25960"
cd tests/ocp/sriov
ginkgo -timeout=60m --keep-going --require-suite --label-filter="$ECO_TEST_LABELS" -v .
```

#### Exclude Specific Tests

**Using `go test`:**
```bash
# Exclude DPDK test (69582) from basic tests
go test ./tests/ocp/sriov/... -v -ginkgo.v -ginkgo.label-filter="basic && !69582" -timeout 60m
```

**Using `ginkgo`:**
```bash
export ECO_TEST_LABELS="basic && !69582"  # Exclude DPDK test
cd tests/ocp/sriov
ginkgo -timeout=60m --keep-going --require-suite --label-filter="$ECO_TEST_LABELS" -v .
```

#### Exclude Reinstallation Test

The reinstallation test only has the `ocpsriov` label (not `basic`), so using the `basic` filter automatically excludes it:

```bash
# This will only run the 9 basic tests, excluding reinstallation
go test ./tests/ocp/sriov/... -v -ginkgo.v -ginkgo.label-filter="basic" -timeout 60m
```

## Test Cases

The basic test suite includes 9 test cases covering various SR-IOV features:

### 1. SR-IOV VF with Spoof Checking Enabled (Test ID: 25959)

**Description**: Validates SR-IOV Virtual Function with spoof checking enabled.

**What it tests**:
- VF initialization with spoof checking enabled
- Network attachment with spoof checking configuration
- Pod network interface readiness
- Traffic passing through the VF

**Configuration**:
- Spoof checking: `on`
- Trust: default
- Link state: default

**Expected Behavior**:
- VF is created successfully
- Network is attached to pod
- Interface is ready and can pass traffic
- Spoof checking is active on the VF

---

### 2. SR-IOV VF with Spoof Checking Disabled (Test ID: 70820)

**Description**: Validates SR-IOV Virtual Function with spoof checking disabled.

**What it tests**:
- VF initialization with spoof checking disabled
- Network attachment with spoof checking disabled
- Pod network interface functionality

**Configuration**:
- Spoof checking: `off`
- Trust: default
- Link state: default

**Expected Behavior**:
- VF is created successfully
- Network is attached to pod
- Interface is ready and can pass traffic
- Spoof checking is disabled on the VF

---

### 3. SR-IOV VF with Trust Disabled (Test ID: 25960)

**Description**: Validates SR-IOV Virtual Function with trust disabled.

**What it tests**:
- VF initialization with trust disabled
- Network attachment with trust configuration
- Pod network interface functionality

**Configuration**:
- Trust: `off`
- Spoof checking: default
- Link state: default

**Expected Behavior**:
- VF is created successfully
- Network is attached to pod
- Interface is ready and can pass traffic

---

### 4. SR-IOV VF with Trust Enabled (Test ID: 70821)

**Description**: Validates SR-IOV Virtual Function with trust enabled.

**What it tests**:
- VF initialization with trust enabled
- Network attachment with trust configuration
- Pod network interface functionality

**Configuration**:
- Trust: `on`
- Spoof checking: default
- Link state: default

**Expected Behavior**:
- VF is created successfully
- Network is attached to pod
- Interface is ready and can pass traffic

---

### 5. SR-IOV VF with VLAN and Rate Limiting Configuration (Test ID: 25963)

**Description**: Validates SR-IOV Virtual Function with VLAN tagging and rate limiting.

**What it tests**:
- VF initialization with VLAN configuration
- Network attachment with VLAN and rate limiting
- Pod network interface with VLAN support
- Traffic rate limiting functionality

**Configuration**:
- VLAN: `100`
- Min TX rate: `1000` (Mbps)
- Max TX rate: `2000` (Mbps)
- Spoof checking: default
- Trust: default

**Expected Behavior**:
- VF is created successfully
- Network is attached to pod with VLAN configuration
- Interface is ready and can pass traffic
- Rate limiting is applied

**Note**: Some devices (e.g., `bcm57414`, `bcm57508`) may not support `minTxRate` and will be skipped.

---

### 6. SR-IOV VF with Auto Link State (Test ID: 25961)

**Description**: Validates SR-IOV Virtual Function with automatic link state management.

**What it tests**:
- VF initialization with auto link state
- Network attachment with link state configuration
- Pod network interface functionality

**Configuration**:
- Link state: `auto`
- Spoof checking: default
- Trust: default

**Expected Behavior**:
- VF is created successfully
- Network is attached to pod
- Interface is ready and can pass traffic
- Link state is managed automatically

---

### 7. SR-IOV VF with Enabled Link State (Test ID: 71006)

**Description**: Validates SR-IOV Virtual Function with enabled link state.

**What it tests**:
- VF initialization with enabled link state
- Network attachment with link state configuration
- Pod network interface functionality

**Configuration**:
- Link state: `enable`
- Spoof checking: default
- Trust: default

**Expected Behavior**:
- VF is created successfully
- Network is attached to pod
- Interface is ready and can pass traffic
- Link state is explicitly enabled

---

### 8. MTU Configuration for SR-IOV Policy (Test ID: 69646)

**Description**: Validates MTU (Maximum Transmission Unit) configuration for SR-IOV policies.

**What it tests**:
- VF initialization
- SR-IOV policy MTU update
- Network attachment with MTU configuration
- Pod network interface with custom MTU

**Configuration**:
- MTU: `9000` (jumbo frames)
- Spoof checking: default
- Trust: default
- Link state: default

**Expected Behavior**:
- VF is created successfully
- Policy MTU is updated to 9000
- Network is attached to pod
- Interface is ready with custom MTU
- Traffic can pass through the interface

**Note**: MTU value must be in range 1-9192 as per SR-IOV specification.

---

### 9. DPDK SR-IOV VF Functionality Validation (Test ID: 69582)

**Description**: Validates SR-IOV Virtual Function functionality with DPDK (Data Plane Development Kit).

**What it tests**:
- DPDK VF initialization with `vfio-pci` device type
- DPDK network attachment
- DPDK test pod creation
- PCI address extraction and validation
- DPDK pod functionality

**Configuration**:
- Device type: `vfio-pci`
- Spoof checking: default
- Trust: default
- Link state: default

**Expected Behavior**:
- DPDK VF is created successfully
- Network is attached to DPDK pod
- DPDK pod is created and ready
- PCI address is correctly extracted from network status
- DPDK functionality is validated

**Note**: This test requires DPDK support and uses a specialized DPDK test container.

---

## Test Execution Flow

### BeforeAll Hook

1. **SR-IOV Operator Status Check**: Verifies the SR-IOV operator is running
2. **Worker Node Discovery**: Discovers worker nodes for SR-IOV initialization

### Test Execution

Each test case follows this pattern:

1. **Device Iteration**: Tests run for each configured device
2. **VF Initialization**: Creates SR-IOV policy and initializes VF
3. **Network Creation**: Creates SR-IOV network with test-specific configuration
4. **Pod Creation**: Creates test pod with SR-IOV network attachment
5. **Validation**: Verifies interface readiness and traffic passing
6. **Cleanup**: Cleans up resources using `DeferCleanup`

### AfterAll Hook

1. **Policy Cleanup**: Removes all SR-IOV policies created during tests
2. **Stability Wait**: Waits for SR-IOV and MCP to be stable after cleanup

## Test Behavior

### Device Skipping

Tests automatically skip devices that:
- Are not found on any worker node
- Have NO-CARRIER status (no physical link)
- Do not support required features (e.g., `minTxRate`)

### Error Handling

- All errors are properly handled and reported
- Test failures include diagnostic information
- Resources are cleaned up even on test failure (via `DeferCleanup`)

### Resource Management

- Each test creates its own namespace (with test case ID prefix)
- Unique network names prevent conflicts
- All resources are cleaned up after each test
- Policies are cleaned up in `AfterAll` hook
- Leftover resources from previous test runs are automatically cleaned up in `BeforeSuite`
- Existing policies with the same name are removed before creating new ones to prevent VF range conflicts

## Troubleshooting

### Common Issues

#### Test Fails with "SR-IOV operator is not running"

**Solution**: Ensure SR-IOV operator is installed and healthy:
```bash
oc get pods -n openshift-sriov-network-operator
```

#### Test Fails with "no running SR-IOV operator pods found"

**Solution**: Check operator deployment status:
```bash
oc get deployment -n openshift-sriov-network-operator
oc describe deployment -n openshift-sriov-network-operator
```

#### Test Skips All Devices

**Possible Causes**:
- No SR-IOV-capable devices available
- Devices not configured correctly
- Worker nodes don't have SR-IOV interfaces

**Solution**: Verify device configuration and worker node setup.

#### Test Fails with "NO-CARRIER" Status

**Solution**: This is expected for some devices. The test will skip devices with NO-CARRIER status automatically.

#### Test Fails with "VF index range is overlapped with existing policy"

**Solution**: This error occurs when a policy with the same VF range already exists. The test suite automatically cleans up existing policies before creating new ones, but if you encounter this error:

1. Manually delete conflicting policies:
   ```bash
   oc delete sriovnetworknodepolicy <policy-name> -n openshift-sriov-network-operator
   ```

2. Wait for the policy to be fully deleted before rerunning the test

3. The `BeforeSuite` hook also performs automatic cleanup of leftover policies matching common test device names

#### DPDK Test Fails

**Possible Causes**:
- DPDK not supported on the device
- DPDK test container not available
- Insufficient permissions for DPDK

**Solution**: Verify DPDK support and test container availability.

### Debugging

Enable verbose logging:
```bash
export ECO_VERBOSE_LEVEL=100
export ECO_TEST_VERBOSE=true
make run-tests
```

Enable test failure dumps:
```bash
export ECO_DUMP_FAILED_TESTS=true
export ECO_REPORTS_DUMP_DIR=/tmp/sriov-test-logs
make run-tests
```

## Test Reports

### XML Reports

XML reports are generated automatically:
- JUnit report: `{ECO_REPORTS_DUMP_DIR}/sriov_suite_test_junit.xml` (default: `/tmp/reports/sriov_suite_test_junit.xml`)
- Test run report: `{ECO_REPORTS_DUMP_DIR}/sriov_testrun.xml` (default: `/tmp/reports/sriov_testrun.xml`)

The reports directory is automatically created if it doesn't exist.

**Important**: The `ECO_REPORTS_DUMP_DIR` environment variable (maps to `ReportsDirAbsPath` in config) must be set to an **absolute path** to ensure JUnit reports are written to the correct location. If not set or set to a relative path, the JUnit report may be written to the test execution directory (e.g., `tests/ocp/sriov/sriov_suite_test_junit.xml`) instead of the reports directory.

**Example**:
```bash
# Set absolute path for reports (recommended)
export ECO_REPORTS_DUMP_DIR="/tmp/reports"

# Or use a custom location
export ECO_REPORTS_DUMP_DIR="/path/to/custom/reports"
```

To disable XML reports:
```bash
export ECO_ENABLE_REPORT=false
```

### Failure Reports

When tests fail, diagnostic information is collected:
- Pod logs
- Network attachment definitions
- SR-IOV policies and networks
- Events from test namespaces

Reports are saved to `/tmp/reports/` by default (configurable via `ECO_REPORTS_DUMP_DIR`).

## Related Documentation

- [Main SR-IOV Test Suite README](../README.md)
- [Migration Guide](../MIGRATION_GUIDE.md)
- [Compliance Review](../FINAL_COMPLIANCE_REVIEW.md)
- [Test Cases Review](../TEST_CASES_REVIEW.md)

## Support

For issues or questions:
1. Check the troubleshooting section above
2. Review test logs and reports
3. Consult the main SR-IOV test suite documentation
4. Contact the SR-IOV test team

