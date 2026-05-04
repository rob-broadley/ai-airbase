// SPDX-License-Identifier: AGPL-3.0-or-later
package container_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/container"
)

// ---------------------------------------------------------------------------
// Fake Runner — records every call, returns pre-queued responses in order.
// Preferred over a mock: no expectations to set up per-call, less brittle.
// ---------------------------------------------------------------------------

type fakeCall struct {
	name string
	args []string
}

type fakeResponse struct {
	err    error
	output []byte
}

type fakeRunner struct {
	imageExistsErr    error
	pullImageErr      error
	pullImageImage    string
	pullOutput        string
	calls             []fakeCall
	responses         []fakeResponse
	imageExistsResult bool
	pullImageCalled   bool
}

func (f *fakeRunner) Run(name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, fakeCall{name: name, args: args})
	idx := len(f.calls) - 1
	if idx < len(f.responses) {
		return f.responses[idx].output, f.responses[idx].err
	}
	return nil, nil
}

func (f *fakeRunner) ImageExists(image string) (bool, error) {
	return f.imageExistsResult, f.imageExistsErr
}

func (f *fakeRunner) PullImage(image string, stdout, stderr io.Writer) error {
	f.pullImageCalled = true
	f.pullImageImage = image
	if f.pullOutput != "" {
		fmt.Fprint(stdout, f.pullOutput)
	}
	return f.pullImageErr
}

// lastCreateArgs returns the args slice from the most recent "podman create" call
// (nil if no create call was recorded). Used to verify user identity args.
func (f *fakeRunner) lastCreateArgs() []string {
	for i := len(f.calls) - 1; i >= 0; i-- {
		if f.calls[i].name == "podman" && len(f.calls[i].args) > 0 && f.calls[i].args[0] == "create" {
			return f.calls[i].args
		}
	}
	return nil
}

// newFake constructs a fakeRunner with pre-queued responses returned in order.
func newFake(responses ...fakeResponse) *fakeRunner {
	return &fakeRunner{responses: responses}
}

// okOut returns a successful fakeResponse whose output is the UTF-8 encoding of s.
func okOut(s string) fakeResponse { return fakeResponse{output: []byte(s)} }

// okEmpty returns a successful fakeResponse with no output.
func okEmpty() fakeResponse { return fakeResponse{} }

// errOut returns a fakeResponse that carries the given error.
func errOut(e error) fakeResponse { return fakeResponse{err: e} }

// hasArg checks whether s appears anywhere in args
func hasArg(args []string, s string) bool {
	for _, a := range args {
		if a == s {
			return true
		}
	}
	return false
}

// hasConsecutiveArgs checks whether first and second appear as adjacent
// elements (in that order) anywhere in args.
func hasConsecutiveArgs(args []string, first, second string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == first && args[i+1] == second {
			return true
		}
	}
	return false
}

// argsCmd returns the sub-command (args[0]) or empty string when args is empty
func argsCmd(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

// ---------------------------------------------------------------------------
// ResolveMounts
// ---------------------------------------------------------------------------

// TestResolveMounts_EmptyMounts_ReturnsCwdToWorkspace verifies that
// ResolveMounts returns a single /workspace/<basename> mount for the cwd when
// the mounts list is empty.
func TestResolveMounts_EmptyMounts_ReturnsCwdToWorkspace(t *testing.T) {
	// Given a cwd path and an empty mounts list
	// When ResolveMounts is called
	got := container.ResolveMounts("/home/user/project", nil)

	// Then a single mount mapping cwd to /workspace/project is returned
	want := []container.MountSpec{
		{HostPath: "/home/user/project", ContainerPath: "/workspace/project"},
	}

	if len(got) != 1 {
		t.Fatalf("len = %d, want 1; got %v", len(got), got)
	}
	if got[0] != want[0] {
		t.Errorf("got %v, want %v", got[0], want[0])
	}
}

// TestResolveMounts_DefaultMountIsInsideWorkspace verifies that ResolveMounts
// always mounts the default CWD inside /workspace/<basename>, never as the
// root /workspace.
func TestResolveMounts_DefaultMountIsInsideWorkspace(t *testing.T) {
	// Given a cwd "/home/rob/myapp" and no explicit mounts
	// When ResolveMounts is called
	got := container.ResolveMounts("/home/rob/myapp", nil)

	// Then ContainerPath is /workspace/myapp, not /workspace
	want := container.MountSpec{HostPath: "/home/rob/myapp", ContainerPath: "/workspace/myapp"}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1; got %v", len(got), got)
	}
	if got[0] != want {
		t.Errorf("got %v, want %v", got[0], want)
	}
}

// TestResolveMounts_WithPaths_MapsToWorkspaceBasename verifies that
// ResolveMounts maps each explicit host path to /workspace/<basename>.
func TestResolveMounts_WithPaths_MapsToWorkspaceBasename(t *testing.T) {
	// Given a cwd path and a list of explicit host paths
	// When ResolveMounts is called with those paths
	got := container.ResolveMounts("/home/user/project", []string{
		"/home/user/src",
		"/data/models",
	})

	// Then each path is mapped under /workspace using its basename
	want := []container.MountSpec{
		{HostPath: "/home/user/src", ContainerPath: "/workspace/src"},
		{HostPath: "/data/models", ContainerPath: "/workspace/models"},
	}

	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d; got %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] got %v, want %v", i, got[i], want[i])
		}
	}
}

// TestResolveMounts_WithPaths_DoesNotMountCwd verifies that ResolveMounts does
// not include the cwd in the returned list when explicit paths are provided.
func TestResolveMounts_WithPaths_DoesNotMountCwd(t *testing.T) {
	// Given a cwd path and a list of explicit host paths
	cwd := "/home/user/project"

	// When ResolveMounts is called with those paths
	got := container.ResolveMounts(cwd, []string{"/home/user/src"})

	// Then the cwd is not included in the returned mounts
	for _, m := range got {
		if m.HostPath == cwd {
			t.Errorf("cwd %q should not appear in mounts when paths provided; got %v", cwd, got)
		}
	}
}

// ---------------------------------------------------------------------------
// WorkdirFromMounts
// ---------------------------------------------------------------------------

