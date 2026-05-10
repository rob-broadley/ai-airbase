// SPDX-License-Identifier: AGPL-3.0-or-later
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// setenv sets an env var for the duration of a test and restores it on cleanup.
func setenv(t *testing.T, key, value string) {
	t.Helper()
	original, existed := os.LookupEnv(key)
	os.Setenv(key, value)
	t.Cleanup(func() {
		if existed {
			os.Setenv(key, original)
		} else {
			os.Unsetenv(key)
		}
	})
}

// unsetenv unsets an env var for the duration of a test and restores it on cleanup.
func unsetenv(t *testing.T, key string) {
	t.Helper()
	original, existed := os.LookupEnv(key)
	os.Unsetenv(key)
	t.Cleanup(func() {
		if existed {
			os.Setenv(key, original)
		}
	})
}

// assertMounts verifies that cfg.Mounts exactly matches want, element by element.
func assertMounts(t *testing.T, cfg *Config, want []string) {
	t.Helper()
	if len(cfg.Mounts) != len(want) {
		t.Fatalf("expected %d mounts, got %d: %v", len(want), len(cfg.Mounts), cfg.Mounts)
	}
	for i, m := range want {
		if cfg.Mounts[i] != m {
			t.Errorf("mount[%d]: expected %q, got %q", i, m, cfg.Mounts[i])
		}
	}
}

// ---------------------------------------------------------------------------
// 1. Project name resolution
// ---------------------------------------------------------------------------

// TestResolveProject_FlagTakesPrecedence verifies that a non-empty flag value
// takes precedence over MARSHAL_PROJECT and the CWD basename.
func TestResolveProject_FlagTakesPrecedence(t *testing.T) {
	// Given MARSHAL_PROJECT is set but a non-empty flag value is also provided
	setenv(t, "MARSHAL_PROJECT", "env-project")

	// When ResolveProject is called with the flag value
	got, err := ResolveProject("flag-project", os.Getwd)

	// Then no error occurs and the flag value is returned
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "flag-project" {
		t.Errorf("expected %q, got %q", "flag-project", got)
	}
}

// TestResolveProject_EnvFallback verifies that MARSHAL_PROJECT is used when
// the flag value is empty.
func TestResolveProject_EnvFallback(t *testing.T) {
	// Given MARSHAL_PROJECT is set and the flag value is empty
	setenv(t, "MARSHAL_PROJECT", "env-project")

	// When ResolveProject is called with an empty flag
	got, err := ResolveProject("", os.Getwd)

	// Then no error occurs and the environment variable value is returned
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "env-project" {
		t.Errorf("expected %q, got %q", "env-project", got)
	}
}

// TestResolveProject_CWDFallback verifies that the CWD basename is used when
// neither a flag value nor MARSHAL_PROJECT is set.
func TestResolveProject_CWDFallback(t *testing.T) {
	// Given no flag value and MARSHAL_PROJECT is unset, with cwd set to a known directory
	unsetenv(t, "MARSHAL_PROJECT")

	// Change into a temp dir so we know the basename.
	tmp := t.TempDir()
	// Rename to a known name via a sub-directory.
	projectDir := filepath.Join(tmp, "myproject")
	if err := os.Mkdir(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(originalDir) }) //nolint:errcheck

	if err := os.Chdir(projectDir); err != nil {
		t.Fatal(err)
	}

	// When ResolveProject is called with an empty flag
	got, err := ResolveProject("", os.Getwd)

	// Then no error occurs and the basename of the current working directory is returned
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "myproject" {
		t.Errorf("expected %q, got %q", "myproject", got)
	}
}

// ---------------------------------------------------------------------------
// 2. ConfigPath
// ---------------------------------------------------------------------------

