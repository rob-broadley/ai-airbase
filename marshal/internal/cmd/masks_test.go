// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/config"
	"github.com/rob-broadley/ai-airbase/marshal/internal/container"
)

// ---------------------------------------------------------------------------
// Unit tests for resolveMaskPaths
// ---------------------------------------------------------------------------

// TestResolveMaskPaths_HappyPath_ValidSingleMask verifies valid resolution for a single mask.
func TestResolveMaskPaths_HappyPath_ValidSingleMask(t *testing.T) {
	// Given a single project mount resolving to a temp dir and mask input ".venv"
	mountRoot := t.TempDir()
	cfg := &config.Config{Mounts: []string{mountRoot}, Masks: []string{}}

	// When mask paths are resolved
	masks, err := resolveMaskPaths(mountRoot, []string{mountRoot}, []string{".venv"}, cfg)

	// Then no error is returned and the resolved mask is the absolute path
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(mountRoot, ".venv")
	if len(masks) != 1 || masks[0] != want {
		t.Errorf("got masks %v, want [%s]", masks, want)
	}
}

// TestResolveMaskPaths_TrailingSlashNormalised verifies trailing slashes are normalised.
func TestResolveMaskPaths_TrailingSlashNormalised(t *testing.T) {
	// Given mask input ".venv/" (with trailing slash)
	mountRoot := t.TempDir()
	cfg := &config.Config{Mounts: []string{mountRoot}, Masks: []string{}}

	// When mask paths are resolved
	masks, err := resolveMaskPaths(mountRoot, []string{mountRoot}, []string{".venv/"}, cfg)

	// Then the result is identical to resolving ".venv"
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(mountRoot, ".venv")
	if len(masks) != 1 || masks[0] != want {
		t.Errorf("got masks %v, want [%s]", masks, want)
	}
}

// TestResolveMaskPaths_NoFlagsReturnsStored covers the no-op early-return path.
func TestResolveMaskPaths_NoFlagsReturnsStored(t *testing.T) {
	// Given stored masks and no flag values
	mountRoot := t.TempDir()
	stored := []string{filepath.Join(mountRoot, ".cache")}
	cfg := &config.Config{Mounts: []string{mountRoot}, Masks: stored}

	// When mask paths are resolved with empty flag values
	masks, err := resolveMaskPaths(mountRoot, []string{mountRoot}, []string{}, cfg)

	// Then stored masks are returned unchanged
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(masks) != 1 || masks[0] != stored[0] {
		t.Errorf("got masks %v, want %v", masks, stored)
	}
}

// TestResolveMaskPaths_RejectionCases covers the colon, dotdot,
// absolute path not under mount, mount root, outside mount, and duplicate
// rejection cases.
func TestResolveMaskPaths_RejectionCases(t *testing.T) {
	mountRoot := t.TempDir()

	tests := []struct {
		name        string
		mountPaths  []string
		maskInputs  []string
		wantErrFrag string // substring that must appear in the error message
	}{
		{
			name:        "colon in path is rejected",
			mountPaths:  []string{mountRoot},
			maskInputs:  []string{"node:modules"},
			wantErrFrag: "must not contain",
		},
		{
			name:        "dotdot component is rejected",
			mountPaths:  []string{mountRoot},
			maskInputs:  []string{"../sibling"},
			wantErrFrag: "no configured mount",
		},
		{
			name:        "absolute path not under any mount is rejected",
			mountPaths:  []string{mountRoot},
			maskInputs:  []string{"/not/under/any/mount"},
			wantErrFrag: "no configured mount",
		},
		{
			name:        "path equal to mount root is rejected",
			mountPaths:  []string{mountRoot},
			maskInputs:  []string{"."},
			wantErrFrag: "mount root",
		},
		{
			name:        "dotdot escape path is rejected",
			mountPaths:  []string{mountRoot},
			maskInputs:  []string{"../../other"},
			wantErrFrag: "no configured mount",
		},
		{
			name:        "duplicate mask paths are rejected",
			mountPaths:  []string{mountRoot},
			maskInputs:  []string{".venv", ".venv"},
			wantErrFrag: "duplicate",
		},
		{
			name:        "nested mask paths are rejected (outer listed first)",
			mountPaths:  []string{mountRoot},
			maskInputs:  []string{"src", "src/vendor"},
			wantErrFrag: "nested inside",
		},
		{
			name:        "nested mask paths are rejected (inner listed first)",
			mountPaths:  []string{mountRoot},
			maskInputs:  []string{"src/vendor", "src"},
			wantErrFrag: "nested inside",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{Mounts: tt.mountPaths, Masks: []string{}}

			// When mask paths are resolved
			_, err := resolveMaskPaths(mountRoot, tt.mountPaths, tt.maskInputs, cfg)

			// Then an error is returned containing the expected fragment
			if err == nil {
				t.Fatalf("expected an error containing %q, got nil", tt.wantErrFrag)
			}
			if !strings.Contains(err.Error(), tt.wantErrFrag) {
				t.Errorf("expected error containing %q, got: %v", tt.wantErrFrag, err)
			}
		})
	}
}

