package deploy

import (
	"context"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/kmm"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// KMMCustomResourceCleaner implements CustomResourceCleaner for KMM operators.
type KMMCustomResourceCleaner struct {
	APIClient *clients.Settings
	Namespace string
	LogLevel  glog.Level
}

// NewKMMCustomResourceCleaner creates a new KMM custom resource cleaner.
func NewKMMCustomResourceCleaner(
	apiClient *clients.Settings,
	namespace string,
	logLevel glog.Level) *KMMCustomResourceCleaner {
	return &KMMCustomResourceCleaner{
		APIClient: apiClient,
		Namespace: namespace,
		LogLevel:  logLevel,
	}
}

// CleanupCustomResources implements the CustomResourceCleaner interface for KMM.
func (k *KMMCustomResourceCleaner) CleanupCustomResources() error {
	glog.V(k.LogLevel).Infof("Deleting KMM custom resources in namespace %s", k.Namespace)

	ctx := context.Background()
	totalDeleted := 0

	namespacesToCheck := []string{k.Namespace}
	if k.Namespace != "openshift-kmm" {
		namespacesToCheck = append(namespacesToCheck, "openshift-kmm")

		glog.V(k.LogLevel).Info("Also checking openshift-kmm namespace for KMM Modules")
	}

	for _, nsToCheck := range namespacesToCheck {
		deleted, err := k.cleanupModulesInNamespace(ctx, nsToCheck)
		if err != nil {
			glog.V(k.LogLevel).Infof("Error cleaning up modules in namespace %s: %v", nsToCheck, err)

			continue
		}

		totalDeleted += deleted
	}

	if totalDeleted == 0 {
		glog.V(k.LogLevel).Info("No KMM custom resources found to delete in any checked namespace")
	} else {
		glog.V(k.LogLevel).Infof("Successfully cleaned up %d KMM custom resources across all namespaces", totalDeleted)
	}

	return nil
}

// Interface verification.
var _ CustomResourceCleaner = (*KMMCustomResourceCleaner)(nil)

func (k *KMMCustomResourceCleaner) cleanupModulesInNamespace(
	ctx context.Context,
	nsToCheck string) (int, error) {
	glog.V(k.LogLevel).Infof("Looking for KMM Modules in namespace: %s", nsToCheck)

	moduleList := &unstructured.UnstructuredList{}
	moduleList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "kmm.sigs.x-k8s.io",
		Version: "v1beta1",
		Kind:    "ModuleList",
	})

	err := k.APIClient.Client.List(ctx, moduleList, client.InNamespace(nsToCheck))
	if err != nil {
		if errors.IsNotFound(err) ||
			strings.Contains(err.Error(), "no matches for kind") ||
			strings.Contains(err.Error(), "resource mapping not found") {
			glog.V(k.LogLevel).Infof("KMM Module CRD not available in namespace %s - skipping", nsToCheck)

			return 0, nil
		}

		glog.V(k.LogLevel).Infof("Error listing KMM Modules in namespace %s: %v", nsToCheck, err)

		return 0, err
	}

	glog.V(k.LogLevel).Infof("Found %d KMM Module(s) to delete in namespace %s", len(moduleList.Items), nsToCheck)

	if len(moduleList.Items) == 0 {
		glog.V(k.LogLevel).Infof("No KMM modules found in namespace %s", nsToCheck)

		return 0, nil
	}

	deletedCount := 0

	for _, module := range moduleList.Items {
		moduleName := module.GetName()

		glog.V(k.LogLevel).Infof("Deleting KMM Module: %s in namespace %s", moduleName, nsToCheck)

		moduleBuilder := kmm.NewModuleBuilder(k.APIClient, moduleName, nsToCheck)
		if moduleBuilder == nil {
			glog.V(k.LogLevel).Infof("Failed to create KMM module builder for %s", moduleName)

			continue
		}

		_, err = moduleBuilder.Delete()
		if err != nil {
			glog.V(k.LogLevel).Infof("Error deleting KMM Module %s: %v", moduleName, err)
		} else {
			glog.V(k.LogLevel).Infof("Successfully deleted KMM Module: %s", moduleName)

			deletedCount++
		}
	}

	if len(moduleList.Items) > 0 {
		glog.V(k.LogLevel).Infof("Waiting for KMM Modules to be fully removed from namespace %s...", nsToCheck)

		err = k.waitForModulesRemoval(ctx, nsToCheck)
		if err != nil {
			glog.V(k.LogLevel).Infof("Timeout waiting for KMM Modules removal from namespace %s: %v", nsToCheck, err)
		}
	}

	return deletedCount, nil
}

func (k *KMMCustomResourceCleaner) waitForModulesRemoval(ctx context.Context, nsToCheck string) error {
	return wait.PollUntilContextTimeout(
		ctx, 10*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
			currentList := &unstructured.UnstructuredList{}
			currentList.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   "kmm.sigs.x-k8s.io",
				Version: "v1beta1",
				Kind:    "ModuleList",
			})

			err := k.APIClient.Client.List(ctx, currentList, client.InNamespace(nsToCheck))
			if err != nil {
				return false, nil
			}

			if len(currentList.Items) == 0 {
				glog.V(k.LogLevel).Infof("All KMM Modules successfully removed from namespace %s", nsToCheck)

				return true, nil
			}

			glog.V(k.LogLevel).
				Infof("Still waiting for %d KMM Modules to be removed from namespace %s",
					len(currentList.Items),
					nsToCheck)

			return false, nil
		})
}
