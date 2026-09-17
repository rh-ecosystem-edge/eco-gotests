# SystemReserved Validation Tests

Functional tests to verify systemReserved kubelet configuration via PerformanceProfile in Telco Core RDS deployments.

## Overview

These tests validate that custom systemReserved CPU and memory values configured via PerformanceProfile patches are:
- Correctly applied to kubelet configuration
- Reflected in node allocatable capacity  
- Adequate for cluster operation per scale lab baseline data

**Related Tasks:**
- CNF-24977: Verify RDS setting of system reserved (this test implementation)
- CNF-16828: systemReserved RDS configuration (the config being tested)

## Test Scope

### Test Cases

1. **PerformanceProfile Configuration** - Verifies PerformanceProfile contains expected systemReserved values
2. **Kubelet Configuration** - Verifies `/etc/node-sizing.env` on nodes reflects systemReserved values
3. **Node Allocatable Capacity** - Verifies node allocatable is correctly reduced by systemReserved
4. **Scale Lab Baseline** - Validates systemReserved values meet scale lab requirements
5. **Enforcement** - Verifies systemReserved enforcement is active

### Target OCP Versions

- OCP 5.0 (initial implementation)
- OCP 4.22, 4.20, 4.18, 4.16 (backports)

## Configuration

### Environment Variables

Override default systemReserved values via environment variables:

```bash
export CONTROL_PLANE_CPU_RESERVED="40"        # Default: 40 cores
export CONTROL_PLANE_MEMORY_RESERVED="30Gi"   # Default: 30Gi
export WORKER_CPU_RESERVED="1000m"            # Default: 1000m (1 core)
export WORKER_MEMORY_RESERVED="11Gi"          # Default: 11Gi
export PERFORMANCE_PROFILE_NAME=""            # Auto-detect if not specified
export TARGET_NODE_ROLE="both"                # master, worker, or both
```

### Default Baseline Values

| Node Type | CPU | Memory | Source |
|-----------|-----|--------|--------|
| Control Plane | 40 cores | 30Gi | Scale lab max 39.1 cores + CNF-16828 |
| Worker | 1000m (1 core) | 11Gi | Telco Core baseline + CNF-16828 |

**Note**: Scale lab data shows control plane memory can exceed 30Gi (33.9 GB observed under max load).

## Prerequisites

### Cluster Requirements

- Spoke MNO (Multi-Node OpenShift) cluster
- Performance Addon Operator (PAO) or Node Tuning Operator (NTO) installed
- PerformanceProfile deployed (via ZTP or manually)
- Nodes in Ready state

### Local Requirements

- Go 1.19+
- `KUBECONFIG` pointing to target cluster
- Network access to cluster

## Running Tests

### All SystemReserved Tests

```bash
cd src/eco-gotests
export KUBECONFIG=/path/to/kubeconfig

go test -v ./tests/cnf/core/performance/systemreserved/... -timeout 30m
```

### Specific Test

```bash
go test -v ./tests/cnf/core/performance/systemreserved/... \
  -run TestSystemReserved \
  -timeout 30m
```

### With Custom Parameters

```bash
export CONTROL_PLANE_CPU_RESERVED="45"
export WORKER_MEMORY_RESERVED="12Gi"

go test -v ./tests/cnf/core/performance/systemreserved/... -timeout 30m
```

### Filtered by Label

```bash
go test -v ./tests/cnf/core/performance/systemreserved/... \
  --ginkgo.label-filter="systemreserved-validation" \
  -timeout 30m
```

## Test Output

### Success Example

```
✓ Node master-0 has correct systemReserved: CPU=40, Memory=30Gi
✓ Node master-1 has correct systemReserved: CPU=40, Memory=30Gi  
✓ Node master-2 has correct systemReserved: CPU=40, Memory=30Gi
✓ Control plane CPU systemReserved (40.0 cores) meets scale lab requirement (39.1 cores)
✓ Control plane memory systemReserved (30.0 GB) meets scale lab requirement (33.9 GB)
✓ Worker CPU systemReserved (1000m) meets minimum requirement (500m)
```

### Warning Example

```
⚠ WARNING: Control plane memory systemReserved (30.0 GB) is BELOW scale lab max (33.9 GB)
⚠ Consider increasing to at least 34Gi for production environments
```

## Verification Methods

Tests use **multiple verification methods** as documented in architecture requirements:

1. **`oc describe node`** - Check Allocatable field
2. **Node Status API** - Programmatic query of allocatable
3. **Node inspection** - Read `/etc/node-sizing.env` via debug pod

## Known Limitations

1. **Scale lab data** - OCP 4.16 baseline data confirmed. Other versions (4.18, 4.20, 4.22, 5.0) baseline data to be integrated as available during backport phase.

2. **Eviction threshold calculation** - Allocatable capacity validation uses approximate 90% threshold calculation. Actual eviction thresholds may vary by cluster configuration. This is acceptable for functional testing; production tuning should use actual cluster eviction settings.

## CI/CD Integration

Tests are designed for integration into eco-gotests CI pipeline:

- Executed on regular cadence against OCP releases prior to GA
- Included in z-stream regression testing suite
- Test lanes for each target OCP version

## References

- **Test Plan**: `TEST_PLAN.md` in workspace root
- **Architecture**: `architecture/` directory
- **Documentation**: `docs/` directory
- **Scale Lab Baseline**: https://docs.google.com/presentation/d/1qbochvGR4N_EToEYG_HBobKZldd6118xeeft1QFlT9A/edit?slide=id.g328e9c3b0b9_0_81
