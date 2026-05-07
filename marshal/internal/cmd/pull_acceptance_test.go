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
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
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
	}

	root := cmd.NewRootCmd(deps)
	root.SetOut(out)

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
	}

	root := cmd.NewRootCmd(deps)
	root.SetErr(&bytes.Buffer{})

	// When the pull subcommand is executed
	root.SetArgs([]string{"pull"})

	// Then an error is returned
	assertError(t, root.Execute())
}

// TestPullFallback_ForwardsPodmanStderrToUser verifies that when pull fails but
// a local image exists, podman's pull stderr (auth failures, rate-limit messages,
// etc.) is forwarded to the user's stderr alongside marshal's own warning.
func TestPullFallback_ForwardsPodmanStderrToUser(t *testing.T) {
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
	var logBuf bytes.Buffer
	stderr := &bytes.Buffer{}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		Logger:              cmd.NewCLILogger(&logBuf),
	}

	// When the root command is executed
	root := cmd.NewRootCmd(deps)
	root.SetErr(stderr)
	root.SetArgs([]string{"--project", "myapp"})
	assertNoError(t, root.Execute())

	// Then podman's pull stderr is forwarded to the user so they can diagnose failures
	got := stderr.String()
	if !strings.Contains(got, "WARN") {
		t.Errorf("expected podman WARN message to be forwarded to user stderr, got: %q", got)
	}
	if !strings.Contains(got, "unable to copy") {
		t.Errorf("expected podman Error message to be forwarded to user stderr, got: %q", got)
	}
	// Marshal's fallback warning is routed through the progress logger, not cobra's stderr
	if strings.Contains(got, "warning:") {
		t.Errorf("expected marshal warning to be absent from cobra stderr (should be in logger), got: %q", got)
	}
	if !strings.Contains(logBuf.String(), "pull failed") {
		t.Errorf("expected marshal warning in progress logger output, got: %q", logBuf.String())
	}
}

// TestLocalImage_SkipsPullWhenImageExists verifies that a bare image name
// (no registry hostname, e.g. MARSHAL_IMAGE=revetment) is treated as a
// local-only image: no pull is attempted even when recreate is called.
func TestLocalImage_SkipsPullWhenImageExists(t *testing.T) {
	// Given a local image that exists and a resolver returning a bare name
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{imageExistsResult: true}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		ResolveImage:        func() string { return "revetment" },
	}

	root := cmd.NewRootCmd(deps)
	root.SetArgs([]string{"--project", "myapp", "recreate"})
	assertNoError(t, root.Execute())

	// Then PullImage is never called
	if runner.pullImageCalled {
		t.Error("expected PullImage NOT to be called for a local-only image name")
	}
}

// TestLocalImage_ErrorsWhenImageAbsent verifies that a bare image name that is
// not present locally produces an actionable error without attempting a pull.
func TestLocalImage_ErrorsWhenImageAbsent(t *testing.T) {
	// Given a local image that does NOT exist
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	runner := &fakeRunner{imageExistsResult: false}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		ResolveImage:        func() string { return "revetment" },
	}

	root := cmd.NewRootCmd(deps)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"--project", "myapp", "recreate"})
	err := root.Execute()

	// Then an error is returned and PullImage was never called
	assertError(t, err)
	assertContains(t, err.Error(), "not found")
	if runner.pullImageCalled {
		t.Error("expected PullImage NOT to be called for a local-only image name")
	}
}

// that when pull fails AND the subsequent ImageExists check also fails, the
// returned error message includes context from both failures so the user is not
// left with a cryptic "exit status 1" message.
func TestPullFallback_BothPullAndImageExistsFail_ErrorMentionsBothFailures(t *testing.T) {
	// Given a runner that fails to pull AND fails to check local image existence
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	pullErr := errors.New("connection refused")
	existsErr := errors.New("images lookup failed")
	runner := &fakeRunner{
		pullImageErr:   pullErr,
		imageExistsErr: existsErr,
	}
	deps := cmd.Deps{
		Runner:              runner,
		ExecFn:              (&fakeExec{}).exec,
		Getwd:               func() (string, error) { return "/projects/myapp", nil },
		Getuid:              stubGetuid,
		Getgid:              stubGetgid,
		EnsureSharedDataDir: stubEnsureSharedDataDir(t),
		ResolveImage:        func() string { return "ghcr.io/rob-broadley/ai-airbase/revetment:latest" },
	}

	root := cmd.NewRootCmd(deps)
	root.SetErr(&bytes.Buffer{})

	// When the recreate subcommand is executed (it calls pullImageRefresh →
	// pullImageWithFallback unconditionally, exercising the double-failure path)
	root.SetArgs([]string{"--project", "myapp", "recreate"})
	err := root.Execute()

	// Then an error is returned that mentions both the pull failure and the
	// secondary ImageExists failure, giving the user actionable context
	assertError(t, err)
	assertContains(t, err.Error(), "also failed to check local copy")
	assertContains(t, err.Error(), "images lookup failed")
}
