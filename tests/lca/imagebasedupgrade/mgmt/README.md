# LCA Image-Based Upgrade - Management

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `ECO_LCA_IBU_MGMT_SEED_IMAGE` | `quay.io/ocp-edge-qe/ib-seedimage-public:ci` | Seed image for image-based upgrade |
| `ECO_LCA_IBU_MGMT_WORKLOAD_IMAGE` | `registry.redhat.io/openshift4/ose-hello-openshift-rhel8@sha256:10dca31348f07e1bfb56ee93c324525cceefe27cb7076b23e42ac181e4d1863e` | Container image for IBU workload validation |
| `ECO_LCA_IBU_MGMT_IDLE_POST_UPGRADE` | `false` | Set IBU to Idle after the upgrade completes |
| `ECO_LCA_IBU_MGMT_ROLLBACK_AFTER_UPGRADE` | `false` | Perform rollback after upgrade completes |
| `ECO_LCA_IBU_MGMT_EXTRA_MANIFESTS` | `true` | Include extra manifests during upgrade |
| `ECO_LCA_IBU_MGMT_ADDITIONAL_NTP_SOURCES` | _(empty)_ | Additional NTP sources for the cluster |
| `ECO_LCA_IBU_MGMT_STATE_TRANSITIONS` | `false` | Enable state transition testing before upgrade |
| `ECO_LCA_IBU_MGMT_SECOND_UPGRADE` | `false` | Perform a second upgrade after the first completes |