// TestResolveMaskPaths_AbsolutePath_UnderMount verifies that an absolute mask
// path that falls inside a configured mount is accepted and stored as-is.
func TestResolveMaskPaths_AbsolutePath_UnderMount(t *testing.T) {
	// Given a single project mount and an absolute mask path under that mount
	mountRoot := t.TempDir()
	absMask := filepath.Join(mountRoot, "subdir", ".venv")
	cfg := &config.Config{Mounts: []string{mountRoot}, Masks: []string{}}

	// When mask paths are resolved with an absolute path
	masks, err := resolveMaskPaths(mountRoot, []string{mountRoot}, []string{absMask}, cfg)

	// Then no error is returned and the absolute path is stored as-is
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(masks) != 1 || masks[0] != absMask {
		t.Errorf("got masks %v, want [%s]", masks, absMask)
	}
}

// TestResolveMaskPaths_MultiMount_MaskUnderSecondMount verifies that with two
// configured project mounts a mask relative to the second mount is accepted.
func TestResolveMaskPaths_MultiMount_MaskUnderSecondMount(t *testing.T) {
	// Given two project mounts; cwd is set to the second mount root so that
	// a relative mask resolves under the second mount
	mountRoot1 := t.TempDir()
	mountRoot2 := t.TempDir()
	cwd := mountRoot2
	cfg := &config.Config{Mounts: []string{mountRoot1, mountRoot2}, Masks: []string{}}

	// When a relative mask path (".venv") is resolved with cwd=mountRoot2
	masks, err := resolveMaskPaths(cwd, []string{mountRoot1, mountRoot2}, []string{".venv"}, cfg)

	// Then the mask resolves to mountRoot2/.venv and succeeds
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.Join(mountRoot2, ".venv")
	if len(masks) != 1 || masks[0] != want {
		t.Errorf("got masks %v, want [%s]", masks, want)
	}
}

// TestResolveMaskPaths_MultiMount_AbsoluteMaskUnderSecondMount verifies that
// an absolute mask path under the second of two configured mounts is accepted.
func TestResolveMaskPaths_MultiMount_AbsoluteMaskUnderSecondMount(t *testing.T) {
	// Given two project mounts and an absolute mask path under the second mount
	mountRoot1 := t.TempDir()
	mountRoot2 := t.TempDir()
	absMask := filepath.Join(mountRoot2, "node_modules")
	cfg := &config.Config{Mounts: []string{mountRoot1, mountRoot2}, Masks: []string{}}

	// When mask paths are resolved
	masks, err := resolveMaskPaths(mountRoot2, []string{mountRoot1, mountRoot2}, []string{absMask}, cfg)

	// Then the absolute path is accepted and returned as-is
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(masks) != 1 || masks[0] != absMask {
		t.Errorf("got masks %v, want [%s]", masks, absMask)
	}
}

