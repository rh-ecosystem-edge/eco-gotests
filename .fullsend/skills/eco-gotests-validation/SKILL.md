---
name: eco-gotests-validation
description: Run the hermetic eco-gotests validation path from Fullsend code and fix agents.
---

# eco-gotests validation

From the repo root, identify the Go packages affected by the change. Check
the suite's documentation and test prerequisites. Pass the package directories
inside the checkout whose unit tests run without a cluster, credentials or
hardware. Each argument must be a direct directory path beginning with `./`:

```console
bash scripts/fullsend-agent-validation.sh ./path/to/affected/package
```

The script checks the declared Go toolchain, formatting and vet, then runs
the selected packages with `UNIT_TEST=true` and the `unit_test` build tag.
With no package arguments it runs a shared smoke test in
`./tests/internal/klogfilter`. The script reports each package with zero
completed tests and fails if the entire run completed zero tests. Include
any compile-only packages in the validation summary.

If the sandbox lacks the declared toolchain or a suite needs OpenShift or
hardware, report the prerequisite and leave the relevant validation to CI.
Use the preinstalled linter when available and report any checks blocked by
the sandbox.
