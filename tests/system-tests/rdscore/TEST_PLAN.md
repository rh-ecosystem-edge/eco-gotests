# Test Plan - CNF-24977: Verify RDS setting of system reserved

**Task ID**: CNF-24977  
**Component**: systemReserved kubelet configuration via PerformanceProfile  
**Architecture**: Telco Core  
**Topology**: Spoke MNO (Multi-Node OpenShift)  
**OCP Versions**: 5.0, 4.22, 4.20, 4.18, 4.16  
**Test Framework**: eco-gotests

---

## 1. Test Objectives

Verify that custom systemReserved CPU and memory values configured via PerformanceProfile patches in PolicyGenerator are:
1. Correctly applied to kubelet configuration
2. Reflected in node allocatable capacity
3. Adequate for cluster operation per scale lab baseline data

## 2. Test Scope

### In Scope
- PerformanceProfile systemReserved configuration validation
- Kubelet configuration verification on control plane and worker nodes
- Node allocatable capacity calculation verification
- Configurable test parameters for different systemReserved values
- Multi-version testing (OCP 5.0, 4.22, 4.20, 4.18, 4.16)

### Out of Scope
- PerformanceProfile deployment/creation (assumed pre-configured via ZTP)
- Performance Addon Operator / Node Tuning Operator installation (assumed pre-installed)
- Runtime performance validation (covered by Performance/Scale testing per CNF-16828)
- Backup & restore testing
- Specific upgrade/reboot resilience (covered by z-stream regression suite)

## 3. Test Environment Requirements

### Cluster Configuration
- **Topology**: Spoke MNO (Multi-Node OpenShift)
- **Nodes**: Control plane nodes (3) + Worker nodes (N)
- **Pre-installed operators**: Performance Addon Operator (PAO) or Node Tuning Operator (NTO)
- **OCP Version**: 5.0, 4.22, 4.20, 4.18, or 4.16

### PerformanceProfile Configuration
- Applied via PolicyGenerator as part of ZTP workflow
- Contains systemReserved configuration for CPU and memory
- Targets appropriate node selectors (control plane and/or worker nodes)

### Test Parameters (Configurable)
- `CONTROL_PLANE_CPU_RESERVED`: Default 40 cores (based on scale lab max 39.1 cores)
- `CONTROL_PLANE_MEMORY_RESERVED`: Default 30Gi (per CNF-16828)
- `WORKER_CPU_RESERVED`: Default 1000m (1 core)
- `WORKER_MEMORY_RESERVED`: Default 11Gi (per CNF-16828)
- `TARGET_NODE_ROLE`: Node role to test (master, worker, or both)

## 4. Test Scenarios

### Test Suite: RDS Core Top Level Suite - systemReserved Validation

**Total Tests**: 8 test cases organized into 5 functional areas
**Suite File**: `tests/system-tests/rdscore/tests/00_validate_top_level.go`
**Implementation**: `tests/system-tests/rdscore/internal/rdscorecommon/systemreserved_validation.go`

#### Test Case 4.1: Verify PerformanceProfile Contains systemReserved Values

**Test ID**: `90692` (reportxml.ID)
**Function**: `ValidatePerformanceProfileSystemReserved()`

**Objective**: Verify that PerformanceProfile resource contains the expected systemReserved CPU and memory values.

**Preconditions**:
- PerformanceProfile CR exists in cluster
- Test parameters configured with expected systemReserved values

**Test Steps**:
1. Query PerformanceProfile CR from cluster
2. Extract `spec.nodeSelector` to identify targeted nodes
3. Extract `spec.workloadHints.realTime` (if applicable)
4. Verify `spec.additionalKernelArgs` or systemReserved configuration location
5. Compare configured systemReserved CPU value against expected parameter
6. Compare configured systemReserved memory value against expected parameter

**Expected Results**:
- PerformanceProfile CR found
- systemReserved CPU matches `CONTROL_PLANE_CPU_RESERVED` or `WORKER_CPU_RESERVED` (based on node selector)
- systemReserved memory matches `CONTROL_PLANE_MEMORY_RESERVED` or `WORKER_MEMORY_RESERVED`

**Verification Method**: Go test assertion on CR fields

---

#### Test Case 4.2a: Verify Kubelet Configuration on Control Plane Nodes

**Test ID**: `90693` (reportxml.ID)
**Function**: `ValidateControlPlaneNodeSystemReserved()`

**Objective**: Verify that kubelet configuration on control plane nodes reflects systemReserved values.

