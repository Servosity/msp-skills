// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-fix regression tests (handfixes.json: secrets-never-persisted), Codex
// round 17 and adversarial round 18.

package store

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// scrubKeeps reports whether the store scrub kept marker in a row whose note
// is text, also when text arrives as JSON in a string.
func scrubKeeps(t *testing.T, text, marker string) (bool, string) {
	t.Helper()
	q := func(s string) string {
		b, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	for _, row := range []string{
		`{"id":"1","note":` + q(text) + `}`,
		`{"id":"1","note":` + q(`{"memo":`+q(text)+`}`) + `}`,
	} {
		if out := ScrubPersistedSecrets(json.RawMessage(row)); bytes.Contains(out, []byte(marker)) {
			return true, string(out)
		}
	}
	return false, ""
}

// A secret name glued to a digit or punctuation inside one scanner word
// (PIN2, PIN.2, "PIN." then a qualifier, uninstall_pin_2, PIN-) names the
// secret and arms the segment, exactly as the same punctuation after a space
// does. Brute force over secret words x glue x qualifier x separator x
// prefix: every combination is withheld by the text rule and the store scrub.
func TestGluedSecretLabelArms(t *testing.T) {
	const marker = "4821Q7"
	words := []string{"PIN", "pin", "PINs", "token", "Tokens", "password", "secret_key", "API Key", "Network secret key", "NKEY", "nKey", "uninstall_pin", "passphrase"}
	glues := []string{"2", ".2", ".", "_2", "-2", "-", "_", "2HQ", "2.", "._", "-."}
	quals := []string{"", " HQ", " (HQ)", " for laptops", " HQ laptops", " 'Main'"}
	seps := []string{": ", "=", " = ", ":\n"}
	prefixes := []string{"", "Uninstall ", "Site ", "msiexec /i agent.msi ", "notes: "}
	n := 0
	for _, w := range words {
		for _, g := range glues {
			for _, q := range quals {
				for _, sep := range seps {
					for _, p := range prefixes {
						text := p + w + g + q + sep + marker
						n++
						if !TextNamesSecret(text) {
							t.Errorf("TextNamesSecret(%q) = false, want true", text)
							continue
						}
						if kept, out := scrubKeeps(t, text, marker); kept {
							t.Errorf("store kept %q: %s", text, out)
						}
					}
				}
			}
		}
	}
	if n < 10000 {
		t.Fatalf("only %d combinations", n)
	}
	for _, text := range []string{
		"Uninstall PIN2: 4821", "Uninstall PIN.2: 4821", "Uninstall PIN. HQ: 4821", "uninstall_pin_2=4821",
		"Site,PIN2\nHQ,4821", "Site,Uninstall PIN2 Code\nHQ,4821", "Site,API Key2\nHQ,abc",
	} {
		if !TextNamesSecret(text) {
			t.Errorf("TextNamesSecret(%q) = false, want true", text)
		}
	}
	// A glued secret name is a secret JSON key too.
	for _, key := range []string{"PIN2", "pin.2", "uninstall_pin_2", "PIN-", "token2fa", "nKey2"} {
		if !KeyNamesSecret(key) {
			t.Errorf("KeyNamesSecret(%q) = false, want true", key)
		}
	}
	// Compound names (a joiner between letters), identifiers and other labels
	// are not secrets.
	for _, key := range []string{"token_type", "pin_code_enabled", "secret_key_id", "api_keys_id", "token.id", "pin2_id", "token.v2", "pinned_at", "monkey", "donkey"} {
		if KeyNamesSecret(key) {
			t.Errorf("KeyNamesSecret(%q) = true, want false", key)
		}
	}
	for _, text := range []string{
		"token_type: Bearer", "secret_key_id=5", "token.id: 5", "Password Policy 2: strict", "api_keys_id: 7",
		"refusing GET api-keys.index while DNSFILTER_MCP_READ_ONLY=1: it returns or manages a credential",
		"hint: check your API key. Set your API key with: export DNSFILTER_API_KEY=\"your-token-here\" (PowerShell: $env:DNSFILTER_API_KEY = \"your-token-here\")",
	} {
		if TextNamesSecret(text) {
			t.Errorf("TextNamesSecret(%q) = true, want false", text)
		}
	}
	// The instruction rule never unlabels the pair inside it, and only a glued
	// arming stops at it: an arming by other punctuation still labels it.
	for _, text := range []string{
		`hint: check your API key. Set your API key with: export DNSFILTER_API_KEY="XX00XX00XX00XX00XX00"`,
		"Uninstall PIN (HQ), set the API key with: 4821",
		"admin password, rotate the token with: hunter2",
	} {
		if !TextNamesSecret(text) {
			t.Errorf("TextNamesSecret(%q) = false, want true", text)
		}
	}
}

// The glued-boundary check reads each word once: a word of a million
// letter/digit pairs, and a long dotted word after secret labels, stay linear.
func TestGluedSecretLabelStaysLinear(t *testing.T) {
	for _, big := range []string{
		"x " + strings.Repeat("a1", 500000) + ": v",
		"API Key " + strings.Repeat("key1", 250000) + ": v",
		"Site," + strings.Repeat("ab1.", 250000) + "\nHQ,1",
		"secret_id" + strings.Repeat("2x", 400000) + ": v",
	} {
		start := time.Now()
		TextNamesSecret(big)
		if d := time.Since(start); d > 20*time.Second {
			t.Fatalf("TextNamesSecret on %d bytes took %v", len(big), d)
		}
	}
}

// A table header cell is judged at any length: no length guard runs before
// the secret check. A header label with a bracketed qualifier heads a column.
func TestTableHeaderAnyLength(t *testing.T) {
	long := "roaming_client_windows_headquarters_network_installation_secret_key"
	if len(long) <= 64 {
		t.Fatalf("fixture header is %d bytes, want more than 64", len(long))
	}
	for _, text := range []string{
		"Site," + long + "\nHQ,4821",
		"Site;" + long + "_" + strings.Repeat("x", 300) + "_token\nHQ;4821",
		"Site,Roaming Client Windows Headquarters Network Installation Secret Key\nHQ,4821",
		"Site,Uninstall PIN (HQ)\nHQ,4821",
		"Site | API Key [prod]\nHQ | abc123",
	} {
		if !TextNamesSecret(text) {
			t.Errorf("TextNamesSecret(%q) = false, want true", text)
		}
		if kept, out := scrubKeeps(t, text, "4821"); kept {
			t.Errorf("store kept %q: %s", text, out)
		}
	}
	for _, text := range []string{"Site,Password Policy\nHQ,strict", "Site,Notes\nCall support, reset password"} {
		if TextNamesSecret(text) {
			t.Errorf("TextNamesSecret(%q) = true, want false", text)
		}
	}
}

// The embedded-JSON scan of a string value fails closed when its decode
// budget runs out with a '{' or '[' left unexamined: a JSON key the text rule
// cannot read ("p a s s w o r d") after 256 stray brackets is never kept.
// Withheld placeholders never spend the budget.
func TestEmbeddedJSONBudgetFailsClosed(t *testing.T) {
	const marker = "SNTL-R17-BUDGET"
	obj := `{"p a s s w o r d":"` + marker + `"}`
	if !KeyNamesSecret("p a s s w o r d") {
		t.Fatal("fixture key is not a secret key")
	}
	if TextNamesSecret(obj) {
		t.Fatal("fixture is caught by the text rule alone; it would not test the budget")
	}
	for _, n := range []int{maxFailedSegmentDecodes, maxFailedSegmentDecodes + 1, 1000} {
		for _, pre := range []string{strings.Repeat("[", n), strings.Repeat("{", n), strings.Repeat("[x ", n)} {
			b, _ := json.Marshal(pre + obj)
			row := `{"id":"1","note":` + string(b) + `}`
			if out := ScrubPersistedSecrets(json.RawMessage(row)); bytes.Contains(out, []byte(marker)) {
				t.Fatalf("%d stray brackets: the secret survived the scrub (%d bytes)", n, len(out))
			}
		}
	}
	// Below the budget the object itself is decoded and scrubbed.
	b, _ := json.Marshal(strings.Repeat("[", 10) + obj + " tail")
	if out := string(ScrubPersistedSecrets(json.RawMessage(`{"note":` + string(b) + `}`))); strings.Contains(out, marker) || !strings.Contains(out, "tail") {
		t.Fatalf("below the budget: %s", out)
	}
	// Placeholders are skipped, not decoded: many of them keep the value.
	text := strings.Repeat(WithheldValuePlaceholder+" ", 400) + `see {"name":"HQ"}`
	b, _ = json.Marshal(text)
	row := `{"note":` + string(b) + `}`
	if out := ScrubPersistedSecrets(json.RawMessage(row)); string(out) != row {
		t.Fatalf("a value of placeholders was withheld: %.200s", out)
	}
}

// NKEY is DNSFilter's roaming-client deployment property for the site secret
// key (msiexec ... NKEY="...", the macOS plist <key>NKEY</key>). It is a
// secret as a whole key word everywhere the text rule runs; monkey and donkey
// are not.
func TestNKEYIsASecret(t *testing.T) {
	const marker = "nk3y-SITE-SECRET"
	for _, key := range []string{"NKEY", "nkey", "site_nkey", "siteNkey", "nKey", "NKey", "n_key", "NKEYS"} {
		if !IsSecretKey(key) {
			t.Errorf("IsSecretKey(%q) = false, want true", key)
		}
	}
	for _, key := range []string{"monkey", "donkey", "Monkey", "nkey_id", "nkeyId", "turnkey", "n", "key"} {
		if IsSecretKey(key) {
			t.Errorf("IsSecretKey(%q) = true, want false", key)
		}
	}
	plistEscaped, _ := json.Marshal(`<key>NKEY</key><string>` + marker + `</string>`) // json.Marshal escapes < as \u003c
	if !bytes.Contains(plistEscaped, []byte(string(rune(92))+"u003c")) {
		t.Fatal("fixture lost its \\u escapes")
	}
	for _, text := range []string{
		"NKEY=" + marker, "NKEY: " + marker, `NKEY="` + marker + `"`,
		`msiexec /qn /i DNSFilter_Agent_Setup.msi NKEY="` + marker + `" TRAYICON=disabled`,
		`<key>NKEY</key><string>` + marker + `</string>`,
		"<dict>\n\t<key>NKEY</key>\n\t<string>" + marker + "</string>\n</dict>",
		`<key> NKEY </key><string>` + marker, // a preview cut mid-value
		`<key>Uninstall PIN</key><integer>` + marker + `</integer>`,
		`<key>password</key><data>` + marker + `</data>`,
		strings.Trim(string(plistEscaped), `"`),
	} {
		if !TextNamesSecret(text) {
			t.Errorf("TextNamesSecret(%q) = false, want true", text)
		}
		if kept, out := scrubKeeps(t, text, marker); kept {
			t.Errorf("store kept %q: %s", text, out)
		}
	}
	for _, row := range []string{`{"NKEY":"` + marker + `"}`, `{"cfg":{"site_nkey":"` + marker + `"}}`, `{"nKey":"` + marker + `"}`} {
		if out := ScrubPersistedSecrets(json.RawMessage(row)); bytes.Contains(out, []byte(marker)) {
			t.Errorf("store kept %s: %s", row, out)
		}
	}
	for _, text := range []string{
		"monkey: banana", "donkey=1", `<key>name</key><string>HQ</string>`, `<key>NKEY</key><string></string>`,
		`<key>NKEY</key><string>[withheld: credential]</string>`, `<key>NKEY</key><true/>`,
	} {
		if TextNamesSecret(text) {
			t.Errorf("TextNamesSecret(%q) = true, want false", text)
		}
	}
	if row := `{"monkey":"x","donkey":"y"}`; string(ScrubPersistedSecrets(json.RawMessage(row))) != row {
		t.Error("monkey/donkey keys were scrubbed")
	}
}

// Past maxNestedJSONDepth a string that is still one JSON value is withheld,
// even with no escapes left to trip the unescape bound: a non-secret object
// JSON-encoded so that the string at the bound is the raw object.
func TestDepthBoundWithheldWithoutEscapes(t *testing.T) {
	const marker = "DEEPMARK-R18"
	encode := func(levels int) string {
		s := `{"name":"` + marker + `"}`
		for i := 0; i < levels; i++ {
			b, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			s = string(b)
		}
		return s
	}
	// The note's value, decoded maxNestedJSONDepth times, is the raw object,
	// which holds no backslash.
	at := encode(maxNestedJSONDepth + 1)
	var s string
	if err := json.Unmarshal([]byte(at), &s); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxNestedJSONDepth; i++ {
		if err := json.Unmarshal([]byte(s), &s); err != nil {
			t.Fatalf("level %d: %v", i, err)
		}
	}
	if strings.Contains(s, string(rune(92))) || !strings.HasPrefix(s, "{") {
		t.Fatalf("fixture at the bound is %q, want a raw object with no backslash", s)
	}
	if TextNamesSecret(s) {
		t.Fatal("the object at the bound names a secret; it would not test the depth bound")
	}
	for _, levels := range []int{maxNestedJSONDepth + 1, maxNestedJSONDepth + 2, maxNestedJSONDepth + 4} {
		row := `{"id":"1","note":` + encode(levels) + `}`
		if out := ScrubPersistedSecrets(json.RawMessage(row)); bytes.Contains(out, []byte(marker)) {
			t.Fatalf("encoded %d times: the object past the depth bound was kept (%d bytes)", levels, len(out))
		}
	}
	// One level shallower, the object is decoded and judged: kept as is.
	row := `{"id":"1","note":` + encode(maxNestedJSONDepth) + `}`
	if out := ScrubPersistedSecrets(json.RawMessage(row)); string(out) != row {
		t.Fatalf("an object inside the depth bound was changed: %.200s", out)
	}
}

// In JSON text a second arming never narrows the first: a qualifier armed in
// a value string stays in force when a later secret label (here NKEY, newly
// a secret word) arms again inside a nested string that then closes.
func TestJSONScanRearmingNeverNarrows(t *testing.T) {
	for _, text := range []string{
		`{"note":"Uninstall PIN (HQ) \"NKEY (x)\" laptops: 4821"}`,
		`{"note":"Uninstall PIN (HQ) \"PIN2 x\" laptops: 4821"}`,
		`{"note":"Uninstall PIN (HQ) \"token (x)\" laptops: 4821"}`,
	} {
		if !SecretPairInJSONText(text) {
			t.Errorf("SecretPairInJSONText(%s) = false, want true", text)
		}
	}
	// An arming still ends where its own string closes.
	if text := `{"name":"Office of the Secretary of State","status":"active"}`; SecretPairInJSONText(text) {
		t.Errorf("SecretPairInJSONText(%s) = true, want false", text)
	}
}
