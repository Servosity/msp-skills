// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"io"
	"unicode/utf8"

	"dnsfilter-pp-cli/internal/cliutil"
)

// c1EscapeWriter rewrites C1 control characters, UTF-8 encoded (U+0080-U+009F)
// or a lone 8-bit byte 0x80-0x9F, and the Unicode bidi embedding, override and
// isolate controls (U+202A-U+202E, U+2066-U+2069) as \u00XX / \u20XX.
// encoding/json escapes C0 controls but emits C1 and bidi controls raw, and a
// raw API body passed through to --json can carry a lone 8-bit C1 byte; xterm
// and VTE act on UTF-8 C1 (CSI, OSC, ST), an 8-bit terminal on the lone byte,
// and bidi controls reorder what the operator reads (Trojan Source). So a
// tenant string (an organization, site or roaming-client name, a policy name)
// in JSON output could drive or spoof the terminal. The escape is valid inside
// a JSON string and decodes to the same text; in plain text it shows as six
// harmless characters. Every other byte, including C0 such as a CSV cell's
// quoted CR and every valid multibyte sequence, is written unchanged. Table,
// CSV, plain and quiet output and the novel commands' human output also drop
// or escape these controls themselves (cliutil.ScrubTerminal, scrubCell).
type c1EscapeWriter struct {
	w     io.Writer
	carry []byte // an incomplete UTF-8 sequence held for the next Write
}

func (c *c1EscapeWriter) Unwrap() io.Writer { return c.w }

func (c *c1EscapeWriter) Write(p []byte) (int, error) {
	in := p
	if len(c.carry) > 0 {
		in = append(append([]byte{}, c.carry...), p...)
		c.carry = nil
	}
	buf := make([]byte, 0, len(in)+8)
	for i := 0; i < len(in); {
		b := in[i]
		if b < 0x80 {
			buf = append(buf, b)
			i++
			continue
		}
		if !utf8.FullRune(in[i:]) {
			c.carry = append(c.carry, in[i:]...)
			break
		}
		r, size := utf8.DecodeRune(in[i:])
		switch {
		case r == utf8.RuneError && size == 1 && b <= 0x9f:
			buf = fmt.Appendf(buf, `\u%04x`, b) // lone 8-bit C1
		case (r >= 0x80 && r <= 0x9f) || cliutil.IsBidiControl(r):
			buf = fmt.Appendf(buf, `\u%04x`, r)
		default:
			buf = append(buf, in[i:i+size]...)
		}
		i += size
	}
	if _, err := c.w.Write(buf); err != nil {
		return 0, err
	}
	return len(p), nil
}

// stderrW is the escaping stderr every CLI write goes through: C0 (except tab
// and newline), DEL and C1 controls in API error text and warnings are
// rewritten as \u00XX, and only the CLI's own colors pass. See
// cliutil.TerminalWriter.
func stderrW() io.Writer { return cliutil.Stderr() }
