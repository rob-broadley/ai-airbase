// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/cmd"
	"github.com/rob-broadley/ai-airbase/marshal/internal/container"
)

// ---------------------------------------------------------------------------
// Per-project directory mount location tests
// ---------------------------------------------------------------------------

// TestDefaultCmd_ConfigDirRelocatedAndIsolated verifies that the project-specific
// configuration directory is located at projects/<project>/opencode/config under XDG_DATA_HOME
// and mounted as a whole, rather than mounting individual files or a shared config directory.
func TestDefaultCmd_ConfigDirRelocatedAndIsolated(t *testing.T) {
	// Given marshal is run for project "myapp"
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	cf := newCredFakes(t)
	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := newCredentialTestDeps(cf, runner)

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp"})

	// When the default command is executed
	assertNoError(t, root.Execute())

	args := runner.createArgs()

	// Then the configuration directory is mounted from the project-specific path "projects/myapp/opencode/config" under XDG_DATA_HOME
	want := cf.expectedDataMount("projects/myapp/opencode/config", container.ContainerOpencodeConfigDir)
	if !sliceContains(args, want) {
		t.Errorf("expected config directory mount %q in create args\ngot: %v", want, args)
	}

	// And individual files under this directory are not mounted separately
	for _, file := range []string{
		"settings.json",
		"mcp-config.json",
	} {
		notWantFile := cf.expectedDataMount("projects/myapp/opencode/config/"+file, container.ContainerOpencodeConfigDir+"/"+file)
		if sliceContains(args, notWantFile) {
			t.Errorf("config file %s must not be mounted individually; found %q in create args", file, notWantFile)
		}
	}
}

// TestDefaultCmd_GitConfigMountedFromPerProjectDir verifies that the git config
// directory is bind-mounted from the per-project data directory to
// ContainerGitConfigDir as a read-only mount. The mount source must be the
// per-project git/config directory (not a shared XDG_CONFIG_HOME path), and
// it must carry the :ro,Z flags so the container cannot write to it.
func TestDefaultCmd_GitConfigMountedFromPerProjectDir(t *testing.T) {
	// Given XDG_DATA_HOME points to a fresh temp directory and no container
	// exists yet for project "myapp"
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	cf := newCredFakes(t)
	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := newCredentialTestDeps(cf, runner)

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp", "create"})

	// When marshal create is executed
	assertNoError(t, root.Execute())

	// Then the git config directory is bind-mounted from the per-project path,
	// read-only, to ContainerGitConfigDir
	args := runner.createArgs()
	wantMount := filepath.Join(cf.dataBase, "projects", "myapp", "git", "config") +
		":" + container.ContainerGitConfigDir + ":ro,Z"
	if !sliceContains(args, wantMount) {
		t.Errorf("expected read-only git config dir mount %q in create args\ngot: %v", wantMount, args)
	}
}

// TestDefaultCmd_SessionStoreMounted verifies that the share/ directory is
// bind-mounted from the per-project XDG_DATA_HOME/marshal/projects/<project>/opencode/share/
// path so conversation history is preserved across container recreates.
func TestDefaultCmd_SessionStoreMounted(t *testing.T) {
	// Given deps configured for project "myapp"
	cf := newCredFakes(t)
	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := newCredentialTestDeps(cf, runner)

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp"})

	// When the default command is executed
	assertNoError(t, root.Execute())

	// Then the share/ directory is mounted from the per-project data directory
	args := runner.createArgs()
	want := cf.expectedDataMount("projects/myapp/opencode/share", container.ContainerOpencodeDataDir)
	if !sliceContains(args, want) {
		t.Errorf("expected share/ directory mount %q in create args\ngot: %v", want, args)
	}
}

// TestDefaultCmd_SessionStateMountedPerProject verifies that the state/
// directory is bind-mounted from a per-project path under XDG_DATA_HOME so
// checkpoint history survives container recreates.
func TestDefaultCmd_SessionStateMountedPerProject(t *testing.T) {
	// Given deps configured for project "myapp"
	cf := newCredFakes(t)
	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := newCredentialTestDeps(cf, runner)

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp"})

	// When the default command is executed
	assertNoError(t, root.Execute())

	// Then state/ directory is mounted from a per-project path under XDG_DATA_HOME
	args := runner.createArgs()
	want := cf.expectedDataMount("projects/myapp/opencode/state", container.ContainerOpencodeStateDir)
	if !sliceContains(args, want) {
		t.Errorf("expected per-project state/ directory mount %q in create args\ngot: %v", want, args)
	}
}

