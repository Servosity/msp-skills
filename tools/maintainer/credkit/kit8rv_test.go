// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// kit8rv: one test per round-1 class of the kit8rv review (Codex session
// 01a19d68, .review/kit8rv-codex-r1.md), each from Codex's own input, with
// the other members of its class and a negative case where one applies.

import (
	"bufio"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
)

// Class normalization-before-piece-counting (contract 2(a), 2(b), 3
// Unicode normalization): a piece is 8 characters of the value as held,
// normalized after it is cut, and a value of 8 characters as held is not
// short because normalizing shortens it.
func TestKit8rvPiecesCountedAsHeld(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:session", "a\u0301bcdefghijkl")
	if !withheld([]byte("a\u0301bcdefg")) { // Codex's input
		t.Error("the first 8 characters of the held value, as written, were clean")
	}
	if !withheld([]byte("xx \u00e1bcdefg yy")) {
		t.Error("the same piece, precomposed, was clean")
	}
	if withheld([]byte("a\u0301bcdef!")) {
		t.Error("over-withholding: 7 characters of the held value were withheld")
	}
	resetForTest(Declarations{})
	Hold("minted:session", "a\u0301bcdefg")
	if !withheld([]byte("a\u0301bcdefgx")) { // Codex's whole-value input
		t.Error("an 8-character held value inside a longer word was clean")
	}
}

// Class decoder-output-kind-dedupe (contract 3 Base64, ninth reading
// bound): an output queued by one decoder is not skipped when another
// decoder queues the same bytes to be read another way.
func TestKit8rvDedupeKeepsTheReadingKind(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:session", "abc")
	if !withheld([]byte("\x00%61bcx\x00YWJjeA==\x00")) { // Codex's input
		t.Error("base64 output seen first as a percent decoding was not read with a short value anywhere")
	}
	if withheld([]byte("\x00%61bcx\x00")) {
		t.Error("over-withholding: abcx from percent alone is no whole token of abc")
	}
}

func utf32BE(s string) []byte {
	var b []byte
	for _, r := range s {
		b = append(b, byte(r>>24), byte(r>>16), byte(r>>8), byte(r))
	}
	return b
}

// Class joined-values-incomplete (contract 3 Charsets, Joined values): a
// charset reading is joined whole, a character no held value has included,
// before it is cut to its relevant runs.
func TestKit8rvCharsetReadingJoinedWhole(t *testing.T) {
	doc := `{"☃":0,"a":"secr","b":"et!!"}`
	cp037, err := charmap.CodePage037.NewEncoder().Bytes([]byte(`{"é":0,"a":"secr","b":"et!!"}`))
	if err != nil {
		t.Fatal(err)
	}
	sjis, err := japanese.ShiftJIS.NewEncoder().Bytes([]byte(`{"あ":0,"a":"secr","b":"et!!"}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		unit []byte
	}{
		{"utf16le (Codex)", mustHex(t, "7b002200032622003a0030002c002200610022003a002200730065006300720022002c002200620022003a002200650074002100210022007d00")},
		{"utf16le built", utf16LE(doc)},
		{"utf16le at an odd offset", append([]byte{'x'}, utf16LE(doc)...)},
		{"utf32be", utf32BE(doc)},
		{"cp037", cp037},
		{"shift_jis", sjis},
	} {
		resetForTest(Declarations{})
		Hold("minted:session", "secret!!")
		if !withheld(tc.unit) {
			t.Errorf("%s: a document whose values join to the credential was clean", tc.name)
		}
	}
	resetForTest(Declarations{})
	Hold("minted:session", "secret!!")
	if withheld(utf16LE(`{"☃":0,"a":"secr","b":"ab!!"}`)) {
		t.Error("over-withholding: a document whose values join to no held value was withheld")
	}
}

// Class text-view-recursion-gap (contract 3 ROT13, reading bounds): ROT13
// reads every text view, a charset reading queued as runs included.
func TestKit8rvROT13ReadsCharsetReadings(t *testing.T) {
	t.Skip("out of scope by the reading bound rot13WholeText (contract 3, Reading bounds; owner order 2026-11-02): ROT13 of a charset reading's runs")
	resetForTest(Declarations{})
	Hold("minted:session", "junkjunk")
	if !withheld(mustHex(t, "6cf6956cf7f56cf6996cf6966cf6956cf7f56cf6996cf696")) { // Codex's input
		t.Error("CP037, then ROT13, then percent was clean")
	}
}

// Class file-stream-state-reset (contract 3 Streams, 5): every Writer of
// one pipe or FIFO, and every AppendFile to a FIFO, continues one stream.
func TestKit8rvPipeAndFIFOShareOneStream(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:session", "secret!!")
	r, f, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan string)
	go func() { b, _ := io.ReadAll(r); got <- string(b) }()
	NewWriter(f, SinkFile).Write([]byte("secr")) // Codex's input
	NewWriter(f, SinkFile).Write([]byte("et!!"))
	f.Close()
	if s := <-got; strings.Contains(s, "secret!!") {
		t.Errorf("two Writers of one pipe completed the credential: %q", s)
	}
	fifoAppends(t)
}

// Class mcp-error-bypasses-guard (contract 2(b), 4, 5): net/http's
// interim 100 Continue response is checked before the body is read.
func TestKit8rvMCPContinueIsChecked(t *testing.T) {
	ran := false
	srv := httptest.NewServer(MCPHTTP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ran = true
		io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{}}`)
	})))
	defer srv.Close()
	send := func() string {
		c, err := net.Dial("tcp", srv.Listener.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		c.SetDeadline(time.Now().Add(5 * time.Second))
		io.WriteString(c, "POST / HTTP/1.1\r\nHost: x\r\nExpect: 100-continue\r\nContent-Type: application/json\r\nContent-Length: 2\r\nConnection: close\r\n\r\n")
		br := bufio.NewReader(c)
		var got strings.Builder
		line, _ := br.ReadString('\n')
		got.WriteString(line)
		if strings.Contains(line, "100") { // the client sends the body after the interim response
			io.WriteString(c, "{}")
		}
		rest, _ := io.ReadAll(br)
		got.Write(rest)
		return got.String()
	}
	resetForTest(Declarations{})
	Hold("minted:session", "xHTTP/1.1") // Codex's input
	if got := send(); strings.Contains(got, "HTTP/1.1") {
		t.Errorf("the client received a held piece: %q", got)
	}
	if ran {
		t.Error("the handler ran on a request whose interim response was withheld")
	}
	resetForTest(Declarations{})
	Hold("minted:session", "secret!!")
	// kit9rv blunt rule: every request that expects 100-continue is
	// refused before its body is read (TestKit9rvMCPExpectContinueRefused).
	if got := send(); strings.Contains(got, "100 Continue") || !strings.HasPrefix(got, "HTTP/1.1 417") {
		t.Errorf("a 100-continue request was not refused: %q", got)
	}
}

