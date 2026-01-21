# Ecosystem Edge Hardware Accelerators - AWS Neuron

## Overview

AWS Neuron tests are developed for the purpose of testing the deployment of the AWS Neuron Operator and its respective DeviceConfig Custom Resource instance to install Neuron drivers, deploy device plugins, and enable AI/ML workloads on AWS Inferentia and Trainium accelerators.

The AWS Neuron Operator manages AWS AI accelerators on OpenShift clusters running on AWS. It leverages:
- **Kernel Module Management (KMM)** for driver installation
- **Node Feature Discovery (NFD)** for node labeling based on PCI device detection
- **Custom Scheduler** for Neuron-aware pod scheduling

### Prerequisites and Supported Setups

* OpenShift 4.14+ cluster on AWS
* Worker nodes with AWS Inferentia (Inf1, Inf2) or Trainium (Trn1) instances
* NFD Operator installed
* KMM Operator installed

### Test Suites

| Name                                          | Description                                                          |
|-----------------------------------------------|----------------------------------------------------------------------|
| [vllm](vllm/vllm_suite_test.go)               | Tests vLLM inference workload deployment on Neuron devices          |
| [metrics](metrics/metrics_suite_test.go)      | Tests metrics provisioning and ServiceMonitor functionality         |
| [3upgrade](3upgrade/upgrade_suite_test.go)    | Tests rolling upgrade of Neuron drivers across cluster nodes        |

Notes:
- `3upgrade` follows the naming convention: kmm=1upgrade, nfd=2upgrade, neuron=3upgrade
- `vllm` tests require a vLLM image with Neuron support
- `metrics` tests verify Prometheus scraping and metric availability

### Internal Packages

[**neuronconfig**](internal/neuronconfig/config.go)
- Configuration management that captures and processes environment variables used for Neuron test execution.

[**neuronparams**](internal/neuronparams/consts.go)
- Constants and variables for Neuron tests including labels, namespaces, and device IDs.