// TestWorkdirFromMounts_SingleMount_ReturnsMountPath verifies that
// WorkdirFromMounts returns the mount's container path when exactly one
// workspace mount is provided, so the user lands directly in their project.
func TestWorkdirFromMounts_SingleMount_ReturnsMountPath(t *testing.T) {
	// Given a single mount spec at /workspace/myapp
	mounts := []container.MountSpec{
		{HostPath: "/home/rob/myapp", ContainerPath: "/workspace/myapp"},
	}

	// When WorkdirFromMounts is called
	got := container.WorkdirFromMounts(mounts)

	// Then the container path of that mount is returned
	const want = "/workspace/myapp"
	if got != want {
		t.Errorf("WorkdirFromMounts = %q, want %q", got, want)
	}
}

// TestWorkdirFromMounts_NoMounts_ReturnsWorkspace verifies that
// WorkdirFromMounts returns /workspace when no mounts are provided.
func TestWorkdirFromMounts_NoMounts_ReturnsWorkspace(t *testing.T) {
	// Given an empty mounts list
	// When WorkdirFromMounts is called
	got := container.WorkdirFromMounts(nil)

	// Then /workspace is returned
	const want = "/workspace"
	if got != want {
		t.Errorf("WorkdirFromMounts = %q, want %q", got, want)
	}
}

// TestWorkdirFromMounts_MultipleMounts_ReturnsWorkspace verifies that
// WorkdirFromMounts returns /workspace when more than one mount is provided,
// since there is no single project to land in.
func TestWorkdirFromMounts_MultipleMounts_ReturnsWorkspace(t *testing.T) {
	// Given two mount specs
	mounts := []container.MountSpec{
		{HostPath: "/a", ContainerPath: "/workspace/a"},
		{HostPath: "/b", ContainerPath: "/workspace/b"},
	}

	// When WorkdirFromMounts is called
	got := container.WorkdirFromMounts(mounts)

	// Then /workspace is returned
	const want = "/workspace"
	if got != want {
		t.Errorf("WorkdirFromMounts = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// Exists
// ---------------------------------------------------------------------------

// TestExists_ContainerFound_ReturnsTrue verifies that Exists returns true when
// the runner output includes the container name.
func TestExists_ContainerFound_ReturnsTrue(t *testing.T) {
	// Given a runner that returns the container name in its output
	r := newFake(okOut("mycontainer\n"))

	// When Exists is called
	got, err := container.Exists(r, "mycontainer")

	// Then true is returned without error
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Error("expected Exists = true")
	}
}

// TestExists_ContainerNotFound_ReturnsFalse verifies that Exists returns false
// when the runner returns empty output (no matching container).
func TestExists_ContainerNotFound_ReturnsFalse(t *testing.T) {
	// Given a runner that returns empty output (no matching container)
	r := newFake(okOut(""))

	// When Exists is called
	got, err := container.Exists(r, "mycontainer")

	// Then false is returned without error
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Error("expected Exists = false")
	}
}

// TestExists_RunnerError_PropagatesError verifies that Exists propagates an
// error returned by the underlying runner.
func TestExists_RunnerError_PropagatesError(t *testing.T) {
	// Given a runner that returns an error
	r := newFake(errOut(errors.New("exec: podman not found")))

	// When Exists is called
	_, err := container.Exists(r, "mycontainer")

	// Then the error is propagated
	if err == nil {
		t.Error("expected error, got nil")
	}
}

// TestExists_UsesCorrectPodmanArgs verifies that Exists calls podman ps --all
// with an anchored name filter.
func TestExists_UsesCorrectPodmanArgs(t *testing.T) {
	// Given a runner that returns the container name
	r := newFake(okOut("mycontainer\n"))

	// When Exists is called
	_, _ = container.Exists(r, "mycontainer")

	// Then podman ps --all with anchored name filter was called
	if len(r.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(r.calls))
	}
	call := r.calls[0]
	if call.name != "podman" {
		t.Errorf("command = %q, want %q", call.name, "podman")
	}
	if argsCmd(call.args) != "ps" {
		t.Errorf("sub-command = %q, want %q", argsCmd(call.args), "ps")
	}
	if !hasArg(call.args, "--all") {
		t.Error("expected --all flag for Exists")
	}
	if !hasArg(call.args, "name=^mycontainer$") {
		t.Error("expected anchored filter name=^mycontainer$ in args")
	}
}

// TestExists_ContainerNameWithDot_EscapesMetacharInFilter verifies that Exists
// escapes regex metacharacters (e.g. a dot) in the podman --filter argument so
// that a project name like "marshal-my.project" cannot match unintended
// containers (e.g. "marshal-myXproject").
func TestExists_ContainerNameWithDot_EscapesMetacharInFilter(t *testing.T) {
	// Given a container name that contains a dot (a regex metacharacter)
	containerName := "marshal-my.project"
	r := newFake(okOut(containerName + "\n"))

	// When Exists is called with that name
	_, _ = container.Exists(r, containerName)

	// Then the --filter argument uses the escaped name (dot becomes \.)
	if len(r.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(r.calls))
	}
	wantFilter := `name=^marshal-my\.project$`
	if !hasArg(r.calls[0].args, wantFilter) {
		t.Errorf("expected filter %q in args; got %v", wantFilter, r.calls[0].args)
	}
}

// ---------------------------------------------------------------------------
// IsRunning
// ---------------------------------------------------------------------------

// TestIsRunning_ContainerRunning_ReturnsTrue verifies that IsRunning returns
// true when the runner output contains the container name.
func TestIsRunning_ContainerRunning_ReturnsTrue(t *testing.T) {
	// Given a runner that returns the container name (indicating it is running)
	r := newFake(okOut("mycontainer\n"))

	// When IsRunning is called
	got, err := container.IsRunning(r, "mycontainer")

	// Then true is returned without error
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Error("expected IsRunning = true")
	}
}

