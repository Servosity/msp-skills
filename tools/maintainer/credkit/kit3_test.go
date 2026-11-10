// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// The ten classes the kitrv review left open, each through the input the
// review gave and other members of the class. Each names the structural
// change that closes it: A (one budget), B (one exit point per output path)
// or C (the public-text rule).

import (
	"bufio"
	"bytes"
	"compress/flate"
	"compress/zlib"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
	"unicode/utf8"

	"go/ast"
	"go/build"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/transform"
	"path/filepath"
	"slices"
	"sync/atomic"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// brokenDeflate is a raw deflate stream of payload and a random tail, cut
// short inside the tail: inflating it yields payload, then an error.
func brokenDeflate(t *testing.T, payload []byte) []byte {
	t.Helper()
	var text bytes.Buffer
	for i := range 64 {
		fmt.Fprintf(&text, "line %d of the log: the job ran and wrote its report\n", i*7919%1000)
	}
	var b bytes.Buffer
	w, _ := flate.NewWriter(&b, flate.BestCompression)
	w.Write(payload)
	w.Write(text.Bytes())
	w.Close()
	if btype := b.Bytes()[0] >> 1 & 3; btype != 2 {
		t.Fatalf("fixture: block type %d, want a dynamic block (2)", btype)
	}
	cut := b.Bytes()[:b.Len()/2]
	out, err := io.ReadAll(flate.NewReader(bytes.NewReader(cut)))
	if err == nil || !bytes.HasPrefix(out, payload) {
		t.Fatalf("fixture: the cut stream does not yield the payload, then an error (%d bytes, %v)", len(out), err)
	}
	if bytes.Contains(cut, payload[6:len(payload)-4]) {
		t.Fatal("fixture: the payload is in the stream as written")
	}
	return cut
}

// Class raw-deflate-error-fail-open (A): what a broken raw deflate stream
// yields before its error is a view, read by every decoder.
func TestKit3RawDeflateBrokenPrefixIsAView(t *testing.T) {
	const v = "secret!!"
	holding(t, v)
	var z bytes.Buffer
	zw := zlib.NewWriter(&z)
	zw.Write([]byte(v))
	zw.Close()
	units := map[string][]byte{
		"review input: base64 in a broken fixed-Huffman prefix": mustHex(t, "4b360acb4ace0d33f00c76b505"),
		"base64 in a broken dynamic block":                      brokenDeflate(t, []byte("note: "+base64.StdEncoding.EncodeToString([]byte(v))+" end")),
		"hex in a broken dynamic block":                         brokenDeflate(t, []byte("note: "+hex.EncodeToString([]byte(v))+" end")),
		"UTF-16 in a broken dynamic block":                      brokenDeflate(t, append(append([]byte("note: "), utf16LE(v)...), " end"...)),
		"a zlib stream in a broken dynamic block":               brokenDeflate(t, append(append([]byte("note: "), z.Bytes()...), " end"...)),
	}
	for name, u := range units {
		if !withheld(u) {
			t.Errorf("%s: released", name)
		}
	}
}

// Class decoder-length-pruning (A): no decoding is dropped for its
// length; a short one can expand (NFKC, decompression) and decode on.
func TestKit3NoLengthGate(t *testing.T) {
	holding(t, "株式会社")
	units := map[string][]byte{
		"review input: a 4-byte broken deflate prefix, NFKC": mustHex(t, "7bdcbb1f"),
		"hex of one 3-byte character, NFKC":                  []byte("e38dbf"),
		"base64 of one 3-byte character, NFKC":               []byte("442/"),
		"base32 of one 3-byte character, NFKC":               []byte("4OG36"),
	}
	for name, u := range units {
		if !withheld(u) {
			t.Errorf("%s: released", name)
		}
	}
}

// Class decoder-binary-intermediate-pruning (A): a run of one character
// inside binary is read like any other run.
func TestKit3OneCharacterRunsAreRead(t *testing.T) {
	// "㎉" normalizes to "kcal", which base64-decodes to 91 c6 a5.
	holding(t, "\x91\xc6\xa5")
	for _, u := range []string{"\x00㎉\x01", "\xff㎉\xfe", "\x00\x00㎉"} {
		if !withheld([]byte(u)) {
			t.Errorf("%q: released", u)
		}
	}
}

// Class charset-decoder-aliases-missed (A): a charset reading is what the
// charset's decoder outputs, every sequence it accepts included: one that
// decodes to several characters, and one the encoder never writes.
func TestKit3CharsetReadingsAreTheDecoders(t *testing.T) {
	cases := []struct {
		name string
		held string
		unit []byte
	}{
		{"review input: Big5 88 62, two characters", strings.Repeat("Ê̄", 4), bytes.Repeat([]byte{0x88, 0x62}, 4)},
		{"review input: GB18030 81 3a 81 30, an alias", strings.Repeat("㒣", 8), bytes.Repeat([]byte{0x81, 0x3a, 0x81, 0x30}, 8)},
		{"Big5 88 64, two characters", strings.Repeat("Ê̌", 4), bytes.Repeat([]byte{0x88, 0x64}, 4)},
		{"Big5 88 a3, two characters", strings.Repeat("ê̄", 4), bytes.Repeat([]byte{0x88, 0xa3}, 4)},
	}
	// Every other GB18030 four-byte sequence that decodes to a character
	// its encoder writes another way: a sample of them.
	dec, enc := simplifiedchinese.GB18030.NewDecoder(), simplifiedchinese.GB18030.NewEncoder()
	found := 0
	for lin := 0; lin < 39420 && found < 4; lin += 97 {
		s := []byte{byte(0x81 + lin/12600), byte(0x30 + lin/1260%10), byte(0x81 + lin/10%126), byte(0x30 + lin%10)}
		d, _, err := transform.Bytes(dec, s)
		r, n := utf8.DecodeRune(d)
		if err != nil || n != len(d) || r == utf8.RuneError || r < 0x80 {
			continue
		}
		if e, _, err := transform.Bytes(enc, d); err == nil && !bytes.Equal(e, s) {
			cases = append(cases, struct {
				name string
				held string
				unit []byte
			}{fmt.Sprintf("GB18030 % x, an alias of % x", s, e), strings.Repeat(string(r), 8), bytes.Repeat(s, 8)})
			found++
		}
	}
	// A GB18030 alias above the BMP: a math letter whose NFKC is ASCII.
	if b, err := simplifiedchinese.GB18030.NewEncoder().Bytes([]byte("\U0001D41A")); err == nil && len(b) == 4 {
		for b2 := byte(0x30); b2 < 0x40; b2++ {
			k := (int(b[0])-0x81)*10 + int(b[1]) - 0x30 - int(b2-0x30)
			if b2 == b[1] || k < 0 || k%10 != 0 {
				continue
			}
			s := []byte{byte(0x81 + k/10), b2, b[2], b[3]}
			if d, _, err := transform.Bytes(dec, s); err == nil && string(d) == "\U0001D41A" {
				cases = append(cases, struct {
					name string
					held string
					unit []byte
				}{fmt.Sprintf("GB18030 % x, an alias of % x", s, b), "aaaaaaaa", bytes.Repeat(s, 8)})
			}
		}
	}
	// Big5's two-character sequences, all of them.
	bdec := traditionalchinese.Big5.NewDecoder()
	for lead := 0x81; lead <= 0xfe; lead++ {
		for trail := 0x40; trail <= 0xfe; trail++ {
			d, _, err := transform.Bytes(bdec, []byte{byte(lead), byte(trail)})
			if err == nil && utf8.RuneCount(d) == 2 && !bytes.ContainsRune(d, utf8.RuneError) {
				cases = append(cases, struct {
					name string
					held string
					unit []byte
				}{fmt.Sprintf("Big5 %02x %02x", lead, trail), strings.Repeat(string(d), 4), bytes.Repeat([]byte{byte(lead), byte(trail)}, 4)})
			}
		}
	}
	for _, c := range cases {
		holding(t, c.held)
		if !withheld(c.unit) {
			t.Errorf("%s: released", c.name)
		}
	}
}

// Class joined-values-incomplete (A): every JSON document (a view, or one
// line of a view), however large, is joined over its string values, and
// the join is decoded with no size cap.
func TestKit3JoinedValuesEveryDocument(t *testing.T) {
	holding(t, "secret!!")
	doc := `{"a":"secr","b":"et!!"}`
	units := map[string]string{
		"review input: padded past 4 KiB":  `{"pad":"` + strings.Repeat("~", 4100) + `","a":"secr","b":"et!!"}`,
		"base64 pieces, padded past 4 KiB": `{"pad":"` + strings.Repeat("x", 4100) + `","a":"c2Vj","b":"cmV0","c":"ISE="}`,
		"one line of several":              "status: ok\n" + doc + "\nend\n",
		"a line of a binary view":          "\x00\x01\n" + doc + "\n\xfe",
		"a document a decoder yields":      base64.StdEncoding.EncodeToString([]byte(doc)),
		"numbers and names between":        `{"a":"secr","n":1,"m":[true,null,2],"b":"et!!"}`,
		"escapes in the values":            `{"a":"se\u0063r","b":"\u0065t!!"}`,
		"still open where the view ends":   `{"a":"secr","b":"et!` + `!`,
	}
	for name, u := range units {
		if !withheld([]byte(u)) {
			t.Errorf("%s: released", name)
		}
	}
	// Member names are not values: the over-withholding guard.
	if withheld([]byte(`{"secr":"x","et!!":"y"}`)) {
		t.Error("member names were joined")
	}
}

// Class stream-context-truncation (A): a JSON document still open keeps
// its head in the stream's context (kit8: the matcher reports it, so
// brackets inside its strings do not close it), so the write that
// completes a join is judged on the whole document; past the context's
// limit the stream is lost and withholds.
func TestKit3StreamDocumentStateIgnoresStrings(t *testing.T) {
	holding(t, "secret!!")
	for _, c := range []struct {
		first string
		joins bool // the stream, whole, joins secret!!
	}{
		{`{"}":"secr",`, true},  // review input
		{`{"a]":"secr",`, true}, // a closing bracket inside a member name
		{`[{"a":"x}"},"secr",`, false},
		{`{"a":"\"}","b":"secr",`, true}, // an escaped quote, then a brace, inside a string
	} {
		for _, gap := range []int{2048, 17 << 10} { // inside the context; past its limit
			var out bytes.Buffer
			w := NewWriter(&out, SinkFile)
			w.Write([]byte(c.first + strings.Repeat(" ", gap)))
			w.Write([]byte(`"z":"et!!"}`))
			want := c.joins || gap > streamTailMax
			if got := !strings.Contains(out.String(), "et!!"); got != want {
				t.Errorf("%s, gap %d: withheld %v, want %v", c.first, gap, got, want)
			}
		}
	}
}

// Class stdout-write-unit-splitting (B): one write to stdout is one unit,
// released only after all of it is checked, however long the pipe's reader
// is held up mid-write.
func TestKit3StdoutStalledReaderNeverSplitsAWrite(t *testing.T) {
	out, errOut, code := runChild(t, "kit3-stall", "")
	if code != 0 {
		t.Fatalf("exit %d, stderr %q", code, errOut)
	}
	if strings.Contains(out, `"tickets"`) || strings.Contains(out, "printer") {
		t.Errorf("part of a write that carried a credential was released (%d bytes): %.120q", len(out), out)
	}
	if !strings.Contains(out, Notice(SinkStdout)) {
		t.Errorf("no notice for the withheld write: %.120q", out)
	}
}

// kit3Child runs a kit3 child mode; it reports whether mode was one.
func kit3Child(mode string) bool {
	switch mode {
	case "kit3-stall":
		var stalled atomic.Bool // the hook runs on both pumps' readers
		pumpReadHook = func() {
			if stalled.CompareAndSwap(false, true) {
				time.Sleep(60 * time.Millisecond)
			}
		}
		Start(Declarations{})
		Hold("minted:x", "secret!!")
		unit := append(benchJSON(300<<10), "\nsecret!!\n"...)
		syscall.Write(syscall.Stdout, unit) // fd 1; on Windows the standard output handle
		Exit(0)
	case "kit3-startup":
		Start(Declarations{SecretEnv: []string{"KIT3_TOKEN"}})
		os.Stdout.WriteString("started\n")
		Exit(0)
	case "kit3-record":
		Start(Declarations{})
		Hold("minted:session", "Bad Request")
		os.Stdout.WriteString("HTTP/1.1 400 Bad Request\n")
		Exit(0)
	}
	return false
}

// C, public text at startup: a startup credential the matcher finds whole
// in one public-text entry (here only after case folding) refuses startup,
// naming the variable and never the value; one that only overlaps public
// text does not collide.
func TestKit3PublicTextStartupCollisionRefuses(t *testing.T) {
	t.Setenv("KIT3_TOKEN", "NOT FOUND")
	out, errOut, code := runChild(t, "kit3-startup", "")
	if code == 0 || strings.Contains(out, "started") {
		t.Fatalf("a startup credential that collides with public text started the program (exit %d): %q", code, out)
	}
	if !strings.Contains(errOut, "env:KIT3_TOKEN") || !strings.Contains(errOut, "public text") || strings.Contains(errOut, "NOT FOUND") {
		t.Errorf("the refusal must name the variable only: %q", errOut)
	}
	t.Setenv("KIT3_TOKEN", "Not Found!")
	if out, errOut, code := runChild(t, "kit3-startup", ""); code != 0 || !strings.Contains(out, "started") {
		t.Errorf("a value that only overlaps public text refused startup (exit %d): %q %q", code, out, errOut)
	}
}

// C, public text while running: a value acquired while running that occurs
// in public text as written is not held and is recorded by its source; one
// that collides only through another spelling, or overlaps, stays held.
func TestKit3PublicTextRunTimeValueNotHeld(t *testing.T) {
	out, errOut, code := runChild(t, "kit3-record", "")
	if code != 0 {
		t.Fatalf("exit %d: %q", code, errOut)
	}
	if !strings.Contains(out, "400 Bad Request") {
		t.Errorf("a run-time value that is public text was held: %q", out)
	}
	if !strings.Contains(errOut, "minted:session") || !strings.Contains(errOut, "public text") {
		t.Errorf("the value that was not held was not recorded by its source: %q", errOut)
	}
	resetForTest(Declarations{})
	Hold("minted:a", "BAD REQUEST", "Bad Requestor", "OK!")
	for _, v := range []string{"BAD REQUEST", "Bad Requestor", "OK!"} {
		if !isHeld(v) {
			t.Errorf("%q is not public text as written but was not held", v)
		}
	}
}

// C, the kit's own framing: a held value whose piece is in the JSON-RPC
// framing withholds the frame, which the replacement stands in for; when
// the replacement's own framing would match too, the MCP stdio session
// fails closed: nothing is written, the stream is closed, and every later
// write fails.
func TestKit3MCPStdioFailsClosed(t *testing.T) {
	frame := []byte(`{"jsonrpc":"2.0","id":1,"result":{}}` + "\n")
	resetForTest(Declarations{})
	Hold("minted:x", `x"result"x`)
	var out closeBuf
	fw := &frameWriter{w: &out}
	if _, err := fw.Write(frame); err != nil || !strings.Contains(out.String(), `"error"`) || strings.Contains(out.String(), `"result"`) {
		t.Errorf("a frame whose framing carried a piece was not replaced: %v %q", err, out.String())
	}
	resetForTest(Declarations{})
	Hold("minted:x", `x"jsonrpc"`)
	out = closeBuf{}
	fw = &frameWriter{w: &out}
	_, err := fw.Write(frame)
	_, err2 := fw.Write(frame)
	if err == nil || err2 == nil || out.Len() != 0 || !out.closed {
		t.Errorf("the session did not fail closed: %v %v %q closed=%v", err, err2, out.String(), out.closed)
	}
}

type closeBuf struct {
	bytes.Buffer
	closed bool
}

func (c *closeBuf) Close() error { c.closed = true; return nil }

// Class trailer-credentials-unheld (B): a response's trailers are held
// before the program reads the response, whatever its body is.
func TestKit3TrailersHeldWhateverTheBody(t *testing.T) {
	for _, body := range []io.ReadCloser{http.NoBody, nil} {
		resetForTest(Declarations{ResponseFields: []string{"session_token"}})
		tr := roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: body, Request: r,
				Trailer: http.Header{"Set-Cookie": {"sid=trailer-cookie-77"}, "Session_token": {"trailer-field-88"}}}, nil
		})
		resp, err := (&http.Client{Transport: Transport(tr)}).Get("https://api.example.com/")
		if err != nil {
			t.Fatal(err)
		}
		if resp.Body != nil {
			io.ReadAll(resp.Body)
			resp.Body.Close()
		}
		for _, v := range []string{"trailer-cookie-77", "trailer-field-88"} {
			if !withheld([]byte(v)) {
				t.Errorf("body %T: a trailer credential was not held: %s", body, v)
			}
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Class child-envelope-unchecked (B): the child's argv and environment are
// checked as the child gets them, after everything os/exec adds: on
// Windows, SYSTEMROOT and the composed command line.
func TestKit3ChildEnvelopeIsWhatTheChildGets(t *testing.T) {
	resetForTest(Declarations{})
	t.Setenv("SYSTEMROOT", "sysroot-held-4321")
	Hold("minted:x", "sysroot-held-4321")
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	argv, env := childEnvelope(cmd, []string{}, "windows")
	if !withheld([]byte(strings.Join(argv, "\n") + "\n" + strings.Join(env, "\n"))) {
		t.Errorf("Windows: SYSTEMROOT, which os/exec adds, is not in the checked envelope: %q", env)
	}
	resetForTest(Declarations{})
	Hold("minted:x", `"two words"`)
	cmd = exec.Command(os.Args[0], "two words")
	argv, env = childEnvelope(cmd, []string{}, "windows")
	if !withheld([]byte(strings.Join(argv, "\n") + "\n" + strings.Join(env, "\n"))) {
		t.Errorf("Windows: the command line the child gets (arguments quoted) is not in the checked envelope: %q", argv)
	}
}

// Class mcp-error-bypasses-guard (B and C): every byte of an MCP HTTP
// response passes the final check as one frame with its status line,
// headers and trailers; the kit's own replacement included.
func TestKit3MCPHTTPEveryFrameChecked(t *testing.T) {
	raw := func(h http.Handler, req string, wrap bool) string {
		t.Helper()
		hh := MCPHTTP(h)
		if wrap {
			inner := hh
			hh = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { inner.ServeHTTP(noHijack{w}, r) })
		}
		srv := httptest.NewServer(hh)
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
	body := `{"jsonrpc":"2.0","id":1,"method":"ping"}`
	post := "POST / HTTP/1.1\r\nHost: x\r\nContent-Type: application/json\r\nConnection: close\r\nContent-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n" + body
	badChunk := "POST / HTTP/1.1\r\nHost: x\r\nTransfer-Encoding: chunked\r\nContent-Type: application/json\r\nConnection: close\r\n\r\nz\r\n"
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
	})

	// A held value a piece of which is in the kit's own notice: the
	// replacement header (and the replacement body) would carry it.
	resetForTest(Declarations{})
	Hold("minted:x", "xxwithheld")
	leaky := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Debug", "xxwithheld")
		ok(w, r)
	})
	if got := raw(leaky, post, false); strings.Contains(got, "withheld") {
		t.Errorf("the kit's replacement carried a piece of a held value: %q", got)
	}

	// A held value a piece of which is in a status line: the non-Hijacker
	// fallback, and a stream's error status.
	resetForTest(Declarations{})
	Hold("minted:x", "400 Bad Requestor")
	if got := raw(ok, badChunk, true); strings.Contains(got, "Bad Requ") {
		t.Errorf("non-Hijacker fallback: a status line carried a held piece: %q", got)
	}
	sse400 := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(400)
		w.Write([]byte("event: message\ndata: {}\n\n"))
	})
	if got := raw(sse400, post, false); strings.Contains(got, "Bad Requ") {
		t.Errorf("a stream's status line carried a held piece: %q", got)
	}

	// A trailer set after the stream was flushed.
	resetForTest(Declarations{})
	Hold("minted:x", "secret!!")
	sseTrailer := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte("event: message\ndata: {}\n\n"))
		w.(http.Flusher).Flush()
		w.Header().Set(http.TrailerPrefix+"X-T", "secret!!")
	})
	if got := raw(sseTrailer, post, false); strings.Contains(got, "secret!!") {
		t.Errorf("a trailer set after a flush carried a held value: %q", got)
	}

	// A held piece in every status line: no frame can pass, so the session
	// fails closed and nothing of the response is written.
	resetForTest(Declarations{})
	Hold("minted:x", "xHTTP/1.1")
	if got := raw(ok, post, false); strings.Contains(got, "HTTP/1.1") {
		t.Errorf("a status line carried a held piece: %q", got)
	}
}

