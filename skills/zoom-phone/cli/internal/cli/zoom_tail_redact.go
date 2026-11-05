// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"io"

	"github.com/spf13/cobra"

	"zoom-phone-pp-cli/internal/store"
)

// `tail` is a read-only generic poller over list endpoints, and the
// alert-settings list carries Team Chat channel tokens. Its NDJSON output is
// passed through the same credential redaction the store applies, so the
// read-only tool (fleet, MCP read-only mode) never prints a secret.
func init() {
	registerNovelCommand(func(root *cobra.Command, _ *rootFlags) {
		cmd := zoomFindExact(root, "tail")
		if cmd == nil || cmd.RunE == nil {
			return
		}
		run := cmd.RunE
		cmd.RunE = func(c *cobra.Command, args []string) error {
			w := &zoomRedactingLineWriter{w: c.OutOrStdout()}
			c.SetOut(w)
			err := run(c, args)
			if flushErr := w.Flush(); err == nil {
				err = flushErr
			}
			c.SetOut(w.w)
			return err
		}
	})
}

// zoomRedactingLineWriter redacts each complete line that parses as JSON and
// passes other lines through unchanged.
type zoomRedactingLineWriter struct {
	w   io.Writer
	buf bytes.Buffer
}

func (z *zoomRedactingLineWriter) Write(p []byte) (int, error) {
	z.buf.Write(p)
	for {
		line, err := z.buf.ReadBytes('\n')
		if err != nil {
			z.buf.Reset()
			z.buf.Write(line) // incomplete line: keep for the next write
			return len(p), nil
		}
		if _, werr := z.w.Write(zoomRedactLine(line)); werr != nil {
			return len(p), werr
		}
	}
}

func (z *zoomRedactingLineWriter) Flush() error {
	if z.buf.Len() == 0 {
		return nil
	}
	_, err := z.w.Write(zoomRedactLine(z.buf.Bytes()))
	z.buf.Reset()
	return err
}

func zoomRedactLine(line []byte) []byte {
	body := bytes.TrimRight(line, "\r\n")
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || (trimmed[0] != '{' && trimmed[0] != '[') {
		return line
	}
	out := store.RedactSecrets(body)
	if len(line) > len(body) {
		out = append(append([]byte{}, out...), line[len(body):]...)
	}
	return out
}
