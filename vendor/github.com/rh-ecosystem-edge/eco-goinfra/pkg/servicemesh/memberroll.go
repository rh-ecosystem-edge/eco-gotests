package servicemesh

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/golang/glog"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/msg"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	istiov1 "maistra.io/api/core/v1"
	goclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// MemberRollBuilder provides a struct for serviceMeshMemberRoll object from the cluster and
// a serviceMeshMemberRoll definition.
type MemberRollBuilder struct {
	// serviceMeshMemberRoll definition, used to create the serviceMeshMemberRoll object.
	Definition *istiov1.ServiceMeshMemberRoll
	// Created serviceMeshMemberRoll object.
	Object *istiov1.ServiceMeshMemberRoll
	// Used in functions that define or mutate serviceMeshMemberRoll definition. errorMsg is processed
	// before the serviceMeshMemberRoll object is created.
	errorMsg string
	// api client to interact with the cluster.
	apiClient goclient.Client
}

// NewMemberRollBuilder method creates new instance of builder.
func NewMemberRollBuilder(apiClient *clients.Settings, name, nsname string) *MemberRollBuilder {
	glog.V(100).Infof("Initializing new serviceMeshMemberRollBuilder structure with the following "+
		"params: name: %s, namespace: %s", name, nsname)

	if apiClient == nil {
		glog.V(100).Infof("The apiClient cannot be nil")

		return nil
	}

	err := apiClient.AttachScheme(istiov1.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add istiov1 scheme to client schemes")

		return nil
	}

	builder := &MemberRollBuilder{
		apiClient: apiClient.Client,
		Definition: &istiov1.ServiceMeshMemberRoll{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the serviceMeshMemberRoll is empty")

		builder.errorMsg = "serviceMeshMemberRoll 'name' cannot be empty"

		return builder
	}

	if nsname == "" {
		glog.V(100).Infof("The namespace of the serviceMeshMemberRoll is empty")

		builder.errorMsg = "serviceMeshMemberRoll 'nsname' cannot be empty"

		return builder
	}

	return builder
}

// PullMemberRoll retrieves an existing serviceMeshMemberRoll object from the cluster.
func PullMemberRoll(apiClient *clients.Settings, name, nsname string) (*MemberRollBuilder, error) {
	glog.V(100).Infof(
		"Pulling serviceMeshMemberRoll object name: %s in namespace: %s", name, nsname)

	if apiClient == nil {
		glog.V(100).Infof("The apiClient is empty")

		return nil, fmt.Errorf("serviceMeshMemberRoll 'apiClient' cannot be empty")
	}

	err := apiClient.AttachScheme(istiov1.AddToScheme)
	if err != nil {
		glog.V(100).Infof("Failed to add istiov1 scheme to client schemes")

		return nil, err
	}

	builder := &MemberRollBuilder{
		apiClient: apiClient.Client,
		Definition: &istiov1.ServiceMeshMemberRoll{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: nsname,
			},
		},
	}

	if name == "" {
		glog.V(100).Infof("The name of the serviceMeshMemberRoll is empty")

		return nil, fmt.Errorf("serviceMeshMemberRoll 'name' cannot be empty")
	}

	if nsname == "" {
		glog.V(100).Infof("The namespace of the serviceMeshMemberRoll is empty")

		return nil, fmt.Errorf("serviceMeshMemberRoll 'nsname' cannot be empty")
	}

	if !builder.Exists() {
		return nil, fmt.Errorf("serviceMeshMemberRoll object %s does not exist in namespace %s", name, nsname)
	}

	builder.Definition = builder.Object

	return builder, nil
}

// Get fetches existing serviceMeshMemberRoll from cluster.
func (builder *MemberRollBuilder) Get() (*istiov1.ServiceMeshMemberRoll, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	glog.V(100).Infof("Fetching existing serviceMeshMemberRoll with name %s under namespace %s from cluster",
		builder.Definition.Name, builder.Definition.Namespace)

	servicemeshmemberroll := &istiov1.ServiceMeshMemberRoll{}

	err := builder.apiClient.Get(context.TODO(), goclient.ObjectKey{
		Name:      builder.Definition.Name,
		Namespace: builder.Definition.Namespace,
	}, servicemeshmemberroll)
	if err != nil {
		return nil, err
	}

	return servicemeshmemberroll, nil
}

// Create makes a serviceMeshMemberRoll in the cluster and stores the created object in struct.
func (builder *MemberRollBuilder) Create() (*MemberRollBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Infof("Creating the serviceMeshMemberRoll %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error
	if !builder.Exists() {
		err = builder.apiClient.Create(context.TODO(), builder.Definition)
		if err == nil {
			builder.Object = builder.Definition
		}
	}

	return builder, err
}

// Delete removes serviceMeshMemberRoll from a cluster.
func (builder *MemberRollBuilder) Delete() error {
	if valid, err := builder.validate(); !valid {
		return err
	}

	glog.V(100).Infof("Deleting the serviceMeshMemberRoll %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		glog.V(100).Infof("The serviceMeshMemberRoll %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)

		builder.Object = nil

		return nil
	}

	err := builder.apiClient.Delete(context.TODO(), builder.Definition)
	if err != nil {
		return fmt.Errorf("can not delete serviceMeshMemberRoll %s in namespace %s due to %w",
			builder.Definition.Name, builder.Definition.Namespace, err)
	}

	builder.Object = nil

	return nil
}

