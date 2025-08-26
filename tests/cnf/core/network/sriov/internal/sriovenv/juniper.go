package sriovenv

import (
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netconfig"
)

// SwitchCredentials creates the struct to ssh to the lab switch.
type SwitchCredentials struct {
	User     string
	Password string
	SwitchIP string
}

// NewSwitchCredentials is the constructor for the SwitchCredentials object.
func NewSwitchCredentials() (*SwitchCredentials, error) {
	switchConfig := netconfig.NewNetConfig()

	user, err := switchConfig.GetSwitchUser()
	if err != nil {
		return nil, err
	}

	pass, err := switchConfig.GetSwitchPass()
	if err != nil {
		return nil, err
	}

	ipAddress, err := switchConfig.GetSwitchIP()
	if err != nil {
		return nil, err
	}

	return &SwitchCredentials{
		User:     user,
		Password: pass,
		SwitchIP: ipAddress,
	}, nil
}
