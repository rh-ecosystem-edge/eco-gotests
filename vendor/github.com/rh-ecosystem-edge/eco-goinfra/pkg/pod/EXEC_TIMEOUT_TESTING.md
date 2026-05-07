# ExecCommandWithTimeout Integration Testing Guide

This guide explains how to run integration tests for the `ExecCommandWithTimeout` timeout enforcement fix.

## Overview

The test in `integration/pod_test.go` verifies that the timeout parameter is properly enforced for:
1. Fast commands that complete within timeout
2. Long-running commands that exceed the timeout
3. Various timeout durations (2s, 3s, 4s, 5s, etc.)

## Prerequisites

### Required

1. **Kubernetes Cluster Access**
   - A running Kubernetes cluster (can be local like kind/minikube or remote)
   - Cluster admin privileges (ability to create namespaces and pods)
   - Network access to cluster API server

2. **Kubeconfig File**
   - Valid kubeconfig with credentials for the cluster
   - Default location: `~/.kube/config`
   - Or custom location set via `KUBECONFIG` environment variable

3. **Go Environment**
   - Go 1.21 or later
   - `go test` command available

4. **Container Image Access**
   - Tests use `quay.io/centos/centos:stream9` by default
   - Cluster must be able to pull this image
   - Or modify `containerImage` constant in the test file

### Optional

- `kubectl` CLI tool (for debugging)
- Cluster node access (for advanced troubleshooting)

## Running the Tests

### Method 1: Using KUBECONFIG Environment Variable (Recommended)

```bash
# Navigate to the eco-goinfra directory
cd eco-goinfra

# Set KUBECONFIG to point to your cluster configuration
export KUBECONFIG=/path/to/your/kubeconfig

# Run all integration tests for pod package
go test -v -tags=integration ./integration/ -timeout 10m

# Run only ExecCommandWithTimeout tests
go test -v -tags=integration ./integration/ -run TestExecCommandWithTimeout -timeout 10m

# Run a specific test case
go test -v -tags=integration ./integration/ -run TestExecCommandWithTimeoutEnforcement -timeout 10m
```

### Method 2: Using Default Kubeconfig Location

If your kubeconfig is in the default location (`~/.kube/config`):

```bash
cd eco-goinfra

# Run the tests (KUBECONFIG will be auto-detected)
go test -v -tags=integration ./integration/ -run TestExecCommandWithTimeout -timeout 10m
```

### Method 3: Testing Against Specific Cluster Contexts

If you have multiple clusters in your kubeconfig:

```bash
# List available contexts
kubectl config get-contexts

# Set the desired context
kubectl config use-context my-cluster-context

# Verify connection
kubectl cluster-info

# Run the tests
go test -v -tags=integration ./integration/ -run TestExecCommandWithTimeout -timeout 10m
```

## Test Execution Details

### What the Test Does

**TestExecCommandWithTimeoutEnforcement**
   - Creates a temporary namespace with random name
   - Deploys a test pod with CentOS Stream 9 container
   - Runs fast commands (should succeed within timeout)
   - Runs slow commands with short timeouts (should timeout ~3 seconds, NOT hang for minutes)
   - Tests multiple timeout scenarios (2s, 3s, 4s, 5s with varying command durations)
   - Verifies timeout occurs at the expected time (not command duration)
   - Cleans up all resources automatically

### Expected Test Duration

- **Test execution:** ~1-2 minutes
- Includes pod creation, waiting for pod ready, multiple command executions with various timeouts, and cleanup

### Resource Usage

Each test creates:
- 1 namespace (auto-deleted after test)
- 1-2 pods (auto-deleted after test)
- Minimal CPU/memory footprint

## Interpreting Results

### Successful Test Output

