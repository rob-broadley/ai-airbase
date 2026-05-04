// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/cmd"
)

// TestPull_CallsPullImageWithCorrectImage verifies that the pull subcommand
// calls PullImage with the revetment image reference.
func TestPull_CallsPullImageWithCorrectImage(t *testing.T) {
	// Given a runner spy
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{}
	out := &bytes.Buffer{}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		Stdout:              out,
		Stderr:              &bytes.Buffer{},
	}

	root := cmd.NewRootCmd(deps)

	// When the pull subcommand is executed
	root.SetArgs([]string{"pull"})
	assertNoError(t, root.Execute())

	// Then PullImage is called with the revetment image
	if !runner.pullImageCalled {
		t.Error("expected PullImage to be called by the pull subcommand")
	}
	if runner.pullImageImage != "ghcr.io/rob-broadley/ai-airbase/revetment:latest" {
		t.Errorf("expected PullImage called with revetment image, got %q", runner.pullImageImage)
	}
}

// TestPull_PrintsSuccessMessage verifies that the pull subcommand prints a
// success message containing the image name on stdout.
func TestPull_PrintsSuccessMessage(t *testing.T) {
	// Given a runner spy
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{}
	out := &bytes.Buffer{}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		Stdout:              out,
		Stderr:              &bytes.Buffer{},
	}

	root := cmd.NewRootCmd(deps)

	// When the pull subcommand is executed
	root.SetArgs([]string{"pull"})
	assertNoError(t, root.Execute())

	// Then stdout contains a success message with the image name
	got := out.String()
	if !strings.Contains(got, "pulled successfully") {
		t.Errorf("expected success message in stdout, got: %q", got)
	}
	if !strings.Contains(got, "ghcr.io/rob-broadley/ai-airbase/revetment:latest") {
		t.Errorf("expected image name in success message, got: %q", got)
	}
}

// TestPull_ReturnsErrorOnPullFailure verifies that the pull subcommand returns
// an error when PullImage fails.
func TestPull_ReturnsErrorOnPullFailure(t *testing.T) {
	// Given a runner that will fail on pull
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	pullErr := errors.New("connection refused")
	runner := &fakeRunner{pullImageErr: pullErr}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		Stdout:              &bytes.Buffer{},
		Stderr:              &bytes.Buffer{},
	}

	root := cmd.NewRootCmd(deps)
	root.SetErr(&bytes.Buffer{})

	// When the pull subcommand is executed
	root.SetArgs([]string{"pull"})

	// Then an error is returned
	assertError(t, root.Execute())
}

// TestPullFallback_SuppressesPodmanStderr verifies that when pull fails but a
// local image exists, podman's internal WARN/Error messages are not written to
// the user's stderr. Only marshal's own warning should appear there.
func TestPullFallback_SuppressesPodmanStderr(t *testing.T) {
	// Given a runner that fails to pull but finds the image locally, and podman noise on stderr
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	podmanNoise := "WARN[0000] Failed, retrying in 1s ... Error: connection refused\nError: unable to copy from source\n"
	runner := &fakeRunner{
		exists:               false,
		imageExistsResult:    false,
		imageExistsAfterPull: true,
		pullImageErr:         errors.New("exit status 125"),
		pullStderrOutput:     podmanNoise,
	}
	stderr := &bytes.Buffer{}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		Stdout:              &bytes.Buffer{},
		Stderr:              stderr,
	}

	// When the root command is executed
	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp"})
	assertNoError(t, root.Execute())

	// Then marshal's own warning appears in stderr but podman noise is suppressed
	got := stderr.String()
	if strings.Contains(got, "WARN") {
		t.Errorf("expected podman WARN messages to be suppressed, got in stderr: %q", got)
	}
	if strings.Contains(got, "unable to copy") {
		t.Errorf("expected podman Error message to be suppressed, got in stderr: %q", got)
	}
	if !strings.Contains(got, "warning:") {
		t.Errorf("expected marshal warning in stderr, got: %q", got)
	}
}
