// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd_test

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/cmd"
	"github.com/rob-broadley/ai-airbase/marshal/internal/config"
	"github.com/rob-broadley/ai-airbase/marshal/internal/container"
)

// listHeader is the exact header line produced by `marshal list`.
// Using a named constant ensures there is one place to update if the
// column names or spacing ever changes.
const listHeader = "NAME  STATUS   PORT   CONFIG\n"

// testContainerName mirrors the production containerNameForProject logic.
// Using this helper in test state-map keys means there is one place to
// update if the naming convention changes, instead of scattered literals.
func testContainerName(project string) string {
	return "marshal-" + project
}

// ---------------------------------------------------------------------------
// listRunner — minimal test double for container.Runner used only by list tests.
// Supports per-container exists/running state via containerStates map.
// ---------------------------------------------------------------------------

// listContainerState holds the exists, running, and per-query error state for a
// single container. queryErr applies to the --all existence query; isRunningErr
// applies to the running-state query without --all.
type listContainerState struct {
	exists       bool
	running      bool
	queryErr     error
	isRunningErr error
}

// listRunner is a minimal container.Runner test double for marshal-list acceptance
// tests. It dispatches `podman ps` responses based on containerStates, returning
// an empty result for containers not in the map (treating them as absent).
type listRunner struct {
	containerStates map[string]listContainerState
}

func (r *listRunner) Run(name string, args ...string) ([]byte, error) {
	if name == "podman" && len(args) > 0 && args[0] == "inspect" {
		containerName := args[len(args)-1]
		if containerName == "marshal-alpha" {
			return []byte("img|2024-01-01|||4096|\n"), nil
		}
		if containerName == "marshal-beta" {
			return []byte("img|2024-01-01|||5000|\n"), nil
		}
		return []byte("img|2024-01-01||||\n"), nil
	}

	if name != "podman" || len(args) == 0 || args[0] != "ps" {
		return []byte(""), nil
	}
	// Extract container name from --filter name=<n> arg.
	containerName := ""
	for i, a := range args {
		if a == "--filter" && i+1 < len(args) && strings.HasPrefix(args[i+1], "name=") {
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
	state, ok := r.containerStates[containerName]
	if !ok {
		return []byte(""), nil // absent
	}
	if includeAll && state.queryErr != nil {
		return nil, state.queryErr
	}
	if !includeAll && state.isRunningErr != nil {
		return nil, state.isRunningErr
	}
	if (includeAll && state.exists) || (!includeAll && state.running) {
		return []byte(containerName + "\n"), nil
	}
	return []byte(""), nil
}

func (r *listRunner) RunStreaming(_ string, _ io.Writer, _ io.Writer, _ ...string) error {
	return nil
}

// ---------------------------------------------------------------------------
// Shared test helpers for list acceptance tests
// ---------------------------------------------------------------------------

// absentListRunner returns a listRunner where no containers exist (empty state map),
// suitable for tests that only care about project enumeration, not container status.
func absentListRunner() *listRunner {
	return &listRunner{containerStates: map[string]listContainerState{}}
}

// newListDeps constructs a cmd.Deps wired with the given runner and test-safe
// stubs for all other dependencies. Used by list acceptance tests to avoid
// repeating the same boilerplate setup across every test function.
func newListDeps(t *testing.T, runner container.Runner) cmd.Deps {
	t.Helper()
	return cmd.Deps{
		Runner: runner,
	}
}

// assertRowContains finds the first line in output that contains rowKey, then
// checks that the same line also contains field. It fails if no matching line
// is found, or if the line does not contain field. This avoids hardcoding
// inter-column spacing in assertions.
func assertRowContains(t *testing.T, output, rowKey, field string) {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == rowKey {
			if !strings.Contains(line, field) {
				t.Errorf("row containing %q: got %q, want it to also contain %q", rowKey, line, field)
			}
			return
		}
	}
	t.Errorf("no row containing %q found in output:\n%s", rowKey, output)
}

func fileHash(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum)
}

// ---------------------------------------------------------------------------
// Acceptance tests for marshal list
// ---------------------------------------------------------------------------

// Walking skeleton

