package tsparams

import "github.com/rh-ecosystem-edge/eco-gotests/tests/system-tests/internal/systemtestsparams"

var (
	// Labels represents the range of labels that can be used for test cases selection.
	Labels = []string{systemtestsparams.Label, LabelSuite}

	// OriginalTPMMaxRetries variable to hold the original TPM max retries parameter already configure in the TPM.
	OriginalTPMMaxRetries int64
)
