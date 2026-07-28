package coreinittools

import (
	"github.com/onsi/ginkgo/v2" //nolint:depguard // necessary for logging
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/internal/coreconfig"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/inittools"
	"k8s.io/klog/v2"
)

var (
	// APIClient provides API access to the cluster.
	APIClient *clients.Settings
	// CoreConfig provides access to core configuration parameters.
	CoreConfig *coreconfig.CoreConfig
)

func init() {
	klog.LogToStderr(false)
	klog.SetOutput(ginkgo.GinkgoWriter)

	CoreConfig = coreconfig.NewCoreConfig()
	APIClient = inittools.APIClient
}
