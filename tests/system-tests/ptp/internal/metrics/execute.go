package metrics

import (
	"context"
	"fmt"
	"time"

	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	"golang.org/x/exp/constraints"
	"k8s.io/klog/v2"
)

// ExecuteQuery executes a Prometheus query and returns the result as a model.Vector. If the query has a non-zero end
// time, it uses that as the query time; otherwise, it uses the current time. It also logs any warnings returned by the
// query.
//
// Type parameter V is the expected type of the query result. It is used to type the query and does not appear in the
// result, since the result is always a model.Vector which represents samples as float64 values.
//
// SECURITY: This function does not perform any sort of sanitization on the query. It should only be used with trusted
// queries.
func ExecuteQuery[V constraints.Integer](
	ctx context.Context, client prometheusv1.API, query Query[V]) (model.Vector, error) {
	if client == nil {
		return nil, fmt.Errorf("cannot execute query with nil client")
	}

	metricQuery := query.ToMetricQuery()

	queryTime := metricQuery.End
	if queryTime.IsZero() {
		queryTime = time.Now()
	}

	queryString := metricQuery.String()

	klog.V(tsparams.LogLevel).Infof("Executing query: %s", queryString)

	result, warnings, err := client.Query(ctx, queryString, queryTime)
	if err != nil {
		return nil, err
	}

	for _, warning := range warnings {
		klog.V(tsparams.LogLevel).Infof("Query returned warning: %s", warning)
	}

	if result.Type() != model.ValVector {
		return nil, fmt.Errorf("unexpected result type: %s", result.Type())
	}

	vector, ok := result.(model.Vector)
	if !ok {
		return nil, fmt.Errorf("failed to cast result to vector")
	}

	return vector, nil
}

// ExecuteQueryRange executes a Prometheus query range and returns the result as a model.Matrix. It logs any warnings
// returned by the query.
//
// Type parameter V is the expected type of the query result. It is used to type the query and does not appear in the
// result, since the result is always a model.Matrix which represents samples as float64 values.
//
// SECURITY: This function does not perform any sort of sanitization on the query. It should only be used with trusted
// queries.
func ExecuteQueryRange[V constraints.Integer](
	ctx context.Context, client prometheusv1.API, query Query[V]) (model.Matrix, error) {
	if client == nil {
		return nil, fmt.Errorf("cannot execute query range with nil client")
	}

	metricQuery := query.ToMetricQuery()
	queryString := metricQuery.String()

	klog.V(tsparams.LogLevel).Infof("Executing query range: %s", queryString)

	result, warnings, err := client.QueryRange(ctx, queryString, metricQuery.Range())
	if err != nil {
		return nil, err
	}

	for _, warning := range warnings {
		klog.V(tsparams.LogLevel).Infof("Query returned warning: %s", warning)
	}

	if result.Type() != model.ValMatrix {
		return nil, fmt.Errorf("unexpected result type: %s", result.Type())
	}

	matrix, ok := result.(model.Matrix)
	if !ok {
		return nil, fmt.Errorf("failed to cast result to matrix")
	}

	return matrix, nil
}
