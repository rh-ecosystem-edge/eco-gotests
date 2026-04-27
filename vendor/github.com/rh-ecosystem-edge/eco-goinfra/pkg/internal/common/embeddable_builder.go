package common

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime/schema"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// EmbeddableBuilder is a struct implementing the Builder interface that can be embedded in existing builder structs. It
// provides the basic fields and methods for CRUD functions that builders need.
type EmbeddableBuilder[O any, SO ObjectPointer[O]] struct {
	// Definition is the desired form of the resource.
	Definition SO
	// Object is the last pulled form of the resource.
	Object SO
	// err is the error stored in the builder.
	err error
	// apiClient is the client used for connecting with the K8s cluster.
	apiClient runtimeclient.Client
	// gvk is the GVK of the resource the builder represents. It is set by [SetGVK] and then returned by all
	// subsequent [GetGVK] calls.
	gvk schema.GroupVersionKind
}

// GetDefinition returns the desired form of the resource. This method returns a pointer to the definition, which can be
// modified directly.
func (b *EmbeddableBuilder[O, SO]) GetDefinition() SO {
	return b.Definition
}

// SetDefinition updates the desired form of the resource. In general, end users would want to use either the builder
// modifiers or make changes to the definition returned from [GetDefinition].
func (b *EmbeddableBuilder[O, SO]) SetDefinition(definition SO) {
	b.Definition = definition
}

// GetObject returns the last pulled form of the resource.
func (b *EmbeddableBuilder[O, SO]) GetObject() SO {
	return b.Object
}

// SetObject updates the last pulled form of the resource. End users should not call this method directly since the
// object is automatically updated when the resource is pulled from the cluster.
func (b *EmbeddableBuilder[O, SO]) SetObject(object SO) {
	b.Object = object
}

// GetError returns the error stored in the builder. End users should not call this method directly since the error is
// returned during validation.
func (b *EmbeddableBuilder[O, SO]) GetError() error {
	return b.err
}

// SetError updates the error stored in the builder. End users should not call this method directly since the error is
// automatically set by the builder modifiers.
func (b *EmbeddableBuilder[O, SO]) SetError(err error) {
	b.err = err
}

// GetClient returns the client used for connecting with the K8s cluster.
func (b *EmbeddableBuilder[O, SO]) GetClient() runtimeclient.Client {
	return b.apiClient
}

// SetClient updates the client used for connecting with the K8s cluster. End users should not call this method directly
// since the client is automatically set when the builder is created.
func (b *EmbeddableBuilder[O, SO]) SetClient(apiClient runtimeclient.Client) {
	b.apiClient = apiClient
}

// GetGVK returns the GVK for the resource the builder represents, even if the builder is zero-valued. However,
// embedders should override this method to return the proper GVK for the embedding builder.
//
// During builder initialization, the [SetGVK] method is called to set the GVK for the builder. This method returns the
// value provided through [SetGVK].
func (b *EmbeddableBuilder[O, SO]) GetGVK() schema.GroupVersionKind {
	return b.gvk
}

// SetGVK updates the GVK for the resource the builder represents. Embedders should not override this method since it
// will be called when initializing the builder to ensure that [GetGVK] returns the proper GVK.
func (b *EmbeddableBuilder[O, SO]) SetGVK(gvk schema.GroupVersionKind) {
	b.gvk = gvk
}

// Get pulls the resource from the cluster and returns it. It does not modify the builder.
func (b *EmbeddableBuilder[O, SO]) Get() (SO, error) {
	return Get(context.TODO(), b)
}

// Exists checks whether the resource exists on the cluster. If the resource does exist, the builder's object is updated
// with the resource and this returns true. If the builder is invalid, or the resource cannot be retrieved, this returns
// false without modifying the builder.
func (b *EmbeddableBuilder[O, SO]) Exists() bool {
	return Exists(context.TODO(), b)
}
