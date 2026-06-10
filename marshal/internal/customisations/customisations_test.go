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

// --- CopyDefaults copies a single file into empty destination ---

func TestCopyDefaults_SingleFile_CopiesToEmptyDestination(t *testing.T) {
	// Given a source tree containing config/settings.json
	src := t.TempDir()
	wantContent := []byte(`{"theme":"dark"}`)
	writeTestFile(t, filepath.Join(src, "config", "settings.json"), wantContent)

	// And an empty destination directory
	dst := t.TempDir()

	// When the copy function is called with source and destination
	assertNoError(t, customisations.CopyDefaults(src, dst))

	// Then config/settings.json exists in the destination with the same content
	gotContent, err := os.ReadFile(filepath.Join(dst, "config", "settings.json"))
	if err != nil {
		t.Fatalf("expected file to exist in destination: %v", err)
	}
	if string(gotContent) != string(wantContent) {
		t.Errorf("file content mismatch: got %q, want %q", gotContent, wantContent)
	}
}

// --- CopyDefaults never overwrites existing destination file ---

func TestCopyDefaults_ExistingFile_NeverOverwrites(t *testing.T) {
	// Given a source tree containing config/settings.json
	src := t.TempDir()
	srcContent := []byte(`{"theme":"light"}`)
	writeTestFile(t, filepath.Join(src, "config", "settings.json"), srcContent)

	// And the destination already contains config/settings.json with different content
	dst := t.TempDir()
	dstContent := []byte(`{"theme":"dark"}`)
	dstFile := filepath.Join(dst, "config", "settings.json")
	writeTestFile(t, dstFile, dstContent)

	// When the copy function is called with source and destination
	assertNoError(t, customisations.CopyDefaults(src, dst))

	// Then the destination file is unchanged
	gotContent, err := os.ReadFile(dstFile)
	if err != nil {
		t.Fatalf("expected to read destination file: %v", err)
	}
	if string(gotContent) != string(dstContent) {
		t.Errorf("destination file was overwritten: got %q, want %q", gotContent, dstContent)
	}
}

// --- CopyDefaults copies nested subdir file to empty destination ---

func TestCopyDefaults_NestedSubdir_CopiesToEmptyDestination(t *testing.T) {
	// Given a source tree containing config/subdir/nested.json
	src := t.TempDir()
	wantContent := []byte(`{"key":"value"}`)
	writeTestFile(t, filepath.Join(src, "config", "subdir", "nested.json"), wantContent)

	// And an empty destination directory
	dst := t.TempDir()

	// When the copy function is called with source and destination
	assertNoError(t, customisations.CopyDefaults(src, dst))

	// Then config/subdir/nested.json exists in the destination with the same content
	gotContent, err := os.ReadFile(filepath.Join(dst, "config", "subdir", "nested.json"))
	if err != nil {
		t.Fatalf("expected file to exist in destination: %v", err)
	}
	if string(gotContent) != string(wantContent) {
		t.Errorf("file content mismatch: got %q, want %q", gotContent, wantContent)
	}
}

// --- CopyDefaults copies multiple files from different subdirs ---

func TestCopyDefaults_MultipleSubdirs_CopiesAllFiles(t *testing.T) {
	// Given a source tree containing config/a.json and share/b.json
	src := t.TempDir()
	aContent := []byte(`{"a":1}`)
	bContent := []byte(`{"b":2}`)
	writeTestFile(t, filepath.Join(src, "config", "a.json"), aContent)
	writeTestFile(t, filepath.Join(src, "share", "b.json"), bContent)

	// And an empty destination directory
	dst := t.TempDir()

	// When the copy function is called with source and destination
	assertNoError(t, customisations.CopyDefaults(src, dst))

	// Then both files exist in the destination
	gotA, err := os.ReadFile(filepath.Join(dst, "config", "a.json"))
	if err != nil {
		t.Fatalf("expected config/a.json to exist: %v", err)
	}
	if string(gotA) != string(aContent) {
		t.Errorf("config/a.json content mismatch: got %q, want %q", gotA, aContent)
	}
	gotB, err := os.ReadFile(filepath.Join(dst, "share", "b.json"))
	if err != nil {
		t.Fatalf("expected share/b.json to exist: %v", err)
	}
	if string(gotB) != string(bContent) {
		t.Errorf("share/b.json content mismatch: got %q, want %q", gotB, bContent)
	}
}

