//go:build unit_test

package o2imstest

import (
	"testing"

	"github.com/google/uuid"
	oranapi "github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
	"github.com/stretchr/testify/assert"
)

func TestVerifyAlarmDictionaryStructure(t *testing.T) {
	t.Parallel()

	t.Run("valid", func(t *testing.T) {
		t.Parallel()
		assert.NoError(t, VerifyAlarmDictionaryStructure(validAlarmDictionary()))
	})

	t.Run("empty alarm definitions", func(t *testing.T) {
		t.Parallel()
		assert.NoError(t, VerifyAlarmDictionaryStructure(validAlarmDictionary(func(dictionary *oranapi.AlarmDictionary) {
			dictionary.AlarmDefinition = []oranapi.AlarmDefinition{}
		})))
	})

	t.Run("empty severity value allowed", func(t *testing.T) {
		t.Parallel()
		assert.NoError(t, VerifyAlarmDictionaryStructure(validAlarmDictionary(func(dictionary *oranapi.AlarmDictionary) {
			dictionary.AlarmDefinition = []oranapi.AlarmDefinition{{
				AlarmName:        "TestAlarm",
				AlarmDescription: "test alarm",
				AlarmAdditionalFields: new(map[string]any{
					tsparams.AlarmDefinitionSeverityField: "",
				}),
			}}
		})))
	})

	for _, testCase := range []struct {
		name       string
		dictionary oranapi.AlarmDictionary
	}{
		{
			name: "nil alarmDictionaryId",
			dictionary: validAlarmDictionary(func(dictionary *oranapi.AlarmDictionary) {
				dictionary.AlarmDictionaryId = uuid.Nil
			}),
		},
		{
			name: "empty entityType",
			dictionary: validAlarmDictionary(func(dictionary *oranapi.AlarmDictionary) {
				dictionary.EntityType = ""
			}),
		},
		{
			name: "empty vendor",
			dictionary: validAlarmDictionary(func(dictionary *oranapi.AlarmDictionary) {
				dictionary.Vendor = ""
			}),
		},
		{
			name: "nil alarm definitions",
			dictionary: validAlarmDictionary(func(dictionary *oranapi.AlarmDictionary) {
				dictionary.AlarmDefinition = nil
			}),
		},
		{
			name: "empty alarmName",
			dictionary: validAlarmDictionary(func(dictionary *oranapi.AlarmDictionary) {
				dictionary.AlarmDefinition[0].AlarmName = ""
			}),
		},
		{
			name: "empty alarmDescription",
			dictionary: validAlarmDictionary(func(dictionary *oranapi.AlarmDictionary) {
				dictionary.AlarmDefinition[0].AlarmDescription = ""
			}),
		},
		{
			name: "missing severity key",
			dictionary: validAlarmDictionary(func(dictionary *oranapi.AlarmDictionary) {
				dictionary.AlarmDefinition[0].AlarmAdditionalFields = new(map[string]any{"other": "value"})
			}),
		},
		{
			name: "nil additional fields",
			dictionary: validAlarmDictionary(func(dictionary *oranapi.AlarmDictionary) {
				dictionary.AlarmDefinition[0].AlarmAdditionalFields = nil
			}),
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			assert.Error(t, VerifyAlarmDictionaryStructure(testCase.dictionary))
		})
	}
}

func validAlarmDictionary(mutate ...func(*oranapi.AlarmDictionary)) oranapi.AlarmDictionary {
	severityFields := map[string]any{tsparams.AlarmDefinitionSeverityField: "critical"}

	dictionary := oranapi.AlarmDictionary{
		AlarmDictionaryId:            uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		AlarmDictionarySchemaVersion: "1.0.0",
		AlarmDictionaryVersion:       "1.0.0",
		EntityType:                   "NodeClusterType",
		Vendor:                       "redhat",
		AlarmDefinition: []oranapi.AlarmDefinition{{
			AlarmName:             "TestAlarm",
			AlarmDescription:      "test alarm",
			AlarmAdditionalFields: &severityFields,
		}},
	}

	if len(mutate) == 0 {
		return dictionary
	}

	if dictionary.AlarmDefinition != nil {
		dictionary.AlarmDefinition = append([]oranapi.AlarmDefinition(nil), dictionary.AlarmDefinition...)
	}

	mutate[0](&dictionary)

	return dictionary
}