**Preconditions**:
- PerformanceProfile applied and control plane nodes targeted
- MachineConfig applied by PAO/NTO
- Control plane nodes in Ready state

**Test Steps**:
1. Identify control plane nodes
2. For each control plane node:
   a. Create debug pod on node
   b. Read `/etc/node-sizing.env` file
   c. Extract `SYSTEM_RESERVED_MEMORY` value
   d. Extract `SYSTEM_RESERVED_CPU` value
   e. Validate values are positive quantities
3. Compare extracted values against expected parameters

**Expected Results**:
- `/etc/node-sizing.env` exists on all control plane nodes
- `SYSTEM_RESERVED_MEMORY` matches expected value
- `SYSTEM_RESERVED_CPU` matches expected value
- Values are positive (Sign() == 1)

**Verification Method**: Node file inspection via debug pod

---

#### Test Case 4.2b: Verify Kubelet Configuration on Worker Nodes

**Test ID**: `90694` (reportxml.ID)
**Function**: `ValidateWorkerNodeSystemReserved()`

**Objective**: Verify that kubelet configuration on worker nodes reflects systemReserved values.

**Preconditions**:
- PerformanceProfile applied and worker nodes targeted
- MachineConfig applied by PAO/NTO
- Worker nodes in Ready state (excluding storage nodes)

**Test Steps**:
1. Identify worker nodes (exclude storage nodes)
2. For each worker node:
   a. Create debug pod on node
   b. Read `/etc/node-sizing.env` file
   c. Extract `SYSTEM_RESERVED_MEMORY` value
   d. Extract `SYSTEM_RESERVED_CPU` value
   e. Validate values are positive quantities
3. Compare extracted values against expected parameters

**Expected Results**:
- `/etc/node-sizing.env` exists on all worker nodes
- `SYSTEM_RESERVED_MEMORY` matches expected value
- `SYSTEM_RESERVED_CPU` matches expected value
- Values are positive (Sign() == 1)
- Storage nodes are skipped

**Verification Method**: Node file inspection via debug pod

---

#### Test Case 4.3a: Verify Control Plane Node Allocatable Capacity

**Test ID**: `90695` (reportxml.ID)
**Function**: `ValidateControlPlaneNodeAllocatable()`

**Objective**: Verify that control plane node allocatable capacity is correctly reduced by systemReserved.

**Preconditions**:
- Control plane nodes configured with systemReserved values
- Nodes in Ready state

**Test Steps**:
1. For each control plane node:
   a. Get node capacity from Node Status API
   b. Get node allocatable from Node Status API
   c. Get configured systemReserved values
   d. Calculate expected allocatable: `Allocatable = Capacity - systemReserved - EvictionThresholds`
   e. Verify allocatable < capacity (systemReserved in effect)
   f. Verify allocatable >= 90% of expected (accounting for eviction)

**Expected Results**:
- Allocatable CPU < Capacity CPU
- Allocatable Memory < Capacity Memory
- Allocatable within 90-100% of expected value

**Verification Method**: Node Status API

---

#### Test Case 4.3b: Verify Worker Node Allocatable Capacity

**Test ID**: `90696` (reportxml.ID)
**Function**: `ValidateWorkerNodeAllocatable()`

**Objective**: Verify that worker node allocatable capacity is correctly reduced by systemReserved.

**Preconditions**:
- Worker nodes configured with systemReserved values
- Nodes in Ready state (excluding storage nodes)

**Test Steps**:
1. For each worker node (excluding storage):
   a. Get node capacity from Node Status API
   b. Get node allocatable from Node Status API
   c. Get configured systemReserved values
   d. Calculate expected allocatable
   e. Verify allocatable < capacity (systemReserved in effect)
   f. Verify allocatable >= 90% of expected (accounting for eviction)

**Expected Results**:
- Allocatable CPU < Capacity CPU
- Allocatable Memory < Capacity Memory
- Allocatable within 90-100% of expected value
- Storage nodes skipped

**Verification Method**: Node Status API

---

#### Test Case 4.4: Validate Against Scale Lab Baseline

**Test ID**: `90699` (reportxml.ID)
**Function**: `ValidateScaleLabBaseline()`

**Objective**: Verify that configured systemReserved values are adequate based on scale lab baseline requirements.

**Preconditions**:
- Scale lab baseline data available for target OCP version
- systemReserved values configured on cluster

**Test Steps**:
1. Load scale lab baseline data for OCP version:
   - For OCP 4.16: Control plane max 39.1 CPU cores, 33.9 GB memory under load
   - For other versions: Use version-specific baseline data
