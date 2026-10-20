// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"io"

	"backup-radar-pp-cli/internal/termio"
)

// stdoutW and stderrW are the process-wide escaping writers (internal/termio).
// Every write to the terminal goes through them: Cobra's SetOut/SetErr, the
// envelope and --deliver tee in root.go, and the generated code's former
// direct fmt.Fprintf calls to os.Stderr (now fmt.Fprintf(stderrW(), ...)).
// Table, CSV, plain and quiet output already drop control characters
// (cliutil.ScrubTerminal, scrubCell), and the novel report printers scrub
// too; the writer is the boundary that catches every other path.
// HAND-FIX(output-escapes-c1-controls)
func stdoutW() io.Writer { return termio.Stdout() }
func stderrW() io.Writer { return termio.Stderr() }
