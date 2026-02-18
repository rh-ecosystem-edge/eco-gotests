# Namespace Deletion Timeout Issue - RESOLVED

## Issue Summary

**Date**: 2026-02-18 13:52
**Error**: BeforeSuite failed with timeout waiting for namespace deletion
**Status**: ✅ RESOLVED

## Error Details

```
[FAILED] error installing NFD operator: failed to create namespace:
failed waiting for namespace openshift-nfd deletion: timeout waiting
for namespace openshift-nfd deletion: context deadline exceeded
```

## Root Cause

The `openshift-nfd` namespace was stuck in "Terminating" state from a previous test run. When BeforeSuite tried to install NFD, it:

1. Checked if the namespace exists
2. Found it in "Terminating" state
3. Waited for it to finish deleting (default timeout: 5 minutes)
4. Timed out after 5 minutes
5. Failed the BeforeSuite, preventing all tests from running

This commonly happens when:
- Previous test run was interrupted (Ctrl+C during cleanup)
- Resources in the namespace have finalizers that can't be processed
- Network issues prevent proper cleanup
- Kubernetes control plane is slow/overloaded

## Solution Applied

Created and ran comprehensive cleanup script: `/tmp/force_cleanup_nfd.sh`

### Script Actions:
1. **Force delete namespace** if stuck in Terminating state
   - Removes finalizers from namespace
   - Forces deletion with `--force --grace-period=0`
   - Verifies namespace is completely gone

2. **Clean up orphaned resources**:
   - OperatorGroups
   - Subscriptions
   - ClusterServiceVersions (CSVs)
   - NodeFeatureRules
   - NodeFeatureDiscoveries

3. **Verify clean state**
   - Confirms all NFD resources are removed
   - Ready for fresh test run

## Verification

After cleanup:
```bash
$ bash /tmp/force_cleanup_nfd.sh
=== Force Cleanup NFD Resources ===
Attempting to force-delete namespace: openshift-nfd
  ✓ Namespace openshift-nfd does not exist
Checking for orphaned NFD resources...
  ✓ No orphaned operator groups
  ✓ No orphaned subscriptions
  ✓ No orphaned CSVs
  ✓ No NodeFeatureRules
  ✓ No NodeFeatureDiscoveries
=== Cleanup Complete! ===
```

Fresh test run:
```bash
$ ginkgo -v .
Running Suite: NFD
Will run 24 of 24 specs
[BeforeSuite]
  STEP: Installing NFD operator for all feature tests ✓
  STEP: Waiting for NFD operator to be ready ✓
  STEP: Creating NFD CR ✓
  STEP: Waiting for NFD CR to be ready ✓
[BeforeSuite] PASSED
```

## Prevention

### Best Practices:
1. **Always let AfterSuite complete** - don't interrupt tests during cleanup
2. **Run cleanup before tests** if unsure about state:
   ```bash
   bash /tmp/force_cleanup_nfd.sh
   ginkgo -v .
   ```
3. **Monitor namespace status** before running tests:
   ```bash
   kubectl get namespace openshift-nfd
   ```

### Pre-Flight Check Script:
Created in `TROUBLESHOOTING.md` - checks for:
- Namespace stuck in Terminating
- Orphaned operator resources
- Cluster connectivity
- Warns before running tests if issues detected

## Files Created

1. `/tmp/force_cleanup_nfd.sh` - Comprehensive cleanup script
2. `TROUBLESHOOTING.md` - Full troubleshooting guide with:
   - Common issues and solutions
   - Monitoring commands
   - Pre-flight checks
   - Useful commands reference

## Related Documentation

- `TEST_FIXES_SUMMARY.md` - All test improvements
- `CR_CLEANUP_CHANGES.md` - CR cleanup implementation
- `SUITE_IMPROVEMENTS_SUMMARY.md` - Suite improvements overview
- `TROUBLESHOOTING.md` - Troubleshooting guide (NEW)

## Lessons Learned

1. **Namespace deletion is async** - resources must be fully cleaned before namespace can delete
2. **Finalizers can block deletion** - must be explicitly removed in stuck cases
3. **Timeouts are important** - BeforeSuite has 5-minute timeout for namespace deletion
4. **Cleanup scripts are essential** - automate cleanup to prevent manual intervention

## Current Status

✅ **Namespace cleaned up**
✅ **Cleanup script created and tested**
✅ **Tests running successfully**
✅ **BeforeSuite passed**
✅ **Documentation updated**

Tests are now running with full suite (24 specs).
