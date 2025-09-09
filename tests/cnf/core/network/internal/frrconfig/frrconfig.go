package frrconfig

import (
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
)

// DefineBaseConfig creates a map of strings for the frr configuration.
func DefineBaseConfig(daemonsConfig, frrConfig, vtyShConfig string) map[string]string {
	configMapData := make(map[string]string)
	configMapData["daemons"] = daemonsConfig
	configMapData["frr.conf"] = frrConfig
	configMapData["vtysh.conf"] = vtyShConfig

	return configMapData
}

// CreateStaticIPAnnotations creates a static ip annotation used together with the nad in a pod for IP configuration.
func CreateStaticIPAnnotations(internalNADName, externalNADName string, internalIPAddresses,
	externalIPAddresses []string) []*types.NetworkSelectionElement {
	ipAnnotation := pod.StaticIPAnnotation(internalNADName, internalIPAddresses)
	ipAnnotation = append(ipAnnotation,
		pod.StaticIPAnnotation(externalNADName, externalIPAddresses)...)

	return ipAnnotation
}
