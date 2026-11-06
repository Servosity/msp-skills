// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// comet-backup handfix (stdin-body-held-whole): every generated command parses
// --stdin into a map, which keeps only the last member of a duplicate key and
// fails on a malformed body, so the request hold would never see the rest. The
// body is held here, before the command reads it, and the command reads the same
// bytes from os.Stdin.

import (
	"io"
	"os"

	"github.com/spf13/cobra"

	"comet-backup-pp-cli/internal/cliutil"
)

func holdStdinBody(cmd *cobra.Command) error {
	f := cmd.Flags().Lookup("stdin")
	if f == nil || f.Value.String() != "true" {
		return nil
	}
	body, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	cliutil.HoldDocument(body)
	// The pipe's two ends are both in this process: nothing leaves it here.
	r, w, err := os.Pipe()
	if err != nil {
		return err
	}
	go func() {
		_, _ = w.Write(body)
		_ = w.Close()
	}()
	os.Stdin = r
	return nil
}
