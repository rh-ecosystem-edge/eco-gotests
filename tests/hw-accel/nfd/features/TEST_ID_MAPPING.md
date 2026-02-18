# NFD Test ID Mapping

## Official OCP- Test IDs (from Test Plan)

This document maps the current test implementation to the official OCP- test IDs from the NFD test plan.

### ✅ Implemented Tests with Official IDs

| Official ID | Test Name | File | Line | Status |
|-------------|-----------|------|------|--------|
| OCP-54222 | Check CPU feature labels | features-test.go | 57 | ✅ PASS |
| OCP-54471 | Check Kernel config | features-test.go | 77 | ✅ PASS |
| OCP-54549 | Check Logs | features-test.go | 104 | ✅ PASS |
| OCP-54538 | Check Restart Count | features-test.go | 132 | ✅ PASS |
| OCP-68298 | Verify CPU Blacklist | features-test.go | 164 | ⏭️ SKIP (needs env var) |
| OCP-68300 | Verify CPU Whitelist | features-test.go | 223 | ⏭️ SKIP (needs env var) |
| OCP-54539 | Add day2 workers | features-test.go | 286 | ⏭️ SKIP (needs AWS) |
| OCP-54408 | NUMA topology tests | features-test.go | 380 | ⏭️ SKIP (topology not enabled) |
| OCP-54491 | NodeResourceTopology | features-test.go | 399 | ⏭️ SKIP (topology not enabled) |

### ⚠️ Implemented Tests WITHOUT Official ID

**These tests need to be either:**
1. Mapped to existing OCP- IDs if they match test plan requirements
2. Added to the test plan and assigned new OCP- IDs
3. Left with empty ID until test plan is updated

| Current Test Name | File | Line | Suggested Mapping | Notes |
|-------------------|------|------|-------------------|-------|
| Validates matchExpressions operators | nodefeaturerule-test.go | 33 | OCP-54472? | Part of Node Feature Rules testing |
| Validates labelsTemplate dynamic label generation | nodefeaturerule-test.go | 115 | OCP-54489? | Templating test |
| Validates matchAny OR logic | nodefeaturerule-test.go | 185 | OCP-54472? | Part of Node Feature Rules testing |
| Validates backreferences from previous rules | nodefeaturerule-test.go | 264 | **OCP-54493** | Exact match! |
| Validates CRUD lifecycle | nodefeaturerule-test.go | 370 | OCP-54472? | Part of Node Feature Rules testing |
| Extended resources from NodeFeatureRule | extended-resources-test.go | 24 | *No ID* | Not in test plan - leave empty |
| Node tainting based on features | extended-resources-test.go | 131 | *No ID* | Not in test plan - leave empty |
| NFD master pod restart | resilience-test.go | ~30 | OCP-54536? | Part of stability testing |
| NFD worker pod restart | resilience-test.go | ~80 | OCP-54536? | Part of stability testing |
| Label persistence after restart | resilience-test.go | ~130 | OCP-54536? | Part of stability testing |
| Master pod restart | resilience-test.go | ~180 | OCP-54536? | Part of stability testing |
| NodeFeature GC test | resilience-test.go | ~230 | *No ID* | Not in test plan - leave empty |
| Device discovery tests | device-discovery-test.go | various | OCP-54412? | PCI device labeling |

### ❌ Missing Tests from Official Test Plan

**These tests are in the test plan but NOT implemented:**

