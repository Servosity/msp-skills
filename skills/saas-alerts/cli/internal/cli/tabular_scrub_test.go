// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/saas-alerts/handfixes.json
// under the "tabular-printers-scrub-terminal" entry.

package cli

import (
	"bytes"
	"strings"
	"testing"
)

// TestTabularPrintersScrubTerminal: an attacker-influenced event field (a
// mailbox rule or file name) must not reach the terminal as ESC, BEL or C1.
func TestTabularPrintersScrubTerminal(t *testing.T) {
	data := []byte(`[{"id":"c1","name":"evil\u001b]0;PWNED\u0007\u001b[2Jrule\u009b31m"}]`)
	for name, print := range map[string]func(*bytes.Buffer) error{
		"csv":   func(b *bytes.Buffer) error { return printCSV(b, data) },
		"plain": func(b *bytes.Buffer) error { return printPlain(b, data) },
		"quiet": func(b *bytes.Buffer) error { return printQuiet(b, data) },
	} {
		var b bytes.Buffer
		if err := print(&b); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if strings.ContainsAny(b.String(), "\x1b\x07\u009b") {
			t.Fatalf("%s output carries terminal controls: %q", name, b.String())
		}
	}
}
