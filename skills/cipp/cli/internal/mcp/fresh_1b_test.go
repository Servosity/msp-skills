// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): regression tests for the batch-1b Codex
// round-1 inputs, pinned by skills/cipp/handfixes.json under
// "cipp-results-failure-vocabulary" and "success-body-credential-funnel".

package mcp

import (
	"context"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// TestExecuteEnvelopeErrorBesideSuccessFails: Codex's input. "success" in
// Results beside an envelope "error" is conflicting, an error.
func TestExecuteEnvelopeErrorBesideSuccessFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Results":["success"],"error":"HTTP 403"}`))
	}))
	defer srv.Close()
	t.Setenv("CIPP_BASE_URL", srv.URL)
	t.Setenv("CIPP_API_KEY", "test-static-token")
	res, err := handleCodeOrchExecute(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{
		Arguments: map[string]any{
			"endpoint_id": "exec-offboard-user.create",
			"confirm":     true,
			"params":      map[string]any{"tenantFilter": "t.example", "userPrincipalName": "u@t.example"},
		},
	}})
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if !res.IsError {
		t.Fatalf("a success beside an envelope error came back as success: %s", resultText(res))
	}
}

// TestReadLargeBodyDoubleBase64Bearer: Codex's input. CIPP_API_KEY is
// aaaaaaaaaaaa and a body past 1 MiB carries it base64-encoded twice; neither
// the result nor anything under HOME may carry that echo.
func TestReadLargeBodyDoubleBase64Bearer(t *testing.T) {
	const echo = "WVdGaFlXRmhZV0ZoWVdGaA=="
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"echo":"` + echo + `","padding":"` + strings.Repeat("z", 1<<20) + `"}`))
	}))
	defer srv.Close()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CIPP_HOME", home)
	t.Setenv("CIPP_BASE_URL", srv.URL)
	t.Setenv("CIPP_API_KEY", "aaaaaaaaaaaa")
	res, err := handleCodeOrchRead(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{
		Arguments: map[string]any{"endpoint_id": "list-users.list", "params": map[string]any{"tenantFilter": "t.example"}},
	}})
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if strings.Contains(resultText(res), echo[:16]) {
		t.Fatal("the double-base64 bearer came back in the result")
	}
	_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasSuffix(p, ".toml") {
			return nil
		}
		if b, rerr := os.ReadFile(p); rerr == nil && strings.Contains(string(b), echo[:16]) {
			t.Errorf("the double-base64 bearer was written to %s", p)
		}
		return nil
	})
}
