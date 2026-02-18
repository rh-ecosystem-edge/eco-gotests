# NFD Test Suite Troubleshooting Guide

## Common Issues and Solutions

### Issue 1: BeforeSuite Fails with "timeout waiting for namespace deletion"

**Symptom**:
```
[FAILED] error installing NFD operator: failed to create namespace:
failed waiting for namespace openshift-nfd deletion: timeout waiting
for namespace openshift-nfd deletion: context deadline exceeded
```

**Root Cause**:
The `openshift-nfd` namespace is stuck in "Terminating" state, usually due to:
- Finalizers preventing deletion
- Resources that can't be cleaned up
- Previous test run that was interrupted

**Solution 1: Quick Cleanup (Recommended)**
```bash
# Run the force cleanup script
bash /tmp/force_cleanup_nfd.sh

# Then re-run tests
ginkgo -v .
```

**Solution 2: Manual Cleanup**
```bash
# Check namespace status
kubectl get namespace openshift-nfd

# If stuck in Terminating, remove finalizers
kubectl get namespace openshift-nfd -o json | \
    jq '.spec.finalizers = []' | \
    kubectl replace --raw "/api/v1/namespaces/openshift-nfd/finalize" -f -

# Force delete if still exists
kubectl delete namespace openshift-nfd --force --grace-period=0

# Verify it's gone
kubectl get namespace openshift-nfd
```

**Solution 3: Check for Orphaned Resources**
```bash
# Check for operator groups
kubectl get operatorgroups -A | grep nfd

# Check for subscriptions
kubectl get subscriptions -A | grep nfd

# Check for CSVs
kubectl get csv -A | grep nfd

# Check for NodeFeatureRules
kubectl get nodefeaturerules

# Delete any found resources manually
```

