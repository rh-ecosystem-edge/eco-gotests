# Test Configuration

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `ECO_REPORTS_DUMP_DIR` | `/tmp/reports` | Directory path for test report output |
| `ECO_VERBOSE_LEVEL` | `0` | Logging verbosity level |
| `ECO_DUMP_FAILED_TESTS` | `false` | Dump logs for failed tests to the reports directory |
| `ECO_ENABLE_REPORT` | `true` | Enable XML test report generation |
| `ECO_DRY_RUN` | `false` | Run tests in dry-run mode without making changes |
| `ECO_SSH_KEY_PATH` | _(empty)_ | Path to SSH private key |
| `ECO_SSH_USER` | `core` | SSH username for node access |
| `ECO_KUBERNETES_ROLE_PREFIX` | `node-role.kubernetes.io` | Prefix for Kubernetes node role labels |
| `ECO_WORKER_LABEL` | `worker` | Worker node role label suffix |
| `ECO_CONTROL_PLANE_LABEL` | `control-plane` | Control plane node role label suffix |
| `ECO_TC_PREFIX` | `TC-` | Prefix for test case identifiers |
| `ECO_MCO_NAMESPACE` | `openshift-machine-config-operator` | Namespace for the Machine Config Operator |
| `ECO_LOGGING_OPERATOR_NAMESPACE` | `openshift-logging` | Namespace for the Logging operator |
| `ECO_MCO_CONFIG_DAEMON_NAME` | `machine-config-daemon` | Name of the Machine Config Daemon DaemonSet |
| `ECO_SRIOV_OPERATOR_NAMESPACE` | `openshift-sriov-network-operator` | Namespace for the SR-IOV Network Operator |
| `ECO_NMSTATE_OPERATOR_NAMESPACE` | `openshift-nmstate` | Namespace for the NMState operator |
| `ECO_SRIOV_FEC_OPERATOR_NAMESPACE` | `vran-acceleration-operators` | Namespace for the SR-IOV FEC operator |
