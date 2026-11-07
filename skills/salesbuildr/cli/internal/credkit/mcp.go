// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credkit

// Contracts 1, 4 and 5 for MCP: arguments named in MCPArguments are held
// before any handler sees the call, and every MCP message (a result, an
// error, a protocol frame, a log notification) is one unit. A withheld
// message is replaced by a JSON-RPC error carrying the request's id and the
// notice, or by a log notification carrying the notice when it has no id,
// and dropped when that replacement would itself match.
//
// Wiring, for each transport the server offers:
//
//	stdio: server.NewStdioServer(s).Listen(ctx, credkit.MCPIn(os.Stdin), credkit.MCPOut())
//	http:  handler := credkit.MCPHTTP(server.NewStreamableHTTPServer(s))
//	tools: server.WithToolHandlerMiddleware(credkit.MCPTools)

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// MCPTools is tool-handler middleware: it holds the call's declared
// arguments and checks the result (or the error) as one MCP message.
func MCPTools(next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if b, err := json.Marshal(req.Params.Arguments); err == nil {
			holdMCP(b, true)
		}
		res, err := next(ctx, req)
		if err != nil {
			if withheld([]byte(err.Error())) {
				return nil, withheldError{SinkMCP}
			}
			return res, err
		}
		if b, merr := json.Marshal(res); merr != nil || withheld(b) {
			n := Notice(SinkMCP)
			if withheld([]byte(n)) {
				n = ""
			}
			return mcp.NewToolResultError(n), nil
		}
		return res, nil
	}
}

// holdMCP holds the declared arguments in one MCP message, or in a bare
// arguments object when args is true.
func holdMCP(b []byte, args bool) {
	reg.Lock()
	defer reg.Unlock()
	names := reg.decl.MCPArguments
	if len(names) == 0 {
		return
	}
	for _, v := range readConfig(b) {
		if v.stray {
			continue
		}
		keys := v.keys
		if !args {
			// Only values under params.arguments of a call are arguments.
			i := indexOf(keys, "arguments")
			if i < 0 {
				continue
			}
			keys = keys[i+1:]
		}
		for _, k := range keys {
			if containsFold(names, k) {
				holdLocked("mcp:"+k, v.value, v.raw)
				break
			}
		}
	}
}

func indexOf(keys []string, k string) int {
	for i, x := range keys {
		if strings.EqualFold(x, k) {
			return i
		}
	}
	return -1
}

// MCPIn wraps the stdio server's input: each frame's declared arguments are
// held before the server reads the frame.
func MCPIn(r io.Reader) io.Reader {
	pr, pw := io.Pipe()
	go func() {
		br := bufio.NewReaderSize(r, 64<<10)
		for {
			line, err := br.ReadBytes('\n')
			if len(line) > 0 {
				holdMCP(line, false)
				if _, werr := pw.Write(line); werr != nil {
					return
				}
			}
			if err != nil {
				pw.CloseWithError(err)
				return
			}
		}
	}()
	return pr
}

// MCPOut is the stdio server's output: the original stdout, each
// newline-ended frame checked as one MCP message. Frames do not pass through
// the stdout pump, so a withheld frame is replaced by a frame the client can
// still parse.
func MCPOut() io.Writer { return &frameWriter{w: originalStdout()} }

type frameWriter struct {
	mu   sync.Mutex
	w    io.Writer
	buf  []byte
	dead bool
}

func (f *frameWriter) Write(p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.dead {
		return 0, errMCPClosed
	}
	f.buf = append(f.buf, p...)
	for {
		i := bytes.IndexByte(f.buf, '\n')
		if i < 0 {
			return len(p), nil
		}
		if err := f.emit(f.buf[:i+1], f.buf[:i]); err != nil {
			return 0, err
		}
		f.buf = append(f.buf[:0], f.buf[i+1:]...)
	}
}

// emit is the one exit point for MCP stdio: frame, one message msg with
// its line ending, checked whole. A withheld frame is replaced by
// mcpReplacement(msg), framed and checked whole again; when that is
// withheld too (the kit's own notice and framing would spell a held value)
// the session fails closed: nothing more is written and the stream, when
// it can be, is closed, so the client sees the session end.
func (f *frameWriter) emit(frame, msg []byte) error {
	defer emitEnd(emitBegin()) // the barrier, to the end of the call (held.go heldMu)
	if withheld(frame) {
		if frame = lineFrame(mcpReplacement(msg)); withheld(frame) {
			f.dead = true
			mcpLost.Store(true)
			if c, ok := f.w.(io.Closer); ok {
				_ = c.Close()
			}
			return errMCPClosed
		}
	}
	_, err := f.w.Write(frame)
	if err != nil {
		mcpLost.Store(true)
	}
	return err
}

