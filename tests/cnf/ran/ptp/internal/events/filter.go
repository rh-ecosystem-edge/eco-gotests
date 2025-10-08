package events

import (
	"fmt"
	"math"
	"strings"

	"github.com/redhat-cne/sdk-go/pkg/event"
	eventptp "github.com/redhat-cne/sdk-go/pkg/event/ptp"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/iface"
)

// EventFilter is an interface that defines a filter for events. It has a single method, Filter, which takes an event
// and returns a boolean indicating whether the event matches the filter. Filters are expected to be stateless and
// callable concurrently.
//
// Implementors of this interface should provide a constructor function that returns an EventFilter instance and reads
// like a sentence starting with "event".
type EventFilter interface {
	Filter(event.Event) bool
}

// eventFilterAny is a filter that matches if any of the provided filters match the event. It composes multiple filters
// using a logical OR.
type eventFilterAny []EventFilter

// Assert at compile time that eventFilterAny implements EventFilter.
var _ EventFilter = eventFilterAny{}

// Any returns a filter that matches if any of the provided filters match the event. This is equivalent to a logical OR
// operation on the filters.
func Any(f ...EventFilter) EventFilter {
	return eventFilterAny(f)
}

// Filter implements the EventFilter interface. It checks if any of the provided filters match the event. If any filter
// matches, it returns true; otherwise, it returns false. This means that Any of an empty filter list will return false.
func (f eventFilterAny) Filter(e event.Event) bool {
	for _, filter := range f {
		if filter.Filter(e) {
			return true
		}
	}

	return false
}

// eventFilterAll is a filter that matches if all of the provided filters match the event. It composes multiple filters
// using a logical AND.
type eventFilterAll []EventFilter

// Assert at compile time that eventFilterAll implements EventFilter.
var _ EventFilter = eventFilterAll{}

// All returns a filter that matches if all of the provided filters match the event. This is equivalent to a logical AND
// operation on the filters.
func All(f ...EventFilter) EventFilter {
	return eventFilterAll(f)
}

// Filter implements the EventFilter interface. It checks if all of the provided filters match the event. If all filters
// match, it returns true; otherwise, it returns false. This means that All of an empty filter list will return true.
func (f eventFilterAll) Filter(e event.Event) bool {
	for _, filter := range f {
		if !filter.Filter(e) {
			return false
		}
	}

	return true
}

// eventFilterIsType is a filter that matches if the event is of the specified type.
type eventFilterIsType eventptp.EventType

// Assert at compile time that eventFilterIsType implements EventFilter.
var _ EventFilter = eventFilterIsType("")

// IsType returns a filter that matches if the event is of the specified type.
func IsType(t eventptp.EventType) EventFilter {
	return eventFilterIsType(t)
}

// Filter implements the EventFilter interface. It checks if the event type matches the specified type.
func (f eventFilterIsType) Filter(e event.Event) bool {
	return e.Type == string(f)
}

// eventFilterHasValue is a filter that matches if the event has a value that matches all of the provided filters. It
// composes multiple value filters using a logical AND.
type eventFilterHasValue []ValueFilter

// Assert at compile time that eventFilterHasValue implements EventFilter.
var _ EventFilter = eventFilterHasValue{}

// HasValue returns a filter that matches if the event has a value that matches all of the provided filters. This is
// equivalent to a logical AND operation on the filters.
func HasValue(f ...ValueFilter) EventFilter {
	return eventFilterHasValue(f)
}

// Filter implements the EventFilter interface. It returns true if the event has a value that matches all of the
// provided value filters. All value filters must be matched in a single value for the event to be considered a match.
func (f eventFilterHasValue) Filter(e event.Event) bool {
	if e.Data == nil {
		return false
	}

	for _, value := range e.Data.Values {
		valueMatched := true

		for _, filter := range f {
			if !filter.Filter(value) {
				valueMatched = false

				break
			}
		}

		if valueMatched {
			return true
		}
	}

	return false
}

// ValueFilter is an interface that defines a filter for event values. It has a single method, Filter, which takes an
// event value and returns a boolean indicating whether the value matches the filter. Value filters are expected to be
// stateless and callable concurrently.
//
// Implementors of this interface should provide a constructor function that returns a ValueFilter instance and reads
// like a sentence inside the [HasValue] function.
type ValueFilter interface {
	Filter(event.DataValue) bool
}

// valueFilterWithSyncState is a filter that matches if the event has the specified sync state.
type valueFilterWithSyncState eventptp.SyncState

