package api

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	provisioningv1alpha1 "github.com/openshift-kni/oran-o2ims/api/provisioning/v1alpha1"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/oran/api/internal/provisioning"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	provisioningRequestKind = "ProvisioningRequest"
)

// ProvisioningClient allows interaction with the O2IMS Infrastructure Provisioning API.
type ProvisioningClient struct {
	provisioning.ClientWithResponsesInterface
}

// Enforce at compile time that ProvisioningClient implements runtimeclient.Client.
var _ runtimeclient.Client = (*ProvisioningClient)(nil)

// Get retrieves a ProvisioningRequest object from the O2IMS API. It uses the name of the object as the UUID of the
// ProvisioningRequest. The object must be a pointer to a ProvisioningRequest object. It will be updated based on the
// object returned from the API. Get options will be ignored.
func (client *ProvisioningClient) Get(
	ctx context.Context, key runtimeclient.ObjectKey, obj runtimeclient.Object, _ ...runtimeclient.GetOption) error {
	provisioningRequest, ok := obj.(*provisioningv1alpha1.ProvisioningRequest)
	if !ok {
		return fmt.Errorf(
			"failed to get ProvisioningRequest %s: object must be pointer to ProvisioningRequest, not %T", key.Name, obj)
	}

	id, err := uuid.Parse(key.Name)
	if err != nil {
		return fmt.Errorf("failed to get ProvisioningRequest %s: name is could not be parsed as uuid: %w", key.Name, err)
	}

	resp, err := client.GetProvisioningRequestWithResponse(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to get ProvisioningRequest %s: error contacting api: %w", key.Name, err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return fmt.Errorf(
			"failed to get ProvisioningRequest %s: received error from api: %w", key.Name, apiErrorFromResponse(resp))
	}

	tempPr, err := provisioningRequestFromInfo(*resp.JSON200)
	if err != nil {
		return err
	}

	*provisioningRequest = tempPr

	return nil
}

// ProvisioningRequestListOptions is a wrapper around the GetProvisioningRequestsParams struct from the O2IMS API. It
// allows providing the GetProvisioningRequestsParams struct as a runtimeclient.ListOption to List.
type ProvisioningRequestListOptions provisioning.GetProvisioningRequestsParams

// Enforce at compile time that ProvisioningRequestListOptions implements runtimeclient.ListOption.
var _ runtimeclient.ListOption = (*ProvisioningRequestListOptions)(nil)

// ApplyToList is a no-op that is required to satisfy the runtimeclient.ListOption interface.
func (opts *ProvisioningRequestListOptions) ApplyToList(*runtimeclient.ListOptions) {}

