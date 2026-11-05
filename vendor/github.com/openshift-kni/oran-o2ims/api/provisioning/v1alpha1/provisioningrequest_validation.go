/*
SPDX-FileCopyrightText: Red Hat

SPDX-License-Identifier: Apache-2.0
*/

package v1alpha1

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"os"

	hwmgmtv1alpha1 "github.com/openshift-kni/oran-o2ims/api/hardwaremanagement/v1alpha1"
	"github.com/openshift-kni/oran-o2ims/internal/constants"
	upgradevalidation "github.com/openshift-kni/oran-o2ims/internal/validation"
	"github.com/r3labs/diff/v3"
	"github.com/xeipuuv/gojsonschema"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	// AllowedClusterInstanceFields contains path patterns for fields that are allowed
	// to be updated from the ProvisioningRequest after cluster installation.
	// The wildcard "*" is used to match any index in a list.
	AllowedClusterInstanceFields = [][]string{
		// Cluster-level non-immutable fields
		{"extraAnnotations"},
		{"extraLabels"},
		// Node-level non-immutable fields (legacy flat nodes)
		{constants.ClusterInstanceNodesKey, "*", "extraAnnotations"},
		{constants.ClusterInstanceNodesKey, "*", "extraLabels"},
		// Node-group-level and nested node non-immutable fields
		{constants.ClusterInstanceNodeGroupsKey, "*", "extraAnnotations"},
		{constants.ClusterInstanceNodeGroupsKey, "*", "extraLabels"},
		{constants.ClusterInstanceNodeGroupsKey, "*", constants.ClusterInstanceNodesKey, "*", "extraAnnotations"},
		{constants.ClusterInstanceNodeGroupsKey, "*", constants.ClusterInstanceNodesKey, "*", "extraLabels"},
	}
)

// ErrSubSchemaNotFound is returned by ExtractSubSchema when the requested key
// does not exist in the schema's properties.
var ErrSubSchemaNotFound = errors.New("sub-schema not found")

