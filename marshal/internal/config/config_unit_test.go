// SPDX-License-Identifier: AGPL-3.0-or-later
package config_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/config"
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
func assertMounts(t *testing.T, cfg *config.Config, want []string) {
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
	got := config.ResolveProject("flag-project", os.Getwd)

	// Then the flag value is returned
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
	got := config.ResolveProject("", os.Getwd)

	// Then the environment variable value is returned
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
	got := config.ResolveProject("", os.Getwd)

	// Then the basename of the current working directory is returned
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
	got := config.ConfigPath("myproject")
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
	got := config.ConfigPath("myproject")
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
	cfg, err := config.Load("nonexistent-project")

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
	cfg, err := config.Load("myproject")

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
	_, err := config.Load("bad-project")

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

	original := &config.Config{
		Mounts: []string{"/workspace", "/home/user"},
	}

	// When Save is called followed by Load
	if err := config.Save("roundtrip", original); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := config.Load("roundtrip")
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

	cfg := &config.Config{Mounts: []string{"/foo"}}

	// When Save is called
	if err := config.Save("newproject", cfg); err != nil {
		t.Fatalf("expected Save to create dirs, got error: %v", err)
	}

	// Then the file exists at the expected path
	// Verify the file was actually created.
	path := config.ConfigPath("newproject")
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

	first := &config.Config{Mounts: []string{"/old"}}
	if err := config.Save("overwrite-project", first); err != nil {
		t.Fatalf("first Save failed: %v", err)
	}

	// When Save is called again with different mount data
	second := &config.Config{Mounts: []string{"/new1", "/new2"}}
	if err := config.Save("overwrite-project", second); err != nil {
		t.Fatalf("second Save failed: %v", err)
	}

	loaded, err := config.Load("overwrite-project")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Then the loaded config reflects the second save
	assertMounts(t, loaded, []string{"/new1", "/new2"})
}

// ---------------------------------------------------------------------------
// 7. SharedDataPath
// ---------------------------------------------------------------------------

// TestSharedDataPath_XDGOverride verifies that SharedDataPath returns a path
// under XDG_DATA_HOME when that environment variable is set.
func TestSharedDataPath_XDGOverride(t *testing.T) {
	// Given XDG_DATA_HOME is set to a temp directory
	tmp := t.TempDir()
	setenv(t, "XDG_DATA_HOME", tmp)

	// When SharedDataPath is called
	got := config.SharedDataPath("copilot")
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
	got := config.SharedDataPath("config/github-copilot")
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
	got := config.SharedDataPath("copilot")
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
	got, err := config.EnsureSharedDataDir("copilot")
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

	if _, err := config.EnsureSharedDataDir("copilot"); err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	// When EnsureSharedDataDir is called a second time
	// Then no error is returned
	if _, err := config.EnsureSharedDataDir("copilot"); err != nil {
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
	got, err := config.EnsureSharedDataDir("config/github-copilot")
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
	err := config.Save("myproject", &config.Config{})

	// Then an error containing "creating config directory" is returned
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "creating config directory") {
		t.Errorf("expected 'creating config directory' in error, got: %v", err)
	}
}

// TestSave_FileCreationFails verifies that Save returns an error containing
// "creating config file" when file creation is blocked.
func TestSave_FileCreationFails(t *testing.T) {
	// Given a directory exists at the path where the config file should be created
	tmp := t.TempDir()
	setenv(t, "XDG_CONFIG_HOME", tmp)

	// Block file creation by placing a directory where the config file should be.
	// os.Create on a directory path fails with EISDIR even when running as root.
	blockPath := filepath.Join(tmp, "marshal", "projects", "myproject.toml")
	if err := os.MkdirAll(blockPath, 0o755); err != nil {
		t.Fatal(err)
	}

	// When Save is called
	err := config.Save("myproject", &config.Config{})

	// Then an error containing "creating config file" is returned
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "creating config file") {
		t.Errorf("expected 'creating config file' in error, got: %v", err)
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
	_, err := config.EnsureSharedDataDir("credentials")

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

// TestResolveProject_GetwdFails verifies that ResolveProject returns an empty
// string when getwd fails and neither a flag value nor MARSHAL_PROJECT is set.
func TestResolveProject_GetwdFails(t *testing.T) {
	// Given MARSHAL_PROJECT is unset and getwd returns an error
	unsetenv(t, "MARSHAL_PROJECT")

	failingGetwd := func() (string, error) { return "", fmt.Errorf("no working directory") }

	// When ResolveProject is called with an empty flag and failing getwd
	got := config.ResolveProject("", failingGetwd)

	// Then an empty string is returned
	if got != "" {
		t.Errorf("expected empty string on getwd failure, got %q", got)
	}
}

// TestLoad_RejectsPathTraversalName verifies that Load returns an error when
// the project name contains path traversal sequences.
func TestLoad_RejectsPathTraversalName(t *testing.T) {
	// Given a project name that contains path traversal sequences
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// When Load is called with the traversal name
	_, err := config.Load("../../evil")

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
	err := config.Save("../../evil", &config.Config{})

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
		_, err := config.Load(name)
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
		_, err := config.Load(name)
		if err == nil {
			t.Errorf("Load(%q) expected error, got nil", name)
		}
	}
}
