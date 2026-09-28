//go:build unit_test

package helper

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetEncryptedDriveList(t *testing.T) {
	type args struct {
		lsblkoutput string
	}

	tests := []struct {
		name          string
		args          args
		wantDriveList []string
	}{
		{
			name: "ok",
			args: args{lsblkoutput: `NAME FSTYPE
sda  
sda1 
sda2 vfat
sda3 ext4
sda4 crypto_LUKS
sdb  
sdb1 crypto_LUKS
root xfs
data xfs
`},
			wantDriveList: []string{"sda4", "sdb1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotDriveList := GetEncryptedDriveList(tt.args.lsblkoutput); !reflect.DeepEqual(gotDriveList, tt.wantDriveList) {
				t.Errorf("getEncryptedDriveList() = %v, want %v", gotDriveList, tt.wantDriveList)
			}
		})
	}
}

func TestIsDiskRoot(t *testing.T) {
	type args struct {
		lsblkMounts string
	}

	tests := []struct {
		name       string
		args       args
		wantIsRoot bool
	}{
		{
			name: "ok",
			args: args{lsblkMounts: `MOUNTPOINTS

/var
/sysroot/ostree/deploy/rhcos/var
/sysroot
/usr
/etc
/
`},
			wantIsRoot: true,
		},
		{
			name: "not root",
			args: args{lsblkMounts: `MOUNTPOINTS

/var
/sysroot/ostree/deploy/rhcos/var
/sysroot
/usr
/etc

`},
			wantIsRoot: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotIsRoot := IsDiskRoot(tt.args.lsblkMounts); gotIsRoot != tt.wantIsRoot {
				t.Errorf("IsDiskRoot() = %v, want %v", gotIsRoot, tt.wantIsRoot)
			}
		})
	}
}

func TestLuksListContainsPCR1And7(t *testing.T) {
	type args struct {
		input string
	}

	tests := []struct {
		name      string
		args      args
		wantFound bool
	}{
		{
			name:      "ok",
			args:      args{input: `1: tpm2 '{"hash":"sha256","key":"ecc","pcr_bank":"sha256","pcr_ids":"1,7"}'`},
			wantFound: true,
		},
		{
			name:      "nok",
			args:      args{input: `1: tpm2 '{"hash":"sha256","key":"ecc","pcr_bank":"sha256","pcr_ids":"1,8"}'`},
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotFound := LuksListContainsPCR1And7(tt.args.input); gotFound != tt.wantFound {
				t.Errorf("LuksListContainsPCR1And7() = %v, want %v", gotFound, tt.wantFound)
			}
		})
	}
}

func TestLuksListContainsReservedSlot(t *testing.T) {
	type args struct {
		input string
	}

	tests := []struct {
		name      string
		args      args
		wantFound bool
	}{
		{
			name: "ok",
			args: args{
				input: `31: tpm2 '{"hash":"sha256","key":"ecc"}'`,
			},
			wantFound: true,
		},
		{
			name: "ok",
			args: args{
				input: `30: tpm2 '{"hash":"sha256","key":"ecc"}'`,
			},
			wantFound: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotFound := LuksListContainsReservedSlot(tt.args.input); gotFound != tt.wantFound {
				t.Errorf("LuksListContainsReservedSlot() = %v, want %v", gotFound, tt.wantFound)
			}
		})
	}
}

