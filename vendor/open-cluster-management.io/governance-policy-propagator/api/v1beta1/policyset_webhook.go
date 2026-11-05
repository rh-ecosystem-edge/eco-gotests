// Copyright Contributors to the Open Cluster Management project

package v1beta1

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var (
	errInvalidExclusionPolicy   = errors.New("policy in spec.exclusions is not listed in spec.policies")
	errDuplicateExclusionPolicy = errors.New("policy in spec.exclusions has a duplicate entry")
)

func (r *PolicySet) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &PolicySet{}).
		WithValidator(&PolicySetCustomValidator{}).
		WithLogConstructor(func(base logr.Logger, req *admission.Request) logr.Logger {
			log := base.WithName("policyset-validating-webhook")

			if req != nil {
				log = log.WithValues("kind", req.Kind, "namespace", req.Namespace, "name", req.Name)
			}

			return log
		}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-policy-open-cluster-management-io-v1beta1-policyset,mutating=false,failurePolicy=Ignore,sideEffects=None,groups=policy.open-cluster-management.io,resources=policysets,verbs=create;update,versions=v1beta1,name=policyset.open-cluster-management.io.webhook,admissionReviewVersions=v1,serviceName=propagator-webhook-service,serviceNamespace=open-cluster-management
// +kubebuilder:object:generate=false

// PolicySetCustomValidator validates PolicySet create and update requests.
type PolicySetCustomValidator struct{}

var _ admission.Validator[*PolicySet] = &PolicySetCustomValidator{}

// ValidateCreate implements admission.Validator so a webhook will be registered for the type
func (r *PolicySetCustomValidator) ValidateCreate(
	ctx context.Context, policySet *PolicySet,
) (admission.Warnings, error) {
	log := log.FromContext(ctx)
	log.V(1).Info("Validate policy set creation request")

	return nil, policySet.validateExclusions(ctx)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type
func (r *PolicySetCustomValidator) ValidateUpdate(
	ctx context.Context, _, policySet *PolicySet,
) (admission.Warnings, error) {
	log := log.FromContext(ctx)
	log.V(1).Info("Validate policy set update request")

	return nil, policySet.validateExclusions(ctx)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type
func (r *PolicySetCustomValidator) ValidateDelete(_ context.Context, _ *PolicySet) (admission.Warnings, error) {
	return nil, nil
}

func (r *PolicySet) validateExclusions(ctx context.Context) error {
	log := log.FromContext(ctx)
	log.V(1).Info("Validating policy set exclusions through a validating webhook")

	if len(r.Spec.Exclusions) == 0 {
		return nil
	}

	policiesInSet := make(map[string]struct{}, len(r.Spec.Policies))
	for _, policyName := range r.Spec.Policies {
		policiesInSet[string(policyName)] = struct{}{}
	}

	exclusionSeen := make(map[string]struct{})
	duplicatePolicyNames := make([]string, 0)
	duplicateSeen := make(map[string]struct{})
	invalidPolicyNames := make([]string, 0)
	invalidSeen := make(map[string]struct{})

	for _, exclusion := range r.Spec.Exclusions {
		policyName := string(exclusion.PolicyName)

		if _, ok := exclusionSeen[policyName]; ok {
			if _, ok := duplicateSeen[policyName]; !ok {
				duplicateSeen[policyName] = struct{}{}
				duplicatePolicyNames = append(duplicatePolicyNames, policyName)
			}
		} else {
			exclusionSeen[policyName] = struct{}{}
		}

		if _, ok := policiesInSet[policyName]; ok {
			continue
		}

		if _, ok := invalidSeen[policyName]; ok {
			continue
		}

		invalidSeen[policyName] = struct{}{}
		invalidPolicyNames = append(invalidPolicyNames, policyName)
	}

	var errs []error

	if len(duplicatePolicyNames) > 0 {
		errs = append(errs, fmt.Errorf("%w: %s", errDuplicateExclusionPolicy, strings.Join(duplicatePolicyNames, ", ")))
	}

	if len(invalidPolicyNames) > 0 {
		errs = append(errs, fmt.Errorf("%w: %s", errInvalidExclusionPolicy, strings.Join(invalidPolicyNames, ", ")))
	}

	if len(errs) > 0 {
		err := errors.Join(errs...)

		log.Info(
			"Invalid policy set exclusions",
			"duplicatePolicyCount", len(duplicatePolicyNames),
			"invalidPolicyCount", len(invalidPolicyNames),
		)

		return err
	}

	return nil
}