// noHijack hides the Hijacker of the ResponseWriter it wraps.
type noHijack struct{ w http.ResponseWriter }

func (n noHijack) Header() http.Header         { return n.w.Header() }
func (n noHijack) Write(b []byte) (int, error) { return n.w.Write(b) }
func (n noHijack) WriteHeader(c int)           { n.w.WriteHeader(c) }
func (n noHijack) Flush()                      { n.w.(http.Flusher).Flush() }

var _ = url.Parse

// B, one exit point per output path. Every call in the kit's own files
// that can put bytes out (a write, a header or trailer set, a status
// write, a flush, a connection taken over, a request sent, a child
// started, a statement run) is one of these, each the exit point of its
// path, after the final check, or named for why it is not output. A new
// one fails the test until it is reviewed and named here; one named here
// that is gone fails it too, so the list stays the code.
var kitExits = map[string]string{
	"stdio.go pump.write p.out.Write":                  "stdout and stderr: the unit, or its notice, after the check",
	"mcp.go frameWriter.emit f.w.Write":                "MCP stdio: the frame, or its replacement, after the check",
	"mcp.go mcpResponse.out m.bw.Write":                "MCP HTTP, connection taken over: the checked unit",
	"mcp.go mcpResponse.out m.bw.Flush":                "MCP HTTP, connection taken over: the checked unit",
	"mcp.go mcpResponse.out maps.Copy":                 "MCP HTTP through net/http: the checked head's header",
	"mcp.go mcpResponse.out h.Set":                     "MCP HTTP through net/http: Transfer-Encoding identity, which net/http removes",
	"mcp.go mcpResponse.out h[]=":                      "MCP HTTP through net/http: Date nil, so net/http writes none",
	"mcp.go mcpResponse.out m.w.WriteHeader":           "MCP HTTP through net/http: the checked status",
	"mcp.go mcpResponse.out m.w.Write":                 "MCP HTTP through net/http: the checked body",
	"mcp.go mcpResponse.out fl.Flush":                  "MCP HTTP through net/http: after the checked unit",
	"mcp.go mcpResponse.takeOver hj.Hijack":            "MCP HTTP: takes the connection over, writes nothing",
	"mcp.go mcpResponse.frame f.h.Set":                 "MCP HTTP: builds a unit's header, checked in send before out",
	"mcp.go mcpResponse.frame f.h[]=":                  "MCP HTTP: builds a unit's header, checked in send before out",
	"files.go WriteFile f.Write":                       "files: the record after the check, under the file's locks",
	"stdio_unix.go appendAt c.Write":                   "files: Writer.put's append, or its notice, by offset at the end it was checked for",
	"stdio_unix.go appendAt f.Write":                   "files: Writer.put's append, or its notice, at the file's end, by the kernel",
	"stdio_windows.go appendAt f.Write":                "files: Writer.put's append, or its notice, at the end it was checked for",
	"stdio_other.go appendAt f.Write":                  "files: Writer.put's append, or its notice, at the end it was checked for",
	"files.go Writer.put w.f.WriteAt":                  "files: a write, or its notice, at the offset it was checked for",
	"files.go Writer.writeStream w.w.Write":            "streams that are not regular files: the record, or its notice, after the check",
	"files.go osAppends f.WriteAt":                     "a zero-length probe for O_APPEND: writes nothing",
	"files.go AppendFile w.Write":                      "files: into a Writer",
	"sql.go sqlStmt.Exec s.Stmt.Exec":                  "SQLite: the statement and its values after the check",
	"sql.go sqlStmt.ExecContext s.Stmt.Exec":           "SQLite: the statement and its values after the check",
	"sql.go sqlStmt.ExecContext e.ExecContext":         "SQLite: the statement and its values after the check",
	"sql.go sqlConn.ExecContext e.ExecContext":         "SQLite: a direct statement and its values after the store rule",
	"exec.go StartCommand cmd.Start":                   "child processes: argv and the final environment after the check",
	"exec.go StartCommand json.NewEncoder().Encode":    "child processes: the held set the child must get (contract 1)",
	"transport.go transport.RoundTrip inner.RoundTrip": "outbound requests: the request after the given check, through the wire-checking transport",
	"transport.go checkConn.Write c.Conn.Write":        "outbound requests: the head after the wire check, then the already-checked body",
	"transport.go render r.Write":                      "outbound requests: renders the request into memory for the check, puts nothing out",
	"transport.go render r.WriteProxy":                 "outbound requests: renders the proxy's request into memory for the check, puts nothing out",
	"transport.go render extra[]=":                     "outbound requests: builds the lines the transport adds, in memory",
	"transport.go proxyUnit connect.Write":             "outbound requests: renders the CONNECT into memory for the check, puts nothing out",
	"transport.go proxyUnit h[]=":                      "outbound requests: builds the rendered CONNECT's header, in memory",
	"public.go refuse os.Stderr.WriteString":           "into the stderr pump, checked there",
	"public.go recordLocked os.Stderr.WriteString":     "into the stderr pump, checked there",
	"mcp.go MCPIn pw.Write":                            "input: the client's frame to the server, not output",
	"start.go readStdin w.Write":                       "input: stdin given back to the program, not output",
	"response.go fieldBody h.Del":                      "input: a response the program reads, not output",
	"acquire.go canonicalMultipart pw.Write":           "input: re-serializes a request body into memory to compare it, puts nothing out",
}

