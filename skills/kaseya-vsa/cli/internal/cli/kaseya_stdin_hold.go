// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Hand-written (NOT generated; handfixes.json body-credential-fields): the
// handlers parse a stdin body before any request is built, and a parse error
// can name what it rejects. The kit's startup read holds a piped --stdin body
// but skips a terminal, which could block forever, and it does not know
// import's --input -. Once the command is known this reads those bodies, holds
// them with the kit's own body reader (every duplicate key, every line, and
// every word of bytes that are no document), and leaves stdin holding the
// same bytes.

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"kaseya-vsa-pp-cli/internal/credkit"
)

func holdStdinBody(cmd *cobra.Command) error {
	if !readsStdinBodyUnheld(cmd) {
		return nil
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("reading stdin: %w", err)
	}
	credkit.HoldBody(data)
	r, w, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("reading stdin: %w", err)
	}
	go func() {
		_, _ = w.Write(data)
		_ = w.Close()
	}()
	os.Stdin = r
	return nil
}

func readsStdinBodyUnheld(cmd *cobra.Command) bool {
	if cmd.Name() == "import" {
		f := cmd.Flags().Lookup("input")
		return f != nil && (f.Value.String() == "-" || f.Value.String() == "")
	}
	f := cmd.Flags().Lookup("stdin")
	if f == nil || f.Value.String() != "true" {
		return false
	}
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
