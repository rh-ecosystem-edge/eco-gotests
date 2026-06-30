package tests

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	provisioningv1alpha1 "github.com/openshift-kni/oran-o2ims/api/provisioning/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ocm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/reportxml"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/secret"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/raninittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/auth"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/helper"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/oran/internal/tsparams"
	"k8s.io/klog/v2"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("ORAN Provision Tests", Label(tsparams.LabelProvision), Ordered, ContinueOnFailure, func() {
	var o2imsAPIClient runtimeclient.Client

	BeforeEach(func() {
		var err error

		By("creating the O2IMS API client")

		clientBuilder, err := auth.NewClientBuilderForConfig(RANConfig)
		Expect(err).ToNot(HaveOccurred(), "Failed to create the O2IMS API client builder")

		o2imsAPIClient, err = clientBuilder.BuildProvisioning()
		Expect(err).ToNot(HaveOccurred(), "Failed to create the O2IMS API client")
	})

	// 77393 - Apply a ProvisioningRequest with missing required input parameter
	It("recovers provisioning when invalid ProvisioningRequest is updated", reportxml.ID("77393"), func() {
		By("verifying the ProvisioningRequest does not already exist")

		_, err := oran.PullPR(o2imsAPIClient, tsparams.TestPRName)
		if err == nil {
			Skip("cannot run provisioning tests if the ProvisioningRequest already exists")
		}

		By("creating a ProvisioningRequest with invalid policyTemplateParameters")

		prBuilder := helper.NewProvisioningRequest(o2imsAPIClient, tsparams.TemplateValid).
			WithTemplateParameter(tsparams.PolicyTemplateParamsKey, map[string]any{
				// By using an integer when the schema specifies a string we can create an invalid
				// ProvisioningRequest without being stopped by the webhook.
				tsparams.TestName: 1,
			})

		prBuilder, err = prBuilder.Create()
		Expect(err).ToNot(HaveOccurred(), "Failed to create an invalid ProvisioningRequest")

		By("waiting for the ProvisioningRequest to be failed")

		err = prBuilder.WaitForPhaseAfter(provisioningv1alpha1.StateFailed, time.Time{}, time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait for the ProvisioningRequest to fail")

		Expect(prBuilder.Object.Status.ProvisioningStatus.ProvisioningPhase).
			To(Equal(provisioningv1alpha1.StateFailed), "Expected ProvisioningRequest to be failed after invalid parameters")
		Expect(prBuilder.Object.Status.ProvisioningStatus.ProvisioningDetails).
			To(ContainSubstring(tsparams.PRValidationFailedDetailsSubstring),
				"Expected provisioning details to report a validation failure")

		updateTime := time.Now()

		By("updating the ProvisioningRequest with valid policyTemplateParameters")

		prBuilder = prBuilder.WithTemplateParameter(tsparams.PolicyTemplateParamsKey, map[string]any{})
		prBuilder, err = prBuilder.Update()
		Expect(err).ToNot(HaveOccurred(), "Failed to update the ProvisioningRequest with valid policyTemplateParameters")

		By("waiting for ProvisioningRequest to start progressing")

		err = prBuilder.WaitForPhaseAfter(provisioningv1alpha1.StateProgressing, updateTime, time.Minute)
		Expect(err).ToNot(HaveOccurred(), "Failed to wait for ProvisioningRequest to start progressing after recovery")
	})

	When("provisioning with a valid ProvisioningRequest", func() {
		AfterEach(func() {
			if RANConfig.Spoke1Kubeconfig != "" {
				By("saving the spoke 1 admin kubeconfig")

				err := saveSpoke1Secret("-admin-kubeconfig", "kubeconfig", RANConfig.Spoke1Kubeconfig)
				Expect(err).ToNot(HaveOccurred(), "Failed to save spoke 1 admin kubeconfig")
			}

			if RANConfig.Spoke1Password != "" {
				By("saving the spoke 1 admin password")

				err := saveSpoke1Secret("-admin-password", "password", RANConfig.Spoke1Password)
				Expect(err).ToNot(HaveOccurred(), "Failed to save the spoke 1 admin password")
			}
		})

		// 77394 - Apply a valid ProvisioningRequest
		It("successfully provisions and generates the correct resources", reportxml.ID("77394"), func() {
			By("pulling the ProvisioningRequest")

			prBuilder, err := oran.PullPR(o2imsAPIClient, tsparams.TestPRName)
			if err != nil {
				By("creating the ProvisioningRequest since it does not exist")

				prBuilder, err = helper.NewProvisioningRequest(o2imsAPIClient, tsparams.TemplateValid).Create()
				Expect(err).ToNot(HaveOccurred(), "Failed to create ProvisioningRequest since it does not exist")
			}

			By("waiting for the ProvisioningRequest to be fulfilled")
			// Since we know the ProvisioningRequest did not already start as fulfilled, we do not need to
			// use WaitForPhaseAfter.
			_, err = prBuilder.WaitUntilFulfilled(2 * time.Hour)
			Expect(err).ToNot(HaveOccurred(), "Failed to wait for the ProvisioningRequest to be fulfilled")

			By("verifying the fulfilled ProvisioningRequest status via the O2IMS API")

			err = verifyProvisioningRequestFulfilled(prBuilder)
			Expect(err).ToNot(HaveOccurred(), "Failed to verify fulfilled ProvisioningRequest status")

			By("verifying provisioning succeeded")

			err = verifySpokeProvisioning()
			Expect(err).ToNot(HaveOccurred(), "Failed to verify spoke provisioning succeeded")
		})
	})
})

// verifyProvisioningRequestFulfilled checks broad ProvisioningRequest status expectations after successful
// provisioning, using fields exposed by the O2IMS API client.
func verifyProvisioningRequestFulfilled(prBuilder *oran.ProvisioningRequestBuilder) error {
	var (
		accumulatedErrors []error
		err               error
	)

	By("refreshing the ProvisioningRequest from the O2IMS API")

	prBuilder.Object, err = prBuilder.Get()
	if err != nil {
		return fmt.Errorf("failed to refresh ProvisioningRequest from O2IMS API: %w", err)
	}

	status := prBuilder.Object.Status

	By("verifying the ProvisioningRequest is in the fulfilled phase")

	if status.ProvisioningStatus.ProvisioningPhase != provisioningv1alpha1.StateFulfilled {
		accumulatedErrors = append(accumulatedErrors, fmt.Errorf("expected provisioning phase %q, got %q",
			provisioningv1alpha1.StateFulfilled, status.ProvisioningStatus.ProvisioningPhase))
	}

	if !strings.Contains(status.ProvisioningStatus.ProvisioningDetails, tsparams.PRFulfilledDetailsSubstring) {
		accumulatedErrors = append(accumulatedErrors, fmt.Errorf(
			"expected provisioning details to contain %q, got %q",
			tsparams.PRFulfilledDetailsSubstring, status.ProvisioningStatus.ProvisioningDetails))
	}

	By("verifying the ProvisionedResources are set")

	if status.ProvisioningStatus.ProvisionedResources == nil ||
		status.ProvisioningStatus.ProvisionedResources.OCloudNodeClusterId == "" {
		accumulatedErrors = append(accumulatedErrors,
			fmt.Errorf("expected provisioned node cluster ID after fulfillment"))
	}

	By("verifying the ClusterDetails are set")

	if status.Extensions.ClusterDetails == nil ||
		status.Extensions.ClusterDetails.Name != RANConfig.Spoke1Name {
		accumulatedErrors = append(accumulatedErrors, fmt.Errorf(
			"expected cluster name %q in extensions, got %#v",
			RANConfig.Spoke1Name, status.Extensions.ClusterDetails))
	}

	By("verifying the InfrastructureResourceStatuses are set")

	resourceStatuses := status.Extensions.InfrastructureResourceStatuses
	if len(resourceStatuses) == 0 {
		accumulatedErrors = append(accumulatedErrors,
			fmt.Errorf("expected provisioned infrastructure nodes in extensions"))
	}

	By("verifying the InfrastructureResourceStatuses are provisioned")

	for _, resourceStatus := range resourceStatuses {
		if resourceStatus.ResourceProvisioningPhase != provisioningv1alpha1.ResourceProvisioningPhaseProvisioned {
			accumulatedErrors = append(accumulatedErrors, fmt.Errorf(
				"expected infrastructure resource %q to be provisioned, got %q",
				resourceStatus.ResourceId, resourceStatus.ResourceProvisioningPhase))

			continue
		}

		_, err := oran.PullAllocatedNode(HubAPIClient, resourceStatus.ResourceId, tsparams.O2IMSNamespace)
		if err != nil {
			klog.V(tsparams.LogLevel).Infof("Failed to verify AllocatedNode %s exists: %v",
				resourceStatus.ResourceId, err)

			accumulatedErrors = append(accumulatedErrors, fmt.Errorf(
				"failed to verify AllocatedNode %s exists: %w", resourceStatus.ResourceId, err))
		}
	}

	return errors.Join(accumulatedErrors...)
}

// saveSpoke1Secret will write the value of key in secret RANConfig.Spoke1Name+suffix to fileName, truncating if the
// file exists, otherwise saving with permissions 644.
func saveSpoke1Secret(suffix, key, fileName string) error {
	spokeSecret, err := secret.Pull(HubAPIClient, RANConfig.Spoke1Name+suffix, RANConfig.Spoke1Name)
	if err != nil {
		return err
	}

	value, exists := spokeSecret.Object.Data[key]
	if !exists {
		return fmt.Errorf("unable to save key %s in secret %s: key does not exist", key, RANConfig.Spoke1Name+suffix)
	}

	return os.WriteFile(fileName, value, 0644)
}

// verifySpokeProvisioning ensures that for a provisioned spoke, its NodeAllocationRequest exists, its BMC details are
// correct, the pull-secret and extra-manifests exist, and finally that its policies are compliant, in that order.
// Errors are accumulated for each validation and returned so that every one of the validations will run and be logged.
func verifySpokeProvisioning() error {
	var accumulatedErrors []error

	By("verifying spoke 1 pull-secret was created")

	_, err := secret.Pull(HubAPIClient, "pull-secret", RANConfig.Spoke1Name)
	if err != nil {
		klog.V(tsparams.LogLevel).Infof("Failed to verify the pull-secret was created: %v", err)

		accumulatedErrors = append(accumulatedErrors, fmt.Errorf("failed to verify the pull-secret was created: %w", err))
	}

	By("verifying spoke 1 extra-manifests was created")

	_, err = configmap.Pull(HubAPIClient, tsparams.ExtraManifestsName, RANConfig.Spoke1Name)
	if err != nil {
		klog.V(tsparams.LogLevel).Infof("Failed to verify the extra-manifests ConfigMap was created: %v", err)

		accumulatedErrors = append(accumulatedErrors,
			fmt.Errorf("failed to verify the extra-manifests ConfigMap was created: %w", err))
	}

	By("verifying spoke 1 policy ConfigMap was created")

	ztpNamespace := fmt.Sprintf("ztp-%s-%s", tsparams.ClusterTemplateName, RANConfig.ClusterTemplateAffix)

	_, err = configmap.Pull(HubAPIClient, RANConfig.Spoke1Name+"-pg", ztpNamespace)
	if err != nil {
		klog.V(tsparams.LogLevel).Infof("Failed to verify spoke 1 policy ConfigMap was created: %v", err)

		accumulatedErrors = append(accumulatedErrors,
			fmt.Errorf("failed to verify spoke 1 policy ConfigMap was created: %w", err))
	}

	By("verifying all the policies are compliant")

	err = ocm.WaitForAllPoliciesComplianceState(
		HubAPIClient, policiesv1.Compliant, time.Minute, runtimeclient.ListOptions{Namespace: RANConfig.Spoke1Name})
	if err != nil {
		klog.V(tsparams.LogLevel).Infof("Failed to verify all policies are compliant: %v", err)

		accumulatedErrors = append(accumulatedErrors, fmt.Errorf("failed to verify all policies are compliant: %w", err))
	}

	return errors.Join(accumulatedErrors...)
}
