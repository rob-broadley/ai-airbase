// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/config"
	"github.com/rob-broadley/ai-airbase/marshal/internal/container"
)

// ---------------------------------------------------------------------------
// Unit tests for ensureConfigFile
// ---------------------------------------------------------------------------

func TestEnsureConfigFile_SymlinkReturnsError(t *testing.T) {
	// Given a symlink planted at the expected config path
	dir := t.TempDir()
	target := filepath.Join(dir, "real-file")
	if err := os.WriteFile(target, []byte("sensitive"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "config.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	// When ensureConfigFile is called with the symlink path
	err := ensureConfigFile(link, []byte("{}\n"))

	// Then it returns an error containing "symlink"
	if err == nil {
		t.Fatal("expected an error for symlink path, got nil")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("expected error to mention 'symlink', got: %v", err)
	}
}

func TestEnsureConfigFile_DirectoryReturnsError(t *testing.T) {
	// Given a directory at the expected config path
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.Mkdir(configPath, 0o755); err != nil {
		t.Fatal(err)
	}

	// When ensureConfigFile is called with the directory path
	err := ensureConfigFile(configPath, []byte("{}\n"))

	// Then it returns an error containing "directory"
	if err == nil {
		t.Fatal("expected an error for directory path, got nil")
	}
	if !strings.Contains(err.Error(), "directory") {
		t.Errorf("expected error to mention 'directory', got: %v", err)
	}
}

func TestEnsureConfigFile_CreatesFileWithCorrectContentAndPermissions(t *testing.T) {
	// Given a path that does not exist
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	content := []byte(`{"mcpServers":{}}` + "\n")

	// When ensureConfigFile is called
	if err := ensureConfigFile(configPath, content); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Then the file exists with the expected content
	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("file not readable: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("file content mismatch: got %q, want %q", got, content)
	}

	// And the file has 0o600 permissions
	fi, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("expected permissions 0o600, got %04o", perm)
	}
}

func TestEnsureConfigFile_ExistingFilePreservesContentAndReturnsNil(t *testing.T) {
	// Given a file that already exists with specific content
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	original := []byte(`{"existing":true}`)
	if err := os.WriteFile(configPath, original, 0o600); err != nil {
		t.Fatal(err)
	}

	// When ensureConfigFile is called with different default content
	err := ensureConfigFile(configPath, []byte("{}\n"))

	// Then it returns nil (no error)
	if err != nil {
		t.Fatalf("expected nil for existing file, got: %v", err)
	}

	// And the original content is unchanged
	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("file not readable: %v", err)
	}
	if string(got) != string(original) {
		t.Errorf("file content was modified: got %q, want %q", got, original)
	}
}

func TestEnsureConfigFile_CalledTwiceIsIdempotent(t *testing.T) {
	// Given a path that does not yet exist
	dir := t.TempDir()
	configPath := filepath.Join(dir, "settings.json")
	content := []byte("{}\n")

	// When ensureConfigFile is called twice in succession
	if err := ensureConfigFile(configPath, content); err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	// Then the second call also returns nil (concurrent-create idempotency path)
	if err := ensureConfigFile(configPath, content); err != nil {
		t.Fatalf("second call failed: %v", err)
	}

	// And the file still contains the original content
	got, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("file not readable: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("content changed after second call: got %q, want %q", got, content)
	}
}

