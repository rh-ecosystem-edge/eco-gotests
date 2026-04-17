package common

import (
	"context"
	"fmt"
	"reflect"

	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/clients"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common/errors"
	"github.com/rh-ecosystem-edge/eco-goinfra/pkg/internal/common/key"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/klog/v2"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ObjectPointer is a type constraint that requires a type be a pointer to O and implement the runtimeclient.Object
// interface. The type parameter O is meant to be a K8s resource, such as corev1.Namespace. In that case,
// *corev1.Namespace would satisfy the constraint ObjectPointer[corev1.Namespace].
type ObjectPointer[O any] interface {
	*O
	runtimeclient.Object
}

// Builder represents the set of methods that must be present to use the common versions of CRUD and other methods.
// Since each builder struct is a different type, this interface allows common functions to update fields on the
// builder. Generally, consumers of eco-goinfra should not call these methods.
//
// The type parameter O (short for object) is expected to be the struct that represents a K8s resource, such as
// corev1.Namespace. SO (short for star O) is the pointer to O, with the additional constraint of that pointer
// implementing runtimeclient.Object. To continue the example, this would be *corev1.Namespace.
//
// Although only SO appears in the interface definition, it is important to have access to the derefenced form of the
// type so we may do new(O) and get a runtimeclient.Object.
type Builder[O any, SO ObjectPointer[O]] interface {
	// GetDefinition allows for getting the desired form of a K8s resource from the builder.
	GetDefinition() SO
	// SetDefinition allows for updating the desired form of the K8s resource.
	SetDefinition(SO)

	// GetObject allows for getting the form of a K8s resource, as last pulled from the cluster.
	GetObject() SO
	// SetObject allows for updating what the K8s resource last was on the cluster.
	SetObject(SO)

	// GetError returns the error stored in the builder. Methods which do not return errors, such as the builder
	// modifiers, will store the error in the builder.
	GetError() error
	// SetError allows for updating the error stored in the builder. It should not be used by consumers of the
	// builder, but rather by methods which do not return errors.
	SetError(error)

	// GetClient returns the client used for connecting with the K8s cluster.
	GetClient() runtimeclient.Client
	// SetClient allows for updating the client used to connect to the K8s cluster. Since this is a simple setter,
	// it will not handle updating the scheme of the client and should generally be avoided outside of creating the
	// builder.
	SetClient(runtimeclient.Client)

	// GetGVK returns the GVK for the resource the builder represents, even if the builder is zero-valued.
	//
	// It is meant to be defined by resource-specific builders as a function returning a constant GVK. An embeddable
	// builder is expected to return from GetGVK the GVK provided through SetGVK when initializing the builder.
	GetGVK() schema.GroupVersionKind
	// SetGVK allows for updating the GVK for the resource the builder represents. This method is not intended to be
	// used by consumers of the builder, but internally as part of initializing the builder.
	//
	// It is expected that an embeddable builder will store the GVK passed to this method and return it from GetGVK.
	SetGVK(schema.GroupVersionKind)
}

// NewResourceKeyFromBuilder creates a new ResourceKey from the given builder. It does not validate the builder, so
// should only be used after validating the builder, or at least ensuring the builder and its definition are not nil.
func NewResourceKeyFromBuilder[O any, SO ObjectPointer[O]](builder Builder[O, SO]) key.ResourceKey {
	return key.ResourceKey{
		Kind:      builder.GetGVK().Kind,
		Name:      builder.GetDefinition().GetName(),
		Namespace: builder.GetDefinition().GetNamespace(),
	}
}

// MixinAttacher is an interface for types which require a step during initialization of a builder to ensure mixins are
// all attached. In practice, this is meant to be implemented by the resource-specific builders so that any mixins they
// embed can be connected to the EmbeddableBuilder.
//
// This interface is defined separately from the Builder interface since while the resource-specific builders must
// implement the Builder interface, they do not need to implement the MixinAttacher interface. Similarly,
// EmbeddableBuilder should not implement this interface even though it is a Builder.
type MixinAttacher interface {
	// AttachMixins ensures that all mixins are attached to their base builders. This method will be called on the
	// zero-value of builder pointers right after allocation, provided the builder also implements MixinAttacher.
	AttachMixins()
}

// BuilderPointer is similar to objectPointer and is a constraint that is satisfied by a Builder that is a pointer. It
// exists for the same reason as objectPointer: needing access to the dereferenced form of builders to construct new
// ones.
type BuilderPointer[B, O any, SO ObjectPointer[O]] interface {
	*B
	Builder[O, SO]
}

// NewClusterScopedBuilder creates a new builder for a cluster-scoped resource. It is generic over the actual builder
// type and uses the methods from the Builder interface to create the actual builder. Generic parameters are ordered so
// that SO and SB can be elided and only O and B must be provided.
func NewClusterScopedBuilder[O, B any, SO ObjectPointer[O], SB BuilderPointer[B, O, SO]](
	apiClient runtimeclient.Client, schemeAttacher clients.SchemeAttacher, name string) SB {
	var builder SB = new(B)

	if mixinAttacher, ok := any(builder).(MixinAttacher); ok {
		mixinAttacher.AttachMixins()
	}

	builder.SetGVK(builder.GetGVK())
	builder.SetClient(apiClient)
	builder.SetDefinition(new(O))
	builder.GetDefinition().SetName(name)

	resourceKey := NewResourceKeyFromBuilder(builder)

	klog.V(100).Infof("Initializing new builder for %s", resourceKey.String())

	if isInterfaceNil(apiClient) {
		klog.V(100).Infof("The apiClient provided for %s is nil", resourceKey.String())

		builder.SetError(errors.NewAPIClientNil(resourceKey))

		return builder
	}

	err := schemeAttacher(apiClient.Scheme())
	if err != nil {
		klog.V(100).Infof("Failed to attach scheme for %s: %v", resourceKey.String(), err)

		builder.SetError(errors.NewSchemeAttacherFailed(resourceKey, err))

		return builder
	}

	if name == "" {
		klog.V(100).Infof("The name of the builder for %s is empty", resourceKey.String())

		builder.SetError(errors.NewBuilderFieldEmpty(resourceKey, errors.BuilderFieldName))

		return builder
	}

	return builder
}

// NewNamespacedBuilder creates a new builder for a namespaced resource. It is generic over the actual builder type and
// uses the methods from the Builder interface to create the actual builder. Generic parameters are ordered so that SO
// and SB can be elided and only O and B must be provided.
func NewNamespacedBuilder[O, B any, SO ObjectPointer[O], SB BuilderPointer[B, O, SO]](
	apiClient runtimeclient.Client, schemeAttacher clients.SchemeAttacher, name, nsname string) SB {
	var builder SB = new(B)

	if mixinAttacher, ok := any(builder).(MixinAttacher); ok {
		mixinAttacher.AttachMixins()
	}

	builder.SetGVK(builder.GetGVK())
	builder.SetClient(apiClient)
	builder.SetDefinition(new(O))
	builder.GetDefinition().SetName(name)
	builder.GetDefinition().SetNamespace(nsname)

	resourceKey := NewResourceKeyFromBuilder(builder)

	klog.V(100).Infof("Initializing new builder for %s", resourceKey.String())

	if isInterfaceNil(apiClient) {
		klog.V(100).Infof("The apiClient provided for %s is nil", resourceKey.String())

		builder.SetError(errors.NewAPIClientNil(resourceKey))

		return builder
	}

	err := schemeAttacher(apiClient.Scheme())
	if err != nil {
		klog.V(100).Infof("Failed to attach scheme for %s: %v", resourceKey.String(), err)

		builder.SetError(errors.NewSchemeAttacherFailed(resourceKey, err))

		return builder
	}

	if name == "" {
		klog.V(100).Infof("The name of the builder for %s is empty", resourceKey.String())

		builder.SetError(errors.NewBuilderFieldEmpty(resourceKey, errors.BuilderFieldName))

		return builder
	}

	if nsname == "" {
		klog.V(100).Infof("The namespace of the builder for %s is empty", resourceKey.String())

		builder.SetError(errors.NewBuilderFieldEmpty(resourceKey, errors.BuilderFieldNamespace))

		return builder
	}

	return builder
}

// PullClusterScopedBuilder creates a new Builder for a cluster-scoped resource, pulling the resource from the cluster.
// It is generic over the actual builder type and uses the methods from the Builder interface to create the actual
// builder. Generic parameters are ordered so that SO and SB can be elided and only O and B must be provided.
func PullClusterScopedBuilder[O, B any, SO ObjectPointer[O], SB BuilderPointer[B, O, SO]](
	ctx context.Context, apiClient runtimeclient.Client, schemeAttacher clients.SchemeAttacher, name string) (SB, error) {
	var builder SB = new(B)

	if mixinAttacher, ok := any(builder).(MixinAttacher); ok {
		mixinAttacher.AttachMixins()
	}

	builder.SetGVK(builder.GetGVK())
	builder.SetClient(apiClient)
	builder.SetDefinition(new(O))
	builder.GetDefinition().SetName(name)

	resourceKey := NewResourceKeyFromBuilder(builder)

	klog.V(100).Infof("Pulling builder for %s", resourceKey.String())

	if isInterfaceNil(apiClient) {
		klog.V(100).Infof("The apiClient provided for %s is nil", resourceKey.String())

		return nil, errors.NewAPIClientNil(resourceKey)
	}

	err := schemeAttacher(apiClient.Scheme())
	if err != nil {
		klog.V(100).Infof("Failed to attach scheme for %s: %v", resourceKey.String(), err)

		return nil, errors.NewSchemeAttacherFailed(resourceKey, err)
	}

	if name == "" {
		klog.V(100).Infof("The name of the builder for %s is empty", resourceKey.String())

		return nil, errors.NewBuilderFieldEmpty(resourceKey, errors.BuilderFieldName)
	}

	object, err := Get(ctx, builder)
	if err != nil {
		klog.V(100).Infof("Failed to pull the builder for %s: %v", resourceKey.String(), err)

		return nil, fmt.Errorf("failed to pull builder: %w", err)
	}

	builder.SetObject(object)
	builder.SetDefinition(object)

	return builder, nil
}

// PullNamespacedBuilder creates a new Builder for a namespaced resource, pulling the resource from the cluster.
// It is generic over the actual builder type and uses the methods from the Builder interface to create the actual
// builder. Generic parameters are ordered so that SO and SB can be elided and only O and B must be provided.
func PullNamespacedBuilder[O, B any, SO ObjectPointer[O], SB BuilderPointer[B, O, SO]](
	ctx context.Context, apiClient runtimeclient.Client, schemeAttacher clients.SchemeAttacher, name, nsname string) (SB, error) {
	var builder SB = new(B)

	if mixinAttacher, ok := any(builder).(MixinAttacher); ok {
		mixinAttacher.AttachMixins()
	}

	builder.SetGVK(builder.GetGVK())
	builder.SetClient(apiClient)
	builder.SetDefinition(new(O))
	builder.GetDefinition().SetName(name)
	builder.GetDefinition().SetNamespace(nsname)

	resourceKey := NewResourceKeyFromBuilder(builder)

	klog.V(100).Infof("Pulling builder for %s", resourceKey.String())

	if isInterfaceNil(apiClient) {
		klog.V(100).Infof("The apiClient provided for %s is nil", resourceKey.String())

		return nil, errors.NewAPIClientNil(resourceKey)
	}

	err := schemeAttacher(apiClient.Scheme())
	if err != nil {
		klog.V(100).Infof("Failed to attach scheme for %s: %v", resourceKey.String(), err)

		return nil, errors.NewSchemeAttacherFailed(resourceKey, err)
	}

	if name == "" {
		klog.V(100).Infof("The name of the builder for %s is empty", resourceKey.String())

		return nil, errors.NewBuilderFieldEmpty(resourceKey, errors.BuilderFieldName)
	}

	if nsname == "" {
		klog.V(100).Infof("The namespace of the builder for %s is empty", resourceKey.String())

		return nil, errors.NewBuilderFieldEmpty(resourceKey, errors.BuilderFieldNamespace)
	}

	object, err := Get(ctx, builder)
	if err != nil {
		klog.V(100).Infof("Failed to pull the builder for %s: %v", resourceKey.String(), err)

		return nil, fmt.Errorf("failed to pull builder: %w", err)
	}

	builder.SetObject(object)
	builder.SetDefinition(object)

	return builder, nil
}

// Get pulls the resource from the cluster and returns it. It does not modify the builder.
func Get[O any, SO ObjectPointer[O]](ctx context.Context, builder Builder[O, SO]) (SO, error) {
	if err := Validate(builder); err != nil {
		return nil, err
	}

	key := NewResourceKeyFromBuilder(builder)

	klog.V(100).Infof("Getting %s", key.String())

	var object SO = new(O)

	err := builder.GetClient().Get(ctx, runtimeclient.ObjectKeyFromObject(builder.GetDefinition()), object)
	if err != nil {
		return nil, errors.NewAPICallFailed("get", key, err)
	}

	return object, nil
}

// Exists checks if the resource exists in the cluster. If the resource does exist, the builder's object is updated with
// the resource and this returns true. If the resource does not exist or an error was encountered getting the resource,
// this returns false without modifying the builder.
func Exists[O any, SO ObjectPointer[O]](ctx context.Context, builder Builder[O, SO]) bool {
	if err := Validate(builder); err != nil {
		return false
	}

	key := NewResourceKeyFromBuilder(builder)

	klog.V(100).Infof("Checking if %s exists", key.String())

	object, err := Get(ctx, builder)
	if err != nil {
		klog.V(100).Infof("Failed to get %s: %v", key.String(), err)

		return false
	}

	builder.SetObject(object)

	return true
}

// Delete deletes the resource from the cluster. It immediately tries to delete the resource and if successful, or the
// resource did not exist, the builder's object is set to nil. Otherwise, the error is wrapped and returned without
// modifying the builder.
func Delete[O any, SO ObjectPointer[O]](ctx context.Context, builder Builder[O, SO]) error {
	if err := Validate(builder); err != nil {
		return err
	}

	key := NewResourceKeyFromBuilder(builder)

	klog.V(100).Infof("Deleting %s", key.String())

	err := builder.GetClient().Delete(ctx, builder.GetDefinition())
	if err == nil || k8serrors.IsNotFound(err) {
		builder.SetObject(nil)

		return nil
	}

	klog.V(100).Infof("Failed to delete %s: %v", key.String(), err)

	return errors.NewAPICallFailed("delete", key, err)
}

// Update updates the resource on the cluster using the builder's definition. It immediately tries to update the
// resource and if successful, will update the builder's object to be the definition. Otherwise, it checks to see if the
// error is because the resource did not exist, returning with an error if so. If the error is for any other reason, the
// behavior depends on the force flag.
//
// If force is true, the resource will be deleted and recreated. Otherwise, the error is wrapped and returned without
// modifying the builder. It is generally discouraged to use the force flag since finalizers may cause unexpected side
// effects and most update errors can be resolved by retrying on conflict.
func Update[O any, SO ObjectPointer[O]](ctx context.Context, builder Builder[O, SO], force bool) error {
	if err := Validate(builder); err != nil {
		return err
	}

	key := NewResourceKeyFromBuilder(builder)

	klog.V(100).Infof("Updating %s with force %t", key.String(), force)

	latestObject, err := Get(ctx, builder)
	if err != nil {
		klog.V(100).Infof("Failed to get latest object for %s: %v", key.String(), err)

		return fmt.Errorf("failed get latest object for update: %w", err)
	}

	builder.GetDefinition().SetResourceVersion(latestObject.GetResourceVersion())

	err = builder.GetClient().Update(ctx, builder.GetDefinition())
	if err == nil {
		builder.SetObject(builder.GetDefinition())

		return nil
	}

	if !force {
		klog.V(100).Infof("Failed to update %s without force: %v", key.String(), err)

		return errors.NewAPICallFailed("update", key, err)
	}

	err = Delete(ctx, builder)
	if err != nil {
		klog.V(100).Infof("Failed to delete %s during force update: %v", key.String(), err)

		return fmt.Errorf("failed to force update: %w", err)
	}

	err = Create(ctx, builder)
	if err != nil {
		klog.V(100).Infof("Failed to create %s during force update: %v", key.String(), err)

		return fmt.Errorf("failed to force update: %w", err)
	}

	return nil
}

// Create creates the definition on the cluster. If the resource already exists, this is a no-op.
func Create[O any, SO ObjectPointer[O]](ctx context.Context, builder Builder[O, SO]) error {
	if err := Validate(builder); err != nil {
		return err
	}

	key := NewResourceKeyFromBuilder(builder)

	klog.V(100).Infof("Creating %s", key.String())

	// Create requests will be rejected if the resource version is set, so we clear it.
	builder.GetDefinition().SetResourceVersion("")

	err := builder.GetClient().Create(ctx, builder.GetDefinition())
	if err == nil {
		builder.SetObject(builder.GetDefinition())

		return nil
	}

	if k8serrors.IsAlreadyExists(err) {
		klog.V(100).Infof("The resource %s already exists and cannot be created", key.String())

		return nil
	}

	klog.V(100).Infof("Failed to create %s: %v", key.String(), err)

	return errors.NewAPICallFailed("create", key, err)
}

// Validate checks that the builder is valid, that is, it is non-nil, has a non-nil definition, has a non-nil client,
// and has no error message. Additional checks are performed on any interface so that we know it is not nil and its
// concrete type is not nil.
func Validate[O any, SO ObjectPointer[O]](builder Builder[O, SO]) error {
	if isInterfaceNil(builder) {
		klog.V(100).Infof("The builder is nil")

		return errors.NewBuilderNil()
	}

	if builder.GetDefinition() == nil {
		klog.V(100).Infof("The %s builder definition is nil", builder.GetGVK().Kind)

		return errors.NewBuilderDefinitionNil(builder.GetGVK().Kind)
	}

	key := NewResourceKeyFromBuilder(builder)

	if isInterfaceNil(builder.GetClient()) {
		klog.V(100).Infof("The apiClient provided for %s is nil", key.String())

		return errors.NewAPIClientNil(key)
	}

	err := builder.GetError()
	if err != nil {
		klog.V(100).Infof("The builder for %s has an error: %v", key.String(), err)

		return fmt.Errorf("failed to validate: %w", err)
	}

	return nil
}

// ListPointer is a type constraint that requires a type be a pointer to L and implement the runtimeclient.ObjectList
// interface. The type parameter L is meant to be a K8s resource list, such as corev1.NamespaceList. In that case,
// *corev1.NamespaceList would satisfy the constraint ListPointer[corev1.NamespaceList].
//
// This is the list equivalent of [ObjectPointer].
type ListPointer[L any] interface {
	*L
	runtimeclient.ObjectList
}

// List lists the resources in the cluster and returns a list of builders for each resource.
func List[O, L, B any, SO ObjectPointer[O], SL ListPointer[L], SB BuilderPointer[B, O, SO]](
	ctx context.Context,
	apiClient runtimeclient.Client,
	schemeAttacher clients.SchemeAttacher,
	options ...runtimeclient.ListOption) ([]SB, error) {
	var dummyBuilder SB = new(B)

	resourceKey := key.NewResourceKey(dummyBuilder.GetGVK().Kind, "", "")

	if isInterfaceNil(apiClient) {
		klog.V(100).Infof("The apiClient provided for listing %s is nil", resourceKey.String())

		return nil, errors.NewAPIClientNil(resourceKey)
	}

	err := schemeAttacher(apiClient.Scheme())
	if err != nil {
		klog.V(100).Infof("Failed to attach scheme for listing %s: %v", resourceKey.String(), err)

		return nil, errors.NewSchemeAttacherFailed(resourceKey, err)
	}

	var list SL = new(L)

	err = apiClient.List(ctx, list, options...)
	if err != nil {
		klog.V(100).Infof("Failed to list %s: %v", resourceKey.String(), err)

		return nil, errors.NewAPICallFailed("list", resourceKey, err)
	}

	items, err := meta.ExtractList(list)
	if err != nil {
		klog.V(100).Infof("Failed to extract list for %s: %v", resourceKey.String(), err)

		return nil, fmt.Errorf("failed to extract list: %w", err)
	}

	var builders []SB

	for _, item := range items {
		typedItem, ok := item.(SO)
		if !ok {
			klog.V(100).Infof("Item with type %T does not match expected type %s", item, resourceKey.String())

			return nil, errors.NewItemTypeMismatch(resourceKey.Kind, reflect.TypeOf(item))
		}

		var builder SB = new(B)

		if mixinAttacher, ok := any(builder).(MixinAttacher); ok {
			mixinAttacher.AttachMixins()
		}

		builder.SetDefinition(typedItem)
		builder.SetObject(typedItem)
		builder.SetClient(apiClient)
		builder.SetGVK(builder.GetGVK())

		builders = append(builders, builder)
	}

	return builders, nil
}

// ConvertListOptionsToOptions converts a slice of runtimeclient.ListOptions to a slice of runtimeclient.ListOption.
// This is to allow for compatibility with existing functions which used ListOptions instead of ListOption.
func ConvertListOptionsToOptions(options []runtimeclient.ListOptions) []runtimeclient.ListOption {
	listOptions := make([]runtimeclient.ListOption, len(options))

	for i, option := range options {
		listOptions[i] = &option
	}

	return listOptions
}

// AdditionalOption is a constraint satisfied by any named function type whose underlying type is
// func(SB) (SB, error). This allows resource-specific option types (e.g. configmap.AdditionalOptions) to be passed
// directly to WithOptions without conversion.
type AdditionalOption[SB any] interface {
	~func(SB) (SB, error)
}

// WithOptions applies the provided functional options to the builder. If the builder is invalid, it is returned as is.
// If any option returns an error, the error is set on the builder and the builder is returned immediately without
// applying subsequent options. Nil options are skipped.
func WithOptions[O, B any, SO ObjectPointer[O], SB BuilderPointer[B, O, SO], AO AdditionalOption[SB]](
	builder SB, options ...AO) SB {
	if err := Validate(builder); err != nil {
		return builder
	}

	resourceKey := NewResourceKeyFromBuilder(builder)

	klog.V(100).Infof("Applying %d options to %s", len(options), resourceKey.String())

	for _, option := range options {
		if option == nil {
			continue
		}

		nextBuilder, err := option(builder)
		if nextBuilder != nil {
			builder = nextBuilder
		}

		if err != nil {
			klog.V(100).Infof("Error occurred while applying option to %s: %v", resourceKey.String(), err)

			builder.SetError(err)

			return builder
		}
	}

	return builder
}

// isInterfaceNil checks if the interface is nil. It checks both equality against nil and the reflect.Value.IsNil
// method. This ensures that neither the interface nor its concrete value are nil.
func isInterfaceNil(v any) bool {
	return v == nil || reflect.ValueOf(v).IsNil()
}
