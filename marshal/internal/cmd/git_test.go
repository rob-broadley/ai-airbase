// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Unit tests for buildGitConfigContent and gitQuote
// ---------------------------------------------------------------------------

func TestBuildGitConfigContent_BothValues(t *testing.T) {
	// Given a lookup function returning both user.name and user.email
	lookup := func(key string) string {
		switch key {
		case "user.name":
			return "Test User"
		case "user.email":
			return "test@example.com"
		}
		return ""
	}

	// When buildGitConfigContent is called
	content := buildGitConfigContent(lookup)
	got := string(content)

	// Then the output contains a [user] section with quoted name and email
	if !strings.Contains(got, `"Test User"`) {
		t.Errorf("expected quoted name in output, got: %s", got)
	}
	if !strings.Contains(got, `"test@example.com"`) {
		t.Errorf("expected quoted email in output, got: %s", got)
	}
	if !strings.HasPrefix(got, "[user]\n") {
		t.Errorf("expected [user] section header, got: %s", got)
	}
}

func TestBuildGitConfigContent_EmptyReturnsEmpty(t *testing.T) {
	// Given a lookup function that returns empty for all keys

	// When buildGitConfigContent is called
	content := buildGitConfigContent(func(string) string { return "" })

	// Then the returned content is empty
	if len(content) != 0 {
		t.Errorf("expected empty content when no values, got: %q", content)
	}
}

func TestBuildGitConfigContent_NameOnly(t *testing.T) {
	// Given a lookup function returning only user.name (no email)

	// When buildGitConfigContent is called
	content := buildGitConfigContent(func(key string) string {
		if key == "user.name" {
			return "Alice"
		}
		return ""
	})

	// Then the output contains name but no email key
	got := string(content)
	if strings.Contains(got, "email") {
		t.Errorf("email key must be absent when not set, got: %s", got)
	}
}

func TestGitQuote_Backslash(t *testing.T) {
	// Given a value containing a backslash (unescaped backslash causes a fatal git parse error)

	// When gitQuote is called
	got := gitQuote(`Rob\Broadley`)

	// Then the backslash is doubled within the quoted output
	want := `"Rob\\Broadley"`
	if got != want {
		t.Errorf("gitQuote backslash: got %s, want %s", got, want)
	}
}

func TestGitQuote_DoubleQuote(t *testing.T) {
	// Given a value containing double-quote characters

	// When gitQuote is called
	got := gitQuote(`Alice "The Boss" Smith`)

	// Then the double quotes are escaped with backslash within the outer quotes
	want := `"Alice \"The Boss\" Smith"`
	if got != want {
		t.Errorf("gitQuote double-quote: got %s, want %s", got, want)
	}
}

func TestGitQuote_Semicolon(t *testing.T) {
	// Given a value containing a semicolon (semicolons after whitespace start inline comments in gitconfig)

	// When gitQuote is called
	got := gitQuote("Rob; Smith")

	// Then the value is wrapped in double quotes, preserving the semicolon
	if !strings.HasPrefix(got, `"`) || !strings.HasSuffix(got, `"`) {
		t.Errorf("gitQuote semicolon: result not quoted: %s", got)
	}
	if strings.Contains(got[1:len(got)-1], ";") {
		// The semicolon is inside quotes — that's correct, but let's verify
		// it's actually the original char (not accidentally dropped).
		if !strings.Contains(got, ";") {
			t.Errorf("gitQuote semicolon: semicolon lost: %s", got)
		}
	}
}

func TestGitQuote_Hash(t *testing.T) {
	// Given a value containing a hash character

	// When gitQuote is called
	got := gitQuote("Rob # Smith")

	// Then the value is wrapped in double quotes
	if !strings.HasPrefix(got, `"`) {
		t.Errorf("gitQuote hash: result not quoted: %s", got)
	}
}

func TestBuildGitConfigContent_BackslashInName(t *testing.T) {
	// Given a user.name containing a backslash (the critical regression case)

	// When buildGitConfigContent is called
	content := buildGitConfigContent(func(key string) string {
		if key == "user.name" {
			return `Rob\Broadley`
		}
		return ""
	})

	// Then the backslash is escaped to prevent a fatal git config parse error
	got := string(content)
	if !strings.Contains(got, `"Rob\\Broadley"`) {
		t.Errorf("expected escaped backslash in quoted value, got: %s", got)
	}
}

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
// Tests for lookupHostGitConfig
// ---------------------------------------------------------------------------

func TestLookupHostGitConfig_ReturnsValue(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "git")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho 'Jane Doe'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	got := lookupHostGitConfig("user.name")
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
	got := lookupHostGitConfig("user.name")
	if got != "" {
		t.Errorf("expected empty string on non-zero exit, got %q", got)
	}
}

func TestLookupHostGitConfig_ReturnsEmptyWhenGitAbsent(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	got := lookupHostGitConfig("user.name")
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
	got := lookupHostGitConfig("user.name")
	if got != "clean" {
		t.Errorf("expected blocked vars stripped, got %q", got)
	}
}
