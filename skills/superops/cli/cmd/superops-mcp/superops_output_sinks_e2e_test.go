// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): the sinks that are not a command's error meet
// the credential holder (credkit-wiring): every stderr line of
// both binaries, and the doctor report, with the doctor verdict kept.

package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func runSinkBinary(t *testing.T, bin string, env []string, args ...string) (string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return string(out), exit.ExitCode()
	}
	if err != nil {
		t.Fatalf("%s: %v", bin, err)
	}
	return string(out), 0
}

func assertSinkWithheld(t *testing.T, where, out string, forms ...string) {
	t.Helper()
	for _, form := range forms {
		if strings.Contains(out, form) {
			t.Errorf("%s shows %q:\n%s", where, form, out)
		}
	}
}

func TestDiagnosticSinksNeverShowAHeldCredential(t *testing.T) {
	t.Parallel()
	cliBin := buildHolderBinary(t, "superops-cli", "../superops-cli")
	mcpBin := buildHolderBinary(t, "superops-mcp", ".")
	b64 := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	hx := func(s string) string { return hex.EncodeToString([]byte(s)) }

	t.Run("doctor code is a held username's base64", func(t *testing.T) {
		// The round-1 input: user "sam", a POST probe answered HTTP 200 with
		// extensions.code "c2Ft".
		host, ca := serveHolderTLS(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"errors":[{"message":"denied","extensions":{"code":"c2Ft"}}]}`))
		}))
		env := holderEnv(t.TempDir(), map[string]string{
			"SUPEROPS_BASE_URL":         "https://sam:LongPassword123@" + host,
			"SUPEROPS_CA_CERT":          ca,
			"SUPEROPS_API_TOKEN":        holderBaseToken,
			"SUPEROPS_SUBDOMAIN":        "acme-holder",
			"SUPEROPS_LEARN_NO_CAPTURE": "1",
		})
		out, _ := runSinkBinary(t, cliBin, env, "doctor", "--json")
		assertSinkWithheld(t, "doctor --json", out, "c2Ft", "sam:", "sam@", hx("sam"), "LongPassword123", b64("LongPassword123"))
		human, _ := runSinkBinary(t, cliBin, env, "doctor")
		assertSinkWithheld(t, "doctor", human, "c2Ft", "sam:", "sam@", hx("sam"), "LongPassword123")
		// The verdict is read before the report is withheld: the denied
		// probe still fails --fail-on warn.
		if o, code := runSinkBinary(t, cliBin, env, "doctor", "--json", "--fail-on", "warn"); code == 0 {
			t.Errorf("doctor --fail-on warn exited 0 on a denied probe:\n%s", o)
		}
	})

	t.Run("round 2: a held value as a preview's map key", func(t *testing.T) {
		const heldValue = "previewKeySecret42"
		sandbox := t.TempDir()
		env := holderEnv(sandbox, map[string]string{"SUPEROPS_BASE_URL": "http://127.0.0.1:9", "SUPEROPS_API_TOKEN": heldValue, "SUPEROPS_SUBDOMAIN": "acme-holder"})
		out, _ := runSinkBinary(t, cliBin, env, "raw", "query", "{ __typename }", "--vars", `{"`+heldValue+`":0}`, "--dry-run")
		if !strings.Contains(out, "withheld") {
			t.Errorf("want the key withheld:\n%s", out)
		}
		assertRunWithheld(t, sandbox, out, []string{heldValue})
	})

	t.Run("round 2: a held value as a flag name, with the journal on", func(t *testing.T) {
		const heldValue = "JournalSecret4488"
		sandbox := t.TempDir()
		env := holderEnv(sandbox, map[string]string{"SUPEROPS_BASE_URL": "http://127.0.0.1:9", "SUPEROPS_API_TOKEN": heldValue, "SUPEROPS_SUBDOMAIN": "acme-holder", "PRINTING_PRESS_DOGFOOD": ""})
		out, _ := runSinkBinary(t, cliBin, env, "--"+heldValue)
		out2, _ := runSinkBinary(t, cliBin, env, "version")
		assertRunWithheld(t, sandbox, out+out2, []string{heldValue})
		journal := false
		_ = filepath.WalkDir(sandbox, func(p string, d os.DirEntry, err error) error {
			journal = journal || (err == nil && !d.IsDir() && strings.Contains(p, "journal"))
			return nil
		})
		if !journal {
			t.Error("the journal wrote nothing, so the check proves nothing")
		}
	})

	t.Run("round 2 P2: a held username inside the denial code keeps the verdict", func(t *testing.T) {
		host, ca := serveHolderTLS(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"errors":[{"message":"denied","extensions":{"code":"UNAUTHENTICATED"}}]}`))
		}))
		sandbox := t.TempDir()
		env := holderEnv(sandbox, map[string]string{
			"SUPEROPS_BASE_URL":  "https://AUTH:LongPassword123@" + host,
			"SUPEROPS_CA_CERT":   ca,
			"SUPEROPS_API_TOKEN": holderBaseToken, "SUPEROPS_SUBDOMAIN": "acme-holder",
		})
		out, code := runSinkBinary(t, cliBin, env, "doctor", "--json", "--fail-on", "error")
		if code == 0 {
			t.Errorf("doctor --fail-on error exited 0 on a denied probe:\n%s", out)
		}
		assertRunWithheld(t, sandbox, out, []string{"LongPassword123"})
	})

	t.Run("round 2: mcp-go's logger in HTTP mode, the token as the session id", func(t *testing.T) {
		const heldValue = "9f4b6e2ca1d3f5072e6b9c4a8d1f3057"
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		addr := l.Addr().String()
		_ = l.Close()
		sandbox := t.TempDir()
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, mcpBin, "--transport", "http", "--addr", addr)
		cmd.Env = holderEnv(sandbox, map[string]string{"SUPEROPS_MCP_HTTP_TOKEN": heldValue, "SUPEROPS_API_TOKEN": holderBaseToken, "SUPEROPS_SUBDOMAIN": "acme-holder"})
		var stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stderr, &stderr
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		var body string
		for i := 0; i < 100; i++ {
			req, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"result":{"ok":true}}`))
			req.Header.Set("Authorization", "Bearer "+heldValue)
			req.Header.Set("Mcp-Session-Id", heldValue)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				time.Sleep(50 * time.Millisecond)
				continue
			}
			b, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			body = string(b)
			break
		}
		time.Sleep(500 * time.Millisecond) // past the guard's idle flush
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		out := stderr.String() + body
		assertRunWithheld(t, sandbox, out, []string{heldValue})
		if !strings.Contains(stderr.String(), "withheld") {
			t.Errorf("want mcp-go's log line withheld, not missing:\n%s", out)
		}
	})

	t.Run("MCP HTTP transport: responses echoing a held value", func(t *testing.T) {
		const heldValue = "zq7Tok0123456789abcdefXyW4mN8pRs"
		var requests atomic.Int64
		host, ca := serveHolderStub(t, []string{holderBaseToken}, &requests)
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		addr := l.Addr().String()
		_ = l.Close()
		sandbox := t.TempDir()
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, mcpBin, "--transport", "http", "--addr", addr)
		cmd.Env = holderEnv(sandbox, map[string]string{"SUPEROPS_MCP_HTTP_TOKEN": heldValue, "SUPEROPS_API_TOKEN": holderBaseToken, "SUPEROPS_CA_CERT": ca, "SUPEROPS_BASE_URL": "https://" + host, "SUPEROPS_SUBDOMAIN": "acme-holder", "SUPEROPS_LEARN_NO_CAPTURE": "1"})
		var stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stderr, &stderr
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
		session := ""
		post := func(body string) (string, error) {
			req, _ := http.NewRequest(http.MethodPost, "http://"+addr+"/mcp", strings.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+heldValue)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			if session != "" {
				req.Header.Set("Mcp-Session-Id", session)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return "", err
			}
			defer resp.Body.Close()
			if id := resp.Header.Get("Mcp-Session-Id"); id != "" {
				session = id
			}
			b, err := io.ReadAll(resp.Body)
			return string(b), err
		}
		var out string
		for i := 0; i < 100; i++ {
			if out, err = post(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"holder-test","version":"0"}}}`); err == nil {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		if err != nil {
			t.Fatalf("initialize: %v\n%s", err, stderr.String())
		}
		_, _ = post(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
		before := requests.Load()
		for i, call := range []string{
			`{"name":"raw_query","arguments":{"args":` + strconv.Quote(holderRawQuery) + `}}`,
			`{"name":"tickets_list","arguments":{}}`,
			// mcp-go answers an unknown tool by name, before any tool code runs.
			`{"name":"tool_` + heldValue + `","arguments":{}}`,
		} {
			body, err := post(`{"jsonrpc":"2.0","id":` + string(rune('2'+i)) + `,"method":"tools/call","params":` + call + `}`)
			if err != nil {
				t.Fatal(err)
			}
			out += "\n" + body
		}
		if requests.Load()-before < 2 {
			t.Errorf("MCP HTTP sent %d requests, want 2:\n%s\n%s", requests.Load()-before, out, stderr.String())
		}
		assertHolderWithheld(t, "MCP HTTP responses", out, []string{holderBaseToken, heldValue})
		assertRunWithheld(t, sandbox, out+stderr.String(), []string{holderBaseToken, heldValue})
	})

}
