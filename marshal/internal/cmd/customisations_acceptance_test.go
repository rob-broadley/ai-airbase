// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/cmd"
)

// TestCreate_UserDefaultsDirCreated verifies the primary acceptance:
// running `marshal create` on a clean host creates the on-host user defaults
// directory tree at $XDG_DATA_HOME/marshal/defaults/opencode/{config,share,state}
// with 0o700 permissions, while leaving the per-project host dir
// (provisioned by provisionProjectDir) unchanged.
func TestCreate_UserDefaultsDirCreated(t *testing.T) {
	// Given XDG_DATA_HOME is set to a fresh temp directory so the real
	// hostinfo.XDGDataHome() reads the env and resolves the user defaults
	// tree to a per-test temp path (real env, real temp dir).
	xdgDataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDataHome)

	cf := newCredFakes(t)
	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := newCredentialTestDeps(cf, runner)

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp", "create"})

	// When the user runs `marshal create`
	assertNoError(t, root.Execute())

	// Then the three subdirs exist as directories with 0o700 permissions
	wantBase := filepath.Join(xdgDataHome, "marshal", "defaults", "opencode")
	for _, sub := range []string{"config", "share", "state"} {
		p := filepath.Join(wantBase, sub)
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("expected %s to exist: %v", p, err)
		}
		if !info.IsDir() {
			t.Errorf("expected %s to be a directory", p)
		}
		if perm := info.Mode().Perm(); perm != 0o700 {
			t.Errorf("expected %s to have permissions 0o700, got %04o", p, perm)
		}
	}

	// And the existing create flow still runs `podman create` (the
	// integration does not regress the existing podman invocation).
	if !runner.calledSubcommand("create") {
		t.Errorf("expected podman create to be called, but it was not")
	}

	// And the per-project host dir is unchanged: provisionProjectDir still
	// creates projects/myapp/opencode/{config,share,state} under the
	// per-test dataBase (NOT under XDG_DATA_HOME; the per-project dir is
	// faked via cf.dataDirFn so the user defaults tree and the per-project
	// tree are independent on disk).
	perProjectConfig := filepath.Join(cf.dataBase, "projects", "myapp", "opencode", "config")
	if _, err := os.Stat(perProjectConfig); err != nil {
		t.Errorf("expected per-project config dir %s to exist unchanged: %v", perProjectConfig, err)
	}
}

// --- marshal recreate also creates the user defaults dir ---

// TestRecreate_UserDefaultsDirCreated verifies that running `marshal recreate`
// also creates the on-host user defaults directory tree with 0o700 permissions.
func TestRecreate_UserDefaultsDirCreated(t *testing.T) {
	// Given XDG_DATA_HOME is set to a fresh temp directory
	xdgDataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDataHome)

	cf := newCredFakes(t)
	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := newCredentialTestDeps(cf, runner)

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp", "recreate"})

	// When the user runs `marshal recreate`
	assertNoError(t, root.Execute())

	// Then the three subdirs exist as directories with 0o700 permissions
	wantBase := filepath.Join(xdgDataHome, "marshal", "defaults", "opencode")
	for _, sub := range []string{"config", "share", "state"} {
		p := filepath.Join(wantBase, sub)
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("expected %s to exist: %v", p, err)
		}
		if !info.IsDir() {
			t.Errorf("expected %s to be a directory", p)
		}
		if perm := info.Mode().Perm(); perm != 0o700 {
			t.Errorf("expected %s to have permissions 0o700, got %04o", p, perm)
		}
	}

	// And the existing recreate flow still runs `podman create`
	if !runner.calledSubcommand("create") {
		t.Errorf("expected podman create to be called, but it was not")
	}
}

// --- the default marshal command also creates the user defaults dir ---

