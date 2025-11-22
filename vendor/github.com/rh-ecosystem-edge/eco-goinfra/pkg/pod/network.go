package pod

import (
	"fmt"
	"net/netip"

	multus "gopkg.in/k8snetworkplumbingwg/multus-cni.v4/pkg/types"
	"k8s.io/klog/v2"
)

// StaticAnnotation defines network annotation for pod object.
func StaticAnnotation(name string) *multus.NetworkSelectionElement {
	klog.V(100).Infof("Build static network annotation for pod object with name %s", name)

	if name == "" {
		klog.V(100).Info("The name of the StaticAnnotation is empty")

		return nil
	}

	return &multus.NetworkSelectionElement{
		Name: name,
	}
}

// StaticIPAnnotation defines static ip address network annotation for pod object.
func StaticIPAnnotation(name string, ipAddr []string) []*multus.NetworkSelectionElement {
	klog.V(100).Infof("Build static IP network annotation for pod object with name %s and ip addresses %v",
		name, ipAddr)

	if name == "" {
		klog.V(100).Info("The name of the StaticIPAnnotation is empty")

		return nil
	}
	// Add new function that doesn't use IP address.
	// Uncomment the following validation when the new function is added.
	// if len(ipAddr) == 0 {
	//	klog.V(100).Info("The ip address list of the StaticIPAnnotation is empty")
	//
	//	return nil
	//}

	if len(ipAddr) > 0 && !ipValid(ipAddr) {
		return nil
	}

	return []*multus.NetworkSelectionElement{
		{
			Name:      name,
			IPRequest: ipAddr,
		},
	}
}

// StaticIPAnnotationWithInterfaceAndNamespace defines static ip address, interface name and namespace
// network annotation for pod object.
func StaticIPAnnotationWithInterfaceAndNamespace(
	name, namespace, intName string, ipAddr []string) []*multus.NetworkSelectionElement {
	klog.V(100).Infof("Build static network ip annotation with interface for pod object with "+
		"name: %s, namespace: %s, interfaceName: %s and ipAddress: %v", name, namespace, intName, ipAddr)

	if intName == "" {
		klog.V(100).Info("The interface name of the pod's static IP annotation with namespace is empty")

		return nil
	}

	baseAnnotation := StaticIPAnnotationWithNamespace(name, namespace, ipAddr)

	if baseAnnotation == nil {
		return nil
	}

	baseAnnotation[0].InterfaceRequest = intName

	return baseAnnotation
}

// StaticIPAnnotationWithMacAddress defines static ip address and static macaddress network annotation for pod object.
func StaticIPAnnotationWithMacAddress(name string, ipAddr []string, macAddr string) []*multus.NetworkSelectionElement {
	klog.V(100).Infof("Build static ip network annotation for pod object with "+
		"name: %s, ip addresses: %v, mac address: %s", name, ipAddr, macAddr)

	baseAnnotation := StaticIPAnnotation(name, ipAddr)

	// Add new function that doesn't use mac address.
	// Uncomment the following validation when the new function is added.
	// if macAddr == "" {
	//	klog.V(100).Info("The mac address of the pod's static IP annotation empty")
	//
	//	return nil
	//}

	if baseAnnotation == nil {
		return nil
	}

	baseAnnotation[0].MacRequest = macAddr

	return baseAnnotation
}

// StaticIPAnnotationWithNamespace defines static ip address and namespace network annotation for pod object.
func StaticIPAnnotationWithNamespace(name, namespace string, ipAddr []string) []*multus.NetworkSelectionElement {
	klog.V(100).Infof("Build static ip network annotation for pod object with "+
		"name: %s, namespace: %s, ip addresses: %v", name, namespace, ipAddr)

	if namespace == "" {
		klog.V(100).Info("The namespace of the pod's static IP annotation with namespace is empty")

		return nil
	}

	baseAnnotation := StaticIPAnnotation(name, ipAddr)

	if baseAnnotation == nil {
		return nil
	}

	baseAnnotation[0].Namespace = namespace

	return baseAnnotation
}

// StaticIPAnnotationWithMacAndNamespace defines static ip address and namespace, mac address network annotation
// for pod object.
func StaticIPAnnotationWithMacAndNamespace(name, namespace, macAddr string) []*multus.NetworkSelectionElement {
	klog.V(100).Infof("Build static ip network annotation for pod object with "+
		"name: %s, namespace: %s, mac address: %s", name, namespace, macAddr)

	// Add new function that doesn't use mac address.
	// Uncomment the following validation when the new function is added.
	// if macAddr == "" {
	//	klog.V(100).Info("The mac address of the pod's static IP annotation empty")
	//
	//	return nil
	//}

	if namespace == "" {
		klog.V(100).Info("The namespace of the pod's static IP annotation with namespace is empty")

		return nil
	}

	baseAnnotation := StaticAnnotation(name)
	if baseAnnotation == nil {
		return nil
	}

	baseAnnotation.Namespace = namespace
	baseAnnotation.MacRequest = macAddr

	return []*multus.NetworkSelectionElement{baseAnnotation}
}

