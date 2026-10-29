// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// --- files (contract 5: every file the program writes) ---

func TestContract5FileWholeWriteWithheldKeepsPriorContent(t *testing.T) {
	holding(t, cred)
	p := filepath.Join(t.TempDir(), "profiles.json")
	os.WriteFile(p, []byte(`{"profiles":["work"]}`), 0o600)
	err := WriteFile(p, []byte(`{"profiles":["work"],"token":"`+cred+`"}`), 0o600)
	if !errors.Is(err, ErrWithheld) {
		t.Fatalf("file write carrying a credential not withheld: %v", err)
	}
	if b, _ := os.ReadFile(p); string(b) != `{"profiles":["work"]}` {
		t.Errorf("a withheld write changed the file: %q", b)
	}
	if strings.Contains(err.Error(), cred) || !strings.Contains(err.Error(), Notice(SinkFile)) {
		t.Errorf("the error is not the fixed notice: %q", err)
	}
	if err := WriteFile(p, []byte(`{"profiles":["home"]}`), 0o600); err != nil {
		t.Errorf("clean write refused: %v", err)
	}
}

func TestContract4StreamedRecordReplacedByNotice(t *testing.T) {
	holding(t, cred)
	p := filepath.Join(t.TempDir(), "journal.jsonl")
	for _, rec := range []string{`{"op":"list"}` + "\n", `{"op":"login","tok":"` + cred + `"}` + "\n", `{"op":"get"}` + "\n"} {
		if err := AppendFile(p, []byte(rec), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	b, _ := os.ReadFile(p)
	want := `{"op":"list"}` + "\n" + Notice(SinkFile) + "\n" + `{"op":"get"}` + "\n"
	if string(b) != want {
		t.Errorf("journal = %q, want %q", b, want)
	}
}

func TestContract2StateAcrossWrites(t *testing.T) {
	holding(t, cred)
	var out bytes.Buffer
	w := NewWriter(&out, SinkFile)
	w.Write([]byte("start " + cred[:5]))
	w.Write([]byte(cred[5:] + " end\n"))
	if strings.Contains(out.String(), cred[5:]) {
		t.Errorf("a credential split across two writes was not caught in the second: %q", out.String())
	}
	enc := encodings(cred)["base64 padded"]
	out.Reset()
	w = NewWriter(&out, SinkFile)
	w.Write([]byte(enc[:6]))
	w.Write([]byte(enc[6:]))
	if strings.Contains(out.String(), enc[6:]) {
		t.Errorf("an encoded credential split across writes was not caught: %q", out.String())
	}
}

func TestContract4NoticeDroppedWhenItMatches(t *testing.T) {
	// A value that is itself public text (the notice) is not held when
	// acquired while running (contract 2, public text); one that only
	// overlaps it is, and its piece is in the notice.
	holding(t, cred, "xwithheld from")
	var out bytes.Buffer
	NewWriter(&out, SinkFile).Write([]byte(cred + "\n"))
	if out.Len() != 0 {
		t.Errorf("a notice that matches a held value was written: %q", out.String())
	}
}

// --- SQLite (contract 2 unit: statement text and bound values) ---

type recDriver struct {
	mu    sync.Mutex
	stmts []string
}

func (d *recDriver) Open(string) (driver.Conn, error) { return &recConn{d}, nil }

type recConn struct{ d *recDriver }

func (c *recConn) Prepare(q string) (driver.Stmt, error) { return &recStmt{c.d, q}, nil }
func (c *recConn) Close() error                          { return nil }
func (c *recConn) Begin() (driver.Tx, error)             { return recTx{}, nil }

// ExecContext is offered so the test proves the wrapper hides it.
func (c *recConn) ExecContext(_ context.Context, q string, _ []driver.NamedValue) (driver.Result, error) {
	c.d.mu.Lock()
	c.d.stmts = append(c.d.stmts, "DIRECT "+q)
	c.d.mu.Unlock()
	return driver.RowsAffected(1), nil
}

type recTx struct{}

func (recTx) Commit() error   { return nil }
func (recTx) Rollback() error { return nil }

type recStmt struct {
	d *recDriver
	q string
}

func (s *recStmt) Close() error  { return nil }
func (s *recStmt) NumInput() int { return -1 }
func (s *recStmt) Exec(args []driver.Value) (driver.Result, error) {
	s.d.mu.Lock()
	s.d.stmts = append(s.d.stmts, s.q)
	s.d.mu.Unlock()
	return driver.RowsAffected(1), nil
}
func (s *recStmt) Query([]driver.Value) (driver.Rows, error) { return nil, errors.New("no rows") }

var sqlTestRuns int

func TestContract5SQLiteStatementAndBoundValues(t *testing.T) {
	holding(t, cred)
	rec := &recDriver{}
	// A name of its own per run: database/sql panics on a second Register
	// of one name, and -count runs this test again in the same process.
	sqlTestRuns++
	name := fmt.Sprintf("credkit-test-%d", sqlTestRuns)
	sql.Register(name, SQLDriver(rec))
	db, err := sql.Open(name, "x")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("INSERT INTO t VALUES (?)", "clean"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO t VALUES (?)", "x"+cred); !errors.Is(err, ErrWithheld) {
		t.Errorf("bound value carrying a credential not withheld: %v", err)
	}
	if _, err := db.Exec("INSERT INTO t VALUES ('" + cred + "')"); !errors.Is(err, ErrWithheld) {
		t.Errorf("statement text carrying a credential not withheld: %v", err)
	}
	if _, err := db.Exec("INSERT INTO t VALUES (?, ?)", cred[:9], cred[9:]); err == nil {
		// Two bound values that join to the credential: each line is a
		// piece of 9 and 10 characters, so each alone is a leak.
		t.Error("a credential split across bound values not withheld")
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.stmts) != 1 || strings.HasPrefix(rec.stmts[0], "DIRECT") {
		t.Errorf("statements reaching the driver = %q, want only the clean one, prepared", rec.stmts)
	}
}

// --- MCP (contract 5: results, errors, protocol frames, logs) ---

func TestContract5MCPToolResultAndArguments(t *testing.T) {
	resetForTest(Declarations{MCPArguments: []string{"api_key"}})
	echo := MCPTools(func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultText("you sent " + req.GetArguments()["api_key"].(string)), nil
	})
	var req mcp.CallToolRequest
	req.Params.Arguments = map[string]any{"api_key": "mcp-arg-secret-1", "limit": 5}
	res, err := echo(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(res)
	if strings.Contains(string(b), "mcp-arg-secret-1") || !strings.Contains(string(b), Notice(SinkMCP)) || !res.IsError {
		t.Errorf("an MCP result echoing a credential argument was not replaced: %s", b)
	}
	if isHeld("5") {
		t.Error("an undeclared argument was held")
	}
	failing := MCPTools(func(context.Context, mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return nil, errors.New("vendor said: bad key mcp-arg-secret-1")
	})
	if _, err := failing(context.Background(), req); !errors.Is(err, ErrWithheld) {
		t.Errorf("an MCP error carrying a credential not withheld: %v", err)
	}
}

func TestContract5MCPStdioFrames(t *testing.T) {
	resetForTest(Declarations{MCPArguments: []string{"api_key"}})
	in := MCPIn(strings.NewReader(`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"x","arguments":{"api_key":"frame-arg-secret-2"}}}` + "\n"))
	line, _ := bufio.NewReader(in).ReadString('\n')
	if !strings.Contains(line, "tools/call") {
		t.Fatalf("MCPIn changed the frame: %q", line)
	}
	if !isHeld("frame-arg-secret-2") {
		t.Fatal("an argument was not held before the server read the frame")
	}
	var out bytes.Buffer
	fw := &frameWriter{w: &out}
	fw.Write([]byte(`{"jsonrpc":"2.0","id":7,"result":{"content":[{"type":"text","text":"frame-arg-secret-2"}]}}` + "\n"))
	fw.Write([]byte(`{"jsonrpc":"2.0","method":"notifications/message","params":{"data":"log frame-arg-secret-2"}}` + "\n"))
	fw.Write([]byte(`{"jsonrpc":"2.0","id":8,"result":{}}` + "\n"))
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 || strings.Contains(out.String(), "frame-arg-secret-2") {
		t.Fatalf("frames = %q", out.String())
	}
	var errFrame struct {
		ID    int `json:"id"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(lines[0]), &errFrame) != nil || errFrame.ID != 7 || errFrame.Error.Message != Notice(SinkMCP) {
		t.Errorf("withheld result frame = %q, want an error for id 7 with the notice", lines[0])
	}
	if !strings.Contains(lines[1], "notifications/message") || !strings.Contains(lines[1], Notice(SinkMCP)) {
		t.Errorf("withheld log frame = %q", lines[1])
	}
	if lines[2] != `{"jsonrpc":"2.0","id":8,"result":{}}` {
		t.Errorf("clean frame changed: %q", lines[2])
	}
}

func TestContract5MCPHTTP(t *testing.T) {
	resetForTest(Declarations{MCPArguments: []string{"api_key"}})
	h := MCPHTTP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(r.URL.Path, "sse") {
			w.Header().Set("Content-Type", "text/event-stream")
			io.WriteString(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{}}\n\n")
			w.(http.Flusher).Flush()
			io.WriteString(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":2,\"result\":\""+string(bytes.TrimSpace(body))+"\"}\n\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", "999")
		io.WriteString(w, `{"jsonrpc":"2.0","id":3,"result":{"echo":`+string(body)+`}}`)
	}))
	srv := httptest.NewServer(h)
	defer srv.Close()
	call := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"arguments":{"api_key":"http-arg-secret-3"}}}`
	resp, err := http.Post(srv.URL+"/mcp", "application/json", strings.NewReader(call))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if strings.Contains(string(b), "http-arg-secret-3") || !strings.Contains(string(b), `"id":3`) || !strings.Contains(string(b), Notice(SinkMCP)) {
		t.Errorf("HTTP JSON message = %q", b)
	}
	resp, err = http.Post(srv.URL+"/sse", "application/json", strings.NewReader("http-arg-secret-3"))
	if err != nil {
		t.Fatal(err)
	}
	b, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if strings.Contains(string(b), "http-arg-secret-3") || !strings.Contains(string(b), `"id":1,"result":{}`) || !strings.Contains(string(b), `"id":2,"error"`) {
		t.Errorf("SSE events = %q", b)
	}
}

// --- child processes (contracts 1 and 5) ---

func TestContract5ChildArgvAndEnv(t *testing.T) {
	d := Declarations{SecretEnv: []string{"CK_TOKEN"}}
	resetForTest(d)
	t.Setenv("CK_TOKEN", cred)
	Hold("env:CK_TOKEN", cred)
	if err := StartCommand(exec.Command(os.Args[0], cred)); !errors.Is(err, ErrWithheld) {
		t.Errorf("a child whose argv carries a credential was started: %v", err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = []string{"X=" + cred}
	if err := StartCommand(cmd); !errors.Is(err, ErrWithheld) {
		t.Errorf("a child whose environment carries a credential was started: %v", err)
	}
	for _, kv := range childEnv() {
		if strings.HasPrefix(kv, "CK_TOKEN=") {
			t.Error("the declared secret variable was passed to a child")
		}
	}
}

func TestContract1ChildGetsTheHeldSet(t *testing.T) {
	if os.Getenv(childModeEnv) != "" {
		t.Skip()
	}
	// The parent runs in a process of its own (heldParent): the held set
	// is process-wide, and in the test process any other test's goroutine
	// that is still running can change it between this test's reset and
	// the check of the child's environment (kit5rv recorded one refusal
	// under -race that 40 reruns did not repeat).
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), childModeEnv+"=held-parent")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("%v\n%s%s", err, out.String(), errb.String())
	}
	if got := strings.TrimSpace(out.String()); got != "held=true allowed=https://api.example.com" {
		t.Errorf("child reported %q", got)
	}
}

// heldParent runs in a process of its own: it holds a value and starts a
// child built on the kit, which reports what it holds. A refusal reports
// what the parent held and which argv or environment entry matched (by
// name only; the environment is the machine's).
func heldParent() {
	resetForTest(Declarations{Origins: []Origin{{Source: "*", Allowed: []string{"https://api.example.com"}}}})
	Hold("minted:x", "inherited-secret-99")
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(childEnv(), childModeEnv+"=held")
	cmd.Stdout = os.Stdout
	if err := RunCommand(cmd); err != nil {
		reg.Lock()
		fmt.Printf("%v\nunknown=%v\n", err, reg.unknown)
		for v, e := range reg.vals {
			fmt.Printf("held %q from %q\n", v, e.src)
		}
		reg.Unlock()
		explainRefusal(cmd)
		os.Exit(1)
	}
}

// explainRefusal reports why the kit refused cmd: the verdict over the
// envelope the child would get, and a smallest set of its entries that is
// still refused, by argv index and environment name and length only (the
// environment is the machine's).
func explainRefusal(cmd *exec.Cmd) {
	m, _ := currentMatcher()
	argv, env := childEnvelope(cmd, cmd.Env, runtime.GOOS)
	type entry struct{ name, v string }
	var es []entry
	for i, a := range argv {
		es = append(es, entry{fmt.Sprintf("argv[%d]", i), a})
	}
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		es = append(es, entry{"env " + name, kv})
	}
	join := func(es []entry) []byte {
		var b []byte
		for i, e := range es {
			if i > 0 {
				b = append(b, '\n')
			}
			b = append(b, e.v...)
		}
		return b
	}
	start := time.Now()
	fmt.Printf("verdict=%d (1 leak, 2 exhausted) bytes=%d entries=%d in %v\n", m.check(join(es)), len(join(es)), len(es), time.Since(start))
	for i := 0; i < len(es); {
		rest := append(slices.Clip(es[:i]), es[i+1:]...)
		if m.check(join(rest)) != clean {
			es = rest
			continue
		}
		i++
	}
	for _, e := range es {
		fmt.Printf("needed: %s (%d bytes, alone verdict=%d)\n", e.name, len(e.v), m.check([]byte(e.v)))
	}
	// Which limit the smallest set spends: each raised alone.
	u := join(es)
	for _, l := range []struct {
		name string
		p    *int
	}{{"budgetPerByte", &budgetPerByte}, {"maxDepth", &maxDepth}, {"deflBase", &deflBase}, {"deflMax", &deflMax}} {
		old := *l.p
		*l.p = old * 64
		fmt.Printf("%s x64: verdict=%d\n", l.name, m.check(u))
		*l.p = old
	}
}

// childHeld runs in the child: it reports whether it holds the parent's
// value, without printing the value.
func childHeld() {
	readInherited()
	reg.Lock()
	e := reg.vals["inherited-secret-99"]
	reg.Unlock()
	if e == nil {
		io.WriteString(os.Stdout, "held=false\n")
		return
	}
	io.WriteString(os.Stdout, "held=true allowed="+strings.Join(e.allowed, ",")+"\n")
}

func TestContract2StateAcrossWritesTrickle(t *testing.T) {
	holding(t, cred)
	var out bytes.Buffer
	w := NewWriter(&out, "test")
	for _, c := range cred {
		w.Write([]byte(string(c)))
	}
	w.Write([]byte("\nclean after\n"))
	emitted := strings.ReplaceAll(out.String(), Notice("test"), "")
	if strings.Contains(emitted, cred[7:]) || len(strings.ReplaceAll(emitted, "\nclean after\n", "")) > 7 {
		t.Errorf("a credential written one character per write leaked: %q", out.String())
	}
	if !strings.HasSuffix(out.String(), "\nclean after\n") {
		t.Errorf("clean output after a withheld write was withheld too: %q", out.String())
	}
}
