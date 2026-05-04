// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/cmd"
	"github.com/rob-broadley/ai-airbase/marshal/internal/config"
)

// TestDefaultCmd_CreateAndStart verifies that marshal creates the container and
// connects to PID 1 via podman start --attach --interactive when no container
// exists. The runner must NOT call podman start (that goes via ExecFn).
func TestDefaultCmd_CreateAndStart(t *testing.T) {
	// Given no container exists
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false, running: false}
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

	// Then create is called via runner
	if !runner.calledSubcommand("create") {
		t.Error("expected 'podman create' to be called")
	}
	// And start is NOT called via runner (it goes via ExecFn instead)
	if runner.calledSubcommand("start") {
		t.Error("expected 'podman start' NOT to be called via runner for new container")
	}
	// And ExecFn is called with podman start --attach --interactive (not podman exec)
	if !fe.called {
		t.Fatal("expected exec to be called")
	}
	if !sliceContains(fe.argv, "start") || !sliceContains(fe.argv, "--attach") {
		t.Errorf("expected 'start' and '--attach' in exec argv, got %v", fe.argv)
	}
}

// TestDefaultCmd_ReuseRunning verifies that marshal skips create and connects
// to the existing PID 1 via podman attach when the container is already running.
func TestDefaultCmd_ReuseRunning(t *testing.T) {
	// Given the container is already running
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: true, running: true}
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

	// Then create and start are NOT called via runner
	if runner.calledSubcommand("create") {
		t.Error("expected 'podman create' NOT to be called")
	}
	if runner.calledSubcommand("start") {
		t.Error("expected 'podman start' NOT to be called via runner")
	}
	if !fe.called {
		t.Fatal("expected exec to be called")
	}
	// Running container → attach to existing PID 1 (not exec + copilot)
	if !sliceContains(fe.argv, "attach") {
		t.Errorf("expected 'attach' in exec argv for running container, got %v", fe.argv)
	}
	if sliceContains(fe.argv, "exec") {
		t.Errorf("expected 'exec' NOT in exec argv for running container, got %v", fe.argv)
	}
}

// TestDefaultCmd_RestartStopped verifies that marshal connects to PID 1 via
// podman start --attach --interactive when the container exists but is stopped.
// The runner must NOT call podman start (that goes via ExecFn).
func TestDefaultCmd_RestartStopped(t *testing.T) {
	// Given the container exists but is stopped
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: true, running: false}
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

	// Then create is NOT called
	if runner.calledSubcommand("create") {
		t.Error("expected 'podman create' NOT to be called")
	}
	// And start is NOT called via runner (it goes via ExecFn instead)
	if runner.calledSubcommand("start") {
		t.Error("expected 'podman start' NOT to be called via runner for stopped container")
	}
	if !fe.called {
		t.Fatal("expected exec to be called")
	}
	// Stopped container → connect to PID 1 with podman start --attach (not podman exec)
	if !sliceContains(fe.argv, "start") || !sliceContains(fe.argv, "--attach") {
		t.Errorf("expected 'start' and '--attach' in exec argv for stopped container, got %v", fe.argv)
	}
}

// TestDefaultCmd_ExecArgv verifies that exec is called with the exact expected
// argument vector when no container exists: podman start --attach --interactive.
func TestDefaultCmd_ExecArgv(t *testing.T) {
	// Given no container exists
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
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp"})

	// When the root command is executed
	assertNoError(t, root.Execute())

	// Then exec is called with the exact expected argv: podman start --attach --interactive
	expected := []string{"podman", "start", "--attach", "--interactive", "marshal-myapp"}
	if len(fe.argv) != len(expected) {
		t.Fatalf("exec argv: got %v, want %v", fe.argv, expected)
	}
	for i, v := range expected {
		if fe.argv[i] != v {
			t.Errorf("exec argv[%d]: got %q, want %q", i, fe.argv[i], v)
		}
	}
}

