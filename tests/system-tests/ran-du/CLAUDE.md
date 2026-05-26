# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Is

RAN DU system tests for OpenShift — Ginkgo v2 tests that validate workload lifecycle, node reboots, kernel crash recovery, PTP synchronization, ZTP policy compliance, cert-manager certificate management, and long-running stability on a live OCP cluster with RAN DU profile.

Part of the [eco-gotests](../../../CLAUDE.local.md) framework. Go 1.26, Ginkgo v2.

## Build and Lint

```bash
# From repo root
make lint                  # Required before PRs
make vet                   # Go vet
make deps-update           # Update vendor directory

# Build check (from this directory)
go build ./...
go vet ./...
```

## Running Tests

Tests require `KUBECONFIG` pointing at a live OCP cluster. Run from repo root:

```bash
export KUBECONFIG=/path/to/kubeconfig
export ECO_TEST_FEATURES="ran-du"
make run-tests
```

Run a single test by label:

```bash
cd tests/system-tests/ran-du
ginkgo -v --label-filter="randu && launch-workload" ./tests
```

Labels match constants in `internal/randuparams/const.go` (e.g., `launch-workload`, `SoftReboot`, `HardReboot`, `KernelCrashKdump`, `NMIKernelCrashKdump`, `ptp-3wpc`, `ZTPPoliciesCompliance`, `cert-manager`, `StabilityWorkload`, `StabilityNoWorkload`).

## Architecture

### Suite layout

```
ran-du/
├── ran_du_suite_test.go              # Ginkgo entrypoint (BeforeSuite/AfterSuite/reporter)
├── internal/
│   ├── randuinittools/               # Exports APIClient + RanDuTestConfig (use via dot-import)
│   ├── randuconfig/config.go         # Config struct loaded from default.yaml + ECO_RANDU_* env vars
│   ├── randuconfig/default.yaml      # Default config values
│   ├── randuparams/const.go          # Labels, timeouts, namespace constants
│   ├── randuparams/randuvars.go      # Reporter config, test namespace name
│   └── randutestworkload/            # Workload namespace cleanup helper
└── tests/                            # Test files (NOT *_test.go — plain .go, package ran_du_system_test)
```

### Key patterns

- **All test files** are in `tests/` as plain `.go` files (not `*_test.go`), all in package `ran_du_system_test`. They are pulled in via blank import `_ ".../ran-du/tests"` in the suite file.
- **Dot-import `randuinittools`** to get `APIClient` and `RanDuTestConfig` in scope.
- **Config**: `RanDuConfig` loads `default.yaml` then overrides from `ECO_RANDU_*` environment variables via `envconfig`. Access nested config like `RanDuTestConfig.TestWorkload.Namespace`, `RanDuTestConfig.CertManager.DNSServer`.
- **Every `It()` must have `reportxml.ID("...")`** with the test case ID.
- **Comment convention**: `// 12345 - Test description` above each `It()`.
- **Use `By("description")`** to document logical steps within tests.
- **Constants** go in `internal/randuparams/`, never in test files.
- **Cleanup**: `AfterAll`/`AfterEach`/`defer` must restore all modified cluster state, even on failure. Use `klog.V(100).Infof` for cleanup error logging (don't fail cleanup with Expect).

### Shared packages (from `tests/system-tests/internal/`)

| Package | Key functions |
|---------|--------------|
| `shell` | `ExecuteCmd()` |
| `await` | `WaitUntilAllDeploymentsReady()`, `WaitUntilAllPodsReady()`, `WaitUntilAllStatefulSetsReady()` |
| `reboot` | `SoftRebootNode()`, `HardRebootNode()`, `KernelCrashKdump()` |
| `ptp` | `ValidatePTPStatus()` |
| `stability` | `SavePTPStatus()`, `SavePolicyStatus()`, `VerifyStabilityStatusChange()` |
| `nmi` | `TriggerNMIViaRedfish()`, `WaitForNodeToBecomeReady()`, `VerifyVmcoreDumpGenerated()` |
| `remote` | `ExecuteOnNodeWithDebugPod()` |

### eco-goinfra

All K8s API interactions use `eco-goinfra` builder types (`namespace`, `pod`, `deployment`, `secret`, `sriov`, `reportxml`, etc.). Never use raw client-go directly.

## Writing New Tests

1. Create a `.go` file in `tests/` with package `ran_du_system_test`.
2. Dot-import `randuinittools` for `APIClient` and `RanDuTestConfig`.
3. Add label constant to `internal/randuparams/const.go`.
4. Add config fields to `internal/randuconfig/config.go` and `default.yaml` if the test needs new env vars.
5. Use `Ordered, ContinueOnFailure` on the `Describe` block when tests within must run sequentially.
6. Skip tests when required configuration is missing (e.g., `Expect(domain).ToNot(BeEmpty(), "ENV must be set")`).
7. Add the new label to this file's "Labels" list and to the README test suite table.

## Commit Convention

```
ran-du: <short summary in lowercase>
```
