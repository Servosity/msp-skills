// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): regression tests for the fresh Codex round-2
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

// TestReadSuccessBodyNumberEncodedBearer: Codex's input (the configured API
// key below) and a success body whose JSON number is the bearer's hex encoding. Neither the
// tool result nor anything under HOME may carry it in any form.
func TestReadSuccessBodyNumberEncodedBearer(t *testing.T) {
	const bearer = "abcdef1234567890"
	const hexForm = "61626364656631323334353637383930"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Results":[{"id":"u1","note":61626364656631323334353637383930}]}`))
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
	text := resultText(res)
	if strings.Contains(text, hexForm) || strings.Contains(text, bearer) {
		t.Fatalf("the bearer's hex form came back in the result:\n%s", text)
	}
	_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasSuffix(p, ".toml") {
			return nil
		}
		if b, rerr := os.ReadFile(p); rerr == nil && (strings.Contains(string(b), hexForm) || strings.Contains(string(b), bearer)) {
			t.Errorf("the bearer was written to %s", p)
		}
		return nil
	})
}

// TestExecuteSuccessLeadStillConflicts: a line that opens "Successfully" and
// then reports a failure is conflicting, an error.
func TestExecuteSuccessLeadStillConflicts(t *testing.T) {
	r := executeAgainst(t, `{"Results":[{"state":"success","message":"Successfully reset password; failed to disable sign-in"}]}`)
	if !r.IsError {
		t.Fatalf("a success line with a failure came back as success: %s", resultText(r))
	}
}
