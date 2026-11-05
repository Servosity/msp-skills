// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/encoding/charmap"
)

func isUnknown() bool {
	reg.Lock()
	defer reg.Unlock()
	return reg.unknown
}

// kit13rv Codex r2, config-reader-drops-values: the blunt rule finds a
// quoted key as the reader reads it (a doubled single quote is one quote),
// and a bare key as the reader cuts it (any character the reader keeps in a
// bare key, not only letters, digits, '_' and '-').
func TestKit14ConfigKeyFoundAsTheReaderReadsIt(t *testing.T) {
	for _, c := range []struct {
		name, field, file, want string
	}{
		{"reviewer: doubled single quote in the key", "\"api'key\"", "bad = [\n'api''key' = \"secret!!\"\n", "secret!!"},
		{"bare key with a character outside the scan's set", "api+key", "bad = [\napi+key = \"Plus7Key9xyz\"\n", "Plus7Key9xyz"},
	} {
		gatherString(t, Declarations{ConfigFields: []string{c.field}}, "c.toml", c.file)
		if !isHeld(c.want) {
			t.Errorf("%s: %q not held; held %q", c.name, c.want, heldValues())
		}
	}
}

// kit13rv Codex r2, decoder-context-dedupe: a run first read in a final
// view (its decodings matched, not decoded further) is read again where
// recursion is allowed; the memo never lets a more limited reading stand
// for a less limited one.
func TestKit14FinalViewReadDoesNotHideALaterFullRead(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:probe", "secret!!")
	m, _ := currentMatcher()
	unit := []byte(`{"3539376134613537363136643465373435363661343234613535333035353339":0,"a":"YzJWamNt","b":"VjBJU0U9!"}`)
	if v := m.check(unit); v == clean {
		t.Errorf("hex, hex, base64, base64 of the held value in a member name read clean beside two string values")
	}
	control := []byte(`{"3539376134613537363136643465373435363661343234613535333035353339":0}`)
	if v := m.check(control); v == clean {
		t.Errorf("the control (member name alone) read clean")
	}
}

func gzipped(t *testing.T, b []byte) []byte {
	var buf bytes.Buffer
	z, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	z.Write(b)
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func rawPost(t *testing.T, addr, headers string, body []byte) string {
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(30 * time.Second))
	fmt.Fprintf(c, "POST / HTTP/1.1\r\nHost: x\r\n%sContent-Length: %d\r\n\r\n", headers, len(body))
	c.Write(body)
	got, _ := io.ReadAll(c)
	return string(got)
}

