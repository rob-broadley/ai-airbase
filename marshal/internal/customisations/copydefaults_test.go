// SPDX-License-Identifier: AGPL-3.0-or-later
package customisations_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/customisations"
)

func TestCopyDefaults_SingleFile_CopiesToEmptyDestination(t *testing.T) {
	defaultsDir := t.TempDir()
	wantContent := []byte(`{"theme":"dark"}`)
	// Given a defaultsDir containing opencode/config/settings.json
	writeTestFile(t, filepath.Join(defaultsDir, "opencode", "config", "settings.json"), wantContent)

	// And an empty destination directory
	dst := t.TempDir()

	// When CopyDefaults is called with defaultsDir and dst
	assertNoError(t, customisations.CopyDefaults(defaultsDir, dst))

	// Then dst/opencode/config/settings.json exists with the same content
	gotContent, err := os.ReadFile(filepath.Join(dst, "opencode", "config", "settings.json"))
	if err != nil {
		t.Fatalf("expected file to exist in destination: %v", err)
	}
	if string(gotContent) != string(wantContent) {
		t.Errorf("file content mismatch: got %q, want %q", gotContent, wantContent)
	}
}

func TestCopyDefaults_ExistingFile_NeverOverwrites(t *testing.T) {
	defaultsDir := t.TempDir()
	srcContent := []byte(`{"theme":"light"}`)
	// Given a defaultsDir containing opencode/config/settings.json
	writeTestFile(t, filepath.Join(defaultsDir, "opencode", "config", "settings.json"), srcContent)

	// And the destination already contains opencode/config/settings.json with different content
	dst := t.TempDir()
	dstContent := []byte(`{"theme":"dark"}`)
	dstFile := filepath.Join(dst, "opencode", "config", "settings.json")
	writeTestFile(t, dstFile, dstContent)

	// When CopyDefaults is called
	assertNoError(t, customisations.CopyDefaults(defaultsDir, dst))

	// Then the destination file is unchanged
	gotContent, err := os.ReadFile(dstFile)
	if err != nil {
		t.Fatalf("expected to read destination file: %v", err)
	}
	if string(gotContent) != string(dstContent) {
		t.Errorf("destination file was overwritten: got %q, want %q", gotContent, dstContent)
	}
}

func TestCopyDefaults_NestedSubdir_CopiesToEmptyDestination(t *testing.T) {
	defaultsDir := t.TempDir()
	wantContent := []byte(`{"key":"value"}`)
	// Given a defaultsDir containing opencode/config/subdir/nested.json
	writeTestFile(t, filepath.Join(defaultsDir, "opencode", "config", "subdir", "nested.json"), wantContent)

	// And an empty destination directory
	dst := t.TempDir()

	// When CopyDefaults is called
	assertNoError(t, customisations.CopyDefaults(defaultsDir, dst))

	// Then dst/opencode/config/subdir/nested.json exists with the same content
	gotContent, err := os.ReadFile(filepath.Join(dst, "opencode", "config", "subdir", "nested.json"))
	if err != nil {
		t.Fatalf("expected file to exist in destination: %v", err)
	}
	if string(gotContent) != string(wantContent) {
		t.Errorf("file content mismatch: got %q, want %q", gotContent, wantContent)
	}
}

func TestCopyDefaults_MultipleSubdirs_CopiesAllFiles(t *testing.T) {
	defaultsDir := t.TempDir()
	aContent := []byte(`{"a":1}`)
	bContent := []byte(`{"b":2}`)
	// Given a defaultsDir containing opencode/config/a.json and opencode/share/b.json
	writeTestFile(t, filepath.Join(defaultsDir, "opencode", "config", "a.json"), aContent)
	writeTestFile(t, filepath.Join(defaultsDir, "opencode", "share", "b.json"), bContent)

	// And an empty destination directory
	dst := t.TempDir()

	// When CopyDefaults is called
	assertNoError(t, customisations.CopyDefaults(defaultsDir, dst))

	// Then both files exist in the destination
	gotA, err := os.ReadFile(filepath.Join(dst, "opencode", "config", "a.json"))
	if err != nil {
		t.Fatalf("expected opencode/config/a.json to exist: %v", err)
	}
	if string(gotA) != string(aContent) {
		t.Errorf("opencode/config/a.json content mismatch: got %q, want %q", gotA, aContent)
	}
	// And opencode/share/b.json exists with the correct content
	gotB, err := os.ReadFile(filepath.Join(dst, "opencode", "share", "b.json"))
	if err != nil {
		t.Fatalf("expected opencode/share/b.json to exist: %v", err)
	}
	if string(gotB) != string(bContent) {
		t.Errorf("opencode/share/b.json content mismatch: got %q, want %q", gotB, bContent)
	}
}