// TestDefaultCmd_AttachArgvWhenRunning verifies that exec is called with the
// exact argv ["podman", "attach", <containerName>] when the container is running.
func TestDefaultCmd_AttachArgvWhenRunning(t *testing.T) {
	// Given the container is already running
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: true, running: true}
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

	// Then ExecFn is called with exactly ["podman", "attach", "marshal-myapp"]
	expected := []string{"podman", "attach", "marshal-myapp"}
	if len(fe.argv) != len(expected) {
		t.Fatalf("exec argv: got %v, want %v", fe.argv, expected)
	}
	for i, v := range expected {
		if fe.argv[i] != v {
			t.Errorf("exec argv[%d]: got %q, want %q", i, fe.argv[i], v)
		}
	}
}

// TestDefaultCmd_ContainerNameConvention verifies that the container name
// follows the marshal-<project> convention.
func TestDefaultCmd_ContainerNameConvention(t *testing.T) {
	// Given no container exists for project my-app
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false}
	fe := &fakeExec{}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              fe.exec,
		Getwd:               func() (string, error) { return "/projects/my-app", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "my-app"})

	// When the root command is executed
	assertNoError(t, root.Execute())

	// Then exec argv contains the marshal-my-app container name
	if !fe.called {
		t.Fatal("expected exec to be called")
	}
	found := false
	for _, a := range fe.argv {
		if a == "marshal-my-app" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected container name 'marshal-my-app' in exec argv, got %v", fe.argv)
	}
}

// TestDefaultCmd_ExistsCheckFails verifies that an error from the Exists check
// (ps --all) is propagated back to the caller.
func TestDefaultCmd_ExistsCheckFails(t *testing.T) {
	// Given a runner that fails on the "ps-all" subcommand used by the Exists check
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{runErrors: map[string]error{"ps-all": fmt.Errorf("ps failed")}}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	// When the root command is executed
	root := cmd.NewRootCmd(deps)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp"})

	// Then an error is returned
	assertError(t, root.Execute())
}

// TestDefaultCmd_ContainerCreateFails verifies that an error from podman create
// is returned with a message mentioning "creating container".
func TestDefaultCmd_ContainerCreateFails(t *testing.T) {
	// Given a runner with no existing container and an injected create error
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		exists:            false,
		imageExistsResult: true,
		runErrors:         map[string]error{"create": fmt.Errorf("create failed")},
	}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	// When the root command is executed
	root := cmd.NewRootCmd(deps)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp"})
	err := root.Execute()

	// Then an error is returned containing "creating container"
	assertError(t, err)
	assertContains(t, err.Error(), "creating container")
}

// TestDefaultCmd_ExecFnError_OnCreate verifies that an error from ExecFn
// (podman start --attach) after creating a container is propagated back.
func TestDefaultCmd_ExecFnError_OnCreate(t *testing.T) {
	// Given a runner with no existing container and an ExecFn that returns an error
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		exists:            false,
		imageExistsResult: true,
	}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{err: fmt.Errorf("start failed")}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	// When the root command is executed
	root := cmd.NewRootCmd(deps)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp"})

	// Then an error is returned
	assertError(t, root.Execute())
}

// TestDefaultCmd_IsRunningCheckFails verifies that an error from the IsRunning
// check (ps without --all) is propagated back to the caller.
func TestDefaultCmd_IsRunningCheckFails(t *testing.T) {
	// Given a container that exists but whose IsRunning check (ps) returns an error
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// Container exists (ps-all succeeds) but IsRunning check (ps) fails.
	runner := &fakeRunner{
		exists:    true,
		running:   false,
		runErrors: map[string]error{"ps": fmt.Errorf("ps failed")},
	}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	// When the root command is executed
	root := cmd.NewRootCmd(deps)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp"})

	// Then an error is returned
	assertError(t, root.Execute())
}

// TestDefaultCmd_ExecFnError_WhenStopped verifies that an error from ExecFn
// (podman start --attach) when the container is stopped is propagated back.
func TestDefaultCmd_ExecFnError_WhenStopped(t *testing.T) {
	// Given an existing but stopped container and an ExecFn that returns an error
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		exists:  true,
		running: false,
	}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{err: fmt.Errorf("start failed")}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	// When the root command is executed
	root := cmd.NewRootCmd(deps)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp"})

	// Then an error is returned
	assertError(t, root.Execute())
}

