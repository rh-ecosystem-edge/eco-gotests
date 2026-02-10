package rdscorecommon

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/statefulset"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/storage"

	. "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreinittools"
	"github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/rdscore/internal/rdscoreparams"
)

// DumpPodStatusOnFailure dumps comprehensive pod status information when a pod fails to become ready.
// This function provides detailed debugging information including pod conditions, container statuses,
// and scheduling information to help diagnose test failures.
//
// Parameters:
//   - podBuilder: The pod.Builder object that failed to become ready
//   - err: The error returned from WaitUntilReady (typically context.DeadlineExceeded)
//
// The function logs to klog and adds a Ginkgo ReportEntry that is only visible on test failure.
//
//nolint:gocognit,gocyclo,funlen
func DumpPodStatusOnFailure(podBuilder *pod.Builder, err error) {
	if err == nil || podBuilder == nil || podBuilder.Object == nil {
		return
	}

	podName := podBuilder.Definition.Name
	podNS := podBuilder.Definition.Namespace

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("========================================")
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod %q in namespace %q Failed to Become Ready", podName, podNS)
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("========================================")

	// 1. Pod Phase
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod Phase: %s", podBuilder.Object.Status.Phase)

	// 2. Pod Conditions (critical for understanding why pod is not ready)
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod Conditions:")

	if len(podBuilder.Object.Status.Conditions) > 0 {
		for _, cond := range podBuilder.Object.Status.Conditions {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  - Type: %s, Status: %s, Reason: %s",
				cond.Type, cond.Status, cond.Reason)

			if cond.Message != "" {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    Message: %s", cond.Message)
			}
		}
	} else {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  <none>")
	}

	// 3. Container Statuses
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")

	if len(podBuilder.Object.Status.ContainerStatuses) > 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Container Statuses:")

		for _, containerStatus := range podBuilder.Object.Status.ContainerStatuses {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  - Name: %s, Ready: %t, RestartCount: %d",
				containerStatus.Name, containerStatus.Ready, containerStatus.RestartCount)

			if containerStatus.State.Waiting != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    State: Waiting")
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    Reason: %s", containerStatus.State.Waiting.Reason)

				if containerStatus.State.Waiting.Message != "" {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    Message: %s", containerStatus.State.Waiting.Message)
				}
			}

			if containerStatus.State.Terminated != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    State: Terminated")
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    Reason: %s, ExitCode: %d",
					containerStatus.State.Terminated.Reason, containerStatus.State.Terminated.ExitCode)

				if containerStatus.State.Terminated.Message != "" {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    Message: %s", containerStatus.State.Terminated.Message)
				}
			}

			if containerStatus.State.Running != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    State: Running since %s",
					containerStatus.State.Running.StartedAt.Format(time.RFC3339))
			}
		}
	}

	// 4. Init Container Statuses (often the cause of Pending state)
	if len(podBuilder.Object.Status.InitContainerStatuses) > 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Init Container Statuses:")

		for _, ics := range podBuilder.Object.Status.InitContainerStatuses {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  - Name: %s, Ready: %t, RestartCount: %d",
				ics.Name, ics.Ready, ics.RestartCount)

			if ics.State.Waiting != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    State: Waiting")
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    Reason: %s", ics.State.Waiting.Reason)

				if ics.State.Waiting.Message != "" {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    Message: %s", ics.State.Waiting.Message)
				}
			}

			if ics.State.Terminated != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    State: Terminated")
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    Reason: %s, ExitCode: %d",
					ics.State.Terminated.Reason, ics.State.Terminated.ExitCode)
			}
		}
	}

	// 5. Scheduling Information
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")

	if podBuilder.Object.Spec.NodeName == "" {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Scheduling: Pod has not been scheduled to any node")

		if podBuilder.Object.Status.NominatedNodeName != "" {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Nominated Node: %s", podBuilder.Object.Status.NominatedNodeName)
		}
	} else {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Scheduled to Node: %s", podBuilder.Object.Spec.NodeName)
	}

	// 6. Owner References (to trace back to Deployment/StatefulSet/etc.)
	if len(podBuilder.Object.OwnerReferences) > 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Owner References:")

		for _, owner := range podBuilder.Object.OwnerReferences {
			// Safely handle nil Controller field
			isController := false
			if owner.Controller != nil {
				isController = *owner.Controller
			}

			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  - Kind: %s, Name: %s, Controller: %t",
				owner.Kind, owner.Name, isController)
		}
	}

	// 7. Resource Requirements (helpful for understanding scheduling failures)
	if len(podBuilder.Object.Spec.Containers) > 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Resource Requirements:")

		for _, container := range podBuilder.Object.Spec.Containers {
			if len(container.Resources.Requests) > 0 || len(container.Resources.Limits) > 0 {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Container %s:", container.Name)

				if len(container.Resources.Requests) > 0 {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    Requests:")

					for resourceName, quantity := range container.Resources.Requests {
						klog.V(rdscoreparams.RDSCoreLogLevel).Infof("      %s: %s", resourceName, quantity.String())
					}
				}

				if len(container.Resources.Limits) > 0 {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    Limits:")

					for resourceName, quantity := range container.Resources.Limits {
						klog.V(rdscoreparams.RDSCoreLogLevel).Infof("      %s: %s", resourceName, quantity.String())
					}
				}
			}
		}
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("========================================")
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("End of Pod %q Status Dump", podName)
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("========================================")

	// Add to Ginkgo report (only visible on failure or in verbose mode)
	AddReportEntry(
		fmt.Sprintf("Pod %s Failure Details", podName),
		fmt.Sprintf("Pod %q in namespace %q failed with phase %s. Check logs above for detailed status.",
			podName, podNS, podBuilder.Object.Status.Phase),
		ReportEntryVisibilityFailureOrVerbose,
	)
}

