// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// The classes the kit3rv review raised (.review/kit3rv-codex-r*.md and
// .review/kit3rv-self-found.md), each through the reported inputs and other
// members of the class.

import (
	"bytes"
	"encoding/ascii85"
	"encoding/base32"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Class base-n-ignored-bytes (contract 3, Candidates): the bytes a base-N
// decoder accepts and ignores are inside its span. Ascii85 across a space
// or a control byte other than a tab or a line break is an open member
// (run-decoder-candidate-splitting): .review/kit3rv-stop-note.md.
func TestKit3rvBaseNIgnoredBytes(t *testing.T) {
	holding(t, "secret!!")
	b64 := base64.StdEncoding.EncodeToString([]byte("secret!!"))
	b32 := base32.StdEncoding.EncodeToString([]byte("secret!!"))
	b32h := strings.ToLower(base32.HexEncoding.EncodeToString([]byte("secret!!")))
	a := make([]byte, ascii85.MaxEncodedLen(8))
	a85 := string(a[:ascii85.Encode(a, []byte("secret!!"))])
	every := func(s, sep string, n int) string {
		var b strings.Builder
		for i := 0; i < len(s); i += n {
			if i > 0 {
				b.WriteString(sep)
			}
			b.WriteString(s[i:min(i+n, len(s))])
		}
		return b.String()
	}
	for _, u := range []string{
		every(b64, "\r\n", 4), // self-found input
		every(b64, "\n", 8),
		b64[:1] + "\r" + b64[1:],
		"x\x00" + every(b64, "\r\n", 3) + "\x00y",
		every(b32, "\r\n", 4), // Codex's probe
		every(b32h, "\n", 5),
		every(a85, "\t", 3),
		"<~" + every(a85, "\r\n", 4) + "~>",
	} {
		if !withheld([]byte(u)) {
			t.Errorf("%q released", u)
		}
	}
}

// Class numbers-spellings (contract 3, Numbers): every spelling the
// bullet names, judged by its own length, and its 8-digit pieces grouped.
func TestKit3rvNumbersEverySpelling(t *testing.T) {
	cases := []struct {
		held  string
		leaks []string
		clean []string
	}{
		{"12345678",
			[]string{
				"0012,345,678", "+0012,345,678", // self-found inputs
				"1'2'3'4'5'6'7'8", "1 2 3 4 5 6 7 8", "1.2.3.4.5.6.7.8", "1,2,3,4,5,6,7,8",
				"1234,5678", "1,234.5_67'8", "id912,345,678", "a1,234,567,8b",
				"x1.2345678e7", "91.2345678e7", "1.2345678e6", "12.345.678x", "12.345678e6", "0.12345678e8", "1234567800e-2",
				"1.23456780E+07", "000.0012345678e10", "1.2345678e07", "1.2345678e+0007",
			},
			[]string{"12345679", "1,234,567", "2345678", "1,,234,567,8", "12,3456,79"},
		},
		{"123456789",
			[]string{"1234,5,6789", "1,2.3_4'5 6,7.8_9"}, // Codex's probe
			nil,
		},
		{"1234",
			// x1'2'3'4 and 5,1,234: a reading from the offset of the 1 is a
			// spelling (kit5rv, tokenAt: the left side is a view boundary).
			[]string{"pin 1'2'3'4", "pin 1 2 3 4", "pin +0,001,234", "(1.234)", "1,2,3,4", "1.234e3", "0001234",
				"x1'2'3'4", "5,1,234"},
			[]string{"id 1,2345", "1,234,5", "1'2'3'4x", "1.234e2", "1,234,56"},
		},
		{"1234567",
			[]string{"x1,234,567y", "a01234567"},
			[]string{"x1234567y"},
		},
		{"123456789012",
			[]string{"3,4,5,6,7,8,9,0", "id 2.345.678.9 x"}, // 8-digit pieces, grouped
			nil,
		},
		{"0012345678",
			[]string{"0,0,1,2,3,4,5,6"}, // a piece as written
			nil,
		},
	}
	for _, c := range cases {
		holding(t, c.held)
		for _, u := range c.leaks {
			if !withheld([]byte(u)) {
				t.Errorf("held %s: %q released", c.held, u)
			}
		}
		for _, u := range c.clean {
			if withheld([]byte(u)) {
				t.Errorf("held %s: %q withheld (not a spelling of it)", c.held, u)
			}
		}
	}
}

// Class config-reader-drops-values (contract 1): a credential file's value
// is held unless the file's own syntax makes it no value: an untagged YAML
// or JSON null. In env, TOML and INI files "~" and "null" are text.
func TestKit3rvConfigHoldsEveryString(t *testing.T) {
	for _, tc := range []struct {
		file, content, value string
		held                 bool
	}{
		{"config.env", "token=~\n", "~", true}, // Codex's input
		{"config.env", "token=null\n", "null", true},
		{"c.ini", "token = NULL\n", "NULL", true},
		{"c.toml", "token = ~\n", "~", true},
		{"c.yaml", "token: !!str ~\n", "~", true}, // Codex's input
		{"c.yaml", "token: !!str null\n", "null", true},
		{"c.yaml", "token: !custom NULL\n", "NULL", true},
		{"c.yaml", "token: ~\n", "~", false},
		{"c.yaml", "token: Null\n", "Null", false},
		{"c.yaml", "token: NULL\n", "NULL", false},
		{"c.json", `{"token":null}`, "null", false},
	} {
		gatherString(t, Declarations{ConfigFields: []string{"token"}}, tc.file, tc.content)
		if isHeld(tc.value) != tc.held {
			t.Errorf("%s %q: %q held = %v, want %v", tc.file, tc.content, tc.value, !tc.held, tc.held)
		}
	}
}

// Class auth-part-origin-union (contract 2(c)): a value held under a
// source with its own origin declaration keeps that binding; it does not
// take the origins of a part held under another source.
func TestKit3rvDeclaredBindingNotWidened(t *testing.T) {
	const part = "f8Lq6N2v9s4K7m1R"
	d := Declarations{Origins: []Origin{
		{Source: "minted:part", Allowed: []string{"https://b.example"}},
		{Source: "minted:full", Allowed: []string{"https://a.example"}},
		{Source: "env:TOKEN", Allowed: []string{"https://api.example"}},
	}}
	send := func(url, body string, h http.Header) bool {
		sent := false
		tr := Transport(roundTripFunc(func(r *http.Request) (*http.Response, error) {
			sent = true
			return &http.Response{StatusCode: 200, Body: http.NoBody, Request: r}, nil
		}))
		req, _ := http.NewRequest("POST", url, strings.NewReader(body))
		for k, v := range h {
			req.Header[k] = v
		}
		if resp, err := tr.RoundTrip(req); err == nil {
			resp.Body.Close()
		}
		return sent
	}
	for _, order := range [][2]string{{"part", "full"}, {"full", "part"}} {
		resetForTest(d)
		for _, o := range order {
			if o == "part" {
				Hold("minted:part", part)
			} else {
				Hold("minted:full", "custom "+part)
			}
		}
		if send("https://b.example/", "custom "+part, nil) {
			t.Errorf("held %v: the full credential reached an origin only its part allows", order)
		}
	}
	// A request header's source has no declaration of its own: its parts
	// lend it their binding, so a token from the environment goes out.
	resetForTest(d)
	Hold("env:TOKEN", part)
	if !send("https://api.example/", "{}", http.Header{"Authorization": {"Bearer " + part}}) {
		t.Error("a declared token did not reach its origin in an Authorization header")
	}
}

// Class stream-context-truncation (contract 3, Streams and Budget): the
// stream's context reaches back to the start of every run a decoder may
// still be reading; when that does not fit, the stream withholds until the
// run ends.
func TestKit3rvStreamKeepsOpenRuns(t *testing.T) {
	holding(t, "secret!!")
	hex8 := "secret!!"
	for range 8 {
		hex8 = fmt.Sprintf("%x", hex8)
	}
	b64 := "secret!!"
	for range 20 {
		b64 = base64.StdEncoding.EncodeToString([]byte(b64))
	}
	hex12 := hex8
	for range 4 {
		hex12 = fmt.Sprintf("%x", hex12)
	}
	for name, w := range map[string][]string{
		"Codex's input: 8 hex layers, 2046 + 2": {hex8[:2046], hex8[2046:]},
		"20 base64 layers in 4 writes":          {b64[:len(b64)/4], b64[len(b64)/4 : len(b64)/2], b64[len(b64)/2 : 3*len(b64)/4], b64[3*len(b64)/4:]},
		"12 hex layers, past the context limit": {hex12[:len(hex12)/2], hex12[len(hex12)/2:]},
	} {
		var out bytes.Buffer
		sw := NewWriter(&out, SinkFile)
		for _, p := range w {
			sw.Write([]byte(p))
		}
		if strings.Contains(out.String(), strings.Join(w, "")) {
			t.Errorf("%s: the encoded credential reached the file", name)
		}
	}
	// A compressed stream longer than the context, written in two writes.
	rnd := make([]byte, 24<<10)
	for i := range rnd {
		rnd[i] = byte(i*7919>>3 ^ i*31)
	}
	gzd := gz(append(rnd, "secret!!"...))
	var out bytes.Buffer
	sw := NewWriter(&out, SinkFile)
	sw.Write(gzd[:len(gzd)-40])
	sw.Write(gzd[len(gzd)-40:])
	if bytes.Contains(out.Bytes(), gzd) {
		t.Error("a gzip stream longer than the context reached the file whole")
	}
	// The over-withholding guard: ordinary lines, streamed past the
	// context many times over, all reach the file.
	out.Reset()
	sw = NewWriter(&out, SinkFile)
	line := `{"id":12,"title":"Printer offline","state":"open","owner":"j.doe@example.com"}` + "\n"
	for range 600 {
		sw.Write([]byte(line))
	}
	if strings.Count(out.String(), line) != 600 {
		t.Errorf("ordinary lines withheld: %d of 600 written", strings.Count(out.String(), line))
	}
}

// Class mcp-preexisting-headers-unchecked (contract 4 and 5, MCP): a
// header set on the response before MCPHTTP runs goes out only checked.
func TestKit3rvMCPHeadersSetBeforeAreChecked(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:x", "s3cr3t-A9x")
	inner := MCPHTTP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("{}"))
	}))
	for _, wrap := range []bool{true, false} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Debug", "s3cr3t-A9x")
			w.Header().Set("X-Plain", "fine")
			if wrap {
				w = noHijack{w}
			}
			inner.ServeHTTP(w, r)
		}))
		resp, err := http.Post(srv.URL, "application/json", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
		srv.Close()
		if err != nil {
			t.Fatalf("hijacker hidden %v: %v", wrap, err)
		}
		resp.Body.Close()
		if strings.Contains(fmt.Sprint(resp.Header), "s3cr3t") {
			t.Errorf("hijacker hidden %v: a header set before MCPHTTP went out unchecked: %v", wrap, resp.Header)
		}
		if resp.Header.Get("X-Plain") != "fine" {
			t.Errorf("hijacker hidden %v: a clean header set before MCPHTTP was lost: %v", wrap, resp.Header)
		}
	}
}

// Class outbound-envelope-unchecked (contract 2(c)): the request checked
// is the request as Go's transports send it, with what they add.
func TestKit3rvOutboundDefaultsChecked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	c := &http.Client{Transport: Transport(nil)}
	for _, tc := range []struct{ held, body string }{
		{"xGo-http-client/1.1", ""}, // Codex's input
		{"zAccept-Encoding: gzip", ""},
		{"17", "seventeen bytes!!"},
		{"Transfer-Encoding: chunked", "x"},
	} {
		resetForTest(Declarations{})
		Hold("minted:x", tc.held)
		var err error
		if tc.body == "" {
			_, err = c.Get(srv.URL)
		} else {
			var body io.Reader = strings.NewReader(tc.body)
			if strings.HasPrefix(tc.held, "Transfer") {
				body = io.MultiReader(body) // no length: chunked
			}
			_, err = c.Post(srv.URL, "text/plain", body)
		}
		if err == nil {
			t.Errorf("held %q: a request whose added header carries it was sent", tc.held)
		}
	}
}
