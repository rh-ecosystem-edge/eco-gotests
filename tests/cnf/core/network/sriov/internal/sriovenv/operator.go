package sriovenv

import (
	"context"
	"fmt"
	"time"

	admregv1 "k8s.io/api/admissionregistration/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/olm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/webhook"
	. "github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/internal/netinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/core/network/sriov/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/internal/sriovoperator"
)

// InstalledSriovOperator holds OLM resources required to reinstall the SR-IOV operator.
type InstalledSriovOperator struct {
	Namespace      *namespace.Builder
	OperatorGroup  *olm.OperatorGroupBuilder
	Subscription   *olm.SubscriptionBuilder
	OperatorConfig *sriov.OperatorConfigBuilder
}

// CollectInstalledSriovOperatorInfo pulls the SR-IOV operator namespace, OperatorGroup, Subscription,
// and SriovOperatorConfig for use during reinstall.
func CollectInstalledSriovOperatorInfo() (*InstalledSriovOperator, error) {
	sriovNs, err := namespace.Pull(APIClient, NetConfig.SriovOperatorNamespace)
	if err != nil {
		return nil, fmt.Errorf("failed to pull SR-IOV operator namespace: %w", err)
	}

	sriovOg, err := olm.PullOperatorGroup(APIClient, tsparams.OperatorGroupName, NetConfig.SriovOperatorNamespace)
	if err != nil {
		return nil, fmt.Errorf("failed to pull SR-IOV OperatorGroup: %w", err)
	}

	sriovSub, err := olm.PullSubscription(APIClient, tsparams.OperatorSubscriptionName, NetConfig.SriovOperatorNamespace)
	if err != nil {
		return nil, fmt.Errorf("failed to pull %s: %w", tsparams.OperatorSubscriptionName, err)
	}

	sriovCfg, err := sriov.PullOperatorConfig(APIClient, NetConfig.SriovOperatorNamespace)
	if err != nil {
		return nil, fmt.Errorf("failed to pull SriovOperatorConfig: %w", err)
	}

	return &InstalledSriovOperator{
		Namespace:      sriovNs,
		OperatorGroup:  sriovOg,
		Subscription:   sriovSub,
		OperatorConfig: sriovCfg,
	}, nil
}

// RemoveSriovOperator removes SR-IOV configuration, operator config, webhooks, and the operator namespace.
func RemoveSriovOperator(sriovNamespace *namespace.Builder) error {
	if err := sriovoperator.RemoveSriovConfigurationAndWaitForSriovAndMCPStable(
		APIClient,
		NetConfig.WorkerLabelEnvVar,
		NetConfig.SriovOperatorNamespace,
		tsparams.MCOWaitTimeout,
		tsparams.DefaultTimeout); err != nil {
		return fmt.Errorf("failed to remove SR-IOV configuration: %w", err)
	}

	sriovOperatorConfig, err := sriov.PullOperatorConfig(APIClient, NetConfig.SriovOperatorNamespace)
	if err != nil {
		return fmt.Errorf("failed to pull SriovOperatorConfig: %w", err)
	}

	if _, err = sriovOperatorConfig.Delete(); err != nil {
		return fmt.Errorf("failed to remove default SR-IOV operator config: %w", err)
	}

	if err := waitForSriovMutatingWebhooksRemoved(
		tsparams.OperatorWebhookWaitTimeout, tsparams.RetryInterval); err != nil {
		return err
	}

	if err := waitForSriovValidatingWebhookRemoved(
		tsparams.OperatorWebhookWaitTimeout, tsparams.RetryInterval); err != nil {
		return err
	}

	if err := sriovNamespace.DeleteAndWait(tsparams.DefaultTimeout); err != nil {
		return fmt.Errorf("failed to delete SR-IOV namespace %s: %w", NetConfig.SriovOperatorNamespace, err)
	}

	return nil
}

