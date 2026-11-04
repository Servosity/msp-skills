// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// Regression tests for the classes the first review found. Each names the
// contract section it holds the kit to.

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"
)

// Contract 5 (config writes are a covered sink, with no exemption).
func TestContract5ConfigWriteHasNoExemption(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "ck.toml")
	resetForTest(Declarations{ConfigFields: []string{"token"}, Files: func([]string) []string { return []string{cfg} }})
	Hold("minted:session", "sensitive-12345")
	if err := WriteFile(cfg, []byte("token = \"sensitive-12345\"\n"), 0o600); !errors.Is(err, ErrWithheld) {
		t.Errorf("a config write carrying a held credential was not withheld: %v", err)
	}
	if b, _ := os.ReadFile(cfg); bytes.Contains(b, []byte("sensitive-12345")) {
		t.Error("the credential reached the config file")
	}
}

// Contract 4: text the kit writes itself passes the check like any other.
func TestContract4KitTextIsChecked(t *testing.T) {
	resetForTest(Declarations{})
	Hold("env:CK_TOKEN", "unreadable")
	called := false
	h := MCPHTTP(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	req := httptest.NewRequest("POST", "/mcp", io.NopCloser(iotest.ErrReader(errors.New("broken"))))
	rec := httptest.NewRecorder()
	serveAborting(h, rec, req) // kit14rv: an unread body marks acquisition unknown, so the refusal may fail closed
	if called || strings.Contains(strings.ToLower(rec.Body.String()), "unreadable") {
		t.Errorf("the kit's own error text carried a held value: %q", rec.Body.String())
	}
}

// Contract 1: every credential of a credential file, whatever its shape.
func TestContract1ConfigShapes(t *testing.T) {
	for name, tc := range map[string]struct {
		decl    Declarations
		file    string
		content string
		want    []string
	}{
		"json root array": {Declarations{StoredRecords: []string{"access_token"}}, "credentials.json",
			"[{\"access_token\":\"array-secret-123\"}]\n", []string{"array-secret-123"}},
		"json duplicate keys": {Declarations{StoredRecords: []string{"token"}}, "c.json",
			`{"token":"dup-first-1111","token":"dup-second-2222"}`, []string{"dup-first-1111", "dup-second-2222"}},
		"yaml block list": {Declarations{ConfigFields: []string{"refresh_tokens"}}, "c.yaml",
			"refresh_tokens:\n  - yaml-list-aaaa1\n  - \"yaml-list-bbbb2\"\nother: x\n", []string{"yaml-list-aaaa1", "yaml-list-bbbb2"}},
		"yaml nested mapping": {Declarations{ConfigFields: []string{"auth"}}, "c.yaml",
			"auth:\n  access: yaml-nested-cccc3\n  user:\n    pass: yaml-deep-dddd4\nport: 8080\n", []string{"yaml-nested-cccc3", "yaml-deep-dddd4"}},
	} {
		gatherString(t, tc.decl, tc.file, tc.content)
		for _, v := range tc.want {
			if !isHeld(v) {
				t.Errorf("%s: %q not held", name, v)
			}
		}
		if !preciseConfig([]byte(tc.content)) {
			// Loop ruling 9: every string value of an imprecise file is
			// held (numbers are not).
			if (strings.Contains(tc.content, "other: x") && !isHeld("x")) || isHeld("8080") {
				t.Errorf("%s: ruling 9 did not hold the file's strings, or held a number", name)
			}
			continue
		}
		if isHeld("8080") || isHeld("x") {
			t.Errorf("%s: a non-credential value was held", name)
		}
	}
}

// Contract 1: an identifier exempts exactly the source it names.
func TestContract1ExemptionIsExact(t *testing.T) {
	resetForTest(Declarations{Identifiers: []Identifier{{Source: "env:token", Reason: "the lowercase variable is an account identifier"}}})
	Hold("env:TOKEN", "upper-secret-123")
	Hold("env:token", "account42")
	if !isHeld("upper-secret-123") {
		t.Error("an identifier for env:token exempted env:TOKEN")
	}
	if isHeld("account42") {
		t.Error("the declared identifier was held")
	}
	gatherString(t, Declarations{ConfigFields: []string{"token"}, Identifiers: []Identifier{{Source: "config:token", Reason: "a display name"}}},
		"c.toml", "Token = \"cfg-upper-4567\"\ntoken = \"display\"\n")
	if !isHeld("cfg-upper-4567") {
		t.Error("an identifier for config:token exempted config:Token")
	}
}

// Contract 1: no exemption for any value, whitespace included.
func TestContract1WhitespaceValueHeld(t *testing.T) {
	resetForTest(Declarations{})
	Hold("env:TOKEN", "        ")
	if !leaks("a        b") {
		t.Error("a credential of eight spaces is not held")
	}
}

// Contract 1: the base URL's secrets, from every source of it.
func TestContract1BaseURLSources(t *testing.T) {
	d := Declarations{BaseURL: BaseURL{Default: "https://u:default-secret-123@api.example"}}
	resetForTest(d)
	gather(d, nil)
	if !isHeld("default-secret-123") {
		t.Error("the default base URL's password is not held")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "c.toml")
	os.WriteFile(p, []byte("[api]\nbase_url = \"https://u:nested-secret-123@api.example\"\n[other]\nbase_url = \"https://elsewhere.example\"\n"), 0o600)
	d = Declarations{BaseURL: BaseURL{ConfigKey: "api.base_url"}, Files: func([]string) []string { return []string{p} }}
	resetForTest(d)
	gather(d, nil)
	if !isHeld("nested-secret-123") {
		t.Error("a base URL under a dotted config key is not held")
	}
	if reg.baseURL != "https://u:nested-secret-123@api.example" {
		t.Errorf("base URL = %q, want the one under api.base_url", reg.baseURL)
	}
}

func responseWith(t *testing.T, h http.HandlerFunc) (*http.Client, func()) {
	t.Helper()
	srv, _ := testServer(t, true, h)
	return guarded(t, srv), srv.Close
}

// Contract 1: a declared response field is held before the program can
// read it, however long or encoded the body.
func TestContract1ResponseFieldAcquisition(t *testing.T) {
	zl := func(s string) []byte {
		var b bytes.Buffer
		w := zlib.NewWriter(&b)
		io.WriteString(w, s)
		w.Close()
		return b.Bytes()
	}
	raw := func(s string) []byte {
		var b bytes.Buffer
		w, _ := flate.NewWriter(&b, 5)
		io.WriteString(w, s)
		w.Close()
		return b.Bytes()
	}
	for name, tc := range map[string]struct {
		enc  string
		body []byte
		tok  string
	}{
		"after 64 MiB":  {"", []byte(strings.Repeat(" ", 64<<20+10) + `{"access_token":"late-token-abcdef"}`), "late-token-abcdef"},
		"deflate zlib":  {"deflate", zl(`{"access_token":"fresh-zlib-token-abcdef"}`), "fresh-zlib-token-abcdef"},
		"deflate raw":   {"deflate", raw(`{"access_token":"fresh-raw-token-abcdef"}`), "fresh-raw-token-abcdef"},
		"form encoding": {"", []byte("access_token=form-secret-678910&scope=read"), "form-secret-678910"},
	} {
		c, done := responseWith(t, func(w http.ResponseWriter, r *http.Request) {
			if tc.enc != "" {
				w.Header().Set("Content-Encoding", tc.enc)
			}
			w.Write(tc.body)
		})
		resetForTest(Declarations{ResponseFields: []string{"access_token"}, Origins: []Origin{{Source: "*", Allowed: []string{"https://api.example.com"}}}})
		req, _ := http.NewRequest("GET", "https://api.example.com/token", nil)
		req.Header.Set("Accept-Encoding", "deflate")
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		// kit15rv: a body with declared fields is read whole before the
		// program reads it; one past maxEncodedBody fails closed (every
		// value unknown, every unit withheld) instead of streaming.
		if !isHeld(tc.tok) && !(name == "after 64 MiB" && isUnknown()) {
			t.Errorf("%s: the response field was not held", name)
		}
		done()
	}
}

// Contract 1: an unreadable encoding cannot hide a response field.
func TestContract1UnknownResponseEncodingFailsClosed(t *testing.T) {
	c, done := responseWith(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "br")
		w.Write([]byte{0x1b, 0x2a, 0x00, 0x00})
	})
	defer done()
	resetForTest(Declarations{ResponseFields: []string{"access_token"}, Origins: []Origin{{Source: "*", Allowed: []string{"https://api.example.com"}}}})
	req, _ := http.NewRequest("GET", "https://api.example.com/token", nil)
	req.Header.Set("Accept-Encoding", "br")
	resp, err := c.Do(req)
	if err == nil {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
	if !leaks("anything at all") {
		t.Error("a response body in an encoding the kit cannot read did not fail closed")
	}
}

