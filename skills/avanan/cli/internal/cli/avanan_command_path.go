// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (NOT generated): handfixes.json credkit-wiring.

package cli

import (
	"strings"

	"avanan-pp-cli/internal/cliutil"
)

// The kit's startup asks which command a command line runs before Execute
// parses it (credwire's feedback --stdin note). Cobra's Find answers as Execute
// will: it knows which flags take a value, so a flag in front of the command,
// or a flag value that reads like a command name, does not move it.
func init() {
	cliutil.CommandPath = func(args []string) []string {
		cmd, _, err := RootCmd().Find(args)
		if err != nil || cmd == nil {
			return nil
		}
		return strings.Fields(cmd.CommandPath())[1:]
	}
}
