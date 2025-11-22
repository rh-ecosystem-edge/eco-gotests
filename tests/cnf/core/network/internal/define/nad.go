package define

import (
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nad"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/internal/coreparams"
	"k8s.io/klog/v2"
)

// MasterNadPlugin sets NetworkAttachmentDefinition master plugin based on given input.
func MasterNadPlugin(nadName, mode string, ipam *nad.IPAM, masterInterface ...string) (*nad.MasterPlugin, error) {
	klog.V(90).Infof("Defining master nad plugin with the following parameters name %s, mode %s, ipam %v",
		nadName, mode, ipam)

	macVlan := nad.NewMasterMacVlanPlugin(nadName)

	if len(masterInterface) > 0 {
		klog.V(90).Infof("Attaching master nad plugin to interface %s", masterInterface[0])
		macVlan.WithMasterInterface(masterInterface[0])
	}

	macVlan.WithMode(mode).WithIPAM(ipam)

	return macVlan.GetMasterPluginConfig()
}

// TapNad defines and creates tap NetworkAttachmentDefinition on a cluster.
func TapNad(
	apiClient *clients.Settings,
	name string,
	nsname string,
	user int,
	group int,
	sysctlConfig map[string]string) (*nad.Builder, error) {
	plugins := []nad.Plugin{*nad.TapPlugin(user, group, true)}

	if sysctlConfig != nil {
		plugins = append(plugins, *nad.TuningSysctlPlugin(true, sysctlConfig))
	}

	tap, err := nad.NewBuilder(apiClient, name, nsname).WithPlugins(name, &plugins).Create()
	if err != nil {
		return nil, err
	}

	return tap, nil
}

// MacVlanNad defines and creates mac-vlan NetworkAttachmentDefinition on a cluster.
func MacVlanNad(apiClient *clients.Settings, name, nsName, intName string, ipam *nad.IPAM) (*nad.Builder, error) {
	masterPlugin, err := nad.NewMasterMacVlanPlugin(name).WithMasterInterface(intName).
		WithIPAM(ipam).WithLinkInContainer().GetMasterPluginConfig()
	if err != nil {
		return nil, err
	}

	return createNadWithMasterPlugin(apiClient, name, nsName, masterPlugin)
}

// VlanNad defines and creates Vlan NetworkAttachmentDefinition on a cluster.
func VlanNad(
	apiClient *clients.Settings, name, nsName, intName string, vlanID uint16, ipam *nad.IPAM) (*nad.Builder, error) {
	masterPlugin, err := nad.NewMasterVlanPlugin(name, vlanID).WithMasterInterface(intName).
		WithIPAM(ipam).WithLinkInContainer().GetMasterPluginConfig()
	if err != nil {
		return nil, err
	}

	return createNadWithMasterPlugin(apiClient, name, nsName, masterPlugin)
}

// IPVlanNad defines and creates IP-Vlan NetworkAttachmentDefinition on a cluster.
func IPVlanNad(apiClient *clients.Settings, name, nsName, intName string, ipam *nad.IPAM) (*nad.Builder, error) {
	masterPlugin, err := nad.NewMasterIPVlanPlugin(name).WithMasterInterface(intName).WithIPAM(ipam).
		WithLinkInContainer().GetMasterPluginConfig()
	if err != nil {
		return nil, err
	}

	return createNadWithMasterPlugin(apiClient, name, nsName, masterPlugin)
}

func createNadWithMasterPlugin(
	apiClient *clients.Settings, name, nsName string, masterPlugin *nad.MasterPlugin) (*nad.Builder, error) {
	createdNad, err := nad.NewBuilder(apiClient, name, nsName).WithMasterPlugin(masterPlugin).Create()
	if err != nil {
		return nil, err
	}

	return createdNad, nil
}

// HostDeviceNad defines and creates HostDeviceNetworkAttachmentDefinition on a cluster.
func HostDeviceNad(
	apiClient *clients.Settings,
	name,
	interfaceName,
	nsName string) (*nad.Builder, error) {
	masterPlugin, err := nad.NewMasterHostDevicePlugin(name, interfaceName).GetHostDevicePluginConfig()
	if err != nil {
		return nil, err
	}

	return createNadWithMasterPlugin(apiClient, name, nsName, masterPlugin)
}

// CreateExternalNad creates an external network-attachment-definition using the br-ex interface.
func CreateExternalNad(apiClient *clients.Settings, name, testNameSpace string) error {
	klog.V(90).Info("Creating external BR-EX NetworkAttachmentDefinition")

	// Define the master NAD plugin
	macVlanPlugin, err := MasterNadPlugin(coreparams.OvnExternalBridge, "bridge", nad.IPAMStatic())
	if err != nil {
		klog.V(90).Infof("Failed to define master NAD plugin: %v", err)

		return err
	}

	// Create the NetworkAttachmentDefinition
	_, err = createNadWithMasterPlugin(apiClient, name, testNameSpace, macVlanPlugin)
	if err != nil {
		klog.V(90).Infof("Failed to create external NetworkAttachmentDefinition: %v", err)

		return err
	}

	klog.V(90).Infof("Successfully created external NetworkAttachmentDefinition: %s", name)

	return nil
}
