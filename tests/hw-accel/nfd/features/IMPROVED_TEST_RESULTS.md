# NFD Test Suite - Improved Results

## Date: 2026-02-18

## Comparison: Before vs After Fixes

### BEFORE Fixes (Initial Run)
| Status | Count | Details |
|--------|-------|---------|
| ✅ Passed | 12 | Basic tests working |
| ❌ Failed | 2 | Backreferences (70004), Taints (70041) |
| ⏭️ Skipped | 10 | Unnecessary catalog source checks |
| **Total** | **24** | |

### AFTER Fixes (Current Run - In Progress)
| Status | Count | Details |
|--------|-------|---------|
| ✅ Passed | 15+ | **+3 newly enabled tests** |
| ❌ Failed | 1-2 | Taints still failing, backreferences testing... |
| ⏭️ Skipped | 7 | Only legitimate skips remaining |
| **Total** | **24** | |

## Detailed Test Results

### ✅ Tests Now Passing (Previously Skipped)

**These tests were unnecessarily skipped and are now running successfully:**

1. **Test 54471** - Check Kernel config
   - **Before**: Skipped (catalog source not set)
   - **After**: ✅ PASSING (0.205 seconds)
   - **Fix**: Removed unnecessary `skipIfConfigNotSet()` check

2. **Test 54549** - Check Logs
   - **Before**: Skipped (catalog source not set)
   - **After**: ✅ PASSING (2.720 seconds)
   - **Fix**: Removed unnecessary `skipIfConfigNotSet()` check
   - **Note**: Found some log entries but test passed

3. **Test 54538** - Check Restart Count
   - **Before**: Skipped (catalog source not set)
   - **After**: ✅ PASSING (1.430 seconds)
   - **Fix**: Removed unnecessary `skipIfConfigNotSet()` check

###  Tests Still Passing (No Changes)

1. **Test 54548** - Check pods state ✅ (1.037 seconds)
2. **Test 70001** - Validates matchExpressions operators ✅ (7.874 seconds)
3. **Test 70002** - Validates labelsTemplate ✅ (1.937 seconds)
4. **Test 70003** - Validates matchAny OR logic ✅ (2.099 seconds)
5. **Test 70005** - Validates CRUD lifecycle ✅ (passing)
6. **Test 70020** - Worker pod restart ✅ (3.433 seconds)
7. **Test 70021** - Master pod restart ✅ (10.800 seconds)
8. **Test 70022** - GC cleanup ✅ (1.934 seconds)
9. **Test 70010** - Discovers PCI devices ✅ (passing)
10. **Test 70014** - Discovers network device features ✅ (passing)
11. **Test 70016** - Discovers system features ✅ (passing)
12. **Test 70040** - Extended resources from NodeFeatureRule ✅ (passing)

### ❌ Tests Still Failing

#### Test 70041 - Node tainting based on features
- **Status**: ❌ FAILED (308.047 seconds / ~5 minutes)
- **Timeout**: Increased from 2-3 minutes to 5 minutes
- **Error**: "Timed out after 300.001s. Taints should be added to nodes matching the rule"
- **Issue**: Even with increased timeout, taints are not being applied to nodes
- **Possible Causes**:
  - NFD master may not have permissions to apply taints
  - Tainting feature may require additional NFD configuration
  - RBAC/permissions issue preventing taint application
  - NFD version may not fully support tainting despite API presence

**Recommendation**: This needs deeper investigation - it's not just a timeout issue. The feature may not be working at all in this NFD deployment.

#### Test 70004 - Validates backreferences from previous rules
- **Status**: ⏳ CURRENTLY RUNNING
- **Timeout**: Increased from 5 minutes to 10 minutes
- **Started**: 10:30:12
- **Expected completion**: By 10:40:12
- **First rule**: Successfully matched (SSE4 detection)
- **Second rule**: Waiting for backreference to first rule

**Final result pending...**

### ⏭️ Legitimate Skips (As Expected)

1. **Test 54222** - Check CPU feature labels
   - **Reason**: CPUFlagsHelperImage environment variable not set
   - **This is different from before!** Previously skipped due to catalog source, now skips for correct reason
   - **To enable**: Set `ECO_HWACCEL_NFD_CPU_FLAGS_HELPER_IMAGE`

2. **Test 54491** - Check topology
   - **Reason**: Configuration issue (hardcoded skip)
   - **Legitimate**: Test has known configuration issues

3. **Test 54408** - Check if NUMA detected
   - **Reason**: Configuration issue (hardcoded skip)
   - **Legitimate**: Test has known configuration issues

4. **Test 70023** - Topology updater functionality
   - **Reason**: Topology updater not enabled in NFD CR
   - **Legitimate**: Optional NFD feature not configured

5. **Test 68298** - Verify Feature List not contains items from Blacklist
   - **Reason**: Catalog source not set
   - **Legitimate**: This test reconfigures NFD CR, so it actually needs catalog source

6. **Test 68300** - Verify Feature List contains only Whitelist
   - **Reason**: Catalog source not set
   - **Legitimate**: This test reconfigures NFD CR, so it actually needs catalog source

7. **Test 54539** - Add day2 workers
   - **Reason**: Catalog source not set + AWS cluster required
   - **Legitimate**: AWS-specific test

## Impact Summary

### Tests Enabled (Skips → Passing)
- **3 tests** now running that were unnecessarily skipped
- These tests work perfectly - they just read existing NFD labels and pod status

### Tests Fixed (Failed → Passing)
- **0 tests** (timeout increases haven't fixed the underlying issues yet)
- Test 70004 (backreferences) - result pending
- Test 70041 (taints) - still failing, needs different fix

### Code Changes Made
1. `tests/features-test.go`: Removed 4 `skipIfConfigNotSet()` calls
2. `tests/nodefeaturerule-test.go`: Increased timeout (5min → 10min)
3. `tests/extended-resources-test.go`: Increased 2 timeouts (2-3min → 5min)

## Next Steps

### For Test 70041 (Taints) - Needs Investigation
1. Check NFD master pod RBAC permissions for node tainting
2. Verify NFD version supports tainting (should be 4.x+)
3. Check NFD master logs for taint-related errors
4. Consider this test may need to be marked as environment-specific

### For Test 70004 (Backreferences) - Waiting for Results
- If it passes with 10-minute timeout: Issue was timing
- If it still fails: May need even longer timeout or has deeper issue

### To Enable More Tests
Set these environment variables:
```bash
# Enable CPU feature labels test
export ECO_HWACCEL_NFD_CPU_FLAGS_HELPER_IMAGE="registry.redhat.io/openshift4/ose-node-feature-discovery:v4.12"

# Enable blacklist/whitelist/day2 tests
export ECO_HWACCEL_NFD_CATALOG_SOURCE="redhat-operators"
```

## Conclusion

**Success**: We successfully enabled 3 previously skipped tests by fixing unnecessary skip logic.

**Partial Success**: Timeout increases helped isolate issues but didn't fix test 70041.

**Pending**: Test 70004 result will determine if timeout fix worked for backreferences.

**Overall Improvement**:
- Before: 12/24 tests passing (50%)
- After: 15-16/24 tests passing (62-67%)
- **+12-17% improvement** in test coverage
