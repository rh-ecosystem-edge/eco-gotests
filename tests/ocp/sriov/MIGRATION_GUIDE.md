# Migration Guide: tests/sriov → tests/ocp/sriov

This guide documents the migration of SR-IOV tests from `tests/sriov/` to `tests/ocp/sriov/` following the cursor rules and project conventions.

## Overview

The migration involves:
1. Restructuring code to follow the new directory structure
2. Refactoring helper functions to remove Gomega/Ginkgo dependencies from `internal/` folders
3. Moving test cases to proper locations with labels and test IDs
4. Updating imports and package names

## Directory Structure Changes

### Old Structure
```
tests/sriov/
├── helpers.go
├── sriov_basic_test.go
├── README.md
└── testdata/
```

### New Structure
```
tests/ocp/sriov/
├── internal/
│   ├── tsparams/
│   │   ├── consts.go          # Constants, labels, timeouts
│   │   └── sriovvars.go       # Configuration and variables
│   └── sriovenv/
│       ├── sriovinittools.go  # APIClient and config initialization
│       └── sriovenv.go        # Helper functions (NO Gomega/Ginkgo)
├── tests/
│   └── basic.go               # Test cases (can use Gomega/Ginkgo)
├── sriov_suite_test.go        # Suite entry point
├── README.md
└── testdata/
```

## Completed Steps

✅ **Step 1**: Created directory structure
- `tests/ocp/sriov/internal/tsparams/`
- `tests/ocp/sriov/internal/sriovenv/`
- `tests/ocp/sriov/tests/`

✅ **Step 2**: Created `tsparams` package
- `consts.go`: Labels, timeouts, constants
- `sriovvars.go`: NetworkConfig, DeviceConfig, configuration functions

✅ **Step 3**: Created `sriovenv` package
- `sriovinittools.go`: APIClient and NetConfig initialization

✅ **Step 4**: Copied testdata directory

## Remaining Work

### Step 5: Refactor Helper Functions

The `helpers.go` file (1760 lines) contains many functions that use Gomega/Ginkgo. These **MUST** be refactored to return errors instead.

#### Functions That Need Refactoring

**Category 1: Functions using `Expect()` - MUST return errors**

| Function Name | Current Behavior | Required Change |
|--------------|------------------|-----------------|
| `chkSriovOperatorStatus()` | Uses `Expect(err).ToNot(HaveOccurred())` | Return `error` instead |
| `waitForSriovPolicyReady()` | Uses `Expect(err).ToNot(HaveOccurred())` | Return `error` instead |
| `rmSriovPolicy()` | Uses `Expect()` and `Eventually()` | Return `error`, use `wait.PollUntilContextTimeout` |
| `createSriovNetwork()` | Uses `Expect(err).ToNot(HaveOccurred())` | Return `error` instead |
| `rmSriovNetwork()` | Uses `Expect()` and `Eventually()` | Return `error`, use `wait.PollUntilContextTimeout` |
| `chkVFStatusWithPassTraffic()` | Uses multiple `Expect()` calls | Return `error` instead |
| `createTestPod()` | Uses `Expect(err).ToNot(HaveOccurred())` | Return `(*pod.Builder, error)` |
| `createSriovTestPod()` | Uses `Expect(err).ToNot(HaveOccurred())` | Return `error` instead |
| `waitForPodWithLabelReady()` | Uses `Expect()` and `Eventually()` | Return `error`, use `wait.PollUntilContextTimeout` |
| `verifyInterfaceReady()` | Uses `Expect()` | Return `error` instead |
| `verifyVFSpoofCheck()` | Uses `Expect()` | Return `error` instead |

**Category 2: Functions using `GinkgoLogr` - Replace with standard logging**

| Function Name | Current Behavior | Required Change |
|--------------|------------------|-----------------|
| `IsSriovDeployed()` | Uses `GinkgoLogr.Info()` | Use `glog.V().Infof()` or remove logging |
| `WaitForSriovAndMCPStable()` | Uses `GinkgoLogr.Info()` | Use `glog.V().Infof()` or remove logging |
| `CleanAllNetworksByTargetNamespace()` | Uses `GinkgoLogr.Info()` | Use `glog.V().Infof()` or remove logging |
| `pullTestImageOnNodes()` | Uses `GinkgoLogr.Info()` | Use `glog.V().Infof()` or remove logging |
| `logOcCommand()` | Uses `GinkgoLogr.Info()` | Use `glog.V().Infof()` or remove logging |
| `logOcCommandYaml()` | Uses `GinkgoLogr.Info()` | Use `glog.V().Infof()` or remove logging |
| `collectPodDiagnostics()` | Uses `GinkgoLogr.Info()` | Use `glog.V().Infof()` or remove logging |
| `collectSriovClusterDiagnostics()` | Uses `GinkgoLogr.Info()` | Use `glog.V().Infof()` or remove logging |

