package logctx

import (
	"context"

	"golang.org/x/exp/slog"
)

type contextKey struct{}

// NewContext returns a context that contains the given Logger.
// Use FromContext to retrieve the Logger.
func NewContext(ctx context.Context, lg *slog.Logger) context.Context {
	return context.WithValue(ctx, contextKey{}, lg)
}

// FromContext returns the Logger stored in ctx by NewContext, or the default
// Logger if there is none.
func FromContext(ctx context.Context) *slog.Logger {
	if lg, ok := ctx.Value(contextKey{}).(*slog.Logger); ok {
		return lg
	}
	return slog.Default()
}
