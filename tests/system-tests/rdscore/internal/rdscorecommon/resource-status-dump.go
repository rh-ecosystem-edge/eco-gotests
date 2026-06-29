package rdscorecommon

import (
	"context"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/deployment"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/nodes"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/pod"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/sriov"
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

// DumpPodLevelBondDeploymentDiagnostics logs comprehensive diagnostics when pod-level bond deployment
// client pod retrieval fails. This helps debug issues where pods remain unavailable after cluster
// operations (e.g., ungraceful reboot) but no artifacts are collected due to test skip.
//
// The function logs:
//   - Deployment status (replicas, conditions) if deployment exists
//   - Pod list with phases, conditions, and container states
//   - Init container statuses (often cause of Pending pods)
//   - Node assignment and scheduling information
//
// Parameters:
//   - apiClient: Kubernetes API client
//   - deploymentName: Name of the pod-level bond deployment to diagnose
//   - namespace: Namespace where the deployment should exist
//
//nolint:gocognit,funlen // Diagnostic logging requires nested loops for comprehensive pod/container status
func DumpPodLevelBondDeploymentDiagnostics(apiClient *clients.Settings, deploymentName, namespace string) {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("========================================")
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod-Level Bond Deployment Diagnostics")
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deployment: %s, Namespace: %s", deploymentName, namespace)
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("========================================")

	// Log deployment status if it exists
	deploymentObj, deployErr := deployment.Pull(
		apiClient,
		deploymentName,
		namespace)

	if deployErr == nil && deploymentObj != nil {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deployment %q exists in namespace %q", deploymentName, namespace)
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("----------------------------------------")

		desiredReplicas := int32(0)
		if deploymentObj.Object.Spec.Replicas != nil {
			desiredReplicas = *deploymentObj.Object.Spec.Replicas
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Replicas: desired=%d, ready=%d, available=%d, unavailable=%d",
			desiredReplicas,
			deploymentObj.Object.Status.ReadyReplicas,
			deploymentObj.Object.Status.AvailableReplicas,
			deploymentObj.Object.Status.UnavailableReplicas)

		// Log deployment conditions
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deployment Conditions:")

		if len(deploymentObj.Object.Status.Conditions) > 0 {
			for _, condition := range deploymentObj.Object.Status.Conditions {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  - Type: %s, Status: %s, Reason: %s",
					condition.Type, condition.Status, condition.Reason)

				if condition.Message != "" {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    Message: %s", condition.Message)
				}
			}
		} else {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  <none>")
		}
	} else {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deployment %q not found in namespace %q: %v",
			deploymentName, namespace, deployErr)
	}

	// Log pod list status if available
	podList, podListErr := pod.ListByNamePattern(
		apiClient,
		deploymentName,
		namespace)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("----------------------------------------")

	switch {
	case podListErr != nil:
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Failed to list pods matching pattern %q in namespace %q: %v",
			deploymentName, namespace, podListErr)
	case len(podList) > 0:
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Found %d pod(s) matching pattern %q in namespace %q",
			len(podList), deploymentName, namespace)

		for _, podItem := range podList {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod: %s", podItem.Definition.Name)
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Phase: %s", podItem.Object.Status.Phase)

			if podItem.Object.DeletionTimestamp != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  DeletionTimestamp: %v", podItem.Object.DeletionTimestamp)
			}

			// Log pod conditions
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Pod Conditions:")

			if len(podItem.Object.Status.Conditions) > 0 {
				for _, condition := range podItem.Object.Status.Conditions {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    - Type: %s, Status: %s, Reason: %s",
						condition.Type, condition.Status, condition.Reason)

					if condition.Message != "" {
						klog.V(rdscoreparams.RDSCoreLogLevel).Infof("      Message: %s", condition.Message)
					}
				}
			} else {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    <none>")
			}

			// Log container statuses
			if len(podItem.Object.Status.ContainerStatuses) > 0 {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Container Statuses:")

				for _, containerStatus := range podItem.Object.Status.ContainerStatuses {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    - Name: %s, Ready: %t, RestartCount: %d",
						containerStatus.Name, containerStatus.Ready, containerStatus.RestartCount)

					if containerStatus.State.Waiting != nil {
						klog.V(rdscoreparams.RDSCoreLogLevel).Infof("      State: Waiting")
						klog.V(rdscoreparams.RDSCoreLogLevel).Infof("      Reason: %s", containerStatus.State.Waiting.Reason)

						if containerStatus.State.Waiting.Message != "" {
							klog.V(rdscoreparams.RDSCoreLogLevel).Infof("      Message: %s", containerStatus.State.Waiting.Message)
						}
					}

					if containerStatus.State.Terminated != nil {
						klog.V(rdscoreparams.RDSCoreLogLevel).Infof("      State: Terminated")
						klog.V(rdscoreparams.RDSCoreLogLevel).Infof("      Reason: %s, ExitCode: %d",
							containerStatus.State.Terminated.Reason,
							containerStatus.State.Terminated.ExitCode)

						if containerStatus.State.Terminated.Message != "" {
							klog.V(rdscoreparams.RDSCoreLogLevel).Infof("      Message: %s", containerStatus.State.Terminated.Message)
						}
					}

					if containerStatus.State.Running != nil {
						klog.V(rdscoreparams.RDSCoreLogLevel).Infof("      State: Running")
					}
				}
			}

			// Log init container statuses (often the cause of Pending pods)
			if len(podItem.Object.Status.InitContainerStatuses) > 0 {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Init Container Statuses:")

				for _, initContainerStatus := range podItem.Object.Status.InitContainerStatuses {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    - Name: %s, Ready: %t, RestartCount: %d",
						initContainerStatus.Name, initContainerStatus.Ready, initContainerStatus.RestartCount)

					if initContainerStatus.State.Waiting != nil {
						klog.V(rdscoreparams.RDSCoreLogLevel).Infof("      State: Waiting")
						klog.V(rdscoreparams.RDSCoreLogLevel).Infof("      Reason: %s", initContainerStatus.State.Waiting.Reason)

						if initContainerStatus.State.Waiting.Message != "" {
							klog.V(rdscoreparams.RDSCoreLogLevel).Infof("      Message: %s", initContainerStatus.State.Waiting.Message)
						}
					}

					if initContainerStatus.State.Terminated != nil {
						klog.V(rdscoreparams.RDSCoreLogLevel).Infof("      State: Terminated")
						klog.V(rdscoreparams.RDSCoreLogLevel).Infof("      Reason: %s, ExitCode: %d",
							initContainerStatus.State.Terminated.Reason,
							initContainerStatus.State.Terminated.ExitCode)

						if initContainerStatus.State.Terminated.Message != "" {
							klog.V(rdscoreparams.RDSCoreLogLevel).Infof("      Message: %s", initContainerStatus.State.Terminated.Message)
						}
					}

					if initContainerStatus.State.Running != nil {
						klog.V(rdscoreparams.RDSCoreLogLevel).Infof("      State: Running")
					}
				}
			}

			// Log node assignment
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")

			if podItem.Object.Spec.NodeName != "" {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Scheduled on node: %s", podItem.Object.Spec.NodeName)
			} else {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Pod not yet scheduled to a node")

				if podItem.Object.Status.NominatedNodeName != "" {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Nominated Node: %s", podItem.Object.Status.NominatedNodeName)
				}
			}
		}
	default:
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("No pods found matching pattern %q in namespace %q",
			deploymentName, namespace)
	}

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("========================================")
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("End of Pod-Level Bond Deployment Diagnostics")
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("========================================")
}

