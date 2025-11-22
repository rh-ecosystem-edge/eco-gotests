package juniper

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Juniper/go-netconf/netconf"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

const (
	// SetAction represents the action to set a configuration.
	SetAction = "set"
	// DeleteAction represents the action to delete a configuration.
	DeleteAction = "delete"
)

var (
	rpcConfigStringSet = "<load-configuration action=\"set\"" +
		" format=\"text\"><configuration-set>%s</configuration-set></load-configuration>"
	rpcGetInterfaceConfig = "<get-configuration><configuration><interfaces><interface><name>%s</name></interface>" +
		"</interfaces></configuration></get-configuration>"
	rpcApplyConfig         = "<load-configuration format=\"xml\" action=\"replace\">%s</load-configuration>"
	rpcCommit              = "<commit-configuration/>"
	rpcCommandJSON         = "<command format=\"json\">%s</command>"
	rpcGetChassisInventory = "<get-chassis-inventory/>"
)

type (
	// JunosSession represents a connection to a Juniper network device.
	JunosSession struct {
		Session           *netconf.Session
		SwitchCredentials SwitchCredentials
	}

	// InterfaceStatus represents the status information of a network interface.
	InterfaceStatus struct {
		InterfaceInformation []struct {
			PhysicalInterface []struct {
				Name []struct {
					Data string `json:"data"`
				} `json:"name"`
				AdminStatus []struct {
					Data       string `json:"data"`
					Attributes struct {
						JunosFormat string `json:"junos:format"`
					} `json:"attributes"`
				} `json:"admin-status"`
				OperStatus []struct {
					Data string `json:"data"`
				} `json:"oper-status"`
				MTU []struct {
					Data string `json:"data"`
				} `json:"mtu"`
				Speed []struct {
					Data string `json:"data"`
				} `json:"speed"`
			} `json:"physical-interface"`
		} `json:"interface-information"`
	}

	commitError struct {
		Path    string `xml:"error-path"`
		Element string `xml:"error-info>bad-element"`
		Message string `xml:"error-message"`
	}

	commitResults struct {
		XMLName xml.Name      `xml:"commit-results"`
		Errors  []commitError `xml:"rpc-error"`
	}
)

// NewSession establishes a new connection to a Junos device that we will use
// to run our commands against.
func NewSession(host, user, password string) (*JunosSession, error) {
	klog.V(90).Infof("Creating a new session for host: %s", host)

	var session *netconf.Session

	err := wait.PollUntilContextTimeout(
		context.TODO(), 30*time.Second, 120*time.Second, true, func(ctx context.Context) (done bool, err error) {
			session, err = netconf.DialSSH(host, netconf.SSHConfigPassword(user, password))
			if err != nil {
				klog.V(90).Infof("Failed to open SSH: %s", err)

				return false, nil
			}

			return true, nil
		})
	if err != nil {
		return nil, err
	}

	return &JunosSession{
		Session:           session,
		SwitchCredentials: SwitchCredentials{SwitchIP: host, Password: password, User: user},
	}, nil
}

// Close disconnects the session to the device.
func (j *JunosSession) Close() {
	klog.V(90).Info("Closing session with switch")
	j.Session.Transport.Close()
}

// Config sends commands to a Juniper switch.
func (j *JunosSession) Config(commands []string) error {
	klog.V(90).Info("Sending configuration commands to a switch")

	err := j.openSessionIfNotExists()
	if err != nil {
		klog.V(90).Infof("Failed to open a session")

		return err
	}

	command := fmt.Sprintf(rpcConfigStringSet, strings.Join(commands, "\n"))

	reply, err := j.Session.Exec(netconf.RawMethod(command))
	if err != nil {
		return err
	}

	err = j.commit()
	if err != nil {
		return err
	}

	if reply.Errors != nil {
		for _, m := range reply.Errors {
			return errors.New(m.Message)
		}
	}

	return nil
}