// mcpLost: a frame for the original stdout did not reach it, past the pump
// that Exit otherwise asks (stdio.go Exit).
var mcpLost atomic.Bool

func lineFrame(msg []byte) []byte { return append(slices.Clip(msg), '\n') }

func sseFrame(msg []byte) []byte {
	return append(append([]byte("event: message\ndata: "), msg...), '\n', '\n')
}

// mcpReplacement is the message that stands in for a withheld one: an
// error for each request id it answers, else a log notification.
func mcpReplacement(msg []byte) []byte {
	notice, _ := json.Marshal(Notice(SinkMCP))
	one := func(id json.RawMessage) []byte {
		if len(id) == 0 || string(id) == "null" {
			return fmt.Appendf(nil, `{"jsonrpc":"2.0","method":"notifications/message","params":{"level":"warning","logger":"credkit","data":%s}}`, notice)
		}
		return fmt.Appendf(nil, `{"jsonrpc":"2.0","id":%s,"error":{"code":-32603,"message":%s}}`, id, notice)
	}
	type idOnly struct {
		ID json.RawMessage `json:"id"`
	}
	var out []byte
	var batch []idOnly
	if t := bytes.TrimSpace(msg); len(t) > 0 && t[0] == '[' && json.Unmarshal(t, &batch) == nil {
		var parts [][]byte
		for _, m := range batch {
			parts = append(parts, one(m.ID))
		}
		out = append(append([]byte("["), bytes.Join(parts, []byte(","))...), ']')
	} else {
		var m idOnly
		_ = json.Unmarshal(msg, &m)
		out = one(m.ID)
	}
	return out
}

// MCPHTTP wraps an MCP HTTP handler (streamable HTTP or SSE): the request's
// declared arguments are held before the handler runs. The handler writes
// into the kit's own response, and every byte the client gets leaves
// through one exit point (mcpResponse.send): a JSON response is one unit,
// its status line, header and body together; a stream's head is one unit
// and each of its events another, each checked with what the stream
// already sent. A withheld unit is replaced by the kit's (each held header
// value the notice, a held reason phrase blank or a held status line
// 500, a held message mcpReplacement), checked whole again; when that is
// withheld too, the response fails closed: the connection is closed (or
// the handler aborted) with nothing more written. No trailer is ever sent.
//
// On HTTP/1.x the kit takes the connection over (Hijack) and writes the
// bytes it checked, with Connection: close. A ResponseWriter that cannot be
// taken over (HTTP/2, a wrapper) gets the head through net/http, set up so
// net/http adds nothing (no Date, a set Content-Length or, for a stream,
// Transfer-Encoding: identity, which net/http removes and which turns
// chunking and trailers off); the unit checked is that head as net/http
// writes it for HTTP/1.x.
func MCPHTTP(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Headers set before this handler (outer middleware) go out
		// through the same check as the handler's: they move into the
		// kit's own map, and the writer's map is left empty.
		before := w.Header().Clone()
		if before == nil {
			before = http.Header{}
		}
		clear(w.Header())
		m := &mcpResponse{w: w, r: r, h: before, status: http.StatusOK}
		defer m.closeConn()
		// Blunt rule: a request that expects 100-continue is refused (417)
		// before its body is read. Reading it makes net/http write its
		// interim response before the kit has held the arguments the
		// body carries (contract 1), so no check of that line can be
		// complete. Ordinary MCP clients send no Expect.
		if r.Body != nil && r.ProtoAtLeast(1, 1) && expectsContinue(r.Header) {
			m.status = http.StatusExpectationFailed
			m.finish()
			return
		}
		if r.Body != nil {
			b, err := io.ReadAll(r.Body)
			_ = r.Body.Close()
			// The body as written and as its headers declare it
			// (acquire.go), held before any response, the error ones
			// included: a read that ends in an error still returns the
			// bytes it read (kit13rv). One the kit cannot read whole
			// never reaches the handler, and the arguments it may carry
			// are unknown: acquisition is marked unknown before the
			// refusal goes out (kit14rv, blunt: every unit is withheld
			// from then on).
			rs, ok := readings(b, r.Header)
			ok = ok && requestWhole("", b, r.Header, r.Trailer) // loop ruling 8
			for _, x := range rs {
				holdMCP(x, false)
			}
			if err != nil || !ok {
				markUnknown()
			}
			if err != nil {
				m.status = http.StatusBadRequest
				m.finish()
				return
			}
			if !ok {
				m.status = http.StatusUnsupportedMediaType
				m.finish()
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(b))
		}
		h.ServeHTTP(m, r)
		m.finish()
	})
}

