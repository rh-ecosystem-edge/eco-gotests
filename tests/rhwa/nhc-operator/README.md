# RHWA Team - Node Health Check Operator

## Overview

NHC operator tests validate that the Node Health Check (NHC) and Self Node Remediation (SNR) operators
work together to detect unhealthy nodes and remediate them by fencing and evicting stateful workloads
to healthy nodes — and, equally important, that they do **not** interfere with planned maintenance
operations such as cluster upgrades.

There are two test scenarios:

1. **Sudden loss of a node**: a healthy MNO cluster experiences the unexpected shutdown of a worker
   node running a stateful application. The NHC operator detects the node failure, creates a
   `SelfNodeRemediation` resource, and the SNR operator applies an `out-of-service` taint to fence
   the node. Kubernetes then force-evicts the stateful pod and reschedules it on a healthy node,
   reattaching its persistent storage.

2. **Planned reboot of a node during cluster upgrade**: a cluster upgrade is initiated while a
   stateful application is running on a worker node. Worker nodes reboot as part of the
   MachineConfigPool rollout. The NHC operator detects the ongoing upgrade (by observing the
   difference between `currentConfig` and `desiredConfig` in the MCP) and does **not** trigger
   remediation. The test verifies that no `SelfNodeRemediation` resources are created during the
   entire upgrade process, and that the stateful application survives the upgrade.

### Prerequisites for running these tests:

The test suite is designed to run on an OCP cluster version 4.19+ with the following components
and configuration.

It has been run successfully on these OCP versions:
- 4.19
- 4.21

#### Notes about the infrastructure

Both scenarios have been tested on bare-metal nodes. To run the **Sudden loss of a node** in a
virtualised infrastructure, a **Redfish endpoint** is required because the test constructs a
Redfish client to control node power. Suitable options include:

  - sushy-emulator (from the sushy project) — exposes a Redfish API that maps to libvirt VM power
  operations
  - VirtualBMC (vbmc) — provides IPMI-only access to libvirt VMs. Since the test uses Redfish
  (not IPMI), vbmc alone is **not sufficient**; it must be paired with a Redfish front-end such
  as sushy-emulator or an equivalent Redfish proxy

With the sushy-emulator running on the hypervisor, the ECO_RHWA_NHC_TARGET_WORKER_BMC environment
variable must point at the Redfish endpoint (not a plain IPMI/vbmc address),
e.g. `{"address":"hypervisor:8000","username":"admin","password":"password"}`. The VMs must have a
watchdog device configured (e.g. i6300esb in libvirt), or set `isSoftwareRebootEnabled: true` as a
fallback.

#### Cluster topology

* A Multi-Node OpenShift (MNO) cluster with **bare-metal** or **virtualised** worker nodes
* At least **2 worker nodes** that will be used by the test (a target node and one or more
  failover nodes). The test labels the target node with `node-role.kubernetes.io/appworker`
  first to guarantee initial pod placement, then labels the failover nodes after the app is
  deployed. All labels are removed at the end
* The target worker node must have **BMC/Redfish** (or iLO/IPMI) access for power control.
  This is required by the **sudden-loss** test only (powers off the node via BMC to simulate
  sudden power loss and powers it back on at the end)

#### Sudden-loss remediation lifecycle

The sudden-loss test observes the full remediation lifecycle:

1. Node `Ready` condition transitions to `Unknown` (~40s after power-off)
2. NHC detects the unhealthy condition and creates a `SelfNodeRemediation` CR (~60s after condition change)
3. SNR fences the node with an `out-of-service` taint (~180s after `safeTimeToAssumeNodeRebootedSeconds`)
4. The stateful pod is evicted and rescheduled on a healthy node
5. The PVC is reattached and the pod becomes Ready on the new node
6. The node is powered back on via BMC and returns to `Ready` state

#### Planned-reboot non-remediation lifecycle

The planned-reboot test observes the **absence** of remediation during a cluster upgrade:

1. A stateful application is deployed on a target worker node
2. A cluster upgrade is initiated by patching the `ClusterVersion` resource
3. Throughout the upgrade (~1.5–2.5 hours), the test polls every 30s to verify that no
   `SelfNodeRemediation` resources are created for any worker node
