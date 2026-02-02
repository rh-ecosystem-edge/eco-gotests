---
name: cnf-ran-review
description: Use this skill to review code changes in the `tests/cnf/ran` directory.
disable-model-invocation: true
---

# `tests/cnf/ran` code review

## Scope and constraints

- Review the changes on this branch, focusing on `tests/cnf/ran/**`.
- Do **not** make code changes unless you are explicitly asked.
- Output should be **review comments supported by code** and a **final PR verdict**.

## Go version

This repository is using the latest stable Go version (check `go.mod` to verify). Ensure any suspected issues are still valid with the latest Go version.

## Project structure (orientation)

Under `tests/cnf/ran`, there is an `internal` directory along with directories for each test suite. Broadly-applicable constants and helpers are in packages under `tests/cnf/ran/internal`.

Inside each test suite directory, there are typically:

- `internal/`: suite-specific helpers and params
  - `internal/tsparams`: parameters for the suite (avoid test assertions here)
- `tests/`: test cases
- `*_suite_test.go`: Ginkgo suite entrypoint (suite-wide setup/teardown + reporting)

## Review workflow (do this order)

1. Summarize the change set:
   - List the changed files and their role (suite test vs suite `internal` vs shared `tests/cnf/ran/internal`).
   - Briefly describe what behavior the change is trying to add/fix.
2. Review file-by-file, prioritizing correctness and flake-risk:
   - Check logic, error handling, cleanup/teardown, timeouts/retries, and any API interactions.
3. Apply the checklist below (only mention items that are violated; don’t restate the whole checklist).

## Output format (be consistent)

- Start with:
  - **Summary**: 2–6 bullets of what changed + main risks
  - **What I did not validate**: e.g., “not runnable without a cluster/env”
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
- [ ] Resources created during tests are cleaned up reliably (including failure paths). Any resources modified during the test are restored to their original state after the test.
- [ ] Changes include the minimal code necessary to achieve the desired behavior. No unnecessary code is added.

### Style

- [ ] Changes follow existing code style and conventions.
- [ ] All new tests have a `reportxml.ID("...")` set on the `It(...)`.
- [ ] Each new test case has a comment above it of the form `// <ID> - <Title>`, matching the `reportxml.ID`.
- [ ] Any new functions have a comment describing purpose + edge cases/limitations.
- [ ] Gomega is only used in test files (`*/tests/*.go`, `*_test.go`), not `internal` packages.
- [ ] Helpers in test files are kept minimal and local; if used across multiple files, prefer moving to a suite `internal/helper` package (or shared `tests/cnf/ran/internal` when broadly applicable).

### Reuse / placement

- [ ] Changes reuse existing constants and helpers where possible.
- [ ] Shared helpers/constants live under `tests/cnf/ran/internal` (not inside a single suite).
- [ ] `github.com/rh-ecosystem-edge/eco-goinfra` packages are used for all Kubernetes API interactions.
- [ ] Broad helpers tied to a specific Kubernetes resource use `eco-goinfra` packages.

### Automated checks

- [ ] `make vet` passes.
- [ ] `make lint` passes.