// DumpSRIOVSyncDiagnostics logs comprehensive diagnostics when SRIOV sync fails on pod-level bond nodes.
// This helps debug issues where SRIOVNetworkNodeState doesn't reach Succeeded status or SR-IOV resources
// don't become available after cluster operations (reboot, configuration changes).
//
// This function is specific to pod-level bond test scenarios and references pod-level bond configuration
// from RDSCoreConfig (PodLevelBondSRIOVNetOne, PodLevelBondSRIOVNetTwo).
//
// The function logs:
//   - SRIOVNetworkNodeState status for specified nodes (sync status, last sync error)
//   - Node allocatable resources (filtered by "openshift.io/sriov" prefix for OpenShift SRIOV operator)
//   - SriovNetwork CR details (resource names, VLAN, link state)
//   - Pod-level bond deployment and pod diagnostics (via DumpPodLevelBondDeploymentDiagnostics)
//
// Parameters:
//   - apiClient: Kubernetes API client
//   - nodeNames: Names of nodes to check SRIOV sync status
//   - sriovNamespace: Namespace where SRIOV operator is running
//   - deploymentName: Name of the pod-level bond deployment to diagnose
//   - deploymentNamespace: Namespace where deployment exists
//
//nolint:gocognit,funlen // Diagnostic logging requires nested loops for comprehensive SRIOV/node/pod status
func DumpSRIOVSyncDiagnostics(
	apiClient *clients.Settings,
	nodeNames []string,
	sriovNamespace string,
	deploymentName string,
	deploymentNamespace string) {
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("========================================")
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("SRIOV Sync Diagnostics for Pod-Level Bond Nodes")
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("========================================")

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Checking SRIOV sync status on nodes: %v", nodeNames)
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("SRIOV Namespace: %s", sriovNamespace)
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Deployment: %s, Namespace: %s", deploymentName, deploymentNamespace)

	// Check SRIOVNetworkNodeState for each node
	for _, nodeName := range nodeNames {
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("----------------------------------------")
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Node: %s", nodeName)

		sriovNodeState := sriov.NewNetworkNodeStateBuilder(apiClient, nodeName, sriovNamespace)
		if sriovNodeState == nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  SRIOVNetworkNodeState: <failed to create builder>")
		} else {
			err := sriovNodeState.Discover()
			if err != nil {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  SRIOVNetworkNodeState: <not found: %v>", err)
			} else {
				// After successful Discover(), Objects field is populated with the SriovNetworkNodeState CR
				// Log sync status
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  SRIOVNetworkNodeState Sync Status: %s",
					sriovNodeState.Objects.Status.SyncStatus)

				// Log last sync error if present
				if sriovNodeState.Objects.Status.LastSyncError != "" {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Last Sync Error: %s",
						sriovNodeState.Objects.Status.LastSyncError)
				}
			}
		}

		// Log node allocatable SR-IOV resources
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  SR-IOV Allocatable Resources:")

		nodeBuilder, nodeErr := nodes.Pull(apiClient, nodeName)

		switch {
		case nodeErr != nil:
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    <failed to pull node: %v>", nodeErr)
		case nodeBuilder != nil && nodeBuilder.Object != nil:
			sriovResourceCount := 0

			// Filter for OpenShift SRIOV operator resources (openshift.io/sriov* prefix)
			for resourceName, quantity := range nodeBuilder.Object.Status.Allocatable {
				if strings.HasPrefix(string(resourceName), "openshift.io/sriov") {
					klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    - %s: %s", resourceName, quantity.String())

					sriovResourceCount++
				}
			}

			if sriovResourceCount == 0 {
				klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    <none>")
			}
		default:
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    <node object unavailable>")
		}
	}

	// Log SriovNetwork CRs for pod-level bond
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("----------------------------------------")
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("SriovNetwork CRs for Pod-Level Bond:")

	// These config values are from RDSCoreConfig - they're set during test initialization
	networkNames := []string{
		RDSCoreConfig.PodLevelBondSRIOVNetOne,
		RDSCoreConfig.PodLevelBondSRIOVNetTwo,
	}

	for _, netName := range networkNames {
		net, netErr := sriov.PullNetwork(apiClient, netName, sriovNamespace)
		if netErr != nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Network %q: <not found: %v>", netName, netErr)

			continue
		}

		if net == nil || net.Object == nil {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Network %q: <object unavailable>", netName)

			continue
		}

		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("  Network: %s", netName)
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    Resource Name: %s", net.Object.Spec.ResourceName)
		klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    Network Namespace: %s", net.Object.Spec.NetworkNamespace)

		if net.Object.Spec.Vlan != 0 {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    VLAN: %d", net.Object.Spec.Vlan)
		}

		if net.Object.Spec.LinkState != "" {
			klog.V(rdscoreparams.RDSCoreLogLevel).Infof("    Link State: %s", net.Object.Spec.LinkState)
		}
	}

	// Call existing pod-level bond deployment diagnostics for comprehensive pod/deployment status
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("----------------------------------------")
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("Pod-Level Bond Deployment Diagnostics:")
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")

	DumpPodLevelBondDeploymentDiagnostics(apiClient, deploymentName, deploymentNamespace)

	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("")
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("========================================")
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("End of SRIOV Sync Diagnostics")
	klog.V(rdscoreparams.RDSCoreLogLevel).Infof("========================================")
}
