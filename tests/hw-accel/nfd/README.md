# NFD (Node Feature Discovery)

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `ECO_HWACCEL_NFD_SUBSCRIPTION_NAME` | _(empty)_ | Name of the NFD operator subscription |
| `ECO_HWACCEL_NFD_CR_IMAGE` | _(empty)_ | Container image for the NFD custom resource |
| `ECO_HWACCEL_NFD_CATALOG_SOURCE` | _(empty)_ | Catalog source for the NFD operator |
| `ECO_HWACCEL_NFD_CUSTOM_NFD_CATALOG_SOURCE` | _(empty)_ | Custom catalog source for the NFD operator |
| `ECO_HWACCEL_NFD_AWS_TESTS` | `false` | Enable AWS-specific NFD tests |
| `ECO_HWACCEL_NFD_UPGRADE_TARGET_VERSION` | _(empty)_ | Target version for NFD operator upgrade tests |
| `ECO_HWACCEL_NFD_CPU_FLAGS_HELPER_IMAGE` | _(empty)_ | Container image for CPU flags helper pod |
