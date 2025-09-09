package stability

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ocm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/ptp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func buildOutputLine(data map[string]string) string {
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	var sb strings.Builder
	for _, key := range keys {
		sb.WriteString(fmt.Sprintf("%s,%s,", key, data[key]))
	}

	line := strings.TrimSuffix(sb.String(), ",")

	currentTime := time.Now().Format(time.RFC3339)

	return fmt.Sprintf("%s,%s\n", currentTime, line)
}

// SavePTPStatus stores the PTP status in the outputFile.
func SavePTPStatus(apiClient *clients.Settings, outputFile string, timeInterval time.Duration) error {
	file, err := os.OpenFile(outputFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	ptpOnSync, err := ptp.ValidatePTPStatus(apiClient, timeInterval)

	currentTime := time.Now().Format(time.RFC3339)

	var line string

	if ptpOnSync {
		line = fmt.Sprintf("%s,Sync,\"%v\"\n", currentTime, err)
	} else {
		line = fmt.Sprintf("%s,Unsync,\"%v\"\n", currentTime, err)
	}

	_, err = file.WriteString(line)
	if err != nil {
		return err
	}

	return nil
}

// SavePodsRestartsInNamespace stores the pod restarts of all pods in a namespace in the outputFile.
func SavePodsRestartsInNamespace(apiClient *clients.Settings, namespace string, outputFile string) error {
	podList, err := pod.List(apiClient, namespace, metav1.ListOptions{})
	if err != nil {
		return err
	}

	file, err := os.OpenFile(outputFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	allPodsInNamespace := make(map[string]string)

	for _, pod := range podList {
		totalRestarts := 0
		for _, containerStatus := range pod.Object.Status.ContainerStatuses {
			totalRestarts += int(containerStatus.RestartCount)
		}

		allPodsInNamespace[pod.Object.Name] = fmt.Sprintf("%d", totalRestarts)
	}

	entry := buildOutputLine(allPodsInNamespace)

	_, err = file.WriteString(entry)
	if err != nil {
		return err
	}

	return nil
}

// SavePolicyStatus stores the status of all policies in the outputFile.
func SavePolicyStatus(apiClient *clients.Settings, clusterName string, outputFile string) error {
	file, err := os.OpenFile(outputFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	allPolicies, err := ocm.ListPoliciesInAllNamespaces(apiClient,
		runtimeclient.ListOptions{Namespace: clusterName})
	if err != nil {
		return fmt.Errorf("failed to get policies in %q NS: %w", clusterName, err)
	}

	allPoliciesStatus := make(map[string]string)

	for _, policy := range allPolicies {
		pName := policy.Definition.Name
		pState := string(policy.Object.Status.ComplianceState)
		allPoliciesStatus[pName] = pState
	}

	entry := buildOutputLine(allPoliciesStatus)

	_, err = file.WriteString(entry)
	if err != nil {
		return err
	}

	return nil
}

// SaveTunedRestarts stores the status of tuned restarts in the outputFile.
func SaveTunedRestarts(apiClient *clients.Settings, outputFile string) error {
	file, err := os.OpenFile(outputFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	// Get the tuned pod by name
	tunedPods, err := pod.ListByNamePattern(apiClient, "tuned", "openshift-cluster-node-tuning-operator")
	if err != nil {
		return err
	}

	tunedRestarts := make(map[string]string)

	for _, tunedPod := range tunedPods {
		tunedLog, err := tunedPod.GetFullLog("tuned")
		if err != nil {
			return err
		}

		pattern := "static tuning from profile .* applied"

		regex, err := regexp.Compile(pattern)
		if err != nil {
			return err
		}

		tunedApplyCount := len(regex.FindAllString(tunedLog, -1))
		tunedRestarts["tuned_restarts"] = fmt.Sprintf("%d", tunedApplyCount)
	}

	entry := buildOutputLine(tunedRestarts)

	_, err = file.WriteString(entry)
	if err != nil {
		return err
	}

	return nil
}

// VerifyStabilityStatusChange checks if there has been a change between in
// the column of the stability output file.
func VerifyStabilityStatusChange(filePath string) (bool, error) {
	file, err := os.OpenFile(filePath, os.O_RDONLY, 0)
	if err != nil {
		return false, err
	}

	defer file.Close()

	var previousLine []string

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		columns := strings.Split(line, ",")

		if previousLine == nil {
			previousLine = columns

			continue
		}

		for i := 1; i < len(columns); i += 2 {
			if columns[i] != previousLine[i] || columns[i+1] != previousLine[i+1] {
				return true, fmt.Errorf("change detected in column %d:\n previous: %s - %s\n current: %s - %s",
					i+1, previousLine[i], previousLine[i+1], columns[i], columns[i+1])
			}
		}

		previousLine = columns
	}

	if err := scanner.Err(); err != nil {
		return false, err
	}

	return false, nil
}