// TestKit3OneExitPoint fails on any output call that goes around the exit
// point of its path.
func TestKit3OneExitPoint(t *testing.T) {
	found := kitOutputCalls(t)
	for _, k := range found {
		if _, ok := kitExits[k]; !ok {
			t.Errorf("an output call outside the exit points: %s", k)
		}
	}
	for k := range kitExits {
		if !slices.Contains(found, k) {
			t.Errorf("a named exit point is gone: %s", k)
		}
	}
}

// kitOutputCalls is every call in the kit's own (non-test) files that can
// put bytes out, named "file func receiver.call" (no line numbers). The
// files of this platform are type-checked: a write whose destination is
// memory (a bytes.Buffer or strings.Builder), a counter's Add, and a
// method of a type that is not a header and puts nothing out are not
// output calls. The files of other platforms are read by name alone.
func kitOutputCalls(t *testing.T) []string {
	t.Helper()
	names := map[string]bool{"Write": true, "WriteAt": true, "WriteProxy": true, "WriteString": true, "WriteByte": true, "WriteRune": true,
		"WriteHeader": true, "Flush": true, "Hijack": true, "WriteTo": true, "ReadFrom": true, "Encode": true,
		"RoundTrip": true, "Exec": true, "ExecContext": true, "Start": true, "Run": true, "Output": true,
		"CombinedOutput": true, "Set": true, "Add": true, "Del": true}
	funcs := map[string]bool{"fmt.Fprint": true, "fmt.Fprintf": true, "fmt.Fprintln": true, "io.Copy": true,
		"io.CopyN": true, "io.CopyBuffer": true, "io.WriteString": true, "os.WriteFile": true,
		"syscall.Write": true, "maps.Copy": true}
	fs := token.NewFileSet()
	files, _ := filepath.Glob("*.go")
	var here []*ast.File
	var other []*ast.File
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		af, err := parser.ParseFile(fs, f, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		if ok, _ := build.Default.MatchFile(".", f); ok {
			here = append(here, af)
		} else {
			other = append(other, af)
		}
	}
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}}
	conf := types.Config{Importer: importer.ForCompiler(fs, "source", nil)}
	if _, err := conf.Check("credkit", fs, here, info); err != nil {
		t.Fatal(err)
	}
	typ := func(e ast.Expr) string {
		if tv, ok := info.Types[e]; ok && tv.Type != nil {
			return tv.Type.String()
		}
		return ""
	}
	memory := func(e ast.Expr) bool {
		s := typ(e)
		return s == "*bytes.Buffer" || s == "*strings.Builder" || s == "bytes.Buffer" || s == "strings.Builder"
	}
	header := func(e ast.Expr) bool { return typ(e) == "net/http.Header" }
	var out []string
	for _, af := range append(here, other...) {
		typed := slices.Contains(here, af)
		f := filepath.Base(fs.Position(af.Pos()).Filename)
		for _, d := range af.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			fn := fd.Name.Name
			if fd.Recv != nil {
				fn = astText(fd.Recv.List[0].Type) + "." + fn
			}
			add := func(s string) { out = append(out, f+" "+fn+" "+s) }
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.CallExpr:
					s, ok := x.Fun.(*ast.SelectorExpr)
					if !ok {
						return true
					}
					if name := astText(s); funcs[name] {
						switch {
						case !typed || len(x.Args) == 0:
							add(name)
						case name == "maps.Copy":
							if header(x.Args[0]) {
								add(name)
							}
						case !memory(x.Args[0]):
							add(name)
						}
						return true
					}
					if !names[s.Sel.Name] {
						return true
					}
					switch {
					case !typed:
						add(astText(s))
					case header(s.X) && (s.Sel.Name == "Write" || s.Sel.Name == "WriteSubset"):
						if !memory(x.Args[0]) {
							add(astText(s))
						}
					case s.Sel.Name == "Set" || s.Sel.Name == "Add" || s.Sel.Name == "Del":
						if header(s.X) {
							add(astText(s))
						}
					case memory(s.X), strings.HasPrefix(typ(s.X), "*sync"), strings.HasPrefix(typ(s.X), "sync"),
						typ(s.X) == "net/url.Values":
					default:
						add(astText(s))
					}
				case *ast.AssignStmt:
					for _, l := range x.Lhs {
						if ix, ok := l.(*ast.IndexExpr); ok && typed && header(ix.X) {
							add(astText(ix.X) + "[]=")
						}
					}
				}
				return true
			})
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func astText(e ast.Expr) string {
	switch x := e.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.StarExpr:
		return astText(x.X)
	case *ast.SelectorExpr:
		return astText(x.X) + "." + x.Sel.Name
	case *ast.CallExpr:
		return astText(x.Fun) + "()"
	case *ast.IndexExpr:
		return astText(x.X) + "[]"
	case *ast.ParenExpr:
		return astText(x.X)
	case *ast.TypeAssertExpr:
		return astText(x.X) + ".(T)"
	case *ast.CompositeLit:
		return "{}"
	}
	return fmt.Sprintf("%T", e)
}