// TestDefaultCmd_GetwdFails verifies that an error from Getwd is propagated
// back with a message mentioning "getting working directory".
func TestDefaultCmd_GetwdFails(t *testing.T) {
	// Given a Getwd function that returns an error
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: true, running: true}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "", fmt.Errorf("getwd failed") },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	// When the root command is executed
	root := cmd.NewRootCmd(deps)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp"})
	err := root.Execute()

	// Then an error is returned containing "getting working directory"
	assertError(t, err)
	assertContains(t, err.Error(), "getting working directory")
}

// TestDefaultCmd_ConfigLoadFails verifies that an invalid TOML config causes an
// error mentioning "loading config".
func TestDefaultCmd_ConfigLoadFails(t *testing.T) {
	// Given a config file that contains malformed TOML
	tmp := t.TempDir()
	cfgDir := filepath.Join(tmp, "marshal", "projects")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "myapp.toml"), []byte("not = valid [toml"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", tmp)

	runner := &fakeRunner{exists: true, running: true}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	// When the root command is executed
	root := cmd.NewRootCmd(deps)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp"})
	err := root.Execute()

	// Then an error is returned containing "loading config"
	assertError(t, err)
	assertContains(t, err.Error(), "loading config")
}

// TestDefaultCmd_CredentialMountsFail verifies that an error from
// EnsureSharedDataDir is propagated back to the caller.
func TestDefaultCmd_CredentialMountsFail(t *testing.T) {
	// Given an EnsureSharedDataDir function that returns an error
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: true, running: true}
	deps := cmd.Deps{
		Runner: runner,
		ExecFn: (&fakeExec{}).exec,
		Getwd:  func() (string, error) { return "/projects/myapp", nil },
		Getuid: stubGetuid,
		Getgid: stubGetgid,
		EnsureSharedDataDir: func(string) (string, error) {
			return "", fmt.Errorf("credentials failed")
		},
	}

	// When the root command is executed
	root := cmd.NewRootCmd(deps)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp"})

	// Then an error is returned
	assertError(t, root.Execute())
}

// TestDefaultCmd_ErrorMessageContainsContext verifies that when podman create
// fails with a message, the error returned to the caller contains both the
// cmd-level wrapper text ("creating container") and the underlying error text.
func TestDefaultCmd_ErrorMessageContainsContext(t *testing.T) {
	// Given a runner with no existing container and a create error containing "no space left on device"
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		exists:            false,
		imageExistsResult: true,
		runErrors: map[string]error{
			"create": errors.New("exit status 125: Error: no space left on device"),
		},
	}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	// When the root command is executed
	root := cmd.NewRootCmd(deps)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp"})
	err := root.Execute()

	// Then the error contains both the wrapper text and the underlying cause
	assertError(t, err)
	assertContains(t, err.Error(), "creating container")
	assertContains(t, err.Error(), "no space left on device")
}

// TestDefaultCmd_CreatePassesLabels verifies that the three required
// io.ai-airbase.* labels are forwarded to `podman create` so that containers
// created by marshal can be identified and filtered by tooling.
func TestDefaultCmd_CreatePassesLabels(t *testing.T) {
	// Given no container exists for project "myapp" using a specific image
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false, running: false, imageExistsResult: true}
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

	// When the root command is executed (triggers podman create)
	assertNoError(t, root.Execute())

	// Then podman create was called and the args contain all three label pairs
	createArgs := runner.createArgs()
	if createArgs == nil {
		t.Fatal("expected 'podman create' to be called but it was not")
	}

	labelPairs := []struct{ flag, value string }{
		{"--label", "io.ai-airbase.managed-by=marshal"},
		{"--label", "io.ai-airbase.project=marshal-myapp"},
	}
	for _, lp := range labelPairs {
		found := false
		for i, a := range createArgs {
			if a == lp.flag && i+1 < len(createArgs) && createArgs[i+1] == lp.value {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %s %s in create args; full create args: %v", lp.flag, lp.value, createArgs)
		}
	}

	// The image label value depends on the configured image; at minimum the
	// --label io.ai-airbase.image=... key must be present.
	imageLabelFound := false
	for i, a := range createArgs {
		if a == "--label" && i+1 < len(createArgs) {
			if len(createArgs[i+1]) > len("io.ai-airbase.image=") &&
				createArgs[i+1][:len("io.ai-airbase.image=")] == "io.ai-airbase.image=" {
				imageLabelFound = true
				break
			}
		}
	}
	if !imageLabelFound {
		t.Errorf("expected --label io.ai-airbase.image=<image> in create args; full create args: %v", createArgs)
	}
}

