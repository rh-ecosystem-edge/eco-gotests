/*
SPDX-FileCopyrightText: Red Hat

SPDX-License-Identifier: Apache-2.0
*/

package v1alpha1

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/r3labs/diff/v3"
	"github.com/xeipuuv/gojsonschema"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	TemplateParamClusterInstance = "clusterInstanceParameters"
	TemplateParamPolicyConfig    = "policyTemplateParameters"
)

var (
	// allowedClusterInstanceFields contains path patterns for fields that are allowed to be updated.
	// The wildcard "*" is used to match any index in a list.
	AllowedClusterInstanceFields = [][]string{
		// Cluster-level non-immutable fields
		{"extraAnnotations"},
		{"extraLabels"},
		// Node-level non-immutable fields
		{"nodes", "*", "extraAnnotations"},
		{"nodes", "*", "extraLabels"},
	}
)

func ExtractSubSchema(mainSchema []byte, subSchemaKey string) (subSchema map[string]any, err error) {
	jsonObject := make(map[string]any)
	if len(mainSchema) == 0 {
		return subSchema, nil
	}
	err = json.Unmarshal(mainSchema, &jsonObject)
	if err != nil {
		return subSchema, fmt.Errorf("failed to UnMarshall Main Schema: %w", err)
	}
	if _, ok := jsonObject["properties"]; !ok {
		return subSchema, fmt.Errorf("non compliant Main Schema, missing 'properties' section: %w", err)
	}
	properties, ok := jsonObject["properties"].(map[string]any)
	if !ok {
		return subSchema, fmt.Errorf("could not cast 'properties' section of schema as map[string]any: %w", err)
	}

	subSchemaValue, ok := properties[subSchemaKey]
	if !ok {
		return subSchema, fmt.Errorf("subSchema '%s' does not exist: %w", subSchemaKey, err)
	}

	subSchema, ok = subSchemaValue.(map[string]any)
	if !ok {
		return subSchema, fmt.Errorf("subSchema '%s' is not a valid map: %w", subSchemaKey, err)
	}
	return subSchema, nil
}

// ExtractMatchingInput extracts the portion of the input data that corresponds to a given subSchema key.
func ExtractMatchingInput(parentSchema []byte, subSchemaKey string) (any, error) {
	inputData := make(map[string]any)
	err := json.Unmarshal(parentSchema, &inputData)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal parent schema: %w", err)
	}

	// Check if the input contains the subSchema key
	matchingInput, ok := inputData[subSchemaKey]
	if !ok {
		return nil, fmt.Errorf("parent schema does not contain key '%s': %w", subSchemaKey, err)
	}
	return matchingInput, nil
}

// DisallowUnknownFieldsInSchema updates a schema by adding "additionalProperties": false
// to all objects/arrays that define "properties". This ensures that any unknown fields
// not defined in the schema will be disallowed during validation.
func DisallowUnknownFieldsInSchema(schema map[string]any) {
	// Check if the current schema level has "properties" defined
	if properties, hasProperties := schema["properties"]; hasProperties {
		// If "additionalProperties" is not already set, add it with the value false
		if _, exists := schema["additionalProperties"]; !exists {
			schema["additionalProperties"] = false
		}

		// Recurse into each property defined under "properties"
		if propsMap, ok := properties.(map[string]any); ok {
			for _, propValue := range propsMap {
				if propSchema, ok := propValue.(map[string]any); ok {
					DisallowUnknownFieldsInSchema(propSchema)
				}
			}
		}
	}

	// Recurse into each property defined under "items"
	if items, hasItems := schema["items"]; hasItems {
		if itemSchema, ok := items.(map[string]any); ok {
			DisallowUnknownFieldsInSchema(itemSchema)
		}
	}

	// Ignore other keywords that could have "properties"
}

// ValidateJsonAgainstJsonSchema validates the input against the schema.
func ValidateJsonAgainstJsonSchema(schema, input any) error {
	schemaLoader := gojsonschema.NewGoLoader(schema)
	inputLoader := gojsonschema.NewGoLoader(input)

	result, err := gojsonschema.Validate(schemaLoader, inputLoader)
	if err != nil {
		return fmt.Errorf("failed when validating the input against the schema: %w", err)
	}

	if result.Valid() {
		return nil
	} else {
		var errs []string
		for _, description := range result.Errors() {
			errs = append(errs, description.String())
		}

		return fmt.Errorf("invalid input: %s", strings.Join(errs, "; "))
	}
}

