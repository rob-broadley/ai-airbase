// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd_test

import (
	"bytes"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/cmd"
)

// TestDefaultCmd_UserConfigSet verifies that the default command passes the
// host UID:GID and HOME=/home/opencode when creating the container.
func TestDefaultCmd_UserConfigSet(t *testing.T) {
	// Given a runner with no existing container and UID 1001 / GID 1002
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := cmd.Deps{
		Runner: runner,
		ExecFn: (&fakeExec{}).exec,
		Getwd:  func() (string, error) { return "/projects/myapp", nil },
		Getuid: func() int { return 1001 },
		Getgid: func() int { return 1002 },
	}

	// When the root command is executed
	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp"})
	assertNoError(t, root.Execute())

	// Then --user, HOME, and --passwd-entry are set correctly in create args
	if !runner.createArgsContain("1001:1002") {
		t.Errorf("expected '--user 1001:1002' in create args\ngot: %v", runner.createArgs())
	}
	if !runner.createArgsContain("HOME=/home/opencode") {
		t.Errorf("expected '-e HOME=/home/opencode' in create args\ngot: %v", runner.createArgs())
	}
	if !runner.createArgsContain("--passwd-entry") {
		t.Errorf("expected --passwd-entry in create args\ngot: %v", runner.createArgs())
	}
	if !runner.createArgsContain("opencode:x:1001:1002::/home/opencode:/bin/bash") {
		t.Errorf("expected opencode:x:1001:1002::/home/opencode:/bin/bash in passwd-entry\ngot: %v", runner.createArgs())
	}
}

// TestDefaultCmd_PasswdEntrySet verifies that --passwd-entry is passed to
// podman create, mapping the host UID:GID to the opencode username.
func TestDefaultCmd_PasswdEntrySet(t *testing.T) {
	// Given a runner with no existing container and UID 1001 / GID 1002
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := cmd.Deps{
		Runner: runner,
		ExecFn: (&fakeExec{}).exec,
		Getwd:  func() (string, error) { return "/projects/myapp", nil },
		Getuid: func() int { return 1001 },
		Getgid: func() int { return 1002 },
	}

	// When the root command is executed
	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp"})
	assertNoError(t, root.Execute())

	// Then --passwd-entry is passed to podman create with the correct opencode user mapping
	if !runner.createArgsContain("--passwd-entry") {
		t.Errorf("expected --passwd-entry in create args\ngot: %v", runner.createArgs())
	}
	// The entry should name the user opencode with UID 1001, GID 1002 and correct home
	if !runner.createArgsContain("opencode:x:1001:1002::/home/opencode:/bin/bash") {
		t.Errorf("expected opencode:x:1001:1002::/home/opencode:/bin/bash in passwd-entry value\ngot: %v", runner.createArgs())
	}
}

// TestDefaultCmd_TtyAllocated verifies that podman create is called with --tty
// so that the container has a pseudo-terminal available for interactive use.
func TestDefaultCmd_TtyAllocated(t *testing.T) {
	// Given a runner with no existing container
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := cmd.Deps{
		Runner: runner,
		ExecFn: (&fakeExec{}).exec,
		Getwd:  func() (string, error) { return "/projects/myapp", nil },
		Getuid: func() int { return 1001 },
		Getgid: func() int { return 1002 },
	}

	// When the root command is executed
	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp"})
	assertNoError(t, root.Execute())

	// Then --tty is passed to podman create
	if !runner.createArgsContain("--tty") {
		t.Errorf("expected --tty in podman create args (required for interactive TTY)\ngot: %v", runner.createArgs())
	}
}

// TestDefaultCmd_StdinOpen verifies that podman create is called with --interactive
// so that stdin is connected to the PTY when the container starts.
// Without --interactive (OpenStdin=false), podman start --attach --interactive
// does not properly connect stdin to the container PTY; the opencode process then
// detects no interactive terminal and exits.
func TestDefaultCmd_StdinOpen(t *testing.T) {
	// Given a runner with no existing container
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := cmd.Deps{
		Runner: runner,
		ExecFn: (&fakeExec{}).exec,
		Getwd:  func() (string, error) { return "/projects/myapp", nil },
		Getuid: func() int { return 1001 },
		Getgid: func() int { return 1002 },
	}

	// When the root command is executed
	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp"})
	assertNoError(t, root.Execute())

	// Then --interactive is passed to podman create
	if !runner.createArgsContain("--interactive") {
		t.Errorf("expected --interactive in podman create args (required for stdin connectivity)\ngot: %v", runner.createArgs())
	}
}

// TestRecreate_UserConfigSet verifies that the recreate command also passes the
// host UID:GID and HOME=/home/opencode when creating the container.
func TestRecreate_UserConfigSet(t *testing.T) {
	// Given a runner with no existing container and UID 1001 / GID 1002
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	runner := &fakeRunner{exists: false, imageExistsResult: true}
	deps := cmd.Deps{
		Runner: runner,
		ExecFn: (&fakeExec{}).exec,
		Getwd:  func() (string, error) { return "/projects/myapp", nil },
		Getuid: func() int { return 1001 },
		Getgid: func() int { return 1002 },
	}

	// When the recreate subcommand is executed
	root := cmd.NewRootCmd(deps)
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "recreate"})
	assertNoError(t, root.Execute())

	// Then --user, HOME, and --passwd-entry are set correctly in create args
	if !runner.createArgsContain("1001:1002") {
		t.Errorf("expected '--user 1001:1002' in recreate create args\ngot: %v", runner.createArgs())
	}
	if !runner.createArgsContain("HOME=/home/opencode") {
		t.Errorf("expected '-e HOME=/home/opencode' in recreate create args\ngot: %v", runner.createArgs())
	}
	if !runner.createArgsContain("--passwd-entry") {
		t.Errorf("expected --passwd-entry in recreate create args\ngot: %v", runner.createArgs())
	}
	if !runner.createArgsContain("opencode:x:1001:1002::/home/opencode:/bin/bash") {
		t.Errorf("expected opencode:x:1001:1002::/home/opencode:/bin/bash in passwd-entry\ngot: %v", runner.createArgs())
	}
}
