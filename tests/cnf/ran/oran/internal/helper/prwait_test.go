//go:build unit_test

package helper

import (
	"testing"
	"time"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestClusterProvisionTimeout(t *testing.T) {
	t.Parallel()

	assert.Equal(t, tsparams.ClusterProvisionBaseTimeout, ClusterProvisionTimeout(0))
	assert.Equal(t,
		tsparams.ClusterProvisionBaseTimeout+5*tsparams.ClusterProvisionPerNodeTimeout,
		ClusterProvisionTimeout(5))
	assert.Equal(t, tsparams.ClusterProvisionBaseTimeout, ClusterProvisionTimeout(-1))

	// 5-node MNO budget from the kni-qe-26 run (30m + 15m*5 = 1h45m).
	assert.Equal(t, 105*time.Minute, ClusterProvisionTimeout(5))
}

func TestConfigurationAppliedTimeout(t *testing.T) {
	t.Parallel()

	assert.Equal(t, tsparams.ConfigurationAppliedBaseTimeout, ConfigurationAppliedTimeout(0))
	assert.Equal(t,
		tsparams.ConfigurationAppliedBaseTimeout+3*tsparams.ConfigurationAppliedPerMasterTimeout,
		ConfigurationAppliedTimeout(3))
	assert.Equal(t, tsparams.ConfigurationAppliedBaseTimeout, ConfigurationAppliedTimeout(-1))

	// 3-master MNO budget (20m + 30m*3 = 1h50m); SNO single master is 50m.
	assert.Equal(t, 110*time.Minute, ConfigurationAppliedTimeout(3))
	assert.Equal(t, 50*time.Minute, ConfigurationAppliedTimeout(1))
}

func TestFormatPRWaitTimeoutError(t *testing.T) {
	t.Parallel()

	expected := metav1.Condition{
		Type:   "ConfigurationApplied",
		Status: metav1.ConditionTrue,
		Reason: "Completed",
	}
	conditions := []metav1.Condition{{
		Type:               "ConfigurationApplied",
		Status:             metav1.ConditionFalse,
		Reason:             "InProgress",
		Message:            "The configuration is still being applied",
		LastTransitionTime: metav1.NewTime(time.Date(2026, 10, 22, 15, 19, 0, 0, time.UTC)),
	}}

	err := FormatPRWaitTimeoutError(
		"9c5372f3-ea1d-4a96-8157-b3b874a55cf9",
		expected,
		2*time.Hour,
		"progressing",
		"Cluster configuration is being applied",
		conditions,
	)
	assert.Error(t, err)

	msg := err.Error()
	assert.Contains(t, msg, "timed out after 2h0m0s")
	assert.Contains(t, msg, "9c5372f3-ea1d-4a96-8157-b3b874a55cf9")
	assert.Contains(t, msg, `type="ConfigurationApplied"`)
	assert.Contains(t, msg, `status="True"`)
	assert.Contains(t, msg, `reason="Completed"`)
	assert.Contains(t, msg, `provisioningPhase="progressing"`)
	assert.Contains(t, msg, `provisioningDetails="Cluster configuration is being applied"`)
	assert.Contains(t, msg, `reason="InProgress"`)
	assert.Contains(t, msg, "The configuration is still being applied")
	assert.Contains(t, msg, "2026-10-22T15:19:00Z")
}

func TestFormatPRWaitTimeoutErrorEmptyConditions(t *testing.T) {
	t.Parallel()

	err := FormatPRWaitTimeoutError(
		"test-pr",
		metav1.Condition{Type: "ProvisioningRequestValidated"},
		5*time.Minute,
		"",
		"",
		nil,
	)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "conditions=[]")
}

func TestConditionMatchesExpected(t *testing.T) {
	t.Parallel()

	observed := metav1.Condition{
		Type:    "ClusterProvisioned",
		Status:  metav1.ConditionTrue,
		Reason:  "Completed",
		Message: "Provisioning completed",
	}

	assert.True(t, conditionMatchesExpected(observed, metav1.Condition{
		Type:   "ClusterProvisioned",
		Status: metav1.ConditionTrue,
		Reason: "Completed",
	}))
	assert.False(t, conditionMatchesExpected(observed, metav1.Condition{
		Type:   "ClusterProvisioned",
		Status: metav1.ConditionFalse,
		Reason: "Completed",
	}))
	assert.True(t, conditionMatchesExpected(observed, metav1.Condition{
		Type:    "ClusterProvisioned",
		Message: "Provisioning",
	}))
	assert.False(t, conditionMatchesExpected(observed, metav1.Condition{
		Type:    "ClusterProvisioned",
		Message: "missing",
	}))
}

func TestPRStatusSnapshotEqual(t *testing.T) {
	t.Parallel()

	transitionTime := metav1.NewTime(time.Date(2026, 10, 22, 15, 0, 0, 0, time.UTC))
	leftSnap := prStatusSnapshot{
		phase:   "progressing",
		details: "Cluster installation is in progress",
		conditions: []metav1.Condition{{
			Type:               "ClusterProvisioned",
			Status:             metav1.ConditionFalse,
			Reason:             "InProgress",
			LastTransitionTime: transitionTime,
		}},
	}
	rightSnap := prStatusSnapshot{
		phase:   "progressing",
		details: "Cluster installation is in progress",
		conditions: []metav1.Condition{{
			Type:               "ClusterProvisioned",
			Status:             metav1.ConditionFalse,
			Reason:             "InProgress",
			LastTransitionTime: transitionTime,
		}},
	}
	assert.True(t, leftSnap.equal(rightSnap))

	rightSnap.details = "changed"
	assert.False(t, leftSnap.equal(rightSnap))
}