| Official ID | Test Name | Priority | Notes |
|-------------|-----------|----------|-------|
| OCP-54412 | PCI device labeling | High | Needs hardware/config |
| OCP-54461 | USB device labeling | Medium | Needs USB devices |
| OCP-54462 | SRIOV networking | High | Needs SRIOV hardware |
| OCP-54464 | RDMA testing | Medium | Needs RDMA hardware |
| OCP-54472 | Node Feature Rules | **Partial** | Some tests exist, need official ID mapping |
| OCP-54474 | Custom Features from operand | Low | |
| OCP-54475 | Local files integration | Medium | |
| OCP-54477 | Local shell scripts | Medium | |
| OCP-54487 | Local custom config | Low | |
| OCP-54489 | Label templating | **Partial** | labelsTemplate test exists |
| OCP-54492 | Vars (variables) | Low | |
| OCP-54493 | Backreferences | **✅ Implemented** | nodefeaturerule-test.go:264 |
| OCP-54536 | Stability tests | **Partial** | Resilience tests cover some |
| OCP-54548 | NFD pods verification | High | Should verify pod count/status |
| OCP-54540 | Cluster upgrade | Low | Requires cluster upgrade |
| OCP-54550 | Metrics testing | Medium | Prometheus metrics |

## Recommended Actions

### 1. Update Test IDs (Immediate)

**File: `nodefeaturerule-test.go`**
- Line 264: Add `reportxml.ID("54493")` to backreferences test (exact match)
- Lines 33, 185, 370: Add empty ID or map to OCP-54472 when test plan is updated

**File: `extended-resources-test.go`**
- Lines 24, 131: Leave ID empty (not in official test plan)

**File: `resilience-test.go`**
- All tests: Consider mapping to OCP-54536 or leave empty

### 2. Add Missing High-Priority Tests

1. **OCP-54548** - NFD pods verification
   - Verify correct number of pods (1 master + N workers for OCP >= 4.12)
   - Check pod status and restarts
   - File: Create new `nfd-pods-test.go`

2. **OCP-54550** - Metrics testing
   - Query NFD Prometheus metrics
   - Verify `nfd_build_info` and `nfd_degraded_info`
   - File: Create new `metrics-test.go`

3. **OCP-54412** - PCI device labeling
   - Requires hardware with PCI devices
   - File: Extend `device-discovery-test.go`

### 3. Improve Test Plan Alignment

**Tests that need clarification in test plan:**
- Extended resources testing (our test at extended-resources-test.go:24)
- Node tainting (our test at extended-resources-test.go:131)
- Device discovery specifics

**Recommendation**: Either:
1. Add these tests to the official test plan with new OCP- IDs
2. Remove them if not needed
3. Leave IDs empty until test plan is updated

## Test Execution by Official ID

### Run Specific Official Test
```bash
# Run by official ID (54XXX or 68XXX)
ginkgo -v . --focus="54222"  # CPU features
ginkgo -v . --focus="54471"  # Kernel config
ginkgo -v . --focus="68298"  # Blacklist
ginkgo -v . --focus="68300"  # Whitelist
ginkgo -v . --focus="54493"  # Backreferences (when ID is added)
```

### Run All Implemented Official Tests
```bash
ginkgo -v . --focus="54222|54471|54549|54538|68298|68300|54539|54408|54491"
```

### Run Tests Without Official IDs
```bash
# These are custom tests not in official test plan
ginkgo -v . --focus="matchExpressions|labelsTemplate|matchAny|CRUD|Extended resources|tainting|resilience"
```

## Summary Statistics

- **Total Official Test IDs in Plan**: 24
- **Implemented with Official ID**: 9 (37.5%)
- **Implemented without Official ID**: ~12 tests
- **Missing from Plan**: 15 (62.5%)
- **Passing Tests**: 15/17 executed (88.2%)

## Next Steps

1. ✅ **Update backreferences test** to use OCP-54493
2. ⏳ **Decide on unmapped tests** - map to existing IDs or leave empty
3. ⏳ **Add missing high-priority tests** (OCP-54548, OCP-54550)
4. ⏳ **Update test plan** to include new tests or remove unsupported ones
5. ⏳ **Document hardware requirements** for tests that need specific hardware

---

**Note**: Test IDs should match the official NFD test plan. For new tests not in the plan, leave the ID field empty until the test plan is updated with an official OCP- ID.
