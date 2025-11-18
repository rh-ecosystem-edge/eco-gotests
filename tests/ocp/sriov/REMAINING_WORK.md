# Remaining Migration Work - Phased Plan

## Overview

This document outlines the remaining work to complete the SR-IOV test suite migration, broken down into manageable phases.

## Phase 7: Refactor Core Helper Functions (Priority 1)
**Goal**: Refactor helper functions needed for the first test case to work

### Functions to Refactor:
1. **`initVF()`** - Initializes VF on nodes
   - **Current**: Uses `GinkgoLogr`, `By()`, `Eventually()`, `Expect()`
   - **Target**: Return `(bool, error)`, use `glog`, use `wait.PollUntilContextTimeout`
   - **Priority**: HIGH - Required for test case 25959

2. **`chkVFStatusWithPassTraffic()`** - Verifies VF status with traffic
   - **Current**: Uses `GinkgoLogr`, `By()`, `Eventually()`, `Expect()`
   - **Target**: Return `error`, use `glog`, use `wait.PollUntilContextTimeout`
   - **Priority**: HIGH - Required for test case 25959

3. **`chkSriovOperatorStatus()`** - Checks SR-IOV operator status
   - **Current**: Uses `GinkgoLogr`, `By()`, `Expect()`
   - **Target**: Return `error`, use `glog`
   - **Priority**: MEDIUM - Used in BeforeAll hook

4. **`waitForSriovPolicyReady()`** - Waits for SR-IOV policy to be ready
   - **Current**: Uses `GinkgoLogr`, `By()`, `Eventually()`, `Expect()`
   - **Target**: Return `error`, use `glog`, use `wait.PollUntilContextTimeout`
   - **Priority**: MEDIUM - Used in AfterAll hook

**Estimated Effort**: Medium
**Dependencies**: None
**Deliverable**: First test case (25959) fully functional

---

## Phase 8: Complete Basic Test Cases Migration
**Goal**: Migrate remaining basic test cases (spoof check, trust, VLAN, link state)

### Test Cases to Migrate:
1. **Test ID: 70820** - "SR-IOV VF with spoof checking disabled"
2. **Test ID: 25960** - "SR-IOV VF with trust disabled"
3. **Test ID: 70821** - "SR-IOV VF with trust enabled"
4. **Test ID: 25963** - "SR-IOV VF with VLAN and rate limiting configuration"
5. **Test ID: 25961** - "SR-IOV VF with auto link state"
6. **Test ID: 71006** - "SR-IOV VF with enabled link state"

**Estimated Effort**: Low (can follow pattern from Phase 7)
**Dependencies**: Phase 7 complete
**Deliverable**: All basic test cases migrated and functional

---

## Phase 9: Refactor Advanced Helper Functions
**Goal**: Refactor helper functions needed for advanced test cases

### Functions to Refactor:
1. **`initDpdkVF()`** - Initializes DPDK VF
   - **Current**: Uses `GinkgoLogr`, `By()`, `Eventually()`, `Expect()`
   - **Target**: Return `(bool, error)`, use `glog`, use `wait.PollUntilContextTimeout`
   - **Priority**: MEDIUM - Required for DPDK test

2. **`createSriovTestPod()`** - Creates SR-IOV test pod
   - **Current**: Uses `GinkgoLogr`, `By()`, `Expect()`
   - **Target**: Return `error`, use `glog`
   - **Priority**: MEDIUM - Required for DPDK test

3. **`deleteSriovTestPod()`** - Deletes SR-IOV test pod
   - **Current**: Uses `GinkgoLogr`, `By()`, `Expect()`
   - **Target**: Return `error`, use `glog`
   - **Priority**: MEDIUM - Required for DPDK test

4. **`getPciAddress()`** - Gets PCI address from pod
   - **Current**: Uses `GinkgoLogr`
   - **Target**: Return `(string, error)`, use `glog`
   - **Priority**: MEDIUM - Required for DPDK test

5. **`verifyWorkerNodesReady()`** - Verifies worker nodes are ready
   - **Current**: Uses `GinkgoLogr`, `By()`
   - **Target**: Return `error`, use `glog`
   - **Priority**: LOW - Optional helper

**Estimated Effort**: Medium
**Dependencies**: None
**Deliverable**: All helper functions refactored

---

## Phase 10: Complete Advanced Test Cases Migration
**Goal**: Migrate remaining advanced test cases (MTU, DPDK)

### Test Cases to Migrate:
1. **Test ID: 69646** - "MTU configuration for SR-IOV policy"
2. **Test ID: 69582** - "DPDK SR-IOV VF functionality validation"

**Estimated Effort**: Low (can follow pattern from previous phases)
**Dependencies**: Phase 9 complete
**Deliverable**: All test cases migrated and functional

---

## Phase 11: Final Testing and Validation
**Goal**: Ensure all tests work correctly and fix any issues

### Tasks:
1. Run all test cases to verify functionality
2. Fix any compilation errors
3. Fix any runtime errors
4. Verify test IDs are correct
5. Verify cleanup works properly
6. Update documentation if needed

**Estimated Effort**: Medium
**Dependencies**: Phases 7-10 complete
**Deliverable**: Fully functional test suite

---

## Recommended Execution Order

1. **Phase 7** (Core helpers) - Enables first test case
2. **Phase 8** (Basic tests) - Completes basic functionality
3. **Phase 9** (Advanced helpers) - Enables advanced tests
4. **Phase 10** (Advanced tests) - Completes all test cases
5. **Phase 11** (Testing) - Final validation

## Notes

- Each phase can be done independently once dependencies are met
- Helper functions can be refactored incrementally
- Test cases follow the same pattern, so migration is straightforward once helpers are ready
- All code must follow the rules in `.cursorrules`

