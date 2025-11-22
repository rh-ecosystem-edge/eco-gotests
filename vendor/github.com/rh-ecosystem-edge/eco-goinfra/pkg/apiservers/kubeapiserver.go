package apiservers

import (
	"context"
	"fmt"
	"time"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	operatorV1 "github.com/openshift/api/operator/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// KubeAPIServerBuilder provides struct for kubeAPIServer object.
type KubeAPIServerBuilder struct {
	// KubeApiServer definition. Used to create an kubeAPIServer object.
	Definition *operatorV1.KubeAPIServer
	// Created kubeAPIServer object.
	Object *operatorV1.KubeAPIServer
	// apiClient opens api connection to the cluster.
	apiClient goclient.Client
	// Used in functions that define or mutate kubeAPIServer definition. errorMsg is processed before the
	// kubeAPIServer object is created.
	errorMsg string
}

var kubeAPIServerObjName = "cluster"

// PullKubeAPIServer pulls existing kubeApiServer from the cluster.
func PullKubeAPIServer(apiClient *clients.Settings) (*KubeAPIServerBuilder, error) {
	klog.V(100).Info("Pulling existing kubeApiServer from cluster")

	if apiClient == nil {
		klog.V(100).Info("The apiClient is empty")

		return nil, fmt.Errorf("kubeApiServer 'apiClient' cannot be empty")
	}

	builder := KubeAPIServerBuilder{
		apiClient: apiClient.Client,
		Definition: &operatorV1.KubeAPIServer{
			ObjectMeta: metav1.ObjectMeta{
				Name: kubeAPIServerObjName,
			},
		},
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("kubeAPIServer object %s does not exist", kubeAPIServerObjName)
	}

	builder.Definition = builder.Object

	return &builder, nil
}

// Exists checks whether the given kubeAPIServer exists.
func (builder *KubeAPIServerBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	var err error

	builder.Object, err = builder.Get()
	if err != nil {
		klog.V(100).Infof("Failed to collect kubeAPIServer object due to %s", err.Error())
	}

	return err == nil || !k8serrors.IsNotFound(err)
}

// Get returns KubeAPIServer object if found.
func (builder *KubeAPIServerBuilder) Get() (*operatorV1.KubeAPIServer, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	kubeAPIServer := &operatorV1.KubeAPIServer{}

	err := builder.apiClient.Get(context.TODO(), goclient.ObjectKey{
		Name: builder.Definition.Name,
	}, kubeAPIServer)
	if err != nil {
		klog.V(100).Info("kubeAPIServer object does not exist")

		return nil, err
	}

	return kubeAPIServer, err
}

// GetCondition get specific kubeAPIServer condition and message if presented.
func (builder *KubeAPIServerBuilder) GetCondition(conditionType string) (*operatorV1.ConditionStatus, string, error) {
	if valid, err := builder.validate(); !valid {
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
	if valid, err := builder.validate(); !valid {
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
					if condition.Status == "True" {
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
	conditionType := "NodeInstallerProgressing"
	verificationStr := "AllNodesAtLatestRevision"

	err := builder.WaitUntilConditionTrue(conditionType, timeout)
	if err != nil {
		return err
	}

	err = wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			var err error

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

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *KubeAPIServerBuilder) validate() (bool, error) {
	resourceCRD := "KubeAPIServer"

	if builder == nil {
		klog.V(100).Infof("The %s builder is uninitialized", resourceCRD)

		return false, fmt.Errorf("error: received nil %s builder", resourceCRD)
	}

	if builder.Definition == nil {
		klog.V(100).Infof("The %s is undefined", resourceCRD)

		return false, fmt.Errorf("%s", msg.UndefinedCrdObjectErrString(resourceCRD))
	}

	if builder.apiClient == nil {
		klog.V(100).Infof("The %s builder apiclient is nil", resourceCRD)

		return false, fmt.Errorf("%s builder cannot have nil apiClient", resourceCRD)
	}

	if builder.errorMsg != "" {
		klog.V(100).Infof("The %s builder has error message: %s", resourceCRD, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}
