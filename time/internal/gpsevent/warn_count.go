package gpsevent

import (
	"context"
	"log/slog"
)

type WarnCountHandler struct {
	slog.Handler
	count int
}

func (h *WarnCountHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Level == slog.LevelWarn {
		h.count++
	}
	return h.Handler.Handle(ctx, r)
}

func (h *WarnCountHandler) WarnCount() int {
	return h.count
}

func NewWarnCountHandler(baseHandler slog.Handler) *WarnCountHandler {
	return &WarnCountHandler{
		Handler: baseHandler,
	}
}
