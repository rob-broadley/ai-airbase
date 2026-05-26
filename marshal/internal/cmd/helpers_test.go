// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeForTerminal(tt.input); got != tt.expected {
				t.Errorf("sanitizeForTerminal(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Unit tests for Deps.ensureSharedDataDir accessor
// ---------------------------------------------------------------------------

// TestDeps_EnsureSharedDataDir_NilFieldDefaultsToConfigFunc verifies that
// when EnsureSharedDataDir is not injected (nil), the accessor returns a
// non-nil function (defaulting to config.EnsureSharedDataDir).
//
// Acceptance criterion: calling deps.ensureSharedDataDir() on a zero-value
// Deps must never panic and must return a usable function.
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
