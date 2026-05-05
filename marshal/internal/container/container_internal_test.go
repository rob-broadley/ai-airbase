// SPDX-License-Identifier: AGPL-3.0-or-later
// Internal (whitebox) tests for unexported helpers in the container package.
// This file uses package container (not package container_test) so it can reach
// unexported functions directly without exporting them for testing.
package container

import "testing"

// TestParseInspectOutput_WithLeadingWarning verifies that parseInspectOutput
// returns the correct image and created values when podman prefixes its output
// with a warning line (e.g. from CombinedOutput mixing stderr into stdout).
func TestParseInspectOutput_WithLeadingWarning(t *testing.T) {
	// Given an inspect output string prefixed with a podman warning line
	raw := "Warning: blah blah\nimage-name|2024-01-01"

	// When the output is parsed
	image, created := parseInspectOutput(raw)

	// Then the image and created fields are correctly extracted, ignoring the warning
	if image != "image-name" {
		t.Errorf("image = %q, want %q", image, "image-name")
	}
	if created != "2024-01-01" {
		t.Errorf("created = %q, want %q", created, "2024-01-01")
	}
}

// TestParseInspectOutput_EmptyInput verifies that parseInspectOutput returns
// empty strings for both fields when given an empty (or all-whitespace) input.
func TestParseInspectOutput_EmptyInput(t *testing.T) {
	// Given an empty input string

	// When the output is parsed
	image, created := parseInspectOutput("")

	// Then both fields are returned as empty strings
	if image != "" {
		t.Errorf("image = %q, want empty string", image)
	}
	if created != "" {
		t.Errorf("created = %q, want empty string", created)
	}
}