// IsErrSubSchemaNotFound returns true if the error is or wraps ErrSubSchemaNotFound.
func IsErrSubSchemaNotFound(err error) bool {
	return errors.Is(err, ErrSubSchemaNotFound)
}

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
		return subSchema, fmt.Errorf("subSchema '%s' does not exist: %w", subSchemaKey, ErrSubSchemaNotFound)
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
	clusterInstanceSubSchema, ok := schemaProperties.(map[string]any)[constants.TemplateParamClusterInstance]
	if !ok {
		return fmt.Errorf(
			"missing required property '%s' in the schema from ClusterTemplate (%s)",
			constants.TemplateParamClusterInstance, clusterTemplate.Name)
	}
	policyTemplateSubSchema, ok := schemaProperties.(map[string]any)[constants.TemplateParamPolicyConfig]
	if !ok {
		return fmt.Errorf(
			"missing required property '%s' in the schema from ClusterTemplate (%s)",
			constants.TemplateParamPolicyConfig, clusterTemplate.Name)
	}

	// The ClusterInstance, PolicyTemplate, and HwMgmt parameters have their own specific
	// validation rules and will be handled separately. For now, remove the subschemas for
	// those parameters to ensure they are not validated at this stage.
	delete(clusterInstanceSubSchema.(map[string]any), "properties")
	delete(policyTemplateSubSchema.(map[string]any), "properties")

	// hwMgmtParameters is validated after merging with hwMgmtDefaults from the ClusterTemplate,
	// so strip its detailed schema here to avoid rejecting partial overrides (e.g. nodeGroupData
	// entries that omit fields like "role" because they come from the defaults).
	if hwMgmtSubSchema, ok := schemaProperties.(map[string]any)[constants.TemplateParamHwMgmt]; ok {
		if hwMgmtMap, ok := hwMgmtSubSchema.(map[string]any); ok {
			delete(hwMgmtMap, "properties")
		}
	}

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
		clusterTemplate.Spec.TemplateParameterSchema.Raw, constants.TemplateParamClusterInstance)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to extract %s subschema: %s", constants.TemplateParamClusterInstance, err.Error())
	}
	// Any unknown fields not defined in the schema will be disallowed
	DisallowUnknownFieldsInSchema(clusterInstanceSubSchema)

	// Get the matching input for ClusterInstanceParameters
	clusterInstanceMatchingInput, err := ExtractMatchingInput(
		r.Spec.TemplateParameters.Raw, constants.TemplateParamClusterInstance)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to extract matching input for subSchema %s: %w", constants.TemplateParamClusterInstance, err)
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
			constants.TemplateParamClusterInstance, clusterTemplate.Name, constants.TemplateParamClusterInstance, err)
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
// to node scaling, indicating nodes that were added, removed or swapped.
func FindClusterInstanceImmutableFieldUpdates(
	oldData, newData map[string]any, ignoredFields [][]string, allowedFields [][]string) ([]string, []string, error) {

	diffs, err := diff.Diff(oldData, newData, diff.AllowTypeMismatch(true))
	if err != nil {
		return nil, nil, fmt.Errorf("error comparing differences between old "+
			"and new ClusterInstance input: %w", err)
	}

	// First pass: find hosts whose hostName changed in-place (swap) under nodeGroups.
	// When a node's hostName is updated in-place, the diff library reports field-level
	// changes (e.g., nodeGroups.0.nodes.1.hostName) rather than a whole-node
	// add/remove. We treat all changes under that host as scaling.
	// Keys are "nodeGroups.<gi>.nodes.<ni>".
	swappedNodeKeys := make(map[string]bool)
	for _, d := range diffs {
		if d.Type != "update" {
			continue
		}
		// ["nodeGroups", "<gi>", "nodes", "<ni>", "hostName", ...]
		if len(d.Path) >= 5 && d.Path[0] == constants.ClusterInstanceNodeGroupsKey &&
			d.Path[2] == constants.ClusterInstanceNodesKey && d.Path[4] == "hostName" {
			swappedNodeKeys[fmt.Sprintf("%s.%s.%s.%s",
				constants.ClusterInstanceNodeGroupsKey, d.Path[1],
				constants.ClusterInstanceNodesKey, d.Path[3])] = true
		}
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

		New nodeGroup added (e.g. first workers after a masters-only provision)
		  {"type": "create", "path": ["nodeGroups", "1"], "from": null, "to": {"name": "worker", "nodes": [...]}}

		Entire nodeGroup removed (e.g. scale workers to zero by omitting the group)
		  {"type": "delete", "path": ["nodeGroups", "1"], "from": {"name": "worker", "nodes": [...]}, "to": null}

		Entire nodes list added under a nodeGroup (e.g. first workers after a masters-only provision)
		  {"type": "create", "path": ["nodeGroups", "1", "nodes"], "from": null, "to": [...]}

		Entire nodes list omitted under a nodeGroup (e.g, scale workers to zero by dropping the nodes key from the group)
		  {"type": "delete", "path": ["nodeGroups", "1", "nodes"], "from": [...], "to": null}

		New node added under a nodeGroup
		  {"type": "create", "path": ["nodeGroups", "1", "nodes", "0"], "from": null, "to": {"hostName": "worker2"}}

		Existing node removed under a nodeGroup
		  {"type": "delete", "path": ["nodeGroups", "1", "nodes", "0"], "from": {"hostName": "worker2"}, "to": null}

		Field updated at the node-level (legacy flat nodes)
		  {"type": "update", "path": ["nodes", "0", "nodeNetwork", "config", "dns-resolver", "config", "server", "0"], "from": "192.10.1.2", "to": "192.10.1.3"}

		Field updated at the node-level (nodeGroups)
		  {"type": "update", "path": ["nodeGroups", "0", "nodes", "0", "nodeNetwork", "config", "dns-resolver", "config", "server", "0"], "from": "192.10.1.2", "to": "192.10.1.3"}

		Node swap (hostName changed in-place)
		  {"type": "update", "path": ["nodeGroups", "0", "nodes", "1", "hostName"], "from": "worker1", "to": "worker2"}
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

		// Scaling: adding or removing an entire nodeGroup entry.
		// Path like ["nodeGroups", "1"].
		if len(diff.Path) == 2 && diff.Path[0] == constants.ClusterInstanceNodeGroupsKey {
			scalingNodes = append(scalingNodes, strings.Join(diff.Path, "."))
			continue
		}

		// Scaling: adding or removing an entire nodes list or single node under a nodeGroup.
		// Path like ["nodeGroups", "1", "nodes"] or ["nodeGroups", "1", "nodes", "0"].
		if (len(diff.Path) == 3 || len(diff.Path) == 4) &&
			diff.Path[0] == constants.ClusterInstanceNodeGroupsKey &&
			diff.Path[2] == constants.ClusterInstanceNodesKey {
			scalingNodes = append(scalingNodes, strings.Join(diff.Path, "."))
			continue
		}

		// Scaling: any change under a node whose hostName was swapped in-place.
		// Path like ["nodeGroups", "0", "nodes", "1", "hostName"].
		if len(diff.Path) > 4 && diff.Path[0] == constants.ClusterInstanceNodeGroupsKey &&
			diff.Path[2] == constants.ClusterInstanceNodesKey {
			parentKey := fmt.Sprintf("%s.%s.%s.%s",
				constants.ClusterInstanceNodeGroupsKey, diff.Path[1],
				constants.ClusterInstanceNodesKey, diff.Path[3])
			if swappedNodeKeys[parentKey] {
				if !slices.Contains(scalingNodes, parentKey) {
					scalingNodes = append(scalingNodes, parentKey)
				}
				continue
			}
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

func getEnvOrDefault(name, defaultValue string) string {
	value := os.Getenv(name)
	if value == "" {
		return defaultValue
	}
	return value
}

// SchemaDefinesHwMgmtParameters checks whether the ClusterTemplate's
// templateParameterSchema defines the hwMgmtParameters property.
func SchemaDefinesHwMgmtParameters(clusterTemplate *ClusterTemplate) bool {
	if clusterTemplate.Spec.TemplateParameterSchema.Raw == nil {
		return false
	}
	var schema map[string]any
	if err := json.Unmarshal(clusterTemplate.Spec.TemplateParameterSchema.Raw, &schema); err != nil {
		return false
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return false
	}
	_, defined := properties[constants.TemplateParamHwMgmt]
	return defined
}

// ValidateHwMgmtHwProfiles validates that hwProfile values in the ProvisioningRequest's
// hwMgmtParameters.nodeGroupData reference existing HardwareProfile CRs.
// This provides early feedback at admission time for user-supplied profile names.
func (r *ProvisioningRequest) ValidateHwMgmtHwProfiles(
	ctx context.Context, c client.Client, clusterTemplate *ClusterTemplate) error {

	if r.Spec.TemplateParameters.Raw == nil {
		return nil
	}

	var params map[string]any
	if err := json.Unmarshal(r.Spec.TemplateParameters.Raw, &params); err != nil {
		return nil
	}

	hwMgmtParams, ok := params[constants.TemplateParamHwMgmt]
	if !ok {
		return nil
	}
	hwMgmtMap, ok := hwMgmtParams.(map[string]any)
	if !ok {
		return nil
	}

	nodeGroupData, ok := hwMgmtMap["nodeGroupData"]
	if !ok {
		return nil
	}
	ngSlice, ok := nodeGroupData.([]any)
	if !ok {
		return nil
	}

	hwmgmtNS := getEnvOrDefault("OCLOUD_MANAGER_NAMESPACE", "oran-o2ims")
	for _, ng := range ngSlice {
		ngMap, ok := ng.(map[string]any)
		if !ok {
			continue
		}
		hwProfile, ok := ngMap["hwProfile"].(string)
		if !ok || hwProfile == "" {
			continue
		}
		name, _ := ngMap["name"].(string)

		hwProfileObj := &hwmgmtv1alpha1.HardwareProfile{}
		if err := c.Get(ctx, client.ObjectKey{Name: hwProfile, Namespace: hwmgmtNS}, hwProfileObj); err != nil {
			if k8serrors.IsNotFound(err) {
				return fmt.Errorf("hardwareProfile %q referenced by nodeGroup %q does not exist", hwProfile, name)
			}
			return fmt.Errorf("failed to get HardwareProfile %q for nodeGroup %q: %w", hwProfile, name, err)
		}
	}

	return nil
}

// ValidateUpgradeInput validates upgrade business rules on the
// ProvisioningRequest's upgradeParameters against the ClusterTemplate's
// release version. Schema validation is handled separately; this covers
// semantic constraints that schema cannot express.
func (r *ProvisioningRequest) ValidateUpgradeInput(clusterTemplate *ClusterTemplate) error {
	if r.Spec.TemplateParameters.Raw == nil {
		return nil
	}

	upgradeParamsRaw, err := ExtractMatchingInput(r.Spec.TemplateParameters.Raw, constants.TemplateParamUpgrade)
	if err != nil {
		return nil
	}
	upgradeParams, ok := upgradeParamsRaw.(map[string]any)
	if !ok {
		return fmt.Errorf("spec.templateParameters.%s must be an object", constants.TemplateParamUpgrade)
	}

	if err := upgradevalidation.ValidateCVUpgradeData(upgradeParams, clusterTemplate.Spec.Release, constants.TemplateParamUpgrade); err != nil {
		return fmt.Errorf("spec.templateParameters.%s: %w", constants.TemplateParamUpgrade, err)
	}

	return nil
}