// TestAcceptance_ListWithNoProjects_PrintsHeaderAndExitsZero verifies that
// when the config directory is absent, `marshal list` prints exactly the
// header line and exits 0.
func TestAcceptance_ListWithNoProjects_PrintsHeaderAndExitsZero(t *testing.T) {
	// Given the config directory is absent
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	deps := newListDeps(t, absentListRunner())

	// When marshal list is invoked with no arguments
	out, _, err := runCmd(t, deps, "list")

	// Then the process exits 0
	assertNoError(t, err)

	// And stdout contains exactly one line: NAME  STATUS   CONFIG
	if got := out.String(); got != listHeader {
		t.Errorf("stdout = %q, want %q", got, listHeader)
	}
}

// TestAcceptance_ListHelp_ShowsSubcommandHelpAndExitsZero verifies that
// `marshal list --help` displays help text for the list subcommand and exits 0.
func TestAcceptance_ListHelp_ShowsSubcommandHelpAndExitsZero(t *testing.T) {
	// Given marshal list --help is invoked
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	deps := newListDeps(t, absentListRunner())

	// When the command is executed
	out, _, err := runCmd(t, deps, "list", "--help")

	// Then the process exits 0
	assertNoError(t, err)

	// And the output contains the usage line for the list subcommand
	got := out.String()
	assertContains(t, got, "Usage:")
	assertContains(t, got, "list")
}

// TestAcceptance_ListWithProjectFlag_ProducesIdenticalOutput verifies that
// supplying --project does not change the output or exit code of `marshal list`.
func TestAcceptance_ListWithProjectFlag_ProducesIdenticalOutput(t *testing.T) {
	// Given the config directory is absent
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	deps := newListDeps(t, absentListRunner())

	// When marshal list --project my-app is invoked
	out, _, err := runCmd(t, deps, "list", "--project", "my-app")

	// Then the process exits 0 and no error is produced
	assertNoError(t, err)

	// And stdout contains exactly one line: NAME  STATUS   CONFIG (identical to marshal list without the flag)
	if got := out.String(); got != listHeader {
		t.Errorf("stdout = %q, want %q", got, listHeader)
	}
}

// runListWithProjectFile creates a temp XDG_CONFIG_HOME, writes filename with
// content into the projects directory, and executes marshal list using the
// provided runner. Returns the captured stdout, stderr, and any error returned
// by Execute. Intended for tests that need to inject a raw project config file
// (e.g. malformed TOML) rather than creating a project via config.Save.
func runListWithProjectFile(t *testing.T, filename string, content []byte, runner container.Runner) (out, errOut *bytes.Buffer, err error) {
	t.Helper()
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	projDir := filepath.Join(configHome, "marshal", "projects")
	if mkErr := os.MkdirAll(projDir, 0o755); mkErr != nil {
		t.Fatalf("creating projects dir: %v", mkErr)
	}
	if wErr := os.WriteFile(filepath.Join(projDir, filename), content, 0o644); wErr != nil {
		t.Fatalf("writing %s: %v", filename, wErr)
	}
	return runCmd(t, newListDeps(t, runner), "list")
}

// TestAcceptance_ListWithRunningContainer_ShowsRunningStatus verifies that
// when a project has a running container, `marshal list` emits a row with
// STATUS=running, and exits 0.
func TestAcceptance_ListWithRunningContainer_ShowsRunningStatus(t *testing.T) {
	// Given a registered project alpha with a running container
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	if err := config.Save("alpha", &config.Config{}); err != nil {
		t.Fatalf("saving alpha config: %v", err)
	}

	runner := &listRunner{
		containerStates: map[string]listContainerState{
			testContainerName("alpha"): {exists: true, running: true},
		},
	}
	deps := newListDeps(t, runner)

	// When marshal list is invoked
	out, _, err := runCmd(t, deps, "list")

	// Then the process exits 0
	assertNoError(t, err)

	// And stdout shows a row for alpha with STATUS=running
	assertRowContains(t, out.String(), "alpha", "running")
}

// TestAcceptance_ListWithStoppedContainer_ShowsStoppedStatus verifies that
// when a project has a stopped (exists but not running) container, `marshal list`
// emits a row with STATUS=stopped, and exits 0.
func TestAcceptance_ListWithStoppedContainer_ShowsStoppedStatus(t *testing.T) {
	// Given a registered project beta with a stopped container
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	if err := config.Save("beta", &config.Config{}); err != nil {
		t.Fatalf("saving beta config: %v", err)
	}

	runner := &listRunner{
		containerStates: map[string]listContainerState{
			testContainerName("beta"): {exists: true, running: false},
		},
	}
	deps := newListDeps(t, runner)

	// When marshal list is invoked
	out, _, err := runCmd(t, deps, "list")

	// Then the process exits 0
	assertNoError(t, err)

	// And stdout shows a row for beta with STATUS=stopped
	assertRowContains(t, out.String(), "beta", "stopped")
}

