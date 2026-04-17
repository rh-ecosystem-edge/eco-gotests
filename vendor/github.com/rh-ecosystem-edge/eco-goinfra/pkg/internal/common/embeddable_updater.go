package common

import "context"

// EmbeddableUpdater is a mixin which provides the Update method to the embedding builder. The Update method does not
// force the update and will return an error if the resource could not be updated.
type EmbeddableUpdater[O any, B any, SO ObjectPointer[O], SB BuilderPointer[B, O, SO]] struct {
	base SB
}

// SetBase sets the base builder for the mixin. When the Update method is called, the common Update method will be
// called on the base builder. This base is also what gets returned by the Update method.
func (updater *EmbeddableUpdater[O, B, SO, SB]) SetBase(base SB) {
	updater.base = base
}

// Update updates the resource in the cluster. It does not force the update and will return an error if the resource
// could not be updated. It checks for the resource's existence and attempts to align resource versions to avoid
// conflict.
func (updater *EmbeddableUpdater[O, B, SO, SB]) Update() (SB, error) {
	return updater.base, Update(context.TODO(), updater.base, false)
}

// EmbeddableForceUpdater is a mixin which provides the Update method to the embedding builder. The Update method
// provides the option to force an update by deleting and recreating the resource.
type EmbeddableForceUpdater[O any, B any, SO ObjectPointer[O], SB BuilderPointer[B, O, SO]] struct {
	base SB
}

// SetBase sets the base builder for the mixin. When the Update method is called, the common Update method will be
// called on the base builder. This base is also what gets returned by the Update method.
func (updater *EmbeddableForceUpdater[O, B, SO, SB]) SetBase(base SB) {
	updater.base = base
}

// Update updates the resource in the cluster. It provides the option to force an update by deleting and recreating the
// resource. It checks for the resource's existence and attempts to align resource versions to avoid conflict.
// Regardless of the force flag, this function returns an error if the resource does not exist. When it exists, the
// resource version just pulled from the cluster is used to avoid conflicts.
func (updater *EmbeddableForceUpdater[O, B, SO, SB]) Update(force bool) (SB, error) {
	return updater.base, Update(context.TODO(), updater.base, force)
}
