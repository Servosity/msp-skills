// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json export-hands-the-kit-whole-records).
//
// The credential kit checks what the program writes as units: each Write to a
// file it wraps, and each run of stdout or stderr between pauses. It checks a
// unit with the stream's tail as context and keeps a JSON document open across
// units; when an open document outgrows its 16 KiB window the stream is lost and
// every later unit is withheld. So output reaches the kit as whole records,
// never in chunks cut at a buffer size: export batches whole records and their
// newlines (export.go exportRecords), and tables go through wholeLines.
// TestNoChunkingWritersOverKitSinks keeps bufio, csv, io.Copy and a bare
// tabwriter out of the module.

package cli

import (
	"bytes"
	"io"
	"text/tabwriter"
)

// wholeLines hands w whole lines, one Write each: a line that has not ended
// waits for its newline, and Flush writes whatever is left.
type wholeLines struct {
	w       io.Writer
	pending []byte
}

func (l *wholeLines) Write(p []byte) (int, error) {
	l.pending = append(l.pending, p...)
	start := 0
	for {
		i := bytes.IndexByte(l.pending[start:], '\n')
		if i < 0 {
			break
		}
		end := start + i + 1
		if _, err := l.w.Write(l.pending[start:end]); err != nil {
			l.pending = append(l.pending[:0], l.pending[end:]...)
			return 0, err
		}
		start = end
	}
	l.pending = append(l.pending[:0], l.pending[start:]...)
	return len(p), nil
}

// Flush writes the last line when it did not end in a newline.
func (l *wholeLines) Flush() error {
	if len(l.pending) == 0 {
		return nil
	}
	_, err := l.w.Write(l.pending)
	l.pending = l.pending[:0]
	return err
}

// tableWriter is a tabwriter whose aligned output reaches its writer one whole
// line per Write (a tabwriter writes each cell and its padding separately).
type tableWriter struct {
	*tabwriter.Writer
	lines *wholeLines
}

// Flush aligns and writes the table, then a last line that did not end.
func (t *tableWriter) Flush() error {
	if err := t.Writer.Flush(); err != nil {
		return err
	}
	return t.lines.Flush()
}

// newTableWriter is the CLI's table layout (two-column minimum, padding two).
func newTableWriter(w io.Writer) *tableWriter {
	lines := &wholeLines{w: w}
	return &tableWriter{Writer: tabwriter.NewWriter(lines, 2, 4, 2, ' ', 0), lines: lines}
}
