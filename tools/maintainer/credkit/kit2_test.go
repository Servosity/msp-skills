// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// Regression tests for the classes the second review left open. Each test
// covers a class through several of its members, not one input.

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
)

// mustNotSend is an inner transport for a request the guard must refuse.
type mustNotSend struct{ t *testing.T }

func (m mustNotSend) RoundTrip(r *http.Request) (*http.Response, error) {
	m.t.Errorf("a request carrying a credential off its origins was sent: %s %s", r.Method, r.URL)
	return &http.Response{StatusCode: 200, Body: http.NoBody, Request: r}, nil
}

// fixedResponse is an inner transport that answers every request with h.
type fixedResponse struct {
	h    http.Header
	body []byte
}

func (f fixedResponse) RoundTrip(r *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Header: f.h.Clone(), Body: io.NopCloser(bytes.NewReader(f.body)), Request: r}, nil
}

func utf16LE(s string) []byte {
	var b []byte
	for _, u := range utf16.Encode([]rune(s)) {
		b = append(b, byte(u), byte(u>>8))
	}
	return b
}

// Class encoded-credential-acquisition: every acquisition path reads a
// declared content or transfer encoding (or a byte-order mark) through the
// kit's decoders before it holds.
func TestKit2EncodedAcquisition(t *testing.T) {
	post := func(body []byte, hdr ...string) error {
		req, _ := http.NewRequest("POST", "http://off.invalid/", bytes.NewReader(body))
		for i := 0; i+1 < len(hdr); i += 2 {
			req.Header.Set(hdr[i], hdr[i+1])
		}
		_, err := Transport(mustNotSend{t}).RoundTrip(req)
		return err
	}
	for name, tc := range map[string]struct {
		body []byte
		hdr  []string
		tok  string
	}{
		"gzip request body":    {gz([]byte(`{"password":"gz-secret-123"}`)), []string{"Content-Type", "application/json", "Content-Encoding", "gzip"}, "gz-secret-123"},
		"deflate request body": {zl([]byte(`{"password":"zlib-secret-456"}`)), []string{"Content-Encoding", "deflate"}, "zlib-secret-456"},
		"stacked codings":      {gz(zl([]byte("password=stacked-secret-789"))), []string{"Content-Encoding", "deflate, gzip"}, "stacked-secret-789"},
		"utf-16 charset body":  {utf16LE(`{"password":"wide-secret-0123"}`), []string{"Content-Type", "application/json; charset=utf-16le"}, "wide-secret-0123"},
	} {
		resetForTest(Declarations{CredentialFields: []string{"password"}})
		if err := post(tc.body, tc.hdr...); !errors.Is(err, ErrWithheld) {
			t.Errorf("%s: the request was not refused: %v", name, err)
		}
		if !isHeld(tc.tok) {
			t.Errorf("%s: %q was not held", name, tc.tok)
		}
	}

	// A response field under a declared charset, and a declared header in
	// a MIME encoded word.
	resetForTest(Declarations{ResponseFields: []string{"access_token", "x-session"}, Origins: []Origin{{Source: "*", Allowed: []string{"https://api.example.com"}}}})
	h := http.Header{"Content-Type": {"application/json; charset=utf-16le"}, "X-Session": {"=?utf-8?b?" + base64.StdEncoding.EncodeToString([]byte("mime-session-1234")) + "?="}}
	req, _ := http.NewRequest("GET", "https://api.example.com/token", nil)
	resp, err := Transport(fixedResponse{h, utf16LE(`{"access_token":"utf16-token-abcdef"}`)}).RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	for _, v := range []string{"utf16-token-abcdef", "mime-session-1234"} {
		if !isHeld(v) {
			t.Errorf("response: %q was not held", v)
		}
	}

	// MCP arguments in a gzip request body.
	resetForTest(Declarations{MCPArguments: []string{"api_key"}})
	srv := httptest.NewServer(MCPHTTP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
	defer srv.Close()
	mreq, _ := http.NewRequest("POST", srv.URL, bytes.NewReader(gz([]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"arguments":{"api_key":"mcp-gz-secret-1"}}}`))))
	mreq.Header.Set("Content-Encoding", "gzip")
	if r, err := http.DefaultClient.Do(mreq); err == nil {
		r.Body.Close()
	}
	if !isHeld("mcp-gz-secret-1") {
		t.Error("an MCP argument in a gzip body was not held")
	}

	// A credential file with a byte-order mark: UTF-8, UTF-16 LE and BE.
	for name, content := range map[string]string{
		"utf-8 bom":    "\ufefftoken = \"bom8-secret-1111\"\n",
		"utf-16le bom": "\xff\xfe" + string(utf16LE("token = \"bom16-secret-2222\"\n")),
		"utf-16be bom": "\xfe\xff" + string(utf16Bytes("token = \"bom16be-secret-3333\"\n", true)),
	} {
		gatherString(t, Declarations{ConfigFields: []string{"token"}}, "c.toml", content)
		want := map[string]string{"utf-8 bom": "bom8-secret-1111", "utf-16le bom": "bom16-secret-2222", "utf-16be bom": "bom16be-secret-3333"}[name]
		if !isHeld(want) {
			t.Errorf("config with a %s: %q not held", name, want)
		}
	}
}

// Class config-reader-drops-values: the config reader attaches every
// multiline and escaped form to its key, and holds a scalar it cannot
// classify (fail closed), in TOML, YAML, JSON and env files.
func TestKit2ConfigReaderHoldsEveryValue(t *testing.T) {
	for name, tc := range map[string]struct {
		file, content string
		want          []string
	}{
		"env multiline double quotes": {".env", "export TOKEN=\"env-line-one-1111\nenv-line-two-2222\"\nOTHER=x\n",
			[]string{"env-line-one-1111\nenv-line-two-2222"}},
		"yaml plain continuation": {"c.yaml", "token: plain-part-aaaa\n  plain-part-bbbb\nother: x\n",
			[]string{"plain-part-aaaa plain-part-bbbb"}},
		"yaml double-quoted multiline": {"c.yaml", "token: \"dq-part-aaaa\n  dq-part-bbbb\"\nother: x\n",
			[]string{"dq-part-aaaa dq-part-bbbb"}},
		"yaml alias": {"c.yaml", "common: &c alias-secret-123\ntoken: *c\nother: x\n",
			[]string{"alias-secret-123"}},
		"toml line-ending backslash": {"c.toml", "token = \"\"\"tok-part-aaaa\\\n    tok-part-bbbb\"\"\"\nother = \"x\"\n",
			[]string{"tok-part-aaaatok-part-bbbb"}},
		"json escapes": {"c.json", `{"token":"esc\u002dsecret\u002d1234","other":"x"}`,
			[]string{"esc-secret-1234"}},
		"unclassified scalar": {"c.toml", "token secret-no-sep-5678\nother = \"x\"\n",
			[]string{"secret-no-sep-5678"}},
	} {
		gatherString(t, Declarations{ConfigFields: []string{"token"}}, tc.file, tc.content)
		for _, v := range tc.want {
			if !isHeld(v) {
				t.Errorf("%s: %q not held", name, v)
			}
		}
		if !preciseConfig([]byte(tc.content)) {
			// Loop ruling 9: a file outside the canonical subset holds
			// every string value in it.
			if !isHeld("x") {
				t.Errorf("%s: an imprecise file's string value was not held", name)
			}
			continue
		}
		for _, v := range []string{"x", "export", "---"} {
			if isHeld(v) {
				t.Errorf("%s: a non-credential %q was held", name, v)
			}
		}
	}
}

// Classes response-field-context-loss and response-key-truncation: the
// response scanner reads JSON as the grammar defines it (whitespace between
// any two tokens, keys of any length) and fails closed on what it cannot
// parse.
func TestKit2ResponseScannerIsJSON(t *testing.T) {
	long := strings.Repeat("k", 1025)
	for _, tc := range []struct {
		field, body string
		held        []string
	}{
		{"access_token", "{\"access_token\"\n:\"newline-secret-123\"}", []string{"newline-secret-123"}},
		{"access_token", "{ \"access_token\" \r\n\t : \n \"ws-secret-1234\" }", []string{"ws-secret-1234"}},
		{"access_token", "[\n{\n\"access_token\"\n:\n[\n\"arr-ws-secret-1\"\n,\n7654321\n]\n}\n]", []string{"arr-ws-secret-1", "7654321"}},
		{long, `{"` + long + `":"long-key-secret-123"}`, []string{"long-key-secret-123"}},
		{"access_token", `{"access\u005ftoken":"esc-key-secret-1"}`, []string{"esc-key-secret-1"}},
		{"access_token", `{"x":1,'access_token':'single-quoted-123'}`, []string{"single-quoted-123"}},
		{"access_token", "\ufeff{\"access_token\":\"bom-secret-12345\"}", []string{"bom-secret-12345"}},
		// Every character escaped, each above U+FFFF a surrogate pair.
		{"🔑🔑🔑🔑🔑🔑", `{"` + strings.Repeat(`\ud83d\udd11`, 6) + `":"astral-key-secret-1"}`, []string{"astral-key-secret-1"}},
	} {
		resetForTest(Declarations{ResponseFields: []string{tc.field}})
		got, err := io.ReadAll(newFieldReader(strings.NewReader(tc.body), []string{tc.field}))
		if err != nil || string(got) != tc.body {
			t.Errorf("%.40q: body changed to %.40q (%v)", tc.body, got, err)
		}
		for _, v := range tc.held {
			if !isHeld(v) {
				t.Errorf("%.40q: %q not held", tc.body, v)
			}
		}
	}

	// What the grammar cannot place fails closed: a JSON body that breaks it
	// makes every value unknown. A body that only looks like JSON is read
	// leniently and does not.
	for _, tc := range []struct {
		ctype, body string
		unknown     bool
	}{
		{"application/json", `{"access_token" "no-colon-secret-1"}`, true},
		{"application/json", `{"a":1,}x:"two"]`, true},
		{"", `[{"access_token":"ok-secret-1"} "no-comma"]`, true},
		{"text/plain", "[INFO] started\n[WARN] access_token: lenient-secret-1\n", false},
		{"application/json", "{\"a\":[1,2,{\"b\":null}],\"c\":true}\n{\"d\":\"x\"}", false},
	} {
		resetForTest(Declarations{ResponseFields: []string{"access_token"}})
		mt, _, _ := mime.ParseMediaType(tc.ctype)
		io.ReadAll(newFieldReader(strings.NewReader(tc.body), []string{"access_token"}, modeFor(mt)))
		if _, unknown := currentMatcher(); unknown != tc.unknown {
			t.Errorf("%s %.40q: unknown = %v, want %v", tc.ctype, tc.body, unknown, tc.unknown)
		}
	}
}

// Class trailer-credentials-unheld: trailers are held and checked like
// headers, on requests and responses.
func TestKit2Trailers(t *testing.T) {
	resetForTest(Declarations{CredentialHeaders: []string{"X-Token"}})
	req, _ := http.NewRequest("POST", "http://off.invalid/", strings.NewReader("x"))
	req.TransferEncoding = []string{"chunked"}
	req.Trailer = http.Header{"X-Token": []string{"trailer-secret-123"}}
	if _, err := Transport(mustNotSend{t}).RoundTrip(req); !errors.Is(err, ErrWithheld) {
		t.Errorf("a request with a credential trailer to a plain-http origin was not refused: %v", err)
	}
	if !isHeld("trailer-secret-123") {
		t.Error("a request trailer was not held")
	}

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Trailer", "X-Session")
		io.WriteString(w, `{"ok":true}`)
		w.Header().Set("X-Session", "resp-trailer-secret-456")
	}))
	defer srv.Close()
	resetForTest(Declarations{ResponseFields: []string{"x-session"}, Origins: []Origin{{Source: "*", Allowed: []string{"https://api.example.com"}}}})
	c := guarded(t, srv)
	resp, err := c.Get("https://api.example.com/")
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	if resp.Trailer.Get("X-Session") == "" {
		t.Fatal("the test server sent no trailer")
	}
	if !isHeld("resp-trailer-secret-456") {
		t.Error("a response trailer was not held before the program could read it")
	}
	resp.Body.Close()
}

// Class mcp-replacement-prefix-unchecked: every byte the MCP layer emits,
// transport framing and replacement included, passes the final check.
func TestKit2MCPEveryEmittedByteChecked(t *testing.T) {
	for _, held := range []string{"event: m", "data: {\"", "\"}}\n\n", "rror\"}}\n"} {
		resetForTest(Declarations{})
		Hold("env:TOKEN", held)
		h := MCPHTTP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			io.WriteString(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":\"ok\"}\n\n")
		}))
		rec := httptest.NewRecorder()
		serveAborting(h, rec, httptest.NewRequest("POST", "/mcp", strings.NewReader("{}")))
		if bytes.Contains(rec.Body.Bytes(), []byte(held)) {
			t.Errorf("held %q: the SSE stream emitted it: %q", held, rec.Body.String())
		}
		var out bytes.Buffer
		fw := &frameWriter{w: &out}
		fw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":"` + strings.TrimSpace(held) + `"}` + "\n"))
		if bytes.Contains(out.Bytes(), []byte(held)) {
			t.Errorf("held %q: the stdio frame writer emitted it: %q", held, out.String())
		}
	}
}

