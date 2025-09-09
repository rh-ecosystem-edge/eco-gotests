# Ecosystem Edge Hardware Accelerators - KMM 

## Overview
KMM tests are developed for the purpose of testing the KMM (Kernel Module Management) and KMM-Hub operators features.

The Kernel Module Management Operator manages the deployment of out-of-tree kernel modules and associated device plug-ins in Kubernetes. Along with deployment it also manages the lifecycle of the kernel modules for new incoming kernel versions attached to upgrades.

The Kernel Module Management Hub Operator manages the deployment of out-of-tree kernel modules and associated device plug-ins in a Hub/Spoke cluster architecture. 

### Prerequisites for running these tests:

1. KMM or KMM-Hub operators deployed before tests are executed.
2. KMM-Hub requires ACM/MCE and additional spoke cluster deployed.

### Test suites:

| Name                                        | Description                              |
|---------------------------------------------|------------------------------------------|
| [1upgrade](1upgrade/upgrade_suite_test.go)  | Tests related to KMM operator upgrade    |
| [mcm](mcm/mcm_suite_test.go)                | Tests performed against KMM-Hub operator |
| [modules](modules/modules_suite_test.go)    | Tests performed against KMM operator     |

Notes: 
- `1upgrade` name is used to assure that ginkgo runs that before modules.
- `mcm` stands for ManagedClusterModule, which is a CRD that wraps a Module in a Hub/Spoke environment.

### Internal pkgs

[**await**](internal/await/await.go)
- Helper for waiting for various states of builds, module, preflight.

[**check**](internal/check/check.go)
- Tool that checks different states for module, dmesg, node labels.

[**define**](internal/define)
- Utility that helps create custom objects like clusterrolebinding, configmap and secret used in tests.

[**get**](internal/get/get.go)
- Utility used to obtain various variables like number of nodes for selector, cluster architecture, node kernel version.

### Eco-goinfra pkgs

- [**kmm**](https://github.com/rh-ecosystem-edge/eco-goinfra/tree/main/pkg/kmm)

### Inputs

#### Modules related
- `ECO_HWACCEL_KMM_PULL_SECRET`: External registry pull-secret 
- `ECO_HWACCEL_KMM_REGISTRY`: External registry url (eg: quay.io/ocp-edge-qe )
- `ECO_HWACCEL_KMM_DEVICE_PLUGIN_IMAGE`: Image used for the device-plugin test. If the image tag includes `%s` it will be replaced with the architecture ( amd64 / arm64 )

#### Upgrade related
- `ECO_HWACCEL_KMM_SUBSCRIPTION_NAME`: Name of subscription used to deploy the KMM operator
- `ECO_HWACCEL_KMM_CATALOG_SOURCE_NAME`: Name of the catalog source used for performing upgrade
- `ECO_HWACCEL_KMM_UPGRADE_TARGET_VERSION`: Expected version of the operator after upgrade

#### MCM related
- `ECO_HWACCEL_KMM_SPOKE_KUBECONFIG`: Path for the spoke cluster kubeconfig
- `ECO_HWACCEL_KMM_SPOKE_CLUSTER_NAME`: Spoke cluster name

In case these inputs are not set, the tests are skiped.

Please refer to the project README for a list of global inputs - [How to run](../../../README.md#how-to-run)

### Running HW-Accel KMM Test Suites

#### Running KMM tests
```
# export ECO_TEST_FEATURES='1upgrade modules'
# export ECO_TEST_LABELS=KMM
# export ECO_HWACCEL_KMM_REGISTRY=quay.io/<your_org>
# export ECO_HWACCEL_KMM_PULL_SECRET=<pullsecret>
# export ECO_HWACCEL_KMM_SUBSCRIPTION_NAME='kernel-module-management-subscription'
# export ECO_HWACCEL_KMM_UPGRADE_TARGET_VERSION='2.0.1'
# export ECO_HWACCEL_KMM_CATALOG_SOURCE_NAME='kmm-brew-catsrc'
# make run-tests
```

#### Running KMM-HUB tests
```
# export ECO_TEST_FEATURES=mcm
# export ECO_TEST_LABELS=mcm
# mare run-tests
```

### Additional Information
KMM and KMM-Hub tests have completely different requirements.  
