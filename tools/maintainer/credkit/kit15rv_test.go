// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"net/url"
	"strings"
	"testing"
)

// sendThrough sends req through the kit to a stub that records whether it
// was sent.
func sendThrough(req *http.Request) (sent bool) {
	rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
		sent = true
		return &http.Response{StatusCode: 204, Body: http.NoBody, Header: http.Header{}}, nil
	})
	Transport(rt).RoundTrip(req)
	return sent
}

func multipartBody(t *testing.T, build func(w *multipart.Writer)) (string, string) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	w.SetBoundary("probe")
	build(w)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.String(), w.FormDataContentType()
}

// Loop ruling 8 ("send what you read", blunt): an outgoing request counts
// as whole only when its header names are canonical, each body-deciding
// header has one value, its body carries no content or transfer coding or
// foreign charset, its query is canonical pairs, and a form or multipart
// body re-serializes byte for byte from what the kit parsed. Anything else
// is refused and acquisition is marked unknown.
func TestKit15RequestReadTheOneWay(t *testing.T) {
	form := "application/x-www-form-urlencoded"
	mpLower, mpCT := multipartBody(t, func(w *multipart.Writer) {
		h := textproto.MIMEHeader{}
		h.Set("Content-Disposition", `form-data; name="password"`)
		p, _ := w.CreatePart(h)
		p.Write([]byte("Mp7Lower9x"))
	})
	mpLower = strings.Replace(mpLower, "Content-Disposition:", "content-disposition:", 1)
	mpGood, _ := multipartBody(t, func(w *multipart.Writer) {
		w.WriteField("password", "Mp7Pre9x")
	})
	mpCTE, _ := multipartBody(t, func(w *multipart.Writer) {
		h := textproto.MIMEHeader{}
		h.Set("Content-Disposition", `form-data; name="password"`)
		h.Set("Content-Transfer-Encoding", "base64")
		p, _ := w.CreatePart(h)
		p.Write([]byte("TXA3Q3RlOXg="))
	})
	for _, c := range []struct {
		name, url, ct, body string
		hdr                 map[string]string
	}{
		{name: "form: a ';' pair", url: "https://api.example.com/t", ct: form, body: "a=1;password=Semi7Form9x"},
		{name: "form: a pair with no '='", url: "https://api.example.com/t", ct: form, body: "password&x=1"},
		{name: "form: a lowercase escape", url: "https://api.example.com/t", ct: form, body: "password=Low7%2fForm9x"},
		{name: "form: an escaped unreserved character", url: "https://api.example.com/t", ct: form, body: "password=%41sc7Form9x"},
		{name: "form: a trailing '&'", url: "https://api.example.com/t", ct: form, body: "password=Trail7Form9x&"},
		{name: "query: a ';' pair", url: "https://api.example.com/t?a=1;token=Semi7Query9x"},
		{name: "query: a bad escape", url: "https://api.example.com/t?token=Bad7Query9x%zz"},
		{name: "query: an escaped unreserved character", url: "https://api.example.com/t?token=%41sc7Query9x"},
		{name: "query: a raw character outside the fleet's set", url: "https://api.example.com/t?token=Bang7Query9x!"},
		{name: "a coding on a request body", url: "https://api.example.com/t", ct: "application/json", body: `{"password":"Gz7Body9x"}`,
			hdr: map[string]string{"Content-Encoding": "identity, gzip"}},
		{name: "a foreign charset", url: "https://api.example.com/t", ct: "application/json; charset=iso-8859-1", body: `{"password":"Cs7Body9x"}`},
		{name: "a byte-order mark", url: "https://api.example.com/t", ct: "application/json", body: "\xef\xbb\xbf" + `{"password":"Bom7Body9x"}`},
		{name: "a non-canonical header name", url: "https://api.example.com/t", ct: "application/json", body: `{}`,
			hdr: map[string]string{"x-api-key": "Hdr7Name9x"}},
		{name: "multipart: a lowercase part header", url: "https://api.example.com/t", ct: mpCT, body: mpLower},
		{name: "multipart: a preamble", url: "https://api.example.com/t", ct: mpCT, body: "pre\r\n" + mpGood},
		{name: "multipart: an epilogue", url: "https://api.example.com/t", ct: mpCT, body: mpGood + "tail"},
		{name: "multipart: a part transfer coding", url: "https://api.example.com/t", ct: mpCT, body: mpCTE},
		{name: "multipart: two spaces after a part header's colon", url: "https://api.example.com/t", ct: mpCT,
			body: strings.Replace(mpGood, "Content-Disposition: ", "Content-Disposition:  ", 1)},
	} {
		resetForTest(Declarations{CredentialFields: []string{"password", "token"}})
		var body io.Reader
		if c.body != "" {
			body = strings.NewReader(c.body)
		}
		req, err := http.NewRequest("POST", c.url, body)
		if err != nil {
			t.Fatal(err)
		}
		if c.ct != "" {
			req.Header.Set("Content-Type", c.ct)
		}
		for k, v := range c.hdr {
			req.Header[k] = []string{v}
		}
		if sendThrough(req) {
			t.Errorf("%s: a request the kit does not read the one way was sent", c.name)
		}
		if !isUnknown() {
			t.Errorf("%s: acquisition not marked unknown", c.name)
		}
	}
}

