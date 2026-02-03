// Package eventmetric provides utilities for working with both events and metrics at the same time. It is designed for
// cases where one wishes to wait for both an event and a metric if events are enabled, or only a metric if events are
// disabled.
//
// It depends on the events package and the metrics package for their respective functionality, rather than
// re-implementing it here.
//
// # Quick Start
//
// Use [NewAssertion] to create an assertion, then chain [ForNode] to specify the target:
//
//	err := eventmetric.NewAssertion(prometheusAPI, metrics.ClockStateQuery{}, metrics.ClockStateLocked,
//	        events.All(events.IsType(eventptp.PtpStateChange), events.HasValue(events.WithSyncState(eventptp.LOCKED)))).
//	        ForNode(client, nodeName).
//	        WithTimeout(10 * time.Minute).
//	        ExecuteAssertion(ctx)
//
// The type parameter is inferred from the query and expected value, so you don't need to specify it explicitly.
//
// # Event Enablement Behavior
//
// When [ExecuteAssertion] is called, the package automatically checks if events are enabled on the cluster before
// running the event assertion. If events are disabled, only the metric assertion runs. This allows tests to work
// correctly regardless of whether events are configured.
package eventmetric

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/consumer"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/events"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/metrics"
	"golang.org/x/exp/constraints"
)

// AssertConfig is a struct that contains the configuration for the assertion. It combines the possible inputs to
// waiting on events and metrics at the same time.
type AssertConfig[V constraints.Integer] struct {
	// PrometheusAPI is the Prometheus API to use for the metric assertion.
	PrometheusAPI prometheusv1.API

	// EventFilter is the filter to use for the event. An event matching this filter must appear in the event pod
	// before the timeout is reached.
	EventFilter events.EventFilter

	// MetricQuery is the metric query to use. The metric value must match the expected value before the timeout is
	// reached.
	MetricQuery metrics.Query[V]

	// ExpectedMetricValue is the expected metric value to match.
	ExpectedMetricValue V

	// StartTime is the start time of the time window to check for events and metrics. If zero, it defaults to the
	// time the assertion is executed.
	StartTime time.Time

	// Timeout is the total time to wait for the event and metric to match the expected values.
	Timeout time.Duration

	// EventOptions are optional configurations for the event assertion.
	EventOptions []events.WaitForEventOption

	// MetricOptions are optional configurations for the metric assertion.
	MetricOptions []metrics.QueryAssertOption

	// ClusterClient is the cluster client to use for looking up the consumer pod.
	ClusterClient *clients.Settings

	// NodeName is the name of the node to use for the event assertion.
	NodeName string
}

// NewAssertion creates a new assertion config with the core assertion parameters. The type parameter V is inferred from
// the query and expected value, so callers do not need to specify it explicitly.
//
// After calling NewAssertion, you must call [ForNode] to specify where to run the assertion, then optionally configure
// timeout and other options before calling [ExecuteAssertion].
func NewAssertion[V constraints.Integer](
	prometheusAPI prometheusv1.API,
	query metrics.Query[V],
	expectedValue V,
	eventFilter events.EventFilter,
) *AssertConfig[V] {
	return &AssertConfig[V]{
		PrometheusAPI:       prometheusAPI,
		MetricQuery:         query,
		ExpectedMetricValue: expectedValue,
		EventFilter:         eventFilter,
	}
}

// ForNode sets the target node for the assertion. The cluster client is used to check if events are enabled and to look
// up the consumer pod for the node.
//
// If events are disabled on the cluster, only the metric assertion will run. If events are enabled, both the metric and
// event assertions run in parallel.
func (assertConfig *AssertConfig[V]) ForNode(clusterClient *clients.Settings, nodeName string) *AssertConfig[V] {
	assertConfig.ClusterClient = clusterClient
	assertConfig.NodeName = nodeName

	return assertConfig
}

// WithStartTime sets the start time for the assertion. If the start time is zero, it defaults to the time the assertion
// is executed.
func (assertConfig *AssertConfig[V]) WithStartTime(startTime time.Time) *AssertConfig[V] {
	assertConfig.StartTime = startTime

	return assertConfig
}

// WithTimeout sets the timeout for the assertion.
func (assertConfig *AssertConfig[V]) WithTimeout(timeout time.Duration) *AssertConfig[V] {
	assertConfig.Timeout = timeout

	return assertConfig
}

