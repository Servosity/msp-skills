// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"eset-protect-pp-cli/internal/client"
	"github.com/spf13/cobra"
)

// writeBinaryStdout writes a download response (quarantined-objects-download,
// quarantined-objects-batch-download) to stdout. A binary payload arrives as
// the client's base64 envelope; it is decoded and written byte for byte past
// the terminal-control writer, which would otherwise rewrite ESC, BEL, CR and
// C1 byte pairs inside the file. A terminal is refused instead: a quarantined
// sample is attacker-chosen bytes. Anything that is not a binary envelope is
// ordinary text and takes the normal, escaped path. With a webhook --deliver
// sink the envelope is kept, since that sink sends JSON. (handfixes.json:
// binary-downloads-bypass-terminal-writer)
func writeBinaryStdout(cmd *cobra.Command, flags *rootFlags, data json.RawMessage) error {
	out := cmd.OutOrStdout()
	raw, _, ok := client.UnwrapBinaryResponse(data)
	if !ok || (flags != nil && flags.deliverBuf != nil) {
		_, err := out.Write(data)
		return err
	}
	var rawOut func([]byte) (int, error)
	switch w := out.(type) {
	case *c1EscapeWriter:
		rawOut = w.writeRaw
	case *os.File:
		rawOut = w.Write
	default:
		// An unknown wrapper may itself rewrite bytes; the envelope is
		// ASCII and decodes to the exact payload.
		_, err := out.Write(data)
		return err
	}
	// Only a real terminal is refused (isTerminal: termios or a Windows
	// console), so '> /dev/null' and '> NUL' work (handfixes.json:
	// terminal-check-is-a-real-tty). --deliver file: leads the advice: it is
	// byte-exact on every OS, while Windows PowerShell 5.1 re-encodes native
	// output on '>' and corrupts a binary file.
	if isTerminal(out) {
		return usageErr(fmt.Errorf("%s", binaryTerminalRefusal(len(raw))))
	}
	_, err := rawOut(raw)
	return err
}

// binaryTerminalRefusal is the error for a binary download whose stdout is a
// terminal.
func binaryTerminalRefusal(n int) string {
	return fmt.Sprintf("refusing to write a %d-byte binary download to a terminal; save it with --deliver file:<path> (byte-exact on every OS; in Windows PowerShell 5.1, '>' re-encodes binary output and corrupts the file), or on macOS and Linux redirect stdout to a file (> sample.zip), or add --json for the base64 envelope", n)
}
