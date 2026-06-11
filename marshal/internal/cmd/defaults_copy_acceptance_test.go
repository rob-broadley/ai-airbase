// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/cmd"
)

// ---------------------------------------------------------------------------
// Defaults copy — integration of CopyDefaults into ensureHostState
// ---------------------------------------------------------------------------

// TestCreate_DefaultsCopiedToProject verifies that running marshal create
// copies defaults files into the per-project config directory.
func TestCreate_DefaultsCopiedToProject(t *testing.T) {
	// Given a user defaults tree containing config/auth.json
	base := t.TempDir()
	t.Setenv("XDG_DATA_HOME", base)

	writeDefaultsFile(t, filepath.Join(base, "marshal", "defaults", "opencode", "config", "auth.json"),
		[]byte(`{"token":"abc"}`))

	cf := newCredFakes(t)
	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := newCredentialTestDeps(cf, runner)

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp", "create"})

	// When marshal create is run for project "myapp"
	assertNoError(t, root.Execute())

	// Then config/auth.json exists in the per-project config directory
	got, err := os.ReadFile(filepath.Join(cf.dataBase, "projects", "myapp", "opencode", "config", "auth.json"))
	if err != nil {
		t.Fatalf("expected config/auth.json to exist in per-project dir: %v", err)
	}
	if string(got) != `{"token":"abc"}` {
		t.Errorf("config/auth.json content mismatch: got %q, want %q", got, `{"token":"abc"}`)
	}
}

// TestCreate_ExistingProjectFileNotOverwritten verifies that marshal create
// never overwrites a pre-existing per-project file (never-overwrite semantics).
func TestCreate_ExistingProjectFileNotOverwritten(t *testing.T) {
	// Given a user defaults tree containing config/auth.json and a per-project
	// directory that already contains config/auth.json with different content
	base := t.TempDir()
	t.Setenv("XDG_DATA_HOME", base)

	writeDefaultsFile(t, filepath.Join(base, "marshal", "defaults", "opencode", "config", "auth.json"),
		[]byte(`{"token":"new"}`))

	cf := newCredFakes(t)
	existingContent := []byte(`{"token":"existing"}`)
	existingFile := filepath.Join(cf.dataBase, "projects", "myapp", "opencode", "config", "auth.json")
	writeDefaultsFile(t, existingFile, existingContent)

	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := newCredentialTestDeps(cf, runner)

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp", "create"})

	// When marshal create is run for the project
	assertNoError(t, root.Execute())

	// Then the existing per-project config/auth.json is unchanged
	got, err := os.ReadFile(existingFile)
	if err != nil {
		t.Fatalf("expected to read existing file: %v", err)
	}
	if string(got) != string(existingContent) {
		t.Errorf("existing file was overwritten: got %q, want %q", got, existingContent)
	}
}

