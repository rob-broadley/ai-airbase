// SPDX-License-Identifier: AGPL-3.0-or-later
package customisations_test

import (
	"strings"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/customisations"
)

// Test_BuildConfig_IncludesNameAndEmail verifies that BuildGitConfigContent
// produces a [user] section with both name and email when the lookup
// function returns values for both keys.
func Test_BuildConfig_IncludesNameAndEmail(t *testing.T) {
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

	// When BuildGitConfigContent is called
	content := customisations.BuildGitConfigContent(lookup)
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

// Test_BuildConfig_ReturnsEmptyWhenNoValues verifies that
// BuildGitConfigContent returns an empty byte slice when the lookup
// function returns empty for all keys — no file should be written.
func Test_BuildConfig_ReturnsEmptyWhenNoValues(t *testing.T) {
	// Given a lookup function that returns empty for all keys

	// When BuildGitConfigContent is called
	content := customisations.BuildGitConfigContent(func(string) string { return "" })

	// Then the returned content is empty
	if len(content) != 0 {
		t.Errorf("expected empty content when no values, got: %q", content)
	}
}

// Test_BuildConfig_OmitsEmailWhenNotSet verifies that BuildGitConfigContent
// produces a config section containing only the name key when the email
// lookup returns empty.
func Test_BuildConfig_OmitsEmailWhenNotSet(t *testing.T) {
	// Given a lookup function returning only user.name (no email)

	// When BuildGitConfigContent is called
	content := customisations.BuildGitConfigContent(func(key string) string {
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

// Test_GitQuote_EscapesBackslash verifies that GitQuote doubles any
// backslash character within the value, preventing git config parse errors.
func Test_GitQuote_EscapesBackslash(t *testing.T) {
	// Given a value containing a backslash (unescaped backslash causes a fatal git parse error)

	// When GitQuote is called
	got := customisations.GitQuote(`Rob\Broadley`)

	// Then the backslash is doubled within the quoted output
	want := `"Rob\\Broadley"`
	if got != want {
		t.Errorf("GitQuote backslash: got %s, want %s", got, want)
	}
}

// Test_GitQuote_EscapesDoubleQuote verifies that GitQuote escapes
// double-quote characters inside the value with a backslash.
func Test_GitQuote_EscapesDoubleQuote(t *testing.T) {
	// Given a value containing double-quote characters

	// When GitQuote is called
	got := customisations.GitQuote(`Alice "The Boss" Smith`)

	// Then the double quotes are escaped with backslash within the outer quotes
	want := `"Alice \"The Boss\" Smith"`
	if got != want {
		t.Errorf("GitQuote double-quote: got %s, want %s", got, want)
	}
}

// Test_GitQuote_PreservesSemicolonInQuotes verifies that GitQuote wraps the
// value in double quotes so that a semicolon (which starts an inline comment
// when unquoted) is treated as a literal character.
func Test_GitQuote_PreservesSemicolonInQuotes(t *testing.T) {
	// Given a value containing a semicolon (semicolons after whitespace start inline comments in gitconfig)

	// When GitQuote is called
	got := customisations.GitQuote("Rob; Smith")

	// Then the value is wrapped in double quotes, preserving the semicolon
	if !strings.HasPrefix(got, `"`) || !strings.HasSuffix(got, `"`) {
		t.Errorf("GitQuote semicolon: result not quoted: %s", got)
	}
	if strings.Contains(got[1:len(got)-1], ";") {
		if !strings.Contains(got, ";") {
			t.Errorf("GitQuote semicolon: semicolon lost: %s", got)
		}
	}
}

// Test_GitQuote_WrapsValueWithHash verifies that GitQuote wraps a value
// containing a hash character in double quotes, preventing it from being
// interpreted as a comment start.
func Test_GitQuote_WrapsValueWithHash(t *testing.T) {
	// Given a value containing a hash character

	// When GitQuote is called
	got := customisations.GitQuote("Rob # Smith")

	// Then the value is wrapped in double quotes
	if !strings.HasPrefix(got, `"`) {
		t.Errorf("GitQuote hash: result not quoted: %s", got)
	}
}

// Test_BuildConfig_EscapesBackslashInName verifies that BuildGitConfigContent
// doubles backslashes in the user.name value, preventing a fatal git config
// parse error when the generated file is read by git.
func Test_BuildConfig_EscapesBackslashInName(t *testing.T) {
	// Given a user.name containing a backslash (the critical regression case)

	// When BuildGitConfigContent is called
	content := customisations.BuildGitConfigContent(func(key string) string {
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
