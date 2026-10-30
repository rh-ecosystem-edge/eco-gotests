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
	conditionTypeNodeInstallerProgressing   = "NodeInstallerProgressing"
	conditionReasonAllNodesAtLatestRevision = "AllNodesAtLatestRevision"
)

// KubeAPIServerBuilder provides struct for kubeAPIServer object.
type KubeAPIServerBuilder struct {
	common.EmbeddableBuilder[operatorV1.KubeAPIServer, *operatorV1.KubeAPIServer]
}

var kubeAPIServerObjName = "cluster"

// AttachMixins wires the embedded mixins to this builder instance.
func (builder *KubeAPIServerBuilder) AttachMixins() {}

// GetGVK returns the KubeAPIServer GVK for this builder.
func (builder *KubeAPIServerBuilder) GetGVK() schema.GroupVersionKind {
	return operatorV1.GroupVersion.WithKind("KubeAPIServer")
}

// PullKubeAPIServer pulls existing kubeApiServer from the cluster.
func PullKubeAPIServer(apiClient *clients.Settings) (*KubeAPIServerBuilder, error) {
	return common.PullClusterScopedBuilder[operatorV1.KubeAPIServer, KubeAPIServerBuilder](
		context.TODO(), apiClient, operatorV1.Install, kubeAPIServerObjName)
}

// GetCondition get specific kubeAPIServer condition and message if presented.
func (builder *KubeAPIServerBuilder) GetCondition(conditionType string) (*operatorV1.ConditionStatus, string, error) {
	if err := common.Validate(builder); err != nil {
		return nil, "", err
	}

	klog.V(100).Infof("Get %s kubeAPIServer %s condition", builder.Definition.Name, conditionType)

	if conditionType == "" {
		return nil, "", fmt.Errorf("kubeAPIServer 'conditionType' cannot be empty")
	}

	if !builder.Exists() {
		return nil, "", fmt.Errorf("%s kubeAPIServer not found", builder.Definition.Name)
	}

	kubeAPIServer, err := builder.Get()
	if err != nil {
		return nil, "", err
	}

	for _, condition := range kubeAPIServer.Status.Conditions {
		if condition.Type == conditionType {
			return &condition.Status, condition.Reason, nil
		}
	}

	return nil, "", fmt.Errorf("the %s kubeAPIServer %s condition not found",
		builder.Definition.Name, conditionType)
}

// WaitUntilConditionTrue waits for timeout duration or until kubeAPIServer gets to a specific status.
func (builder *KubeAPIServerBuilder) WaitUntilConditionTrue(
	conditionType string, timeout time.Duration) error {
	if err := common.Validate(builder); err != nil {
		return err
	}

	if conditionType == "" {
		return fmt.Errorf("kubeAPIServer 'conditionType' cannot be empty")
	}

	if !builder.Exists() {
		return fmt.Errorf("%s kubeAPIServer not found", builder.Definition.Name)
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

// WaitAllNodesAtTheLatestRevision waits for timeout duration or until all nodes
// will be at the latest revision.
func (builder *KubeAPIServerBuilder) WaitAllNodesAtTheLatestRevision(timeout time.Duration) error {
	conditionType := conditionTypeNodeInstallerProgressing
	verificationStr := conditionReasonAllNodesAtLatestRevision

	err := builder.WaitUntilConditionTrue(conditionType, timeout)
	if err != nil {
		return err
	}

	err = wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
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
