// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "mcp-redact-exact-json-and-duplicate-keys" entry.

package redact

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"dnsfilter-pp-cli/internal/cliutil"
)

// propGen builds small JSON documents as RAW text (so duplicate keys,
// escaped key spellings and trailing text are possible) and puts every
// secret it mints only where the read-only contract says a secret may never
// survive: under a credential-named key (token, secret_key, pin, any case or
// \u-escaped spelling), or in a key=value / key: value pair or a multi-line
// table header inside a string. Non-secret values never contain the
// sentinel markers, so the oracle is simply "no marker in the output".
type propGen struct {
	r *rand.Rand
	n int
}

const (
	secretMarker = "SENTINEL"
	numberMarker = "99999"
)

func (g *propGen) secret() string {
	g.n++
	return fmt.Sprintf("%s%04dX", secretMarker, g.n)
}

func (g *propGen) secretNumber() string {
	g.n++
	return fmt.Sprintf("%s%04d", numberMarker, g.n)
}

func quoteJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

var propKeys = []string{"name", "token", "secret_key", "pin", "id", "meta"}

func isCredName(k string) bool { return k == "token" || k == "secret_key" || k == "pin" }

// spell renders a key as raw JSON, sometimes upper-cased or with one letter
// \u-escaped (the decoded key is the same).
func (g *propGen) spell(k string) string {
	switch g.r.Intn(6) {
	case 0:
		return quoteJSON(strings.ToUpper(k))
	case 1:
		i := g.r.Intn(len(k))
		return `"` + k[:i] + fmt.Sprintf(`\u%04x`, k[i]) + k[i+1:] + `"`
	}
	return quoteJSON(k)
}

func (g *propGen) credValue(depth int) string {
	switch g.r.Intn(8) {
	case 0:
		return "null"
	case 1:
		return []string{"true", "false"}[g.r.Intn(2)]
	case 2:
		return g.secretNumber()
	case 3:
		if depth < 3 {
			return g.object(depth + 1)
		}
	case 4:
		return "[" + quoteJSON(g.secret()) + "," + quoteJSON(g.secret()) + "]"
	case 5:
		return quoteJSON(g.object(depth + 1))
	}
	return quoteJSON(g.secret())
}

// literalPrefixedValue is a value that starts with a JSON literal or the
// placeholder (or a cut placeholder) and continues with a secret, joined by a
// character that is not a value terminator.
func (g *propGen) literalPrefixedValue() string {
	lit := []string{"true", "false", "null", "[withheld: credential]", "[withhe", "[w"}[g.r.Intn(6)]
	// None of these ends a value token (the store's delimiters are space,
	// tab, line ends, quotes, , ; & } ] and ), so the token is the literal
	// plus the secret, never exactly the literal.
	sep := []string{"", "!", "#", "+", ":", "/", ".", "@", "%", "|", "~", "*", "="}[g.r.Intn(13)]
	return lit + sep + g.secret()
}

func (g *propGen) textWithSecret() string {
	if g.r.Intn(3) == 0 {
		key := []string{"password", "token", "secret_key", "pin", "client_secret"}[g.r.Intn(5)]
		op := []string{"=", ": ", " = ", ":"}[g.r.Intn(4)]
		return []string{"", "note ", "a=b "}[g.r.Intn(3)] + key + op + g.literalPrefixedValue()
	}
	s := g.secret()
	switch g.r.Intn(5) {
	case 0:
		return "token=" + s
	case 1:
		return "note secret_key: " + s + " end"
	case 2:
		return "id,name,pin\n1,site," + s + "\n"
	case 3:
		return `{"ok":1}} token=` + s
	}
	return `prefix {"name":"x"} pin = ` + s
}

func (g *propGen) plainValue(depth int) string {
	switch g.r.Intn(10) {
	case 0:
		return fmt.Sprint(g.r.Intn(1000))
	case 1, 2:
		if depth < 3 {
			return g.object(depth + 1)
		}
	case 3:
		if depth < 3 {
			return g.array(depth + 1)
		}
	case 4:
		if depth < 3 {
			// JSON in a string.
			return quoteJSON(g.object(depth + 1))
		}
	case 5:
		if depth < 3 {
			// JSON in a string with trailing text after it.
			return quoteJSON(g.object(depth+1) + []string{"}", "]", " }}", ""}[g.r.Intn(4)] + " " + g.textWithSecret())
		}
	case 6:
		return quoteJSON(g.textWithSecret())
	case 7:
		if depth < 2 {
			// JSON in a string in a string.
			return quoteJSON(quoteJSON(g.object(depth + 1)))
		}
	}
	return quoteJSON(fmt.Sprintf("plain%d", g.r.Intn(1000)))
}

func (g *propGen) object(depth int) string {
	var members []string
	n := g.r.Intn(5)
	for i := 0; i < n; i++ {
		k := propKeys[g.r.Intn(len(propKeys))]
		if isCredName(k) {
			members = append(members, g.spell(k)+":"+g.credValue(depth))
			if g.r.Intn(3) == 0 {
				// Duplicate key: a later null/bool/plain occurrence must not
				// hide the earlier secret.
				members = append(members, g.spell(k)+":"+[]string{"null", "false", `"[withheld: credential]"`}[g.r.Intn(3)])
			}
		} else {
			members = append(members, g.spell(k)+":"+g.plainValue(depth))
		}
	}
	return "{" + strings.Join(members, ",") + "}"
}