// Contract 1: the field is held before any byte of it reaches the program.
func TestContract1ResponseFieldHeldBeforeItsBytes(t *testing.T) {
	resetForTest(Declarations{ResponseFields: []string{"access_token"}})
	body := `{"note":"x","access_token":"stream-secret-24680","n":1}`
	r := newFieldReader(iotest.OneByteReader(strings.NewReader(body)), []string{"access_token"})
	var got []byte
	buf := make([]byte, 1)
	for {
		n, err := r.Read(buf)
		got = append(got, buf[:n]...)
		if bytes.Contains(got, []byte(`"access_token":"s`)) && !isHeld("stream-secret-24680") {
			t.Fatalf("the program read part of the field before it was held: %q", got)
		}
		if err != nil {
			break
		}
	}
	if string(got) != body {
		t.Errorf("the body changed: %q", got)
	}
}

// Contract 3: every decoding, recursively, with no decoder limited to once
// per path.
func TestContract3RecursionWithoutOncePerPath(t *testing.T) {
	holding(t, "abcdefgh")
	if !leaks("oz9jpKWmqUH=") { // rot13(base64(rot13("abcdefgh")))
		t.Error("rot13, base64, rot13 not decoded")
	}
	holding(t, cred)
	u16 := string(utf16Bytes(string(utf16Bytes(cred, false)), false))
	if !leaks(u16) {
		t.Error("UTF-16 of UTF-16 not decoded")
	}
	pwsh := string(utf16Bytes("powershell -EncodedCommand "+base64.StdEncoding.EncodeToString(utf16Bytes("$t='"+cred+"'", false)), false))
	if !leaks(pwsh) {
		t.Error("UTF-16 of base64 of UTF-16 (a PowerShell encoded command) not decoded")
	}
	inner := "\x00\x01\x02" + base64.StdEncoding.EncodeToString([]byte(cred)) + "\x03"
	outer := "\x00\x01\x02" + base64.StdEncoding.EncodeToString([]byte(inner)) + "\x03"
	if !leaks(outer) {
		t.Error("base64 inside binary inside base64 inside binary not decoded")
	}
	if !leaks(string(df([]byte(base64.StdEncoding.EncodeToString(df([]byte(cred))))))) {
		t.Error("raw deflate of base64 of raw deflate not decoded")
	}
}

