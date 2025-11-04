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

### Environment Variable
Set `SRIOV_DEVICES` environment variable with the format:
```
SRIOV_DEVICES="name1:deviceid1:vendor1:interface1,name2:deviceid2:vendor2:interface2,..."
```

Example:
```
SRIOV_DEVICES="e810xxv:159b:8086:ens2f0,e810c:1593:8086:ens2f2"
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

## Running the Tests

```bash
go test ./tests/sriov/... -v
```

## Test Data

The `testdata/` directory contains YAML templates for:
- SRIOV network configurations
- DPDK test pod specifications
- Network attachment definitions

## Notes

- Tests are marked as `[Disruptive]` and `[Serial]` as they modify cluster networking configuration and must run sequentially
- Some tests skip certain device types (e.g., x710, bcm devices) due to hardware limitations
- Tests clean up resources after completion
- DPDK tests require specific hardware support and may be skipped on unsupported platforms