// TestDefaultCmd_RecreatesWhenMountsChangedAndStopped verifies that marshal
// automatically removes and recreates a stopped container when the -m flags
// specify different mounts than those saved in config.
func TestDefaultCmd_RecreatesWhenMountsChangedAndStopped(t *testing.T) {
	// Given: container exists (stopped) with mount /old/path saved in config
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := config.Save("myapp", &config.Config{Mounts: []string{"/old/path"}}); err != nil {
		t.Fatalf("saving config: %v", err)
	}

	runner := &fakeRunner{exists: true, running: false, imageExistsResult: true}
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
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "--mount", "/new/path"})

	// When: marshal is run with -m /new/path (different from saved /old/path)
	assertNoError(t, root.Execute())

	// Then: podman rm is called (to remove the old container)
	if !runner.calledSubcommand("rm") {
		t.Error("expected 'podman rm' to be called when mounts change and container is stopped")
	}
	// And: podman create is called with the new mount
	if !runner.calledSubcommand("create") {
		t.Error("expected 'podman create' to be called with new mount")
	}
	if !runner.createArgsContain("/new/path:/workspace/path:Z") {
		t.Errorf("expected new mount '/new/path:/workspace/path:Z' in create args, got %v", runner.createArgs())
	}
	// And: the old mount is NOT used
	if runner.createArgsContain("/old/path:/workspace/path:Z") {
		t.Error("expected old mount NOT to appear in create args after recreate")
	}
	// And: podman start --attach is called via ExecFn
	if !fe.called {
		t.Fatal("expected ExecFn to be called")
	}
	if !sliceContains(fe.argv, "start") || !sliceContains(fe.argv, "--attach") {
		t.Errorf("expected 'start' and '--attach' in ExecFn argv, got %v", fe.argv)
	}
}

// TestDefaultCmd_ErrorsWhenMountsChangedAndRunning verifies that marshal returns
// an actionable error when the -m flags differ from saved config and the
// container is currently running.
func TestDefaultCmd_ErrorsWhenMountsChangedAndRunning(t *testing.T) {
	// Given: container exists (running) with mount /old/path saved in config
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := config.Save("myapp", &config.Config{Mounts: []string{"/old/path"}}); err != nil {
		t.Fatalf("saving config: %v", err)
	}

	runner := &fakeRunner{exists: true, running: true, imageExistsResult: true}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	root := cmd.NewRootCmd(deps)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "--mount", "/new/path"})

	// When: marshal is run with -m /new/path (different from saved /old/path)
	err := root.Execute()

	// Then: an error is returned mentioning the running state and how to resolve it
	assertError(t, err)
	assertContains(t, err.Error(), "running")
	assertContains(t, err.Error(), "marshal stop")
}

// TestDefaultCmd_NoRecreateWhenMountsSame verifies that marshal does NOT
// remove or recreate a stopped container when the -m flags match the mounts
// already saved in config (order-independent comparison).
func TestDefaultCmd_NoRecreateWhenMountsSame(t *testing.T) {
	// Given: container exists (stopped) with mounts [/foo, /bar] saved in config
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := config.Save("myapp", &config.Config{Mounts: []string{"/foo", "/bar"}}); err != nil {
		t.Fatalf("saving config: %v", err)
	}

	runner := &fakeRunner{exists: true, running: false, imageExistsResult: true}
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
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "--mount", "/foo", "--mount", "/bar"})

	// When: marshal is run with -m /foo -m /bar (same mounts as saved config)
	assertNoError(t, root.Execute())

	// Then: no rm or create call is made (container is reused as-is)
	if runner.calledSubcommand("rm") {
		t.Error("expected 'podman rm' NOT to be called when mounts are unchanged")
	}
	if runner.calledSubcommand("create") {
		t.Error("expected 'podman create' NOT to be called when mounts are unchanged")
	}
	// And: the container is simply started via ExecFn
	if !fe.called {
		t.Fatal("expected ExecFn to be called")
	}
	if !sliceContains(fe.argv, "start") || !sliceContains(fe.argv, "--attach") {
		t.Errorf("expected 'start' and '--attach' in ExecFn argv, got %v", fe.argv)
	}
}
