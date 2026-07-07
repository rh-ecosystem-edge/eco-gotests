# IBI Preinstall Tests

This test suite automates the Image-Based Installation (IBI) preinstall process, which includes:

1. Generating an IBI ISO from a seed image and site configuration
2. Provisioning a bare-metal host using the generated ISO
3. Waiting for the preinstall process to complete

Layout matches other LCA suites (for example IBU CNF `upgrade-talm` and IBI mgmt `deploy`):

- `../internal/ranparams`, `../internal/ranconfig`, `../internal/raninittools` — shared CNF ran configuration and hub client
- `internal/tsparams` — suite labels and failure reporter namespaces / CRDs
- `internal/helpers` — suite-specific utilities:
  - ZTP clone (go-git) and kustomize
  - Typed ClusterInstance (`siteconfig.open-cluster-management.io/v1alpha1`) parsing
  - Hub resource retrieval (pull secret, CA, SSH)
  - Image digest mirrors (IDMS / ICSP / mirror-registry-ca)
  - Installer API types (`github.com/openshift/installer/pkg/types/imagebased.InstallationConfig`) for `image-based-installation-config.yaml`
  - BMH + BMC secret (live-ISO), SSH/SCP
  - `oc adm release extract` for `openshift-install` + `oc` (hub pull secret as `--registry-config`, hub kubeconfig as `--kubeconfig`)
- `tests/` — Ginkgo specs (blank-imported from `preinstall_suite_test.go`)

Defaults live in `cnf/ran/internal/ranconfig/default.yaml`; environment variables override them (`ECO_LCA_IBI_*` via `envconfig`).

## Path scope (test container vs provisioning host)

The suite runs inside the eco-gotests container. Some paths are local to that container; others are on the **provisioning host**, reached via SSH/SCP from the container.

| Variable | Scope | Description |
| --- | --- | --- |
| `ECO_LCA_IBI_CNF_RAN_HUB_KUBECONFIG` | **Container** | Path to the mounted hub kubeconfig inside the container |
| `ECO_LCA_IBI_BOOTSTRAP_OC` | **Container** | Path to `oc` used for `oc adm release extract` (or `oc` on `PATH`) |
| `ECO_LCA_IBI_PROVISIONING_SSH_DIR` | **Container** | Directory containing the SSH private key **inside the container** (typically bind-mounted read-only from the lab host) |
| `ECO_LCA_IBI_PROVISIONING_SSH_KEY` | **Container** | Explicit private key path **inside the container** (overrides key discovery under `ECO_LCA_IBI_PROVISIONING_SSH_DIR`) |
| `ECO_LCA_IBI_REMOTE_ISO_PATH` | **Provisioning host** | Destination path for the ISO copied via SCP (e.g. `/opt/cached_disconnected_images/rhcos-ibi.iso`) |
| `ECO_LCA_IBI_ISO_HTTP_BASE_URL` | **Provisioning host** | HTTP base URL where the provisioning host serves the live ISO (no trailing slash) |
| `ECO_LCA_IBI_PROVISIONING_HOST` | **Provisioning host** | Hostname or IP of the provisioning host (SSH/SCP target) |
| `ECO_LCA_IBI_PROVISIONING_USER` | **Provisioning host** | SSH user on the provisioning host |
| `ECO_LCA_IBI_PREINSTALL_NODE_SSH_USER` | **Spoke node** | SSH user on the provisioned node for journal checks (typically `core`) |

## Required Environment Variables

### Mandatory Variables

- `ECO_LCA_IBI_CNF_RAN_HUB_KUBECONFIG` — Path to kubeconfig for the hub cluster (pull secret, BMC secrets, BareMetalHost CRs) **inside the container**
- `ECO_LCA_IBI_SEED_IMAGE` — Seed image reference (e.g., `registry.example.com:5000/ibu/seed:4.16.7`)
- `ECO_LCA_IBI_SITECONFIG_REPO` — Git repository URL containing ZTP site configurations
- `ECO_LCA_IBI_SITECONFIG_BRANCH` — Branch name to use from the site config repository
- `ECO_LCA_IBI_RELEASE_IMAGE` — OpenShift release image used to extract `openshift-install` and release-matched `oc` (via bootstrap `oc`; see optional variables)
- `ECO_LCA_IBI_PROVISIONING_HOST` — Hostname/IP of the provisioning host
- `ECO_LCA_IBI_BMC_USERNAME` / `ECO_LCA_IBI_BMC_PASSWORD` — BMC credentials as **plain text** (required; not fetched over HTTP; Kubernetes base64-encodes Secret data on persist)
- `ECO_LCA_IBI_ISO_HTTP_BASE_URL` — Base URL for the live ISO on the **provisioning host** (no trailing slash), e.g. `http://192.168.1.5:8080/images` (override `default.yaml` if empty there)

### Optional Variables

Values below can be set in `default.yaml` or overridden via env:

- `ECO_LCA_IBI_PROVISIONING_USER` — SSH user for provisioning host
- `ECO_LCA_IBI_PROVISIONING_SSH_DIR` — Directory containing the provisioning SSH private key **inside the container**
- `ECO_LCA_IBI_PROVISIONING_SSH_KEY` — Explicit path to the private key **inside the container** (overrides `id_rsa` / `id_ed25519` selection under `ECO_LCA_IBI_PROVISIONING_SSH_DIR`)
- `ECO_LCA_IBI_BOOTSTRAP_OC` — Path to host `oc` for `oc adm release extract` **inside the container** (default in `default.yaml` is `/home/kni/.local/bin/oc` when that file exists; otherwise configure or rely on `oc` on `PATH`)
- `ECO_LCA_IBI_SITECONFIG_GIT_SKIP_TLS` — Set to `true` to skip TLS verification when cloning the siteconfig repo (go-git)
- `ECO_LCA_IBI_SITECONFIG_KUSTOMIZE_PATH` — Directory under the cloned repo for `kustomize build`
- `ECO_LCA_IBI_REMOTE_ISO_PATH` — Path on the **provisioning host** for `scp`
- `ECO_LCA_IBI_PREINSTALL_NODE_SSH_USER` — SSH user on the spoke for journal checks
- `ECO_LCA_IBI_PREINSTALL_WAIT_TIMEOUT_SECONDS` — Max wait for `install-rhcos-and-restore-seed`
- `ECO_LCA_IBI_EXTRA_PARTITION_LABEL` — Optional `extraPartitionLabel` in the IBI config
- `ECO_LCA_IBI_SEED_VERSION` — Optional override for the seed version tag when `ECO_LCA_IBI_SEED_IMAGE` is digest-pinned or has no tag

## Hub Cluster Resources

The following values are automatically fetched from the hub cluster:

1. **Pull Secret** — Retrieved from `openshift-config/pull-secret` secret
2. **SSH Public Key** — Retrieved from `99-master-ssh` MachineConfig
3. **CA Certificate Bundle** — Retrieved from `openshift-config/user-ca-bundle` configmap

Use `internal/helpers` (`GetPullSecretFromHub`, `GetSSHKeyFromHub`, `GetCACertFromHub`) with `raninittools.HubAPIClient`.

## Site Configuration

The following values are read from the ClusterInstance CR in the site config (typed `ClusterInstance` from kustomize output):

1. **Node Hostname** — From `spec.nodes[0].hostName`
2. **BMC Address** — From `spec.nodes[0].bmcAddress`
3. **Boot MAC Address** — From `spec.nodes[0].bootMACAddress`
4. **Network Configuration** — From `spec.nodes[0].nodeNetwork.config` (as assisted-service `NetConfig` for the installer manifest)
5. **Installation Disk** — From `spec.nodes[0].rootDeviceHints` (converted to device path)

BMC **username/password** for creating the hub BMC secret come **only** from configuration (`ECO_LCA_IBI_BMC_USERNAME` / `ECO_LCA_IBI_BMC_PASSWORD` / `default.yaml`), not from a remote secrets file.

## Running the Tests

From the repository root:

```bash
export ECO_LCA_IBI_CNF_RAN_HUB_KUBECONFIG="/path/to/hub/kubeconfig"
export ECO_LCA_IBI_SEED_IMAGE="registry.example.com:5000/ibu/seed:4.16.7"
export ECO_LCA_IBI_SITECONFIG_REPO="https://gitlab.example.com/ztp-site-configs.git"
export ECO_LCA_IBI_SITECONFIG_BRANCH="main"
export ECO_LCA_IBI_RELEASE_IMAGE="quay.io/openshift-release-dev/ocp-release:4.16.7-x86_64"
export ECO_LCA_IBI_PROVISIONING_HOST="provisioning.example.com"
export ECO_LCA_IBI_ISO_HTTP_BASE_URL="http://provisioning.example.com:8080/images"
export ECO_LCA_IBI_BMC_USERNAME="admin"
export ECO_LCA_IBI_BMC_PASSWORD="password"

ginkgo -v --label-filter="preinstall" ./tests/lca/imagebasedinstall/cnf/ran/preinstall
```

Labels applied by the suite are `lca`, `ibi`, `ran`, and `preinstall` (see `ranparams` and `internal/tsparams`). Narrow further with compound filters as needed (for example `e2e && preinstall`).

## Test Flow

1. **BeforeAll**: Extract `openshift-install` binary from release image
2. **Test 1**: Generate IBI ISO
   - Clone ZTP site config repository (go-git)
   - Run kustomize to get ClusterInstance
   - Parse node configuration from typed ClusterInstance
   - Fetch hub cluster resources (pull secret, SSH key, CA cert)
   - Generate `image-based-installation-config.yaml` via `imagebased.InstallationConfig`
   - Create IBI ISO using `openshift-install image-based create image`
3. **Test 2**: Provision bare-metal host
   - Copy ISO to provisioning host HTTP server
   - Create BMC secret on hub cluster
   - Create BareMetalHost CR pointing to the ISO
   - Wait for preinstall to complete (monitor journalctl on provisioned node)
   - Clean up BareMetalHost
4. **AfterAll**: Clean up working directory

## Notes

- The hub cluster must have the required resources (pull-secret, user-ca-bundle, 99-master-ssh MachineConfig)
- The provisioning host must serve the live ISO at the URL implied by `ECO_LCA_IBI_ISO_HTTP_BASE_URL`
- SSH/SCP to the provisioning host and spoke node use the container's `ssh` and `scp` commands (`internal/helpers/ssh_utils.go`), not `golang.org/x/crypto/ssh`; the test image includes openssh-clients and subprocess invocation is sufficient for ISO copy and journal polling
