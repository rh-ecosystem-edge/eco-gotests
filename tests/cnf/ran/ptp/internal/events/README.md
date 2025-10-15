# events Package

The `events` package provides utilities for monitoring and filtering Precision Time Protocol (PTP) related events within a Kubernetes environment. It enables waiting for specific PTP events from pod logs and offers a flexible filtering mechanism to match desired event criteria.

## Core Functionality

### `WaitForEvent`

The primary function, `WaitForEvent`, allows users to asynchronously wait for PTP events. It continuously polls a specified pod's logs for events that match a given set of filters, returning once a match is found or a timeout occurs.

**Signature:**

```go
func WaitForEvent(
    eventPod *pod.Builder,
    startTime time.Time,
    timeout time.Duration,
    filter EventFilter,
    options ...WaitForEventOption) error
```

- `eventPod`: A `*pod.Builder` instance representing the pod from which to retrieve logs.
- `startTime`: The timestamp from which to begin collecting logs for event extraction. Logs preceding this time are ignored.
- `timeout`: The maximum duration to wait for a matching event.
- `filter`: An `EventFilter` implementation that defines the criteria for a matching event.
- `options`: Optional parameters to customize log retrieval and filtering behavior.

### Options

The `WaitForEvent` function accepts several optional parameters:

#### `WithContainer(container string)`

Specifies the container name within the pod from which to retrieve logs. If not specified, logs are retrieved from the default container. This is particularly useful when monitoring PTP events from specific containers like `"cloud-event-proxy"`.

#### `WithoutCurrentState(ignoreCurrentState bool)`

Controls whether to ignore messages about the current state of events. When set to `true`, only events received as subscriptions are considered, filtering out initial state reports. This is useful when you want to wait for new events rather than existing state information.

### Event Filtering

The package introduces two main interfaces for filtering: `EventFilter` and `ValueFilter`. These interfaces allow for highly customizable event matching logic, supporting logical AND/OR operations and specific field comparisons.

#### `EventFilter`

Filters an `event.Event` object based on its properties.

**Combinational Filters:**

- `All(filters ...EventFilter)`: Returns an `EventFilter` that matches an event only if *all* provided `filters` match.
- `Any(filters ...EventFilter)`: Returns an `EventFilter` that matches an event if *any* of the provided `filters` match.

**Specific Event Filters:**

- `IsType(eventType eventptp.EventType)`: Matches events of a specific PTP event type (e.g., `eventptp.PtpSyncState`).

#### `ValueFilter`

Filters an `event.DataValue` object, typically used within an `EventFilter` to inspect event data.

**Usage with `HasValue`:**

- `HasValue(filters ...ValueFilter)`: An `EventFilter` that matches if an event contains at least one `event.DataValue` that satisfies *all* provided `ValueFilter`s.

**Specific Value Filters:**

- `WithSyncState(state eventptp.SyncState)`: Matches `event.DataValue` if its value represents a specific PTP sync state (e.g., `eventptp.PtpSyncStateLocked`).
- `WithMetric(metricValue int64)`: Matches `event.DataValue` if its value is a specific PTP metric.
- `OnNode(nodeName string)`: Matches `event.DataValue` if its resource path is associated with a given node name.
- `OnInterface(nicName iface.NICName)`: Matches `event.DataValue` if its resource path is associated with a given network interface (NIC) name.
- `ContainingResource(subString string)`: A generic filter that matches `event.DataValue` if its resource path contains the specified substring.

## Example Usage

Here's an example demonstrating how to use `WaitForEvent` to check for a PTP event indicating a "locked" sync state on a specific interface:

```go
package main

import (
    "fmt"
    "time"

    "github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
    "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/events"
    "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/iface"
    eventptp "github.com/redhat-cne/sdk-go/pkg/event/ptp"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func main() {
    // Assume 'myPtpPod' is an initialized *pod.Builder pointing to your PTP event source pod.
    // For demonstration, we'll create a dummy one. In a real scenario, this would come from your test setup.
    myPtpPod := pod.NewBuilder("ptp-daemon-pod", "openshift-ptp")

    startTime := time.Now()
    timeout := 2 * time.Minute
    targetInterface := iface.NICName("eth0") // Replace with your actual interface name

    // Define the event filter:
    // We want an event that is of type PtpSyncState
    // AND has a value where the SyncState is PtpSyncStateLocked
    // AND that value is associated with the targetInterface.
    eventFilter := events.All(
        events.IsType(eventptp.PtpSyncState),
        events.HasValue(
            events.WithSyncState(eventptp.PtpSyncStateLocked),
            events.OnInterface(targetInterface),
        ),
    )

    fmt.Printf("Waiting for PTP Sync State Locked event on interface %s...\n", targetInterface)

    // Use both WithContainer and WithoutCurrentState options
    err := events.WaitForEvent(
        myPtpPod,
        startTime,
        timeout,
        eventFilter,
        events.WithContainer("cloud-event-proxy"),
        events.WithoutCurrentState(true), // Only wait for new events, not current state
    )
    if err != nil {
        fmt.Printf("Error waiting for event: %v\n", err)
    } else {
        fmt.Println("Successfully received PTP Sync State Locked event!")
    }
}
