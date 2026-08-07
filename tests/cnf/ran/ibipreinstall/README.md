# IBI CNF RAN Preinstall

End-to-end test for disconnected Image Based Install (IBI) preinstall workflow.

## Overview

This suite orchestrates the full preinstall lifecycle:
1. Validates all data sources (env vars, hub API, remote YAML configs)
2. Resolves a Go `text/template` from the config repo into `image-based-installation-config.yaml`
3. Generates the IBI live installation ISO via pre-staged `openshift-install`
4. Copies ISO to the local HTTP serving directory
5. Creates BareMetalHost on the preinstall hub to boot the target
6. Waits for the `install-rhcos-and-restore-seed` service to complete on the spoke

## Template-Based Config Generation

The `image-based-installation-config.yaml` is generated from a Go `text/template` file stored in
the config repo (e.g., `siteconfig/helix54/preinstall-test/image-based-installation-config.yaml`).

Placeholders like `{{.SeedImage}}`, `{{.PullSecret}}`, `{{.NetworkConfig}}`, etc. are resolved
at runtime from hub cluster resources and environment variables. This keeps environment-specific
values (disk paths, ignition overrides) in the config repo while secrets/mirrors are fetched live.

## Environment Variables

### Shared with other cnf/ran suites

| Variable | Purpose |
|----------|---------|
| `ECO_CNF_RAN_KUBECONFIG_HUB` | Path to hub kubeconfig |
| `ECO_CNF_RAN_BMC_USERNAME` | BMC username |
| `ECO_CNF_RAN_BMC_PASSWORD` | BMC password |
| `ECO_CNF_RAN_SKIP_TLS_VERIFY` | Skip TLS verification for HTTPS fetches (default: false) |

### IBI preinstall specific

| Variable | Required | Purpose |
|----------|----------|---------|
| `ECO_CNF_RAN_IBI_SEED_IMAGE` | Yes | Seed image reference |
| `ECO_CNF_RAN_IBI_SEED_VERSION` | No | Explicit seed version override (needed if digest-pinned) |
| `ECO_CNF_RAN_IBI_CLUSTER_INSTANCE_URL` | Yes | Raw URL to ClusterInstance YAML |
| `ECO_CNF_RAN_IBI_CONFIG_TEMPLATE_URL` | No | Override URL for IBI config template (auto-derived from ClusterInstance URL) |
| `ECO_CNF_RAN_IBI_PREINSTALL_REGISTRY` | No | Disconnected registry host:port (auto-derived from MCE mirror) |
| `ECO_CNF_RAN_IBI_OPENSHIFT_INSTALL` | Yes | Path to `openshift-install` binary |
| `ECO_CNF_RAN_IBI_PREINSTALL_HTTP_DIR` | Yes | Local directory served by the HTTP server |
| `ECO_CNF_RAN_IBI_PREINSTALL_HTTP_BASE_URL` | Yes | HTTP URL mapping to the directory above |
| `ECO_CNF_RAN_IBI_PREINSTALL_SSH_KEY` | Yes | Path to SSH private key (for spoke access) |

## Constraints

- Runs must be serialized per `PreinstallHTTPDir`. The published ISO uses a
  fixed filename (`rhcos-ibi.iso`), so concurrent runs sharing the same HTTP
  directory would overwrite each other.

## Internal Hardcodes

- SSH user for target node: `core`
- Preinstall wait timeout: 60 minutes
- ISO filename: `rhcos-ibi.iso`

## Running

```bash
ginkgo --label-filter="preinstall-e2e" ./tests/cnf/ran/ibipreinstall/...
```
