// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// Tenant names reach --json and --agent output; encoding/json writes UTF-8 C1
// controls raw, and a terminal acts on them (handfixes.json:
// output-escapes-c1-controls, native from press 4.35.1 through
// EscapeJSONControls). Piped output must stay escaped and lossless.
func TestJSONOutputEscapesC1Controls(t *testing.T) {
	name := "Evil\u009b31mCo\u001b]52;c;eA==\u0007"
	for _, agent := range []bool{false, true} {
		var out bytes.Buffer
		flags := &rootFlags{asJSON: true, agent: agent, compact: agent}
		if err := printJSONFiltered(&out, []map[string]any{{"name": name, "endpoints": 3}}, flags); err != nil {
			t.Fatal(err)
		}
		if strings.ContainsAny(out.String(), "\u009b\x1b\x07") {
			t.Fatalf("agent=%v: raw control in JSON output: %q", agent, out.String())
		}
		var v any
		if err := json.Unmarshal(out.Bytes(), &v); err != nil || !strings.Contains(out.String(), `\u009b31mCo\u001b]52`) {
			t.Fatalf("agent=%v: JSON not valid and lossless: %q (%v)", agent, out.String(), err)
		}
	}
}
