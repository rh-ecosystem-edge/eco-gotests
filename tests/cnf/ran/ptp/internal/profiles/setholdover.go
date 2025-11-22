package profiles

import (
	"context"
	"fmt"
	"strings"
	"time"

	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	ptpv1 "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/ptp/v1"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/metrics"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/ptp/internal/tsparams"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

// HoldOverMap is a map of profile references to the old HoldOverTimeout values. The value, in seconds, is nil if there
// is no PtpClockThreshold in the profile.
type HoldOverMap map[ProfileReference]*int64

func (h HoldOverMap) String() string {
	var stringBuilder strings.Builder

	stringBuilder.WriteString("HoldOverMap: {\n")

	for reference, holdover := range h {
		if holdover == nil {
			stringBuilder.WriteString(fmt.Sprintf("  %s: nil\n", reference.ProfileName))

			continue
		}

		stringBuilder.WriteString(fmt.Sprintf("  %s: %d\n", reference.ProfileName, *holdover))
	}

	stringBuilder.WriteString("}")

	return stringBuilder.String()
}

// SetHoldOverTimeouts sets the HoldOverTimeout for the provided profiles to the provided value. It returns a map of
// profile references to the old HoldOverTimeout values. Note that any errors in pulling or updating the configs may
// result in a partially modified state.
//
// Each profile provided must be unique, otherwise the function may return the wrong original HoldOverTimeout values.
//
// The returned map will have a key for each profile provided, with the value being nil if there is no PtpClockThreshold
// in the profile. Assuming it is not nil, the value will be the original HoldOverTimeout value.
func SetHoldOverTimeouts(
	client *clients.Settings, profiles []*ProfileInfo, holdoverTimeout int64) (HoldOverMap, error) {
	oldHoldovers := make(HoldOverMap, len(profiles))

	for _, profile := range profiles {
		ptpConfig, err := profile.Reference.PullPtpConfig(client)
		if err != nil {
			return nil, fmt.Errorf("failed to pull PTP config for profile %s: %w", profile.Reference.ProfileName, err)
		}

		profileIndex := profile.Reference.ProfileIndex
		if profileIndex < 0 || profileIndex >= len(ptpConfig.Definition.Spec.Profile) {
			return nil, fmt.Errorf("failed to reset profile %s at index %d: index out of bounds",
				profile.Reference.ProfileName, profileIndex)
		}

		if ptpConfig.Definition.Spec.Profile[profileIndex].PtpClockThreshold == nil {
			oldHoldovers[profile.Reference] = nil
			ptpConfig.Definition.Spec.Profile[profileIndex].PtpClockThreshold = &ptpv1.PtpClockThreshold{}
		} else {
			copiedHoldover := ptpConfig.Definition.Spec.Profile[profileIndex].PtpClockThreshold.HoldOverTimeout
			oldHoldovers[profile.Reference] = &copiedHoldover
		}

		ptpConfig.Definition.Spec.Profile[profileIndex].PtpClockThreshold.HoldOverTimeout = holdoverTimeout

		_, err = ptpConfig.Update()
		if err != nil {
			return nil, fmt.Errorf("failed to update PTP config for profile %s: %w", profile.Reference.ProfileName, err)
		}
	}

	klog.V(tsparams.LogLevel).Infof(
		"Set holdover timeout to %d seconds while saving original values: %v", holdoverTimeout, oldHoldovers)

	return oldHoldovers, nil
}

// WaitForHoldOverTimeouts waits for the HoldOverTimeout for the provided profiles to be set to the provided value. It
// does the assertion for all profiles at once, so the timeout applies to all of them.
func WaitForHoldOverTimeouts(
	prometheusAPI prometheusv1.API,
	nodeName string,
	profiles []*ProfileInfo,
	holdoverTimeout int64,
	timeout time.Duration) error {
	klog.V(tsparams.LogLevel).Infof(
		"Waiting for holdover timeout to be set to %d seconds for profiles on node %s",
		holdoverTimeout, nodeName)

	expected := make(map[string]ptpv1.PtpClockThreshold, len(profiles))
	for _, profile := range profiles {
		expected[profile.Reference.ProfileName] = ptpv1.PtpClockThreshold{
			HoldOverTimeout: holdoverTimeout,
		}
	}

	thresholdQuery := metrics.ThresholdQuery{Node: metrics.Equals(nodeName)}

	return wait.PollUntilContextTimeout(
		context.TODO(), 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			err := metrics.AssertThresholds(ctx, prometheusAPI, thresholdQuery, expected)
			if err != nil {
				klog.V(tsparams.LogLevel).Infof("Holdover timeout assertion failed: %v", err)

				return false, nil
			}

			return true, nil
		})
}

// ResetHoldOverTimeouts resets the HoldOverTimeout for the provided profiles to the original values. The same caveats
// apply where any errors in pulling or updating the configs may result in a partially modified state.
func ResetHoldOverTimeouts(client *clients.Settings, oldHoldovers HoldOverMap) error {
	klog.V(tsparams.LogLevel).Infof("Resetting holdover timeout to original values: %v", oldHoldovers)

	for reference, oldHoldover := range oldHoldovers {
		ptpConfig, err := reference.PullPtpConfig(client)
		if err != nil {
			return fmt.Errorf("failed to pull PTP config for profile %s: %w", reference.ProfileName, err)
		}

		profileIndex := reference.ProfileIndex
		if profileIndex < 0 || profileIndex >= len(ptpConfig.Definition.Spec.Profile) {
			return fmt.Errorf("failed to reset profile %s at index %d: index out of bounds", reference.ProfileName, profileIndex)
		}

		if oldHoldover == nil {
			ptpConfig.Definition.Spec.Profile[profileIndex].PtpClockThreshold = nil
		} else {
			if ptpConfig.Definition.Spec.Profile[profileIndex].PtpClockThreshold == nil {
				ptpConfig.Definition.Spec.Profile[profileIndex].PtpClockThreshold = &ptpv1.PtpClockThreshold{}
			}

			ptpConfig.Definition.Spec.Profile[profileIndex].PtpClockThreshold.HoldOverTimeout = *oldHoldover
		}

		_, err = ptpConfig.Update()
		if err != nil {
			return fmt.Errorf("failed to update PTP config for profile %s: %w", reference.ProfileName, err)
		}
	}

	return nil
}

// WaitForOldHoldOverTimeouts waits for the HoldOverTimeout for the provided profiles to be set to the original values.
// It does the assertion for all profiles at once, so the timeout applies to all of them.
func WaitForOldHoldOverTimeouts(
	prometheusAPI prometheusv1.API,
	nodeName string,
	oldHoldovers HoldOverMap,
	timeout time.Duration) error {
	klog.V(tsparams.LogLevel).Infof(
		"Waiting for holdover timeout to be set to original values %v for profiles on node %s",
		oldHoldovers, nodeName)

	expected := make(map[string]ptpv1.PtpClockThreshold, len(oldHoldovers))

	for reference, oldHoldover := range oldHoldovers {
		if oldHoldover == nil {
			continue
		}

		expected[reference.ProfileName] = ptpv1.PtpClockThreshold{
			HoldOverTimeout: *oldHoldover,
		}
	}

	thresholdQuery := metrics.ThresholdQuery{Node: metrics.Equals(nodeName)}

	return wait.PollUntilContextTimeout(
		context.TODO(), 5*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			err := metrics.AssertThresholds(ctx, prometheusAPI, thresholdQuery, expected)
			if err != nil {
				klog.V(tsparams.LogLevel).Infof("Holdover timeout assertion failed: %v", err)

				return false, nil
			}

			return true, nil
		})
}
