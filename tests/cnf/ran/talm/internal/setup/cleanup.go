package setup

import (
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/cgu"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ocm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/olm"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/talm/internal/tsparams"
)

// CleanupTestResourcesOnHub is used to delete all test resources, if they exist, in the provided namespace.
func CleanupTestResourcesOnHub(client *clients.Settings, namespace, suffix string) []error {
	var errorList []error

	// Only errors that come from deletions are kept since an error pulling usually means it doesn't exist.
	cgu, err := cgu.Pull(client, tsparams.CguName+suffix, namespace)
	if err == nil {
		// CGUs often take a few seconds to delete, so make sure it is actually gone before moving on.
		_, err = cgu.DeleteAndWait(5 * time.Minute)
		if err != nil {
			errorList = append(errorList, err)
		}
	}

	policy, err := ocm.PullPolicy(client, tsparams.PolicyName+suffix, namespace)
	if err == nil {
		_, err = policy.Delete()
		if err != nil {
			errorList = append(errorList, err)
		}
	}

	placementBinding, err := ocm.PullPlacementBinding(client, tsparams.PlacementBindingName+suffix, namespace)
	if err == nil {
		_, err = placementBinding.Delete()
		if err != nil {
			errorList = append(errorList, err)
		}
	}

	placementRule, err := ocm.PullPlacementRule(client, tsparams.PlacementRuleName+suffix, namespace)
	if err == nil {
		_, err = placementRule.Delete()
		if err != nil {
			errorList = append(errorList, err)
		}
	}

	policySet, err := ocm.PullPolicySet(client, tsparams.PolicySetName+suffix, namespace)
	if err == nil {
		_, err = policySet.Delete()
		if err != nil {
			errorList = append(errorList, err)
		}
	}

	return errorList
}

// CleanupTestResourcesOnSpokes deletes the catalog sources and temporary namespaces on the provided spoke clusters.
func CleanupTestResourcesOnSpokes(clusters []*clients.Settings, suffix string) []error {
	var errorList []error

	for _, client := range clusters {
		// Only errors that come from deletions are kept since an error pulling usually means it doesn't exist.
		catalogSource, err := olm.PullCatalogSource(client, tsparams.CatalogSourceName+suffix, tsparams.TemporaryNamespace)
		if err == nil {
			err = catalogSource.Delete()
			if err != nil {
				errorList = append(errorList, err)
			}
		}

		err = namespace.NewBuilder(client, tsparams.TemporaryNamespace).DeleteAndWait(5 * time.Minute)
		if err != nil {
			errorList = append(errorList, err)
		}
	}

	return errorList
}

// DeleteTalmTestNamespace deletes the TALM test namespace.
func DeleteTalmTestNamespace() error {
	clusters := []*clients.Settings{HubAPIClient, Spoke1APIClient, Spoke2APIClient}

	for _, client := range clusters {
		if client != nil {
			err := namespace.NewBuilder(client, tsparams.TestNamespace).DeleteAndWait(5 * time.Minute)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
