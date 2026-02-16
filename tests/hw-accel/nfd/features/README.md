# NFD E2E Test Suite

This directory contains comprehensive end-to-end tests for Node Feature Discovery (NFD) functionality.

## Test Coverage

The test suite covers approximately **75-90%** of NFD functionality across multiple test files:

### 1. Basic Features (`features-test.go`) - **Existing**
- CPU feature detection
- Kernel configuration detection
- Pod health monitoring
- Operator installation/upgrade
- Feature whitelisting/blacklisting
- Day-2 worker node addition

### 2. NodeFeatureRule Tests (`nodefeaturerule-test.go`) - **NEW**
- Test ID 70001: matchExpressions operators (In, Exists, Gt, Lt, IsTrue, etc.)
- Test ID 70002: labelsTemplate dynamic label generation
- Test ID 70003: matchAny OR logic
- Test ID 70004: Backreferences (rule.matched)
- Test ID 70005: CRUD lifecycle

### 3. Device Discovery Tests (`device-discovery-test.go`) - **NEW**
- Test ID 70010: PCI device discovery
- Test ID 70011: USB device discovery
- Test ID 70012: SR-IOV capability detection
- Test ID 70013: Storage SSD/HDD features
- Test ID 70014: Network device features
- Test ID 70015: Non-volatile memory (NVDIMM)
- Test ID 70016: System features (OS, kernel)

### 4. Resilience Tests (`resilience-test.go`) - **NEW**
- Test ID 70020: Worker pod restart - labels persist
- Test ID 70021: Master pod restart - rule processing continues
- Test ID 70022: GC cleanup - stale NodeFeature objects removed
- Test ID 70023: Topology updater functionality

### 5. Local Source Tests (`local-source-test.go`) - **NEW**
- Test ID 70030: User-defined features via ConfigMap
- Test ID 70031: Feature files from hostPath

### 6. Extended Resources Tests (`extended-resources-test.go`) - **NEW**
- Test ID 70040: Extended resources from NodeFeatureRule
- Test ID 70041: Node tainting based on features

## Hardware Requirements

| Test ID | Test Name | Required Hardware | Skip if Missing |
|---------|-----------|-------------------|-----------------|
| **Basic Features** ||||
| 54548 | Pod health | None | No |
| 54222 | CPU features | Any CPU | No |
| 54471 | Kernel config | None | No |
| 54549 | Logs check | None | No |
| 54538 | Restart count | None | No |
| 68298 | Blacklist | None | No |
| 68300 | Whitelist | None | No |
| 54539 | Day-2 workers | AWS cluster | Yes (AWS only) |
| **NodeFeatureRule** ||||
| 70001 | matchExpressions | Any CPU with AVX | No (common) |
| 70002 | labelsTemplate | None | No |
| 70003 | matchAny | Any CPU with AVX/AVX2 | No (common) |
| 70004 | Backreferences | Any CPU with SSE4 | No (common) |
| 70005 | CRUD lifecycle | None | No |
| **Device Discovery** ||||
| 70010 | PCI discovery | Any PCI device | No (always present) |
| 70011 | USB discovery | USB devices | Yes |
| 70012 | SR-IOV | SR-IOV capable NIC | Yes |
| 70013 | Storage SSD | SSD/NVME storage | No (common) |
| 70014 | Network devices | Network interface | No (always present) |
| 70015 | NVDIMM | Non-volatile memory | Yes (rare) |
| 70016 | System features | None | No |
| **Resilience** ||||
| 70020 | Worker restart | None | No |
| 70021 | Master restart | None | No |
| 70022 | GC cleanup | None | No |
| 70023 | Topology updater | Topology updater enabled | Yes |
| **Local Source** ||||
| 70030 | ConfigMap features | Local source enabled | Yes (optional) |
| 70031 | hostPath features | hostPath configured | Yes (optional) |
| **Extended Resources** ||||
| 70040 | Extended resources | CPU with AVX | No (common) |
| 70041 | Node taints | CPU with AVX2 | No (common) |

## Running Tests

### Run All Tests
```bash
cd eco-gotests/tests/hw-accel/nfd/features
ginkgo -v ./tests
```

### Run Specific Test Suites

#### NodeFeatureRule Tests
```bash
ginkgo -v -focus="NodeFeatureRule" ./tests
# Or by label:
ginkgo -v -label-filter="custom-rules" ./tests
```

#### Device Discovery Tests
```bash
ginkgo -v -focus="Device Discovery" ./tests
# Or by label:
ginkgo -v -label-filter="device-discovery" ./tests
```

