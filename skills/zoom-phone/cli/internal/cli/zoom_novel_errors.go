// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"io"
	"strings"

	"github.com/spf13/cobra"
)

// zoomNovelCommandPaths are the hand-written commands. Generated endpoint
// commands already print {"error","code"} to stdout under --json; these get the
// same envelope when they fail without writing anything to stdout, so an agent
// reading stdout never sees an empty document. Usage errors (exit 2) keep the
// plain stderr + help behavior.
var zoomNovelCommandPaths = []string{
	"footprint", "offboard", "e911 audit", "numbers inventory", "devices health",
	"callbacks", "routing trace", "routing audit", "fleet",
	"accounts connect", "accounts list", "accounts remove", "accounts path",
}

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		for _, path := range zoomNovelCommandPaths {
			cmd, _, err := root.Find(strings.Fields(path))
			if err != nil || cmd == nil || cmd.CommandPath() != root.Name()+" "+path || cmd.RunE == nil {
				continue
			}
			zoomWrapJSONErrorEnvelope(cmd, flags)
		}
	})
}

type zoomCountingWriter struct {
	w io.Writer
	n int
}

func (c *zoomCountingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += n
	return n, err
}

func zoomWrapJSONErrorEnvelope(cmd *cobra.Command, flags *rootFlags) {
	run := cmd.RunE
	cmd.RunE = func(c *cobra.Command, args []string) error {
		// The envelope is a --json contract only. Wrapping stdout otherwise
		// would hide the terminal from isTerminal (it needs the *os.File), so
		// human tables would never render on a TTY.
		if !flags.asJSON {
			return run(c, args)
		}
		out := &zoomCountingWriter{w: c.OutOrStdout()}
		c.SetOut(out)
		err := run(c, args)
		c.SetOut(out.w)
		if err != nil && out.n == 0 && flags.asJSON {
			if code := ExitCode(err); code != 2 {
				writeAPIErrorEnvelope(out.w, flags, err, code)
			}
		}
		return err
	}
}
