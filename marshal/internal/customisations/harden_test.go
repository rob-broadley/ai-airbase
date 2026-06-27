// SPDX-License-Identifier: AGPL-3.0-or-later
package customisations_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/customisations"
)

func TestHardenSourceTree_EmptySourceTree_NoError(t *testing.T) {
	root := t.TempDir()

	err := customisations.HardenSourceTree(root)

	if err != nil {
		t.Fatalf("expected no error for empty source tree, got: %v", err)
	}
}

func TestHardenSourceTree_RegularFiles_NoError(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "subdir")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "nested.txt"), []byte("nested"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := customisations.HardenSourceTree(root)

	if err != nil {
		t.Fatalf("expected no error for regular files, got: %v", err)
	}
}

func TestHardenSourceTree_DotDotSymlinkEscapingRoot_ReturnsSecurityViolation(t *testing.T) {
	root := t.TempDir()
	outsideFile := createOutsideFile(t)
	relTarget, err := filepath.Rel(root, outsideFile)
	if err != nil {
		t.Fatal(err)
	}
	symlinkPath := filepath.Join(root, "escape")
	if err := os.Symlink(relTarget, symlinkPath); err != nil {
		t.Fatal(err)
	}

	err = customisations.HardenSourceTree(root)

	if err == nil {
		t.Fatal("expected an error for symlink with .. escaping source root, got nil")
	}
	if !strings.Contains(err.Error(), "security violation") {
		t.Errorf("error %q should contain %q", err.Error(), "security violation")
	}
}

func TestHardenSourceTree_SymlinkChainEscapingRoot_ReturnsSecurityViolation(t *testing.T) {
	root := t.TempDir()
	outsideFile := createOutsideFile(t)
	linkB := filepath.Join(root, "linkB")
	if err := os.Symlink(outsideFile, linkB); err != nil {
		t.Fatal(err)
	}
	linkA := filepath.Join(root, "linkA")
	if err := os.Symlink("linkB", linkA); err != nil {
		t.Fatal(err)
	}

	err := customisations.HardenSourceTree(root)

	if err == nil {
		t.Fatal("expected an error for symlink chain escaping source root, got nil")
	}
	if !strings.Contains(err.Error(), "security violation") {
		t.Errorf("error %q should contain %q", err.Error(), "security violation")
	}
}

func TestHardenSourceTree_BrokenAbsoluteSymlinkOutsideRoot_ReturnsSecurityViolation(t *testing.T) {
	root := t.TempDir()
	symlinkPath := filepath.Join(root, "broken")
	// Broken absolute symlink whose raw target is outside the source root;
	// after the security fix this must be rejected.
	if err := os.Symlink("/nonexistent/target", symlinkPath); err != nil {
		t.Fatal(err)
	}

	err := customisations.HardenSourceTree(root)

	if err == nil {
		t.Fatal("expected security violation for broken absolute symlink outside source root, got nil")
	}
	if !strings.Contains(err.Error(), "security violation") {
		t.Errorf("expected error containing %q, got: %v", "security violation", err)
	}
}

func TestHardenSourceTree_RelativeSymlinkInsideRoot_NoError(t *testing.T) {
	root := t.TempDir()
	subdir := filepath.Join(root, "sub")
	if err := os.MkdirAll(subdir, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(subdir, "target.txt")
	if err := os.WriteFile(target, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	symlinkPath := filepath.Join(root, "link")
	if err := os.Symlink("sub/target.txt", symlinkPath); err != nil {
		t.Fatal(err)
	}

	err := customisations.HardenSourceTree(root)

	if err != nil {
		t.Fatalf("expected no error for symlink inside root, got: %v", err)
	}
}

func TestHardenSourceTree_SymlinkEscapingRoot_ReturnsSecurityViolation(t *testing.T) {
	root := t.TempDir()
	outsideFile := createOutsideFile(t)
	symlinkPath := filepath.Join(root, "escape")
	if err := os.Symlink(outsideFile, symlinkPath); err != nil {
		t.Fatal(err)
	}

	err := customisations.HardenSourceTree(root)

	if err == nil {
		t.Fatal("expected an error for symlink escaping source root, got nil")
	}
	if !strings.Contains(err.Error(), "security violation") {
		t.Errorf("error %q should contain %q", err.Error(), "security violation")
	}
	if !strings.Contains(err.Error(), symlinkPath) {
		t.Errorf("error %q should mention the offending path %q", err.Error(), symlinkPath)
	}
	if !strings.Contains(err.Error(), outsideFile) {
		t.Errorf("error %q should mention the resolved target %q", err.Error(), outsideFile)
	}
}