// set lowers (or raises) a limit for one case and returns its restore.
func set(p *int, v int) func() {
	old := *p
	*p = v
	return func() { *p = old }
}

// A, the table: each limit, fed its limit plus one, withholds the unit (the
// request is refused, for a limit on reading input); under its limit the
// same input is checked clean. A limit added to limits.go without a case
// here fails the test.
func TestKit3LimitsWithhold(t *testing.T) {
	nest := func(n int, f func([]byte) []byte, b []byte) string {
		for range n {
			b = f(b)
		}
		return string(b)
	}
	b64 := func(b []byte) []byte { return []byte(base64.StdEncoding.EncodeToString(b)) }
	stored := func(b []byte) []byte { // one final stored raw deflate block
		return append([]byte{1, byte(len(b)), byte(len(b) >> 8), ^byte(len(b)), ^byte(len(b) >> 8)}, b...)
	}
	plain := []byte("plain words, nothing held here")
	cases := map[string]func(t *testing.T){
		"maxDepth": func(t *testing.T) {
			// The depth this unit's closure needs (D) is measured; at D
			// it is clean, and one level past the limit (limit D-1) it
			// is withheld as exhausted.
			holding(t, cred)
			m, _ := currentMatcher()
			u := []byte(nest(4, b64, plain))
			defer set(&maxDepth, 1)()
			for maxDepth < 64 && m.check(u) != clean {
				maxDepth++
			}
			if maxDepth < 5 || maxDepth >= 64 {
				t.Fatalf("four base64 levels closed at depth %d", maxDepth)
			}
			maxDepth--
			if v := m.check(u); v != exhausted {
				t.Errorf("one level past the depth limit: %v, want exhausted", v)
			}
		},
		"budget": func(t *testing.T) {
			holding(t, cred)
			u := nest(1, b64, plain)
			defer set(&budgetPerByte, 0)()
			defer set(&budgetBase, 1<<20)()
			if leaks(u) {
				t.Error("withheld under the budget")
			}
			budgetBase = 0 // one decoded byte past it
			if !leaks(u) {
				t.Error("checked clean past the budget")
			}
		},
		"deflBudget": func(t *testing.T) {
			holding(t, cred)
			d := df(bytes.Repeat(plain, 4))
			u := string(d[:len(d)-3]) // broken: what it yields before its error is read
			if leaks(u) {
				t.Error("a broken stream was withheld under its budget")
			}
			defer set(&deflPerByte, 0)()
			defer set(&deflBase, 0)()
			defer set(&deflMax, 0)()
			if !leaks(u) {
				t.Error("a broken stream past its budget was checked clean")
			}
		},
		"maxUnit": func(t *testing.T) {
			holding(t, cred)
			defer set(&maxUnit, 16)()
			if leaks(strings.Repeat("a", 16)) || !leaks(strings.Repeat("a", 17)) {
				t.Error("maxUnit: 16 bytes must be checked, 17 withheld")
			}
		},
		"maxRawNest": func(t *testing.T) {
			holding(t, cred)
			defer set(&maxDepth, 4*maxRawNest)()
			if leaks(nest(maxRawNest/2, stored, plain)) {
				t.Error("raw deflate streams nested under the limit were withheld")
			}
			if !leaks(nest(maxRawNest+1, stored, plain)) {
				t.Error("raw deflate streams nested past the limit were checked clean")
			}
		},
		"maxEncodedBody": func(t *testing.T) {
			body := gz([]byte(`{"token":"resp-token-4242","note":"` + string(plain) + `"}`))
			read := func() {
				resetForTest(Declarations{ResponseFields: []string{"token"}})
				tr := roundTripFunc(func(r *http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: 200, Request: r, Body: io.NopCloser(bytes.NewReader(body)),
						Header: http.Header{"Content-Type": {"application/json"}, "Content-Encoding": {"gzip"}}}, nil
				})
				resp, err := (&http.Client{Transport: Transport(tr)}).Get("https://api.example.com/")
				if err != nil {
					t.Fatal(err)
				}
				io.ReadAll(resp.Body)
				resp.Body.Close()
			}
			defer set(&maxEncodedBody, len(body))()
			if read(); leaks("clean") {
				t.Error("a body at the limit failed closed")
			}
			maxEncodedBody = len(body) - 1
			if read(); !leaks("clean") {
				t.Error("a body past the limit did not fail closed")
			}
		},
		"maxDeclared": func(t *testing.T) {
			// Loop ruling 8b refuses any coding on a request body, so the
			// decoding limit is reached through a response the kit decodes
			// to hold its declared fields.
			plain := []byte(`{"session_token":"declared-field-1","pad":"` + strings.Repeat("p", 64) + `"}`)
			read := func() (string, error) {
				resetForTest(Declarations{ResponseFields: []string{"session_token"}})
				rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
					h := http.Header{}
					h.Set("Content-Type", "application/json")
					h.Set("Content-Encoding", "gzip")
					return &http.Response{StatusCode: 200, Header: h, Body: io.NopCloser(bytes.NewReader(gz(plain))), Request: r}, nil
				})
				req, _ := http.NewRequest("GET", "https://api.example.com/s", nil)
				resp, err := Transport(rt).RoundTrip(req)
				if err != nil {
					return "", err
				}
				b, err := io.ReadAll(resp.Body)
				return string(b), err
			}
			defer set(&maxDeclared, len(plain))()
			if got, err := read(); got != string(plain) || err != nil || leaks("clean") {
				t.Errorf("a decoding at the limit failed closed: %q %v", got, err)
			}
			maxDeclared = len(plain) - 1
			if read(); !leaks("clean") {
				t.Error("a decoding past the limit did not fail closed")
			}
		},
		"streamTail": func(t *testing.T) {
			holding(t, cred)
			run := func() string {
				var out bytes.Buffer
				w := NewWriter(&out, SinkFile)
				w.Write([]byte(`{"a":"`))
				w.Write([]byte("0123456789abcdef"))
				w.Write([]byte(`"}`))
				return out.String()
			}
			if got := run(); strings.Contains(got, Notice(SinkFile)) {
				t.Errorf("withheld with the document in the context: %q", got)
			}
			defer set(&streamTailMin, 16)()
			defer set(&streamTailPer, 0)()
			defer set(&streamTailMax, 16)()
			// kit8: the stream is lost, so the write and its notice are
			// withheld (the notice cannot be checked with what was dropped).
			if got := run(); strings.HasSuffix(got, `"}`) {
				t.Errorf("the context past its limit, inside a document, did not withhold: %q", got)
			}
		},
	}
	for _, name := range limitNames {
		f, ok := cases[name]
		if !ok {
			t.Errorf("limit %s has no case", name)
			continue
		}
		t.Run(name, f)
	}
	if len(cases) != len(limitNames) {
		t.Errorf("cases %d, limits %d", len(cases), len(limitNames))
	}
}

