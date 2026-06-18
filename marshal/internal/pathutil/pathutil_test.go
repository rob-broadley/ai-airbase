// SPDX-License-Identifier: AGPL-3.0-or-later
package pathutil

import (
	"path/filepath"
	"testing"
)

// TestIsPathUnder_EqualPaths verifies that child == parent is treated as "under".
func TestIsPathUnder_EqualPaths(t *testing.T) {
	if !IsPathUnder("/a/b", "/a/b") {
		t.Error("expected /a/b to be under /a/b (equal)")
	}
}

// TestIsPathUnder_ChildUnderParent verifies the basic containment case.
func TestIsPathUnder_ChildUnderParent(t *testing.T) {
	if !IsPathUnder("/a/b/c", "/a/b") {
		t.Error("expected /a/b/c to be under /a/b")
	}
}

// TestIsPathUnder_PrefixBoundaryNotMatched verifies that /a/bc is NOT considered
// under /a/b. This is the safety property the trailing-separator logic enforces.
func TestIsPathUnder_PrefixBoundaryNotMatched(t *testing.T) {
	if IsPathUnder("/a/bc", "/a/b") {
		t.Error("expected /a/bc to NOT be under /a/b (prefix boundary)")
	}
}

// TestIsPathUnder_ParentAlreadyHasTrailingSeparator verifies that the function
// is robust to the caller having already appended a separator to parent.
func TestIsPathUnder_ParentAlreadyHasTrailingSeparator(t *testing.T) {
	if !IsPathUnder("/a/b/c", "/a/b/") {
		t.Error("expected /a/b/c to be under /a/b/")
	}
	if IsPathUnder("/a/bc", "/a/b/") {
		t.Error("expected /a/bc to NOT be under /a/b/ (prefix boundary)")
	}
}

// TestIsPathUnder_UncleanedChildNotMatched verifies that a child containing
// ../ elements is not incorrectly reported as under a parent it resolves
// outside of. Without filepath.Clean in IsPathUnder, /a/b/../c would match
// prefix /a/b/ even though it resolves to /a/c which is not under /a/b.
func TestIsPathUnder_UncleanedChildNotMatched(t *testing.T) {
	if IsPathUnder("/a/b/../c", "/a/b") {
		t.Error("expected /a/b/../c (resolves to /a/c) to NOT be under /a/b")
	}
}

// TestIsPathUnder_ChildOutsideParent verifies the negative case.
func TestIsPathUnder_ChildOutsideParent(t *testing.T) {
	if IsPathUnder("/x/y", "/a/b") {
		t.Error("expected /x/y to NOT be under /a/b")
	}
}

// TestIsPathUnder_PathWithSymlinkComponents is a placeholder for the
// security-critical test that hardenProjectDir relies on. The actual
// symlink-resolution tests live in helpers_test.go where the full
// EvalSymlinks integration is exercised.
func TestIsPathUnder_PathWithSymlinkComponents(t *testing.T) {
	// This test exists to document that isPathUnder must work correctly
	// with paths that have symlink components resolved. The actual
	// integration tests are in helpers_test.go.
	_ = filepath.EvalSymlinks
}