func TestSanitizeForTerminal(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "plain name unchanged", input: "Alice", expected: "Alice"},
		{name: "NUL stripped", input: "Alice\x00Bob", expected: "AliceBob"},
		{name: "all control chars stripped", input: "\x01\x1f", expected: ""},
		{name: "DEL stripped", input: "\x7f", expected: ""},
		{name: "DEL in middle stripped", input: "Alice\x7fBob", expected: "AliceBob"},
		{name: "space preserved and tab stripped", input: " spaces and\ttabs", expected: " spaces andtabs"},
		{name: "newline stripped", input: "Alice\nBob", expected: "AliceBob"},
		{name: "empty string", input: "", expected: ""},
		{name: "non-ASCII printable preserved", input: "café", expected: "café"},
		// C1 control characters (U+0080–U+009F) — added to guard against 8-bit CSI injection
		{name: "C1 boundary-start stripped", input: "\u0080", expected: ""},
		{name: "C1 8-bit CSI stripped", input: "\u009b", expected: ""},
		{name: "C1 boundary-end stripped", input: "\u009f", expected: ""},
		{name: "C1 embedded in string stripped", input: "sha256:\u009b1A", expected: "sha256:1A"},
		{name: "U+00A0 no-break space preserved (just above C1)", input: "\u00a0", expected: "\u00a0"},
		// Unicode bidi / line-separator characters — Trojan-source visual-spoof guard
		{name: "U+202E RLO stripped (right-to-left override)", input: "/mnt/\u202egnp.txt", expected: "/mnt/gnp.txt"},
		{name: "U+200E LRM stripped (left-to-right mark)", input: "path\u200e/sub", expected: "path/sub"},
		{name: "U+200F RLM stripped (right-to-left mark)", input: "path\u200f/sub", expected: "path/sub"},
		{name: "U+202A bidi embedding start stripped", input: "\u202apath", expected: "path"},
		{name: "U+202B bidi embedding start stripped (RLE)", input: "\u202bpath", expected: "path"},
		{name: "U+202C PDF stripped (pop directional formatting)", input: "path\u202c", expected: "path"},
		{name: "U+202D LRO stripped (left-to-right override)", input: "\u202dpath", expected: "path"},
		{name: "U+2066 LRI stripped (left-to-right isolate)", input: "\u2066path\u2069", expected: "path"},
		{name: "U+2067 RLI stripped (right-to-left isolate)", input: "\u2067path\u2069", expected: "path"},
		{name: "U+2068 FSI stripped (first strong isolate)", input: "\u2068path\u2069", expected: "path"},
		{name: "U+2069 PDI stripped (pop directional isolate)", input: "path\u2069", expected: "path"},
		{name: "U+2028 line separator stripped", input: "line\u2028break", expected: "linebreak"},
		{name: "U+2029 paragraph separator stripped", input: "para\u2029break", expected: "parabreak"},
		{name: "bidi chars mixed with plain text stripped, plain preserved", input: "real\u202e/etc/passwd", expected: "real/etc/passwd"},
		// Zero-width formatting characters — visual-distortion guard for mount paths
		{name: "zero-width chars stripped", input: "/mnt/\u200bfoo\u200cbar\u200dbaz\ufeffe", expected: "/mnt/foobarbaze"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeForTerminal(tt.input); got != tt.expected {
				t.Errorf("sanitizeForTerminal(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// TestSanitizeForTerminal_StripsArabicLetterMark verifies that U+061C (ARABIC
// LETTER MARK), a Unicode format character that some terminals honour for
// display reordering, is removed from a path string before terminal output.
func TestSanitizeForTerminal_StripsArabicLetterMark(t *testing.T) {
	// Given a path containing the Arabic Letter Mark (U+061C)
	input := "/home/user/\u061Cpath"

	// When sanitizeForTerminal is called
	got := sanitizeForTerminal(input)

	// Then the output must not contain U+061C
	if strings.ContainsRune(got, '\u061C') {
		t.Errorf("sanitizeForTerminal(%q) = %q; output still contains U+061C (Arabic Letter Mark)", input, got)
	}
}

// ---------------------------------------------------------------------------
// Unit tests for Deps.ensureSharedDataDir accessor
// ---------------------------------------------------------------------------

// TestDeps_EnsureSharedDataDir_NilFieldDefaultsToConfigFunc verifies that
// when EnsureSharedDataDir is not injected (nil), the accessor returns a
// non-nil function (defaulting to config.EnsureSharedDataDir).
func TestDeps_EnsureSharedDataDir_NilFieldDefaultsToConfigFunc(t *testing.T) {
	// Given a Deps with EnsureSharedDataDir left nil (zero-value)
	deps := Deps{}

	// When the nil-safe accessor is called
	fn := deps.ensureSharedDataDir()

	// Then a non-nil function is returned (the default is config.EnsureSharedDataDir)
	if fn == nil {
		t.Fatal("ensureSharedDataDir() returned nil; expected config.EnsureSharedDataDir as default")
	}

	// And the returned function behaves like config.EnsureSharedDataDir —
	// call it with a temp-dir-rooted subdir and expect no error.
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	dir, err := fn("projects/test-project")
	if err != nil {
		t.Fatalf("default ensureSharedDataDir returned unexpected error: %v", err)
	}
	if dir == "" {
		t.Fatal("default ensureSharedDataDir returned empty directory path")
	}
}

// TestDeps_EnsureSharedDataDir_InjectedFunctionIsUsed verifies that when
// EnsureSharedDataDir is set, the accessor returns that exact function.
func TestDeps_EnsureSharedDataDir_InjectedFunctionIsUsed(t *testing.T) {
	// Given a Deps with a custom EnsureSharedDataDir injected
	called := false
	stub := func(subdir string) (string, error) {
		called = true
		return "/stub/" + subdir, nil
	}
	deps := Deps{EnsureSharedDataDir: stub}

	// When the accessor is invoked and called
	fn := deps.ensureSharedDataDir()
	got, err := fn("projects/myproject")

	// Then the injected function is called, not config.EnsureSharedDataDir
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("injected EnsureSharedDataDir was not called")
	}
	if got != "/stub/projects/myproject" {
		t.Errorf("got %q, want %q", got, "/stub/projects/myproject")
	}
}

// ---------------------------------------------------------------------------
// Unit tests for renderMountsAndMasks — terminal sanitization of configured paths
// ---------------------------------------------------------------------------

// TestRenderMountsAndMasks_WrongSourceSameDestination verifies that when a
// running container has a bind mount at the expected container destination
// (/workspace/<basename>) but with a DIFFERENT host source than the configured
// path, the configured entry is shown as ✗ (missing) and the actual source
// appears as ? (untracked). This guards against destination-only matching,
// which would incorrectly show ✓ for the configured path. Source and
// destination must both match for a configured entry to show as ✓.
func TestRenderMountsAndMasks_WrongSourceSameDestination(t *testing.T) {
	// Given a configured mount /my/project whose expected container dest is
	// /workspace/project, and an actual bind mount with Source=/other/project
	// (a different host path) landing at the same Destination=/workspace/project.
	configuredMount := "/my/project"
	actualSource := "/other/project"
	containerDest := containerWorkspaceDir + "/project" // /workspace/project

	var buf bytes.Buffer

	// When renderMountsAndMasks is called
	renderMountsAndMasks(slog.New(slog.NewTextHandler(io.Discard, nil)), &buf, []string{configuredMount}, nil, []container.ContainerMount{
		{Type: "bind", Source: actualSource, Destination: containerDest},
	})

	output := buf.String()

	// Then the configured path /my/project must show ✗ (not mounted)
	if !strings.Contains(output, "✗ "+configuredMount) {
		t.Errorf("expected configured mount %q to show ✗ (missing), but got:\n%s", configuredMount, output)
	}

	// And the actual source /other/project must show ? (untracked)
	if !strings.Contains(output, "? "+actualSource) {
		t.Errorf("expected actual source %q to show ? (untracked), but got:\n%s", actualSource, output)
	}

	// And the configured path must NOT show ✓ (must not be falsely active)
	if strings.Contains(output, "✓ "+configuredMount) {
		t.Errorf("configured mount %q must NOT show ✓ when source does not match, but got:\n%s", configuredMount, output)
	}
}

// TestRenderMountsAndMasks_ConfiguredMountPathSanitised verifies that a
// configured mount host path containing a terminal control character is
// stripped before it is written to the output, for both the active (✓) and
// missing (✗) symbol cases.
func TestRenderMountsAndMasks_ConfiguredMountPathSanitised(t *testing.T) {
	// \x1b is ESC (C0 control char, 0x1b). sanitizeForTerminal strips it,
	// leaving the surrounding text joined: "/mnt/project\x1bpath" → "/mnt/projectpath".
	const rawEsc = "\x1b"
	mountPath := "/mnt/project" + rawEsc + "path"
	sanitised := "/mnt/projectpath"

	tests := []struct {
		name         string
		actualMounts []container.ContainerMount
		wantSymbol   string
	}{
		{
			name: "active mount (checkmark) sanitises configured path",
			actualMounts: []container.ContainerMount{
				{
					Type:        "bind",
					Source:      mountPath,
					Destination: containerWorkspaceDir + "/project" + rawEsc + "path",
				},
			},
			wantSymbol: "✓",
		},
		{
			name:         "missing mount (cross) sanitises configured path",
			actualMounts: nil,
			wantSymbol:   "✗",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer

			// When renderMountsAndMasks is called with the tainted mount path
			renderMountsAndMasks(slog.New(slog.NewTextHandler(io.Discard, nil)), &buf, []string{mountPath}, nil, tt.actualMounts)

			output := buf.String()

			// Then the raw escape byte must not appear in the output
			if strings.Contains(output, rawEsc) {
				t.Errorf("output contains raw escape byte; got: %q", output)
			}

			// And the sanitised path must appear in the output
			if !strings.Contains(output, sanitised) {
				t.Errorf("output does not contain sanitised path %q; got: %q", sanitised, output)
			}

			// And the expected symbol must appear
			if !strings.Contains(output, tt.wantSymbol) {
				t.Errorf("output does not contain symbol %q; got: %q", tt.wantSymbol, output)
			}
		})
	}
}

// TestRenderMountsAndMasks_MaskWithNoParentMountEmitsDebugLog verifies that when
// a configured mask path does not fall under any configured mount (so
// maskContainerPath returns ""), a Debug-level log entry is emitted with key
// "mask" set to the mask path. Specifically, the message must be
// "configured mask has no parent bind mount" with attribute mask=<mask path>.
func TestRenderMountsAndMasks_MaskWithNoParentMountEmitsDebugLog(t *testing.T) {
	// Given a configured mask whose path does NOT fall under any configured mount.
	const maskPath = "/home/user/secrets"

	// And a logger backed by a buffer that captures Debug-level records.
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// When renderMountsAndMasks is called with no configured mounts but with the mask.
	var out bytes.Buffer
	renderMountsAndMasks(logger, &out, nil, []string{maskPath}, nil)

	// Then the log buffer must contain the debug message.
	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "configured mask has no parent bind mount") {
		t.Errorf("expected debug log message %q, but got log output:\n%s",
			"configured mask has no parent bind mount", logOutput)
	}

	// And the log entry must include the mask attribute set to the mask path.
	if !strings.Contains(logOutput, "mask="+maskPath) {
		t.Errorf("expected log attribute mask=%q, but got log output:\n%s",
			maskPath, logOutput)
	}
}

