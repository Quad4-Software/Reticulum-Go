// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2024-2026 Quad4.io

package debug

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"sync"

	"quad4/reticulum-go/pkg/term"
)

// colorState is shared across WithAttrs/WithGroup clones.
type colorState struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	w    io.Writer
	file *os.File
}

// colorHandler wraps a text handler and injects ANSI around the level token.
// slog.TextHandler quotes strings that contain ESC, so ReplaceAttr cannot be used.
type colorHandler struct {
	inner slog.Handler
	state *colorState
}

func newColorHandler(w io.Writer, opts *slog.HandlerOptions) *colorHandler {
	st := &colorState{w: w, file: os.Stderr}
	return &colorHandler{
		inner: slog.NewTextHandler(&st.buf, opts),
		state: st,
	}
}

func (h *colorHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *colorHandler) Handle(ctx context.Context, r slog.Record) error {
	h.state.mu.Lock()
	defer h.state.mu.Unlock()
	h.state.buf.Reset()
	if err := h.inner.Handle(ctx, r); err != nil {
		return err
	}
	line := colorizeLevelBytes(h.state.buf.Bytes(), r.Level, h.state.file)
	_, err := h.state.w.Write(line)
	return err
}

func (h *colorHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &colorHandler{inner: h.inner.WithAttrs(attrs), state: h.state}
}

func (h *colorHandler) WithGroup(name string) slog.Handler {
	return &colorHandler{inner: h.inner.WithGroup(name), state: h.state}
}

func colorizeLevelBytes(line []byte, level slog.Level, f *os.File) []byte {
	plain := level.String()
	token := append([]byte("level="), plain...)
	idx := bytes.Index(line, token)
	if idx < 0 {
		return line
	}
	colored := colorLevelLabel(f, level, plain)
	out := make([]byte, 0, len(line)+len(colored)-len(plain))
	out = append(out, line[:idx+len("level=")]...)
	out = append(out, colored...)
	out = append(out, line[idx+len(token):]...)
	return out
}

func colorLevelLabel(f *os.File, level slog.Level, label string) string {
	switch {
	case level >= slog.LevelError:
		return term.Red(f, label)
	case level >= slog.LevelWarn:
		return term.Yellow(f, label)
	case level >= slog.LevelInfo:
		return term.Cyan(f, label)
	default:
		return term.Bold(f, label)
	}
}

// useColorLogs reports whether text logs should use ANSI on stderr only.
// Disabled for JSON, file-only, or dual stderr+file writers so log files stay plain.
func useColorLogs() bool {
	if jsonFormat || omitStderr || extraWriter != nil {
		return false
	}
	return term.ColorEnabled(os.Stderr)
}
