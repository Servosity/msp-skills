// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"io"

	"cloudally-pp-cli/internal/cliutil"
	"github.com/spf13/cobra"
)

// tenantText makes one tenant-supplied string (a mailbox, a file name, a
// job status, an error the portal wrote) safe to show on a terminal. Every
// human-readable download printer goes through it, directly or through
// tenantf and tenantCells, so a name like "\x1b]0;OWNED\x07" is shown as
// text instead of being run by the terminal. JSON output is not passed
// through it: the encoder already escapes control characters there.
func tenantText(s string) string { return cliutil.ScrubTerminal(s) }

// tenantf is fmt.Fprintf with every string and error argument passed
// through tenantText. The format string is this program's own text, so its
// newlines survive; numbers and other values are printed unchanged.
func tenantf(w io.Writer, format string, args ...any) {
	clean := make([]any, len(args))
	for i, a := range args {
		switch v := a.(type) {
		case string:
			clean[i] = tenantText(v)
		case error:
			clean[i] = tenantText(v.Error())
		default:
			clean[i] = a
		}
	}
	fmt.Fprintf(w, format, clean...)
}

// tenantCells scrubs every cell of a table before it is printed.
func tenantCells(rows [][]string) [][]string {
	out := make([][]string, len(rows))
	for i, r := range rows {
		cells := make([]string, len(r))
		for j, c := range r {
			cells[j] = tenantText(c)
		}
		out[i] = cells
	}
	return out
}

// scrubbedError is an error whose text is safe to print. It unwraps to the
// original, so errors.Is, errors.As and the exit code are unchanged.
type scrubbedError struct{ err error }

func (e scrubbedError) Error() string { return tenantText(e.err.Error()) }
func (e scrubbedError) Unwrap() error { return e.err }

// tenantErr makes an error safe to print. Errors carry portal text (a
// failure message, a mailbox, a status) to wherever the root prints them.
func tenantErr(err error) error {
	if err == nil {
		return nil
	}
	return scrubbedError{err: err}
}

// scrubRunE wraps a command's RunE so every error it returns is safe to
// print. newDownloadCmd applies it to each download subcommand.
func scrubRunE(run func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		return tenantErr(run(cmd, args))
	}
}
