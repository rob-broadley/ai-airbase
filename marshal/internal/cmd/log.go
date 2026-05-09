// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
)

// ---------------------------------------------------------------------------
// CLI progress logger
// ---------------------------------------------------------------------------

// NewCLILogger returns a *slog.Logger backed by a cliHandler that writes
// human-readable progress output to w. It is exported so tests can inject it
// with a buffer to capture and verify progress messages.
//
// Output format:
//
//	INFO  → "msg [key=value ...]"
//	WARN  → "warning: msg [key=value ...]"
//	ERROR → "error: msg [key=value ...]"
//	DEBUG → suppressed
func NewCLILogger(w io.Writer) *slog.Logger {
	return slog.New(&cliHandler{w: w, level: slog.LevelInfo, mu: &sync.Mutex{}})
}

// cliHandler is a slog.Handler that writes clean, human-readable progress
// lines to an io.Writer. INFO records are written as plain text with no level
// prefix or timestamp; WARN and ERROR records are prefixed accordingly.
// mu is a pointer so sibling handlers derived via WithAttrs share the same
// lock and serialise writes to the underlying writer correctly.
type cliHandler struct {
	w     io.Writer
	mu    *sync.Mutex // shared with sibling handlers created by WithAttrs
	attrs []slog.Attr // pre-attached attributes from WithAttrs
	level slog.Level
}

// Enabled reports whether the handler will handle records at the given level.
func (h *cliHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

// Handle formats r and writes the line to h.w.
func (h *cliHandler) Handle(_ context.Context, r slog.Record) error {
	var buf bytes.Buffer

	switch {
	case r.Level >= slog.LevelError:
		buf.WriteString("error: ")
	case r.Level >= slog.LevelWarn:
		buf.WriteString("warning: ")
	}

	buf.WriteString(r.Message)

	// Append pre-attached attributes then record attributes.
	writeAttrs := func(attrs []slog.Attr) {
		for _, a := range attrs {
			if a.Equal(slog.Attr{}) {
				continue
			}
			fmt.Fprintf(&buf, " %s=%v", a.Key, a.Value)
		}
	}
	writeAttrs(h.attrs)
	r.Attrs(func(a slog.Attr) bool {
		writeAttrs([]slog.Attr{a})
		return true
	})

	buf.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.w.Write(buf.Bytes())
	return err
}

// WithAttrs returns a new handler with the given attributes pre-attached to
// every subsequent record. The shared mutex pointer is forwarded so siblings
// serialise writes to the same underlying writer.
func (h *cliHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	merged := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(merged, h.attrs)
	copy(merged[len(h.attrs):], attrs)
	return &cliHandler{w: h.w, level: h.level, mu: h.mu, attrs: merged}
}

// WithGroup returns the handler unchanged; cliHandler does not support groups.
func (h *cliHandler) WithGroup(_ string) slog.Handler {
	return h
}