// TestRenderMountsAndMasks_UntrackedVolumeWithNoParentBindEmitsDebugLog verifies
// that when an untracked volume mount has no parent bind mount (the
// hostEquivalentPath fallback activates), a Debug-level log entry is emitted
// with key "path" set to the original container path. Specifically, the message
// must be "untracked mask path not resolved to host path" with attribute
// path=<container dest>.
func TestRenderMountsAndMasks_UntrackedVolumeWithNoParentBindEmitsDebugLog(t *testing.T) {
	// Given an untracked volume mount at /workspace/untracked with no configured
	// mounts and no actual bind mounts (so hostEquivalentPath cannot resolve it).
	const containerDest = "/workspace/untracked"
	actualMounts := []container.ContainerMount{
		{Type: "volume", Destination: containerDest},
	}

	// And a logger backed by a buffer that captures Debug-level records.
	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// When renderMountsAndMasks is called with no configured mounts or masks.
	var out bytes.Buffer
	renderMountsAndMasks(logger, &out, nil, nil, actualMounts)

	// Then the log buffer must contain the debug message.
	logOutput := logBuf.String()
	if !strings.Contains(logOutput, "untracked mask path not resolved to host path") {
		t.Errorf("expected debug log message %q, but got log output:\n%s",
			"untracked mask path not resolved to host path", logOutput)
	}

	// And the log entry must include the path attribute.
	if !strings.Contains(logOutput, "path="+containerDest) {
		t.Errorf("expected log attribute path=%q, but got log output:\n%s",
			containerDest, logOutput)
	}
}

