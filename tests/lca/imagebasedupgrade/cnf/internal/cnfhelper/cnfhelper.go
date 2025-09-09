package cnfhelper

import (
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/cgu"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/lca/imagebasedupgrade/cnf/internal/cnfinittools"
)

// GetAllTestClients is used to quickly obtain a list of all the test clients.
func GetAllTestClients() []*clients.Settings {
	return []*clients.Settings{
		cnfinittools.TargetHubAPIClient,
		cnfinittools.TargetSNOAPIClient,
	}
}

// DeleteIbuTestCguOnTargetHub is used to delete ibu test cgu's, if they exist, in the provided namespace.
func DeleteIbuTestCguOnTargetHub(client *clients.Settings, name, namespace string) error {
	// Only errors that come from deletions are kept since an error pulling usually means it doesn't exist.
	cgu, err := cgu.Pull(client, name, namespace)
	if err == nil {
		_, err = cgu.DeleteAndWait(1 * time.Minute)
		if err != nil {
			return err
		}
	}

	return err
}
