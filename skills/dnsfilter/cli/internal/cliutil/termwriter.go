// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sync"
	"unicode/utf8"
)

// TerminalWriter is the one boundary every stderr write goes through. API
// error bodies, retry notices and warnings embed tenant and server text, and
// a terminal acts on control characters in it: ESC ] 52 rewrites the
// clipboard, CSI moves the cursor or clears the screen, a bare CR overwrites
// the line. The writer rewrites every C0 control except tab and newline (and
// CR directly before a newline), DEL, UTF-8 C1 and lone 8-bit C1 bytes as
// \u00XX. It passes through exactly the SGR sequences the CLI's own color
// helpers emit, so colored output keeps working; those can only change text
// style. Text without controls is written byte for byte.
type TerminalWriter struct {
	stream func() io.Writer
	mu     sync.Mutex
	carry  []byte // an incomplete UTF-8 sequence held for the next Write
}

// allowedSGR lists the sequences bold(), green(), red() and yellow() emit.
var allowedSGR = [][]byte{
	[]byte("\x1b[0m"), []byte("\x1b[1m"), []byte("\x1b[31m"), []byte("\x1b[32m"), []byte("\x1b[33m"),
}

var stderrWriter = &TerminalWriter{stream: func() io.Writer { return os.Stderr }}

// Stderr returns the escaping writer for the process stderr. It resolves
// os.Stderr on every write, so a test that swaps the stream still captures.
func Stderr() io.Writer { return stderrWriter }

// NewTerminalWriter wraps w in the same escaping.
func NewTerminalWriter(w io.Writer) *TerminalWriter {
	return &TerminalWriter{stream: func() io.Writer { return w }}
}

// Unwrap exposes the underlying stream for terminal detection.
func (t *TerminalWriter) Unwrap() io.Writer { return t.stream() }

func (t *TerminalWriter) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	in := p
	if len(t.carry) > 0 {
		in = append(append([]byte{}, t.carry...), p...)
		t.carry = nil
	}
	out := make([]byte, 0, len(in)+16)
	for i := 0; i < len(in); {
		b := in[i]
		switch {
		case b == '\t' || b == '\n':
			out = append(out, b)
			i++
		case b == '\r' && i+1 < len(in) && in[i+1] == '\n':
			out = append(out, '\r', '\n')
			i += 2
		case b == 0x1b:
			if n := allowedSGRLen(in[i:]); n > 0 {
				out = append(out, in[i:i+n]...)
				i += n
				continue
			}
			out = fmt.Appendf(out, `\u%04x`, b)
			i++
		case b < 0x20 || b == 0x7f:
			out = fmt.Appendf(out, `\u%04x`, b)
			i++
		case b < 0x80:
			out = append(out, b)
			i++
		default:
			if !utf8.FullRune(in[i:]) {
				t.carry = append(t.carry, in[i:]...)
				i = len(in)
				continue
			}
			r, size := utf8.DecodeRune(in[i:])
			switch {
			case r == utf8.RuneError && size == 1 && b <= 0x9f:
				out = fmt.Appendf(out, `\u%04x`, b) // lone 8-bit C1
			case r >= 0x80 && r <= 0x9f:
				out = fmt.Appendf(out, `\u%04x`, r)
			default:
				out = append(out, in[i:i+size]...)
			}
			i += size
		}
	}
	if _, err := t.stream().Write(out); err != nil {
		return 0, err
	}
	return len(p), nil
}

func allowedSGRLen(p []byte) int {
	for _, seq := range allowedSGR {
		if bytes.HasPrefix(p, seq) {
			return len(seq)
		}
	}
	return 0
}

// RawStderr is for the CLI's own "\r" progress redraws (resource names and
// counts it generates itself): escaping the CR would break the redraw.
// Never write tenant or API text through it; use Stderr().
func RawStderr() io.Writer { return os.Stderr }
