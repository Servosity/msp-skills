// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Package termio owns the process-wide escaping writers for stdout and
// stderr. Every package that writes to the terminal (the CLI, the HTTP
// client's retry and dry-run notes, the store's and config's warnings, the
// MCP server's log lines) writes through Stdout() / Stderr(), never through
// os.Stdout / os.Stderr directly; TestNoDirectTerminalWrites in internal/cli
// fails on any new direct write anywhere in the module.
// HAND-FIX(output-escapes-c1-controls)
package termio

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"unicode/utf8"
)

// Writer rewrites terminal control characters as \u00XX on their way to
// stdout and stderr: UTF-8 C1 controls (U+0080-U+009F), which encoding/json
// emits raw and xterm/VTE act on (CSI, OSC, ST); lone 8-bit C1 bytes from a
// non-UTF-8 body, which 8-bit terminals act on the same way; and the C0
// controls and DEL (0x00-0x1f except tab, newline and CR before newline;
// 0x7f), which reach the terminal raw through human output, notes, warnings,
// dry-run bodies and error messages that embed tenant strings (ESC, BEL,
// OSC 52 clipboard writes, a bare CR that overwrites the line). JSON output
// never carries a raw C0 byte (encoding/json escapes them), so JSON stays
// valid and lossless; in plain text the escape shows as six harmless
// characters. Exactly the SGR sequences the CLI's own color helpers emit
// (allowedSGR) pass through, so --human-friendly keeps its colors; any other
// ESC sequence, SGR included, is escaped.
type Writer struct {
	w       io.Writer
	stream  func() io.Writer // resolved per write, so tests that swap os.Stderr still capture
	mu      sync.Mutex
	pending []byte // an incomplete UTF-8 sequence or a lone CR that ended the previous Write
}

// New wraps w in an escaping writer (tests and in-memory captures).
func New(w io.Writer) *Writer { return &Writer{w: w} }

func (c *Writer) target() io.Writer {
	if c.stream != nil {
		return c.stream()
	}
	return c.w
}

// stdoutEsc and stderrEsc are the process-wide escaping writers. Sharing one
// instance per stream keeps the split-sequence state consistent across packages.
var (
	stdoutEsc = &Writer{stream: func() io.Writer { return os.Stdout }}
	stderrEsc = &Writer{stream: func() io.Writer { return os.Stderr }}
)

// Progress redraws a CLI-owned progress line on stderr. The leading CR is the
// CLI's own (it returns to column 0 so the next count overwrites the last);
// the text is escaped like every other write, so a bare CR inside it cannot
// overwrite anything. HAND-FIX(sync-progress-redraw)
func Progress(format string, args ...any) {
	stderrEsc.mu.Lock()
	defer stderrEsc.mu.Unlock()
	_, _ = io.WriteString(stderrEsc.target(), "\r"+EscapeString(fmt.Sprintf(format, args...)))
}

// Stdout is the escaping writer over os.Stdout.
func Stdout() io.Writer { return stdoutEsc }

// Stderr is the escaping writer over os.Stderr.
func Stderr() io.Writer { return stderrEsc }

// Unwrap exposes the underlying stream so terminal detection still works.
func (c *Writer) Unwrap() io.Writer { return c.target() }

// allowedSGR lists exactly the style sequences the CLI's own color helpers
// emit (bold, green, red, yellow, bold red, reset). A tenant string carrying
// any other SGR (conceal, 256-color, blink) is escaped like every other ESC.
var allowedSGR = map[string]bool{
	"\x1b[0m": true, "\x1b[1m": true, "\x1b[31m": true, "\x1b[32m": true, "\x1b[33m": true, "\x1b[1;31m": true,
}

