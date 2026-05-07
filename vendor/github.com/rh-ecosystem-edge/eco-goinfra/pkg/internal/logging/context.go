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

// WithDiscardLogger returns a copy of ctx with a logr.Discard logger attached, suppressing verbose logging from
// functions that extract a logger from the context.
func WithDiscardLogger(ctx context.Context) context.Context {
	return logr.NewContext(ctx, logr.Discard())
}
