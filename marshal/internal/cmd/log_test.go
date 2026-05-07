// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd

import (
	"bytes"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Unit tests for cliHandler
// ---------------------------------------------------------------------------

func TestCLIHandler_InfoWritesMessageOnly(t *testing.T) {
	// Given a cliHandler writing to a buffer at INFO level

	// When an INFO record with no attributes is logged
	var buf bytes.Buffer
	logger := NewCLILogger(&buf)
	logger.Info("pulling image")

	// Then only the message is written (no timestamp, no level prefix)
	got := buf.String()
	if !strings.Contains(got, "pulling image") {
		t.Errorf("expected message in output, got: %q", got)
	}
	if strings.Contains(got, "INFO") || strings.Contains(got, "level") {
		t.Errorf("INFO level prefix must not appear in output, got: %q", got)
	}
	if strings.Contains(got, "time=") {
		t.Errorf("timestamp must not appear in output, got: %q", got)
	}
}

func TestCLIHandler_InfoWithAttrsWritesKeyValue(t *testing.T) {
	// Given a cliHandler writing to a buffer

	// When an INFO record with attributes is logged
	var buf bytes.Buffer
	logger := NewCLILogger(&buf)
	logger.Info("creating container", "container", "marshal-myapp")

	// Then the message and key=value pair appear in the output
	got := buf.String()
	if !strings.Contains(got, "creating container") {
		t.Errorf("expected message in output, got: %q", got)
	}
	if !strings.Contains(got, "marshal-myapp") {
		t.Errorf("expected attribute value in output, got: %q", got)
	}
}

func TestCLIHandler_WarnWritesWarningPrefix(t *testing.T) {
	// Given a cliHandler writing to a buffer

	// When a WARN record is logged
	var buf bytes.Buffer
	logger := NewCLILogger(&buf)
	logger.Warn("pull failed, using local image")

	// Then the output is prefixed with "warning: "
	got := buf.String()
	if !strings.HasPrefix(got, "warning: ") {
		t.Errorf("expected 'warning: ' prefix, got: %q", got)
	}
	if !strings.Contains(got, "pull failed") {
		t.Errorf("expected message in output, got: %q", got)
	}
}

func TestCLIHandler_ErrorWritesErrorPrefix(t *testing.T) {
	// Given a cliHandler writing to a buffer

	// When an ERROR record is logged
	var buf bytes.Buffer
	logger := NewCLILogger(&buf)
	logger.Error("registry unreachable")

	// Then the output is prefixed with "error: "
	got := buf.String()
	if !strings.HasPrefix(got, "error: ") {
		t.Errorf("expected 'error: ' prefix, got: %q", got)
	}
}

func TestCLIHandler_DebugSuppressedAtInfoLevel(t *testing.T) {
	// Given a cliHandler at INFO level

	// When a DEBUG record is logged
	var buf bytes.Buffer
	logger := NewCLILogger(&buf)
	logger.Debug("verbose debug output")

	// Then nothing is written to the buffer
	if buf.Len() > 0 {
		t.Errorf("DEBUG must be suppressed at INFO level, got: %q", buf.String())
	}
}

func TestCLIHandler_EachRecordEndsWithNewline(t *testing.T) {
	// Given a cliHandler

	// When an INFO record is logged
	var buf bytes.Buffer
	logger := NewCLILogger(&buf)
	logger.Info("some message")

	// Then the output ends with a newline
	got := buf.String()
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("output must end with newline, got: %q", got)
	}
}

func TestNewCLILogger_ReturnsNonNilLogger(t *testing.T) {
	// Given a buffer

	// When NewCLILogger is called (exported constructor used by tests)
	var buf bytes.Buffer
	logger := NewCLILogger(&buf)

	// Then a non-nil logger is returned that accepts records
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
	logger.Info("hello")
	if !strings.Contains(buf.String(), "hello") {
		t.Errorf("expected message in output, got: %q", buf.String())
	}
}

func TestCLIHandler_IsThreadSafe(t *testing.T) {
	// Given a cliHandler shared across goroutines

	// When multiple goroutines log concurrently
	var buf bytes.Buffer
	logger := NewCLILogger(&buf)
	done := make(chan struct{}, 10)
	for i := 0; i < 10; i++ {
		go func() {
			logger.Info("concurrent message")
			done <- struct{}{}
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}

	// Then all 10 messages appear as complete lines (no dropped or partial writes)
	if got := strings.Count(buf.String(), "\n"); got != 10 {
		t.Errorf("expected 10 complete lines from 10 goroutines, got %d: %q", got, buf.String())
	}
}

func TestCLIHandler_WithAttrs_PreAttachesFields(t *testing.T) {
	// Given a logger with a pre-attached attribute

	// When an INFO record is logged
	var buf bytes.Buffer
	logger := NewCLILogger(&buf).With("component", "lifecycle")
	logger.Info("creating container")

	// Then the pre-attached attribute appears alongside the message
	got := buf.String()
	if !strings.Contains(got, "component=lifecycle") {
		t.Errorf("expected pre-attached attr in output, got: %q", got)
	}
	if !strings.Contains(got, "creating container") {
		t.Errorf("expected message in output, got: %q", got)
	}
}

func TestCLIHandler_WithGroup_DropsGroupName(t *testing.T) {
	// Given a logger with a group applied (cliHandler does not support groups)

	// When an INFO record with an attribute is logged via the grouped logger
	var buf bytes.Buffer
	logger := NewCLILogger(&buf).WithGroup("phase")
	logger.Info("starting", "step", 1)

	// Then the group prefix is not present in the output (documented drop behaviour)
	got := buf.String()
	if strings.Contains(got, "phase.") {
		t.Errorf("expected group name to be dropped, got: %q", got)
	}
	if !strings.Contains(got, "starting") {
		t.Errorf("expected message in output, got: %q", got)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------