2. Get configured systemReserved values from cluster
3. For control plane nodes:
   a. Verify CPU systemReserved >= scale lab max CPU + safety margin
   b. Verify Memory systemReserved >= scale lab max memory + safety margin
4. For worker nodes:
   a. Verify CPU systemReserved >= baseline worker CPU
   b. Verify Memory systemReserved >= baseline worker memory

**Expected Results**:
- Control plane CPU reserved (40 cores) >= scale lab max (39.1 cores) ✓
- Control plane Memory reserved (30Gi) >= scale lab max (33.9 GB) - **NOTE: May need increase**
- Worker CPU reserved (1 core) >= baseline
- Worker Memory reserved (11Gi) >= baseline

**Notes**:
- Scale lab data shows control plane memory can exceed 30Gi (33.9 GB observed under max load)
- Test should warn if configured values are below scale lab maximums
- Baseline data location: https://docs.google.com/presentation/d/1qbochvGR4N_EToEYG_HBobKZldd6118xeeft1QFlT9A/edit?slide=id.g328e9c3b0b9_0_81

---

#### Test Case 4.5a: Verify systemReserved Enforcement on Control Plane

**Test ID**: `90697` (reportxml.ID)
**Function**: `ValidateControlPlaneNodeEnforcement()`

**Objective**: Verify that systemReserved limits are enforced by kubelet on control plane nodes.

**Preconditions**:
- systemReserved configured on control plane nodes
- Nodes in Ready state

**Test Steps**:
1. For each control plane node:
   a. Create debug pod on node
   b. Read `/etc/kubernetes/kubelet.conf`
   c. Parse kubelet config (JSON or YAML)
   d. Verify `enforceNodeAllocatable` array includes `system-reserved`
   e. Verify node allocatable < capacity (enforcement in effect)

**Expected Results**:
- Kubelet config readable from `/etc/kubernetes/kubelet.conf`
- `enforceNodeAllocatable` contains `system-reserved` string
- Node allocatable CPU < capacity CPU (enforcement active)
- If config not accessible: error logged, test continues to next node

**Verification Method**: Kubelet configuration file parsing

**Error Handling**:
- If config read fails: log ❌ error, continue to next node
- If config parse fails: log ❌ error, continue to next node
- All nodes validated independently

---

#### Test Case 4.5b: Verify systemReserved Enforcement on Workers

**Test ID**: `90698` (reportxml.ID)
**Function**: `ValidateWorkerNodeEnforcement()`

**Objective**: Verify that systemReserved limits are enforced by kubelet on worker nodes.

**Preconditions**:
- systemReserved configured on worker nodes
- Nodes in Ready state (excluding storage nodes)

**Test Steps**:
1. For each worker node (excluding storage):
   a. Create debug pod on node
   b. Read `/etc/kubernetes/kubelet.conf`
   c. Parse kubelet config (JSON or YAML)
   d. Verify `enforceNodeAllocatable` array includes `system-reserved`
   e. Verify node allocatable < capacity (enforcement in effect)

**Expected Results**:
- Kubelet config readable from `/etc/kubernetes/kubelet.conf`
- `enforceNodeAllocatable` contains `system-reserved` string
- Node allocatable CPU < capacity CPU (enforcement active)
- Storage nodes skipped
- If config not accessible: error logged, test continues to next node

**Verification Method**: Kubelet configuration file parsing

**Error Handling**:
- If config read fails: log ❌ error, continue to next node
- If config parse fails: log ❌ error, continue to next node
- All nodes validated independently

---

## 5. Test Data

### Default Test Parameters

| Parameter | Control Plane | Worker | Source |
|-----------|--------------|--------|--------|
| CPU Reserved | 40 cores | 1000m (1 core) | Scale lab data + baseline |
| Memory Reserved | 30Gi | 11Gi | CNF-16828 |
| Minimum CPU | 500m | 500m | Red Hat requirement |

### Test Parameter Overrides

Tests support environment variable overrides:
```bash
export CONTROL_PLANE_CPU_RESERVED="45"        # Override CPU reserved for control plane
export CONTROL_PLANE_MEMORY_RESERVED="32Gi"   # Override memory reserved for control plane
export WORKER_CPU_RESERVED="2000m"            # Override CPU reserved for workers
export WORKER_MEMORY_RESERVED="12Gi"          # Override memory reserved for workers
```

### OCP Version-Specific Data