// TestAcceptance_ListWithAbsentContainerProject_ShowsAbsentStatus verifies
// that a registered project whose container does not exist shows status
// "absent" in the list output, and the process exits 0.
func TestAcceptance_ListWithAbsentContainerProject_ShowsAbsentStatus(t *testing.T) {
	// Given a registered project gamma that has no container
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	if err := config.Save("gamma", &config.Config{}); err != nil {
		t.Fatalf("saving gamma config: %v", err)
	}

	// containerStates is empty — gamma has no container
	deps := newListDeps(t, absentListRunner())

	// When marshal list is invoked
	out, _, err := runCmd(t, deps, "list")

	// Then the process exits 0
	assertNoError(t, err)

	// And gamma's row shows status absent
	assertRowContains(t, out.String(), "gamma", "absent")
}

// TestAcceptance_ListProjectsAreSortedCaseSensitiveLexicographically verifies
// that project rows are emitted in case-sensitive lexicographic ascending order
// (uppercase letters sort before lowercase), regardless of the order in which
// project config files appear on disk.
func TestAcceptance_ListProjectsAreSortedCaseSensitiveLexicographically(t *testing.T) {
	// Given registered projects zebra, Alpha, and apple (each with no container)
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	for _, name := range []string{"zebra", "Alpha", "apple"} {
		if err := config.Save(name, &config.Config{}); err != nil {
			t.Fatalf("saving %s config: %v", name, err)
		}
	}

	runner := absentListRunner()
	deps := newListDeps(t, runner)

	// When marshal list is invoked
	out, _, err := runCmd(t, deps, "list")

	// Then the process exits 0
	assertNoError(t, err)

	// And rows appear in case-sensitive lexicographic order: Alpha, apple, zebra
	got := out.String()
	posAlpha := strings.Index(got, "Alpha")
	posApple := strings.Index(got, "apple")
	posZebra := strings.Index(got, "zebra")
	if posAlpha < 0 || posApple < 0 || posZebra < 0 {
		t.Fatalf("expected all three project rows in output, got:\n%s", got)
	}
	if posAlpha >= posApple || posApple >= posZebra {
		t.Errorf("rows not in expected order (Alpha < apple < zebra); positions: Alpha=%d apple=%d zebra=%d\noutput:\n%s",
			posAlpha, posApple, posZebra, got)
	}
}

// TestAcceptance_ListStatusColumnStartsAfterLongestName verifies that when
// the longest registered project name is 17 characters, the STATUS column
// begins at character position 19 or later (0-indexed) in every line —
// the header and all data rows — with names left-aligned (no leading spaces).
func TestAcceptance_ListStatusColumnStartsAfterLongestName(t *testing.T) {
	// Given registered projects long-project-name (17 chars) and short (5 chars), each with no container
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	for _, name := range []string{"long-project-name", "short"} {
		if err := config.Save(name, &config.Config{}); err != nil {
			t.Fatalf("saving %s config: %v", name, err)
		}
	}

	runner := absentListRunner()
	deps := newListDeps(t, runner)

	// When marshal list is invoked
	out, _, err := runCmd(t, deps, "list")

	// Then the process exits 0
	assertNoError(t, err)

	// And in every line, STATUS / status value starts at position ≥ 19 (0-indexed)
	// and each name is left-aligned (no leading whitespace on data rows)
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) == 0 {
		t.Fatal("no output lines")
	}
	// Check header: "STATUS" starts at position ≥ len("long-project-name") + 2
	minStatusPos := len("long-project-name") + 2
	headerStatusPos := strings.Index(lines[0], "STATUS")
	if headerStatusPos < minStatusPos {
		t.Errorf("header STATUS starts at position %d, want ≥ %d; header: %q", headerStatusPos, minStatusPos, lines[0])
	}
	// Check data rows: no leading space, and status value starts at position ≥ len("long-project-name") + 2
	for _, line := range lines[1:] {
		if len(line) == 0 {
			continue
		}
		if line[0] == ' ' {
			t.Errorf("data row has leading space (not left-aligned): %q", line)
		}
		// The status value ("absent" in this test) appears after the name + padding
		statusPos := strings.Index(line, "absent")
		if statusPos < minStatusPos {
			t.Errorf("status value in row starts at position %d, want ≥ %d; row: %q", statusPos, minStatusPos, line)
		}
	}
}

