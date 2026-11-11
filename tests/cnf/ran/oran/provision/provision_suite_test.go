package provision

import (
	"runtime"
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/oransuite"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
	_ "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/provision/tests"
)

var _, currentFile, _, _ = runtime.Caller(0)

func TestProvision(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	oransuite.Run(t, "RAN O-RAN Provision Suite", currentFile, tsparams.ProvisionLabels)
}
