// SPDX-License-Identifier: AGPL-3.0-or-later
package hostinfo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Unit tests for filterEnv
// ---------------------------------------------------------------------------

func TestFilterEnv_StripsBlockedKey(t *testing.T) {
	env := []string{"GIT_DIR=/some/path", "HOME=/home/user"}
	got := filterEnv(env, "GIT_DIR")
	for _, e := range got {
		if strings.HasPrefix(e, "GIT_DIR=") {
			t.Errorf("GIT_DIR should have been stripped, got %v", got)
		}
	}
	found := false
	for _, e := range got {
		if e == "HOME=/home/user" {
			found = true
		}
	}
	if !found {
		t.Errorf("HOME should be preserved, got %v", got)
	}
}

func TestFilterEnv_StripsAllFourBlockedKeys(t *testing.T) {
	env := []string{
		"GIT_DIR=/a",
		"GIT_CONFIG=/b",
		"GIT_CONFIG_GLOBAL=/c",
		"GIT_CONFIG_NOSYSTEM=1",
		"HOME=/home/user",
	}
	got := filterEnv(env, "GIT_DIR", "GIT_CONFIG", "GIT_CONFIG_GLOBAL", "GIT_CONFIG_NOSYSTEM")
	if len(got) != 1 || got[0] != "HOME=/home/user" {
		t.Errorf("expected only HOME preserved, got %v", got)
	}
}

func TestFilterEnv_PreservesNonBlockedVars(t *testing.T) {
	env := []string{"PATH=/usr/bin", "HOME=/home/user", "TERM=xterm"}
	got := filterEnv(env, "GIT_DIR")
	if len(got) != len(env) {
		t.Errorf("expected all vars preserved, got %v", got)
	}
}

func TestFilterEnv_DoesNotStripPrefixMatch(t *testing.T) {
	// GIT_DIR_EXTRA must not be stripped when only GIT_DIR is blocked
	env := []string{"GIT_DIR_EXTRA=x", "GIT_DIR=/blocked"}
	got := filterEnv(env, "GIT_DIR")
	for _, e := range got {
		if strings.HasPrefix(e, "GIT_DIR=") {
			t.Errorf("GIT_DIR should be stripped, got %v", got)
		}
	}
	found := false
	for _, e := range got {
		if e == "GIT_DIR_EXTRA=x" {
			found = true
		}
	}
	if !found {
		t.Errorf("GIT_DIR_EXTRA should be preserved, got %v", got)
	}
}

func TestFilterEnv_EmptyInput(t *testing.T) {
	got := filterEnv(nil, "GIT_DIR")
	if len(got) != 0 {
		t.Errorf("expected empty result for nil input, got %v", got)
	}
}

func TestFilterEnv_EntryWithNoEquals(t *testing.T) {
	// An entry with no '=' should not be stripped unless name matches exactly
	env := []string{"GIT_DIR", "HOME=/home/user"}
	got := filterEnv(env, "GIT_DIR")
	// "GIT_DIR" (no =) — strings.Cut returns ("GIT_DIR","",false), name=="GIT_DIR" matches → stripped
	for _, e := range got {
		if e == "GIT_DIR" {
			t.Errorf("GIT_DIR with no = should be stripped, got %v", got)
		}
	}
}

// ---------------------------------------------------------------------------
// Tests for LookupHostGitConfig
// ---------------------------------------------------------------------------

func TestLookupHostGitConfig_ReturnsValue(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "git")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho 'Jane Doe'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	got := LookupHostGitConfig("user.name")
	if got != "Jane Doe" {
		t.Errorf("expected 'Jane Doe', got %q", got)
	}
}

func TestLookupHostGitConfig_ReturnsEmptyOnNonZeroExit(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "git")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	got := LookupHostGitConfig("user.name")
	if got != "" {
		t.Errorf("expected empty string on non-zero exit, got %q", got)
	}
}

func TestLookupHostGitConfig_ReturnsEmptyWhenGitAbsent(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	got := LookupHostGitConfig("user.name")
	if got != "" {
		t.Errorf("expected empty string when git absent, got %q", got)
	}
}

func TestLookupHostGitConfig_StripsBlockedEnvVars(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "git")
	// Fail with exit 2 if any blocked var is present in the environment
	if err := os.WriteFile(script, []byte(`#!/bin/sh
if [ -n "$GIT_DIR" ] || [ -n "$GIT_CONFIG" ] || [ -n "$GIT_CONFIG_GLOBAL" ] || [ -n "$GIT_CONFIG_NOSYSTEM" ]; then
  exit 2
fi
echo "clean"
`), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	t.Setenv("GIT_DIR", "/should/be/stripped")
	t.Setenv("GIT_CONFIG", "/should/be/stripped")
	t.Setenv("GIT_CONFIG_GLOBAL", "/should/be/stripped")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	got := LookupHostGitConfig("user.name")
	if got != "clean" {
		t.Errorf("expected blocked vars stripped, got %q", got)
	}
}