// validateTemplateInputMatchesSchema validates the input parameters from the ProvisioningRequest
// against the schema defined in the ClusterTemplate. This function focuses on validating the
// input other than clusterInstanceParameters and policyTemplateParameters, as those will be
// validated separately. It ensures the input parameters have the expected types and any
// required parameters are present.
func (r *ProvisioningRequest) ValidateTemplateInputMatchesSchema(
	clusterTemplate *ClusterTemplate) error {
	// Unmarshal the full schema from the ClusterTemplate
	templateParamSchema := make(map[string]any)
	err := json.Unmarshal(clusterTemplate.Spec.TemplateParameterSchema.Raw, &templateParamSchema)
	if err != nil {
		// Unlikely to happen since it has been validated by API server
		return fmt.Errorf("error unmarshaling template schema: %w", err)
	}

	// Unmarshal the template input from the ProvisioningRequest
	templateParamsInput := make(map[string]any)
	if err = json.Unmarshal(r.Spec.TemplateParameters.Raw, &templateParamsInput); err != nil {
		// Unlikely to happen since it has been validated by API server
		return fmt.Errorf("error unmarshaling templateParameters: %w", err)
	}

	// The following errors of missing keys are unlikely since the schema should already
	// be validated by ClusterTemplate controller
	schemaProperties, ok := templateParamSchema["properties"]
	if !ok {
		return fmt.Errorf(
			"missing keyword 'properties' in the schema from ClusterTemplate (%s)", clusterTemplate.Name)
	}
	clusterInstanceSubSchema, ok := schemaProperties.(map[string]any)[TemplateParamClusterInstance]
	if !ok {
		return fmt.Errorf(
			"missing required property '%s' in the schema from ClusterTemplate (%s)",
			TemplateParamClusterInstance, clusterTemplate.Name)
	}
	policyTemplateSubSchema, ok := schemaProperties.(map[string]any)[TemplateParamPolicyConfig]
	if !ok {
		return fmt.Errorf(
			"missing required property '%s' in the schema from ClusterTemplate (%s)",
			TemplateParamPolicyConfig, clusterTemplate.Name)
	}

	// The ClusterInstance and PolicyTemplate parameters have their own specific validation rules
	// and will be handled separately. For now, remove the subschemas for those parameters to
	// ensure they are not validated at this stage.
	delete(clusterInstanceSubSchema.(map[string]any), "properties")
	delete(policyTemplateSubSchema.(map[string]any), "properties")

	err = ValidateJsonAgainstJsonSchema(templateParamSchema, templateParamsInput)
	if err != nil {
		return fmt.Errorf(
			"spec.templateParameters does not match the schema defined in ClusterTemplate (%s) spec.templateParameterSchema: %w",
			clusterTemplate.Name, err)
	}

	return nil
}

// ValidateClusterInstanceInputMatchesSchema validates that the ClusterInstance input
// from the ProvisioningRequest matches the schema defined in the ClusterTemplate.
// If valid, the merged ClusterInstance data is stored in the clusterInput.
func (r *ProvisioningRequest) ValidateClusterInstanceInputMatchesSchema(
	clusterTemplate *ClusterTemplate) (any, error) {

	// Get the subschema for ClusterInstanceParameters
	clusterInstanceSubSchema, err := ExtractSubSchema(
		clusterTemplate.Spec.TemplateParameterSchema.Raw, TemplateParamClusterInstance)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to extract %s subschema: %s", TemplateParamClusterInstance, err.Error())
	}
	// Any unknown fields not defined in the schema will be disallowed
	DisallowUnknownFieldsInSchema(clusterInstanceSubSchema)

	// Get the matching input for ClusterInstanceParameters
	clusterInstanceMatchingInput, err := ExtractMatchingInput(
		r.Spec.TemplateParameters.Raw, TemplateParamClusterInstance)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to extract matching input for subSchema %s: %w", TemplateParamClusterInstance, err)
	}

	// The schema defined in ClusterTemplate's spec.templateParameterSchema for
	// clusterInstanceParameters represents a subschema of ClusterInstance parameters that
	// are allowed/exposed only to the ProvisioningRequest. Therefore, validate the ClusterInstance
	// input from the ProvisioningRequest against this schema, rather than validating the merged
	// ClusterInstance data. A full validation of the complete ClusterInstance input will be
	// performed during the ClusterInstance dry-run later in the controller.
	err = ValidateJsonAgainstJsonSchema(
		clusterInstanceSubSchema, clusterInstanceMatchingInput)
	if err != nil {
		return nil, fmt.Errorf(
			"spec.templateParameters.%s does not match the schema defined in ClusterTemplate (%s) spec.templateParameterSchema.%s: %w",
			TemplateParamClusterInstance, clusterTemplate.Name, TemplateParamClusterInstance, err)
	}

	return clusterInstanceMatchingInput, nil
}