4. After the upgrade completes, the test verifies that NHC reports all nodes healthy,
   no `out-of-service` taints exist, all cluster operators are available, and the stateful
   application survived

#### Operators

* **Node Health Check operator** (namespace: `openshift-workload-availability`)
* **Self Node Remediation operator** (installed as default remediation provider by NHC)

#### Operator configuration

* A `SelfNodeRemediationTemplate` CR with `remediationStrategy: OutOfServiceTaint`
* A `NodeHealthCheck` CR (named `nhc-worker-self`) configured with:
  * A `selector` matching the worker nodes monitored by NHC (e.g. `node-role.kubernetes.io/worker`).
    The selector must match the target and failover nodes
  * `minHealthy` set to a value that is **still satisfied** when one node goes down.
    For example, with 4 workers under NHC, use `75%` — losing 1 node leaves 3/4 = 75% healthy,
    which meets the threshold. If `minHealthy` is too high (e.g. `90%` with 4 nodes requires
    all 4 healthy), NHC will not remediate
  * `unhealthyConditions` with `duration: 60s` for `Ready` in `False` and `Unknown` status
  * A `remediationTemplate` pointing to the `SelfNodeRemediationTemplate` above
* A `SelfNodeRemediationConfig` CR with `safeTimeToAssumeNodeRebootedSeconds: 180`

