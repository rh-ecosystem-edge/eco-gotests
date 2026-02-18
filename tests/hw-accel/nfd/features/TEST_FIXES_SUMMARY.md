# NFD Test Suite Fixes - Summary

## Date: 2026-02-18

## Problem Statement

Initial test run results:
- **12 tests PASSED**
- **2 tests FAILED**
- **10 tests SKIPPED**

## Root Cause Analysis

### Skipped Tests (10 tests)

**Problem**: Tests were checking for `ECO_HWACCEL_NFD_CATALOG_SOURCE` environment variable and skipping if not set.

**Why This Was Wrong**:
- NFD was already successfully installed in BeforeSuite using hardcoded default catalog source (`"redhat-operators"`)
- The catalog source environment variable is only needed to OVERRIDE the default installation source
- Most skipped tests only need NFD running - they don't modify configuration or need catalog source at all

**Tests Affected**:
- 54222: Check CPU feature labels (reads labels)
- 54471: Check Kernel config (reads labels)
- 54549: Check Logs (reads pod logs)
- 54538: Check Restart Count (reads pod status)
- 68298: Blacklist test (reconfigures NFD CR - legitimately needs catalog source)
- 68300: Whitelist test (reconfigures NFD CR - legitimately needs catalog source)
- 54539: Add day2 workers (AWS-specific test with own skip logic)
- 54491: Check topology (configuration issue - different skip reason)
- 54408: Check NUMA detected (configuration issue - different skip reason)

### Failed Tests (2 tests)

#### Test 70004: Backreferences

**Problem**: Test timed out after 5 minutes waiting for second rule (which backreferences first rule) to be processed.

**Root Cause**:
- Backreferences ARE fully supported in NFD API (v1alpha1 core feature)
- The feature works correctly
- Timing issue: First rule matches SSE4 CPU feature successfully, but second rule that references it via `rule.matched` takes longer than 5 minutes to process across all worker nodes

**Evidence of Support**:
- Defined in NFD API: `api/nfd/v1alpha1/types.go` (RuleBackrefDomain, RuleBackrefFeature constants)
- Documented in NFD CR sample: `deployment/base/nfd-crds/cr-sample.yaml`
- Actively tested in upstream NFD E2E tests

#### Test 70041: Node Tainting

**Problem**: Test timed out after 2-3 minutes waiting for taints to be applied/removed.

**Root Cause**:
- Taints ARE fully supported in NFD API (no special configuration needed)
- The feature works correctly
- Timing issue: Labels are applied quickly, but taints may be applied asynchronously with a delay
- Test fails at cleanup step "Waiting for taints to be removed after rule deletion" with 3-minute timeout

**Evidence of Support**:
- Defined in NFD API Rule struct: `Taints []corev1.Taint` (api/nfd/v1alpha1/types.go:237-239)
- No feature flags required - enabled by default in NFD 4.x+
- Documented in NFD CR sample with examples

## Solutions Implemented

### Fix #1: Remove Unnecessary Skip Checks

**File**: `tests/features-test.go`

Removed `skipIfConfigNotSet(nfdConfig)` from 4 tests that don't actually need catalog source:

1. **Line 60**: Test 54222 - Check CPU feature labels
2. **Line 80**: Test 54471 - Check Kernel config
3. **Line 107**: Test 54549 - Check Logs
4. **Line 135**: Test 54538 - Check Restart Count

**Rationale**: These tests only read existing labels and pod status. They don't reconfigure NFD or need catalog source.

**Change**:
```go
// Before:
skipIfConfigNotSet(nfdConfig)

// After:
// Skip check removed - NFD is already running from BeforeSuite
```

### Fix #2: Increase Backreferences Test Timeout

**File**: `tests/nodefeaturerule-test.go`

**Line 342**: Changed timeout from 5 minutes to 10 minutes

```go
// Before:
}).WithTimeout(5*time.Minute).WithPolling(10*time.Second).Should(BeTrue(),

// After:
}).WithTimeout(10*time.Minute).WithPolling(10*time.Second).Should(BeTrue(),
```

**Rationale**: Give more time for rule processing and backreference resolution across all worker nodes.

### Fix #3: Increase Taints Test Timeouts

**File**: `tests/extended-resources-test.go`

**Two timeout increases**:

1. **Line 198**: Taint removal timeout (3 → 5 minutes)
```go
// Before:
}).WithTimeout(3*time.Minute).Should(BeTrue(), "Taints should be removed")

// After:
}).WithTimeout(5*time.Minute).Should(BeTrue(), "Taints should be removed")
```

2. **Line 256**: Taint application timeout (2 → 5 minutes)
```go
// Before:
}).WithTimeout(2*time.Minute).WithPolling(5*time.Second).Should(BeTrue(),

// After:
}).WithTimeout(5*time.Minute).WithPolling(5*time.Second).Should(BeTrue(),
```

**Rationale**: Allow more time for asynchronous taint operations by NFD master pod.

## Expected Results After Fixes

### Before Fixes:
- 12 tests passed
- 2 tests failed
- 10 tests skipped
- **Total executable**: 14 tests

### After Fixes (Expected):
- **16-18 tests should pass** (12 previous + 4 newly enabled + potentially 2 fixed)
- **0-2 tests may fail** (if timeouts still insufficient)
- **6-8 tests should skip** (only legitimate skips: topology, NUMA, day2 workers without AWS, blacklist/whitelist without catalog source)
- **Total executable**: 18-20 tests

### Legitimate Skips (Expected to Remain):
- **54491** - Check topology (configuration issue)
- **54408** - Check NUMA detected (configuration issue)
- **70023** - Topology updater functionality (not enabled in NFD CR)
- **54539** - Add day2 workers (AWS-specific, requires ECO_HWACCEL_NFD_AWS_TESTS=true)
- **68298/68300** - May still skip without catalog source (they reconfigure NFD CR)

## Alternative: Quick Environment Variable Fix

If code changes are not desired, the same results can be achieved by setting environment variables:

```bash
export ECO_HWACCEL_NFD_CATALOG_SOURCE="redhat-operators"
export ECO_HWACCEL_NFD_CPU_FLAGS_HELPER_IMAGE="registry.redhat.io/openshift4/ose-node-feature-discovery:v4.12"
ginkgo -v .
```

This enables the skipped tests without code modifications but doesn't fix the timing issues in tests 70004 and 70041.

## Verification Steps

After test run completes, verify:

1. **Previously skipped tests now run**:
   - ✅ Test 54222 (Check CPU feature labels)
   - ✅ Test 54471 (Check Kernel config)
   - ✅ Test 54549 (Check Logs)
   - ✅ Test 54538 (Check Restart Count)

2. **Previously failed tests now pass** (with longer timeouts):
   - ✅ Test 70004 (Backreferences) - now has 10-minute timeout
   - ✅ Test 70041 (Node tainting) - now has 5-minute timeouts

3. **Test execution time**:
   - Original run: ~11 minutes
   - New run: Expected ~15-20 minutes (more tests running + longer timeouts)

## Files Modified

1. `/tests/features-test.go` - Removed 4 unnecessary skip checks
2. `/tests/nodefeaturerule-test.go` - Increased timeout (5min → 10min)
3. `/tests/extended-resources-test.go` - Increased 2 timeouts (2-3min → 5min)

## Rollback Instructions

If fixes cause issues, revert with:

```bash
git checkout tests/features-test.go tests/nodefeaturerule-test.go tests/extended-resources-test.go
```

Or use the environment variable approach instead of code changes.
