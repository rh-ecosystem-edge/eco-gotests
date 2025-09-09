package o_cloud_system_test

import (
	. "github.com/onsi/ginkgo/v2"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/o-cloud/internal/ocloudcommon"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/o-cloud/internal/ocloudparams"
)

var _ = Describe(
	"Image Based Installer based SNO provisioning Test Suite",
	Ordered,
	ContinueOnFailure,
	Label(ocloudparams.Label), func() {
		Context("Configured hub cluster", Label("ocloud-ibi-provisioning"), func() {
			It("Verifies the successful provisioning of a single SNO cluster using Image Based Installer",
				ocloudcommon.VerifySuccessfulIbiSnoProvisioning)

			It("Verifies the failed provisioning of a single SNO cluster using Image Based Installer",
				ocloudcommon.VerifyFailedIbiSnoProvisioning)
		})
	})
