// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// Regression tests for the classes the fresh review (kitrv) raised. Each
// test covers its class through the reported input and other members.

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

func b64d(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// sent records the requests the guard let through.
type sent struct{ urls []string }

func (s *sent) RoundTrip(r *http.Request) (*http.Response, error) {
	s.urls = append(s.urls, r.URL.String())
	return &http.Response{StatusCode: 200, Header: http.Header{}, Body: http.NoBody, Request: r}, nil
}

// Class origin-idna-collision (contract 2(c)): the host the kit compares is
// the host the transport dials. A host outside ASCII carrying a credential
// is refused, and an allowed origin must be written as the ASCII host it
// is dialled as.
func TestKitrvOriginHostIsTheDialledHost(t *testing.T) {
	d := Declarations{Origins: []Origin{{Source: "env:CK_TOKEN", Allowed: []string{"https://fass.example"}}}}
	resetForTest(d)
	Hold("env:CK_TOKEN", "secret!!")
	for _, u := range []string{"https://faß.example/", "https://FAß.example/", "https://ｆａｓｓ.example/"} {
		req, _ := http.NewRequest("GET", u, nil)
		req.Header.Set("X-Note", "secret!!")
		if _, err := Transport(mustNotSend{t}).RoundTrip(req); !errors.Is(err, ErrWithheld) {
			t.Errorf("%s: a credential went to a host the kit folded onto its allowed origin: %v", u, err)
		}
	}
	for _, a := range []string{"https://faß.example", "https://bücher.example", "https://*.ｅｘａｍｐｌｅ.com"} {
		bad := Declarations{Origins: []Origin{{Source: "*", Allowed: []string{a}}}}
		if err := bad.validate(); err == nil {
			t.Errorf("allowed origin %q outside ASCII was accepted", a)
		}
	}
	// The ASCII host itself, in any case, still works.
	s := &sent{}
	req, _ := http.NewRequest("GET", "https://FASS.example/", nil)
	req.Header.Set("X-Note", "secret!!")
	if _, err := Transport(s).RoundTrip(req); err != nil || len(s.urls) != 1 {
		t.Errorf("a credential to its own allowed origin was refused: %v", err)
	}
}

// Class outbound-short-value-exemption (contract 2(c)): an outbound
// request is judged by (c) alone, so a held value of any length counts
// anywhere in it, not only as a whole token.
func TestKitrvOutboundShortValueAnywhere(t *testing.T) {
	d := Declarations{Origins: []Origin{{Source: "minted:review", Allowed: []string{"https://api.example.com"}}}}
	resetForTest(d)
	Hold("minted:review", "abc")
	reqs := map[string]func() *http.Request{
		"query": func() *http.Request { r, _ := http.NewRequest("GET", "https://off.invalid/?note=xabcx", nil); return r },
		"header": func() *http.Request {
			r, _ := http.NewRequest("GET", "https://off.invalid/", nil)
			r.Header.Set("X-Note", "xabcx")
			return r
		},
		"body": func() *http.Request {
			r, _ := http.NewRequest("POST", "https://off.invalid/", strings.NewReader("zzabczz"))
			return r
		},
		"base64": func() *http.Request {
			r, _ := http.NewRequest("POST", "https://off.invalid/", strings.NewReader("x"+base64.StdEncoding.EncodeToString([]byte("xabcx"))))
			return r
		},
		"plain": func() *http.Request { r, _ := http.NewRequest("GET", "http://api.example.com/?q=xabcx", nil); return r },
	}
	for name, mk := range reqs {
		if _, err := Transport(mustNotSend{t}).RoundTrip(mk()); !errors.Is(err, ErrWithheld) {
			t.Errorf("%s: a short held value inside a word went off its origins: %v", name, err)
		}
	}
	s := &sent{}
	r, _ := http.NewRequest("GET", "https://api.example.com/?note=xabcx", nil)
	if _, err := Transport(s).RoundTrip(r); err != nil || len(s.urls) != 1 {
		t.Errorf("a short held value to its own origin was refused: %v", err)
	}
}

// trailerResponse answers with body, then trailer once the body is read.
type trailerResponse struct {
	body    string
	trailer http.Header
}

func (f trailerResponse) RoundTrip(r *http.Request) (*http.Response, error) {
	resp := &http.Response{StatusCode: 200, Header: http.Header{}, Request: r, Trailer: http.Header{}}
	resp.Body = io.NopCloser(&trailerBody{r: strings.NewReader(f.body), resp: resp, t: f.trailer})
	return resp, nil
}

type trailerBody struct {
	r    io.Reader
	resp *http.Response
	t    http.Header
}

func (b *trailerBody) Read(p []byte) (int, error) {
	n, err := b.r.Read(p)
	if err == io.EOF {
		for k, v := range b.t {
			b.resp.Trailer[k] = v
		}
	}
	return n, err
}

// Class trailer-credentials-unheld (contract 1): headers and trailers,
// of requests and responses, are held through one path: a cookie's value
// in a Cookie or Set-Cookie line is held wherever the line is, and
// whether or not Go's cookie parser accepts it.
func TestKitrvCookiesHeldInHeadersAndTrailers(t *testing.T) {
	refused := func(name string, req *http.Request, tok string) {
		t.Helper()
		if _, err := Transport(mustNotSend{t}).RoundTrip(req); !errors.Is(err, ErrWithheld) {
			t.Errorf("%s: not refused: %v", name, err)
		}
		if !isHeld(tok) {
			t.Errorf("%s: %q not held: %q", name, tok, heldValues())
		}
	}
	resetForTest(Declarations{})
	req, _ := http.NewRequest("POST", "http://off.invalid/", strings.NewReader("x"))
	req.TransferEncoding = []string{"chunked"}
	req.Trailer = http.Header{"Cookie": {"sid=trailer-secret-123"}}
	refused("request trailer cookie", req, "trailer-secret-123")

	resetForTest(Declarations{})
	req, _ = http.NewRequest("GET", "http://off.invalid/", nil)
	req.Header.Set("Cookie", "sid=spaced secret 456")
	refused("cookie Go's parser rejects", req, "spaced secret 456")

	resetForTest(Declarations{})
	req, _ = http.NewRequest("GET", "http://off.invalid/", nil)
	req.Header.Set("Cookie", `a=1; sid="quoted-secret-789"`)
	refused("quoted cookie", req, "quoted-secret-789")

	for name, tr := range map[string]http.Header{
		"response trailer set-cookie": {"Set-Cookie": {"sid=trailer-set-cookie-1; Path=/"}},
		"set-cookie Go rejects":       {"Set-Cookie": {"sid=bad cookie value 2; HttpOnly"}},
	} {
		resetForTest(Declarations{})
		req, _ := http.NewRequest("GET", "https://api.example.com/", nil)
		resp, err := Transport(trailerResponse{body: "ok", trailer: tr}).RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		io.ReadAll(resp.Body)
		want := strings.TrimPrefix(strings.Split(tr.Get("Set-Cookie"), ";")[0], "sid=")
		if !isHeld(want) {
			t.Errorf("%s: %q not held after the body ended: %q", name, want, heldValues())
		}
	}
	resetForTest(Declarations{})
	req, _ = http.NewRequest("GET", "https://api.example.com/", nil)
	resp, _ := Transport(fixedResponse{h: http.Header{"Set-Cookie": {"sid=header bad value 3"}}}).RoundTrip(req)
	resp.Body.Close()
	if !isHeld("header bad value 3") {
		t.Errorf("a Set-Cookie value Go rejects was not held: %q", heldValues())
	}
}

// Class config-reader-drops-values (contract 1): YAML node properties (an
// anchor, a tag, both, in either order) in front of any scalar form leave
// the scalar read exactly as it is without them.
func TestKitrvConfigNodeProperties(t *testing.T) {
	for name, tc := range map[string]struct{ file, want string }{
		"anchor, double quoted":  {"token: &a \"s\\u0065cret!!\"\n", "secret!!"},
		"tag, double quoted":     {"token: !!str \"s\\u0065cret!!\"\n", "secret!!"},
		"tag and anchor":         {"token: !!str &a \"t\\x61g-secret-1\"\n", "tag-secret-1"},
		"anchor and tag":         {"token: &a !!str 'it''s-secret-2'\n", "it's-secret-2"},
		"anchor, block scalar":   {"token: &a |\n  block-secret-3\n", "block-secret-3"},
		"tag, folded scalar":     {"token: !!str >-\n  folded\n  secret-4\n", "folded secret-4"},
		"list item with anchor":  {"tokens:\n  - &a \"l\\u0069st-secret-5\"\n", "list-secret-5"},
		"flow value with anchor": {"{token: &a \"fl\\u006fw-secret-6\"}\n", "flow-secret-6"},
		"env value starting !":   {"TOKEN=!bang-secret-7\n", "!bang-secret-7"},
	} {
		gatherString(t, Declarations{ConfigFields: []string{"token", "tokens", "TOKEN"}}, "config.yaml", tc.file)
		if !isHeld(tc.want) {
			t.Errorf("%s: %q not held: %q", name, tc.want, heldValues())
		}
	}
}

// Class case-insensitive-source-exemption (contract 1): a declared source
// names a source exactly, whitespace included.
func TestKitrvSourceNamesAreExact(t *testing.T) {
	resetForTest(Declarations{
		SecretEnv:   []string{"TOKEN"},
		Identifiers: []Identifier{{Source: "env:TOKEN ", Reason: "the variable whose name ends in a space is an account id"}},
	})
	Hold("env:TOKEN", "secret!!")
	if !isHeld("secret!!") {
		t.Error("an identifier with a trailing space exempted a different source")
	}
	resetForTest(Declarations{
		Identifiers: []Identifier{{Source: " config:user", Reason: "an account id"}},
	})
	Hold("config:user", "user-secret-1")
	if !isHeld("user-secret-1") {
		t.Error("an identifier with a leading space exempted a different source")
	}
}

// Class credential-flag-cluster-missed (contract 1): a credential flag's
// value is held in every spelling the flag parser accepts, inside a
// cluster of one-letter flags too.
func TestKitrvCredentialFlagClusters(t *testing.T) {
	d := Declarations{CredentialFlags: []string{"--token", "-t"}}
	for name, tc := range map[string]struct {
		args []string
		want string
	}{
		"cluster, value attached": {[]string{"-vtsecret!!"}, "secret!!"},
		"cluster, value next":     {[]string{"-vt", "next-secret-1"}, "next-secret-1"},
		"cluster, equals":         {[]string{"-vxt=eq-secret-2"}, "eq-secret-2"},
		"plain short":             {[]string{"-t", "short-secret-3"}, "short-secret-3"},
	} {
		resetForTest(d)
		gather(d, tc.args)
		if !isHeld(tc.want) {
			t.Errorf("%s: %q not held: %q", name, tc.want, heldValues())
		}
	}
}

// Class decoder-binary-intermediate-pruning (contract 3): every character a
// held value has is relevant, an ASCII control character included.
func TestKitrvHeldControlCharactersAreRelevant(t *testing.T) {
	holding(t, "\x1b")
	if !leaks("\x27") { // IBM Code Page 037 reads 0x27 as ESC
		t.Error("a held control character spelled in EBCDIC was not found")
	}
	holding(t, "a\x01b\x02c\x03d\x04")
	if !leaks("x a\x01b\x02c\x03d\x04 y") {
		t.Error("a held value with control characters was not found as written")
	}
	if !leaks(string(utf16LE("a\x01b\x02c\x03d\x04"))) {
		t.Error("a held value with control characters was not found in UTF-16")
	}
}

// Class charset-decoder-aliases-missed (contract 3): a multi-byte charset
// is read by what its decoder accepts, not only by what its encoder writes.
func TestKitrvCharsetDecoderAliases(t *testing.T) {
	holding(t, "∪∪∪∪∪∪∪∪")
	b, _ := hex.DecodeString("879c879c879c879c879c879c879c879c") // Shift-JIS: 87 9C reads as U+222A
	if !leaks(string(b)) {
		t.Error("a Shift-JIS spelling the encoder does not write was not read")
	}
	holding(t, "ⅰⅱⅲⅳⅴⅵⅶⅷ")
	b, _ = hex.DecodeString("fa40fa41fa42fa43fa44fa45fa46fa47") // Shift-JIS IBM extension row: small roman numerals
	if !leaks(string(b)) {
		t.Error("a Shift-JIS IBM-extension spelling was not read")
	}
}

// Class decoder-length-pruning (contract 3): no decoder declines an input
// for its size without spending the budget; a number spelled with any
// exponent, and a code list with any amount of space, are read.
func TestKitrvNoDecoderSizeLimit(t *testing.T) {
	holding(t, "1"+strings.Repeat("0", 1025))
	if !leaks("1e1025") {
		t.Error("a 1026-digit credential spelled 1e1025 was not found")
	}
	holding(t, "1"+strings.Repeat("0", 20))
	if !leaks("1e00020") {
		t.Error("an exponent written with five digits was not read")
	}
	holding(t, "secret!!")
	for _, u := range []string{"115    101    99    114    101    116    33    33", "115,\n\n\n\n101, 99, 114, 101, 116, 33, 33", "0x73 ,   0x65 , 0x63,0x72,0x65,0x74,0x21,0x21"} {
		if !leaks(u) {
			t.Errorf("a code list was not read: %q", u)
		}
	}
	holding(t, "1"+strings.Repeat("0", 1025))
	for _, u := range []string{"1.0e1025", "10e1024", "0.1e1026", "100e+1023"} {
		if !leaks(u) {
			t.Errorf("a long digit-only credential spelled %s was not found", u)
		}
	}
}

// Class raw-deflate-depth-fail-open (contract 3): headerless raw deflate
// nests like any other decoding; where the kit stops reading it, the unit
// is withheld.
func TestKitrvRawDeflateNesting(t *testing.T) {
	holding(t, "secret!!")
	if !withheld(b64d(t, "S66KrLIwTzLwy/IodylzrExJN/etytYPStEPMLAozzIIKXP1SvHwMyjxCXVzdLS1BQA=")) {
		t.Error("four layers of raw deflate over base64 were not read")
	}
	u := []byte("prefix secret!! suffix")
	for range 5 {
		u = df(u)
	}
	if !withheld(u) {
		t.Error("five layers of raw deflate were not read")
	}
}

// Class raw-deflate-error-fail-open (contract 3): what a raw deflate
// stream decodes before it breaks is a decoding, like a gzip or zlib
// stream's.
func TestKitrvBrokenRawDeflatePrefix(t *testing.T) {
	holding(t, "secret!!")
	b, _ := hex.DecodeString("2b4e4d2e4a2d515404")
	if !withheld(b) {
		t.Error("a fixed-Huffman stream cut before its end was not read")
	}
	full := df([]byte(strings.Repeat("filler text ", 20) + "secret!! " + strings.Repeat("more filler ", 40)))
	if !withheld(full[:len(full)-6]) {
		t.Error("a dynamic-Huffman stream cut before its end was not read")
	}
	stored := []byte{0x00, 0x20, 0x00, 0xdf, 0xff}
	stored = append(stored, "the secret!! inside a stored block"...)
	if !withheld(stored) {
		t.Error("a stored block cut before its end was not read")
	}
}

// Class decompressed-repeat-folding (contract 3): every decoding is read
// exactly as decoded; a repeat in it is never shortened before the
// decoders that read it run.
func TestKitrvNoFolding(t *testing.T) {
	holding(t, "secret!!")
	if !withheld(b64d(t, "H4sIAAAAAAAC/2NYwx/8wXEUjIJRMOxBslFY1mgojIJRMALyem6YwWgojIJRMPyBZ7Cr7WgojIJRMPwB89aHjqXWGueZGQCkupzGuw8AAA==")) {
		t.Error("a raw deflate stream inside gzip whose stored dictionary repeats was not read")
	}
	// A stored block whose data repeats, gzipped: folding the gzip output
	// breaks the stored block's length.
	data := append(bytes.Repeat([]byte("ab"), 300), []byte(base64.StdEncoding.EncodeToString([]byte("secret!!")))...)
	blk := append([]byte{0x01, byte(len(data)), byte(len(data) >> 8), ^byte(len(data)), ^byte(len(data) >> 8)}, data...)
	if !withheld(gz(blk)) {
		t.Error("a stored block with a repeat, inside gzip, was not read")
	}
}

// Class joined-values-incomplete (contract 3): the joined values of a
// structured document are a decoding like any other, read by every
// decoder, and every structured format's values are joined without its
// keys and separators.
func TestKitrvJoinedValuesDecoded(t *testing.T) {
	holding(t, "secret!!")
	// The amended contract (section 3, joined values) joins JSON documents
	// only, each a view or one line of one: the TOML, YAML, env and
	// mid-line cases this class once listed are out of scope.
	for _, u := range []string{
		`{"a":"c2Vj","b":"cmV0","c":"ISE="}`,
		"log line\n{\"a\":\"secr\",\"b\":\"et!!\"}\n",
		"[{\"a\":\"c2Vj\"},{\"b\":\"cmV0ISE=\"}]",
	} {
		if !leaks(u) {
			t.Errorf("a credential split across the values of one document was not found: %q", u)
		}
	}
}

// Class invalid-utf8-piece-counting (contract 2): for a value that is not
// valid UTF-8 a character is a byte, so every 8-byte piece counts.
func TestKitrvInvalidUTF8PiecesAreBytes(t *testing.T) {
	holding(t, "\xff"+strings.Repeat("é", 8))
	for _, u := range []string{"éééé", "x éééé y"} {
		if !leaks(u) {
			t.Errorf("an 8-byte piece of a held value that is not valid UTF-8 was not found: %q", u)
		}
	}
	holding(t, "\xfe\xfd"+"abcdefgh")
	if !leaks("\xfe\xfdabcdef") {
		t.Error("an 8-byte piece holding invalid bytes was not found")
	}
}

// Class stream-context-truncation (contract 2): a stream's state keeps
// what the matcher needs across writes, a structured document's values
// included, however much lies between them.
func TestKitrvStreamStateKeepsJoinedValues(t *testing.T) {
	holding(t, "secret!!")
	var out bytes.Buffer
	w := NewWriter(&out, SinkFile)
	w.Write([]byte(`{"a":"secr",` + strings.Repeat(" ", 2048)))
	w.Write([]byte(`"b":"et!!"}`))
	if strings.Contains(out.String(), `et!!`) {
		t.Errorf("a credential joined across writes was released: %.60q", out.String()[max(0, out.Len()-60):])
	}
	out.Reset()
	w = NewWriter(&out, SinkFile)
	w.Write([]byte(`{"a":"secr",` + strings.Repeat(`"n":1,`, 2000)))
	w.Write([]byte(`"b":"et!!"}`))
	if strings.Contains(out.String(), `et!!`) {
		t.Error("a credential joined across writes, numbers between, was released")
	}
}

// Class stdout-write-unit-splitting (contract 4): a pause in reading the
// pipe while a write is still being delivered never cuts the write.
func TestKitrvStdoutStalledReadKeepsTheWrite(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:x", "abcdefgh")
	startedOnce.Do(func() { close(started) })
	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	first := true
	pumpReadHook = func() { // the reader is descheduled after its first read
		if first {
			first = false
			time.Sleep(40 * time.Millisecond)
		}
	}
	defer func() { pumpReadHook = nil }()
	p := &pump{fd: 1, sink: SinkStdout, in: inR, out: outW, done: make(chan struct{})}
	go p.run()
	got := make(chan []byte)
	go func() { b, _ := io.ReadAll(outR); got <- b }()
	inW.Write(append(bytes.Repeat([]byte("x"), 1<<20), "abcdefgh\n"...))
	time.Sleep(50 * time.Millisecond)
	inW.Close()
	<-p.done
	outW.Close()
	out := <-got
	if bytes.Contains(out, []byte("abcdefgh")) {
		t.Fatal("the credential was released")
	}
	if n := bytes.Count(out, []byte("x")); n > 0 {
		t.Errorf("%d bytes of a withheld write were released", n)
	}
}

// Class child-envelope-unchecked (contract 5): the child's argv and
// environment are checked as the child gets them, the kit's own
// additions included.
func TestKitrvChildEnvelopeCheckedWhole(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:x", "CREDKIT_HELD")
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = []string{}
	if err := StartCommand(cmd); !errors.Is(err, ErrWithheld) {
		if err == nil {
			cmd.Wait()
		}
		t.Errorf("a child whose environment, with the kit's addition, carries a held value was started: %v", err)
	}
}

// Class mcp-error-bypasses-guard (contract 4 and 5): every byte an MCP
// HTTP response carries is checked, its status line and headers too, and
// nothing net/http would write on the kit's behalf goes unchecked.
func TestKitrvMCPHTTPHeadChecked(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
	})
	raw := func(h http.Handler, req string) string {
		t.Helper()
		srv := httptest.NewServer(MCPHTTP(h))
		defer srv.Close()
		c, err := net.Dial("tcp", srv.Listener.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		c.SetDeadline(time.Now().Add(5 * time.Second))
		io.WriteString(c, req)
		b, _ := io.ReadAll(bufio.NewReader(c))
		return string(b)
	}
	resetForTest(Declarations{})
	// "Bad Request" alone is public text, not held when acquired while
	// running (contract 2); a value that overlaps it is, by its pieces.
	Hold("minted:x", "Bad Request!")
	if got := raw(ok, "POST / HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\nContent-Type: application/json\r\nConnection: close\r\n\r\nz\r\n"); strings.Contains(got, "Bad Request") {
		t.Errorf("the kit's error status line carried a held value: %q", got)
	}
	resetForTest(Declarations{})
	Hold("minted:x", "header-secret-123")
	leaky := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Debug", "header-secret-123")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
	})
	body := `{"jsonrpc":"2.0","id":1,"method":"ping"}`
	req := "POST / HTTP/1.1\r\nHost: x\r\nContent-Type: application/json\r\nConnection: close\r\nContent-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n" + body
	if got := raw(leaky, req); strings.Contains(got, "header-secret-123") {
		t.Errorf("a handler header carried a held value: %q", got)
	}
	sse := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("X-Debug", "header-secret-123")
		w.WriteHeader(200)
		w.Write([]byte("event: message\ndata: {}\n\n"))
	})
	if got := raw(sse, req); strings.Contains(got, "header-secret-123") {
		t.Errorf("a stream's header carried a held value: %q", got)
	}
	resetForTest(Declarations{})
	Hold("minted:x", "Internal Server Error!")
	fail := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) })
	if got := raw(fail, req); strings.Contains(got, "Internal Server Error") {
		t.Errorf("a status line carried a held value: %q", got)
	}
}
