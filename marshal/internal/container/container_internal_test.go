// SPDX-License-Identifier: AGPL-3.0-or-later
// Internal (whitebox) tests for unexported helpers in the container package.
// This file uses package container (not package container_test) so it can reach
// unexported functions directly without exporting them for testing.
package container

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestImageExists_UnexpectedFailureIncludesOutput verifies that when
// `podman image exists` fails with an unexpected (non-exit-1) error AND
// produces output (e.g. "Error: cannot connect to Podman socket"), that output
// is included in the error returned to the caller.
//
// Acceptance criterion: error messages from unexpected ImageExists failures
// include Podman output.
func TestImageExists_UnexpectedFailureIncludesOutput(t *testing.T) {
	// Given: a fake podman binary that exits with an unexpected error code (2)
	// and writes a diagnostic message to stderr.
	dir := t.TempDir()
	fakePodman := filepath.Join(dir, "fake-podman")
	script := "#!/bin/sh\necho 'Error: cannot connect to Podman socket' >&2\nexit 2\n"
	if err := os.WriteFile(fakePodman, []byte(script), 0o755); err != nil {
		t.Fatalf("writing fake podman script: %v", err)
	}

	podmanBin = fakePodman
	t.Cleanup(func() { podmanBin = "podman" })

	// When: ImageExists is called.
	_, err := ImageExists(PodmanRunner{}, "any-image")

	// Then: an error is returned AND it includes the Podman diagnostic output.
	if err == nil {
		t.Fatal("expected an error from unexpected Podman failure, got nil")
	}
	const wantSubstr = "cannot connect to Podman socket"
	if !strings.Contains(err.Error(), wantSubstr) {
		t.Errorf("error %q does not contain expected Podman output %q", err.Error(), wantSubstr)
	}
}

// TestParseInspectOutput_WithLeadingWarning verifies that parseInspectOutput
// returns the correct image and created values when podman prefixes its output
// with a warning line (e.g. from CombinedOutput mixing stderr into stdout).
func TestParseInspectOutput_WithLeadingWarning(t *testing.T) {
	// Given an inspect output string prefixed with a podman warning line
	// Field order: image|created|imageDigest|version
	raw := "Warning: blah blah\nimage-name|2024-01-01|sha256:abc123|v2.0.0"

	// When the output is parsed
	image, created, imageDigest, version := parseInspectOutput(raw)

	// Then the image, created, version, and imageDigest fields are correctly extracted, ignoring the warning
	if image != "image-name" {
		t.Errorf("image = %q, want %q", image, "image-name")
	}
	if created != "2024-01-01" {
		t.Errorf("created = %q, want %q", created, "2024-01-01")
	}
	if version != "v2.0.0" {
		t.Errorf("version = %q, want %q", version, "v2.0.0")
	}
	if imageDigest != "sha256:abc123" {
		t.Errorf("imageDigest = %q, want %q", imageDigest, "sha256:abc123")
	}
}

// TestParseInspectOutput_AbsentVersionLabel_ReturnsEmptyString verifies that
// parseInspectOutput returns an empty version when the fourth field is empty
// (i.e. the org.opencontainers.image.version label is not set on the image).
func TestParseInspectOutput_AbsentVersionLabel_ReturnsEmptyString(t *testing.T) {
	// Given inspect output with an empty version field (field 4); digest is field 3
	raw := "Warning: blah blah\nimage-name|2024-01-01|sha256:abc123|"

	// When the output is parsed
	_, _, imageDigest, version := parseInspectOutput(raw)

	// Then version is the empty string and digest is correctly parsed
	if version != "" {
		t.Errorf("version = %q, want empty string", version)
	}
	if imageDigest != "sha256:abc123" {
		t.Errorf("imageDigest = %q, want %q", imageDigest, "sha256:abc123")
	}
}

// TestParseInspectOutput_VersionLabelWithPipe_DigestUnaffected verifies that a
// version label containing a pipe character does not corrupt the imageDigest
// field. imageDigest (field 3) is placed before the version label (field 4) so
// that SplitN absorbs any extra pipes into the version slot.
func TestParseInspectOutput_VersionLabelWithPipe_DigestUnaffected(t *testing.T) {
	// Given a version label that contains a pipe character
	raw := "image-name|2024-01-01|sha256:abc123|1.0|injected"

	// When the output is parsed
	_, _, imageDigest, version := parseInspectOutput(raw)

	// Then imageDigest is unaffected and version absorbs the extra pipe content
	if imageDigest != "sha256:abc123" {
		t.Errorf("imageDigest = %q, want %q; pipe in version label must not corrupt digest", imageDigest, "sha256:abc123")
	}
	if version != "1.0|injected" {
		t.Errorf("version = %q, want %q", version, "1.0|injected")
	}
}

// TestParseInspectOutput_EmptyInput verifies that parseInspectOutput returns
// empty strings for all four fields when given an empty (or all-whitespace) input.
func TestParseInspectOutput_EmptyInput(t *testing.T) {
	// Given an empty input string

	// When the output is parsed
	image, created, imageDigest, version := parseInspectOutput("")

	// Then all four fields are returned as empty strings
	if image != "" {
		t.Errorf("image = %q, want empty string", image)
	}
	if created != "" {
		t.Errorf("created = %q, want empty string", created)
	}
	if version != "" {
		t.Errorf("version = %q, want empty string", version)
	}
	if imageDigest != "" {
		t.Errorf("imageDigest = %q, want empty string", imageDigest)
	}
}
