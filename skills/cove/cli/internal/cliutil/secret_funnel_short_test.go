// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated), cove only: tests for the short-credential
// funnel and the JSON success-data check (secret_funnel_short.go). Hand-fix
// code: cove-short-credential-funnel.

package cliutil

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"net/url"
	"strings"
	"testing"
)

// shortBS is one backslash, built at run time so no fixture loses an escape.
var shortBS = string(rune(92))

// shortEncoders are the encodings a server or a log line applies, each of
// which the shared funnel's steps decode.
var shortEncoders = map[string]func(string) string{
	"b64":     func(v string) string { return base64.StdEncoding.EncodeToString([]byte(v)) },
	"b64url":  func(v string) string { return base64.RawURLEncoding.EncodeToString([]byte(v)) },
	"hex":     func(v string) string { return hex.EncodeToString([]byte(v)) },
	"HEX":     func(v string) string { return strings.ToUpper(hex.EncodeToString([]byte(v))) },
	"query":   url.QueryEscape,
	"pct-all": func(v string) string { return pctEveryShort(v) },
	"json-u":  func(v string) string { return jsonUShort(v) },
	"html":    html.EscapeString,
	"html-#":  func(v string) string { return htmlNumShort(v) },
}

func pctEveryShort(v string) string {
	var b strings.Builder
	for i := 0; i < len(v); i++ {
		fmt.Fprintf(&b, "%%%02x", v[i])
	}
	return b.String()
}

func jsonUShort(v string) string {
	var b strings.Builder
	for _, r := range v {
		fmt.Fprintf(&b, "%su%04x", shortBS, r)
	}
	return b.String()
}

func htmlNumShort(v string) string {
	var b strings.Builder
	for _, r := range v {
		fmt.Fprintf(&b, "&#%d;", r)
	}
	return b.String()
}

// Codex round 2: every stack of up to three encodings of a short secret is
// withheld, not just one layer.
func TestShortFunnelComposesEveryStackOfEncodings(t *testing.T) {
	names := make([]string, 0, len(shortEncoders))
	for n := range shortEncoders {
		names = append(names, n)
	}
	for _, secret := range []string{"Kp4$w", "secret", "a1b"} {
		for _, a := range names {
			for _, b := range names {
				for _, c := range append([]string{""}, names...) {
					v := shortEncoders[b](shortEncoders[a](secret))
					label := a + "+" + b
					if c != "" {
						v = shortEncoders[c](v)
						label += "+" + c
					}
					text := "upstream said " + v + " and stopped"
					if !FunnelEchoesAll(text, []string{secret}) {
						t.Errorf("%q under %s not withheld: %q", secret, label, text)
					}
				}
			}
		}
	}
	for _, clean := range []string{"upstream said nothing", "token aGVsbG8gd29ybGQ= refused", "id 4f2a9c refused"} {
		if FunnelEchoesAll(clean, []string{"Kp4$w"}) {
			t.Errorf("clean text withheld: %q", clean)
		}
	}
}

// The short search fails closed exactly as the shared one does: past the
// input limit or the work cap it reports an echo.
func TestShortFunnelFailsClosed(t *testing.T) {
	if !funnelShortEchoes(strings.Repeat("x", funnelMaxInput+1), []string{"Kp4$w"}) {
		t.Fatal("text over the input limit was not withheld")
	}
	perByte, base := funnelWorkPerByte, funnelWorkBase
	funnelWorkPerByte, funnelWorkBase = 1, 1
	defer func() { funnelWorkPerByte, funnelWorkBase = perByte, base }()
	if !funnelShortEchoes("token "+base64.StdEncoding.EncodeToString([]byte("unrelated words here")), []string{"Kp4$w"}) {
		t.Fatal("work cap reached without withholding")
	}
}

