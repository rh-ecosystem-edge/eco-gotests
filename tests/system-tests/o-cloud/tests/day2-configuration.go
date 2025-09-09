package o_cloud_system_test

import (
	. "github.com/onsi/ginkgo/v2"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/o-cloud/internal/ocloudcommon"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/o-cloud/internal/ocloudparams"
)

var _ = Describe(
	"ORAN Day 2 Configuration Test Suite",
	Ordered,
	ContinueOnFailure,
	Label(ocloudparams.Label), func() {
		Context("Configured hub cluster", Label("ocloud-day2-configuration"), func() {
			It("Successful operator upgrade",
				ocloudcommon.VerifySuccessfulOperatorUpgrade)

			It("Failed operator upgrade in all the SNOs",
				ocloudcommon.VerifyFailedOperatorUpgradeAllSnos)

			It("Failed operator upgrade in a subset of the SNOs",
				ocloudcommon.VerifyFailedOperatorUpgradeSubsetSnos)
		})
	})
