package cnfcluster

import (
	"errors"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
)

// CheckClustersPresent can be used to check for the presence of specific clusters.
func CheckClustersPresent(clients []*clients.Settings) error {
	for _, client := range clients {
		if client == nil {
			return errors.New("provided nil client in cluster list")
		}
	}

	return nil
}
