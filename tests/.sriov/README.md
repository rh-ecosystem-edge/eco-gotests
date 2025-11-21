# SRIOV Basic Tests

This directory contains adapted SRIOV basic tests copied from the OpenShift tests private repository. The tests have been modified to work with the eco-gotests framework and infrastructure.

## Test Files

- `sriov_basic_test.go` - Main test file containing the SRIOV basic test cases
- `helpers.go` - Helper functions for SRIOV test operations
- `testdata/` - Template files and test data

## Test Cases

The following test cases are included:

1. **SR-IOV VF with spoof checking enabled** - Tests SRIOV VF with spoof checking enabled
2. **SR-IOV VF with spoof checking disabled** - Tests SRIOV VF with spoof checking disabled
3. **SR-IOV VF with trust disabled** - Tests SRIOV VF with trust disabled
4. **SR-IOV VF with trust enabled** - Tests SRIOV VF with trust enabled
5. **SR-IOV VF with VLAN and rate limiting configuration** - Tests SRIOV VF with VLAN and rate limiting
6. **SR-IOV VF with auto link state** - Tests SRIOV VF with auto link state
7. **SR-IOV VF with enabled link state** - Tests SRIOV VF with enabled link state
8. **MTU configuration for SR-IOV policy** - Tests SRIOV VF with custom MTU settings
9. **DPDK SR-IOV VF functionality validation** - Tests SRIOV VF with DPDK

## Device Configuration

The tests support both environment variable configuration and default device configurations:

### Environment Variables

#### SRIOV_DEVICES
Set `SRIOV_DEVICES` environment variable with the format:
```bash
export SRIOV_DEVICES="name1:deviceid1:vendor1:interface1,name2:deviceid2:vendor2:interface2,..."
```

Example:
```bash
export SRIOV_DEVICES="e810xxv:159b:8086:ens2f0,e810c:1593:8086:ens2f2"
```

#### ECO_SRIOV_TEST_CONTAINER
Override the default test container image (useful for ARM64/multi-arch support):
```bash
export ECO_SRIOV_TEST_CONTAINER="quay.io/ocp-edge-qe/eco-gotests-network-client:v4.15.2"
```

**Note:** The default image `quay.io/openshift-kni/cnf-tests:4.16` may not support ARM64 architecture. For ARM64 clusters, use an image that supports multi-arch.

**ARM64 Image Options:**
- `quay.io/ocp-edge-qe/eco-gotests-network-client:v4.15.2` - Recommended for ARM64, multi-arch support
- `quay.io/ocp-edge-qe/eco-gotests-network-client:latest` - Latest version (may have newer ARM64 fixes)
- `quay.io/ocp-edge-qe/eco-gotests-network-client:v4.16.0` - If available, newer version

**Troubleshooting ARM64 Issues:**
If containers keep restarting on ARM64:
1. Verify image architecture compatibility:
   ```bash
   docker manifest inspect quay.io/ocp-edge-qe/eco-gotests-network-client:v4.15.2 | grep architecture
   ```
2. Check if the image has `/bin/bash` or `/bin/sh`:
   ```bash
   docker run --rm --entrypoint ls quay.io/ocp-edge-qe/eco-gotests-network-client:v4.15.2 /bin/bash /bin/sh
   ```
3. Try using a different tag or check pod logs:
   ```bash
   kubectl logs <pod-name> -n <namespace> --previous
   ```

### Default Devices
If no environment variable is set, the following default devices are used:
- e810xxv (159b:8086) - ens2f0
- e810c (1593:8086) - ens2f2
- x710 (1572:8086) - ens5f0
- bcm57414 (16d7:14e4) - ens4f1np1
- bcm57508 (1750:14e4) - ens3f0np0
- e810back (1591:8086) - ens4f2
- cx7anl244 (1021:15b3) - ens2f0np0

## Prerequisites

- SRIOV operator must be deployed and running
- Worker nodes must have SRIOV-capable network interfaces
- Test images must be available on the cluster
- Sufficient privileges to create SRIOV policies and networks

## Test Stability and Readiness Checks

The tests include comprehensive stability checks before starting test execution:

### SR-IOV Operator Readiness
- Verifies SR-IOV operator pods are running in the operator namespace
- Confirms SR-IOV CRDs are available by checking `SriovNetwork` resources
- Validates that `SriovNetworkNodeState` objects have been created by the operator

### SR-IOV Node State Synchronization
- Waits for all worker nodes to have `SriovNetworkNodeState` objects populated by the operator
- Validates each node's `SyncStatus` is "Succeeded" (indicates operator has synced config to node)
- Retries if any node state is missing or not fully synced
- Treats empty node states as "not ready" to prevent premature test execution

