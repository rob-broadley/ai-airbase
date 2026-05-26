// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd_test

import (
	"fmt"
	"io"
	"os"
	"os/exec"
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
// The directory is created so that callers can write files into it.
func stubEnsureSharedDataDir(t *testing.T) func(string) (string, error) {
	t.Helper()
	base := t.TempDir()
	return func(subdir string) (string, error) {
		p := filepath.Join(base, subdir)
		if err := os.MkdirAll(p, 0o700); err != nil {
			return "", err
		}
		return p, nil
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
	runErrors        map[string]error
	pullImageErr     error
	imageExistsErr   error
	pullOutput       string
	pullStderrOutput string
	image            string
	created          string
	imageVersion     string
	pullImageImage   string
	// imageInspectVolumeJSON overrides the JSON returned for image inspect
	// --format "{{json .Config.Volumes}}". Defaults to "{}" when empty.
	imageInspectVolumeJSON string
	// imageInspectLabelJSON overrides the JSON returned for image inspect
	// --format "{{json .Config.Labels}}". Defaults to "{}" when empty.
	imageInspectLabelJSON string
	calls                 [][]string
	// projectVolumes is the list of volume names returned by "podman volume ls
	// --filter label=io.ai-airbase.project=..." to simulate pre-existing
	// project-labelled volumes.
	projectVolumes       []string
	imageExistsCalls     int
	exists               bool
	running              bool
	imageExistsResult    bool
	imageExistsAfterPull bool
	pullImageCalled      bool
	// renameErrorFn, when set, is called for every "podman rename <from> <to>"
	// invocation. Return a non-nil error to simulate a failure for specific
	// rename operations (e.g. only fail the promotion rename, not the aside).
	// This complements runErrors["rename"], which fails ALL renames uniformly.
	renameErrorFn func(from, to string) error
}

func (f *fakeRunner) Run(name string, args ...string) ([]byte, error) {
	call := make([]string, 0, 1+len(args))
	call = append(call, name)
	call = append(call, args...)
	f.calls = append(f.calls, call)

	// Error injection: if a runError is set for this subcommand, return it.
	if len(f.runErrors) > 0 && name == "podman" && len(args) > 0 {
		key := args[0]
		specificKey := ""
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
		if args[0] == "volume" && len(args) > 1 {
			specificKey = "volume-" + args[1]
		}
		if specificKey != "" {
			if err, ok := f.runErrors[specificKey]; ok {
				return nil, err
			}
		}
		if err, ok := f.runErrors[key]; ok {
			return nil, err
		}
	}

	// Selective rename error injection: allows tests to fail only specific
	// renames (e.g. only the promotion rename, not the aside) without using
	// the blunt runErrors["rename"] which would fail every rename.
	if name == "podman" && len(args) >= 3 && args[0] == "rename" && f.renameErrorFn != nil {
		if err := f.renameErrorFn(args[1], args[2]); err != nil {
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

	case "image":
		if len(args) > 1 && args[1] == "exists" {
			f.imageExistsCalls++
			if f.imageExistsErr != nil {
				return nil, f.imageExistsErr
			}
			exists := f.imageExistsResult
			if f.imageExistsCalls > 1 {
				exists = f.imageExistsAfterPull
			}
			if exists {
				return []byte(""), nil
			}
			return nil, fakeExitError(1)
		}
		// Differentiate between the two image inspect format calls so tests
		// can inject custom volume and label JSON for volume provisioning.
		for i, a := range args {
			if a == "--format" && i+1 < len(args) {
				switch {
				case strings.Contains(args[i+1], "Volumes"):
					if f.imageInspectVolumeJSON != "" {
						return []byte(f.imageInspectVolumeJSON), nil
					}
				case strings.Contains(args[i+1], "Labels"):
					if f.imageInspectLabelJSON != "" {
						return []byte(f.imageInspectLabelJSON), nil
					}
				}
			}
		}
		// Return empty JSON for any image inspect format query (Config.Volumes,
		// Config.Labels, etc.) so callers see no declared volumes by default.
		return []byte("{}"), nil

	case "volume":
		if len(args) > 1 && args[1] == "ls" {
			// Return any pre-configured project volumes, one name per line.
			return []byte(strings.Join(f.projectVolumes, "\n")), nil
		}
		// create, rm — succeed silently.
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
		return []byte(img + "|" + cr + "|" + f.imageVersion + "\n"), nil

	default:
		// create, start, stop, rm — succeed silently
		return []byte(""), nil
	}
}

// RunStreaming is a spy: records the streamed command via the same call log as
// Run, tracks pull invocations, writes configured output to the provided
// writers, and returns pullImageErr (or runErrors["pull"] if set).
func (f *fakeRunner) RunStreaming(name string, stdout io.Writer, stderr io.Writer, args ...string) error {
	call := make([]string, 0, 1+len(args))
	call = append(call, name)
	call = append(call, args...)
	f.calls = append(f.calls, call)

	f.pullImageCalled = true
	f.pullImageImage = ""
	if len(args) > 0 {
		f.pullImageImage = args[len(args)-1]
	}
	if f.pullOutput != "" {
		_, _ = io.WriteString(stdout, f.pullOutput)
	}
	if f.pullStderrOutput != "" {
		_, _ = io.WriteString(stderr, f.pullStderrOutput)
	}
	if err, ok := f.runErrors["pull"]; ok {
		return err
	}
	return f.pullImageErr
}

func fakeExitError(code int) error {
	cmd := exec.Command("sh", "-c", fmt.Sprintf("exit %d", code))
	return cmd.Run()
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

// rmCalledFor reports whether "podman rm <name>" (or "podman rm --force <name>")
// appears in the recorded calls. Unlike calledSubcommand("rm"), this checks the
// specific container argument so tests can distinguish cleanup of a pending
// container from removal of the original one. Handles both the regular remove
// ("podman rm <name>") and force-remove ("podman rm --force <name>") variants.
func (f *fakeRunner) rmCalledFor(name string) bool {
	for _, call := range f.calls {
		if len(call) < 3 || call[0] != "podman" || call[1] != "rm" {
			continue
		}
		// Scan args after "rm" so both "podman rm <name>" and
		// "podman rm --force <name>" are matched.
		for _, arg := range call[2:] {
			if arg == name {
				return true
			}
		}
	}
	return false
}

// rmCalledForPrefix reports whether "podman rm" (or "podman rm --force") was
// called for any container whose name starts with prefix. Use this when the
// exact staging-container name is not known in advance because it includes a
// non-deterministic nanosecond timestamp.
func (f *fakeRunner) rmCalledForPrefix(prefix string) bool {
	for _, call := range f.calls {
		if len(call) < 3 || call[0] != "podman" || call[1] != "rm" {
			continue
		}
		for _, arg := range call[2:] {
			if strings.HasPrefix(arg, prefix) {
				return true
			}
		}
	}
	return false
}

// createArgsContain reports whether any single arg in the create call contains s.
func (f *fakeRunner) createArgsContain(s string) bool {
	for _, a := range f.createArgs() {
		if strings.Contains(a, s) {
			return true
		}
	}
	return false
}

// renameCalledFromPrefix reports whether a "podman rename <from> <to>" was
// recorded where <from> starts with fromPrefix and <to> is exactly to.
// Use this when the staging-container name is not predictable (PID+nano suffix).
func (f *fakeRunner) renameCalledFromPrefix(fromPrefix, to string) bool {
	for _, call := range f.calls {
		if len(call) >= 4 && call[0] == "podman" && call[1] == "rename" &&
			strings.HasPrefix(call[2], fromPrefix) && call[3] == to {
			return true
		}
	}
	return false
}

// renameCalledWithToPrefix reports whether a "podman rename <from> <to>" was
// recorded where <from> is exactly from and <to> starts with toPrefix.
// Use this to verify a container was renamed aside to a retiring/staging name.
func (f *fakeRunner) renameCalledWithToPrefix(from, toPrefix string) bool {
	for _, call := range f.calls {
		if len(call) >= 4 && call[0] == "podman" && call[1] == "rename" &&
			call[2] == from && strings.HasPrefix(call[3], toPrefix) {
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

// credFakes bundles injectable credential-related deps so tests can inspect
// calls and compute expected mount values. It maintains separate base
// directories for XDG_CONFIG (configBase) and XDG_DATA (dataBase) paths so
// tests can assert that files end up in the correct XDG location.
type credFakes struct {
	dataDirFn   func(string) (string, error)
	configDirFn func(string) (string, error)
	dataBase    string
	configBase  string
	calls       []string
}

// newCredFakes returns a credFakes wired to per-test temp directories.
// Both directories are created so that callers can write files into them.
func newCredFakes(t *testing.T) *credFakes {
	t.Helper()
	dataBase := t.TempDir()
	configBase := t.TempDir()
	cf := &credFakes{dataBase: dataBase, configBase: configBase}
	cf.dataDirFn = func(subdir string) (string, error) {
		cf.calls = append(cf.calls, "data:"+subdir)
		p := filepath.Join(dataBase, subdir)
		if err := os.MkdirAll(p, 0o700); err != nil {
			return "", err
		}
		return p, nil
	}
	cf.configDirFn = func(subdir string) (string, error) {
		cf.calls = append(cf.calls, "config:"+subdir)
		p := filepath.Join(configBase, subdir)
		if err := os.MkdirAll(p, 0o700); err != nil {
			return "", err
		}
		return p, nil
	}
	return cf
}

// expectedDataMount returns the -v flag value for a data subdir→containerPath pair.
func (cf *credFakes) expectedDataMount(subdir, containerPath string) string {
	return filepath.Join(cf.dataBase, subdir) + ":" + containerPath + ":Z"
}

// expectedConfigMount returns the -v flag value for a config subdir→containerPath pair.
func (cf *credFakes) expectedConfigMount(subdir, containerPath string) string {
	return filepath.Join(cf.configBase, subdir) + ":" + containerPath + ":Z"
}
