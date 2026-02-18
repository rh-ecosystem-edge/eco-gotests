# NFD Testing Skill

Help with Node Feature Discovery (NFD) testing tasks including test execution, test plan management, and debugging.

## Capabilities

1. **Run NFD Tests**
   - Execute full test suite or specific tests
   - Handle test cleanup and troubleshooting
   - Monitor test progress

2. **Test Plan Management**
   - Map tests to official OCP- IDs
   - Identify missing tests from test plan
   - Help create new tests aligned with test plan

3. **Debugging**
   - Diagnose stuck tests
   - Analyze test failures
   - Cluster state verification

## Official Test IDs (from Test Plan)

### Functional Tests
- **OCP-68300**: Edit CPU Whitelist
- **OCP-68298**: Edit CPU Blacklist
- **OCP-54222**: Test discovery of standard node features (CPU, kernel, OS)
- **OCP-54471**: Kernel config tests
- **OCP-54408**: NUMA tests (topology detection)
- **OCP-54412**: PCI device labeling tests
- **OCP-54461**: USB device labeling tests
- **OCP-54462**: SRIOV networking tests
- **OCP-54464**: RDMA tests
- **OCP-54472**: Node Feature Rules (custom labels)
- **OCP-54474**: Custom Features from operand config
- **OCP-54475**: Local files integration
- **OCP-54477**: Local shell scripts integration
- **OCP-54487**: Local custom config from files
- **OCP-54489**: Label templating
- **OCP-54491**: NodeResourceTopology
- **OCP-54492**: Vars (variables in rules)
- **OCP-54493**: Backreferences (rules referencing other rules)
- **OCP-54550**: Metrics testing

### Day2 and Stability Tests
- **OCP-54536**: Stability tests (no restarts, no errors)
- **OCP-54548**: NFD pods verification
- **OCP-54538**: Restart tests (node and cluster restarts)
- **OCP-54539**: Add day2 workers
- **OCP-54540**: Cluster upgrade tests
- **OCP-54549**: Log growth monitoring

## Current Test Suite Status

### Implemented Tests (with Official IDs)
```
✅ OCP-54222 - Check CPU feature labels (test file: features-test.go)
✅ OCP-54471 - Check Kernel config (test file: features-test.go)
✅ OCP-54549 - Check Logs (test file: features-test.go)
✅ OCP-54538 - Check Restart Count (test file: features-test.go)
✅ OCP-68298 - Blacklist CPU features (test file: features-test.go)
✅ OCP-68300 - Whitelist CPU features (test file: features-test.go)
✅ OCP-54539 - Add day2 workers (test file: features-test.go)
✅ OCP-54408 - NUMA/Topology tests (test file: features-test.go)
✅ OCP-54491 - NodeResourceTopology (test file: features-test.go)
```

### Implemented Tests (NO Official ID - Custom Tests)
```
⚠️  matchExpressions operators test (test file: nodefeaturerule-test.go:33)
⚠️  labelsTemplate dynamic label generation (test file: nodefeaturerule-test.go:115)
⚠️  matchAny OR logic (test file: nodefeaturerule-test.go:185)
⚠️  Backreferences test (test file: nodefeaturerule-test.go:264) - Could map to OCP-54493
⚠️  CRUD lifecycle (test file: nodefeaturerule-test.go:370)
⚠️  Extended resources test (test file: extended-resources-test.go:24)
⚠️  Node tainting test (test file: extended-resources-test.go:131)
⚠️  Pod restart resilience tests (test file: resilience-test.go)
⚠️  Device discovery tests (test file: device-discovery-test.go)
```

### Missing Tests (from Official Test Plan)
```
❌ OCP-54412 - PCI device labeling
❌ OCP-54461 - USB device labeling
❌ OCP-54462 - SRIOV networking
❌ OCP-54464 - RDMA
❌ OCP-54472 - Node Feature Rules (partially covered by custom tests)
❌ OCP-54474 - Custom Features from operand config
❌ OCP-54475 - Local files integration
❌ OCP-54477 - Local shell scripts
❌ OCP-54487 - Local custom config files
❌ OCP-54489 - Label templating (may be covered by custom labelsTemplate test)
❌ OCP-54492 - Vars testing
❌ OCP-54536 - Stability tests
❌ OCP-54548 - NFD pods verification
❌ OCP-54540 - Cluster upgrade tests
❌ OCP-54550 - Metrics testing
```

