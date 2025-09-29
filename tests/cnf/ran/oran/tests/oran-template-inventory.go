package tests

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran"
	oranapi "github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api/filter"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/auth"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
	"gopkg.in/yaml.v3"
)

// We use the pre provision label since these tests are intended to be run before provisioning. Although really they can
// be run at any point unrelated to the provisioning process, by having them before provisioning they can be run
// regardless of whether the provisioning succeeded.
var _ = Describe("ORAN Template Inventory", Label(tsparams.LabelPreProvision, tsparams.LabelTemplateInventory), func() {
	var artifactsClient *oranapi.ArtifactsClient

	BeforeEach(func() {
		var err error

		By("creating the O2IMS API client")
		clientBuilder, err := auth.NewClientBuilderForConfig(RANConfig)
		Expect(err).ToNot(HaveOccurred(), "Failed to create the O2IMS API client builder")

		artifactsClient, err = clientBuilder.BuildArtifacts()
		Expect(err).ToNot(HaveOccurred(), "Failed to create the O2IMS API artifacts client")
	})

	// 82940 - Successfully list ManagedInfrastructureTemplates
	It("successfully lists ManagedInfrastructureTemplates", reportxml.ID("82940"), func() {
		By("listing all ClusterTemplate resources")
		clusterTemplates, err := oran.ListClusterTemplates(HubAPIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to list ClusterTemplate resources")

		By("listing all ManagedInfrastructureTemplates")
		managedInfrastructureTemplates, err := artifactsClient.ListManagedInfrastructureTemplates()
		Expect(err).ToNot(HaveOccurred(), "Failed to list ManagedInfrastructureTemplates")

		By("verifying the ClusterTemplates match the ManagedInfrastructureTemplates exactly")
		Expect(managedInfrastructureTemplates).
			To(HaveLen(len(clusterTemplates)), "Number of ManagedInfrastructureTemplates does not match ClusterTemplates")

		for _, clusterTemplate := range clusterTemplates {
			found := slices.ContainsFunc(managedInfrastructureTemplates,
				func(managedInfrastructureTemplate oranapi.ManagedInfrastructureTemplate) bool {
					return managedInfrastructureTemplateMatchesClusterTemplate(managedInfrastructureTemplate, clusterTemplate)
				})
			Expect(found).To(BeTrue(), "ClusterTemplate %s (version %s) does not have a matching ManagedInfrastructureTemplate",
				clusterTemplate.Definition.Spec.Name, clusterTemplate.Definition.Spec.Version)
		}
	})

	// 82941 - Successfully filter ManagedInfrastructureTemplates
	It("successfully filters ManagedInfrastructureTemplates", reportxml.ID("82941"), func() {
		By("getting the specific ClusterTemplate resource for the valid template")
		clusterTemplateNamespace := tsparams.ClusterTemplateName + "-" + RANConfig.ClusterTemplateAffix
		clusterTemplateName := fmt.Sprintf("%s.%s-%s",
			tsparams.ClusterTemplateName, RANConfig.ClusterTemplateAffix, tsparams.TemplateValid)

		chosenClusterTemplate, err := oran.PullClusterTemplate(HubAPIClient, clusterTemplateName, clusterTemplateNamespace)
		Expect(err).ToNot(HaveOccurred(),
			"Failed to pull ClusterTemplate %s from namespace %s", clusterTemplateName, clusterTemplateNamespace)

		chosenClusterTemplateName := chosenClusterTemplate.Definition.Spec.Name
		chosenClusterTemplateVersion := chosenClusterTemplate.Definition.Spec.Version

		By("verifying the ManagedInfrastructureTemplate exists from the API")
		_, err = artifactsClient.GetManagedInfrastructureTemplate(clusterTemplateName)
		Expect(err).ToNot(HaveOccurred(), "Failed to get ManagedInfrastructureTemplate %s", clusterTemplateName)

		By("filtering ManagedInfrastructureTemplates by name and version")
		nameFilter := filter.Equals("name", chosenClusterTemplateName)
		versionFilter := filter.Equals("version", chosenClusterTemplateVersion)
		combinedFilter := filter.And(nameFilter, versionFilter)

		filteredTemplates, err := artifactsClient.ListManagedInfrastructureTemplates(combinedFilter)
		Expect(err).ToNot(HaveOccurred(), "Failed to filter ManagedInfrastructureTemplates")
		Expect(filteredTemplates).To(HaveLen(1), "Expected exactly one filtered ManagedInfrastructureTemplate")

		By("verifying the chosen ClusterTemplate matches the filtered ManagedInfrastructureTemplate")
		filteredTemplate := filteredTemplates[0]
		Expect(managedInfrastructureTemplateMatchesClusterTemplate(filteredTemplate, chosenClusterTemplate)).
			To(BeTrue(), "Filtered ManagedInfrastructureTemplate does not match the chosen ClusterTemplate")
	})

	// 82942 - Successfully retrieve ManagedInfrastructureTemplate defaults
	It("successfully retrieves ManagedInfrastructureTemplate defaults", reportxml.ID("82942"), func() {
		By("getting the specific ClusterTemplate resource for the valid template")
		clusterTemplateNamespace := tsparams.ClusterTemplateName + "-" + RANConfig.ClusterTemplateAffix
		clusterTemplateName := fmt.Sprintf("%s.%s-%s",
			tsparams.ClusterTemplateName, RANConfig.ClusterTemplateAffix, tsparams.TemplateValid)

		chosenClusterTemplate, err := oran.PullClusterTemplate(HubAPIClient, clusterTemplateName, clusterTemplateNamespace)
		Expect(err).ToNot(HaveOccurred(),
			"Failed to pull ClusterTemplate %s from namespace %s", clusterTemplateName, clusterTemplateNamespace)

		By("retrieving ManagedInfrastructureTemplate defaults")
		managedTemplateDefaults, err := artifactsClient.GetManagedInfrastructureTemplateDefaults(clusterTemplateName)
		Expect(err).ToNot(HaveOccurred(), "Failed to retrieve ManagedInfrastructureTemplate defaults")
		Expect(managedTemplateDefaults).ToNot(BeNil(),
			"ManagedInfrastructureTemplate defaults should not be nil")

		By("verifying the ManagedInfrastructureTemplate defaults are not nil and have the expected keys")
		Expect(managedTemplateDefaults.ClusterInstanceDefaults).ToNot(BeNil(),
			"ClusterInstanceDefaults in API response should not be nil")
		Expect(*managedTemplateDefaults.ClusterInstanceDefaults).To(HaveKey("editable"),
			"ClusterInstanceDefaults should have key 'editable'")
		Expect(managedTemplateDefaults.PolicyTemplateDefaults).ToNot(BeNil(),
			"PolicyTemplateDefaults in API response should not be nil")
		Expect(*managedTemplateDefaults.PolicyTemplateDefaults).To(HaveKey("editable"),
			"PolicyTemplateDefaults should have key 'editable'")

		By("retrieving the clusterInstanceDefaults and policyTemplateDefaults ConfigMaps from the ClusterTemplate spec")
		clusterInstanceDefaultsCMName := chosenClusterTemplate.Definition.Spec.Templates.ClusterInstanceDefaults
		policyTemplateDefaultsCMName := chosenClusterTemplate.Definition.Spec.Templates.PolicyTemplateDefaults

		ciDefaultsCM, err := configmap.Pull(HubAPIClient, clusterInstanceDefaultsCMName, clusterTemplateNamespace)
		Expect(err).ToNot(HaveOccurred(), "Failed to pull clusterInstanceDefaults ConfigMap")

		policyTemplateDefaultsCM, err := configmap.Pull(HubAPIClient, policyTemplateDefaultsCMName, clusterTemplateNamespace)
		Expect(err).ToNot(HaveOccurred(), "Failed to pull policyTemplateDefaults ConfigMap")

		By("parsing the YAML content from the clusterInstanceDefaults ConfigMap")
		Expect(ciDefaultsCM.Definition.Data).To(HaveKey(tsparams.ClusterInstanceDefaultsKey),
			"ClusterInstance defaults ConfigMap should have key %s", tsparams.ClusterInstanceDefaultsKey)
		clusterInstanceDefaultsYAML := ciDefaultsCM.Definition.Data[tsparams.ClusterInstanceDefaultsKey]

		var expectedClusterInstanceDefaults map[string]any
		err = yaml.Unmarshal([]byte(clusterInstanceDefaultsYAML), &expectedClusterInstanceDefaults)
		Expect(err).ToNot(HaveOccurred(), "Failed to parse ClusterInstance defaults YAML")

		By("verifying the parsed extraLabels match the API's extraLabels")
		// Due to some differences between the editable content returned from the API vs the ConfigMap on the
		// cluster, we only check the extraLabels, which should be the same.
		ciDefaultsEditable, isMap := (*managedTemplateDefaults.ClusterInstanceDefaults)["editable"].(map[string]any)
		Expect(isMap).To(BeTrue(), "editable key in ManagedInfrastructureTemplateDefaults should be a map")
		Expect(ciDefaultsEditable["extraLabels"]).To(Equal(expectedClusterInstanceDefaults["extraLabels"]),
			"extraLabels in ManagedInfrastructureTemplateDefaults should match the ConfigMap")

		By("parsing the YAML content from the policyTemplateDefaults ConfigMap")
		Expect(policyTemplateDefaultsCM.Definition.Data).To(HaveKey(tsparams.PolicyTemplateDefaultsKey),
			"PolicyTemplate defaults ConfigMap should have key %s", tsparams.PolicyTemplateDefaultsKey)
		policyTemplateDefaultsYAML := policyTemplateDefaultsCM.Definition.Data[tsparams.PolicyTemplateDefaultsKey]

		var expectedPolicyTemplateDefaults map[string]any
		err = yaml.Unmarshal([]byte(policyTemplateDefaultsYAML), &expectedPolicyTemplateDefaults)
		Expect(err).ToNot(HaveOccurred(), "Failed to parse PolicyTemplate defaults YAML")

		By("verifying the parsed editable policy content matches the API's editable content")
		// The editable content should match the ConfigMap, so we can compare the entire map.
		policyTemplateEditable, isMap := (*managedTemplateDefaults.PolicyTemplateDefaults)["editable"].(map[string]any)
		Expect(isMap).To(BeTrue(), "editable key in ManagedInfrastructureTemplateDefaults should be a map")
		Expect(policyTemplateEditable).To(Equal(expectedPolicyTemplateDefaults),
			"editable content in ManagedInfrastructureTemplateDefaults should match the ConfigMap")
	})
})

// managedInfrastructureTemplateMatchesClusterTemplate checks if a ManagedInfrastructureTemplate matches the data from
// a ClusterTemplate. It includes doing a deep comparison of the parameter schemas.
func managedInfrastructureTemplateMatchesClusterTemplate(
	managedInfrastructureTemplate oranapi.ManagedInfrastructureTemplate,
	clusterTemplate *oran.ClusterTemplateBuilder) bool {
	if managedInfrastructureTemplate.Name != clusterTemplate.Definition.Spec.Name ||
		managedInfrastructureTemplate.Version != clusterTemplate.Definition.Spec.Version ||
		managedInfrastructureTemplate.Description != clusterTemplate.Definition.Spec.Description ||
		managedInfrastructureTemplate.ArtifactResourceId.String() != clusterTemplate.Definition.Spec.TemplateID {
		return false
	}

	var clusterTemplateSchema map[string]any

	err := json.Unmarshal(clusterTemplate.Definition.Spec.TemplateParameterSchema.Raw, &clusterTemplateSchema)
	if err != nil {
		return false
	}

	return reflect.DeepEqual(managedInfrastructureTemplate.ParameterSchema, clusterTemplateSchema)
}
