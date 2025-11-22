package prometheus

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"k8s.io/klog/v2"
)

type queryOutput struct {
	Data data
}
type data struct {
	Result []result
}
type result struct {
	Metric metric
}

type metric struct {
	Pod string
}

// PodMetricsPresentInDB returns true if the given metrics are present for the given pod in a prometheus database.
func PodMetricsPresentInDB(prometheusPod *pod.Builder, podName string, uniqueMetricKeys []string) (bool, error) {
	for _, metricsKey := range uniqueMetricKeys {
		metricFound := false
		command := []string{
			"curl",
			fmt.Sprintf("%squery?query=%s", "http://localhost:9090/api/v1/", metricsKey),
		}

		stdout, err := prometheusPod.ExecCommand(command)
		if err != nil {
			klog.V(90).Infof("Fail to collect metric %s. Stdout %s", metricsKey, stdout.String())

			return false, err
		}

		var queryOutput queryOutput

		err = json.Unmarshal(stdout.Bytes(), &queryOutput)
		if err != nil {
			return false, err
		}

		if len(queryOutput.Data.Result) < 1 {
			return false, fmt.Errorf("failed to detect metric %s", metricsKey)
		}

		for _, result := range queryOutput.Data.Result {
			if strings.Contains(result.Metric.Pod, podName) {
				metricFound = true
			}
		}

		if !metricFound {
			return false, nil
		}
	}

	return true, nil
}
