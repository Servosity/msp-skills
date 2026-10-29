// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package main

// pax8 handfix (output-chokepoint): every tool argument named like a
// credential (cliutil.HoldCredentialValues, at any depth) is held before the
// call runs. The MCP server's output paths pass the one chokepoint
// (cliutil.WithholdText): every tool result and error through
// withholdToolResults (outermost middleware), stderr and the standard logger
// through InstallStderrFunnel from init (installed before the startup sources
// load), and every JSON-RPC message the server writes on either transport
// through withheldMessageWriter (serveStdioWithheld, withheldHTTP): mcp-go answers an
// unknown tool or prompt name with an error that repeats the name before any
// tool middleware runs (Codex r1 class mcp-protocol-output-bypasses-chokepoint).
//
// A mirrored tool runs the CLI as a child process, which holds only what it
// loads itself (the environment and the configuration); a value only this
// server holds (a credential a response or an argument carried) would reach
// the child's files unwithheld. So a call whose
// arguments repeat a held value in any form is refused before any tool runs,
// in-process or mirrored (fleet credential-rules class
// held-value-crosses-process-boundary). Credential-named arguments are held
// and left out of that check: a child holds a credential flag itself.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"pax8-pp-cli/internal/cliutil"
	"pax8-pp-cli/internal/config"
)

func init() {
	// The chokepoint is installed first and delivers nothing until the
	// startup sources are held: config.Load prints its warnings while it
	// loads (Codex r1 class output-before-chokepoint).
	if err := cliutil.InstallStderrFunnel(func() {
		cliutil.HoldEnvCredentials()
		_, _ = config.Load("") // holds the config file's and the credential store's values; errors are left to the tools
	}); err != nil {
		fmt.Fprintln(os.Stderr, "error: cannot start the output filter:", err)
		os.Exit(1)
	}
}

func withholdToolResults(next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcplib.CallToolRequest) (*mcplib.CallToolResult, error) {
		cliutil.HoldCredentialValues(req.GetArguments())
		if argumentsRepeatHeld(req) {
			return mcplib.NewToolResultError(heldArgumentRefusal), nil
		}
		res, err := next(ctx, req)
		if err != nil {
			if msg := cliutil.WithholdText(err.Error()); msg != err.Error() {
				err = errors.New(msg)
			}
		}
		if res == nil {
			return res, err
		}
		for i, c := range res.Content {
			switch tc := c.(type) {
			case mcplib.TextContent:
				tc.Text = cliutil.WithholdText(tc.Text)
				res.Content[i] = tc
			case *mcplib.TextContent:
				tc.Text = cliutil.WithholdText(tc.Text)
			case mcplib.EmbeddedResource:
				res.Content[i] = withholdEmbedded(tc)
			case *mcplib.EmbeddedResource:
				*tc = withholdEmbedded(*tc)
			}
		}
		if res.StructuredContent != nil {
			b, mErr := json.Marshal(res.StructuredContent)
			if mErr != nil {
				res.StructuredContent = nil
			} else if w := cliutil.WithholdText(string(b)); w != string(b) {
				var v any
				if json.Unmarshal([]byte(w), &v) != nil {
					v = nil
				}
				res.StructuredContent = v
			}
		}
		return res, err
	}
}

const heldArgumentRefusal = "refused: an argument repeats a credential this MCP server holds (the Pax8 token, the client secret, or a credential a Pax8 response carried); credentials are never passed to a tool"

// argumentsRepeatHeld reports a call whose arguments, with credential-named
// members redacted, repeat a held value in any form the chokepoint finds
// (whole, encoded or split across values). Arguments it cannot read are judged
// as text; anything it cannot re-encode is refused.
func argumentsRepeatHeld(req mcplib.CallToolRequest) bool {
	var v any = req.Params.Arguments
	if raw := req.Params.RawArguments; len(raw) > 0 {
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if dec.Decode(&v) != nil {
			return cliutil.HeldEchoes(string(raw))
		}
	} else if v != nil {
		b, err := json.Marshal(v)
		if err != nil {
			return true
		}
		dec := json.NewDecoder(bytes.NewReader(b))
		dec.UseNumber()
		if dec.Decode(&v) != nil {
			return true
		}
	}
	if v == nil {
		return false
	}
	b, err := json.Marshal(cliutil.RedactSecretFields(v))
	if err != nil {
		return true
	}
	return cliutil.HeldEchoes(string(b))
}

