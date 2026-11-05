// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "secrets-never-persisted" entry (Codex round 17): the read-only
// filter stays at least as strict as the store scrub on the round-17 classes.

package redact

import (
	"encoding/json"
	"math/rand"
	"strings"
	"testing"

	"dnsfilter-pp-cli/internal/cliutil"
	"dnsfilter-pp-cli/internal/store"
)

const r17Marker = "SNTLR17"

// r17Texts are credential shapes from Codex round 17: a secret name glued to a
// digit or punctuation, a header cell longer than 64 bytes, DNSFilter's NKEY
// deploy property (msiexec and plist forms), and a JSON key the text rule
// cannot read after more stray brackets than the decode budget.
func r17Texts() []string {
	return []string{
		"Uninstall PIN2: " + r17Marker, "Uninstall PIN.2: " + r17Marker, "Uninstall PIN. HQ: " + r17Marker,
		"uninstall_pin_2=" + r17Marker, "API Key2 (prod): " + r17Marker,
		"Site,roaming_client_windows_headquarters_network_installation_secret_key\nHQ," + r17Marker,
		"NKEY=" + r17Marker, "NKEY: " + r17Marker, `NKEY="` + r17Marker + `"`,
		`msiexec /qn /i DNSFilter_Agent_Setup.msi NKEY="` + r17Marker + `"`,
		"<dict>\n\t<key>NKEY</key>\n\t<string>" + r17Marker + "</string>\n</dict>",
		strings.Repeat("[", maxFailedSegmentDecodes) + `{"p a s s w o r d":"` + r17Marker + `"}`,
		strings.Repeat("[x ", maxFailedSegmentDecodes+40) + `{"p a s s w o r d":"` + r17Marker + `"}`,
	}
}

// Every round-17 shape is withheld by read-only MCP as a whole result, as a
// string value in a JSON result, and as JSON in a string, and the marker never
// reaches the client.
func TestReadOnlyWithholdsRound17Shapes(t *testing.T) {
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	for _, text := range r17Texts() {
		resetWithheldMemory()
		for _, in := range []string{
			text,
			`{"note":` + quoteJSON(text) + `}`,
			`{"note":` + quoteJSON(`{"memo":`+quoteJSON(text)+`}`) + `}`,
			`[{"id":1,"note":` + quoteJSON(text) + `}]`,
		} {
			if out := Final(in); strings.Contains(out, r17Marker) {
				t.Errorf("Final kept %.120q: %.200s", in, out)
			}
			if out := Final(Raw(in)); strings.Contains(out, r17Marker) {
				t.Errorf("Final(Raw) kept %.120q: %.200s", in, out)
			}
		}
	}
	for _, key := range []string{"NKEY", "PIN2", "uninstall_pin_2"} {
		if !IsCredentialKey(key) {
			t.Errorf("IsCredentialKey(%q) = false, want true", key)
		}
	}
	// The CLI's own hint and an ordinary bracket-heavy text stay readable.
	for _, keep := range []string{
		`hint: check your API key. Set your API key with: export DNSFILTER_API_KEY="your-token-here" (PowerShell: $env:DNSFILTER_API_KEY = "your-token-here")`,
		strings.Repeat(WithheldFieldPlaceholder+" ", 400) + "done",
		`{"monkey":"x","donkey":"y"}`,
	} {
		if got := Final(keep); got != keep {
			t.Errorf("Final withheld %.80q: %.200s", keep, got)
		}
	}
}

// A string value whose embedded-JSON scan runs out of budget is withheld by
// the redactor exactly when the store withholds it (one rule), over random
// bracket-heavy strings around the budget.
func TestBudgetRuleMatchesStore(t *testing.T) {
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	r := rand.New(rand.NewSource(17))
	frags := []string{"[", "{", "[x", "]", "}", " ", `{"a":1}`, `["b"]`, WithheldFieldPlaceholder, "note", ","}
	mismatches, withheldN, keptN := 0, 0, 0
	for i := 0; i < 3000; i++ {
		var b strings.Builder
		for n := 3*maxFailedSegmentDecodes - 150 + r.Intn(300); n > 0; n-- {
			b.WriteString(frags[r.Intn(len(frags))])
		}
		s := b.String()
		doc, _ := json.Marshal(map[string]string{"v": s})
		resetWithheldMemory()
		out := Final(string(doc))
		var got map[string]any
		mcpWithholds := out == WithheldBlockMessage || json.Unmarshal([]byte(out), &got) != nil || got["v"] != s
		storeWithholds := withheldValue(store.ScrubPersistedSecrets(doc))
		if storeWithholds {
			withheldN++
		} else {
			keptN++
		}
		if storeWithholds && !mcpWithholds {
			mismatches++
			if mismatches <= 3 {
				t.Errorf("the store withholds %.80q but read-only MCP keeps it", s)
			}
		}
	}
	if mismatches > 0 {
		t.Fatalf("%d strings the store withholds were kept by read-only MCP", mismatches)
	}
	if withheldN == 0 || keptN == 0 {
		t.Fatalf("the generator does not straddle the budget: %d withheld, %d kept", withheldN, keptN)
	}
}