func TestStringInSlice(t *testing.T) {
	testCases := []struct {
		testSlice       []string
		testString      string
		containsFeature bool
		expected        bool
	}{
		{
			testSlice: []string{
				"apples",
				"bananas",
				"oranges",
			},
			testString:      "apples",
			containsFeature: false,
			expected:        true,
		},
		{
			testSlice: []string{
				"apples",
				"bananas",
				"oranges",
			},
			testString:      "tacos",
			containsFeature: false,
			expected:        false,
		},
		{
			testSlice: []string{
				"intree: Y",
				"intree: N",
				"outoftree: Y",
			},
			testString:      "intree:",
			containsFeature: true, // Note: Turn 'on' the contains check
			expected:        true,
		},
		{
			testSlice: []string{
				"intree: Y",
				"intree: N",
				"outoftree: Y",
			},
			testString:      "intree:",
			containsFeature: false, // Note: Turn 'off' the contains check
			expected:        false,
		},
	}

	for _, tc := range testCases {
		assert.Equal(t, tc.expected, StringInSlice(tc.testSlice, tc.testString, tc.containsFeature))
	}
}

type otherString string

func TestStringInSlice_other(t *testing.T) {
	testCases := []struct {
		testSlice       []otherString
		testString      otherString
		containsFeature bool
		expected        bool
	}{
		{
			testSlice: []otherString{
				"apples",
				"bananas",
				"oranges",
			},
			testString:      "apples",
			containsFeature: false,
			expected:        true,
		},
		{
			testSlice: []otherString{
				"apples",
				"bananas",
				"oranges",
			},
			testString:      "tacos",
			containsFeature: false,
			expected:        false,
		},
		{
			testSlice: []otherString{
				"intree: Y",
				"intree: N",
				"outoftree: Y",
			},
			testString:      "intree:",
			containsFeature: true, // Note: Turn 'on' the contains check
			expected:        true,
		},
		{
			testSlice: []otherString{
				"intree: Y",
				"intree: N",
				"outoftree: Y",
			},
			testString:      "intree:",
			containsFeature: false, // Note: Turn 'off' the contains check
			expected:        false,
		},
	}

	for _, tc := range testCases {
		assert.Equal(t, tc.expected, StringInSlice(tc.testSlice, tc.testString, tc.containsFeature))
	}
}

func TestSubSlice(t *testing.T) {
	testCases := []struct {
		testSliceA     []string
		testSliceB     []string
		expectedOutput bool
	}{
		{ // Test #1 - SliceB exists in SliceA
			testSliceA:     []string{"one", "two", "three"},
			testSliceB:     []string{"one", "two"},
			expectedOutput: true,
		},
		{ // Test #2 - SliceB does not exist in SliceA
			testSliceA:     []string{"one", "two", "three"},
			testSliceB:     []string{"four", "five"},
			expectedOutput: false,
		},
		{ // Test #3 - Same slices, return true
			testSliceA:     []string{"one", "two", "three"},
			testSliceB:     []string{"one", "two", "three"},
			expectedOutput: true,
		},
		{ // Test Case #4 - Empty SliceA
			testSliceA:     []string{},
			testSliceB:     []string{"one", "two", "three"},
			expectedOutput: false,
		},
		{ // Test #5 - SliceB's elements exist out of order in SliceA
			testSliceA:     []string{"one", "two", "three"},
			testSliceB:     []string{"two", "one"},
			expectedOutput: true,
		},
	}

	for _, tc := range testCases {
		assert.Equal(t, tc.expectedOutput, SubSlice(tc.testSliceA, tc.testSliceB))
	}
}

func TestSwapFirstAndSecondSliceItems(t *testing.T) {
	type args struct {
		slice []string
	}

	tests := []struct {
		name         string
		args         args
		wantNewSlice []string
		wantErr      bool
	}{
		{
			name:         "ok",
			args:         args{slice: []string{"item1", "item2", "item3", "item4"}},
			wantNewSlice: []string{"item2", "item1", "item3", "item4"},
			wantErr:      false,
		},
		{
			name:         "too small",
			args:         args{slice: []string{"item1"}},
			wantNewSlice: []string{"item1"},
			wantErr:      true,
		},
	}
	for _, aTest := range tests {
		t.Run(aTest.name, func(t *testing.T) {
			gotNewSlice, err := SwapFirstAndSecondSliceItems(aTest.args.slice)
			if (err != nil) != aTest.wantErr {
				t.Errorf("swapFirstAndSecondSliceItems() error = %v, wantErr %v", err, aTest.wantErr)

				return
			}

			if !reflect.DeepEqual(gotNewSlice, aTest.wantNewSlice) {
				t.Errorf("swapFirstAndSecondSliceItems() = %v, want %v", gotNewSlice, aTest.wantNewSlice)
			}
		})
	}
}