// TestIsRunning_ContainerStopped_ReturnsFalse verifies that IsRunning returns
// false when the runner returns empty output (container not in running state).
func TestIsRunning_ContainerStopped_ReturnsFalse(t *testing.T) {
	// Given a runner that returns empty output (container not in running state)
	r := newFake(okOut(""))

	// When IsRunning is called
	got, err := container.IsRunning(r, "mycontainer")

	// Then false is returned without error
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Error("expected IsRunning = false")
	}
}

// TestIsRunning_RunnerError_PropagatesError verifies that IsRunning propagates
// an error returned by the underlying runner.
func TestIsRunning_RunnerError_PropagatesError(t *testing.T) {
	// Given a runner that returns an error
	r := newFake(errOut(errors.New("exec: podman not found")))

	// When IsRunning is called
	_, err := container.IsRunning(r, "mycontainer")

	// Then the error is propagated
	if err == nil {
		t.Error("expected error, got nil")
	}
}

// TestIsRunning_DoesNotUseAllFlag verifies that IsRunning omits the --all flag
// so only currently running containers are considered.
func TestIsRunning_DoesNotUseAllFlag(t *testing.T) {
	// Given a runner that returns empty output
	r := newFake(okOut(""))

	// When IsRunning is called
	_, _ = container.IsRunning(r, "mycontainer")

	// Then the --all flag is NOT used (only running containers are checked)
	if len(r.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(r.calls))
	}
	if hasArg(r.calls[0].args, "--all") {
		t.Error("IsRunning must NOT use --all flag (only shows running containers)")
	}
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

// TestCreate_InvokesCorrectPodmanArgs verifies that Create calls podman with
// the expected create arguments, including name, userns, tty, interactive,
// passwd-entry, volume mounts, workdir, and image.
func TestCreate_InvokesCorrectPodmanArgs(t *testing.T) {
	// Given a runner that succeeds and a single mount spec
	r := newFake(okEmpty())
	mounts := []container.MountSpec{
		{HostPath: "/host/src", ContainerPath: "/workspace/src"},
	}

	// When Create is called with workdir matching the mount path
	err := container.Create(r, "mycontainer", "myimage:latest", mounts, container.UserConfig{}, "/workspace/src")

	// Then the correct podman create arguments are passed
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(r.calls))
	}
	args := r.calls[0].args

	for _, want := range []string{"create", "--name", "mycontainer",
		"--userns=keep-id", "--tty", "--interactive", "--passwd-entry",
		"-v", "/host/src:/workspace/src:Z", "-v", "mycontainer-nix:/nix/store",
		"-w", "/workspace/src", "myimage:latest"} {
		if !hasArg(args, want) {
			t.Errorf("args missing %q; full args: %v", want, args)
		}
	}
}

// TestCreate_PassesWorkdirToContainer verifies that Create passes the supplied
// workdir to the -w flag rather than using a hardcoded /workspace value.
func TestCreate_PassesWorkdirToContainer(t *testing.T) {
	// Given a runner that succeeds and a custom workdir path
	r := newFake(okEmpty())
	mounts := []container.MountSpec{
		{HostPath: "/home/rob/myapp", ContainerPath: "/workspace/myapp"},
	}
	const workdir = "/workspace/myapp"

	// When Create is called with that workdir
	err := container.Create(r, "mycontainer", "myimage:latest", mounts, container.UserConfig{}, workdir)

	// Then the -w flag is set to the supplied workdir, not /workspace
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	args := r.calls[0].args
	if !hasConsecutiveArgs(args, "-w", workdir) {
		t.Errorf("expected consecutive args \"-w\" %q; full args: %v", workdir, args)
	}
}

// TestCreate_MultipleMount_AllMountsPresent verifies that Create includes all
// provided mount specs in the podman create arguments.
func TestCreate_MultipleMount_AllMountsPresent(t *testing.T) {
	// Given a runner that succeeds and two mount specs
	r := newFake(okEmpty())
	mounts := []container.MountSpec{
		{HostPath: "/a", ContainerPath: "/workspace/a"},
		{HostPath: "/b", ContainerPath: "/workspace/b"},
	}

	// When Create is called
	err := container.Create(r, "c", "img", mounts, container.UserConfig{}, "/workspace")

	// Then both mounts appear in the podman create arguments
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	args := r.calls[0].args
	if !hasArg(args, "/a:/workspace/a:Z") {
		t.Error("missing first mount with :Z suffix in args")
	}
	if !hasArg(args, "/b:/workspace/b:Z") {
		t.Error("missing second mount with :Z suffix in args")
	}
}

// TestCreate_RunnerError_PropagatesError verifies that Create propagates an
// error returned by the runner when podman create fails.
func TestCreate_RunnerError_PropagatesError(t *testing.T) {
	// Given a runner that returns an error
	r := newFake(errOut(errors.New("image not found")))

	// When Create is called
	err := container.Create(r, "c", "bad-image", nil, container.UserConfig{}, "/workspace")

	// Then the error is propagated
	if err == nil {
		t.Error("expected error, got nil")
	}
}

// TestCreate_AllMountsHaveZSELinuxSuffix verifies that every bind mount passed
// to Create includes the :Z SELinux relabelling suffix.
func TestCreate_AllMountsHaveZSELinuxSuffix(t *testing.T) {
	// Given a runner that succeeds and multiple mount specs
	r := newFake(okEmpty())
	mounts := []container.MountSpec{
		{HostPath: "/host/creds", ContainerPath: "/run/creds"},
		{HostPath: "/host/work", ContainerPath: "/workspace"},
	}

	// When Create is called
	_ = container.Create(r, "c", "img", mounts, container.UserConfig{}, "/workspace")

	// Then every mount argument includes the :Z SELinux suffix
	args := r.calls[0].args
	for _, m := range mounts {
		want := m.HostPath + ":" + m.ContainerPath + ":Z"
		if !hasArg(args, want) {
			t.Errorf("expected mount arg %q with :Z suffix; full args: %v", want, args)
		}
		// The plain form without :Z must NOT appear.
		plain := m.HostPath + ":" + m.ContainerPath
		if hasArg(args, plain) {
			t.Errorf("mount arg %q must not appear without :Z suffix; full args: %v", plain, args)
		}
	}
}

