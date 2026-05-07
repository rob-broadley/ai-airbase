// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/cmd"
)

// TestAutoPull_ImageAbsent_PullsThenCreatesContainer verifies that marshal
// pulls the image before creating the container when the image is absent locally.
func TestAutoPull_ImageAbsent_PullsThenCreatesContainer(t *testing.T) {
	// Given the image does not exist locally
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		exists:            false,
		running:           false,
		imageExistsResult: false,
	}
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

	// Then PullImage is called with the correct image and the container is created
	if !runner.pullImageCalled {
		t.Error("expected PullImage to be called when image is absent locally")
	}
	if runner.pullImageImage != "ghcr.io/rob-broadley/ai-airbase/revetment:latest" {
		t.Errorf("expected PullImage called with revetment image, got %q", runner.pullImageImage)
	}
	if !runner.calledSubcommand("create") {
		t.Error("expected container to be created after successful pull")
	}
}

// TestAutoPull_ImagePresent_PullNotCalled verifies that marshal skips pulling
// when the image already exists locally.
func TestAutoPull_ImagePresent_PullNotCalled(t *testing.T) {
	// Given the image already exists locally
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		exists:            false,
		running:           false,
		imageExistsResult: true,
	}
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

	// Then PullImage is NOT called and the container is still created
	if runner.pullImageCalled {
		t.Error("expected PullImage NOT to be called when image already exists locally")
	}
	if !runner.calledSubcommand("create") {
		t.Error("expected container to be created")
	}
}

// TestAutoPull_PullFails_NoLocalImage_ReturnsError verifies that marshal returns
// a descriptive error and does not create the container when pull fails and no
// local image is available.
func TestAutoPull_PullFails_NoLocalImage_ReturnsError(t *testing.T) {
	// Given pull will fail and no local image exists
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	pullErr := errors.New("network unreachable")
	runner := &fakeRunner{
		exists:               false,
		imageExistsResult:    false,
		imageExistsAfterPull: false,
		pullImageErr:         pullErr,
	}
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
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp"})

	// When the root command is executed
	err := root.Execute()

	// Then an error is returned mentioning the image and manual action, and no container is created
	assertError(t, err)
	errMsg := err.Error()
	if !strings.Contains(errMsg, "revetment:latest") {
		t.Errorf("expected error to mention image name, got: %v", errMsg)
	}
	if !strings.Contains(errMsg, "manually") {
		t.Errorf("expected error to mention 'manually', got: %v", errMsg)
	}
	if runner.calledSubcommand("create") {
		t.Error("expected container NOT to be created after pull failure with no local image")
	}
}

// TestAutoPull_PullFails_ImageExistsLocally_WarnsAndContinues verifies that
// marshal warns to stderr and continues creating the container when pull fails
// but the image is already in the local store.
func TestAutoPull_PullFails_ImageExistsLocally_WarnsAndContinues(t *testing.T) {
	// Given pull will fail but the image already exists locally
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	pullErr := errors.New("registry timeout")
	runner := &fakeRunner{
		exists:               false,
		imageExistsResult:    false,
		imageExistsAfterPull: true,
		pullImageErr:         pullErr,
	}
	fe := &fakeExec{}
	var logBuf bytes.Buffer
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              fe.exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		Logger:              cmd.NewCLILogger(&logBuf),
	}

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp"})

	// When the root command is executed
	assertNoError(t, root.Execute())

	// Then a pull-failure warning is emitted via the progress logger and the container is created
	if !strings.Contains(logBuf.String(), "pull failed") {
		t.Errorf("expected a pull-failure warning in logger output, got: %q", logBuf.String())
	}
	if !runner.calledSubcommand("create") {
		t.Error("expected container to be created despite pull failure (image exists locally)")
	}
}

// TestDefaultCmd_ImageExistsCheckFails verifies that an error from ImageExists
// is returned with a message mentioning "checking image".
func TestDefaultCmd_ImageExistsCheckFails(t *testing.T) {
	// Given ImageExists returns an error
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{
		exists:         false,
		imageExistsErr: errors.New("image check failed"),
	}
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
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp"})

	// When the root command is executed
	err := root.Execute()

	// Then an error is returned mentioning "checking image"
	assertError(t, err)
	assertContains(t, err.Error(), "checking image")
}
