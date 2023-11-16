package daemon

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"log/syslog"
	"sync"
)

type SdHandler struct {
	handler slog.Handler
	shared  *sdShared
}

type sdShared struct {
	mu  sync.Mutex
	w   io.Writer
	buf bytes.Buffer
}

func NewSdHandler(minLevel slog.Level, w io.Writer) slog.Handler {
	shared := &sdShared{w: w}
	opts := slog.HandlerOptions{
		Level: minLevel,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			switch a.Key {
			case slog.TimeKey, slog.LevelKey, slog.MessageKey:
				return slog.Attr{}
			}
			return a
		},
	}
	return &SdHandler{
		handler: slog.NewTextHandler(&shared.buf, &opts),
		shared:  shared,
	}
}

func (h *SdHandler) Enabled(c context.Context, level slog.Level) bool {
	return h.handler.Enabled(c, level)
}

func (h *SdHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &SdHandler{
		handler: h.handler.WithAttrs(attrs),
		shared:  h.shared,
	}
}

func (h *SdHandler) WithGroup(name string) slog.Handler {
	return &SdHandler{
		handler: h.handler.WithGroup(name),
		shared:  h.shared,
	}
}

func (h *SdHandler) Handle(c context.Context, r slog.Record) error {
	line := ([]byte)(fmt.Sprintf("<%d>%s", int(syslogPriority(r.Level)), r.Message))
	h.shared.mu.Lock()
	defer h.shared.mu.Unlock()
	err := h.handler.Handle(c, r)
	fields := h.shared.buf.Bytes()
	last := len(fields) - 1
	if last > 0 && fields[last] == '\n' {
		line = append(line, ' ', '{')
		line = append(line, fields[:last]...)
		line = append(line, '}', '\n')
	} else {
		line = append(line, fields...)
	}
	h.shared.buf.Reset()
	if err != nil {
		return err
	}
	_, err = h.shared.w.Write(line)
	return err
}

func syslogPriority(level slog.Level) syslog.Priority {
	switch level {
	case slog.LevelDebug:
		return syslog.LOG_DEBUG
	case slog.LevelInfo:
		return syslog.LOG_INFO
	case slog.LevelWarn:
		return syslog.LOG_WARNING
	case slog.LevelError:
		return syslog.LOG_ERR
	}
	if level < slog.LevelInfo {
		return syslog.LOG_DEBUG
	}
	if level < slog.LevelWarn {
		return syslog.LOG_NOTICE
	}
	if level < slog.LevelError {
		return syslog.LOG_ERR
	}
	return syslog.LOG_CRIT
}