**Category 3: Functions using `By()` - Remove or move to test files**

| Function Name | Current Behavior | Required Change |
|--------------|------------------|-----------------|
| All helper functions | Use `By()` for step documentation | Remove `By()` calls (only in test files) |

**Category 4: Functions using `Eventually()` - Use `wait.PollUntilContextTimeout`**

| Function Name | Current Behavior | Required Change |
|--------------|------------------|-----------------|
| `rmSriovPolicy()` | Uses `Eventually()` | Use `wait.PollUntilContextTimeout` |
| `rmSriovNetwork()` | Uses `Eventually()` | Use `wait.PollUntilContextTimeout` |
| `createSriovNetwork()` | Uses `Eventually()` | Use `wait.PollUntilContextTimeout` |
| `waitForPodWithLabelReady()` | Uses `Eventually()` | Use `wait.PollUntilContextTimeout` |

**Category 5: Functions using `Skip()` - Return error instead**

| Function Name | Current Behavior | Required Change |
|--------------|------------------|-----------------|
| `getAPIClient()` | Uses `Skip()` | Return `(*clients.Settings, error)` |

### Refactoring Examples

#### Example 1: Simple Function Refactoring

**Before (❌ INCORRECT - uses Gomega):**
```go
// In helpers.go (internal folder)
func chkSriovOperatorStatus(sriovOpNs string) {
    By("Checking SRIOV operator status")
    err := IsSriovDeployed(getAPIClient(), NetConfig)
    Expect(err).ToNot(HaveOccurred(), "SRIOV operator is not deployed")
}
```

**After (✅ CORRECT - returns error):**
```go
// In internal/sriovenv/sriovenv.go
package sriovenv

import (
    "github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
    "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/tsparams"
)

func CheckSriovOperatorStatus(apiClient *clients.Settings, config *tsparams.NetworkConfig) error {
    return IsSriovDeployed(apiClient, config)
}
```

**Usage in test file:**
```go
// In tests/basic.go (test file - can use Gomega)
err := sriovenv.CheckSriovOperatorStatus(APIClient, NetConfig)
Expect(err).ToNot(HaveOccurred(), "SRIOV operator is not deployed")
```

#### Example 2: Function with Eventually() Refactoring

**Before (❌ INCORRECT):**
```go
func rmSriovPolicy(name, sriovOpNs string) {
    By(fmt.Sprintf("Removing SRIOV policy %s if it exists", name))
    
    policyBuilder := sriov.NewPolicyBuilder(...)
    
    if policyBuilder.Exists() {
        err := policyBuilder.Delete()
        // ... error handling ...
        
        Eventually(func() bool {
            checkPolicy := sriov.NewPolicyBuilder(...)
            return !checkPolicy.Exists()
        }, 30*time.Second, 2*time.Second).Should(BeTrue(), ...)
    }
}
```

**After (✅ CORRECT):**
```go
// In internal/sriovenv/sriovenv.go
package sriovenv

import (
    "context"
    "fmt"
    "time"
    
    "github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
    "github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
    "k8s.io/apimachinery/pkg/util/wait"
)

func RemoveSriovPolicy(apiClient *clients.Settings, name, sriovOpNs string, timeout time.Duration) error {
    policyBuilder := sriov.NewPolicyBuilder(
        apiClient,
        name,
        sriovOpNs,
        "",
        0,
        []string{},
        map[string]string{},
    )
    
    if !policyBuilder.Exists() {
        return nil // Already deleted
    }
    
    err := policyBuilder.Delete()
    if err != nil {
        return fmt.Errorf("failed to delete SRIOV policy %q: %w", name, err)
    }
    
    // Wait for deletion using wait.PollUntilContextTimeout
    err = wait.PollUntilContextTimeout(
        context.TODO(),
        2*time.Second,
        timeout,
        true,
        func(ctx context.Context) (bool, error) {
            checkPolicy := sriov.NewPolicyBuilder(
                apiClient,
                name,
                sriovOpNs,
                "",
                0,
                []string{},
                map[string]string{},
            )
            return !checkPolicy.Exists(), nil
        })
    
    if err != nil {
        return fmt.Errorf("timeout waiting for SRIOV policy %q to be deleted: %w", name, err)
    }
    
    return nil
}
```

**Usage in test file:**
```go
// In tests/basic.go
By("Removing SRIOV policy")
err := sriovenv.RemoveSriovPolicy(APIClient, policyName, sriovOpNs, 30*time.Second)
Expect(err).ToNot(HaveOccurred(), "Failed to remove SRIOV policy %q", policyName)
```

#### Example 3: Function with Logging Refactoring

