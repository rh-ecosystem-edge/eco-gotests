package metrics

import (
	"fmt"
	"strings"
	"time"

	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/iface"
	"golang.org/x/exp/constraints"
)

// metricOperator is an enum representing the possible operators for labels in PromQL queries. It should only be used by
// MetricLabel internally.
type metricOperator string

const (
	metricOperatorEquals       metricOperator = "="
	metricOperatorDoesNotEqual metricOperator = "!="
	metricOperatorMatches      metricOperator = "=~"
	metricOperatorDoesNotMatch metricOperator = "!~"
)

// MetricLabel represents the value of a label in a PromQL query. It should not be constructed directly and the zero
// value should be ignored. No escaping is done so double quotes in values should already be escaped.
//
// Type parameter T is used to strongly type the value of the label, particularly for using with enums.
type MetricLabel[T any] struct {
	value    string
	operator metricOperator
}

// IsZero returns true if the label is considered zero and should be ignored.
func (label MetricLabel[T]) IsZero() bool {
	// Since both the operator and value must be set in a valid MetricLabel, one being empty is enough to consider
	// the MetricLabel zero. However, this logic is not exposed in the API since MetricLabels should be opaque.
	return label.operator == "" || label.value == ""
}

// String returns the label as a string. This is the value with the operator (=, !=, =~, !~) included.
func (label MetricLabel[T]) String() string {
	return fmt.Sprintf("%s\"%s\"", label.operator, label.value)
}

// ToAny converts the label to a label with any type. This is essentially type erasure and allows for forcing the use of
// the Equals, DoesNotEqual, Matches and DoesNotMatch functions with a specific type but then storing the labels values
// in a collection as the same type.
func (label MetricLabel[T]) ToAny() MetricLabel[any] {
	return MetricLabel[any](label)
}

// ensureNIC treats T as [iface.NICName] and ensures that the value is a NIC name. Since Go does not allow generic
// specialization, this method cannot be guaranteed to be type safe at compile time. It is the caller's responsibility
// to ensure that the type of T is actually [iface.NICName].
func (label MetricLabel[T]) ensureNIC() MetricLabel[iface.NICName] {
	label.value = string(iface.NICName(label.value).EnsureNIC())

	return MetricLabel[iface.NICName](label)
}

// Equals returns a MetricLabel with the value and the = operator. It is used to match the value exactly. Callers are
// recommended to elide the type parameter.
func Equals[T any](value T) MetricLabel[T] {
	return MetricLabel[T]{value: fmt.Sprint(value), operator: metricOperatorEquals}
}

// DoesNotEqual returns a MetricLabel with the value and the != operator. It is used to match values that are exactly
// not equal to the value. Callers are recommended to elide the type parameter.
func DoesNotEqual[T any](value T) MetricLabel[T] {
	return MetricLabel[T]{value: fmt.Sprint(value), operator: metricOperatorDoesNotEqual}
}

// Matches returns a MetricLabel with the value and the =~ operator. It is used to match values that are regular
// expression matches of the value. Callers are recommended to elide the type parameter.
func Matches[T any](value T) MetricLabel[T] {
	return MetricLabel[T]{value: fmt.Sprint(value), operator: metricOperatorMatches}
}

// DoesNotMatch returns a MetricLabel with the value and the !~ operator. It is used to match values that are not
// regular expression matches of the value. Callers are recommended to elide the type parameter.
func DoesNotMatch[T any](value T) MetricLabel[T] {
	return MetricLabel[T]{value: fmt.Sprint(value), operator: metricOperatorDoesNotMatch}
}

// Includes returns a MetricLabel that includes values regex matching any of the provided values. Callers are
// recommended to elide the type parameter.
//
// If only one value is provided, this is equivalent to [Matches].
func Includes[T any](values ...T) MetricLabel[T] {
	if len(values) == 0 {
		return MetricLabel[T]{}
	}

	if len(values) == 1 {
		return Matches(values[0])
	}

	var stringBuilder strings.Builder

	stringBuilder.WriteString("(")

	for i, value := range values {
		if i > 0 {
			stringBuilder.WriteString("|")
		}

		stringBuilder.WriteString(fmt.Sprintf("%v", value))
	}

	stringBuilder.WriteString(")")

	return MetricLabel[T]{value: stringBuilder.String(), operator: metricOperatorMatches}
}