### MachineConfigPool Stability
- Checks `MachineConfigPool` resources (if available in cluster schema)
- Verifies MCP `Updated=True` condition on worker pools
- Ensures MCP is not in `Degraded` or `Updating` state
- Gracefully falls back to SR-IOV node state sync if MCP check is unavailable
- Retries if MCP conditions indicate pending configuration updates

### Worker Node Readiness
- Verifies all worker nodes have `Ready` condition
- Checks for resource pressure conditions (memory, disk) that indicate instability
- Retries if any node is not ready or has resource pressure

**Default Timeouts:**
- Stability check timeout: 20 minutes (configurable)
- Polling interval: 30 seconds (configurable)
- Timeout is increased automatically if needed based on cluster load

These checks ensure tests only execute when the cluster is in a stable state and SR-IOV is fully operational.

## Running the Tests

### Basic test execution:
```bash
export GOSUMDB=sum.golang.org
export GOTOOLCHAIN=auto
go test ./tests/sriov/... -v
```

### With additional options:
```bash
export GOSUMDB=sum.golang.org
export GOTOOLCHAIN=auto
go test ./tests/sriov/... -v -ginkgo.v -timeout 60m
```

### Run specific tests by label:
```bash
export GOSUMDB=sum.golang.org
export GOTOOLCHAIN=auto
go test ./tests/sriov/... -v -ginkgo.label-filter="Disruptive && Serial" -timeout 60m
```

### Run with debugging options:
```bash
export GOSUMDB=sum.golang.org
export GOTOOLCHAIN=auto
go test ./tests/sriov/... -v -ginkgo.v -ginkgo.trace -timeout 60m
```

**Common Options:**
- `-v`: Verbose output
- `-ginkgo.v`: Ginkgo verbose output (shows detailed test progress)
- `-ginkgo.trace`: Include full stack trace when a failure occurs
- `-timeout 60m`: Sets test timeout to 60 minutes (adjust as needed)
- `-ginkgo.label-filter`: Filter tests by labels (e.g., `"Disruptive && Serial"`, `"!Serial"`)
- `-ginkgo.focus`: Run only tests matching the given regex (e.g., `-ginkgo.focus="DPDK"`)
- `-ginkgo.skip`: Skip tests matching the given regex
- `-ginkgo.keep-going`: Continue running tests even after a failure
- `-ginkgo.fail-fast`: Stop on first failure
- `-ginkgo.reportFile`: Generate test report to specified file (e.g., `-ginkgo.reportFile=test-report.json`)

**Note:** `GOTOOLCHAIN=auto` ensures Go uses the correct toolchain version as specified in `go.mod`. `GOSUMDB=sum.golang.org` enables checksum verification for module downloads.

## Test Data

The `testdata/` directory contains YAML templates for:
- SRIOV network configurations
- DPDK test pod specifications
- Network attachment definitions

## Troubleshooting Stability Checks

If tests fail waiting for stability, check the following:

### "No SR-IOV node states available yet"
- The SR-IOV operator is running but hasn't populated `SriovNetworkNodeState` objects
- **Solution:** Wait for the operator to complete initialization, or check operator logs:
  ```bash
  kubectl logs -n openshift-sriov-network-operator deployment/sriov-network-operator
  ```

### "SR-IOV node not yet synced"
- The operator has created node states but sync is still in progress
- **Solution:** Check individual node state status:
  ```bash
  kubectl get sriovnetworknodestates -n openshift-sriov-network-operator
  kubectl describe sriovnetworknodestate <node-name> -n openshift-sriov-network-operator
  ```

### "MachineConfigPool not yet updated" or "MachineConfigPool is degraded"
- Machine configuration updates are still in progress or have encountered errors
- **Solution:** Check MCP status:
  ```bash
  kubectl get mcp
  kubectl describe mcp worker
  ```

### "Worker node is not ready" or "Node has resource pressure"
- Nodes are experiencing issues or resource constraints
- **Solution:** Inspect node status:
  ```bash
  kubectl get nodes -o wide
  kubectl describe node <node-name>
  kubectl top node  # Check resource usage
  ```

### Increasing Timeout
If the cluster is healthy but needs more time, increase the stability check timeout by setting environment variables:
```bash
export SRIOV_STABILITY_TIMEOUT=1800  # 30 minutes (in seconds)
export SRIOV_STABILITY_INTERVAL=30   # Poll interval in seconds
```

## Notes

- Tests are marked as `[Disruptive]` and `[Serial]` as they modify cluster networking configuration and must run sequentially
- Some tests skip certain device types (e.g., x710, bcm devices) due to hardware limitations
- Tests clean up resources after completion
- DPDK tests require specific hardware support and may be skipped on unsupported platforms
- Comprehensive stability checks prevent test flakiness from races with operator reconciliation
