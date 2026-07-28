package o_cloud_system_test

import (
	. "github.com/onsi/ginkgo/v2"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/o-cloud/internal/ocloudcommon"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/o-cloud/internal/ocloudparams"
)

var _ = Describe(
	"ORAN Alarms Tests", Ordered, ContinueOnFailure, Label(ocloudparams.Label, "ocloud-alarms"), func() {
		It("Successful alarm retrieval from the API",
			VerifySuccessfulAlarmRetrieval)

		It("Successful alarms cleanup from the database after the retention period",
			VerifySuccessfulAlarmsCleanup)
	})