// expectsContinue reports whether net/http would send 100 Continue for h:
// a 100-continue token in Expect, in any case.
func expectsContinue(h http.Header) bool {
	for _, v := range headerValues(h, "Expect") { // any spelling (kit14rv)
		for _, t := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(t), "100-continue") {
				return true
			}
		}
	}
	return false
}

var errMCPClosed = errors.New("credkit: the MCP response failed closed")

type mcpResponse struct {
	mu     sync.Mutex
	w      http.ResponseWriter
	r      *http.Request
	h      http.Header // the handler's: nothing in it reaches the client but through send
	status int
	set    bool // the handler set the status
	mode   int  // 0 undecided, 1 the connection taken over, 2 net/http writes
	conn   net.Conn
	bw     *bufio.ReadWriter
	head   bool // the head is out
	dead   bool // failed closed
	st     streamCtx
	buf    []byte
}

func (m *mcpResponse) Header() http.Header { return m.h }

func (m *mcpResponse) WriteHeader(code int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if code >= 100 && code < 200 {
		// An informational status is not forwarded (kit15rv: through
		// net/http it went out, and the final response net/http then
		// wrote for the body carried framing the kit never checked). The
		// response's own status stays to be set.
		return
	}
	if !m.set && !m.head {
		m.status, m.set = code, true
	}
	if m.sse() {
		_ = m.start()
	}
}

func (m *mcpResponse) sse() bool {
	return strings.HasPrefix(m.h.Get("Content-Type"), "text/event-stream")
}

func (m *mcpResponse) Write(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dead {
		return 0, errMCPClosed
	}
	m.buf = append(m.buf, p...)
	if m.sse() {
		if err := m.events(false); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

// Flush passes complete events on (a JSON body waits for the handler to end).
func (m *mcpResponse) Flush() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sse() {
		_ = m.events(false)
	}
}

// start sends a stream's head, once.
func (m *mcpResponse) start() error {
	if m.head {
		return nil
	}
	return m.send(m.frame(nil, false), func() mcpFrame { return m.frame(nil, true) })
}

// events sends each complete event in buf (all of buf at the end).
func (m *mcpResponse) events(final bool) error {
	if err := m.start(); err != nil {
		return err
	}
	for {
		i := bytes.Index(m.buf, []byte("\n\n"))
		if i < 0 && !(final && len(m.buf) > 0) {
			return nil
		}
		end := i + 2
		if i < 0 {
			end = len(m.buf)
		}
		ev := slices.Clone(m.buf[:end])
		m.buf = append(m.buf[:0], m.buf[end:]...)
		if err := m.send(mcpFrame{body: ev}, func() mcpFrame {
			return mcpFrame{body: sseFrame(mcpReplacement(sseData(ev)))}
		}); err != nil {
			return err
		}
	}
}

// finish ends the response: a stream's last event, or a JSON response
// whole. A response that failed closed through net/http is aborted.
func (m *mcpResponse) finish() {
	m.mu.Lock()
	if m.sse() {
		_ = m.events(true)
	} else if !m.head {
		body := m.buf
		_ = m.send(m.frame(body, false), func() mcpFrame {
			if m.st.withheld(body) {
				body = mcpReplacement(body)
			}
			return m.frame(body, true)
		})
	}
	abort := m.dead && m.mode == 2
	m.mu.Unlock()
	if abort {
		panic(http.ErrAbortHandler)
	}
}

func (m *mcpResponse) closeConn() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.conn != nil {
		_ = m.conn.Close()
		m.conn = nil
	}
}

// mcpFrame is one unit of a response: a head (code, phrase, h) with the
// body that follows it, or (h nil) bytes after the head.
type mcpFrame struct {
	code   int
	phrase string
	h      http.Header
	body   []byte
}