// TestAcceptance_ListDoesNotModifyProjectFiles verifies that `marshal list`
// is read-only: no files in $XDG_CONFIG_HOME/marshal/ are created, modified,
// or deleted, and the modification timestamp of each project config file is
// unchanged after the command completes.
func TestAcceptance_ListDoesNotModifyProjectFiles(t *testing.T) {
	// Given a registered project read-only-check exists in the projects dir
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	if err := config.Save("read-only-check", &config.Config{}); err != nil {
		t.Fatalf("saving read-only-check config: %v", err)
	}

	// Collect the state of all files under configHome before the command.
	type fileState struct {
		modTime     int64
		contentHash string
	}
	snapshotBefore := map[string]fileState{}
	if err := filepath.Walk(configHome, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		state := fileState{modTime: info.ModTime().UnixNano()}
		if info.Mode().IsRegular() {
			state.contentHash = fileHash(t, path)
		}
		snapshotBefore[path] = state
		return nil
	}); err != nil {
		t.Fatalf("walking configHome before: %v", err)
	}

	runner := absentListRunner()
	deps := newListDeps(t, runner)

	// When marshal list is invoked
	_, _, err := runCmd(t, deps, "list")

	// Then the process exits 0
	assertNoError(t, err)

	// And no files in configHome were created, modified, or deleted
	snapshotAfter := map[string]fileState{}
	if err := filepath.Walk(configHome, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		state := fileState{modTime: info.ModTime().UnixNano()}
		if info.Mode().IsRegular() {
			state.contentHash = fileHash(t, path)
		}
		snapshotAfter[path] = state
		return nil
	}); err != nil {
		t.Fatalf("walking configHome after: %v", err)
	}

	// Check no files were added or removed
	for path := range snapshotAfter {
		if _, existed := snapshotBefore[path]; !existed {
			t.Errorf("file created by marshal list: %s", path)
		}
	}
	for path := range snapshotBefore {
		if _, exists := snapshotAfter[path]; !exists {
			t.Errorf("file deleted by marshal list: %s", path)
		}
	}

	// Check modification timestamps and file contents are unchanged
	for path, before := range snapshotBefore {
		if after, ok := snapshotAfter[path]; ok {
			if after.modTime != before.modTime {
				t.Errorf("file modification time changed by marshal list: %s", path)
			}
			if after.contentHash != before.contentHash {
				t.Errorf("file contents changed by marshal list: %s", path)
			}
		}
	}
}

// TestAcceptance_ListWithUnreadableProjectsDir_ExitsOneWithError verifies that
// when $XDG_CONFIG_HOME/marshal/projects/ exists but is not readable,
// `marshal list` exits 1 and stderr contains an error message.
func TestAcceptance_ListWithUnreadableProjectsDir_ExitsOneWithError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("chmod 0o000 has no effect when running as root")
	}

	// Given $XDG_CONFIG_HOME/marshal/projects/ exists but is not readable
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	dir := filepath.Join(configHome, "marshal", "projects")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("creating projects dir: %v", err)
	}
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatalf("chmod projects dir: %v", err)
	}
	defer os.Chmod(dir, 0o755) //nolint:errcheck // restore so t.TempDir cleanup works

	runner := absentListRunner()
	deps := newListDeps(t, runner)

	// When marshal list is invoked
	_, errOut, err := runCmd(t, deps, "list")

	// Then the process exits 1
	assertError(t, err)

	// And stderr contains an error message
	if errOut.Len() == 0 {
		t.Error("expected stderr to contain an error message, got empty string")
	}
}

// TestAcceptance_ListWithRegisteredProjectAndPodmanUnavailable_ShowsUnknownRowAndStderrWarning
// verifies that when one or more projects are registered and Podman is entirely
// unavailable (all runner calls return an error), `marshal list` exits 0,
// shows each project with status "unknown", and writes a per-project warning
// to stderr naming the failing project.
func TestAcceptance_ListWithRegisteredProjectAndPodmanUnavailable_ShowsUnknownRowAndStderrWarning(t *testing.T) {
	// Given one registered project and Podman is entirely unavailable
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := config.Save("myproject", &config.Config{}); err != nil {
		t.Fatalf("setup: saving myproject config: %v", err)
	}
	runner := &listRunner{
		containerStates: map[string]listContainerState{
			testContainerName("myproject"): {queryErr: errors.New("podman: command not found")},
		},
	}
	deps := newListDeps(t, runner)

	// When marshal list is invoked
	out, errOut, err := runCmd(t, deps, "list")

	// Then the process exits 0
	assertNoError(t, err)

	// And stdout shows myproject with status unknown
	assertContains(t, out.String(), "myproject")
	assertContains(t, out.String(), "unknown")

	// And stderr contains a warning naming the failing project
	assertContains(t, errOut.String(), "myproject")
}

