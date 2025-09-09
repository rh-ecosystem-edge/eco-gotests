# Ecosystem Edge Core Team - Assisted ZTP

## Overview
Assisted ZTP tests are developed for the purpose of testing the infrastructure operator and spoke cluster features

### Prerequisites for running these tests:

1. Infrastructure or assisted-service operator deployed via upstream or through multicluster engine or advanced cluster management operators
2. Operand in healthy state [^1]

[^1]: assisted-service and assisted-image-service pods in `Ready` state

### Test suites:

| Name     | Description                                                                                         |
| -------- | --------------------------------------------------------------------------------------------------- |
| [operator](operator/operator_suite_test.go) | Tests that are run directly against the operator on the hub cluster without need of spoke cluster resources |
| [spoke](spoke/spoke_suite_test.go)    | Tests that are run on a combination of hub and spoke cluster resources                                      |

### Internal pkgs

[**ztpconfig**](internal/ztpconfig/config.go)
- Configuration structure used for embedding global test configuration
- Contains configuration for spoke cluster under test and contains inputs for spoke kubeconfig and clusterimageset

[**find**](internal/find/find.go)
- Helper for finding various resources on the hub and spoke clusters such as pods, cluster versions and spoke names

[**installconfig**](internal/installconfig/installconfig.go)
- Utility for creating InstallConfig structs from unstructured representations of install-configs

[**meets**](internal/meets/meets.go)
- Tool for discovering various details about the environment
- Used for determining if a given environment meets the criteria required by the test
- Examples include minimum OCP versions to run against, connected or disconnected environments, network configuration of the cluster, etc.

### Eco-goinfra pkgs

- [**assisted**](https://github.com/rh-ecosystem-edge/eco-goinfra/tree/main/pkg/assisted)
- [**hive**](https://github.com/rh-ecosystem-edge/eco-goinfra/tree/main/pkg/hive)

### Inputs
- `ECO_ASSISTED_ZTP_SPOKE_KUBECONFIG`: Location of the spoke cluster kubeconfig file
- `ECO_ASSISTED_ZTP_SPOKE_CLUSTERIMAGESET`: The clusterimageset that should be used by real/mocked spoke cluster resources

Please refer to the project README for a list of global inputs - [How to run](../../../README.md#how-to-run)

### Running Assisted ZTP Test Suites
```
# export KUBECONFIG=</path/to/hub/kubeconfig>
# export ECO_TEST_FEATURES=ztp
# export ECO_TEST_LABELS="assisted,ztp"
# export ECO_ASSISTED_ZTP_SPOKE_KUBECONFIG=</path/to/spoke/kubeconfig>
# export ECO_ASSISTED_ZTP_SPOKE_CLUSTERIMAGESET=4.14
# make run-tests
```
### Additional Information
Assisted ZTP tests have two different modes:

1. Tests that can run on any cluster regardless of the cluster's configuration 

These are test that can fully encompass their own setup and teardown

Example: Test that an AgentClusterInstall validation fails due to a specific input. ([Reference](operator/tests/platform-selection-test.go))

Users can create the resources necessary to produce the desired state, check that the values are reported as intended, and then teardown the test resources.

2. Tests that run against a cluster that has a specific configuration

These tests inspect the environment that they are running on and selectively include or exclude tests based on how the cluster is configured

Example: Test that a spoke can successfully be deployed with the OpenShiftSDN network type. ([Reference](spoke/tests/check_network_type.go))

After reading the AgentClusterInstall .spec.networking.NetworkType value, users can infer if the spoke was deployed with OpenShiftSDN or not. If true, the user can continue with the test. If false, the user will skip the test since the environment doesn't match the configuration needed.