// frame is the head unit, with body when the response is JSON (nil for a
// stream). kit is the kit's replacement: each header line that is
// withheld on its own carries the notice, and a status line that is
// withheld on its own loses its reason phrase (or, through net/http,
// which cannot write another phrase, becomes 500).
func (m *mcpResponse) frame(body []byte, kit bool) mcpFrame {
	m.takeOver()
	f := mcpFrame{code: m.status, phrase: statusPhrase(m.status), h: http.Header{}, body: body}
	for k, v := range m.h {
		switch {
		case strings.HasPrefix(k, http.TrailerPrefix):
		case slices.Contains([]string{"Connection", "Content-Length", "Date", "Keep-Alive", "Trailer", "Transfer-Encoding"}, k):
		default:
			f.h[k] = slices.Clone(v)
		}
	}
	if _, ok := f.h["Content-Type"]; !ok && len(body) > 0 {
		f.h.Set("Content-Type", http.DetectContentType(body))
	}
	if m.r.ProtoMajor == 1 {
		f.h.Set("Connection", "close")
	}
	if !m.sse() && f.code >= 200 && f.code != http.StatusNoContent && f.code != http.StatusNotModified {
		f.h.Set("Content-Length", strconv.Itoa(len(body)))
	}
	if !kit {
		return f
	}
	if withheld(mcpHead(m.r, f.code, f.phrase, nil)) {
		if m.mode == 1 {
			f.phrase = ""
		} else {
			f.code, f.phrase = http.StatusInternalServerError, statusPhrase(http.StatusInternalServerError)
		}
	}
	for k, vs := range f.h {
		for i, v := range vs {
			if withheld([]byte(k + ": " + v)) {
				vs[i] = Notice(SinkMCP)
			}
		}
	}
	return f
}

// statusPhrase is the reason phrase net/http writes for code.
func statusPhrase(code int) string {
	if t := http.StatusText(code); t != "" {
		return t
	}
	return "status code " + strconv.Itoa(code)
}

// mcpHead is a response head as it goes out: the status line, then the
// header lines as net/http writes them (sorted, each value's line breaks
// turned to spaces), then the blank line.
func mcpHead(r *http.Request, code int, phrase string, h http.Header) []byte {
	proto := "HTTP/1.0"
	if r.ProtoAtLeast(1, 1) {
		proto = "HTTP/1.1"
	}
	b := fmt.Appendf(nil, "%s %03d %s\r\n", proto, code, phrase)
	if h == nil {
		return b
	}
	var hb bytes.Buffer
	_ = h.Write(&hb)
	return append(append(b, hb.Bytes()...), "\r\n"...)
}

func (f mcpFrame) bytes(r *http.Request) []byte {
	if f.h == nil {
		return f.body
	}
	return append(mcpHead(r, f.code, f.phrase, f.h), f.body...)
}

// takeOver decides, once, who writes: the kit on the connection it takes
// over (HTTP/1.x), or net/http.
func (m *mcpResponse) takeOver() {
	if m.mode != 0 {
		return
	}
	m.mode = 2
	if hj, ok := m.w.(http.Hijacker); ok && m.r.ProtoMajor == 1 {
		if conn, bw, err := hj.Hijack(); err == nil {
			m.mode, m.conn, m.bw = 1, conn, bw
		}
	}
}

// send is the one exit point for MCP HTTP: unit f as the client gets it,
// else the kit's alt(), each checked whole with what the response already
// sent; when both are withheld the response fails closed.
func (m *mcpResponse) send(f mcpFrame, alt func() mcpFrame) error {
	defer emitEnd(emitBegin()) // the barrier, to the end of the call (held.go heldMu)
	if m.dead {
		return errMCPClosed
	}
	for i := range 2 {
		if i == 1 {
			f = alt()
		}
		b := f.bytes(m.r)
		if m.st.withheld(b) {
			continue
		}
		m.st.done(b, b, false)
		if f.h != nil {
			m.head = true
		}
		if m.out(f, b) != nil {
			break
		}
		return nil
	}
	m.dead = true
	if m.conn != nil {
		_ = m.conn.Close()
		m.conn = nil
	}
	return errMCPClosed
}

// out writes a checked unit: b on the connection taken over, or f through
// net/http, set up so net/http writes no byte of its own.
func (m *mcpResponse) out(f mcpFrame, b []byte) error {
	if m.mode == 1 {
		if m.conn == nil {
			return errMCPClosed
		}
		if _, err := m.bw.Write(b); err != nil {
			return err
		}
		return m.bw.Flush()
	}
	if f.h != nil {
		h := m.w.Header()
		maps.Copy(h, f.h)
		h["Date"] = nil
		if m.r.ProtoMajor == 1 && m.sse() {
			h.Set("Transfer-Encoding", "identity")
		}
		m.w.WriteHeader(f.code)
	}
	if len(f.body) > 0 {
		if _, err := m.w.Write(f.body); err != nil {
			return err
		}
	}
	if fl, ok := m.w.(http.Flusher); ok {
		fl.Flush()
	}
	return nil
}

// sseData is the data of one event.
func sseData(ev []byte) []byte {
	var out []byte
	for _, l := range bytes.Split(ev, []byte("\n")) {
		if d, ok := bytes.CutPrefix(l, []byte("data:")); ok {
			out = append(out, bytes.TrimPrefix(d, []byte(" "))...)
		}
	}
	return out
}