```text
=== RUN   TestExecCommandWithTimeoutEnforcement
=== RUN   TestExecCommandWithTimeoutEnforcement/fast_command_with_long_timeout_succeeds
=== RUN   TestExecCommandWithTimeoutEnforcement/long_command_with_short_timeout_fails
=== RUN   TestExecCommandWithTimeoutEnforcement/timeout_occurs_at_expected_time
=== RUN   TestExecCommandWithTimeoutEnforcement/timeout_occurs_at_expected_time/2s_sleep_with_5s_timeout_-_succeeds
=== RUN   TestExecCommandWithTimeoutEnforcement/timeout_occurs_at_expected_time/10s_sleep_with_2s_timeout_-_times_out
=== RUN   TestExecCommandWithTimeoutEnforcement/timeout_occurs_at_expected_time/20s_sleep_with_4s_timeout_-_times_out
--- PASS: TestExecCommandWithTimeoutEnforcement (65.23s)
    --- PASS: TestExecCommandWithTimeoutEnforcement/fast_command_with_long_timeout_succeeds (0.15s)
    --- PASS: TestExecCommandWithTimeoutEnforcement/long_command_with_short_timeout_fails (3.12s)
    --- PASS: TestExecCommandWithTimeoutEnforcement/timeout_occurs_at_expected_time (12.45s)
        --- PASS: TestExecCommandWithTimeoutEnforcement/timeout_occurs_at_expected_time/2s_sleep_with_5s_timeout_-_succeeds (2.18s)
        --- PASS: TestExecCommandWithTimeoutEnforcement/timeout_occurs_at_expected_time/10s_sleep_with_2s_timeout_-_times_out (2.09s)
        --- PASS: TestExecCommandWithTimeoutEnforcement/timeout_occurs_at_expected_time/20s_sleep_with_4s_timeout_-_times_out (4.11s)
PASS
ok      github.com/rh-ecosystem-edge/eco-goinfra/integration        65.450s
```

**Key Success Indicators:**
- Test completes without hanging
- Timeout tests show elapsed time close to timeout value (e.g., 3.12s for 3s timeout)
- No tests timeout at the Go test level (10m timeout)

### Failure Indicators

**Bug NOT Fixed (Old Behavior):**
```
--- FAIL: TestExecCommandWithTimeoutEnforcement/long_command_with_short_timeout_fails (900.45s)
    pod_test.go:78:
        Error Trace:    pod_test.go:78
        Error:          Should be true
        Test:           TestExecCommandWithTimeoutEnforcement/long_command_with_short_timeout_fails
        Messages:       Command should timeout after ~3s, but took 15m0.45s
```
This indicates the timeout bug still exists (command hung for 15 minutes instead of 3 seconds).

**Cluster Connection Issues:**
```
pod_test.go:25: Failed to create Kubernetes client
```
This indicates KUBECONFIG is not set or cluster is not accessible.

## Troubleshooting

### Test Hangs or Times Out

**Problem:** Test runs for 10+ minutes without completing

**Solutions:**
1. Check cluster is accessible:
   ```bash
   kubectl cluster-info
   kubectl get nodes
   ```

2. Check pod can be created:
   ```bash
   kubectl run test-pod --image=quay.io/centos/centos:stream9 --command -- sleep 3600
   kubectl get pods
   kubectl delete pod test-pod
   ```

3. Verify image can be pulled:
   ```bash
   kubectl run test-pull --image=quay.io/centos/centos:stream9 --rm -it -- echo "success"
   ```

### "Failed to create Kubernetes client"

**Problem:** Client creation fails

**Solutions:**
1. Verify KUBECONFIG is set:
   ```bash
   echo $KUBECONFIG
   # Should show path to your kubeconfig file
   ```

2. Check kubeconfig is valid:
   ```bash
   kubectl config view
   ```

3. Test cluster connectivity:
   ```bash
   kubectl version
   ```

### "Failed to create test namespace"

**Problem:** Namespace creation fails

**Solutions:**
1. Check you have admin privileges:
   ```bash
   kubectl auth can-i create namespace
   # Should return "yes"
   ```

2. Try creating namespace manually:
   ```bash
   kubectl create namespace test-timeout-fix
   kubectl delete namespace test-timeout-fix
   ```

### Pod Creation Fails

**Problem:** Pod doesn't start or remains in Pending state

**Solutions:**
1. Check cluster has available resources:
   ```bash
   kubectl top nodes
   kubectl describe nodes
   ```

2. Check image pull policy:
   ```bash
   kubectl get events --all-namespaces | grep -i pull
   ```