// Classes decoder-length-pruning and decoder-binary-intermediate-pruning:
// no length gate or character-class filter skips a covered candidate.
func TestKit2NoPruning(t *testing.T) {
	gzOf := func(s string) string { return string(gz([]byte(s))) }
	for _, tc := range []struct {
		name, held, unit string
	}{
		{"base64 of a scientific spelling", "100000000", "MWU4"},
		{"base64 of it inside binary", "100000000", "\x00\x01MWU4\x02"},
		{"gzip of a scientific spelling", "100000000", gzOf("1e8")},
		{"raw deflate of a scientific spelling", "100000000", string(df([]byte("1e8"))) + "\x00"},
		{"UTF-16 of a scientific spelling", "100000000", "\x00\x01" + string(utf16LE("1e8")) + "\x00\x02"},
		{"Shift-JIS fullwidth scientific spelling", "100000000", func() string { s, _ := japanese.ShiftJIS.NewEncoder().String("１ｅ８"); return "\x01" + s + "\x02" }()},
		{"UTF-16 three times", "aaaaaaaa", string(bytes.Repeat([]byte{0x61, 0, 0, 0, 0, 0, 0, 0}, 8))},
		{"UTF-16 twice inside base64", cred, base64.StdEncoding.EncodeToString(utf16LE(string(utf16LE(cred))))},
		{"UTF-16BE of compatibility forms in a text view", "iiiiiiii", string(utf16Bytes("ⅰⅰⅰⅰⅰⅰⅰⅰ", true))},
		{"EBCDIC of UTF-16", "abcdefgh", func() string {
			s, _ := charmap.CodePage037.NewEncoder().String(string(utf16LE("abcdefgh")))
			return s
		}()},
	} {
		holding(t, tc.held)
		if !leaks(tc.unit) {
			t.Errorf("%s: %q not withheld", tc.name, tc.unit)
		}
	}
	holding(t, "100000000")
	if leaks("MWU5") || leaks(base64.StdEncoding.EncodeToString(utf16LE("1e9"))) {
		t.Error("a different number was withheld")
	}
}

