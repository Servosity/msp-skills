// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-fix regression test (handfixes.json: secrets-never-persisted): a
// brute-force property check of ScrubPersistedSecrets with an oracle that
// does not depend on the implementation.

package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"
)

// secretGen builds small JSON documents by hand (so objects can repeat a
// key) in which every sentinel value sits under a secret-named key, or in a
// secret name=value pair inside a string. No other value ever contains the
// sentinel prefix, so any sentinel in the scrubbed output is a leak, whatever
// the scrubber's internals.
type secretGen struct {
	r *rand.Rand
	n int
}

const sentinelPrefix = "SNTNL"

var genKeys = []string{"name", "token", "secret_key", "pin", "id", "meta"}

func (g *secretGen) sentinel() string {
	g.n++
	return fmt.Sprintf("%s%d", sentinelPrefix, g.n)
}

func (g *secretGen) quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// escKey spells a key as JSON text (quotes included) with some of its
// characters written as \u00XX escapes in random hex case, so tok\u0065n and
// S\u0045CRET-style spellings reach both the decoder and the text rule.
func (g *secretGen) escKey(key string) string {
	var b strings.Builder
	b.WriteByte('"')
	escaped := false
	for i := 0; i < len(key); i++ {
		if g.r.Intn(2) == 0 || (!escaped && i == len(key)-1) {
			hex := fmt.Sprintf("%04x", key[i])
			if g.r.Intn(2) == 0 {
				hex = strings.ToUpper(hex)
			}
			b.WriteString(`\u` + hex)
			escaped = true
			continue
		}
		b.WriteByte(key[i])
	}
	b.WriteByte('"')
	return b.String()
}

// keyText is the JSON spelling of key, escaped some of the time for the
// secret-named keys.
func (g *secretGen) keyText(key string) string {
	if IsSecretKey(key) && g.r.Intn(3) == 0 {
		return g.escKey(key)
	}
	return g.quote(key)
}

func (g *secretGen) secretKeyName() string {
	return []string{"token", "secret_key", "pin"}[g.r.Intn(3)]
}

// plainValue is any JSON value that carries no secret.
func (g *secretGen) plainValue(depth int) string {
	switch k := g.r.Intn(9); {
	case k == 0:
		return "null"
	case k == 1:
		return []string{"true", "false"}[g.r.Intn(2)]
	case k == 2:
		return fmt.Sprintf("%d", g.r.Intn(1000))
	case k == 3 && depth < 4:
		return g.object(depth + 1)
	case k == 4 && depth < 4:
		return g.array(depth + 1)
	case k == 5 && depth < 4:
		return g.quote(g.textValue(depth + 1))
	default:
		return g.quote([]string{"ok", "HQ", "a b", "x:1", "Top Secret, Finance", "{name}", "[1,2", "token: null"}[g.r.Intn(8)])
	}
}

// secretValue is the value of a secret-named key: a sentinel string,
// number-like string, object or array holding a sentinel, or a harmless
// null / bool flag.
func (g *secretGen) secretValue(depth int) string {
	switch g.r.Intn(6) {
	case 0:
		return "null"
	case 1:
		return "true"
	case 2:
		return `{"v":` + g.quote(g.sentinel()) + `}`
	case 3:
		return `[` + g.quote(g.sentinel()) + `]`
	default:
		return g.quote(g.sentinel())
	}
}

func (g *secretGen) value(key string, depth int) string {
	if IsSecretKey(key) {
		return g.secretValue(depth)
	}
	return g.plainValue(depth)
}

