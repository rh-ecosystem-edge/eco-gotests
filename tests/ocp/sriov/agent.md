# OCP SR-IOV Test Suite — Agent Reference

This file captures investigation findings and non-obvious details to help agents work on this
suite without re-reading every file from scratch.

---

## Infrastructure requirements by test file

This matrix shows what each suite needs to run. Use it to decide whether a given lab
environment can run a suite before reading the test code.

| Test file | Min workers | SR-IOV ifaces | Switch connectivity | Switch credentials | PerformanceProfile | Special NIC / operator |
|-----------|-------------|---------------|--------------------|--------------------|-------------------|------------------------|
| basic.go | 1 | 1 per device | No | No | No¹ | None |
| reinstallation.go | 1 | 2 | No | No | No | OLM + subscription |
| allmulti.go | **2** | 1 | **Yes** (cross-node tests) | No² | No | None |
| exposemtu.go | 1 | 1 | No | No | No¹ | None |
| metricsExporter.go | 1 (2 for some) | 1 (2 for cross-PF) | **Yes** (cross-worker tests) | **Yes**³ | **Yes** (vfio contexts) | Prometheus/monitoring stack |
| paralleldraining.go | **2** (3 for some) | 1 | No | No | No | None |
| rdmametricsapi.go | 1 | **2** | No | No | No | **Mellanox + RDMA** |
| app-ns-sriovnet.go | 1 | **2** | No | No | No | None |
| qinq.go | **2** | 1 | **Yes** (cross-node tests) | **Yes** | **Yes** (DPDK context) | E810 for 802.1AD tests |

**Notes:**
1. DPDK/vfio-pci tests in basic.go and exposemtu.go may need hugepages pre-configured on the node if no PerformanceProfile is deployed.
2. allmulti cross-node tests (67815, 67816) send traffic across the switch but do not need switch credentials — just physical L2 connectivity between the two worker nodes.
3. metricsExporter vfio-pci contexts call `clearClientServerMacTableFromSwitch()` in BeforeEach even for same-PF tests — switch credentials (`ECO_OCP_SRIOV_SWITCH_*`) are required for all vfio-pci and vfio→vfio tests.

### Per-test cross-node traffic detail

Tests that send actual traffic across the switch (need L2 connectivity between workers):

| ID | File | Notes |
|----|------|-------|
| 67815, 67816 | allmulti.go | Multicast source on workers[1], client on workers[0] |
| 75930 | metricsExporter.go | netdev→netdev different worker |
| 75932 | metricsExporter.go | netdev→vfiopci different worker |
| 75934 | metricsExporter.go | vfiopci→vfiopci different worker |
| 71678, 71680 | qinq.go | 802.1AD cross-node (E810 only) |
| 71679, 71683 | qinq.go | 802.1Q cross-node |

---

## All test files — quick reference

### `tests/basic.go` — Label: `basic`

Iterates over all devices in `ECO_OCP_SRIOV_DEVICES`. Each test may skip per-device if
NO-CARRIER detected or device type doesn't match.

| ID | Description | Skip condition |
|----|-------------|----------------|
| 25959 | VF with spoof check enabled | NO-CARRIER |
| 70820 | VF with spoof check disabled | NO-CARRIER |
| 25960 | VF with trust disabled | NO-CARRIER |
| 70821 | VF with trust enabled | NO-CARRIER |
| 25963 | VF with VLAN + rate limiting | device `SupportsMinTxRate=false`, or NO-CARRIER |
| 25961 | VF with auto link state | NO-CARRIER |
| 71006 | VF with enabled link state | NO-CARRIER |
| 69646 | MTU 9000 policy | NO-CARRIER |
| 69582 | DPDK VF validation | Broadcom NIC (OCPBUGS-30909), NO-CARRIER |

---

### `tests/reinstallation.go` — Label: `sriovreinstall`

**Requires**: `ECO_OCP_SRIOV_INTERFACE_LIST` (≥2 interfaces). Tests run in strict order
(Ordered container). Exercises full operator removal + reinstall lifecycle via OLM.

| ID | Description |
|----|-------------|
| 46528 | Control plane operational before removal |
| 46529 | Data plane operational before removal |
| 46530 | All components deleted when operator removed |
| 46531 | SR-IOV resources cannot be deployed without operator |
| 46532 | Reinstalled operator control plane is up |
| 46533 | Reinstalled operator data plane is up |

---

### `tests/allmulti.go` — Labels: `allmulti`, `sriov-hw-enabled`

**Requires**: 2+ workers, 1 SR-IOV interface, MTU 9000 capable NIC. BeforeAll creates
policies on nodes[0] and nodes[1] with 6 VFs each, 7 SR-IOV networks, 2 bonded NADs.