**Before (❌ INCORRECT):**
```go
func IsSriovDeployed(apiClient *clients.Settings, config *NetworkConfig) error {
    GinkgoLogr.Info("Checking if SR-IOV operator is deployed", "namespace", config.SriovOperatorNamespace)
    // ... implementation ...
    GinkgoLogr.Info("SR-IOV operator is deployed and ready", ...)
    return nil
}
```

**After (✅ CORRECT):**
```go
// In internal/sriovenv/sriovenv.go
package sriovenv

import (
    "context"
    "fmt"
    "time"
    
    "github.com/golang/glog"
    "github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
    "github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
    "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/tsparams"
    corev1 "k8s.io/api/core/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

func IsSriovDeployed(apiClient *clients.Settings, config *tsparams.NetworkConfig) error {
    glog.V(90).Infof("Checking if SR-IOV operator is deployed in namespace %q", config.SriovOperatorNamespace)
    
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    // ... implementation using standard error returns ...
    
    glog.V(90).Infof("SR-IOV operator is deployed and ready in namespace %q", config.SriovOperatorNamespace)
    return nil
}
```

### Step 6: Create Suite File

Create `tests/ocp/sriov/sriov_suite_test.go`:

```go
package sriov

import (
    "os"
    "path"
    "runtime"
    "testing"
    "time"
    
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
    "github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
    . "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/sriovenv"
    "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/tsparams"
    "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/params"
    "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/reporter"
    _ "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/tests"
)

var _, currentFile, _, _ = runtime.Caller(0)

var (
    testNS *namespace.Builder
)

func TestSriov(t *testing.T) {
    _, reporterConfig := GinkgoConfiguration()
    reporterConfig.JUnitReport = NetConfig.GetJunitReportPath()
    
    RegisterFailHandler(Fail)
    RunSpecs(t, "OCP SR-IOV Suite", Label(tsparams.Labels...), reporterConfig)
}

var _ = BeforeSuite(func() {
    By("Cleaning up leftover resources from previous test runs")
    // Use refactored helper function
    err := sriovenv.CleanupLeftoverResources(APIClient, NetConfig.SriovOperatorNamespace)
    Expect(err).ToNot(HaveOccurred(), "Failed to cleanup leftover resources")
    
    By("Creating test namespace with privileged labels")
    testNS = namespace.NewBuilder(APIClient, tsparams.TestNamespaceName)
    for key, value := range params.PrivilegedNSLabels {
        testNS.WithLabel(key, value)
    }
    _, err = testNS.Create()
    Expect(err).ToNot(HaveOccurred(), "Failed to create test namespace %q", testNS.Definition.Name)
    
    By("Verifying if sriov tests can be executed on given cluster")
    err = sriovenv.IsSriovDeployed(APIClient, NetConfig)
    Expect(err).ToNot(HaveOccurred(), "Cluster doesn't support sriov test cases")
    
    By("Pulling test images on cluster before running test cases")
    err = sriovenv.PullTestImageOnNodes(APIClient, NetConfig.WorkerLabel, NetConfig.CnfNetTestContainer, 300)
    Expect(err).ToNot(HaveOccurred(), "Failed to pull test image on nodes")
})

var _ = AfterSuite(func() {
    By("Deleting test namespace")
    if testNS != nil {
        err := testNS.DeleteAndWait(tsparams.DefaultTimeout)
        Expect(err).ToNot(HaveOccurred(), "Failed to delete test namespace")
    }
})

var _ = JustAfterEach(func() {
    reporter.ReportIfFailed(
        CurrentSpecReport(),
        currentFile,
        tsparams.ReporterNamespacesToDump,
        tsparams.ReporterCRDsToDump)
})

var _ = ReportAfterSuite("", func(report Report) {
    reportxml.Create(report, NetConfig.GetReportPath(), NetConfig.TCPrefix)
})
```

### Step 7: Move Test Cases

Create `tests/ocp/sriov/tests/basic.go`:

```go
package tests

import (
    "path/filepath"
    
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
    . "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/sriovenv"
    "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/tsparams"
    "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/params"
)

var _ = Describe(
    "SR-IOV Basic Tests",
    Ordered,
    Label(tsparams.LabelSuite, tsparams.LabelBasic),
    ContinueOnFailure,
    func() {
        var (
            buildPruningBaseDir  = filepath.Join("testdata", "networking", "sriov")
            sriovNetworkTemplate = filepath.Join(buildPruningBaseDir, "sriovnetwork-whereabouts-template.yaml")
            sriovOpNs            = NetConfig.SriovOperatorNamespace
            vfNum                = tsparams.GetVFNum()
            testData             = tsparams.GetDefaultDeviceConfig()
        )
        
        BeforeEach(func() {
            By("Check the sriov operator is running")
            err := sriovenv.CheckSriovOperatorStatus(APIClient, NetConfig)
            Expect(err).ToNot(HaveOccurred(), "SRIOV operator is not deployed")
            
            // ... rest of setup ...
        })
        
        AfterEach(func() {
            // Use refactored helper functions
            for _, item := range testData {
                err := sriovenv.RemoveSriovPolicy(APIClient, item.Name, sriovOpNs, 30*time.Second)
                Expect(err).ToNot(HaveOccurred(), "Failed to remove SRIOV policy %q", item.Name)
            }
            err := sriovenv.WaitForSriovPolicyReady(APIClient, sriovOpNs, tsparams.WaitTimeout, tsparams.RetryInterval, NetConfig.CnfMcpLabel)
            Expect(err).ToNot(HaveOccurred(), "SRIOV policy is not ready")
        })
        
        It("SR-IOV VF with spoof checking enabled", reportxml.ID("25959"), func() {
            // ... test implementation using refactored helpers ...
        })
        
        // ... other test cases ...
    })
```