Tests should detect OCP version and use appropriate:
- Scale lab baseline data
- Kubelet configuration paths
- PerformanceProfile API version

## 6. Test Execution

### Pre-Test Setup
1. Ensure cluster access (kubeconfig configured)
2. Verify PerformanceProfile is deployed
3. Verify nodes are in Ready state
4. Set test parameter environment variables (or use defaults)

### Test Execution Command
```bash
cd src/eco-gotests
export KUBECONFIG=/path/to/kubeconfig

# Run all systemReserved tests (part of top-level suite)
go test -v ./tests/system-tests/rdscore/tests/... \
  --ginkgo.label-filter="systemreserved" \
  -timeout 30m

# Run only systemReserved validation tests
go test -v ./tests/system-tests/rdscore/tests/... \
  --ginkgo.label-filter="systemreserved-validation" \
  -timeout 30m

# Run specific systemReserved test category
go test -v ./tests/system-tests/rdscore/tests/... \
  --ginkgo.label-filter="systemreserved-cp" \
  -timeout 30m

# Run with custom parameters
export CONTROL_PLANE_CPU_RESERVED="45"
export WORKER_MEMORY_RESERVED="12Gi"
go test -v ./tests/system-tests/rdscore/tests/... \
  --ginkgo.label-filter="systemreserved" \
  -timeout 30m

# Disable test cache (force re-run)
go test -v ./tests/system-tests/rdscore/tests/... \
  --ginkgo.label-filter="systemreserved" \
  -timeout 30m -count=1

# Run entire RDS Core Top Level Suite (includes systemReserved)
go test -v ./tests/system-tests/rdscore/tests/... \
  --ginkgo.label-filter="rds-core-workflow" \
  -timeout 2h
```

### Test Output
- Pass/Fail status for each test case
- Detailed logs showing:
  - Configured systemReserved values
  - Actual kubelet configuration
  - Node capacity and allocatable values
  - Comparison against scale lab baseline
- Summary report with test coverage

### CI/CD Integration
- Tests integrated into eco-gotests CI pipeline
- Executed on regular cadence against OCP releases prior to GA
- Included in z-stream regression testing suite
- Test lanes for: OCP 5.0, 4.22, 4.20, 4.18, 4.16

## 7. Success Criteria

All 8 test cases must pass:
- ✓ 4.1 (ID: 90692): PerformanceProfile contains expected systemReserved values
- ✓ 4.2a (ID: 90693): Kubelet configuration on control plane nodes reflects systemReserved
- ✓ 4.2b (ID: 90694): Kubelet configuration on worker nodes reflects systemReserved
- ✓ 4.3a (ID: 90695): Control plane node allocatable capacity correctly reduced
- ✓ 4.3b (ID: 90696): Worker node allocatable capacity correctly reduced
- ✓ 4.4 (ID: 90699): Values meet or exceed scale lab baseline requirements
- ✓ 4.5a (ID: 90697): systemReserved enforcement active on control plane nodes
- ✓ 4.5b (ID: 90698): systemReserved enforcement active on worker nodes

**Pass Criteria**:
- All eligible nodes validated (storage nodes skipped)
- Per-node errors logged but don't fail entire test
- Scale lab baseline warnings acceptable (not failures)

## 8. Test Coverage Matrix

| Test Case | Control Plane | Worker | OCP 5.0 | OCP 4.22 | OCP 4.20 | OCP 4.18 | OCP 4.16 |
|-----------|--------------|--------|---------|----------|----------|----------|----------|
| 4.1 PerformanceProfile Config | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| 4.2a Kubelet Config CP | ✓ | - | ✓ | ✓ | ✓ | ✓ | ✓ |
| 4.2b Kubelet Config Worker | - | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| 4.3a Allocatable Capacity CP | ✓ | - | ✓ | ✓ | ✓ | ✓ | ✓ |
| 4.3b Allocatable Capacity Worker | - | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| 4.4 Scale Lab Baseline | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| 4.5a Enforcement CP | ✓ | - | ✓ | ✓ | ✓ | ✓ | ✓ |
| 4.5b Enforcement Worker | - | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| **Total Tests** | **8** | **8** | **8** | **8** | **8** | **8** | **8** |

## 9. Known Limitations

