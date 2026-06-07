// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Unit tests for checkBasenameConflicts — overlap / nesting detection
// ---------------------------------------------------------------------------

// TestCheckBasenameConflicts_NestedPaths verifies that checkBasenameConflicts
// returns an error when one resolved mount path is a strict subdirectory of
// another (outer → inner nesting), as this defeats the --mask security
// guarantee.
func TestCheckBasenameConflicts_NestedPaths(t *testing.T) {
	// Given /home/user/project/secrets is strictly nested inside /home/user/project
	err := checkBasenameConflicts([]string{"/home/user/project", "/home/user/project/secrets"})

	// Then an error is returned containing "nested"
	if err == nil {
		t.Fatal("expected an error for nested mount paths, got nil")
	}
	if !strings.Contains(err.Error(), "nested") {
		t.Errorf("expected error to contain 'nested', got: %v", err)
	}
}

// TestCheckBasenameConflicts_NestedPathsReverseOrder verifies the nesting
// check works regardless of the order the paths are supplied.
func TestCheckBasenameConflicts_NestedPathsReverseOrder(t *testing.T) {
	// Given inner path listed before outer path
	err := checkBasenameConflicts([]string{"/home/user/project/secrets", "/home/user/project"})

	// Then an error is returned containing "nested"
	if err == nil {
		t.Fatal("expected an error for nested mount paths (reverse order), got nil")
	}
	if !strings.Contains(err.Error(), "nested") {
		t.Errorf("expected error to contain 'nested', got: %v", err)
	}
}

// TestCheckBasenameConflicts_FalsePrefixNotRejected verifies that a path that
// is a string-prefix of another but NOT a strict parent directory is accepted
// (e.g. /foo should not match /foobar).
func TestCheckBasenameConflicts_FalsePrefixNotRejected(t *testing.T) {
	// Given /foo and /foobar — /foo is a string-prefix of /foobar but not a
	// parent directory, so no nesting conflict should be reported.
	err := checkBasenameConflicts([]string{"/foo", "/foobar"})

	// Then no error is returned (basenames are different, no nesting)
	if err != nil {
		t.Errorf("unexpected error for non-nested paths /foo and /foobar: %v", err)
	}
}

// TestCheckBasenameConflicts_DisjointPaths verifies that two unrelated paths
// do not trigger a nesting error.
func TestCheckBasenameConflicts_DisjointPaths(t *testing.T) {
	// Given two independent absolute paths with different basenames
	err := checkBasenameConflicts([]string{"/home/user/project", "/home/user/other"})

	// Then no error is returned
	if err != nil {
		t.Errorf("unexpected error for disjoint paths: %v", err)
	}
}

// TestCheckBasenameConflicts_ErrorIdentifiesBothPaths verifies the error
// message explicitly names both the inner and outer paths so the user can
// understand which mounts conflict.
func TestCheckBasenameConflicts_ErrorIdentifiesBothPaths(t *testing.T) {
	// Given a nesting pair
	outer := "/home/user/project"
	inner := "/home/user/project/secrets"
	err := checkBasenameConflicts([]string{outer, inner})

	// Then the error message names both paths
	if err == nil {
		t.Fatal("expected an error, got nil")
	}
	if !strings.Contains(err.Error(), inner) {
		t.Errorf("error does not mention inner path %q: %v", inner, err)
	}
	if !strings.Contains(err.Error(), outer) {
		t.Errorf("error does not mention outer path %q: %v", outer, err)
	}
}

func TestValidateSavedMounts_AllValid(t *testing.T) {
	mounts := []string{"/home/user/project", "/home/user/shared"}
	if err := validateSavedMounts(mounts); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidateSavedMounts_StaleNestedMountReturnsError(t *testing.T) {
	err := validateSavedMounts([]string{"/home/user/project", "/home/user/project/src"})
	if err == nil {
		t.Fatal("expected error for nested saved mounts, got nil")
	}
	if !strings.Contains(err.Error(), "saved mount config is invalid") {
		t.Errorf("expected wrapped saved-config error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "nested") {
		t.Errorf("expected nested conflict error, got: %v", err)
	}
}