// Class cost: ordinary output is checked clean, with 20 credentials held
// and with a credential that has non-ASCII characters (the benchmarks time
// the same units).
func TestKit2OrdinaryOutputIsClean(t *testing.T) {
	for name, unit := range map[string][]byte{"150 KB JSON": benchJSON(150 << 10), "7 KB environment": benchEnv()} {
		for _, extra := range [][]string{nil, {nonASCIICred}} {
			resetForTest(Declarations{})
			Hold("bench", append(benchCreds(), extra...)...)
			m, _ := currentMatcher()
			if v := m.check(unit); v != clean {
				t.Errorf("%s (non-ASCII credential held: %v): verdict %d, want clean", name, extra != nil, v)
			}
		}
	}
	// And the same units still catch a held value hidden in them.
	resetForTest(Declarations{})
	Hold("bench", append(benchCreds(), nonASCIICred)...)
	j := benchJSON(150 << 10)
	for _, hidden := range []string{
		benchCreds()[7],
		base64.StdEncoding.EncodeToString([]byte(nonASCIICred)),
		func() string {
			s, _ := charmap.ISO8859_1.NewEncoder().String(nonASCIICred)
			return base64.StdEncoding.EncodeToString([]byte(s))
		}(),
	} {
		u := append(bytes.Clone(j[:len(j)/2]), fmt.Sprintf(`,"x":%q`, hidden)...)
		u = append(u, j[len(j)/2:]...)
		if m, _ := currentMatcher(); m.check(u) != leak {
			t.Errorf("a held value hidden in the JSON was not found: %.30q", hidden)
		}
	}
}

