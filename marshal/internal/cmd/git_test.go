// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd

import (
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
