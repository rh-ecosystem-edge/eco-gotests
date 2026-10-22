# Regular KMM Prow jobs

This directory contains the reusable commands for migrating the regular,
single-cluster KMM jobs from Jenkins to Prow. Hub/Spoke jobs are intentionally
out of scope here.

The Jenkins equivalent is the regular `vm-connected-ipv4_dtk-kmm` profile:

1. claim a disposable OCP cluster;
2. install the KMM bundle produced by the KMM Prow config;
3. run the existing `modules` KMM eco-gotests;
4. publish JUnit/Ginkgo reports and cluster diagnostics.

The actual Prow job registration belongs in `openshift/release`. For KMM,
register the job in the KMM component config because KMM owns the operator
bundle and the cluster claim; use an `eco-gotests` image as the test
dependency. These commands remain versioned with the test repository for
local reuse and future image updates. The first job should use the KMM pool's
`generic-claim` workflow and the regular `modules` feature.
Factory-clone parity, ARM/GPU, upgrade, Secure Boot, and Hub/Spoke jobs should
be added only after this baseline job is green.

## Release configuration skeleton

Add the following test to the KMM component config in `openshift/release`
(under `ci-operator/config/rh-ecosystem-edge/kernel-module-management`).
Build an `eco-gotests` image dependency, install the KMM bundle produced by
the KMM config, and then invoke the reusable KMM test step.

```yaml
- as: eco-gotests-kmm-modules
  cluster_claim:
    architecture: amd64
    cloud: aws
    owner: rh-ecosystem-edge
    product: ocp
    timeout: 1h0m0s
    version: "4.21"
  steps:
    test:
    - as: install
      cli: latest
      commands: |
        oc create namespace openshift-kmm
        operator-sdk run bundle -n openshift-kmm --timeout 5m0s "$OO_BUNDLE"
        oc wait --for=condition=Available -n openshift-kmm --timeout=10m deployment/kmm-operator-controller
      dependencies:
      - env: OO_BUNDLE
        name: operator-bundle
      from: operator-sdk
    - ref: kernel-module-management-eco-gotests
  workflow: generic-claim
```

The version above follows the currently documented KMM pool configuration.
Change it only after confirming that the pool has a newer release available.

The baseline job can run without an external-registry token; tests that require
one will skip. If those tests are enabled, provide
`ECO_HWACCEL_KMM_PULL_SECRET` through the Prow cluster-profile credential
mechanism. Do not put the token in this repository. The claimed pool must also
provide an Available OpenShift integrated registry.

The KMM repository's existing `ci/prow` scripts are a separate native KMM
test path. They should remain unchanged while this regular eco-gotests path is
migrated.

## Local validation

```bash
bash -n scripts/test-runner.sh ci/prow/kmm-install ci/prow/kmm-eco-gotests
git diff --check
```