// A, the mechanism that keeps every stop on the budget: in the matcher's
// files, a length compared with a constant other than an emptiness test
// is a limit from limits.go, or its line (or the line above) says why it
// is exact ("// exact: ..."); and no path stops clean. A new length gate
// fails here until it is a limit or shown exact.
func TestKit3StopsGoThroughLimits(t *testing.T) {
	fs := token.NewFileSet()
	lf, err := parser.ParseFile(fs, "limits.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	limits := map[string]bool{}
	for _, d := range lf.Decls {
		if g, ok := d.(*ast.GenDecl); ok {
			for _, sp := range g.Specs {
				if v, ok := sp.(*ast.ValueSpec); ok {
					for _, n := range v.Names {
						limits[n.Name] = true
					}
				}
			}
		}
	}
	for _, f := range []string{"match.go", "decode.go", "charset.go", "deflateprobe.go", "consttable.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(src), "\n")
		exact := func(l int) bool {
			return strings.Contains(lines[l-1], "// exact:") || l > 1 && strings.Contains(lines[l-2], "// exact:")
		}
		af, err := parser.ParseFile(fs, f, src, parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		hasLen := func(e ast.Expr) bool {
			found := false
			ast.Inspect(e, func(n ast.Node) bool {
				if c, ok := n.(*ast.CallExpr); ok {
					if id, ok := c.Fun.(*ast.Ident); ok && id.Name == "len" {
						found = true
					}
				}
				return true
			})
			return found
		}
		// constant reports whether e is a constant (literals, constants,
		// limits), and whether it names a limit; a length compared with a
		// position or another length is a bounds check, not a gate.
		constant := func(e ast.Expr) (isConst, limit bool) {
			isConst = true
			ast.Inspect(e, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.Ident:
					switch {
					case limits[x.Name]:
						limit = true
					case x.Obj == nil || x.Obj.Kind != ast.Con:
						isConst = false
					}
				case *ast.CallExpr, *ast.IndexExpr, *ast.SelectorExpr:
					isConst = false
				}
				return true
			})
			return
		}
		ast.Inspect(af, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.CallExpr:
				if s, ok := x.Fun.(*ast.SelectorExpr); ok && s.Sel.Name == "stop" && len(x.Args) == 1 && astText(x.Args[0]) == "clean" {
					t.Errorf("%s: a path stops clean", fs.Position(x.Pos()))
				}
			case *ast.BinaryExpr:
				var other ast.Expr
				switch {
				case x.Op != token.LSS && x.Op != token.LEQ && x.Op != token.GTR && x.Op != token.GEQ:
					return true
				case hasLen(x.X):
					other = x.Y
				case hasLen(x.Y):
					other = x.X
				default:
					return true
				}
				isConst, limit := constant(other)
				if !isConst && !limit {
					return true // a bounds check against a position or another length
				}
				lit, _ := other.(*ast.BasicLit)
				empty := lit != nil && (lit.Value == "0" || lit.Value == "1" && hasLen(x.X) && (x.Op == token.GEQ || x.Op == token.LSS))
				if limit || empty || exact(fs.Position(x.Pos()).Line) {
					return true
				}
				t.Errorf("%s: a length gate that is neither a limit in limits.go nor marked exact", fs.Position(x.Pos()))
			}
			return true
		})
	}
}
