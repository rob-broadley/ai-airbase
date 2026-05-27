// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