var _ = context.Background

// Class stdout-write-unit-splitting: no byte of a write is released before
// the whole write has been checked, and a write over the unit limit is
// withheld whole.
func TestKit2StdoutWriteIsOneUnit(t *testing.T) {
	out, _, code := runChild(t, "bigwrite", "")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if strings.ContainsRune(out, 0) || strings.Contains(out, "abcdefgh") || !strings.Contains(out, Notice(SinkStdout)) {
		t.Errorf("a write carrying a held value was released in part: %d bytes, %d NUL", len(out), strings.Count(out, "\x00"))
	}
	out, _, _ = runChild(t, "overbudget", "")
	if strings.Contains(out, "clean output") || !strings.Contains(out, Notice(SinkStdout)) || !strings.Contains(out, "after") {
		t.Errorf("a write over the unit limit was released in part: %d bytes: %.80q", len(out), out)
	}
}

// The stdout pump cuts units at pauses where the bytes arrive: while it is
// held up passing on one unit, writes with pauses between them stay units
// of their own, so a clean line is never withheld with a credential that
// came before it.
func TestKit2StdoutPauseMeasuredOnArrival(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:session", "pause-secret-123456")
	startedOnce.Do(func() { close(started) })
	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	p := &pump{fd: 1, sink: SinkStdout, in: inR, out: outW, done: make(chan struct{})}
	go p.run()
	// A clean unit larger than the pipe: passing it on blocks until read.
	inW.Write(bytes.Repeat([]byte("clean bulk\n"), 32<<10))
	time.Sleep(50 * time.Millisecond)
	inW.Write([]byte("token pause-secret-123456\n"))
	time.Sleep(50 * time.Millisecond)
	inW.Write([]byte("clean line two\n"))
	time.Sleep(50 * time.Millisecond)
	got := make(chan []byte)
	go func() { b, _ := io.ReadAll(outR); got <- b }()
	inW.Close()
	<-p.done
	outW.Close()
	out := <-got
	if bytes.Contains(out, []byte("pause-secret")) {
		t.Fatal("the credential was released")
	}
	if !bytes.HasSuffix(out, []byte("clean line two\n")) || !bytes.Contains(out, []byte(Notice(SinkStdout))) {
		t.Errorf("the units joined: tail %q", out[max(0, len(out)-160):])
	}
}

// serveAborting serves r as net/http would: a handler that fails closed
// aborts with http.ErrAbortHandler, which the server recovers.
func serveAborting(h http.Handler, w http.ResponseWriter, r *http.Request) {
	defer func() {
		if v := recover(); v != nil && v != http.ErrAbortHandler {
			panic(v)
		}
	}()
	h.ServeHTTP(w, r)
}
