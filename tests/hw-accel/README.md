# Ecosystem Edge Hardware Accelerators - NFD

## Overview
NFD tests are developed for the purpose of testing the deployment of the Node Feature Discovery (NFD) operator and its respective NodeFeatureDiscovery Custom Resource instance to automatically detect and label hardware and software capabilities of cluster nodes.

The Node Feature Discovery Operator manages the deployment of out-of-tree kernel modules and associated device plug-ins in Kubernetes. It discovers hardware features available on each node in a Kubernetes cluster, and advertises those features using node labels.

### Prerequisites and Supported setups
* Regular cluster with multiple master nodes (VMs or BMs) and workers (VMs or BMs)/SNO
* Public Clouds Cluster (AWS, GCP and Azure)
* On Premise Cluster

### Test suites:

| Name                                          | Description                                                          |
|-----------------------------------------------|----------------------------------------------------------------------|
| [features](features/nfd_suite_test.go)        | Tests related to NFD operator deployment and feature discovery      |
| [2upgrade](2upgrade/upgrade_suite_test.go)    | Tests related to NFD operator upgrade functionality                 |

Notes:
- `2upgrade` name is used to assure that ginkgo runs that before features.
- `features` contains the main NFD functionality tests including label discovery, pod status checks, and node feature detection.

### Internal pkgs

[**get**](internal/get)
- Utilities to obtain various objects like pod status, logs, restart count, node feature labels, CPU features, and resource counts.

[**wait**](internal/wait/wait.go)
- Helpers for waiting for various states of pods, nodes, and label discovery completion.

[**set**](internal/set/featurelist.go)
- Helper for setting CPU feature configurations including blacklist/whitelist management and custom NFD worker configurations.

[**nfdconfig**](internal/nfdconfig/config.go)
- Configuration management that captures and processes environment variables used for NFD test execution.

[**nfddelete**](internal/nfddelete/feature_labels.go)
- Helpers for cleaning up NFD-specific labels from cluster nodes and custom resources after test execution.
- **`custom_resources.go`**: Comprehensive NFD custom resource cleanup with finalizer handling for operator uninstallation.
- **`feature_labels.go`**: Node label cleanup utilities for test isolation.

[**search**](internal/search/common_utils.go)
- Common utilities for searching and string manipulation operations.

[**nfdhelpersparams**](internal/nfdhelpersparams)
- Constants and variables for NFD helper functions including valid pod name lists and container definitions.

[**params**](internal/params/consts.go)
- Configuration constants including default NFD worker configuration templates.

### Eco-goinfra pkgs

