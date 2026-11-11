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
		{"missing operation", func(app *argocdtypes.Application) { app.Status.OperationState = nil }, false},
		{"missing sync result", func(app *argocdtypes.Application) { app.Status.OperationState.SyncResult = nil }, false},
		{"old result path", func(app *argocdtypes.Application) {
			app.Status.OperationState.SyncResult.Source.Path = "old"
		}, false},
		{"running operation", func(app *argocdtypes.Application) {
			app.Status.OperationState.Phase = synccommon.OperationRunning
		}, false},
		{"failed operation", func(app *argocdtypes.Application) {
			app.Status.OperationState.Phase = synccommon.OperationFailed
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