// Excludes returns a MetricLabel that excludes values regex matching any of the provided values. Callers are
// recommended to elide the type parameter.
//
// If only one value is provided, this is equivalent to [DoesNotMatch].
func Excludes[T any](values ...T) MetricLabel[T] {
	if len(values) == 0 {
		return MetricLabel[T]{}
	}

	if len(values) == 1 {
		return DoesNotMatch(values[0])
	}

	var stringBuilder strings.Builder

	stringBuilder.WriteString("(")

	for i, value := range values {
		if i > 0 {
			stringBuilder.WriteString("|")
		}

		stringBuilder.WriteString(fmt.Sprintf("%v", value))
	}

	stringBuilder.WriteString(")")

	return MetricLabel[T]{value: stringBuilder.String(), operator: metricOperatorDoesNotMatch}
}

// Query is an interface that represents any query that can be converted to a MetricQuery. This allows for more specific
// validation for queries on different metrics while providing a common way to execute them.
//
// The type parameter V refers to the type of the value returned by the query. It must be an integer type since all PTP
// metrics are integers.
type Query[V constraints.Integer] interface {
	ToMetricQuery() MetricQuery[V]
}

// MetricQuery is a query for a specific metric. It contains the name of the metric, the range parameters (optional),
// and the metric labels.
//
// Like with [Query], the type parameter V refers to the type of the value returned by the query.
type MetricQuery[V constraints.Integer] struct {
	// Start is the start time of the query for range queries. For instant queries, this value is ignored.
	Start time.Time
	// End is the end time of the query for range queries. For instant queries, this value is used as the query time
	// if non-zero.
	End time.Time
	// Step is the step time of the query for range queries. For instant queries, this value is ignored.
	Step time.Duration
	// Metric is the name of the metric to query. It is restricted to the available PTP metrics.
	Metric PtpMetric
	// Labels is a map of labels to query for. Although individual queries may have different types for labels, they
	// undergo type erasure when stored in the final MetricQuery. Since the types are purely for static validation
	// and do not affect the generated PromQL, the fact that they are all stored as MetricLabel[any] does not change
	// how they work.
	Labels map[PtpMetricKey]MetricLabel[any]
}

// This asserts at compile time that MetricQuery implements the Query interface.
var _ Query[int64] = MetricQuery[int64]{}

// String returns MetricQuery as PromQL query string.
func (query MetricQuery[V]) String() string {
	var stringBuilder strings.Builder

	stringBuilder.WriteString(string(query.Metric))
	stringBuilder.WriteString("{")

	for key, value := range query.Labels {
		// Since the queries work by setting all the possible labels but leaving some as the zero value, we need
		// to skip any labels that are the zero value.
		if value.IsZero() {
			continue
		}

		stringBuilder.WriteString(string(key))
		// When the MetricLabel is converted to a string, it already includes the operator and quotes.
		stringBuilder.WriteString(value.String())
		// Trailing commas are allowed in PromQL, so we don't need to check if this is the last label.
		stringBuilder.WriteString(",")
	}

	stringBuilder.WriteString("}")

	return stringBuilder.String()
}

// Range returns the range parameters for the query. It does not validate the values, so the caller should ensure that
// they are set correctly when executing range queries.
func (query MetricQuery[V]) Range() prometheusv1.Range {
	return prometheusv1.Range{
		Start: query.Start,
		End:   query.End,
		Step:  query.Step,
	}
}

// ToMetricQuery fulfills the Query interface and is equivalent to shallow copying the MetricQuery struct. It is
// provided for completeness and allows lower-level access to queries and supporting queries that are not statically
// typed.
func (query MetricQuery[V]) ToMetricQuery() MetricQuery[V] {
	return query
}