The [Telco Reference CRs](https://github.com/openshift-kni/telco-reference/)
can provide an up-to-date configuration and values for the settings above.

#### Storage

* A **StorageClass** capable of dynamically provisioning `ReadWriteOnce` PersistentVolumes
  (e.g. NFS-based). The test creates a 1Gi PVC for the stateful application. The storage
  must support volume reattachment to a different node after the original node is fenced
* The test verifies `VolumeAttachment` resources for CSI-backed storage. For non-CSI storage
  (e.g. NFS), this check is skipped — the PVC being Bound and the pod Running on the new node
  is sufficient verification

#### Container image

* A container image accessible from the cluster (e.g. `ubi-minimal`). In disconnected
  environments, mirror it to the local registry. The test uses this image to run a simple
  heartbeat loop as the stateful application

### Test suites:

| Name | Label | Description |
|------|-------|-------------|
| [sudden-node-loss](tests/sudden-node-loss.go) | `sudden-loss` | Powers off a worker node via BMC and verifies NHC/SNR remediation and pod rescheduling |
| [planned-node-reboot](tests/planned-node-reboot.go) | `planned-reboot` | Initiates a cluster upgrade and verifies NHC does **not** remediate during planned node reboots |

### Internal pkgs

| Name | Description |
|------|-------------|
| [nhcparams](internal/nhcparams/const.go) | Constants, labels, timeouts, and reporter configuration for NHC tests |

### Inputs

Environment variables for test configuration:

#### Common (both tests)

- `ECO_RHWA_NHC_TARGET_WORKER`: FQDN of the worker node to target
- `ECO_RHWA_NHC_FAILOVER_WORKERS`: comma-separated list of worker FQDNs eligible for pod rescheduling
- `ECO_RHWA_NHC_STORAGE_CLASS`: StorageClass name for the test PVC (e.g. `standard`)
- `ECO_RHWA_NHC_APP_IMAGE`: container image for the stateful test application

#### Sudden-loss only

- `ECO_RHWA_NHC_TARGET_WORKER_BMC`: JSON object with BMC connection details, e.g. `{"address":"10.1.29.13","username":"user","password":"pass"}`

#### Planned-reboot only

- `ECO_RHWA_NHC_UPGRADE_IMAGE`: the target OCP release image for the upgrade (must be pre-mirrored in disconnected environments)
- `ECO_RHWA_NHC_UPGRADE_CHANNEL`: the update channel (e.g. `stable-4.22`)

Please refer to the project README for a list of global inputs - [How to run](../../../README.md#how-to-run)

### Running NHC Test Suites

#### Running the sudden-loss test

```bash
export KUBECONFIG=/path/to/kubeconfig
export ECO_RHWA_NHC_TARGET_WORKER=openshift-worker-0.example.com
export ECO_RHWA_NHC_FAILOVER_WORKERS=openshift-worker-1.example.com
export ECO_RHWA_NHC_STORAGE_CLASS=standard
export ECO_RHWA_NHC_APP_IMAGE=registry.example.com:5000/test/ubi-minimal:latest
export ECO_RHWA_NHC_TARGET_WORKER_BMC='{"address":"10.1.29.13","username":"admin","password":"secret"}'

go test ./tests/rhwa/nhc-operator/... -timeout=30m -ginkgo.label-filter="sudden-loss" -ginkgo.timeout=20m -v
```

#### Running the planned-reboot test

```bash
export KUBECONFIG=/path/to/kubeconfig
export ECO_RHWA_NHC_TARGET_WORKER=openshift-worker-0.example.com
export ECO_RHWA_NHC_FAILOVER_WORKERS=openshift-worker-1.example.com
export ECO_RHWA_NHC_STORAGE_CLASS=standard
export ECO_RHWA_NHC_APP_IMAGE=registry.example.com:5000/test/ubi-minimal:latest
export ECO_RHWA_NHC_UPGRADE_IMAGE=registry.example.com:5000/ocp/release:4.22.1
export ECO_RHWA_NHC_UPGRADE_CHANNEL=stable-4.22

go test ./tests/rhwa/nhc-operator/... -timeout=180m -ginkgo.label-filter="planned-reboot" -ginkgo.timeout=170m -v
```

**Note on timeouts:** The `go test` command must use `-timeout` greater than the ginkgo timeout.
If `go test` uses its default of 10 minutes, the Go test harness will kill the process before
ginkgo can complete the test and run cleanup (AfterAll).

**Important:** The planned-reboot test **upgrades the cluster** and this operation is
**irreversible**. The upgrade target image must be pre-mirrored to the local registry in
disconnected environments. Plan for 1.5–2.5 hours of runtime.

### Expected durations

#### Sudden-loss test: ~11–15 minutes

Observed on a 4-worker bare-metal cluster with `unhealthyConditions.duration=60s`
and `safeTimeToAssumeNodeRebootedSeconds=180`:

| Phase | Typical duration | Notes |
|-------|-----------------|-------|
| Step 3: Deploy app & verify placement | ~10s | PVC binding + pod scheduling |
| Step 4: Power off node & detect failure | ~50s | ~40s for kubelet heartbeat timeout |
| Step 5: NHC marks unhealthy & creates SNR | ~60s | Matches `unhealthyConditions.duration` |
| Step 6: SNR fences node (out-of-service taint) | 3–5 min | 180s fence timer + SNR waits for all pods on the dead node to finish terminating; system pods like `dns-default` can extend this |
| Step 7: Verify rescheduling | < 1s | Pod is rescheduled as soon as taint is applied |
| AfterAll: Power on node & wait for Ready | ~5 min | Bare metal boot + kubelet registration |

Step 6 is the most variable: after the 180s `safeTimeToAssumeNodeRebootedSeconds` timer expires,
the SNR operator waits for all terminating pods on the fenced node to complete deletion before
marking fencing as complete. System pods (e.g. `dns-default`, `ingress-canary`) on an unreachable
node can take several additional minutes to terminate, pushing Step 6 to 5–8 minutes in the worst
case. Combined with the AfterAll node recovery, the total can reach ~17–20 minutes, which is why
the ginkgo timeout is set to 20 minutes and the Go test timeout to 30 minutes.

#### Planned-reboot test: ~1.5–2.5 hours

The test is dominated by the cluster upgrade time. The test itself polls every 30 seconds and
adds minimal overhead:

| Phase | Typical duration | Notes |
|-------|-----------------|-------|
| BeforeAll: Deploy app & verify placement | ~1 min | Same as sudden-loss |
| Step 4: Initiate upgrade & wait for start | ~5 min | Patches ClusterVersion, waits for Progressing |
| Step 5: Poll during upgrade | 1–2 hours | Polls every 30s for SNR resources (fail-fast) and upgrade completion |
| Steps 6–7: Post-upgrade verification | ~5 min | NHC/SNR clean, cluster operators available, app healthy |
| AfterAll: Namespace cleanup | ~1 min | Labels restored |

