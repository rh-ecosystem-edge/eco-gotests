# Enhanced Failure Reporting for RDS Core Tests

This document explains the enhanced failure reporting capabilities added to RDS Core tests and how to use the k8sreporter framework for debugging test failures.

## Overview

This enhancement adds comprehensive resource status dumping when tests fail, making it easier to diagnose issues with pods, deployments, and statefulsets without needing to manually inspect the cluster after failures.

## Changes Implemented

### Enhanced Reporter Configuration

**File:** `tests/system-tests/rdscore/internal/rdscoreparams/rdscorevars.go`

**What Changed:**

1. **Fixed Namespace Configuration** - Replaced demo namespaces with actual RDS Core test namespaces:
   - SR-IOV workloads: `rds-sriov-wlkd`
   - Storage (ODF/Ceph): `rds-cephfs-ns`, `rds-cephrbd-ns`, `rds-cephrbd-block-ns`
   - Whereabouts: `rds-whereabouts`
   - MACVLAN: `rds-macvlan`
   - IPVLAN: `rds-ipvlan`
   - EgressIP: `rds-egressip-ns-one`, `rds-egressip-ns-two`
   - Egress Service: `rds-egress-ns`
   - MetalLB/FRR: `rds-metallb-supporttools-ns`, `openshift-frr-k8s`
   - NROP: `rds-nrop`
   - Pod-level bonding: `rds-pod-level-bond`
   - Rootless DPDK: `rds-dpdk`
   - OpenShift networking:  `openshift-multus`

2. **Enhanced Resource Types** - Added critical resource types for better debugging:
   ```go
   ReporterCRDsToDump = []k8sreporter.CRData{
       {Cr: &corev1.PodList{}},           // Pods
       {Cr: &appsv1.DeploymentList{}},     // Deployments
       {Cr: &appsv1.StatefulSetList{}},    // StatefulSets
       {Cr: &appsv1.ReplicaSetList{}},     // ReplicaSets
       {Cr: &corev1.EventList{}},          // Events (critical!)
   }
   ```

**Why This Matters:**
- **Events** are crucial for understanding scheduling failures (e.g., "0/7 nodes available")
- **Deployments/StatefulSets** show replica readiness and rollout status
- **ReplicaSets** help trace pod ownership and version issues

### Resource Status Dump Functions

**File:** `tests/system-tests/rdscore/internal/rdscorecommon/resource-status-dump.go` (NEW)

Three new helper functions for comprehensive debugging:

#### 1. `DumpPodStatusOnFailure(podBuilder *pod.Builder, err error)`

**Purpose:** Automatically dumps detailed pod status when `WaitUntilReady()` fails

**Information Dumped:**
- Pod phase (Pending, Running, Failed, etc.)
- Pod conditions with reasons and messages
- Container statuses (Ready, Waiting, Terminated)
- Init container statuses (often cause of Pending state)
- Scheduling information (node assignment, nomination)
- Owner references (traces back to Deployment/StatefulSet)
- Resource requirements (helps diagnose scheduling failures)

**Usage Example:**
```go
// After calling WaitUntilReady
err := podTwo.WaitUntilReady(3 * time.Minute)

// Dump detailed status if it failed
if err != nil {
    DumpPodStatusOnFailure(podTwo, err)
}

Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Pod %q in %q ns is not Ready",
    podTwo.Definition.Name, podTwo.Definition.Namespace))
```

**Output Example:**
```text
========================================
Pod "rdscore-sriov2-two-8b47b4-5rwz6" in namespace "rds-sriov-wlkd" Failed to Become Ready
========================================
Pod Phase: Pending

Pod Conditions:
  - Type: PodScheduled, Status: False, Reason: Unschedulable
    Message: 0/7 nodes are available: 3 node(s) didn't match pod affinity rules, 3 node(s) had untolerated taint(s)...

Container Statuses:
  - Name: testpmd-container, Ready: false, RestartCount: 0
    State: Waiting
    Reason: ContainerCreating

Scheduling: Pod has not been scheduled to any node

Owner References:
  - Kind: ReplicaSet, Name: rdscore-sriov2-two-8b47b4, Controller: true

Resource Requirements:
  Container testpmd-container:
    Requests:
      openshift.io/sriovnic: 2
      cpu: 4
      memory: 1Gi
========================================
```

#### 2. `DumpDeploymentStatus(ctx SpecContext, namespace string)`

**Purpose:** Dumps all Deployment statuses in a namespace (useful in AfterEach hooks)

**Information Dumped:**
- Replica counts (Desired, Current, Ready, Available, Updated, Unavailable)
- Deployment conditions (Available, Progressing, ReplicaFailure)
- Update strategy (RollingUpdate, Recreate)
- Pod selector labels

**Usage Example:**
```go
AfterEach(func(ctx SpecContext) {
    if CurrentSpecReport().Failed() {
        By("Dumping deployment status due to test failure")
        DumpDeploymentStatus(ctx, "rds-sriov-wlkd")
    }
})
```

