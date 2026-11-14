package metal3day2

import (
	"runtime"
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/oransuite"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
	_ "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/metal3-day2/tests"
)

var _, currentFile, _, _ = runtime.Caller(0)

func TestMetal3Day2(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	oransuite.Run(t, "RAN O-RAN Metal3 Day2 Suite", currentFile, tsparams.Metal3Day2Labels)
}
