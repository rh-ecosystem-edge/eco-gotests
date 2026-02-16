package set

import (
	"context"
	"fmt"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/configmap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

// CreateLocalSourceConfigMap creates a ConfigMap for NFD local source features.
func CreateLocalSourceConfigMap(apiClient *clients.Settings, namespace string, features map[string]string) error {
	klog.V(100).Infof("Creating local source ConfigMap in namespace %s", namespace)

	cmName := "nfd-worker-local-features"

	// Check if ConfigMap already exists
	existingCM, err := configmap.Pull(apiClient, cmName, namespace)
	if err == nil && existingCM != nil {
		klog.V(100).Infof("ConfigMap %s already exists, updating it", cmName)
		// Update existing ConfigMap
		existingCM.Definition.Data = features
		_, err = existingCM.Update()
		if err != nil {
			return fmt.Errorf("failed to update ConfigMap %s: %w", cmName, err)
		}
		return nil
	}

	// Create new ConfigMap
	cmBuilder := configmap.NewBuilder(apiClient, cmName, namespace)
	cmBuilder.WithData(features)

	_, err = cmBuilder.Create()
	if err != nil {
		return fmt.Errorf("failed to create ConfigMap %s: %w", cmName, err)
	}

	klog.V(100).Infof("Successfully created ConfigMap %s", cmName)
	return nil
}

// DeleteLocalSourceConfigMap deletes the NFD local source ConfigMap.
func DeleteLocalSourceConfigMap(apiClient *clients.Settings, namespace string) error {
	cmName := "nfd-worker-local-features"
	klog.V(100).Infof("Deleting ConfigMap %s from namespace %s", cmName, namespace)

	cm, err := configmap.Pull(apiClient, cmName, namespace)
	if err != nil {
		klog.V(100).Infof("ConfigMap %s does not exist or error pulling: %v", cmName, err)
		return nil
	}

	err = cm.Delete()
	if err != nil {
		return fmt.Errorf("failed to delete ConfigMap %s: %w", cmName, err)
	}

	klog.V(100).Infof("Successfully deleted ConfigMap %s", cmName)
	return nil
}

// CreateFeatureFile creates a feature file content for NFD local source.
func CreateFeatureFile(features map[string]string) string {
	var content string
	for key, value := range features {
		content += fmt.Sprintf("%s=%s\n", key, value)
	}
	return content
}

// UpdateNFDWorkerWithLocalSource updates NFD worker configuration to enable local source.
func UpdateNFDWorkerWithLocalSource(apiClient *clients.Settings, namespace string, enable bool) error {
	klog.V(100).Infof("Updating NFD worker to %s local source", map[bool]string{true: "enable", false: "disable"}[enable])

	// This would typically involve updating the NFD CR or worker DaemonSet
	// to mount the ConfigMap or hostPath for local features
	// Implementation depends on specific NFD deployment method

	ctx := context.Background()

	// Get NFD worker DaemonSet
	dsList, err := apiClient.AppsV1Interface.DaemonSets(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app=nfd-worker",
	})

	if err != nil {
		return fmt.Errorf("failed to list NFD worker DaemonSets: %w", err)
	}

	if len(dsList.Items) == 0 {
		return fmt.Errorf("no NFD worker DaemonSet found")
	}

	ds := dsList.Items[0]

	// Add or remove ConfigMap volume mount
	if enable {
		// Add volume for ConfigMap
		volume := corev1.Volume{
			Name: "local-features",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "nfd-worker-local-features",
					},
				},
			},
		}

		// Check if volume already exists
		volumeExists := false
		for _, v := range ds.Spec.Template.Spec.Volumes {
			if v.Name == "local-features" {
				volumeExists = true
				break
			}
		}

		if !volumeExists {
			ds.Spec.Template.Spec.Volumes = append(ds.Spec.Template.Spec.Volumes, volume)

			// Add volume mount to container
			for i := range ds.Spec.Template.Spec.Containers {
				if ds.Spec.Template.Spec.Containers[i].Name == "nfd-worker" {
					volumeMount := corev1.VolumeMount{
						Name:      "local-features",
						MountPath: "/etc/kubernetes/node-feature-discovery/features.d",
						ReadOnly:  true,
					}
					ds.Spec.Template.Spec.Containers[i].VolumeMounts = append(
						ds.Spec.Template.Spec.Containers[i].VolumeMounts,
						volumeMount,
					)
					break
				}
			}

			_, err = apiClient.AppsV1Interface.DaemonSets(namespace).Update(ctx, &ds, metav1.UpdateOptions{})
			if err != nil {
				return fmt.Errorf("failed to update NFD worker DaemonSet: %w", err)
			}
		}
	}

	klog.V(100).Info("Successfully updated NFD worker configuration")
	return nil
}
