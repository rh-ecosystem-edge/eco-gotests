package cmd

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

var (
	rpcConfigStringSet = "<load-configuration action=\"set\"" +
		" format=\"text\"><configuration-set>%s</configuration-set></load-configuration>"
	rpcGetInterfaceConfig = "<get-configuration><configuration><interfaces><interface><name>%s</name></interface>" +
		"</interfaces></configuration></get-configuration>"
	rpcApplyConfig = "<load-configuration format=\"xml\" action=\"replace\">%s</load-configuration>"
	rpcCommit      = "<commit-configuration/>"
	rpcCommandJSON = "<command format=\"json\">%s</command>"
)

type (
	// Junos creates a struct to retrieve output from the lab Juniper switch.
	Junos struct {
		Session *netconf.Session
	}
	// InterfaceStatus is struct that collects the data from the Juniper interfaces.
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
func NewSession(host, user, password string) (*Junos, error) {
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

// Commit commits the configuration.
func (j *Junos) Commit() error {
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

// Config sends commands to a Juniper switch.
func (j *Junos) Config(commands []string) error {
	command := fmt.Sprintf(rpcConfigStringSet, strings.Join(commands, "\n"))

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

// ApplyConfigInterface applies given interface configuration to a switch.
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

// RunCommand executes any operational mode command, such as "show" or "request".
func (j *Junos) RunCommand(cmd string) (string, error) {
	command := fmt.Sprintf(rpcCommandJSON, cmd)

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

// SaveInterfaceConfigs saves the configuration of multiple switch interfaces.
func (j *Junos) SaveInterfaceConfigs(interfaces []string) ([]string, error) {
	if len(interfaces) == 0 {
		return nil, fmt.Errorf("interfaces list cannot be empty")
	}

	var configs []string

	for _, iface := range interfaces {
		config, err := j.GetInterfaceConfig(iface)
		if err != nil {
			// Interface might not exist or have no config - that's okay, we'll just skip it
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

// DisableLACP removes LACP configuration from switch interfaces.
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
