# IBI Preinstall Tests

This test suite automates the Image-Based Installation (IBI) preinstall process, which includes:
1. Generating an IBI ISO from a seed image and site configuration
2. Provisioning a bare metal host using the generated ISO
3. Waiting for the preinstall process to complete

## Required Environment Variables

### Mandatory Variables

- `ECO_LCA_IBI_SEED_IMAGE` - Seed image reference (e.g., `registry.example.com:5000/ibu/seed:4.16.7`)
- `ECO_LCA_IBI_SITECONFIG_REPO` - Git repository URL containing ZTP site configurations
- `ECO_LCA_IBI_SITECONFIG_BRANCH` - Branch name to use from the site config repository
- `ECO_LCA_IBI_RELEASE_IMAGE` - OpenShift release image to extract `openshift-install` from
- `ECO_LCA_IBI_PROVISIONING_HOST` - Hostname/IP of the provisioning host
- `ECO_LCA_IBI_BMC_USERNAME` - BMC username for the bare metal host
- `ECO_LCA_IBI_BMC_PASSWORD` - BMC password for the bare metal host

### Optional Variables

- `ECO_LCA_IBI_PROVISIONING_USER` - SSH user for provisioning host (defaults to `kni`)
- `ECO_LCA_IBI_PROVISIONING_SSH_KEY` - Path to SSH private key for provisioning host access

## Hub Cluster Resources

The following values are automatically fetched from the hub cluster:

1. **Pull Secret** - Retrieved from `openshift-config/pull-secret` secret
2. **SSH Public Key** - Retrieved from `99-master-ssh` MachineConfig
3. **CA Certificate Bundle** - Retrieved from `openshift-config/user-ca-bundle` configmap

## Site Configuration

The following values are parsed from the ClusterInstance CR in the site config:

1. **Node Hostname** - From `spec.nodes[0].hostName`
2. **BMC Address** - From `spec.nodes[0].bmcAddress`
3. **Boot MAC Address** - From `spec.nodes[0].bootMACAddress`
4. **Network Configuration** - From `spec.nodes[0].nodeNetwork.config`
5. **Installation Disk** - From `spec.nodes[0].rootDeviceHints` (converted to device path)

## Running the Tests

```bash
export ECO_LCA_IBI_SEED_IMAGE="registry.example.com:5000/ibu/seed:4.16.7"
export ECO_LCA_IBI_SITECONFIG_REPO="https://gitlab.example.com/ztp-site-configs.git"
export ECO_LCA_IBI_SITECONFIG_BRANCH="main"
export ECO_LCA_IBI_RELEASE_IMAGE="quay.io/openshift-release-dev/ocp-release:4.16.7-x86_64"
export ECO_LCA_IBI_PROVISIONING_HOST="provisioning.example.com"
export ECO_LCA_IBI_BMC_USERNAME="admin"
export ECO_LCA_IBI_BMC_PASSWORD="password"

ginkgo -v --label-filter="ibi-preinstall" ./tests/lca/imagebasedinstall/cnf/ran/preinstall/tests/
```

## Test Flow

1. **BeforeAll**: Extract `openshift-install` binary from release image
2. **Test 1**: Generate IBI ISO
   - Clone ZTP site config repository
   - Run kustomize to get ClusterInstance
   - Parse node configuration from ClusterInstance
   - Fetch hub cluster resources (pull secret, SSH key, CA cert)
   - Generate image-based-installation-config.yaml
   - Create IBI ISO using `openshift-install image-based create image`
3. **Test 2**: Provision bare metal host
   - Copy ISO to provisioning host HTTP server
   - Create BMC secret on hub cluster
   - Create BareMetalHost CR pointing to the ISO
   - Wait for preinstall to complete (monitor journalctl on provisioned node)
   - Clean up BareMetalHost
4. **AfterAll**: Clean up working directory

## Notes

- The BMC credentials must be provided via environment variables as they are specific to your lab environment
- The hub cluster must have the required resources (pull-secret, user-ca-bundle, 99-master-ssh MachineConfig)
- The provisioning host must have an HTTP server running on port 8080 serving from `/opt/cached_disconnected_images`
