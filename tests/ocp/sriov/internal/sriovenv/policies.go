package sriovenv

import (
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/sriovoperator"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/ocpsriovinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/ocp/sriov/internal/tsparams"
	"k8s.io/klog/v2"
)

const logLevelDebug = "debug"

// ValidateSriovInterfaces checks that the requested interfaces (from env) exist on every worker
// in workerNodeList. This ensures "Different Node" and other multi-worker tests do not fail
// later when scheduling on a worker that does not expose the requested PF names.
func ValidateSriovInterfaces(workerNodeList []*nodes.Builder, requestedNumber int) error {
	requestedSriovInterfaceList, err := SriovOcpConfig.GetSriovInterfaces(requestedNumber)
	if err != nil {
		return err
	}

	for _, worker := range workerNodeList {
		availableUpSriovInterfaces, err := sriov.NewNetworkNodeStateBuilder(APIClient,
			worker.Definition.Name, SriovOcpConfig.OcpSriovOperatorNamespace).GetUpNICs()
		if err != nil {
			return fmt.Errorf("failed to get SR-IOV devices from node %s: %w", worker.Definition.Name, err)
		}

		var validCount int

		for _, availableUpSriovInterface := range availableUpSriovInterfaces {
			for _, requestedSriovInterface := range requestedSriovInterfaceList {
				if availableUpSriovInterface.Name == requestedSriovInterface {
					validCount++

					break
				}
			}
		}

		if validCount < requestedNumber {
			return fmt.Errorf("requested interfaces %v are not all present on node %s (found %d of %d)",
				requestedSriovInterfaceList, worker.Definition.Name, validCount, requestedNumber)
		}
	}

	return nil
}

// CreateSriovNetworkAndWaitForNADCreation creates a SriovNetwork and waits for NAD Creation on the test namespace.
func CreateSriovNetworkAndWaitForNADCreation(sNet *sriov.NetworkBuilder, timeout time.Duration) error {
	klog.V(90).Infof("Creating SriovNetwork %s and waiting for net-attach-def to be created", sNet.Definition.Name)

	sriovNetwork, err := sNet.Create()
	if err != nil {
		return err
	}

	return WaitForNADCreation(sriovNetwork.Object.Name, TargetNamespaceOf(sriovNetwork), timeout)
}

// CreateSriovNetworkWithStaticIPAM creates an SR-IOV network with static IPAM, IP address, and MAC address support.
func CreateSriovNetworkWithStaticIPAM(name, resourceName string) error {
	klog.V(90).Infof("Creating SR-IOV network %s with static IPAM", name)

	networkBuilder := sriov.NewNetworkBuilder(
		APIClient, name, SriovOcpConfig.OcpSriovOperatorNamespace,
		tsparams.TestNamespaceName, resourceName).
		WithStaticIpam().
		WithIPAddressSupport().
		WithMacAddressSupport().
		WithLogLevel(logLevelDebug)

	return CreateSriovNetworkAndWaitForNADCreation(networkBuilder, tsparams.NADWaitTimeout)
}

// CreateSriovNetworkWithWhereaboutsIPAM creates an SR-IOV network with whereabouts IPAM for dynamic IP assignment.
// ipRange should be in CIDR notation (e.g., "2001:100::/64" for IPv6 or "192.168.1.0/24" for IPv4).
// gateway is used for single-stack only. Dual-stack uses ranges without gateway (ipv6Gateway is ignored).
func CreateSriovNetworkWithWhereaboutsIPAM(
	name,
	resourceName,
	ipRange,
	gateway,
	networkName,
	ipv6Range string,
) error {
	klog.V(90).Infof("Creating SR-IOV network %s with whereabouts IPAM, range %s, gateway %s",
		name, ipRange, gateway)

	networkBuilder := sriov.NewNetworkBuilder(
		APIClient, name, SriovOcpConfig.OcpSriovOperatorNamespace,
		tsparams.TestNamespaceName, resourceName)

	if ipv6Range != "" {
		return fmt.Errorf("dual-stack whereabouts IPAM is not implemented for OCP SR-IOV helpers")
	}

	networkBuilder = networkBuilder.WithWhereaboutsIPAM(ipRange, gateway, "", networkName)

	return CreateSriovNetworkAndWaitForNADCreation(networkBuilder, tsparams.NADWaitTimeout)
}

// CreateSriovNetworkWithVLANAndWhereabouts creates an SR-IOV network with Whereabouts IPAM and VLAN tagging.
func CreateSriovNetworkWithVLANAndWhereabouts(
	name,
	resourceName string,
	vlanID uint16,
	ipRange,
	gateway,
	ipv6Range string,
) error {
	klog.V(90).Infof("Creating SR-IOV network %s with Whereabouts IPAM, VLAN %d, range %s",
		name, vlanID, ipRange)

	networkBuilder := sriov.NewNetworkBuilder(
		APIClient, name, SriovOcpConfig.OcpSriovOperatorNamespace,
		tsparams.TestNamespaceName, resourceName).WithVLAN(vlanID)

	if ipv6Range != "" {
		return fmt.Errorf("dual-stack whereabouts IPAM is not implemented for OCP SR-IOV helpers")
	}

	networkBuilder = networkBuilder.WithWhereaboutsIPAM(ipRange, gateway, "", "")

	return CreateSriovNetworkAndWaitForNADCreation(networkBuilder, tsparams.NADWaitTimeout)
}