3. Verify network policies allow pod creation

### Tests Fail Intermittently

**Problem:** Tests pass sometimes but fail other times

**Possible Causes:**
1. **Network latency** - Add buffer to timeout assertions
2. **Resource contention** - Run tests serially instead of parallel
3. **Cluster instability** - Verify cluster health

## Advanced Usage

### Running Tests Against Multiple Clusters

```bash
# Test against development cluster
export KUBECONFIG=~/.kube/dev-cluster-config
go test -v -tags=integration ./integration/ -run TestExecCommandWithTimeout -timeout 10m

# Test against staging cluster
export KUBECONFIG=~/.kube/staging-cluster-config
go test -v -tags=integration ./integration/ -run TestExecCommandWithTimeout -timeout 10m
```

### Debugging Failed Tests

```bash
# Run with verbose output and print logs
go test -v -tags=integration ./integration/ -run TestExecCommandWithTimeout -timeout 10m -test.v

# Run individual test with even more verbosity
go test -v -tags=integration ./integration/ -run TestExecCommandWithTimeoutEnforcement/long_command_with_short_timeout_fails -timeout 10m
```

### Customizing Test Parameters

The pod integration tests use shared constants defined in `integration/deployment_test.go`:

```go
const (
    containerImage  = "nginx:latest"           // Container image for tests
    timeoutDuration = 60 * time.Second        // Pod operations timeout
)
```

To use a different container image for testing, edit these constants in `integration/deployment_test.go`.

### Running the Timeout Test

```bash
# Run the timeout enforcement test
go test -v -tags=integration ./integration/ -run TestExecCommandWithTimeoutEnforcement -timeout 10m

# Run with specific sub-test
go test -v -tags=integration ./integration/ -run TestExecCommandWithTimeoutEnforcement/long_command_with_short_timeout_fails -timeout 10m
```

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Integration Tests

on: [push, pull_request]

jobs:
  integration-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.21'

      - name: Create test cluster
        uses: helm/kind-action@v1.5.0
        with:
          cluster_name: test-cluster

      - name: Run integration tests
        run: |
          export KUBECONFIG=$HOME/.kube/config
          go test -v -tags=integration ./integration/ -run TestExecCommandWithTimeout -timeout 10m
```

### Jenkins Pipeline Example

```groovy
pipeline {
    agent any

    environment {
        KUBECONFIG = credentials('kubernetes-config')
    }

    stages {
        stage('Integration Tests') {
            steps {
                sh '''
                    export KUBECONFIG=${KUBECONFIG}
                    go test -v -tags=integration ./integration/ -run TestExecCommandWithTimeout -timeout 10m
                '''
            }
        }
    }
}
```

## Verifying the Fix

To manually verify the timeout fix works:

```bash
# 1. Connect to cluster
export KUBECONFIG=/path/to/kubeconfig

# 2. Run the test
go test -v -tags=integration ./integration/ -run TestExecCommandWithTimeoutEnforcement/long_command_with_short_timeout_fails -timeout 10m

# 3. Observe the output - should show:
#    - Test completes in ~3 seconds (not 15+ minutes)
#    - Error message contains "context deadline exceeded"
#    - Elapsed time is close to timeout value (3s)
```

## Related Files

- **Test file:** `integration/pod_test.go`
- **Implementation:** `pkg/pod/pod.go` (ExecCommandWithTimeout method)
- **Original bug:** eco-gotests test 81422 failure (16-minute hang)

## Questions or Issues?

If you encounter issues running these tests:

1. Verify all prerequisites are met
2. Check cluster connectivity and permissions
3. Review troubleshooting section above
4. Check test logs for specific error messages
5. Verify the timeout fix is applied (check `getExecutorFromRequestConfigurable` method exists)

## Summary

**Minimum Required Setup:**
```bash
export KUBECONFIG=/path/to/your/kubeconfig
cd eco-goinfra
go test -v -tags=integration ./integration/ -run TestExecCommandWithTimeout -timeout 10m
```

**Expected Result:** All tests pass, timeout tests complete in seconds (not minutes), no hangs.
