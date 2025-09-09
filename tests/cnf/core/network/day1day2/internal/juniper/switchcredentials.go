package juniper

import . "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"

// SwitchCredentials stores the authentication and connection details for a network switch.
type SwitchCredentials struct {
	// User is the username for authenticating with the switch.
	User string
	// Password is the password for authenticating with the switch.
	Password string
	// SwitchIP is the IP address of the network switch.
	SwitchIP string
}

// NewSwitchCredentials is the constructor for the SwitchCredentials object.
func NewSwitchCredentials() (*SwitchCredentials, error) {
	user, err := NetConfig.GetSwitchUser()
	if err != nil {
		return nil, err
	}

	pass, err := NetConfig.GetSwitchPass()
	if err != nil {
		return nil, err
	}

	ipAddress, err := NetConfig.GetSwitchIP()
	if err != nil {
		return nil, err
	}

	return &SwitchCredentials{
		User:     user,
		Password: pass,
		SwitchIP: ipAddress,
	}, nil
}
