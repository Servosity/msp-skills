// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): regression test for the final Codex session's
// round-2 input, pinned by skills/cipp/handfixes.json under
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

// TestReadEntityEncodedBearerPastCap: Codex's exact input. CIPP_API_KEY is
// aaaaaaaaaaaa; one JSON value holds the key base64-encoded twice as HTML
// entities, then 1048576 periods. Neither the result nor anything under HOME
// may carry it.
func TestReadEntityEncodedBearerPastCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"echo":"&#87;&#86;&#100;&#71;&#97;&#70;&#108;&#88;&#82;&#109;&#104;&#90;&#86;&#48;&#90;&#111;&#87;&#86;&#100;&#71;&#97;&#65;&#61;&#61;` + strings.Repeat(".", 1048576) + `"}`))
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
	// MCP's JSON output escapes & as \u0026; read it back before looking.
	if strings.Contains(strings.ReplaceAll(resultText(res), `\u0026`, "&"), "&#87;&#86;") {
		t.Fatal("the entity-encoded bearer came back in the result")
	}
	_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasSuffix(p, ".toml") {
			return nil
		}
		if b, rerr := os.ReadFile(p); rerr == nil && strings.Contains(strings.ReplaceAll(string(b), `\u0026`, "&"), "&#87;&#86;") {
			t.Errorf("the entity-encoded bearer was written to %s", p)
		}
		return nil
	})
}