// List retrieves a list of ProvisioningRequest objects from the O2IMS API. The list must be of type
// ProvisioningRequestList and will be updated from the API. Options may be provided to filter the returned list, but
// they must be of type ProvisioningRequestListOptions, or an error will be returned. Only the first option is
// considered.
func (client *ProvisioningClient) List(
	ctx context.Context, list runtimeclient.ObjectList, opts ...runtimeclient.ListOption) error {
	provisioningRequestList, ok := list.(*provisioningv1alpha1.ProvisioningRequestList)
	if !ok {
		return fmt.Errorf(
			"failed to list ProvisioningRequests: object must be pointer to ProvisioningRequestList, not %T", list)
	}

	var params *provisioning.GetProvisioningRequestsParams

	if len(opts) > 0 {
		listOpts, ok := opts[0].(*ProvisioningRequestListOptions)
		if !ok {
			return fmt.Errorf(
				"failed to list ProvisioningRequests: options must be pointer to ProvisioningRequestListOptions, not %T", opts[0])
		}

		params = (*provisioning.GetProvisioningRequestsParams)(listOpts)
	}

	resp, err := client.GetProvisioningRequestsWithResponse(ctx, params)
	if err != nil {
		return fmt.Errorf("failed to list ProvisioningRequests: error contacting api: %w", err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return fmt.Errorf("failed to list ProvisioningRequests: received error from api: %w", apiErrorFromResponse(resp))
	}

	for _, info := range *resp.JSON200 {
		tempPr, err := provisioningRequestFromInfo(info)
		if err != nil {
			return err
		}

		provisioningRequestList.Items = append(provisioningRequestList.Items, tempPr)
	}

	return nil
}

// Create creates a ProvisioningRequest object in the O2IMS API. The object must be a pointer to a ProvisioningRequest
// object. The object is updated with the ProvisioningRequest object returned by the O2IMS API. Create options will be
// ignored.
func (client *ProvisioningClient) Create(
	ctx context.Context, obj runtimeclient.Object, _ ...runtimeclient.CreateOption) error {
	provisioningRequest, ok := obj.(*provisioningv1alpha1.ProvisioningRequest)
	if !ok {
		return fmt.Errorf("failed to create ProvisioningRequest: object must be pointer to ProvisioningRequest, not %T", obj)
	}

	data, err := dataFromProvisioningRequest(provisioningRequest)
	if err != nil {
		return fmt.Errorf("failed to create ProvisioningRequest %s: %w", provisioningRequest.Name, err)
	}

	resp, err := client.CreateProvisioningRequestWithResponse(ctx, data)
	if err != nil {
		return fmt.Errorf("failed to create ProvisioningRequest %s: error contacting api: %w", provisioningRequest.Name, err)
	}

	if resp.StatusCode() != 201 || resp.JSON201 == nil {
		return fmt.Errorf("failed to create ProvisioningRequest %s: received error from api: %w",
			provisioningRequest.Name, apiErrorFromResponse(resp))
	}

	tempPr, err := provisioningRequestFromInfo(*resp.JSON201)
	if err != nil {
		return fmt.Errorf("failed to create ProvisioningRequest %s: %w", provisioningRequest.Name, err)
	}

	*provisioningRequest = tempPr

	return nil
}

// Delete removes a ProvisioningRequest object from the O2IMS API. The object must be a pointer to a ProvisioningRequest
// object. Delete options will be ignored. The provided object will not be modified.
func (client *ProvisioningClient) Delete(
	ctx context.Context, obj runtimeclient.Object, _ ...runtimeclient.DeleteOption) error {
	provisioningRequest, ok := obj.(*provisioningv1alpha1.ProvisioningRequest)
	if !ok {
		return fmt.Errorf("failed to delete ProvisioningRequest: object must be pointer to ProvisioningRequest, not %T", obj)
	}

	prID, err := uuid.Parse(provisioningRequest.Name)
	if err != nil {
		return fmt.Errorf(
			"failed to delete ProvisioningRequest %s: name is could not be parsed as uuid: %w", provisioningRequest.Name, err)
	}

	resp, err := client.DeleteProvisioningRequestWithResponse(ctx, prID)
	if err != nil {
		return fmt.Errorf("failed to delete ProvisioningRequest %s: error contacting api: %w", provisioningRequest.Name, err)
	}

	if resp.StatusCode() != 200 {
		return fmt.Errorf("failed to delete ProvisioningRequest %s: received error from api: %w",
			provisioningRequest.Name, apiErrorFromResponse(resp))
	}

	return nil
}

// Update modifies a ProvisioningRequest object in the O2IMS API. The object must be a pointer to a ProvisioningRequest
// object. The object will not be modified. Update options will be ignored.
func (client *ProvisioningClient) Update(
	ctx context.Context, obj runtimeclient.Object, _ ...runtimeclient.UpdateOption) error {
	provisioningRequest, ok := obj.(*provisioningv1alpha1.ProvisioningRequest)
	if !ok {
		return fmt.Errorf("failed to update ProvisioningRequest: object must be pointer to ProvisioningRequest, not %T", obj)
	}

	data, err := dataFromProvisioningRequest(provisioningRequest)
	if err != nil {
		return fmt.Errorf("failed to update ProvisioningRequest %s: %w", provisioningRequest.Name, err)
	}

	resp, err := client.UpdateProvisioningRequestWithResponse(ctx, data.ProvisioningRequestId, data)
	if err != nil {
		return fmt.Errorf("failed to update ProvisioningRequest %s: error contacting api: %w", provisioningRequest.Name, err)
	}

	if resp.StatusCode() != 200 || resp.JSON200 == nil {
		return fmt.Errorf("failed to update ProvisioningRequest %s: received error from api: %w",
			provisioningRequest.Name, apiErrorFromResponse(resp))
	}

	tempPr, err := provisioningRequestFromInfo(*resp.JSON200)
	if err != nil {
		return fmt.Errorf("failed to update ProvisioningRequest %s: %w", provisioningRequest.Name, err)
	}

	*provisioningRequest = tempPr

	return nil
}

// Patch always returns an unimplementedError since the ProvisioningRequest cannot be patched via the O2IMS API.
func (client *ProvisioningClient) Patch(
	_ context.Context, _ runtimeclient.Object, _ runtimeclient.Patch, _ ...runtimeclient.PatchOption) error {
	return &unimplementedError{
		clientType: ProvisioningClientType,
		method:     "Patch",
	}
}

// DeleteAllOf always returns an unimplementedError since ProvisioningRequests cannot be deleted in bulk via the O2IMS
// API.
func (client *ProvisioningClient) DeleteAllOf(
	_ context.Context, _ runtimeclient.Object, _ ...runtimeclient.DeleteAllOfOption) error {
	return &unimplementedError{
		clientType: ProvisioningClientType,
		method:     "DeleteAllOf",
	}
}

// Status always returns nil since the ProvisioningRequest cannot have its status updated via the O2IMS API.
//
//nolint:ireturn // forced to return interface to satisfy runtimeclient.Client interface
func (client *ProvisioningClient) Status() runtimeclient.SubResourceWriter {
	return nil
}

// SubResource always returns nil since the ProvisioningRequest has no subresources that can be updated via the O2IMS
// API.
//
//nolint:ireturn // forced to return interface to satisfy runtimeclient.Client interface
func (client *ProvisioningClient) SubResource(_ string) runtimeclient.SubResourceClient {
	return nil
}

// Scheme always returns a new Scheme since the ProvisioningClient does not use a scheme.
func (client *ProvisioningClient) Scheme() *runtime.Scheme {
	return runtime.NewScheme()
}

// RESTMapper always returns nil since the ProvisioningClient does not use a REST mapper.
//
//nolint:ireturn // forced to return interface to satisfy runtimeclient.Client interface
func (client *ProvisioningClient) RESTMapper() meta.RESTMapper {
	return nil
}

// GroupVersionKindFor returns the GroupVersionKind for the ProvisioningRequest object, if the object is of that type.
// Otherwise, it returns the unimplementedError.
func (client *ProvisioningClient) GroupVersionKindFor(obj runtime.Object) (schema.GroupVersionKind, error) {
	if _, ok := obj.(*provisioningv1alpha1.ProvisioningRequest); ok {
		return provisioningv1alpha1.GroupVersion.WithKind(provisioningRequestKind), nil
	}

	return schema.GroupVersionKind{}, &unimplementedError{
		clientType: ProvisioningClientType,
		method:     "GroupVersionKindFor",
	}
}

// IsObjectNamespaced returns false if the object is a ProvisioningRequest, since it is not namespaced. Otherwise, it
// returns an unimplementedError.
func (client *ProvisioningClient) IsObjectNamespaced(obj runtime.Object) (bool, error) {
	if _, ok := obj.(*provisioningv1alpha1.ProvisioningRequest); ok {
		return false, nil
	}

	return false, &unimplementedError{
		clientType: ProvisioningClientType,
		method:     "IsObjectNamespaced",
	}
}

// dataFromProvisioningRequest converts the ProvisioningRequest object from the Kubernetes API to the
// ProvisioningRequestData sent to the O2IMS API.
func dataFromProvisioningRequest(
	provisioningRequest *provisioningv1alpha1.ProvisioningRequest) (provisioning.ProvisioningRequestData, error) {
	prID, err := uuid.Parse(provisioningRequest.Name)
	if err != nil {
		return provisioning.ProvisioningRequestData{}, fmt.Errorf("failed to parse ProvisioningRequest name as UUID: %w", err)
	}

	params := make(map[string]any)

	err = json.Unmarshal(provisioningRequest.Spec.TemplateParameters.Raw, &params)
	if err != nil {
		return provisioning.ProvisioningRequestData{}, fmt.Errorf("failed to unmarshal TemplateParameters: %w", err)
	}

	return provisioning.ProvisioningRequestData{
		Name:                  provisioningRequest.Spec.Name,
		Description:           provisioningRequest.Spec.Description,
		ProvisioningRequestId: prID,
		TemplateName:          provisioningRequest.Spec.TemplateName,
		TemplateParameters:    params,
		TemplateVersion:       provisioningRequest.Spec.TemplateVersion,
	}, nil
}

// provisioningRequestTypeMeta is the TypeMeta for the ProvisioningRequest object. It is attached to all the
// ProvisioningRequests returned by provisioningRequestFromInfo.
var provisioningRequestTypeMeta = metav1.TypeMeta{
	Kind:       provisioningRequestKind,
	APIVersion: provisioningv1alpha1.GroupVersion.String(),
}

// provisioningRequestFromInfo converts the ProvisioningRequestInfo returned by the O2IMS API to the ProvisioningRequest
// object used by the Kubernetes API.
func provisioningRequestFromInfo(
	info provisioning.ProvisioningRequestInfo) (provisioningv1alpha1.ProvisioningRequest, error) {
	jsonTemplateParameters, err := json.Marshal(info.ProvisioningRequestData.TemplateParameters)
	if err != nil {
		return provisioningv1alpha1.ProvisioningRequest{}, fmt.Errorf(
			"could not convert info to ProvisioningRequest: failed to marshal TemplateParameters: %w", err)
	}

	return provisioningv1alpha1.ProvisioningRequest{
		TypeMeta: provisioningRequestTypeMeta,
		ObjectMeta: metav1.ObjectMeta{
			Name: info.ProvisioningRequestData.ProvisioningRequestId.String(),
		},
		Spec: provisioningv1alpha1.ProvisioningRequestSpec{
			Name:               info.ProvisioningRequestData.Name,
			Description:        info.ProvisioningRequestData.Description,
			TemplateName:       info.ProvisioningRequestData.TemplateName,
			TemplateVersion:    info.ProvisioningRequestData.TemplateVersion,
			TemplateParameters: runtime.RawExtension{Raw: jsonTemplateParameters},
		},
		Status: provisioningv1alpha1.ProvisioningRequestStatus{
			ProvisioningStatus: provisioningv1alpha1.ProvisioningStatus{
				ProvisioningPhase:   provisioningv1alpha1.ProvisioningPhase(unwrapOrDefault(info.Status.ProvisioningPhase)),
				ProvisioningDetails: unwrapOrDefault(info.Status.Message),
				ProvisionedResources: &provisioningv1alpha1.ProvisionedResources{
					OCloudNodeClusterId: unwrapOrDefault(info.ProvisionedResourceSets.NodeClusterId).String(),
				},
				UpdateTime: metav1.Time{Time: unwrapOrDefault(info.Status.UpdateTime)},
			},
		},
	}, nil
}

// unwrapOrDefault functions the same as Option::unwrap_or_default in Rust. If T is nil, it returns the zero value of T.
// Otherwise, it returns a dereferenced T.
//
//nolint:ireturn // T is expected to be a concrete type, so the return should not be an interface.
func unwrapOrDefault[T any](value *T) T {
	if value == nil {
		return *new(T)
	}

	return *value
}
