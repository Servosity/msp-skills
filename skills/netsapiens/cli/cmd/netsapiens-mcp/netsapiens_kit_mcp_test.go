// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// End-to-end class test: successful MCP tool results, over stdio, from the built
// server, through the shared credential kit (credkit.MCPTools, credkit.MCPOut).

package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"netsapiens-pp-cli/internal/cliutil/testenv"
)

// vendorTLS starts a fake vendor over https and returns the variables that make
// the built binary trust it: the credential kit refuses to send a credential over
// plain http, as it should.
func vendorTLS(t *testing.T, h http.Handler) (*httptest.Server, []string) {
	t.Helper()
	srv := httptest.NewTLSServer(h)
	t.Cleanup(srv.Close)
	ca := filepath.Join(t.TempDir(), "ca.pem")
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	if err := os.WriteFile(ca, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return srv, []string{"NETSAPIENS_CA_CERT=" + ca, "SSL_CERT_FILE=" + ca}
}

func mcpBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "netsapiens-mcp")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return bin
}

// callTool starts the server over stdio, calls one tool and returns the raw
// JSON-RPC response line.
func callTool(t *testing.T, bin string, env []string, name string, args map[string]any) string {
	t.Helper()
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), env...)
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	call, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}})
	for _, line := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		string(call),
	} {
		_, _ = io.WriteString(stdin, line+"\n")
	}
	lines := make(chan string)
	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 1<<20), 16<<20)
		for sc.Scan() {
			lines <- sc.Text()
		}
		close(lines)
	}()
	deadline := time.After(60 * time.Second)
	for {
		select {
		case l, ok := <-lines:
			if !ok {
				t.Fatal("server closed stdout before answering")
			}
			if strings.Contains(l, `"id":2`) {
				return l
			}
		case <-deadline:
			t.Fatal("no answer to tools/call")
		}
	}
}

func TestClassSuccessfulToolResultsCarryNoCredential(t *testing.T) {
	bin := mcpBinary(t) // before Isolate: the build needs the real module and build caches
	testenv.Isolate(t)
	const apiKey = "DifferentApiKey55443322"
	b64Key := base64.StdEncoding.EncodeToString([]byte(apiKey))
	for name, body := range map[string]string{
		"vendor credential field (Domain.email-send-smtp-password)": `[{"domain":"acme.example","email-send-smtp-password":"SmtpSecret55443322"}]`,
		"operator key echoed base64":                                `[{"domain":"acme.example","description":"` + b64Key + `"}]`,
	} {
		t.Run(name, func(t *testing.T) {
			srv, trust := vendorTLS(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, body)
			}))
			env := append(trust, "NETSAPIENS_BASE_URL="+srv.URL, "NETSAPIENS_API_KEY="+apiKey, "NETSAPIENS_RETRY_MAX=0")
			got := callTool(t, bin, env, "netsapiens_read", map[string]any{"endpoint_id": "domains.list", "params": map[string]any{}})
			for _, s := range []string{"SmtpSecret55443322", apiKey, b64Key} {
				if strings.Contains(got, s) {
					t.Fatalf("a successful tool result carries %q:\n%s", s, got)
				}
			}
		})
	}
}