func withholdEmbedded(e mcplib.EmbeddedResource) mcplib.EmbeddedResource {
	switch r := e.Resource.(type) {
	case mcplib.TextResourceContents:
		r.Text = cliutil.WithholdText(r.Text)
		e.Resource = r
	case *mcplib.TextResourceContents:
		r.Text = cliutil.WithholdText(r.Text)
	}
	return e
}

// serveStdioWithheld is server.ServeStdio with the protocol stream (every
// response and notification) written through the chokepoint.
func serveStdioWithheld(s *server.MCPServer) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sig
		cancel()
	}()
	out := &withheldMessageWriter{dst: os.Stdout}
	defer func() { _ = out.Close() }()
	return server.NewStdioServer(s).Listen(ctx, os.Stdin, out)
}

// withheldMessageWriter passes each line (one JSON-RPC message, or one SSE
// field) through the chokepoint before it reaches dst. A message the
// chokepoint can withhold in place stays valid JSON; one it cannot (a value
// split across members) is replaced by a JSON-RPC error with the same id.
type withheldMessageWriter struct {
	mu  sync.Mutex
	dst io.Writer
	buf []byte
}

func (m *withheldMessageWriter) Write(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.buf = append(m.buf, p...)
	for {
		i := bytes.IndexByte(m.buf, '\n')
		if i < 0 {
			return len(p), nil
		}
		line := withheldMessageLine(string(m.buf[:i]))
		m.buf = m.buf[i+1:]
		if len(m.buf) == 0 {
			m.buf = nil
		}
		if _, err := io.WriteString(m.dst, line+"\n"); err != nil {
			return len(p), err
		}
	}
}

// Close writes a last line that had no newline.
func (m *withheldMessageWriter) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.buf) == 0 {
		return nil
	}
	line := withheldMessageLine(string(m.buf))
	m.buf = nil
	_, err := io.WriteString(m.dst, line)
	return err
}

func withheldMessageLine(line string) string {
	out := cliutil.WithholdText(line)
	if out == line {
		return line
	}
	prefix, payload := "", line
	if strings.HasPrefix(line, "data:") {
		prefix, payload = "data: ", strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " ")
	}
	if !json.Valid([]byte(payload)) {
		return out // not a protocol message: the withheld text as is
	}
	if w := cliutil.WithholdText(payload); json.Valid([]byte(w)) && !cliutil.HeldEchoes(w) {
		return prefix + w
	}
	return prefix + withheldMessage(payload)
}

// withheldMessage stands in for a message the chokepoint cannot withhold in
// place: an error answering the same request id, or a log notification.
func withheldMessage(payload string) string {
	var m struct {
		ID json.RawMessage `json:"id"`
	}
	_ = json.Unmarshal([]byte(payload), &m)
	id := strings.TrimSpace(string(m.ID))
	var v map[string]any
	if id == "" || id == "null" {
		v = map[string]any{"jsonrpc": "2.0", "method": "notifications/message",
			"params": map[string]any{"level": "warning", "logger": "pax8-mcp", "data": cliutil.FunnelWithheldText}}
	} else {
		if cliutil.HeldEchoes(id) {
			id = "null"
		}
		v = map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id), "error": map[string]any{"code": -32603, "message": cliutil.FunnelWithheldText}}
	}
	b, err := json.Marshal(v)
	if err != nil {
		return `{"jsonrpc":"2.0","id":null,"error":{"code":-32603,"message":"response withheld"}}`
	}
	return string(b)
}

// withheldHTTP writes every streamable-HTTP response body (JSON replies and
// SSE events) through the chokepoint.
func withheldHTTP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := &withheldResponseWriter{ResponseWriter: w, body: &withheldMessageWriter{dst: w}}
		defer func() { _ = ww.body.Close() }()
		next.ServeHTTP(ww, r)
	})
}

type withheldResponseWriter struct {
	http.ResponseWriter
	body *withheldMessageWriter
}

// WriteHeader drops a Content-Length: withholding can change the body size.
func (w *withheldResponseWriter) WriteHeader(code int) {
	w.Header().Del("Content-Length")
	w.ResponseWriter.WriteHeader(code)
}

func (w *withheldResponseWriter) Write(p []byte) (int, error) { return w.body.Write(p) }

func (w *withheldResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the connection for deadlines;
// writes and flushes stay on this wrapper.
func (w *withheldResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
