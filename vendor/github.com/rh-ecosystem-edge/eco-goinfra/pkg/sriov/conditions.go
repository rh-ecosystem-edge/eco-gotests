package sriov

import (
	"context"
	"strings"
	"time"

	srIovV1 "github.com/k8snetworkplumbingwg/sriov-network-operator/api/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

type conditionGetter func(ctx context.Context) ([]metav1.Condition, int64, error)

func waitForCondition(
	kind, namespace, name string, expected metav1.Condition, timeout time.Duration, get conditionGetter,
) error {
	klog.V(100).Infof("Waiting up to %s until %s %s/%s has condition %v",
		timeout, kind, namespace, name, expected)

	return wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			conditions, generation, err := get(ctx)
			if err != nil {
				klog.V(100).Infof("Failed to get %s %s/%s: %v", kind, namespace, name, err)

				return false, nil
			}

			return conditionMatches(conditions, generation, expected), nil
		})
}

func conditionMatches(conditions []metav1.Condition, generation int64, expected metav1.Condition) bool {
	for _, condition := range conditions {
		if !isConditionGenerationCurrent(generation, &condition) {
			continue
		}

		if expected.Type != "" && condition.Type != expected.Type {
			continue
		}

		if expected.Status != "" && condition.Status != expected.Status {
			continue
		}

		if expected.Reason != "" && condition.Reason != expected.Reason {
			continue
		}

		if expected.Message != "" && !strings.Contains(condition.Message, expected.Message) {
			continue
		}

		if expected.ObservedGeneration != 0 &&
			condition.ObservedGeneration != expected.ObservedGeneration {
			continue
		}

		return true
	}

	return false
}

func isConditionGenerationCurrent(generation int64, condition *metav1.Condition) bool {
	if condition == nil {
		return false
	}

	return condition.ObservedGeneration == generation
}

func isReadyConditionTrue(conditions []metav1.Condition, generation int64) bool {
	ready := meta.FindStatusCondition(conditions, srIovV1.ConditionReady)

	return isConditionGenerationCurrent(generation, ready) && ready.Status == metav1.ConditionTrue
}
