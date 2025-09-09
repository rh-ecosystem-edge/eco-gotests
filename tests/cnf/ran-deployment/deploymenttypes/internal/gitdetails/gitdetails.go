package gitdetails

import (
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/argocd"
	argocdtypes "github.com/rh-ecosystem-edge/eco-goinfra/pkg/schemes/argocd/argocdtypes/v1alpha1"
)

// GetGitSource gets the ApplicationSource from the ApplicationsBuilder.
func GetGitSource(app *argocd.ApplicationBuilder) (*argocdtypes.ApplicationSource, error) {
	appSourceInfo := app.Object.Spec.GetSource()

	switch {
	case len(appSourceInfo.RepoURL) == 0:
		return nil, fmt.Errorf("%s application git RepoURL is empty", app.Object.Name)
	case len(appSourceInfo.Path) == 0:
		return nil, fmt.Errorf("%s application git Path is empty", app.Object.Name)
	case len(appSourceInfo.TargetRevision) == 0:
		return nil, fmt.Errorf("%s application git TargetRevision is empty", app.Object.Name)
	}

	return &appSourceInfo, nil
}
