// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd_test

import (
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/cmd"
	"github.com/rob-broadley/ai-airbase/marshal/internal/config"
)

// TestDefaultCmd_MountsCWD verifies that marshal mounts the current working
// directory inside /workspace/<basename> when no mounts are configured.
func TestDefaultCmd_MountsCWD(t *testing.T) {
	// Given no mounts configured and empty config
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false, imageExistsResult: true}
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
	if !runner.createArgsContain("/projects/myapp:/workspace/myapp:z") {
		t.Errorf("expected create args to contain '/projects/myapp:/workspace/myapp:z', got %v", runner.createArgs())
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

	runner := &fakeRunner{exists: false, imageExistsResult: true}
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
	if !runner.createArgsContain("/abs/saved:/workspace/saved:z") {
		t.Errorf("expected '/abs/saved:/workspace/saved:z' in create args, got %v", runner.createArgs())
	}
	if runner.createArgsContain("/projects/myapp:/workspace/myapp:z") {
		t.Error("CWD should NOT be mounted when saved mounts exist")
	}
}

// TestDefaultCmd_CustomImage verifies that setting MARSHAL_IMAGE overrides the
// default container image.
func TestDefaultCmd_CustomImage(t *testing.T) {
	// Given MARSHAL_IMAGE is set to a custom image
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("MARSHAL_IMAGE", "myregistry/revetment:v2")

	runner := &fakeRunner{exists: false, imageExistsResult: true}
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
