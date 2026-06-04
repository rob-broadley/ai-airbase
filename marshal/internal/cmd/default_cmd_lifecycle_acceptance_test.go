// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/cmd"
)

// TestDefaultCmd_CreateAndStart verifies that marshal creates the container and
// starts it in the background when no container exists.
func TestDefaultCmd_CreateAndStart(t *testing.T) {
	// Given no container exists
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
		IsPortBound:         func(int) bool { return false },
	}

	// When the root command is executed
	out, _, err := runCmd(t, deps, "--project", "myapp")
	assertNoError(t, err)

	// Then create is called via runner
	if !runner.calledSubcommand("create") {
		t.Error("expected 'podman create' to be called")
	}
	// And start IS called via runner
	if !runner.calledSubcommand("start") {
		t.Error("expected 'podman start' to be called via runner for new container")
	}
	// And start is called with the expected container name
	foundStart := false
	for _, call := range runner.calls {
		if len(call) >= 3 && call[0] == "podman" && call[1] == "start" && call[2] == "marshal-myapp" {
			foundStart = true
			break
		}
	}
	if !foundStart {
		t.Errorf("expected 'podman start marshal-myapp' call, got %v", runner.calls)
	}
	// And ExecFn is NOT called
	if fe.called {
		t.Error("expected exec NOT to be called")
	}

	// And terminal prints the start messages
	expectedOutput := "container marshal-myapp started\nOpenCode Web is available at http://127.0.0.1:4096/\n"
	if out.String() != expectedOutput {
		t.Errorf("expected stdout:\n%q\ngot:\n%q", expectedOutput, out.String())
	}
}

// TestDefaultCmd_ReuseRunning verifies that marshal skips create and start
// and prints informational status when the container is already running.
func TestDefaultCmd_ReuseRunning(t *testing.T) {
	// Given the container is already running
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
		IsPortBound:         func(int) bool { return false },
	}

	// When the root command is executed
	out, _, err := runCmd(t, deps, "--project", "myapp")
	assertNoError(t, err)

	// Then create and start are NOT called via runner
	if runner.calledSubcommand("create") {
		t.Error("expected 'podman create' NOT to be called")
	}
	if runner.calledSubcommand("start") {
		t.Error("expected 'podman start' NOT to be called via runner")
	}
	// And ExecFn is NOT called
	if fe.called {
		t.Error("expected exec NOT to be called")
	}

	// And terminal prints the already running messages
	expectedOutput := "container marshal-myapp is already running\nOpenCode Web is available at http://127.0.0.1:4096/\n"
	if out.String() != expectedOutput {
		t.Errorf("expected stdout:\n%q\ngot:\n%q", expectedOutput, out.String())
	}
}

// TestDefaultCmd_RestartStopped verifies that marshal starts the container in
// the background when the container exists but is stopped.
func TestDefaultCmd_RestartStopped(t *testing.T) {
	// Given the container exists but is stopped
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: true, running: false, imageExistsResult: true}
	fe := &fakeExec{}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              fe.exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		IsPortBound:         func(int) bool { return false },
	}

	// When the root command is executed
	out, _, err := runCmd(t, deps, "--project", "myapp")
	assertNoError(t, err)

	// Then create is NOT called
	if runner.calledSubcommand("create") {
		t.Error("expected 'podman create' NOT to be called")
	}
	// And start IS called via runner
	if !runner.calledSubcommand("start") {
		t.Error("expected 'podman start' to be called via runner for stopped container")
	}
	// And ExecFn is NOT called
	if fe.called {
		t.Error("expected exec NOT to be called")
	}

	// And terminal prints the start messages
	expectedOutput := "container marshal-myapp started\nOpenCode Web is available at http://127.0.0.1:4096/\n"
	if out.String() != expectedOutput {
		t.Errorf("expected stdout:\n%q\ngot:\n%q", expectedOutput, out.String())
	}
}

