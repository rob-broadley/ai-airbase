// SPDX-License-Identifier: AGPL-3.0-or-later
package customisations_test

import (
	"os"
	"path/filepath"
	"testing"
)

func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func writeTestFile(t *testing.T, path string, content []byte) {
	t.Helper()
	assertNoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	assertNoError(t, os.WriteFile(path, content, 0o644))
}

func createOutsideFile(t *testing.T) string {
	t.Helper()
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	return outsideFile
}