#### 3. `DumpStatefulSetStatus(ctx SpecContext, namespace string)`

**Purpose:** Dumps all StatefulSet statuses in a namespace

**Information Dumped:**
- Replica counts (Desired, Current, Ready, Updated)
- Current and update revisions
- StatefulSet conditions
- Update strategy
- Service name and selector labels

**Usage Example:**
```go
AfterEach(func(ctx SpecContext) {
    if CurrentSpecReport().Failed() {
        By("Dumping statefulset status due to test failure")
        DumpStatefulSetStatus(ctx, "rds-whereabouts")
    }
})
```

## How k8sreporter Works

### Architecture

k8sreporter is a Go library that dumps Kubernetes cluster state to the filesystem for post-test analysis.

**GitHub Repository:** https://github.com/openshift-kni/k8sreporter

### Core Components

1. **KubernetesReporter** - Main object that manages dumping operations
2. **Namespace Filter** - Selects which namespaces to dump
3. **CRData** - Specifies which Custom Resources to collect
4. **AddToScheme** - Extends the runtime scheme for custom CRDs

### How It's Integrated

```go
// In rds_suite_test.go
var _ = JustAfterEach(func() {
    reporter.ReportIfFailed(
        CurrentSpecReport(),
        currentFile,
        rdscoreparams.ReporterNamespacesToDump,  // ← Our enhanced config
        rdscoreparams.ReporterCRDsToDump)        // ← Our enhanced config
})
```

### Dump Trigger Flow

```text
Test Fails
    ↓
JustAfterEach Hook Executes
    ↓
reporter.ReportIfFailed() Checks Failure State
    ↓
Creates k8sreporter.KubernetesReporter
    ↓
Calls reporter.Dump(duration, folderName)
    ↓
Collects Resources from Configured Namespaces
    ↓
Writes to Filesystem at {ReportsDirAbsPath}/failed_{testname}/{test_full_text}/
```

### Configuration

Reporter behavior is controlled by environment variables or YAML config:

```bash
# Enable dump on test failure
export ECO_DUMP_FAILED_TESTS=true

# Specify dump directory
export ECO_REPORTS_DUMP_DIR=/tmp/test-reports

# Set logging verbosity
export ECO_VERBOSE_LEVEL=100
```

**Default Config:** `tests/internal/config/default.yaml`

## Checking k8sreporter Results

### 1. Locate Dump Directory

The dump location is determined by:
- Environment variable: `ECO_REPORTS_DUMP_DIR`
- YAML config: `reports_dump_dir`
- Default: Usually project root or `/tmp`

### 2. Directory Structure

```text
{ReportsDirAbsPath}/
└── failed_{testname}/
    └── {Test_Description_With_Underscores}/
        ├── nodes.log                                    # All cluster nodes (JSON)
        ├── events.log                                   # Kubernetes events
        ├── rds-sriov-wlkd_rdscore-sriov2-two-xxx_pods_logs.log
        ├── rds-sriov-wlkd_rdscore-sriov2-two-xxx_pods_specs.log
        ├── rds-sriov-wlkd_deployments.log               # All deployments in namespace
        ├── rds-sriov-wlkd_statefulsets.log              # All statefulsets
        ├── rds-sriov-wlkd_replicasets.log               # All replicasets
        ├── rds-sriov-wlkd_events.log                    # Namespace-specific events
        └── pod_exec_logs.log                            # Custom pod execution logs
```

### 3. Example: Investigating Test ID 80423 Failure

**Test:** "Verifies SR-IOV workloads on different nodes and same SR-IOV network post reboot"

**Failure:** Pod `rdscore-sriov2-two-8b47b4-5rwz6` not ready

**Investigation Steps:**

1. **Find the dump directory:**
   ```bash
   cd {ReportsDirAbsPath}/failed_rds_suite_test
   ls -la
   # Look for folder matching test description
   cd "Verifies_SR-IOV_workloads_on_different_nodes_..."
   ```

2. **Check pod spec and logs:**
   ```bash
   # Pod specification (JSON)
   jq '.' rds-sriov-wlkd_rdscore-sriov2-two-*_pods_specs.log

   # Pod logs
   cat rds-sriov-wlkd_rdscore-sriov2-two-*_pods_logs.log
   ```

3. **Check events for scheduling issues:**
   ```bash
   # Namespace-specific events
   cat rds-sriov-wlkd_events.log | grep -i "failed\|error\|insufficient"

   # Common patterns:
   # - "Insufficient openshift.io/sriovnic"
   # - "Failed to pull image"
   # - "0/N nodes available"
   ```

4. **Check deployment status:**
   ```bash
   # View deployment conditions
   jq '.[] | {name: .metadata.name, replicas: .status, conditions: .status.conditions}' \
      rds-sriov-wlkd_deployments.log
   ```

