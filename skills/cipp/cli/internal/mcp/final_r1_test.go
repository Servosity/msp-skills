// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): regression test for the final Codex session's
// round-1 input, pinned by skills/cipp/handfixes.json under
// "success-body-credential-funnel".

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

// TestReadSpacedDoubleBase64Bearer: Codex's exact input. CIPP_API_KEY is
// aaaaaaaaaaaa; one JSON string carries it base64-encoded twice, cut by
// spaces, then 70000 spaces. Neither the result nor anything under HOME may
// carry it.
func TestReadSpacedDoubleBase64Bearer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"echo":"WVdG aFlX RmhZ V0Zo WVdG aA==` + strings.Repeat(" ", 70000) + `"}`))
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
	if strings.Contains(resultText(res), "WVdG") {
		t.Fatal("the spaced double-base64 bearer came back in the result")
	}
	_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasSuffix(p, ".toml") {
			return nil
		}
		if b, rerr := os.ReadFile(p); rerr == nil && strings.Contains(string(b), "WVdG") {
			t.Errorf("the spaced double-base64 bearer was written to %s", p)
		}
		return nil
	})
}
