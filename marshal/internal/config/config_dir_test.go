// SPDX-License-Identifier: AGPL-3.0-or-later
package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/config"
)

// ---------------------------------------------------------------------------
// 13. EnsureSharedConfigDir
// ---------------------------------------------------------------------------

// TestEnsureSharedConfigDir_UsesXDGConfigHome verifies that EnsureSharedConfigDir
// returns a path rooted at XDG_CONFIG_HOME when that env var is set, and that
// the directory is created with mode 0o700.
//
// Acceptance criterion: Returns the correct path under XDG_CONFIG_HOME when
// that env var is set, and creates the directory.
func TestEnsureSharedConfigDir_UsesXDGConfigHome(t *testing.T) {
	// Given XDG_CONFIG_HOME is set to a temp directory
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	// When EnsureSharedConfigDir is called with a subdir name
	got, err := config.EnsureSharedConfigDir("git")

	// Then no error occurs and the path is rooted under XDG_CONFIG_HOME/marshal
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(tmp, "marshal", "git")
	if got != want {
		t.Errorf("expected path %q, got %q", want, got)
	}

	// And the directory exists with mode 0o700
	info, err := os.Stat(got)
	if err != nil {
		t.Fatalf("expected directory to exist at %q: %v", got, err)
	}
	if !info.IsDir() {
		t.Errorf("expected %q to be a directory", got)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("expected permissions 0o700, got %04o", perm)
	}
}

// TestEnsureSharedConfigDir_FallsBackToHomeConfig verifies that when
// XDG_CONFIG_HOME is not set, EnsureSharedConfigDir falls back to
// ~/.config and returns a path rooted there.
//
// Acceptance criterion: Returns the correct path under ~/.config when
// XDG_CONFIG_HOME is not set.
func TestEnsureSharedConfigDir_FallsBackToHomeConfig(t *testing.T) {
	// Given XDG_CONFIG_HOME is unset and HOME is set to a known temp directory
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "") // empty → treated as unset by xdgBaseDir
	t.Setenv("HOME", tmp)

	// When EnsureSharedConfigDir is called
	got, err := config.EnsureSharedConfigDir("copilot")

	// Then no error occurs and the path is rooted under $HOME/.config/marshal
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(tmp, ".config", "marshal", "copilot")
	if got != want {
		t.Errorf("expected path %q, got %q", want, got)
	}

	// And the directory exists
	if _, err := os.Stat(got); os.IsNotExist(err) {
		t.Errorf("expected directory to exist at %q", got)
	}
}

// TestEnsureSharedConfigDir_CreatesDirectoryWhenAbsent verifies that
// EnsureSharedConfigDir creates the target directory when it does not yet exist.
//
// Acceptance criterion: Creates the directory when it does not exist.
func TestEnsureSharedConfigDir_CreatesDirectoryWhenAbsent(t *testing.T) {
	// Given XDG_CONFIG_HOME is set to a temp directory and the subdir does not exist
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	target := filepath.Join(tmp, "marshal", "newdir")
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("test setup error: directory %q should not exist yet", target)
	}

	// When EnsureSharedConfigDir is called
	got, err := config.EnsureSharedConfigDir("newdir")

	// Then no error occurs and the directory is created
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != target {
		t.Errorf("expected path %q, got %q", target, got)
	}
	if _, err := os.Stat(got); os.IsNotExist(err) {
		t.Errorf("expected directory to be created at %q", got)
	}
}

// TestEnsureSharedConfigDir_Idempotent verifies that EnsureSharedConfigDir
// returns no error when the target directory already exists.
//
// Acceptance criterion: Returns successfully (no error) when the directory
// already exists.
func TestEnsureSharedConfigDir_Idempotent(t *testing.T) {
	// Given XDG_CONFIG_HOME is set and the directory was already created
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	if _, err := config.EnsureSharedConfigDir("copilot"); err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	// When EnsureSharedConfigDir is called a second time on the same subdir
	// Then no error is returned
	if _, err := config.EnsureSharedConfigDir("copilot"); err != nil {
		t.Fatalf("second call failed (not idempotent): %v", err)
	}
}

// TestEnsureSharedConfigDir_DirectoryCreationFails verifies that
// EnsureSharedConfigDir returns an error containing "creating shared config
// directory" when a file blocks directory creation.
//
// Acceptance criterion: Returns an error when the path exists as a file
// (or cannot be created).
func TestEnsureSharedConfigDir_DirectoryCreationFails(t *testing.T) {
	// Given a file exists at the path where the shared config directory should be
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	// Block directory creation by placing a file where the directory should be.
	blockPath := filepath.Join(tmp, "marshal")
	if err := os.WriteFile(blockPath, []byte("block"), 0o644); err != nil {
		t.Fatal(err)
	}

	// When EnsureSharedConfigDir is called
	_, err := config.EnsureSharedConfigDir("credentials")

	// Then an error containing "creating shared config directory" is returned
	if err == nil {
		t.Fatal("expected an error when a file blocks directory creation, got nil")
	}
	if !strings.Contains(err.Error(), "creating shared config directory") {
		t.Errorf("expected 'creating shared config directory' in error, got: %v", err)
	}
}

// TestEnsureSharedConfigDir_NonAbsolutePath_ReturnsUnavailableError verifies
// that EnsureSharedConfigDir returns an actionable "unavailable" error when
// neither XDG_CONFIG_HOME nor HOME is usable, leaving a non-absolute path.
//
// Acceptance criterion: Returns an error when the path cannot be created
// (degenerate environment — no absolute base available).
func TestEnsureSharedConfigDir_NonAbsolutePath_ReturnsUnavailableError(t *testing.T) {
	// Given XDG_CONFIG_HOME is set to a relative (non-absolute) path
	t.Setenv("XDG_CONFIG_HOME", "relative")
	t.Setenv("HOME", "")

	// When EnsureSharedConfigDir is called
	_, err := config.EnsureSharedConfigDir("somedir")

	// Then an error containing "unavailable" is returned
	if err == nil {
		t.Fatal("expected error when config path is not absolute, got nil")
	}
	if !strings.Contains(err.Error(), "unavailable") {
		t.Errorf("expected 'unavailable' in error, got: %v", err)
	}
}
