package vcore_system_test

import (
	. "github.com/onsi/ginkgo/v2"

	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/vcore/internal/vcorecommon"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/vcore/internal/vcoreparams"
)

var _ = Describe(
	"vCore Basic Deployment Suite",
	Ordered,
	ContinueOnFailure,
	Label(vcoreparams.Label), func() {
		vcorecommon.VerifyInitialDeploymentConfig()

		vcorecommon.VerifyCGroupDefault()

		vcorecommon.VerifyPostDeploymentConfig()
	})
