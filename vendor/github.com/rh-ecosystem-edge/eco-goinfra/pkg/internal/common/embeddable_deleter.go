package common

import "context"

// EmbeddableDeleter is a mixin which provides the Delete method to the embedding builder.
type EmbeddableDeleter[O any, SO ObjectPointer[O]] struct {
	base Builder[O, SO]
}

// SetBase sets the base builder for the mixin. When the Delete method is called, the common Delete method will be
// called on the base builder. In practice, this can be either the EmbeddableBuilder or the resource-specific builder.
func (deleter *EmbeddableDeleter[O, SO]) SetBase(base Builder[O, SO]) {
	deleter.base = base
}

// Delete deletes the resource from the cluster. It immediately tries to delete the resource and if successful, or the
// resource did not exist, the builder's object is set to nil. Otherwise, the error is wrapped and returned without
// modifying the builder.
func (deleter *EmbeddableDeleter[O, SO]) Delete() error {
	return Delete(context.TODO(), deleter.base)
}

// EmbeddableDeleteReturner is a mixin which provides the Delete method to the embedding builder. The Delete method
// returns the builder and the error from the Delete method. To maintain compatibility with existing Delete methods
// which return the builder, this struct has more complicated type parameters than the EmbeddableDeleter.
//
// Consumers of this mixin should set the base to the embedding builder rather than the EmbeddableBuilder so that Delete
// returns the correct type.
type EmbeddableDeleteReturner[O any, B any, SO ObjectPointer[O], SB BuilderPointer[B, O, SO]] struct {
	base SB
}

// SetBase sets the base builder for the mixin. When the Delete method is called, the common Delete method will be
// called on the base builder. For EmbeddableDeleteReturner, the base should be the resource-specific builder rather
// than EmbeddableBuilder.
func (deleter *EmbeddableDeleteReturner[O, B, SO, SB]) SetBase(base SB) {
	deleter.base = base
}

// Delete deletes the resource from the cluster. It immediately tries to delete the resource and if successful, or the
// resource did not exist, the builder's object is set to nil. Otherwise, the error is wrapped and returned without
// modifying the builder. Regardless of the error, the builder is returned.
func (deleter *EmbeddableDeleteReturner[O, B, SO, SB]) Delete() (SB, error) {
	return deleter.base, Delete(context.TODO(), deleter.base)
}
