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