// InstallSriovOperator deploys the SR-IOV operator namespace, OperatorGroup, Subscription, and config.
// Existing OLM resources are left in place so the call is safe after a partial reinstall.
// SriovOperatorConfig is recreated from the spec captured before removal when missing.
func InstallSriovOperator(installed *InstalledSriovOperator) error {
	ns := namespace.NewBuilder(APIClient, installed.Namespace.Definition.Name)
	if !ns.Exists() {
		if _, err := ns.Create(); err != nil {
			return fmt.Errorf("failed to create SR-IOV namespace %s: %w", installed.Namespace.Definition.Name, err)
		}
	}

	og := olm.NewOperatorGroupBuilder(
		APIClient,
		installed.OperatorGroup.Definition.Name,
		installed.OperatorGroup.Definition.Namespace)
	if !og.Exists() {
		if _, err := og.Create(); err != nil {
			return fmt.Errorf("failed to create SR-IOV OperatorGroup %s: %w", installed.OperatorGroup.Definition.Name, err)
		}
	}

	sub := olm.NewSubscriptionBuilder(
		APIClient,
		installed.Subscription.Definition.Name,
		installed.Subscription.Definition.Namespace,
		installed.Subscription.Definition.Spec.CatalogSource,
		installed.Subscription.Definition.Spec.CatalogSourceNamespace,
		installed.Subscription.Definition.Spec.Package)
	if channel := installed.Subscription.Definition.Spec.Channel; channel != "" {
		sub = sub.WithChannel(channel)
	}

	if !sub.Exists() {
		if _, err := sub.Create(); err != nil {
			return fmt.Errorf("failed to create SR-IOV Subscription %s: %w", installed.Subscription.Definition.Name, err)
		}
	}

	return restoreOperatorConfig(installed)
}

func restoreOperatorConfig(installed *InstalledSriovOperator) error {
	cfg := sriov.NewOperatorConfigBuilder(APIClient, installed.Namespace.Definition.Name)
	if cfg.Exists() {
		return nil
	}

	if installed.OperatorConfig != nil && installed.OperatorConfig.Definition != nil {
		cfg.Definition.Spec = *installed.OperatorConfig.Definition.Spec.DeepCopy()
	} else {
		cfg.WithOperatorWebhook(true).WithInjector(true)
	}

	if _, err := cfg.Create(); err != nil {
		return fmt.Errorf("failed to create SR-IOV operator config: %w", err)
	}

	return nil
}

// WaitForSriovOperatorDeployed polls until SR-IOV operator daemonsets are present and ready.
func WaitForSriovOperatorDeployed(timeout, interval time.Duration) error {
	err := wait.PollUntilContextTimeout(context.TODO(), interval, timeout, true, func(ctx context.Context) (bool, error) {
		deployErr := sriovoperator.IsSriovDeployed(APIClient, NetConfig.SriovOperatorNamespace)

		return deployErr == nil, nil
	})
	if err != nil {
		return fmt.Errorf("SR-IOV operator is not ready: %w", err)
	}

	return nil
}

// ValidateReinstalledSriovWebhookFailurePolicies checks mutating and validating webhook FailurePolicy values.
func ValidateReinstalledSriovWebhookFailurePolicies() error {
	resourceInjector, err := webhook.PullMutatingConfiguration(APIClient, tsparams.MutatingWebhookResourceInjectorConfig)
	if err != nil {
		return fmt.Errorf("failed to pull mutating webhook %s: %w", tsparams.MutatingWebhookResourceInjectorConfig, err)
	}

	if err := expectMutatingWebhookFailurePolicy(
		resourceInjector.Object.Webhooks, tsparams.MutatingWebhookResourceInjectorEntry, admregv1.Ignore); err != nil {
		return err
	}

	operatorMutating, err := webhook.PullMutatingConfiguration(APIClient, tsparams.MutatingWebhookSriovOperatorConfig)
	if err != nil {
		return fmt.Errorf("failed to pull mutating webhook %s: %w", tsparams.MutatingWebhookSriovOperatorConfig, err)
	}

	if err := expectMutatingWebhookFailurePolicy(
		operatorMutating.Object.Webhooks, tsparams.SriovOperatorWebhookEntry, admregv1.Fail); err != nil {
		return err
	}

	validating, err := webhook.PullValidatingConfiguration(APIClient, tsparams.ValidatingWebhookSriovOperatorConfig)
	if err != nil {
		return fmt.Errorf("failed to pull validating webhook %s: %w", tsparams.ValidatingWebhookSriovOperatorConfig, err)
	}

	return expectValidatingWebhookFailurePolicy(
		validating.Object.Webhooks, tsparams.SriovOperatorWebhookEntry, admregv1.Fail)
}

