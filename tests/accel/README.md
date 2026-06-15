# Accel

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `ECO_ACCEL_PULL_SECRET` | _(empty)_ | Pull secret for accessing container registries |
| `ECO_ACCEL_REGISTRY` | _(empty)_ | Container registry URL |
| `ECO_ACCEL_UPGRADE_TARGET_IMAGE` | _(empty)_ | Target image for OCP upgrade |
| `ECO_ACCEL_SPOKE_KUBECONFIG` | _(empty)_ | Path to spoke cluster kubeconfig |
| `ECO_ACCEL_HUB_CLUSTER_NAME` | _(empty)_ | Name of the hub cluster |
| `ECO_ACCEL_HUB_MINOR_VERSION` | _(empty)_ | Minor version of the hub cluster |
| `ECO_ACCEL_WORKLOAD_IMAGE` | `registry.redhat.io/openshift4/ose-hello-openshift-rhel8@sha256:10dca31348f07e1bfb56ee93c324525cceefe27cb7076b23e42ac181e4d1863e` | Container image for IBU workload validation |
