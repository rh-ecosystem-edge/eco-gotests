package gitdetails

import (
	"context"
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/argocd"
	argocdtypes "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/argocd/argocdtypes/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/argocd/gitopsengine/synccommon"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/gitopsztp/internal/tsparams"
	"k8s.io/apimachinery/pkg/util/wait"
)

// GetGitPath retrieves the git path from the provided Argo CD application. It returns an error if it encounters any nil
// pointers trying to access the source path.
func GetGitPath(app *argocd.ApplicationBuilder) (string, error) {
	if app == nil {
		return "", fmt.Errorf("application is nil")
	}

	if app.Definition == nil {
		return "", fmt.Errorf("application definition is nil")
	}

	if app.Definition.Spec.Source == nil {
		return "", fmt.Errorf("application source is nil")
	}

	return app.Definition.Spec.Source.Path, nil
}

// UpdateAndWaitForSync appends elements to the git path and waits for the resulting comparison or sync.
func UpdateAndWaitForSync(app *argocd.ApplicationBuilder, synced bool, elements ...string) error {
	app.WithGitPathAppended(elements...)

	return UpdatePathAndWait(app, synced)
}

// UpdatePathAndWait applies the path in the definition and waits for its reconciliation and, when requested, sync.
func UpdatePathAndWait(app *argocd.ApplicationBuilder, synced bool) error {
	path, err := GetGitPath(app)
	if err != nil {
		return err
	}

	current, err := app.Get()
	if err != nil {
		return fmt.Errorf("failed to get the application before updating it: %w", err)
	}

	// A repeated cleanup does not create another sync operation. Wait for any existing operation to finish.
	if current.Spec.Source != nil && current.Spec.Source.Path == path {
		return wait.PollUntilContextTimeout(context.TODO(), time.Second, tsparams.ArgoCdChangeTimeout, true,
			func(ctx context.Context) (bool, error) {
				current, err := app.Get()
				if err != nil {
					return false, nil
				}

				operation := current.Status.OperationState

				return current.Spec.Source != nil && current.Spec.Source.Path == path &&
					current.Status.Sync.ComparedTo.Source.Path == path &&
					(!synced || (current.Status.Sync.Status == argocdtypes.SyncStatusCodeSynced &&
						(operation == nil || operation.Phase == synccommon.OperationSucceeded))), nil
			})
	}

	if _, err = app.Update(true); err != nil {
		return fmt.Errorf("failed to update the application: %w", err)
	}

	if !synced {
		if err = app.WaitForSourceUpdate(false, tsparams.ArgoCdChangeTimeout); err != nil {
			return fmt.Errorf("failed to wait for the application source update: %w", err)
		}

		return nil
	}

	err = wait.PollUntilContextTimeout(context.TODO(), time.Second, tsparams.ArgoCdChangeTimeout, true,
		func(ctx context.Context) (bool, error) {
			current, err := app.Get()
			if err != nil {
				return false, nil
			}

			return sourceSyncCompleted(current, path), nil
		})
	if err != nil {
		return fmt.Errorf("failed to wait for application sync to path %q: %w", path, err)
	}

	return nil
}

// sourceSyncCompleted verifies that a successful sync used the expected path and the application is synced to it.
func sourceSyncCompleted(app *argocdtypes.Application, path string) bool {
	operation := app.Status.OperationState

	return app.Spec.Source != nil && app.Spec.Source.Path == path &&
		operation != nil && operation.Phase == synccommon.OperationSucceeded &&
		operation.SyncResult != nil && operation.SyncResult.Source.Path == path &&
		app.Status.Sync.ComparedTo.Source.Path == path &&
		app.Status.Sync.Status == argocdtypes.SyncStatusCodeSynced
}
