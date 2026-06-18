// SPDX-License-Identifier: AGPL-3.0-or-later
package customisations_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/customisations"
)

// --- Ensure creates subdirs from MountDirs ---
func TestEnsure_CreatesSubdirsFromMountDirs(t *testing.T) {
	base := t.TempDir()
	xdgDataHome := func() string { return base }

	// Given an injected xdgDataHome function that returns a t.TempDir()-based path

	// When Ensure is called
	err := customisations.Ensure(xdgDataHome)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Then all MountDirs entries exist as directories with 0o700 permissions
	wantBase := filepath.Join(base, "marshal", "defaults")
	for _, entry := range customisations.MountDirs() {
		p := filepath.Join(wantBase, entry)
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
	base := t.TempDir()
	xdgDataHome := func() string { return base }
	// Given the three subdirs exist (config=0o700, share=0o755, state=0o700)
	assertNoError(t, customisations.Ensure(xdgDataHome))

	wantBase := filepath.Join(base, "marshal", "defaults", "opencode")
	shareDir := filepath.Join(wantBase, "share")
	assertNoError(t, os.Chmod(shareDir, 0o755))

	// When Ensure is called again
	err := customisations.Ensure(xdgDataHome)

	// Then no error is returned
	if err != nil {
		t.Fatalf("expected no error on second call, got: %v", err)
	}

	// And the three subdirs still exist with their pre-existing permissions
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
	base := t.TempDir()
	xdgDataHome := func() string { return base }
	// Given a regular file exists at the config subdir path
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
	base := t.TempDir()
	xdgDataHome := func() string { return base }
	// Given a symlink exists at the config subdir path
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

	// When DefaultsDir is called and Ensure is run
	defaultsDir := customisations.DefaultsDir(base)
	wantSubdirs := []string{"opencode/config", "opencode/share", "opencode/state"}

	assertNoError(t, customisations.Ensure(xdgDataHome))

	// Then each path matches the corresponding path that Ensure would create
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
