package sriovocpenv

import (
	"context"
	"encoding/json"
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
	rpcGetInterfaceConfig = "<get-configuration><configuration><interfaces><interface><name>%s</name></interface>" +
		"</interfaces></configuration></get-configuration>"
	rpcApplyConfig = "<load-configuration format=\"xml\" action=\"replace\">%s</load-configuration>"
	rpcCommit      = "<commit-configuration/>"
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

// InterfaceStatus collects Juniper interface operational data from JSON output.
type InterfaceStatus struct {
	InterfaceInformation []struct {
		PhysicalInterface []struct {
			OperStatus []struct {
				Data string `json:"data"`
			} `json:"oper-status"`
		} `json:"physical-interface"`
	} `json:"interface-information"`
}

// IsSwitchInterfaceUp reports whether the given switch interface oper-status is up.
func (j *Junos) IsSwitchInterfaceUp(switchInterface string) (bool, error) {
	jsonOutput, err := j.RunCommand(fmt.Sprintf("show interfaces %s", switchInterface))
	if err != nil {
		return false, err
	}

	var interfaceStatus InterfaceStatus

	if err := json.Unmarshal([]byte(jsonOutput), &interfaceStatus); err != nil {
		return false, err
	}

	if len(interfaceStatus.InterfaceInformation) == 0 ||
		len(interfaceStatus.InterfaceInformation[0].PhysicalInterface) == 0 ||
		len(interfaceStatus.InterfaceInformation[0].PhysicalInterface[0].OperStatus) == 0 {
		return false, fmt.Errorf("no oper-status for switch interface %s", switchInterface)
	}

	return interfaceStatus.InterfaceInformation[0].PhysicalInterface[0].OperStatus[0].Data == "up", nil
}

// GetInterfaceConfig returns configuration for given interface.
func (j *Junos) GetInterfaceConfig(switchInterface string) (string, error) {
	command := fmt.Sprintf(rpcGetInterfaceConfig, switchInterface)

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

// ApplyConfigInterface replaces a switch interface configuration from saved XML.
func (j *Junos) ApplyConfigInterface(config string) error {
	command := fmt.Sprintf(rpcApplyConfig, config)

	reply, err := j.Session.Exec(netconf.RawMethod(command))
	if err != nil {
		return err
	}

	err = j.Commit()
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

// SaveInterfaceConfigs saves the configuration of multiple switch interfaces.
func (j *Junos) SaveInterfaceConfigs(interfaces []string) ([]string, error) {
	if len(interfaces) == 0 {
		return nil, fmt.Errorf("interfaces list cannot be empty")
	}

	var configs []string

	for _, iface := range interfaces {
		config, err := j.GetInterfaceConfig(iface)
		if err != nil {
			klog.V(90).Infof("Failed to get config for interface %s: %v", iface, err)

			continue
		}

		if config != "" {
			configs = append(configs, config)
		}
	}

	return configs, nil
}

// RestoreInterfaceConfigs restores the configuration of multiple switch interfaces.
func (j *Junos) RestoreInterfaceConfigs(configs []string) error {
	if len(configs) == 0 {
		return fmt.Errorf("configs list cannot be empty")
	}

	for _, config := range configs {
		err := j.ApplyConfigInterface(config)
		if err != nil {
			return fmt.Errorf("failed to restore interface config: %w", err)
		}
	}

	return nil
}

// DisableLACP removes LACP/802.3ad configuration from switch interfaces.
func (j *Junos) DisableLACP(lacpInterfaces, physicalInterfaces []string) error {
	if len(lacpInterfaces) == 0 {
		return fmt.Errorf("lacpInterfaces list cannot be empty")
	}

	if len(physicalInterfaces) == 0 {
		return fmt.Errorf("physicalInterfaces list cannot be empty")
	}

	var commands []string

	for _, physicalInterface := range physicalInterfaces {
		commands = append(commands, fmt.Sprintf("delete interfaces %s ether-options 802.3ad", physicalInterface))
	}

	for _, lacpInterface := range lacpInterfaces {
		commands = append(commands, fmt.Sprintf("delete interfaces %s", lacpInterface))
	}

	for _, physicalInterface := range physicalInterfaces {
		commands = append(commands, fmt.Sprintf("delete interfaces %s", physicalInterface))
	}

	err := j.Config(commands)
	if err != nil {
		return fmt.Errorf("failed to execute switch cleanup commands: %w", err)
	}

	return nil
}
