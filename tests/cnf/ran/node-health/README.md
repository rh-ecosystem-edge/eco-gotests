# Node Health Monitoring Test Suite for CNF/RAN

This test suite provides comprehensive monitoring and validation of node health specifically designed for **CNF (Cloud Native Functions)** and **RAN (Radio Access Network)** deployments in OpenShift/OKD clusters. It follows the eco-gotests framework patterns and integrates seamlessly with the existing CNF/RAN testing infrastructure.

## Overview

The Node Health Monitoring Test Suite validates various aspects of node health that are critical for CNF and RAN workloads:

- **Node Readiness**: Ensures all nodes are in Ready state for CNF workloads
- **Pressure Conditions**: Monitors disk, memory, and network pressure critical for RAN performance
- **Resource Usage**: Tracks CPU, memory, and disk utilization for CNF resource planning
- **Kubelet Status**: Verifies kubelet service health essential for CNF pod scheduling
- **Node Conditions**: Comprehensive validation of all node conditions for RAN stability
- **Resource Monitoring**: Continuous monitoring over time for CNF performance analysis

## Why CNF/RAN Location?

This test suite is strategically placed in the **CNF/RAN area** because:

1. **CNF Workloads**: Cloud Native Functions require robust node health monitoring for optimal performance
2. **RAN Deployments**: Radio Access Network deployments are highly sensitive to node health issues
3. **Infrastructure Testing**: Node health is fundamental infrastructure that CNF/RAN tests depend on
4. **Integration**: Seamlessly integrates with existing CNF/RAN test suites and tooling

## Test Structure

```
tests/cnf/ran/node-health/
├── node_health_suite_test.go          # Main test suite entry point
├── internal/
│   ├── nodehealthparams/
│   │   └── const.go                   # Test parameters and constants
│   └── nodehealthinittools/
│       └── nodehealthinittools.go     # API client initialization
├── tests/
│   └── node_health_validation.go      # Main test implementations
└── README.md                          # This documentation
```

## Test Categories

### 1. Node Readiness Validation
- **Test ID**: `node-health-001`
- **Purpose**: Verify all nodes are in Ready state for CNF workloads
- **Labels**: `node-readiness-check`
- **Description**: Checks that each node has `NodeReady` condition set to `True`

### 2. Node Pressure Validation
- **Test ID**: `node-health-003` to `node-health-005`
- **Purpose**: Monitor node pressure conditions critical for RAN performance
- **Labels**: `disk-pressure-check`, `memory-pressure-check`, `network-pressure-check`
- **Description**: Validates that nodes are not under disk, memory, or network pressure

### 3. Node Resource Usage Validation
- **Test ID**: `node-health-006` to `node-health-008`
- **Purpose**: Monitor resource utilization for CNF resource planning
- **Labels**: `disk-usage-check`, `memory-usage-check`, `cpu-usage-check`
- **Description**: Verifies resource capacity and allocatable values are within acceptable limits

### 4. Kubelet Status Validation
- **Test ID**: `node-health-009` to `node-health-010`
- **Purpose**: Ensure kubelet service health essential for CNF pod scheduling
- **Labels**: `kubelet-pod-status`, `kubelet-service-check`
- **Description**: Verifies kubelet pods are running and ready on all nodes

### 5. Node Conditions Validation
- **Test ID**: `node-health-011` to `node-health-012`
- **Purpose**: Comprehensive condition monitoring for RAN stability
- **Labels**: `node-conditions-check`, `node-transition-time-check`
- **Description**: Validates all node conditions and transition times

### 6. Resource Monitoring
- **Test ID**: `node-health-013`
- **Purpose**: Continuous monitoring over time for CNF performance analysis
- **Labels**: `resource-monitoring`
- **Description**: Monitors node health continuously for a configurable duration

## Configuration

### Thresholds
The test suite uses configurable thresholds for resource monitoring:

```go
const (
    DefaultDiskPressureThreshold = 85.0      // 85% disk pressure threshold
    DefaultMemoryPressureThreshold = 85.0    // 85% memory pressure threshold
    DefaultDiskUsageThreshold = 80.0         // 80% disk usage threshold
    DefaultMemoryUsageThreshold = 80.0       // 80% memory usage threshold
)
```