func TestCopyDefaults_EmptySource_DestinationUnchanged(t *testing.T) {
	// Given an empty defaultsDir
	defaultsDir := t.TempDir()

	// And a destination directory with pre-existing files
	dst := t.TempDir()
	dstFile := filepath.Join(dst, "existing.txt")
	dstContent := []byte("keep me")
	assertNoError(t, os.WriteFile(dstFile, dstContent, 0o644))

	// When CopyDefaults is called
	assertNoError(t, customisations.CopyDefaults(defaultsDir, dst))

	// Then the destination directory is unchanged
	gotContent, err := os.ReadFile(dstFile)
	if err != nil {
		t.Fatalf("expected existing file to still exist: %v", err)
	}
	if string(gotContent) != string(dstContent) {
		t.Errorf("existing file was modified: got %q, want %q", gotContent, dstContent)
	}
}

func TestCopyDefaults_PreExistingFiles_NotDeleted(t *testing.T) {
	defaultsDir := t.TempDir()
	srcContent := []byte(`{"src":"yes"}`)
	// Given a defaultsDir containing opencode/config/a.json
	writeTestFile(t, filepath.Join(defaultsDir, "opencode", "config", "a.json"), srcContent)

	// And a destination with pre-existing files not in the source
	dst := t.TempDir()
	unrelatedContent := []byte("do not delete me")
	unrelatedFile := filepath.Join(dst, "unrelated.txt")
	assertNoError(t, os.WriteFile(unrelatedFile, unrelatedContent, 0o644))

	// When CopyDefaults is called
	assertNoError(t, customisations.CopyDefaults(defaultsDir, dst))

	// Then the pre-existing destination files are left untouched
	gotContent, err := os.ReadFile(unrelatedFile)
	if err != nil {
		t.Fatalf("expected unrelated file to still exist: %v", err)
	}
	if string(gotContent) != string(unrelatedContent) {
		t.Errorf("unrelated file was modified: got %q, want %q", gotContent, unrelatedContent)
	}

	// And the source file was copied
	gotSrc, err := os.ReadFile(filepath.Join(dst, "opencode", "config", "a.json"))
	if err != nil {
		t.Fatalf("expected source file to exist in destination: %v", err)
	}
	if string(gotSrc) != string(srcContent) {
		t.Errorf("source file content mismatch: got %q, want %q", gotSrc, srcContent)
	}
}

