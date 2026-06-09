// SPDX-License-Identifier: AGPL-3.0-or-later
package customisations_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/customisations"
)

// --- Ensure creates subdirs ---

func TestEnsure_CreatesThreeSubdirsWith0o700(t *testing.T) {
	// Given an injected xdgDataHome function that returns a t.TempDir()-based path
	base := t.TempDir()
	xdgDataHome := func() string { return base }

	// When Ensure is called
	err := customisations.Ensure(xdgDataHome)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Then the three subdirs exist as directories with 0o700 permissions
	wantBase := filepath.Join(base, "marshal", "defaults", "opencode")
	for _, sub := range []string{"config", "share", "state"} {
		p := filepath.Join(wantBase, sub)
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("expected %s to exist: %v", p, err)
		}
		if !info.IsDir() {
			t.Errorf("expected %s to be a directory", p)
		}
		if perm := info.Mode().Perm(); perm != 0o700 {
			t.Errorf("expected %s to have permissions 0o700, got %04o", p, perm)
		}
	}
}

// --- Ensure is idempotent ---

func TestEnsure_Idempotent_DoesNotChmodPreExistingDirs(t *testing.T) {
	// Given the three subdirs exist (config=0o700, share=0o755, state=0o700)
	base := t.TempDir()
	xdgDataHome := func() string { return base }
	assertNoError(t, customisations.Ensure(xdgDataHome))

	wantBase := filepath.Join(base, "marshal", "defaults", "opencode")
	shareDir := filepath.Join(wantBase, "share")
	assertNoError(t, os.Chmod(shareDir, 0o755))

	// When Ensure is called
	err := customisations.Ensure(xdgDataHome)

	// Then no error is returned
	if err != nil {
		t.Fatalf("expected no error on second call, got: %v", err)
	}

	// And the three subdirs still exist with their pre-existing permissions
	// (config=0o700, share=0o755, state=0o700)
	for _, tc := range []struct {
		sub      string
		wantPerm os.FileMode
	}{
		{"config", 0o700},
		{"share", 0o755},
		{"state", 0o700},
	} {
		p := filepath.Join(wantBase, tc.sub)
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("expected %s to exist: %v", p, err)
		}
		if !info.IsDir() {
			t.Errorf("expected %s to be a directory", p)
		}
		if perm := info.Mode().Perm(); perm != tc.wantPerm {
			t.Errorf("expected %s to have permissions %04o, got %04o", p, tc.wantPerm, perm)
		}
	}
}

// --- Ensure refuses to clobber pre-existing file ---

func TestEnsure_RefusesToClobberPreExistingFile(t *testing.T) {
	// Given a regular file exists at the config subdir path
	base := t.TempDir()
	xdgDataHome := func() string { return base }
	configPath := filepath.Join(base, "marshal", "defaults", "opencode", "config")
	assertNoError(t, os.MkdirAll(filepath.Dir(configPath), 0o700))
	wantContent := []byte("not a directory")
	assertNoError(t, os.WriteFile(configPath, wantContent, 0o644))

	// When Ensure is called
	err := customisations.Ensure(xdgDataHome)

	// Then a non-nil error is returned
	if err == nil {
		t.Fatal("expected an error when a file exists at a subdir path, got nil")
	}

	// And the error message contains the offending path
	if !strings.Contains(err.Error(), configPath) {
		t.Errorf("error %q should mention the offending path %q", err.Error(), configPath)
	}

	// And the regular file is left untouched on disk
	gotContent, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("expected to read file at %s: %v", configPath, err)
	}
	if string(gotContent) != string(wantContent) {
		t.Errorf("file content changed: got %q, want %q", gotContent, wantContent)
	}
}

// --- Ensure refuses to follow symlink ---

