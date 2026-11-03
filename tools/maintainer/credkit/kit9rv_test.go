// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// kit9rv: one test per round-2 class of the kit8rv/kit9rv review (Codex
// session 01a19d68, .review/kit9rv-codex-r2.md), each from Codex's own
// input, with a negative case where one applies.

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

// Class normalized-piece-relevance (contract 2(b), 3 Charsets): a charset
// reading keeps every character of every pattern the matcher holds, a
// piece normalized after it is cut included.
func TestKit9rvNormalizedPieceRelevance(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:session", "bcdefga\u0302\u0301hijkl")
	if !withheld([]byte("bcdefg\u00e2")) {
		t.Error("the normalized piece, as UTF-8, was clean")
	}
	if !withheld(mustHex(t, "620063006400650066006700e200")) { // Codex's input
		t.Error("the normalized piece, as UTF-16LE, was clean")
	}
	if withheld(utf16LE("bcdefg\u00e4")) {
		t.Error("over-withholding: a different last character was withheld")
	}
}

// Class mcp-error-bypasses-guard (contract 1 MCP arguments, 2(b), 4, 5):
// blunt rule, the class's third round. A request that expects 100-continue
// is refused (417) before its body is read, so net/http never writes the
// interim response: the arguments it carries are not yet held.
func TestKit9rvMCPExpectContinueRefused(t *testing.T) {
	resetForTest(Declarations{MCPArguments: []string{"token"}})
	ran := false
	srv := httptest.NewServer(MCPHTTP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ran = true
		io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":{}}`)
	})))
	defer srv.Close()
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"x","arguments":{"token":"xHTTP/1.1"}}}`
	got := rawExchange(t, srv, "POST / HTTP/1.1\r\nHost: x\r\nExpect: 100-continue\r\nContent-Type: application/json\r\n"+
		fmt.Sprintf("Content-Length: %d\r\nConnection: close\r\n\r\n", len(body)), body) // Codex's input
	if strings.Contains(got, "100 Continue") || !strings.HasPrefix(got, "HTTP/1.1 417") || ran {
		t.Errorf("a 100-continue request was not refused before its body: ran %v, %q", ran, got)
	}
	// Ordinary MCP requests carry no Expect and are served.
	resp, err := http.Post(srv.URL, "application/json", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !bytes.Contains(b, []byte(`"result"`)) {
		t.Errorf("over-withholding: an ordinary request failed: %d %q", resp.StatusCode, b)
	}
}

// Class sqlite-open-statements-unchecked (contract 2 SQLite unit, 4, 5):
// blunt rule, the class's second round. Every statement the driver can
// build from the connection name (a _pragma value, any other _ key in each
// spelling a driver writes it, the _txlock begin) is checked before Open.
func TestKit9rvSQLiteOpenDerivedStatements(t *testing.T) {
	for _, tc := range []struct{ held, name string }{
		{"pragma a", "file:round2?mode=memory&cache=shared&_pragma=application_id(17)"}, // Codex's input
		{"pragma b", "file:x.db?_busy_timeout=5000"},
		{"_timeout = 5", "file:x.db?_busy_timeout=5000"},
		{"begin im", "file:x.db?_txlock=immediate"},
		{"pragma f", "x.db?_pragma=foreign_keys(1)"},
		{"pragma f", "file:x.db?_pragma=%66oreign_keys(1)"},
	} {
		resetForTest(Declarations{})
		Hold("minted:session", tc.held)
		rec := &openRec{}
		sqlTestRuns++
		name := fmt.Sprintf("credkit-kit9rv-%d", sqlTestRuns)
		sql.Register(name, SQLDriver(rec))
		db, _ := sql.Open(name, tc.name)
		if err := db.Ping(); err == nil || rec.opened != 0 {
			t.Errorf("held %q, name %q: opened (err %v, opens %d)", tc.held, tc.name, err, rec.opened)
		}
		db.Close()
	}
	// The fleet's connection names open with 20 credentials held.
	resetForTest(Declarations{})
	Hold("bench", append(benchCreds(), nonASCIICred)...)
	rec := &openRec{}
	sqlTestRuns++
	name := fmt.Sprintf("credkit-kit9rv-%d", sqlTestRuns)
	sql.Register(name, SQLDriver(rec))
	for _, dsn := range []string{
		"file:/srv/jane/.cache/zammad/data.db?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=temp_store(MEMORY)&_pragma=mmap_size(268435456)&_pragma=synchronous(NORMAL)&_txlock=immediate",
		"/srv/jane/.cache/x.db?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on&_synchronous=NORMAL&_temp_store=MEMORY&_mmap_size=0",
	} {
		db, _ := sql.Open(name, dsn)
		if err := db.Ping(); err != nil {
			t.Errorf("over-withholding: %q: %v", dsn, err)
		}
		db.Close()
	}
}

// Class child-held-byte-loss (contract 1 child inheritance, 2(a)): the
// held set a child inherits keeps every value byte for byte, one that is
// not valid UTF-8 included.
func TestKit9rvChildKeepsHeldBytes(t *testing.T) {
	resetForTest(Declarations{})
	Hold("minted:session", "\xffsecret!", "plain-secret-1")
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(heldRecords())
	resetForTest(Declarations{})
	holdInherited(&buf)
	if !isHeld("\xffsecret!") || !isHeld("plain-secret-1") || reg.unknown {
		t.Errorf("the inherited set lost a value: %q (unknown %v)", heldValues(), reg.unknown)
	}
	if !withheld([]byte("\xffsecret!")) {
		t.Error("the child released the parent's credential")
	}
	// A record a child cannot read (no value) makes its set unknown.
	resetForTest(Declarations{})
	holdInherited(strings.NewReader(`[{"a":["x"]}]`))
	if !reg.unknown {
		t.Error("a record without a value was read as clean")
	}
}

// Class file-stream-identity-collision (contract 3 Streams, 5): on Windows
// a pipe or a device has no file identity (os.SameFile compares zero ids).
// kit9rv round 3 blunt rule: the kit follows one such stream, the first
// handle it wraps; any other handle, a second pipe's or a duplicate of the
// first's, and any pipe opened by path, is unknown and withholds.
func TestKit9rvWindowsPipesKeptApart(t *testing.T) {
	defer dropWindowsStreams()
	dropWindowsStreams()
	r1, f1, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r1.Close()
	defer f1.Close()
	r2, f2, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r2.Close()
	defer f2.Close()
	fi1, _ := f1.Stat()
	fi2, _ := f2.Stat()
	_, s1 := streamOfFile(fi1, streamID("windows", "", f1))
	_, s2 := streamOfFile(fi2, streamID("windows", "", f2))
	_, s1b := streamOfFile(fi1, streamID("windows", "", f1))
	_, p1 := streamOfFile(fi1, streamID("windows", `C:\pipe\a`, nil))
	if s1.lost || s1 != s1b || !s2.lost || !p1.lost {
		t.Errorf("windows streams: first lost %v, one handle split %v, second pipe followed %v, path followed %v", s1.lost, s1 != s1b, !s2.lost, !p1.lost)
	}
}

// dropWindowsStreams forgets the streams a test keyed the Windows way.
func dropWindowsStreams() {
	fileStreams.Lock()
	defer fileStreams.Unlock()
	fileStreams.list = slices.DeleteFunc(fileStreams.list, func(x fileStream) bool { return x.id != "" })
}

// rawExchange writes head, then body only when the server answers 100
// Continue first (as a client does), and returns everything it reads.
func rawExchange(t *testing.T, srv *httptest.Server, head, body string) string {
	t.Helper()
	c, err := net.Dial("tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(5 * time.Second))
	io.WriteString(c, head)
	br := bufio.NewReader(c)
	line, _ := br.ReadString('\n')
	if strings.Contains(line, " 100 ") {
		io.WriteString(c, body)
	}
	rest, _ := io.ReadAll(br)
	return line + string(rest)
}
