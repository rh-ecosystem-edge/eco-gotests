//go:build unit_test

package profiles

import (
	"testing"

	ptpv1 "github.com/k8snetworkplumbingwg/ptp-operator/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"
)

func TestParsePtpProfileDualTBC(t *testing.T) {
	t.Parallel()

	dualClientConf := `
[ens7f1]
masterOnly 0
[ens7f3]
masterOnly 0
[global]
slaveOnly 1
`

	tests := []struct {
		name            string
		profile         ptpv1.PtpProfile
		controlledNames map[string]bool
		want            PtpProfileType
	}{
		{
			name: "WPC dual T-BC with clockType and controllingProfile",
			profile: ptpv1.PtpProfile{
				Name:      ptr.To("tbc-tr"),
				Ptp4lConf: ptr.To(dualClientConf),
				PtpSettings: map[string]string{
					"clockType":    "T-BC",
					"upstreamPort": "ens7f1,ens7f3",
				},
				Ts2PhcConf:  ptr.To("[ens7f0]\nts2phc.master 0\n"),
				Phc2sysOpts: ptr.To("-s ens7f1"),
			},
			controlledNames: map[string]bool{"tbc-tr": true},
			want:            ProfileTypeDualTBCReceiver,
		},
		{
			name: "GNRD dual T-BC with clockType and no controllingProfile",
			profile: ptpv1.PtpProfile{
				Name:      ptr.To("01-bc-tr"),
				Ptp4lConf: ptr.To(dualClientConf),
				PtpSettings: map[string]string{
					"clockType":    "T-BC",
					"upstreamPort": "eno8503np2,eno8603np3",
				},
				Ts2PhcConf:  ptr.To("[eno8703np0]\nts2phc.master 0\n"),
				Phc2sysOpts: ptr.To("-s eno8503np2"),
			},
			want: ProfileTypeDualTBCReceiver,
		},
		{
			name: "two-port OC is not classified as dual T-BC",
			profile: ptpv1.PtpProfile{
				Name:      ptr.To("oc-2-port"),
				Ptp4lConf: ptr.To(dualClientConf),
			},
			want: ProfileTypeTwoPortOC,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := parsePtpProfile(testCase.profile, ProfileReference{ProfileName: *testCase.profile.Name},
				testCase.controlledNames)
			require.NoError(t, err)
			assert.Equal(t, testCase.want, got.ProfileType)
		})
	}
}