## Test Execution

### Run All Tests
```bash
ginkgo -v .
```

### Run Specific Test by OCP ID
```bash
# For tests with official IDs
ginkgo -v . --focus="54222"  # CPU features
ginkgo -v . --focus="68298"  # Blacklist
ginkgo -v . --focus="68300"  # Whitelist
```

### Run Tests with Cleanup
```bash
bash /tmp/force_cleanup_nfd.sh
ginkgo -v .
```

### Skip Known Failures
```bash
# Skip backreferences and taints (deployment limitations)
ginkgo -v . --focus="54222|54471|54549|54538"
```

## Common Issues

### Test Stuck at "Installing NFD operator"
**Symptoms**: BeforeSuite hangs before creating namespace
**Fix**:
```bash
# Kill stuck process
pkill -f "features.test"

# Clean up
bash /tmp/force_cleanup_nfd.sh

# Retry
ginkgo -v .
```

### Namespace Stuck in Terminating
**Fix**:
```bash
bash /tmp/force_cleanup_nfd.sh
```

### Tests Skip Due to Missing Config
**Fix**:
```bash
export ECO_HWACCEL_NFD_CATALOG_SOURCE="redhat-operators"
export ECO_HWACCEL_NFD_CPU_FLAGS_HELPER_IMAGE="registry.redhat.io/openshift4/ose-node-feature-discovery:v4.12"
ginkgo -v .
```

## Test Plan Alignment

### Priority 1: Map Existing Tests to Official IDs
Need to update test files to use official OCP- IDs:
1. Backreferences test → OCP-54493
2. Custom NodeFeatureRule tests → OCP-54472
3. Templating test → OCP-54489

### Priority 2: Add Missing Critical Tests
1. OCP-54550 - Metrics (monitoring)
2. OCP-54536 - Stability (production readiness)
3. OCP-54548 - Pod verification (deployment validation)

### Priority 3: Add Hardware-Specific Tests
1. OCP-54412 - PCI devices
2. OCP-54462 - SRIOV
3. OCP-54464 - RDMA
4. OCP-54461 - USB devices

## NFD Installation (per Test Plan)

### From CLI (Section 3.2)
1. Create namespace: `openshift-nfd`
2. Create Subscription to `redhat-operators` catalog
3. Create NodeFeatureDiscovery CR instance

### Verification
```bash
# Check namespace
kubectl get namespace openshift-nfd

# Check operator
kubectl get csv -n openshift-nfd | grep nfd

# Check NFD CR
kubectl get nodefeaturediscovery -n openshift-nfd

# Check pods (3 masters + 3 workers for OCP < 4.12, 1 master + N workers for OCP >= 4.12)
kubectl get pods -n openshift-nfd
```

## Test Matrix (per Test Plan)

### CPU Architectures
- x86_64 ✅ (current cluster)
- aarch64 ❌ (needs separate cluster)

### Hardware
- Virtualized ✅ (current cluster type)
- Bare metal ❌ (needs bare metal cluster for NUMA, SRIOV, etc.)
- AWS ✅ (current cluster)
- GCE, vSphere, Azure ❌ (needs separate clusters)

### Cluster Types
- Multi-node ✅ (current cluster)
- SNO ❌ (needs separate cluster)

## References

- NFD Standard Features: https://kubernetes-sigs.github.io/node-feature-discovery/v0.11/get-started/features.html
- OpenShift Docs: https://docs.openshift.com/container-platform/4.11/hardware_enablement/psap-node-feature-discovery-operator.html
- Test Plan Document: (provided by user)

## Notes

- **IMPORTANT**: Only use official OCP- IDs from the test plan
- For new tests without official IDs, leave the ID field empty
- Tests with temporary IDs (70001, 70002, etc.) need to be mapped to official IDs or left empty
- Always reference the official test plan for test requirements and expected results