// WithEventOptions sets the event options for the assertion.
func (assertConfig *AssertConfig[V]) WithEventOptions(eventOptions ...events.WaitForEventOption) *AssertConfig[V] {
	assertConfig.EventOptions = eventOptions

	return assertConfig
}

// WithMetricOptions sets the metric options for the assertion.
func (assertConfig *AssertConfig[V]) WithMetricOptions(metricOptions ...metrics.QueryAssertOption) *AssertConfig[V] {
	assertConfig.MetricOptions = metricOptions

	return assertConfig
}

// ExecuteAssertion executes the assertion. It first validates the config, then checks if events are enabled. If events
// are enabled, both the event and metric assertions run in parallel. Otherwise, only the metric assertion runs. Returns
// an error if either assertion fails.
func (assertConfig *AssertConfig[V]) ExecuteAssertion(ctx context.Context) error {
	if err := assertConfig.validate(); err != nil {
		return fmt.Errorf("invalid assert config: %w", err)
	}

	startTime := assertConfig.StartTime
	if startTime.IsZero() {
		startTime = time.Now()
	}

	eventsEnabled, err := consumer.AreEventsEnabled(assertConfig.ClusterClient)
	if err != nil {
		return fmt.Errorf("failed to check if events are enabled: %w", err)
	}

	errChan := make(chan error, 2)
	waitGroup := sync.WaitGroup{}

	if eventsEnabled {
		eventPod, err := consumer.GetConsumerPodforNode(assertConfig.ClusterClient, assertConfig.NodeName)
		if err != nil {
			return fmt.Errorf("failed to get consumer pod for node: %w", err)
		}

		waitGroup.Go(func() {
			err := events.WaitForEvent(
				eventPod, startTime, assertConfig.Timeout, assertConfig.EventFilter, assertConfig.EventOptions...)
			if err != nil {
				errChan <- fmt.Errorf("event assertion failed: %w", err)
			}
		})
	}

	metricOptions := make([]metrics.QueryAssertOption, 0, len(assertConfig.MetricOptions)+2)
	metricOptions = append(metricOptions,
		metrics.AssertWithStartTime(startTime), metrics.AssertWithTimeout(assertConfig.Timeout))
	metricOptions = append(metricOptions, assertConfig.MetricOptions...)

	waitGroup.Go(func() {
		err := metrics.AssertQuery(
			ctx,
			assertConfig.PrometheusAPI,
			assertConfig.MetricQuery,
			assertConfig.ExpectedMetricValue,
			metricOptions...,
		)
		if err != nil {
			errChan <- fmt.Errorf("metric assertion failed: %w", err)
		}
	})

	waitGroup.Wait()
	close(errChan)

	var combinedErr error
	for err := range errChan {
		combinedErr = errors.Join(combinedErr, err)
	}

	return combinedErr
}

// validate validates the assert config. It ensures all the required options are provided.
func (assertConfig *AssertConfig[V]) validate() error {
	if isInterfaceNil(assertConfig.PrometheusAPI) {
		return fmt.Errorf("prometheus API is required and cannot be nil")
	}

	if isInterfaceNil(assertConfig.EventFilter) {
		return fmt.Errorf("event filter is required and cannot be nil")
	}

	if isInterfaceNil(assertConfig.MetricQuery) {
		return fmt.Errorf("metric query is required and cannot be nil")
	}

	if assertConfig.ClusterClient == nil {
		return fmt.Errorf("cluster client is required and cannot be nil")
	}

	if assertConfig.NodeName == "" {
		return fmt.Errorf("node name is required and cannot be empty")
	}

	return nil
}

// isInterfaceNil checks if the interface is nil. It compares both the interface itself and its concrete type. It will
// not panic even if the concrete type is not nilable.
func isInterfaceNil(v any) bool {
	if v == nil {
		return true
	}

	reflectValue := reflect.ValueOf(v)

	kind := reflectValue.Kind()
	if kind == reflect.Chan ||
		kind == reflect.Func ||
		kind == reflect.Interface ||
		kind == reflect.Map ||
		kind == reflect.Ptr ||
		kind == reflect.Slice {
		return reflectValue.IsNil()
	}

	return false
}