| ID | Description |
|----|-------------|
| 67813 | Receive non-member IPv6 multicast via allmulti — same PF source |
| 67815 | Receive non-member IPv4 multicast via allmulti — different node source |
| 67816 | Receive non-member IPv6 multicast via allmulti — different node source |
| 67817 | Receive non-member dual-stack multicast via allmulti — same PF |
| 67818 | Receive non-member IPv4 multicast via bonded SR-IOV — same PF |
| 67819 | Receive non-member IPv6 multicast via bonded SR-IOV — same PF |
| 67820 | Third interface does NOT receive multicast when allmulti on second interface |

---

### `tests/exposemtu.go` — Label: `exposemtu`

**Requires**: 1 SR-IOV interface, 1+ worker. Tests netdev and vfio-pci devices at MTU
1500 and 9000. AfterEach removes SR-IOV config and pods.

| ID | Description |
|----|-------------|
| 73786 | Expose MTU — netdev 1500 |
| 73787 | Expose MTU — netdev 9000 |
| 73789 | Expose MTU — vfio 1500 |
| 73790 | Expose MTU — vfio 9000 |
| 73788 | Expose MTU — 2 policies with different MTU |

---

### `tests/metricsExporter.go` — Labels: `sriovmetricsexporter`, `sriov-hw-enabled`

**Requires**: 1 SR-IOV interface (2 for cross-PF tests), 1+ worker. BeforeAll enables
the `metricsExporter` feature gate in SriovOperatorConfig and waits for the
`sriov-network-metrics-exporter` daemonset. Adds Prometheus scrape labels to namespace.
AfterAll removes all of that.

Vfio-pci contexts deploy a PerformanceProfile. BeforeEach in vfio-pci contexts
clears the switch MAC table (uses switch credentials).

| ID | Context | Description | Skip condition |
|----|---------|-------------|----------------|
| 74762 | netdev→netdev | Same PF | — |
| 75929 | netdev→netdev | Different PF | <2 SR-IOV interfaces |
| 75930 | netdev→netdev | Different worker | <2 workers |
| 74797 | netdev→vfiopci | Same PF | — |
| 75931 | netdev→vfiopci | Different PF | <2 SR-IOV interfaces |
| 75932 | netdev→vfiopci | Different worker | <2 workers |
| 74800 | vfiopci→vfiopci | Same PF | — |
| 75933 | vfiopci→vfiopci | Different PF | <2 SR-IOV interfaces |
| 75934 | vfiopci→vfiopci | Different worker | <2 workers |

Known issue: test 75931 ("Different PF", netdev→vfiopci) had a reconcile race bug fixed
in PR #1433 (opened 2026-06-09).

---

### `tests/paralleldraining.go` — Label: `paralleldraining`

**Requires**: 1 SR-IOV interface, 2+ workers (3+ for some tests). BeforeEach creates
SR-IOV config, creates test pods with VFs on workers, verifies traffic, then triggers
draining to test parallel drain behavior.

| ID | Description | Skip condition |
|----|-------------|----------------|
| 68640 | Draining without SriovNetworkPoolConfig | — |
| 68661 | Draining without maxUnavailable field | — |
| 68662 | PoolConfig maxUnavailable=2 | <3 workers |
| 68663 | 2 SriovNetworkPoolConfigs | <3 workers |
| 68664 | Draining does not remove non-SR-IOV pod | — |

---

### `tests/rdmametricsapi.go` — Labels: `rdmametricsapi`, `sriov-hw-enabled`

**Requires**: 2 SR-IOV interfaces, Mellanox NIC (non-Mellanox skips entire suite).
Deploys PerformanceProfile. Each context sets a different RDMA mode (exclusive/shared)
via SriovNetworkPoolConfig and verifies metrics API.

| ID | Context | Description |
|----|---------|-------------|
| 77651 | exclusive | 1 pod, 1 VF |
| 77650 | exclusive | 1 pod, 2 VFs same PF |
| 77649 | exclusive | 1 pod, 2 VFs different PF |
| 77653 | shared | 1 pod, 1 VF |

---

### `tests/app-ns-sriovnet.go` — Label: `sriovnetappns`

**Requires**: 1 worker, 2 SR-IOV interfaces. BeforeAll creates two extra test namespaces
with privileged labels plus two SR-IOV policies. Tests verify NAD/SriovNetwork scoping
across namespaces, including targetNamespace and resource update behavior.

| ID | Description |
|----|-------------|
| 83121 | SriovNetwork with 1 resource, 2 user namespaces, no targetNamespace |
| 83123 | SriovNetwork in user namespace with targetNamespace defined |
| 83125 | Update SriovNetwork ResourceName in user namespace |
| 83124 | SriovNetwork with 2 resources, 2 user namespaces, no targetNamespace |
| 83142 | Delete SriovNetwork in user namespace triggers NAD deletion |

---

### `tests/qinq.go` — Labels: `qinq`, `sriov-hw-enabled`

**Requires**: 2+ workers, switch credentials, Juniper switch reachable via NETCONF.
802.1AD and DPDK tests are Intel E810 only (device IDs `1592`/`1593`).

#### Test structure

Three nested Contexts, each with its own BeforeAll/AfterAll:

```text
Describe("QinQ") [Label: qinq, sriov-hw-enabled]
  BeforeAll (outer)      — validates workers, fetches switch credentials, enables VF promisc
  Context("802.1AD")
    BeforeAll            — SKIPS unless Intel E810 (device IDs 1592 or 1593)
                         — calls enableDot1ADonSwitchInterfaces + creates policies/networks
    AfterAll             — calls removeSwitchTPID + cleanTestEnvSRIOVConfiguration
  Context("802.1Q")
    BeforeAll            — creates SR-IOV policies and networks only; NO switch setup
    AfterAll             — cleanTestEnvSRIOVConfiguration only
  Context("DPDK")
    BeforeAll            — deploys PerformanceProfile, creates vfio-pci policy + DPDK networks
    AfterAll             — cleanTestEnvSRIOVConfiguration
  AfterAll (outer)       — calls disableQinQOnSwitch (full restore to access mode)
```

#### Known failure: 802.1Q cross-node test 71679 when using CX6-DX

**Root cause**: The 802.1Q BeforeAll has no switch configuration step. It silently relies on
the 802.1AD BeforeAll having already put the switch into trunk/QinQ mode. When the NIC is a
Mellanox CX6-DX (device ID `101d`), the 802.1AD BeforeAll **skips** (E810-only guard), so
`enableDot1ADonSwitchInterfaces` is never called. The switch stays in access mode, which
strips the outer S-VLAN tag, breaking cross-node QinQ traffic.

Same-node tests (71677) are unaffected because traffic never crosses the switch.

**Fix needed**: Add a switch setup call to the 802.1Q BeforeAll — same as
`enableDot1ADonSwitchInterfaces` but without the `ether-options` TPID line (802.1Q uses
the default 0x8100 TPID). See `enableDot1ADonSwitchInterfaces` at line ~955 for the
command set to adapt.

**Note on `ether-options ethernet-switch-profile tag-protocol-id 0x88a8`**: This is an EX
series Junos command; verify it actually applies on QFX5200 before relying on it.

#### Test IDs

| ID | Context | Scope | Notes |
|----|---------|-------|-------|
| 71676 | 802.1AD | same node | E810 only |
| 71678 | 802.1AD | cross-node | E810 only; uses switch |
| 71682 | 802.1AD | same node | multi-CVLAN; E810 only |
| 71680 | 802.1AD | cross-node | mixed 1ad+1q; E810 only |
| 73105 | 802.1AD | same node | bond; E810 only |
| 71684 | 802.1AD | same node | bond+multi-CVLAN; E810 only |
| 71677 | 802.1Q  | same node | works on CX6-DX |
| 71679 | 802.1Q  | cross-node | **fails on CX6-DX** — see above |
| 71683 | 802.1Q  | cross-node | mixed 1q+1ad; E810 only (server side) |
| 72636 | DPDK    | same node | 802.1AD; E810 only |
| 72638 | DPDK    | same node | 802.1Q; E810 only |
| 71681 | NMState | — | E810 only + NMState operator required |

#### QinQ frame structure

- Outer S-VLAN: value from `ECO_OCP_SRIOV_VLAN` (e.g. 3010), applied by the NIC PF
- Inner C-VLAN 100: hardcoded in `nadcvlan100` NAD
- Inner C-VLAN 101: hardcoded in `nadcvlan101` NAD

#### Switch helper functions (all in `tests/qinq.go`)

| Function | Line | What it does |
|----------|------|--------------|
| `enableDot1ADonSwitchInterfaces` | ~955 | Deletes unit 0, sets TPID 0x88a8, flexible-vlan-tagging + extended-vlan-bridge, trunk with all VLANs |
| `removeSwitchTPID` | ~983 | Removes only the TPID setting; leaves trunk/encap in place |
| `disableQinQOnSwitch` | ~1005 | Full restore: deletes TPID/encap/unit 0, recreates as access + `vlan members <ECO_OCP_SRIOV_VLAN>` |

Switch session uses NETCONF (`sriovocpenv.NewJunosSession`), not plain SSH.
Credentials come from `sriovocpenv.NewSwitchCredentials()` → `SriovOcpConfig.GetSwitchCredentials()`.

---

## Environment variables

See [README.md](README.md) for the full variable list with descriptions. Switch-specific
variables used by QinQ and metricsExporter tests:

| Variable | Notes |
|----------|-------|
| `ECO_OCP_SRIOV_SWITCH_IP` | Switch address |
| `ECO_OCP_SRIOV_SWITCH_USER` | Switch login user |
| `ECO_OCP_SRIOV_SWITCH_PASS` | Jenkins secret `SWITCH_PASS_CRED`; masked in console but real value reaches container |
| `ECO_OCP_SRIOV_SWITCH_INTERFACES` | Junos port names, comma-separated |
| `ECO_OCP_SRIOV_VLAN` | Outer S-VLAN ID |

Config parsing: `internal/ocpsriovconfig/config.go`  
Switch NETCONF session: `internal/sriovocpenv/switch.go`