func TestCopyDefaults_EmptyDirectory_CopiesToDestination(t *testing.T) {
	// Given a defaultsDir containing an empty directory opencode/config/empty/
	defaultsDir := t.TempDir()
	emptyDir := filepath.Join(defaultsDir, "opencode", "config", "empty")
	assertNoError(t, os.MkdirAll(emptyDir, 0o700))

	// And an empty destination directory
	dst := t.TempDir()

	// When CopyDefaults copies the tree to the destination
	assertNoError(t, customisations.CopyDefaults(defaultsDir, dst))

	// Then dst/opencode/config/empty/ exists in the destination as a directory
	dstEmptyDir := filepath.Join(dst, "opencode", "config", "empty")
	info, err := os.Stat(dstEmptyDir)
	if err != nil {
		t.Fatalf("expected dst/opencode/config/empty/ to exist in destination: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("expected dst/opencode/config/empty/ to be a directory, got %v", info.Mode())
	}
}

func TestCopyDefaults_SymlinkToFile_PreservesSymlink(t *testing.T) {
	// Given a defaultsDir containing a relative symlink opencode/config/link.json -> real.json
	defaultsDir := t.TempDir()
	realContent := []byte(`{"real":"data"}`)
	realFile := filepath.Join(defaultsDir, "opencode", "config", "real.json")
	writeTestFile(t, realFile, realContent)
	symlinkPath := filepath.Join(defaultsDir, "opencode", "config", "link.json")
	assertNoError(t, os.Symlink("real.json", symlinkPath))

	// And an empty destination directory
	dst := t.TempDir()

	// When CopyDefaults copies the tree to the destination
	assertNoError(t, customisations.CopyDefaults(defaultsDir, dst))

	// Then opencode/config/link.json at the destination is a symlink
	dstLink := filepath.Join(dst, "opencode", "config", "link.json")
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

func TestCopyDefaults_SymlinkToDir_PreservesSymlink(t *testing.T) {
	// Given a defaultsDir containing a relative symlink opencode/config/subdir/link -> sibling
	defaultsDir := t.TempDir()
	siblingDir := filepath.Join(defaultsDir, "opencode", "config", "subdir", "sibling")
	assertNoError(t, os.MkdirAll(siblingDir, 0o700))
	symlinkPath := filepath.Join(defaultsDir, "opencode", "config", "subdir", "link")
	assertNoError(t, os.Symlink("sibling", symlinkPath))

	// And an empty destination directory
	dst := t.TempDir()

	// When CopyDefaults copies the tree to the destination
	assertNoError(t, customisations.CopyDefaults(defaultsDir, dst))

	// Then opencode/config/subdir/link at the destination is a symlink
	dstLink := filepath.Join(dst, "opencode", "config", "subdir", "link")
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

func TestCopyDefaults_BrokenSymlink_PreservesSymlink(t *testing.T) {
	// Given a defaultsDir containing a broken relative symlink opencode/config/dead -> nonexistent
	defaultsDir := t.TempDir()
	assertNoError(t, os.MkdirAll(filepath.Join(defaultsDir, "opencode", "config"), 0o700))
	symlinkPath := filepath.Join(defaultsDir, "opencode", "config", "dead")
	assertNoError(t, os.Symlink("nonexistent", symlinkPath))

	// And an empty destination directory
	dst := t.TempDir()

	// When CopyDefaults copies the tree to the destination
	assertNoError(t, customisations.CopyDefaults(defaultsDir, dst))

	// Then opencode/config/dead at the destination is a symlink
	dstLink := filepath.Join(dst, "opencode", "config", "dead")
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

func TestCopyDefaults_BrokenAbsoluteSymlinkOutsideRoot_ReturnsSecurityViolation(t *testing.T) {
	// Given a defaults tree containing a broken absolute symlink inside opencode/config
	// whose raw target is outside the source root
	defaultsDir := t.TempDir()
	assertNoError(t, os.MkdirAll(filepath.Join(defaultsDir, "opencode", "config"), 0o700))
	symlinkPath := filepath.Join(defaultsDir, "opencode", "config", "escape")
	assertNoError(t, os.Symlink("/nonexistent/outside", symlinkPath))

	// And an empty destination directory
	dst := t.TempDir()

	// When CopyDefaults is called
	err := customisations.CopyDefaults(defaultsDir, dst)

	// Then it returns an error containing "security violation"
	if err == nil {
		t.Fatal("expected error for broken absolute symlink outside source root, got nil")
	}
	if !strings.Contains(err.Error(), "security violation") {
		t.Errorf("expected error to contain %q, got: %q", "security violation", err.Error())
	}
}

func TestCopyDefaults_ExistingFile_NotOverwrittenBySymlink(t *testing.T) {
	// Given a defaultsDir containing a relative symlink opencode/config/link.json -> real.json
	defaultsDir := t.TempDir()
	realContent := []byte(`{"real":"data"}`)
	writeTestFile(t, filepath.Join(defaultsDir, "opencode", "config", "real.json"), realContent)
	symlinkPath := filepath.Join(defaultsDir, "opencode", "config", "link.json")
	assertNoError(t, os.Symlink("real.json", symlinkPath))

	// And the destination already contains a regular file opencode/config/link.json with different content
	dst := t.TempDir()
	dstContent := []byte(`{"existing":"file"}`)
	dstFile := filepath.Join(dst, "opencode", "config", "link.json")
	writeTestFile(t, dstFile, dstContent)

	// When CopyDefaults copies the tree to the destination
	assertNoError(t, customisations.CopyDefaults(defaultsDir, dst))

	// Then the existing opencode/config/link.json at the destination is left untouched
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
	gotReal, err := os.ReadFile(filepath.Join(dst, "opencode", "config", "real.json"))
	if err != nil {
		t.Fatalf("expected real.json to exist in destination: %v", err)
	}
	if string(gotReal) != string(realContent) {
		t.Errorf("real.json content mismatch: got %q, want %q", gotReal, realContent)
	}
}

func TestCopyDefaults_ExistingSymlink_NotOverwrittenBySymlink(t *testing.T) {
	// Given a defaultsDir containing a relative symlink opencode/config/link.json -> real.json
	defaultsDir := t.TempDir()
	realContent := []byte(`{"real":"data"}`)
	writeTestFile(t, filepath.Join(defaultsDir, "opencode", "config", "real.json"), realContent)
	symlinkPath := filepath.Join(defaultsDir, "opencode", "config", "link.json")
	assertNoError(t, os.Symlink("real.json", symlinkPath))

	// And the destination already contains a symlink opencode/config/link.json pointing elsewhere
	dst := t.TempDir()
	assertNoError(t, os.MkdirAll(filepath.Join(dst, "opencode", "config"), 0o700))
	dstLink := filepath.Join(dst, "opencode", "config", "link.json")
	assertNoError(t, os.Symlink("other-target", dstLink))

	// When CopyDefaults copies the tree to the destination
	assertNoError(t, customisations.CopyDefaults(defaultsDir, dst))

	// Then the existing symlink at the destination is left untouched
	gotTarget, err := os.Readlink(dstLink)
	if err != nil {
		t.Fatalf("expected to read destination symlink: %v", err)
	}
	if gotTarget != "other-target" {
		t.Errorf("destination symlink was changed: got target %q, want %q", gotTarget, "other-target")
	}

	// And the source real.json was still copied to the destination
	gotReal, err := os.ReadFile(filepath.Join(dst, "opencode", "config", "real.json"))
	if err != nil {
		t.Fatalf("expected real.json to exist in destination: %v", err)
	}
	if string(gotReal) != string(realContent) {
		t.Errorf("real.json content mismatch: got %q, want %q", gotReal, realContent)
	}
}

func TestCopyDefaults_AbsoluteSymlinkToFile_ConvertedToRelative(t *testing.T) {
	// Given a defaultsDir containing a file opencode/config/z/y with content
	defaultsDir := t.TempDir()
	wantContent := []byte("hello")
	writeTestFile(t, filepath.Join(defaultsDir, "opencode", "config", "z", "y"), wantContent)
	// and an absolute symlink opencode/config/x -> <defaultsDir>/opencode/config/z/y
	symlinkPath := filepath.Join(defaultsDir, "opencode", "config", "x")
	assertNoError(t, os.Symlink(filepath.Join(defaultsDir, "opencode", "config", "z", "y"), symlinkPath))

	// And an empty destination directory
	dst := t.TempDir()

	// When CopyDefaults copies the tree to the destination
	assertNoError(t, customisations.CopyDefaults(defaultsDir, dst))

	// Then opencode/config/x at the destination is a symlink
	dstLink := filepath.Join(dst, "opencode", "config", "x")
	info, err := os.Lstat(dstLink)
	if err != nil {
		t.Fatalf("expected opencode/config/x to exist in destination: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected opencode/config/x to be a symlink, got mode %v", info.Mode())
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
	// Given a defaultsDir containing a directory opencode/config/targetdir
	defaultsDir := t.TempDir()
	targetDir := filepath.Join(defaultsDir, "opencode", "config", "targetdir")
	assertNoError(t, os.MkdirAll(targetDir, 0o700))
	// and an absolute symlink opencode/config/link -> <defaultsDir>/opencode/config/targetdir
	symlinkPath := filepath.Join(defaultsDir, "opencode", "config", "link")
	assertNoError(t, os.Symlink(targetDir, symlinkPath))

	// And an empty destination directory
	dst := t.TempDir()

	// When CopyDefaults copies the tree to the destination
	assertNoError(t, customisations.CopyDefaults(defaultsDir, dst))

	// Then opencode/config/link at the destination is a symlink
	dstLink := filepath.Join(dst, "opencode", "config", "link")
	info, err := os.Lstat(dstLink)
	if err != nil {
		t.Fatalf("expected opencode/config/link to exist in destination: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected opencode/config/link to be a symlink, got mode %v", info.Mode())
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
	// Given a defaultsDir containing a file opencode/config/shallow/target.txt
	defaultsDir := t.TempDir()
	wantContent := []byte("nested content")
	writeTestFile(t, filepath.Join(defaultsDir, "opencode", "config", "shallow", "target.txt"), wantContent)
	// and an absolute symlink opencode/config/deep/nested/link -> <defaultsDir>/opencode/config/shallow/target.txt
	symlinkPath := filepath.Join(defaultsDir, "opencode", "config", "deep", "nested", "link")
	assertNoError(t, os.MkdirAll(filepath.Dir(symlinkPath), 0o700))
	assertNoError(t, os.Symlink(filepath.Join(defaultsDir, "opencode", "config", "shallow", "target.txt"), symlinkPath))

	// And an empty destination directory
	dst := t.TempDir()

	// When CopyDefaults copies the tree to the destination
	assertNoError(t, customisations.CopyDefaults(defaultsDir, dst))

	// Then opencode/config/deep/nested/link at the destination is a symlink
	dstLink := filepath.Join(dst, "opencode", "config", "deep", "nested", "link")
	info, err := os.Lstat(dstLink)
	if err != nil {
		t.Fatalf("expected opencode/config/deep/nested/link to exist in destination: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected opencode/config/deep/nested/link to be a symlink, got mode %v", info.Mode())
	}

	// And os.Readlink on the destination symlink returns "../../shallow/target.txt"
	gotTarget, err := os.Readlink(dstLink)
	if err != nil {
		t.Fatalf("expected to read symlink target: %v", err)
	}
	if gotTarget != "../../shallow/target.txt" {
		t.Errorf("symlink target = %q, want %q", gotTarget, "../../shallow/target.txt")
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

func TestCopyDefaults_AbsoluteSymlinkSibling_ConvertedToRelative(t *testing.T) {
	// Given a defaultsDir containing a file opencode/config/y
	defaultsDir := t.TempDir()
	wantContent := []byte("sibling content")
	writeTestFile(t, filepath.Join(defaultsDir, "opencode", "config", "y"), wantContent)
	// and an absolute symlink opencode/config/x -> <defaultsDir>/opencode/config/y
	symlinkPath := filepath.Join(defaultsDir, "opencode", "config", "x")
	assertNoError(t, os.Symlink(filepath.Join(defaultsDir, "opencode", "config", "y"), symlinkPath))

	// And an empty destination directory
	dst := t.TempDir()

	// When CopyDefaults copies the tree to the destination
	assertNoError(t, customisations.CopyDefaults(defaultsDir, dst))

	// Then opencode/config/x at the destination is a symlink
	dstLink := filepath.Join(dst, "opencode", "config", "x")
	info, err := os.Lstat(dstLink)
	if err != nil {
		t.Fatalf("expected opencode/config/x to exist in destination: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected opencode/config/x to be a symlink, got mode %v", info.Mode())
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

func TestCopyDefaults_FileOutsideMountDirs_NotCopied(t *testing.T) {
	// Given a defaultsDir containing a file logs/debug.log inside a path not listed in MountDirs
	defaultsDir := t.TempDir()
	debugContent := []byte("should not be copied")
	writeTestFile(t, filepath.Join(defaultsDir, "logs", "debug.log"), debugContent)

	// And an empty destination directory
	dst := t.TempDir()

	// When CopyDefaults is called
	assertNoError(t, customisations.CopyDefaults(defaultsDir, dst))

	// Then logs/debug.log does NOT exist in the destination
	_, err := os.Stat(filepath.Join(dst, "logs", "debug.log"))
	if err == nil {
		t.Fatal("expected logs/debug.log to NOT exist in destination, but it does")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("expected IsNotExist error, got: %v", err)
	}
}

func TestCopyDefaults_StrayFileAtRoot_NotCopied(t *testing.T) {
	// Given a defaultsDir containing a file stray.txt at the root (not inside any MountDirs path)
	defaultsDir := t.TempDir()
	strayContent := []byte("should not be copied")
	writeTestFile(t, filepath.Join(defaultsDir, "stray.txt"), strayContent)

	// And an empty destination directory
	dst := t.TempDir()

	// When CopyDefaults is called
	assertNoError(t, customisations.CopyDefaults(defaultsDir, dst))

	// Then stray.txt does NOT exist in the destination
	_, err := os.Stat(filepath.Join(dst, "stray.txt"))
	if err == nil {
		t.Fatal("expected stray.txt to NOT exist in destination, but it does")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("expected IsNotExist error, got: %v", err)
	}
}

func TestCopyDefaults_DanglingSymlinkNeverOverwrites(t *testing.T) {
	// Given a defaultsDir containing a regular file opencode/config/link.json
	defaultsDir := t.TempDir()
	wantContent := []byte(`{"real":"data"}`)
	writeTestFile(t, filepath.Join(defaultsDir, "opencode", "config", "link.json"), wantContent)

	// And the destination already contains a dangling symlink opencode/config/link.json
	dst := t.TempDir()
	assertNoError(t, os.MkdirAll(filepath.Join(dst, "opencode", "config"), 0o700))
	dstLink := filepath.Join(dst, "opencode", "config", "link.json")
	assertNoError(t, os.Symlink("/nonexistent/target", dstLink))

	// When CopyDefaults copies the tree to the destination
	err := customisations.CopyDefaults(defaultsDir, dst)

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
		t.Fatalf("expected link.json to remain a symlink, got mode %v", info.Mode())
	}
	gotTarget, err := os.Readlink(dstLink)
	if err != nil {
		t.Fatalf("expected to read symlink target: %v", err)
	}
	if gotTarget != "/nonexistent/target" {
		t.Errorf("symlink target changed: got %q, want %q", gotTarget, "/nonexistent/target")
	}
}

func TestCopyDefaults_StatPermissionDenied_ReturnsError(t *testing.T) {
	// Given the test process is not running as root
	if os.Getuid() == 0 {
		t.Skip("skipping: running as root, permission checks do not apply")
	}

	// And the opencode subdirectory exists but has mode 0o000,
	// causing os.Stat on any child path to return permission denied
	defaultsDir := t.TempDir()
	restrictedDir := filepath.Join(defaultsDir, "opencode")
	assertNoError(t, os.MkdirAll(restrictedDir, 0o700))
	assertNoError(t, os.Chmod(restrictedDir, 0o000))
	defer func() { _ = os.Chmod(restrictedDir, 0o700) }()

	// And an empty destination directory
	dst := t.TempDir()

	// When CopyDefaults is called
	err := customisations.CopyDefaults(defaultsDir, dst)

	// Then it returns a non-nil permission error
	if err == nil {
		t.Fatal("expected error for permission-denied stat, got nil")
	}
	if !errors.Is(err, os.ErrPermission) {
		t.Errorf("expected permission error (os.ErrPermission), got: %v", err)
	}
	// And the error is the direct stat error, not the HardenSourceTree fall-through error
	if strings.Contains(err.Error(), "resolving source tree root") {
		t.Errorf("error should not originate from HardenSourceTree, got: %q", err.Error())
	}
}

func TestCopyDefaults_MultipleApps_CopiesAll(t *testing.T) {
	// Given defaults exist for all MountDirs entries plus a file outside MountDirs
	defaultsDir := t.TempDir()
	for _, entry := range customisations.MountDirs() {
		writeTestFile(t, filepath.Join(defaultsDir, entry, "test.json"), []byte(entry))
	}
	writeTestFile(t, filepath.Join(defaultsDir, "logs", "debug.log"), []byte("skip"))

	dst := t.TempDir()
	// When CopyDefaults is called
	assertNoError(t, customisations.CopyDefaults(defaultsDir, dst))

	// Then files land under each MountDirs entry
	for _, entry := range customisations.MountDirs() {
		wantPath := filepath.Join(entry, "test.json")
		wantContent := []byte(entry)
		got, err := os.ReadFile(filepath.Join(dst, wantPath))
		if err != nil {
			t.Fatalf("expected %s to exist: %v", wantPath, err)
		}
		if string(got) != string(wantContent) {
			t.Errorf("%s content mismatch: got %q, want %q", wantPath, got, wantContent)
		}
	}

	// And the file outside MountDirs is not copied
	_, err := os.Stat(filepath.Join(dst, "logs", "debug.log"))
	if err == nil {
		t.Fatal("expected logs/debug.log to NOT exist in destination, but it does")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("expected IsNotExist error, got: %v", err)
	}
}