// --- CopyDefaults with empty source is a no-op ---

func TestCopyDefaults_EmptySource_DestinationUnchanged(t *testing.T) {
	// Given an empty source tree (no files)
	src := t.TempDir()

	// And a destination directory with pre-existing files
	dst := t.TempDir()
	dstFile := filepath.Join(dst, "existing.txt")
	dstContent := []byte("keep me")
	assertNoError(t, os.WriteFile(dstFile, dstContent, 0o644))

	// When the copy function is called
	assertNoError(t, customisations.CopyDefaults(src, dst))

	// Then the destination directory is unchanged
	gotContent, err := os.ReadFile(dstFile)
	if err != nil {
		t.Fatalf("expected existing file to still exist: %v", err)
	}
	if string(gotContent) != string(dstContent) {
		t.Errorf("existing file was modified: got %q, want %q", gotContent, dstContent)
	}
}

// --- CopyDefaults never deletes pre-existing destination files ---

func TestCopyDefaults_PreExistingFiles_NotDeleted(t *testing.T) {
	// Given a source tree containing config/a.json
	src := t.TempDir()
	srcContent := []byte(`{"src":"yes"}`)
	writeTestFile(t, filepath.Join(src, "config", "a.json"), srcContent)

	// And a destination with pre-existing files not in the source
	dst := t.TempDir()
	unrelatedContent := []byte("do not delete me")
	unrelatedFile := filepath.Join(dst, "unrelated.txt")
	assertNoError(t, os.WriteFile(unrelatedFile, unrelatedContent, 0o644))

	// When the copy function is called
	assertNoError(t, customisations.CopyDefaults(src, dst))

	// Then the pre-existing destination files are left untouched
	gotContent, err := os.ReadFile(unrelatedFile)
	if err != nil {
		t.Fatalf("expected unrelated file to still exist: %v", err)
	}
	if string(gotContent) != string(unrelatedContent) {
		t.Errorf("unrelated file was modified: got %q, want %q", gotContent, unrelatedContent)
	}

	// And the source file was copied
	gotSrc, err := os.ReadFile(filepath.Join(dst, "config", "a.json"))
	if err != nil {
		t.Fatalf("expected source file to exist in destination: %v", err)
	}
	if string(gotSrc) != string(srcContent) {
		t.Errorf("source file content mismatch: got %q, want %q", gotSrc, srcContent)
	}
}

// --- CopyDefaults copies empty directory from source to destination ---

