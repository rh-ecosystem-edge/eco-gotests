package apiservers

import (
	"context"
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	operatorV1 "github.com/openshift/api/operator/v1"
)

const (
	conditionTypeAPIServerDeploymentProgressing = "APIServerDeploymentProgressing"
	conditionReasonAsExpected                   = "AsExpected"
)

var openshiftAPIServerObjName = "cluster"

// OpenshiftAPIServerBuilder provides struct for openshiftAPIServer object.
type OpenshiftAPIServerBuilder struct {
	common.EmbeddableBuilder[operatorV1.OpenShiftAPIServer, *operatorV1.OpenShiftAPIServer]
}

// AttachMixins wires the embedded mixins to this builder instance.
func (builder *OpenshiftAPIServerBuilder) AttachMixins() {}

// GetGVK returns the OpenShiftAPIServer GVK for this builder.
func (builder *OpenshiftAPIServerBuilder) GetGVK() schema.GroupVersionKind {
	return operatorV1.GroupVersion.WithKind("OpenShiftAPIServer")
}

// PullOpenshiftAPIServer pulls existing openshiftApiServer from the cluster.
func PullOpenshiftAPIServer(apiClient *clients.Settings) (*OpenshiftAPIServerBuilder, error) {
	return common.PullClusterScopedBuilder[operatorV1.OpenShiftAPIServer, OpenshiftAPIServerBuilder](
		context.TODO(), apiClient, operatorV1.Install, openshiftAPIServerObjName)
}

// GetCondition get specific openshiftAPIServer condition and message if presented.
func (builder *OpenshiftAPIServerBuilder) GetCondition(conditionType string) (
	*operatorV1.ConditionStatus, string, error) {
	if err := common.Validate(builder); err != nil {
		return nil, "", err
	}

	klog.V(100).Infof("Get %s openshiftAPIServer %s condition", builder.Definition.Name, conditionType)

	if conditionType == "" {
		return nil, "", fmt.Errorf("openshiftAPIServer 'conditionType' cannot be empty")
	}

	if !builder.Exists() {
		return nil, "", fmt.Errorf("%s openshiftAPIServer not found", builder.Definition.Name)
	}

	openshiftAPIServer, err := builder.Get()
	if err != nil {
		return nil, "", err
	}

	for _, condition := range openshiftAPIServer.Status.Conditions {
		if condition.Type == conditionType {
			return &condition.Status, condition.Reason, nil
		}
	}

	return nil, "", fmt.Errorf("the %s openshiftAPIServer %s condition not found",
		builder.Definition.Name, conditionType)
}

// WaitUntilConditionTrue waits for timeout duration or until openshiftAPIServer gets to a specific status.
func (builder *OpenshiftAPIServerBuilder) WaitUntilConditionTrue(
	conditionType string, timeout time.Duration) error {
	if err := common.Validate(builder); err != nil {
		return err
	}

	if conditionType == "" {
		return fmt.Errorf("openshiftAPIServer 'conditionType' cannot be empty")
	}

	if !builder.Exists() {
		return fmt.Errorf("%s openshiftAPIServer not found", builder.Definition.Name)
	}

	var errMsg error

	err := wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			builder.Object, errMsg = builder.Get()
			if errMsg != nil {
				return false, nil
			}

			for _, condition := range builder.Object.Status.Conditions {
				if condition.Type == conditionType {
					if condition.Status == operatorV1.ConditionTrue {
						return true, nil
					}

					errMsg = fmt.Errorf("the %s condition did not reach True state yet", conditionType)

					return false, nil
				}
			}

			errMsg = fmt.Errorf("the %s condition not found exists", conditionType)

			return false, nil
		})
	if err != nil {
		return fmt.Errorf("%w: %w", errMsg, err)
	}

	return nil
}

// WaitAllPodsAtTheLatestGeneration waits for timeout duration or until openshiftAPIServer
// pods will reach the latest generation.
func (builder *OpenshiftAPIServerBuilder) WaitAllPodsAtTheLatestGeneration(timeout time.Duration) error {
	conditionType := conditionTypeAPIServerDeploymentProgressing
	verificationStr := conditionReasonAsExpected

	err := builder.WaitUntilConditionTrue(conditionType, timeout)
	if err != nil {
		return err
	}

	err = wait.PollUntilContextTimeout(
		context.TODO(),
		time.Second,
		timeout,
		true,
		func(ctx context.Context) (bool, error) {
			_, reasonMsg, err := builder.GetCondition(conditionType)
			if err != nil {
				return false, nil
			}

			klog.V(100).Infof("Found reason message: %s", reasonMsg)

			if reasonMsg != verificationStr {
				return false, nil
			}

			return true, nil
		})
	if err != nil {
		return err
	}

	return nil
}