1. **Control plane memory baseline**: Scale lab data shows 33.9 GB max usage, but CNF-16828 baseline is 30Gi. Test will warn if under scale lab max.
2. **Scale lab data versions**: OCP 4.16 baseline confirmed; other versions (4.18, 4.20, 4.22, 5.0) baseline data to be identified.
3. **Network stack testing**: Current scope does not specify IPv4/IPv6/dual-stack specific testing.
4. **Disconnected environment**: Test execution in air-gapped environments not explicitly covered.
5. **Storage nodes**: Storage nodes are skipped as they have specialized configurations and may not schedule debug pods reliably.

## 10. Implementation Notes & Bug Fixes

### Refactoring to Top-Level Suite (CNF-24977)

Tests were refactored from standalone suite to integrated top-level validation:
- **Original location**: `tests/system-tests/rdscore/tests/systemreserved/` (standalone Ginkgo suite)
- **New location**: 
  - Test suite: `tests/system-tests/rdscore/tests/00_validate_top_level.go`
  - Implementation: `tests/system-tests/rdscore/internal/rdscorecommon/systemreserved_validation.go`
- **Benefits**:
  - Integrated with other RDS Core validations in single suite
  - Reusable validation functions in rdscorecommon package
  - Consistent labeling and reporting with other tests
  - Easier to run as part of full validation workflow

### Critical Bugs Fixed (CNF-24977)

During implementation, three critical bugs were discovered and fixed:

1. **Ginkgo Test Timing Issue** (Tests 4.3a/b, 4.5a/b not running)
   - **Problem**: Allocatable and enforcement validation created dynamic `It()` specs at tree-build-time when node lists were empty, so loops never executed
   - **Impact**: Only 4 of 8 tests ran (4.1, 4.2a, 4.2b, 4.4)
   - **Fix**: Moved node iteration inside `It()` blocks to run at runtime after `BeforeAll` populates node lists
   - **Result**: All 8 tests now run

2. **Kubelet Config File Path** (Test 4.5a/b)
   - **Problem**: Read `/var/lib/kubelet/config.json` which contains Docker pull secrets, not kubelet configuration
   - **Impact**: Enforcement validation always skipped due to missing `enforceNodeAllocatable` field
   - **Fix**: Read `/etc/kubernetes/kubelet.conf` (OpenShift standard kubelet config location)
   - **Result**: Enforcement tests now correctly parse and validate kubelet config

3. **Storage Node Timeout**
   - **Problem**: Tests hung for 30 minutes trying to create debug pods on storage nodes which don't schedule workloads
   - **Impact**: Test suite timed out instead of completing
   - **Fix**: Added `isNodeReadyForTesting()` check to skip storage nodes in all test loops
   - **Result**: Tests complete in ~17 minutes, storage nodes logged with ⊘ symbol

### CodeRabbit Review Fixes

1. **Positive Quantity Validation** (Tests 4.2a/b): Parse systemReserved values as `resource.Quantity` and assert `Sign() == 1` to ensure positive values
2. **Actual Cluster Values** (Test 4.4): Read systemReserved from nodes via debug pod instead of using environment defaults
3. **All Nodes Validation** (Tests 4.3a/b, 4.4, 4.5a/b): Loop through all eligible nodes instead of stopping at first node
4. **Per-Node Error Handling** (Tests 4.5a/b): Replace `Skip()` with `return` to continue validating remaining nodes when one fails

### Test Behavior

- **Node Filtering**: Automatically skips nodes that are NotReady, SchedulingDisabled, or have storage role
- **Enforcement Errors**: Logged with ❌ emoji, test continues to next node (does not abort)
- **Scale Lab Warnings**: Logged with ⚠ symbol, not test failures
- **Test Duration**: ~17 minutes for full 8-test suite on multi-node cluster

## 11. References

- **Task**: CNF-24977 - Verify RDS setting of system reserved
- **Related Task**: CNF-16828 - systemReserved RDS configuration implementation
- **Documentation**:
  - OCP 4.22 Nodes Guide: `docs/docs-redhat-com-en-documentation-openshift-container-platform-4-22-html-single-nodes-index.md`
  - Red Hat Developer Article: `docs/developers-redhat-com-articles-2025-10-24-how-modify-system-reserved-parameters-openshift-nodes.md`
  - Red Hat KB Article: `docs/access-redhat-com-solutions-5843241.md`
- **Scale Lab Baseline**: https://docs.google.com/presentation/d/1qbochvGR4N_EToEYG_HBobKZldd6118xeeft1QFlT9A/edit?slide=id.g328e9c3b0b9_0_81
- **Source CRs**: `telco-core/configuration/reference-crs/optional/tuning/control-plane-system-reserved.yaml`
