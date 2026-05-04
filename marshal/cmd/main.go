// SPDX-License-Identifier: AGPL-3.0-or-later
// Package main is the entry point for the marshal CLI.
package main

import (
	"github.com/rob-broadley/ai-airbase/marshal/internal/cmd"
)

// version is the build-time version string injected via -ldflags; defaults to
// "dev" when the binary is built without explicit version information.
var version = "dev"

// main is the program entry point; it delegates immediately to cmd.Execute.
func main() {
	cmd.Execute(version)
}
