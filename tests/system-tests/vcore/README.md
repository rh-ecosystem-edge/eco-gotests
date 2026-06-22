# vCore

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `ECO_SYSTEM_VCORE_NS` | `vcore-test` | Default namespace for vCore tests |
| `ECO_SYSTEM_VCORE_ODF_MCP` | `odf` | MachineConfigPool name for ODF nodes |
| `ECO_SYSTEM_VCORE_PP_MCP` | `user-plane-worker` | MachineConfigPool name for user plane worker nodes |
| `ECO_SYSTEM_VCORE_CP_MCP` | `control-plane-worker` | MachineConfigPool name for control plane worker nodes |
| `ECO_SYSTEM_VCORE_HOST` | _(empty)_ | Hostname or IP of the target host |
| `ECO_SYSTEM_VCORE_USER` | `kni` | SSH username for the target host |
| `ECO_SYSTEM_VCORE_PASS` | _(empty)_ | SSH password for the target host |
| `ECO_SYSTEM_VCORE_MIRROR_REGISTRY_USER` | `ocp-edge` | Username for the mirror registry |
| `ECO_SYSTEM_VCORE_MIRROR_REGISTRY_PASSWORD` | `ocp-edge-pass` | Password for the mirror registry |
| `ECO_SYSTEM_VCORE_COMBINED_PULL_SECRET` | `combined-secret.json` | Filename for the combined pull secret |
| `ECO_SYSTEM_VCORE_PRIVATE_KEY` | `.ssh/id_rsa` | Path to the SSH private key |
| `ECO_SYSTEM_VCORE_REGISTRY_REPOSITORY` | `openshift` | Repository name in the mirror registry |
| `ECO_SYSTEM_VCORE_CPU_ISOLATED` | `2-27,30-55` | CPU cores reserved for isolated workloads |
| `ECO_SYSTEM_VCORE_CPU_RESERVED` | `0-1,28-29` | CPU cores reserved for system use |
| `ECO_SYSTEM_VCORE_KUBECONFIG` | `/home/kni/clusterconfigs/auth/kubeconfig` | Path to the kubeconfig file |