func TestAcceptance_ListWhenIsRunningQueryFails_ShowsUnknownRowAndStderrWarning(t *testing.T) {
	// Given a registered project whose container exists but whose running-state
	// query returns an error
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := config.Save("myproject", &config.Config{}); err != nil {
		t.Fatalf("setup: saving myproject config: %v", err)
	}
	runner := &listRunner{
		containerStates: map[string]listContainerState{
			testContainerName("myproject"): {
				exists:       true,
				running:      false,
				isRunningErr: errors.New("isrunning failed"),
			},
		},
	}

	// When marshal list is invoked
	out, errOut, err := runCmd(t, newListDeps(t, runner), "list")

	// Then the process exits 0, STATUS shows "unknown", and a warning appears on stderr
	assertNoError(t, err)
	assertRowContains(t, out.String(), "myproject", "unknown")
	assertContains(t, errOut.String(), "warning")
}

// TestAcceptance_ListWithConfigError_ShowsConfigErrorAndRealStatus verifies
// that when a project's config file is unloadable — either because it contains
// valid TOML with only unrecognised fields, or because it contains invalid TOML
// syntax — the project shows CONFIG=error and its real Podman STATUS (not
// "unknown"), a warning naming the project and the config issue appears in
// stderr, and the process exits 0.
func TestAcceptance_ListWithConfigError_ShowsConfigErrorAndRealStatus(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
	}{
		{
			name:    "valid TOML but unrecognised fields",
			content: []byte("[other]\nkey = \"value\"\n"),
		},
		{
			name:    "invalid TOML syntax",
			content: []byte("not valid toml ]["),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Given $XDG_CONFIG_HOME/marshal/projects/my-app.toml has the given content
			out, errOut, err := runListWithProjectFile(t, "my-app.toml", tc.content, absentListRunner())

			// When marshal list is invoked (performed inside runListWithProjectFile)

			// Then the process exits 0
			assertNoError(t, err)

			// And stdout shows my-app with CONFIG=error and real Podman STATUS (absent, since no container)
			assertContains(t, out.String(), "my-app")
			assertContains(t, out.String(), "error")
			assertNotContains(t, out.String(), "unknown")

			// And stderr contains a warning mentioning my-app and the config problem
			assertContains(t, errOut.String(), "my-app")
			assertContains(t, errOut.String(), "config")
		})
	}
}

// TestAcceptance_ListWithSpaceInFilename_SkipsRowAndWritesStderrWarning verifies
// that a projects-dir file whose stem contains a space is excluded from stdout
// and that a warning mentioning the filename is written to stderr. The process
// must exit 0 so that a single bad filename does not hide the healthy projects.
func TestAcceptance_ListWithSpaceInFilename_SkipsRowAndWritesStderrWarning(t *testing.T) {
	// Given a file named "my app.toml" (space in stem) exists in the projects directory
	out, errOut, err := runListWithProjectFile(t, "my app.toml", []byte(""), absentListRunner())

	// When marshal list is invoked (performed inside runListWithProjectFile)

	// Then the process exits 0
	assertNoError(t, err)

	// And no row for "my app" appears in stdout
	assertNotContains(t, out.String(), "my app")

	// And stderr contains a warning mentioning the invalid filename
	assertContains(t, errOut.String(), "my app.toml")
}