## Import Path Changes

### Old Imports
```go
import (
    "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/params"
    // Global APIClient and NetConfig from helpers.go
)
```

### New Imports
```go
import (
    . "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/sriovenv"  // APIClient, NetConfig
    "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/tsparams"  // Constants, config
    "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/params"              // Common params
)
```

## Function Name Changes

Many helper functions should be renamed to follow Go conventions (exported functions start with capital letter):

| Old Name | New Name | Location |
|----------|----------|----------|
| `getAPIClient()` | Use `APIClient` from `sriovenv` | Removed (use global) |
| `getTestNS()` | Use `testNS` from suite | Removed (use suite var) |
| `chkSriovOperatorStatus()` | `CheckSriovOperatorStatus()` | `sriovenv/sriovenv.go` |
| `waitForSriovPolicyReady()` | `WaitForSriovPolicyReady()` | `sriovenv/sriovenv.go` |
| `rmSriovPolicy()` | `RemoveSriovPolicy()` | `sriovenv/sriovenv.go` |
| `rmSriovNetwork()` | `RemoveSriovNetwork()` | `sriovenv/sriovenv.go` |
| `createSriovNetwork()` | `CreateSriovNetwork()` | `sriovenv/sriovenv.go` |
| `chkVFStatusWithPassTraffic()` | `CheckVFStatusWithPassTraffic()` | `sriovenv/sriovenv.go` |
| `createTestPod()` | `CreateTestPod()` | `sriovenv/sriovenv.go` |
| `cleanupLeftoverResources()` | `CleanupLeftoverResources()` | `sriovenv/sriovenv.go` |

## Key Refactoring Rules

1. **NO Gomega/Ginkgo in `internal/` folders**
   - Remove all `Expect()`, `Eventually()`, `Consistently()` calls
   - Remove all `By()`, `GinkgoLogr`, `Skip()` calls
   - Functions must return `error` instead of calling `Fail()`

2. **Use `wait.PollUntilContextTimeout` instead of `Eventually()`**
   ```go
   err := wait.PollUntilContextTimeout(
       context.TODO(),
       interval,
       timeout,
       true,
       func(ctx context.Context) (bool, error) {
           // Check condition
           return condition, nil
       })
   ```

3. **Use `glog` for logging in helpers**
   ```go
   glog.V(90).Infof("Message: %s", value)
   ```

4. **All helper functions must return errors**
   ```go
   func HelperFunction(...) error {
       if err != nil {
           return fmt.Errorf("context: %w", err)
       }
       return nil
   }
   ```

5. **Test files can use Gomega/Ginkgo**
   - Only test files in `tests/` directory can import Gomega/Ginkgo
   - Suite file can use Gomega/Ginkgo

## Checklist

- [ ] Refactor all helper functions in `helpers.go` to remove Gomega/Ginkgo
- [ ] Move refactored helpers to `internal/sriovenv/sriovenv.go`
- [ ] Create `sriov_suite_test.go` with proper setup
- [ ] Move test cases to `tests/basic.go` with proper structure
- [ ] Add test IDs using `reportxml.ID()`
- [ ] Add proper labels using `Label(tsparams.LabelSuite, tsparams.LabelBasic)`
- [ ] Update all imports
- [ ] Update function calls to use new names
- [ ] Test compilation
- [ ] Run `make lint`
- [ ] Verify tests run correctly

## Testing the Migration

1. **Compile check:**
   ```bash
   cd tests/ocp/sriov
   go build ./...
   ```

2. **Lint check:**
   ```bash
   make lint
   ```

3. **Run tests:**
   ```bash
   export KUBECONFIG=/path/to/kubeconfig
   export ECO_TEST_FEATURES="sriov"
   export ECO_TEST_LABELS="sriov && basic"
   make run-tests
   ```

## Notes

- The migration is incremental - you can refactor functions one at a time
- Keep the old `tests/sriov/` directory until migration is complete
- Test each refactored function before moving to the next
- Use this guide as a reference during refactoring

