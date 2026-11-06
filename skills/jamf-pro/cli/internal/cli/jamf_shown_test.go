// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// terminalBuffer answers the CLI's terminal check with yes.
type terminalBuffer struct{ *bytes.Buffer }

func (terminalBuffer) TerminalOverride() bool { return true }

// Class one-rule-everywhere: every generated formatter shows only what the
// one credential rule leaves, in every format, the human table and cards a
// terminal gets included (the black-box runs are not terminals).
func TestEveryFormatterAppliesTheOneRule(t *testing.T) {
	const pin, script = "864209", "ScriptOnly-7abc"
	rows := `[{"id":"1","name":"HQ-Shown","pin":` + pin + `,"extensionAttributes":[{"name":"Device PIN","values":["` + pin + `"]}],"scriptContents":"password=` + script + `"}]`
	wide := `[{"id":"1","name":"HQ-Shown","pin":` + pin + `,"a":"1","b":"2","c":"3","d":"4","e":"5","f":"6","g":"7"}]`
	tty := func(b *bytes.Buffer) terminalBuffer { return terminalBuffer{b} }
	cases := map[string]func(*bytes.Buffer) error{
		"table (terminal)": func(b *bytes.Buffer) error { return printOutput(tty(b), json.RawMessage(rows), false) },
		"cards (terminal)": func(b *bytes.Buffer) error { return printOutput(tty(b), json.RawMessage(wide), false) },
		"json":             func(b *bytes.Buffer) error { return printOutput(b, json.RawMessage(rows), true) },
		"csv":              func(b *bytes.Buffer) error { return printCSV(b, json.RawMessage(rows)) },
		"plain":            func(b *bytes.Buffer) error { return printPlain(b, json.RawMessage(rows)) },
		// --quiet prints one identity per row; "key" is one of them.
		"quiet": func(b *bytes.Buffer) error {
			return printQuiet(b, json.RawMessage(`[{"key":"`+script+`","name":"HQ-Shown"}]`))
		},
		// --select can keep a record's value and drop its label: the rule
		// runs before the projection.
		"select": func(b *bytes.Buffer) error {
			out, err := filterFieldsChecked(json.RawMessage(`{"id":"1","name":"HQ-Shown","general":{"extensionAttributes":[{"name":"Device PIN","values":["`+pin+`"]}]}}`), "name,general.extensionAttributes.values")
			b.Write(out)
			return err
		},
		"auto table": func(b *bytes.Buffer) error {
			var items []map[string]any
			_ = json.Unmarshal([]byte(rows), &items)
			return printAutoTable(tty(b), items)
		},
	}
	for name, run := range cases {
		var b bytes.Buffer
		if err := run(&b); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		out := b.String()
		if strings.Contains(out, pin) || strings.Contains(out, script) {
			t.Errorf("%s shows a credential the one rule redacts:\n%s", name, out)
		}
		if name != "quiet" && !strings.Contains(out, "HQ-Shown") || name == "quiet" && !strings.Contains(out, "[redacted]") {
			t.Errorf("%s lost the record itself:\n%s", name, out)
		}
	}
}
