package tests

import (
	"net/http"
	"slices"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ocm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran"
	oranapi "github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api/fields"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api/filter"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/auth"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/inventory"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
	mocksmo "github.com/rh-ecosystem-edge/eco-gotests/tests/internal/oran-mock-smo"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("ORAN Inventory API Tests", Label(tsparams.LabelPostProvision, tsparams.LabelInventory), func() {
	var inventoryClient *oranapi.InventoryClient

	BeforeEach(func() {
		By("creating the O2IMS inventory API client")

		clientBuilder, err := auth.NewClientBuilderForConfig(RANConfig)
		Expect(err).ToNot(HaveOccurred(), "Failed to create the O2IMS API client builder")

		inventoryClient, err = clientBuilder.BuildInventory()
		Expect(err).ToNot(HaveOccurred(), "Failed to create the O2IMS inventory API client")
	})

	// 89890 - Retrieve O-Cloud information
	It("retrieves O-Cloud information", reportxml.ID("89890"), func() {
		By("getting O-Cloud information from the inventory API")

		cloudInfo, err := inventoryClient.GetCloudInfo()
		Expect(err).ToNot(HaveOccurred(), "Failed to get O-Cloud information")

		By("verifying O-Cloud identity fields are populated")
		Expect(cloudInfo.OCloudId).ToNot(Equal(uuid.Nil), "oCloudId should be populated")
		Expect(cloudInfo.GlobalCloudId).ToNot(Equal(uuid.Nil), "globalCloudId should be populated")
		Expect(cloudInfo.Name).ToNot(BeEmpty(), "name should be populated")
		Expect(cloudInfo.Description).ToNot(BeEmpty(), "description should be populated")
		Expect(cloudInfo.ServiceUri).ToNot(BeEmpty(), "serviceUri should be populated")
	})

	// 89891 - Retrieve API versions
	It("retrieves API versions", reportxml.ID("89891"), func() {
		By("getting all inventory API versions")

		allVersions, err := inventoryClient.GetAllVersions()
		Expect(err).ToNot(HaveOccurred(), "Failed to get all inventory API versions")

		verifyErr := inventory.VerifyAPIVersions(allVersions)
		Expect(verifyErr).ToNot(HaveOccurred(), "All API versions response failed verification")

		By("getting minor inventory API versions")

		minorVersions, err := inventoryClient.GetMinorVersions()
		Expect(err).ToNot(HaveOccurred(), "Failed to get minor inventory API versions")

		verifyErr = inventory.VerifyAPIVersions(minorVersions)
		Expect(verifyErr).ToNot(HaveOccurred(), "Minor API versions response failed verification")
	})

	// 89892 - List and retrieve Locations
	It("lists and retrieves Locations", reportxml.ID("89892"), func() {
		By("listing Ready Location CRs on the hub")

		readyLocations, err := oran.ListReadyLocations(HubAPIClient, client.InNamespace(tsparams.O2IMSNamespace))
		Expect(err).ToNot(HaveOccurred(), "Failed to list Ready Location CRs")
		Expect(readyLocations).ToNot(BeEmpty(), "At least one Ready Location CR is required")

		By("listing Locations from the inventory API")

		apiLocations, err := inventoryClient.ListLocations()
		Expect(err).ToNot(HaveOccurred(), "Failed to list Locations from the inventory API")
		Expect(apiLocations).To(HaveLen(len(readyLocations)),
			"Number of API Locations should match Ready Location CRs")

		By("verifying each Ready Location CR matches an API Location")

		readySites, err := oran.ListReadyOCloudSites(HubAPIClient, client.InNamespace(tsparams.O2IMSNamespace))
		Expect(err).ToNot(HaveOccurred(), "Failed to list Ready OCloudSite CRs")

		for _, locationCR := range readyLocations {
			locationIdx := slices.IndexFunc(apiLocations, func(loc oranapi.LocationInfo) bool {
				return loc.GlobalLocationId == locationCR.Definition.Name
			})
			Expect(locationIdx).ToNot(Equal(-1), "Location CR %s missing from API response", locationCR.Definition.Name)
			verifyErr := inventory.VerifyLocationMatchesCR(apiLocations[locationIdx], locationCR, readySites)
			Expect(verifyErr).ToNot(HaveOccurred(),
				"Location CR %s does not match API Location", locationCR.Definition.Name)
		}

		By("retrieving a Location by globalLocationId")

		chosen := apiLocations[0]
		retrieved, err := inventoryClient.GetLocation(chosen.GlobalLocationId)
		Expect(err).ToNot(HaveOccurred(), "Failed to get Location %s", chosen.GlobalLocationId)
		Expect(retrieved).To(Equal(chosen), "Retrieved Location should match the listed Location")
	})

	// 89893 - List and retrieve O-Cloud Sites
	It("lists and retrieves O-Cloud Sites", reportxml.ID("89893"), func() {
		By("listing Ready OCloudSite CRs on the hub")

		readySites, err := oran.ListReadyOCloudSites(HubAPIClient, client.InNamespace(tsparams.O2IMSNamespace))
		Expect(err).ToNot(HaveOccurred(), "Failed to list Ready OCloudSite CRs")
		Expect(readySites).ToNot(BeEmpty(), "At least one Ready OCloudSite CR is required")

		By("listing O-Cloud Sites from the inventory API")

		apiSites, err := inventoryClient.ListOCloudSites()
		Expect(err).ToNot(HaveOccurred(), "Failed to list O-Cloud Sites from the inventory API")
		Expect(apiSites).To(HaveLen(len(readySites)),
			"Number of API O-Cloud Sites should match Ready OCloudSite CRs")

		By("verifying each Ready OCloudSite CR matches an API O-Cloud Site")

		readyPools, err := oran.ListReadyResourcePools(HubAPIClient, client.InNamespace(tsparams.O2IMSNamespace))
		Expect(err).ToNot(HaveOccurred(), "Failed to list Ready ResourcePool CRs")

		for _, siteCR := range readySites {
			siteID := string(siteCR.Definition.UID)
			siteIdx := slices.IndexFunc(apiSites, func(site oranapi.OCloudSiteInfo) bool {
				return site.OCloudSiteId.String() == siteID
			})
			Expect(siteIdx).ToNot(Equal(-1), "OCloudSite CR %s missing from API response", siteCR.Definition.Name)
			verifyErr := inventory.VerifyOCloudSiteMatchesCR(apiSites[siteIdx], siteCR, readyPools)
			Expect(verifyErr).ToNot(HaveOccurred(),
				"OCloudSite CR %s does not match API O-Cloud Site", siteCR.Definition.Name)
		}

		By("retrieving an O-Cloud Site by oCloudSiteId")

		chosen := apiSites[0]
		retrieved, err := inventoryClient.GetOCloudSite(chosen.OCloudSiteId)
		Expect(err).ToNot(HaveOccurred(), "Failed to get O-Cloud Site %s", chosen.OCloudSiteId)
		Expect(retrieved).To(Equal(chosen), "Retrieved O-Cloud Site should match the listed site")
	})

	// 89894 - List and retrieve Resource Pools
	It("lists and retrieves Resource Pools", reportxml.ID("89894"), func() {
		By("listing Ready ResourcePool CRs on the hub")

		readyPools, err := oran.ListReadyResourcePools(HubAPIClient, client.InNamespace(tsparams.O2IMSNamespace))
		Expect(err).ToNot(HaveOccurred(), "Failed to list Ready ResourcePool CRs")
		Expect(readyPools).ToNot(BeEmpty(), "At least one Ready ResourcePool CR is required")

		By("listing Resource Pools from the inventory API")

		apiPools, err := inventoryClient.ListResourcePools()
		Expect(err).ToNot(HaveOccurred(), "Failed to list Resource Pools from the inventory API")
		Expect(apiPools).To(HaveLen(len(readyPools)),
			"Number of API Resource Pools should match Ready ResourcePool CRs")

		By("verifying each Ready ResourcePool CR matches an API Resource Pool")

		for _, poolCR := range readyPools {
			poolID := string(poolCR.Definition.UID)
			poolIdx := slices.IndexFunc(apiPools, func(pool oranapi.ResourcePool) bool {
				return pool.ResourcePoolId.String() == poolID
			})
			Expect(poolIdx).ToNot(Equal(-1), "ResourcePool CR %s missing from API response", poolCR.Definition.Name)
			verifyErr := inventory.VerifyResourcePoolMatchesCR(apiPools[poolIdx], poolCR)
			Expect(verifyErr).ToNot(HaveOccurred(),
				"ResourcePool CR %s does not match API Resource Pool", poolCR.Definition.Name)
		}

		By("retrieving a Resource Pool by resourcePoolId")

		chosen := apiPools[0]
		retrieved, err := inventoryClient.GetResourcePool(chosen.ResourcePoolId)
		Expect(err).ToNot(HaveOccurred(), "Failed to get Resource Pool %s", chosen.ResourcePoolId)
		Expect(retrieved.ResourcePoolId).To(Equal(chosen.ResourcePoolId),
			"Retrieved Resource Pool ID should match listed pool")
		Expect(retrieved.Name).To(Equal(chosen.Name),
			"Retrieved Resource Pool name should match listed pool")
		Expect(retrieved.Description).To(Equal(chosen.Description),
			"Retrieved Resource Pool description should match listed pool")
		Expect(retrieved.OCloudSiteId).To(Equal(chosen.OCloudSiteId),
			"Retrieved Resource Pool oCloudSiteId should match listed pool")
		Expect(retrieved.Extensions).ToNot(BeNil(), "Resource Pool extensions should be populated")
	})

	// 89895 - List and retrieve Resources in a Resource Pool
	It("lists and retrieves Resources in a Resource Pool", reportxml.ID("89895"), func() {
		By("identifying a ResourcePool with inventory-eligible BMHs")

		poolCR, expectedBMHs, err := inventory.FindResourcePoolWithInventoryBMHs(HubAPIClient, tsparams.O2IMSNamespace)
		Expect(err).ToNot(HaveOccurred(), "A ResourcePool with inventory-eligible BMHs is required")
		Expect(expectedBMHs).ToNot(BeEmpty(), "At least one inventory-eligible BMH is required")

		poolID, err := uuid.Parse(string(poolCR.Definition.UID))
		Expect(err).ToNot(HaveOccurred(), "Failed to parse ResourcePool UID")

		By("listing Resources from the inventory API")

		apiResources, err := inventoryClient.ListResources(poolID)
		Expect(err).ToNot(HaveOccurred(), "Failed to list Resources for pool %s", poolID)
		Expect(apiResources).To(HaveLen(len(expectedBMHs)),
			"Number of API Resources should match inventory-eligible BMHs")

		By("verifying each BMH matches an API Resource")

		for _, host := range expectedBMHs {
			resourceID := string(host.Definition.UID)
			resourceIdx := slices.IndexFunc(apiResources, func(resource oranapi.Resource) bool {
				return resource.ResourceId.String() == resourceID
			})
			Expect(resourceIdx).ToNot(Equal(-1), "BMH %s/%s missing from API response",
				host.Definition.Namespace, host.Definition.Name)
			verifyErr := inventory.VerifyResourceMatchesBMH(HubAPIClient, apiResources[resourceIdx], host, poolID)
			Expect(verifyErr).ToNot(HaveOccurred(),
				"BMH %s/%s does not match API Resource", host.Definition.Namespace, host.Definition.Name)
		}

		By("retrieving a Resource by resourceId")

		chosen := apiResources[0]
		retrieved, err := inventoryClient.GetResource(poolID, chosen.ResourceId)
		Expect(err).ToNot(HaveOccurred(), "Failed to get Resource %s", chosen.ResourceId)
		Expect(retrieved).To(Equal(chosen), "Retrieved Resource should match the listed Resource")
	})

	// 89896 - List and retrieve Resource Types
	It("lists and retrieves Resource Types", reportxml.ID("89896"), func() {
		By("collecting distinct vendor/model pairs from inventory-eligible BMHs")

		expectedPairs, err := inventory.CollectResourceTypePairs(HubAPIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to collect vendor/model pairs from inventory-eligible BMHs")
		Expect(expectedPairs).ToNot(BeEmpty(), "At least one vendor/model pair is required")

		By("listing Resource Types from the inventory API")

		apiResourceTypes, err := inventoryClient.ListResourceTypes()
		Expect(err).ToNot(HaveOccurred(), "Failed to list Resource Types")
		Expect(apiResourceTypes).To(HaveLen(len(expectedPairs)),
			"Number of API Resource Types should match distinct vendor/model pairs")

		By("verifying each vendor/model pair has a matching Resource Type")

		for pair := range expectedPairs {
			typeIdx := slices.IndexFunc(apiResourceTypes, func(resourceType oranapi.ResourceType) bool {
				return resourceType.Vendor == pair.Vendor && resourceType.Model == pair.Model
			})
			Expect(typeIdx).ToNot(Equal(-1), "Missing ResourceType for vendor=%s model=%s", pair.Vendor, pair.Model)
			Expect(apiResourceTypes[typeIdx].ResourceTypeId).ToNot(Equal(uuid.Nil),
				"ResourceType vendor=%s model=%s should have resourceTypeId populated", pair.Vendor, pair.Model)
			Expect(apiResourceTypes[typeIdx].Name).ToNot(BeEmpty(),
				"ResourceType vendor=%s model=%s should have name populated", pair.Vendor, pair.Model)
			Expect(string(apiResourceTypes[typeIdx].ResourceKind)).To(Equal("PHYSICAL"),
				"ResourceType vendor=%s model=%s should have resourceKind PHYSICAL", pair.Vendor, pair.Model)
			Expect(string(apiResourceTypes[typeIdx].ResourceClass)).To(Equal("COMPUTE"),
				"ResourceType vendor=%s model=%s should have resourceClass COMPUTE", pair.Vendor, pair.Model)
		}

		By("retrieving a Resource Type by resourceTypeId")

		chosen := apiResourceTypes[0]
		retrieved, err := inventoryClient.GetResourceType(chosen.ResourceTypeId)
		Expect(err).ToNot(HaveOccurred(), "Failed to get Resource Type %s", chosen.ResourceTypeId)
		Expect(retrieved.ResourceTypeId).To(Equal(chosen.ResourceTypeId),
			"Retrieved Resource Type ID should match listed type")
		Expect(retrieved.Vendor).To(Equal(chosen.Vendor),
			"Retrieved Resource Type vendor should match listed type")
		Expect(retrieved.Model).To(Equal(chosen.Model),
			"Retrieved Resource Type model should match listed type")
		Expect(retrieved.Version).To(Equal(chosen.Version),
			"Retrieved Resource Type version should match listed type")
		Expect(retrieved.AlarmDictionaryId).ToNot(BeNil(), "alarmDictionaryId should be populated")
	})

	// 89897 - Retrieve Resource Type Alarm Dictionary
	It("retrieves Resource Type Alarm Dictionary", reportxml.ID("89897"), func() {
		By("listing Resource Types and selecting one with an alarm dictionary")

		apiResourceTypes, err := inventoryClient.ListResourceTypes()
		Expect(err).ToNot(HaveOccurred(), "Failed to list Resource Types")

		var chosen *oranapi.ResourceType

		for i := range apiResourceTypes {
			if apiResourceTypes[i].AlarmDictionaryId != nil {
				chosen = &apiResourceTypes[i]

				break
			}
		}

		Expect(chosen).ToNot(BeNil(), "At least one Resource Type with alarmDictionaryId is required")

		By("retrieving the Resource Type alarm dictionary")

		alarmDictionary, err := inventoryClient.GetResourceTypeAlarmDictionary(chosen.ResourceTypeId)
		Expect(err).ToNot(HaveOccurred(), "Failed to get Resource Type alarm dictionary")
		Expect(alarmDictionary.AlarmDictionaryId).To(Equal(*chosen.AlarmDictionaryId),
			"Retrieved alarm dictionary ID should match Resource Type alarmDictionaryId")
		Expect(alarmDictionary.AlarmDictionarySchemaVersion).ToNot(BeEmpty(),
			"alarmDictionarySchemaVersion should be populated")
		Expect(alarmDictionary.AlarmDefinition).ToNot(BeNil(),
			"alarmDefinition should be present")
	})

	// 89898 - List and retrieve Deployment Managers
	It("lists and retrieves Deployment Managers", reportxml.ID("89898"), func() {
		By("listing eligible ManagedClusters on the hub")

		eligibleClusters, err := ocm.ListDeploymentManagerEligibleManagedClusters(HubAPIClient)
		Expect(err).ToNot(HaveOccurred(), "Failed to list Deployment Manager eligible ManagedClusters")
		Expect(eligibleClusters).ToNot(BeEmpty(), "At least one eligible ManagedCluster is required")

		By("listing Deployment Managers from the inventory API")

		apiManagers, err := inventoryClient.ListDeploymentManagers()
		Expect(err).ToNot(HaveOccurred(), "Failed to list Deployment Managers")
		Expect(apiManagers).To(HaveLen(len(eligibleClusters)),
			"Number of API Deployment Managers should match eligible ManagedClusters")

		By("verifying each eligible ManagedCluster matches an API Deployment Manager")

		for _, cluster := range eligibleClusters {
			managerIdx := slices.IndexFunc(apiManagers, func(manager oranapi.DeploymentManager) bool {
				return manager.Name == cluster.Definition.Name
			})
			Expect(managerIdx).ToNot(Equal(-1), "ManagedCluster %s missing from API response", cluster.Definition.Name)
			verifyErr := inventory.VerifyDeploymentManagerMatchesCluster(apiManagers[managerIdx], cluster)
			Expect(verifyErr).ToNot(HaveOccurred(),
				"ManagedCluster %s does not match API Deployment Manager", cluster.Definition.Name)
		}

		By("retrieving a Deployment Manager by deploymentManagerId")

		chosen := apiManagers[0]
		retrieved, err := inventoryClient.GetDeploymentManager(chosen.DeploymentManagerId)
		Expect(err).ToNot(HaveOccurred(), "Failed to get Deployment Manager %s", chosen.DeploymentManagerId)
		Expect(retrieved.DeploymentManagerId).To(Equal(chosen.DeploymentManagerId),
			"Retrieved Deployment Manager ID should match listed manager")
		Expect(retrieved.Name).To(Equal(chosen.Name),
			"Retrieved Deployment Manager name should match listed manager")
		Expect(retrieved.Extensions).ToNot(BeNil(), "extensions should be populated")
		Expect(retrieved.Capabilities).ToNot(BeNil(), "capabilities should be present")
		Expect(retrieved.Capacity).ToNot(BeNil(), "capacity should be present")
	})

	// 89899 - List and retrieve Alarm Dictionaries
	It("lists and retrieves Alarm Dictionaries", reportxml.ID("89899"), func() {
		By("listing Alarm Dictionaries from the inventory API")

		alarmDictionaries, err := inventoryClient.ListInventoryAlarmDictionaries()
		Expect(err).ToNot(HaveOccurred(), "Failed to list Alarm Dictionaries")
		Expect(alarmDictionaries).ToNot(BeEmpty(), "At least one Alarm Dictionary is required")

		for _, alarmDictionary := range alarmDictionaries {
			Expect(alarmDictionary.AlarmDictionaryId).ToNot(Equal(uuid.Nil),
				"alarmDictionaryId should be populated")
			Expect(alarmDictionary.AlarmDictionarySchemaVersion).ToNot(BeEmpty(),
				"alarmDictionarySchemaVersion should be populated")
			Expect(alarmDictionary.AlarmDefinition).ToNot(BeNil(),
				"alarmDefinition should be present")
		}

		By("retrieving an Alarm Dictionary by ID")

		chosen := alarmDictionaries[0]
		retrieved, err := inventoryClient.GetInventoryAlarmDictionary(chosen.AlarmDictionaryId)
		Expect(err).ToNot(HaveOccurred(), "Failed to get Alarm Dictionary %s", chosen.AlarmDictionaryId)
		Expect(retrieved.AlarmDictionaryId).To(Equal(chosen.AlarmDictionaryId),
			"Retrieved Alarm Dictionary ID should match listed dictionary")
		Expect(retrieved.AlarmDefinition).ToNot(BeNil(),
			"alarmDefinition should be present")
	})

	// 89900 - Subscription lifecycle (create, list, get, delete)
	It("handles inventory subscription lifecycle", reportxml.ID("89900"), func() {
		By("creating an inventory subscription")

		consumerSubscriptionID := uuid.New()
		callbackURL := mocksmo.ObserverCallbackURL(mockSMOBaseURL, consumerSubscriptionID.String())
		created, err := inventoryClient.CreateInventorySubscription(oranapi.InventorySubscription{
			ConsumerSubscriptionId: &consumerSubscriptionID,
			Filter:                 new(""),
			Callback:               callbackURL,
		})
		Expect(err).ToNot(HaveOccurred(), "Failed to create inventory subscription")
		Expect(created.SubscriptionId).ToNot(BeNil(), "subscriptionId should be assigned")

		subscriptionID := *created.SubscriptionId

		By("listing inventory subscriptions")

		subscriptions, err := inventoryClient.ListInventorySubscriptions()
		Expect(err).ToNot(HaveOccurred(), "Failed to list inventory subscriptions")

		found := slices.ContainsFunc(subscriptions, func(subscription oranapi.InventorySubscription) bool {
			return subscription.SubscriptionId != nil &&
				*subscription.SubscriptionId == subscriptionID &&
				subscription.ConsumerSubscriptionId != nil &&
				*subscription.ConsumerSubscriptionId == consumerSubscriptionID
		})
		Expect(found).To(BeTrue(), "Created subscription should appear in the list")

		By("retrieving the inventory subscription by ID")

		retrieved, err := inventoryClient.GetInventorySubscription(subscriptionID)
		Expect(err).ToNot(HaveOccurred(), "Failed to get inventory subscription")
		Expect(*retrieved.SubscriptionId).To(Equal(subscriptionID),
			"Retrieved subscription ID should match created subscription")
		Expect(*retrieved.ConsumerSubscriptionId).To(Equal(consumerSubscriptionID),
			"Retrieved consumer subscription ID should match created subscription")
		Expect(retrieved.Callback).To(Equal(callbackURL),
			"Retrieved callback should match created subscription")
		Expect(retrieved.Filter).ToNot(BeNil(), "filter should be present on retrieved subscription")
		Expect(*retrieved.Filter).To(BeEmpty(), "retrieved filter should match created subscription")

		By("deleting the inventory subscription")

		err = inventoryClient.DeleteInventorySubscription(subscriptionID)
		Expect(err).ToNot(HaveOccurred(), "Failed to delete inventory subscription")

		By("verifying the deleted subscription returns 404")

		_, err = inventoryClient.GetInventorySubscription(subscriptionID)
		apiErr := oranapi.AsAPIError(err)
		Expect(apiErr).ToNot(BeNil(), "Expected API error after deleting subscription")
		Expect(apiErr.Status).To(Equal(http.StatusNotFound),
			"Deleted subscription should return HTTP 404")
	})

	// 89901 - Receive inventory change notification from a subscription
	It("receives inventory change notifications from a subscription", reportxml.ID("89901"), func() {
		By("selecting a Ready OCloudSite for the temporary ResourcePool")

		readySites, err := oran.ListReadyOCloudSites(HubAPIClient, client.InNamespace(tsparams.O2IMSNamespace))
		Expect(err).ToNot(HaveOccurred(), "Failed to list Ready OCloudSite CRs")
		Expect(readySites).ToNot(BeEmpty(), "At least one Ready OCloudSite is required")
		parentSite := readySites[0]

		By("creating an inventory subscription")

		consumerSubscriptionID := uuid.New()
		created, err := inventoryClient.CreateInventorySubscription(oranapi.InventorySubscription{
			ConsumerSubscriptionId: &consumerSubscriptionID,
			Callback:               mocksmo.ObserverCallbackURL(mockSMOBaseURL, consumerSubscriptionID.String()),
		})
		Expect(err).ToNot(HaveOccurred(), "Failed to create inventory subscription")
		Expect(created.SubscriptionId).ToNot(BeNil(), "subscriptionId should be assigned")

		subscriptionID := *created.SubscriptionId

		DeferCleanup(func() {
			err := inventoryClient.DeleteInventorySubscription(subscriptionID)
			Expect(err).ToNot(HaveOccurred(), "Failed to delete inventory subscription during cleanup")
		})

		By("creating a temporary ResourcePool and waiting until Ready")

		createTime := time.Now()
		poolBuilder := oran.NewResourcePoolBuilder(
			HubAPIClient, tsparams.TestInventoryResourcePool, tsparams.O2IMSNamespace).
			WithOCloudSiteName(parentSite.Definition.Name).
			WithDescription("temporary pool for inventory notification test")
		poolBuilder, err = poolBuilder.Create()
		Expect(err).ToNot(HaveOccurred(), "Failed to create temporary ResourcePool")

		poolBuilder, err = poolBuilder.WaitForCondition(tsparams.InventoryReadyCondition, 2*time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait for temporary ResourcePool to become Ready")

		DeferCleanup(func() {
			pool, err := oran.PullResourcePool(
				HubAPIClient, tsparams.TestInventoryResourcePool, tsparams.O2IMSNamespace)
			if err == nil {
				err = pool.Delete()
				Expect(err).ToNot(HaveOccurred(), "Failed to delete temporary ResourcePool during cleanup")
			}
		})

		poolID := string(poolBuilder.Definition.UID)

		By("waiting for a CREATE inventory notification")

		err = mocksmo.WaitFor(HubAPIClient, RANConfig.MockSMONamespace,
			mocksmo.WithStart[oranapi.InventoryChangeNotification](createTime),
			mocksmo.WithObserverID[oranapi.InventoryChangeNotification](consumerSubscriptionID.String()),
			mocksmo.WithTimeout[oranapi.InventoryChangeNotification](2*time.Minute),
			mocksmo.WithMatch(func(notification *oranapi.InventoryChangeNotification) bool {
				if notification.NotificationEventType != oranapi.InventoryChangeNotificationEventTypeCreate {
					return false
				}

				if notification.ConsumerSubscriptionId == nil ||
					*notification.ConsumerSubscriptionId != consumerSubscriptionID {
					return false
				}

				return inventory.NotificationRefersToPool(
					notification, poolID, tsparams.TestInventoryResourcePool)
			}),
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to receive CREATE inventory notification")

		By("deleting the temporary ResourcePool")

		deleteTime := time.Now()
		err = poolBuilder.Delete()
		Expect(err).ToNot(HaveOccurred(), "Failed to delete temporary ResourcePool")

		By("waiting for a DELETE inventory notification")

		err = mocksmo.WaitFor(HubAPIClient, RANConfig.MockSMONamespace,
			mocksmo.WithStart[oranapi.InventoryChangeNotification](deleteTime),
			mocksmo.WithObserverID[oranapi.InventoryChangeNotification](consumerSubscriptionID.String()),
			mocksmo.WithTimeout[oranapi.InventoryChangeNotification](2*time.Minute),
			mocksmo.WithMatch(func(notification *oranapi.InventoryChangeNotification) bool {
				if notification.NotificationEventType != oranapi.InventoryChangeNotificationEventTypeDelete {
					return false
				}

				if notification.ConsumerSubscriptionId == nil ||
					*notification.ConsumerSubscriptionId != consumerSubscriptionID {
					return false
				}

				return inventory.NotificationRefersToPool(
					notification, poolID, tsparams.TestInventoryResourcePool)
			}),
		)
		Expect(err).ToNot(HaveOccurred(), "Failed to receive DELETE inventory notification")
	})

	// 89902 - Filter inventory resources
	It("filters inventory Locations", reportxml.ID("89902"), func() {
		By("creating two test Location CRs")

		alpha, err := inventory.CreateReadyTestLocation(
			HubAPIClient, tsparams.TestLocationAlpha, tsparams.O2IMSNamespace, "Alpha Site",
			tsparams.InventoryReadyCondition, 2*time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Failed to create test Location %s", tsparams.TestLocationAlpha)

		DeferCleanup(func() {
			err := alpha.Delete()
			Expect(err).ToNot(HaveOccurred(), "Failed to delete test Location %s during cleanup", tsparams.TestLocationAlpha)
		})

		beta, err := inventory.CreateReadyTestLocation(
			HubAPIClient, tsparams.TestLocationBeta, tsparams.O2IMSNamespace, "Beta Site",
			tsparams.InventoryReadyCondition, 2*time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Failed to create test Location %s", tsparams.TestLocationBeta)

		DeferCleanup(func() {
			err := beta.Delete()
			Expect(err).ToNot(HaveOccurred(), "Failed to delete test Location %s during cleanup", tsparams.TestLocationBeta)
		})

		By("waiting for both test Locations to appear in the inventory API")

		err = inventory.WaitForLocationInAPI(inventoryClient, tsparams.TestLocationAlpha, 2*time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait for Location %s in API", tsparams.TestLocationAlpha)

		err = inventory.WaitForLocationInAPI(inventoryClient, tsparams.TestLocationBeta, 2*time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait for Location %s in API", tsparams.TestLocationBeta)

		By("listing all Locations and verifying both test Locations are present")

		allLocations, err := inventoryClient.ListLocations()
		Expect(err).ToNot(HaveOccurred(), "Failed to list Locations")

		allContainsAlpha := slices.ContainsFunc(allLocations, func(location oranapi.LocationInfo) bool {
			return location.Name == tsparams.TestLocationAlpha
		})
		allContainsBeta := slices.ContainsFunc(allLocations, func(location oranapi.LocationInfo) bool {
			return location.Name == tsparams.TestLocationBeta
		})

		Expect(allContainsAlpha).To(BeTrue(), "Unfiltered list should include %s", tsparams.TestLocationAlpha)
		Expect(allContainsBeta).To(BeTrue(), "Unfiltered list should include %s", tsparams.TestLocationBeta)

		By("filtering for test-location-alpha")

		alphaFilter := filter.Equals("name", tsparams.TestLocationAlpha)
		alphaLocations, err := inventoryClient.ListLocations(oranapi.WithFilter(alphaFilter))
		Expect(err).ToNot(HaveOccurred(), "Failed to filter Locations by name equals alpha")
		Expect(alphaLocations).To(HaveLen(1),
			"Filter name=%s should return exactly one Location", tsparams.TestLocationAlpha)
		Expect(alphaLocations[0].Name).To(Equal(tsparams.TestLocationAlpha),
			"Filtered Location name should match filter value")

		By("filtering to exclude test-location-alpha")

		neqFilter := filter.DoesNotEqual("name", tsparams.TestLocationAlpha)
		neqLocations, err := inventoryClient.ListLocations(oranapi.WithFilter(neqFilter))
		Expect(err).ToNot(HaveOccurred(), "Failed to filter Locations by name not-equals alpha")

		containsAlpha := slices.ContainsFunc(neqLocations, func(location oranapi.LocationInfo) bool {
			return location.Name == tsparams.TestLocationAlpha
		})
		containsBeta := slices.ContainsFunc(neqLocations, func(location oranapi.LocationInfo) bool {
			return location.Name == tsparams.TestLocationBeta
		})

		Expect(containsAlpha).To(BeFalse(), "Filtered list should not include test-location-alpha")
		Expect(containsBeta).To(BeTrue(), "Filtered list should include test-location-beta")

		By("filtering for test-location-beta")

		betaFilter := filter.Equals("name", tsparams.TestLocationBeta)
		betaLocations, err := inventoryClient.ListLocations(oranapi.WithFilter(betaFilter))
		Expect(err).ToNot(HaveOccurred(), "Failed to filter Locations by name equals beta")
		Expect(betaLocations).To(HaveLen(1),
			"Filter name=%s should return exactly one Location", tsparams.TestLocationBeta)
		Expect(betaLocations[0].Name).To(Equal(tsparams.TestLocationBeta),
			"Filtered Location name should match filter value")
	})

	// 89903 - Field selection and exclusion
	It("supports field selection and exclusion", reportxml.ID("89903"), func() {
		By("listing Resource Pools with fields=name")

		poolsWithFields, err := inventoryClient.ListResourcePools(oranapi.WithFields(fields.Include("name")))
		Expect(err).ToNot(HaveOccurred(), "Failed to list Resource Pools with fields=name")
		Expect(poolsWithFields).ToNot(BeEmpty(), "At least one Resource Pool is required")

		for _, pool := range poolsWithFields {
			Expect(pool.ResourcePoolId).ToNot(Equal(uuid.Nil), "resourcePoolId is mandatory")
			Expect(pool.Name).ToNot(BeEmpty(), "name is mandatory")
			Expect(pool.Description).ToNot(BeEmpty(), "description is mandatory")
			Expect(pool.OCloudSiteId).ToNot(Equal(uuid.Nil), "oCloudSiteId is mandatory")
			Expect(pool.Extensions).To(BeNil(), "extensions should be absent when not selected")
		}

		By("listing Resource Pools with exclude_fields=extensions")

		poolsWithoutExtensions, err := inventoryClient.ListResourcePools(
			oranapi.WithFields(fields.Exclude("extensions")))
		Expect(err).ToNot(HaveOccurred(), "Failed to list Resource Pools excluding extensions")
		Expect(poolsWithoutExtensions).ToNot(BeEmpty(),
			"At least one Resource Pool is required when excluding extensions")

		for _, pool := range poolsWithoutExtensions {
			Expect(pool.ResourcePoolId).ToNot(Equal(uuid.Nil), "resourcePoolId is mandatory")
			Expect(pool.Name).ToNot(BeEmpty(), "name is mandatory")
			Expect(pool.Description).ToNot(BeEmpty(), "description is mandatory")
			Expect(pool.OCloudSiteId).ToNot(Equal(uuid.Nil), "oCloudSiteId is mandatory")
			Expect(pool.Extensions).To(BeNil(), "extensions should be absent when excluded")
		}

		By("listing Deployment Managers excluding complex attributes")

		managersWithoutComplex, err := inventoryClient.ListDeploymentManagers(
			oranapi.WithFields(fields.Exclude("extensions", "capacity", "capabilities")))
		Expect(err).ToNot(HaveOccurred(), "Failed to list Deployment Managers excluding complex fields")
		Expect(managersWithoutComplex).ToNot(BeEmpty(),
			"At least one Deployment Manager is required when excluding complex fields")

		for _, manager := range managersWithoutComplex {
			Expect(manager.DeploymentManagerId).ToNot(Equal(uuid.Nil),
				"deploymentManagerId is mandatory")
			Expect(manager.Name).ToNot(BeEmpty(), "name is mandatory")
			Expect(manager.Description).ToNot(BeEmpty(), "description is mandatory")
			Expect(manager.OCloudId).ToNot(Equal(uuid.Nil), "oCloudId is mandatory")
			Expect(manager.ServiceUri).ToNot(BeEmpty(), "serviceUri is mandatory")
			Expect(manager.Extensions).To(BeNil(), "extensions should be absent")
			Expect(manager.Capacity).To(Or(BeNil(), BeEmpty()), "capacity should be absent")
			Expect(manager.Capabilities).To(Or(BeNil(), BeEmpty()), "capabilities should be absent")
		}

		By("listing Deployment Managers with all_fields=true")

		managersWithAllFields, err := inventoryClient.ListDeploymentManagers(oranapi.WithFields(fields.All()))
		Expect(err).ToNot(HaveOccurred(), "Failed to list Deployment Managers with all_fields")
		Expect(managersWithAllFields).ToNot(BeEmpty(),
			"At least one Deployment Manager is required with all_fields")

		for _, manager := range managersWithAllFields {
			Expect(manager.Extensions).ToNot(BeNil(), "extensions should be present with all_fields")
			Expect(manager.Capacity).ToNot(BeNil(), "capacity should be present with all_fields")
			Expect(manager.Capabilities).ToNot(BeNil(), "capabilities should be present with all_fields")
		}
	})

	// 89904 - Retrieve a non-existent resource by ID
	It("returns 404 for non-existent resources", reportxml.ID("89904"), func() {
		nonexistentID := uuid.MustParse(tsparams.NonExistentUUID)

		By("requesting a non-existent Resource Pool")

		_, err := inventoryClient.GetResourcePool(nonexistentID)
		apiErr := oranapi.AsAPIError(err)
		Expect(apiErr).ToNot(BeNil(), "Expected API error when requesting non-existent Resource Pool")
		Expect(apiErr.Status).To(Equal(http.StatusNotFound),
			"Expected 404 when requesting non-existent Resource Pool")

		By("requesting a non-existent Deployment Manager")

		_, err = inventoryClient.GetDeploymentManager(nonexistentID)
		apiErr = oranapi.AsAPIError(err)
		Expect(apiErr).ToNot(BeNil(), "Expected API error when requesting non-existent Deployment Manager")
		Expect(apiErr.Status).To(Equal(http.StatusNotFound),
			"Expected 404 when requesting non-existent Deployment Manager")

		By("requesting a non-existent Location")

		_, err = inventoryClient.GetLocation("nonexistent-location-id")
		apiErr = oranapi.AsAPIError(err)
		Expect(apiErr).ToNot(BeNil(), "Expected API error when requesting non-existent Location")
		Expect(apiErr.Status).To(Equal(http.StatusNotFound),
			"Expected 404 when requesting non-existent Location")

		By("listing Resources under a non-existent Resource Pool")

		_, err = inventoryClient.ListResources(nonexistentID)
		apiErr = oranapi.AsAPIError(err)
		Expect(apiErr).ToNot(BeNil(), "Expected API error when listing Resources under non-existent Resource Pool")
		Expect(apiErr.Status).To(Equal(http.StatusNotFound),
			"Expected 404 when listing Resources under non-existent Resource Pool")
	})

	// 89905 - Create subscription with invalid callback
	It("rejects subscriptions with invalid callbacks", reportxml.ID("89905"), func() {
		consumerSubscriptionID := uuid.New()

		By("attempting to create a subscription with an http callback")

		_, err := inventoryClient.CreateInventorySubscription(oranapi.InventorySubscription{
			ConsumerSubscriptionId: &consumerSubscriptionID,
			Callback: "http://" + RANConfig.GetAppsURL(RANConfig.MockSMOSubdomain) +
				"/mock_smo/v1/observers/" + consumerSubscriptionID.String(),
		})
		apiErr := oranapi.AsAPIError(err)
		Expect(apiErr).ToNot(BeNil(), "Expected API error when creating subscription with http callback")
		Expect(apiErr.Status).To(Equal(http.StatusBadRequest),
			"Expected 400 when creating subscription with http callback")

		By("attempting to create a subscription with a mismatched SMO hostname")

		_, err = inventoryClient.CreateInventorySubscription(oranapi.InventorySubscription{
			ConsumerSubscriptionId: new(uuid.New()),
			Callback:               "https://invalid-smo.example.com/mock_smo/v1/observers/" + uuid.NewString(),
		})
		apiErr = oranapi.AsAPIError(err)
		Expect(apiErr).ToNot(BeNil(), "Expected API error when creating subscription with mismatched SMO hostname")
		Expect(apiErr.Status).To(Equal(http.StatusBadRequest),
			"Expected 400 when creating subscription with mismatched SMO hostname")

		By("attempting to create a subscription without a callback")

		_, err = inventoryClient.CreateInventorySubscription(oranapi.InventorySubscription{
			ConsumerSubscriptionId: new(uuid.New()),
		})
		apiErr = oranapi.AsAPIError(err)
		Expect(apiErr).ToNot(BeNil(), "Expected API error when creating subscription without callback")
		Expect(apiErr.Status).To(Equal(http.StatusBadRequest),
			"Expected 400 when creating subscription without callback")
	})

	// 89906 - Request with invalid filter syntax
	It("rejects invalid filter syntax", reportxml.ID("89906"), func() {
		By("requesting Resource Pools with invalid filter syntax")

		_, err := inventoryClient.ListResourcePools(oranapi.WithFilter(filter.Raw("invalid-syntax")))
		apiErr := oranapi.AsAPIError(err)
		Expect(apiErr).ToNot(BeNil(), "Expected API error when using invalid filter syntax")
		Expect(apiErr.Status).To(Equal(http.StatusBadRequest),
			"Expected 400 when using invalid filter syntax")

		By("requesting Resource Pools with an invalid filter field")

		_, err = inventoryClient.ListResourcePools(
			oranapi.WithFilter(filter.Raw("(eq,nonexistentField,value)")))
		apiErr = oranapi.AsAPIError(err)
		Expect(apiErr).ToNot(BeNil(), "Expected API error when filtering on invalid field")
		Expect(apiErr.Status).To(Equal(http.StatusBadRequest),
			"Expected 400 when filtering on invalid field")
	})

	// 89907 - Unauthenticated API request
	It("rejects unauthenticated API requests", reportxml.ID("89907"), func() {
		By("creating an unauthenticated inventory API client")

		clientBuilder, err := auth.NewUnauthenticatedClientBuilderForConfig(RANConfig)
		Expect(err).ToNot(HaveOccurred(), "Failed to create unauthenticated O2IMS client builder")

		unauthenticatedClient, err := clientBuilder.BuildInventory()
		Expect(err).ToNot(HaveOccurred(), "Failed to create unauthenticated inventory client")

		By("requesting Resource Pools without an Authorization header")

		_, err = unauthenticatedClient.ListResourcePools()
		apiErr := oranapi.AsAPIError(err)
		Expect(apiErr).ToNot(BeNil(), "Expected API error for unauthenticated request")
		Expect(apiErr.Status).To(Equal(http.StatusUnauthorized),
			"Unauthenticated request should return HTTP 401")
	})
})
