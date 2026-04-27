package common

// EmbeddableWithOptions is a mixin which provides the WithOptions method to the embedding builder. It allows applying
// functional options to the builder in a chainable manner. The AO type parameter accepts named option types (e.g.
// configmap.AdditionalOptions) that satisfy the AdditionalOption constraint.
type EmbeddableWithOptions[O, B any, SO ObjectPointer[O], SB BuilderPointer[B, O, SO], AO AdditionalOption[SB]] struct {
	base SB
}

// SetBase sets the base builder for the mixin. When the WithOptions method is called, the common WithOptions function
// will be called on the base builder. For EmbeddableWithOptions, the base should be the resource-specific builder
// rather than EmbeddableBuilder so that WithOptions returns the correct type.
func (mixin *EmbeddableWithOptions[O, B, SO, SB, AO]) SetBase(base SB) {
	mixin.base = base
}

// WithOptions applies the provided functional options to the builder. If the builder is invalid, it is returned as is.
// If any option returns an error, the error is set on the builder and the builder is returned immediately without
// applying subsequent options. Nil options are skipped.
func (mixin *EmbeddableWithOptions[O, B, SO, SB, AO]) WithOptions(options ...AO) SB {
	return WithOptions[O, B, SO, SB, AO](mixin.base, options...)
}