// ApplyConfigInterface applies given interface configuration to a switch.
func (j *JunosSession) ApplyConfigInterface(config string) error {
	klog.V(90).Info("Applying switch interface configuration")

	err := j.openSessionIfNotExists()
	if err != nil {
		klog.V(90).Infof("Failed to open a session")

		return err
	}

	command := fmt.Sprintf(rpcApplyConfig, config)

	reply, err := j.Session.Exec(netconf.RawMethod(command))
	if err != nil {
		return err
	}

	err = j.commit()
	if err != nil {
		return err
	}

	if reply.Errors != nil {
		for _, m := range reply.Errors {
			return errors.New(m.Message)
		}
	}

	return nil
}

// RunOperationalCMD executes any operational mode command, such as "show" or "request".
func (j *JunosSession) RunOperationalCMD(cmd string) (string, error) {
	klog.V(90).Infof("Running command on a switch: %s", cmd)

	err := j.openSessionIfNotExists()
	if err != nil {
		klog.V(90).Infof("Failed to open a session")

		return "", err
	}

	command := fmt.Sprintf(rpcCommandJSON, cmd)

	return j.runCommand(command)
}

// GetInterfaceConfig returns configuration for given interface.
func (j *JunosSession) GetInterfaceConfig(switchInterface string) (string, error) {
	klog.V(90).Infof("Getting configuration for switch interface: %s", switchInterface)

	err := j.openSessionIfNotExists()
	if err != nil {
		klog.V(90).Infof("Failed to open a session")

		return "", err
	}

	command := fmt.Sprintf(rpcGetInterfaceConfig, switchInterface)

	return j.runCommand(command)
}

// getChassisInventory returns chassis inventory information.
func (j *JunosSession) getChassisInventory() (string, error) {
	klog.V(90).Infof("Getting Chassis inventory")

	return j.runCommand(rpcGetChassisInventory)
}

// commit commits the configuration.
func (j *JunosSession) commit() error {
	klog.V(90).Info("Committing switch configuration")

	var errs commitResults

	reply, err := j.Session.Exec(netconf.RawMethod(rpcCommit))
	if err != nil {
		return err
	}

	if reply.Errors != nil {
		for _, m := range reply.Errors {
			return errors.New(m.Message)
		}
	}

	err = xml.Unmarshal([]byte(reply.Data), &errs)
	if err != nil {
		return err
	}

	if errs.Errors != nil {
		for _, m := range errs.Errors {
			message := fmt.Sprintf("[%s]\n    %s\nError: %s", strings.Trim(m.Path, "[\r\n]"),
				strings.Trim(m.Element, "[\r\n]"), strings.Trim(m.Message, "[\r\n]"))

			return errors.New(message)
		}
	}

	return nil
}

// openSessionIfNotExists opens a new junosSession if current session doesn't exist.
func (j *JunosSession) openSessionIfNotExists() error {
	klog.V(90).Infof("Opening a new session if current session doesn't exist")

	if j == nil || !j.doesSessionExist() {
		klog.V(90).Infof("Current session doesn't exist, opening a new one")

		jnpr, err := NewSession(
			j.SwitchCredentials.SwitchIP,
			j.SwitchCredentials.User,
			j.SwitchCredentials.Password)
		if err != nil {
			return err
		}

		j.Session = jnpr.Session

		return nil
	}

	klog.V(90).Infof("Current session exists")

	return nil
}

func (j *JunosSession) runCommand(command string) (string, error) {
	reply, err := j.Session.Exec(netconf.RawMethod(command))
	if err != nil {
		return "", err
	}

	if reply.Errors != nil {
		for _, m := range reply.Errors {
			return "", errors.New(m.Message)
		}
	}

	if reply.Data == "" {
		return "", errors.New("no output available, please check the syntax of your command")
	}

	return reply.Data, nil
}

func (j *JunosSession) doesSessionExist() bool {
	klog.V(90).Infof("Checking that the SSH session %d exists", j.Session.SessionID)
	reply, err := j.getChassisInventory()

	if err != nil || reply == "" {
		return false
	}

	return true
}