// sgrSequenceLen returns the length of the allowed SGR sequence at the start
// of p (ESC '[' then only digits and ';' then 'm', and in allowedSGR), or 0.
func sgrSequenceLen(p []byte) int {
	if len(p) < 3 || p[0] != 0x1b || p[1] != '[' {
		return 0
	}
	for i := 2; i < len(p) && i < 8; i++ {
		switch {
		case p[i] == 'm':
			if allowedSGR[string(p[:i+1])] {
				return i + 1
			}
			return 0
		case p[i] >= '0' && p[i] <= '9', p[i] == ';':
			continue
		default:
			return 0
		}
	}
	return 0
}

func isRawC0(b byte) bool {
	return (b < 0x20 && b != '\t' && b != '\n' && b != '\r') || b == 0x7f
}

func (c *Writer) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := len(p)
	if len(c.pending) > 0 {
		p = append(append(make([]byte, 0, len(c.pending)+len(p)), c.pending...), p...)
		c.pending = c.pending[:0]
	}
	buf := make([]byte, 0, len(p)+8)
	for i := 0; i < len(p); {
		b := p[i]
		switch {
		case b == 0x1b:
			if l := sgrSequenceLen(p[i:]); l > 0 {
				buf = append(buf, p[i:i+l]...)
				i += l
				continue
			}
			buf = fmt.Appendf(buf, `\u%04x`, b)
			i++
		case b == '\r':
			if i+1 == len(p) {
				c.pending = append(c.pending, b) // decided once the next byte arrives
				i++
				continue
			}
			if p[i+1] == '\n' {
				buf = append(buf, '\r', '\n')
				i += 2
				continue
			}
			buf = append(buf, `\u000d`...) // a bare CR overwrites the line
			i++
		case isRawC0(b):
			buf = fmt.Appendf(buf, `\u%04x`, b)
			i++
		case b < utf8.RuneSelf:
			buf = append(buf, b)
			i++
		default:
			if !utf8.FullRune(p[i:]) {
				c.pending = append(c.pending, p[i:]...) // an incomplete sequence ends this Write
				i = len(p)
				continue
			}
			r, size := utf8.DecodeRune(p[i:])
			if r == utf8.RuneError && size == 1 {
				// not UTF-8: a lone 8-bit byte; 0x80-0x9F are C1 controls on 8-bit terminals
				if b <= 0x9f {
					buf = fmt.Appendf(buf, `\u%04x`, b)
				} else {
					buf = append(buf, b)
				}
				i++
				continue
			}
			if r <= 0x9f { // U+0080-U+009F
				buf = fmt.Appendf(buf, `\u%04x`, r)
				i += size
				continue
			}
			buf = append(buf, p[i:i+size]...)
			i += size
		}
	}
	if len(buf) > 0 {
		if _, err := c.target().Write(buf); err != nil {
			return 0, err
		}
	}
	return n, nil
}

// EscapeString rewrites C0 controls (except tab, newline and CR before
// newline), DEL and C1 controls (U+0080-U+009F) in s as \u00XX. MCP tool
// results carry tenant strings (client, device and job names, notes) straight
// from API JSON and the local store, and a host that prints a result to a
// terminal would otherwise execute an OSC 52 clipboard write or a CSI
// sequence carried in a name, or let a bare CR overwrite the line. Every ESC
// is escaped here (an MCP result has no colors of its own); the escape keeps
// JSON text valid and lossless.
func EscapeString(s string) string {
	if !strings.ContainsFunc(s, isTerminalControl) && !strings.Contains(s, "\r") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 16)
	for i, r := range s {
		if isTerminalControl(r) || (r == '\r' && (i+1 >= len(s) || s[i+1] != '\n')) {
			b.WriteString(`\u00`)
			b.WriteByte("0123456789abcdef"[r>>4])
			b.WriteByte("0123456789abcdef"[r&0xf])
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func isTerminalControl(r rune) bool {
	return (r < 0x20 && r != '\t' && r != '\n' && r != '\r') || r == 0x7f || (r >= 0x80 && r <= 0x9f)
}
