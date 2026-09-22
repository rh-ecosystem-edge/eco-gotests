package tests

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	provisioningv1alpha1 "github.com/openshift-kni/oran-o2ims/api/provisioning/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran"
	oranapi "github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/auth"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/helper"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("ORAN Security Tests", Label(tsparams.LabelPreProvision, tsparams.LabelSecurity), func() {
	var o2imsAPIClient runtimeclient.Client

	BeforeEach(func() {
		var err error

		By("creating the O2IMS API client")

		clientBuilder, err := auth.NewClientBuilderForConfig(RANConfig)
		Expect(err).ToNot(HaveOccurred(), "Failed to create the O2IMS API client builder")

		o2imsAPIClient, err = clientBuilder.BuildProvisioning()
		Expect(err).ToNot(HaveOccurred(), "Failed to create the O2IMS API client")
	})

	// 89915 - DeploymentManager RBAC gating
	It("denies o2ims-reader access to deployment managers", reportxml.ID("89915"), func() {
		if RANConfig.O2IMSOAuthClientID == "" || RANConfig.O2IMSOAuthClientSecret == "" {
			Skip("OAuth client credentials are required for o2ims-reader RBAC test")
		}

		By("creating an O2IMS inventory API client with o2ims-reader scopes")

		clientBuilder, err := auth.NewClientBuilderForConfigWithScopes(RANConfig, auth.ReaderOAuthScopes)
		Expect(err).ToNot(HaveOccurred(), "Failed to create o2ims-reader O2IMS client builder")

		inventoryClient, err := clientBuilder.BuildInventory()
		Expect(err).ToNot(HaveOccurred(), "Failed to create o2ims-reader inventory API client")

		By("listing deployment managers as o2ims-reader")

		_, err = inventoryClient.ListDeploymentManagers()
		apiErr := oranapi.AsAPIError(err)
		Expect(apiErr).ToNot(BeNil(), "Expected API error when o2ims-reader lists deployment managers")
		Expect(apiErr.Status).To(Equal(http.StatusForbidden),
			"o2ims-reader should receive 403 Forbidden for deployment managers endpoint")
	})

	// 89916 - ProvisioningRequest clusterName validation rejects reserved namespaces
	It("rejects ProvisioningRequest with reserved or invalid clusterName", reportxml.ID("89916"), func() {
		// We use a for loop instead of a DescribeTable so that this entire test shares a single ID in the JUnit
		// report. An invariant of cnf/ran is that test cases are bijective with IDs, and DescribeTable would
		// violate that.
		testCases := []struct {
			clusterName     string
			detailSubstring string
		}{
			{"default", tsparams.PRReservedNamespaceDetailsSubstring},
			{"openshift-test", tsparams.PROpenshiftPrefixDetailsSubstring},
			{"kube-system", tsparams.PRKubePrefixDetailsSubstring},
			{"INVALID_NAME", tsparams.PRDNS1123DetailsSubstring},
		}

		for _, testCase := range testCases {
			By("creating a ProvisioningRequest with clusterName " + testCase.clusterName)

			prName := uuid.New().String()

			namedBuilder, err := helper.NewProvisioningRequestNamed(o2imsAPIClient, prName, tsparams.TemplateValid)
			Expect(err).ToNot(HaveOccurred(), "Failed to build ProvisioningRequest for clusterName %q", testCase.clusterName)

			prBuilder, err := helper.WithClusterInstanceClusterName(namedBuilder, testCase.clusterName)
			Expect(err).ToNot(HaveOccurred(), "Failed to set clusterName %q", testCase.clusterName)

			_, err = prBuilder.Create()
			if err == nil {
				By("deleting unexpectedly created ProvisioningRequest")

				deleteErr := prBuilder.DeleteAndWait(10 * time.Minute)
				Expect(deleteErr).ToNot(HaveOccurred(), "Failed to delete unexpectedly created ProvisioningRequest")
			}

			Expect(err).To(HaveOccurred(),
				"Creating a ProvisioningRequest with clusterName %q should fail", testCase.clusterName)

			apiErr := oranapi.AsAPIError(err)
			Expect(apiErr).ToNot(BeNil(),
				"Expected API error for clusterName %q", testCase.clusterName)
			Expect(apiErr.Status).To(Equal(http.StatusBadRequest),
				"Expected 400 for clusterName %q", testCase.clusterName)
			Expect(apiErr.Detail).To(ContainSubstring(testCase.detailSubstring),
				"Expected validation detail for clusterName %q", testCase.clusterName)
		}
	})

	When("a namespace already exists", func() {
		AfterEach(func() {
			By("deleting the security test ProvisioningRequest if it exists")

			prBuilder, err := oran.PullPR(o2imsAPIClient, tsparams.TestPRNameSecurity)
			if err == nil {
				err := prBuilder.DeleteAndWait(10 * time.Minute)
				Expect(err).ToNot(HaveOccurred(), "Failed to delete the security test ProvisioningRequest")
			}

			By("deleting the test namespace if it exists")

			nsBuilder := namespace.NewBuilder(HubAPIClient, tsparams.TestExistingNamespace)
			if nsBuilder.Exists() {
				err := nsBuilder.DeleteAndWait(10 * time.Minute)
				Expect(err).ToNot(HaveOccurred(), "Failed to delete the test namespace")
			}
		})

		// 89917 - ProvisioningRequest clusterName validation prevents namespace takeover
		It("fails when clusterName matches an existing unowned namespace", reportxml.ID("89917"), func() {
			By("creating the test namespace")

			_, err := namespace.NewBuilder(HubAPIClient, tsparams.TestExistingNamespace).Create()
			Expect(err).ToNot(HaveOccurred(), "Failed to create the test namespace")

			By("creating a ProvisioningRequest with clusterName matching the existing namespace")

			namedBuilder, err := helper.NewProvisioningRequestNamed(
				o2imsAPIClient, tsparams.TestPRNameSecurity, tsparams.TemplateValid)
			Expect(err).ToNot(HaveOccurred(), "Failed to build ProvisioningRequest for clusterName %q",
				tsparams.TestExistingNamespace)

			prBuilder, err := helper.WithClusterInstanceClusterName(namedBuilder, tsparams.TestExistingNamespace)
			Expect(err).ToNot(HaveOccurred(), "Failed to set clusterName %q", tsparams.TestExistingNamespace)

			_, err = prBuilder.Create()
			Expect(err).ToNot(HaveOccurred(),
				"Creating a ProvisioningRequest with an existing clusterName should be accepted at POST time")

			By("waiting for ProvisioningRequest to fail during reconciliation")

			err = prBuilder.WaitForPhaseAfter(provisioningv1alpha1.StateFailed, time.Time{}, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for ProvisioningRequest to fail")

			By("verifying failure reason indicates namespace already exists")

			currentPR, err := oran.PullPR(o2imsAPIClient, tsparams.TestPRNameSecurity)
			Expect(err).ToNot(HaveOccurred(), "Failed to get ProvisioningRequest status")
			Expect(currentPR.Definition.Status.ProvisioningStatus.ProvisioningPhase).
				To(Equal(provisioningv1alpha1.StateFailed))
			Expect(currentPR.Definition.Status.ProvisioningStatus.ProvisioningDetails).
				To(ContainSubstring(tsparams.PRExistingNamespaceDetailsSubstring),
					"Expected provisioning details to report namespace already exists")
		})
	})
})