// Contract 3: a short spelling of a value is never pruned by a length gate.
func TestContract3NoLengthPruning(t *testing.T) {
	holding(t, string([]byte{0, 0, 0, 0, 0, 0, 0, 0}))
	if !leaks("<~zz~>") {
		t.Error("Ascii85 'z' groups decoding to the held value were pruned")
	}
	holding(t, "0000042")
	if !leaks("NDI=") { // base64("42")
		t.Error("base64 of the short integer spelling of a digit-only credential was pruned")
	}
}

// Contract 1: the streaming scan holds a declared field in every shape a
// response carries it, holds nothing else, and changes no byte.
func TestContract1ResponseFieldShapes(t *testing.T) {
	for _, tc := range []struct {
		body       string
		held, free []string
	}{
		{`{"access_token":"json-tok-11111","token_type":"bearer"}`, []string{"json-tok-11111"}, []string{"bearer", "token_type"}},
		{`{"data":{"access_token":"nested-tok-2222"}}`, []string{"nested-tok-2222"}, []string{"data"}},
		{`{"access_token":{"value":"obj-tok-3333","ttl":60},"after":"plain-77"}`, []string{"obj-tok-3333", "60", "ttl"}, []string{"plain-77"}},
		{`[{"access_token":"arr-tok-4444"},{"access_token": ["a-55555555", 9876543210]}]`, []string{"arr-tok-4444", "a-55555555", "9876543210"}, nil},
		{`{"Access_Token" : "case-tok-6666" , "n": 1}`, []string{"case-tok-6666"}, []string{"1"}},
		{`{"access_token":"esc-tok\"7777"}`, []string{`esc-tok"7777`, `esc-tok\"7777`}, nil},
		{`scope=read&access_token=form%2Btok+8888`, []string{"form+tok 8888", "form%2Btok+8888"}, []string{"read"}},
		{"expires_in: 30\naccess_token: yaml-tok 9999\n", []string{"yaml-tok 9999"}, []string{"30"}},
		{`{"note":"access_token","x":"not-a-field-1"}`, nil, []string{"not-a-field-1"}},
		{`{"access_token":"unterminated-tok-1`, []string{"unterminated-tok-1"}, nil},
	} {
		resetForTest(Declarations{ResponseFields: []string{"access_token"}})
		got, err := io.ReadAll(newFieldReader(iotest.HalfReader(strings.NewReader(tc.body)), []string{"access_token"}))
		if err != nil || string(got) != tc.body {
			t.Errorf("%s: body changed to %q (%v)", tc.body, got, err)
		}
		for _, v := range tc.held {
			if !isHeld(v) {
				t.Errorf("%s: %q not held", tc.body, v)
			}
		}
		for _, v := range tc.free {
			if isHeld(v) {
				t.Errorf("%s: %q held", tc.body, v)
			}
		}
	}
}

// Contract 3: normalization can spell a value in fewer bytes than it has
// characters ("Ⅷ" is three bytes for "viii"); no gate prunes that spelling.
func TestContract3NoPruningOfCompatibilitySpellings(t *testing.T) {
	holding(t, "VIIIabcd")
	if !leaks(base64.StdEncoding.EncodeToString([]byte("Ⅷabcd"))) {
		t.Error("base64 of a compatibility spelling shorter than the value was pruned")
	}
	if !leaks("\x00\x01" + string(utf16Bytes("Ⅷabcd", false)) + "\x02") {
		t.Error("UTF-16 of a compatibility spelling was cut out of its run")
	}
}