// TestHostEquivalentPath_ExactMountRootMatch verifies that when containerPath
// equals a mount's containerDest exactly (no subdirectory suffix),
// hostEquivalentPath returns the corresponding hostPath unchanged.
func TestHostEquivalentPath_ExactMountRootMatch(t *testing.T) {
	// Given a mountContainerDest mapping where a host path maps to a container dest
	mountContainerDest := map[string]string{
		"/home/user/project": "/workspace/project",
	}
	// And an actualBind map (not needed for this case)
	activeBind := map[string]string{}

	// When containerPath is exactly the mount root (no subdirectory)
	result := hostEquivalentPath("/workspace/project", mountContainerDest, activeBind)

	// Then the corresponding host path is returned
	if result != "/home/user/project" {
		t.Errorf("hostEquivalentPath returned %q, want %q", result, "/home/user/project")
	}
}

// TestMaskContainerPath_ExactMountRootMatch verifies that when mask equals a
// configured mount exactly (not a subdirectory of it), maskContainerPath
// returns the mount's containerDest (the mask == m branch).
func TestMaskContainerPath_ExactMountRootMatch(t *testing.T) {
	// Given a configured mount and a mask path equal to that mount root
	configuredMounts := []string{"/home/user/project"}
	mountContainerDest := map[string]string{
		"/home/user/project": "/workspace/project",
	}

	// When mask equals the mount root exactly
	result := maskContainerPath("/home/user/project", configuredMounts, mountContainerDest)

	// Then the container destination for the mount root is returned
	if result != "/workspace/project" {
		t.Errorf("maskContainerPath returned %q, want %q", result, "/workspace/project")
	}
}

// recordingHandler is an slog.Handler spy that captures every record it is
// asked to handle. It is safe for concurrent use; records are appended under a
// mutex in the order Handle is called. It is used by the log-spy tests
// TestEnsurePortIsConfigured_EmitsInfoLogOnAutoAllocation and
// TestEnsurePortIsConfigured_DoesNotLogWhenPortAlreadyConfigured below to
// assert that an INFO log call is emitted on the auto-allocation path and
// that no log call is emitted on the early-return path.
type recordingHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

// Enabled always reports true so the spy records records at any level.
func (h *recordingHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

// Handle appends a clone of r to the spy's record slice.
func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

// WithAttrs is a no-op: the spy is not used with pre-attached attributes.
func (h *recordingHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }

// WithGroup is a no-op: the spy does not model groups.
func (h *recordingHandler) WithGroup(_ string) slog.Handler { return h }

// recordCount returns the number of captured records.
func (h *recordingHandler) recordCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.records)
}

// describeRecords returns a one-line human-readable dump of every captured
// record (level + message), used in test failure messages so an operator can
// see what was actually logged.
func (h *recordingHandler) describeRecords() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.records) == 0 {
		return "(no records)"
	}
	parts := make([]string, 0, len(h.records))
	for _, r := range h.records {
		parts = append(parts, fmt.Sprintf("{level=%s msg=%q}", r.Level, r.Message))
	}
	return strings.Join(parts, ", ")
}

// findInfoRecordWithValue returns the first record at INFO level whose
// attributes include a key==attrKey with a value matching the string form of
// want, OR whose rendered message text contains the string form of want. This
// lets the test pass against either an attribute-style log call
// (slog.Int("port", n)) or a printf-style log call whose message embeds the
// port number. Returns the record and true on success, or a zero slog.Record
// and false if no matching record exists.
func (h *recordingHandler) findInfoRecordWithValue(attrKey string, want any) (slog.Record, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	wantStr := fmt.Sprintf("%v", want)
	for _, r := range h.records {
		if r.Level != slog.LevelInfo {
			continue
		}
		if strings.Contains(r.Message, wantStr) {
			return r, true
		}
		found := false
		r.Attrs(func(a slog.Attr) bool {
			if a.Key == attrKey && fmt.Sprintf("%v", a.Value.Any()) == wantStr {
				found = true
				return false
			}
			return true
		})
		if found {
			return r, true
		}
	}
	return slog.Record{}, false
}

// ---------------------------------------------------------------------------
// Unit tests for port allocation and validation
// ---------------------------------------------------------------------------

// TestEnsurePortIsConfigured_AutoAllocatesFreePortAndPersists verifies that
// when no port is configured (cfg.Port == 0) and the host has a free port
// available, ensurePortIsConfigured auto-allocates it, updates the in-memory
// config to reflect the new port, and persists the configuration via
// SaveConfig.
func TestEnsurePortIsConfigured_AutoAllocatesFreePortAndPersists(t *testing.T) {
	// Given a project "my-project" with no port configured (cfg.Port is 0)
	cfg := &config.Config{Port: 0}
	// And there is a free port available (first available is 4096)
	// And deps has functions configured for listing projects, checking port bounds, and saving configuration
	var savedProject string
	var savedCfg *config.Config
	saveCalled := 0

	deps := Deps{
		ListProjects: func() ([]string, []string, error) {
			return []string{"my-project"}, nil, nil
		},
		IsPortBound: func(port int) bool {
			return false
		},
		SaveConfig: func(project string, cfg *config.Config) error {
			savedProject = project
			savedCfg = cfg
			saveCalled++
			return nil
		},
	}

	// When ensurePortIsConfigured is called with the project and configuration
	port, err := ensurePortIsConfigured(deps, "my-project", cfg)

	// Then the configuration port (cfg.Port) is updated to 4096
	if cfg.Port != 4096 {
		t.Errorf("expected cfg.Port to be updated to 4096, got %d", cfg.Port)
	}
	// And deps.saveConfig is called to save the configuration
	if saveCalled != 1 {
		t.Errorf("expected SaveConfig to be called once, called %d times", saveCalled)
	}
	if savedProject != "my-project" {
		t.Errorf("expected saved project to be %q, got %q", "my-project", savedProject)
	}
	if savedCfg != cfg {
		t.Errorf("expected saved config to be the same config pointer")
	}
	// And the resolved port 4096 is returned without error
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if port != 4096 {
		t.Errorf("expected resolved port 4096, got %d", port)
	}
}

