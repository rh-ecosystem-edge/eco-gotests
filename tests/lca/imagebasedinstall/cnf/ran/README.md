# LCA Image-Based Install - CNF RAN

Configuration for IBI CNF RAN workflows (for example the `preinstall` suite). Defaults live in `internal/ranconfig/default.yaml`; environment variables override them via `envconfig`.

See [preinstall/README.md](preinstall/README.md) for suite workflow, path scope (container vs provisioning host), and running the tests.

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `ECO_LCA_IBI_CNF_RAN_HUB_KUBECONFIG` | _(empty)_ | Path to kubeconfig for the hub cluster (pull secret, BMC secrets, BareMetalHost CRs) inside the test container |
| `ECO_LCA_IBI_SEED_IMAGE` | _(empty)_ | Seed image reference (e.g. `registry.example.com:5000/ibu/seed:4.16.7`) |
| `ECO_LCA_IBI_SEED_VERSION` | _(empty)_ | Optional override for seed version when `ECO_LCA_IBI_SEED_IMAGE` is digest-pinned or has no tag |
| `ECO_LCA_IBI_SITECONFIG_REPO` | _(empty)_ | Git repository URL containing ZTP site configurations |
| `ECO_LCA_IBI_SITECONFIG_BRANCH` | _(empty)_ | Branch name to use from the siteconfig repository |
| `ECO_LCA_IBI_SITECONFIG_KUSTOMIZE_PATH` | `siteconfig` | Directory under the cloned siteconfig repo for `kustomize build` |
| `ECO_LCA_IBI_SITECONFIG_GIT_SKIP_TLS` | `false` | Skip TLS verification when cloning the siteconfig repository |
| `ECO_LCA_IBI_RELEASE_IMAGE` | _(empty)_ | OpenShift release image used to extract `openshift-install` and release-matched `oc` |
| `ECO_LCA_IBI_BOOTSTRAP_OC` | `/home/kni/.local/bin/oc` | Path to `oc` for `oc adm release extract` inside the container; falls back to `oc` on `PATH` if unset or missing |
| `ECO_LCA_IBI_PROVISIONING_HOST` | _(empty)_ | Hostname or IP of the provisioning host (SSH/SCP target) |
| `ECO_LCA_IBI_PROVISIONING_USER` | `kni` | SSH user on the provisioning host |
| `ECO_LCA_IBI_PROVISIONING_SSH_DIR` | `/home/kni/.ssh` | Directory containing the SSH private key inside the container |
| `ECO_LCA_IBI_PROVISIONING_SSH_KEY` | _(empty)_ | Explicit SSH private key path inside the container (overrides key discovery under `ECO_LCA_IBI_PROVISIONING_SSH_DIR`) |
| `ECO_LCA_IBI_REMOTE_ISO_PATH` | `/opt/cached_disconnected_images/rhcos-ibi.iso` | Destination path on the provisioning host for the IBI ISO copied via SCP |
| `ECO_LCA_IBI_ISO_HTTP_BASE_URL` | _(empty)_ | HTTP base URL on the provisioning host for the live ISO (no trailing slash) |
| `ECO_LCA_IBI_PREINSTALL_NODE_SSH_USER` | `core` | SSH user on the provisioned spoke node for journal checks |
| `ECO_LCA_IBI_PREINSTALL_WAIT_TIMEOUT_SECONDS` | `3600` | Max wait in seconds for `install-rhcos-and-restore-seed` to complete |
| `ECO_LCA_IBI_BMC_USERNAME` | _(empty)_ | BMC username for the hub BMC secret (plain text; env-only) |
| `ECO_LCA_IBI_BMC_PASSWORD` | _(empty)_ | BMC password for the hub BMC secret (plain text; env-only) |
| `ECO_LCA_IBI_EXTRA_PARTITION_LABEL` | _(empty)_ | Optional `extraPartitionLabel` in the image-based installation config |