// Assert at compile time that valueFilterWithSyncState implements ValueFilter.
var _ ValueFilter = valueFilterWithSyncState("")

// WithSyncState returns a filter that matches if the event has the specified sync state.
func WithSyncState(s eventptp.SyncState) ValueFilter {
	return valueFilterWithSyncState(s)
}

// Filter implements the ValueFilter interface. It checks if the event value type is an enumeration and if the value
// matches the specified sync state after converting both to strings.
func (f valueFilterWithSyncState) Filter(value event.DataValue) bool {
	if value.ValueType != event.ENUMERATION {
		return false
	}

	valueString, ok := value.Value.(string)
	if !ok {
		return false
	}

	return valueString == string(f)
}

// valueFilterWithMetric is a filter that matches if the event has the specified metric value.
type valueFilterWithMetric int64

// Assert at compile time that valueFilterWithMetric implements ValueFilter.
var _ ValueFilter = valueFilterWithMetric(0)

// WithMetric returns a filter that matches if the event has the specified metric value.
func WithMetric(m int64) ValueFilter {
	return valueFilterWithMetric(m)
}

// Filter implements the ValueFilter interface. It checks if the event value type is a decimal and if the value data
// type is metric before comparing the value to the specified metric value. The value is rounded to the nearest integer
// before comparing to the expected metric value.
func (f valueFilterWithMetric) Filter(value event.DataValue) bool {
	if value.ValueType != event.DECIMAL || value.DataType != event.METRIC {
		return false
	}

	valueFloat, ok := value.Value.(float64)
	if !ok {
		return false
	}

	// Since all the PTP metrics are integers, we do the comparison as integers. To be extra cautious, we round the
	// float value to the nearest integer before comparing. This prevents a case such as 1.99 from being considered
	// not equal to 2 since truncating the float value would result in 1.
	return int64(math.Round(valueFloat)) == int64(f)
}

// valueFilterOnNode is a filter that matches if the event is on the specified node.
type valueFilterOnNode string

// Assert at compile time that valueFilterOnNode implements ValueFilter.
var _ ValueFilter = valueFilterOnNode("")

// OnNode returns a filter that matches if the event is on the specified node. Currently, this is redundant since events
// are collected separately for each node.
func OnNode(nodeName string) ValueFilter {
	return valueFilterOnNode(nodeName)
}

// Filter implements the ValueFilter interface. It checks if the event value resource starts with the specified node
// name. The resource is expected to be in the format "/cluster/node/<node-name>/...".
func (f valueFilterOnNode) Filter(value event.DataValue) bool {
	return strings.HasPrefix(value.Resource, fmt.Sprintf("/cluster/node/%s", string(f)))
}

// valueFilterOnInterface is a filter that matches if the event is on the specified interface.
type valueFilterOnInterface iface.NICName

// Assert at compile time that valueFilterOnInterface implements ValueFilter.
var _ ValueFilter = valueFilterOnInterface("")

// OnInterface returns a filter that matches if the event is on the specified interface. Since events are aggregated by
// NIC name rather than interface name, the function accepts a NIC name and converts it to one if it is actually an
// interface name.
func OnInterface(nicName iface.NICName) ValueFilter {
	return valueFilterOnInterface(nicName.EnsureNIC())
}

// Filter implements the ValueFilter interface. It checks if the event value resource has at least 5 fields (since the
// leading slash results in an empty first field) and if the fifth field matches the specified interface name. The
// resource is expected to be in the format "/cluster/node/<node-name>/<interface-name>/...".
func (f valueFilterOnInterface) Filter(value event.DataValue) bool {
	resourceFields := strings.Split(value.Resource, "/")
	if len(resourceFields) < 5 {
		return false
	}

	return resourceFields[4] == string(f)
}

// valueFilterResourceContains is a filter that matches if the event resource contains the specified string.
type valueFilterResourceContains string

// Assert at compile time that valueFilterResourceContains implements ValueFilter.
var _ ValueFilter = valueFilterResourceContains("")

// ContainingResource returns a filter that matches if the event resource contains the specified string. It is like a
// more generic version of [OnInterface] or [OnNode] that can be used to match any part of the resource string.
func ContainingResource(s string) ValueFilter {
	return valueFilterResourceContains(s)
}

// Filter implements the ValueFilter interface. It checks if the event value resource contains the specified string. It
// makes no assumptions about the format of the resource string, so it can be used to match any part of the resource.
func (f valueFilterResourceContains) Filter(value event.DataValue) bool {
	return strings.Contains(value.Resource, string(f))
}