// TestConfigPath_XDGOverride verifies that ConfigPath returns a path under
// XDG_CONFIG_HOME when that environment variable is set.
func TestConfigPath_XDGOverride(t *testing.T) {
	// Given XDG_CONFIG_HOME is set to a temp directory
	tmp := t.TempDir()
	setenv(t, "XDG_CONFIG_HOME", tmp)

	// When ConfigPath is called
	got := configPath("myproject")
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

	// When ConfigPath is called
	got := configPath("myproject")
	want := filepath.Join(home, ".config", "marshal", "projects", "myproject.toml")

	// Then the path falls back to ~/.config/marshal/projects/myproject.toml
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

// ---------------------------------------------------------------------------
// 3. Load — file missing
// ---------------------------------------------------------------------------

// TestLoad_FileMissing_ReturnsEmptyConfig verifies that Load returns an empty
// Config without error when the config file does not exist.
func TestLoad_FileMissing_ReturnsEmptyConfig(t *testing.T) {
	// Given no config file exists for the project
	tmp := t.TempDir()
	setenv(t, "XDG_CONFIG_HOME", tmp)

	// When Load is called for a nonexistent project
	cfg, err := Load("nonexistent-project")

	// Then an empty config is returned without error
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}
	if len(cfg.Mounts) != 0 {
		t.Errorf("expected empty mounts, got %v", cfg.Mounts)
	}
}

// ---------------------------------------------------------------------------
// 4. Load — file exists with mounts
// ---------------------------------------------------------------------------

// TestLoad_FileExists_ReturnsMounts verifies that Load parses mounts correctly
// from an existing TOML config file.
func TestLoad_FileExists_ReturnsMounts(t *testing.T) {
	// Given a TOML config file exists at the expected path with mount entries
	tmp := t.TempDir()
	setenv(t, "XDG_CONFIG_HOME", tmp)

	// Write a TOML file at the expected path.
	projectsDir := filepath.Join(tmp, "marshal", "projects")
	if err := os.MkdirAll(projectsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tomlContent := `mounts = ["/home/user/code", "/data"]` + "\n"
	if err := os.WriteFile(filepath.Join(projectsDir, "myproject.toml"), []byte(tomlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// When Load is called for the project
	cfg, err := Load("myproject")

	// Then the mounts from the file are returned
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	assertMounts(t, cfg, []string{"/home/user/code", "/data"})
}

// TestLoad_MalformedTOML_ReturnsError verifies that Load returns an error when
// the config file contains invalid TOML.
func TestLoad_MalformedTOML_ReturnsError(t *testing.T) {
	// Given a config file exists but contains malformed TOML
	tmp := t.TempDir()
	setenv(t, "XDG_CONFIG_HOME", tmp)

	projectsDir := filepath.Join(tmp, "marshal", "projects")
	if err := os.MkdirAll(projectsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectsDir, "bad-project.toml"), []byte("mounts = [unclosed"), 0o644); err != nil {
		t.Fatal(err)
	}

	// When Load is called for that project
	_, err := Load("bad-project")

	// Then an error is returned
	if err == nil {
		t.Error("expected an error for malformed TOML, got nil")
	}
}

// ---------------------------------------------------------------------------
// 5. Save — round-trip
// ---------------------------------------------------------------------------

// TestSave_RoundTrip verifies that Save writes config to disk and a subsequent
// Load returns the same data.
func TestSave_RoundTrip(t *testing.T) {
	// Given a config with two mount paths
	tmp := t.TempDir()
	setenv(t, "XDG_CONFIG_HOME", tmp)

	original := &Config{
		Mounts: []string{"/workspace", "/home/user"},
	}

	// When Save is called followed by Load
	if err := Save("roundtrip", original); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := Load("roundtrip")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Then the loaded config matches the saved config
	assertMounts(t, loaded, original.Mounts)
}

// TestSave_CreatesDirectories verifies that Save creates intermediate directories
// that don't yet exist before writing the config file.
func TestSave_CreatesDirectories(t *testing.T) {
	// Given XDG_CONFIG_HOME points to a nested directory that does not yet exist
	tmp := t.TempDir()
	// Point XDG at a sub-directory that doesn't exist yet.
	setenv(t, "XDG_CONFIG_HOME", filepath.Join(tmp, "nested", "xdg"))

	cfg := &Config{Mounts: []string{"/foo"}}

	// When Save is called
	if err := Save("newproject", cfg); err != nil {
		t.Fatalf("expected Save to create dirs, got error: %v", err)
	}

	// Then the file exists at the expected path
	// Verify the file was actually created.
	path := configPath("newproject")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("expected file at %q to exist after Save", path)
	}
}

// ---------------------------------------------------------------------------
// 6. Save — overwrite
// ---------------------------------------------------------------------------

// TestSave_Overwrite verifies that Save overwrites an existing config file with
// new data, with the second set of mounts returned on the next Load.
func TestSave_Overwrite(t *testing.T) {
	// Given a config file that was previously saved with one set of mounts
	tmp := t.TempDir()
	setenv(t, "XDG_CONFIG_HOME", tmp)

	first := &Config{Mounts: []string{"/old"}}
	if err := Save("overwrite-project", first); err != nil {
		t.Fatalf("first Save failed: %v", err)
	}

	// When Save is called again with different mount data
	second := &Config{Mounts: []string{"/new1", "/new2"}}
	if err := Save("overwrite-project", second); err != nil {
		t.Fatalf("second Save failed: %v", err)
	}

	loaded, err := Load("overwrite-project")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Then the loaded config reflects the second save
	assertMounts(t, loaded, []string{"/new1", "/new2"})
}

// ---------------------------------------------------------------------------
// 7. Delete
// ---------------------------------------------------------------------------

// TestDelete_RemovesConfigFile verifies that Delete removes a previously saved
// config file from disk.
func TestDelete_RemovesConfigFile(t *testing.T) {
	// Given a config file that was previously saved
	tmp := t.TempDir()
	setenv(t, "XDG_CONFIG_HOME", tmp)

	if err := Save("deleteproject", &Config{Mounts: []string{"/data"}}); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	path := configPath("deleteproject")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("expected config file to exist before Delete")
	}

	// When Delete is called
	if err := Delete("deleteproject"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Then the file no longer exists
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected config file to be absent after Delete, but it exists")
	}
}