// TestAcceptance_ListWithPartialPodmanQueryFailure_ShowsUnknownRowAndStderrWarning
// verifies that when two projects are registered and the Podman query for one
// of them fails, marshal list shows the failing project with status "unknown",
// the passing project with its real status, writes a warning naming the failing
// project to stderr, and exits 0.
func TestAcceptance_ListWithPartialPodmanQueryFailure_ShowsUnknownRowAndStderrWarning(t *testing.T) {
	// Given registered projects alpha and beta, where the Podman query for alpha
	// fails but beta is running
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := config.Save("alpha", &config.Config{}); err != nil {
		t.Fatalf("saving alpha config: %v", err)
	}
	if err := config.Save("beta", &config.Config{}); err != nil {
		t.Fatalf("saving beta config: %v", err)
	}
	runner := &listRunner{
		containerStates: map[string]listContainerState{
			testContainerName("alpha"): {queryErr: errors.New("podman: connection refused")},
			testContainerName("beta"):  {exists: true, running: true},
		},
	}
	deps := newListDeps(t, runner)

	// When marshal list is invoked
	out, errOut, err := runCmd(t, deps, "list")

	// Then the process exits 0
	assertNoError(t, err)

	// And stdout shows alpha with status unknown
	assertRowContains(t, out.String(), "alpha", "unknown")

	// And stdout shows beta with status running
	assertRowContains(t, out.String(), "beta", "running")

	// And stderr contains a warning that names alpha
	assertContains(t, errOut.String(), "alpha")
}

// TestAcceptance_ListWithValidConfigAndRunningContainer_ShowsRunningAndConfigOk
// verifies that when a registered project has a valid config and a running
// container, the row shows STATUS=running and CONFIG=ok.
func TestAcceptance_ListWithValidConfigAndRunningContainer_ShowsRunningAndConfigOk(t *testing.T) {
	// Given a registered project myproject with a valid config and a running container
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := config.Save("myproject", &config.Config{}); err != nil {
		t.Fatalf("saving myproject config: %v", err)
	}
	runner := &listRunner{
		containerStates: map[string]listContainerState{
			testContainerName("myproject"): {exists: true, running: true},
		},
	}
	deps := newListDeps(t, runner)

	// When marshal list is invoked
	out, _, err := runCmd(t, deps, "list")

	// Then the process exits 0
	assertNoError(t, err)

	// And the row for myproject shows STATUS=running and CONFIG=ok
	got := out.String()
	assertRowContains(t, got, "myproject", "running")
	assertRowContains(t, got, "myproject", "ok")
}

// TestAcceptance_ListWithConfigErrorAndRunningContainer_ShowsRunningAndConfigError
// verifies that when a project's config file is unloadable — either because it
// contains valid TOML with only unrecognised fields, or because it contains
// invalid TOML syntax — and the project's container is running, marshal list
// shows STATUS=running (the real Podman status, not "unknown") and
// CONFIG=error, writes a warning to stderr naming the project, and exits 0.
func TestAcceptance_ListWithConfigErrorAndRunningContainer_ShowsRunningAndConfigError(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
	}{
		{
			name:    "valid TOML but unrecognised fields",
			content: []byte("[other]\nkey = \"value\"\n"),
		},
		{
			name:    "invalid TOML syntax",
			content: []byte("not valid toml ]["),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Given a registered project my-app whose config file has the given content and whose container is running
			runner := &listRunner{
				containerStates: map[string]listContainerState{
					testContainerName("my-app"): {exists: true, running: true},
				},
			}
			out, errOut, err := runListWithProjectFile(t, "my-app.toml", tc.content, runner)

			// When marshal list is invoked (performed inside runListWithProjectFile)

			// Then the process exits 0
			assertNoError(t, err)

			// And the row for my-app shows STATUS=running (real Podman status) and CONFIG=error (not "unknown")
			got := out.String()
			assertContains(t, got, "my-app")
			assertRowContains(t, got, "my-app", "running")
			assertRowContains(t, got, "my-app", "error")
			assertNotContains(t, got, "unknown")

			// And stderr contains a warning naming my-app and the config problem
			assertContains(t, errOut.String(), "my-app")
			assertContains(t, errOut.String(), "config")
		})
	}
}

// TestAcceptance_ListWhenConfigFileDeletedBetweenScanAndLoad_ShowsErrorConfigStatus
// verifies that when a config file disappears after project discovery but before
// config load, marshal list treats it as CONFIG=error rather than as an absent
// project.
func TestAcceptance_ListWhenConfigFileDeletedBetweenScanAndLoad_ShowsErrorConfigStatus(t *testing.T) {
	// Given a project discovered by ListProjects whose config file has since vanished
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := config.Save("volatile", &config.Config{}); err != nil {
		t.Fatalf("saving volatile config: %v", err)
	}

	deps := newListDeps(t, absentListRunner())
	deps.ListProjects = func() ([]string, []string, error) {
		return []string{"volatile"}, nil, nil
	}
	deps.LoadConfig = func(project string) (*config.Config, error) {
		if project != "volatile" {
			t.Fatalf("LoadConfig called with project %q, want %q", project, "volatile")
		}
		return nil, os.ErrNotExist
	}

	// When marshal list is invoked
	out, errOut, err := runCmd(t, deps, "list")

	// Then the process exits 0, CONFIG shows "error", and stderr names the project
	// and mentions the file disappearance
	assertNoError(t, err)
	assertRowContains(t, out.String(), "volatile", "error")
	assertContains(t, errOut.String(), "volatile")
	assertContains(t, errOut.String(), "config file disappeared")
}