// ClockStateQuery is a query for the openshift_ptp_clock_state metric. It has the special case that the interface label
// is converted to ending in x and will default to ignoring the master interface if not set.
type ClockStateQuery struct {
	Process   MetricLabel[PtpProcess]
	Interface MetricLabel[iface.NICName]
	Node      MetricLabel[string]
}

// This asserts at compile time that ClockStateQuery implements the Query interface.
var _ Query[PtpClockState] = ClockStateQuery{}

// ToMetricQuery converts the ClockStateQuery to a MetricQuery. If the interface is not set, it will default to
// ignoring the master interface.
func (query ClockStateQuery) ToMetricQuery() MetricQuery[PtpClockState] {
	ifaceLabel := query.Interface
	if query.Interface.IsZero() {
		ifaceLabel = DoesNotEqual(iface.Master)
	}

	ifaceLabel = ifaceLabel.ensureNIC()

	return MetricQuery[PtpClockState]{
		Metric: MetricClockState,
		Labels: map[PtpMetricKey]MetricLabel[any]{
			KeyProcess:   query.Process.ToAny(),
			KeyInterface: ifaceLabel.ToAny(),
			KeyNode:      query.Node.ToAny(),
		},
	}
}

// ProcessStatusQuery is a query for the openshift_ptp_process_status metric.
type ProcessStatusQuery struct {
	Process MetricLabel[PtpProcess]
	Node    MetricLabel[string]
	Config  MetricLabel[string]
}

// This asserts at compile time that ProcessStatusQuery implements the Query interface.
var _ Query[PtpProcessStatus] = ProcessStatusQuery{}

// ToMetricQuery converts the ProcessStatusQuery to a MetricQuery to fulfill the Query interface.
func (query ProcessStatusQuery) ToMetricQuery() MetricQuery[PtpProcessStatus] {
	return MetricQuery[PtpProcessStatus]{
		Metric: MetricProcessStatus,
		Labels: map[PtpMetricKey]MetricLabel[any]{
			KeyProcess: query.Process.ToAny(),
			KeyNode:    query.Node.ToAny(),
			KeyConfig:  query.Config.ToAny(),
		},
	}
}

// InterfaceRoleQuery is a query for the openshift_ptp_interface_role metric. Unlike other queries, this query does not
// aggregate interfaces by NIC and instead uses the interface name directly.
type InterfaceRoleQuery struct {
	Interface MetricLabel[iface.Name]
	Node      MetricLabel[string]
	Process   MetricLabel[PtpProcess]
}

// This asserts at compile time that InterfaceRoleQuery implements the Query interface.
var _ Query[PtpInterfaceRole] = InterfaceRoleQuery{}

// ToMetricQuery converts the InterfaceRoleQuery to a MetricQuery to fulfill the Query interface.
func (query InterfaceRoleQuery) ToMetricQuery() MetricQuery[PtpInterfaceRole] {
	return MetricQuery[PtpInterfaceRole]{
		Metric: MetricInterfaceRole,
		Labels: map[PtpMetricKey]MetricLabel[any]{
			KeyInterface: query.Interface.ToAny(),
			KeyNode:      query.Node.ToAny(),
			KeyProcess:   query.Process.ToAny(),
		},
	}
}

// ThresholdQuery is a query for the openshift_ptp_threshold metric.
type ThresholdQuery struct {
	Node          MetricLabel[string]
	Profile       MetricLabel[string]
	ThresholdType MetricLabel[PtpThresholdType]
}

// This asserts at compile time that ThresholdQuery implements the Query interface.
var _ Query[int64] = ThresholdQuery{}

// ToMetricQuery converts the ThresholdQuery to a MetricQuery to fulfill the Query interface.
func (query ThresholdQuery) ToMetricQuery() MetricQuery[int64] {
	return MetricQuery[int64]{
		Metric: MetricThreshold,
		Labels: map[PtpMetricKey]MetricLabel[any]{
			KeyNode:      query.Node.ToAny(),
			KeyProfile:   query.Profile.ToAny(),
			KeyThreshold: query.ThresholdType.ToAny(),
		},
	}
}

