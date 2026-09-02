# Metrics Package

This package provides a robust and type-safe way to query and assert Prometheus metrics related to Precision Time Protocol (PTP) in OpenShift. It abstracts the complexities of PromQL, offering Go-native structs and functions for easier interaction with Prometheus.

## Features

* **Type-Safe Metric Definitions**: Enumerations for PTP metric keys (`PtpMetricKey`), metric names (`PtpMetric`), clock states (`PtpClockState`), process statuses (`PtpProcessStatus`), interface roles (`PtpInterfaceRole`), and threshold types (`PtpThresholdType`) ensure compile-time safety and prevent common PromQL typos.
* **Structured Queries**: The `Query` interface and `MetricQuery` struct allow for building well-defined Prometheus queries with support for instant and range queries. Specific query structs like `ClockStateQuery` and `ProcessStatusQuery` provide tailored interfaces for common PTP metrics.
* **Flexible Label Matching**: `MetricLabel` and its associated helper functions (`Equals`, `DoesNotEqual`, `Matches`, `DoesNotMatch`, `Includes`, `Excludes`) enable precise control over label matching in PromQL queries, supporting exact matches, negative matches, and regex-based filtering.
* **Query Execution**: `ExecuteQuery` and `ExecuteQueryRange` functions simplify the execution of Prometheus queries against a Prometheus API client, handling result parsing and warning logging.
* **Assertion Capabilities**: `AssertQuery`, `AssertQuerySet`, and `AssertThresholds` provide powerful mechanisms to verify metric values over time. These functions support timeouts, polling intervals, and stable duration checks, essential for robust test automation.

### How to Use

#### Constructing a Query

Queries are constructed using the specific query structs that implement the `Query` interface. For example, to query the PTP clock state:

```go
import (
    "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/metrics"
    "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/iface" // Assuming iface package is imported
)

// Create a ClockStateQuery
query := metrics.ClockStateQuery{
    Process:   metrics.Equals(metrics.ProcessPTP4L),
    Interface: metrics.DoesNotEqual(iface.Master),
    Node:      metrics.Equals("worker-0"),
}

// Convert to a generic MetricQuery (optional, for lower-level access)
metricQuery := query.ToMetricQuery()
```

#### Executing a Query

Once a query is constructed, it can be executed using `ExecuteQuery` for instant queries or `ExecuteQueryRange` for range queries.

```go
import (
    "context"
    "fmt"
    "time"

    prometheusapi "github.com/prometheus/client_golang/api/prometheus/v1"
    "github.com/prometheus/common/model"

    "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/metrics"
)

func executeExample(ctx context.Context, client prometheusapi.API) {
    query := metrics.ClockStateQuery{
        Process: metrics.Equals(metrics.ProcessPTP4L),
        Node:    metrics.Equals("worker-0"),
    }

    result, err := metrics.ExecuteQuery(ctx, client, query)
    if err != nil {
        fmt.Printf("Error executing query: %v\n", err)
        return
    }

    for _, sample := range result {
        fmt.Printf("Metric: %s, Value: %f\n", sample.Metric.String(), sample.Value)
    }

    // For a range query
    rangeQuery := metrics.MetricQuery[metrics.PtpClockState]{
        Metric: metrics.MetricClockState,
        Start:  time.Now().Add(-5 * time.Minute),
        End:    time.Now(),
        Step:   15 * time.Second,
        Labels: map[metrics.PtpMetricKey]metrics.MetricLabel[any]{
            metrics.KeyProcess: metrics.Equals(metrics.ProcessPTP4L).ToAny(),
        },
    }

    matrixResult, err := metrics.ExecuteQueryRange(ctx, client, rangeQuery)
    if err != nil {
        fmt.Printf("Error executing range query: %v\n", err)
        return
    }

    for _, stream := range matrixResult {
        fmt.Printf("Stream: %s\n", stream.Metric.String())
        for _, sample := range stream.Values {
            fmt.Printf("  Timestamp: %s, Value: %f\n", sample.Timestamp.String(), sample.Value)
        }
    }
}
```

#### Asserting Query Results

The `AssertQuery` function allows for verifying metric values with various options for polling and stability.

