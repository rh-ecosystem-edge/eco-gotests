package gitdetails

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/argocd"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/gitopsztp/internal/tsparams"
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

// UpdateAndWaitForSync appends elements to the git path of the provided Argo CD application and waits for the source to
// be updated. The synced parameter indicates whether to wait for the application to be in a synced state or not.
func UpdateAndWaitForSync(app *argocd.ApplicationBuilder, synced bool, elements ...string) error {
	_, err := app.WithGitPathAppended(elements...).Update(true)
	if err != nil {
		return fmt.Errorf("failed to update the application: %w", err)
	}

	err = app.WaitForSourceUpdate(synced, tsparams.ArgoCdChangeTimeout)
	if err != nil {
		return fmt.Errorf("failed to wait for the application to sync: %w", err)
	}

	return nil
}
