package nethelper

import . "gitlab.cee.redhat.com/cnf/cnf-gotests/test/helper"

type SwitchCredentials struct {
	User     string
	Password string
	SwitchIP string
}

// NewSwitchCredentials is the constructor for the SwitchCredentials object.
func NewSwitchCredentials() (*SwitchCredentials, error) {
	user, err := Config.GetSwitchUser()
	if err != nil {
		return nil, err
	}

	pass, err := Config.GetSwitchPass()
	if err != nil {
		return nil, err
	}

	ipAddress, err := Config.GetSwitchIP()
	if err != nil {
		return nil, err
	}

	return &SwitchCredentials{
		User:     user,
		Password: pass,
		SwitchIP: ipAddress,
	}, nil
}
