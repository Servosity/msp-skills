// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): regression tests for the fresh Codex round-3
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
	"unicode"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

func dropFormat(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, s)
}

// TestReadSuccessBodyZeroWidthSplitBearer: Codex's input. The configured API
// key split by 200000 U+200B characters in a success body must not come back
// or reach the disk in a form that recovers it.
func TestReadSuccessBodyZeroWidthSplitBearer(t *testing.T) {
	const bearer = "abcdefghijklmnop"
	note := "abcdefgh" + strings.Repeat("\u200b", 200000) + "ijklmnop"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Results":[{"id":"u1","note":"` + note + `"}]}`))
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
	if strings.Contains(dropFormat(resultText(res)), bearer) {
		t.Fatal("the zero-width-split bearer came back in the result")
	}
	_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasSuffix(p, ".toml") {
			return nil
		}
		if b, rerr := os.ReadFile(p); rerr == nil && strings.Contains(dropFormat(string(b)), bearer) {
			t.Errorf("the bearer was written to %s", p)
		}
		return nil
	})
}

// TestExecuteSuccessStateWithErrorFieldFails: Codex's input. A populated
// error field beside a success state is conflicting, an error, whatever its
// wording.
func TestExecuteSuccessStateWithErrorFieldFails(t *testing.T) {
	r := executeAgainst(t, `{"Results":[{"state":"success","error":"HTTP 403"}]}`)
	if !r.IsError {
		t.Fatalf("a success state with a populated error came back as success: %s", resultText(r))
	}
}