// TestDefaultCmd_UserDefaultsDirCreated verifies that running the default
// `marshal` command (no subcommand) also creates the on-host user defaults
// directory tree with 0o700 permissions. The default command's RunE invokes
// ensureContainerAndStart, which calls ensureHostState, reaching the
// customisations.Ensure call.
func TestDefaultCmd_UserDefaultsDirCreated(t *testing.T) {
	// Given XDG_DATA_HOME is set to a fresh temp directory
	xdgDataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDataHome)

	cf := newCredFakes(t)
	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := newCredentialTestDeps(cf, runner)

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"start", "--project", "myapp"})

	// When the user runs the default marshal command (no subcommand)
	assertNoError(t, root.Execute())

	// Then the three subdirs exist as directories with 0o700 permissions
	wantBase := filepath.Join(xdgDataHome, "marshal", "defaults", "opencode")
	for _, sub := range []string{"config", "share", "state"} {
		p := filepath.Join(wantBase, sub)
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("expected %s to exist: %v", p, err)
		}
		if !info.IsDir() {
			t.Errorf("expected %s to be a directory", p)
		}
		if perm := info.Mode().Perm(); perm != 0o700 {
			t.Errorf("expected %s to have permissions 0o700, got %04o", p, perm)
		}
	}
}

// --- error from Ensure aborts the create flow ---

// TestCreate_UserDefaultsDirError_AbortsCreate verifies that when
// customisations.Ensure returns an error (because XDG_DATA_HOME and HOME
// are both empty), marshal create aborts with a non-nil error and no
// podman create call is made. The error is triggered via real t.Setenv
// driving the real hostinfo.XDGDataHome() to return "" — not via a stub.
func TestCreate_UserDefaultsDirError_AbortsCreate(t *testing.T) {
	// Given XDG_DATA_HOME="" and HOME="" so the real hostinfo.XDGDataHome()
	// returns "" naturally (real env, real resolution path)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", "")

	cf := newCredFakes(t)
	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := newCredentialTestDeps(cf, runner)

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp", "create"})

	// When the user runs `marshal create`
	err := root.Execute()

	// Then marshal create aborts with a non-nil error
	if err == nil {
		t.Fatal("expected an error when XDG_DATA_HOME and HOME are empty, got nil")
	}

	// And the error message indicates the directory is unavailable
	if !strings.Contains(err.Error(), "directory unavailable") {
		t.Errorf("error %q should contain %q", err.Error(), "directory unavailable")
	}

	// And no podman create call is made
	if runner.calledSubcommand("create") {
		t.Error("expected no podman create call, but one was made")
	}
}

// --- git config seeding into the defaults tree ---

// Test_Create_SeedsGitConfigInDefaultsTree verifies that when marshal create runs
// and the user's git identity is available, a git [user] config is written into
// the defaults tree so it is available to copy into new per-project directories.
func Test_Create_SeedsGitConfigInDefaultsTree(t *testing.T) {
	// Given XDG_DATA_HOME points to a fresh temp directory
	xdgDataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDataHome)

	// And LookupGitConfig returns name and email
	cf := newCredFakes(t)
	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := newCredentialTestDeps(cf, runner)
	deps.XDGDataHome = func() string { return xdgDataHome }
	deps.LookupGitConfig = func(key string) string {
		switch key {
		case "user.name":
			return "Test User"
		case "user.email":
			return "test@example.com"
		}
		return ""
	}

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp", "create"})

	// When marshal create is executed
	assertNoError(t, root.Execute())

	// Then the git config file is seeded in the defaults tree
	configPath := filepath.Join(xdgDataHome, "marshal", "defaults", "git", "config", "config")
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("expected git config to be seeded at %s: %v", configPath, err)
	}

	// And the file contains exactly the expected [user] section
	wantContent := "[user]\n\tname = \"Test User\"\n\temail = \"test@example.com\"\n"
	if string(content) != wantContent {
		t.Errorf("git config content mismatch:\ngot:\n%s\nwant:\n%s", content, wantContent)
	}
}
