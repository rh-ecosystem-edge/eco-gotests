package raninittools

import (
	"github.com/onsi/ginkgo/v2" //nolint:depguard // necessary for logging
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/bmc"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	"k8s.io/klog/v2"
)

var (
	// HubAPIClient provides API access to the first spoke cluster.
	HubAPIClient *clients.Settings
	// Spoke1APIClient provides API access to cluster.
	Spoke1APIClient *clients.Settings
	// Spoke2APIClient provides API access to the second spoke cluster.
	Spoke2APIClient *clients.Settings
	// RANConfig provides access to configuration.
	RANConfig *ranconfig.RANConfig
	// BMCClient provides access to the BMC. Nil when BMC configs are not provided.
	BMCClient *bmc.BMC
)

func init() {
	// If LogToStderr is true, klog will ignore the output and only write to stderr. Instead, we want to write to
	// the GinkgoWriter, which is also written to stderr when using the -v flag.
	klog.LogToStderr(false)
	klog.SetOutput(ginkgo.GinkgoWriter)

	Spoke1APIClient = inittools.APIClient
	RANConfig = ranconfig.NewRANConfig()
	HubAPIClient = RANConfig.HubAPIClient
	Spoke2APIClient = RANConfig.Spoke2APIClient
	BMCClient = RANConfig.Spoke1BMC
}