// TestAcceptance_ListWithEmptyProjectsDir_PrintsHeaderAndExitsZero verifies
// that when the projects directory exists on disk but contains no files at all,
// `marshal list` prints only the header line and exits 0. This exercises the
// empty-directory branch of ListProjects (distinct from the absent-directory
// branch tested by TestAcceptance_ListWithNoProjects_PrintsHeaderAndExitsZero).
func TestAcceptance_ListWithEmptyProjectsDir_PrintsHeaderAndExitsZero(t *testing.T) {
	// Given $XDG_CONFIG_HOME/marshal/projects/ exists but contains no files
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	projDir := filepath.Join(configHome, "marshal", "projects")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatalf("creating projects dir: %v", err)
	}

	deps := newListDeps(t, absentListRunner())

	// When marshal list is invoked
	out, _, err := runCmd(t, deps, "list")

	// Then the process exits 0
	assertNoError(t, err)

	// And stdout contains exactly the header line and nothing else
	if got := out.String(); got != listHeader {
		t.Errorf("stdout = %q, want %q", got, listHeader)
	}
}

// TestAcceptance_ListWithOnlyNonTomlFilesInProjectsDir_PrintsHeaderAndExitsZero
// verifies that when the projects directory contains only non-.toml files
// (e.g. a .gitkeep placeholder), `marshal list` silently skips those files,
// prints only the header line, and exits 0.
func TestAcceptance_ListWithOnlyNonTomlFilesInProjectsDir_PrintsHeaderAndExitsZero(t *testing.T) {
	// Given $XDG_CONFIG_HOME/marshal/projects/ contains only a .gitkeep file
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	projDir := filepath.Join(configHome, "marshal", "projects")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatalf("creating projects dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projDir, ".gitkeep"), []byte{}, 0o644); err != nil {
		t.Fatalf("writing .gitkeep: %v", err)
	}

	deps := newListDeps(t, absentListRunner())

	// When marshal list is invoked
	out, _, err := runCmd(t, deps, "list")

	// Then the process exits 0
	assertNoError(t, err)

	// And stdout contains exactly the header line (no row for .gitkeep)
	got := out.String()
	if got != listHeader {
		t.Errorf("stdout = %q, want %q", got, listHeader)
	}
	assertNotContains(t, got, ".gitkeep")
}

// TestAcceptance_List_DisplaysPorts verifies that the list command prints the port
// number for each project.
func TestAcceptance_List_DisplaysPorts(t *testing.T) {
	// Given two projects, one with default port (4096) and one with a custom port (5000)
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)

	projDir := filepath.Join(configHome, "marshal", "projects")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatalf("creating projects dir: %v", err)
	}

	// Project alpha has default port 4096 (port field omitted/0)
	alphaCfg := &config.Config{Mounts: []string{"/projects/alpha"}}
	if err := config.Save("alpha", alphaCfg); err != nil {
		t.Fatalf("saving config for alpha: %v", err)
	}

	// Project beta has custom port 5000
	betaCfg := &config.Config{Mounts: []string{"/projects/beta"}, Port: 5000}
	if err := config.Save("beta", betaCfg); err != nil {
		t.Fatalf("saving config for beta: %v", err)
	}

	runner := &listRunner{
		containerStates: map[string]listContainerState{
			testContainerName("alpha"): {exists: true, running: true},
			testContainerName("beta"):  {exists: true, running: false},
		},
	}
	deps := newListDeps(t, runner)

	// When marshal list is executed
	out, _, err := runCmd(t, deps, "list")
	assertNoError(t, err)

	// Then alpha is listed with port 4096 and beta is listed with port 5000
	got := out.String()
	assertRowContains(t, got, "alpha", "4096")
	assertRowContains(t, got, "beta", "5000")
}
