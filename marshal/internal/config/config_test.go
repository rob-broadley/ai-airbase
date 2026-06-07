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
// Project name resolution
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
// Load — file missing
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
// Load — file exists with mounts
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
// Save — round-trip
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
// Save — overwrite
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
// Delete
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

// TestDelete_InvalidProjectName_ReturnsValidationError verifies that Delete
// returns an error immediately when the project name is invalid, without
// attempting any filesystem operation.
func TestDelete_InvalidProjectName_ReturnsValidationError(t *testing.T) {
	// Given XDG_CONFIG_HOME is set to a temp directory
	tmp := t.TempDir()
	setenv(t, "XDG_CONFIG_HOME", tmp)

	// When Delete is called with a path-traversal project name
	err := Delete("../../evil")

	// Then an error is returned (validation fails before any filesystem access)
	if err == nil {
		t.Error("expected an error for invalid project name, got nil")
	}
}

// TestDelete_RemoveFailsWithNonErrNotExist_ReturnsRemovingProjectConfigError
// verifies that Delete propagates an os.Remove error (other than ErrNotExist)
// as an error whose message contains "removing project config".
func TestDelete_RemoveFailsWithNonErrNotExist_ReturnsRemovingProjectConfigError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping permission test: running as root bypasses directory permission checks")
	}

	// Given XDG_CONFIG_HOME is set to a temp directory, a config file exists for
	// "perm-delete-project", and the parent projects directory has permissions
	// set to 0o000, making os.Remove fail with EACCES
	tmp := t.TempDir()
	setenv(t, "XDG_CONFIG_HOME", tmp)

	if err := Save("perm-delete-project", &Config{}); err != nil {
		t.Fatalf("Save failed during setup: %v", err)
	}

	projDir := filepath.Join(tmp, "marshal", "projects")
	if err := os.Chmod(projDir, 0o000); err != nil {
		t.Fatalf("Chmod failed during setup: %v", err)
	}
	t.Cleanup(func() { os.Chmod(projDir, 0o755) }) //nolint:errcheck

	// When Delete is called
	err := Delete("perm-delete-project")

	// Then an error is returned whose message contains "removing project config"
	if err == nil {
		t.Fatal("expected an error when os.Remove fails, got nil")
	}
	if !strings.Contains(err.Error(), "removing project config") {
		t.Errorf("expected 'removing project config' in error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Save — error paths
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

// TestSave_MkdirAllFailsAtProjectsPath_ReturnsCreatingConfigDirectoryError
// verifies that Save returns an error containing "creating config directory"
// when os.MkdirAll cannot create the projects directory because a regular file
// already occupies that exact path.
func TestSave_MkdirAllFailsAtProjectsPath_ReturnsCreatingConfigDirectoryError(t *testing.T) {
	// Given XDG_CONFIG_HOME is set to a temp directory and a regular file exists
	// at "<tmp>/marshal/projects" — the exact path MkdirAll would create as a directory
	tmp := t.TempDir()
	setenv(t, "XDG_CONFIG_HOME", tmp)

	marshallDir := filepath.Join(tmp, "marshal")
	if err := os.MkdirAll(marshallDir, 0o755); err != nil {
		t.Fatal(err)
	}
	blockPath := filepath.Join(marshallDir, "projects")
	if err := os.WriteFile(blockPath, []byte("block"), 0o644); err != nil {
		t.Fatal(err)
	}

	// When Save is called
	err := Save("mkdirall-block-project", &Config{})

	// Then an error containing "creating config directory" is returned
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "creating config directory") {
		t.Errorf("expected 'creating config directory' in error, got: %v", err)
	}
}

// TestSave_ReadOnlyProjectsDir_ReturnsCreatingTempConfigFileError verifies that
// Save returns an error containing "creating temp config file" when the projects
// directory exists but is not writable, causing os.CreateTemp to fail.
func TestSave_ReadOnlyProjectsDir_ReturnsCreatingTempConfigFileError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping permission test: running as root bypasses directory permission checks")
	}

	// Given XDG_CONFIG_HOME is set to a temp directory, the projects directory has
	// been created successfully, and its permissions are then set to 0o555
	// (readable/executable but not writable) — preventing os.CreateTemp from
	// creating a temp file inside it
	tmp := t.TempDir()
	setenv(t, "XDG_CONFIG_HOME", tmp)

	projDir := filepath.Join(tmp, "marshal", "projects")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(projDir, 0o555); err != nil {
		t.Fatalf("Chmod failed during setup: %v", err)
	}
	t.Cleanup(func() { os.Chmod(projDir, 0o755) }) //nolint:errcheck

	// When Save is called
	err := Save("readonly-dir-project", &Config{})

	// Then an error containing "creating temp config file" is returned
	if err == nil {
		t.Fatal("expected an error when the projects directory is not writable, got nil")
	}
	if !strings.Contains(err.Error(), "creating temp config file") {
		t.Errorf("expected 'creating temp config file' in error, got: %v", err)
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
// Guard: non-absolute config path (Fix 1)
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

// ---------------------------------------------------------------------------
// Atomic Save: file is valid after Save (Fix 2)
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

// ---------------------------------------------------------------------------
// Helpers for masks assertions
// ---------------------------------------------------------------------------

// assertMasks verifies that cfg.Masks exactly matches want, element by element,
// and that the slice is non-nil even when want is empty.
func assertMasks(t *testing.T, cfg *Config, want []string) {
	t.Helper()
	if cfg.Masks == nil {
		t.Fatal("expected non-nil Masks slice, got nil")
	}
	if len(cfg.Masks) != len(want) {
		t.Fatalf("expected %d masks, got %d: %v", len(want), len(cfg.Masks), cfg.Masks)
	}
	for i, m := range want {
		if cfg.Masks[i] != m {
			t.Errorf("mask[%d]: expected %q, got %q", i, m, cfg.Masks[i])
		}
	}
}

// makeProjectsDir creates a temporary XDG_CONFIG_HOME, registers t.Setenv for
// cleanup, creates the marshal/projects subdirectory with permissions 0o755,
// and returns its absolute path. It is used by ListProjects tests that need a
// writable projects directory to exist before calling the function.
func makeProjectsDir(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	projDir := filepath.Join(tmp, "marshal", "projects")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return projDir
}

// ---------------------------------------------------------------------------
// Masks — config persistence
// ---------------------------------------------------------------------------

// TestLoad_BackwardCompatible_NoMasksKey verifies that a TOML config without a
// masks key loads cleanly, returning an empty (non-nil) Masks slice.
func TestLoad_BackwardCompatible_NoMasksKey(t *testing.T) {
	// Given a TOML config file that contains mounts but no masks key
	tmp := t.TempDir()
	setenv(t, "XDG_CONFIG_HOME", tmp)

	projectsDir := filepath.Join(tmp, "marshal", "projects")
	if err := os.MkdirAll(projectsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tomlContent := `mounts = ["/home/user/myapp"]` + "\n"
	if err := os.WriteFile(filepath.Join(projectsDir, "myapp.toml"), []byte(tomlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// When the config is loaded
	cfg, err := Load("myapp")

	// Then Mounts equals ["/home/user/myapp"], Masks is an empty non-nil slice, and no error is returned
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	assertMounts(t, cfg, []string{"/home/user/myapp"})
	assertMasks(t, cfg, []string{})
}

// TestLoad_MasksPresent verifies that masks listed in the TOML config are loaded
// in order into the Masks field.
func TestLoad_MasksPresent(t *testing.T) {
	// Given a TOML config file containing a masks key
	tmp := t.TempDir()
	setenv(t, "XDG_CONFIG_HOME", tmp)

	projectsDir := filepath.Join(tmp, "marshal", "projects")
	if err := os.MkdirAll(projectsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tomlContent := "masks = [\".venv\", \"node_modules\"]\n"
	if err := os.WriteFile(filepath.Join(projectsDir, "masked-project.toml"), []byte(tomlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// When the config is loaded
	cfg, err := Load("masked-project")

	// Then Masks equals [".venv", "node_modules"] in that order
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	assertMasks(t, cfg, []string{".venv", "node_modules"})
}

// TestLoad_MasksRoundTrip verifies that masks saved via Save are returned
// unchanged by a subsequent Load.
func TestLoad_MasksRoundTrip(t *testing.T) {
	// Given Config{Masks: [".venv", "node_modules"]} is saved for project "myapp"
	tmp := t.TempDir()
	setenv(t, "XDG_CONFIG_HOME", tmp)

	original := &Config{Masks: []string{".venv", "node_modules"}}
	if err := Save("myapp", original); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// When the config is loaded back for project "myapp"
	loaded, err := Load("myapp")

	// Then Masks equals [".venv", "node_modules"]
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	assertMasks(t, loaded, []string{".venv", "node_modules"})
}

// TestLoad_AbsentFile_MasksEmptySlice verifies that a completely absent config
// file returns an empty (non-nil) Masks slice and no error.
func TestLoad_AbsentFile_MasksEmptySlice(t *testing.T) {
	// Given a project that has no config file on disk
	tmp := t.TempDir()
	setenv(t, "XDG_CONFIG_HOME", tmp)

	// When the config is loaded
	cfg, err := Load("no-such-project")

	// Then Masks is an empty slice and no error is returned
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	assertMasks(t, cfg, []string{})
}

// TestLoad_UnrecognisedKey_ReturnsError verifies that a TOML config containing
// an unrecognised key causes Load to return an error naming the bad field.
func TestLoad_UnrecognisedKey_ReturnsError(t *testing.T) {
	// Given a TOML config file containing the unrecognised key "masqs"
	tmp := t.TempDir()
	setenv(t, "XDG_CONFIG_HOME", tmp)

	projectsDir := filepath.Join(tmp, "marshal", "projects")
	if err := os.MkdirAll(projectsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tomlContent := "masqs = [\".venv\"]\n"
	if err := os.WriteFile(filepath.Join(projectsDir, "typo-project.toml"), []byte(tomlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// When the config is loaded
	_, err := Load("typo-project")

	// Then an error is returned identifying the unrecognised field name
	if err == nil {
		t.Fatal("expected an error for unrecognised key, got nil")
	}
	if !strings.Contains(err.Error(), "masqs") {
		t.Errorf("expected error to mention %q, got: %v", "masqs", err)
	}
}

// TestLoad_ZeroMaskRoundTrip verifies that saving a Config with nil Masks and
// loading it back yields an empty (non-nil) Masks slice.
func TestLoad_ZeroMaskRoundTrip(t *testing.T) {
	// Given Config{Mounts: ["/home/user/myapp"], Masks: nil} is saved
	tmp := t.TempDir()
	setenv(t, "XDG_CONFIG_HOME", tmp)

	original := &Config{Mounts: []string{"/home/user/myapp"}, Masks: nil}
	if err := Save("zero-mask-project", original); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// When the saved file is loaded back
	loaded, err := Load("zero-mask-project")

	// Then Masks is an empty slice and no error is returned
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	assertMasks(t, loaded, []string{})
}

// ---------------------------------------------------------------------------
// ListProjects
// ---------------------------------------------------------------------------

// TestListProjects_ProjectsDirAbsent_ReturnsEmptySliceNoError verifies that
// ListProjects returns an empty (non-nil) names slice, nil warnings, and nil
// error when the projects configuration directory does not exist.
func TestListProjects_ProjectsDirAbsent_ReturnsEmptySliceNoError(t *testing.T) {
	// Given XDG_CONFIG_HOME is set to a temp directory with no marshal/projects subdirectory
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// When ListProjects is called
	names, warnings, err := ListProjects()

	// Then an empty (non-nil) names slice, nil warnings, and nil error are returned
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if warnings != nil {
		t.Errorf("expected nil warnings, got: %v", warnings)
	}
	if names == nil {
		t.Fatal("expected non-nil names slice, got nil")
	}
	if len(names) != 0 {
		t.Errorf("expected empty names slice, got: %v", names)
	}
}

// TestListProjects_ProjectsDirEmpty_ReturnsEmptySliceNoError verifies that
// ListProjects returns an empty (non-nil) names slice, nil warnings, and nil
// error when the projects directory exists but contains no files.
func TestListProjects_ProjectsDirEmpty_ReturnsEmptySliceNoError(t *testing.T) {
	// Given XDG_CONFIG_HOME is set to a temp directory and the marshal/projects directory exists but is empty
	makeProjectsDir(t)

	// When ListProjects is called
	names, warnings, err := ListProjects()

	// Then an empty (non-nil) names slice, nil warnings, and nil error are returned
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if warnings != nil {
		t.Errorf("expected nil warnings, got: %v", warnings)
	}
	if names == nil {
		t.Fatal("expected non-nil names slice, got nil")
	}
	if len(names) != 0 {
		t.Errorf("expected empty names slice, got: %v", names)
	}
}

// TestListProjects_NonTomlFile_SkipsFile verifies that ListProjects ignores
// files that do not have the .toml extension — they must not appear in names.
func TestListProjects_NonTomlFile_SkipsFile(t *testing.T) {
	// Given XDG_CONFIG_HOME is set to a temp directory and the projects directory contains only a file named "README.md"
	projDir := makeProjectsDir(t)
	if err := os.WriteFile(filepath.Join(projDir, "README.md"), []byte("docs"), 0o644); err != nil {
		t.Fatal(err)
	}

	// When ListProjects is called
	names, warnings, err := ListProjects()

	// Then names does not contain "README.md" or "README", and no error is returned
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if warnings != nil {
		t.Errorf("expected nil warnings, got: %v", warnings)
	}
	for _, n := range names {
		if n == "README.md" || n == "README" {
			t.Errorf("expected non-.toml file to be skipped, but found %q in names", n)
		}
	}
}

// TestListProjects_Subdirectory_SkipsDirectoryEntry verifies that ListProjects
// ignores directory entries — they must not appear in names.
// The directory is named "myproject.toml" so only the non-regular-entry guard
// (not the suffix check) is responsible for excluding it.
func TestListProjects_Subdirectory_SkipsDirectoryEntry(t *testing.T) {
	// Given XDG_CONFIG_HOME is set to a temp directory and the projects directory contains only a subdirectory named "myproject.toml"
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	projDir := filepath.Join(tmp, "marshal", "projects")
	if err := os.MkdirAll(filepath.Join(projDir, "myproject.toml"), 0o755); err != nil {
		t.Fatal(err)
	}

	// When ListProjects is called
	names, warnings, err := ListProjects()

	// Then names does not contain "myproject", and no error or warnings are returned
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if warnings != nil {
		t.Errorf("expected nil warnings, got: %v", warnings)
	}
	for _, n := range names {
		if n == "myproject" {
			t.Errorf("expected subdirectory to be skipped, but found %q in names", n)
		}
	}
}

// TestListProjects_Symlink_SkipsSymlink verifies that ListProjects ignores
// symlinks even when they point to a valid .toml file.
func TestListProjects_Symlink_SkipsSymlink(t *testing.T) {
	// Given the projects directory contains a symlink named "linked.toml" pointing to a valid .toml file
	projDir := makeProjectsDir(t)
	targetDir := t.TempDir()
	targetPath := filepath.Join(targetDir, "linked.toml")
	if err := os.WriteFile(targetPath, []byte("mounts = []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(projDir, "linked.toml")
	if err := os.Symlink(targetPath, linkPath); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}

	// When ListProjects is called
	names, warnings, err := ListProjects()

	// Then names does not contain "linked", and no error or warnings are returned
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if warnings != nil {
		t.Errorf("expected nil warnings, got: %v", warnings)
	}
	for _, n := range names {
		if n == "linked" {
			t.Errorf("expected symlink to be skipped, but found %q in names", n)
		}
	}
}

// TestListProjects_ValidTomlFiles_ReturnsNamesInLexicographicOrder verifies
// that ListProjects returns bare project names (no path, no extension) in
// lexicographic order and produces nil warnings and nil error.
func TestListProjects_ValidTomlFiles_ReturnsNamesInLexicographicOrder(t *testing.T) {
	// Given XDG_CONFIG_HOME is set to a temp directory and the projects directory contains three valid .toml files: "zebra.toml", "alpha.toml", "middle.toml"
	projDir := makeProjectsDir(t)
	for _, name := range []string{"zebra", "alpha", "middle"} {
		if err := os.WriteFile(filepath.Join(projDir, name+".toml"), []byte(""), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// When ListProjects is called
	names, warnings, err := ListProjects()

	// Then names equals ["alpha", "middle", "zebra"] (bare stems, no extension, in lexicographic order), warnings is nil, and error is nil
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if warnings != nil {
		t.Errorf("expected nil warnings, got: %v", warnings)
	}
	want := []string{"alpha", "middle", "zebra"}
	if len(names) != len(want) {
		t.Fatalf("expected %d names, got %d: %v", len(want), len(names), names)
	}
	for i, w := range want {
		if names[i] != w {
			t.Errorf("names[%d]: expected %q, got %q", i, w, names[i])
		}
	}
}

// TestListProjects_InvalidStemTomlFile_ReturnsWarningExcludesName verifies
// that a .toml file whose stem is not a valid project name (e.g. ".hidden.toml")
// is excluded from names and produces a warning message containing "skipping".
func TestListProjects_InvalidStemTomlFile_ReturnsWarningExcludesName(t *testing.T) {
	// Given XDG_CONFIG_HOME is set to a temp directory and the projects directory contains a file named ".hidden.toml"
	projDir := makeProjectsDir(t)
	if err := os.WriteFile(filepath.Join(projDir, ".hidden.toml"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	// When ListProjects is called
	names, warnings, err := ListProjects()

	// Then names does not contain ".hidden", warnings contains at least one entry with the substring "skipping", and error is nil
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	for _, n := range names {
		if n == ".hidden" {
			t.Errorf("expected invalid stem to be excluded from names, but found %q", n)
		}
	}
	if len(warnings) == 0 {
		t.Fatal("expected at least one warning, got none")
	}
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "skipping") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a warning containing %q, got: %v", "skipping", warnings)
	}
}

// TestListProjects_UnreadableDir_ReturnsReadError verifies that ListProjects
// returns nil names, nil warnings, and an error containing "reading projects
// directory" when the projects directory exists but cannot be read.
func TestListProjects_UnreadableDir_ReturnsReadError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping unreadable-directory test: running as root can read any directory")
	}

	// Given XDG_CONFIG_HOME is set to a temp directory, the projects directory exists, and its permissions are set to 0o000
	projDir := makeProjectsDir(t)
	if err := os.Chmod(projDir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(projDir, 0o755) }) //nolint:errcheck

	// When ListProjects is called
	names, warnings, err := ListProjects()

	// Then names is nil, warnings is nil, and error contains the substring "reading projects directory"
	if err == nil {
		t.Fatal("expected an error for unreadable directory, got nil")
	}
	if !strings.Contains(err.Error(), "reading projects directory") {
		t.Errorf("expected error to contain %q, got: %v", "reading projects directory", err)
	}
	if names != nil {
		t.Errorf("expected nil names, got: %v", names)
	}
	if warnings != nil {
		t.Errorf("expected nil warnings, got: %v", warnings)
	}
}

// ---------------------------------------------------------------------------
// xdgBaseDir — home directory unavailable
// ---------------------------------------------------------------------------
//
// TestXdgConfigHome_BothEnvAndHomeMissing_ReturnsEmptyString moved to
// hostinfo/xdg_test.go (covers hostinfo.XDGConfigHome, the new home of
// this logic).