// StaticIPAnnotationWithInterfaceMacAndNamespace defines static ip address and namespace, interface name,
// mac address network annotation for pod object.
func StaticIPAnnotationWithInterfaceMacAndNamespace(
	name, namespace, intName, macAddr string) []*multus.NetworkSelectionElement {
	klog.V(100).Infof("Build static network ip annotation for pod object with "+
		"name: %s, namespace: %s, interfaceName: %s and mac address: %v", name, namespace, intName, macAddr)

	if intName == "" {
		klog.V(100).Info("The interface name of the pod's static IP annotation with namespace is empty")

		return nil
	}

	baseAnnotation := StaticIPAnnotationWithMacAndNamespace(name, namespace, macAddr)

	if baseAnnotation == nil {
		return nil
	}

	baseAnnotation[0].InterfaceRequest = intName

	return baseAnnotation
}

// StaticIPBondAnnotationWithInterface defines static name for bonded interfaces and name, interface and IP for the
// main bond int.
func StaticIPBondAnnotationWithInterface(
	bondNadName, bondIntName string, sriovNetworkNameList, ipAddrBond []string) []*multus.NetworkSelectionElement {
	klog.V(100).Infof("Build static network bond ip annotation for pod object with "+
		"name: %s, bond interface name: %s, sriov network name list: %v and ip addresses bond: %v",
		bondIntName, bondIntName, sriovNetworkNameList, ipAddrBond)

	if bondIntName == "" {
		klog.V(100).Info("The bond interface name of the StaticIPBondAnnotationWithInterface is empty")

		return nil
	}

	if len(sriovNetworkNameList) == 0 {
		klog.V(100).Info("The sriov network name list of the StaticIPBondAnnotationWithInterface is empty")

		return nil
	}

	if len(ipAddrBond) == 0 {
		klog.V(100).Info("The ip address list of the StaticIPBondAnnotationWithInterface is empty")

		return nil
	}

	if !ipValid(ipAddrBond) {
		return nil
	}

	var annotation []*multus.NetworkSelectionElement

	for _, sriovNetName := range sriovNetworkNameList {
		staticAnnotation := StaticAnnotation(sriovNetName)
		if staticAnnotation == nil {
			return nil
		}

		annotation = append(annotation, staticAnnotation)
	}

	bond := StaticIPAnnotation(bondNadName, ipAddrBond)
	if bond == nil {
		return nil
	}

	bond[0].InterfaceRequest = bondIntName

	return append(annotation, bond[0])
}

// StaticIPMultiNetDualStackAnnotation defines network annotation for multiple interfaces with dual stack addresses.
func StaticIPMultiNetDualStackAnnotation(sriovNets, ipAddr []string) ([]*multus.NetworkSelectionElement, error) {
	klog.V(100).Infof("Build static dual-stack network ip annotation for pod object with "+
		"sriovNets: %v, ipAddr: %v", sriovNets, ipAddr)

	if len(sriovNets) == 0 {
		klog.V(100).Info("sriovNets cannot be empty")

		return nil, fmt.Errorf("sriovNets []string cannot be empty")
	}

	annotation := []*multus.NetworkSelectionElement{}

	// Verify ipAddr has an even number of IP addresses and not empty.
	if len(ipAddr) == 0 || len(ipAddr)%2 != 0 {
		klog.V(100).Info("ipAddr needs to contain an even number of IP addresses")

		return nil, fmt.Errorf("ipAddr []string cannot be empty or an odd number")
	}

	if !ipValid(ipAddr) {
		klog.V(100).Info("ipAddr is in invalid format")

		return nil, fmt.Errorf("ipAddr []string contain invalid ip address")
	}

	for i, sriovNetName := range sriovNets {
		if i*2+1 < len(ipAddr) {
			annotation = append(annotation, StaticIPAnnotation(sriovNetName, []string{ipAddr[i*2], ipAddr[i*2+1]})...)
		}
	}

	return annotation, nil
}

func ipValid(ipAddrBond []string) bool {
	for _, ipAddr := range ipAddrBond {
		_, err := netip.ParsePrefix(ipAddr)
		if err != nil {
			_, err = netip.ParseAddr(ipAddr)
			if err != nil {
				klog.V(100).Infof("The ip address %s in ip address list is invalid", ipAddr)

				return false
			}
		}
	}

	return true
}