// TestCreate_EmptyDefaults_NoFilesCopied verifies that marshal create with
// an empty defaults tree creates the per-project dirs but copies no files.
func TestCreate_EmptyDefaults_NoFilesCopied(t *testing.T) {
	// Given an empty user defaults tree (no files in defaults)
	base := t.TempDir()
	t.Setenv("XDG_DATA_HOME", base)

	cf := newCredFakes(t)
	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := newCredentialTestDeps(cf, runner)

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp", "create"})

	// When marshal create is run for a new project
	assertNoError(t, root.Execute())

	// Then the per-project directory is created with the three subdirs
	projectBase := filepath.Join(cf.dataBase, "projects", "myapp", "opencode")
	for _, sub := range []string{"config", "share", "state"} {
		info, err := os.Stat(filepath.Join(projectBase, sub))
		if err != nil {
			t.Errorf("expected subdirectory %s to exist: %v", sub, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("expected %s to be a directory", sub)
		}
	}

	// And no files are copied (no-op)
	var files []string
	err := filepath.Walk(projectBase, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("error walking project directory: %v", err)
	}
	if len(files) > 0 {
		t.Errorf("expected no files copied from empty defaults, found: %v", files)
	}
}

// TestCreate_SymlinkEscape_AbortsWithSecurityViolation verifies that marshal
// create aborts when the defaults tree contains a symlink escaping the source root.
func TestCreate_SymlinkEscape_AbortsWithSecurityViolation(t *testing.T) {
	// Given a user defaults tree containing a symlink whose target escapes the source root
	base := t.TempDir()
	t.Setenv("XDG_DATA_HOME", base)

	defaultsBase := filepath.Join(base, "marshal", "defaults", "opencode")
	writeDefaultsFile(t, filepath.Join(defaultsBase, "config", "real.json"), []byte(`{}`))
	outsideFile := filepath.Join(t.TempDir(), "outside.json")
	writeDefaultsFile(t, outsideFile, []byte(`evil`))
	symlinkPath := filepath.Join(defaultsBase, "config", "escape.json")
	if err := os.Symlink(outsideFile, symlinkPath); err != nil {
		t.Fatal(err)
	}

	cf := newCredFakes(t)
	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := newCredentialTestDeps(cf, runner)

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp", "create"})

	// When marshal create is run for a project
	err := root.Execute()

	// Then marshal create aborts with an error containing "security violation"
	if err == nil {
		t.Fatal("expected Execute() to return error due to symlink escaping source root")
	}
	if !containsSubstr(err.Error(), "security violation") {
		t.Errorf("expected error to contain %q, got: %q", "security violation", err.Error())
	}
}

// TestRecreate_DefaultsCopiedAndExistingUntouched verifies that marshal
// recreate copies defaults files while leaving pre-existing per-project files untouched.
func TestRecreate_DefaultsCopiedAndExistingUntouched(t *testing.T) {
	// Given a user defaults tree containing config/settings.json and an
	// existing per-project directory with config/existing.json
	base := t.TempDir()
	t.Setenv("XDG_DATA_HOME", base)

	writeDefaultsFile(t, filepath.Join(base, "marshal", "defaults", "opencode", "config", "settings.json"),
		[]byte(`{"theme":"dark"}`))

	cf := newCredFakes(t)
	existingFile := filepath.Join(cf.dataBase, "projects", "myapp", "opencode", "config", "existing.json")
	existingContent := []byte(`{"custom":"value"}`)
	writeDefaultsFile(t, existingFile, existingContent)

	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := newCredentialTestDeps(cf, runner)

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp", "recreate"})

	// When marshal recreate is run for the project
	assertNoError(t, root.Execute())

	// Then config/settings.json is present in the per-project config directory
	got, err := os.ReadFile(filepath.Join(cf.dataBase, "projects", "myapp", "opencode", "config", "settings.json"))
	if err != nil {
		t.Fatalf("expected config/settings.json to exist: %v", err)
	}
	if string(got) != `{"theme":"dark"}` {
		t.Errorf("settings.json content mismatch: got %q, want %q", got, `{"theme":"dark"}`)
	}

	// And pre-existing per-project files are untouched
	gotExisting, err := os.ReadFile(existingFile)
	if err != nil {
		t.Fatalf("expected existing file to still exist: %v", err)
	}
	if string(gotExisting) != string(existingContent) {
		t.Errorf("existing file was modified: got %q, want %q", gotExisting, existingContent)
	}
}

// TestCreate_DefaultsAndExistingCoexist verifies that when defaults contains
// config/shared.json and the per-project dir already has config/project-specific.json,
// both files exist after marshal create.
func TestCreate_DefaultsAndExistingCoexist(t *testing.T) {
	// Given a user defaults tree containing config/shared.json and a per-project
	// directory that already has config/project-specific.json
	base := t.TempDir()
	t.Setenv("XDG_DATA_HOME", base)

	writeDefaultsFile(t, filepath.Join(base, "marshal", "defaults", "opencode", "config", "shared.json"),
		[]byte(`{"from":"defaults"}`))

	cf := newCredFakes(t)
	projectFile := filepath.Join(cf.dataBase, "projects", "myapp", "opencode", "config", "project-specific.json")
	projectContent := []byte(`{"from":"project"}`)
	writeDefaultsFile(t, projectFile, projectContent)

	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := newCredentialTestDeps(cf, runner)

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp", "create"})

	// When marshal create is run
	assertNoError(t, root.Execute())

	// Then both config/shared.json (from defaults) and config/project-specific.json (pre-existing) exist
	gotShared, err := os.ReadFile(filepath.Join(cf.dataBase, "projects", "myapp", "opencode", "config", "shared.json"))
	if err != nil {
		t.Fatalf("expected shared.json to exist: %v", err)
	}
	if string(gotShared) != `{"from":"defaults"}` {
		t.Errorf("shared.json content mismatch: got %q", gotShared)
	}

	gotProject, err := os.ReadFile(projectFile)
	if err != nil {
		t.Fatalf("expected project-specific.json to exist: %v", err)
	}
	if string(gotProject) != string(projectContent) {
		t.Errorf("project-specific.json was modified: got %q, want %q", gotProject, projectContent)
	}
}

// TestDefaultCmd_DefaultsCopiedToProject verifies that the bare marshal
// command (default command) also copies defaults files into the per-project directory.
func TestDefaultCmd_DefaultsCopiedToProject(t *testing.T) {
	// Given a user defaults tree containing config/new.json
	base := t.TempDir()
	t.Setenv("XDG_DATA_HOME", base)

	writeDefaultsFile(t, filepath.Join(base, "marshal", "defaults", "opencode", "config", "new.json"),
		[]byte(`{"added":"new"}`))

	cf := newCredFakes(t)
	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := newCredentialTestDeps(cf, runner)

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp"})

	// When bare marshal (default command) is run for the project
	assertNoError(t, root.Execute())

	// Then config/new.json exists in the per-project config directory
	got, err := os.ReadFile(filepath.Join(cf.dataBase, "projects", "myapp", "opencode", "config", "new.json"))
	if err != nil {
		t.Fatalf("expected config/new.json to exist: %v", err)
	}
	if string(got) != `{"added":"new"}` {
		t.Errorf("new.json content mismatch: got %q", got)
	}
}

// TestCreate_IdempotentRunningTwice verifies that running marshal create
// twice copies new defaults files on the second run while leaving
// already-copied files unchanged.
func TestCreate_IdempotentRunningTwice(t *testing.T) {
	// Given a user defaults tree containing config/old.json already present
	// in the per-project directory and a new config/new.json added to defaults
	base := t.TempDir()
	t.Setenv("XDG_DATA_HOME", base)

	writeDefaultsFile(t, filepath.Join(base, "marshal", "defaults", "opencode", "config", "old.json"),
		[]byte(`{"from":"defaults"}`))
	writeDefaultsFile(t, filepath.Join(base, "marshal", "defaults", "opencode", "config", "new.json"),
		[]byte(`{"added":"new"}`))

	cf := newCredFakes(t)
	oldFile := filepath.Join(cf.dataBase, "projects", "myapp", "opencode", "config", "old.json")
	oldContent := []byte(`{"existing":"kept"}`)
	writeDefaultsFile(t, oldFile, oldContent)

	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := newCredentialTestDeps(cf, runner)

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp", "create"})

	// When marshal create is run for the same project
	assertNoError(t, root.Execute())

	// Then the already-copied file (old.json) is unchanged
	gotOld, err := os.ReadFile(oldFile)
	if err != nil {
		t.Fatalf("expected old.json to exist: %v", err)
	}
	if string(gotOld) != string(oldContent) {
		t.Errorf("old.json was modified: got %q, want %q", gotOld, oldContent)
	}

	// And the new defaults file (new.json) is copied
	gotNew, err := os.ReadFile(filepath.Join(cf.dataBase, "projects", "myapp", "opencode", "config", "new.json"))
	if err != nil {
		t.Fatalf("expected new.json to exist: %v", err)
	}
	if string(gotNew) != `{"added":"new"}` {
		t.Errorf("new.json content mismatch: got %q", gotNew)
	}
}

// TestCreate_CopyFailure_AbortsWithError verifies that marshal create aborts
// with an error when the copy of a defaults file fails (e.g. read-only destination).
func TestCreate_CopyFailure_AbortsWithError(t *testing.T) {
	// Given a user defaults tree containing config/auth.json
	base := t.TempDir()
	t.Setenv("XDG_DATA_HOME", base)

	writeDefaultsFile(t, filepath.Join(base, "marshal", "defaults", "opencode", "config", "auth.json"),
		[]byte(`{"token":"abc"}`))

	cf := newCredFakes(t)
	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := newCredentialTestDeps(cf, runner)

	// Pre-create the per-project config directory with read-only permissions
	// so that the copy of config/auth.json fails
	projectDir := filepath.Join(cf.dataBase, "projects", "myapp", "opencode")
	if err := os.MkdirAll(projectDir, 0o700); err != nil {
		t.Fatal(err)
	}
	configDir := filepath.Join(projectDir, "config")
	if err := os.MkdirAll(configDir, 0o500); err != nil {
		t.Fatal(err)
	}
	// Restore permissions after test so t.TempDir cleanup works
	t.Cleanup(func() { _ = os.Chmod(configDir, 0o700) })

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp", "create"})

	// When marshal create is run and the copy fails
	err := root.Execute()

	// Then marshal create aborts with a non-nil error
	if err == nil {
		t.Fatal("expected Execute() to return error due to copy failure")
	}
}

// TestCreate_SymlinkPreservation_CopiesSymlinkAsSymlink verifies that marshal
// create preserves relative symlinks in the defaults tree as symlinks in the
// per-project directory, not as resolved file content.
func TestCreate_SymlinkPreservation_CopiesSymlinkAsSymlink(t *testing.T) {
	// Given a user defaults tree containing a relative symlink config/link.json -> real.json
	base := t.TempDir()
	t.Setenv("XDG_DATA_HOME", base)

	defaultsBase := filepath.Join(base, "marshal", "defaults", "opencode")
	realContent := []byte(`{"real":"data"}`)
	writeDefaultsFile(t, filepath.Join(defaultsBase, "config", "real.json"), realContent)
	symlinkPath := filepath.Join(defaultsBase, "config", "link.json")
	if err := os.Symlink("real.json", symlinkPath); err != nil {
		t.Fatal(err)
	}

	cf := newCredFakes(t)
	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := newCredentialTestDeps(cf, runner)

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp", "create"})

	// When marshal create is run for a project
	assertNoError(t, root.Execute())

	// Then config/link.json at the per-project directory is a symlink
	projectLink := filepath.Join(cf.dataBase, "projects", "myapp", "opencode", "config", "link.json")
	info, err := os.Lstat(projectLink)
	if err != nil {
		t.Fatalf("expected config/link.json to exist: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected config/link.json to be a symlink, got mode %v", info.Mode())
	}

	// And os.Readlink on the destination symlink returns real.json
	gotTarget, err := os.Readlink(projectLink)
	if err != nil {
		t.Fatalf("expected to read symlink target: %v", err)
	}
	if gotTarget != "real.json" {
		t.Errorf("symlink target = %q, want %q", gotTarget, "real.json")
	}

	// And reading through the destination symlink produces the same content as source real.json
	gotContent, err := os.ReadFile(projectLink)
	if err != nil {
		t.Fatalf("expected to read through symlink: %v", err)
	}
	if string(gotContent) != string(realContent) {
		t.Errorf("content through symlink = %q, want %q", gotContent, realContent)
	}
}

// TestCreate_AbsoluteSymlink_ConvertedToRelative verifies that marshal create
// converts an absolute symlink in the defaults tree to a relative symlink in
// the per-project directory.
func TestCreate_AbsoluteSymlink_ConvertedToRelative(t *testing.T) {
	// Given a user defaults tree containing an absolute symlink config/x -> <defaults>/config/y
	base := t.TempDir()
	t.Setenv("XDG_DATA_HOME", base)

	defaultsBase := filepath.Join(base, "marshal", "defaults", "opencode")
	wantContent := []byte(`{"y":"data"}`)
	writeDefaultsFile(t, filepath.Join(defaultsBase, "config", "y"), wantContent)
	absTarget := filepath.Join(defaultsBase, "config", "y")
	symlinkPath := filepath.Join(defaultsBase, "config", "x")
	if err := os.Symlink(absTarget, symlinkPath); err != nil {
		t.Fatal(err)
	}

	cf := newCredFakes(t)
	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := newCredentialTestDeps(cf, runner)

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp", "create"})

	// When marshal create is run for a project
	assertNoError(t, root.Execute())

	// Then config/x at the per-project directory is a symlink
	projectLink := filepath.Join(cf.dataBase, "projects", "myapp", "opencode", "config", "x")
	info, err := os.Lstat(projectLink)
	if err != nil {
		t.Fatalf("expected config/x to exist: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected config/x to be a symlink, got mode %v", info.Mode())
	}

	// And os.Readlink on the destination symlink returns a relative path
	gotTarget, err := os.Readlink(projectLink)
	if err != nil {
		t.Fatalf("expected to read symlink target: %v", err)
	}
	if filepath.IsAbs(gotTarget) {
		t.Errorf("symlink target should be relative, got absolute path %q", gotTarget)
	}

	// And the relative path resolves correctly within the per-project directory
	gotContent, err := os.ReadFile(projectLink)
	if err != nil {
		t.Fatalf("expected to read through symlink: %v", err)
	}
	if string(gotContent) != string(wantContent) {
		t.Errorf("content through symlink = %q, want %q", gotContent, wantContent)
	}
}

// writeDefaultsFile creates parent directories and writes content to path.
func writeDefaultsFile(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
}

// containsSubstr reports whether s contains substr.
func containsSubstr(s, substr string) bool {
	return len(substr) == 0 || (len(s) >= len(substr) && searchSubstr(s, substr))
}

func searchSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
