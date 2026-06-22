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

#### PTP inputs

These inputs are all specific to the PTP test suites and are optional.

* `ECO_CNF_RAN_PTP_STABILITY_DURATION`: Duration for PTP stability analysis (Go duration string).
* `ECO_CNF_RAN_PTP_STABILITY_THRESHOLD`: Absolute offset threshold in nanoseconds for PTP stability analysis.
* `ECO_CNF_RAN_PTP_EVENT_CONSUMER_IMAGE`: URL of the PTP event consumer image (without tag).
* `ECO_CNF_RAN_PTP_EVENT_CONSUMER_V1_TAG`: Tag of the PTP event consumer image for v1 (include leading colon).
* `ECO_CNF_RAN_PTP_EVENT_CONSUMER_V2_TAG`: Tag of the PTP event consumer image for v2 (include leading colon).
* `ECO_CNF_RAN_PTP_MUST_GATHER_IMAGE`: Image to use for PTP must-gather. Falls back to CSV annotation or registry.redhat.io if unset.

#### Spoke inputs

* `ECO_CNF_RAN_SPOKE1_NAME`: Name of the spoke 1 cluster. Automatically updated if Spoke1Kubeconfig exists, otherwise provided as input.
* `ECO_CNF_RAN_SPOKE1_HOSTNAME`: Hostname for the spoke 1 cluster, used as input for the O-RAN suite.
* `ECO_CNF_RAN_SPOKE1_PASSWORD`: Path to the admin password for spoke 1, saved in the O-RAN suite.

#### ACM inputs

* `ECO_CNF_RAN_ACM_OPERATOR_NAMESPACE`: Namespace that the ACM operator uses.

#### O-RAN inputs

These inputs are specific to the O-RAN test suite.

* `ECO_CNF_RAN_HUB_APPS_DOMAIN`: Subdomain for the hub cluster routes (e.g. `apps.<hub-cluster-name>.<hub-cluster-domain>`).
* `ECO_CNF_RAN_O2IMS_CLIENT_CERT_SECRET`: Name of the secret containing the client certificate for O2IMS and OAuth APIs.
* `ECO_CNF_RAN_O2IMS_CLIENT_CERT_SECRET_NAMESPACE`: Namespace for the O2IMS client certificate secret.
* `ECO_CNF_RAN_O2IMS_OAUTH_CLIENT_ID`: Client ID for requesting an access token from the OAuth endpoint.
* `ECO_CNF_RAN_O2IMS_OAUTH_CLIENT_SECRET`: Client secret for requesting an access token from the OAuth endpoint.
* `ECO_CNF_RAN_O2IMS_TOKEN`: Token for authenticating with the O2IMS API (used when OAuth is not configured).
* `ECO_CNF_RAN_CLUSTER_TEMPLATE_AFFIX`: Version-dependent affix for naming ClusterTemplates and O-RAN resources.

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
