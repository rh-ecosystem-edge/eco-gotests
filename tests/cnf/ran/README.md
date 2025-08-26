# Ecosystem Telco FT Team - CNF vRAN

## Overview

CNF vRAN tests clusters for use in vRAN deployments.

### Prerequisites for running these tests

Designed for supported OCP versions with the following installed:

* Core OCP
* DU profile

Some test suites (ZTP, TALM) require clusters to be deployed via GitOps ZTP and managed by a hub cluster.

### Test suites

| Name                                                             | Description                                         |
|------------------------------------------------------------------|-----------------------------------------------------|
| [containernshide](containernshide/containernshide_suite_test.go) | Tests that containers have a hidden mount namespace |
| [powermanagement](powermanagement/powermanagement_suite_test.go) | Tests powersave settings using workload hints       |
| [talm](talm/talm_suite_test.go)                                  | Tests the topology aware lifecycle manager (TALM)   |
| [gitopsztp](gitopsztp/ztp_suite_test.go)                         | Tests zero touch provisioning (ZTP) and Argo CD     |

### Internal pkgs

| Name                                                 | Description                                                 |
|------------------------------------------------------|-------------------------------------------------------------|
| [rancluster](internal/rancluster/rancluster.go)      | Helpers for viewing the state of a cluster itself           |
| [ranhelper](internal/ranhelper/ranhelper.go)         | Common helpers that do not fit into a more specific package |
| [ranconfig](internal/ranconfig/config.go)            | Configures environment variables and default values         |
| [raninittools](internal/raninittools/raninitools.go) | Provides an APIClient for access to cluster                 |
| [ranparam](internal/ranparam/const.go)               | Labels and other constants used in the test suites          |
| [stats](internal/stats/stats.go)                     | Basic statistics functions with unit tests                  |
| [version](internal/version/version.go)               | Allows getting and checking cluster and operator versions   |

### Eco-goinfra pkgs

[**README**](https://github.com/rh-ecosystem-edge/eco-goinfra#readme)

### Inputs

Please refer to the project README for a list of global inputs - [How to run](../../../README.md#how-to-run).

For the optional inputs listed below, see [default.yaml](internal/ranconfig/default.yaml) for the default values.

#### Kubeconfigs

Currently, only the TALM tests need more than the first spoke kubeconfig.

* `KUBECONFIG`: Global input that refers to the first spoke cluster.
* `ECO_CNF_RAN_KUBECONFIG_HUB`: For tests that need a hub cluster, this is the path to its kubeconfig.
* `ECO_CNF_RAN_KUBECONFIG_SPOKE2`: For tests that need a second spoke cluster, this is the path to its kubeconfig.

#### BMC credentials

Only the powermanagement and TALM pre-cache tests need BMC credentials.

* `ECO_CNF_RAN_BMC_USERNAME`: Username used for the Redfish API.
* `ECO_CNF_RAN_BMC_PASSWORD`: Password used for the Redfish API.
* `ECO_CNF_RAN_BMC_HOSTS`: IP address (without the leading `https://`) used for the Redfish API. Can be comma separated, but only the first host IP will be used.
* `ECO_CNF_RAN_BMC_TIMEOUT`: Timeout in the form of a Go duration string to use when connecting to the Redfish API. Defaults to 15s which should usually be plenty.

#### Power management inputs

All of these inputs are optional.

* `ECO_CNF_RAN_METRIC_SAMPLING_INTERVAL`: Time between samples when gathering power usage metrics.
* `ECO_CNF_RAN_NO_WORKLOAD_DURATION`: Duration to sample power usage metrics for the no workload scenario.
* `ECO_CNF_RAN_WORKLOAD_DURATION`: Duration to sample power usage metrics for the workload scenario.
* `ECO_CNF_RAN_STRESSNG_TEST_IMAGE`: Container image to use for the workload pods during the workload scenario.
* `ECO_CNF_RAN_TEST_IMAGE`: Container image to use for testing container resource limits.

#### TALM pre-cache inputs

These inputs are all specific to the TALM pre-cache tests. They are also all optional.

* `ECO_CNF_RAN_OCP_UPGRADE_UPSTREAM_URL`: URL of upstream upgrade graph.
* `ECO_CNF_RAN_PTP_OPERATOR_NAMESPACE`: Namespace that the PTP operator uses.
* `ECO_CNF_RAN_TALM_PRECACHE_POLICIES`: List of policies to copy for the precache operator tests.

#### ZTP generator inputs

This input is specific to the ZTP generator tests and is optional.

- `ECO_CNF_RAN_ZTP_SITE_GENERATE_IMAGE`: Container image to use for generating CRs from the site config.

### Running the RAN test suites

Except for the container namespace hiding tests, a dump of relevant CRs will be generated for failed tests only when `ECO_ENABLE_REPORT=true`.

#### Running the container namespace hiding test suite

```bash
# export KUBECONFIG=</path/to/spoke/kubeconfig>
# export ECO_TEST_FEATURES=containernshide
# make run-tests
```

#### Running the power management test suite

```bash
# export KUBECONFIG=</path/to/spoke/kubeconfig>
# export ECO_TEST_FEATURES=powermanagement
# export ECO_CNF_RAN_BMC_USERNAME=<bmc username>
# export ECO_CNF_RAN_BMC_PASSWORD=<bmc password>
# export ECO_CNF_RAN_BMC_HOSTS=<bmc ip address>
# make run-tests
```

If using more selective labels that do not include the powersaving tests, such as `ECO_TEST_LABELS="powermanagement && !powersave"`, then the `ECO_CNF_RAN_BMC_*` environment variables are not required.

#### Running the TALM test suite

```bash
# export KUBECONFIG=</path/to/spoke/kubeconfig>
# export ECO_TEST_FEATURES=talm
# export ECO_CNF_RAN_KUBECONFIG_HUB=</path/to/hub/kubeconfig>
# export ECO_CNF_RAN_KUBECONFIG_SPOKE2=</path/to/spoke2/kubeconfig>
# export ECO_CNF_RAN_BMC_USERNAME=<bmc username>
# export ECO_CNF_RAN_BMC_PASSWORD=<bmc password>
# export ECO_CNF_RAN_BMC_HOSTS=<bmc ip address>
# make run-tests
```

If using more selective labels that do not include TALM pre-cache, such as with `ECO_TEST_LABELS="talm && !precache"`, then the `ECO_CNF_RAN_BMC_*` environment variables are not required.

#### Running the ZTP test suite

```
# export KUBECONFIG=</path/to/spoke/kubeconfig>
# export ECO_TEST_FEATURES=ran
# export ECO_TEST_LABELS="ran-ztp && !no-container"
# export ECO_CNF_RAN_KUBECONFIG_HUB=</path/to/hub/kubeconfig>
# make run-tests
```

The ZTP generator test cannot be run in a container and thus has the `no-container` label. To run it, set `ECO_TEST_LABELS="ran-ztp && no-container"`.

### Additional Information

Note that excluding a label using `ECO_TEST_LABELS=!my-label` may require `set +H` in the shell first. If not, you may see errors like `bash: !my: event not found`.
