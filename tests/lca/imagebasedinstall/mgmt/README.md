# LCA Image Based Install - Management

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `ECO_LCA_IBI_MGMT_CLUSTER_INFO` | _(empty)_ | Path to YAML file with spoke cluster network and BMC info |
| `ECO_LCA_IBI_MGMT_SEED_IMAGE` | `quay.io/ocp-edge-qe/ib-seedimage-public:ci` | Seed image for image-based installation |
| `ECO_LCA_IBI_MGMT_SSHKEY_PATH` | _(empty)_ | Path to public SSH key file for spoke cluster access |
| `ECO_LCA_IBI_MGMT_STATIC_NETWORK` | `false` | Enable static networking for spoke cluster |
| `ECO_LCA_IBI_EXTRA_MANIFESTS` | `true` | Include extra manifests during installation |
| `ECO_LCA_IBI_CA_BUNDLE` | `true` | Include CA bundle during installation |
| `ECO_LCA_IBI_SITECONFIG` | `true` | Use SiteConfig for installation |
| `ECO_LCA_IBI_MGMT_EXTRA_PARTITION_NAME` | _(empty)_ | Name of extra disk partition to create |
| `ECO_LCA_IBI_MGMT_EXTRA_PARTITION_SIZE` | `50000` | Size of extra disk partition in MiB |
| `ECO_LCA_IBI_REINSTALL_GENERATION` | `generate1` | Reinstall generation label for reinstall tests |
| `ECO_LCA_IBI_ADDITIONAL_NTP_SOURCES` | _(empty)_ | Additional NTP sources for the cluster |
