// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

// TestDeliverTeeIsSerialized (handfixes.json: deliver-tee-serialized): the
// --deliver tee is an io.MultiWriter over the terminal writer and deliverBuf,
// and concurrent sync workers write progress through it; only the terminal
// writer had its own lock, so deliverBuf raced (go test -race). The tee must
// be wrapped in lockedWriter.
func TestDeliverTeeIsSerialized(t *testing.T) {
	root := RootCmd()
	var seen bool
	for _, c := range root.Commands() {
		if c.Name() != "version" {
			continue
		}
		c.Run = nil
		c.RunE = func(cmd *cobra.Command, args []string) error {
			_, seen = cmd.OutOrStdout().(*lockedWriter)
			return nil
		}
	}
	root.SetArgs([]string{"version", "--deliver", "file:" + filepath.Join(t.TempDir(), "out.txt")})
	_ = root.Execute()
	if !seen {
		t.Fatal("--deliver output is not serialized through lockedWriter")
	}
}