- [**nfd**](https://github.com/rh-ecosystem-edge/eco-goinfra/tree/main/pkg/nfd)
- [**olm**](https://github.com/rh-ecosystem-edge/eco-goinfra/tree/main/pkg/olm)
- [**nodes**](https://github.com/rh-ecosystem-edge/eco-goinfra/tree/main/pkg/nodes)
- [**pod**](https://github.com/rh-ecosystem-edge/eco-goinfra/tree/main/pkg/pod)

### Inputs and Environment Variables

Parameters for the script are controlled by the following environment variables:

#### Feature Discovery related
- `ECO_HWACCEL_NFD_CR_IMAGE`: Custom NFD container image to use for NodeFeatureDiscovery CR. If not specified, the default operator image is used - _optional_
- `ECO_HWACCEL_NFD_CATALOG_SOURCE`: Custom catalog source to be used for NFD operator installation. If not specified, the default "certified-operators" catalog is used - _optional_
- `ECO_HWACCEL_NFD_AWS_TESTS`: Enable AWS-specific tests including day2 worker node addition. Set to "true" when running NFD tests against AWS clusters - _optional_
- `ECO_HWACCEL_NFD_CPU_FLAGS_HELPER_IMAGE`: Container image used for CPU flags helper tests. Required when running AWS day2 worker tests - _required for AWS tests_

#### Upgrade related
- `ECO_HWACCEL_NFD_SUBSCRIPTION_NAME`: Name of subscription used to deploy the NFD operator - _required for upgrade tests_
- `ECO_HWACCEL_NFD_CUSTOM_NFD_CATALOG_SOURCE`: Custom catalog source name used for performing operator upgrades - _required for upgrade tests_
- `ECO_HWACCEL_NFD_UPGRADE_TARGET_VERSION`: Expected version of the operator after upgrade completion - _required for upgrade tests_

#### General Test Framework Variables
- `ECO_TEST_LABELS`: ginkgo query passed to the label-filter option for including/excluding tests - _optional_
- `ECO_VERBOSE_SCRIPT`: prints verbose script information when executing the script - _optional_
- `ECO_TEST_VERBOSE`: executes ginkgo with verbose test output - _optional_
- `ECO_TEST_TRACE`: includes full stack trace from ginkgo tests when a failure occurs - _optional_
- `ECO_TEST_FEATURES`: list of features to be tested. Should include "nfd" for NFD tests - _required_

In case the required inputs are not set, the tests are skipped.

### Running HW-Accel NFD Test Suites

#### Running NFD Feature Discovery tests
```bash
$ export KUBECONFIG=/path/to/kubeconfig
$ export ECO_DUMP_FAILED_TESTS=true
$ export ECO_REPORTS_DUMP_DIR=/tmp/eco-gotests-logs-dir
$ export ECO_TEST_FEATURES="nfd"
$ export ECO_TEST_LABELS='nfd,discovery-of-labels'
$ export ECO_VERBOSE_LEVEL=100
$ export ECO_HWACCEL_NFD_CATALOG_SOURCE="certified-operators"
$ export ECO_HWACCEL_NFD_CR_IMAGE="registry.redhat.io/openshift4/ose-node-feature-discovery:latest"
$ make run-tests
```

#### Running NFD tests on AWS with day2 worker addition
```bash
$ export KUBECONFIG=/path/to/kubeconfig
$ export ECO_TEST_FEATURES="nfd"
$ export ECO_TEST_LABELS='nfd'
$ export ECO_HWACCEL_NFD_AWS_TESTS=true
$ export ECO_HWACCEL_NFD_CPU_FLAGS_HELPER_IMAGE="<cpu-flags-helper image specific to cluster architecture>"
$ export ECO_HWACCEL_NFD_CATALOG_SOURCE="certified-operators"
$ make run-tests
```

#### Running NFD Upgrade tests
```bash
$ export KUBECONFIG=/path/to/kubeconfig
$ export ECO_TEST_FEATURES="nfd"
$ export ECO_TEST_LABELS='nfd_upgrade'
$ export ECO_HWACCEL_NFD_SUBSCRIPTION_NAME='nfd-subscription'
$ export ECO_HWACCEL_NFD_UPGRADE_TARGET_VERSION='4.17.0'
$ export ECO_HWACCEL_NFD_CUSTOM_NFD_CATALOG_SOURCE='custom-nfd-catalog'
$ export ECO_HWACCEL_NFD_CATALOG_SOURCE="certified-operators"
$ make run-tests
```

### Test Scenarios

The NFD test suite covers the following scenarios:

1. **Pod Status Verification**: Ensures all NFD pods (controller-manager, master, worker, topology) are running correctly
2. **Log Analysis**: Checks NFD pod logs for error messages and exceptions
3. **Restart Count Monitoring**: Verifies that NFD pods have zero restart counts
4. **Feature Label Discovery**: Tests detection and labeling of CPU features, kernel configurations, and hardware capabilities
5. **NUMA Detection**: Validates NUMA topology detection and labeling (when supported)
6. **Blacklist/Whitelist Functionality**: Tests CPU feature filtering using blacklist and whitelist configurations
7. **Day2 Worker Addition**: Tests NFD functionality when adding new worker nodes to AWS clusters
8. **Operator Upgrade**: Tests upgrading NFD operator to newer versions

### Generic Operator Installer and NFD CR Utilities

The NFD tests now use a modern, clean approach with generic operator installer and standalone custom resource utilities:

#### Basic NFD Deployment
```go

installConfig := nfdhelpers.GetStandardNFDConfig(apiClient)
installer := deploy.NewOperatorInstaller(installConfig)
err := installer.Install()

// Wait for operator readiness
ready, err := installer.IsReady(5 * time.Minute)

/
crUtils := deploy.NewNFDCRUtils(apiClient, "openshift-nfd")

nfdConfig := deploy.NFDCRConfig{
    EnableTopology: true,
    Image:          "registry.redhat.io/openshift4/ose-node-feature-discovery:latest",
}

err = crUtils.DeployNFDCR("nfd-instance", nfdConfig)

// Wait for CR readiness
crReady, err := crUtils.IsNFDCRReady("nfd-instance", 3*time.Minute)
```

#### NFD with Custom Configuration
```go
// Use helper with custom catalog source
options := &nfdhelpers.NFDInstallConfigOptions{
    CatalogSource: nfdhelpers.StringPtr("custom-catalog"),
}
installConfig := nfdhelpers.GetDefaultNFDInstallConfig(apiClient, options)
installer := deploy.NewOperatorInstaller(installConfig)
```

#### NFD with Custom Worker Configuration
```go
// Deploy NFD CR with custom worker configuration for CPU features blacklist/whitelist
workerConfig := `
sources:
  cpu:
    cpuid:
      attributeBlacklist: ["BMI1", "BMI2"]
      attributeWhitelist: ["SSE", "SSE2"]
`

err = crUtils.DeployNFDCRWithWorkerConfig("nfd-custom", nfdConfig, workerConfig)
```

#### Clean Uninstallation
```go
// Individual CR deletion (for tests)
err := nfdCRUtils.DeleteNFDCR("nfd-instance")

// Complete uninstallation with automatic CR cleanup
uninstallConfig := nfdhelpers.GetDefaultNFDUninstallConfig(
    apiClient,
    "nfd-operator-group",
    "nfd-subscription")
uninstaller := deploy.NewOperatorUninstaller(uninstallConfig)
err = uninstaller.Uninstall()
// ✅ Automatically deletes CRs first (including finalizer handling)
// ✅ Then removes operator, subscription, operator group
```

#### Alternative: Direct CR Cleanup
```go
// Direct cleanup of all NFD CRs (useful for tests)
err := nfddelete.AllNFDCustomResources(apiClient, "openshift-nfd")

// Or cleanup specific CRs
err := nfddelete.AllNFDCustomResources(apiClient, "openshift-nfd",
    "nfd-instance", "nfd-instance-custom")
```

### Benefits of the New Approach

1. **Clean Separation**: Operator installation is separate from custom resource management
2. **Reusable Components**: The generic installer works for NFD, KMM, AMD GPU, and other operators
3. **Standalone Utilities**: NFD CR utilities can be used independently of operator installation
4. **Better Error Handling**: Comprehensive validation and improved logging for troubleshooting
5. **Simplified Usage**: Direct, intuitive API without complex factory patterns
6. **Configuration Helpers**: NFD-specific helper functions eliminate repetitive configuration setup
7. **Reduced Duplication**: Common NFD patterns are centralized in reusable helper functions

### Additional Information

NFD tests verify that hardware features are properly detected and labeled on cluster nodes. The tests support both standard feature detection and custom configurations through worker config modifications. Special attention is given to CPU feature detection, topology awareness, and proper cleanup of labels after test execution.

The new deployment framework provides better reliability and easier maintenance while supporting all existing test scenarios and configurations.