// TestEnsurePortIsConfigured_LeavesConfiguredPortUnchangedAndDoesNotSave
// verifies that when a port is already configured (cfg.Port != 0),
// ensurePortIsConfigured returns it unchanged and does not invoke SaveConfig.
func TestEnsurePortIsConfigured_LeavesConfiguredPortUnchangedAndDoesNotSave(t *testing.T) {
	// Given a project "my-project" with port 8080 already configured
	cfg := &config.Config{Port: 8080}
	// And a SaveConfig spy that counts invocations
	saveCalled := 0
	deps := Deps{
		SaveConfig: func(project string, cfg *config.Config) error {
			saveCalled++
			return nil
		},
	}

	// When ensurePortIsConfigured is called
	port, err := ensurePortIsConfigured(deps, "my-project", cfg)

	// Then the configured port 8080 is returned with no error
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if port != 8080 {
		t.Errorf("expected configured port 8080, got %d", port)
	}
	// And cfg.Port is left unchanged at 8080
	if cfg.Port != 8080 {
		t.Errorf("expected cfg.Port to remain 8080, got %d", cfg.Port)
	}
	// And SaveConfig is not called
	if saveCalled != 0 {
		t.Errorf("expected SaveConfig not to be called, called %d times", saveCalled)
	}
}

// TestEnsurePortIsConfigured_PropagatesFreePortLookupError verifies that when
// no port is configured and the free-port lookup (via ListProjects) returns an
// error, ensurePortIsConfigured surfaces that error and does not call
// SaveConfig.
func TestEnsurePortIsConfigured_PropagatesFreePortLookupError(t *testing.T) {
	// Given a project "my-project" with no port configured (cfg.Port is 0)
	// And ListProjects returns a sentinel error that findFreePort will surface
	cfg := &config.Config{Port: 0}
	listErr := errors.New("list projects failed")
	saveCalled := 0
	deps := Deps{
		ListProjects: func() ([]string, []string, error) {
			return nil, nil, listErr
		},
		SaveConfig: func(project string, cfg *config.Config) error {
			saveCalled++
			return nil
		},
	}

	// When ensurePortIsConfigured is called
	port, err := ensurePortIsConfigured(deps, "my-project", cfg)

	// Then the sentinel error is returned and port is 0
	if !errors.Is(err, listErr) {
		t.Errorf("expected error %v, got %v", listErr, err)
	}
	if port != 0 {
		t.Errorf("expected port 0 on error, got %d", port)
	}
	// And SaveConfig is not called
	if saveCalled != 0 {
		t.Errorf("expected SaveConfig not to be called, called %d times", saveCalled)
	}
}

// TestEnsurePortIsConfigured_PropagatesConfigurationSaveError verifies that
// when SaveConfig fails after a free port is allocated, ensurePortIsConfigured
// propagates the save error and returns a port of 0.
func TestEnsurePortIsConfigured_PropagatesConfigurationSaveError(t *testing.T) {
	// Given a project "my-project" with no port configured (cfg.Port is 0)
	// And ListProjects returns an empty list (no port conflicts to probe)
	// And IsPortBound returns false (port 4096 is free)
	// And SaveConfig returns a sentinel error
	cfg := &config.Config{Port: 0}
	saveErr := errors.New("disk full")
	deps := Deps{
		ListProjects: func() ([]string, []string, error) {
			return nil, nil, nil
		},
		IsPortBound: func(port int) bool {
			return false
		},
		SaveConfig: func(project string, cfg *config.Config) error {
			return saveErr
		},
	}

	// When ensurePortIsConfigured is called
	port, err := ensurePortIsConfigured(deps, "my-project", cfg)

	// Then the SaveConfig error is propagated
	if !errors.Is(err, saveErr) {
		t.Errorf("expected error %v, got %v", saveErr, err)
	}
	if port != 0 {
		t.Errorf("expected port 0 on error, got %d", port)
	}
}

// TestEnsurePortIsConfigured_LeavesInMemoryPortUnchangedWhenPersistenceFails
// verifies that when SaveConfig fails after the free port has been written to
// cfg.Port, the in-memory config is reverted to 0 so it remains consistent
// with the unchanged on-disk config.
func TestEnsurePortIsConfigured_LeavesInMemoryPortUnchangedWhenPersistenceFails(t *testing.T) {
	// Given a project "my-project" with no port configured (cfg.Port is 0)
	// And ListProjects returns an empty list (no port conflicts to probe)
	// And IsPortBound returns false (port 4096 is free, so findFreePort returns 4096)
	// And SaveConfig returns a sentinel error
	cfg := &config.Config{Port: 0}
	saveErr := errors.New("disk full")
	deps := Deps{
		ListProjects: func() ([]string, []string, error) {
			return nil, nil, nil
		},
		IsPortBound: func(port int) bool {
			return false
		},
		SaveConfig: func(project string, cfg *config.Config) error {
			return saveErr
		},
	}

	// When ensurePortIsConfigured is called
	port, err := ensurePortIsConfigured(deps, "my-project", cfg)

	// Then the SaveConfig error is propagated
	if !errors.Is(err, saveErr) {
		t.Errorf("expected error %v, got %v", saveErr, err)
	}
	if port != 0 {
		t.Errorf("expected port 0 on error, got %d", port)
	}
	// And cfg.Port is NOT mutated — on-disk and in-memory must remain consistent
	if cfg.Port != 0 {
		t.Errorf("expected cfg.Port to remain 0 after save failure, got %d", cfg.Port)
	}
}

