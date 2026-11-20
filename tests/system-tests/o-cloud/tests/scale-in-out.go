package o_cloud_system_test

import (
	. "github.com/onsi/ginkgo/v2"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/o-cloud/internal/ocloudcommon"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/o-cloud/internal/ocloudparams"
)

var _ = Describe(
	"O-Cloud ProvisioningRequest Scale In/Out",
	Ordered,
	Label(ocloudparams.Label, "scale-in-out"), func() {
		It("scales in and restores one worker", Label("scale-one-worker"), ocloudcommon.VerifyScaleOneWorker)
		It("scales in and restores two workers", Label("scale-two-workers"), ocloudcommon.VerifyScaleTwoWorkers)
	})