### Timeouts
```go
const (
    KubeletHealthCheckTimeout = 30           // 30 seconds for kubelet health checks
    NodeConditionCheckTimeout = 60           // 60 seconds for node condition checks
    ResourceCheckInterval = 10               // 10 seconds between resource checks
)
```

## Running the Tests

### Run All Node Health Tests
```bash
go test ./tests/cnf/ran/node-health/ -v
```

### Run Specific Test Categories
```bash
# Run only readiness tests
go test ./tests/cnf/ran/node-health/ -v -ginkgo.label-filter="node-readiness"

# Run only pressure tests
go test ./tests/cnf/ran/node-health/ -v -ginkgo.label-filter="node-pressure"

# Run only resource tests
go test ./tests/cnf/ran/node-health/ -v -ginkgo.label-filter="node-resources"
```

### Run with Specific Labels
```bash
# Run tests with specific labels
go test ./tests/cnf/ran/node-health/ -v -ginkgo.label-filter="node-health-readiness"
go test ./tests/cnf/ran/node-health/ -v -ginkgo.label-filter="node-health-pressure"
go test ./tests/cnf/ran/node-health/ -v -ginkgo.label-filter="node-health-kubelet"
```

## Integration with CNF/RAN Tests

This test suite integrates seamlessly with the existing CNF/RAN testing infrastructure:

- **CNF Workloads**: Can be run before/after CNF deployment tests to ensure node health
- **RAN Performance**: Monitors node health during RAN performance testing
- **Infrastructure**: Provides baseline node health validation for other CNF/RAN tests
- **Reporting**: Uses the standard CNF/RAN reporting infrastructure

## Test Output

The test suite provides detailed logging for each check:

```
INFO: Found 3 nodes for health monitoring
INFO: Checking node: worker-0
INFO: Node worker-0 is Ready
INFO: Checking node: worker-0 for disk pressure
INFO: Node worker-0 has no disk pressure
INFO: Checking node: worker-0 memory usage
INFO: Node worker-0 memory: Capacity=16Gi, Allocatable=15Gi
```

## Integration with eco-gotests

This test suite integrates seamlessly with the existing eco-gotests framework:

- **Reporting**: Uses the standard reporting infrastructure
- **Labels**: Follows the established labeling conventions
- **Parameters**: Uses the parameter management system
- **Error Handling**: Integrates with the failure reporting system

## Dependencies

- **eco-goinfra**: For Kubernetes resource management
- **Ginkgo v2**: For BDD-style testing
- **Gomega**: For assertions and matchers
- **Kubernetes client-go**: For API interactions

## Customization

### Adding New Health Checks
To add new health checks, extend the test file with new `It` blocks:

```go
It("Verify custom health check",
    Label("custom-health-check"),
    reportxml.ID("node-health-014"),
    func() {
        // Your custom health check logic here
    })
```

### Modifying Thresholds
Update the constants in `internal/nodehealthparams/const.go`:

```go
const (
    CustomThreshold = 90.0  // Your custom threshold
)
```

### Adding New Resource Types
Extend the resource validation tests to include new resource types:

```go
// Add to the resource validation context
It("Verify custom resource usage",
    Label("custom-resource-check"),
    func() {
        // Custom resource validation logic
    })
```

## Troubleshooting

### Common Issues

1. **Permission Denied**: Ensure the test runner has sufficient RBAC permissions
2. **Node Not Found**: Verify the cluster has nodes and they are accessible
3. **Kubelet Pod Not Running**: Check if kubelet is properly deployed

### Debug Mode
Enable verbose logging by setting the log level:

```bash
export GLOMAXLEVEL=5
go test ./tests/cnf/ran/node-health/ -v
```

## Contributing

When contributing to this test suite:

1. Follow the existing CNF/RAN naming conventions
2. Add appropriate labels and test IDs
3. Include comprehensive error messages
4. Add documentation for new features
5. Ensure tests are idempotent and can run multiple times

## License

This test suite is part of the eco-gotests project and follows the same licensing terms.
