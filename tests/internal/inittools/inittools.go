package inittools

import (
	"flag"
	"os"

	"github.com/onsi/ginkgo/v2"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/config"
	"k8s.io/klog/v2"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	// APIClient provides access to cluster.
	APIClient *clients.Settings
	// GeneralConfig provides access to general configuration parameters.
	GeneralConfig *config.GeneralConfig
)

// init loads all variables automatically when this package is imported. Once package is imported a user has full
// access to all vars within init function. It is recommended to import this package using dot import.
func init() {
	// Work around bug in glog lib
	logf.SetLogger(zap.New(zap.WriteTo(ginkgo.GinkgoWriter), zap.UseDevMode(true)))

	// Skip loading config if running unit tests
	if os.Getenv("UNIT_TEST") == "true" {
		return
	}

	if GeneralConfig = config.NewConfig(); GeneralConfig == nil {
		klog.Fatalf("error to load general config")
	}

	klog.InitFlags(nil)

	_ = flag.Set("logtostderr", "true")
	_ = flag.Set("v", GeneralConfig.VerboseLevel)

	if APIClient = clients.New(""); APIClient == nil {
		if GeneralConfig.DryRun {
			return
		}

		klog.Exitf("can not load ApiClient. Please check your KUBECONFIG env var")
	}
}