func TestEnsure_RefusesToFollowSymlink(t *testing.T) {
	// Given a symlink exists at the config subdir path
	base := t.TempDir()
	xdgDataHome := func() string { return base }
	configPath := filepath.Join(base, "marshal", "defaults", "opencode", "config")
	assertNoError(t, os.MkdirAll(filepath.Dir(configPath), 0o700))
	target := filepath.Join(t.TempDir(), "evil-target")
	assertNoError(t, os.Symlink(target, configPath))

	// When Ensure is called
	err := customisations.Ensure(xdgDataHome)

	// Then a non-nil error is returned
	if err == nil {
		t.Fatal("expected an error when a symlink exists at a subdir path, got nil")
	}

	// And the error message contains "refusing to follow symlink"
	if !strings.Contains(err.Error(), "refusing to follow symlink") {
		t.Errorf("error %q should contain %q", err.Error(), "refusing to follow symlink")
	}

	// And the error message contains the offending path
	if !strings.Contains(err.Error(), configPath) {
		t.Errorf("error %q should mention the offending path %q", err.Error(), configPath)
	}

	// And the symlink is left untouched on disk
	info, err := os.Lstat(configPath)
	if err != nil {
		t.Fatalf("expected symlink to exist at %s: %v", configPath, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("expected symlink at %s, got regular entry", configPath)
	}
}

// --- Ensure returns error when XDG base is empty ---

func TestEnsure_XDGUnavailable_ReturnsError(t *testing.T) {
	// Given an xdgDataHome function that returns empty string
	xdgDataHome := func() string { return "" }

	// When Ensure is called
	err := customisations.Ensure(xdgDataHome)

	// Then a non-nil error is returned
	if err == nil {
		t.Fatal("expected an error when XDG base is empty, got nil")
	}

	// And the error message contains "data directory unavailable" and "set HOME or"
	if !strings.Contains(err.Error(), "data directory unavailable") {
		t.Errorf("error %q should contain %q", err.Error(), "data directory unavailable")
	}
	if !strings.Contains(err.Error(), "set HOME or") {
		t.Errorf("error %q should contain %q", err.Error(), "set HOME or")
	}
}

// --- DefaultsDir returns expected path ---

func TestDefaultsDir_ReturnsExpectedPath(t *testing.T) {
	tests := []struct {
		desc string
		base string
		want string
	}{
		{
			desc: "absolute path",
			base: "/tmp/fake-xdg",
			want: filepath.Join("/tmp/fake-xdg", "marshal", "defaults"),
		},
		{
			desc: "home share path",
			base: "/home/user/.local/share",
			want: filepath.Join("/home/user/.local/share", "marshal", "defaults"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			// Given xdgDataHome returns a known path
			// When DefaultsDir is called
			got := customisations.DefaultsDir(tc.base)

			// Then the result equals the expected path
			if got != tc.want {
				t.Errorf("DefaultsDir() = %q, want %q", got, tc.want)
			}
		})
	}
}

// --- Ensure and DefaultsDir agree on layout ---

func TestEnsureAndDefaultsDir_AgreeOnLayout(t *testing.T) {
	// Given a temp directory used as the XDG base
	base := t.TempDir()
	xdgDataHome := func() string { return base }

	// When DefaultsDir is called and the returned path is joined with each subdir
	defaultsDir := customisations.DefaultsDir(base)
	wantSubdirs := []string{"opencode/config", "opencode/share", "opencode/state"}

	// And Ensure is called
	assertNoError(t, customisations.Ensure(xdgDataHome))

	// Then each path matches the corresponding path that Ensure would create
	// and the on-disk state matches: each subdir exists with 0o700
	for _, sub := range wantSubdirs {
		wantPath := filepath.Join(defaultsDir, sub)
		info, err := os.Stat(wantPath)
		if err != nil {
			t.Fatalf("expected %s to exist: %v", wantPath, err)
		}
		if !info.IsDir() {
			t.Errorf("expected %s to be a directory", wantPath)
		}
		if perm := info.Mode().Perm(); perm != 0o700 {
			t.Errorf("expected %s to have permissions 0o700, got %04o", wantPath, perm)
		}
	}
}

// --- Ensure is safe under concurrent calls ---

func TestEnsure_ConcurrentCalls_AllSucceed(t *testing.T) {
	// Given the injected xdgDataHome returns a fresh temp directory
	base := t.TempDir()
	xdgDataHome := func() string { return base }

	// When Ensure is called from N=8 concurrent goroutines
	const n = 8
	errs := make(chan error, n)
	for range n {
		go func() {
			errs <- customisations.Ensure(xdgDataHome)
		}()
	}

	// Then all N calls return nil
	for range n {
		if err := <-errs; err != nil {
			t.Errorf("concurrent Ensure returned error: %v", err)
		}
	}

	// And the three subdirs exist on the real filesystem with 0o700 permissions
	wantBase := filepath.Join(base, "marshal", "defaults", "opencode")
	for _, sub := range []string{"config", "share", "state"} {
		p := filepath.Join(wantBase, sub)
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("expected %s to exist: %v", p, err)
		}
		if !info.IsDir() {
			t.Errorf("expected %s to be a directory", p)
		}
		if perm := info.Mode().Perm(); perm != 0o700 {
			t.Errorf("expected %s to have permissions 0o700, got %04o", p, perm)
		}
	}
}