[**eco-goinfra neuron package**](https://github.com/rh-ecosystem-edge/eco-goinfra/tree/main/pkg/neuron)
- DeviceConfig CRD builder for creating, updating, and managing DeviceConfig custom resources (exported to eco-goinfra).

[**neuronhelpers**](internal/neuronhelpers/)
- `deploy.go`: Operator deployment and uninstallation helpers
- `wait.go`: Cluster stability and node readiness wait utilities
- `config.go`: NFD rule creation and DeviceConfig helpers

[**await**](internal/await/await.go)
- Async wait patterns for device plugin deployment, resource availability, and upgrade operations.

[**check**](internal/check/check.go)
- Verification utilities for node resources, pod health, and metrics.

[**neuronmetrics**](internal/neuronmetrics/metrics.go)
- ServiceMonitor verification and Prometheus query utilities.

### Environment Variables

#### Required for All Tests

| Variable | Description |
|----------|-------------|
| `ECO_HWACCEL_NEURON_DRIVERS_IMAGE` | Neuron kernel module driver image (e.g., `public.ecr.aws/q5p6u7h8/neuron-openshift/neuron-kernel-module:2.24.7.0`) |
| `ECO_HWACCEL_NEURON_DRIVER_VERSION` | Neuron driver version (e.g., `2.24.7.0`) - **REQUIRED** for DeviceConfig creation |
| `ECO_HWACCEL_NEURON_DEVICE_PLUGIN_IMAGE` | Neuron device plugin image (e.g., `public.ecr.aws/neuron/neuron-device-plugin:2.24.23.0`) |

#### Optional Configuration

| Variable | Description |
|----------|-------------|
| `ECO_HWACCEL_NEURON_SCHEDULER_IMAGE` | Custom kube-scheduler image for Neuron-aware scheduling |
| `ECO_HWACCEL_NEURON_SCHEDULER_EXTENSION_IMAGE` | Neuron scheduler extension image |
| `ECO_HWACCEL_NEURON_CATALOG_SOURCE` | Custom catalog source for operator installation |
| `ECO_HWACCEL_NEURON_CATALOG_SOURCE_NAMESPACE` | Namespace for catalog source (default: `openshift-marketplace`) |
| `ECO_HWACCEL_NEURON_SUBSCRIPTION_NAME` | Name of operator subscription (default: `aws-neuron-operator`) |
| `ECO_HWACCEL_NEURON_IMAGE_REPO_SECRET` | Secret name for pulling private images |
| `ECO_HWACCEL_NEURON_INSTANCE_TYPE` | AWS instance type for scaling tests (e.g., `inf2.xlarge`) |

#### vLLM Test Variables

The vLLM tests follow the [Red Hat Developers blog](https://developers.redhat.com/articles/2025/12/02/cost-effective-ai-workloads-openshift-aws-neuron-operator) for running LLM inference on AWS Neuron.

| Variable | Description |
|----------|-------------|
| `ECO_HWACCEL_NEURON_VLLM_IMAGE` | vLLM container image with Neuron support. Default: `public.ecr.aws/neuron/pytorch-inference-vllm-neuronx:0.7.2-neuronx-py310-sdk2.24.1-ubuntu22.04` |
| `ECO_HWACCEL_NEURON_MODEL_NAME` | Model to load for inference (default: `meta-llama/Llama-3.1-8B-Instruct`). Must use a Neuron-supported architecture: LlamaForCausalLM, MistralForCausalLM, Qwen2ForCausalLM, etc. |
| `ECO_HWACCEL_NEURON_HF_TOKEN` | **REQUIRED for vLLM tests** - HuggingFace token for downloading gated models (e.g., Llama). Get your token from https://huggingface.co/settings/tokens |
| `ECO_HWACCEL_NEURON_STORAGE_CLASS` | Storage class for model PVC (default: `gp3-csi`). The PVC caches downloaded models to avoid re-downloading on pod restart. |

#### Upgrade Test Variables

| Variable | Description |
|----------|-------------|
| `ECO_HWACCEL_NEURON_UPGRADE_TARGET_VERSION` | Target driver version for upgrade tests |
| `ECO_HWACCEL_NEURON_UPGRADE_TARGET_DRIVERS_IMAGE` | Target drivers image for upgrade tests |

#### General Test Framework Variables

| Variable | Description |
|----------|-------------|
| `ECO_TEST_LABELS` | Ginkgo query for test case selection |
| `ECO_VERBOSE_SCRIPT` | Print verbose script information |
| `ECO_TEST_VERBOSE` | Execute ginkgo with verbose output |
| `ECO_TEST_TRACE` | Include full stack trace on failures |
| `ECO_TEST_FEATURES` | List of features to test (should include `neuron`) |

### Running AWS Neuron Test Suites

#### Running vLLM Inference Tests

```bash
$ export KUBECONFIG=/path/to/kubeconfig
$ export ECO_DUMP_FAILED_TESTS=true
$ export ECO_REPORTS_DUMP_DIR=/tmp/eco-gotests-logs-dir
$ export ECO_TEST_FEATURES="neuron"
$ export ECO_TEST_LABELS='neuron,vllm'
$ export ECO_VERBOSE_LEVEL=100
$ export ECO_HWACCEL_NEURON_DRIVERS_IMAGE="public.ecr.aws/q5p6u7h8/neuron-openshift/neuron-kernel-module:2.24.7.0"
$ export ECO_HWACCEL_NEURON_DRIVER_VERSION="2.24.7.0"
$ export ECO_HWACCEL_NEURON_DEVICE_PLUGIN_IMAGE="public.ecr.aws/neuron/neuron-device-plugin:2.24.23.0"
$ export ECO_HWACCEL_NEURON_SCHEDULER_IMAGE="public.ecr.aws/eks-distro/kubernetes/kube-scheduler:v1.32.9-eks-1-32-24"
$ export ECO_HWACCEL_NEURON_SCHEDULER_EXTENSION_IMAGE="public.ecr.aws/neuron/neuron-scheduler:2.24.23.0"
$ export ECO_HWACCEL_NEURON_VLLM_IMAGE="public.ecr.aws/neuron/pytorch-inference-vllm-neuronx:0.7.2-neuronx-py310-sdk2.24.1-ubuntu22.04"
$ export ECO_HWACCEL_NEURON_MODEL_NAME="TinyLlama/TinyLlama-1.1B-Chat-v1.0"
$ make run-tests
```

#### Running Metrics Tests

```bash
$ export KUBECONFIG=/path/to/kubeconfig
$ export ECO_TEST_FEATURES="neuron"
$ export ECO_TEST_LABELS='neuron,metrics'
$ export ECO_HWACCEL_NEURON_DRIVERS_IMAGE="public.ecr.aws/q5p6u7h8/neuron-openshift/neuron-kernel-module:2.24.7.0"
$ export ECO_HWACCEL_NEURON_DRIVER_VERSION="2.24.7.0"
$ export ECO_HWACCEL_NEURON_DEVICE_PLUGIN_IMAGE="public.ecr.aws/neuron/neuron-device-plugin:2.24.23.0"
$ make run-tests
```

#### Running Upgrade Tests

The rolling upgrade is triggered by updating the `driverVersion` field in the DeviceConfig. You must set both the initial version and the target version.

```bash
$ export KUBECONFIG=/path/to/kubeconfig
$ export ECO_TEST_FEATURES="neuron"
$ export ECO_TEST_LABELS='neuron,upgrade'
# Initial driver configuration
$ export ECO_HWACCEL_NEURON_DRIVERS_IMAGE="public.ecr.aws/q5p6u7h8/neuron-openshift/neuron-kernel-module:2.24.7.0"
$ export ECO_HWACCEL_NEURON_DRIVER_VERSION="2.24.7.0"
$ export ECO_HWACCEL_NEURON_DEVICE_PLUGIN_IMAGE="public.ecr.aws/neuron/neuron-device-plugin:2.24.23.0"
# Upgrade target configuration
$ export ECO_HWACCEL_NEURON_UPGRADE_TARGET_VERSION="2.25.0.0"
$ export ECO_HWACCEL_NEURON_UPGRADE_TARGET_DRIVERS_IMAGE="public.ecr.aws/q5p6u7h8/neuron-openshift/neuron-kernel-module:2.25.0.0"
$ make run-tests
```


### DeviceConfig Example

**Important**: The `driverVersion` field is **required** when creating a DeviceConfig. A rolling upgrade is triggered when the `driverVersion` field is updated to a different value.

```yaml
apiVersion: k8s.aws/v1alpha1
kind: DeviceConfig
metadata:
  name: neuron
  namespace: ai-operator-on-aws
spec:
  driversImage: public.ecr.aws/q5p6u7h8/neuron-openshift/neuron-kernel-module:2.24.7.0
  driverVersion: 2.24.7.0
  devicePluginImage: public.ecr.aws/neuron/neuron-device-plugin:2.24.23.0
  customSchedulerImage: public.ecr.aws/eks-distro/kubernetes/kube-scheduler:v1.32.9-eks-1-32-24
  schedulerExtensionImage: public.ecr.aws/neuron/neuron-scheduler:2.24.23.0
  selector:
    feature.node.kubernetes.io/aws-neuron: "true"
```

### Supported Neuron Devices

| Device ID | Chip Type |
|-----------|-----------|
| 7064-7067 | Inferentia1 |
| 7164 | Inferentia2 |
| 7264 | Trainium1 |
| 7364 | Trainium2 |

### vLLM Image for Neuron

AWS provides a pre-built vLLM image with Neuron support in the public ECR registry:

```bash
# Use the official AWS Neuron vLLM image (recommended)
export ECO_HWACCEL_NEURON_VLLM_IMAGE="public.ecr.aws/neuron/pytorch-inference-vllm-neuronx:0.7.2-neuronx-py310-sdk2.24.1-ubuntu22.04"
```

See the [Red Hat Developers blog](https://developers.redhat.com/articles/2025/12/02/cost-effective-ai-workloads-openshift-aws-neuron-operator) for more details.

#### Building Your Own vLLM Image (Optional)

If you need a custom image, follow the [vLLM Neuron Installation Guide](https://docs.vllm.ai/en/latest/getting_started/neuron-installation.html):

```dockerfile
FROM public.ecr.aws/neuron/pytorch-training-neuronx:2.1.2-neuronx-py310-sdk2.18.2-ubuntu20.04

# Install vLLM with Neuron support
RUN pip install vllm[neuronx]

# Set entrypoint
ENTRYPOINT ["python", "-m", "vllm.entrypoints.openai.api_server"]
```

**Note**: If using a private registry, set `ECO_HWACCEL_NEURON_IMAGE_REPO_SECRET` to the name of your image pull secret.


