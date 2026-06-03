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
// returns all six fields (image, created, imageDigest, imageRef, port, version)
// correctly when podman prefixes its output with a warning line (e.g. from
// CombinedOutput mixing stderr into stdout).
func TestParseInspectOutput_WithLeadingWarning(t *testing.T) {
	// Given an inspect output string prefixed with a podman warning line
	// Field order: image|created|imageDigest|imageRef|port|version
	raw := "Warning: blah blah\nimage-name|2024-01-01|sha256:abc123|ghcr.io/foo:latest|4096|v2.0.0"

	// When the output is parsed
	f := parseInspectOutput(raw)

	// Then the fields are correctly extracted, ignoring the warning
	if f.image != "image-name" {
		t.Errorf("image = %q, want %q", f.image, "image-name")
	}
	if f.created != "2024-01-01" {
		t.Errorf("created = %q, want %q", f.created, "2024-01-01")
	}
	if f.version != "v2.0.0" {
		t.Errorf("version = %q, want %q", f.version, "v2.0.0")
	}
	if f.imageDigest != "sha256:abc123" {
		t.Errorf("imageDigest = %q, want %q", f.imageDigest, "sha256:abc123")
	}
	if f.imageRef != "ghcr.io/foo:latest" {
		t.Errorf("imageRef = %q, want %q", f.imageRef, "ghcr.io/foo:latest")
	}
	if f.port != "4096" {
		t.Errorf("port = %q, want %q", f.port, "4096")
	}
}

// TestParseInspectOutput_AbsentVersionLabel_ReturnsEmptyString verifies that
// parseInspectOutput returns an empty version when the sixth field is empty
// (i.e. the org.opencontainers.image.version label is not set on the image).
func TestParseInspectOutput_AbsentVersionLabel_ReturnsEmptyString(t *testing.T) {
	// Given inspect output with an empty version field (field 6); digest is field 3, imageRef is field 4, port is field 5
	raw := "Warning: blah blah\nimage-name|2024-01-01|sha256:abc123||4096|"

	// When the output is parsed
	f := parseInspectOutput(raw)

	// Then version and imageRef are empty strings and digest and port are correctly parsed
	if f.version != "" {
		t.Errorf("version = %q, want empty string", f.version)
	}
	if f.imageRef != "" {
		t.Errorf("imageRef = %q, want empty string", f.imageRef)
	}
	if f.imageDigest != "sha256:abc123" {
		t.Errorf("imageDigest = %q, want %q", f.imageDigest, "sha256:abc123")
	}
	if f.port != "4096" {
		t.Errorf("port = %q, want %q", f.port, "4096")
	}
}

// TestParseInspectOutput_VersionLabelWithPipe_DigestUnaffected verifies that a
// version label containing a pipe character does not corrupt imageDigest or
// imageRef. imageRef (field 4), port (field 5) and version (field 6, last) are placed after
// imageDigest (field 3); SplitN(..., 6) absorbs any extra pipes into the
// version slot, so neither digest, imageRef nor port can be corrupted.
func TestParseInspectOutput_VersionLabelWithPipe_DigestUnaffected(t *testing.T) {
	// Given a version label that contains a pipe character
	// Field order: image|created|imageDigest|imageRef|port|version (version absorbs extra pipes via SplitN)
	raw := "image-name|2024-01-01|sha256:abc123|ghcr.io/foo:latest|4096|1.0|injected"

	// When the output is parsed
	f := parseInspectOutput(raw)

	// Then imageDigest, imageRef and port are unaffected; version absorbs the extra pipe content
	if f.imageDigest != "sha256:abc123" {
		t.Errorf("imageDigest = %q, want %q; pipe in version label must not corrupt digest", f.imageDigest, "sha256:abc123")
	}
	if f.imageRef != "ghcr.io/foo:latest" {
		t.Errorf("imageRef = %q, want %q; pipe in version label must not corrupt imageRef", f.imageRef, "ghcr.io/foo:latest")
	}
	if f.port != "4096" {
		t.Errorf("port = %q, want %q", f.port, "4096")
	}
	if f.version != "1.0|injected" {
		t.Errorf("version = %q, want %q", f.version, "1.0|injected")
	}
}

// TestParseInspectOutput_EmptyInput verifies that parseInspectOutput returns
// empty strings for all six fields when given an empty (or all-whitespace) input.
func TestParseInspectOutput_EmptyInput(t *testing.T) {
	// Given an empty input string

	// When the output is parsed
	f := parseInspectOutput("")

	// Then all six fields are returned as empty strings
	if f.image != "" {
		t.Errorf("image = %q, want empty string", f.image)
	}
	if f.created != "" {
		t.Errorf("created = %q, want empty string", f.created)
	}
	if f.version != "" {
		t.Errorf("version = %q, want empty string", f.version)
	}
	if f.imageDigest != "" {
		t.Errorf("imageDigest = %q, want empty string", f.imageDigest)
	}
	if f.imageRef != "" {
		t.Errorf("imageRef = %q, want empty string", f.imageRef)
	}
	if f.port != "" {
		t.Errorf("port = %q, want empty string", f.port)
	}
}
