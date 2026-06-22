# RAN DU System Tests

## Overview

System-level test automation for RAN Distributed Unit (DU) workloads on OpenShift. Tests validate workload lifecycle, node reboot recovery, kernel crash handling, PTP synchronization, ZTP policy compliance, cert-manager certificate management, and long-running stability scenarios against a live OCP cluster.

### Prerequisites

* OCP cluster >=4.13 with RAN DU profile applied
* `KUBECONFIG` set to a valid kubeconfig for the target cluster
* Workload deployment scripts available on the test runner (for workload-based tests)

#### Optional

* PTP operator deployed (for PTP validation tests)
* SR-IOV operator deployed (for SR-IOV device verification after reboots)
* SR-IOV FEC operator deployed (for SriovFecNodeConfig tests)
* cert-manager operator deployed (for certificate management tests)
* BMC/Redfish access configured (for NMI kernel crash tests)

### Test suites

| File | Label | ID(s) | Description |
|------|-------|-------|-------------|
| [launch-workload.go](tests/launch-workload.go) | `launch-workload` | 55465 | Deploy workload, verify all pods are ready, validate PTP |
| [launch-workload-multiple-iter-loadavg.go](tests/launch-workload-multiple-iter-loadavg.go) | `LaunchWorkloadMultipleIterations` | 45698 | Repeat workload deploy/delete cycles, monitor node load average < 100 |
| [soft-reboot.go](tests/soft-reboot.go) | `SoftReboot` | 42738 | Soft reboot all nodes, validate workload, SR-IOV, and PTP recovery |
| [hard-reboot.go](tests/hard-reboot.go) | `HardReboot` | 42736 | Hard reboot worker nodes via ipmitool, validate recovery |
| [kernel-crash-kdump.go](tests/kernel-crash-kdump.go) | `KernelCrashKdump` | 56216 | Trigger kernel crash via sysrq, verify vmcore dump in `/var/crash` |
| [nmi-kernel-crash-kdump.go](tests/nmi-kernel-crash-kdump.go) | `NMIKernelCrashKdump` | 85975 | Trigger NMI via Redfish (BMC), verify node recovery and vmcore generation |
| [ptp-3wpc.go](tests/ptp-3wpc.go) | `ptp-3wpc` | 99991-99997 | 7 PTP cases: GNSS lock, inter-card sync, DPLL stability, clock class, GNSS recovery, iperf3 stress, packet loss |
| [du-ztp-policies-compliance.go](tests/du-ztp-policies-compliance.go) | `ZTPPoliciesCompliance` | — | Verify all cluster policies are Compliant |
| [cert-manager.go](tests/cert-manager.go) | `cert-manager` | 89041-89048 | cert-manager operator verification, DNS-01 ACME certificate generation, alert escalation/resolution, API server and ingress certificate renewal |
| [stability-workload.go](tests/stability-workload.go) | `StabilityWorkload` | 42744 | Long-running stability with workload: PTP status, policy compliance, and pod restarts at configurable intervals |
| [stability-no-workload.go](tests/stability-no-workload.go) | `StabilityNoWorkload` | 74522 | Same as above without workload deployed |
| [sriovfecnodeconfig-status.go](tests/sriovfecnodeconfig-status.go) | `SriovFecNodeConfigStatus` | — | Poll until all SriovFecNodeConfig status conditions are True |
| [workload-guaranteed-force-delete.go](tests/workload-guaranteed-force-delete.go) | `WorkloadForceCleanup` | 74462 | Force-delete guaranteed QoS pods 3 times, verify automatic recovery |

### Internal packages

[**randuconfig**](internal/randuconfig/config.go)
- Configuration loading from `default.yaml` with environment variable overrides (`ECO_RANDU_*` prefix via `envconfig`). Includes BMC credentials parsing from semicolon-separated format.

[**randuinittools**](internal/randuinittools/randuinittools.go)
- Exports `APIClient` and `RanDuTestConfig` for use via dot-import in all test files.

[**randuparams**](internal/randuparams/const.go)
- Suite-level constants (labels, timeouts, namespace names) and reporter configuration (namespaces and CRDs to dump on failure).

[**randutestworkload**](internal/randutestworkload/randutestworkload.go)
- Workload namespace cleanup: removes Deployments, StatefulSets, the namespace, and associated SR-IOV networks.

### Shared system-tests packages

The suite depends on shared helpers from `tests/system-tests/internal/`:

| Package | Purpose |
|---------|---------|
| `shell` | `ExecuteCmd()` — run shell commands |
| `await` | `WaitUntilAllDeploymentsReady()`, `WaitUntilAllPodsReady()`, `WaitUntilAllStatefulSetsReady()` |
| `reboot` | `SoftRebootNode()`, `HardRebootNode()`, `KernelCrashKdump()` |
| `sriov` | `ListNetworksByDeviceType()`, `ExtractNetworkNames()` |
| `ptp` | `ValidatePTPStatus()` |
| `stability` | `SavePTPStatus()`, `SavePolicyStatus()`, `VerifyStabilityStatusChange()` |
| `platform` | `GetOCPClusterName()` |
| `remote` | `ExecuteOnNodeWithDebugPod()` |
| `nmi` | `TriggerNMIViaRedfish()`, `WaitForNodeToBecomeReady()`, `VerifyVmcoreDumpGenerated()` |

### Eco-goinfra packages

- [**namespace**](https://github.com/rh-ecosystem-edge/eco-goinfra/tree/main/pkg/namespace)
- [**pod**](https://github.com/rh-ecosystem-edge/eco-goinfra/tree/main/pkg/pod)
- [**deployment**](https://github.com/rh-ecosystem-edge/eco-goinfra/tree/main/pkg/deployment)
- [**nodes**](https://github.com/rh-ecosystem-edge/eco-goinfra/tree/main/pkg/nodes)
- [**sriov**](https://github.com/rh-ecosystem-edge/eco-goinfra/tree/main/pkg/sriov)
- [**reportxml**](https://github.com/rh-ecosystem-edge/eco-goinfra/tree/main/pkg/reportxml)

## Inputs and Environment Variables

### Workload configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `KUBECONFIG` | _(required)_ | Path to cluster kubeconfig |
| `ECO_RANDU_TESTWORKLOAD_NAMESPACE` | `test` | Namespace for test workloads |
| `ECO_RANDU_TESTWORKLOAD_CREATE_METHOD` | `shell` | Method used to create the test workload |
| `ECO_RANDU_TESTWORKLOAD_CREATE_SHELLCMD` | `/opt/vdu-workload-emulator/add_test-deployments.sh` | Shell command to create workload |
| `ECO_RANDU_TESTWORKLOAD_DELETE_SHELLCMD` | `/opt/vdu-workload-emulator/delete_test-deployments.sh` | Shell command to delete workload |

### Iteration and timing

| Variable | Default | Description |
|----------|---------|-------------|
| `ECO_RANDU_LAUNCH_WL_ITER` | `5` | Number of workload launch/delete cycles |
| `ECO_RANDU_SOFT_REBOOT_ITERATIONS` | `5` | Number of soft reboot iterations |
| `ECO_RANDU_HARD_REBOOT_ITERATIONS` | `5` | Number of hard reboot iterations |
| `ECO_RANDU_RECOVERY_TIME` | `2` | Post-reboot recovery wait time (minutes) |

### Stability tests

| Variable | Default | Description |
|----------|---------|-------------|
| `ECO_RANDU_STAB_W_DUR_MINS` | `30` | Stability with workload duration (minutes) |
| `ECO_RANDU_STAB_W_INT_MINS` | `5` | Stability with workload check interval (minutes) |
| `ECO_RANDU_STAB_NW_DUR_MINS` | `30` | Stability without workload duration (minutes) |
| `ECO_RANDU_STAB_NW_INT_MINS` | `5` | Stability without workload check interval (minutes) |
| `ECO_RANDU_STABILITY_OUTPUT_PATH` | `/tmp/reports` | Stability report output directory |
| `ECO_RANDU_STABILITY_POLICIES_CHECK` | `true` | Enable policy compliance checks in stability tests |

### PTP configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `ECO_RANDU_PTP_ENABLED` | `true` | Enable PTP validation across tests |
| `ECO_RANDU_PTP_IPERF3_SERVER` | _(empty)_ | iperf3 server address (enables PTP Case 06) |
| `ECO_RANDU_PTP_IPERF3_CLIENT_BIND` | _(empty)_ | iperf3 client bind address (`-B` flag) |
| `ECO_RANDU_PTP_IPERF3_DURATION_SEC` | `300` | iperf3 test duration in seconds |
| `ECO_RANDU_PTP_LOCKED_WAIT_SEC` | `0` | Wait for stable PTP lock before Case 06/07 (seconds) |
| `ECO_RANDU_PTP_NETEM_INTERFACE` | _(empty)_ | netem interface for packet loss test (enables PTP Case 07) |
| `ECO_RANDU_PTP_WPC_SYNC_INTERFACES` | _(empty)_ | Comma-separated PHC sync interfaces for Case 01/02 |
| `ECO_RANDU_PTP_WPC_PRIMARY_IFACE` | _(empty)_ | Primary PHC interface |

### BMC / NMI

| Variable | Default | Description |
|----------|---------|-------------|
| `ECO_RANDU_NODES_CREDENTIALS_MAP` | _(empty)_ | BMC credentials. Format: `node1,user,pass,https://bmc:443;node2,user,pass,https://bmc2:443`. NMI tests are skipped when empty. |

### Cert-manager

| Variable | Default | Description |
|----------|---------|-------------|
| `ECO_RANDU_CERTMANAGER_DNS_SERVER` | _(empty)_ | DNS server for DNS-01 ACME challenge |
| `ECO_RANDU_CERTMANAGER_CERT_DOMAIN` | _(empty)_ | Domain for test certificates |
| `ECO_RANDU_CERTMANAGER_API_DOMAIN` | _(empty)_ | API server domain for certificate replacement |
| `ECO_RANDU_CERTMANAGER_APPS_DOMAIN` | _(empty)_ | Wildcard apps domain for ingress certificate |
| `ECO_RANDU_CERTMANAGER_INGRESS_IP` | _(empty)_ | Ingress IP address |
| `ECO_RANDU_CERTMANAGER_ISSUER_NAME` | `acme-issuer` | ClusterIssuer name for ACME certificates |

Tests requiring configuration that is not set will be skipped.

## Running RAN DU Test Suites

### Running all tests

```bash
export KUBECONFIG=/path/to/kubeconfig
export ECO_TEST_FEATURES="ran-du"
export ECO_RANDU_TESTWORKLOAD_CREATE_SHELLCMD="/path/to/create-workload.sh"
export ECO_RANDU_TESTWORKLOAD_DELETE_SHELLCMD="/path/to/delete-workload.sh"
make run-tests
```

### Running a specific test by label

```bash
cd tests/system-tests/ran-du
ginkgo -v --label-filter="randu && launch-workload" ./tests
```

### Running PTP 3WPC validation

```bash
export KUBECONFIG=/path/to/kubeconfig
export ECO_RANDU_PTP_ENABLED=true
export ECO_RANDU_PTP_WPC_SYNC_INTERFACES="ens1f0,ens2f0,ens3f0"
export ECO_RANDU_PTP_IPERF3_SERVER="192.168.1.100"
export ECO_RANDU_PTP_NETEM_INTERFACE="ens1f0"
export ECO_TEST_FEATURES="ran-du"
export ECO_TEST_LABELS="randu && ptp-3wpc"
make run-tests
```

### Running NMI kernel crash test (requires BMC)

```bash
export KUBECONFIG=/path/to/kubeconfig
export ECO_RANDU_NODES_CREDENTIALS_MAP="worker-0,admin,password,https://bmc-0.example.com:443;worker-1,admin,password,https://bmc-1.example.com:443"
export ECO_TEST_FEATURES="ran-du"
export ECO_TEST_LABELS="randu && NMIKernelCrashKdump"
make run-tests
```

### Running cert-manager tests

```bash
export KUBECONFIG=/path/to/kubeconfig
export ECO_RANDU_CERTMANAGER_DNS_SERVER="192.168.1.1:53"
export ECO_RANDU_CERTMANAGER_CERT_DOMAIN="example.com"
export ECO_RANDU_CERTMANAGER_API_DOMAIN="api.cluster.example.com"
export ECO_RANDU_CERTMANAGER_APPS_DOMAIN="apps.cluster.example.com"
export ECO_RANDU_CERTMANAGER_INGRESS_IP="10.0.0.100"
export ECO_TEST_FEATURES="ran-du"
export ECO_TEST_LABELS="randu && cert-manager"
make run-tests
```

### Running stability tests (30-minute duration)

```bash
export KUBECONFIG=/path/to/kubeconfig
export ECO_RANDU_STAB_W_DUR_MINS=30
export ECO_RANDU_STAB_W_INT_MINS=5
export ECO_RANDU_STABILITY_OUTPUT_PATH="/tmp/stability-reports"
export ECO_TEST_FEATURES="ran-du"
export ECO_TEST_LABELS="randu && StabilityWorkload"
make run-tests
```

### Using the Docker image

```bash
make build-docker-image-ran-du
podman run --rm -e KUBECONFIG=/kubeconfig -v /path/to/kubeconfig:/kubeconfig:Z eco-gotests-ran-du:latest
```

## Additional Information

Please refer to the [project README](../../../README.md) for global configuration, reporting options, and general test framework documentation.
