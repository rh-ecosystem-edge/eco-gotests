package tests

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/assisted"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ocm"
	oranapi "github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api/fields"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api/filter"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/auth"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/o2imscluster"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/o2imstest"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
	mocksmo "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/oran-mock-smo"
)

var _ = Describe("ORAN Cluster API Tests", Label(tsparams.LabelPostProvision, tsparams.LabelClusterAPI), func() {
	var clusterClient *oranapi.ClusterClient

	BeforeEach(func() {
		By("creating the O2IMS cluster API client")

		clientBuilder, err := auth.NewClientBuilderForConfig(RANConfig)
		Expect(err).ToNot(HaveOccurred(), "Failed to create the O2IMS API client builder")

		clusterClient, err = clientBuilder.BuildCluster()
		Expect(err).ToNot(HaveOccurred(), "Failed to create the O2IMS cluster API client")
	})

	// 89867 - Retrieve all API versions
	It("retrieves all API versions", reportxml.ID("89867"), func() {
		By("getting all cluster API versions")

		allVersions, err := clusterClient.GetAllVersions()
		Expect(err).ToNot(HaveOccurred(), "Failed to get all cluster API versions")

		verifyErr := o2imstest.VerifyAPIVersions(allVersions, tsparams.ClusterAPIVersion, tsparams.ClusterAPIURIPrefix)
		Expect(verifyErr).ToNot(HaveOccurred(), "All API versions response failed verification")
	})

	// 89868 - Retrieve minor API versions
	It("retrieves minor API versions", reportxml.ID("89868"), func() {
		By("getting minor cluster API versions")

		minorVersions, err := clusterClient.GetMinorVersions()
		Expect(err).ToNot(HaveOccurred(), "Failed to get minor cluster API versions")

		verifyErr := o2imstest.VerifyAPIVersions(minorVersions, tsparams.ClusterAPIVersion, tsparams.ClusterAPIURIPrefix)
		Expect(verifyErr).ToNot(HaveOccurred(), "Minor API versions response failed verification")
	})

	// 89869 - List node cluster types
	It("lists node cluster types", reportxml.ID("89869"), func() {
		By("collecting unique NodeClusterType keys from eligible ManagedClusters")

		expectedKeys, err := o2imscluster.CollectNodeClusterTypeKeys(HubAPIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to collect NodeClusterType keys from ManagedClusters")
		Expect(len(expectedKeys)).To(BeNumerically(">=", 2),
			"At least hub-cluster and managed-cluster types are required")

		hasHub := false
		hasManaged := false

		for key := range expectedKeys {
			if key.Model == tsparams.ClusterModelHubCluster {
				hasHub = true
			}

			if key.Model == tsparams.ClusterModelManagedCluster {
				hasManaged = true
			}
		}

		Expect(hasHub).To(BeTrue(), "Expected a hub-cluster NodeClusterType key")
		Expect(hasManaged).To(BeTrue(), "Expected a managed-cluster NodeClusterType key")

		By("listing NodeClusterTypes from the cluster API")

		apiTypes, err := clusterClient.ListNodeClusterTypes()
		Expect(err).ToNot(HaveOccurred(), "Failed to list NodeClusterTypes")
		Expect(apiTypes).To(HaveLen(len(expectedKeys)),
			"Number of API NodeClusterTypes should match unique ManagedCluster combinations")

		By("verifying each unique combination has a matching NodeClusterType")

		for key := range expectedKeys {
			typeIndex := slices.IndexFunc(apiTypes, func(nodeType oranapi.NodeClusterType) bool {
				return nodeType.Name == key.Name()
			})
			Expect(typeIndex).ToNot(Equal(-1), "Missing NodeClusterType named %s", key.Name())
			Expect(apiTypes[typeIndex].NodeClusterTypeId).ToNot(Equal(uuid.Nil),
				"NodeClusterType %s should have nodeClusterTypeId populated", key.Name())
			Expect(apiTypes[typeIndex].Description).ToNot(BeEmpty(),
				"NodeClusterType %s should have description populated", key.Name())

			verifyErr := o2imscluster.VerifyNodeClusterTypeMatchesKey(apiTypes[typeIndex], key)
			Expect(verifyErr).ToNot(HaveOccurred(),
				"NodeClusterType %s failed verification", key.Name())
		}
	})

	// 89870 - Get a specific node cluster type
	It("gets a specific node cluster type", reportxml.ID("89870"), func() {
		By("listing NodeClusterTypes to obtain a valid ID")

		apiTypes, err := clusterClient.ListNodeClusterTypes()
		Expect(err).ToNot(HaveOccurred(), "Failed to list NodeClusterTypes")
		Expect(apiTypes).ToNot(BeEmpty(), "At least one NodeClusterType is required")

		chosen := apiTypes[0]

		By("retrieving the NodeClusterType by ID")

		retrieved, err := clusterClient.GetNodeClusterType(chosen.NodeClusterTypeId)
		Expect(err).ToNot(HaveOccurred(), "Failed to get NodeClusterType %s", chosen.NodeClusterTypeId)
		Expect(retrieved.NodeClusterTypeId).To(Equal(chosen.NodeClusterTypeId),
			"Retrieved NodeClusterType ID should match requested ID")
		Expect(retrieved.Name).ToNot(BeEmpty(), "name should be populated")
		Expect(retrieved.Description).ToNot(BeEmpty(), "description should be populated")

		By("verifying the NodeClusterType against eligible ManagedClusters")

		eligible, err := ocm.ListNodeClusterEligibleManagedClusters(HubAPIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to list eligible ManagedClusters")

		matchingClusters := 0

		for _, cluster := range eligible {
			key, keyErr := o2imscluster.NodeClusterTypeKeyFromManagedCluster(cluster)
			Expect(keyErr).ToNot(HaveOccurred(),
				"Failed to derive NodeClusterType key for ManagedCluster %s", cluster.Definition.Name)

			if key.Name() != retrieved.Name {
				continue
			}

			matchingClusters++
			verifyErr := o2imscluster.VerifyNodeClusterTypeMatchesKey(retrieved, key)
			Expect(verifyErr).ToNot(HaveOccurred(),
				"NodeClusterType %s does not match ManagedCluster %s", retrieved.Name, cluster.Definition.Name)
		}

		Expect(matchingClusters).To(BeNumerically(">=", 1),
			"At least one eligible ManagedCluster should map to NodeClusterType %s", retrieved.Name)
	})

	// 89871 - Get alarm dictionary for a node cluster type
	It("gets an alarm dictionary for a node cluster type", reportxml.ID("89871"), func() {
		By("listing NodeClusterTypes to obtain a valid ID")

		apiTypes, err := clusterClient.ListNodeClusterTypes()
		Expect(err).ToNot(HaveOccurred(), "Failed to list NodeClusterTypes")
		Expect(apiTypes).ToNot(BeEmpty(), "At least one NodeClusterType is required")

		chosen := apiTypes[0]

		By("retrieving the NodeClusterType alarm dictionary")

		dictionary, err := clusterClient.GetNodeClusterTypeAlarmDictionary(chosen.NodeClusterTypeId)
		Expect(err).ToNot(HaveOccurred(),
			"Failed to get alarm dictionary for NodeClusterType %s", chosen.NodeClusterTypeId)

		verifyErr := o2imstest.VerifyAlarmDictionaryStructure(dictionary)
		Expect(verifyErr).ToNot(HaveOccurred(),
			"Alarm dictionary for NodeClusterType %s failed verification", chosen.NodeClusterTypeId)
	})

	// 89872 - List node clusters
	It("lists node clusters", reportxml.ID("89872"), func() {
		By("listing eligible ManagedClusters on the hub")

		eligible, err := ocm.ListNodeClusterEligibleManagedClusters(HubAPIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to list eligible ManagedClusters")
		Expect(len(eligible)).To(BeNumerically(">=", 2),
			"At least local-cluster and one spoke ManagedCluster are required")

		By("listing NodeClusters from the cluster API")

		apiClusters, err := clusterClient.ListNodeClusters()
		Expect(err).ToNot(HaveOccurred(), "Failed to list NodeClusters")
		Expect(apiClusters).To(HaveLen(len(eligible)),
			"Number of API NodeClusters should match eligible ManagedClusters")

		apiTypes, err := clusterClient.ListNodeClusterTypes()
		Expect(err).ToNot(HaveOccurred(), "Failed to list NodeClusterTypes")

		apiResources, err := clusterClient.ListClusterResources()
		Expect(err).ToNot(HaveOccurred(), "Failed to list ClusterResources")

		By("verifying each eligible ManagedCluster matches an API NodeCluster")

		for _, cluster := range eligible {
			clusterIdx := slices.IndexFunc(apiClusters, func(nodeCluster oranapi.NodeCluster) bool {
				return nodeCluster.Name == cluster.Definition.Name
			})
			Expect(clusterIdx).ToNot(Equal(-1),
				"ManagedCluster %s missing from API response", cluster.Definition.Name)

			verifyErr := o2imscluster.VerifyNodeClusterMatchesManagedCluster(
				apiClusters[clusterIdx], cluster, apiTypes)
			Expect(verifyErr).ToNot(HaveOccurred(),
				"ManagedCluster %s does not match API NodeCluster", cluster.Definition.Name)

			if len(apiClusters[clusterIdx].ClusterResourceIds) > 0 {
				verifyErr = o2imscluster.VerifyClusterResourceIDsExist(
					apiClusters[clusterIdx].ClusterResourceIds, apiResources)
				Expect(verifyErr).ToNot(HaveOccurred(),
					"NodeCluster %s has invalid clusterResourceIds", cluster.Definition.Name)
			}
		}
	})

	// 89873 - Get a specific node cluster
	It("gets a specific node cluster", reportxml.ID("89873"), func() {
		By("selecting an eligible ManagedCluster and reading its clusterID label")

		spoke, err := ocm.PullManagedCluster(HubAPIClient, RANConfig.Spoke1Name)
		Expect(err).ToNot(HaveOccurred(), "Failed to pull spoke ManagedCluster")
		Expect(ocm.IsNodeClusterEligibleManagedCluster(spoke.Definition)).To(BeTrue(),
			"ManagedCluster %s is not eligible for NodeCluster conversion", spoke.Definition.Name)

		nodeClusterID, err := o2imscluster.ManagedClusterID(spoke)
		Expect(err).ToNot(HaveOccurred(), "Failed to parse ManagedCluster clusterID")

		By("retrieving the NodeCluster by clusterID")

		retrieved, err := clusterClient.GetNodeCluster(nodeClusterID)
		Expect(err).ToNot(HaveOccurred(), "Failed to get NodeCluster %s", nodeClusterID)
		Expect(retrieved.NodeClusterId).To(Equal(nodeClusterID),
			"Retrieved nodeClusterId should match ManagedCluster clusterID")

		apiTypes, err := clusterClient.ListNodeClusterTypes()
		Expect(err).ToNot(HaveOccurred(), "Failed to list NodeClusterTypes")

		verifyErr := o2imscluster.VerifyNodeClusterMatchesManagedCluster(retrieved, spoke, apiTypes)
		Expect(verifyErr).ToNot(HaveOccurred(),
			"ManagedCluster %s does not match retrieved NodeCluster", spoke.Definition.Name)
	})

	// 89874 - Filter node clusters by name
	It("filters node clusters by name", reportxml.ID("89874"), func() {
		By("selecting an eligible spoke ManagedCluster name")

		spoke, err := ocm.PullManagedCluster(HubAPIClient, RANConfig.Spoke1Name)
		Expect(err).ToNot(HaveOccurred(), "Failed to pull spoke ManagedCluster")
		Expect(ocm.IsNodeClusterEligibleManagedCluster(spoke.Definition)).To(BeTrue(),
			"ManagedCluster %s is not eligible for NodeCluster conversion", spoke.Definition.Name)

		spokeID, err := o2imscluster.ManagedClusterID(spoke)
		Expect(err).ToNot(HaveOccurred(), "Failed to parse ManagedCluster clusterID")

		By("filtering NodeClusters by name equals the spoke name")

		eqClusters, err := clusterClient.ListNodeClusters(oranapi.WithFilter(filter.Equals("name", RANConfig.Spoke1Name)))
		Expect(err).ToNot(HaveOccurred(), "Failed to filter NodeClusters by name equals")
		Expect(eqClusters).To(HaveLen(1), "Filter name=%s should return exactly one NodeCluster", RANConfig.Spoke1Name)
		Expect(eqClusters[0].Name).To(Equal(RANConfig.Spoke1Name), "Filtered NodeCluster name should match")
		Expect(eqClusters[0].NodeClusterId).To(Equal(spokeID),
			"Filtered NodeCluster ID should match ManagedCluster clusterID")

		By("filtering NodeClusters by name not-equals the spoke name")

		neqClusters, err := clusterClient.ListNodeClusters(
			oranapi.WithFilter(filter.DoesNotEqual("name", RANConfig.Spoke1Name)))
		Expect(err).ToNot(HaveOccurred(), "Failed to filter NodeClusters by name not-equals")

		containsSpoke := slices.ContainsFunc(neqClusters, func(nodeCluster oranapi.NodeCluster) bool {
			return nodeCluster.Name == RANConfig.Spoke1Name
		})
		Expect(containsSpoke).To(BeFalse(), "Filtered list should not include %s", RANConfig.Spoke1Name)

		containsHub := slices.ContainsFunc(neqClusters, func(nodeCluster oranapi.NodeCluster) bool {
			if nodeCluster.Name == "local-cluster" {
				return true
			}

			return nodeCluster.Extensions != nil &&
				o2imstest.ExtensionString(*nodeCluster.Extensions, tsparams.ClusterModelExtension) ==
					tsparams.ClusterModelHubCluster
		})
		Expect(containsHub).To(BeTrue(), "Filtered list should still include the hub NodeCluster")
	})

	// 89875 - Use field selection on node clusters
	It("supports field selection on node clusters", reportxml.ID("89875"), func() {
		By("listing NodeClusters with fields=nodeClusterId,name")

		withFields, err := clusterClient.ListNodeClusters(
			oranapi.WithFields(fields.Include("nodeClusterId", "name")))
		Expect(err).ToNot(HaveOccurred(), "Failed to list NodeClusters with fields selection")
		Expect(withFields).ToNot(BeEmpty(), "At least one NodeCluster is required")

		for _, nodeCluster := range withFields {
			Expect(nodeCluster.NodeClusterId).ToNot(Equal(uuid.Nil), "nodeClusterId is mandatory")
			Expect(nodeCluster.Name).ToNot(BeEmpty(), "name is mandatory")
			Expect(nodeCluster.Extensions).To(BeNil(),
				"extensions should be absent when not selected via fields")
		}

		By("listing NodeClusters with exclude_fields=extensions")

		withoutExtensions, err := clusterClient.ListNodeClusters(
			oranapi.WithFields(fields.Exclude("extensions")))
		Expect(err).ToNot(HaveOccurred(), "Failed to list NodeClusters excluding extensions")
		Expect(withoutExtensions).ToNot(BeEmpty(), "At least one NodeCluster is required")

		for _, nodeCluster := range withoutExtensions {
			Expect(nodeCluster.NodeClusterId).ToNot(Equal(uuid.Nil), "nodeClusterId is mandatory")
			Expect(nodeCluster.Name).ToNot(BeEmpty(), "name is mandatory")
			Expect(nodeCluster.Description).ToNot(BeEmpty(), "description should remain present")
			Expect(nodeCluster.NodeClusterTypeId).ToNot(Equal(uuid.Nil), "nodeClusterTypeId should remain present")
			Expect(nodeCluster.Extensions).To(BeNil(), "extensions should be absent when excluded")
		}
	})

	// 89876 - List cluster resource types
	It("lists cluster resource types", reportxml.ID("89876"), func() {
		By("collecting unique ClusterResourceType keys from ORAN-eligible Agents")

		expectedKeys, err := o2imscluster.CollectClusterResourceTypeKeys(HubAPIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to collect ClusterResourceType keys from Agents")
		Expect(expectedKeys).ToNot(BeEmpty(),
			"At least one ORAN-eligible assisted-service Agent is required")

		By("listing ClusterResourceTypes from the cluster API")

		apiTypes, err := clusterClient.ListClusterResourceTypes()
		Expect(err).ToNot(HaveOccurred(), "Failed to list ClusterResourceTypes")
		Expect(apiTypes).To(HaveLen(len(expectedKeys)),
			"Number of API ClusterResourceTypes should match unique Agent CPU combinations")

		By("verifying each unique Agent type has a matching ClusterResourceType")

		for key := range expectedKeys {
			typeIndex := slices.IndexFunc(apiTypes, func(resourceType oranapi.ClusterResourceType) bool {
				return resourceType.Name == key.Name()
			})
			Expect(typeIndex).ToNot(Equal(-1), "Missing ClusterResourceType for %s", key.Name())
			verifyErr := o2imscluster.VerifyClusterResourceTypeMatchesKey(apiTypes[typeIndex], key)
			Expect(verifyErr).ToNot(HaveOccurred(),
				"ClusterResourceType %s failed verification", key.Name())
		}
	})

	// 89877 - Get a specific cluster resource type
	It("gets a specific cluster resource type", reportxml.ID("89877"), func() {
		By("listing ClusterResourceTypes to obtain a valid ID")

		apiTypes, err := clusterClient.ListClusterResourceTypes()
		Expect(err).ToNot(HaveOccurred(), "Failed to list ClusterResourceTypes")
		Expect(apiTypes).ToNot(BeEmpty(),
			"At least one ClusterResourceType is required (assisted-installer Agents)")

		chosen := apiTypes[0]

		By("retrieving the ClusterResourceType by ID")

		retrieved, err := clusterClient.GetClusterResourceType(chosen.ClusterResourceTypeId)
		Expect(err).ToNot(HaveOccurred(), "Failed to get ClusterResourceType %s", chosen.ClusterResourceTypeId)
		Expect(retrieved.ClusterResourceTypeId).To(Equal(chosen.ClusterResourceTypeId),
			"Retrieved ClusterResourceType ID should match requested ID")

		By("verifying the ClusterResourceType against ORAN-eligible Agents")

		agents, err := assisted.ListORANEligibleAgents(HubAPIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to list ORAN-eligible Agents")

		matchingAgents := 0

		for _, agent := range agents {
			key := o2imscluster.ClusterResourceTypeKeyFromAgent(agent.Definition)
			if key.Name() != retrieved.Name {
				continue
			}

			matchingAgents++
			verifyErr := o2imscluster.VerifyClusterResourceTypeMatchesKey(retrieved, key)
			Expect(verifyErr).ToNot(HaveOccurred(),
				"ClusterResourceType %s does not match Agent %s/%s",
				retrieved.Name, agent.Definition.Namespace, agent.Definition.Name)
		}

		Expect(matchingAgents).To(BeNumerically(">=", 1),
			"At least one Agent should map to ClusterResourceType %s", retrieved.Name)
	})

	// 89878 - List cluster resources
	It("lists cluster resources", reportxml.ID("89878"), func() {
		By("listing ORAN-eligible Agents on the hub")

		agents, err := assisted.ListORANEligibleAgents(HubAPIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to list ORAN-eligible Agents")
		Expect(agents).ToNot(BeEmpty(),
			"At least one ORAN-eligible assisted-service Agent is required")

		By("listing ClusterResourceTypes for resource association verification")

		apiTypes, err := clusterClient.ListClusterResourceTypes()
		Expect(err).ToNot(HaveOccurred(), "Failed to list ClusterResourceTypes")

		By("listing ClusterResources from the cluster API")

		apiResources, err := clusterClient.ListClusterResources()
		Expect(err).ToNot(HaveOccurred(), "Failed to list ClusterResources")
		Expect(apiResources).To(HaveLen(len(agents)),
			"Number of API ClusterResources should match ORAN-eligible Agents")

		By("verifying each eligible Agent matches an API ClusterResource")

		for _, agent := range agents {
			matched, matchErr := o2imscluster.FindClusterResourceForAgent(apiResources, agent.Definition)
			Expect(matchErr).ToNot(HaveOccurred(),
				"Agent %s missing from API response", o2imscluster.ExpectedAgentExternalID(agent.Definition))

			verifyErr := o2imscluster.VerifyClusterResourceMatchesAgent(matched, agent.Definition, apiTypes)
			Expect(verifyErr).ToNot(HaveOccurred(),
				"Agent %s does not match API ClusterResource",
				o2imscluster.ExpectedAgentExternalID(agent.Definition))
		}
	})

	// 89879 - Get a specific cluster resource
	It("gets a specific cluster resource", reportxml.ID("89879"), func() {
		By("listing ClusterResources to obtain a valid ID and name")

		apiResources, err := clusterClient.ListClusterResources()
		Expect(err).ToNot(HaveOccurred(), "Failed to list ClusterResources")
		Expect(apiResources).ToNot(BeEmpty(),
			"At least one ClusterResource is required (assisted-installer Agents)")

		chosen := apiResources[0]

		By("listing ClusterResourceTypes for resource association verification")

		apiTypes, err := clusterClient.ListClusterResourceTypes()
		Expect(err).ToNot(HaveOccurred(), "Failed to list ClusterResourceTypes")

		By("retrieving the ClusterResource by ID")

		retrieved, err := clusterClient.GetClusterResource(chosen.ClusterResourceId)
		Expect(err).ToNot(HaveOccurred(), "Failed to get ClusterResource %s", chosen.ClusterResourceId)
		Expect(retrieved.ClusterResourceId).To(Equal(chosen.ClusterResourceId),
			"Retrieved ClusterResource ID should match requested ID")
		Expect(retrieved.ClusterResourceTypeId).ToNot(Equal(uuid.Nil),
			"clusterResourceTypeId should be populated")

		By("cross-checking the ClusterResource against the matching Agent")

		agents, err := assisted.ListORANEligibleAgents(HubAPIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to list ORAN-eligible Agents")

		matched := false

		for _, agent := range agents {
			_, matchErr := o2imscluster.FindClusterResourceForAgent(
				[]oranapi.ClusterResource{retrieved}, agent.Definition)
			if matchErr != nil {
				continue
			}

			matched = true
			verifyErr := o2imscluster.VerifyClusterResourceMatchesAgent(retrieved, agent.Definition, apiTypes)
			Expect(verifyErr).ToNot(HaveOccurred(),
				"Agent %s does not match retrieved ClusterResource",
				o2imscluster.ExpectedAgentExternalID(agent.Definition))

			break
		}

		Expect(matched).To(BeTrue(),
			"No Agent matched ClusterResource %s (name %s)",
			retrieved.ClusterResourceId, retrieved.Name)
	})

	// 89880 - Create a cluster subscription
	It("creates a cluster subscription", reportxml.ID("89880"), func() {
		By("creating a cluster subscription")

		consumerSubscriptionID := uuid.New()
		callbackURL := mocksmo.ObserverCallbackURL(mockSMOBaseURL, consumerSubscriptionID.String())
		created, err := clusterClient.CreateClusterSubscription(oranapi.ClusterSubscription{
			ConsumerSubscriptionId: &consumerSubscriptionID,
			Callback:               callbackURL,
		})
		Expect(err).ToNot(HaveOccurred(), "Failed to create cluster subscription")
		Expect(created.SubscriptionId).ToNot(BeNil(), "subscriptionId should be assigned")

		subscriptionID := *created.SubscriptionId

		DeferCleanup(func() {
			err := clusterClient.DeleteClusterSubscription(subscriptionID)
			Expect(err).ToNot(HaveOccurred(), "Failed to delete cluster subscription during cleanup")
		})

		Expect(created.Callback).To(Equal(callbackURL), "callback should match request")
		Expect(created.ConsumerSubscriptionId).ToNot(BeNil(), "consumerSubscriptionId should be present")
		Expect(*created.ConsumerSubscriptionId).To(Equal(consumerSubscriptionID),
			"consumerSubscriptionId should match request")
	})

	// 89881 - List cluster subscriptions
	It("lists cluster subscriptions", reportxml.ID("89881"), func() {
		By("creating a cluster subscription")

		consumerSubscriptionID := uuid.New()
		callbackURL := mocksmo.ObserverCallbackURL(mockSMOBaseURL, consumerSubscriptionID.String())
		created, err := clusterClient.CreateClusterSubscription(oranapi.ClusterSubscription{
			ConsumerSubscriptionId: &consumerSubscriptionID,
			Callback:               callbackURL,
		})
		Expect(err).ToNot(HaveOccurred(), "Failed to create cluster subscription")
		Expect(created.SubscriptionId).ToNot(BeNil(), "subscriptionId should be assigned")

		subscriptionID := *created.SubscriptionId

		DeferCleanup(func() {
			err := clusterClient.DeleteClusterSubscription(subscriptionID)
			Expect(err).ToNot(HaveOccurred(), "Failed to delete cluster subscription during cleanup")
		})

		By("listing cluster subscriptions")

		subscriptions, err := clusterClient.ListClusterSubscriptions()
		Expect(err).ToNot(HaveOccurred(), "Failed to list cluster subscriptions")

		found := slices.ContainsFunc(subscriptions, func(subscription oranapi.ClusterSubscription) bool {
			return subscription.SubscriptionId != nil &&
				*subscription.SubscriptionId == subscriptionID &&
				subscription.Callback == callbackURL
		})
		Expect(found).To(BeTrue(), "Created subscription should appear in the list")
	})

	// 89882 - Get a specific cluster subscription
	It("gets a specific cluster subscription", reportxml.ID("89882"), func() {
		By("creating a cluster subscription")

		consumerSubscriptionID := uuid.New()
		callbackURL := mocksmo.ObserverCallbackURL(mockSMOBaseURL, consumerSubscriptionID.String())
		created, err := clusterClient.CreateClusterSubscription(oranapi.ClusterSubscription{
			ConsumerSubscriptionId: &consumerSubscriptionID,
			Callback:               callbackURL,
		})
		Expect(err).ToNot(HaveOccurred(), "Failed to create cluster subscription")
		Expect(created.SubscriptionId).ToNot(BeNil(), "subscriptionId should be assigned")

		subscriptionID := *created.SubscriptionId

		DeferCleanup(func() {
			err := clusterClient.DeleteClusterSubscription(subscriptionID)
			Expect(err).ToNot(HaveOccurred(), "Failed to delete cluster subscription during cleanup")
		})

		By("retrieving the cluster subscription by ID")

		retrieved, err := clusterClient.GetClusterSubscription(subscriptionID)
		Expect(err).ToNot(HaveOccurred(), "Failed to get cluster subscription")
		Expect(retrieved.SubscriptionId).ToNot(BeNil(), "subscriptionId should be present")
		Expect(*retrieved.SubscriptionId).To(Equal(subscriptionID),
			"Retrieved subscription ID should match created subscription")
		Expect(retrieved.Callback).To(Equal(callbackURL),
			"Retrieved callback should match created subscription")
		Expect(retrieved.ConsumerSubscriptionId).ToNot(BeNil(), "consumerSubscriptionId should be present")
		Expect(*retrieved.ConsumerSubscriptionId).To(Equal(consumerSubscriptionID),
			"Retrieved consumer subscription ID should match created subscription")
	})

	// 89883 - Delete a cluster subscription
	It("deletes a cluster subscription", reportxml.ID("89883"), func() {
		By("creating a cluster subscription")

		consumerSubscriptionID := uuid.New()
		created, err := clusterClient.CreateClusterSubscription(oranapi.ClusterSubscription{
			ConsumerSubscriptionId: &consumerSubscriptionID,
			Callback:               mocksmo.ObserverCallbackURL(mockSMOBaseURL, consumerSubscriptionID.String()),
		})
		Expect(err).ToNot(HaveOccurred(), "Failed to create cluster subscription")
		Expect(created.SubscriptionId).ToNot(BeNil(), "subscriptionId should be assigned")

		subscriptionID := *created.SubscriptionId

		By("deleting the cluster subscription")

		err = clusterClient.DeleteClusterSubscription(subscriptionID)
		Expect(err).ToNot(HaveOccurred(), "Failed to delete cluster subscription")

		By("verifying the deleted subscription returns 404")

		_, err = clusterClient.GetClusterSubscription(subscriptionID)
		apiErr := oranapi.AsAPIError(err)
		Expect(apiErr).ToNot(BeNil(), "Expected API error after deleting subscription")
		Expect(apiErr.Status).To(Equal(http.StatusNotFound),
			"Deleted subscription should return HTTP 404")
	})

	// 89884 - List alarm dictionaries
	It("lists alarm dictionaries", reportxml.ID("89884"), func() {
		By("listing NodeClusterTypes to determine the expected dictionary count")

		apiTypes, err := clusterClient.ListNodeClusterTypes()
		Expect(err).ToNot(HaveOccurred(), "Failed to list NodeClusterTypes")
		Expect(apiTypes).ToNot(BeEmpty(), "At least one NodeClusterType is required")

		By("listing alarm dictionaries from the cluster API")

		dictionaries, err := clusterClient.ListAlarmDictionaries()
		Expect(err).ToNot(HaveOccurred(), "Failed to list alarm dictionaries")
		Expect(dictionaries).To(HaveLen(len(apiTypes)),
			"Number of alarm dictionaries should match NodeClusterTypes")

		By("verifying each alarm dictionary structure")

		for _, dictionary := range dictionaries {
			verifyErr := o2imstest.VerifyAlarmDictionaryStructure(dictionary)
			Expect(verifyErr).ToNot(HaveOccurred(),
				"Alarm dictionary %s failed verification", dictionary.AlarmDictionaryId)
		}
	})

	// 89885 - Get a specific alarm dictionary
	It("gets a specific alarm dictionary", reportxml.ID("89885"), func() {
		By("listing alarm dictionaries to obtain a valid ID")

		dictionaries, err := clusterClient.ListAlarmDictionaries()
		Expect(err).ToNot(HaveOccurred(), "Failed to list alarm dictionaries")
		Expect(dictionaries).ToNot(BeEmpty(), "At least one AlarmDictionary is required")

		chosen := dictionaries[0]

		By("retrieving the alarm dictionary by ID")

		retrieved, err := clusterClient.GetAlarmDictionary(chosen.AlarmDictionaryId)
		Expect(err).ToNot(HaveOccurred(), "Failed to get AlarmDictionary %s", chosen.AlarmDictionaryId)
		Expect(retrieved.AlarmDictionaryId).To(Equal(chosen.AlarmDictionaryId),
			"Retrieved AlarmDictionary ID should match requested ID")

		verifyErr := o2imstest.VerifyAlarmDictionaryStructure(retrieved)
		Expect(verifyErr).ToNot(HaveOccurred(),
			"Retrieved AlarmDictionary %s failed verification", retrieved.AlarmDictionaryId)
	})

	// 89886 - Verify subscription notification on cluster change
	It("receives a cluster change notification from a subscription", reportxml.ID("89886"), func() {
		By("selecting an eligible spoke ManagedCluster")

		spoke, err := ocm.PullManagedCluster(HubAPIClient, RANConfig.Spoke1Name)
		Expect(err).ToNot(HaveOccurred(), "Failed to pull spoke ManagedCluster")
		Expect(ocm.IsNodeClusterEligibleManagedCluster(spoke.Definition)).To(BeTrue(),
			"ManagedCluster %s is not eligible for NodeCluster conversion", spoke.Definition.Name)

		nodeClusterID, err := o2imscluster.ManagedClusterID(spoke)
		Expect(err).ToNot(HaveOccurred(), "Failed to parse ManagedCluster clusterID")

		By("creating a cluster subscription")

		consumerSubscriptionID := uuid.New()
		created, err := clusterClient.CreateClusterSubscription(oranapi.ClusterSubscription{
			ConsumerSubscriptionId: &consumerSubscriptionID,
			Callback:               mocksmo.ObserverCallbackURL(mockSMOBaseURL, consumerSubscriptionID.String()),
		})
		Expect(err).ToNot(HaveOccurred(), "Failed to create cluster subscription")
		Expect(created.SubscriptionId).ToNot(BeNil(), "subscriptionId should be assigned")

		subscriptionID := *created.SubscriptionId

		DeferCleanup(func() {
			err := clusterClient.DeleteClusterSubscription(subscriptionID)
			Expect(err).ToNot(HaveOccurred(), "Failed to delete cluster subscription during cleanup")
		})

		By("adding a unique test label to the spoke ManagedCluster")

		labelValue := uuid.NewString()
		changeTime := time.Now()

		if spoke.Definition.Labels == nil {
			spoke.Definition.Labels = map[string]string{}
		}

		spoke.Definition.Labels[tsparams.TestNotificationLabel] = labelValue
		spoke, err = spoke.Update()
		Expect(err).ToNot(HaveOccurred(), "Failed to label ManagedCluster %s", spoke.Definition.Name)

		DeferCleanup(func() {
			cluster, pullErr := ocm.PullManagedCluster(HubAPIClient, spoke.Definition.Name)
			Expect(pullErr).ToNot(HaveOccurred(),
				"Failed to pull ManagedCluster %s during cleanup", spoke.Definition.Name)

			if cluster.Definition.Labels != nil {
				delete(cluster.Definition.Labels, tsparams.TestNotificationLabel)
				_, updateErr := cluster.Update()
				Expect(updateErr).ToNot(HaveOccurred(),
					"Failed to remove test label from ManagedCluster %s during cleanup", spoke.Definition.Name)
			}
		})

		By("waiting for a MODIFY cluster change notification")

		err = mocksmo.WaitFor(HubAPIClient, RANConfig.MockSMONamespace,
			mocksmo.WithStart[oranapi.ClusterChangeNotification](changeTime),
			mocksmo.WithObserverID[oranapi.ClusterChangeNotification](consumerSubscriptionID.String()),
			mocksmo.WithTimeout[oranapi.ClusterChangeNotification](2*time.Minute),
			mocksmo.WithMatch(o2imscluster.MatchModifyNodeCluster(
				consumerSubscriptionID,
				nodeClusterID,
				spoke.Definition.Name,
				tsparams.TestNotificationLabel,
				labelValue,
			)),
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to receive MODIFY cluster change notification")
	})

	// 89887 - Get a non-existent node cluster returns 404
	It("returns 404 for a non-existent node cluster", reportxml.ID("89887"), func() {
		By("requesting a non-existent NodeCluster")

		nonexistentID := uuid.MustParse(tsparams.NonExistentUUID)
		_, err := clusterClient.GetNodeCluster(nonexistentID)
		apiErr := oranapi.AsAPIError(err)
		Expect(apiErr).ToNot(BeNil(), "Expected API error when requesting non-existent NodeCluster")
		Expect(apiErr.Status).To(Equal(http.StatusNotFound),
			"Expected 404 when requesting non-existent NodeCluster")
	})

	// 89888 - Delete a non-existent subscription returns 404
	It("returns 404 when deleting a non-existent subscription", reportxml.ID("89888"), func() {
		By("deleting a non-existent cluster subscription")

		nonexistentID := uuid.MustParse(tsparams.NonExistentUUID)
		err := clusterClient.DeleteClusterSubscription(nonexistentID)
		apiErr := oranapi.AsAPIError(err)
		Expect(apiErr).ToNot(BeNil(), "Expected API error when deleting non-existent subscription")
		Expect(apiErr.Status).To(Equal(http.StatusNotFound),
			"Expected 404 when deleting non-existent subscription")
	})

	// 89889 - Create subscription without required callback returns 400
	It("rejects subscriptions without a callback", reportxml.ID("89889"), func() {
		By("attempting to create a subscription without a callback field")

		consumerSubscriptionID := uuid.New()
		body := fmt.Sprintf(`{"consumerSubscriptionId":%q}`, consumerSubscriptionID.String())
		resp, err := clusterClient.CreateSubscriptionWithBodyWithResponse(
			context.TODO(), "application/json", strings.NewReader(body))
		Expect(err).ToNot(HaveOccurred(), "Failed to send create subscription request")
		Expect(resp.StatusCode()).To(Equal(http.StatusBadRequest),
			"Expected 400 when creating subscription without callback")
	})
})