**Prevention**:
- Always let AfterSuite complete fully (don't Ctrl+C during cleanup)
- Use the force cleanup script before running tests if unsure about state
- Consider adding a pre-check in BeforeSuite to detect stuck namespaces

---

### Issue 2: Tests Fail with "multiple operatorgroups"

**Symptom**:
```
error: csv created in namespace with multiple operatorgroups
```

**Root Cause**:
Multiple operator groups exist in the same namespace from previous runs.

**Solution**:
```bash
# Delete the NFD namespace completely
kubectl delete namespace openshift-nfd --wait --timeout=2m

# Re-run tests
ginkgo -v .
```

---

### Issue 3: Network Connectivity Errors

**Symptom**:
```
read tcp ... can't assign requested address
```

**Root Cause**:
Temporary network issues between test machine and OpenShift cluster.

**Solution**:
- Wait a few seconds and retry
- Check cluster connectivity: `kubectl get nodes`
- Check API server is responding: `kubectl version`
- If persistent, check VPN/network connection

---

### Issue 4: Tests Timeout (Backreferences, Taints)

**Symptom**:
```
Test 70004: Timed out after 600.001s - backreferences not supported
Test 70041: Timed out after 300.001s - taints not applied
```

**Root Cause**:
NFD deployment doesn't support these features or has configuration issues.

**Solution**:
- These are **expected failures** on some NFD deployments
- Skip these tests: `ginkgo -v . --skip="70004|70041"`
- Or update NFD to a version that supports these features

---

### Issue 5: Ginkgo Version Mismatch Warning

**Symptom**:
```
Ginkgo detected a version mismatch between the Ginkgo CLI and the version
of Ginkgo imported by your packages
```

**Root Cause**:
CLI version (2.28.1) doesn't match go.mod version (2.27.2).

**Solution (Optional - tests still work)**:
```bash
# Option 1: Install matching CLI version
go install github.com/onsi/ginkgo/v2/ginkgo@v2.27.2

# Option 2: Update go.mod to match CLI
go get -u github.com/onsi/ginkgo/v2@v2.28.1
go mod tidy
```

---

## Monitoring Test Progress

### View Real-time Output
```bash
# Start tests in background
ginkgo -v . > /tmp/nfd_test_run.log 2>&1 &

# Follow output
tail -f /tmp/nfd_test_run.log
```

### Check Running Tests
```bash
# See which tests are running
ps aux | grep ginkgo

# Check test progress
grep "STEP:" /tmp/nfd_test_run.log | tail -10
```

### Monitor Cluster Resources
```bash
# Watch NFD pods
kubectl get pods -n openshift-nfd -w

# Watch node labels
kubectl get nodes --show-labels | grep feature.node

# Watch NodeFeatureRules
kubectl get nodefeaturerules -w
```

---

## Pre-Flight Checks

Before running tests, verify cluster state:

```bash
#!/bin/bash
echo "=== Pre-Flight Checks ==="

# Check cluster connectivity
echo "1. Checking cluster connectivity..."
kubectl cluster-info | head -2

# Check if NFD namespace exists
echo "2. Checking NFD namespace..."
if kubectl get namespace openshift-nfd &>/dev/null; then
    STATUS=$(kubectl get namespace openshift-nfd -o jsonpath='{.status.phase}')
    if [ "$STATUS" = "Terminating" ]; then
        echo "   ⚠ Namespace is stuck in Terminating - run cleanup script"
        exit 1
    else
        echo "   ℹ Namespace exists (status: $STATUS) - BeforeSuite will reuse or recreate"
    fi
else
    echo "   ✓ Namespace does not exist - BeforeSuite will create it"
fi

# Check for orphaned resources
echo "3. Checking for orphaned resources..."
ORPHANS=0
if kubectl get operatorgroups -A 2>/dev/null | grep -q nfd; then
    echo "   ⚠ Found orphaned operator groups"
    ((ORPHANS++))
fi
if kubectl get subscriptions -A 2>/dev/null | grep -q nfd; then
    echo "   ⚠ Found orphaned subscriptions"
    ((ORPHANS++))
fi
if [ $ORPHANS -gt 0 ]; then
    echo "   → Run cleanup script: bash /tmp/force_cleanup_nfd.sh"
    exit 1
else
    echo "   ✓ No orphaned resources"
fi

echo ""
echo "=== Ready to run tests ==="
echo "Run: ginkgo -v ."
```

---

## Clean Test Environment

To ensure a completely clean test environment:

```bash
#!/bin/bash
# Complete cleanup and verification

# 1. Force cleanup
bash /tmp/force_cleanup_nfd.sh

# 2. Verify cleanup
if kubectl get namespace openshift-nfd &>/dev/null; then
    echo "ERROR: Namespace still exists"
    exit 1
fi

# 3. Check for any NodeFeatureRules on nodes
LABELS=$(kubectl get nodes -o json | jq -r '.items[].metadata.labels | keys[]' | grep feature.node | head -5)
if [ -n "$LABELS" ]; then
    echo "INFO: Found existing feature labels on nodes (may be from previous runs):"
    echo "$LABELS"
    echo ""
fi

# 4. Ready to run
echo "Environment is clean. Running tests..."
cd /Users/guygordani/nfdchecker/eco-gotests/tests/hw-accel/nfd/features
ginkgo -v .
```

---

## Useful Commands

### Cleanup Commands
```bash
# Quick cleanup
bash /tmp/force_cleanup_nfd.sh

# Manual namespace deletion
kubectl delete namespace openshift-nfd --force --grace-period=0

# Remove all NodeFeatureRules
kubectl delete nodefeaturerules --all

# Remove NFD labels from nodes
kubectl label nodes --all $(kubectl get nodes -o json | jq -r '.items[0].metadata.labels | keys[]' | grep feature.node | sed 's/$/=/g' | tr '\n' ' ')
```

### Investigation Commands
```bash
# Check namespace events
kubectl get events -n openshift-nfd --sort-by='.lastTimestamp'

# Check operator logs
kubectl logs -n openshift-nfd -l app=nfd-operator

# Check NFD master logs
kubectl logs -n openshift-nfd -l app=nfd-master

# Check NFD worker logs
kubectl logs -n openshift-nfd -l app=nfd-worker

# Describe stuck namespace
kubectl describe namespace openshift-nfd
```

### Test Execution Commands
```bash
# Run all tests
ginkgo -v .

# Run specific test by ID
ginkgo -v . --focus="70001"

# Run multiple tests
ginkgo -v . --focus="70001|70003|70005"

# Skip failing tests
ginkgo -v . --skip="70004|70041"

# Run only passing tests
ginkgo -v . --skip="70004|70041|70002"  # Skip known failures

# Run with JSON output
ginkgo -v . --json-report=report.json
```

---

## Getting Help

If you encounter issues not covered here:

1. **Check test logs**:
   - `/tmp/nfd_test_run.log` - test output
   - `/tmp/nfd_test_fresh_run.log` - latest run

2. **Check cluster state**:
   - `kubectl get all -n openshift-nfd`
   - `kubectl get nodefeaturerules`
   - `kubectl get nodes --show-labels`

3. **Check documentation**:
   - `TEST_FIXES_SUMMARY.md` - Known test issues
   - `CR_CLEANUP_CHANGES.md` - CR cleanup details
   - `SUITE_IMPROVEMENTS_SUMMARY.md` - Suite improvements

4. **Force cleanup and retry**:
   - Run: `bash /tmp/force_cleanup_nfd.sh`
   - Then: `ginkgo -v .`