// TestNixStoreVolumeName verifies that NixStoreVolumeName derives the
// per-project volume name by appending "-nix" to the container name.
func TestNixStoreVolumeName(t *testing.T) {
	// Given a container name "marshal-myapp"
	// When NixStoreVolumeName is called
	got := container.NixStoreVolumeName("marshal-myapp")

	// Then the volume name is "marshal-myapp-nix"
	want := "marshal-myapp-nix"
	if got != want {
		t.Errorf("NixStoreVolumeName(\"marshal-myapp\") = %q, want %q", got, want)
	}
}

// TestCreate_HasNixStoreVolume verifies that podman create always includes the
// per-project Nix-store volume so the Nix store persists across recreate calls.
func TestCreate_HasNixStoreVolume(t *testing.T) {
	// Given a runner that succeeds (no mounts)
	r := newFake(okEmpty())

	// When Create is called with container name "c"
	_ = container.Create(r, "c", "img", nil, container.UserConfig{}, "/workspace")

	// Then -v c-nix:/nix/store is present in the podman create arguments
	args := r.calls[0].args
	if !hasConsecutiveArgs(args, "-v", "c-nix:/nix/store") {
		t.Errorf("expected consecutive args \"-v\" \"c-nix:/nix/store\"; full args: %v", args)
	}
}

// TestCreate_NixStoreVolume_HasNoZSuffix verifies that the Nix-store volume
// mount does not carry a :Z SELinux suffix, which is only valid for bind mounts.
func TestCreate_NixStoreVolume_HasNoZSuffix(t *testing.T) {
	// Given a runner that succeeds
	r := newFake(okEmpty())

	// When Create is called with container name "c"
	_ = container.Create(r, "c", "img", nil, container.UserConfig{}, "/workspace")

	// Then c-nix:/nix/store:Z must NOT appear in the arguments
	args := r.calls[0].args
	if hasArg(args, "c-nix:/nix/store:Z") {
		t.Errorf("Nix-store volume must not have :Z suffix; full args: %v", args)
	}
}

// TestRemoveNixStore_InvokesVolumeRm verifies that RemoveNixStore calls
// "podman volume rm <containerName>-nix".
func TestRemoveNixStore_InvokesVolumeRm(t *testing.T) {
	// Given a runner that succeeds
	r := newFake(okEmpty())

	// When RemoveNixStore is called for "marshal-myapp"
	err := container.RemoveNixStore(r, "marshal-myapp")

	// Then the error is nil
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// And exactly one call was made
	if len(r.calls) != 1 {
		t.Fatalf("expected 1 call, got %d: %v", len(r.calls), r.calls)
	}
	// And it was "podman volume rm marshal-myapp-nix"
	args := r.calls[0].args
	if !hasConsecutiveArgs(args, "volume", "rm") {
		t.Errorf("expected consecutive args \"volume\" \"rm\"; full args: %v", args)
	}
	if !hasArg(args, "marshal-myapp-nix") {
		t.Errorf("expected volume name \"marshal-myapp-nix\" in args; full args: %v", args)
	}
}

// TestRemove_DoesNotRemoveNixVolume verifies that the standard Remove function
// does not call "podman volume rm" so that recreate preserves the Nix store.
func TestRemove_DoesNotRemoveNixVolume(t *testing.T) {
	// Given a stopped container
	r := newFake(
		okOut(""), // IsRunning → not running
		okEmpty(), // rm succeeds
	)

	// When Remove is called
	err := container.Remove(r, "mycontainer")

	// Then no error occurs
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// And "podman volume rm" was never invoked
	for _, call := range r.calls {
		if len(call.args) >= 2 && call.args[0] == "volume" && call.args[1] == "rm" {
			t.Errorf("Remove must not call 'podman volume rm'; calls: %v", r.calls)
		}
	}
}

// TestCreate_HasManagedByLabel verifies that podman create includes the
// io.ai-airbase.managed-by=marshal label so containers can be identified by
// tooling.
func TestCreate_HasManagedByLabel(t *testing.T) {
	// Given a runner that succeeds
	r := newFake(okEmpty())

	// When Create is called with a container name and image
	_ = container.Create(r, "marshal-myapp", "ghcr.io/org/img:latest", nil, container.UserConfig{}, "/workspace")

	// Then --label io.ai-airbase.managed-by=marshal is present in the podman create arguments
	args := r.calls[0].args
	if !hasArg(args, "--label") {
		t.Fatalf("expected at least one --label flag in args; got %v", args)
	}
	if !hasArg(args, "io.ai-airbase.managed-by=marshal") {
		t.Errorf("expected label value io.ai-airbase.managed-by=marshal in args; got %v", args)
	}
}

// TestCreate_HasProjectLabel verifies that podman create includes the
// io.ai-airbase.project label set to the container name.
func TestCreate_HasProjectLabel(t *testing.T) {
	// Given a runner that succeeds and a specific container name
	r := newFake(okEmpty())

	// When Create is called with containerName "marshal-myapp"
	_ = container.Create(r, "marshal-myapp", "ghcr.io/org/img:latest", nil, container.UserConfig{}, "/workspace")

	// Then --label io.ai-airbase.project=marshal-myapp is present in the podman create arguments
	args := r.calls[0].args
	if !hasArg(args, "io.ai-airbase.project=marshal-myapp") {
		t.Errorf("expected label value io.ai-airbase.project=marshal-myapp in args; got %v", args)
	}
}

// TestCreate_HasImageLabel verifies that podman create includes the
// io.ai-airbase.image label set to the image reference used to create the container.
func TestCreate_HasImageLabel(t *testing.T) {
	// Given a runner that succeeds and a specific image name
	r := newFake(okEmpty())

	// When Create is called with image "ghcr.io/org/img:latest"
	_ = container.Create(r, "marshal-myapp", "ghcr.io/org/img:latest", nil, container.UserConfig{}, "/workspace")

	// Then --label io.ai-airbase.image=ghcr.io/org/img:latest is present in the podman create arguments
	args := r.calls[0].args
	if !hasArg(args, "io.ai-airbase.image=ghcr.io/org/img:latest") {
		t.Errorf("expected label value io.ai-airbase.image=ghcr.io/org/img:latest in args; got %v", args)
	}
}