func waitForSriovMutatingWebhooksRemoved(timeout, interval time.Duration) error {
	for _, webhookName := range []string{
		tsparams.MutatingWebhookResourceInjectorConfig,
		tsparams.MutatingWebhookSriovOperatorConfig,
	} {
		name := webhookName

		err := wait.PollUntilContextTimeout(context.TODO(), interval, timeout, true, func(ctx context.Context) (bool, error) {
			return isMutatingWebhookConfigurationAbsent(name)
		})
		if err != nil {
			return fmt.Errorf("mutating webhook %s was not removed: %w", name, err)
		}
	}

	return nil
}

func waitForSriovValidatingWebhookRemoved(timeout, interval time.Duration) error {
	err := wait.PollUntilContextTimeout(context.TODO(), interval, timeout, true, func(ctx context.Context) (bool, error) {
		return isValidatingWebhookConfigurationAbsent(tsparams.ValidatingWebhookSriovOperatorConfig)
	})
	if err != nil {
		return fmt.Errorf("validating webhook %s was not removed: %w", tsparams.ValidatingWebhookSriovOperatorConfig, err)
	}

	return nil
}

func isMutatingWebhookConfigurationAbsent(name string) (bool, error) {
	if err := APIClient.AttachScheme(admregv1.AddToScheme); err != nil {
		return false, fmt.Errorf("failed to attach admissionregistration scheme: %w", err)
	}

	mutatingWebhook := &admregv1.MutatingWebhookConfiguration{}

	err := APIClient.Client.Get(context.TODO(), runtimeclient.ObjectKey{Name: name}, mutatingWebhook)
	if k8serrors.IsNotFound(err) {
		return true, nil
	}

	if err != nil {
		return false, fmt.Errorf("failed to get mutating webhook %s: %w", name, err)
	}

	return false, nil
}

func isValidatingWebhookConfigurationAbsent(name string) (bool, error) {
	if err := APIClient.AttachScheme(admregv1.AddToScheme); err != nil {
		return false, fmt.Errorf("failed to attach admissionregistration scheme: %w", err)
	}

	validatingWebhook := &admregv1.ValidatingWebhookConfiguration{}

	err := APIClient.Client.Get(context.TODO(), runtimeclient.ObjectKey{Name: name}, validatingWebhook)
	if k8serrors.IsNotFound(err) {
		return true, nil
	}

	if err != nil {
		return false, fmt.Errorf("failed to get validating webhook %s: %w", name, err)
	}

	return false, nil
}

func expectMutatingWebhookFailurePolicy(
	webhooks []admregv1.MutatingWebhook,
	name string,
	expected admregv1.FailurePolicyType) error {
	webhook, found := findMutatingWebhookByName(webhooks, name)
	if !found || webhook.FailurePolicy == nil {
		return fmt.Errorf("webhook %s has no failure policy configured", name)
	}

	if *webhook.FailurePolicy != expected {
		return fmt.Errorf("webhook %s failure policy is %q, want %q", name, *webhook.FailurePolicy, expected)
	}

	return nil
}

func expectValidatingWebhookFailurePolicy(
	webhooks []admregv1.ValidatingWebhook,
	name string,
	expected admregv1.FailurePolicyType) error {
	webhook, found := findValidatingWebhookByName(webhooks, name)
	if !found || webhook.FailurePolicy == nil {
		return fmt.Errorf("webhook %s has no failure policy configured", name)
	}

	if *webhook.FailurePolicy != expected {
		return fmt.Errorf("webhook %s failure policy is %q, want %q", name, *webhook.FailurePolicy, expected)
	}

	return nil
}

func findMutatingWebhookByName(webhooks []admregv1.MutatingWebhook, name string) (admregv1.MutatingWebhook, bool) {
	entryName := name + ".k8s.io"
	for _, webhook := range webhooks {
		if webhook.Name == name || webhook.Name == entryName {
			return webhook, true
		}
	}

	return admregv1.MutatingWebhook{}, false
}

func findValidatingWebhookByName(
	webhooks []admregv1.ValidatingWebhook,
	name string,
) (admregv1.ValidatingWebhook, bool) {
	entryName := name + ".k8s.io"
	for _, webhook := range webhooks {
		if webhook.Name == name || webhook.Name == entryName {
			return webhook, true
		}
	}

	return admregv1.ValidatingWebhook{}, false
}