// DumpDeploymentStatus dumps comprehensive status information for all Deployments in a given namespace.
// This function is useful for debugging test failures related to deployment readiness issues.
//
// Parameters:
//   - ctx: The SpecContext for the current test (can be canceled)
//   - namespace: The namespace to query for deployments
//
// The function creates a fresh context if the spec context is canceled and logs deployment details.
//
//nolint:funlen
func DumpDeploymentStatus(ctx SpecContext, namespace string) {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("========================================")
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deployment Status Dump for Namespace: %s", namespace)
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("========================================")

	// Check if the incoming context was already canceled
	if ctx.Err() != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"WARNING: SpecContext was already canceled (%v), using fresh context for dump", ctx.Err())
	}

	var deployments []*deployment.Builder

	// Create a fresh context to ensure dump works even if spec context is canceled
	dumpCtx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	err := wait.PollUntilContextTimeout(dumpCtx, 15*time.Second, 1*time.Minute, true,
		func(context.Context) (bool, error) {
			var listErr error

			deployments, listErr = deployment.List(APIClient, namespace)
			if listErr != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to list deployments (retrying...): %v", listErr)

				return false, nil
			}

			return true, nil
		})
	if err != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to retrieve deployment list after retries: %v", err)

		return
	}

	if len(deployments) == 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("No deployments found in namespace %q", namespace)
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("========================================")

		return
	}

	for _, deploy := range deployments {
		if deploy.Object == nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Skipping deployment with nil Object")

			continue
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deployment: %s", deploy.Object.Name)
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("----------------------------------------")

		// Replica Status
		desiredReplicas := int32(0)
		if deploy.Object.Spec.Replicas != nil {
			desiredReplicas = *deploy.Object.Spec.Replicas
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Replicas:")
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Desired: %d", desiredReplicas)
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Current: %d", deploy.Object.Status.Replicas)
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Ready: %d", deploy.Object.Status.ReadyReplicas)
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Available: %d", deploy.Object.Status.AvailableReplicas)
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Updated: %d", deploy.Object.Status.UpdatedReplicas)
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Unavailable: %d", deploy.Object.Status.UnavailableReplicas)

		// Deployment Conditions
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Conditions:")

		if len(deploy.Object.Status.Conditions) > 0 {
			for _, cond := range deploy.Object.Status.Conditions {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  - Type: %s, Status: %s, Reason: %s",
					cond.Type, cond.Status, cond.Reason)

				if cond.Message != "" {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    Message: %s", cond.Message)
				}
			}
		} else {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  <none>")
		}

		// Strategy
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Strategy: %s", deploy.Object.Spec.Strategy.Type)

		// Selector
		if deploy.Object.Spec.Selector != nil && len(deploy.Object.Spec.Selector.MatchLabels) > 0 {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Selector Labels:")

			for key, value := range deploy.Object.Spec.Selector.MatchLabels {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  %s: %s", key, value)
			}
		}
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("========================================")
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("End of Deployment Status Dump for Namespace: %s", namespace)
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("========================================")
}