5. **Check node resources:**
   ```bash
   # See allocatable resources on all nodes
   jq '.[] | {name: .metadata.name, allocatable: .status.allocatable}' nodes.log
   ```

### 4. Common Failure Patterns and Where to Look

| Failure Pattern | Check These Files | Look For |
|----------------|-------------------|----------|
| Pod stuck in Pending | `events.log`, pod specs | Scheduling reasons, resource requests, node taints |
| Pod CrashLoopBackOff | Pod logs, pod specs | Container exit codes, OOMKilled, application errors |
| ImagePullBackOff | `events.log`, pod specs | Image name/tag, registry credentials |
| Deployment not progressing | `deployments.log`, `replicasets.log` | Deployment conditions, ReplicaSet status |
| StatefulSet pods not ready | `statefulsets.log`, `events.log` | Revision mismatches, PVC issues |
| Network connectivity issues | Pod logs, `events.log` | CNI errors, IP allocation failures |
| SR-IOV resource unavailable | `nodes.log`, `events.log` | Allocatable SR-IOV resources, device plugin status |

### 5. Automated Analysis Tips

```bash
# Find all failed pods
jq -r '.[] | select(.status.phase != "Running" and .status.phase != "Succeeded") |
  "\(.metadata.namespace)/\(.metadata.name): \(.status.phase)"' *_pods_specs.log

# Find all pods with high restart counts
jq -r '.[] | select(.status.containerStatuses[]?.restartCount > 0) |
  "\(.metadata.name): \(.status.containerStatuses[0].restartCount) restarts"' *_pods_specs.log

# Extract all error/warning events
jq -r '.[] | select(.type == "Warning" or .type == "Error") |
  "[\(.type)] \(.involvedObject.name): \(.message)"' events.log

# Check node pressure conditions
jq -r '.[] | .status.conditions[] | select(.status == "True" and .type != "Ready") |
  "\(.type): \(.message)"' nodes.log
```

## Integration with Existing Tests

### For Existing Test Cases

**Before:**
```go
err = podTwo.WaitUntilReady(3 * time.Minute)
Expect(err).ToNot(HaveOccurred(),
    fmt.Sprintf("Pod %q in %q ns is not Ready", podTwo.Definition.Name, podTwo.Definition.Namespace))
```

**After (Recommended):**
```go
err = podTwo.WaitUntilReady(3 * time.Minute)

if err != nil {
    DumpPodStatusOnFailure(podTwo, err)
}

Expect(err).ToNot(HaveOccurred(),
    fmt.Sprintf("Pod %q in %q ns is not Ready", podTwo.Definition.Name, podTwo.Definition.Namespace))
```

### For New Test Cases

Consider adding resource dumps in AfterEach hooks for comprehensive failure debugging:

```go
AfterEach(func(ctx SpecContext) {
    if CurrentSpecReport().Failed() {
        By("Dumping resource status due to test failure")

        // Dump node status (already exists)
        rdscorecommon.DumpNodeStatus(ctx)

        // NEW: Dump deployment status
        rdscorecommon.DumpDeploymentStatus(ctx, "rds-sriov-wlkd")

        // NEW: Dump statefulset status (if applicable)
        // rdscorecommon.DumpStatefulSetStatus(ctx, "rds-whereabouts")
    }

    // Cleanup
    By("Ensure all nodes are Ready")
    rdscorecommon.EnsureInNodeReadiness(ctx)
})
```

## Benefits

1. **Reduced Debugging Time** - Immediate visibility into failure root causes
2. **No Manual Cluster Inspection** - All relevant data captured automatically
3. **Historical Analysis** - Dumps preserved for post-mortem review
4. **Better Bug Reports** - Include dumps in bug reports for faster triage
5. **Pattern Recognition** - Easily identify recurring failure patterns
6. **CI/CD Integration** - Dumps available in Jenkins artifacts

## Ginkgo Integration

The `DumpPodStatusOnFailure` function uses Ginkgo's `AddReportEntry` with `ReportEntryVisibilityFailureOrVerbose`:

```go
AddReportEntry(
    fmt.Sprintf("Pod %s Failure Details", podName),
    debugInfo,
    ReportEntryVisibilityFailureOrVerbose,  // Only shown on failure or with -v
)
```

This means:
- Report entries appear in JUnit XML output
- Only visible in console when tests fail or run with `--vv`
- Structured data for CI/CD parsing

## References

- **k8sreporter GitHub:** https://github.com/openshift-kni/k8sreporter
- **Ginkgo v2 Documentation:** https://onsi.github.io/ginkgo/
- **Ginkgo AddReportEntry:** https://pkg.go.dev/github.com/onsi/ginkgo/v2#AddReportEntry
- **eco-goinfra Library:** https://github.com/rh-ecosystem-edge/eco-goinfra