// TestCredentialMounts_SessionStateIsolatedByProject verifies that two
// different projects receive different state/ and share/
// host paths.
func TestCredentialMounts_SessionStateIsolatedByProject(t *testing.T) {
	// Given two separate projects "alpha" and "beta" sharing the same data fakes
	cf := newCredFakes(t)

	argsFor := func(project string) []string {
		runner := &fakeRunner{exists: false, imageExistsResult: true}
		deps := cmd.Deps{
			Runner:              runner,
			ExecFn:              (&fakeExec{}).exec,
			Getwd:               func() (string, error) { return "/projects/" + project, nil },
			Getuid:              func() int { return 1001 },
			Getgid:              func() int { return 1001 },
			EnsureSharedDataDir: cf.dataDirFn,
		}
		root := cmd.NewRootCmd(deps)
		root.SetArgs([]string{"--project", project})
		if err := root.Execute(); err != nil {
			t.Fatalf("project %s: unexpected error: %v", project, err)
		}
		return runner.createArgs()
	}

	// When the default command is executed for each project
	alphaArgs := argsFor("alpha")
	betaArgs := argsFor("beta")

	// Then each project's session paths are distinct and not shared
	for _, tc := range []struct {
		alphaSubdir   string
		betaSubdir    string
		containerPath string
	}{
		{"projects/alpha/opencode/state", "projects/beta/opencode/state", container.ContainerOpencodeStateDir},
		{"projects/alpha/opencode/share", "projects/beta/opencode/share", container.ContainerOpencodeDataDir},
	} {
		alphaMount := cf.expectedDataMount(tc.alphaSubdir, tc.containerPath)
		betaMount := cf.expectedDataMount(tc.betaSubdir, tc.containerPath)

		if !sliceContains(alphaArgs, alphaMount) {
			t.Errorf("alpha: expected mount %q\ngot: %v", alphaMount, alphaArgs)
		}
		if !sliceContains(betaArgs, betaMount) {
			t.Errorf("beta: expected mount %q\ngot: %v", betaMount, betaArgs)
		}
		if sliceContains(alphaArgs, betaMount) {
			t.Errorf("alpha: must not have beta's mount %q", betaMount)
		}
		if sliceContains(betaArgs, alphaMount) {
			t.Errorf("beta: must not have alpha's mount %q", alphaMount)
		}
	}
}

// TestCredentialMounts_ConfigIsolatedByProject verifies that the opencode
// configuration directory is isolated by project and NOT shared.
func TestCredentialMounts_ConfigIsolatedByProject(t *testing.T) {
	// Given two separate projects "alpha" and "beta" sharing the same config/data fakes
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	cf := newCredFakes(t)

	argsFor := func(project string) []string {
		runner := &fakeRunner{exists: false, imageExistsResult: true}
		deps := cmd.Deps{
			Runner:              runner,
			ExecFn:              (&fakeExec{}).exec,
			Getwd:               func() (string, error) { return "/projects/" + project, nil },
			Getuid:              func() int { return 1001 },
			Getgid:              func() int { return 1001 },
			EnsureSharedDataDir: cf.dataDirFn,
		}
		root := cmd.NewRootCmd(deps)
		root.SetArgs([]string{"--project", project})
		if err := root.Execute(); err != nil {
			t.Fatalf("project %s: unexpected error: %v", project, err)
		}
		return runner.createArgs()
	}

	// When the default command is executed for each project
	alphaArgs := argsFor("alpha")
	betaArgs := argsFor("beta")

	// Then the config mounts are isolated and distinct for both projects
	alphaMount := cf.expectedDataMount("projects/alpha/opencode/config", container.ContainerOpencodeConfigDir)
	betaMount := cf.expectedDataMount("projects/beta/opencode/config", container.ContainerOpencodeConfigDir)

	if !sliceContains(alphaArgs, alphaMount) {
		t.Errorf("alpha: expected isolated mount %q\ngot: %v", alphaMount, alphaArgs)
	}
	if !sliceContains(betaArgs, betaMount) {
		t.Errorf("beta: expected isolated mount %q\ngot: %v", betaMount, betaArgs)
	}
	if sliceContains(alphaArgs, betaMount) {
		t.Errorf("alpha: must not have beta's mount %q", betaMount)
	}
	if sliceContains(betaArgs, alphaMount) {
		t.Errorf("beta: must not have alpha's mount %q", alphaMount)
	}
}