// TestEnsurePortIsConfigured_EmitsInfoLogOnAutoAllocation verifies that when
// ensurePortIsConfigured auto-allocates a free port (cfg.Port == 0 path), an
// INFO-level log record carrying the allocated port value is emitted via the
// injected Logger.
func TestEnsurePortIsConfigured_EmitsInfoLogOnAutoAllocation(t *testing.T) {
	// Given a project "my-project" with cfg.Port == 0 (triggers auto-allocation)
	// And ListProjects returns ["my-project"] (no port conflicts)
	// And IsPortBound always returns false (every port is free)
	// And SaveConfig is a no-op spy
	// And deps.Logger is a custom slog.Handler spy that records every record
	cfg := &config.Config{Port: 0}
	spy := &recordingHandler{}
	deps := Deps{
		Logger: slog.New(spy),
		ListProjects: func() ([]string, []string, error) {
			return []string{"my-project"}, nil, nil
		},
		IsPortBound: func(port int) bool {
			return false
		},
		SaveConfig: func(project string, cfg *config.Config) error {
			return nil
		},
	}

	// When ensurePortIsConfigured is called
	port, err := ensurePortIsConfigured(deps, "my-project", cfg)

	// Then no error is returned
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if port == 0 {
		t.Fatalf("expected a non-zero auto-allocated port, got 0")
	}
	// And the logger captured at least one INFO-level record carrying the allocated port value
	if _, ok := spy.findInfoRecordWithValue("port", port); !ok {
		t.Errorf("expected an INFO log record carrying port=%d, got %d record(s): %s",
			port, spy.recordCount(), spy.describeRecords())
	}
}

// TestEnsurePortIsConfigured_DoesNotLogWhenPortAlreadyConfigured verifies that
// when a port is already configured (cfg.Port != 0), the early-return path
// emits no log records.
func TestEnsurePortIsConfigured_DoesNotLogWhenPortAlreadyConfigured(t *testing.T) {
	// Given a project "my-project" with cfg.Port == 8080 (early-return path)
	// And deps.Logger is the same custom slog.Handler spy
	cfg := &config.Config{Port: 8080}
	spy := &recordingHandler{}
	deps := Deps{
		Logger: slog.New(spy),
	}

	// When ensurePortIsConfigured is called
	port, err := ensurePortIsConfigured(deps, "my-project", cfg)

	// Then no error is returned and the configured port is preserved
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if port != 8080 {
		t.Errorf("expected configured port 8080, got %d", port)
	}
	// And the logger captured zero log records on the early-return path
	if n := spy.recordCount(); n != 0 {
		t.Errorf("expected no log calls when port is already configured, got %d record(s): %s",
			n, spy.describeRecords())
	}
}

// TestResolveAndValidatePort_FreePortInRange_ReturnsUnchanged verifies that
// when portFlag is non-zero (the only path the call site ever exercises),
// resolveAndValidatePort validates the port range, checks for cross-project
// port conflicts, checks whether the port is already bound on the host, and
// returns the port unchanged on success.
func TestResolveAndValidatePort_FreePortInRange_ReturnsUnchanged(t *testing.T) {
	// Given a project "my-project" with no other projects configured to use port 5000
	// And port 5000 is not bound on host loopback
	// And no other project lists port 5000 in its config
	deps := Deps{
		ListProjects: func() ([]string, []string, error) {
			return []string{"my-project"}, nil, nil
		},
		IsPortBound: func(port int) bool {
			return false
		},
	}

	// When resolveAndValidatePort is called with port 5000
	port, err := resolveAndValidatePort(deps, "my-project", 5000)

	// Then it returns 5000 with no error
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if port != 5000 {
		t.Errorf("expected resolved port 5000, got %d", port)
	}
}

// ---------------------------------------------------------------------------
// Unit tests for hardenProjectDir
// ---------------------------------------------------------------------------

// TestHardenProjectDir_MissingDirectoryIsNoop verifies that hardening a path that
// does not exist returns nil without error. This is the freshly-created case:
// the caller (e.g. ensureSharedDataDir) has not yet created the directory.
func TestHardenProjectDir_MissingDirectoryIsNoop(t *testing.T) {
	// Given a path that does not exist
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	// When hardenProjectDir is called
	err := hardenProjectDir(missing)

	// Then it returns nil (no error, no panic)
	if err != nil {
		t.Errorf("expected nil error for missing directory, got: %v", err)
	}
}

