// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"fmt"
	"io"
	"sync"

	"eset-protect-pp-cli/internal/secrets"
)

// c1EscapeWriter wraps the CLI's stdout and stderr and neutralizes every
// byte that would make a terminal act instead of print. A tenant string (a
// device, group or customer name, a quarantined file path) reaches these
// writers through JSON, tables, prose and error text alike, so this is the
// one funnel that covers them all:
//
//   - UTF-8 C1 controls (U+0080-U+009F: CSI, OSC, ST) become \u00XX.
//     encoding/json leaves C1 raw.
//   - C0 controls other than tab and newline (ESC, BEL, backspace, NUL, ...)
//     and DEL become \u00XX, so OSC 52 clipboard writes, ESC[2K line erases
//     and backspace overwrites cannot run. encoding/json already escapes C0,
//     so JSON output is unchanged by this; DEL inside a JSON string becomes
//     \u007f, the same value.
//   - A carriage return passes only as part of CRLF; a bare CR (which
//     returns the cursor so later text overprints the line) becomes \u000d.
//   - The Unicode bidi embedding, override and isolate controls (U+202A-
//     U+202E, U+2066-U+2069) become \u20XX, so a name cannot render reversed.
//
// Every escape is valid inside a JSON string and reads as six harmless
// characters in plain text. The one pass-through: while colors are on
// (--human-friendly), the exact SGR sequences the CLI's own bold, red, green
// and yellow helpers emit. Binary downloads never pass through here; see
// writeBinaryStdout. (handfixes.json: output-escapes-c1-controls,
// output-neutralizes-terminal-controls)
//
// It is also the last line of the credential masking (handfixes.json:
// secret-registry-masks-every-boundary): while the process holds any
// registered secret, text is masked a complete line at a time with
// secrets.Mask before it is escaped, so a credential that a path missed is
// still replaced, and one split across two Writes (a bufio or tabwriter
// flush boundary) is masked whole. The unfinished last line waits in
// pending until its newline arrives or Flush runs; Execute flushes every
// writer it created (flushTerminalWriters) before it returns.
type c1EscapeWriter struct {
	w  io.Writer
	mu sync.Mutex // sync workers can write concurrently
	// carry is the undecided tail of the previous Write: a 0xC2 or 0xE2 lead
	// byte, a CR that may start CRLF, or the start of a style sequence.
	carry []byte
	// pending is the unmasked text after the last complete line.
	pending []byte
}

var (
	terminalWritersMu sync.Mutex
	terminalWriters   []*c1EscapeWriter
)

// newTerminalWriter wraps w and records the writer so flushTerminalWriters
// can emit its pending line when the command ends.
func newTerminalWriter(w io.Writer) *c1EscapeWriter {
	tw := &c1EscapeWriter{w: w}
	terminalWritersMu.Lock()
	terminalWriters = append(terminalWriters, tw)
	terminalWritersMu.Unlock()
	return tw
}

// flushTerminalWriters flushes, and forgets, every writer newTerminalWriter
// made.
func flushTerminalWriters() {
	terminalWritersMu.Lock()
	ws := terminalWriters
	terminalWriters = nil
	terminalWritersMu.Unlock()
	for _, w := range ws {
		_ = w.Flush()
	}
}

// maskUnit masks one unit of complete lines. When the unit cannot be masked
// in place it is masked line by line, so only the lines that hold an
// encoded credential are withheld and the output does not depend on how
// the text was split into Writes. (A credential whose raw form holds a line
// break is masked with its whole unit; HoldBack keeps it in one.)
func maskUnit(unit []byte) []byte {
	m := secrets.MaskBytes(unit)
	if string(m) != secrets.Withheld {
		return m
	}
	if secrets.HasLineBreakSecret() || bytes.Count(unit, []byte("\n")) < 2 {
		if bytes.HasSuffix(unit, []byte("\n")) {
			m = append(m, '\n')
		}
		return m
	}
	var out []byte
	for _, line := range bytes.SplitAfter(unit, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		ml := secrets.MaskBytes(line)
		if string(ml) == secrets.Withheld && bytes.HasSuffix(line, []byte("\n")) {
			ml = append(ml, '\n')
		}
		out = append(out, ml...)
	}
	return out
}

// Flush masks and writes the pending partial line and any undecided tail.
func (c *c1EscapeWriter) Flush() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.flushLocked()
}