// The same rule never fires on what the fleet sends: url.Values forms,
// url.Values and cliutil.EncodeQuery queries, the fleet's literal request
// paths, multipart.Writer bodies, JSON with charset=utf-8.
func TestKit15OrdinaryRequestsAreWhole(t *testing.T) {
	mp, mpCT := multipartBody(t, func(w *multipart.Writer) {
		w.WriteField("name", "report q3")
		fw, _ := w.CreateFormFile("file", "a b.pdf")
		fw.Write([]byte("%PDF-1.4\r\n\x00\xff binary"))
	})
	q := url.Values{"filter": {"name eq 'x' & y"}, "time": {"2024-01-01T00:00:00+00:00"}, "ids": {"1,2,3"}}
	for _, c := range []struct{ name, url, ct, body string }{
		{"url.Values form", "https://api.example.com/token", "application/x-www-form-urlencoded",
			url.Values{"grant_type": {"client_credentials"}, "scope": {"a b:c/d"}, "client_id": {"x@y"}}.Encode()},
		{"url.Values query", "https://api.example.com/t?" + q.Encode(), "", ""},
		{"EncodeQuery query", "https://api.example.com/t?" + strings.ReplaceAll(q.Encode(), "+", "%20"), "", ""},
		{"key-only pair", "https://api.example.com/documents?upload", "", ""},
		{"escaped colon", "https://api.example.com/v1.0/scopes?scope=farm%3Atenant&limit=10", "", ""},
		{"raw colon and comma", "https://api.example.com/x?time=2024-01-01T00:00:00Z&ids=1,2&u=a@b/c", "", ""},
		{"escaped brackets", "https://api.example.com/rule-triggers?statuses%5B%5D=PENDING", "", ""},
		{"escaped star", "https://api.example.com/query?minorversion=75&query=select%20%2A%20from%20CompanyInfo", "", ""},
		{"empty query", "https://api.example.com/ListUsers?", "", ""},
		{"multipart.Writer", "https://api.example.com/upload", mpCT, mp},
		{"JSON utf-8", "https://api.example.com/x", "application/json; charset=utf-8", `{"a":"é"}`},
		{"no Content-Type", "https://api.example.com/x", "", `{"a":1}`},
	} {
		resetForTest(Declarations{CredentialFields: []string{"password", "token"}})
		var body io.Reader
		if c.body != "" {
			body = strings.NewReader(c.body)
		}
		req, _ := http.NewRequest("POST", c.url, body)
		if c.ct != "" {
			req.Header.Set("Content-Type", c.ct)
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-Api-Key", "k")
		if !sendThrough(req) || isUnknown() {
			t.Errorf("%s: an ordinary request was refused", c.name)
		}
	}
}

// Ruling 8a: a JSON body is read as it is, and a duplicate key at any depth
// holds every value under it.
func TestKit15JSONDuplicateKeysHoldEveryValue(t *testing.T) {
	resetForTest(Declarations{CredentialFields: []string{"password"}})
	req, _ := http.NewRequest("POST", "https://api.example.com/x",
		strings.NewReader(`{"a":{"password":"Dup7One9x","b":[{"password":"Dup7Two9x","password":"Dup7Three9x"}]},"password":"Dup7Four9x"}`))
	req.Header.Set("Content-Type", "application/json")
	sendThrough(req)
	for _, v := range []string{"Dup7One9x", "Dup7Two9x", "Dup7Three9x", "Dup7Four9x"} {
		if !isHeld(v) {
			t.Errorf("%q not held; held %q", v, heldValues())
		}
	}
}

// Ruling 8 for an inbound MCP request: the same rule (but the query, which
// the kit does not read for arguments).
func TestKit15MCPRequestReadTheOneWay(t *testing.T) {
	msg := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"x","arguments":{"token":"Mcp7Arg9x"}}}`
	for _, c := range []struct{ name, headers, body string }{
		{"a foreign charset", "Content-Type: application/json; charset=iso-8859-1\r\n", msg},
		{"a coding", "Content-Type: application/json\r\nContent-Encoding: gzip\r\n", string(gzippedS(msg))},
		{"a form that does not re-serialize", "Content-Type: application/x-www-form-urlencoded\r\n", "a=1;token=Mcp7Arg9x"},
	} {
		resetForTest(Declarations{MCPArguments: []string{"token"}})
		called := false
		h := MCPHTTP(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
		srv := newTestServer(h)
		rawPost(t, srv.Listener.Addr().String(), c.headers, []byte(c.body))
		srv.Close()
		if called || !isUnknown() {
			t.Errorf("%s: handler called %v, unknown %v", c.name, called, isUnknown())
		}
	}
	resetForTest(Declarations{MCPArguments: []string{"token"}})
	called := false
	srv := newTestServer(MCPHTTP(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })))
	rawPost(t, srv.Listener.Addr().String(), "Content-Type: application/json\r\n", []byte(msg))
	srv.Close()
	if !called || isUnknown() {
		t.Errorf("an ordinary MCP request: handler called %v, unknown %v", called, isUnknown())
	}
}

// Ruling 8d ("deliver what you read"): when the kit undoes a response's
// codings to hold its fields, the program reads the decoded body, with no
// Content-Encoding or Content-Length left to decode it again or differently.
func TestKit15ResponseDeliversWhatTheKitRead(t *testing.T) {
	plain := `{"session_token":"Resp7Tok9x","n":1}`
	for _, c := range []struct {
		name, coding string
		body         []byte
	}{
		{"gzip", "gzip", gzippedS(plain)},
		{"zlib deflate", "deflate", zlibbed(plain)},
	} {
		resetForTest(Declarations{ResponseFields: []string{"session_token"}})
		rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
			h := http.Header{}
			h.Set("Content-Type", "application/json")
			h.Set("Content-Encoding", c.coding)
			h.Set("Content-Length", fmt.Sprint(len(c.body)))
			return &http.Response{StatusCode: 200, Header: h, Body: io.NopCloser(bytes.NewReader(c.body)), ContentLength: int64(len(c.body)), Request: r}, nil
		})
		req, _ := http.NewRequest("GET", "https://api.example.com/s", nil)
		resp, err := Transport(rt).RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(resp.Body)
		if err != nil || string(got) != plain {
			t.Errorf("%s: the program read %q (%v), want the decoded body", c.name, got, err)
		}
		if headerValues(resp.Header, "Content-Encoding") != nil || headerValues(resp.Header, "Content-Length") != nil || resp.ContentLength != -1 {
			t.Errorf("%s: coding headers left for the program: %v, length %d", c.name, resp.Header, resp.ContentLength)
		}
		if isUnknown() {
			t.Errorf("%s: an ordinary coded response made acquisition unknown", c.name)
		}
		if !isHeld("Resp7Tok9x") {
			t.Errorf("%s: the response field is not held", c.name)
		}
	}
}

// Loop ruling 9 (blunt): a credential file the kit cannot read the one
// canonical way holds every string value in it.
func TestKit15ImpreciseConfigHoldsEveryString(t *testing.T) {
	for _, c := range []struct{ name, file string }{
		{"an array", "ids = [\"x\"]\nzone = \"Zone7Imp9x\"\n"},
		{"an escape", "name = \"a\\tb\"\nzone = \"Zone7Imp9x\"\n"},
		{"a duplicate key", "zone = \"Zone7Imp9x\"\nzone = \"b\"\n"},
		{"a duplicate key by case", "zone = \"Zone7Imp9x\"\nZONE = \"b\"\n"},
		{"a quoted key", "\"k\" = 1\nzone = \"Zone7Imp9x\"\n"},
		{"a dotted key", "a.b = 1\nzone = \"Zone7Imp9x\"\n"},
		{"a multiline string", "m = \"\"\"\nx\n\"\"\"\nzone = \"Zone7Imp9x\"\n"},
		{"an inline table", "t = { a = 1 }\nzone = \"Zone7Imp9x\"\n"},
		{"a YAML mapping", "zone: Zone7Imp9x\n"},
		{"a parse error", "zone = \"Zone7Imp9x\"\n= nothing\n"},
		{"an array of tables", "[[t]]\nzone = \"Zone7Imp9x\"\n"},
		{"an unquoted value with a blank", "zone = Zone7Imp9x # c\n"},
		{"a JSON object with a nested object", `{"a":{"b":1},"zone":"Zone7Imp9x"}`},
		{"a JSON object with a duplicate key", `{"zone":"Zone7Imp9x","zone":"x"}`},
		{"a dotenv export line (an INI key \"export ZONE\")", "export ZONE=Zone7Imp9x\n"},
		{"a lone CR ends a comment", "n = 1 # c\rzone = \"Zone7Imp9x\"\n"},
		{"a lone CR inside a string", "a = \"x\rzone = Zone7Imp9x\"\n"},
		{"a NEL inside a string", "a = \"x\xc2\x85zone: Zone7Imp9x\"\n"},
		{"an LS inside a comment", "n = 1 # c\xe2\x80\xa8zone = \"Zone7Imp9x\"\n"},
		{"a PS inside a string", "a = 'x\xe2\x80\xa9zone = Zone7Imp9x'\n"},
		{"a JSON string with an escape", "{\"a\":\"\x5cu0041\",\"zone\":\"Zone7Imp9x\"}"},
	} {
		gatherString(t, Declarations{}, "c.toml", c.file)
		if !isHeld("Zone7Imp9x") {
			t.Errorf("%s: a string value of an imprecise file is not held; held %q", c.name, heldValues())
		}
	}
}

// What the connectors write themselves (go-toml v2's Marshal of the
// generated Config), INI and dotenv files, and JSON objects of scalars are
// read precisely: nothing but the credentials is held.
func TestKit15CanonicalConfigStaysPrecise(t *testing.T) {
	for _, c := range []struct{ name, file string }{
		{"go-toml v2, set", "base_url = 'https://api.example.com/v1'\nauth_header = 'Bearer abc.def'\naccess_token = 'tok'\n" +
			"token_expiry = 2026-01-02T03:04:05.000000006Z\nclient_id = 'has\"quote'\nn = 3\nb = true\n\n[headers]\nX-Api-Key = 'k1'\n\n[template_vars]\nshop = 's'\n"},
		{"go-toml v2, empty", "base_url = ''\nauth_header = ''\naccess_token = ''\ntoken_expiry = 0001-01-01T00:00:00Z\nclient_id = ''\nn = 0\nb = false\n"},
		{"basic strings and comments", "# comment\nbase_url = \"https://api.example.com\" # trailing\n\n[profile]\nregion = \"us\"\n"},
		{"INI", "[default]\nregion=us-east-1\nbase_url = https://api.example.com/v1\n; comment\n"},
		{"dotenv", "BASE_URL=https://api.example.com\nREGION=us\nNAME='x y'\nZONE=\"a b\"\r\n"},
		{"JSON object of scalars", `{"base_url":"https://api.example.com","n":1,"b":true,"z":null}`},
	} {
		gatherString(t, Declarations{}, "c.toml", c.file)
		if !preciseConfig([]byte(c.file)) {
			t.Errorf("%s: an ordinary file is not read precisely", c.name)
		}
		if isHeld("https://api.example.com/v1") || isHeld("https://api.example.com") || isHeld("us") {
			t.Errorf("%s: a non-credential value of an ordinary file is held; held %q", c.name, heldValues())
		}
	}
}

func gzippedS(s string) []byte {
	var buf bytes.Buffer
	z := gzip.NewWriter(&buf)
	z.Write([]byte(s))
	z.Close()
	return buf.Bytes()
}

func zlibbed(s string) []byte {
	var buf bytes.Buffer
	z := zlib.NewWriter(&buf)
	z.Write([]byte(s))
	z.Close()
	return buf.Bytes()
}

func newTestServer(h http.Handler) *httptest.Server { return httptest.NewServer(h) }
