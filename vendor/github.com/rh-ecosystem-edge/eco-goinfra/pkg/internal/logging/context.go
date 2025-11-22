package logging

import (
	"context"

	"github.com/go-logr/logr"
)

// DiscardContext returns a context with a logr.Discard logger. This is useful for ignoring the logging of functions
// which receive this context.
func DiscardContext() context.Context {
	return logr.NewContext(context.TODO(), logr.Discard())
}