// DumpStatefulSetStatus dumps comprehensive status information for all StatefulSets in a given namespace.
// This function is useful for debugging test failures related to statefulset readiness issues.
//
// Parameters:
//   - ctx: The SpecContext for the current test (can be canceled)
//   - namespace: The namespace to query for statefulsets
//
// The function creates a fresh context if the spec context is canceled and logs statefulset details.
//
//nolint:funlen
func DumpStatefulSetStatus(ctx SpecContext, namespace string) {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("========================================")
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("StatefulSet Status Dump for Namespace: %s", namespace)
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("========================================")

	// Check if the incoming context was already canceled
	if ctx.Err() != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"WARNING: SpecContext was already canceled (%v), using fresh context for dump", ctx.Err())
	}

	var statefulsets []*statefulset.Builder

	// Create a fresh context to ensure dump works even if spec context is canceled
	dumpCtx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	err := wait.PollUntilContextTimeout(dumpCtx, 15*time.Second, 1*time.Minute, true,
		func(context.Context) (bool, error) {
			var listErr error

			statefulsets, listErr = statefulset.List(APIClient, namespace)
			if listErr != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to list statefulsets (retrying...): %v", listErr)

				return false, nil
			}

			return true, nil
		})
	if err != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to retrieve statefulset list after retries: %v", err)

		return
	}

	if len(statefulsets) == 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("No statefulsets found in namespace %q", namespace)
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("========================================")

		return
	}

	for _, sts := range statefulsets {
		if sts.Object == nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Skipping statefulset with nil Object")

			continue
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("StatefulSet: %s", sts.Object.Name)
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("----------------------------------------")

		// Replica Status
		desiredReplicas := int32(0)
		if sts.Object.Spec.Replicas != nil {
			desiredReplicas = *sts.Object.Spec.Replicas
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Replicas:")
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Desired: %d", desiredReplicas)
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Current: %d", sts.Object.Status.Replicas)
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Ready: %d", sts.Object.Status.ReadyReplicas)
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Updated: %d", sts.Object.Status.UpdatedReplicas)

		// StatefulSet-specific status
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Current Revision: %s", sts.Object.Status.CurrentRevision)
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Update Revision: %s", sts.Object.Status.UpdateRevision)

		if sts.Object.Status.ObservedGeneration > 0 {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Observed Generation: %d", sts.Object.Status.ObservedGeneration)
		}

		// StatefulSet Conditions
		if len(sts.Object.Status.Conditions) > 0 {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Conditions:")

			for _, cond := range sts.Object.Status.Conditions {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  - Type: %s, Status: %s, Reason: %s",
					cond.Type, cond.Status, cond.Reason)

				if cond.Message != "" {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    Message: %s", cond.Message)
				}
			}
		}

		// Update Strategy
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Update Strategy: %s", sts.Object.Spec.UpdateStrategy.Type)

		// Service Name
		if sts.Object.Spec.ServiceName != "" {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Service Name: %s", sts.Object.Spec.ServiceName)
		}

		// Selector
		if sts.Object.Spec.Selector != nil && len(sts.Object.Spec.Selector.MatchLabels) > 0 {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Selector Labels:")

			for key, value := range sts.Object.Spec.Selector.MatchLabels {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  %s: %s", key, value)
			}
		}
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("========================================")
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("End of StatefulSet Status Dump for Namespace: %s", namespace)
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("========================================")
}