// Classes config-reader-drops-values and response-reader-drops-values
// (contract 1): a comment after a tab, or after a space in a lenient
// response body, is not part of the value.
func TestKit8rvTrailingCommentAfterTab(t *testing.T) {
	for _, tc := range []struct{ file, content string }{
		{"c.yaml", "token: 1234567\t# note\n"}, // Codex's input
		{"c.toml", "token = 1234567\t# note\n"},
		{"c.ini", "token = 1234567\t# note\n"},
	} {
		gatherString(t, Declarations{ConfigFields: []string{"token"}}, tc.file, tc.content)
		if !isHeld("1234567") {
			t.Errorf("%s %q: the credential before the comment was not held: %q", tc.file, tc.content, heldValues())
		}
	}
	for _, body := range []string{"token: 1234567 # note\n", "token: 1234567\t# note\n"} { // Codex's, and a tab
		resetForTest(Declarations{ResponseFields: []string{"token"}})
		io.ReadAll(newFieldReader(strings.NewReader(body), []string{"token"}, modeFor("text/yaml")))
		if !isHeld("1234567") {
			t.Errorf("response %q: the credential before the comment was not held: %q", body, heldValues())
		}
	}
}

// Class basic-auth-byte-parts-unheld (contract 1, Basic auth parts; 2: a
// character is a byte for a value that is not valid UTF-8).
func TestKit8rvBasicPairThatIsNotUTF8(t *testing.T) {
	for _, h := range []http.Header{
		{"authorization": {"Basic dTr/"}}, // Codex's input
		{"Authorization": {"Basic dTr/"}},
	} {
		resetForTest(Declarations{})
		reg.Lock()
		holdHeaderLocked(h)
		reg.Unlock()
		if !isHeld("\xff") || !isHeld("u:\xff") {
			t.Errorf("%v: the Basic pair's parts were not held: %q", h, heldValues())
		}
	}
}

type openRec struct {
	recDriver
	opened int
}

func (d *openRec) Open(n string) (driver.Conn, error) { d.opened++; return d.recDriver.Open(n) }

// Classes sqlite-open-statements-unchecked and sqlite-bound-value-mismatch
// (contract 2 SQLite unit, 4, 5): the connection name is checked before
// the driver opens it, and a time value is checked in the spelling the
// driver stores, its zone's name included.
func TestKit8rvSQLiteOpenNameAndTimeValue(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:session", "12345678")
	rec := &openRec{}
	sqlTestRuns++
	name := fmt.Sprintf("credkit-kit8rv-%d", sqlTestRuns)
	sql.Register(name, SQLDriver(rec))
	db, err := sql.Open(name, "file:review?mode=memory&cache=shared&_pragma=application_id(12345678)") // Codex's input
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Ping(); err == nil || rec.opened != 0 {
		t.Errorf("a connection name carrying a credential was opened: err %v, opens %d", err, rec.opened)
	}
	db.Close()

	resetForTest(Declarations{})
	Hold("minted:session", "secret!!")
	db, err = sql.Open(name, "file:clean")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.FixedZone("secret!!", 0)) // Codex's input
	if _, err := db.Exec("INSERT INTO t VALUES (?)", ts); err == nil {
		t.Errorf("a time whose zone name is the credential reached the driver (it stores %q)", ts.String())
	}
	if _, err := db.Exec("INSERT INTO t VALUES (?)", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Errorf("over-withholding: an ordinary time was withheld: %v", err)
	}
}