func TestCopyDefaults_EmptyDirectory_CopiesToDestination(t *testing.T) {
	// Given a source tree containing an empty directory config/empty/
	src := t.TempDir()
	emptyDir := filepath.Join(src, "config", "empty")
	assertNoError(t, os.MkdirAll(emptyDir, 0o700))

	// And an empty destination directory
	dst := t.TempDir()

	// When the copy function is called with source and destination
	assertNoError(t, customisations.CopyDefaults(src, dst))

	// Then config/empty/ exists in the destination as a directory
	dstEmptyDir := filepath.Join(dst, "config", "empty")
	info, err := os.Stat(dstEmptyDir)
	if err != nil {
		t.Fatalf("expected config/empty/ to exist in destination: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("expected config/empty/ to be a directory, got %v", info.Mode())
	}
}

// --- CopyDefaults preserves relative symlink to file ---

func TestCopyDefaults_SymlinkToFile_PreservesSymlink(t *testing.T) {
	// Given a source tree containing a relative symlink config/link.json -> real.json
	src := t.TempDir()
	realContent := []byte(`{"real":"data"}`)
	realFile := filepath.Join(src, "config", "real.json")
	writeTestFile(t, realFile, realContent)
	symlinkPath := filepath.Join(src, "config", "link.json")
	assertNoError(t, os.Symlink("real.json", symlinkPath))

	// And an empty destination directory
	dst := t.TempDir()

	// When CopyDefaults copies the tree to the destination
	assertNoError(t, customisations.CopyDefaults(src, dst))

	// Then config/link.json at the destination is a symlink (not a regular file)
	dstLink := filepath.Join(dst, "config", "link.json")
	info, err := os.Lstat(dstLink)
	if err != nil {
		t.Fatalf("expected link.json to exist in destination: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected link.json to be a symlink, got mode %v", info.Mode())
	}

	// And os.Readlink on the destination symlink returns "real.json"
	gotTarget, err := os.Readlink(dstLink)
	if err != nil {
		t.Fatalf("expected to read symlink target: %v", err)
	}
	if gotTarget != "real.json" {
		t.Errorf("symlink target = %q, want %q", gotTarget, "real.json")
	}

	// And reading through the destination symlink produces the same content as source real.json
	gotContent, err := os.ReadFile(dstLink)
	if err != nil {
		t.Fatalf("expected to read through symlink: %v", err)
	}
	if string(gotContent) != string(realContent) {
		t.Errorf("content through symlink = %q, want %q", gotContent, realContent)
	}
}

// --- CopyDefaults preserves relative symlink to directory ---

func TestCopyDefaults_SymlinkToDir_PreservesSymlink(t *testing.T) {
	// Given a source tree containing a relative symlink config/subdir/link -> sibling
	src := t.TempDir()
	siblingDir := filepath.Join(src, "config", "subdir", "sibling")
	assertNoError(t, os.MkdirAll(siblingDir, 0o700))
	symlinkPath := filepath.Join(src, "config", "subdir", "link")
	assertNoError(t, os.Symlink("sibling", symlinkPath))

	// And an empty destination directory
	dst := t.TempDir()

	// When CopyDefaults copies the tree to the destination
	assertNoError(t, customisations.CopyDefaults(src, dst))

	// Then config/subdir/link at the destination is a symlink
	dstLink := filepath.Join(dst, "config", "subdir", "link")
	info, err := os.Lstat(dstLink)
	if err != nil {
		t.Fatalf("expected link to exist in destination: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected link to be a symlink, got mode %v", info.Mode())
	}

	// And os.Readlink on the destination symlink returns "sibling"
	gotTarget, err := os.Readlink(dstLink)
	if err != nil {
		t.Fatalf("expected to read symlink target: %v", err)
	}
	if gotTarget != "sibling" {
		t.Errorf("symlink target = %q, want %q", gotTarget, "sibling")
	}

	// And resolving the destination symlink yields a directory
	resolved, err := filepath.EvalSymlinks(dstLink)
	if err != nil {
		t.Fatalf("expected to resolve symlink: %v", err)
	}
	resolvedInfo, err := os.Stat(resolved)
	if err != nil {
		t.Fatalf("expected resolved path to exist: %v", err)
	}
	if !resolvedInfo.IsDir() {
		t.Errorf("resolved symlink should be a directory, got %v", resolvedInfo.Mode())
	}
}

// --- CopyDefaults preserves broken relative symlink ---

func TestCopyDefaults_BrokenSymlink_PreservesSymlink(t *testing.T) {
	// Given a source tree containing a broken relative symlink config/dead -> nonexistent
	src := t.TempDir()
	assertNoError(t, os.MkdirAll(filepath.Join(src, "config"), 0o700))
	symlinkPath := filepath.Join(src, "config", "dead")
	assertNoError(t, os.Symlink("nonexistent", symlinkPath))

	// And an empty destination directory
	dst := t.TempDir()

	// When CopyDefaults copies the tree to the destination
	assertNoError(t, customisations.CopyDefaults(src, dst))

	// Then config/dead at the destination is a symlink
	dstLink := filepath.Join(dst, "config", "dead")
	info, err := os.Lstat(dstLink)
	if err != nil {
		t.Fatalf("expected dead to exist in destination: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected dead to be a symlink, got mode %v", info.Mode())
	}

	// And os.Readlink on the destination symlink returns "nonexistent"
	gotTarget, err := os.Readlink(dstLink)
	if err != nil {
		t.Fatalf("expected to read symlink target: %v", err)
	}
	if gotTarget != "nonexistent" {
		t.Errorf("symlink target = %q, want %q", gotTarget, "nonexistent")
	}

	// And os.Stat on the destination symlink returns a "not exist" error
	_, err = os.Stat(dstLink)
	if err == nil {
		t.Fatal("expected Stat on broken symlink to return error, got nil")
	}
	if !os.IsNotExist(err) {
		t.Errorf("expected IsNotExist error, got: %v", err)
	}
}

// --- CopyDefaults leaves existing regular file untouched when source has symlink ---

func TestCopyDefaults_ExistingFile_NotOverwrittenBySymlink(t *testing.T) {
	// Given a source tree containing a relative symlink config/link.json -> real.json
	src := t.TempDir()
	realContent := []byte(`{"real":"data"}`)
	writeTestFile(t, filepath.Join(src, "config", "real.json"), realContent)
	symlinkPath := filepath.Join(src, "config", "link.json")
	assertNoError(t, os.Symlink("real.json", symlinkPath))

	// And the destination already contains a regular file config/link.json with different content
	dst := t.TempDir()
	dstContent := []byte(`{"existing":"file"}`)
	dstFile := filepath.Join(dst, "config", "link.json")
	writeTestFile(t, dstFile, dstContent)

	// When CopyDefaults copies the tree to the destination
	assertNoError(t, customisations.CopyDefaults(src, dst))

	// Then the existing config/link.json at the destination is left untouched
	info, err := os.Lstat(dstFile)
	if err != nil {
		t.Fatalf("expected link.json to exist in destination: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("expected link.json to remain a regular file, got symlink")
	}
	gotContent, err := os.ReadFile(dstFile)
	if err != nil {
		t.Fatalf("expected to read destination file: %v", err)
	}
	if string(gotContent) != string(dstContent) {
		t.Errorf("destination file was overwritten: got %q, want %q", gotContent, dstContent)
	}

	// And the source real.json was still copied to the destination
	gotReal, err := os.ReadFile(filepath.Join(dst, "config", "real.json"))
	if err != nil {
		t.Fatalf("expected real.json to exist in destination: %v", err)
	}
	if string(gotReal) != string(realContent) {
		t.Errorf("real.json content mismatch: got %q, want %q", gotReal, realContent)
	}
}

// --- CopyDefaults leaves existing symlink untouched when source has symlink ---

func TestCopyDefaults_ExistingSymlink_NotOverwrittenBySymlink(t *testing.T) {
	// Given a source tree containing a relative symlink config/link.json -> real.json
	src := t.TempDir()
	realContent := []byte(`{"real":"data"}`)
	writeTestFile(t, filepath.Join(src, "config", "real.json"), realContent)
	symlinkPath := filepath.Join(src, "config", "link.json")
	assertNoError(t, os.Symlink("real.json", symlinkPath))

	// And the destination already contains a symlink config/link.json pointing to a different target
	dst := t.TempDir()
	assertNoError(t, os.MkdirAll(filepath.Join(dst, "config"), 0o700))
	dstLink := filepath.Join(dst, "config", "link.json")
	assertNoError(t, os.Symlink("other-target", dstLink))

	// When CopyDefaults copies the tree to the destination
	assertNoError(t, customisations.CopyDefaults(src, dst))

	// Then the existing config/link.json at the destination is left untouched
	gotTarget, err := os.Readlink(dstLink)
	if err != nil {
		t.Fatalf("expected to read destination symlink: %v", err)
	}
	if gotTarget != "other-target" {
		t.Errorf("destination symlink was changed: got target %q, want %q", gotTarget, "other-target")
	}

	// And the source real.json was still copied to the destination
	gotReal, err := os.ReadFile(filepath.Join(dst, "config", "real.json"))
	if err != nil {
		t.Fatalf("expected real.json to exist in destination: %v", err)
	}
	if string(gotReal) != string(realContent) {
		t.Errorf("real.json content mismatch: got %q, want %q", gotReal, realContent)
	}
}

// --- CopyDefaults converts absolute symlink to relative when copying ---

func TestCopyDefaults_AbsoluteSymlinkToFile_ConvertedToRelative(t *testing.T) {
	// Given a source defaults tree at src containing a file config/z/y with content
	// and an absolute symlink config/x -> <src>/config/z/y
	src := t.TempDir()
	wantContent := []byte("hello")
	writeTestFile(t, filepath.Join(src, "config", "z", "y"), wantContent)
	symlinkPath := filepath.Join(src, "config", "x")
	assertNoError(t, os.Symlink(filepath.Join(src, "config", "z", "y"), symlinkPath))

	// And an empty destination directory
	dst := t.TempDir()

	// When CopyDefaults copies the tree to the destination
	assertNoError(t, customisations.CopyDefaults(src, dst))

	// Then config/x at the destination is a symlink
	dstLink := filepath.Join(dst, "config", "x")
	info, err := os.Lstat(dstLink)
	if err != nil {
		t.Fatalf("expected config/x to exist in destination: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected config/x to be a symlink, got mode %v", info.Mode())
	}

	// And os.Readlink on the destination symlink returns "z/y"
	gotTarget, err := os.Readlink(dstLink)
	if err != nil {
		t.Fatalf("expected to read symlink target: %v", err)
	}
	if gotTarget != "z/y" {
		t.Errorf("symlink target = %q, want %q", gotTarget, "z/y")
	}

	// And resolving the destination symlink yields the same content as the source target
	gotContent, err := os.ReadFile(dstLink)
	if err != nil {
		t.Fatalf("expected to read through symlink: %v", err)
	}
	if string(gotContent) != string(wantContent) {
		t.Errorf("content through symlink = %q, want %q", gotContent, wantContent)
	}
}

func TestCopyDefaults_AbsoluteSymlinkToDir_ConvertedToRelative(t *testing.T) {
	// Given a source defaults tree at src containing a directory config/targetdir
	// and an absolute symlink config/link -> <src>/config/targetdir
	src := t.TempDir()
	targetDir := filepath.Join(src, "config", "targetdir")
	assertNoError(t, os.MkdirAll(targetDir, 0o700))
	symlinkPath := filepath.Join(src, "config", "link")
	assertNoError(t, os.Symlink(targetDir, symlinkPath))

	// And an empty destination directory
	dst := t.TempDir()

	// When CopyDefaults copies the tree to the destination
	assertNoError(t, customisations.CopyDefaults(src, dst))

	// Then config/link at the destination is a symlink
	dstLink := filepath.Join(dst, "config", "link")
	info, err := os.Lstat(dstLink)
	if err != nil {
		t.Fatalf("expected config/link to exist in destination: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected config/link to be a symlink, got mode %v", info.Mode())
	}

	// And os.Readlink on the destination symlink returns "targetdir"
	gotTarget, err := os.Readlink(dstLink)
	if err != nil {
		t.Fatalf("expected to read symlink target: %v", err)
	}
	if gotTarget != "targetdir" {
		t.Errorf("symlink target = %q, want %q", gotTarget, "targetdir")
	}

	// And resolving the destination symlink yields a directory
	resolved, err := filepath.EvalSymlinks(dstLink)
	if err != nil {
		t.Fatalf("expected to resolve symlink: %v", err)
	}
	resolvedInfo, err := os.Stat(resolved)
	if err != nil {
		t.Fatalf("expected resolved path to exist: %v", err)
	}
	if !resolvedInfo.IsDir() {
		t.Errorf("resolved symlink should be a directory, got %v", resolvedInfo.Mode())
	}
}

func TestCopyDefaults_AbsoluteSymlinkDeepNested_ConvertedToRelative(t *testing.T) {
	// Given a source defaults tree at src containing a file config/shallow/target.txt
	// and an absolute symlink config/deep/nested/link -> <src>/config/shallow/target.txt
	src := t.TempDir()
	wantContent := []byte("nested content")
	writeTestFile(t, filepath.Join(src, "config", "shallow", "target.txt"), wantContent)
	symlinkPath := filepath.Join(src, "config", "deep", "nested", "link")
	assertNoError(t, os.MkdirAll(filepath.Dir(symlinkPath), 0o700))
	assertNoError(t, os.Symlink(filepath.Join(src, "config", "shallow", "target.txt"), symlinkPath))

	// And an empty destination directory
	dst := t.TempDir()

	// When CopyDefaults copies the tree to the destination
	assertNoError(t, customisations.CopyDefaults(src, dst))

	// Then config/deep/nested/link at the destination is a symlink
	dstLink := filepath.Join(dst, "config", "deep", "nested", "link")
	info, err := os.Lstat(dstLink)
	if err != nil {
		t.Fatalf("expected config/deep/nested/link to exist in destination: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected config/deep/nested/link to be a symlink, got mode %v", info.Mode())
	}

	// And os.Readlink on the destination symlink returns "../../shallow/target.txt"
	gotTarget, err := os.Readlink(dstLink)
	if err != nil {
		t.Fatalf("expected to read symlink target: %v", err)
	}
	if gotTarget != "../../shallow/target.txt" {
		t.Errorf("symlink target = %q, want %q", gotTarget, "../../shallow/target.txt")
	}

	// And the relative path correctly traverses from the symlink's location to the target
	gotContent, err := os.ReadFile(dstLink)
	if err != nil {
		t.Fatalf("expected to read through symlink: %v", err)
	}
	if string(gotContent) != string(wantContent) {
		t.Errorf("content through symlink = %q, want %q", gotContent, wantContent)
	}
}

func TestCopyDefaults_AbsoluteSymlinkSibling_ConvertedToRelative(t *testing.T) {
	// Given a source defaults tree at src containing a file config/y
	// and an absolute symlink config/x -> <src>/config/y where x and y are siblings
	src := t.TempDir()
	wantContent := []byte("sibling content")
	writeTestFile(t, filepath.Join(src, "config", "y"), wantContent)
	symlinkPath := filepath.Join(src, "config", "x")
	assertNoError(t, os.Symlink(filepath.Join(src, "config", "y"), symlinkPath))

	// And an empty destination directory
	dst := t.TempDir()

	// When CopyDefaults copies the tree to the destination
	assertNoError(t, customisations.CopyDefaults(src, dst))

	// Then config/x at the destination is a symlink
	dstLink := filepath.Join(dst, "config", "x")
	info, err := os.Lstat(dstLink)
	if err != nil {
		t.Fatalf("expected config/x to exist in destination: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected config/x to be a symlink, got mode %v", info.Mode())
	}

	// And os.Readlink on the destination symlink returns "y"
	gotTarget, err := os.Readlink(dstLink)
	if err != nil {
		t.Fatalf("expected to read symlink target: %v", err)
	}
	if gotTarget != "y" {
		t.Errorf("symlink target = %q, want %q", gotTarget, "y")
	}

	// And resolving the destination symlink yields the same content as the source target
	gotContent, err := os.ReadFile(dstLink)
	if err != nil {
		t.Fatalf("expected to read through symlink: %v", err)
	}
	if string(gotContent) != string(wantContent) {
		t.Errorf("content through symlink = %q, want %q", gotContent, wantContent)
	}
}

// --- CopyDefaults leaves existing dangling symlink untouched when source has regular file ---

func TestCopyDefaults_DanglingSymlinkNeverOverwrites(t *testing.T) {
	// Given a source defaults tree containing a regular file config/link.json
	src := t.TempDir()
	wantContent := []byte(`{"real":"data"}`)
	writeTestFile(t, filepath.Join(src, "config", "link.json"), wantContent)

	// And the destination already contains a dangling symlink config/link.json
	dst := t.TempDir()
	assertNoError(t, os.MkdirAll(filepath.Join(dst, "config"), 0o700))
	dstLink := filepath.Join(dst, "config", "link.json")
	assertNoError(t, os.Symlink("/nonexistent/target", dstLink))

	// When CopyDefaults copies the tree to the destination
	err := customisations.CopyDefaults(src, dst)

	// Then no error is returned
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// And the existing dangling symlink at the destination is left untouched
	info, err := os.Lstat(dstLink)
	if err != nil {
		t.Fatalf("expected dangling symlink to exist: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected config/link.json to remain a symlink, got mode %v", info.Mode())
	}
	gotTarget, err := os.Readlink(dstLink)
	if err != nil {
		t.Fatalf("expected to read symlink target: %v", err)
	}
	if gotTarget != "/nonexistent/target" {
		t.Errorf("symlink target changed: got %q, want %q", gotTarget, "/nonexistent/target")
	}
}

func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

// writeTestFile creates parent directories and writes content to path.
func writeTestFile(t *testing.T, path string, content []byte) {
	t.Helper()
	assertNoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	assertNoError(t, os.WriteFile(path, content, 0o644))
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