const TPMPropertiesTestData = `TPM2_PT_PERMANENT:
  ownerAuthSet:              0
  endorsementAuthSet:        0
  lockoutAuthSet:            0
  reserved1:                 0
  disableClear:              0
  inLockout:                 0
  tpmGeneratedEPS:           1
  reserved2:                 0
TPM2_PT_STARTUP_CLEAR:
  phEnable:                  1
  shEnable:                  1
  ehEnable:                  1
  phEnableNV:                1
  reserved1:                 0
  orderly:                   1
TPM2_PT_HR_NV_INDEX: 0x9
TPM2_PT_HR_LOADED: 0x0
TPM2_PT_HR_LOADED_AVAIL: 0x5
TPM2_PT_HR_ACTIVE: 0x0
TPM2_PT_HR_ACTIVE_AVAIL: 0x40
TPM2_PT_HR_TRANSIENT_AVAIL: 0x5
TPM2_PT_HR_PERSISTENT: 0x0
TPM2_PT_HR_PERSISTENT_AVAIL: 0xD
TPM2_PT_NV_COUNTERS: 0x0
TPM2_PT_NV_COUNTERS_AVAIL: 0xC
TPM2_PT_ALGORITHM_SET: 0xFFFFFFFF
TPM2_PT_LOADED_CURVES: 0x3
TPM2_PT_LOCKOUT_COUNTER: 0x7
TPM2_PT_MAX_AUTH_FAIL: 0x3E8
TPM2_PT_LOCKOUT_INTERVAL: 0x1C20
TPM2_PT_LOCKOUT_RECOVERY: 0x15180
TPM2_PT_NV_WRITE_RECOVERY: 0x0
TPM2_PT_AUDIT_COUNTER_0: 0x0
TPM2_PT_AUDIT_COUNTER_1: 0x0
`

func TestGetTPMMaxRetries(t *testing.T) {
	type args struct {
		tpmProperties string
	}

	tests := []struct {
		name    string
		args    args
		want    int64
		wantErr bool
	}{
		{
			name: "ok",
			args: args{
				tpmProperties: TPMPropertiesTestData,
			},
			want:    1000,
			wantErr: false,
		},
	}

	for _, testcase := range tests {
		t.Run(testcase.name, func(t *testing.T) {
			got, err := parseTPMMaxRetries(testcase.args.tpmProperties)
			if (err != nil) != testcase.wantErr {
				t.Errorf("GetTPMMaxRetries() error = %s, wantErr %v", err, testcase.wantErr)

				return
			}

			if got != testcase.want {
				t.Errorf("GetTPMMaxRetries() = %v, want %v", got, testcase.want)
			}
		})
	}
}

func TestGetTPMLockoutCounter(t *testing.T) {
	type args struct {
		tpmProperties string
	}

	tests := []struct {
		name    string
		args    args
		want    int64
		wantErr bool
	}{
		{
			name: "ok",
			args: args{
				tpmProperties: TPMPropertiesTestData,
			},
			want:    7,
			wantErr: false,
		},
	}

	for _, testcase := range tests {
		t.Run(testcase.name, func(t *testing.T) {
			got, err := parseTPMLockoutCounter(testcase.args.tpmProperties)
			if (err != nil) != testcase.wantErr {
				t.Errorf("GetTPMMaxRetries() error = %s, wantErr %v", err, testcase.wantErr)

				return
			}

			if got != testcase.want {
				t.Errorf("GetTPMMaxRetries() = %v, want %v", got, testcase.want)
			}
		})
	}
}
