package sriovocpenv

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Juniper/go-netconf/netconf"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/ocpsriovinittools"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

// Junos creates a struct to interact with a Juniper switch via NETCONF.
type Junos struct {
	Session *netconf.Session
}

// SwitchCredentials holds the credentials for connecting to a lab switch.
type SwitchCredentials struct {
	User     string
	Password string
	SwitchIP string
}

// NewSwitchCredentials reads switch connection credentials from SriovOcpConfig.
func NewSwitchCredentials() (*SwitchCredentials, error) {
	user, password, switchIP, err := SriovOcpConfig.GetSwitchCredentials()
	if err != nil {
		return nil, err
	}

	return &SwitchCredentials{
		User:     user,
		Password: password,
		SwitchIP: switchIP,
	}, nil
}

// NewJunosSession establishes a new NETCONF connection to a Junos device.
func NewJunosSession(host, user, password string) (*Junos, error) {
	var session *netconf.Session

	err := wait.PollUntilContextTimeout(context.TODO(), 30*time.Second, 120*time.Second, true,
		func(ctx context.Context) (done bool, err error) {
			session, err = netconf.DialSSH(host, netconf.SSHConfigPassword(user, password))
			if err != nil {
				klog.V(90).Infof("Error: %v", err)

				return false, nil
			}

			return true, nil
		})
	if err != nil {
		return nil, err
	}

	return &Junos{
		Session: session,
	}, nil
}

// Close disconnects the session to the device.
func (j *Junos) Close() {
	j.Session.Transport.Close()
}

// RunCommand executes any operational mode command, such as "show" or "request".
func (j *Junos) RunCommand(cmd string) (string, error) {
	command := fmt.Sprintf("<command format=\"json\">%s</command>", cmd)

	reply, err := j.Session.Exec(netconf.RawMethod(command))
	if err != nil {
		return "", err
	}

	if len(reply.Errors) > 0 {
		return "", errors.New(reply.Errors[0].Message)
	}

	if reply.Data == "" {
		return "", errors.New("no output available, please check the syntax of your command")
	}

	return reply.Data, nil
}
