package metrics

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"slices"
	"time"

	promapiv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"k8s.io/klog/v2"
)

// timedSample is an intermediate for flatenning the prometheus matrix data structure.
type timedSample struct {
	Metric    model.Metric
	Value     model.SampleValue
	Timestamp model.Time
}

// Collect performs a PromQL range query specificly for published PTP metric within the failed test case time
// window & writes them to chronologically ordered log file.
func Collect(
	client promapiv1.API,
	timeFrom, timeTo time.Time,
	logPath string,
) {
	timeRange := promapiv1.Range{
		Start: timeFrom,
		End:   timeTo,
		Step:  time.Second * 5,
	}
	q := "{__name__=~\"openshift_ptp_.*\"}"
	klog.V(ranparam.LogLevel).Infof("Query the metrics API: %s, %v", q, timeRange)

	res, warns, err := client.QueryRange(context.TODO(), q, timeRange)
	if err != nil {
		klog.V(ranparam.LogLevel).Infof("Failed to query the metrics API: %v", err)

		return
	}

	if warns != nil {
		klog.V(ranparam.LogLevel).Infof("Warnings from query the metrics API: %v", warns)
	}

	matrix, ok := res.(model.Matrix)
	if !ok {
		klog.V(ranparam.LogLevel).Infof("Expected type matrix, got %T", res)

		return
	}

	var timedSamples []timedSample

	for _, stream := range matrix {
		for _, timeValuePairs := range stream.Values {
			timedSamples = append(timedSamples, timedSample{
				Metric:    stream.Metric,
				Value:     timeValuePairs.Value,
				Timestamp: timeValuePairs.Timestamp,
			})
		}
	}

	slices.SortFunc(timedSamples, func(i, j timedSample) int {
		return cmp.Compare(i.Timestamp, j.Timestamp)
	})

	logFile, err := os.Create(logPath)
	if err != nil {
		klog.V(ranparam.LogLevel).Infof("Failed to create the metrics log file: %v", err)

		return
	}

	defer logFile.Close()

	for _, s := range timedSamples {
		if _, err := fmt.Fprintf(logFile,
			"%s %s %s\n",
			s.Timestamp.Time().Format(time.RFC3339Nano),
			s.Value,
			s.Metric); err != nil {
			klog.V(ranparam.LogLevel).Infof("Failed to write to log file: %s", err)

			return
		}
	}

	klog.V(ranparam.LogLevel).Infof(
		"PTP Metrics collected successfully, output saved to: %s", logPath)
}