func (r *ProvisioningRequest) GetClusterTemplateRef(ctx context.Context, client client.Client) (*ClusterTemplate, error) {
	// Check the clusterTemplateRef references an existing template in the same namespace
	// as the current provisioningRequest.
	clusterTemplateRefName := fmt.Sprintf("%s.%s", r.Spec.TemplateName, r.Spec.TemplateVersion)
	clusterTemplates := &ClusterTemplateList{}

	// Get the one clusterTemplate that's valid.
	err := client.List(ctx, clusterTemplates)
	// If there was an error in trying to get the ClusterTemplate, return it.
	if err != nil {
		return nil, fmt.Errorf("failed to get ClusterTemplate: %w", err)
	}
	for _, ct := range clusterTemplates.Items {
		if ct.Name == clusterTemplateRefName {
			validatedCond := meta.FindStatusCondition(
				ct.Status.Conditions,
				string(CTconditionTypes.Validated))
			if validatedCond != nil && validatedCond.Status == metav1.ConditionTrue {
				return &ct, nil
			}
		}
	}

	// If the referenced ClusterTemplate does not exist, log and return an appropriate error.
	return nil, fmt.Errorf(
		"a valid ClusterTemplate (%s) does not exist in any namespace",
		clusterTemplateRefName)
}

// FindClusterInstanceImmutableFieldUpdates identifies updates made to immutable fields
// in the ClusterInstance fields. It returns two lists of paths: a list of updated fields
// that are considered immutable and should not be modified and a list of fields related
// to node scaling, indicating nodes that were added or removed.
func FindClusterInstanceImmutableFieldUpdates(
	old, new map[string]any, ignoredFields [][]string, allowedFields [][]string) ([]string, []string, error) {

	diffs, err := diff.Diff(old, new, diff.AllowTypeMismatch(true))
	if err != nil {
		return nil, nil, fmt.Errorf("error comparing differences between old "+
			"and new ClusterInstance input: %w", err)
	}

	var updatedFields []string
	var scalingNodes []string
	for _, diff := range diffs {
		if diff.Type == "update" {
			if diff.From == nil || diff.To == nil {
				continue
			}
			// Get value and type of the initial field.
			from := reflect.ValueOf(diff.From).Interface()
			fromValue := fmt.Sprintf("%v", from)
			fromType := fmt.Sprintf("%T", from)
			// Get value and type of the new field.
			to := reflect.ValueOf(diff.To).Interface()
			toValue := fmt.Sprintf("%v", to)
			toType := fmt.Sprintf("%T", to)

			// If the type has changed, also check the value. For the IMS usecase we do no support type
			// changes, so this is the case of a mismatch from unmarshalling and it should be ignored if
			// the value has been kept.
			if fromType != toType {
				if fromValue == toValue {
					continue
				}
			}
		}
		/* Examples of diff result in json format

		Label added at the cluster-level
		  {"type": "create", "path": ["extraLabels", "ManagedCluster", "newLabelKey"], "from": null, "to": "newLabelValue"}

		Field updated at the cluster-level
		  {"type": "update", "path": ["baseDomain"], "from": "domain.example.com", "to": "newdomain.example.com"}

		New node added
		  {"type": "create", "path": ["nodes", "1"], "from": null, "to": {"hostName": "worker2"}}

		Existing node removed
		  {"type": "delete", "path": ["nodes", "1"], "from": {"hostName": "worker2"}, "to": null}

		Field updated at the node-level
		  {"type": "update", "path": ["nodes", "0", "nodeNetwork", "config", "dns-resolver", "config", "server", "0"], "from": "192.10.1.2", "to": "192.10.1.3"}
		*/

		// Check if the path matches any ignored fields
		if matchesAnyPattern(diff.Path, ignoredFields) {
			// Ignored field; skip
			continue
		}

		provisioningrequestlog.Info(
			fmt.Sprintf(
				"ClusterInstance input diff: type: %s, path: %s, from: %+v, to: %+v",
				diff.Type, strings.Join(diff.Path, "."), diff.From, diff.To,
			),
		)

		// Check if the path matches any allowed fields
		if matchesAnyPattern(diff.Path, allowedFields) {
			// Allowed field; skip
			continue
		}

		// Check if the change is adding or removing a node.
		// Path like ["nodes", "1"], indicating node addition or removal.
		if diff.Path[0] == "nodes" && len(diff.Path) == 2 {
			scalingNodes = append(scalingNodes, strings.Join(diff.Path, "."))
			continue
		}
		updatedFields = append(updatedFields, strings.Join(diff.Path, "."))
	}

	return updatedFields, scalingNodes, nil
}

// matchesPattern checks if the path matches the pattern
func matchesPattern(path, pattern []string) bool {
	if len(path) < len(pattern) {
		return false
	}

	for i, p := range pattern {
		if p == "*" {
			// Wildcard matches any single element
			continue
		}
		if path[i] != p {
			return false
		}
	}

	return true
}

// matchesAnyPattern checks if the given path matches any pattern in the provided list.
func matchesAnyPattern(path []string, patterns [][]string) bool {
	for _, pattern := range patterns {
		if matchesPattern(path, pattern) {
			return true
		}
	}
	return false
}