// Update renovates the existing serviceMeshMemberRoll object with serviceMeshMemberRoll definition in builder.
func (builder *MemberRollBuilder) Update(force bool) (*MemberRollBuilder, error) {
	if valid, err := builder.validate(); !valid {
		return builder, err
	}

	glog.V(100).Info("Updating serviceMeshMemberRoll %s in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	err := builder.apiClient.Update(context.TODO(), builder.Definition)
	if err != nil {
		if force {
			glog.V(100).Infof(
				msg.FailToUpdateNotification("serviceMeshMemberRoll", builder.Definition.Name, builder.Definition.Namespace))

			err := builder.Delete()
			if err != nil {
				glog.V(100).Infof(
					msg.FailToUpdateError("serviceMeshMemberRoll", builder.Definition.Name, builder.Definition.Namespace))

				return nil, err
			}

			return builder.Create()
		}
	}

	if err == nil {
		builder.Object = builder.Definition
	}

	return builder, err
}

// Exists checks whether the given serviceMeshMemberRoll exists.
func (builder *MemberRollBuilder) Exists() bool {
	if valid, _ := builder.validate(); !valid {
		return false
	}

	glog.V(100).Infof("Checking if serviceMeshMemberRoll %s exists in namespace %s",
		builder.Definition.Name, builder.Definition.Namespace)

	var err error

	builder.Object, err = builder.Get()

	return err == nil || !k8serrors.IsNotFound(err)
}

// WithMembersList adds member list section to the MemberRollBuilder.
func (builder *MemberRollBuilder) WithMembersList(membersList []string) *MemberRollBuilder {
	glog.V(100).Infof("Adding member list %v section to the MemberRollBuilder", membersList)

	if len(membersList) == 0 {
		glog.V(100).Infof("Cannot add empty membersList to the memberRoll structure")

		builder.errorMsg = "can not modify memberRoll config with empty membersList"

		return builder
	}

	if builder.Definition.Spec.Members == nil {
		builder.Definition.Spec.Members = membersList
	} else {
		builder.Definition.Spec.Members = append(builder.Definition.Spec.Members, membersList...)
	}

	return builder
}

// GetMembersList fetches memberRoll's membersList.
func (builder *MemberRollBuilder) GetMembersList() (*[]string, error) {
	if valid, err := builder.validate(); !valid {
		return nil, err
	}

	glog.V(100).Infof("Getting memberRoll %s in namespace %s membersList configuration",
		builder.Definition.Name, builder.Definition.Namespace)

	if !builder.Exists() {
		return nil, fmt.Errorf("memberRoll object %s does not exist in namespace %s",
			builder.Definition.Name, builder.Definition.Namespace)
	}

	return &builder.Object.Spec.Members, nil
}

// IsReady check if the ServiceMesh MemberRoll is Ready.
func (builder *MemberRollBuilder) IsReady(timeout time.Duration) (bool, error) {
	if valid, err := builder.validate(); !valid {
		return false, err
	}

	err := wait.PollUntilContextTimeout(
		context.TODO(), time.Second, timeout, true, func(ctx context.Context) (bool, error) {
			if !builder.Exists() {
				return false, nil
			}

			for _, condition := range builder.Object.Status.Conditions {
				if condition.Type == istiov1.ConditionTypeMemberRollReady {
					if condition.Status == corev1.ConditionTrue {
						return true, nil
					}
				}
			}

			return false, nil
		})
	if err != nil {
		return false, fmt.Errorf("the Ready condition did not reached for the Service Mesh MemberRoll %s in "+
			"namespace %s during %v; %v", builder.Definition.Name, builder.Definition.Namespace, timeout, err)
	}

	return true, nil
}

// validate will check that the builder and builder definition are properly initialized before
// accessing any member fields.
func (builder *MemberRollBuilder) validate() (bool, error) {
	resourceCRD := "ServiceMeshMemberRoll"

	if builder == nil {
		glog.V(100).Infof("The %s builder is uninitialized", resourceCRD)

		return false, fmt.Errorf("error: received nil %s builder", resourceCRD)
	}

	if builder.Definition == nil {
		glog.V(100).Infof("The %s is undefined", resourceCRD)

		return false, fmt.Errorf("%s", msg.UndefinedCrdObjectErrString(resourceCRD))
	}

	if builder.apiClient == nil {
		glog.V(100).Infof("The %s builder apiclient is nil", resourceCRD)

		return false, fmt.Errorf("%s builder cannot have nil apiClient", resourceCRD)
	}

	if builder.errorMsg != "" {
		glog.V(100).Infof("The %s builder has error message: %s", resourceCRD, builder.errorMsg)

		return false, fmt.Errorf("%s", builder.errorMsg)
	}

	return true, nil
}
