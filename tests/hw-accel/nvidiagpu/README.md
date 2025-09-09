# Ecosystem Edge Hardware Accelerators - NVIDIAGPU 

## Overview
NVIDIAGPU tests are developed for the purpose of testing the deployment of the NVIDIA GPU Operator and its respective 
ClusterPolicy Custom Resource instance to build and load in memory the kernel drivers needed to run a GPU workload.
These testcases rely on the Node Feature Discovery Operator to be deployed with a NodeFeatureDiscovery custom resource
instance in order to label the cluster worker nodes with nvidia gpu-specific labels.

### Prerequisites and Supported setups
* Regular cluster 3 master nodes (VMs or BMs) and minimum of 2 workers (VMs or BMs)
* Single Node Cluster (VM or BM)
* Public Clouds Cluster (AWS, GCP and Azure)
* On Premise Cluster

### Test suites:

| Name                                          | Description                                                          |
|-----------------------------------------------|----------------------------------------------------------------------|
| [gpu_suite_test](gpudeploy/gpu_suite_test.go) | Tests related to deploy NFD and GPU operators and run a GPU workload |


### Internal pkgs

[**check**](internal/check/check.go)
- Helpers to check for different objects states and node labels.

[**deploy**](internal/deploy/deploy-nfd.go)
- Helpers to manage deploying and un-deploying NFD.

[**get**](internal/get/get.go)
- Helpers to obtain various objects like pod objects with specific names, cluster architecture, clusterserviceversion (CSV) from Subscription, installed CSVs, etc. versions.

[**gpu-burn**](internal/gpu-burn/gpu-burn.go)
- Helpers to build the gpu-burn pod and create the config map needed to run a GPU workload.

[**nvidiagpuconfig**](internal/nvidiagpuconfig/config.go)
- Helpers to capture and process environment variables used before starting a test.

[**wait**](internal/wait/wait.go)
- Helpers for waiting for various states of deployments, and other objects created.

### Eco-goinfra pkgs

- [**nfd**](https://github.com/rh-ecosystem-edge/eco-goinfra/tree/main/pkg/nfd)
- [**olm**](https://github.com/rh-ecosystem-edge/eco-goinfra/tree/main/pkg/olm)
- [**nvidiagpu**](https://github.com/rh-ecosystem-edge/eco-goinfra/tree/main/pkg/nvidiagpu)

### Inputs and General environment variables

Parameters for the script are controlled by the following environment variables:
- `ECO_TEST_LABELS`: ginkgo query passed to the label-filter option for including/excluding tests - _optional_
- `ECO_VERBOSE_SCRIPT`: prints verbose script information when executing the script - _optional_
- `ECO_TEST_VERBOSE`: executes ginkgo with verbose test output - _optional_
- `ECO_TEST_TRACE`: includes full stack trace from ginkgo tests when a failure occurs - _optional_
- `ECO_TEST_FEATURES`: list of features to be tested.  Subdirectories under `tests` dir that match a feature will be included (internal directories are excluded).  When we have more than one subdirectory ot tests, they can be listed comma separated.- _required_
- `ECO_HWACCEL_NVIDIAGPU_INSTANCE_TYPE`: Use only when cluster is on a public cloud, and when you need to scale the cluster to add a GPU-enabled compute node. If cluster already has a GPU enabled worker node, this variable should be unset.
    - Example instance type: "g4dn.xlarge" in AWS, or "a2-highgpu-1g" in GCP, or "Standard_NC4as_T4_v3" in Azure - _required when need to scale cluster to add GPU node_
- `ECO_HWACCEL_NVIDIAGPU_CATALOGSOURCE`: custom catalogsource to be used.  If not specified, the default "certified-operators" catalog is used - _optional_
- `ECO_HWACCEL_NVIDIAGPU_SUBSCRIPTION_CHANNEL`: specific subscription channel to be used.  If not specified, the latest channel is used - _optional_
- `ECO_HWACCEL_NVIDIAGPU_GPUBURN_IMAGE`: GPU burn container image specific to cluster architecture _required_

It is recommended to execute the runner script through the `make run-tests` make target.

### Running HW-Accel NVIDIAGPU Test Suites

#### Running GPU tests
```
$ export KUBECONFIG=/path/to/kubeconfig
$ export ECO_DUMP_FAILED_TESTS=true
$ export ECO_REPORTS_DUMP_DIR=/tmp/eco-gotests-logs-dir
$ export ECO_TEST_FEATURES="nvidiagpu"
$ export ECO_TEST_LABELS='nvidiagpu,48452'
$ export ECO_VERBOSE_LEVEL=100
$ export ECO_HWACCEL_NVIDIAGPU_INSTANCE_TYPE="g4dn.xlarge"
$ export ECO_HWACCEL_NVIDIAGPU_CATALOGSOURCE="certified-operators"
$ export ECO_HWACCEL_NVIDIAGPU_SUBSCRIPTION_CHANNEL="v23.9"
$ export ECO_HWACCEL_NVIDIAGPU_GPUBURN_IMAGE="<gpu-burn image to run, specific to cluster architecture>"
$ make run-tests                    
Executing eco-gotests test-runner script
scripts/test-runner.sh
ginkgo -timeout=24h --keep-going --require-suite -r --label-filter="nvidiagpu,48452" ./tests/nvidiagpu
```

In case the required inputs are not set, the tests are skiped.

Please refer to the project README for a list of global inputs - [How to run](../../../README.md#how-to-run)