// DumpPersistentVolumeStatus dumps comprehensive status information for all PersistentVolumes in the cluster.
// This function is useful for debugging test failures related to storage provisioning and PVC binding issues.
//
// Parameters:
//   - ctx: The SpecContext for the current test (can be canceled)
//
// The function creates a fresh context if the spec context is canceled and logs PV details.
//
//nolint:funlen
func DumpPersistentVolumeStatus(ctx SpecContext) {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("========================================")
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("PersistentVolume Status Dump (Cluster-wide)")
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("========================================")

	// Check if the incoming context was already canceled
	if ctx.Err() != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"WARNING: SpecContext was already canceled (%v), using fresh context for dump", ctx.Err())
	}

	var pvs []*storage.PVBuilder

	// Create a fresh context to ensure dump works even if spec context is canceled
	dumpCtx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	err := wait.PollUntilContextTimeout(dumpCtx, 15*time.Second, 1*time.Minute, true,
		func(context.Context) (bool, error) {
			var listErr error

			pvs, listErr = storage.ListPV(APIClient)
			if listErr != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to list PersistentVolumes (retrying...): %v", listErr)

				return false, nil
			}

			return true, nil
		})
	if err != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to retrieve PersistentVolume list after retries: %v", err)

		return
	}

	if len(pvs) == 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("No PersistentVolumes found in cluster")
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("========================================")

		return
	}

	for _, persistentVolume := range pvs {
		if persistentVolume.Object == nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Skipping PersistentVolume with nil Object")

			continue
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("PersistentVolume: %s", persistentVolume.Object.Name)
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("----------------------------------------")

		// Phase and basic info
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Phase: %s", persistentVolume.Object.Status.Phase)

		// Capacity
		if capacity, ok := persistentVolume.Object.Spec.Capacity["storage"]; ok {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Capacity: %s", capacity.String())
		}

		// Access Modes
		if len(persistentVolume.Object.Spec.AccessModes) > 0 {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Access Modes:")

			for _, mode := range persistentVolume.Object.Spec.AccessModes {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  - %s", mode)
			}
		}

		// Reclaim Policy
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Reclaim Policy: %s",
			persistentVolume.Object.Spec.PersistentVolumeReclaimPolicy)

		// Storage Class
		if persistentVolume.Object.Spec.StorageClassName != "" {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Storage Class: %s", persistentVolume.Object.Spec.StorageClassName)
		}

		// Volume Mode
		if persistentVolume.Object.Spec.VolumeMode != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Volume Mode: %s", *persistentVolume.Object.Spec.VolumeMode)
		}

		// Claim Reference (which PVC is bound)
		if persistentVolume.Object.Spec.ClaimRef != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Claim Reference:")
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Namespace: %s", persistentVolume.Object.Spec.ClaimRef.Namespace)
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Name: %s", persistentVolume.Object.Spec.ClaimRef.Name)
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  UID: %s", persistentVolume.Object.Spec.ClaimRef.UID)
		} else {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Claim Reference: <none> (unbound)")
		}

		// Node Affinity (important for local volumes)
		if persistentVolume.Object.Spec.NodeAffinity != nil && persistentVolume.Object.Spec.NodeAffinity.Required != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Node Affinity:")
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Required NodeSelectorTerms: %d",
				len(persistentVolume.Object.Spec.NodeAffinity.Required.NodeSelectorTerms))
		}

		// Volume Source Type (helps understand backend)
		volumeSource := "Unknown"

		switch {
		case persistentVolume.Object.Spec.HostPath != nil:
			volumeSource = "HostPath"
		case persistentVolume.Object.Spec.NFS != nil:
			volumeSource = "NFS"
		case persistentVolume.Object.Spec.ISCSI != nil:
			volumeSource = "iSCSI"
		case persistentVolume.Object.Spec.RBD != nil:
			volumeSource = "RBD (Ceph)"
		case persistentVolume.Object.Spec.CephFS != nil:
			volumeSource = "CephFS"
		case persistentVolume.Object.Spec.CSI != nil:
			volumeSource = fmt.Sprintf("CSI (%s)", persistentVolume.Object.Spec.CSI.Driver)
		case persistentVolume.Object.Spec.Local != nil:
			volumeSource = "Local"
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Volume Source: %s", volumeSource)
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("========================================")
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("End of PersistentVolume Status Dump")
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("========================================")
}