// CreateAllSriovPolicies creates all SR-IOV policies for testing.
// It creates policies for PF1 and PF2 at two MTU sizes (small and large).
// VF allocation: 10 total VFs per PF, VFs 0-4 for small MTU, VFs 5-9 for large MTU.
// mtuSmall is typically 500 for IPv4 or 1280 for IPv6.
func CreateAllSriovPolicies(
	pf1,
	pf2,
	resourcePF1SmallMTU,
	resourcePF1LargeMTU,
	resourcePF2SmallMTU,
	resourcePF2LargeMTU,
	policyPrefix string,
	mtuSmall,
	mtuLarge int,
) error {
	klog.V(90).Infof("Creating SR-IOV policies for testing")

	const (
		vfStartSmallMTU = 0
		vfEndSmallMTU   = 4
		vfStartLargeMTU = 5
		vfEndLargeMTU   = 9
		totalVFs        = 10
	)

	if err := CreateSriovPolicy(
		fmt.Sprintf("%s-policy-pf1-mtu%d", policyPrefix, mtuSmall),
		resourcePF1SmallMTU, pf1, mtuSmall,
		vfStartSmallMTU, vfEndSmallMTU, totalVFs); err != nil {
		return fmt.Errorf("failed to create PF1 MTU%d policy: %w", mtuSmall, err)
	}

	if err := CreateSriovPolicy(
		fmt.Sprintf("%s-policy-pf1-mtu%d", policyPrefix, mtuLarge),
		resourcePF1LargeMTU, pf1, mtuLarge,
		vfStartLargeMTU, vfEndLargeMTU, totalVFs); err != nil {
		return fmt.Errorf("failed to create PF1 MTU%d policy: %w", mtuLarge, err)
	}

	if err := CreateSriovPolicy(
		fmt.Sprintf("%s-policy-pf2-mtu%d", policyPrefix, mtuSmall),
		resourcePF2SmallMTU, pf2, mtuSmall,
		vfStartSmallMTU, vfEndSmallMTU, totalVFs); err != nil {
		return fmt.Errorf("failed to create PF2 MTU%d policy: %w", mtuSmall, err)
	}

	if err := CreateSriovPolicy(
		fmt.Sprintf("%s-policy-pf2-mtu%d", policyPrefix, mtuLarge),
		resourcePF2LargeMTU, pf2, mtuLarge,
		vfStartLargeMTU, vfEndLargeMTU, totalVFs); err != nil {
		return fmt.Errorf("failed to create PF2 MTU%d policy: %w", mtuLarge, err)
	}

	if err := sriovoperator.WaitForSriovAndMCPStable(
		APIClient,
		tsparams.MCOWaitTimeout,
		tsparams.DefaultStableDuration,
		SriovOcpConfig.WorkerLabelEnvVar,
		SriovOcpConfig.OcpSriovOperatorNamespace); err != nil {
		return fmt.Errorf("failed to wait for SR-IOV and MCP stability: %w", err)
	}

	return nil
}

// CreateSriovPolicy creates a single SR-IOV policy without waiting for MCP stability.
func CreateSriovPolicy(
	name,
	resourceName,
	pfName string,
	mtu,
	vfStart,
	vfEnd,
	numVFs int,
) error {
	klog.V(90).Infof("Creating SR-IOV policy %s", name)

	_, err := sriov.NewPolicyBuilder(
		APIClient,
		name,
		SriovOcpConfig.OcpSriovOperatorNamespace,
		resourceName,
		numVFs,
		[]string{pfName},
		SriovOcpConfig.WorkerLabelMap).
		WithMTU(mtu).
		WithVFRange(vfStart, vfEnd).
		Create()

	return err
}

// CreateSriovNetworksForBothMTUs creates SR-IOV networks for two MTU configurations.
func CreateSriovNetworksForBothMTUs(
	networkNameSmallMTU,
	networkNameLargeMTU,
	resourceSmallMTU,
	resourceLargeMTU string,
) error {
	klog.V(90).Infof("Creating SR-IOV networks for both MTU sizes")

	err := CreateSriovNetworkWithStaticIPAM(networkNameSmallMTU, resourceSmallMTU)
	if err != nil {
		return fmt.Errorf("failed to create SR-IOV network for small MTU: %w", err)
	}

	err = CreateSriovNetworkWithStaticIPAM(networkNameLargeMTU, resourceLargeMTU)
	if err != nil {
		return fmt.Errorf("failed to create SR-IOV network for large MTU: %w", err)
	}

	return nil
}
