package preprovision

import (
	"runtime"
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/oransuite"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
	_ "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/pre-provision/tests"
)

var _, currentFile, _, _ = runtime.Caller(0)

func TestPreProvision(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	oransuite.Run(t, "RAN O-RAN Pre-provision Suite", currentFile, tsparams.PreProvisionLabels)
}
