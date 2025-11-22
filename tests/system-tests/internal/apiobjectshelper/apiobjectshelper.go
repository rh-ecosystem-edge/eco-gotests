package apiobjectshelper

import (
	"context"
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/rbac"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/service"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/serviceaccount"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/namespace"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/olm"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/await"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/csv"
	"k8s.io/apimachinery/pkg/util/wait"
)

// VerifyNamespaceExists asserts specific namespace exists.
func VerifyNamespaceExists(apiClient *clients.Settings, nsname string, timeout time.Duration) error {
	klog.V(90).Infof("Verify namespace %q exists", nsname)

	err := wait.PollUntilContextTimeout(context.TODO(), time.Second, timeout, true,
		func(ctx context.Context) (bool, error) {
			_, pullErr := namespace.Pull(apiClient, nsname)
			if pullErr != nil {
				klog.V(90).Infof("Failed to pull in namespace %q - %v", nsname, pullErr)

				return false, nil
			}

			return true, nil
		})
	if err != nil {
		return fmt.Errorf("failed to pull in %s namespace", nsname)
	}

	return nil
}

// VerifyOperatorDeployment assert that specific deployment succeeded.
func VerifyOperatorDeployment(apiClient *clients.Settings,
	subscriptionName, deploymentName, nsname string, timeout time.Duration) error {
	klog.V(90).Infof("Verify deployment %s in namespace %s", deploymentName, nsname)

	if deploymentName == "" {
		return fmt.Errorf("operator deployment name have to be provided")
	}

	if subscriptionName != "" {
		csvName, err := csv.GetCurrentCSVNameFromSubscription(apiClient, subscriptionName, nsname)
		if err != nil {
			return fmt.Errorf("csv %s not found in namespace %s", csvName, nsname)
		}

		csvObj, err := olm.PullClusterServiceVersion(apiClient, csvName, nsname)
		if err != nil {
			return fmt.Errorf("failed to pull %q csv from the %s namespace", csvName, nsname)
		}

		isSuccessful, err := csvObj.IsSuccessful()
		if err != nil {
			return fmt.Errorf("failed to verify csv %s in the namespace %s status", csvName, nsname)
		}

		if !isSuccessful {
			return fmt.Errorf("failed to deploy %s; the csv %s in the namespace %s status %v",
				subscriptionName, csvName, nsname, isSuccessful)
		}
	}

	klog.V(90).Infof("Confirm that operator %s is running in namespace %s", deploymentName, nsname)

	err := await.WaitUntilDeploymentReady(apiClient, deploymentName, nsname, timeout)
	if err != nil {
		return fmt.Errorf("deployment %s not found in %s namespace; %w", deploymentName, nsname, err)
	}

	return nil
}

// CreateServiceAccount creates the service account and verifies it was created.
func CreateServiceAccount(apiClient *clients.Settings, saName, nsName string) error {
	klog.V(100).Infof("Creating ServiceAccount %q in %q namespace",
		saName, nsName)
	klog.V(100).Infof("Creating SA %q in %q namespace", saName, nsName)

	deploySa := serviceaccount.NewBuilder(apiClient, saName, nsName)

	err := wait.PollUntilContextTimeout(
		context.TODO(),
		time.Second*15,
		time.Minute,
		true,
		func(ctx context.Context) (bool, error) {
			deploySa, err := deploySa.Create()
			if err != nil {
				klog.V(100).Infof("Error creating SA %q in %q namespace: %v", saName, nsName, err)

				return false, nil
			}

			klog.V(100).Infof("Created SA %q in %q namespace",
				deploySa.Definition.Name, deploySa.Definition.Namespace)

			return true, nil
		})
	if err != nil {
		return fmt.Errorf("failed to create ServiceAccount %q in %q namespace", saName, nsName)
	}

	return nil
}

// CreateClusterRBAC creates the RBAC and verifies it was created.
func CreateClusterRBAC(
	apiClient *clients.Settings,
	rbacName, clusterRole, saName, nsName string) error {
	klog.V(100).Infof("Creating RBAC for SA %s", saName)

	klog.V(100).Infof("Creating ClusterRoleBinding %q", rbacName)
	crbSa := rbac.NewClusterRoleBindingBuilder(
		apiClient,
		rbacName,
		clusterRole,
		rbacv1.Subject{
			Name:      saName,
			Kind:      "ServiceAccount",
			Namespace: nsName,
		})

	err := wait.PollUntilContextTimeout(
		context.TODO(),
		time.Second*15,
		time.Minute,
		true,
		func(ctx context.Context) (bool, error) {
			crbSa, err := crbSa.Create()
			if err != nil {
				klog.V(100).Infof(
					"Error Creating ClusterRoleBinding %q : %v", crbSa.Definition.Name, err)

				return false, nil
			}

			klog.V(100).Infof("ClusterRoleBinding %q created:\n\t%v",
				crbSa.Definition.Name, crbSa)

			return true, nil
		})
	if err != nil {
		return fmt.Errorf("failed to create ClusterRoleBinding '%s' during timeout %v; %w",
			rbacName, time.Minute, err)
	}

	return nil
}

