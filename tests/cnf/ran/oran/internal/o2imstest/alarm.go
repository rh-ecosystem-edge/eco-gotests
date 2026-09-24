package o2imstest

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	oranapi "github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
)

// VerifyAlarmDictionaryStructure checks that an AlarmDictionary has the expected top-level fields and definitions.
func VerifyAlarmDictionaryStructure(dictionary oranapi.AlarmDictionary) error {
	var errs []error

	if dictionary.AlarmDictionaryId == uuid.Nil {
		errs = append(errs, fmt.Errorf("alarmDictionaryId: want non-nil UUID, got %s", dictionary.AlarmDictionaryId))
	}

	if dictionary.EntityType == "" {
		errs = append(errs, fmt.Errorf("entityType: want non-empty"))
	}

	if dictionary.Vendor == "" {
		errs = append(errs, fmt.Errorf("vendor: want non-empty"))
	}

	if dictionary.AlarmDefinition == nil {
		errs = append(errs, fmt.Errorf("alarmDefinition: want present"))
	}

	for definitionIndex, definition := range dictionary.AlarmDefinition {
		if definition.AlarmName == "" {
			errs = append(errs, fmt.Errorf("alarmDefinition[%d].alarmName: want non-empty", definitionIndex))
		}

		if definition.AlarmDescription == "" {
			errs = append(errs, fmt.Errorf("alarmDefinition[%d].alarmDescription: want non-empty", definitionIndex))
		}

		// Severity lives in additionalFields; require the key but allow empty values when a Prometheus
		// rule omits the severity label.
		if definition.AlarmAdditionalFields == nil {
			errs = append(errs, fmt.Errorf(
				"alarmDefinition[%d].alarmAdditionalFields: want non-nil with %s key",
				definitionIndex, tsparams.AlarmDefinitionSeverityField))

			continue
		}

		if _, ok := (*definition.AlarmAdditionalFields)[tsparams.AlarmDefinitionSeverityField]; !ok {
			errs = append(errs, fmt.Errorf(
				"alarmDefinition[%d].alarmAdditionalFields.%s: want key present",
				definitionIndex, tsparams.AlarmDefinitionSeverityField))
		}
	}

	return errors.Join(errs...)
}