func (c *c1EscapeWriter) flushLocked() error {
	if len(c.pending) > 0 {
		unit := maskUnit(c.pending)
		c.pending = nil
		if err := c.escape(unit); err != nil {
			return err
		}
	}
	if len(c.carry) > 0 {
		tail := c.carry
		c.carry = nil
		for _, b := range tail {
			if _, err := fmt.Fprintf(c.w, `\u%04x`, b); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *c1EscapeWriter) Unwrap() io.Writer { return c.w }

// cliStyleSequences are the SGR sequences bold, red, green and yellow emit
// (helpers.go). TestTerminalWriterKeepsCLIStyles ties this list to them.
var cliStyleSequences = [][]byte{
	[]byte("\x1b[0m"),
	[]byte("\x1b[1m"),
	[]byte("\x1b[31m"),
	[]byte("\x1b[32m"),
	[]byte("\x1b[33m"),
}

// matchCLIStyle reports the length of the CLI style sequence at the start of
// p, or partial=true when p is a strict prefix of one (split across Writes).
func matchCLIStyle(p []byte) (n int, partial bool) {
	for _, seq := range cliStyleSequences {
		if bytes.HasPrefix(p, seq) {
			return len(seq), false
		}
		if len(p) < len(seq) && bytes.HasPrefix(seq, p) {
			partial = true
		}
	}
	return 0, partial
}

// isBidiControl reports whether the UTF-8 sequence E2 b1 b2 is one of the
// bidi embedding/override (U+202A-U+202E) or isolate (U+2066-U+2069) controls.
func isBidiControl(b1, b2 byte) bool {
	return (b1 == 0x80 && b2 >= 0xAA && b2 <= 0xAE) || (b1 == 0x81 && b2 >= 0xA6 && b2 <= 0xA9)
}

func (c *c1EscapeWriter) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	unit := p
	if secrets.Active() || len(c.pending) > 0 {
		data := append(append(make([]byte, 0, len(c.pending)+len(p)), c.pending...), p...)
		cut := secrets.HoldBack(data, bytes.LastIndexByte(data, '\n')+1)
		c.pending = append([]byte(nil), data[cut:]...)
		unit = nil
		if cut > 0 {
			unit = maskUnit(data[:cut])
		}
	}
	if err := c.escape(unit); err != nil {
		return 0, err
	}
	return len(p), nil
}

// escape neutralizes p (after any carried tail) and writes it.
func (c *c1EscapeWriter) escape(p []byte) error {
	if len(p) == 0 {
		return nil
	}
	data := p
	if len(c.carry) > 0 {
		data = append(append(make([]byte, 0, len(c.carry)+len(p)), c.carry...), p...)
		c.carry = nil
	}
	styles := colorEnabled()
	buf := make([]byte, 0, len(data)+16)
	for i := 0; i < len(data); i++ {
		b := data[i]
		rest := len(data) - i
		switch {
		case b == 0xC2: // in UTF-8, 0xC2 is only ever a lead byte
			if rest == 1 {
				c.carry = []byte{b}
				continue
			}
			if n := data[i+1]; n >= 0x80 && n <= 0x9f {
				buf = fmt.Appendf(buf, `\u%04x`, n)
				i++
				continue
			}
			buf = append(buf, b)
		case b == 0xE2:
			if rest < 3 {
				c.carry = append([]byte(nil), data[i:]...)
				i = len(data)
				continue
			}
			if isBidiControl(data[i+1], data[i+2]) {
				r := rune(b&0x0f)<<12 | rune(data[i+1]&0x3f)<<6 | rune(data[i+2]&0x3f)
				buf = fmt.Appendf(buf, `\u%04x`, r)
				i += 2
				continue
			}
			buf = append(buf, b)
		case b == '\r':
			if rest == 1 {
				c.carry = []byte{b}
				continue
			}
			if data[i+1] == '\n' {
				buf = append(buf, b)
				continue
			}
			buf = fmt.Appendf(buf, `\u%04x`, b)
		case b == 0x1b && styles:
			n, partial := matchCLIStyle(data[i:])
			if n > 0 {
				buf = append(buf, data[i:i+n]...)
				i += n - 1
				continue
			}
			if partial {
				c.carry = append([]byte(nil), data[i:]...)
				i = len(data)
				continue
			}
			buf = fmt.Appendf(buf, `\u%04x`, b)
		case (b < 0x20 && b != '\t' && b != '\n') || b == 0x7f:
			buf = fmt.Appendf(buf, `\u%04x`, b)
		default:
			buf = append(buf, b)
		}
	}
	_, err := c.w.Write(buf)
	return err
}

// writeRaw sends p to the underlying writer untouched. Only binary payloads
// use it (writeBinaryStdout); the pending line and any carried tail are
// flushed first, masked and escaped.
func (c *c1EscapeWriter) writeRaw(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.flushLocked(); err != nil {
		return 0, err
	}
	return c.w.Write(p)
}