// --- HardenSourceTree allows empty source tree ---

func TestHardenSourceTree_EmptySourceTree_NoError(t *testing.T) {
	// Given an empty source tree (no files, no symlinks)
	root := t.TempDir()

	// When the source tree is checked for symlink escapes
	err := customisations.HardenSourceTree(root)

	// Then no error is returned
	if err != nil {
		t.Fatalf("expected no error for empty source tree, got: %v", err)
	}
}

// --- HardenSourceTree detects relative symlink with ".." escaping root ---

func TestHardenSourceTree_DotDotSymlinkEscapingRoot_ReturnsSecurityViolation(t *testing.T) {
	// Given a source tree containing a relative symlink with ".." traversal that escapes the source root
	root := t.TempDir()
	outsideFile := createOutsideFile(t)
	// Create a symlink that uses ".." to escape: root/escape -> ../<outsideDir>/secret.txt
	relTarget, err := filepath.Rel(root, outsideFile)
	if err != nil {
		t.Fatal(err)
	}
	symlinkPath := filepath.Join(root, "escape")
	if err := os.Symlink(relTarget, symlinkPath); err != nil {
		t.Fatal(err)
	}

	// When the source tree is checked for symlink escapes
	err = customisations.HardenSourceTree(root)

	// Then an error is returned containing "security violation"
	if err == nil {
		t.Fatal("expected an error for symlink with .. escaping source root, got nil")
	}
	if !strings.Contains(err.Error(), "security violation") {
		t.Errorf("error %q should contain %q", err.Error(), "security violation")
	}
}

// --- HardenSourceTree detects symlink chain escaping root ---

func TestHardenSourceTree_SymlinkChainEscapingRoot_ReturnsSecurityViolation(t *testing.T) {
	// Given a source tree containing a symlink chain (A -> B -> outside)
	root := t.TempDir()
	outsideFile := createOutsideFile(t)
	// B points outside
	linkB := filepath.Join(root, "linkB")
	if err := os.Symlink(outsideFile, linkB); err != nil {
		t.Fatal(err)
	}
	// A points to B (chain: A -> B -> outside)
	linkA := filepath.Join(root, "linkA")
	if err := os.Symlink("linkB", linkA); err != nil {
		t.Fatal(err)
	}

	// When the source tree is checked for symlink escapes
	err := customisations.HardenSourceTree(root)

	// Then an error is returned containing "security violation"
	if err == nil {
		t.Fatal("expected an error for symlink chain escaping source root, got nil")
	}
	if !strings.Contains(err.Error(), "security violation") {
		t.Errorf("error %q should contain %q", err.Error(), "security violation")
	}
}

// --- HardenSourceTree allows broken symlink ---

func TestHardenSourceTree_BrokenSymlink_NoError(t *testing.T) {
	// Given a source tree containing a broken symlink (target does not exist)
	root := t.TempDir()
	symlinkPath := filepath.Join(root, "broken")
	if err := os.Symlink("/nonexistent/target", symlinkPath); err != nil {
		t.Fatal(err)
	}

	// When the source tree is checked for symlink escapes
	err := customisations.HardenSourceTree(root)

	// Then no error is returned
	if err != nil {
		t.Fatalf("expected no error for broken symlink, got: %v", err)
	}
}

// --- HardenSourceTree allows relative symlink staying inside root ---

func TestHardenSourceTree_RelativeSymlinkInsideRoot_NoError(t *testing.T) {
	// Given a source tree containing a relative symlink whose target stays inside the source root
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

	// When the source tree is checked for symlink escapes
	err := customisations.HardenSourceTree(root)

	// Then no error is returned
	if err != nil {
		t.Fatalf("expected no error for symlink inside root, got: %v", err)
	}
}

// --- HardenSourceTree detects symlink escaping source root ---

func TestHardenSourceTree_SymlinkEscapingRoot_ReturnsSecurityViolation(t *testing.T) {
	// Given a source tree containing a symlink whose resolved target falls outside the source root
	root := t.TempDir()
	outsideFile := createOutsideFile(t)
	symlinkPath := filepath.Join(root, "escape")
	if err := os.Symlink(outsideFile, symlinkPath); err != nil {
		t.Fatal(err)
	}

	// When the source tree is checked for symlink escapes
	err := customisations.HardenSourceTree(root)

	// Then an error is returned containing "security violation", the offending path, and the resolved target
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

func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

// createOutsideFile creates a temp directory with a file and returns its path.
func createOutsideFile(t *testing.T) string {
	t.Helper()
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	return outsideFile
}
