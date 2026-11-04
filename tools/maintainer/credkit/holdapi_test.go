// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
)

var holdAPIDecl = Declarations{CredentialFields: []string{"password", "token", "api_key"}}

func heldAfterBody(body string) func(string) bool {
	resetForTest(holdAPIDecl)
	HoldBody([]byte(body))
	return isHeld
}

// The inputs reviews found in connectors' own readers (superops nb-codex-r1 to
// r3), and the shapes around them.
func TestHoldBodyReadsWhatTheReviewsFound(t *testing.T) {
	pad := "{\"pad\":[" + strings.Repeat("{\"token\":null},", 255) + "{\"token\":null}],\"password\":987654321}"
	for _, tc := range []struct{ name, body, want string }{
		{"a number", `{"password":123456789}`, "123456789"},
		{"a number's spelling", `{"password":1.50e3}`, "1.50e3"},
		{"a duplicate key, nested", `{"password":{"value":987654321,"value":""}}`, "987654321"},
		{"past 256 fields", pad, "987654321"},
		{"the first of duplicates", `{"token":"first-value-A1","token":"second-value"}`, "first-value-A1"},
		{"the second of duplicates", `{"token":"first-value-A1","token":"second-value"}`, "second-value"},
		{"a TOML list", "password=['AmberCobaltRiver']", "AmberCobaltRiver"},
		{"a signed number in a list", "password=[+987654321]", "+987654321"},
		{"an escaped key", `{"password":"escaped-key-value"}`, "escaped-key-value"},
		{"any case", `{"PassWord":"casefold-value"}`, "casefold-value"},
		{"deep", `{"a":{"b":[{"c":{"token":"deep-value-77"}}]}}`, "deep-value-77"},
		{"a form", "a=b&password=form-value-77", "form-value-77"},
		{"a percent-encoded form name", "a=b&%70assword=form-value-78", "form-value-78"},
		{"JSON lines", "{\"a\":1}\n{\"token\":\"line-value-77\"}\n", "line-value-77"},
		{"TOML", "[auth]\napi_key = \"toml-value-77\"\n", "toml-value-77"},
		{"a built-in name", `{"authorization":"Bearer built-in-value"}`, "built-in-value"},
		{"a line break after the colon (Codex r1)", "{\"password\":\n\"AmberCobalt77\"}", "AmberCobalt77"},
		{"pretty-printed, a nested number", "{\n  \"password\" :\n  {\n    \"value\" :\n    123456789\n  }\n}\n", "123456789"},
		{"JSON lines, a line break after the colon", "{\"a\":1}\n{\"token\":\n\"line-value-78\"}\n", "line-value-78"},
		{"truncated JSON: the field", `{"password":"cut-short-77`, "cut-short-77"},
		{"truncated JSON: any other string", `{"other":"unparsed-string-77","x":`, "unparsed-string-77"},
		{"a truncated string of digits (Codex r2)", "\"987654321", "987654321"},
		{"a truncated string of punctuation (Codex r2)", "\"!!!!!!!!", "!!!!!!!!"},
		{"a bare number in no document", "{ 987654321", "987654321"},
		{"a space JSON does not allow, before JSON (Codex r4)", "\u00a0{\"password\":\"AmberCobalt79\"}", "AmberCobalt79"},
		{"the same, in JSON lines", "\u00a0{\"token\":\"AmberCobalt80\"}\n{\"a\":1}\n", "AmberCobalt80"},
		{"the same, before TOML", "\u00a0password = \"AmberCobalt81\"\n", "AmberCobalt81"},
		{"the same, before a form", "\u00a0password=" + "AmberCobalt82", "AmberCobalt82"},
		{"a truncated string of structure (Codex r3)", "\"{}[]{}[]", "\"{}[]{}[]"},
		{"not a document", "<xml a=\"unparsed-attr-77\">", "unparsed-attr-77"},
	} {
		if held := heldAfterBody(tc.body); !held(tc.want) {
			t.Errorf("%s: %q was not held from %.80q; held %q", tc.name, tc.want, tc.body, heldValues())
		}
	}
	// Bytes that are no document are held whole, so no spelling of a value in
	// them can reach a sink in 8 characters or more.
	for _, body := range []string{"\"{}[]{}[]", "\"{} {}{}{}{}", "\"!!!! !!!!", "not a [document] {at all} !!!!!!!!"} {
		heldAfterBody(body)
		if !withheld([]byte(body)) {
			t.Errorf("%q reached a sink", body)
		}
	}
	// A value held is withheld at a sink, the preview included.
	heldAfterBody(`{"password":123456789}`)
	var out bytes.Buffer
	NewWriter(&out, SinkFile).Write([]byte(`{"password":123456789}`))
	if strings.Contains(out.String(), "123456789") {
		t.Errorf("the document reached a sink: %q", out.String())
	}
}

