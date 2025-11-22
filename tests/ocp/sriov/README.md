# Generic Rules for Test Cases - OCP SR-IOV

## Overview

This document outlines generic rules and best practices for developing test cases in the eco-gotests framework, specifically for SR-IOV test suites. These rules ensure consistency, maintainability, and reliability across all test implementations.

### Purpose

The eco-gotests framework is designed to test pre-installed OCP clusters using golang and [ginkgo](https://onsi.github.io/ginkgo). This document provides generic guidelines that should be followed when creating new test cases or modifying existing ones.

## Prerequisites for Test Development

### Framework Requirements

* OCP cluster version >=4.13
* Access to eco-goinfra packages

### Cluster Prerequisites

Test cases should clearly document their cluster prerequisites. Common requirements include:

* SR-IOV operator installed and healthy
* Appropriate hardware or virtualized SR-IOV interfaces available

## Test Suite Organization

### Directory Structure

Follow the established project structure pattern:

```text
tests/ocp/sriov/
├── internal/                    # Internal packages used within test suite
│   ├── tsparams/               # Test suite constants and parameters
│   │   ├── consts.go           # Constants (labels, timeouts, names)
│   │   └── sriovvars.go        # Variables and configuration
│   └── sriovenv/               # Environment validation and helpers
│       └── sriovenv.go
├── tests/                      # Test case implementations
│   ├── testcase1.go
│   └── testcase2.go
└── sriov_suite_test.go         # Ginkgo test suite entry point
```

### Naming Conventions

1. **Package Names**: Use lowercase, descriptive names (e.g., `tsparams`, `sriovenv`)
2. **File Names**: Use lowercase with underscores or camelCase (e.g., `metricsExporter.go`, `webhook-matchConditions.go`)
3. **Test Files**: Place in `tests/` subdirectory, one file per test scenario or feature group
4. **Suite File**: Named as `{suite}_suite_test.go` (e.g., `sriov_suite_test.go`)

## Test Case Structure

### Ginkgo Test Patterns

#### Basic Test Structure

```go
var _ = Describe(
    "TestFeatureName",
    Ordered,
    Label(tsparams.LabelFeatureName),
    ContinueOnFailure,
    func() {
        var (
            // Shared test variables
        )

        BeforeAll(func() {
            // Setup required for all tests in this Describe block
            By("Setting up test environment")
            // ... setup code ...
        })

        AfterAll(func() {
            // Cleanup after all tests
            By("Cleaning up test resources")
            // ... cleanup code ...
        })

        BeforeEach(func() {
            // Setup for each test
        })

        AfterEach(func() {
            // Cleanup after each test
        })

        It("should perform specific test action", reportxml.ID("12345"), func() {
            By("Step description")
            // Test implementation
        })
    })
```

#### Test Organization Principles

1. **Ordered Tests**: Use `Ordered` when tests must run in sequence
2. **Labels**: Always use labels for test filtering (`Label(tsparams.LabelFeatureName)`)
3. **ContinueOnFailure**:
   - **Important**: `ContinueOnFailure` is typically used in conjunction with `Ordered` containers
   - In an `Ordered` container, if a test fails, the remaining tests will not execute by default. Use `ContinueOnFailure` to allow remaining tests to run even after a failure
   - If `Ordered` is not used, continuing on failure is the default behavior for the rest of the test cases, so `ContinueOnFailure` is not necessary
4. **Descriptive Names**: Test descriptions should clearly state what is being tested

### Test Labels

#### Label Naming Convention

- Use lowercase with hyphens or underscores
- Be specific and descriptive
- Group related tests under the same label
- Define labels in `internal/tsparams/consts.go`

Example:
```go
const (
    LabelSuite = "sriov"
    LabelFeatureName = "feature-name"
    LabelHWEnabled = "sriov-hw-enabled"
)
```

### Test IDs

- Use `reportxml.ID("12345")` for each test case
- Test IDs should be unique and traceable to test case management systems
- Include test ID in test description or comments when helpful

## Internal Packages

### Test Parameters Package (`tsparams`)

Purpose: Centralize constants, variables, and configuration for the test suite.

#### Structure

```go
package tsparams

const (
    // Labels
    LabelSuite = "sriov"
    LabelFeatureName = "feature-name"

    // Namespaces
    TestNamespaceName = "sriov-tests"

    // Timeouts
    WaitTimeout = 3 * time.Minute
    DefaultTimeout = 300 * time.Second
)

var (
    // Labels list for test selection
    Labels = []string{LabelSuite, LabelFeatureName}

    // Reporter configuration
    ReporterCRDsToDump = []k8sreporter.CRData{...}
    ReporterNamespacesToDump = map[string]string{...}
)
```

#### Best Practices

1. **Constants**: Use for values that don't change (labels, namespaces, default values)
2. **Variables**: Use for values that may be configured or computed
3. **Timeouts**: Define reasonable defaults; make them configurable when needed
4. **Labels**: Aggregate in a `Labels` variable for suite-level filtering

### Environment Validation Package (`sriovenv` or similar)

Purpose: Validate cluster state and test prerequisites.

#### Responsibilities

- Verify operator deployment status
- Validate available hardware resources
- Check cluster configuration
- Discover and validate test interfaces

Example:
```go
func ValidateSriovInterfaces(nodeList []*nodes.Builder, minCount int) error {
    // Validation logic
}

func IsSriovDeployed(apiClient *client.Client, config *netconfig.Config) error {
    // Deployment check
}
```

### Code Organization and Reusability

#### Reusable Functions Location

**Important**: Reusable functions that can be used across multiple test suites should be placed in the upper-level `internal/` folder (e.g., `tests/internal/`), not in the test suite-specific `internal/` folder.

1. **Suite-specific helpers**: Place in `tests/ocp/sriov/internal/`
   - Functions specific to SR-IOV test suite only
   - Test parameters and constants specific to this suite

2. **Reusable/common helpers**: Place in `tests/internal/`
   - Functions that can be used by multiple test suites
   - Common utilities and helpers
   - Shared validation logic

Example structure:
```text
tests/
├── internal/                    # Reusable across all test suites
│   ├── cluster/                # Cluster-level utilities
│   ├── params/                 # Common parameters
│   └── reporter/               # Common reporting utilities
└── ocp/
    └── sriov/
        └── internal/            # SR-IOV suite-specific only
            ├── tsparams/       # SR-IOV test parameters
            └── sriovenv/        # SR-IOV environment validation
```

### Helper Function Guidelines

**Important**: Helper functions in `internal/` folders must follow these rules:

1. **No Gomega/Ginkgo imports in helpers**: Gomega and Ginkgo should only be imported in test suite files (e.g., `*_suite_test.go` and test files in `tests/` directory). Helper functions in `internal/` folders must not import Gomega or Ginkgo packages.

2. **No Eventually in internal folders**: The `Eventually` function from Gomega must not be used in any `internal/` folder. It is acceptable to use `Eventually` in test suite files where Gomega is properly imported.

3. **Helpers return errors**: Helper functions should always return errors instead of calling `Fail()` or using Gomega matchers. Let the test code handle failures using Gomega assertions.

Example of correct helper function:
```go
// ✅ Correct: Helper in internal/ folder
package sriovenv

func ValidateSriovInterfaces(nodeList []*nodes.Builder, minCount int) error {
    // Validation logic
    if len(nodeList) < minCount {
        return fmt.Errorf("insufficient nodes: got %d, need %d", len(nodeList), minCount)
    }
    // ... more validation
    return nil
}
```

Example of incorrect helper function:
```go
// ❌ Incorrect: Using Gomega in helper
package sriovenv

import . "github.com/onsi/gomega"  // ❌ Don't import Gomega in helpers

func ValidateSriovInterfaces(nodeList []*nodes.Builder, minCount int) {
    Expect(len(nodeList)).To(BeNumerically(">=", minCount))  // ❌ Don't use Gomega matchers
}
```

Example of correct test usage:
```go
// ✅ Correct: Test file using helper
package tests

import (
    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
    "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/sriovenv"
)

var _ = Describe("TestFeature", func() {
    It("should validate interfaces", func() {
        err := sriovenv.ValidateSriovInterfaces(nodeList, 2)
        Expect(err).ToNot(HaveOccurred(), "Failed to validate SR-IOV interfaces")
    })
})
```

## Test Implementation Rules

### Error Handling

1. **Always check errors**: Don't ignore errors; handle them appropriately
2. **Use Gomega matchers**: Prefer `Expect(err).ToNot(HaveOccurred())` for assertions
3. **Descriptive error messages**: Include context in error messages
4. **Matcher arguments support formatting**: Gomega matcher error messages support formatting (similar to `fmt.Printf`). Use `%q`, `%s`, `%d`, etc. to format values in error messages
5. **Add diagnostic callbacks**: When assertions fail, provide additional context using diagnostic callbacks. Pass a function that returns a string as the optional description parameter. This function will be called only when the assertion fails, allowing you to gather diagnostic information (e.g., resource states, logs, cluster conditions) at the time of failure
6. **Cleanup on failure**: Use `AfterEach` or `DeferCleanup` for resource cleanup

Example with diagnostic callback:
```go
// Simple error message with formatting
Expect(err).ToNot(HaveOccurred(), "Failed to create test namespace %q", testNS.Definition.Name)

// Error message with diagnostic callback for additional context
Expect(pod.IsReady()).To(BeTrue(), func() string {
    // This callback is only executed when the assertion fails
    podLogs, _ := pod.GetLogs()
    podStatus, _ := pod.GetStatus()
    return fmt.Sprintf("Pod %q is not ready. Status: %s\nPod logs:\n%s",
        pod.Definition.Name, podStatus, podLogs)
})
```

### Resource Management

1. **Create resources in BeforeAll/BeforeEach**: Set up test prerequisites
2. **Cleanup in AfterAll/AfterEach**: Always clean up created resources
3. **Use DeferCleanup**: For guaranteed cleanup even on test failure
4. **Unique names**: Use unique namespaces or resource names to avoid conflicts
5. **Recover existing CRs**: If tests delete or modify existing Custom Resources (CRs), capture them at `BeforeAll`/`BeforeSuite` level and restore them at `AfterAll`/`AfterSuite` level to maintain cluster state

Example:
```go
var existingCRs []*sriov.PolicyBuilder

BeforeAll(func() {
    // Capture existing CRs before test modifications
    existingCRs = captureExistingPolicies(APIClient)

    testNS := namespace.NewBuilder(APIClient, tsparams.TestNamespaceName)
    _, err := testNS.Create()
    Expect(err).ToNot(HaveOccurred(), "Failed to create test namespace %q", testNS.Definition.Name)
})

AfterAll(func() {
    // Restore captured CRs
    restoreExistingPolicies(APIClient, existingCRs)

    err := testNS.DeleteAndWait(tsparams.DefaultTimeout)
    Expect(err).ToNot(HaveOccurred(), "Failed to delete test namespace")
})
```

### Timeouts and Polling

1. **Use appropriate timeouts**: Don't use hardcoded timeouts; use constants from `tsparams`
2. **Consistent polling intervals**: Use `tsparams.RetryInterval` for consistency
3. **Eventually vs Consistently**:
   - Use `Eventually` for waiting for a condition to become true
   - Use `Consistently` for verifying a condition remains true
4. **Stable duration**: Use `StableFor` or similar when verifying stability
5. **Eventually for tests, WaitForX for helpers**:
   - **In test files**: Use `Eventually` from Gomega for polling and waiting
   - **In helper functions** (in `internal/` folders): Use either:
     - **eco-goinfra WaitFor methods**: Use built-in `WaitForX` methods from eco-goinfra builders (e.g., `pod.WaitForReady()`, `deployment.WaitForReady()`, `builder.WaitForCondition()`)
     - **wait.PollUntilContextTimeout**: Use `k8s.io/apimachinery/pkg/util/wait.PollUntilContextTimeout` for custom polling logic
   - Helper functions must not use `Eventually` as they cannot import Gomega

Example in test file:
```go
Eventually(func() bool {
    // Check condition
    return condition
}, tsparams.WaitTimeout, tsparams.RetryInterval).Should(BeTrue(), "Condition description")
```

Example in helper function using eco-goinfra WaitFor:
```go
// ✅ Correct: Using eco-goinfra WaitFor method
func WaitForPodReady(apiClient *clients.Settings, name, namespace string, timeout time.Duration) error {
    podBuilder, err := pod.Pull(apiClient, name, namespace)
    if err != nil {
        return fmt.Errorf("failed to pull pod: %w", err)
    }

    _, err = podBuilder.WaitForReady(timeout)
    return err
}
```

Example in helper function using wait.PollUntilContextTimeout:
```go
// ✅ Correct: Using wait.PollUntilContextTimeout for custom polling
import (
    "context"
    "time"
    "k8s.io/apimachinery/pkg/util/wait"
)

func WaitForResourceExists(apiClient *clients.Settings, name, namespace string, timeout time.Duration) error {
    err := wait.PollUntilContextTimeout(
        context.TODO(),
        3*time.Second,
        timeout,
        true,
        func(ctx context.Context) (bool, error) {
            resource, err := builder.Pull(apiClient, name, namespace)
            if err != nil {
                return false, nil // Continue polling
            }
            return resource.Exists(), nil
        })

    if err != nil {
        return fmt.Errorf("resource %q in namespace %q not found within %v: %w", name, namespace, timeout, err)
    }
    return nil
}
```

### Logging and Debugging

1. **Use By() statements**: Document test steps with `By("Description")`
2. **Glog for verbose logging**: Use `klog.V(level).Infof()` for detailed logging
3. **Environment variable control**: Respect `ECO_VERBOSE_LEVEL` for logging verbosity
4. **Meaningful messages**: Include context in log messages

Example:
```go
By("Creating SR-IOV network policy")
klog.V(90).Infof("Creating policy with name: %s", policyName)
```

### Test Isolation

1. **Independent tests**: Tests should be able to run independently
2. **Isolated namespaces**: Use separate namespaces per test suite
3. **No shared state**: Avoid shared state between tests unless using `Ordered`
4. **Cleanup verification**: Verify cleanup completed successfully

### Reporter Integration

1. **Report on failure**: Use `JustAfterEach` with `reporter.ReportIfFailed`
2. **Configure CRDs to dump**: Define in `tsparams.ReporterCRDsToDump`
3. **Configure namespaces**: Define in `tsparams.ReporterNamespacesToDump`
4. **XML reports**: Ensure suite file includes `ReportAfterSuite` for XML generation

Example:
```go
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

## Environment Variables and Configuration

### Configuration Management

1. **Centralized config**: Use configuration structs from `internal/netconfig` or similar
2. **Environment variable mapping**: Map environment variables to config structs
3. **Default values**: Provide sensible defaults
4. **Validation**: Validate required configuration before tests run

### Required Environment Variables

Document all required and optional environment variables:

```text
#### Required
- `KUBECONFIG`: Path to kubeconfig file

#### Optional
- `ECO_OCP_SRIOV_INTERFACE_LIST`: Comma-separated list of SR-IOV interfaces
- `ECO_OCP_SRIOV_TEST_IMAGE`: Container image for test workloads
```

### Environment Variable Naming

Follow the pattern: `ECO_{SUITE}_{FEATURE}_{PARAMETER}`

Examples:
- `ECO_OCP_SRIOV_INTERFACE_LIST`
- `ECO_OCP_SRIOV_TEST_IMAGE`
- `ECO_OCP_SRIOV_WORKER_LABEL`

## Code Quality and Best Practices

### Function Formatting

Follow the project's function formatting conventions:

```go
// Single line if arguments fit
func Function(arg1, arg2 int, arg3 string) error {
    // ...
}

// Multi-line if arguments don't fit
func Function(
    arg1 int,
    arg2 int,
    arg3 string,
    arg4 []string) error {
    // ...
}
```

### Use of eco-goinfra Packages

1. **All API calls must use eco-goinfra**: All Kubernetes API interactions must go through eco-goinfra packages. Do not use raw Kubernetes client calls directly.
2. **Prefer eco-goinfra**: Use eco-goinfra packages for Kubernetes resource management
3. **Avoid raw client calls**: Use builder patterns when available. Direct API client usage is not allowed.
4. **Check package availability**: Ensure required packages exist before using
5. **If eco-goinfra lacks functionality**: If a required feature is missing in eco-goinfra, contribute it to the eco-goinfra project rather than implementing raw client calls in test code

Common packages:
- `github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov`
- `github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod`
- `github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace`
- `github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes`

### Commit Message Guidelines

**Important**: All commit messages must identify the team in the commit title.

#### Commit Message Format

Commit title format has two parts:
1. **Team name prefix**: Must start with the team identifier (e.g., `ocp-sriov:`, `cnf network:`, `hw-accel:`)
2. **Short summary**: Brief description of the code changes

#### Commit Message Examples

```text
ocp-sriov: added metrics exporter test case
ocp-sriov: fixed timeout handling in network policy test
ocp-sriov: updated environment variable documentation
```

#### Rules for Commit Messages

- **Team prefix is mandatory**: Always prefix with team name (e.g., `ocp-sriov:`)
- **Use lowercase**: Team prefix should be lowercase with hyphens
- **No test IDs**: Don't include internal test IDs in commit messages
- **No capital letters**: Avoid capital letters in commit message titles
- **72 character limit**: Commit title should be limited to 72 characters
- **Description optional**: Detailed explanation can be added in commit description if needed

If a PR changes multiple team's directories or common infrastructure code, use `infra:` instead of team name.

### Test Documentation

1. **README updates**: Update README when adding new test suites or features
2. **Inline comments**: Comment complex logic or non-obvious decisions
3. **Test descriptions**: Make test descriptions clear and self-documenting
4. **Environment variables**: Document all environment variables in README

## Running Tests

### Test Execution

Tests should be runnable using the standard test runner:

```bash
export KUBECONFIG=/path/to/kubeconfig
export ECO_TEST_FEATURES="sriov"
export ECO_TEST_LABELS="sriov"
make run-tests
```

### Test Filtering

1. **By feature**: Use `ECO_TEST_FEATURES` to select test suites
2. **By label**: Use `ECO_TEST_LABELS` with ginkgo label filtering syntax
3. **By test ID**: Test IDs can be used as labels. Use `ECO_TEST_LABELS` with the test ID number to filter by test ID

Examples:
```bash
# Run only specific label
export ECO_TEST_LABELS="sriov && feature-name"

# Exclude specific label
export ECO_TEST_LABELS="sriov && !skip-in-ci"

# Run specific test ID (test IDs can be used as labels)
export ECO_TEST_LABELS="12345"

# Run multiple test IDs
export ECO_TEST_LABELS="12345 || 12346"
```

## Common Patterns

### Resource Creation Pattern

```go
resource := builder.NewBuilder(APIClient, name, namespace)
resource.WithLabel("key", "value")
resourceObject, err := resource.Create()
Expect(err).ToNot(HaveOccurred(), "Failed to create resource")
```

### Resource Cleanup Pattern

```go
DeferCleanup(func() {
    err := resource.Delete()
    Expect(err).ToNot(HaveOccurred(), "Failed to delete resource")
})
```

### Validation Pattern

```go
Eventually(func() bool {
    obj, err := builder.Pull(APIClient, name, namespace)
    if err != nil {
        return false
    }
    return obj.IsReady()
}, timeout, interval).Should(BeTrue(), "Resource should be ready")
```

### Test Data Pattern

```go
type testData struct {
    policy  *sriov.PolicyBuilder
    network *sriov.NetworkBuilder
    pod     *pod.Builder
}

var testResources []testData

BeforeEach(func() {
    testResources = []testData{}
})
```

## Pre-Submit Checklist

Before submitting a test case for review, ensure:

- [ ] Test follows directory structure conventions
- [ ] All required labels are defined in `tsparams/consts.go`
- [ ] Test IDs are included using `reportxml.ID()`
- [ ] Resources are properly cleaned up in `AfterEach` or `AfterAll`
- [ ] Error handling is comprehensive
- [ ] Timeouts use constants from `tsparams`
- [ ] `By()` statements document test steps
- [ ] Reporter is configured for failure reporting
- [ ] Environment variables are documented in README
- [ ] Test can run independently (unless using `Ordered`)
- [ ] Code passes linting (`make lint`)
- [ ] Test descriptions are clear and descriptive
- [ ] README is updated with new features or environment variables
- [ ] **All API calls use eco-goinfra packages** (no raw Kubernetes client calls)
- [ ] **Reusable functions are placed in upper-level `tests/internal/` folder**
- [ ] **Commit message includes team prefix** (e.g., `ocp-sriov: description`)

## Additional Resources

### Framework Documentation

- [Project README](../../../README.md): General project information
- [CNF Network README](../../cnf/core/network/README.md): Network test patterns
- [Ginkgo Documentation](https://onsi.github.io/ginkgo): Ginkgo framework reference

### Eco-goinfra Packages

- [Eco-goinfra README](https://github.com/rh-ecosystem-edge/eco-goinfra#readme): Package documentation
- [SR-IOV Package](https://github.com/rh-ecosystem-edge/eco-goinfra/tree/main/pkg/sriov): SR-IOV resource management

### Best Practices References

- Review existing test implementations in `tests/cnf/core/network/sriov/tests/`
- Follow patterns from other test suites in the project
- Consult team documentation for domain-specific guidelines
