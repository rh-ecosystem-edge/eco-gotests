---
name: cnf-core-network-review
description: Use this skill to review code changes in the `tests/cnf/core/network` directory.
disable-model-invocation: true
---

# `tests/cnf/core/network` code review

## Scope and constraints

- Review the changes on this branch, focusing on `tests/cnf/core/network/**`.
- Do **not** make code changes unless you are explicitly asked.
- Output should be **review comments supported by code** and a **final PR verdict**.

## Go version

This repository is using the latest stable Go version (check `go.mod` to verify). Ensure any suspected issues are still valid with the latest Go version.

## Project structure (orientation)

Under `tests/cnf/core/network`, there is a shared `internal` directory along with directories for each test suite: `accelerator`, `cni`, `day1day2`, `dpdk`, `metallb`, `policy`, `security`, and `sriov`.

### Shared internal packages (`tests/cnf/core/network/internal`)

- `netinittools`: exports `APIClient` (`*clients.Settings`) and `NetConfig` (`*netconfig.NetworkConfig`). An `init()` function auto-initializes both on package load. Designed for dot-import (`. "…/netinittools"`) so `APIClient` and `NetConfig` are available without a package prefix.
- `netconfig`: configuration via `default.yaml` and `ECO_CNF_CORE_NET_*` environment variables (struct tags: `envconfig:"ECO_CNF_CORE_NET_…"`).
- `netenv`: cluster environment helpers (`IsSNOCluster()`, `DoesClusterHasEnoughNodes()`, `DeployPerformanceProfile()`, `SetStaticRoute()`, etc.).
- `netparam`: shared constants (IP families, subnets, timeouts, labels) and types (e.g. `BFDDescription`).
- `cmd`: command and connectivity helpers (`ICMPConnectivityCheck()`, `ValidateTCPTraffic()`, `RunCommandOnHostNetworkPod()`, `GetSrIovPf()`). Also provides Juniper switch management (`NewSession()` → `Junos` type with `Config()`, `RunCommand()`, `SaveInterfaceConfigs()`, etc.).
- `define`: NAD builders (`TapNad()`, `MacVlanNad()`, `VlanNad()`, `IPVlanNad()`, `HostDeviceNad()`, `CreateExternalNad()`).
- `ipaddr`: IP address utilities (`RemovePrefix()`).
- `frrconfig`: FRR daemon configuration generation (`DefineBaseConfig()`, `CreateStaticIPAnnotations()`).
- `netnmstate`: NMState instance helpers (`CreateNewNMStateAndWaitUntilItsRunning()`, `CreatePolicyAndWaitUntilItsAvailable()`, `ConfigureVFsAndWaitUntilItsConfigured()`, `CheckThatWorkersDeployedWithBondVfs()`, etc.).

### Test suite directory layout

Inside each test suite directory (e.g. `sriov/`, `metallb/`), there are typically:

- `internal/`: suite-specific helpers and params
  - `internal/tsparams/`: parameters and constants for the suite (avoid test assertions here)
  - `internal/*env/`: optional environment/setup helpers (e.g. `sriovenv`, `metallbenv`, `dpdkenv`)
  - Other suite-specific packages (e.g. `metallb/internal/frr/`, `metallb/internal/prometheus/`, `metallb/internal/cmd/`, `day1day2/internal/juniper/`, `dpdk/internal/link/`)
- `tests/`: test cases (`.go` files, not `_test.go`)
- `*_suite_test.go`: Ginkgo suite entrypoint (suite-wide setup/teardown + reporting)

### Cleanup

- Per-test cleanup commonly uses `namespace.NewBuilder(APIClient, nsName).CleanObjects(…)` in `AfterEach` to remove pods/resources.
- Suite-level cleanup deletes the test namespace in `AfterSuite`.
- Resources modified during a test (e.g. SR-IOV policies, NMState policies) must be restored in `AfterAll` or `AfterEach`, even on failure.

### `By()` blocks

`By("description")` is used extensively to document logical steps inside test cases. New tests should follow this convention.

## Review workflow (do this order)

1. Summarize the change set:
   - List the changed files and their role (suite test vs suite `internal` vs shared `tests/cnf/core/network/internal`).
   - Briefly describe what behavior the change is trying to add/fix.
2. Review file-by-file, prioritizing correctness and flake-risk:
   - Check logic, error handling, cleanup/teardown, timeouts/retries, and any API interactions.
3. Apply the checklist below (only mention items that are violated; don't restate the whole checklist).

## Output format (be consistent)

- Start with:
  - **Summary**: 2-6 bullets of what changed + main risks
  - **What I did not validate**: e.g., "not runnable without a cluster/env"
- Then list **Comments**, grouped by severity and globally numbered:
  - **Blocker** (must fix), **Major**, **Minor**, **Nit**
  - Each comment must include:
    - **Location**: `path/to/file.go` (+ function name and/or approximate line range)
    - **Evidence**: a small code quote
    - **Why it matters**: correctness/maintenance/flake-risk
    - **Suggested fix**: concrete change
- End with **Verdict**: Approve / Approve with nits / Request changes.

## Code review checklist

### Functionality

- [ ] Changes are functional and behave as expected.
- [ ] Changes do not break existing functionality.
- [ ] No obvious bugs or logic errors are introduced.
- [ ] Test behavior is deterministic (no unnecessary `time.Sleep`, reasonable timeouts/poll intervals).
- [ ] `Eventually` / `Consistently` assertions use explicit timeout and poll-interval values (prefer named constants from `netparam` or `tsparams` over magic numbers).
- [ ] Resources created during tests are cleaned up reliably (including failure paths). Any resources modified during the test are restored to their original state after the test.
- [ ] Changes include the minimal code necessary to achieve the desired behavior. No unnecessary code is added.

### Style

- [ ] Changes follow existing code style and conventions.
- [ ] All new test specs have a `reportxml.ID("...")` set on the `It(...)` or `DescribeTable(...)`.
- [ ] Parameterized tests (`DescribeTable`/`Entry`) use `reportxml.SetProperty(…)` on `Entry` calls to tag variations.
- [ ] Any new functions have a comment describing purpose + edge cases/limitations.
- [ ] Logical steps within a test case are documented with `By("…")` calls.
- [ ] Gomega is only used in test files (`*/tests/*.go`, `*_test.go`), not `internal` packages.
- [ ] Helpers in test files are kept minimal and local; if used across multiple files, prefer moving to a suite `internal/helper` package (or shared `tests/cnf/core/network/internal` when broadly applicable).

### Reuse / placement

- [ ] Changes reuse existing constants and helpers where possible.
- [ ] Shared helpers/constants live under `tests/cnf/core/network/internal` (not inside a single suite).
- [ ] Suite-specific params live in `<suite>/internal/tsparams/`, not in test files or shared `internal`.
- [ ] `github.com/rh-ecosystem-edge/eco-goinfra` packages are used for all Kubernetes API interactions.
- [ ] Broad helpers tied to a specific Kubernetes resource use `eco-goinfra` packages.
- [ ] Configuration values use `NetConfig` (via `netconfig`/`netinittools`) rather than hardcoded strings.

### Automated checks

- [ ] `make vet` passes.
- [ ] `make lint` passes.