// TestDefaultCmd_ContainerNameConvention verifies that the container name
// follows the marshal-<project> convention.
func TestDefaultCmd_ContainerNameConvention(t *testing.T) {
	// Given no container exists for project my-app
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false, imageExistsResult: true}
	fe := &fakeExec{}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              fe.exec,
		Getwd:               func() (string, error) { return "/projects/my-app", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	// When the root command is executed
	_, _, err := runCmd(t, deps, "--project", "my-app")
	assertNoError(t, err)

	// Then runner start is called with container name 'marshal-my-app'
	found := false
	for _, call := range runner.calls {
		if len(call) >= 3 && call[0] == "podman" && call[1] == "start" && call[2] == "marshal-my-app" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected container name 'marshal-my-app' in runner start call, got %v", runner.calls)
	}
}

// TestDefaultCmd_ExistsCheckFails verifies that an error from the Exists check
// (ps --all) is propagated back to the caller.
func TestDefaultCmd_ExistsCheckFails(t *testing.T) {
	// Given a runner that fails on the "ps-all" subcommand used by the Exists check
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{runErrors: map[string]error{"ps-all": fmt.Errorf("ps failed")}, imageExistsResult: true}
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

// TestDefaultCmd_StartError_OnCreate verifies that an error from starting
// the container after creating it is propagated back.
func TestDefaultCmd_StartError_OnCreate(t *testing.T) {
	// Given a runner with no existing container and a start error
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		exists:            false,
		imageExistsResult: true,
		runErrors:         map[string]error{"start": fmt.Errorf("start failed")},
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

// TestDefaultCmd_StartError_WhenStopped verifies that an error from starting
// the container when it is stopped is propagated back.
func TestDefaultCmd_StartError_WhenStopped(t *testing.T) {
	// Given an existing but stopped container and a start error
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		exists:    true,
		running:   false,
		runErrors: map[string]error{"start": fmt.Errorf("start failed")},
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

// TestDefaultCmd_GetwdFails verifies that an error from Getwd is propagated
// back with a message mentioning "getting working directory".
func TestDefaultCmd_GetwdFails(t *testing.T) {
	// Given a Getwd function that returns an error
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: true, running: true, imageExistsResult: true}
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

	runner := &fakeRunner{exists: true, running: true, imageExistsResult: true}
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

	runner := &fakeRunner{exists: true, running: true, imageExistsResult: true}
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

// TestDefaultCmd_CreateUsesImageCMD verifies that when marshal creates a container,
// it does not specify any trailing command arguments. This allows the container
// to fall back to the CMD defined inside the image's Containerfile, enabling
// better compatibility across different or older versions of the container image.
func TestDefaultCmd_CreateUsesImageCMD(t *testing.T) {
	// Given no container exists for project "myapp" and deps are configured
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
		IsPortBound:         func(int) bool { return false },
	}

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp"})

	// When the default command is executed
	assertNoError(t, root.Execute())

	// Then no trailing command arguments are forwarded to podman create, allowing it to fall back to the image CMD
	createArgs := runner.createArgs()
	if createArgs == nil {
		t.Fatal("expected 'podman create' to be called but it was not")
	}

	lastArg := createArgs[len(createArgs)-1]
	if strings.Contains(lastArg, "opencode") || strings.Contains(lastArg, "web") {
		t.Errorf("expected no trailing command in create args; got: %v", createArgs)
	}
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
		IsPortBound:         func(int) bool { return false },
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

// TestDefaultCmd_InvalidProjectName_ErrorNamesTheRules verifies that if marshal
// is run with an invalid project name containing disallowed characters, the execution
// is aborted with an error, and the error message names the exact validation rules
// (1-128 chars, alphanumeric, hyphens, underscores, or dots).
func TestDefaultCmd_InvalidProjectName_ErrorNamesTheRules(t *testing.T) {
	// Given no container exists
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{exists: false, running: false, imageExistsResult: true}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/workspace/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
	}

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "invalid@project"})

	var errBuf bytes.Buffer
	root.SetErr(&errBuf)

	// When the command is executed
	err := root.Execute()

	// Then the execution is aborted with an error
	if err == nil {
		t.Fatal("expected Execute() to return error due to invalid project name")
	}

	// And the error message contains the exact constraints (1-128 chars, alphanumeric, hyphens, underscores, or dots)
	errMsg := err.Error()
	for _, expectedStr := range []string{
		"1-128 chars",
		"alphanumeric",
		"hyphens",
		"underscores",
		"dots",
	} {
		if !strings.Contains(errMsg, expectedStr) {
			t.Errorf("expected error message to contain %q; got: %q", expectedStr, errMsg)
		}
	}
}
