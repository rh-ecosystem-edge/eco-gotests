package helper

import (
	"context"
	"fmt"
	"strings"
	"time"

	operatorv1 "github.com/openshift/api/operator/v1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/imageregistry"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/ocm"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/serviceaccount"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/siteconfig"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/storage"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/gitopsztp/internal/tsparams"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/cnf/ran/internal/ranhelper"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	configurationPolicyV1 "open-cluster-management.io/config-policy-controller/api/v1"
)

// WaitForPolicyToExist waits for up to the specified timeout until the policy exists.
func WaitForPolicyToExist(
	client *clients.Settings, name, namespace string, timeout time.Duration) (*ocm.PolicyBuilder, error) {
	var policy *ocm.PolicyBuilder

	err := wait.PollUntilContextTimeout(
		context.TODO(), tsparams.ArgoCdChangeInterval, timeout, true, func(ctx context.Context) (bool, error) {
			var err error

			policy, err = ocm.PullPolicy(client, name, namespace)
			if err == nil {
				return true, nil
			}

			if strings.Contains(err.Error(), "does not exist") {
				return false, nil
			}

			return false, err
		})

	return policy, err
}

// WaitForServiceAccountToExist waits for up to the specified timeout until the service account exists.
func WaitForServiceAccountToExist(
	client *clients.Settings, name, namespace string, timeout time.Duration) (*serviceaccount.Builder, error) {
	var builder *serviceaccount.Builder

	err := wait.PollUntilContextTimeout(
		context.TODO(), tsparams.ArgoCdChangeInterval, timeout, true, func(ctx context.Context) (bool, error) {
			var err error

			builder, err = serviceaccount.Pull(client, name, namespace)
			if err == nil {
				return true, nil
			}

			if strings.Contains(err.Error(), "does not exist") {
				return false, nil
			}

			return false, err
		})

	return builder, err
}

// GetPolicyEvaluationIntervals is used to get the configured evaluation intervals for the specified policy.
func GetPolicyEvaluationIntervals(policy *ocm.PolicyBuilder) (string, string, error) {
	klog.V(tsparams.LogLevel).Infof(
		"Checking policy '%s' in namespace '%s' to fetch evaluation intervals",
		policy.Definition.Name, policy.Definition.Namespace)

	policyTemplates := policy.Definition.Spec.PolicyTemplates
	if len(policyTemplates) < 1 {
		return "", "", fmt.Errorf(
			"could not find policy template for policy %s/%s", policy.Definition.Namespace, policy.Definition.Name)
	}

	configPolicy, err := ranhelper.UnmarshalRaw[configurationPolicyV1.ConfigurationPolicy](
		policyTemplates[0].ObjectDefinition.Raw)
	if err != nil {
		return "", "", err
	}

	complianceInterval := configPolicy.Spec.EvaluationInterval.Compliant
	nonComplianceInterval := configPolicy.Spec.EvaluationInterval.NonCompliant

	return complianceInterval, nonComplianceInterval, nil
}

// RestoreImageRegistry restores the image registry with the provided name back to imageRegistryConfig, copying over the
// labels, annotations, and spec from imageRegistryConfig, then waiting until the image registry is available again.
func RestoreImageRegistry(
	client *clients.Settings, imageRegistryName string, imageRegistryConfig *imageregistry.Builder) error {
	currentImageRegistry, err := imageregistry.Pull(client, imageRegistryName)
	if err != nil {
		return err
	}

	if imageRegistryConfig.Definition.GetAnnotations() != nil {
		currentImageRegistry.Definition.SetAnnotations(imageRegistryConfig.Definition.GetAnnotations())
	}

	if imageRegistryConfig.Definition.GetLabels() != nil {
		currentImageRegistry.Definition.SetLabels(imageRegistryConfig.Definition.GetLabels())
	}

	currentImageRegistry.Definition.Spec = imageRegistryConfig.Definition.Spec

	currentImageRegistry, err = currentImageRegistry.Update()
	if err != nil {
		return err
	}

	_, err = currentImageRegistry.WaitForCondition(operatorv1.OperatorCondition{
		Type:   "Available",
		Reason: "Removed",
		Status: operatorv1.ConditionTrue,
	}, tsparams.ArgoCdChangeTimeout)

	return err
}

// CleanupImageRegistryConfig deletes the specified resources in the necessary order.
func CleanupImageRegistryConfig(client *clients.Settings) error {
	klog.V(tsparams.LogLevel).Infof(
		"Cleaning up image registry resources with sc=%s, pv=%s, pvc=%s",
		tsparams.ImageRegistrySC, tsparams.ImageRegistryPV, tsparams.ImageRegistryPVC)

	// The resources must be deleted in the order of PVC, PV, then SC to avoid errors.
	pvc, err := storage.PullPersistentVolumeClaim(client, tsparams.ImageRegistryPVC, tsparams.ImageRegistryNamespace)
	if err == nil {
		err = pvc.DeleteAndWait(tsparams.ArgoCdChangeTimeout)
		if err != nil {
			return err
		}
	}

	pv, err := storage.PullPersistentVolume(client, tsparams.ImageRegistryPV)
	if err == nil {
		err = pv.DeleteAndWait(tsparams.ArgoCdChangeTimeout)
		if err != nil {
			return err
		}
	}

	sc, err := storage.PullClass(client, tsparams.ImageRegistrySC)
	if err == nil {
		err = sc.DeleteAndWait(tsparams.ArgoCdChangeTimeout)
		if err != nil {
			return err
		}
	}

	return nil
}

// DoesCIExtraLabelsExists looks for a extraLabels on a cluster instance CR and returns true if it exists.
func DoesCIExtraLabelsExists(
	client *clients.Settings, name string, namespace string, testKey string, testLabelKey string) (bool, error) {
	clusterInstance, err := siteconfig.PullClusterInstance(client, name, namespace)
	if err != nil {
		return false, err
	}

	_, exists := clusterInstance.Object.Spec.ExtraLabels[testKey][testLabelKey]

	return exists, nil
}
