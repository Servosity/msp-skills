// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/synology/handfixes.json
// under the "output-escapes-c1-controls" entry.

package cli

import (
	"io"

	"synology-pp-cli/internal/termio"
)

// stdoutW and stderrW are the process-wide escaping writers (internal/termio).
// Every write to the terminal goes through them: Cobra's SetOut/SetErr, the
// envelope and --deliver tee in root.go, and the generated code's former
// direct fmt.Fprintf calls to os.Stderr (now fmt.Fprintf(stderrW(), ...)).
// Table, CSV, plain and quiet output already drop control characters
// (cliutil.ScrubTerminal, scrubCell); the writer is the boundary that catches
// every other path, including the UTF-8 C1 controls encoding/json leaves raw
// in --json, --agent and pretty-JSON output.
func stdoutW() io.Writer { return termio.Stdout() }
func stderrW() io.Writer { return termio.Stderr() }