// TestCreate_LabelFlagsAreAdjacentPairs verifies that each label value is
// immediately preceded by its --label flag in the podman create argument list.
func TestCreate_LabelFlagsAreAdjacentPairs(t *testing.T) {
	// Given a runner that succeeds
	r := newFake(okEmpty())

	// When Create is called
	_ = container.Create(r, "marshal-proj", "myimage:1.0", nil, container.UserConfig{}, "/workspace")

	// Then each label value is immediately preceded by --label (pair format)
	args := r.calls[0].args
	labelValues := []string{
		"io.ai-airbase.managed-by=marshal",
		"io.ai-airbase.project=marshal-proj",
		"io.ai-airbase.image=myimage:1.0",
	}
	for _, val := range labelValues {
		found := false
		for i, a := range args {
			if a == "--label" && i+1 < len(args) && args[i+1] == val {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected --label %q pair in args; full args: %v", val, args)
		}
	}
}

// TestCreate_HasUsernsKeepId verifies that podman create includes
// --userns=keep-id so host UID/GID are mapped into the container.
func TestCreate_HasUsernsKeepId(t *testing.T) {
	// Given a runner that succeeds
	r := newFake(okEmpty())

	// When Create is called
	_ = container.Create(r, "c", "img", nil, container.UserConfig{}, "/workspace")

	// Then --userns=keep-id is present in the podman create arguments
	if !hasArg(r.calls[0].args, "--userns=keep-id") {
		t.Errorf("expected --userns=keep-id in args; got %v", r.calls[0].args)
	}
}

// TestCreate_UserConfig_SetsUserFlag verifies that Create passes --user
// <UID>:<GID> when a UserConfig with UID and GID is provided.
func TestCreate_UserConfig_SetsUserFlag(t *testing.T) {
	// Given a runner that succeeds and a UserConfig with UID 1001, GID 1001
	r := newFake(okEmpty())
	uc := container.UserConfig{UID: 1001, GID: 1001, HomeDir: "/home/alice"}

	// When Create is called
	_ = container.Create(r, "c", "img", nil, uc, "/workspace")

	// Then --user 1001:1001 is present in the podman create arguments
	args := r.calls[0].args
	if !hasArg(args, "--user") {
		t.Fatalf("expected --user flag in args; got %v", args)
	}
	// --user value must immediately follow the --user flag.
	for i, a := range args {
		if a == "--user" && i+1 < len(args) {
			if args[i+1] != "1001:1001" {
				t.Errorf("--user value = %q, want %q", args[i+1], "1001:1001")
			}
			return
		}
	}
	t.Errorf("--user flag found but value missing; args: %v", args)
}

// TestCreate_UserConfig_SetsHomeEnv verifies that Create passes -e HOME=<path>
// so tools inside the container resolve ~ to the correct home directory.
func TestCreate_UserConfig_SetsHomeEnv(t *testing.T) {
	// Given a runner that succeeds and a UserConfig with a HomeDir set
	r := newFake(okEmpty())
	uc := container.UserConfig{UID: 1001, GID: 1001, HomeDir: "/home/alice"}

	// When Create is called
	_ = container.Create(r, "c", "img", nil, uc, "/workspace")

	// Then -e HOME=/home/alice is present in the podman create arguments
	args := r.calls[0].args
	if !hasArg(args, "-e") {
		t.Fatalf("expected -e flag in args; got %v", args)
	}
	for i, a := range args {
		if a == "-e" && i+1 < len(args) {
			if args[i+1] == "HOME=/home/alice" {
				return // found it
			}
		}
	}
	t.Errorf("expected -e HOME=/home/alice in args; got %v", args)
}

// ---------------------------------------------------------------------------
// Start
// ---------------------------------------------------------------------------

// TestStart_InvokesCorrectPodmanArgs verifies that Start calls podman start
// with the container name.
func TestStart_InvokesCorrectPodmanArgs(t *testing.T) {
	// Given a runner that succeeds
	r := newFake(okEmpty())

	// When Start is called
	err := container.Start(r, "mycontainer")

	// Then podman start with the container name is called
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(r.calls))
	}
	args := r.calls[0].args
	if argsCmd(args) != "start" {
		t.Errorf("sub-command = %q, want %q", argsCmd(args), "start")
	}
	if !hasArg(args, "mycontainer") {
		t.Errorf("missing container name in args: %v", args)
	}
}

