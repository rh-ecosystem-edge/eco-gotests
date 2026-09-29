package sriovocpenv

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
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

var (
	rpcConfigStringSet = "<load-configuration action=\"set\"" +
		" format=\"text\"><configuration-set>%s</configuration-set></load-configuration>"
	rpcCommit = "<commit-configuration/>"
)

type commitError struct {
	Path    string `xml:"error-path"`
	Element string `xml:"error-info>bad-element"`
	Message string `xml:"error-message"`
}

type commitResults struct {
	XMLName xml.Name      `xml:"commit-results"`
	Errors  []commitError `xml:"rpc-error"`
}

// Commit commits the configuration.
func (j *Junos) Commit() error {
	var errs commitResults

	reply, err := j.Session.Exec(netconf.RawMethod(rpcCommit))
	if err != nil {
		return err
	}

	if len(reply.Errors) > 0 {
		return errors.New(reply.Errors[0].Message)
	}

	err = xml.Unmarshal([]byte(reply.Data), &errs)
	if err != nil {
		return err
	}

	if errs.Errors != nil {
		for _, commitErr := range errs.Errors {
			if strings.Contains(commitErr.Message, "license") {
				continue
			}

			message := fmt.Sprintf("[%s]\n    %s\nError: %s", strings.Trim(commitErr.Path, "[\r\n]"),
				strings.Trim(commitErr.Element, "[\r\n]"), strings.Trim(commitErr.Message, "[\r\n]"))

			return errors.New(message)
		}
	}

	return nil
}

// Config sends commands to a Juniper switch.
func (j *Junos) Config(commands []string) error {
	command := fmt.Sprintf(rpcConfigStringSet, strings.Join(commands, "\n"))

	reply, err := j.Session.Exec(netconf.RawMethod(command))
	if err != nil {
		return err
	}

	if len(reply.Errors) > 0 {
		return errors.New(reply.Errors[0].Message)
	}

	err = j.Commit()
	if err != nil {
		return err
	}

	return nil
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