// TestResolveMaskPaths_RegularFileOnHostIsRejected verifies host files are rejected.
func TestResolveMaskPaths_RegularFileOnHostIsRejected(t *testing.T) {
	// Given a mount root and a regular file ".gitignore" that exists on the host
	mountRoot := t.TempDir()
	gitignore := filepath.Join(mountRoot, ".gitignore")
	if err := os.WriteFile(gitignore, []byte("*.log\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{Mounts: []string{mountRoot}, Masks: []string{}}

	// When mask paths are resolved for ".gitignore"
	_, err := resolveMaskPaths(mountRoot, []string{mountRoot}, []string{".gitignore"}, cfg)

	// Then an error is returned indicating the path exists as a file
	if err == nil {
		t.Fatal("expected an error for regular file mask, got nil")
	}
	if !strings.Contains(err.Error(), "file") {
		t.Errorf("expected error to mention 'file', got: %v", err)
	}
}

// TestResolveMaskPaths_AbsentPathOnHostIsAccepted verifies absent host paths are accepted.
func TestResolveMaskPaths_AbsentPathOnHostIsAccepted(t *testing.T) {
	// Given a mount root and a mask input ".venv" where no .venv exists on the host
	mountRoot := t.TempDir()
	cfg := &config.Config{Mounts: []string{mountRoot}, Masks: []string{}}

	// When mask paths are resolved
	_, err := resolveMaskPaths(mountRoot, []string{mountRoot}, []string{".venv"}, cfg)

	// Then no error is returned (absent path is valid)
	if err != nil {
		t.Fatalf("expected no error for absent path, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Unit tests for buildMaskVolumes
// ---------------------------------------------------------------------------

// TestBuildMaskVolumes_StaleConfigMaskReturnsError verifies that a mask path
// no longer under any configured mount fails closed with an error.
func TestBuildMaskVolumes_StaleConfigMaskReturnsError(t *testing.T) {
	// Given a container with a single mount and a stale mask that points
	// outside that mount (e.g. after the mount path changed)
	mountRoot := t.TempDir()
	staleMask := "/some/other/path/.venv"
	mountSpecs := []container.MountSpec{{HostPath: mountRoot, ContainerPath: "/workspace/proj"}}

	// When buildMaskVolumes is called
	volumes, err := container.BuildMaskVolumes("testcontainer", []string{staleMask}, []string{mountRoot}, mountSpecs)

	// Then an error is returned and no volumes are produced
	if err == nil {
		t.Fatal("expected an error for stale mask, got nil")
	}
	if !strings.Contains(err.Error(), "no longer inside any configured mount") {
		t.Errorf("expected stale-mask error, got: %v", err)
	}
	if len(volumes) != 0 {
		t.Errorf("expected no volumes on error, got %v", volumes)
	}
}

// TestBuildMaskVolumes_ValidMaskProducesVolume verifies that a valid mask path
// produces a NamedVolumeMount with the correct name and container path.
func TestBuildMaskVolumes_ValidMaskProducesVolume(t *testing.T) {
	// Given a mount root and a mask under that mount
	mountRoot := t.TempDir()
	mask := filepath.Join(mountRoot, ".venv")
	mountSpecs := []container.MountSpec{{HostPath: mountRoot, ContainerPath: "/workspace/proj"}}

	// When buildMaskVolumes is called
	volumes, err := container.BuildMaskVolumes("testcontainer", []string{mask}, []string{mountRoot}, mountSpecs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Then one volume is returned with the expected name and path
	if len(volumes) != 1 {
		t.Fatalf("expected 1 volume, got %d", len(volumes))
	}
	expectedName := "testcontainer-mask-" + filepath.Base(mountRoot) + "-.venv"
	if volumes[0].Name != expectedName {
		t.Errorf("unexpected volume name: %s", volumes[0].Name)
	}
	if volumes[0].ContainerPath != "/workspace/proj/.venv" {
		t.Errorf("unexpected container path: %s", volumes[0].ContainerPath)
	}
}

// ---------------------------------------------------------------------------
// CWD outside all configured mounts
// ---------------------------------------------------------------------------

// TestResolveMaskPaths_CWDOutsideMounts_AbsolutePathRequired verifies that
// when CWD is outside all configured mounts, relative mask paths resolve
// outside the mounts and are rejected, while an absolute path under a mount
// is accepted.
func TestResolveMaskPaths_CWDOutsideMounts_AbsolutePathRequired(t *testing.T) {
	// Given a project mount and a CWD that is NOT under that mount
	mountRoot := t.TempDir()
	cwdOutside := t.TempDir() // different temp dir, not under mountRoot

	// Relative mask resolves against cwdOutside → outside mountRoot → rejected
	cfg := &config.Config{Mounts: []string{mountRoot}, Masks: []string{}}
	_, err := resolveMaskPaths(cwdOutside, []string{mountRoot}, []string{".venv"}, cfg)
	if err == nil {
		t.Fatal("expected an error when relative mask resolves outside all mounts, got nil")
	}
	if !strings.Contains(err.Error(), "no configured mount") {
		t.Errorf("expected 'no configured mount' error, got: %v", err)
	}

	// Absolute mask under mountRoot is accepted even with cwdOutside
	absMask := filepath.Join(mountRoot, ".venv")
	cfg2 := &config.Config{Mounts: []string{mountRoot}, Masks: []string{}}
	masks, err2 := resolveMaskPaths(cwdOutside, []string{mountRoot}, []string{absMask}, cfg2)
	if err2 != nil {
		t.Fatalf("expected absolute path under mount to succeed, got: %v", err2)
	}
	if len(masks) != 1 || masks[0] != absMask {
		t.Errorf("got masks %v, want [%s]", masks, absMask)
	}
}