// TestStart_RunnerError_PropagatesError verifies that Start propagates an error
// returned by the underlying runner.
func TestStart_RunnerError_PropagatesError(t *testing.T) {
	// Given a runner that returns an error
	r := newFake(errOut(errors.New("no such container")))

	// When Start is called
	err := container.Start(r, "mycontainer")

	// Then the error is propagated
	if err == nil {
		t.Error("expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Stop
// ---------------------------------------------------------------------------

// TestStop_InvokesCorrectPodmanArgs verifies that Stop calls podman stop with
// the container name.
func TestStop_InvokesCorrectPodmanArgs(t *testing.T) {
	// Given a runner that succeeds
	r := newFake(okEmpty())

	// When Stop is called
	err := container.Stop(r, "mycontainer")

	// Then podman stop with the container name is called
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(r.calls))
	}
	args := r.calls[0].args
	if argsCmd(args) != "stop" {
		t.Errorf("sub-command = %q, want %q", argsCmd(args), "stop")
	}
	if !hasArg(args, "mycontainer") {
		t.Errorf("missing container name in args: %v", args)
	}
}

// TestStop_RunnerError_PropagatesError verifies that Stop propagates an error
// returned by the underlying runner.
func TestStop_RunnerError_PropagatesError(t *testing.T) {
	// Given a runner that returns an error
	r := newFake(errOut(errors.New("container not running")))

	// When Stop is called
	err := container.Stop(r, "mycontainer")

	// Then the error is propagated
	if err == nil {
		t.Error("expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Remove
// ---------------------------------------------------------------------------

// TestRemove_WhenRunning_StopsThenRemoves verifies that Remove calls stop
// before rm when the container is currently running.
func TestRemove_WhenRunning_StopsThenRemoves(t *testing.T) {
	// Given a running container
	r := newFake(
		okOut("mycontainer\n"), // IsRunning → running
		okEmpty(),              // stop
		okEmpty(),              // rm
	)

	// When Remove is called
	err := container.Remove(r, "mycontainer")

	// Then stop is called before rm
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.calls) != 3 {
		t.Fatalf("expected 3 calls (isRunning, stop, rm), got %d: %v", len(r.calls), r.calls)
	}
	if argsCmd(r.calls[1].args) != "stop" {
		t.Errorf("call[1] sub-command = %q, want %q", argsCmd(r.calls[1].args), "stop")
	}
	if argsCmd(r.calls[2].args) != "rm" {
		t.Errorf("call[2] sub-command = %q, want %q", argsCmd(r.calls[2].args), "rm")
	}
}

// TestRemove_WhenStopped_RemovesWithoutStop verifies that Remove calls rm
// directly without a preceding stop when the container is already stopped.
func TestRemove_WhenStopped_RemovesWithoutStop(t *testing.T) {
	// Given a stopped (not running) container
	r := newFake(
		okOut(""), // IsRunning → not running
		okEmpty(), // rm
	)

	// When Remove is called
	err := container.Remove(r, "mycontainer")

	// Then rm is called without a prior stop
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.calls) != 2 {
		t.Fatalf("expected 2 calls (isRunning, rm), got %d: %v", len(r.calls), r.calls)
	}
	if argsCmd(r.calls[1].args) != "rm" {
		t.Errorf("call[1] sub-command = %q, want %q", argsCmd(r.calls[1].args), "rm")
	}
}

// TestRemove_IsRunningError_PropagatesError verifies that Remove propagates an
// error returned by the IsRunning check.
func TestRemove_IsRunningError_PropagatesError(t *testing.T) {
	// Given a runner that returns an error on the IsRunning check
	r := newFake(errOut(errors.New("podman error")))

	// When Remove is called
	err := container.Remove(r, "mycontainer")

	// Then the error is propagated
	if err == nil {
		t.Error("expected error, got nil")
	}
}

// TestRemove_StopError_PropagatesError verifies that Remove propagates an error
// from the stop call when the container is running.
func TestRemove_StopError_PropagatesError(t *testing.T) {
	// Given a running container whose Stop call returns an error
	r := newFake(
		okOut("mycontainer\n"),            // IsRunning → running
		errOut(errors.New("stop failed")), // Stop → error
	)

	// When Remove is called
	err := container.Remove(r, "mycontainer")

	// Then the stop error is propagated
	if err == nil {
		t.Error("expected error when Stop fails, got nil")
	}
}

// TestRemove_RmError_PropagatesError verifies that Remove propagates an error
// from the rm call when the container is stopped.
func TestRemove_RmError_PropagatesError(t *testing.T) {
	// Given a stopped container whose rm call returns an error
	r := newFake(
		okOut(""),                              // IsRunning → not running
		errOut(errors.New("container locked")), // rm fails
	)

	// When Remove is called
	err := container.Remove(r, "mycontainer")

	// Then the rm error is propagated
	if err == nil {
		t.Error("expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// GetStatus
// ---------------------------------------------------------------------------

// TestGetStatus_ContainerDoesNotExist_ReturnsNotExistsStatus verifies that
// GetStatus returns a status with Exists and Running both false when no
// matching container is found.
func TestGetStatus_ContainerDoesNotExist_ReturnsNotExistsStatus(t *testing.T) {
	// Given a runner that returns no matching container (Exists → false)
	r := newFake(okOut("")) // Exists → false

	// When GetStatus is called
	got, err := container.GetStatus(r, "mycontainer")

	// Then the status has Exists and Running both false
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Exists {
		t.Error("expected Exists = false")
	}
	if got.Running {
		t.Error("expected Running = false")
	}
}

// TestGetStatus_ContainerRunning_ReturnsFullStatus verifies that GetStatus
// returns full status (Exists, Running, Image, Created) for a running container.
func TestGetStatus_ContainerRunning_ReturnsFullStatus(t *testing.T) {
	// Given a running container with inspect data available
	r := newFake(
		okOut("mycontainer\n"), // Exists → true
		okOut("mycontainer\n"), // IsRunning → true
		okOut("docker.io/myimage:latest|2024-01-15T10:30:00Z\n"), // inspect
	)

	// When GetStatus is called
	got, err := container.GetStatus(r, "mycontainer")

	// Then full status including image and creation time is returned
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Exists {
		t.Error("expected Exists = true")
	}
	if !got.Running {
		t.Error("expected Running = true")
	}
	if got.Image != "docker.io/myimage:latest" {
		t.Errorf("Image = %q, want %q", got.Image, "docker.io/myimage:latest")
	}
	if got.Created != "2024-01-15T10:30:00Z" {
		t.Errorf("Created = %q, want %q", got.Created, "2024-01-15T10:30:00Z")
	}
}

// TestGetStatus_ContainerStopped_ReturnsExistsNotRunning verifies that
// GetStatus returns Exists=true and Running=false for a stopped container.
func TestGetStatus_ContainerStopped_ReturnsExistsNotRunning(t *testing.T) {
	// Given an existing but stopped container with inspect data available
	r := newFake(
		okOut("mycontainer\n"), // Exists → true
		okOut(""),              // IsRunning → false
		okOut("docker.io/myimage:latest|2024-01-10T08:00:00Z\n"), // inspect
	)

	// When GetStatus is called
	got, err := container.GetStatus(r, "mycontainer")

	// Then Exists is true and Running is false
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.Exists {
		t.Error("expected Exists = true")
	}
	if got.Running {
		t.Error("expected Running = false")
	}
}

// TestGetStatus_ExistsError_PropagatesError verifies that GetStatus propagates
// an error returned by the Exists check.
func TestGetStatus_ExistsError_PropagatesError(t *testing.T) {
	// Given a runner that returns an error on the Exists check
	r := newFake(errOut(errors.New("podman error")))

	// When GetStatus is called
	_, err := container.GetStatus(r, "mycontainer")

	// Then the error is propagated
	if err == nil {
		t.Error("expected error, got nil")
	}
}

// TestGetStatus_IsRunningError_PropagatesError verifies that GetStatus
// propagates an error returned by the IsRunning check.
func TestGetStatus_IsRunningError_PropagatesError(t *testing.T) {
	// Given a container that exists but whose IsRunning check returns an error
	r := newFake(
		okOut("mycontainer\n"),             // Exists → true
		errOut(errors.New("podman error")), // IsRunning → error
	)

	// When GetStatus is called
	_, err := container.GetStatus(r, "mycontainer")

	// Then the error is propagated
	if err == nil {
		t.Error("expected error, got nil")
	}
}

// TestGetStatus_InspectError_PropagatesError verifies that GetStatus propagates
// an error returned by the inspect call.
func TestGetStatus_InspectError_PropagatesError(t *testing.T) {
	// Given an existing running container whose inspect call returns an error
	r := newFake(
		okOut("mycontainer\n"),               // Exists → true
		okOut("mycontainer\n"),               // IsRunning → true
		errOut(errors.New("inspect failed")), // inspect → error
	)

	// When GetStatus is called
	_, err := container.GetStatus(r, "mycontainer")

	// Then the inspect error is propagated
	if err == nil {
		t.Error("expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// ImageExists
// ---------------------------------------------------------------------------

// TestImageExists_ImagePresent_ReturnsTrue verifies that ImageExists returns
// true without error when the runner reports the image as present.
func TestImageExists_ImagePresent_ReturnsTrue(t *testing.T) {
	// Given a runner that reports the image as present
	r := &fakeRunner{imageExistsResult: true}

	// When ImageExists is called
	got, err := container.ImageExists(r, "myimage:latest")

	// Then true is returned without error
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Error("expected ImageExists = true when image present")
	}
}

// TestImageExists_ImageAbsent_ReturnsFalse verifies that ImageExists returns
// false without error when the runner reports the image as absent.
func TestImageExists_ImageAbsent_ReturnsFalse(t *testing.T) {
	// Given a runner that reports the image as absent
	r := &fakeRunner{imageExistsResult: false}

	// When ImageExists is called
	got, err := container.ImageExists(r, "myimage:latest")

	// Then false is returned without error
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Error("expected ImageExists = false when image absent")
	}
}

// TestImageExists_UnexpectedError_PropagatesError verifies that ImageExists
// propagates an unexpected error from the runner.
func TestImageExists_UnexpectedError_PropagatesError(t *testing.T) {
	// Given a runner that returns an unexpected error
	r := &fakeRunner{imageExistsErr: errors.New("unexpected podman failure")}

	// When ImageExists is called
	_, err := container.ImageExists(r, "myimage:latest")

	// Then the error is propagated
	if err == nil {
		t.Error("expected error to be propagated, got nil")
	}
}

// ---------------------------------------------------------------------------
// PullImage
// ---------------------------------------------------------------------------

// TestPullImage_CallsRunnerWithCorrectImage verifies that PullImage invokes the
// runner's PullImage method with the requested image name.
func TestPullImage_CallsRunnerWithCorrectImage(t *testing.T) {
	// Given a runner that succeeds and output buffers
	r := &fakeRunner{}
	var stdout, stderr bytes.Buffer

	// When PullImage is called
	_ = container.PullImage(r, "myimage:latest", &stdout, &stderr)

	// Then the runner's PullImage was called with the correct image name
	if !r.pullImageCalled {
		t.Fatal("expected PullImage to be called on runner, but it was not")
	}
	if r.pullImageImage != "myimage:latest" {
		t.Errorf("pullImageImage = %q, want %q", r.pullImageImage, "myimage:latest")
	}
}

// TestPullImage_StreamsOutputToWriter verifies that PullImage writes pull
// progress output to the provided stdout writer.
func TestPullImage_StreamsOutputToWriter(t *testing.T) {
	// Given a runner that produces output during pull
	const fakeOutput = "Pulling from docker.io/myimage:latest\nDone\n"
	r := &fakeRunner{pullOutput: fakeOutput}
	var stdout, stderr bytes.Buffer

	// When PullImage is called
	_ = container.PullImage(r, "myimage:latest", &stdout, &stderr)

	// Then the pull output is written to the stdout writer
	if stdout.String() != fakeOutput {
		t.Errorf("stdout = %q, want %q", stdout.String(), fakeOutput)
	}
}

// TestPullImage_ReturnsNilOnSuccess verifies that PullImage returns nil when
// the runner succeeds.
func TestPullImage_ReturnsNilOnSuccess(t *testing.T) {
	// Given a runner that succeeds (pullImageErr is nil by default)
	r := &fakeRunner{} // pullImageErr is nil by default

	// When PullImage is called
	err := container.PullImage(r, "myimage:latest", io.Discard, io.Discard)

	// Then nil is returned
	if err != nil {
		t.Errorf("expected nil error on success, got %v", err)
	}
}

// TestPullImage_ReturnsErrorOnFailure verifies that PullImage returns an error
// when the runner's PullImage call fails.
func TestPullImage_ReturnsErrorOnFailure(t *testing.T) {
	// Given a runner that returns a pull error
	r := &fakeRunner{pullImageErr: errors.New("image not found")}

	// When PullImage is called
	err := container.PullImage(r, "nosuchimage:latest", io.Discard, io.Discard)

	// Then the error is returned
	if err == nil {
		t.Error("expected error on pull failure, got nil")
	}
}

// ---------------------------------------------------------------------------
// PodmanRunner — OS-boundary unit tests
// ---------------------------------------------------------------------------

// TestPodmanRunner_Run_ErrorIncludesOutput verifies that PodmanRunner.Run
// returns an error that includes the stderr output of the failed command.
func TestPodmanRunner_Run_ErrorIncludesOutput(t *testing.T) {
	// Given a shell command that writes to stderr and exits with a non-zero status
	runner := container.PodmanRunner{}

	// When Run is called with the failing command
	_, err := runner.Run("sh", "-c", "echo 'some error' >&2; exit 1")

	// Then the error includes the stderr output
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "some error") {
		t.Errorf("expected error to contain %q, got: %v", "some error", err)
	}
}

// TestPodmanRunner_Run_SuccessReturnsOutput verifies that PodmanRunner.Run
// returns the command's stdout output when it exits successfully.
func TestPodmanRunner_Run_SuccessReturnsOutput(t *testing.T) {
	// Given a shell command that writes to stdout and exits successfully
	runner := container.PodmanRunner{}

	// When Run is called
	out, err := runner.Run("sh", "-c", "echo hello")

	// Then the output is returned without error
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if !strings.Contains(string(out), "hello") {
		t.Errorf("expected output to contain %q, got: %q", "hello", string(out))
	}
}

// ---------------------------------------------------------------------------
// UserIdentityArgs
// ---------------------------------------------------------------------------

// TestUserIdentityArgs_IncludesPasswdEntry verifies that Create includes
// --passwd-entry with the correct copilot user mapping when building the
// podman create arguments.
func TestUserIdentityArgs_IncludesPasswdEntry(t *testing.T) {
	// Given a fakeRunner and a UserConfig with UID 1001, GID 1002, and home /home/copilot
	runner := &fakeRunner{}
	uc := container.UserConfig{UID: 1001, GID: 1002, HomeDir: "/home/copilot"}

	// When Create is called
	_ = container.Create(runner, "marshal-myapp", "img", nil, uc, "/workspace")

	// Then --passwd-entry with copilot:x:1001:1002 is included in the args
	args := runner.lastCreateArgs()
	// Must include --passwd-entry flag
	if !hasArg(args, "--passwd-entry") {
		t.Errorf("expected --passwd-entry in args\ngot: %v", args)
	}
	// Entry must map to copilot user with correct UID:GID and home
	wantSubstr := "copilot:x:1001:1002"
	found := false
	for _, a := range args {
		if strings.Contains(a, wantSubstr) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected passwd entry containing %q\ngot: %v", wantSubstr, args)
	}
}

// TestUserIdentityArgs_HomeInPasswdEntry verifies that Create embeds the correct
// home directory and UID:GID in the passwd entry value.
func TestUserIdentityArgs_HomeInPasswdEntry(t *testing.T) {
	// Given a fakeRunner and a UserConfig with UID 500, GID 500, and home /home/copilot
	runner := &fakeRunner{}
	uc := container.UserConfig{UID: 500, GID: 500, HomeDir: "/home/copilot"}

	// When Create is called
	_ = container.Create(runner, "marshal-myapp", "img", nil, uc, "/workspace")

	// Then the passwd entry contains the correct home directory and UID:GID
	args := runner.lastCreateArgs()
	found := false
	for _, a := range args {
		if strings.Contains(a, "/home/copilot") && strings.Contains(a, "copilot:x:500:500") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected passwd entry with home /home/copilot for UID 500:500\ngot: %v", args)
	}
}

// ---------------------------------------------------------------------------
// Exec
// ---------------------------------------------------------------------------

// TestExec_CallsPodmanExec verifies that Exec calls execFn with "podman" as
// argv[0] and "exec" as argv[1].
func TestExec_CallsPodmanExec(t *testing.T) {
	// Given a fakeExec spy and a container name
	var captured []string
	fe := func(argv []string) error { captured = argv; return nil }

	// When Exec is called
	err := container.Exec(fe, "marshal-myapp", "bash")

	// Then argv[0] is "podman" and argv[1] is "exec"
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(captured) < 2 {
		t.Fatalf("expected at least 2 args, got %d: %v", len(captured), captured)
	}
	if captured[0] != "podman" {
		t.Errorf("argv[0] = %q, want %q", captured[0], "podman")
	}
	if captured[1] != "exec" {
		t.Errorf("argv[1] = %q, want %q", captured[1], "exec")
	}
}

// TestExec_InteractiveFlagsPresent verifies that Exec passes "-it" as a single
// token in argv so that podman allocates a pseudo-TTY and keeps stdin open.
func TestExec_InteractiveFlagsPresent(t *testing.T) {
	// Given a fakeExec spy
	var captured []string
	fe := func(argv []string) error { captured = argv; return nil }

	// When Exec is called
	_ = container.Exec(fe, "marshal-myapp", "bash")

	// Then "-it" appears somewhere in argv
	if !hasArg(captured, "-it") {
		t.Errorf("expected \"-it\" in argv; got %v", captured)
	}
}

// TestExec_ContainerNameInArgs verifies that Exec places containerName at
// argv[3], immediately after "podman exec -it".
func TestExec_ContainerNameInArgs(t *testing.T) {
	// Given a fakeExec spy and a known container name
	var captured []string
	fe := func(argv []string) error { captured = argv; return nil }
	containerName := "marshal-myapp"

	// When Exec is called with that container name
	_ = container.Exec(fe, containerName, "bash")

	// Then containerName is at argv[3]
	if len(captured) < 4 {
		t.Fatalf("expected at least 4 args, got %d: %v", len(captured), captured)
	}
	if captured[3] != containerName {
		t.Errorf("argv[3] = %q, want %q", captured[3], containerName)
	}
}

// TestExec_CommandAppendsCorrectly verifies that Exec appends all command
// tokens in order after the container name so that multi-word commands are
// passed through intact.
func TestExec_CommandAppendsCorrectly(t *testing.T) {
	// Given a fakeExec spy and a multi-token command
	var captured []string
	fe := func(argv []string) error { captured = argv; return nil }
	cmd := []string{"sh", "-c", "echo hello"}

	// When Exec is called with those command tokens
	_ = container.Exec(fe, "marshal-myapp", cmd...)

	// Then all command tokens appear in order after containerName
	// argv layout: podman exec -it <name> sh -c "echo hello"
	want := []string{"podman", "exec", "-it", "marshal-myapp", "sh", "-c", "echo hello"}
	if len(captured) != len(want) {
		t.Fatalf("argv = %v, want %v", captured, want)
	}
	for i, w := range want {
		if captured[i] != w {
			t.Errorf("argv[%d] = %q, want %q", i, captured[i], w)
		}
	}
}
