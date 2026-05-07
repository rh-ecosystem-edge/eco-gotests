package common

import "context"

// EmbeddableCreator is a mixin which provides the Create method to the embedding builder. The Create method immediately
// creates the resource in the cluster and returns the builder and the error from the Create method.
type EmbeddableCreator[O any, B any, SO ObjectPointer[O], SB BuilderPointer[B, O, SO]] struct {
	base SB
}

// SetBase sets the base builder for the mixin. When the Create method is called, the common Create method will be
// called on the base builder. This base is also what gets returned by the Create method.
func (creator *EmbeddableCreator[O, B, SO, SB]) SetBase(base SB) {
	creator.base = base
}

// Create creates the resource in the cluster. It first checks if the resource already exists and if so, does nothing.
// Otherwise, it tries to create the resource and returns the builder and the error from the Create method.
func (creator *EmbeddableCreator[O, B, SO, SB]) Create() (SB, error) {
	return creator.base, Create(context.TODO(), creator.base)
}