// TestDefaultCmd_ShareAndStateDirsCreatedUnderOpencode verifies that running
// the default command for a project creates the share/ and state/ directories
// under projects/<project>/opencode/ (the new location) and does NOT create
// them under projects/<project>/ directly (the legacy location). The legacy
// location was used before per-project config was isolated under the opencode
// subdirectory; a regression that writes to the legacy path would leave stale
// files on the host and could cause confusion if both locations existed.
func TestDefaultCmd_ShareAndStateDirsCreatedUnderOpencode(t *testing.T) {
	// Given marshal is run for project "myapp"
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	cf := newCredFakes(t)
	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := newCredentialTestDeps(cf, runner)

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp"})

	// When the default command is executed
	assertNoError(t, root.Execute())

	// Then the new-style directories exist under projects/<project>/opencode/
	newShareDir := filepath.Join(cf.dataBase, "projects", "myapp", "opencode", "share")
	newStateDir := filepath.Join(cf.dataBase, "projects", "myapp", "opencode", "state")
	for _, dir := range []string{newShareDir, newStateDir} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Errorf("expected new-style directory %q to exist, got: %v", dir, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("expected %q to be a directory", dir)
		}
	}

	// And the legacy directories under projects/<project>/ directly do NOT exist
	legacyShareDir := filepath.Join(cf.dataBase, "projects", "myapp", "share")
	legacyStateDir := filepath.Join(cf.dataBase, "projects", "myapp", "state")
	for _, dir := range []string{legacyShareDir, legacyStateDir} {
		if _, err := os.Stat(dir); err == nil {
			t.Errorf("legacy directory %q must not be created; found on disk", dir)
		} else if !os.IsNotExist(err) {
			t.Errorf("unexpected error stat-ing legacy dir %q: %v", dir, err)
		}
	}
}

// ---------------------------------------------------------------------------
// Recreate — credential mounts included
// ---------------------------------------------------------------------------

// TestRecreate_CredentialMountsIncluded verifies that the recreate command also
// mounts credential files when creating a container.
func TestRecreate_CredentialMountsIncluded(t *testing.T) {
	// Given deps configured for project "myapp" with the recreate subcommand
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	cf := newCredFakes(t)
	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := newCredentialTestDeps(cf, runner)

	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "recreate"})

	// When recreate is executed
	assertNoError(t, root.Execute())

	// Then credential mounts (config and share directories) are included
	args := runner.createArgs()
	opencodeConfigMount := cf.expectedDataMount("projects/myapp/opencode/config", container.ContainerOpencodeConfigDir)
	sessionMount := cf.expectedDataMount("projects/myapp/opencode/share", container.ContainerOpencodeDataDir)

	if !sliceContains(args, opencodeConfigMount) {
		t.Errorf("recreate: expected settings mount %q\ngot: %v", opencodeConfigMount, args)
	}
	if !sliceContains(args, sessionMount) {
		t.Errorf("recreate: expected share mount %q\ngot: %v", sessionMount, args)
	}
}

// ---------------------------------------------------------------------------
// Configuration directory hardening — permissions, symlinks, fallbacks
// ---------------------------------------------------------------------------

