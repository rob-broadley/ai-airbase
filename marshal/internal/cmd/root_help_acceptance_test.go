// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd_test

import (
	"bytes"
	"testing"

	"github.com/rob-broadley/ai-airbase/marshal/internal/cmd"
)

// TestRootCmd_NoArgs_PrintsHelp verifies that running marshal with no
// arguments or subcommand prints cobra usage help and exits 0.
func TestRootCmd_NoArgs_PrintsHelp(t *testing.T) {
	// Given no arguments
	deps := cmd.Deps{
		Getuid: stubGetuid,
		Getgid: stubGetgid,
	}

	outBuf := &bytes.Buffer{}
	root := cmd.NewRootCmd(deps)
	root.SetOut(outBuf)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{})

	// When marshal is executed with no arguments
	err := root.Execute()

	// Then exit code is 0 (help is not an error)
	if err != nil {
		t.Errorf("expected no error, got: %v", err)
	}

	// And stdout contains cobra usage sections
	out := outBuf.String()
	assertContains(t, out, "Usage:")
	assertContains(t, out, "Available Commands:")

	// And all subcommands are listed
	for _, sub := range []string{"start", "create", "list", "stop", "status", "recreate", "remove", "shell", "pull"} {
		assertContains(t, out, sub)
	}
}
