package api

import (
	"context"
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api/filter"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api/internal/artifacts"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api/internal/common"
	"k8s.io/utils/ptr"
)

// ManagedInfrastructureTemplate is the type of the ManagedInfrastructureTemplate resource returned by the API.
type ManagedInfrastructureTemplate = artifacts.ManagedInfrastructureTemplate

// ManagedInfrastructureTemplateDefaults aggregates all kinds of defaults for the template's schema.
type ManagedInfrastructureTemplateDefaults = artifacts.ManagedInfrastructureTemplateDefaults

// ArtifactsClient provides access to the O2IMS infrastructure artifacts API.
type ArtifactsClient struct {
	artifacts.ClientWithResponsesInterface
}

// ListManagedInfrastructureTemplates lists all managed infrastructure templates. Optionally, a filter can be provided
// to filter the list of templates. If more than one filter is provided, only the first one is used. filter.And() can be
// used to combine multiple filters.
func (client *ArtifactsClient) ListManagedInfrastructureTemplates(
	filter ...filter.Filter) ([]ManagedInfrastructureTemplate, error) {
	var filterString *common.Filter

	if len(filter) > 0 {
		filterString = ptr.To(filter[0].Filter())
	}

	resp, err := client.GetManagedInfrastructureTemplatesWithResponse(context.TODO(),
		&artifacts.GetManagedInfrastructureTemplatesParams{Filter: filterString})
	if err != nil {
		return nil, fmt.Errorf("failed to list ManagedInfrastructureTemplates: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return nil, fmt.Errorf("failed to list ManagedInfrastructureTemplates: received error from api: %w",
			apiErrorFromResponse(resp))
	}

	return *resp.JSON200, nil
}

// GetManagedInfrastructureTemplate gets a managed infrastructure template by its ID. This ID is the spec.templateId of
// the corresponding ClusterTemplate.
func (client *ArtifactsClient) GetManagedInfrastructureTemplate(id string) (*ManagedInfrastructureTemplate, error) {
	resp, err := client.GetManagedInfrastructureTemplateWithResponse(context.TODO(), id)
	if err != nil {
		return nil, fmt.Errorf("failed to get ManagedInfrastructureTemplate: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return nil, fmt.Errorf("failed to get ManagedInfrastructureTemplate: received error from api: %w",
			apiErrorFromResponse(resp))
	}

	return resp.JSON200, nil
}

// GetManagedInfrastructureTemplateDefaults gets the defaults for a managed infrastructure template. These correspond to
// the defaults ConfigMaps for a ClusterTemplate.
func (client *ArtifactsClient) GetManagedInfrastructureTemplateDefaults(
	id string) (*ManagedInfrastructureTemplateDefaults, error) {
	resp, err := client.GetManagedInfrastructureTemplateDefaultsWithResponse(context.TODO(), id)
	if err != nil {
		return nil, fmt.Errorf("failed to get ManagedInfrastructureTemplateDefaults: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return nil, fmt.Errorf("failed to get ManagedInfrastructureTemplateDefaults: received error from api: %w",
			apiErrorFromResponse(resp))
	}

	return resp.JSON200, nil
}