// DeleteService deletes the service and verifies it was removed.
func DeleteService(apiClient *clients.Settings, svcName, nsName string) error {
	klog.V(100).Infof("Delete service %q from namespace %s", svcName, nsName)

	if svcObj, err := service.Pull(
		apiClient, svcName, nsName); err == nil {
		klog.V(100).Infof("Service %q found in %q nsname", svcName, nsName)
		klog.V(100).Infof("Deleting service %q in %q nsname", svcName, nsName)

		err = wait.PollUntilContextTimeout(
			context.TODO(),
			time.Second*15,
			time.Minute,
			true,
			func(ctx context.Context) (bool, error) {
				err := svcObj.Delete()
				if err != nil {
					klog.V(100).Infof("Error deleting service %q in %q nsname: %v",
						svcName, nsName, err)

					return false, nil
				}

				klog.V(100).Infof("Deleted service %q in %q nsname", svcName, nsName)

				return true, nil
			})
		if err != nil {
			return fmt.Errorf("failed to delete service %q from %q ns", svcName, nsName)
		}
	} else {
		klog.V(100).Infof("service %q not found in %q nsname", svcName, nsName)
	}

	return nil
}

// DeleteClusterRBAC deletes the RBAC and verifies it was removed.
func DeleteClusterRBAC(apiClient *clients.Settings, rbacName string) error {
	klog.V(100).Infof("Deleting Cluster RBAC")

	klog.V(100).Infof("Assert ClusterRoleBinding %q exists", rbacName)

	crbSa, err := rbac.PullClusterRoleBinding(apiClient, rbacName)
	if err != nil {
		klog.V(100).Infof("ClusterRoleBinding %q not found; %v", rbacName, err)

		return nil
	}

	klog.V(100).Infof("ClusterRoleBinding %q found. Deleting...", rbacName)

	err = wait.PollUntilContextTimeout(
		context.TODO(),
		time.Second*15,
		time.Minute,
		true,
		func(ctx context.Context) (bool, error) {
			err = crbSa.Delete()
			if err != nil {
				klog.V(100).Infof("Error deleting ClusterRoleBinding %q : %v", rbacName, err)

				return false, nil
			}

			klog.V(100).Infof("Deleted ClusterRoleBinding %q", rbacName)

			return true, nil
		})
	if err != nil {
		return fmt.Errorf("failed to delete Cluster RBAC %q", rbacName)
	}

	return nil
}

// DeleteServiceAccount deletes the service account and verifies it was removed.
func DeleteServiceAccount(apiClient *clients.Settings, saName, nsName string) error {
	klog.V(100).Infof("Removing Service Account")
	klog.V(100).Infof("Assert SA %q exists in %q namespace", saName, nsName)

	if deploySa, err := serviceaccount.Pull(
		apiClient, saName, nsName); err == nil {
		klog.V(100).Infof("ServiceAccount %q found in %q namespace", saName, nsName)
		klog.V(100).Infof("Deleting ServiceAccount %q in %q namespace", saName, nsName)

		err = wait.PollUntilContextTimeout(
			context.TODO(),
			time.Second*15,
			time.Minute,
			true,
			func(ctx context.Context) (bool, error) {
				err := deploySa.Delete()
				if err != nil {
					klog.V(100).Infof("Error deleting ServiceAccount %q in %q namespace: %v",
						saName, nsName, err)

					return false, nil
				}

				klog.V(100).Infof("Deleted ServiceAccount %q in %q namespace", saName, nsName)

				return true, nil
			})
		if err != nil {
			return fmt.Errorf("failed to delete ServiceAccount %q from %q ns", saName, nsName)
		}
	} else {
		klog.V(100).Infof("ServiceAccount %q not found in %q namespace", saName, nsName)
	}

	return nil
}

// DeleteDeployment deletes the deployment and verifies it and all related pods were removed.
func DeleteDeployment(
	apiClient *clients.Settings,
	deploymentName, nsName string) error {
	klog.V(100).Infof("Removing test deployment %q from %q ns", deploymentName, nsName)

	if deploymentObj, err := deployment.Pull(apiClient, deploymentName, nsName); err == nil {
		klog.V(100).Infof("Deleting deployment %q from %q namespace", deploymentName, nsName)

		err = deploymentObj.DeleteAndWait(300 * time.Second)
		if err != nil {
			klog.V(100).Infof("Error deleting deployment %q from %q namespace: %v",
				deploymentName, nsName, err)

			return fmt.Errorf("failed to delete deployment %q from %q namespace: %w",
				deploymentName, nsName, err)
		}
	} else {
		klog.V(100).Infof("deployment %q not found in %q namespace", deploymentName, nsName)
	}

	return nil
}

// EnsureAllPodsRemoved Ensure all deployment pods in namespace with the specific pod label were removed.
func EnsureAllPodsRemoved(
	apiClient *clients.Settings,
	nsName, podLabel string) error {
	klog.V(100).Infof("Ensuring pods in %q namespace with label %q are gone", nsName, podLabel)

	err := wait.PollUntilContextTimeout(
		context.TODO(),
		time.Second*3,
		time.Minute*6,
		true,
		func(ctx context.Context) (bool, error) {
			oldPods, _ := pod.List(apiClient, nsName,
				metav1.ListOptions{LabelSelector: podLabel})

			return len(oldPods) == 0, nil
		})
	if err != nil {
		return fmt.Errorf("pods matching label(%q) still present in namespace %q",
			podLabel, nsName)
	}

	return nil
}
