# O-Cloud

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `ECO_OCLOUD_IBI_GENERATE_SEED_IMAGE` | `true` | Generate the seed image for Image Based Install |
| `ECO_OCLOUD_IBI_BASE_IMAGE_PATH` | _(empty)_ | Local path to the IBI base image |
| `ECO_OCLOUD_IBI_BASE_IMAGE_URL` | _(empty)_ | URL to the IBI base image |
| `ECO_OCLOUD_VIRTUAL_MEDIA_ID` | _(empty)_ | Virtual media ID for BMC boot |
| `ECO_OCLOUD_LOCAL_REGISTRY_AUTH` | _(empty)_ | Authentication credentials for the local registry |
| `ECO_OCLOUD_SEED_IMAGE` | _(empty)_ | Seed container image reference |
| `ECO_OCLOUD_SEED_VERSION` | _(empty)_ | Version of the seed image |
| `ECO_OCLOUD_REGISTRY_5000` | _(empty)_ | URL for registry on port 5000 |
| `ECO_OCLOUD_REGISTRY_5005` | _(empty)_ | URL for registry on port 5005 |
| `ECO_OCLOUD_SSH_KEY` | _(empty)_ | SSH public key for cluster node access |
| `ECO_OCLOUD_PULL_SECRET` | _(empty)_ | Pull secret for container image registries |
| `ECO_OCLOUD_BASE_IMAGE_NAME` | _(empty)_ | Name of the base image |
| `ECO_OCLOUD_INTERFACE_NAME` | _(empty)_ | Network interface name on the spoke nodes |
| `ECO_OCLOUD_INTERFACE_IPV6_1` | _(empty)_ | IPv6 address of the interface for the first cluster |
| `ECO_OCLOUD_INTERFACE_IPV6_2` | _(empty)_ | IPv6 address of the interface for the second cluster |
| `ECO_OCLOUD_DNS_IPV6` | _(empty)_ | IPv6 address of the DNS server |
| `ECO_OCLOUD_NEXT_HOP_IPV6` | _(empty)_ | IPv6 address of the next hop gateway |
| `ECO_OCLOUD_NEXT_HOP_INTERFACE` | _(empty)_ | Network interface for the next hop route |
| `ECO_OCLOUD_SPOKE1_BMC_USERNAME` | _(empty)_ | BMC username for spoke 1 |
| `ECO_OCLOUD_SPOKE1_BMC_PASSWORD` | _(empty)_ | BMC password for spoke 1 |
| `ECO_OCLOUD_SPOKE1_BMC_HOST` | _(empty)_ | BMC IP address for spoke 1 |
| `ECO_OCLOUD_SPOKE1_BMC_TIMEOUT` | `15s` | BMC operation timeout for spoke 1 |
| `ECO_OCLOUD_SPOKE2_BMC_USERNAME` | _(empty)_ | BMC username for spoke 2 |
| `ECO_OCLOUD_SPOKE2_BMC_PASSWORD` | _(empty)_ | BMC password for spoke 2 |
| `ECO_OCLOUD_SPOKE2_BMC_HOST` | _(empty)_ | BMC IP address for spoke 2 |
| `ECO_OCLOUD_SPOKE2_BMC_TIMEOUT` | `15s` | BMC operation timeout for spoke 2 |
| `ECO_OCLOUD_INVENTORY_POOL_NAMESPACE` | _(empty)_ | Namespace of the inventory pool |
| `ECO_OCLOUD_BMH_SPOKE1` | _(empty)_ | BareMetalHost resource name for spoke 1 |
| `ECO_OCLOUD_BMH_SPOKE2` | _(empty)_ | BareMetalHost resource name for spoke 2 |
| `ECO_OCLOUD_TEMPLATE_NAME` | _(empty)_ | Base name of the referenced ClusterTemplate |
| `ECO_OCLOUD_TEMPLATE_VERSION_AI_SUCCESS` | _(empty)_ | ClusterTemplate version for successful AI-based SNO provisioning |
| `ECO_OCLOUD_TEMPLATE_VERSION_AI_FAILURE` | _(empty)_ | ClusterTemplate version for failing AI-based SNO provisioning |
| `ECO_OCLOUD_TEMPLATE_VERSION_SIMULTANEOUS_1` | _(empty)_ | First ClusterTemplate version for multi-cluster provisioning |
| `ECO_OCLOUD_TEMPLATE_VERSION_SIMULTANEOUS_2` | _(empty)_ | Second ClusterTemplate version for multi-cluster provisioning |
| `ECO_OCLOUD_TEMPLATE_VERSION_IBI_SUCCESS` | _(empty)_ | ClusterTemplate version for successful IBI-based SNO provisioning |
| `ECO_OCLOUD_TEMPLATE_VERSION_IBI_FAILURE` | _(empty)_ | ClusterTemplate version for failing IBI-based SNO provisioning |
| `ECO_OCLOUD_TEMPLATE_VERSION_DAY2` | _(empty)_ | ClusterTemplate version for Day 2 operations |
| `ECO_OCLOUD_TEMPLATE_VERSION_SEED` | _(empty)_ | ClusterTemplate version for IBI seed cluster provisioning |
| `ECO_OCLOUD_NODE_CLUSTER_NAME_1` | _(empty)_ | Name of the first ORAN Node Cluster |
| `ECO_OCLOUD_NODE_CLUSTER_NAME_2` | _(empty)_ | Name of the second ORAN Node Cluster |
| `ECO_OCLOUD_OCLOUD_SITE_ID` | _(empty)_ | ID of the ORAN O-Cloud Site |
| `ECO_OCLOUD_CLUSTER_NAME_1` | _(empty)_ | Name of the first cluster |
| `ECO_OCLOUD_CLUSTER_NAME_2` | _(empty)_ | Name of the second cluster |
| `ECO_OCLOUD_SSH_CLUSTER_2` | _(empty)_ | SSH address for the second cluster |
| `ECO_OCLOUD_HOSTNAME_1` | _(empty)_ | Hostname of the first cluster node |
| `ECO_OCLOUD_HOSTNAME_2` | _(empty)_ | Hostname of the second cluster node |
| `ECO_OCLOUD_AUTHFILE_PATH` | `/kubeconfig/docker/config.json` | Path to the auth file for Skopeo commands |
| `ECO_OCLOUD_SUBSCRIBER_URL` | _(empty)_ | URL of the O-Cloud event subscriber |
| `ECO_OCLOUD_SUBSCRIBER_DOMAIN` | _(empty)_ | Domain of the O-Cloud event subscriber |
| `ECO_OCLOUD_O2IMS_BASE_URL` | _(empty)_ | Base URL for the O2IMS API |