// Codex round 2: every scalar of a success document is checked, in every
// rendering, and the document as written.
func TestFunnelJSONEchoesChecksEveryScalar(t *testing.T) {
	esc := func(s string) string { return strings.ReplaceAll(s, "~", shortBS) }
	for name, tc := range map[string]struct{ secret, doc string }{
		"integer":            {"12345678", `{"n":12345678}`},
		"exponent":           {"12345678", `{"n":1.2345678e7}`},
		"upper exponent":     {"12345678", `{"n":1.2345678E+7}`},
		"fraction":           {"12345678", `{"n":1234.5678}`},
		"negative exponent":  {"12345678", `{"n":12345678000e-3}`},
		"float rendering":    {"1.2345678e+07", `{"n":12345678}`},
		"deep array":         {"12345678", `[[[[{"a":[1,2,[12345678]]}]]]]`},
		"true":               {"true", `{"ok":true}`},
		"false":              {"false", `[false]`},
		"null":               {"null", `{"v":null}`},
		"member name":        {"secret123456", `{"secret123456":1}`},
		"escaped name":       {"secret123456", esc(`{"~u0073ecret123456":1}`)},
		"duplicate member":   {"secret123456", `{"a":"c2VjcmV0MTIzNDU2","a":"x"}`},
		"second value":       {"secret123456", `{} {"a":"secret123456"}`},
		"invalid json":       {"secret123456", `{"a":c2VjcmV0MTIzNDU2}`},
		"invalid utf8":       {"secret123456", "{\"a\":\"secret\xff123456\"}"},
		"short as written":   {"Kp4$w", `{"a":1,"Kp4$w":2}`},
		"short number":       {"4711", `{"n":4.711e3}`},
		"short in base64x2":  {"secret", `{"m":"YzJWamNtVjA="}`},
		"huge exponent":      {"secret123456", `{"n":1e999999}`},
		"exact over a bound": {"secret123456", `{"n":1e5000}`},
	} {
		t.Run(name, func(t *testing.T) {
			if !FunnelJSONEchoes([]byte(tc.doc), []string{tc.secret}) {
				t.Fatalf("%s: %q not refused for %q", name, tc.doc, tc.secret)
			}
		})
	}
	clean := `{"result":{"n":1234567,"f":0.5,"ok":false,"v":null,"rows":[{"Id":42,"Name":"fine"}]}}`
	if FunnelJSONEchoes([]byte(clean), []string{"12345678", "secret123456"}) {
		t.Fatalf("clean document refused: %s", clean)
	}
}

// FunnelJSONScan visits every token with the decoded names of the members it
// sits under, keeps duplicates, and reports whether the stream was complete.
func TestFunnelJSONScanVisitsEveryToken(t *testing.T) {
	doc := strings.ReplaceAll(`{"~u0061":{"b":[1,{"c":"x"}]},"a":true} [null]`, "~", shortBS)
	var got []string
	complete := FunnelJSONScan([]byte(doc), func(s FunnelJSONScalar) bool {
		got = append(got, fmt.Sprintf("%v/%s=%v", s.Key, strings.Join(s.Members, "."), s.Value))
		return false
	})
	want := []string{"true/=a", "true/a=b", "false/a.b=1", "true/a.b=c", "false/a.b.c=x", "true/=a", "false/a=true", "false/=<nil>"}
	if !complete || strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("complete=%v tokens:\n got %q\nwant %q", complete, got, want)
	}
	for _, bad := range []string{`{"a":1`, `{"a":1}}`, `{"a" 1}`, `[1,]`} {
		if FunnelJSONScan([]byte(bad), func(FunnelJSONScalar) bool { return false }) {
			t.Errorf("%s reported complete", bad)
		}
	}
	var n json.Number
	FunnelJSONScan([]byte(`{"n":1.50e2}`), func(s FunnelJSONScalar) bool {
		if v, ok := s.Value.(json.Number); ok {
			n = v
		}
		return false
	})
	if n != "1.50e2" {
		t.Fatalf("a number was not kept as written: %q", n)
	}
}

// The number renderings: exact decimal without exponent or stray zeros, the
// digits alone, and the float64 forms.
func TestFunnelNumberForms(t *testing.T) {
	for lit, want := range map[string][]string{
		"1.2345678e7":  {"12345678", "1.2345678e+07"},
		"-0.00120e3":   {"1.2", "-1.2", "120"},
		"123456.78E2":  {"12345678"},
		"0":            {"0"},
		"12345678e-10": {"0.0012345678", "12345678"},
	} {
		forms, ok := funnelNumberForms(lit)
		if !ok {
			t.Fatalf("%s refused", lit)
		}
		for _, w := range want {
			found := false
			for _, f := range forms {
				if f == w {
					found = true
				}
			}
			if !found {
				t.Errorf("%s: form %q missing from %q", lit, w, forms)
			}
		}
	}
	if _, ok := funnelNumberForms("1e5000"); ok {
		t.Error("an unbounded exact decimal was rendered")
	}
}

// FunnelTextViews reaches a structure hidden under two text encodings.
func TestFunnelTextViewsDecodesTwoLayers(t *testing.T) {
	text := "&quot;" + shortBS + "u0076isa&quot;"
	found := false
	for _, v := range FunnelTextViews(text) {
		if v == `"visa"` {
			found = true
		}
	}
	if !found {
		t.Fatalf("two-layer view missing: %q", FunnelTextViews(text))
	}
	if FunnelSettle(shortBS+"u0076isa") != "visa" {
		t.Fatal("FunnelSettle did not decode an escaped name")
	}
}
