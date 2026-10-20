// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "mcp-redact-exact-literal-exemptions" entry.

package redact

import (
	"encoding/json"
	"math/rand"
	"strings"
	"testing"

	"dnsfilter-pp-cli/internal/cliutil"
	"dnsfilter-pp-cli/internal/store"
)

// crossCheckString builds a random string from fragments that exercise the
// text rule: backslashes, quotes, \u escapes, key=value and key: value pairs
// with literal-prefixed or placeholder-prefixed values, newlines and table
// cells. It never contains '{' and keeps '[' only inside placeholder-like
// tokens, so neither side finds an embedded JSON value and both decisions
// reduce to the text rule.
func crossCheckString(r *rand.Rand) string {
	frags := []string{
		"password", "token", "secret_key", "pin", "name", "id", "note", "DNSFILTER_API_KEY",
		"=", ": ", ":", " = ", " ", "\t", ",", ";", "\n", "|", "\"", "'", `\`, `\\`, `\\"`, `\\n`, `\u0065`, `\u003d`, `\u0022`, `\u`,
		"true", "false", "null", "true!", "null#", "false ", "truex", "nullable", "[withheld: credential]", "withheld", "[w", "[withhe",
		"SENT1", "abc123", "x", "}", "]", ")", "!", "#", "+", "~",
	}
	var b strings.Builder
	for i := 1 + r.Intn(12); i > 0; i-- {
		b.WriteString(frags[r.Intn(len(frags))])
	}
	s := strings.ReplaceAll(b.String(), "{", "")
	s = strings.NewReplacer("[withheld: credential]", "\x00PH\x00", "[withhe", "\x00CUT\x00", "[w", "\x00W\x00").Replace(s)
	s = strings.ReplaceAll(s, "[", "")
	s = strings.NewReplacer("\x00PH\x00", "[withheld: credential]", "\x00CUT\x00", "[withhe", "\x00W\x00", "[w").Replace(s)
	return strings.TrimLeft(s, " \t\n\"")
}

func withheldValue(out []byte) bool {
	var doc map[string]any
	if json.Unmarshal(out, &doc) != nil {
		return true
	}
	v, ok := doc["v"].(string)
	return !ok || v == WithheldFieldPlaceholder
}

// TestTextRuleMatchesStore: for 100k random strings the MCP redactor and the
// store scrub make the same withhold decision on a string value, so there is
// one text rule.
func TestTextRuleMatchesStore(t *testing.T) {
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	r := rand.New(rand.NewSource(7))
	mismatches := 0
	for i := 0; i < 100000; i++ {
		s := crossCheckString(r)
		doc, _ := json.Marshal(map[string]string{"v": s})
		ours := withheldValue([]byte(Raw(string(doc))))
		theirs := withheldValue(store.ScrubPersistedSecrets(doc))
		if ours != theirs {
			mismatches++
			if mismatches <= 5 {
				t.Errorf("decision differs for %q: redact withholds=%v, store withholds=%v", s, ours, theirs)
			}
		}
	}
	if mismatches > 0 {
		t.Fatalf("%d of 100000 strings got different decisions", mismatches)
	}
}

// TestQuotedValueRuleMatchesStore: quoted secret values whose content starts
// with a delimiter or an escaped quote, at 0 to 2 extra escape levels, with
// embedded JSON around them, get the same decision from the redactor and the
// store scrub, and the redactor never returns the secret.
func TestQuotedValueRuleMatchesStore(t *testing.T) {
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	g := &propGen{r: rand.New(rand.NewSource(11))}
	mismatches := 0
	for i := 0; i < 100000; i++ {
		s := g.quotedSecretPair()
		doc, _ := json.Marshal(map[string]string{"v": s})
		resetWithheldMemory()
		out := Raw(string(doc))
		ours := withheldValue([]byte(out))
		theirs := withheldValue(store.ScrubPersistedSecrets(doc))
		if strings.Contains(Final(string(doc)), secretMarker) {
			t.Fatalf("quoted value leaked: %q -> %s", s, Final(string(doc)))
		}
		if ours != theirs {
			mismatches++
			if mismatches <= 5 {
				t.Errorf("decision differs for %q: redact withholds=%v, store withholds=%v", s, ours, theirs)
			}
		}
	}
	if mismatches > 0 {
		t.Fatalf("%d of 100000 quoted values got different decisions", mismatches)
	}
}

// TestQualifiedLabelAsJSONKeyInString: a string holding exactly one JSON
// value still gets the plain rule on its cleaned form, so a JSON key that
// carries a qualified label is withheld in a result and in a stored row.
func TestQualifiedLabelAsJSONKeyInString(t *testing.T) {
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	q := func(s string) string { b, _ := json.Marshal(s); return string(b) }
	for _, inner := range []string{
		`{"Uninstall PIN (HQ)":"` + secretMarker + `"}`,
		`{"name":"x","Admin password for HQ":"` + secretMarker + `"}`,
		`["ok",{"PIN for laptops":"` + secretMarker + `"}]`,
	} {
		for _, doc := range []string{`{"note":` + q(inner) + `}`, `{"note":` + q(q(inner)) + `}`} {
			if out := Final(doc); strings.Contains(out, secretMarker) {
				t.Errorf("read-only result kept a qualified JSON-key label: %s -> %s", doc, out)
			}
			if out := store.ScrubPersistedSecrets(json.RawMessage(doc)); strings.Contains(string(out), secretMarker) {
				t.Errorf("store kept a qualified JSON-key label: %s -> %s", doc, out)
			}
		}
	}
	// A JSON-in-string with no secret is kept as written.
	if doc := `{"cfg":` + q(`{"name":"x","pin_code_enabled":true}`) + `}`; Final(doc) != doc {
		t.Errorf("a clean JSON-in-string changed: %s -> %s", doc, Final(doc))
	}
}
