// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd_test

import (
	"fmt"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/cmd"
	"github.com/rob-broadley/ai-airbase/marshal/internal/config"
)

// TestDefaultCmd_MountsCWD verifies that marshal mounts the current working
// directory inside /workspace/<basename> when no mounts are configured.
func TestDefaultCmd_MountsCWD(t *testing.T) {
	// Given no --mount flags and empty config
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false}
	fe := &fakeExec{}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              fe.exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp"})

	// When the root command is executed
	assertNoError(t, root.Execute())

	// Then CWD is mounted inside /workspace/myapp (not as /workspace root)
	if !runner.createArgsContain("/projects/myapp:/workspace/myapp:Z") {
		t.Errorf("expected create args to contain '/projects/myapp:/workspace/myapp:Z', got %v", runner.createArgs())
	}
}

// TestDefaultCmd_MountFlagAbsolute verifies that an absolute --mount path is
// mounted under /workspace/<basename> and CWD is not mounted.
func TestDefaultCmd_MountFlagAbsolute(t *testing.T) {
	// Given --mount /abs/shared-lib
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false}
	fe := &fakeExec{}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              fe.exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp", "--mount", "/abs/shared-lib"})

	// When the root command is executed
	assertNoError(t, root.Execute())

	// Then the path is mounted under /workspace/shared-lib and CWD is not mounted
	if !runner.createArgsContain("/abs/shared-lib:/workspace/shared-lib:Z") {
		t.Errorf("expected create args to contain '/abs/shared-lib:/workspace/shared-lib:Z', got %v", runner.createArgs())
	}
	if runner.createArgsContain("/projects/myapp:/workspace:Z") {
		t.Error("CWD should NOT be mounted when --mount flags are provided")
	}
}

// TestDefaultCmd_MultipleMountFlags verifies that multiple --mount flags each
// get mounted under /workspace/<basename>.
func TestDefaultCmd_MultipleMountFlags(t *testing.T) {
	// Given two --mount flags
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false}
	fe := &fakeExec{}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              fe.exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp", "--mount", "/abs/lib1", "--mount", "/abs/lib2"})

	// When the root command is executed
	assertNoError(t, root.Execute())

	// Then both paths are mounted
	if !runner.createArgsContain("/abs/lib1:/workspace/lib1:Z") {
		t.Errorf("expected '/abs/lib1:/workspace/lib1:Z' in create args, got %v", runner.createArgs())
	}
	if !runner.createArgsContain("/abs/lib2:/workspace/lib2:Z") {
		t.Errorf("expected '/abs/lib2:/workspace/lib2:Z' in create args, got %v", runner.createArgs())
	}
}

// TestDefaultCmd_PersistsMountsToConfig verifies that --mount paths are saved
// to the project config for future use.
func TestDefaultCmd_PersistsMountsToConfig(t *testing.T) {
	// Given --mount /abs/shared-lib with a writable config dir
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	runner := &fakeRunner{exists: false}
	fe := &fakeExec{}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              fe.exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp", "--mount", "/abs/shared-lib"})

	// When the root command is executed
	assertNoError(t, root.Execute())

	// Then the mount path is persisted to config
	cfg, err := config.Load("myapp")
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}
	if len(cfg.Mounts) != 1 || cfg.Mounts[0] != "/abs/shared-lib" {
		t.Errorf("expected config.Mounts = ['/abs/shared-lib'], got %v", cfg.Mounts)
	}
}

// TestDefaultCmd_ReusesSavedMounts verifies that marshal applies mounts saved
// in config without requiring --mount flags.
func TestDefaultCmd_ReusesSavedMounts(t *testing.T) {
	// Given mounts saved in project config
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := config.Save("myapp", &config.Config{Mounts: []string{"/abs/saved"}}); err != nil {
		t.Fatalf("saving config: %v", err)
	}

	runner := &fakeRunner{exists: false}
	fe := &fakeExec{}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              fe.exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp"})

	// When the root command is executed without --mount flags
	assertNoError(t, root.Execute())

	// Then saved mounts are used and CWD is not mounted
	if !runner.createArgsContain("/abs/saved:/workspace/saved:Z") {
		t.Errorf("expected '/abs/saved:/workspace/saved:Z' in create args, got %v", runner.createArgs())
	}
	if runner.createArgsContain("/projects/myapp:/workspace:Z") {
		t.Error("CWD should NOT be mounted when saved mounts exist")
	}
}

// TestDefaultCmd_CustomImage verifies that setting MARSHAL_IMAGE overrides the
// default container image.
func TestDefaultCmd_CustomImage(t *testing.T) {
	// Given MARSHAL_IMAGE is set to a custom image
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("MARSHAL_IMAGE", "myregistry/revetment:v2")

	runner := &fakeRunner{exists: false}
	fe := &fakeExec{}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              fe.exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp"})

	// When the root command is executed
	assertNoError(t, root.Execute())

	// Then the custom image appears in the create args
	if !runner.createArgsContain("myregistry/revetment:v2") {
		t.Errorf("expected custom image in create args, got %v", runner.createArgs())
	}
}

// TestDefaultCmd_RelativeMountResolved verifies that a relative --mount path is
// resolved against the current working directory.
func TestDefaultCmd_RelativeMountResolved(t *testing.T) {
	// Given --mount reldir with CWD set
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false}
	fe := &fakeExec{}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              fe.exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp", "--mount", "reldir"})

	// When the root command is executed
	assertNoError(t, root.Execute())

	// Then the relative path is resolved to an absolute path
	if !runner.createArgsContain("/projects/myapp/reldir:/workspace/reldir:Z") {
		t.Errorf("expected '/projects/myapp/reldir:/workspace/reldir:Z' in create args, got %v", runner.createArgs())
	}
}

// TestDefaultCmd_MountSaveFails verifies that a --mount flag triggers a config
// save and that the error is propagated with a message mentioning "saving config"
// when the config file cannot be written.
func TestDefaultCmd_MountSaveFails(t *testing.T) {
	// Given a running container and a SaveConfig function that returns an error
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: true, running: true, imageExistsResult: true}
	fe := &fakeExec{}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              fe.exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		SaveConfig:          func(string, *config.Config) error { return fmt.Errorf("disk full") },
	}

	// When the root command is executed with a --mount flag
	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp", "--mount", "/some/path"})
	err := root.Execute()

	// Then an error is returned containing "saving config"
	assertError(t, err)
	assertContains(t, err.Error(), "saving config")
}