#### Resilience Tests
```bash
ginkgo -v -focus="Resilience" ./tests
# Or by label:
ginkgo -v -label-filter="resilience" ./tests
```

#### Local Source Tests
```bash
ginkgo -v -focus="Local Source" ./tests
# Or by label:
ginkgo -v -label-filter="local-source" ./tests
```

#### Extended Resources Tests
```bash
ginkgo -v -focus="Extended Resources" ./tests
# Or by label:
ginkgo -v -label-filter="extended-resources" ./tests
```

### Run Specific Test by ID
```bash
# Run a specific test by reportxml ID
ginkgo -v -focus="70001" ./tests
```

### Run Tests in Parallel
```bash
ginkgo -v -p ./tests
```

## Configuration

### Environment Variables

The following environment variables can be set to configure the tests:

- `ECO_HWACCEL_NFD_AWS_TESTS`: Set to `true` to run AWS-specific tests (default: `false`)
- `ECO_HWACCEL_NFD_CATALOG_SOURCE`: Catalog source for NFD operator installation
- `ECO_HWACCEL_NFD_IMAGE`: NFD image to use for testing
- `ECO_HWACCEL_NFD_CPU_FLAGS_HELPER_IMAGE`: Helper image for CPU flag detection

### Skip Behavior

Tests implement intelligent skip logic:

- **Always available tests** (CPU, kernel, system): Never skip
- **Commonly available tests** (PCI, network, storage): Warn if missing, may skip
- **Conditionally available tests** (SR-IOV, NUMA): Skip gracefully with message
- **Rare hardware tests** (NVDIMM): Skip if not present (expected behavior)

## Test Architecture

### Helper Functions

The test suite uses helper functions organized by purpose:

- **`internal/get/`**: Functions to retrieve information
  - `nodefeaturerule.go`: Get NodeFeatureRule objects and nodes with labels
  - `getCpuFeature.go`: CPU feature detection
  - `getFeatureLabels.go`: Node label retrieval

- **`internal/set/`**: Functions to create/configure resources
  - `nodefeaturerule.go`: Create/delete NodeFeatureRule objects
  - `featurelist.go`: Configure CPU feature lists
  - `local-source.go`: Manage local source features

- **`internal/wait/`**: Functions to wait for conditions
  - `nodefeaturerule.go`: Wait for rule processing and labels
  - `wait.go`: Generic wait helpers

- **`internal/validation/`**: Functions to validate hardware/features
  - `devices.go`: Check for PCI, USB, SR-IOV, storage devices

### Test Structure

Each test file follows this structure:

```go
var _ = Describe("Test Category", Ordered, Label("label-name"), func() {
    Context("Test Subcategory", func() {
        AfterEach(func() {
            // Cleanup code
        })

        It("Test description", reportxml.ID("XXXXX"), func() {
            By("Step 1")
            // Test code

            By("Step 2")
            // More test code
        })
    })
})
```

## Troubleshooting

### Tests Skipped

If tests are being skipped, check:

1. **Hardware availability**: Some tests require specific hardware (SR-IOV, NVDIMM, etc.)
2. **NFD configuration**: Local source and topology updater must be enabled in NFD CR
3. **Operator installation**: Ensure NFD operator is properly installed

### Labels Not Appearing

If labels are not appearing:

1. Check NFD worker and master pods are running: `kubectl get pods -n openshift-nfd`
2. Check NFD logs: `kubectl logs -n openshift-nfd -l app=nfd-worker`
3. Verify NodeFeatureRule was created: `kubectl get nodefeaturerule -n openshift-nfd`
4. Check node labels: `kubectl get nodes --show-labels | grep feature`

### Timeout Errors

If experiencing timeout errors:

1. Increase timeout values in test code (default: 5 minutes)
2. Check cluster resources and performance
3. Verify network connectivity to image registries
4. Check for pod evictions or resource constraints

## Contributing

When adding new tests:

1. Follow existing patterns in the codebase
2. Add unique reportxml.ID for each test
3. Implement proper cleanup in AfterEach/AfterAll
4. Use Eventually for async validation
5. Add skip logic for hardware dependencies
6. Update this README with new test information
7. Add hardware requirements to the table above

## References

- [NFD Documentation](https://kubernetes-sigs.github.io/node-feature-discovery/)
- [NodeFeatureRule API](https://kubernetes-sigs.github.io/node-feature-discovery/stable/usage/custom-resources.html)
- [Ginkgo v2 Documentation](https://onsi.github.io/ginkgo/)
- [eco-goinfra Documentation](https://github.com/rh-ecosystem-edge/eco-goinfra)
