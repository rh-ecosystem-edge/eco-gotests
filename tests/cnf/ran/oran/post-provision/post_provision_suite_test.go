package postprovision

import (
	"runtime"
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/oransuite"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
	_ "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/post-provision/tests"
)

var _, currentFile, _, _ = runtime.Caller(0)

func TestPostProvision(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	oransuite.Run(t, "RAN O-RAN Post-provision Suite", currentFile, tsparams.PostProvisionLabels)
}
