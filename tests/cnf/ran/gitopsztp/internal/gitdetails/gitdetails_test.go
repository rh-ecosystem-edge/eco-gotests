//go:build unit_test

package gitdetails

import (
	"testing"
	"time"

	argocdtypes "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/argocd/argocdtypes/v1alpha1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSourceSyncCompleted(t *testing.T) {
	updated := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	later := metav1.NewTime(updated.Add(time.Second))
	valid := &argocdtypes.Application{}
	valid.Spec.Source = &argocdtypes.ApplicationSource{Path: "new"}
	valid.Status.Sync.ComparedTo.Source.Path = "new"
	valid.Status.Sync.Status = argocdtypes.SyncStatusCodeSynced
	valid.Status.ReconciledAt = &later
	valid.Status.OperationState = &argocdtypes.OperationState{StartedAt: later, Phase: "Succeeded"}
	valid.Status.OperationState.Operation.Sync = &argocdtypes.SyncOperation{
		Source: &argocdtypes.ApplicationSource{Path: "new"},
	}

	tests := []struct {
		name   string
		change func(*argocdtypes.Application)
		want   bool
	}{
		{"completed", func(*argocdtypes.Application) {}, true},
		{"missing source", func(app *argocdtypes.Application) { app.Spec.Source = nil }, false},
		{"changed source", func(app *argocdtypes.Application) { app.Spec.Source.Path = "other" }, false},
		{"missing operation", func(app *argocdtypes.Application) { app.Status.OperationState = nil }, false},
		{"missing sync", func(app *argocdtypes.Application) { app.Status.OperationState.Operation.Sync = nil }, false},
		{"missing operation source", func(app *argocdtypes.Application) {
			app.Status.OperationState.Operation.Sync.Source = nil
		}, false},
		{"old operation path", func(app *argocdtypes.Application) {
			app.Status.OperationState.Operation.Sync.Source.Path = "old"
		}, false},
		{"old operation", func(app *argocdtypes.Application) {
			app.Status.OperationState.StartedAt = metav1.NewTime(updated)
		}, false},
		{"running operation", func(app *argocdtypes.Application) { app.Status.OperationState.Phase = "Running" }, false},
		{"failed operation", func(app *argocdtypes.Application) { app.Status.OperationState.Phase = "Failed" }, false},
		{"missing reconciliation", func(app *argocdtypes.Application) { app.Status.ReconciledAt = nil }, false},
		{"old reconciliation", func(app *argocdtypes.Application) {
			old := metav1.NewTime(updated)
			app.Status.ReconciledAt = &old
		}, false},
		{"old comparison", func(app *argocdtypes.Application) { app.Status.Sync.ComparedTo.Source.Path = "old" }, false},
		{"out of sync", func(app *argocdtypes.Application) {
			app.Status.Sync.Status = argocdtypes.SyncStatusCodeOutOfSync
		}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app := valid.DeepCopy()
			test.change(app)
			assert.Equal(t, test.want, sourceSyncCompleted(app, "new", updated))
		})
	}
}
