package gitdetails

import (
	"context"
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/argocd"
	argocdtypes "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/argocd/argocdtypes/v1alpha1"
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
						(operation == nil || operation.Phase == "Succeeded"))), nil
			})
	}

	// Argo timestamps have one-second precision. Requiring a later second avoids accepting an old operation.
	updatedAfter := time.Now().UTC().Truncate(time.Second)

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

			return sourceSyncCompleted(current, path, updatedAfter), nil
		})
	if err != nil {
		return fmt.Errorf("failed to wait for application sync to path %q after %s: %w", path, updatedAfter, err)
	}

	return nil
}

// sourceSyncCompleted verifies that a successful operation and reconciliation followed the path update.
func sourceSyncCompleted(app *argocdtypes.Application, path string, updatedAfter time.Time) bool {
	operation := app.Status.OperationState

	return app.Spec.Source != nil && app.Spec.Source.Path == path &&
		operation != nil && operation.Operation.Sync != nil && operation.Operation.Sync.Source != nil &&
		operation.Operation.Sync.Source.Path == path && operation.StartedAt.Time.After(updatedAfter) &&
		operation.Phase == "Succeeded" && app.Status.ReconciledAt != nil &&
		app.Status.ReconciledAt.Time.After(updatedAfter) && app.Status.Sync.ComparedTo.Source.Path == path &&
		app.Status.Sync.Status == argocdtypes.SyncStatusCodeSynced
}