func (g *secretGen) object(depth int) string {
	n := g.r.Intn(4)
	parts := make([]string, 0, n+1)
	for i := 0; i < n; i++ {
		key := genKeys[g.r.Intn(len(genKeys))]
		parts = append(parts, g.keyText(key)+":"+g.value(key, depth))
	}
	if g.r.Intn(3) == 0 && n > 0 {
		// Repeat a key, often with the secret first and a null after it.
		key := []string{"token", "secret_key", "pin", "name"}[g.r.Intn(4)]
		parts = append(parts, g.keyText(key)+":"+g.value(key, depth), g.keyText(key)+":null")
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func (g *secretGen) array(depth int) string {
	n := g.r.Intn(3)
	parts := make([]string, 0, n)
	for i := 0; i < n; i++ {
		parts = append(parts, g.plainValue(depth))
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// textValue is the content of a string value: JSON in a string, JSON with
// trailing or leading text, a secret name=value pair, or plain text.
func (g *secretGen) textValue(depth int) string {
	pair := func() string {
		return []string{"token=", "pin: ", "secret_key=", "Token:"}[g.r.Intn(4)] + g.sentinel()
	}
	switch g.r.Intn(17) {
	case 15:
		// A spaced credential label (API Key, Network secret key ...), in
		// key=value / key: value text or as a table header.
		label := []string{"API Key", "Secret Key", "Network secret key", "Client Secret", "Access Token", "Uninstall PIN", "Private Key", "Shared Key", "api key", "SECRET KEY"}[g.r.Intn(10)]
		if g.r.Intn(3) == 0 {
			return "Site,Name," + label + "\nHQ,x," + g.sentinel()
		}
		return "note " + label + []string{": ", "=", " : ", ":"}[g.r.Intn(4)] + g.sentinel()
	case 16:
		// A YAML block value: the next line, indented deeper than the key.
		indent := []string{"", "  ", "\t"}[g.r.Intn(3)]
		v := g.sentinel()
		if g.r.Intn(3) == 0 {
			v = `"` + v + `"`
		}
		return indent + g.secretKeyName() + ":\n" + indent + "    " + v
	case 13:
		// A quoted value whose content begins with a delimiter, a literal,
		// the placeholder or an escaped quote, closed or not.
		q := []string{`"`, "'"}[g.r.Intn(2)]
		lead := []string{",", ";", "&", "}", "]", ")", " ", "true", "null,", "false;", WithheldValuePlaceholder, "\\" + q, "\\" + q + ",", ""}[g.r.Intn(14)]
		tail := q
		if g.r.Intn(3) == 0 {
			tail = "" // unclosed: the rest of the text is the value
		}
		sep := []string{"=", ": ", ":"}[g.r.Intn(3)]
		return "x " + []string{"password", "token", "pin", "secret_key"}[g.r.Intn(4)] + sep + q + lead + g.sentinel() + tail
	case 14:
		// The same as a cut-off JSON blob: "token":",SECRET
		lead := []string{",", "}", "true!", "null,", "\\" + `"`, ""}[g.r.Intn(6)]
		return `{"a":1,` + g.keyText(g.secretKeyName()) + `:"` + lead + g.sentinel()
	case 10:
		// A value that STARTS with an exempt literal or the placeholder and
		// continues, with no delimiter, into the secret.
		lead := []string{"true", "false", "null", WithheldValuePlaceholder}[g.r.Intn(4)]
		glue := []string{"!", "-", "_", ".", ":", "", "/", "+"}[g.r.Intn(8)]
		sep := []string{"=", ": ", ":", " = "}[g.r.Intn(4)]
		return "x " + []string{"password", "token", "pin", "secret_key"}[g.r.Intn(4)] + sep + lead + glue + g.sentinel()
	case 11:
		// The same inside a cut-off JSON blob: "token":"true...secret.
		return `{"name":"x",` + g.keyText(g.secretKeyName()) + `:"` + []string{"true", "null", "false"}[g.r.Intn(3)] + "!" + g.sentinel()
	case 12:
		// The value on the next line (YAML-style), after an empty separator.
		return g.secretKeyName() + ":\n  " + g.sentinel()
	case 7:
		// A cut-off JSON blob (undecodable) whose secret key is \u-escaped.
		cut := `{"name":"x",` + g.escKey(g.secretKeyName()) + `:"` + g.sentinel()
		if g.r.Intn(2) == 0 {
			cut += `"`
		}
		return cut
	case 8:
		// The same blob quoted once more (escaped quotes and backslashes).
		return g.quote(`{"id":1,` + g.escKey(g.secretKeyName()) + `:"` + g.sentinel())
	case 9:
		// A name=value pair whose name is \u-escaped text.
		k := g.escKey(g.secretKeyName())
		return "x " + k[1:len(k)-1] + "=" + g.sentinel()
	case 0:
		return g.object(depth)
	case 1:
		return g.object(depth) + "} " + pair()
	case 2:
		return "note " + g.object(depth) + " end"
	case 3:
		return g.object(depth) + " " + g.object(depth)
	case 4:
		return "x " + pair() + " y"
	case 5:
		return g.quote(g.object(depth)) // JSON string holding JSON
	default:
		return "plain text"
	}
}

func TestScrubPersistedSecretsProperty(t *testing.T) {
	for _, seed := range []int64{20261020, 1, 7, 42} {
		checkScrubProperty(t, seed, 20000)
	}
}

func checkScrubProperty(t *testing.T, seed int64, cases int) {
	t.Helper()
	g := &secretGen{r: rand.New(rand.NewSource(seed))}
	for i := 0; i < cases; i++ {
		var doc string
		switch i % 3 {
		case 0:
			doc = g.object(0)
		case 1:
			doc = `{"id":"1","attributes":` + g.object(1) + `,"meta":` + g.quote(g.textValue(1)) + `}`
		default:
			doc = `[` + g.object(0) + `,` + g.object(0) + `]`
		}
		if !json.Valid([]byte(doc)) {
			t.Fatalf("generator produced invalid JSON: %s", doc)
		}
		out := ScrubPersistedSecrets(json.RawMessage(doc))
		if bytes.Contains(out, []byte(sentinelPrefix)) {
			t.Fatalf("seed %d case %d: a secret survived the scrub\n in  %s\n out %s", seed, i, doc, out)
		}
		if !json.Valid(out) {
			t.Fatalf("seed %d case %d: scrubbed output is not JSON\n in  %s\n out %s", seed, i, doc, out)
		}
	}
}

// The duplicate-key class, pinned case by case: the decoded value keeps only
// the last occurrence, so the first one's secret must not be carried by the
// original bytes, at the top level, inside JSON in a string, inside JSON
// embedded in text, and with text after an embedded value.
func TestScrubDuplicateKeysAndTrailingText(t *testing.T) {
	q := func(s string) string { b, _ := json.Marshal(s); return string(b) }
	for name, in := range map[string]string{
		"top-level duplicate":                            `{"id":"a1","meta":{"token":"SNTNL-1","token":null}}`,
		"duplicate inside array":                         `[{"secret_key":"SNTNL-2","secret_key":false}]`,
		"duplicate in json string":                       `{"meta":` + q(`{"token":"SNTNL-3","token":null}`) + `}`,
		"duplicate in embedded json":                     `{"meta":` + q(`note {"pin":"SNTNL-4","pin":true} end`) + `}`,
		"text after embedded value":                      `{"meta":` + q(`{"ok":1}} token=SNTNL-5`) + `}`,
		"second value in string":                         `{"meta":` + q(`{"ok":1} {"token":"SNTNL-6"}`) + `}`,
		"escaped key, cut-off blob":                      `{"x":"{\"a\":1,\"tok\\u0065n\":\"SNTNL-7\""}`,
		"escaped key, upper hex":                         `{"x":` + q(`{"a":1,"S\u0045CRET_KEY":"SNTNL-8`) + `}`,
		"escaped key, decodable":                         `{"x":` + q(`{"p\u0069n":"SNTNL-9"}`) + `}`,
		"escaped key, top level":                         `{"tok\u0045n":"SNTNL-10","name":"a"}`,
		"literal prefix (Codex)":                         `{"meta":"password=true!SNTNL-11"}`,
		"null prefix":                                    `{"meta":"token: null-SNTNL-12"}`,
		"placeholder prefix":                             `{"meta":` + q("pin="+WithheldValuePlaceholder+"SNTNL-13") + `}`,
		"literal prefix in blob":                         `{"meta":` + q(`{"a":1,"token":"false.SNTNL-14`) + `}`,
		"value on the next line":                         `{"meta":` + q("secret_key:\n  SNTNL-15") + `}`,
		"quoted value starting with a delimiter (Codex)": `{"meta":"password=\",SNTNL-16\""}`,
		"quoted literal prefix":                          `{"meta":"token='true,SNTNL-17'"}`,
		"quoted placeholder prefix":                      `{"meta":` + q(`pin="`+WithheldValuePlaceholder+`SNTNL-18"`) + `}`,
		"quoted escaped quote":                           `{"meta":` + q("secret_key=\""+"\\"+"\"SNTNL-19\"") + `}`,
		"unclosed quote":                                 `{"meta":"token=\";SNTNL-20"}`,
		"empty quotes run on":                            `{"meta":` + q(`token=""SNTNL-21`) + `}`,
		"escaped quote then a delimiter":                 `{"meta":` + q(`token="`+"\\"+`",SNTNL-22"`) + `}`,
		"spaced label API Key":                           `{"meta":"API Key: SNTNL-23"}`,
		"spaced label 3 words":                           `{"meta":"Network secret key: SNTNL-24"}`,
		"spaced label with =":                            `{"meta":"Secret Key = SNTNL-25"}`,
		"spaced table header":                            `{"meta":` + q("Site,API Key\nHQ,SNTNL-26") + `}`,
		"yaml block, deeper key":                         `{"meta":` + q("  password:\n    SNTNL-27") + `}`,
		"yaml block, quoted value":                       `{"meta":` + q("token:\n  \"SNTNL-28\"") + `}`,
	} {
		out := ScrubPersistedSecrets(json.RawMessage(in))
		if bytes.Contains(out, []byte(sentinelPrefix)) || !json.Valid(out) {
			t.Errorf("%s:\n in  %s\n out %s", name, in, out)
		}
	}
	// A duplicate of a non-secret key is collapsed by re-encoding (the raw
	// bytes are never trusted once an object repeats a key).
	in := `{"name":"a","name":"b"}`
	if out := string(ScrubPersistedSecrets(json.RawMessage(in))); out != `{"name":"b"}` {
		t.Errorf("duplicate non-secret key: %s", out)
	}
}

func TestUnescapeForTextRule(t *testing.T) {
	// Built from bs so no escape sequence is written literally in this file.
	bs := "\\"
	u := func(hex string) string { return bs + "u" + hex }
	cases := []struct{ in, want string }{
		{"tok" + u("0065") + "n", "token"},
		{"S" + u("0045") + "CRET" + u("005F") + "KEY", "SECRET_KEY"},
		{"p" + u("0069") + "n", "pin"},
		{bs + `"pin` + bs + `":` + bs + `"1` + bs + `"`, `"pin":"1"`},
		{"a" + bs + bs + bs + `"b`, `a"b`}, // two levels
		{"line" + bs + "nnext" + bs + "tx" + bs + "/y", "line\nnext\tx/y"},
		{"bad " + bs + "u12 and " + bs + "q stay", "bad " + bs + "u12 and " + bs + "q stay"},
		{"trailing " + bs, "trailing " + bs},
		{"p" + bs + bs + "u0069n=1", "pin=1"},
		{"no escapes at all", "no escapes at all"},
	}
	for _, c := range cases {
		if got := UnescapeForTextRule(c.in); got != c.want {
			t.Errorf("UnescapeForTextRule(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	// One level per round, at most maxUnescapeRounds rounds.
	deep := "t"
	for i := 0; i < maxUnescapeRounds+1; i++ {
		deep = strings.ReplaceAll(deep, bs, bs+bs)
		if i == 0 {
			deep = u("0074")
		}
	}
	if got := UnescapeForTextRule(deep + "oken=X"); got == "token=X" {
		t.Errorf("escaping deeper than %d levels must not be fully undone, got %q", maxUnescapeRounds, got)
	}
	shallow := u("0074")
	for i := 1; i < maxUnescapeRounds; i++ {
		shallow = strings.ReplaceAll(shallow, bs, bs+bs)
	}
	if got := UnescapeForTextRule(shallow + "oken=X"); got != "token=X" {
		t.Errorf("%d levels must be undone, got %q", maxUnescapeRounds, got)
	}
}

// The text rule's exemptions match exactly, never as a prefix: a value that
// starts with true / false / null / the placeholder and goes on is a value.
func TestSecretPairExemptionsAreExact(t *testing.T) {
	bs := "\\"
	carries := []string{
		"password=true!SECRET", "token: null-SECRET", "pin=falsey", "token=truex",
		"secret_key=" + WithheldValuePlaceholder + "SECRET", "token=[withheld", "token=nul",
		"token:\n  SECRET", `"token":"false.SECRET`, "token=" + bs + "SECRET", "tok" + bs + "u0065n=SECRET",
		// A value that starts with a delimiter is a value unless nothing but
		// delimiters follows (fail closed: an empty token followed by another
		// pair is withheld too).
		`{"meta":"password=;hunter2"}`, "password=,hunter2", "token=&hunter2", "pin=)4821", "token=,next=1", "token=&a=1",
	}
	for _, text := range carries {
		if !TextNamesSecret(text) {
			t.Errorf("TextNamesSecret(%q) = false, want true (a value follows)", text)
		}
	}
	empty := []string{
		"token=", "token=,", "token=)}", "token=true", "token=true,x=1", "token=null}",
		`"token":"false"`, `"token": null,`, "pin:true", "token=" + WithheldValuePlaceholder,
		"token=" + WithheldValuePlaceholder + "&a=1", "token=true next", "password=", `{"token":""}`,
	}
	for _, text := range empty {
		if TextNamesSecret(text) {
			t.Errorf("TextNamesSecret(%q) = true, want false (no value)", text)
		}
	}
	// SecretPairInText expects text that is already unescaped.
	if SecretPairInText("tok" + bs + "u0065n=SECRET") {
		t.Error("SecretPairInText must not unescape by itself")
	}
	if !SecretPairInText(UnescapeForTextRule("tok" + bs + "u0065n=SECRET")) {
		t.Error("SecretPairInText missed an unescaped pair")
	}
}

// Value extraction is one quote-aware tokenizer: a quoted value runs to its
// closing quote (backslash escapes honored) or to the end when unclosed, and
// every exemption compares the extracted value only.
func TestPairValueTokenizer(t *testing.T) {
	bs := "\\"
	carries := []string{
		`password=",SECRET"`, `token='true,x'`, `token="true"x`, `token=""x`, `token=";x`,
		`token="` + bs + `"x"`, `token="x` + bs + `"y"`, `pin="` + WithheldValuePlaceholder + `x"`,
		`token=` + bs + `"x` + bs + `"`, `token: "x"`, "token:\n  \"x\"", `token='&x'`,
	}
	for _, text := range carries {
		if !TextNamesSecret(text) {
			t.Errorf("TextNamesSecret(%q) = false, want true (a value follows)", text)
		}
	}
	empty := []string{
		`token=""`, `token=''`, `token="",x=1`, `token='true'`, `token="null"}`,
		`token="` + WithheldValuePlaceholder + `"`, `{` + bs + `"token` + bs + `":` + bs + `"` + bs + `"}`,
		`token="`, `token="true`, `"token": "false" `, `token=''` + bs + `n`, `token=" "`, `token=" ` + WithheldValuePlaceholder + `"`,
		`token="},`,
	}
	for _, text := range empty {
		if TextNamesSecret(text) {
			t.Errorf("TextNamesSecret(%q) = true, want false (no value)", text)
		}
	}
}

// The CLI's own documentation placeholders next to a secret name, and prose
// whose next line is not a YAML block value, carry no credential.
func TestTextRuleKeepsPlaceholdersAndProse(t *testing.T) {
	for _, text := range []string{
		`export DNSFILTER_API_KEY="your-token-here"`,
		`$env:DNSFILTER_API_KEY = "your-token-here" (the bare token, no Bearer prefix)`,
		`set a real token with: export DNSFILTER_API_KEY=<your-token-here> (PowerShell: $env:DNSFILTER_API_KEY = "<your-token-here>")`,
		`DNSFILTER_API_KEY=<value> dnsfilter-cli doctor`,
		`$env:DNSFILTER_API_KEY="<value>"; $env:DNSFILTER_MCP_HTTP_TOKEN="<long-random-value>"; dnsfilter-mcp --transport http`,
		"export DNSFILTER_API_KEY=<token> on macOS / Linux, $env:DNSFILTER_API_KEY = \"<token>\" in PowerShell",
		"authentication failed: HTTP 401\nhint: check your API key.\n      Set your API key with: export DNSFILTER_API_KEY=\"your-token-here\"\n      In PowerShell: $env:DNSFILTER_API_KEY = \"your-token-here\" (the bare token, no Bearer prefix)",
		"Site,Notes\nCall support, reset password",
		// A word after the secret word that is not a qualifier is another label.
		"Password Policy: strict", "Password Policy 2: strict", "public key: ssh-ed25519", "Token Expiry Policy: 90 days",
		"Set your API key with: export DNSFILTER_API_KEY=\"your-token-here\" (PowerShell: $env:DNSFILTER_API_KEY = \"your-token-here\")",
		// A qualifier never reaches past the end of its JSON string.
		`{"id":1,"name":"Office of the Secretary of State","status":"active"}`,
		`{"block_page_message":"Ask the front desk for the Wi-Fi password at check-in.","block_org_name":"Acme"}`,
		`{"name":"Spin (cycling studio)","organization_id":7}`,
		`{"note":"Unblocked so staff can reset password for payroll portal","fqdn":"payroll.example"}`,
		`[{"id":7,"name":"Secretary of State Office","industry":"gov"},{"id":8,"name":"Acme"}]`,
	} {
		if TextNamesSecret(text) {
			t.Errorf("TextNamesSecret(%q) = true, want false", text)
		}
	}
	for _, text := range []string{
		"password:\n  hunter2", "token:\n\n    abc123", "  pin:\n    \"4938\"", "API Key: configured",
		"DNSFILTER_API_KEY=your-token-here2", `token="<value>x"`, "API Key: <value>z",
		// A value on the next line counts at any indent and any word count
		// (fail closed: prose after a label that ends its line is withheld).
		"Uninstall PIN:\n482915", "Router admin password:\nP4ss-word!", "VPN token:\n  Bearer eyJhbGciOi",
		"wifi password:\n  hunter2 # rotate monthly", "admin password:\n  - hunter2", "password:\rhunter2",
		"\tpassword:\n\thunter2", "password:\n  correct horse battery staple", "API Key:\r\nsk-live-abc123",
		"Network secret key:\nAbC123xyz789", `{"note":"password:\n  correct horse battery staple"}`,
		"Credentials:\n  stored in IT Glue", "Uninstall PIN:\nask the NOC", "Rotate the token:\nnext quarter",
		// A qualifier between the secret label and its separator.
		"Uninstall PIN for Acme laptops: 482915", "Uninstall PIN (Acme): 482915", "PIN - HQ: 4821", "password #2: hunter2",
		"admin password, HQ: hunter2", "Uninstall PIN(Acme)=4821", "wifi password for the guest network at HQ: hunter2",
		"Uninstall PIN [Acme]: 482915", "Uninstall PIN \u2013 HQ: 482915", "Uninstall PIN \u2014 HQ: 4821", "Uninstall PIN @ HQ: 4821",
		"Uninstall PIN {Acme}: 4821", "Uninstall PIN <Acme>: 4821", "secret_key#1: abc", "Uninstall PIN#2: 482915",
		"password for " + strings.Repeat("A", 130) + ": hunter2", `{"note":"password for ` + strings.Repeat("A ", 300) + `: hunter2"}`,
		// Quoted names inside a qualifier, and Windows UNC paths, do not end it.
		"Admin password for 'Main Office', HQ: hunter2", `Admin password for "Main Office", backup: hunter2`,
		"Uninstall PIN for ['HQ', 'Branch'] laptops: 482915", "Wi-Fi password for 'Guest', staff: hunter2",
		`{"note":"Admin password for \"Main Office\", HQ: hunter2"}`, "Uninstall PIN 'HQ' laptops: 482915",
		`Local admin password for \\dc01\netlogon: hunter2`, `svc_backup password on \\nas\records: hunter2`,
		`Uninstall PIN for \\srv\new-hires laptops: 482923`, `{"note":"Admin password for \\\\dc01\\netlogon: hunter2"}`,
		`PIN "Acme" (laptops): 482915`, `Admin password "Main Office" backup: hunter2`, `{"note":"PIN \"Acme\" (laptops): 482915"}`,
		// Semicolon tables and long spaced headers.
		"Site;Network Secret Key;Address\nHQ;abc123;1 Main", "Site;secret_key;Address\nHQ;abc123;1 Main",
		"Site,Roaming Client Uninstall PIN\nHQ,482915",
		// A quoted comma in a CSV row adds cells; a spaced secret header still counts.
		"Site,Network Secret Key,Address\nHQ,abc123,\"1 Main St, Springfield\"", "Site | Network Secret Key\nHQ | abc | extra",
	} {
		if !TextNamesSecret(text) {
			t.Errorf("TextNamesSecret(%q) = false, want true", text)
		}
	}
}

// TestTextRuleStaysLinear: the label window is bounded before any scan, so a
// large mirrored string is judged in linear time (a whole-prefix search per
// separator took 100s on 1.2 MB).
func TestTextRuleStaysLinear(t *testing.T) {
	row := `{"name":"Acme Dental HQ","organization_id":12345,"policy_ids":[1,2,3],"address":"1 Main St, Springfield"},`
	for _, big := range []string{
		"[" + strings.Repeat(row, 12000) + "]",
		strings.Repeat("(:", 500000),
		strings.Repeat("(:", 500000) + strings.Repeat("\\", 64),
		strings.Repeat("PIN (a): ", 100000),
		strings.Repeat("API Key, ", 60000) + strings.Repeat("\n", 500000) + "x",
	} {
		start := time.Now()
		TextNamesSecret(big)
		if d := time.Since(start); d > 20*time.Second {
			t.Fatalf("TextNamesSecret on %d bytes took %v", len(big), d)
		}
	}
}
