// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd_test

import (
	"io"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Credential dep helpers for existing tests that don't assert on credential
// mount values but still need non-nil injections.
// ---------------------------------------------------------------------------

// stubEnsureSharedDataDir returns a fake EnsureSharedDataDir that maps subdirs
// into a per-test temp directory without touching the real XDG_DATA_HOME.
func stubEnsureSharedDataDir(t *testing.T) func(string) (string, error) {
	t.Helper()
	base := t.TempDir()
	return func(subdir string) (string, error) {
		return filepath.Join(base, subdir), nil
	}
}

// stubGetuid returns a fixed host UID (1001) for use in dependency injection.
func stubGetuid() int { return 1001 }

// stubGetgid returns a fixed host GID (1001) for use in dependency injection.
func stubGetgid() int { return 1001 }

// ---------------------------------------------------------------------------
// fakeRunner — simulates podman responses based on container state.
// ---------------------------------------------------------------------------

type fakeRunner struct {
	runErrors            map[string]error
	pullImageErr         error
	imageExistsErr       error
	pullOutput           string
	pullStderrOutput     string
	image                string
	created              string
	pullImageImage       string
	calls                [][]string
	imageExistsCalls     int
	exists               bool
	running              bool
	imageExistsResult    bool
	imageExistsAfterPull bool
	pullImageCalled      bool
}

func (f *fakeRunner) Run(name string, args ...string) ([]byte, error) {
	call := make([]string, 0, 1+len(args))
	call = append(call, name)
	call = append(call, args...)
	f.calls = append(f.calls, call)

	// Error injection: if a runError is set for this subcommand, return it.
	if len(f.runErrors) > 0 && name == "podman" && len(args) > 0 {
		key := args[0]
		if args[0] == "ps" {
			hasAll := false
			for _, a := range args {
				if a == "--all" {
					hasAll = true
					break
				}
			}
			if hasAll {
				key = "ps-all"
			}
		}
		if err, ok := f.runErrors[key]; ok {
			return nil, err
		}
	}

	if name != "podman" || len(args) == 0 {
		return []byte(""), nil
	}

	switch args[0] {
	case "ps":
		containerName := ""
		for i, a := range args {
			if a == "--filter" && i+1 < len(args) && strings.HasPrefix(args[i+1], "name=") {
				// Strip "name=" prefix and any anchoring regex chars (^ and $)
				// added by queryContainerNames for exact-match filtering.
				raw := strings.TrimPrefix(args[i+1], "name=")
				raw = strings.TrimPrefix(raw, "^")
				raw = strings.TrimSuffix(raw, "$")
				containerName = raw
			}
		}
		includeAll := false
		for _, a := range args {
			if a == "--all" {
				includeAll = true
				break
			}
		}
		if (includeAll && f.exists) || (!includeAll && f.running) {
			return []byte(containerName + "\n"), nil
		}
		return []byte(""), nil

	case "inspect":
		img := f.image
		if img == "" {
			img = "ghcr.io/rob-broadley/ai-airbase/revetment:latest"
		}
		cr := f.created
		if cr == "" {
			cr = "2024-01-01"
		}
		return []byte(img + "|" + cr + "\n"), nil

	default:
		// create, start, stop, rm — succeed silently
		return []byte(""), nil
	}
}

// ImageExists satisfies the container.Runner interface. The first call returns
// imageExistsResult; subsequent calls return imageExistsAfterPull (to simulate
// the "image appeared locally after a failed pull" scenario).
func (f *fakeRunner) ImageExists(_ string) (bool, error) {
	f.imageExistsCalls++
	if f.imageExistsCalls > 1 {
		return f.imageExistsAfterPull, f.imageExistsErr
	}
	return f.imageExistsResult, f.imageExistsErr
}

// PullImage is a spy: records that it was called and which image was requested,
// writes pullOutput to stdout and pullStderrOutput to stderr when non-empty,
// and returns pullImageErr.
func (f *fakeRunner) PullImage(image string, stdout io.Writer, stderr io.Writer) error {
	f.pullImageCalled = true
	f.pullImageImage = image
	if f.pullOutput != "" {
		_, _ = io.WriteString(stdout, f.pullOutput)
	}
	if f.pullStderrOutput != "" {
		_, _ = io.WriteString(stderr, f.pullStderrOutput)
	}
	return f.pullImageErr
}

// calledSubcommand returns true if "podman <sub>" appears in the recorded calls.
func (f *fakeRunner) calledSubcommand(sub string) bool {
	for _, call := range f.calls {
		if len(call) >= 2 && call[0] == "podman" && call[1] == sub {
			return true
		}
	}
	return false
}

// createArgs returns the arg slice for the first "podman create" call (nil if absent).
func (f *fakeRunner) createArgs() []string {
	for _, call := range f.calls {
		if len(call) >= 2 && call[0] == "podman" && call[1] == "create" {
			return call[2:]
		}
	}
	return nil
}

// createArgsContain reports whether any single arg in the create call equals s.
func (f *fakeRunner) createArgsContain(s string) bool {
	for _, a := range f.createArgs() {
		if a == s {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// fakeExec — captures argv and returns a pre-set error.
// ---------------------------------------------------------------------------

type fakeExec struct {
	err    error
	argv   []string
	called bool
}

// exec records the call and returns the pre-configured error, if any.
func (fe *fakeExec) exec(argv []string) error {
	fe.called = true
	fe.argv = argv
	return fe.err
}

// ---------------------------------------------------------------------------
// assertion helpers (standard library style — no external test frameworks)
// ---------------------------------------------------------------------------

// assertNoError fails the test immediately if err is non-nil.
func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

// assertError fails the test if err is nil.
func assertError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
}

// assertContains fails the test if s does not contain substr.
func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !strings.Contains(s, substr) {
		t.Errorf("expected %q to contain %q", s, substr)
	}
}

// sliceContains reports whether any element of ss equals s (exact match).
// Useful when asserting that a captured args slice contains a specific value
// after the originating runner is no longer in scope.
func sliceContains(ss []string, s string) bool {
	for _, a := range ss {
		if a == s {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// credFakes — injectable credential dependencies for mount and isolation tests
// ---------------------------------------------------------------------------

// credFakes bundles the three injectable credential-related deps so tests can
// inspect calls and compute expected mount values.
type credFakes struct {
	ensureFn func(string) (string, error)
	base     string
	calls    []string
}

// newCredFakes returns a credFakes wired to a per-test temp directory.
func newCredFakes(t *testing.T) *credFakes {
	t.Helper()
	base := t.TempDir()
	cf := &credFakes{base: base}
	cf.ensureFn = func(subdir string) (string, error) {
		cf.calls = append(cf.calls, subdir)
		return filepath.Join(base, subdir), nil
	}
	return cf
}

// expectedMount returns the -v flag value for a subdir→containerPath pair.
func (cf *credFakes) expectedMount(subdir, containerPath string) string {
	return filepath.Join(cf.base, subdir) + ":" + containerPath + ":Z"
}