func (g *propGen) array(depth int) string {
	var items []string
	for i := g.r.Intn(4); i > 0; i-- {
		items = append(items, g.object(depth))
	}
	return "[" + strings.Join(items, ",") + "]"
}

// topLevel returns a result text and whether it is pure JSON at the top level
// (no plain text outside a JSON value), which is what Raw alone must clean.
func (g *propGen) topLevel() (string, bool) {
	doc := g.object(0)
	switch g.r.Intn(6) {
	case 0:
		return doc + "\n" + g.object(0) + "\n", true
	case 1:
		return doc + "} token=" + g.secret(), false
	case 2:
		return "cli failed (output: " + doc + ")", false
	case 3:
		// A bounded preview: a JSON string holding the document cut short.
		cut := doc[:g.r.Intn(len(doc)+1)]
		return `{"_pp_truncated":true,"preview":` + quoteJSON(cut) + `}`, true
	}
	return doc, true
}

func resetWithheldMemory() {
	withheldMu.Lock()
	withheldValues = map[string]bool{}
	withheldMu.Unlock()
}

func leaked(out string) bool {
	return strings.Contains(out, secretMarker) || strings.Contains(out, numberMarker)
}

// TestReadOnlyRedactionProperty is the brute-force guard for the class: for
// thousands of random documents no minted secret survives Final, Final(Raw)
// (the bound-then-funnel pipeline), or Raw on top-level JSON, whatever the
// nesting, duplicate keys, key spelling, JSON-in-string or trailing text.
func TestReadOnlyRedactionProperty(t *testing.T) {
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	for seed := int64(1); seed <= 4000; seed++ {
		g := &propGen{r: rand.New(rand.NewSource(seed))}
		text, pureJSON := g.topLevel()
		resetWithheldMemory()
		if out := Final(text); leaked(out) {
			t.Fatalf("seed %d: Final leaked a secret\ninput:  %s\noutput: %s", seed, text, out)
		}
		resetWithheldMemory()
		if out := Final(Raw(text)); leaked(out) {
			t.Fatalf("seed %d: Final(Raw) leaked a secret\ninput:  %s\noutput: %s", seed, text, out)
		}
		if pureJSON {
			resetWithheldMemory()
			if out := Raw(text); leaked(out) {
				t.Fatalf("seed %d: Raw leaked a secret from top-level JSON\ninput:  %s\noutput: %s", seed, text, out)
			}
		}
	}
}

// TestRedactionKnownBypasses pins the two reported shapes.
func TestRedactionKnownBypasses(t *testing.T) {
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	for _, in := range []string{
		`{"data":{"meta":"{\"ok\":1}} token=SECRETX1"}}`,
		`{"token":"SECRETX2","token":null}`,
		`{"meta":"{\"token\":\"SECRETX3\",\"token\":null}"}`,
		`{"name":"a","meta":"{\"tok\\u0065n\":\"SECRETX4\"","x":1}`,
		`{"meta":"token=[\"SECRETX5\"]"}`,
		`{"meta":"{\"token\":\"SECRETX6\"}","meta":"x"}`,
		`x {"name":"a","name":"{\"pin\":\"SECRETX7\"}","name":"b"} y`,
		`{"meta":"password=true!SECRETX8"}`,
		`{"meta":"token=null~SECRETX9"}`,
		`{"meta":"pin: [withheld: credential]SECRETX10"}`,
		`{"meta":"secret_key=false#SECRETX11"}`,
	} {
		resetWithheldMemory()
		for name, fn := range map[string]func(string) string{"Final": Final, "Raw": Raw} {
			if out := fn(in); strings.Contains(out, "SECRETX") {
				t.Fatalf("%s leaked: %s -> %s", name, in, out)
			}
		}
	}
}

// TestStringIsJSONOnlyWhenExactlyOneValue: a string is decoded as JSON only
// when it is exactly one JSON value followed by whitespace; otherwise the
// text rule runs over the whole string (the reported meta value becomes the
// placeholder, its trailing text is not silently dropped or kept).
func TestStringIsJSONOnlyWhenExactlyOneValue(t *testing.T) {
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	for in, want := range map[string]bool{`{"a":1}`: true, "{\"a\":1} \n\t": true, `"x"`: true, `{"ok":1}} token=S`: false, `{"a":1}{"b":2}`: false, `{"a":1} x`: false, `[1]]`: false} {
		if _, got := decodeExactlyOne(in); got != want {
			t.Errorf("decodeExactlyOne(%q) = %v, want %v", in, got, want)
		}
	}
	resetWithheldMemory()
	out := Final(`{"data":{"meta":"{\"ok\":1}} token=SECRETX1"}}`)
	if !strings.Contains(out, `"meta":"`+WithheldFieldPlaceholder+`"`) {
		t.Fatalf("the trailing-text string was not handed to the text rule: %s", out)
	}
}
