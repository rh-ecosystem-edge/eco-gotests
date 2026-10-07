package o_cloud_system_test

import (
	. "github.com/onsi/ginkgo/v2"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/o-cloud/internal/ocloudcommon"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/o-cloud/internal/ocloudparams"
)

var _ = Describe(
	"ORAN MNO Upgrade Test Suite",
	Ordered,
	ContinueOnFailure,
	Label(ocloudparams.Label), func() {
		Context("Pre-provisioned MNO spoke", Label(ocloudparams.LabelMNOUpgrade), func() {
			// 123456789 - Successfully performs MNO z-stream upgrade via ProvisioningRequest
			It("Successfully performs MNO z-stream upgrade",
				reportxml.ID("123456789"),
				ocloudcommon.VerifySuccessfulMNOZStreamUpgrade)
		})
	})