// NMEAStatusQuery is a query for the openshift_ptp_nmea_status metric.
type NMEAStatusQuery struct {
	Interface MetricLabel[iface.NICName]
	Node      MetricLabel[string]
	Process   MetricLabel[PtpProcess]
}

// This asserts at compile time that NMEAStatusQuery implements the Query interface.
var _ Query[PtpNMEAStatus] = NMEAStatusQuery{}

// ToMetricQuery converts the NMEAStatusQuery to a MetricQuery to fulfill the Query interface.
func (query NMEAStatusQuery) ToMetricQuery() MetricQuery[PtpNMEAStatus] {
	return MetricQuery[PtpNMEAStatus]{
		Metric: MetricNMEAStatus,
		Labels: map[PtpMetricKey]MetricLabel[any]{
			KeyInterface: query.Interface.ensureNIC().ToAny(),
			KeyNode:      query.Node.ToAny(),
			KeyProcess:   query.Process.ToAny(),
		},
	}
}

// HAProfileStatusQuery is a query for the openshift_ptp_ha_profile_status metric.
type HAProfileStatusQuery struct {
	Node    MetricLabel[string]
	Process MetricLabel[PtpProcess]
	Profile MetricLabel[string]
}

// This asserts at compile time that HAProfileStatusQuery implements the Query interface.
var _ Query[PtpHAProfileStatus] = HAProfileStatusQuery{}

// ToMetricQuery converts the HAProfileStatusQuery to a MetricQuery to fulfill the Query interface.
func (query HAProfileStatusQuery) ToMetricQuery() MetricQuery[PtpHAProfileStatus] {
	return MetricQuery[PtpHAProfileStatus]{
		Metric: MetricHAProfileStatus,
		Labels: map[PtpMetricKey]MetricLabel[any]{
			KeyNode:    query.Node.ToAny(),
			KeyProfile: query.Profile.ToAny(),
			KeyProcess: query.Process.ToAny(),
		},
	}
}

// PPSStatusQuery is a query for the openshift_ptp_pps_status metric.
type PPSStatusQuery struct {
	From      MetricLabel[PtpProcess]
	Interface MetricLabel[iface.NICName]
	Node      MetricLabel[string]
	Process   MetricLabel[PtpProcess]
}

// This asserts at compile time that PPSStatusQuery implements the Query interface.
var _ Query[PtpPPSStatus] = PPSStatusQuery{}

// ToMetricQuery converts the PPSStatusQuery to a MetricQuery to fulfill the Query interface.
func (query PPSStatusQuery) ToMetricQuery() MetricQuery[PtpPPSStatus] {
	return MetricQuery[PtpPPSStatus]{
		Metric: MetricPPSStatus,
		Labels: map[PtpMetricKey]MetricLabel[any]{
			KeyFrom:      query.From.ToAny(),
			KeyInterface: query.Interface.ensureNIC().ToAny(),
			KeyNode:      query.Node.ToAny(),
			KeyProcess:   query.Process.ToAny(),
		},
	}
}

// ClockClassQuery is a query for the openshift_ptp_clock_class metric.
type ClockClassQuery struct {
	Node    MetricLabel[string]
	Process MetricLabel[PtpProcess]
}

// This asserts at compile time that ClockClassQuery implements the Query interface.
var _ Query[uint8] = ClockClassQuery{}

// ToMetricQuery converts the ClockClassQuery to a MetricQuery to fulfill the Query interface. Since the clock class is
// between 0 and 255, it is stored as a uint8. No enum exists as the meaning is context dependent.
func (query ClockClassQuery) ToMetricQuery() MetricQuery[uint8] {
	return MetricQuery[uint8]{
		Metric: MetricClockClass,
		Labels: map[PtpMetricKey]MetricLabel[any]{
			KeyNode:    query.Node.ToAny(),
			KeyProcess: query.Process.ToAny(),
		},
	}
}
