// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): regression tests for the fresh Codex round-1
// inputs, pinned by skills/cipp/handfixes.json under
// "success-body-credential-funnel" and "cipp-results-failure-vocabulary".

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

// TestReadSuccessBodyWithholdsConfiguredBearer: CIPP_API_KEY=opaqueBearer123456,
// cipp_read list-users.list, HTTP 200 body echoing the bearer in a display
// name. Neither the tool result nor the response cache may carry it.
func TestReadSuccessBodyWithholdsConfiguredBearer(t *testing.T) {
	const bearer = "opaqueBearer123456"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Results":[{"id":"u1","displayName":"opaqueBearer123456"}]}`))
	}))
	defer srv.Close()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CIPP_HOME", home)
	t.Setenv("CIPP_BASE_URL", srv.URL)
	t.Setenv("CIPP_API_KEY", bearer)
	res, err := handleCodeOrchRead(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{
		Arguments: map[string]any{"endpoint_id": "list-users.list", "params": map[string]any{"tenantFilter": "t.example"}},
	}})
	if err != nil {
		t.Fatalf("transport error: %v", err)
	}
	if res == nil || res.IsError {
		t.Fatalf("the read failed: %s", resultText(res))
	}
	if text := resultText(res); strings.Contains(text, bearer) {
		t.Fatalf("the configured bearer came back in a success result:\n%s", text)
	}
	if !strings.Contains(resultText(res), "u1") {
		t.Fatalf("the row was lost: %s", resultText(res))
	}
	_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if b, rerr := os.ReadFile(p); rerr == nil && strings.Contains(string(b), bearer) && !strings.HasSuffix(p, ".toml") {
			t.Errorf("the configured bearer was written to %s", p)
		}
		return nil
	})
}

// TestExecuteSuccessStateWithFailureTextIsAnError: a success state beside
// failure text is conflicting and must come back as an error.
func TestExecuteSuccessStateWithFailureTextIsAnError(t *testing.T) {
	r := executeAgainst(t, `{"Results":[{"state":"success","resultText":"","message":"The operation failed: access denied"}]}`)
	if !r.IsError {
		t.Fatalf("a success state with failure text came back as success: %s", resultText(r))
	}
}
