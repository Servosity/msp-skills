// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): regression tests for the batch-1b Codex
// round-2 inputs, pinned by skills/cipp/handfixes.json under
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

// executeOffboard runs Codex's confirmed cipp_execute call against body.
func executeOffboard(t *testing.T, body string) *mcplib.CallToolResult {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
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
	return res
}

// TestExecuteTenantShapedFailureIsAnError: Codex's input. A write answer that
// carries defaultDomainName gets no tenant-record exemption: its
// LastGraphError beside a success is conflicting, an error.
func TestExecuteTenantShapedFailureIsAnError(t *testing.T) {
	if r := executeOffboard(t, `{"defaultDomainName":"t.example","Results":["success"],"LastGraphError":"Failed to disable sign-in"}`); !r.IsError {
		t.Fatalf("a failure in LastGraphError came back as success: %s", resultText(r))
	}
}

// TestExecuteDuplicateResultsKeyIsAnError: Codex's input. Two Results keys:
// a decoder keeps one, so the answer cannot be trusted and is an error.
func TestExecuteDuplicateResultsKeyIsAnError(t *testing.T) {
	if r := executeOffboard(t, `{"Results":["Failed to disable user"],"Results":["success"]}`); !r.IsError {
		t.Fatalf("a duplicate Results key came back as success: %s", resultText(r))
	}
}

// TestReadLargeTextDoubleBase64Bearer: Codex's input. CIPP_API_KEY is
// aaaaaaaaaaaa; a text/plain body past 1 MiB starts with the key
// base64-encoded twice. Neither the result nor anything under HOME may carry it.
func TestReadLargeTextDoubleBase64Bearer(t *testing.T) {
	const echo = "WVdGaFlXRmhZV0ZoWVdGaA=="
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(echo + "\n" + strings.Repeat("z", 1<<20)))
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