// TestDefaultCmd_ConfigDirPermissionsEnforced verifies that the configuration directory and its
// subdirectories and files have strict permissions enforced recursively (0o700 for directories, 0o600 for files).
func TestDefaultCmd_ConfigDirPermissionsEnforced(t *testing.T) {
	// Given marshal is run for project "myapp" and the configuration directory contains
	// a subdirectory and a file with overly permissive permissions (e.g. 0o755 for directory, 0o644 for file)
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	cf := newCredFakes(t)
	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := newCredentialTestDeps(cf, runner)

	// Pre-create the relocated config directory and nested structures with permissive modes
	configDir := filepath.Join(cf.dataBase, "projects/myapp/opencode/config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	nestedDir := filepath.Join(configDir, "nested-subdir")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatalf("failed to create nested dir: %v", err)
	}
	testFile := filepath.Join(configDir, "settings.json")
	if err := os.WriteFile(testFile, []byte("{}"), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Set modes explicitly to make sure they are not strictly owner-only
	if err := os.Chmod(nestedDir, 0o755); err != nil {
		t.Fatalf("chmod nested dir: %v", err)
	}
	if err := os.Chmod(testFile, 0o644); err != nil {
		t.Fatalf("chmod test file: %v", err)
	}

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp"})

	// When the default command is executed
	assertNoError(t, root.Execute())

	// Then the permissions are corrected to 0o700 for directories and 0o600 for files on the host
	info, err := os.Stat(configDir)
	if err != nil {
		t.Fatalf("stat config dir: %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Errorf("expected config dir to have 0o700, got %04o", info.Mode().Perm())
	}

	info, err = os.Stat(nestedDir)
	if err != nil {
		t.Fatalf("stat nested dir: %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Errorf("expected nested dir to have 0o700, got %04o", info.Mode().Perm())
	}

	info, err = os.Stat(testFile)
	if err != nil {
		t.Fatalf("stat test file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("expected test file to have 0o600, got %04o", info.Mode().Perm())
	}
}

// TestDefaultCmd_ConfigDirNestedSymlink_Allowed verifies that if a symlink exists inside
// the configuration directory and its target stays within the bind mount (e.g. a relative
// symlink created by opencode for package management), marshal does NOT abort. The sandbox
// is preserved because the container cannot follow the symlink outside the bind mount.
func TestDefaultCmd_ConfigDirNestedSymlink_Allowed(t *testing.T) {
	// Given marshal is run for project "myapp" and the configuration directory contains a
	// relative symlink whose target is inside the config directory itself
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	cf := newCredFakes(t)
	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := newCredentialTestDeps(cf, runner)

	// Pre-create the relocated config directory
	configDir := filepath.Join(cf.dataBase, "projects/myapp/opencode/config")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	// Create a target file inside the config directory
	targetFile := filepath.Join(configDir, "real-settings.json")
	if err := os.WriteFile(targetFile, []byte("{}"), 0o600); err != nil {
		t.Fatalf("failed to write target file: %v", err)
	}
	// Create a relative symlink that stays inside the config directory
	symlinkPath := filepath.Join(configDir, "settings.json")
	if err := os.Symlink("real-settings.json", symlinkPath); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp"})

	var errBuf bytes.Buffer
	root.SetErr(&errBuf)

	// When the default command is executed
	err := root.Execute()

	// Then the command does NOT abort (the symlink stays inside the bind mount)
	if err != nil {
		t.Fatalf("expected Execute() to succeed with internal symlink, got error: %v\nstderr: %s", err, errBuf.String())
	}

	// And the symlink still exists (was not followed or removed)
	if _, err := os.Lstat(symlinkPath); err != nil {
		t.Errorf("expected symlink to still exist, got: %v", err)
	}
}

// TestDefaultCmd_ConfigDirNestedSymlinkEscapesAborts verifies that if a symlink inside the
// configuration directory resolves to a path OUTSIDE the bind mount, marshal aborts. The
// container is untrusted AI code; following such a symlink would let it read or write host
// files outside the sandbox.
func TestDefaultCmd_ConfigDirNestedSymlinkEscapesAborts(t *testing.T) {
	// Given marshal is run for project "myapp" and the configuration directory contains an
	// absolute symlink pointing outside the bind mount
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	cf := newCredFakes(t)
	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := newCredentialTestDeps(cf, runner)

	// Pre-create the relocated config directory
	configDir := filepath.Join(cf.dataBase, "projects/myapp/opencode/config")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	// Create a target file OUTSIDE the config directory (sibling under dataBase)
	targetFile := filepath.Join(cf.dataBase, "external-file.json")
	if err := os.WriteFile(targetFile, []byte("{}"), 0o600); err != nil {
		t.Fatalf("failed to write external file: %v", err)
	}
	// Create an absolute symlink inside the config directory pointing outside it
	symlinkPath := filepath.Join(configDir, "settings.json")
	if err := os.Symlink(targetFile, symlinkPath); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp"})

	var errBuf bytes.Buffer
	root.SetErr(&errBuf)

	// When the default command is executed
	err := root.Execute()

	// Then the command aborts with a security violation
	if err == nil {
		t.Fatal("expected Execute() to return error due to symlink escaping bind mount")
	}

	// And "security violation: symlink ... escapes the bind mount" is written to stderr
	gotErr := errBuf.String()
	if !strings.Contains(gotErr, "escapes the bind mount") {
		t.Errorf("expected stderr to mention symlink escape, got: %q", gotErr)
	}
}

// TestDefaultCmd_ConfigDirRootSymlinkAborts verifies that if the configuration directory
// itself is a symlink (which would make the bind mount target a symlink, and os.RemoveAll
// would follow it), marshal aborts with a security violation error.
func TestDefaultCmd_ConfigDirRootSymlinkAborts(t *testing.T) {
	// Given marshal is run for project "myapp" and the configuration directory IS a symlink
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	cf := newCredFakes(t)
	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := newCredentialTestDeps(cf, runner)

	// Create a real directory that will be the symlink target
	realDir := filepath.Join(cf.dataBase, "real-config")
	if err := os.MkdirAll(realDir, 0o700); err != nil {
		t.Fatalf("failed to create real dir: %v", err)
	}

	// Ensure the parent path exists, then create the symlink at the config dir location
	configDirParent := filepath.Join(cf.dataBase, "projects/myapp/opencode")
	if err := os.MkdirAll(configDirParent, 0o700); err != nil {
		t.Fatalf("failed to create parent dir: %v", err)
	}
	configDir := filepath.Join(configDirParent, "config")
	if err := os.Symlink(realDir, configDir); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp"})

	var errBuf bytes.Buffer
	root.SetErr(&errBuf)

	// When the default command is executed
	err := root.Execute()

	// Then the command aborts with an error
	if err == nil {
		t.Fatal("expected Execute() to return error due to root symlink")
	}

	// And "security violation: directory is a symlink" is written to stderr
	gotErr := errBuf.String()
	if !strings.Contains(gotErr, "security violation: directory is a symlink") {
		t.Errorf("expected stderr to contain %q, got: %q", "security violation: directory is a symlink", gotErr)
	}
}

// TestDefaultCmd_ConfigDirPermissionDeniedAborts verifies that if the configuration directory
// lacks write permissions, marshal prints "permission denied" with the target directory path and aborts.
func TestDefaultCmd_ConfigDirPermissionDeniedAborts(t *testing.T) {
	// Given marshal is run for project "myapp" and the configuration directory has no write permissions (0o500)
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	cf := newCredFakes(t)
	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := newCredentialTestDeps(cf, runner)

	// Pre-create the relocated config directory
	configDir := filepath.Join(cf.dataBase, "projects/myapp/opencode/config")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}

	// Make it read-only (no write permissions)
	if err := os.Chmod(configDir, 0o500); err != nil {
		t.Fatalf("chmod read-only: %v", err)
	}
	defer os.Chmod(configDir, 0o700) // cleanup so temp dir can be cleaned up

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp"})

	var errBuf bytes.Buffer
	root.SetErr(&errBuf)

	// When the default command is executed
	err := root.Execute()

	// Then the command aborts with an error
	if err == nil {
		t.Fatal("expected Execute() to return error due to write permission violation")
	}

	// And "permission denied: <path>" is written to stderr
	gotErr := errBuf.String()
	expectedStr := "permission denied: " + configDir
	if !strings.Contains(gotErr, expectedStr) {
		t.Errorf("expected stderr to contain %q, got: %q", expectedStr, gotErr)
	}
}

// TestDefaultCmd_ConfigDirCreationDenied_ErrorIdentifiesPath verifies that
// when EnsureSharedDataDir returns a permission error while creating the
// per-project config directory (e.g. XDG_DATA_HOME parent is read-only or
// the marshal data dir is not writable), the surfaced error identifies the
// path that was being created (e.g. "projects/myapp/opencode/config") or
// wraps the original error so the chain is preserved.
func TestDefaultCmd_ConfigDirCreationDenied_ErrorIdentifiesPath(t *testing.T) {
	// Given marshal is run for project "myapp" and the injected EnsureSharedDataDir
	// returns a permission error for the project config subdirectory
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	cf := newCredFakes(t)
	permPath := filepath.Join(cf.dataBase, "projects", "myapp", "opencode", "config")
	permErr := &os.PathError{Op: "mkdir", Path: permPath, Err: os.ErrPermission}

	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := newCredentialTestDeps(cf, runner)
	deps.EnsureSharedDataDir = func(string) (string, error) {
		return "", permErr
	}

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp"})

	var errBuf bytes.Buffer
	root.SetErr(&errBuf)

	// When the default command is executed
	err := root.Execute()

	// Then the command aborts with a permission error
	if err == nil {
		t.Fatal("expected Execute() to return error due to permission denied on EnsureSharedDataDir")
	}

	// And the error preserves context: it either contains the path being created
	// or wraps the original permission error so the error chain is preserved.
	errMsg := err.Error()
	hasPath := strings.Contains(errMsg, "projects/myapp/opencode/config")
	wraps := errors.Is(err, permErr)
	if !hasPath && !wraps {
		t.Errorf("expected error to include path %q or wrap original error %v; got: %v",
			"projects/myapp/opencode/config", permErr, err)
	}
}

// TestDefaultCmd_ConfigDirWriteCheck_HasNoSideEffects verifies that the
// write-permission check performed while assembling credential mounts is
// stateless: it must not create any file in the project's config directory
// tree. A previous implementation created a `.write_test` file as a side
// effect, which could be left behind on a crash between create and remove.
func TestDefaultCmd_ConfigDirWriteCheck_HasNoSideEffects(t *testing.T) {
	// Given marshal is run for project "myapp"
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	cf := newCredFakes(t)
	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := newCredentialTestDeps(cf, runner)

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp"})

	// When the default command is executed
	assertNoError(t, root.Execute())

	// Then no .write_test file exists anywhere in the project's config directory tree
	configDir := filepath.Join(cf.dataBase, "projects", "myapp", "opencode", "config")
	var leaked []string
	walkErr := filepath.WalkDir(configDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() && d.Name() == ".write_test" {
			leaked = append(leaked, path)
		}
		return nil
	})
	assertNoError(t, walkErr)
	if len(leaked) > 0 {
		t.Errorf("write-permission check must not create files in the config dir; found: %v", leaked)
	}
}

// TestDefaultCmd_ConfigDirEmptyDataHomeFallback verifies that if XDG_DATA_HOME is set to empty,
// marshal correctly falls back to using ~/.local/share as the base for the project configuration.
func TestDefaultCmd_ConfigDirEmptyDataHomeFallback(t *testing.T) {
	// Given XDG_DATA_HOME is empty, and HOME is set to a temp directory
	t.Setenv("XDG_DATA_HOME", "")
	homeTemp := t.TempDir()
	t.Setenv("HOME", homeTemp)

	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := cmd.Deps{
		Runner: runner,
		ExecFn: (&fakeExec{}).exec,
		Getwd:  func() (string, error) { return "/projects/myapp", nil },
		Getuid: func() int { return 1001 },
		Getgid: func() int { return 1002 },
		// EnsureSharedDataDir is nil so it falls back to hostinfo.EnsureSharedDataDir
	}

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp"})

	// When the command is executed
	assertNoError(t, root.Execute())

	// Then the config directory is created and mounted under the fallback path in HOME
	args := runner.createArgs()
	expectedHostPath := filepath.Join(homeTemp, ".local", "share", "marshal", "projects", "myapp", "opencode", "config")
	expectedMount := expectedHostPath + ":" + container.ContainerOpencodeConfigDir + ":Z"

	if !sliceContains(args, expectedMount) {
		t.Errorf("expected config mount %q in create args; got: %v", expectedMount, args)
	}

	// Verify the directory actually exists on the host
	info, err := os.Stat(expectedHostPath)
	if err != nil {
		t.Fatalf("failed to stat fallback config directory %q: %v", expectedHostPath, err)
	}
	if !info.IsDir() {
		t.Errorf("expected fallback path %q to be a directory", expectedHostPath)
	}
}

func newCredentialTestDeps(cf *credFakes, runner *fakeRunner) cmd.Deps {
	return cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              func() int { return 1001 },
		Getgid:              func() int { return 1002 },
		EnsureSharedDataDir: cf.dataDirFn,
	}
}
