// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/synology/handfixes.json
// under the "tabular-printers-scrub-terminal" entry.

package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// hostileName is a task name a NAS could return: an OSC 52 clipboard write
// in UTF-8 C1 form (U+009D ... U+009C), the same in 7-bit form (ESC ] ... BEL)
// and a screen-clearing CSI.
const hostileName = "PC-\u009d52;c;QQ==\u009c-\x1b]52;c;SGk=\x07-\x1b[2J"

func assertNoTerminalControls(t *testing.T, what, out string) {
	t.Helper()
	if strings.ContainsAny(out, "\x1b\x07") {
		t.Fatalf("%s carries a raw ESC or BEL: %q", what, out)
	}
	for _, r := range out {
		if r >= 0x80 && r <= 0x9f {
			t.Fatalf("%s carries a raw C1 control %U: %q", what, r, out)
		}
	}
}

func TestTabularPrintersScrubTerminalControls(t *testing.T) {
	rows, _ := json.Marshal([]map[string]any{{"task_id": 5, "task_name": hostileName, "devices": []any{map[string]any{"host_name": hostileName}}}})
	obj, _ := json.Marshal(map[string]any{"name": hostileName, "ok": true})
	scalars, _ := json.Marshal([]any{hostileName, 5})
	for name, print := range map[string]func(*bytes.Buffer) error{
		"csv rows":     func(b *bytes.Buffer) error { return printCSV(b, rows) },
		"plain rows":   func(b *bytes.Buffer) error { return printPlain(b, rows) },
		"quiet object": func(b *bytes.Buffer) error { return printQuiet(b, obj) },
		"quiet scalar": func(b *bytes.Buffer) error { return printQuiet(b, scalars) },
		"csv object":   func(b *bytes.Buffer) error { return printCSV(b, obj) },
		"plain object": func(b *bytes.Buffer) error { return printPlain(b, obj) },
	} {
		var b bytes.Buffer
		if err := print(&b); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		assertNoTerminalControls(t, name, b.String())
		if !strings.Contains(b.String(), "PC-") {
			t.Fatalf("%s lost the printable text: %q", name, b.String())
		}
	}
}
