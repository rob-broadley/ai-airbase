// SPDX-License-Identifier: AGPL-3.0-or-later
package hostinfo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// ConfigPath — XDG resolution (moved from config/config_unit_test.go)
// ---------------------------------------------------------------------------
//
// These tests originally verified the unexported config.configPath function.
// They test the XDG resolution path: $XDG_CONFIG_HOME/marshal/projects/<name>.toml.
// The equivalent in hostinfo is SharedConfigPath("projects/<name>.toml"), which
// resolves to the same path. The assertions are unchanged; only the subject
// of the test shifts from config.configPath to hostinfo.SharedConfigPath.

// TestConfigPath_XDGOverride verifies that ConfigPath returns a path under
// XDG_CONFIG_HOME when that environment variable is set.
func TestConfigPath_XDGOverride(t *testing.T) {
	// Given XDG_CONFIG_HOME is set to a temp directory
	tmp := t.TempDir()
	setenv(t, "XDG_CONFIG_HOME", tmp)

	// When SharedConfigPath is called for a per-project config file
	got := SharedConfigPath("projects/myproject.toml")
	want := filepath.Join(tmp, "marshal", "projects", "myproject.toml")

	// Then the path is under the XDG_CONFIG_HOME directory
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

// TestConfigPath_DefaultXDG verifies that ConfigPath falls back to
// ~/.config/marshal/projects when XDG_CONFIG_HOME is unset.
func TestConfigPath_DefaultXDG(t *testing.T) {
	// Given XDG_CONFIG_HOME is unset
	unsetenv(t, "XDG_CONFIG_HOME")

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	// When SharedConfigPath is called for a per-project config file
	got := SharedConfigPath("projects/myproject.toml")
	want := filepath.Join(home, ".config", "marshal", "projects", "myproject.toml")

	// Then the path falls back to ~/.config/marshal/projects/myproject.toml
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

// ---------------------------------------------------------------------------
// EnsureSharedConfigDir
// ---------------------------------------------------------------------------

// TestEnsureSharedConfigDir_XDGOverride verifies that EnsureSharedConfigDir
// returns a path under XDG_CONFIG_HOME when that environment variable is set.
func TestEnsureSharedConfigDir_XDGOverride_ReturnsExpectedPath(t *testing.T) {
	// Given XDG_CONFIG_HOME is set to a temp directory
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	// When EnsureSharedConfigDir is called
	got, err := EnsureSharedConfigDir("opencode")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Then the path is under the XDG_CONFIG_HOME directory
	want := filepath.Join(tmp, "marshal", "opencode")
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

// TestEnsureSharedConfigDir_DefaultXDG verifies that EnsureSharedConfigDir falls
// back to ~/.config when XDG_CONFIG_HOME is unset.
func TestEnsureSharedConfigDir_DefaultXDG_UsesHomeConfig(t *testing.T) {
	// Given XDG_CONFIG_HOME is cleared and HOME is set to a known directory
	homeRoot := t.TempDir()
	home := filepath.Join(homeRoot, "test-home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", home)

	// When EnsureSharedConfigDir is called
	got, err := EnsureSharedConfigDir("opencode")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Then the path falls back to ~/.config/marshal/<subdir>
	want := filepath.Join(home, ".config", "marshal", "opencode")
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

// TestEnsureSharedConfigDir_CreatesDir verifies that EnsureSharedConfigDir
// creates the target directory when it does not yet exist.
func TestEnsureSharedConfigDir_CreatesDirWhenAbsent(t *testing.T) {
	// Given XDG_CONFIG_HOME is set to a temp directory and the target is absent
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	target := filepath.Join(tmp, "marshal", "credentials")

	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("expected %q to be absent before test, got err=%v", target, err)
	}

	// When EnsureSharedConfigDir is called
	got, err := EnsureSharedConfigDir("credentials")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Then the directory is created at the expected path
	if got != target {
		t.Errorf("expected %q, got %q", target, got)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("expected directory to exist at %q: %v", target, err)
	}
	if !info.IsDir() {
		t.Errorf("expected %q to be a directory", target)
	}
}

// TestEnsureSharedConfigDir_FileAtTargetPath_ReturnsError verifies that
// EnsureSharedConfigDir returns an error when the target path exists as a file.
func TestEnsureSharedConfigDir_FileAtTargetPath_ReturnsCreateError(t *testing.T) {
	// Given XDG_CONFIG_HOME is set and a file exists at the target path
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	if err := os.MkdirAll(filepath.Join(tmp, "marshal"), 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(tmp, "marshal", "opencode")
	if err := os.WriteFile(target, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	// When EnsureSharedConfigDir is called
	_, err := EnsureSharedConfigDir("opencode")

	// Then an error is returned
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "creating shared config directory") {
		t.Errorf("expected 'creating shared config directory' in error, got: %v", err)
	}
}

// TestEnsureSharedConfigDir_UsesXDGConfigHome verifies that EnsureSharedConfigDir
// returns a path rooted at XDG_CONFIG_HOME when that env var is set, and that
// the directory is created with mode 0o700.
func TestEnsureSharedConfigDir_UsesXDGConfigHome(t *testing.T) {
	// Given XDG_CONFIG_HOME is set to a temp directory
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	// When EnsureSharedConfigDir is called with a subdir name
	got, err := EnsureSharedConfigDir("git")

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
func TestEnsureSharedConfigDir_FallsBackToHomeConfig(t *testing.T) {
	// Given XDG_CONFIG_HOME is unset and HOME is set to a known temp directory
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "") // empty → treated as unset by xdgBaseDir
	t.Setenv("HOME", tmp)

	// When EnsureSharedConfigDir is called
	got, err := EnsureSharedConfigDir("opencode")

	// Then no error occurs and the path is rooted under $HOME/.config/marshal/opencode
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(tmp, ".config", "marshal", "opencode")
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
func TestEnsureSharedConfigDir_CreatesDirectoryWhenAbsent(t *testing.T) {
	// Given XDG_CONFIG_HOME is set to a temp directory and the subdir does not exist
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	target := filepath.Join(tmp, "marshal", "newdir")
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("test setup error: directory %q should not exist yet", target)
	}

	// When EnsureSharedConfigDir is called
	got, err := EnsureSharedConfigDir("newdir")

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

// TestEnsureSharedConfigDir_IdempotentReRun verifies that EnsureSharedConfigDir
// returns no error when the target directory already exists.
func TestEnsureSharedConfigDir_IdempotentReRun(t *testing.T) {
	// Given XDG_CONFIG_HOME is set and the directory was already created
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	if _, err := EnsureSharedConfigDir("opencode"); err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	// When EnsureSharedConfigDir is called a second time on the same subdir
	// Then no error is returned
	if _, err := EnsureSharedConfigDir("opencode"); err != nil {
		t.Fatalf("second call failed (not idempotent): %v", err)
	}
}

// TestEnsureSharedConfigDir_DirectoryCreationFails verifies that
// EnsureSharedConfigDir returns an error containing "creating shared config
// directory" when a file blocks directory creation.
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
	_, err := EnsureSharedConfigDir("credentials")

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
func TestEnsureSharedConfigDir_NonAbsolutePath_ReturnsUnavailableError(t *testing.T) {
	// Given XDG_CONFIG_HOME is set to a relative (non-absolute) path
	t.Setenv("XDG_CONFIG_HOME", "relative")
	t.Setenv("HOME", "")

	// When EnsureSharedConfigDir is called
	_, err := EnsureSharedConfigDir("somedir")

	// Then an error containing "unavailable" is returned
	if err == nil {
		t.Fatal("expected error when config path is not absolute, got nil")
	}
	if !strings.Contains(err.Error(), "unavailable") {
		t.Errorf("expected 'unavailable' in error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// SharedDataPath
// ---------------------------------------------------------------------------

// TestSharedDataPath_XDGOverride verifies that SharedDataPath returns a path
// under XDG_DATA_HOME when that environment variable is set.
func TestSharedDataPath_XDGOverride(t *testing.T) {
	// Given XDG_DATA_HOME is set to a temp directory
	tmp := t.TempDir()
	setenv(t, "XDG_DATA_HOME", tmp)

	// When SharedDataPath is called
	got := SharedDataPath("opencode")
	want := filepath.Join(tmp, "marshal", "opencode")

	// Then the path is under the XDG_DATA_HOME directory
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

// TestSharedDataPath_XDGOverride_MultiSegment verifies that SharedDataPath
// correctly joins multi-segment subdirectory paths under XDG_DATA_HOME.
func TestSharedDataPath_XDGOverride_MultiSegment(t *testing.T) {
	// Given XDG_DATA_HOME is set and a multi-segment subdir path is provided
	tmp := t.TempDir()
	setenv(t, "XDG_DATA_HOME", tmp)

	// When SharedDataPath is called with a multi-segment subdir
	got := SharedDataPath("config/opencode")
	want := filepath.Join(tmp, "marshal", "config", "opencode")

	// Then all path segments are correctly joined
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

// TestSharedDataPath_DefaultXDG verifies that SharedDataPath falls back to
// ~/.local/share/marshal when XDG_DATA_HOME is unset.
func TestSharedDataPath_DefaultXDG(t *testing.T) {
	// Given XDG_DATA_HOME is unset
	unsetenv(t, "XDG_DATA_HOME")

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	// When SharedDataPath is called
	got := SharedDataPath("opencode")
	want := filepath.Join(home, ".local", "share", "marshal", "opencode")

	// Then the path falls back to ~/.local/share/marshal/opencode
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

// ---------------------------------------------------------------------------
// EnsureSharedDataDir
// ---------------------------------------------------------------------------

// TestEnsureSharedDataDir_CreatesDir verifies that EnsureSharedDataDir creates
// the directory with mode 0o700 and returns its path.
func TestEnsureSharedDataDir_CreatesDir(t *testing.T) {
	// Given XDG_DATA_HOME is set to a temp directory
	tmp := t.TempDir()
	setenv(t, "XDG_DATA_HOME", tmp)

	// When EnsureSharedDataDir is called
	got, err := EnsureSharedDataDir("opencode")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Then the directory is created with mode 0o700 and its path is returned
	want := filepath.Join(tmp, "marshal", "opencode")
	if got != want {
		t.Errorf("expected path %q, got %q", want, got)
	}

	info, err := os.Stat(got)
	if err != nil {
		t.Fatalf("expected directory to exist at %q: %v", got, err)
	}
	if !info.IsDir() {
		t.Errorf("expected %q to be a directory", got)
	}
	// Verify restrictive permissions (owner-only).
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("expected permissions 0o700, got %04o", perm)
	}
}

// TestEnsureSharedDataDir_Idempotent verifies that EnsureSharedDataDir is
// idempotent and does not return an error when called twice.
func TestEnsureSharedDataDir_Idempotent(t *testing.T) {
	// Given XDG_DATA_HOME is set and the directory was already created
	tmp := t.TempDir()
	setenv(t, "XDG_DATA_HOME", tmp)

	if _, err := EnsureSharedDataDir("opencode"); err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	// When EnsureSharedDataDir is called a second time
	// Then no error is returned
	if _, err := EnsureSharedDataDir("opencode"); err != nil {
		t.Fatalf("second call failed (not idempotent): %v", err)
	}
}

// TestEnsureSharedDataDir_CreatesNestedDirs verifies that EnsureSharedDataDir
// creates all nested subdirectories in a single call.
func TestEnsureSharedDataDir_CreatesNestedDirs(t *testing.T) {
	// Given XDG_DATA_HOME is set and a multi-segment subdir path is requested
	tmp := t.TempDir()
	setenv(t, "XDG_DATA_HOME", tmp)

	// When EnsureSharedDataDir is called with a nested subdir path
	got, err := EnsureSharedDataDir("config/opencode")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Then all nested directories are created
	if _, err := os.Stat(got); os.IsNotExist(err) {
		t.Errorf("expected nested directory to exist at %q", got)
	}
}

// TestEnsureSharedDataDir_DirectoryCreationFails verifies that
// EnsureSharedDataDir returns an error containing "creating shared data
// directory" when MkdirAll fails.
func TestEnsureSharedDataDir_DirectoryCreationFails(t *testing.T) {
	// Given a file exists at the path where the shared data directory should be created
	tmp := t.TempDir()
	setenv(t, "XDG_DATA_HOME", tmp)

	// Block directory creation by placing a file where the directory should be.
	blockPath := filepath.Join(tmp, "marshal")
	if err := os.WriteFile(blockPath, []byte("block"), 0o644); err != nil {
		t.Fatal(err)
	}

	// When EnsureSharedDataDir is called
	_, err := EnsureSharedDataDir("credentials")

	// Then an error containing "creating shared data directory" is returned
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "creating shared data directory") {
		t.Errorf("expected 'creating shared data directory' in error, got: %v", err)
	}
}

// TestEnsureSharedDataDir_NonAbsoluteDataPath_ReturnsUnavailableError verifies
// that EnsureSharedDataDir returns an actionable error when the resolved data
// path is not absolute.
func TestEnsureSharedDataDir_NonAbsoluteDataPath_ReturnsUnavailableError(t *testing.T) {
	// Given XDG_DATA_HOME is set to a relative (non-absolute) path
	t.Setenv("XDG_DATA_HOME", "relative")
	t.Setenv("HOME", "")

	// When EnsureSharedDataDir is called
	_, err := EnsureSharedDataDir("somedir")

	// Then an error containing "unavailable" is returned
	if err == nil {
		t.Fatal("expected error when data path is not absolute, got nil")
	}
	if !strings.Contains(err.Error(), "unavailable") {
		t.Errorf("expected 'unavailable' in error, got: %v", err)
	}
}