// A body that parses keeps what is no credential, or every preview of an
// ordinary document would be withheld.
func TestHoldBodyLeavesOrdinaryDocumentsReadable(t *testing.T) {
	for _, body := range []string{
		`{"name":"Printer offline","nested":{"state":"open","tags":["a","b"]},"count":3}`,
		"{\"a\":\"first line\"}\n{\"a\":\"second line\"}\n",
		"name = \"Printer offline\"\nstate = \"open\"\n",
		"state=open&limit=50",
		`{"variables":{"id":"123"},"query":"{ ticket { id } }"}`,
		"\xef\xbb\xbf{\"name\":\"Printer offline\"}", // Codex r1: a byte-order mark
	} {
		resetForTest(holdAPIDecl)
		HoldBody([]byte(body))
		if v := heldValues(); len(v) != 0 {
			t.Errorf("%.60q held %q", body, v)
		}
		if withheld([]byte(body)) {
			t.Errorf("%.60q was withheld", body)
		}
	}
}

// A request's body gets the same reading: the value after a line break is held
// before the request is judged.
func TestRequestBodyReadsALineBreakAfterTheColon(t *testing.T) {
	resetForTest(holdAPIDecl)
	req, _ := http.NewRequest("POST", "https://outside.example/", strings.NewReader("{\"password\":\n\"AmberCobalt78\"}"))
	(&http.Client{Transport: Transport(&http.Transport{})}).Do(req)
	if !isHeld("AmberCobalt78") {
		t.Errorf("not held; held %q", heldValues())
	}
}

// Too large to read does not end clean.
func TestHoldBodyPastTheLimitWithholds(t *testing.T) {
	prev := maxDeclared
	maxDeclared = 16
	defer func() { maxDeclared = prev }()
	resetForTest(holdAPIDecl)
	HoldBody([]byte(`{"name":"a body past the limit"}`))
	if !withheld([]byte("anything at all")) {
		t.Error("a body too large to read left the held set known")
	}
}

// The inputs reviews found in connectors' own URL readers (quickbooks and
// zammad nb-codex rounds), and the two forms they were missing.
func TestHoldURLReadsWhatTheReviewsFound(t *testing.T) {
	for _, tc := range []struct{ name, url, want string }{
		{"a password", "https://ops:pass-word-77@host.example/", "pass-word-77"},
		{"a password with a semicolon", "https://ops:;ProfilePass77@wiki.test/", ";ProfilePass77"},
		{"an escaped password, decoded", "https://ops:p%40ss-word-77@host.example/", "p@ss-word-77"},
		{"an escaped password, as written", "https://ops:p%40ss-word-77@host.example/", "p%40ss-word-77"},
		{"schemeless", "ops:secret-77@proxy.example:3128", "secret-77"},
		{"a network path", "//ops:secret-78@host.example/", "secret-78"},
		{"a port no parser reads", "https://ops:secret-79@host.example:bad/", "secret-79"},
		{"a percent-encoded query name", "https://example.test/?%74oken=rosebud987654", "rosebud987654"},
		{"a value of punctuation", "https://example.test/?%74oken=!!!!!!!!", "!!!!!!!!"},
		{"an encoded value before a fragment", "https://h.test/?token=%71%37%5A#top", "q7Z"},
		{"the same, as written", "https://h.test/?token=%71%37%5A#top", "%71%37%5A"},
		{"a value trimming would empty", "https://h.test/?token=%20%20%20", "   "},
		{"a fragment parameter", "https://h.test/cb#access_token=frag-value-77", "frag-value-77"},
		{"around whitespace", "  https://ops:spaced-77@host.example/?k=v  ", "spaced-77"},
		{"a password holding a scheme's delimiter (Codex r1)", "ops:Cobalt7://River9@proxy.example:3128", "Cobalt7://River9"},
		{"a query that does not parse", "https://h.test:bad/?token=broken-url-77", "broken-url-77"},
	} {
		resetForTest(Declarations{})
		HoldURL("minted:test", tc.url)
		if !isHeld(tc.want) {
			t.Errorf("%s: %q not held from %q; held %q", tc.name, tc.want, tc.url, heldValues())
		}
	}
}

func TestHoldURLLeavesOrdinaryParts(t *testing.T) {
	resetForTest(Declarations{})
	for _, u := range []string{"https://host.example/a/b", "proxy.example:3128", "host.example", "//host.example/x", "ops@example.test", "https://host.example/a@b"} {
		HoldURL("minted:test", u)
	}
	if v := heldValues(); len(v) != 0 {
		t.Errorf("held %q", v)
	}
}

// A declared identifier exempts a query value by its decoded name, under its
// source, and nothing else.
func TestHoldURLHonorsIdentifiers(t *testing.T) {
	d := Declarations{Identifiers: []Identifier{{Source: "query:version", Reason: "an API version"}, {Source: "minted:public", Reason: "test"}}}
	resetForTest(d)
	HoldURL("minted:test", "https://h.test/?%76ersion=2026-11&token=still-held-77")
	if isHeld("2026-11") {
		t.Error("an exempt query value was held")
	}
	if !isHeld("still-held-77") {
		t.Error("the query value beside it was not held")
	}
	resetForTest(d)
	HoldURL("minted:public", "https://h.test/?token=exempt-source-77")
	if isHeld("exempt-source-77") {
		t.Error("an exempt source was held")
	}
}
