# RHWA Team - Node Health Check Operator

## Overview

NHC operator tests validate that the Node Health Check (NHC) and Self Node Remediation (SNR) operators
work together to detect unhealthy nodes and remediate them by fencing and evicting stateful workloads
to healthy nodes.

The first test scenario is **Sudden loss of a node**: a healthy MNO cluster experiences the unexpected
shutdown of a worker node running a stateful application. The NHC operator detects the node failure,
creates a `SelfNodeRemediation` resource, and the SNR operator applies an `out-of-service` taint to
fence the node. Kubernetes then force-evicts the stateful pod and reschedules it on a healthy node,
reattaching its persistent storage.

### Prerequisites for running these tests:

The test suite is designed to run on an OCP cluster version 4.19+ with the following components
and configuration.

It has been run successfully on these OCP versions:
- 4.19

<<<<<<< HEAD
It has been tested on bare-metal nodes. For virtualised infrastructure, a virtual BMC must be used, 
=======
It has been tested on bare metal nodes. For virtualised infrastructure, a virtual BMC must be used, 
>>>>>>> 3cc48744 (rhwa nhc: add NHC & SNR sudden-loss system test)
such as:

  - sushy-emulator (from the sushy project) — exposes a Redfish API that maps to libvirt VM power
  operations
  - VirtualBMC (vbmc) — maps IPMI commands to libvirt, though the test uses Redfish not IPMI

With the sushy-emulator running on the hypervisor, the ECO_RHWA_NHC_TARGET_WORKER_BMC environment
variable must point at the sushy endpoint, 
e.g. `{"address":"hypervisor:8000","username":"admin","password":"password"}`). The VMs must have a
watchdog device configured (e.g. i6300esb in libvirt), or set `isSoftwareRebootEnabled: true` as a
fallback.

#### Cluster topology

<<<<<<< HEAD
* A Multi-Node OpenShift (MNO) cluster with **bare-metal** or **virtualised** worker nodes
=======
* A Multi-Node OpenShift (MNO) cluster with **bare metal** or **virtualised** worker nodes
>>>>>>> 3cc48744 (rhwa nhc: add NHC & SNR sudden-loss system test)
* At least **2 worker nodes** that will be used by the test (a target node and one or more
  failover nodes). The test labels the target node with `node-role.kubernetes.io/appworker`
  first to guarantee initial pod placement, then labels the failover nodes after the app is
  deployed. All labels are removed at the end
* The target worker node must have **BMC/Redfish** (or iLO/IPMI) access for power control.
  The test powers it off via BMC to simulate sudden power loss and powers it back on at the end

The test observes the full remediation lifecycle:

1. Node `Ready` condition transitions to `Unknown` (~40s after power-off)
2. NHC detects the unhealthy condition and creates a `SelfNodeRemediation` CR (~60s after condition change)
3. SNR fences the node with an `out-of-service` taint (~180s after `safeTimeToAssumeNodeRebootedSeconds`)
4. The stateful pod is evicted and rescheduled on a healthy node
5. The PVC is reattached and the pod becomes Ready on the new node
6. The node is powered back on via BMC and returns to `Ready` state

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

| Name | Description |
|------|-------------|
| [sudden-node-loss](tests/sudden-node-loss.go) | Powers off a worker node via BMC and verifies NHC/SNR remediation and pod rescheduling |

### Internal pkgs

| Name | Description |
|------|-------------|
| [nhcparams](internal/nhcparams/const.go) | Constants, labels, timeouts, and reporter configuration for NHC tests |

### Inputs

Environment variables for test configuration:

- `ECO_RHWA_NHC_TARGET_WORKER`: FQDN of the worker node to power off (must match the BMC address)
- `ECO_RHWA_NHC_FAILOVER_WORKERS`: comma-separated list of worker FQDNs eligible for pod rescheduling
- `ECO_RHWA_NHC_STORAGE_CLASS`: StorageClass name for the test PVC (e.g. `standard`)
- `ECO_RHWA_NHC_APP_IMAGE`: container image for the stateful test application
- `ECO_RHWA_NHC_TARGET_WORKER_BMC`: JSON object with BMC connection details, e.g. `{"address":"10.1.29.13","username":"user","password":"pass"}`

Please refer to the project README for a list of global inputs - [How to run](../../../README.md#how-to-run)

### Running NHC Test Suites

```bash
# export KUBECONFIG=</path/to/kubeconfig>
# export ECO_RHWA_NHC_TARGET_WORKER=openshift-worker-0.example.com
# export ECO_RHWA_NHC_FAILOVER_WORKERS=openshift-worker-1.example.com
# export ECO_RHWA_NHC_STORAGE_CLASS=standard
# export ECO_RHWA_NHC_APP_IMAGE=registry.example.com:5000/test/ubi-minimal:latest
# export ECO_RHWA_NHC_TARGET_WORKER_BMC='{"address":"10.1.29.13","username":"admin","password":"secret"}'
# make run-tests
```

**Note on timeouts:** The `go test` command must use `-timeout` greater than the ginkgo timeout
(e.g. `-timeout=30m` with `-ginkgo.timeout=20m`). If `go test` uses its default of 10 minutes,
the Go test harness will kill the process before ginkgo can complete the test and run cleanup
(AfterAll), which includes powering the node back on.

**Expected duration:** A full sudden-node-loss run typically takes **11–15 minutes** end-to-end,
<<<<<<< HEAD
broken down as follows (observed on a 4-worker bare-metal cluster with `unhealthyConditions.duration=60s`
=======
broken down as follows (observed on a 4-worker bare metal cluster with `unhealthyConditions.duration=60s`
>>>>>>> 3cc48744 (rhwa nhc: add NHC & SNR sudden-loss system test)
and `safeTimeToAssumeNodeRebootedSeconds=180`):

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