// TestDelete_NoopWhenFileAbsent verifies that Delete returns nil when the config
// file does not exist (idempotent behaviour).
func TestDelete_NoopWhenFileAbsent(t *testing.T) {
	// Given XDG_CONFIG_HOME points to a temp dir with no config file
	tmp := t.TempDir()
	setenv(t, "XDG_CONFIG_HOME", tmp)

	// When Delete is called for a project that has no config file
	err := Delete("nonexistent")

	// Then no error is returned
	if err != nil {
		t.Errorf("expected nil error for absent config, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 8. EnsureSharedConfigDir
// ---------------------------------------------------------------------------

// TestEnsureSharedConfigDir_XDGOverride verifies that EnsureSharedConfigDir
// returns a path under XDG_CONFIG_HOME when that environment variable is set.
func TestEnsureSharedConfigDir_XDGOverride_ReturnsExpectedPath(t *testing.T) {
	// Given XDG_CONFIG_HOME is set to a temp directory
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	// When EnsureSharedConfigDir is called
	got, err := EnsureSharedConfigDir("copilot")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Then the path is under the XDG_CONFIG_HOME directory
	want := filepath.Join(tmp, "marshal", "copilot")
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
	got, err := EnsureSharedConfigDir("copilot")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Then the path falls back to ~/.config/marshal/<subdir>
	want := filepath.Join(home, ".config", "marshal", "copilot")
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

// TestEnsureSharedConfigDir_Idempotent verifies that EnsureSharedConfigDir
// succeeds when the target directory already exists.
func TestEnsureSharedConfigDir_SucceedsWhenDirAlreadyExists(t *testing.T) {
	// Given XDG_CONFIG_HOME is set and the target directory already exists
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	target := filepath.Join(tmp, "marshal", "copilot")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}

	// When EnsureSharedConfigDir is called
	got, err := EnsureSharedConfigDir("copilot")

	// Then no error is returned and the existing directory path is returned
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != target {
		t.Errorf("expected %q, got %q", target, got)
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
	target := filepath.Join(tmp, "marshal", "copilot")
	if err := os.WriteFile(target, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	// When EnsureSharedConfigDir is called
	_, err := EnsureSharedConfigDir("copilot")

	// Then an error is returned
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "creating shared config directory") {
		t.Errorf("expected 'creating shared config directory' in error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 9. SharedDataPath
// ---------------------------------------------------------------------------

// TestSharedDataPath_XDGOverride verifies that SharedDataPath returns a path
// under XDG_DATA_HOME when that environment variable is set.
func TestSharedDataPath_XDGOverride(t *testing.T) {
	// Given XDG_DATA_HOME is set to a temp directory
	tmp := t.TempDir()
	setenv(t, "XDG_DATA_HOME", tmp)

	// When SharedDataPath is called
	got := sharedDataPath("copilot")
	want := filepath.Join(tmp, "marshal", "copilot")

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
	got := sharedDataPath("config/github-copilot")
	want := filepath.Join(tmp, "marshal", "config", "github-copilot")

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
	got := sharedDataPath("copilot")
	want := filepath.Join(home, ".local", "share", "marshal", "copilot")

	// Then the path falls back to ~/.local/share/marshal/copilot
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

// ---------------------------------------------------------------------------
// 8. EnsureSharedDataDir
// ---------------------------------------------------------------------------

// TestEnsureSharedDataDir_CreatesDir verifies that EnsureSharedDataDir creates
// the directory with mode 0o700 and returns its path.
func TestEnsureSharedDataDir_CreatesDir(t *testing.T) {
	// Given XDG_DATA_HOME is set to a temp directory
	tmp := t.TempDir()
	setenv(t, "XDG_DATA_HOME", tmp)

	// When EnsureSharedDataDir is called
	got, err := EnsureSharedDataDir("copilot")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Then the directory is created with mode 0o700 and its path is returned
	want := filepath.Join(tmp, "marshal", "copilot")
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

	if _, err := EnsureSharedDataDir("copilot"); err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	// When EnsureSharedDataDir is called a second time
	// Then no error is returned
	if _, err := EnsureSharedDataDir("copilot"); err != nil {
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
	got, err := EnsureSharedDataDir("config/github-copilot")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Then all nested directories are created
	if _, err := os.Stat(got); os.IsNotExist(err) {
		t.Errorf("expected nested directory to exist at %q", got)
	}
}

// ---------------------------------------------------------------------------
// 9. Save — error paths
// ---------------------------------------------------------------------------

// TestSave_DirectoryCreationFails verifies that Save returns an error containing
// "creating config directory" when MkdirAll fails.
func TestSave_DirectoryCreationFails(t *testing.T) {
	// Given a file exists at the path where the config directory should be created
	tmp := t.TempDir()
	setenv(t, "XDG_CONFIG_HOME", tmp)

	// Block directory creation by placing a file where the directory should be.
	blockPath := filepath.Join(tmp, "marshal")
	if err := os.WriteFile(blockPath, []byte("block"), 0o644); err != nil {
		t.Fatal(err)
	}

	// When Save is called
	err := Save("myproject", &Config{})

	// Then an error containing "creating config directory" is returned
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "creating config directory") {
		t.Errorf("expected 'creating config directory' in error, got: %v", err)
	}
}

// TestSave_FileCreationFails verifies that Save returns an error when the config
// file path is occupied by a directory, preventing the atomic rename from completing.
// With atomic Save (temp-file+rename), MkdirAll succeeds (parent exists), the temp
// file is created successfully, but os.Rename fails because the target is a directory.
func TestSave_FileCreationFails(t *testing.T) {
	// Given a directory exists at the path where the config file should be created
	tmp := t.TempDir()
	setenv(t, "XDG_CONFIG_HOME", tmp)

	// Block file creation by placing a directory where the config file should be.
	// With atomic save, os.Rename to a directory path fails with EISDIR/ENOTEMPTY.
	blockPath := filepath.Join(tmp, "marshal", "projects", "myproject.toml")
	if err := os.MkdirAll(blockPath, 0o755); err != nil {
		t.Fatal(err)
	}

	// When Save is called
	err := Save("myproject", &Config{})

	// Then an error is returned (the atomic rename fails)
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "committing config file") {
		t.Errorf("expected 'committing config file' in error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 10. EnsureSharedDataDir — error paths
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// ResolveProject — injected getwd
// ---------------------------------------------------------------------------

// TestResolveProject_GetwdFails_PropagatesError verifies that when getwd fails
// and neither a flag value nor MARSHAL_PROJECT is set, the getwd error is
// propagated to the caller rather than silently returning an empty string that
// would cause a misleading "invalid project name" validation error downstream.
func TestResolveProject_GetwdFails_PropagatesError(t *testing.T) {
	// Given MARSHAL_PROJECT is unset and getwd returns an error
	unsetenv(t, "MARSHAL_PROJECT")
	getwdErr := fmt.Errorf("no working directory: filesystem unavailable")
	failingGetwd := func() (string, error) { return "", getwdErr }

	// When ResolveProject is called with an empty flag and failing getwd
	_, err := ResolveProject("", failingGetwd)

	// Then the getwd error is propagated (not swallowed as an empty string)
	if err == nil {
		t.Fatal("expected an error when getwd fails, got nil")
	}
	if !errors.Is(err, getwdErr) {
		t.Errorf("expected error to wrap getwdErr, got: %v", err)
	}
}

// TestLoad_RejectsPathTraversalName verifies that Load returns an error when
// the project name contains path traversal sequences.
func TestLoad_RejectsPathTraversalName(t *testing.T) {
	// Given a project name that contains path traversal sequences
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// When Load is called with the traversal name
	_, err := Load("../../evil")

	// Then an error is returned
	if err == nil {
		t.Error("expected error for path traversal name, got nil")
	}
}

// TestSave_RejectsPathTraversalName verifies that Save returns an error when
// the project name contains path traversal sequences.
func TestSave_RejectsPathTraversalName(t *testing.T) {
	// Given a project name that contains path traversal sequences
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// When Save is called with the traversal name
	err := Save("../../evil", &Config{})

	// Then an error is returned
	if err == nil {
		t.Error("expected error for path traversal name, got nil")
	}
}

// TestLoad_AcceptsValidProjectNames verifies that Load does not return an error
// for a range of valid project name strings.
func TestLoad_AcceptsValidProjectNames(t *testing.T) {
	// Given XDG_CONFIG_HOME is set and a list of valid project name strings
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// When Load is called with each valid name
	// Then no error is returned (missing config file returns empty config, not error)
	for _, name := range []string{"myapp", "my-app", "my_app", "my.app", "a", "MyApp2"} {
		// Load of a non-existent config should return empty config, not an error.
		_, err := Load(name)
		if err != nil {
			t.Errorf("Load(%q) unexpected error: %v", name, err)
		}
	}
}

// TestLoad_RejectsInvalidProjectNames verifies that Load returns an error for
// each entry in a set of invalid project name strings (traversal, slashes, etc.).
func TestLoad_RejectsInvalidProjectNames(t *testing.T) {
	// Given XDG_CONFIG_HOME is set and a list of invalid project name strings
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// When Load is called with each invalid name
	// Then an error is returned for each
	for _, name := range []string{"../../evil", "/etc/passwd", "bad/name", ".hidden", "", "has space"} {
		_, err := Load(name)
		if err == nil {
			t.Errorf("Load(%q) expected error, got nil", name)
		}
	}
}

// ---------------------------------------------------------------------------
// 11. Guard: non-absolute config path (Fix 1)
// ---------------------------------------------------------------------------

// TestLoad_NonAbsoluteConfigPath_ReturnsUnavailableError verifies that Load
// returns an actionable error when the resolved config path is not absolute.
// This happens when xdgBaseDir returns a relative path — e.g. when
// XDG_CONFIG_HOME is set to a relative value, or (in non-CGo environments)
// when both XDG_CONFIG_HOME and HOME are unset and os.UserHomeDir fails.
//
// We deliberately set XDG_CONFIG_HOME to a relative path ("relative") so
// that xdgBaseDir returns it directly, producing a relative ConfigPath.
// This reliably exercises the filepath.IsAbs guard in all build environments.
func TestLoad_NonAbsoluteConfigPath_ReturnsUnavailableError(t *testing.T) {
	// Given XDG_CONFIG_HOME is set to a relative (non-absolute) path
	t.Setenv("XDG_CONFIG_HOME", "relative")
	t.Setenv("HOME", "")

	// When Load is called
	_, err := Load("someproject")

	// Then an error containing "unavailable" is returned
	if err == nil {
		t.Fatal("expected error when config path is not absolute, got nil")
	}
	if !strings.Contains(err.Error(), "unavailable") {
		t.Errorf("expected 'unavailable' in error, got: %v", err)
	}
}

// TestSave_NonAbsoluteConfigPath_ReturnsUnavailableError verifies that Save
// returns an actionable error when the resolved config path is not absolute.
func TestSave_NonAbsoluteConfigPath_ReturnsUnavailableError(t *testing.T) {
	// Given XDG_CONFIG_HOME is set to a relative (non-absolute) path
	t.Setenv("XDG_CONFIG_HOME", "relative")
	t.Setenv("HOME", "")

	// When Save is called
	err := Save("someproject", &Config{})

	// Then an error containing "unavailable" is returned
	if err == nil {
		t.Fatal("expected error when config path is not absolute, got nil")
	}
	if !strings.Contains(err.Error(), "unavailable") {
		t.Errorf("expected 'unavailable' in error, got: %v", err)
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

// ---------------------------------------------------------------------------
// 12. Atomic Save: file is valid after Save (Fix 2)
// ---------------------------------------------------------------------------

// TestSave_AtomicRoundTrip verifies that a completed Save always leaves a
// valid, decodable config file — i.e. there is no observable zero-byte or
// partial-write intermediate state visible to a subsequent Load.
func TestSave_AtomicRoundTrip(t *testing.T) {
	// Given XDG_CONFIG_HOME points to a clean temp directory
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	cfg := &Config{Mounts: []string{"/workspace", "/data"}}

	// When Save is called
	if err := Save("atomic-project", cfg); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Then the file can immediately be decoded back by Load with the same data
	loaded, err := Load("atomic-project")
	if err != nil {
		t.Fatalf("Load after Save failed: %v", err)
	}
	assertMounts(t, loaded, cfg.Mounts)
}

// ---------------------------------------------------------------------------
// Unrecognised TOML fields must be rejected (silent misconfiguration)
// ---------------------------------------------------------------------------

// TestLoad_RejectsOldTableSyntax verifies that Load returns an error when a
// config file uses the old, incorrect schema ([mounts] table with a paths key)
// instead of the correct top-level array (mounts = [...]).
//
// Acceptance criterion: Load must not silently accept files written against the
// old README schema — the TOML decoder's own type checking catches the mismatch
// between a table value and the expected []string, so the user is never silently
// given zero mounts.
func TestLoad_RejectsOldTableSyntax(t *testing.T) {
	// Given a config file written with the old, incorrect schema
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	projectsDir := filepath.Join(tmp, "marshal", "projects")
	if err := os.MkdirAll(projectsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	oldSchema := "[mounts]\npaths = [\"/foo\"]\n"
	if err := os.WriteFile(filepath.Join(projectsDir, "oldschema.toml"), []byte(oldSchema), 0o644); err != nil {
		t.Fatal(err)
	}

	// When Load is called for that project
	_, err := Load("oldschema")

	// Then an error is returned (the TOML library catches the type mismatch
	// between a table and []string — the user is not silently given zero mounts)
	if err == nil {
		t.Fatal("expected an error for old-schema config file, got nil — silent misconfiguration bug is present")
	}
}

// TestLoad_RejectsUnknownTopLevelKey verifies that Load returns an error when
// a config file contains an unknown top-level key.
//
// Acceptance criterion: Any unrecognised field in the config file must cause
// Load to return an error so the user knows their config is malformed.
func TestLoad_RejectsUnknownTopLevelKey(t *testing.T) {
	// Given a config file with an unknown top-level key
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	projectsDir := filepath.Join(tmp, "marshal", "projects")
	if err := os.MkdirAll(projectsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	badConfig := "unknown_key = \"value\"\n"
	if err := os.WriteFile(filepath.Join(projectsDir, "badkeys.toml"), []byte(badConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	// When Load is called for that project
	_, err := Load("badkeys")

	// Then an error is returned containing "unrecognised"
	if err == nil {
		t.Fatal("expected an error for config file with unknown key, got nil")
	}
	if !strings.Contains(err.Error(), "unrecognised") {
		t.Errorf("expected 'unrecognised' in error message, got: %v", err)
	}
}

// TestLoad_ValidConfig_StillWorks verifies that Load continues to parse a
// correctly-formed config file (mounts = [...]) without error after the
// undecoded-key check is added.
//
// Acceptance criterion: A valid config file must still be accepted and its
// mounts returned correctly.
func TestLoad_ValidConfig_StillWorks(t *testing.T) {
	// Given a config file with the correct schema
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	projectsDir := filepath.Join(tmp, "marshal", "projects")
	if err := os.MkdirAll(projectsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	validConfig := "mounts = [\"/foo\", \"/bar\"]\n"
	if err := os.WriteFile(filepath.Join(projectsDir, "validproject.toml"), []byte(validConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	// When Load is called for that project
	cfg, err := Load("validproject")

	// Then no error is returned and the mounts are correctly parsed
	if err != nil {
		t.Fatalf("expected no error for valid config, got: %v", err)
	}
	assertMounts(t, cfg, []string{"/foo", "/bar"})
}

// TestValidateProjectName_RejectsPendingNames verifies that ValidateProjectName
// rejects project names ending with the staging-container suffix pattern
// "-pending-<pid>-<nano>" or "-retiring-<pid>-<nano>". These suffixes are
// reserved for the atomic recreate operation's staging and retiring containers.
//
// Names that end with just "-pending" or "-pending-<digits>" (without a second
// digit group) are now valid project names — the reservation only covers the
// full PID+nanosecond format used by the current implementation.
//
// Names that merely contain "-pending" or "-retiring" as an infix are valid.

// ---------------------------------------------------------------------------
// Project names ending with the staging-container suffix must be rejected
// ---------------------------------------------------------------------------

// names ending with "-pending-<pid>-<nano>" or "-retiring-<pid>-<nano>".
func TestValidateProjectName_RejectsPendingNames(t *testing.T) {
	rejectCases := []string{
		"myapp-pending-123-456789",
		"myapp-retiring-123-456789",
		"a-pending-1-2",
		"foo-retiring-99999-1234567890",
	}
	for _, name := range rejectCases {
		name := name
		t.Run("reject/"+name, func(t *testing.T) {
			err := ValidateProjectName(name)
			if err == nil {
				t.Errorf("ValidateProjectName(%q) returned nil; want error for reserved suffix", name)
			}
		})
	}

	// Names containing "-pending" or "-retiring" as an infix (not the full
	// reserved suffix) are valid.
	allowCases := []string{
		"my-pending-tasks",
		"pending-review",
		"foo-pending-bar",
		"myapp-pending",
		"myapp-pending-123",
		"a-pending",
		"foo-pending-99999",
		"my-retiring-project",
	}
	for _, name := range allowCases {
		name := name
		t.Run("allow/"+name, func(t *testing.T) {
			err := ValidateProjectName(name)
			if err != nil {
				t.Errorf("ValidateProjectName(%q) returned %v; want nil for non-reserved name", name, err)
			}
		})
	}
}
