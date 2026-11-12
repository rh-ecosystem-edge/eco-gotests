//go:build unit_test

package gitdetails

import (
	"testing"

	argocdtypes "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/argocd/argocdtypes/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/argocd/gitopsengine/synccommon"
	"github.com/stretchr/testify/assert"
)

func TestSourceSyncCompleted(t *testing.T) {
	valid := &argocdtypes.Application{}
	valid.Spec.Source = &argocdtypes.ApplicationSource{Path: "new"}
	valid.Status.Sync.ComparedTo.Source.Path = "new"
	valid.Status.Sync.Status = argocdtypes.SyncStatusCodeSynced
	valid.Status.OperationState = &argocdtypes.OperationState{
		Phase: synccommon.OperationSucceeded,
		Operation: argocdtypes.Operation{
			Sync: &argocdtypes.SyncOperation{
				Source: &argocdtypes.ApplicationSource{Path: "new"},
			},
		},
		SyncResult: &argocdtypes.SyncOperationResult{
			Source: argocdtypes.ApplicationSource{Path: "new"},
		},
	}

	tests := []struct {
		name   string
		change func(*argocdtypes.Application)
		want   bool
	}{
		{"completed", func(*argocdtypes.Application) {}, true},
		{"missing source", func(app *argocdtypes.Application) { app.Spec.Source = nil }, false},
		{"changed source", func(app *argocdtypes.Application) { app.Spec.Source.Path = "other" }, false},
		{"no operation state", func(app *argocdtypes.Application) { app.Status.OperationState = nil }, true},
		{"stale operation without sync result", func(app *argocdtypes.Application) {
			app.Status.OperationState.SyncResult = nil
		}, true},
		{"stale operation result path", func(app *argocdtypes.Application) {
			app.Status.OperationState.SyncResult.Source.Path = "old"
		}, true},
		{"running operation", func(app *argocdtypes.Application) {
			app.Status.OperationState.Phase = synccommon.OperationRunning
		}, false},
		{"failed operation for path", func(app *argocdtypes.Application) {
			app.Status.OperationState.Phase = synccommon.OperationFailed
		}, false},
		{"error operation for path", func(app *argocdtypes.Application) {
			app.Status.OperationState.Phase = synccommon.OperationError
		}, false},
		{"failed operation for another path", func(app *argocdtypes.Application) {
			app.Status.OperationState.Phase = synccommon.OperationFailed
			app.Status.OperationState.Operation.Sync.Source.Path = "old"
			app.Status.OperationState.SyncResult.Source.Path = "old"
		}, true},
		{"failed operation without source", func(app *argocdtypes.Application) {
			app.Status.OperationState.Phase = synccommon.OperationFailed
			app.Status.OperationState.Operation.Sync.Source = nil
			app.Status.OperationState.SyncResult = nil
		}, false},
		{"queued operation", func(app *argocdtypes.Application) {
			app.Operation = &argocdtypes.Operation{Sync: &argocdtypes.SyncOperation{}}
		}, false},
		{"queued operation without state", func(app *argocdtypes.Application) {
			app.Operation = &argocdtypes.Operation{Sync: &argocdtypes.SyncOperation{}}
			app.Status.OperationState = nil
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
			assert.Equal(t, test.want, sourceSyncCompleted(app, "new"))
		})
	}
}
