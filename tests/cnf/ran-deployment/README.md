# Ecosystem Telco FT Team - CNF vRAN Deployment

## Overview

CNF vRAN tests clusters for deployment and policies kinds.

### Prerequisites for running these tests

Designed for supported OCP versions with the following installed:

* Core OCP
* DU profile for SNO
* Multi-node clusters with appropriate profile
* Multiple-cluster (max 2) deployments.

The deploymenttypes test suite logics reports a PASS result when a matching deployment type, policy kind, or number of cluster deployments are matched. Non-matching tests are SKIPPED. This allows this suite to run on multiple environments so that the
results can be aggregated in a "union" set of results.

### Test suites

| Name                                                             | Description                                         |
|------------------------------------------------------------------|-----------------------------------------------------|
| [deploymenttypes](deploymenttypes/deployment_suite_test.go)      | Tests for various policy and deployment configs     |

### Internal pkgs

| Name                                                 | Description                                                 |
|------------------------------------------------------|-------------------------------------------------------------|
| [ranconfig](internal/ranconfig/config.go)            | Configures environment variables and default values         |
| [raninittools](internal/raninittools/raninitools.go) | Provides an APIClient for access to cluster                 |
| [ranparam](internal/ranparam/const.go)               | Labels and other constants used in the test suites          |
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

#### Other environment variables

* `ECO_CNF_RAN_SKIP_TLS_VERIFY`: Boolean for allowing the go-git library to skip TLS certificate validation for internal repositories.
  * Set to either `true` or `false`. Default is `false`.

### Running the RAN deployment test suites

See below.

#### Running the power management test suite

```bash
# export KUBECONFIG=<spoke1_kubeconfig>
# export ECO_CNF_RAN_KUBECONFIG_HUB=<hub_kubeconfig>
## optional
#ECO_CNF_RAN_KUBECONFIG_SPOKE2=<spoke2_kubeconfig>
#make run-tests

```