// TestHardenProjectDir_RootIsSymlinkAborts verifies that if the bind mount target
// itself is a symlink (os.RemoveAll would follow it), hardenProjectDir aborts with
// the "directory is a symlink" error before attempting any walk.
func TestHardenProjectDir_RootIsSymlinkAborts(t *testing.T) {
	// Given a real directory and a symlink pointing to it
	base := t.TempDir()
	realDir := filepath.Join(base, "real")
	if err := os.MkdirAll(realDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	symlinkDir := filepath.Join(base, "link")
	if err := os.Symlink(realDir, symlinkDir); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	// When hardenProjectDir is called on the symlink
	err := hardenProjectDir(symlinkDir)

	// Then it returns the "directory is a symlink" error
	if err == nil {
		t.Fatal("expected error for symlink root, got nil")
	}
	if !strings.Contains(err.Error(), "security violation: directory is a symlink") {
		t.Errorf("expected 'directory is a symlink' error, got: %v", err)
	}
}

// TestHardenProjectDir_RootNotWritableAborts verifies that if the bind mount target
// is not owner-writable (the 0o200 bit is clear), hardenProjectDir aborts with a
// "permission denied" error mentioning the directory path.
func TestHardenProjectDir_RootNotWritableAborts(t *testing.T) {
	// Given a directory with no owner write bit
	dir := filepath.Join(t.TempDir(), "readonly")
	if err := os.MkdirAll(dir, 0o500); err != nil { // r-x for owner
		t.Fatalf("MkdirAll: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) }) // allow TempDir cleanup

	// When hardenProjectDir is called
	err := hardenProjectDir(dir)

	// Then it returns a "permission denied" error mentioning the path
	if err == nil {
		t.Fatal("expected error for non-writable root, got nil")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("expected 'permission denied' error, got: %v", err)
	}
	if !strings.Contains(err.Error(), dir) {
		t.Errorf("expected error to mention %q, got: %v", dir, err)
	}
}

// TestHardenProjectDir_NestedRelativeSymlinkAllowed verifies that a relative
// symlink whose target stays inside the bind mount is allowed. This is the
// opencode/package-manager case that was previously broken.
func TestHardenProjectDir_NestedRelativeSymlinkAllowed(t *testing.T) {
	// Given a directory containing a file and a relative symlink to it
	dir := t.TempDir()
	target := filepath.Join(dir, "real.json")
	if err := os.WriteFile(target, []byte("{}"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	link := filepath.Join(dir, "link.json")
	if err := os.Symlink("real.json", link); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	// When hardenProjectDir is called
	err := hardenProjectDir(dir)

	// Then it returns nil (the symlink stays inside the bind mount)
	if err != nil {
		t.Errorf("expected nil error for internal symlink, got: %v", err)
	}

	// And the symlink still exists
	if _, err := os.Lstat(link); err != nil {
		t.Errorf("expected symlink to still exist, got: %v", err)
	}
}

// TestHardenProjectDir_NestedAbsoluteSymlinkEscapesAborts verifies that a
// symlink inside the bind mount whose resolved target is outside the bind mount
// causes hardenProjectDir to abort with the "escapes the bind mount" error.
func TestHardenProjectDir_NestedAbsoluteSymlinkEscapesAborts(t *testing.T) {
	// Given a bind mount directory and a target file outside it
	base := t.TempDir()
	dir := filepath.Join(base, "mount")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	outside := filepath.Join(base, "outside.json")
	if err := os.WriteFile(outside, []byte("{}"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// Symlink inside the mount pointing to the outside file
	link := filepath.Join(dir, "escape.json")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	// When hardenProjectDir is called
	err := hardenProjectDir(dir)

	// Then it returns an "escapes the bind mount" error
	if err == nil {
		t.Fatal("expected error for escaping symlink, got nil")
	}
	if !strings.Contains(err.Error(), "escapes the bind mount") {
		t.Errorf("expected 'escapes the bind mount' error, got: %v", err)
	}
}

// TestHardenProjectDir_NestedRelativeSymlinkEscapesAborts verifies that a
// relative symlink whose target is outside the bind mount is caught. The
// relative path is evaluated from the symlink's parent directory, so a
// symlink deep in the tree can use enough ".." levels to reach outside.
func TestHardenProjectDir_NestedRelativeSymlinkEscapesAborts(t *testing.T) {
	// Given a bind mount directory nested two levels deep, and a file
	// outside the mount, and a relative symlink inside the mount that
	// resolves to that file
	base := t.TempDir()
	mountRoot := filepath.Join(base, "mount")
	nestedDir := filepath.Join(mountRoot, "a", "b")
	if err := os.MkdirAll(nestedDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// Create the target file outside the mount at /base/evil
	evil := filepath.Join(base, "evil")
	if err := os.WriteFile(evil, []byte("pwned"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// Link at /base/mount/a/b/escape -> ../../../evil
	// From /base/mount/a/b/, three ".." levels reaches /base/, then evil → /base/evil
	link := filepath.Join(nestedDir, "escape")
	if err := os.Symlink("../../../evil", link); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	// When hardenProjectDir is called on the mount root
	err := hardenProjectDir(mountRoot)

	// Then it returns an "escapes the bind mount" error
	if err == nil {
		t.Fatal("expected error for traversal symlink, got nil")
	}
	if !strings.Contains(err.Error(), "escapes the bind mount") {
		t.Errorf("expected 'escapes the bind mount' error, got: %v", err)
	}
}

// TestHardenProjectDir_NestedBrokenSymlinkAllowed verifies that a symlink whose
// target does not exist on the host is allowed. A broken symlink cannot be
// used for a sandbox escape: the host resolves the symlink, and if the target
// is missing the resolution fails. The container cannot create files outside
// the bind mount, so a broken symlink stays broken.
func TestHardenProjectDir_NestedBrokenSymlinkAllowed(t *testing.T) {
	// Given a directory containing a symlink whose target does not exist
	dir := t.TempDir()
	link := filepath.Join(dir, "broken")
	if err := os.Symlink("does-not-exist", link); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	// When hardenProjectDir is called
	err := hardenProjectDir(dir)

	// Then it returns nil (broken symlink is not an escape)
	if err != nil {
		t.Errorf("expected nil error for broken symlink, got: %v", err)
	}
}

// TestHardenProjectDir_NestedSymlinkChainEscapesAborts verifies that a
// multi-hop symlink chain (A -> B -> outside) is caught. EvalSymlinks follows
// the entire chain, so a symlink that points to another symlink which points
// outside the bind mount is still detected as an escape.
func TestHardenProjectDir_NestedSymlinkChainEscapesAborts(t *testing.T) {
	// Given a bind mount directory and a two-hop symlink chain inside it
	// where the final hop resolves to a file outside the mount
	base := t.TempDir()
	dir := filepath.Join(base, "mount")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	outside := filepath.Join(base, "outside.json")
	if err := os.WriteFile(outside, []byte("pwned"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// First hop: mount/inner -> mount/outer (both inside the mount)
	// Second hop: mount/outer -> ../outside.json (escapes the mount)
	outer := filepath.Join(dir, "outer")
	if err := os.Symlink("../outside.json", outer); err != nil {
		t.Fatalf("Symlink outer: %v", err)
	}
	inner := filepath.Join(dir, "inner")
	if err := os.Symlink("outer", inner); err != nil {
		t.Fatalf("Symlink inner: %v", err)
	}

	// When hardenProjectDir is called
	err := hardenProjectDir(dir)

	// Then it returns an "escapes the bind mount" error (the inner link's
	// chain resolves to ../outside.json which is outside the mount)
	if err == nil {
		t.Fatal("expected error for chained escaping symlink, got nil")
	}
	if !strings.Contains(err.Error(), "escapes the bind mount") {
		t.Errorf("expected 'escapes the bind mount' error, got: %v", err)
	}
}

// TestHardenProjectDir_CorrectsPermissionsOnDirAndFile verifies that existing
// directories and files have their permissions corrected to 0o700 and 0o600
// respectively by the walk.
func TestHardenProjectDir_CorrectsPermissionsOnDirAndFile(t *testing.T) {
	// Given a directory tree with loosened permissions
	dir := t.TempDir()
	nested := filepath.Join(dir, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	file := filepath.Join(nested, "file.txt")
	if err := os.WriteFile(file, []byte("data"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// When hardenProjectDir is called
	if err := hardenProjectDir(dir); err != nil {
		t.Fatalf("hardenProjectDir: %v", err)
	}

	// Then the root, nested dir, and file all have hardened permissions
	wantDir := os.FileMode(0o700)
	wantFile := os.FileMode(0o600)
	for path, want := range map[string]os.FileMode{dir: wantDir, nested: wantDir, file: wantFile} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat %s: %v", path, err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s: expected perm %04o, got %04o", path, want, got)
		}
	}
}

// TestHardenProjectDir_PreservesAlreadyCorrectPermissions verifies that when
// permissions are already 0o700/0o600, the walk is a no-op (no chmod calls
// needed, no error).
func TestHardenProjectDir_PreservesAlreadyCorrectPermissions(t *testing.T) {
	// Given a directory tree already at 0o700/0o600
	dir := t.TempDir()
	nested := filepath.Join(dir, "nested")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	file := filepath.Join(nested, "file.txt")
	if err := os.WriteFile(file, []byte("data"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// When hardenProjectDir is called
	if err := hardenProjectDir(dir); err != nil {
		t.Fatalf("hardenProjectDir: %v", err)
	}

	// Then permissions are unchanged
	for path, want := range map[string]os.FileMode{dir: 0o700, nested: 0o700, file: 0o600} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat %s: %v", path, err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s: expected perm %04o, got %04o", path, want, got)
		}
	}
}

// ---------------------------------------------------------------------------
// Unit tests for isPathUnder
// ---------------------------------------------------------------------------

// TestIsPathUnder_EqualPaths verifies that child == parent is treated as "under".
// The bind mount root itself is the first path the walk visits (via d.IsDir()),
// and it must be considered inside its own bound.
func TestIsPathUnder_EqualPaths(t *testing.T) {
	if !isPathUnder("/a/b", "/a/b") {
		t.Error("expected /a/b to be under /a/b (equal)")
	}
}

// TestIsPathUnder_ChildUnderParent verifies the basic containment case.
func TestIsPathUnder_ChildUnderParent(t *testing.T) {
	if !isPathUnder("/a/b/c", "/a/b") {
		t.Error("expected /a/b/c to be under /a/b")
	}
}

// TestIsPathUnder_PrefixBoundaryNotMatched verifies that /a/bc is NOT considered
// under /a/b. This is the safety property the trailing-separator logic enforces.
func TestIsPathUnder_PrefixBoundaryNotMatched(t *testing.T) {
	if isPathUnder("/a/bc", "/a/b") {
		t.Error("expected /a/bc to NOT be under /a/b (prefix boundary)")
	}
}

// TestIsPathUnder_ParentAlreadyHasTrailingSeparator verifies that the function
// is robust to the caller having already appended a separator to parent.
func TestIsPathUnder_ParentAlreadyHasTrailingSeparator(t *testing.T) {
	if !isPathUnder("/a/b/c", "/a/b/") {
		t.Error("expected /a/b/c to be under /a/b/")
	}
	if isPathUnder("/a/bc", "/a/b/") {
		t.Error("expected /a/bc to NOT be under /a/b/ (prefix boundary)")
	}
}

// TestIsPathUnder_ChildOutsideParent verifies the negative case.
func TestIsPathUnder_ChildOutsideParent(t *testing.T) {
	if isPathUnder("/x/y", "/a/b") {
		t.Error("expected /x/y to NOT be under /a/b")
	}
}