```go
import (
    "context"
    "fmt"
    "time"

    prometheusapi "github.com/prometheus/client_golang/api/prometheus/v1"

    "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/metrics"
)

func assertExample(ctx context.Context, client prometheusapi.API) {
    query := metrics.ClockStateQuery{
        Process: metrics.Equals(metrics.ProcessPTP4L),
        Node:    metrics.Equals("worker-0"),
    }

    // Assert that the clock state is Locked within a 2-minute timeout,
    // checking every 5 seconds, and remains stable for at least 30 seconds.
    err := metrics.AssertQuery(
        ctx,
        client,
        query,
        metrics.ClockStateLocked,
        metrics.AssertWithTimeout(2*time.Minute),
        metrics.AssertWithPollInterval(5*time.Second),
        metrics.AssertWithStableDuration(30*time.Second),
    )

    if err != nil {
        fmt.Printf("Assertion failed: %v\n", err)
        return
    }

    fmt.Println("PTP clock state is locked and stable.")
}
```

#### Asserting Multiple Metrics as a Set

Use `AssertQuerySet` when related metrics must be validated at the same Prometheus query time. Each separate `AssertQuery` call uses its own poll loop and timestamp, so in unstable environments one metric may change before the next is retrieved.

Build expectations with `Expect`, `ExpectLocked`, or `Set` and pass them to `AssertQuerySet`:

```go
func assertLockedMetricsExample(ctx context.Context, client prometheusapi.API, nodeName string) {
    clockStateQuery := metrics.ClockStateQuery{
        Node:    metrics.Equals(nodeName),
        Process: metrics.Equals(metrics.ProcessTBC),
    }
    clockClassQuery := metrics.ClockClassQuery{
        Node:    metrics.Equals(nodeName),
        Process: metrics.Equals(metrics.ProcessPTP4L),
    }

    err := metrics.AssertQuerySet(
        ctx,
        client,
        metrics.Set(
            metrics.ExpectLocked(clockStateQuery),
            metrics.Expect(clockClassQuery, metrics.ClockClass6),
        ),
        metrics.AssertWithTimeout(1*time.Minute),
    )
    if err != nil {
        fmt.Printf("Metric set assertion failed: %v\n", err)
        return
    }

    fmt.Println("Clock state and clock class match at the same snapshot.")
}
```

`AssertQuerySet` accepts the same `QueryAssertOption` values as `AssertQuery` (timeout, poll interval, stable duration, start time). At each poll tick, all expectations are evaluated at the same `queryTime`. If any expectation fails, the tick fails and the stable-duration timer resets.

Use separate `AssertQuery` calls when each assertion waits for a different state transition (for example, waiting for one interface to become FAULTY before another becomes FOLLOWER during failover). Use `AssertQuerySet` when the metrics should be consistent at a single point in time.

For the standard cluster-wide locked baseline (excluding chronyd), use `LockedClockExpectations` or `AssertAllClocksLocked`. `EnsureClocksAreLocked` and `EnsureClocksAreStable` build on these helpers for BeforeEach/AfterEach checks.

To assert PTP clock thresholds, use `AssertThresholds`:

```go
import (
    "context"
    "fmt"

    ptpv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ptp/v1"
    prometheusapi "github.com/prometheus/client_golang/api/prometheus/v1"

    "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/metrics"
)

func assertThresholdsExample(ctx context.Context, client prometheusapi.API) {
    query := metrics.ThresholdQuery{
        Node: metrics.Equals("worker-0"),
    }

    expectedThresholds := map[string]ptpv1.PtpClockThreshold{
        "ocp-ptp-profile-1": {
            MaxOffsetThreshold: 100,
            MinOffsetThreshold: -100,
            HoldOverTimeout:    300,
        },
        "ocp-ptp-profile-2": {
            MaxOffsetThreshold: 50,
            MinOffsetThreshold: -50,
            HoldOverTimeout:    120,
        },
    }

    err := metrics.AssertThresholds(ctx, client, query, expectedThresholds)
    if err != nil {
        fmt.Printf("Threshold assertion failed: %v\n", err)
        return
    }

    fmt.Println("PTP clock thresholds are as expected.")
}
