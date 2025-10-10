package consumer

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ptp"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranparam"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/version"
)

type ptpEventAPIVersion string

const (
	eventAPIVersionV1 ptpEventAPIVersion = "1.0"
	eventAPIVersionV2 ptpEventAPIVersion = "2.0"
)

// DeployConsumersOnWorkers deploys the cloud-event-consumer on all worker nodes. It checks the event API version based
// on the PTP version and event version in the PtpOperatorConfig then delegates to either [DeployV1ConsumersOnWorkers]
// or [DeployV2ConsumersOnWorkers] to deploy the consumers.
func DeployConsumersOnWorkers(client *clients.Settings) error {
	eventAPIVersion, err := getEventAPIVersion(client)
	if err != nil {
		return fmt.Errorf("failed to get event API version trying to deploy consumers: %w", err)
	}

	switch eventAPIVersion {
	case eventAPIVersionV1:
		err := DeployV1ConsumersOnWorkers(client)
		if err != nil {
			return fmt.Errorf("failed to deploy v1 consumers on workers: %w", err)
		}
	case eventAPIVersionV2:
		err := DeployV2ConsumersOnWorkers(client)
		if err != nil {
			return fmt.Errorf("failed to deploy v2 consumers on workers: %w", err)
		}
	}

	return nil
}

// CleanupConsumersOnWorkers deletes the cloud-event-consumer on all worker nodes. It uses the same logic as
// [DeployConsumersOnWorkers] to determine the event API version and then delegates to either
// [CleanupV1ConsumersOnWorkers] or [CleanupV2ConsumersOnWorkers] to delete the consumers.
func CleanupConsumersOnWorkers(client *clients.Settings) error {
	eventAPIVersion, err := getEventAPIVersion(client)
	if err != nil {
		return fmt.Errorf("failed to get event API version trying to cleanup consumers: %w", err)
	}

	switch eventAPIVersion {
	case eventAPIVersionV1:
		err := CleanupV1ConsumersOnWorkers(client)
		if err != nil {
			return fmt.Errorf("failed to cleanup v1 consumers on workers: %w", err)
		}
	case eventAPIVersionV2:
		err := CleanupV2ConsumersOnWorkers(client)
		if err != nil {
			return fmt.Errorf("failed to cleanup v2 consumers on workers: %w", err)
		}
	}

	return nil
}

// getEventAPIVersion retrieves the event API version from the PTP operator config. If the PTP version on spoke 1 is at
// least 4.19, the version will always be [eventAPIVersionV2].
func getEventAPIVersion(client *clients.Settings) (ptpEventAPIVersion, error) {
	ptpVersion, ok := RANConfig.Spoke1OperatorVersions[ranparam.PTP]
	if !ok {
		return "", fmt.Errorf("PTP operator version not found in spoke 1 operator versions")
	}

	if atLeast419, err := version.IsVersionStringInRange(ptpVersion, "4.19", ""); err == nil && atLeast419 {
		return eventAPIVersionV2, nil
	}

	ptpOperatorConfig, err := ptp.PullPtpOperatorConfig(client)
	if err != nil {
		return "", fmt.Errorf("failed to pull PTP operator config: %w", err)
	}

	switch apiVersion := ptpOperatorConfig.Definition.Spec.EventConfig.ApiVersion; apiVersion {
	case string(eventAPIVersionV1):
		return eventAPIVersionV1, nil
	case string(eventAPIVersionV2):
		return eventAPIVersionV2, nil
	default:
		return "", fmt.Errorf("unknown event API version %s in PTP operator config", apiVersion)
	}
}