// DumpPersistentVolumeClaimStatus dumps comprehensive status information for all PersistentVolumeClaims in a namespace.
// This function is useful for debugging test failures related to PVC binding and provisioning issues.
//
// Parameters:
//   - ctx: The SpecContext for the current test (can be canceled)
//   - namespace: The namespace to query for PVCs
//
// The function creates a fresh context if the spec context is canceled and logs PVC details.
//
//nolint:funlen,gocognit
func DumpPersistentVolumeClaimStatus(ctx SpecContext, namespace string) {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("========================================")
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("PersistentVolumeClaim Status Dump for Namespace: %s", namespace)
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("========================================")

	// Check if the incoming context was already canceled
	if ctx.Err() != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"WARNING: SpecContext was already canceled (%v), using fresh context for dump", ctx.Err())
	}

	var pvcs []*storage.PVCBuilder

	// Create a fresh context to ensure dump works even if spec context is canceled
	dumpCtx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	err := wait.PollUntilContextTimeout(dumpCtx, 15*time.Second, 1*time.Minute, true,
		func(context.Context) (bool, error) {
			var listErr error

			pvcs, listErr = storage.ListPVC(APIClient, namespace)
			if listErr != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
					"Failed to list PersistentVolumeClaims in namespace %q (retrying...): %v", namespace, listErr)

				return false, nil
			}

			return true, nil
		})
	if err != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof(
			"Failed to retrieve PersistentVolumeClaim list for namespace %q after retries: %v", namespace, err)

		return
	}

	if len(pvcs) == 0 {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("No PersistentVolumeClaims found in namespace %q", namespace)
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("========================================")

		return
	}

	for _, pvc := range pvcs {
		if pvc.Object == nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Skipping PersistentVolumeClaim with nil Object")

			continue
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("PersistentVolumeClaim: %s", pvc.Object.Name)
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("----------------------------------------")

		// Phase
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Phase: %s", pvc.Object.Status.Phase)

		// Access Modes
		if len(pvc.Object.Spec.AccessModes) > 0 {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Access Modes:")

			for _, mode := range pvc.Object.Spec.AccessModes {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  - %s", mode)
			}
		}

		// Requested Storage
		if storage, ok := pvc.Object.Spec.Resources.Requests["storage"]; ok {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Requested Storage: %s", storage.String())
		}

		// Allocated Storage (actual)
		if storage, ok := pvc.Object.Status.Capacity["storage"]; ok {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Allocated Storage: %s", storage.String())
		}

		// Storage Class
		if pvc.Object.Spec.StorageClassName != nil && *pvc.Object.Spec.StorageClassName != "" {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Storage Class: %s", *pvc.Object.Spec.StorageClassName)
		} else {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Storage Class: <default>")
		}

		// Volume Mode
		if pvc.Object.Spec.VolumeMode != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Volume Mode: %s", *pvc.Object.Spec.VolumeMode)
		}

		// Volume Name (bound PV)
		if pvc.Object.Spec.VolumeName != "" {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Volume Name: %s", pvc.Object.Spec.VolumeName)
		} else {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Volume Name: <none> (not bound)")
		}

		// PVC Conditions
		if len(pvc.Object.Status.Conditions) > 0 {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Conditions:")

			for _, cond := range pvc.Object.Status.Conditions {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  - Type: %s, Status: %s, Reason: %s",
					cond.Type, cond.Status, cond.Reason)

				if cond.Message != "" {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    Message: %s", cond.Message)
				}

				if !cond.LastTransitionTime.IsZero() {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    Last Transition: %s",
						cond.LastTransitionTime.Format(time.RFC3339))
				}
			}
		}

		// Selector (if present)
		if pvc.Object.Spec.Selector != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Selector:")

			if len(pvc.Object.Spec.Selector.MatchLabels) > 0 {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Match Labels:")

				for key, value := range pvc.Object.Spec.Selector.MatchLabels {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    %s: %s", key, value)
				}
			}
		}
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("========================================")
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("End of PersistentVolumeClaim Status Dump for Namespace: %s", namespace)
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("========================================")
}