// kit13rv Codex r2, mcp-error-bypasses-guard (blunt): an MCP request body
// the kit cannot read whole (a decoding past the limit, a coding it cannot
// undo, a read that ends in an error) leaves the arguments it may carry
// unknown, so acquisition is marked unknown before the refusal goes out:
// every unit is withheld from then on.
func TestKit14MCPUnreadBodyMarksUnknown(t *testing.T) {
	msg := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"x","arguments":{"token":"UNSUPPORTED MEDIA TYPE"}}}`)
	for _, c := range []struct {
		name, headers string
		body          []byte
	}{
		{"reviewer: gzip decoding past the limit", "Content-Type: application/json\r\nContent-Encoding: gzip\r\n",
			gzipped(t, append(bytes.Repeat([]byte(" "), (64<<20)+1), msg...))},
		{"a coding the kit cannot undo", "Content-Type: application/json\r\nContent-Encoding: x-unknown\r\n", msg},
	} {
		resetForTest(Declarations{MCPArguments: []string{"token"}})
		srv := httptest.NewServer(MCPHTTP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
		got := rawPost(t, srv.Listener.Addr().String(), c.headers, c.body)
		srv.Close()
		if !isUnknown() {
			t.Errorf("%s: acquisition not marked unknown; response %q", c.name, got)
		}
		if strings.Contains(strings.ToLower(got), "unsupported media type") {
			t.Errorf("%s: the response carries the argument the kit could not read: %q", c.name, got)
		}
	}
}

// The same class on the outbound transport: a request body the kit could
// not read whole (a read error, a declared coding it cannot undo) marks
// acquisition unknown before the error returns.
func TestKit14RequestUnreadBodyMarksUnknown(t *testing.T) {
	resetForTest(Declarations{CredentialFields: []string{"password"}})
	req, _ := http.NewRequest("POST", "https://api.example.com/login", io.MultiReader(strings.NewReader("password="+"Partial7Body9x"), iotestErr{}))
	if _, err := Transport(http.DefaultTransport).RoundTrip(req); err == nil {
		t.Fatal("the request was sent")
	}
	if !isUnknown() {
		t.Errorf("read error: acquisition not marked unknown")
	}
	resetForTest(Declarations{CredentialFields: []string{"password"}})
	req, _ = http.NewRequest("POST", "https://api.example.com/login", strings.NewReader("password=Coded7Body9x"))
	req.Header.Set("Content-Encoding", "x-unknown")
	if _, err := Transport(http.DefaultTransport).RoundTrip(req); err == nil {
		t.Fatal("the request was sent")
	}
	if !isUnknown() {
		t.Errorf("unreadable coding: acquisition not marked unknown")
	}
}

// kit14rv Codex r3, config-reader-drops-values: a quoted TOML table key may
// carry ']' (and any other punctuation); the header is still a header, in a
// well-formed file and inside an unclosed array.
func TestKit14QuotedBracketInTableHeader(t *testing.T) {
	for _, c := range []struct{ name, file, want string }{
		{"reviewer: inside an unclosed array", "bad = [\n[token.\"x]y\"]\nvalue = \"secret!!\"\n", "secret!!"},
		{"well-formed", "[token.\"x]y\"]\nvalue = \"Wf7Bracket9x\"\n", "Wf7Bracket9x"},
		{"quoted punctuation, array of tables", "[[token.'a=b,c{d}']]\nvalue = \"Arr7Bracket9x\"\n", "Arr7Bracket9x"},
	} {
		gatherString(t, Declarations{ConfigFields: []string{"token"}}, "c.toml", c.file)
		if !isHeld(c.want) {
			t.Errorf("%s: %q not held; held %q", c.name, c.want, heldValues())
		}
	}
}

// kit14rv Codex r3, request-body-parser-fail-open: a multipart body the
// reader stops reading early (a part header past mime/multipart's limit,
// a part that does not end) is a body the kit could not read whole: the
// request is not sent and acquisition is marked unknown.
func TestKit14MultipartReadErrorFailsClosed(t *testing.T) {
	var b strings.Builder
	b.WriteString("--probe\r\nContent-Disposition: form-data; name=\"password\"\r\n")
	for i := 0; i <= 10000; i++ {
		fmt.Fprintf(&b, "X-%d: h\r\n", i)
	}
	b.WriteString("\r\nsecret!!\r\n--probe--\r\n")
	resetForTest(Declarations{CredentialFields: []string{"password"}})
	sent := false
	rt := roundTripFunc(func(*http.Request) (*http.Response, error) { sent = true; return nil, fmt.Errorf("stop") })
	req, _ := http.NewRequest("POST", "https://outside.example/", strings.NewReader(b.String()))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=probe")
	Transport(rt).RoundTrip(req)
	if sent {
		t.Errorf("a multipart body the kit could not read whole was sent")
	}
	if !isUnknown() {
		t.Errorf("acquisition not marked unknown")
	}
	// Control: an ordinary multipart body is read whole and its field held.
	resetForTest(Declarations{CredentialFields: []string{"password"}})
	req, _ = http.NewRequest("POST", "https://outside.example/", strings.NewReader("--probe\r\nContent-Disposition: form-data; name=\"password\"\r\n\r\nOrd7Multi9x\r\n--probe--\r\n"))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=probe")
	Transport(rt).RoundTrip(req)
	if isUnknown() || !isHeld("Ord7Multi9x") {
		t.Errorf("an ordinary multipart body: unknown %v, held %q", isUnknown(), heldValues())
	}
}

// kit14rv Codex r3, decoder-length-pruning: one character is a candidate
// of Ascii85, whose z (and btoa's y) group decodes to four bytes; the
// one-character shortcuts skip only what no decoder reads.
func TestKit14OneCharacterAscii85Group(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:probe", "\x00\x00\x00\x00")
	m, _ := currentMatcher()
	if v := m.check([]byte{0x80, 'z', 0x80}); v == clean {
		t.Errorf("a lone z between non-text bytes read clean")
	}
	if v := m.check([]byte("z")); v == clean {
		t.Errorf("control z read clean")
	}
}

// kit14rv Codex r4, config-reader-drops-values: a quoted key in a dotted
// path or a table header is unescaped as the reader unescapes any quoted
// key ("token" is token).
func TestKit14EscapedQuotedKeyInHeader(t *testing.T) {
	for _, c := range []struct{ name, file, want string }{
		{"reviewer: escaped header", "[\"to\\u006ben\"]\nvalue = \"secret!!\"\n", "secret!!"},
		{"escaped header in an unclosed array", "bad = [\n[\"to\\u006ben\"]\nvalue = \"Esc7Hdr9Open\"\n", "Esc7Hdr9Open"},
		{"escaped quote in a header key", "[a.\"x\\\"y\".token]\nvalue = \"Esc7Quote9Hdr\"\n", "Esc7Quote9Hdr"},
	} {
		gatherString(t, Declarations{ConfigFields: []string{"token"}}, "c.toml", c.file)
		if !isHeld(c.want) {
			t.Errorf("%s: %q not held; held %q", c.name, c.want, heldValues())
		}
	}
}

// kit14rv Codex r4, request-body-parser-fail-open: a Content-Type the kit
// cannot parse leaves the body's reading (multipart, charset) unknown: the
// request is refused and acquisition marked unknown.
func TestKit14UnparsableContentTypeFailsClosed(t *testing.T) {
	resetForTest(Declarations{CredentialFields: []string{"password"}})
	sent := false
	rt := roundTripFunc(func(*http.Request) (*http.Response, error) { sent = true; return nil, fmt.Errorf("stop") })
	req, _ := http.NewRequest("POST", "https://outside.example/", strings.NewReader("--probe\r\nContent-Disposition: form-data; name=\"password\"\r\n\r\nsecret!!\r\n--probe--\r\n"))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=probe; bad")
	Transport(rt).RoundTrip(req)
	if sent || !isUnknown() {
		t.Errorf("a body under an unparsable Content-Type: sent %v, unknown %v", sent, isUnknown())
	}
}

// kit14rv Codex r4, quoted-printable-decoding-incomplete: RFC 2045 drops
// the blanks at the end of an encoded line.
func TestKit14QuotedPrintableTrailingBlanks(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:probe", "secret!!")
	m, _ := currentMatcher()
	for _, u := range []string{"c2Vj \r\ncmV0ISE=3D", "c2Vj\t \ncmV0ISE=3D", "c2Vj \t\r\ncmV0ISE=3D \t"} {
		if v := m.check([]byte(u)); v == clean {
			t.Errorf("%q read clean", u)
		}
	}
}

// kit14rv Codex r5, config-reader-drops-values: a dotted key whose parts
// are quoted ("token".value, a."b") is one key, read as TOML reads it.
func TestKit14QuotedDottedKey(t *testing.T) {
	for _, c := range []struct{ name, file, want string }{
		{"reviewer: in an unclosed array", "bad = [\n\"token\".value = \"secret!!\"\n", "secret!!"},
		{"well-formed", "\"token\".value = \"Qd7Wf9Key\"\n", "Qd7Wf9Key"},
		{"bare then quoted", "a.\"token\" = \"Qd7Mixed9Key\"\n", "Qd7Mixed9Key"},
		{"quoted, blanks around the dot", "'token' . 'v' = \"Qd7Blank9Key\"\n", "Qd7Blank9Key"},
	} {
		gatherString(t, Declarations{ConfigFields: []string{"token"}}, "c.toml", c.file)
		if !isHeld(c.want) {
			t.Errorf("%s: %q not held; held %q", c.name, c.want, heldValues())
		}
	}
}

// kit14rv Codex r5, request-body-parser-fail-open: a multipart part whose
// Content-Disposition does not parse hides its form name: not whole.
func TestKit14UnparsablePartDispositionFailsClosed(t *testing.T) {
	resetForTest(Declarations{CredentialFields: []string{"password"}})
	sent := false
	rt := roundTripFunc(func(*http.Request) (*http.Response, error) { sent = true; return nil, fmt.Errorf("stop") })
	req, _ := http.NewRequest("POST", "https://outside.example/", strings.NewReader("--probe\r\nContent-Disposition: form-data; name=\"password\"; bad\r\n\r\nsecret!!\r\n--probe--\r\n"))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=probe")
	Transport(rt).RoundTrip(req)
	if sent || !isUnknown() {
		t.Errorf("a part with an unparsable Content-Disposition: sent %v, unknown %v", sent, isUnknown())
	}
}

// kit14rv Codex r5, response-body-parser-fail-open: a response whose
// Content-Type does not parse is not read as plain UTF-8 JSON: its charset
// is unknown, so acquisition is marked unknown.
func TestKit14ResponseUnparsableContentType(t *testing.T) {
	resetForTest(Declarations{ResponseFields: []string{"token"}})
	body, _ := charmap.CodePage037.NewEncoder().Bytes([]byte(`{"token":"secret!!"}`))
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json; charset=IBM037; bad"}},
			Body: io.NopCloser(bytes.NewReader(body)), Request: r}, nil
	})
	req, _ := http.NewRequest("GET", "https://api.example.com/token", nil)
	resp, err := Transport(rt).RoundTrip(req)
	if err == nil {
		io.ReadAll(resp.Body)
	}
	if !isUnknown() && !isHeld("secret!!") {
		t.Errorf("a response under an unparsable Content-Type: nothing held, acquisition known")
	}
}

// The unparsable-media-type blunt rule (kit14rv r4, r5) never fires on
// ordinary headers: every Content-Type literal under skills/*/cli and the
// common server and form shapes parse.
func TestKit14OrdinaryMediaTypesParse(t *testing.T) {
	for _, v := range []string{
		"application/csv",
		"application/ecmascript",
		"application/graphql",
		"application/gzip",
		"application/hal+json;charset=UTF-8;version=2",
		"application/javascript",
		"application/json",
		"application/json; charset=utf-8;",
		"application/json; odata.metadata=minimal; odata.streaming=true; IEEE754Compatible=false; charset=utf-8",
		"application/json;charset=UTF-8",
		"application/octet-stream",
		"application/pdf",
		"application/vnd.api+json",
		"application/x-",
		"application/x-gzip",
		"application/x-ndjson",
		"application/x-www-form-urlencoded",
		"application/xhtml+xml",
		"application/xml",
		"application/zip",
		"attachment; filename=report.pdf",
		"form-data; name=\"f\"; filename*=UTF-8''r%C3%A9sum%C3%A9.pdf",
		"form-data; name=\"file\"; filename=\"a b.txt\"",
		"multipart/form-data",
		"text/csv",
		"text/csv; header=present",
		"text/html",
		"text/json",
		"text/plain",
		"text/plain; charset=utf-8",
		"text/plain; charset=utf-8; format=flowed",
		"text/tabwriter",
	} {
		if _, _, err := mime.ParseMediaType(v); err != nil {
			t.Errorf("%q: %v", v, err)
		}
	}
}

// The blunt rule holds what it reads from a declared name's position under
// that name, even where the reader gives the value other keys (a header
// the format does not allow: the quoted name runs into a bare part).
func TestKit14BluntRuleKeepsTheName(t *testing.T) {
	gatherString(t, Declarations{ConfigFields: []string{"token"}}, "c.toml", "[\"token\"x]\nvalue = \"Forced7Name9x\"\n")
	if !isHeld("Forced7Name9x") {
		t.Errorf("not held; held %q", heldValues())
	}
}

// kit14rv Codex r6, query-parser-error-drops-values: a form body or base
// URL query is read pair by pair, none dropped (url.ParseQuery drops a pair
// with a ';' or a bad escape).
func TestKit14QueryPairsNeverDropped(t *testing.T) {
	resetForTest(Declarations{CredentialFields: []string{"password"}})
	rt := roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, fmt.Errorf("stop") })
	for _, body := range []string{"pass%77ord=secret!!;bad", "password=Semi7Colon9x;rest", "password=Bad7Esc9x%zz&a=b"} {
		resetForTest(Declarations{CredentialFields: []string{"password"}})
		req, _ := http.NewRequest("POST", "https://outside.example/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		Transport(rt).RoundTrip(req)
		if isUnknown() {
			continue // loop ruling 8: refused and acquisition unknown, every unit withheld
		}
		m, _ := currentMatcher()
		if m == nil || m.check([]byte("the value was secret!!")) == clean && m.check([]byte("Semi7Colon9x Bad7Esc9x")) == clean {
			t.Errorf("%q: the password was not held; held %q", body, heldValues())
		}
	}
	resetForTest(Declarations{})
	atStartup(func() { reg.Lock(); holdBaseURLLocked("https://api.example/?token=secret!!;bad"); reg.Unlock() })
	if !slicesContainsPrefix(heldValues(), "secret!!") {
		t.Errorf("base URL query with ';': held %q", heldValues())
	}
}

func slicesContainsPrefix(vs []string, p string) bool {
	for _, v := range vs {
		if strings.HasPrefix(v, p) {
			return true
		}
	}
	return false
}

// kit14rv Codex r6, request-header-case-misses-acquisition: a header map
// entry set directly under another spelling ("content-type") decides how
// the body is read, as it goes on the wire.
func TestKit14HeaderSpellingDecidesReading(t *testing.T) {
	resetForTest(Declarations{CredentialFields: []string{"password"}})
	sent := false
	rt := roundTripFunc(func(*http.Request) (*http.Response, error) { sent = true; return nil, fmt.Errorf("stop") })
	req, _ := http.NewRequest("POST", "https://outside.example/", strings.NewReader("--probe\r\nContent-Disposition: form-data; name=\"password\"\r\n\r\nsecret!!\r\n--probe--\r\n"))
	req.Header = http.Header{"content-type": {"multipart/form-data; boundary=probe"}}
	Transport(rt).RoundTrip(req)
	if !isHeld("secret!!") && !isUnknown() {
		t.Errorf("lowercase content-type: the multipart field was not held and acquisition is known; sent %v", sent)
	}
	resetForTest(Declarations{CredentialFields: []string{"password"}})
	req, _ = http.NewRequest("POST", "https://outside.example/", strings.NewReader("password=Low7Enc9x"))
	req.Header = http.Header{"content-encoding": {"x-unknown"}, "Content-Type": {"application/x-www-form-urlencoded"}}
	Transport(rt).RoundTrip(req)
	if !isUnknown() {
		t.Errorf("lowercase content-encoding the kit cannot undo: acquisition known")
	}
}

// kit14rv Codex r7 (blunt, the header class's third round): a header that
// decides how a body is read, under more than one spelling, or more than
// one Content-Type value (an empty one included), makes the reading not
// whole; Expect is found under any spelling.
func TestKit14AmbiguousBodyHeadersFailClosed(t *testing.T) {
	var zb, gb bytes.Buffer
	zw := zlib.NewWriter(&zb)
	zw.Write([]byte(`{"password":"secret!!"}`))
	zw.Close()
	gw := gzip.NewWriter(&gb)
	gw.Write(zb.Bytes())
	gw.Close()
	rt := roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, fmt.Errorf("stop") })
	for i := 0; i < 20; i++ { // map order varies: every order must fail closed
		resetForTest(Declarations{CredentialFields: []string{"password"}})
		req, _ := http.NewRequest("POST", "https://outside.example/", bytes.NewReader(gb.Bytes()))
		req.Header = http.Header{"Content-Type": {"application/json"}, "Content-Encoding": {"deflate"}, "content-encoding": {"gzip"}}
		Transport(rt).RoundTrip(req)
		if !isUnknown() && !isHeld("secret!!") {
			t.Errorf("coding headers under two spellings: not held, acquisition known")
			break
		}
	}
	resetForTest(Declarations{ResponseFields: []string{"token"}})
	body, _ := charmap.CodePage037.NewEncoder().Bytes([]byte(`{"token":"secret!!"}`))
	rrt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"", "application/json; charset=IBM037"}},
			Body: io.NopCloser(bytes.NewReader(body)), Request: r}, nil
	})
	req, _ := http.NewRequest("GET", "https://api.example.com/token", nil)
	if resp, err := Transport(rrt).RoundTrip(req); err == nil {
		io.ReadAll(resp.Body)
	}
	if !isUnknown() && !isHeld("secret!!") {
		t.Errorf("an empty Content-Type before a charset one: not held, acquisition known")
	}
	if !expectsContinue(http.Header{"expect": {"100-continue"}}) {
		t.Errorf("a lowercase expect key was not seen")
	}
}

// The ambiguous-header rule never fires on ordinary headers: a request built
// with Header.Set and a response net/http parsed from the wire.
func TestKit14OrdinaryHeadersNotAmbiguous(t *testing.T) {
	h := http.Header{}
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Set("Content-Encoding", "gzip")
	h.Add("Transfer-Encoding", "chunked")
	if ambiguousBodyHeaders(h) {
		t.Errorf("a Header.Set request is ambiguous")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if ambiguousBodyHeaders(resp.Header) {
		t.Errorf("a parsed response is ambiguous: %v", resp.Header)
	}
}

// kit14rv Codex r8, multipart-field-name-ambiguity: a part with more than
// one Content-Disposition (or Content-Type) is not read whole.
func TestKit14PartWithTwoDispositions(t *testing.T) {
	resetForTest(Declarations{CredentialFields: []string{"password"}})
	sent := false
	rt := roundTripFunc(func(*http.Request) (*http.Response, error) { sent = true; return nil, fmt.Errorf("stop") })
	req, _ := http.NewRequest("POST", "https://outside.example/", strings.NewReader("--probe\r\nContent-Disposition: form-data; name=\"other\"\r\nContent-Disposition: form-data; name=\"password\"\r\n\r\nsecret!!\r\n--probe--\r\n"))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=probe")
	Transport(rt).RoundTrip(req)
	if sent || !isUnknown() {
		t.Errorf("a part with two Content-Disposition headers: sent %v, unknown %v", sent, isUnknown())
	}
}

// kit14rv Codex r8, coding-header-order-misses-acquisition: a body is
// content-coded first and transfer-coded last, so Transfer-Encoding is
// undone first; and when more than one coding is declared, a step that is
// not its format or stops part way makes the reading not whole.
func TestKit14CodingOrderAcrossHeaders(t *testing.T) {
	var zb, gb bytes.Buffer
	zw := zlib.NewWriter(&zb)
	zw.Write([]byte(`{"password":"secret!!"}`))
	zw.Close()
	gw := gzip.NewWriter(&gb)
	gw.Write(zb.Bytes())
	gw.Close()
	rt := roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, fmt.Errorf("stop") })
	resetForTest(Declarations{CredentialFields: []string{"password"}})
	req, _ := http.NewRequest("POST", "https://outside.example/", bytes.NewReader(gb.Bytes()))
	req.Header = http.Header{"Content-Type": {"application/json"}, "Content-Encoding": {"deflate"}, "Transfer-Encoding": {"gzip"}}
	Transport(rt).RoundTrip(req)
	if !isHeld("secret!!") && !isUnknown() {
		t.Errorf("deflate then transfer gzip: not held, acquisition known")
	}
	// The same bytes declared in the wrong order within one header: a step
	// that is not its format fails the reading closed.
	resetForTest(Declarations{CredentialFields: []string{"password"}})
	req, _ = http.NewRequest("POST", "https://outside.example/", bytes.NewReader(gb.Bytes()))
	req.Header = http.Header{"Content-Type": {"application/json"}, "Content-Encoding": {"gzip, deflate"}}
	Transport(rt).RoundTrip(req)
	if !isHeld("secret!!") && !isUnknown() {
		t.Errorf("codings declared in the wrong order: not held, acquisition known")
	}
}
