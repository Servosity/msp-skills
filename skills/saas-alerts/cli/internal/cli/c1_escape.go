// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"io"
)

// c1EscapeWriter rewrites UTF-8 C1 control characters (U+0080-U+009F) as
// \u00XX. encoding/json escapes C0 controls but emits C1 raw, and xterm and
// VTE terminals act on UTF-8 C1 (CSI, OSC, ST), so a tenant string (an
// event's mailbox rule or file name, a user name) in JSON output could drive
// the terminal. The escape is valid inside a JSON string, and C1 never appears
// outside one; in plain text it shows as six harmless characters. Table,
// CSV, plain and quiet output already drop C1 (cliutil.ScrubTerminal).
type c1EscapeWriter struct {
	w       io.Writer
	pending bool // the previous Write ended on a 0xC2 lead byte
}

func (c *c1EscapeWriter) Unwrap() io.Writer { return c.w }

func (c *c1EscapeWriter) Write(p []byte) (int, error) {
	buf := make([]byte, 0, len(p)+8)
	i := 0
	if c.pending {
		c.pending = false
		if len(p) > 0 && p[0] >= 0x80 && p[0] <= 0x9f {
			buf = fmt.Appendf(buf, `\u%04x`, p[0])
			i = 1
		} else {
			buf = append(buf, 0xC2)
		}
	}
	for ; i < len(p); i++ {
		if p[i] == 0xC2 { // in UTF-8, 0xC2 is only ever a lead byte
			if i+1 == len(p) {
				c.pending = true
				continue
			}
			if n := p[i+1]; n >= 0x80 && n <= 0x9f {
				buf = fmt.Appendf(buf, `\u%04x`, n)
				i++
				continue
			}
		}
		buf = append(buf, p[i])
	}
	if _, err := c.w.Write(buf); err != nil {
		return 0, err
	}
	return len(p), nil
}
