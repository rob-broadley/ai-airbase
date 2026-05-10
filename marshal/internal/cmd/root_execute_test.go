// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd_test

// Tests for the Execute() function's error-handling contract.
//
// Execute() calls os.Exit(1) on error, so it cannot be called in a test
// directly. Instead these tests exercise the error-propagation path through
// NewRootCmd — verifying that:
//
//  1. When a command built by NewRootCmd fails, the error is returned to the
//     caller (not swallowed).
//  2. When SilenceErrors = true is set (as Execute() sets it after the fix),
//     Cobra does NOT write to the command's error writer — meaning the only
//     thing that prints the error is the explicit fmt.Fprintln(os.Stderr, err)
//     in Execute().
//
// These two properties together confirm that the explicit-print contract is
// sound: if SilenceErrors = true and Cobra is quiet, the user only sees the
// error because Execute() explicitly prints it.

import (
	"bytes"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/cmd"
)

// TestNewRootCmd_ErrorPropagatesWhenSilenceErrorsTrue verifies that when
// SilenceErrors is set to true on a command built by NewRootCmd (as Execute()
// will do after the fix), a command failure still returns a non-nil error to
// the caller.
//
// Given  SilenceErrors = true is set on a root command built by NewRootCmd
// When   Execute is called with an argument that triggers a command error
// Then   Execute returns a non-nil error (the error is not swallowed)
// And    Cobra writes nothing to the command's error writer
//
// This guards the "Execute owns printing" contract: if SilenceErrors silences
// Cobra's printing AND Execute() did not call fmt.Fprintln(os.Stderr, err),
// the user would see nothing. The test proves the error is available for
// explicit printing.
func TestNewRootCmd_ErrorPropagatesWhenSilenceErrorsTrue(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	// Given a root command with SilenceErrors = true (as Execute() will set)
	deps := cmd.Deps{
		// Minimal deps: no runner, no exec — we want the command to fail fast
		// on a bad subcommand, not reach container logic.
	}
	root := cmd.NewRootCmd(deps)
	root.SilenceErrors = true
	root.SilenceUsage = true

	var errBuf bytes.Buffer
	root.SetErr(&errBuf)

	// When an unknown subcommand is given — Cobra returns an error for this
	root.SetArgs([]string{"this-subcommand-does-not-exist"})
	err := root.Execute()

	// Then the error must be non-nil — it must NOT be swallowed
	if err == nil {
		t.Fatal("expected a non-nil error from Execute(), got nil")
	}

	// And Cobra must NOT have written to the err buffer (because
	// SilenceErrors = true).  If Cobra wrote the error, the test fails —
	// confirming that with SilenceErrors = true the only printing must come
	// from the explicit fmt.Fprintln(os.Stderr, err) in Execute().
	if errBuf.Len() > 0 {
		t.Errorf(
			"expected Cobra to write nothing to err buffer when SilenceErrors=true,"+
				" but got: %q", errBuf.String(),
		)
	}
}

// TestNewRootCmd_ErrorMessageDescribesFailure verifies that the error returned
// by NewRootCmd's Execute() contains a human-readable description of the
// failure, so that Execute()'s explicit fmt.Fprintln(os.Stderr, err) produces
// a useful message for the user.
//
// Given  a command built by NewRootCmd
// When   it is invoked with an unknown subcommand
// Then   the returned error message is non-empty
func TestNewRootCmd_ErrorMessageDescribesFailure(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	deps := cmd.Deps{}
	root := cmd.NewRootCmd(deps)
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetArgs([]string{"this-subcommand-does-not-exist"})

	err := root.Execute()

	if err == nil {
		t.Fatal("expected a non-nil error, got nil")
	}
	if err.Error() == "" {
		t.Error("expected the error message to be non-empty so fmt.Fprintln produces useful output")
	}
}
