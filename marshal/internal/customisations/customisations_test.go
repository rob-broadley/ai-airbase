// SPDX-License-Identifier: AGPL-3.0-or-later
package customisations_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/customisations"
)

// validLookup returns a lookup function that returns John Doe / john@example.com
// for the standard git config keys, used by multiple SeedGitConfigDefaults tests.
func validLookup(key string) string {
	switch key {
	case "user.name":
		return "John Doe"
	case "user.email":
		return "john@example.com"
	}
	return ""
}

// Test_SeedGitConfigDefaults_CreatesConfigFileWithUserSection verifies that when the
// git config file does not yet exist and the lookup returns user information,
// SeedGitConfigDefaults writes the file with the correct [user] section content.
//
// Given a defaultsDir that does not contain git/config/config
// And a valid lookup returning user.name and user.email
// When SeedGitConfigDefaults is called
// Then a git/config/config file is created containing the correct [user] section
func Test_SeedGitConfigDefaults_CreatesConfigFileWithUserSection(t *testing.T) {
	// Given
	defaultsDir := t.TempDir()
	lookup := validLookup

	// When
	err := customisations.SeedGitConfigDefaults(defaultsDir, lookup)
	assertNoError(t, err)

	// Then a file at defaultsDir/git/config/config is created
	configPath := filepath.Join(defaultsDir, "git", "config", "config")
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("expected file to exist at %s: %v", configPath, err)
	}

	// And its content matches BuildGitConfigContent
	wantContent := customisations.BuildGitConfigContent(lookup)
	if string(content) != string(wantContent) {
		t.Errorf("file content mismatch:\ngot:\n%s\nwant:\n%s", content, wantContent)
	}
}

// Test_SeedGitConfigDefaults_DoesNotOverwriteExistingFile verifies that SeedGitConfigDefaults
// is idempotent — when the config file already exists, the call is a no-op
// and the existing content is preserved.
//
// Given a defaultsDir that already contains git/config/config
// And a valid lookup returning user.name and user.email
// When SeedGitConfigDefaults is called
// Then the existing file is NOT modified (content remains unchanged)
func Test_SeedGitConfigDefaults_DoesNotOverwriteExistingFile(t *testing.T) {
	// Given a pre-existing config file
	preExistingContent := "pre-existing content\n"
	defaultsDir := t.TempDir()
	configPath := filepath.Join(defaultsDir, "git", "config", "config")
	writeTestFile(t, configPath, []byte(preExistingContent))

	lookup := validLookup

	// When
	err := customisations.SeedGitConfigDefaults(defaultsDir, lookup)

	// Then the call returns nil
	if err != nil {
		t.Fatalf("expected nil error for existing file, got: %v", err)
	}

	// And the existing file content remains unchanged
	gotContent, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("expected to read existing file: %v", err)
	}
	if string(gotContent) != preExistingContent {
		t.Errorf("file was modified:\ngot:\n%s\nwant:\n%s", gotContent, preExistingContent)
	}
}

// Test_SeedGitConfigDefaults_PropagatesIoError verifies that SeedGitConfigDefaults does not
// swallow I/O errors — when the parent directory cannot be created (e.g. a
// regular file blocks the path), the error is returned to the caller.
//
// Given a defaultsDir where defaultsDir/git is a regular file (blocking subdirectory creation)
// And a valid lookup returning user.name and user.email
// When SeedGitConfigDefaults is called
// Then the I/O error is propagated (not swallowed)
func Test_SeedGitConfigDefaults_PropagatesIoError(t *testing.T) {
	// Skip when running as root since permission/propagation behaviour differs
	if os.Getuid() == 0 {
		t.Skip("skipping: running as root, I/O error propagation behaviour differs")
	}

	defaultsDir := t.TempDir()
	lookup := validLookup

	// Given defaultsDir/git exists as a regular file, blocking directory creation
	blockingFile := filepath.Join(defaultsDir, "git")
	if err := os.WriteFile(blockingFile, []byte("blocker"), 0o644); err != nil {
		t.Fatal(err)
	}

	// When
	err := customisations.SeedGitConfigDefaults(defaultsDir, lookup)

	// Then the error is propagated (not swallowed)
	if err == nil {
		t.Fatal("expected an I/O error when parent is a file, got nil")
	}
}

// Test_SeedGitConfigDefaults_SkipsWhenNoUserInfo verifies that SeedGitConfigDefaults does not
// create a file when the lookup returns empty for all keys — no user info
// means nothing to write.
//
// Given a lookup that returns empty strings for both user.name and user.email
// When SeedGitConfigDefaults is called
// Then no file is created (nothing to write)
func Test_SeedGitConfigDefaults_SkipsWhenNoUserInfo(t *testing.T) {
	defaultsDir := t.TempDir()
	lookup := func(string) string { return "" }

	// When
	err := customisations.SeedGitConfigDefaults(defaultsDir, lookup)

	// Then the call returns nil
	if err != nil {
		t.Fatalf("expected nil error for empty lookup, got: %v", err)
	}

	// And no file exists at defaultsDir/git/config/config
	configPath := filepath.Join(defaultsDir, "git", "config", "config")
	if _, err := os.Stat(configPath); err == nil {
		t.Fatal("expected no file to be created for empty lookup, but file exists")
	} else if !os.IsNotExist(err) {
		t.Fatalf("unexpected error stat'ing config path: %v", err)
	}
}
