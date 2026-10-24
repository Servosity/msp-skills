// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/servosity/handfixes.json under
// the "secret-family-never-persisted" entry. See docs/reprint-survival.md.

package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
)

const mcpSentinel = "mcp-sentinel"

// TestSecretReadsAreGatedOverMCP pins that every listed secret GET exists in
// the endpoint table and needs a confirm, that servosity-msp_read refuses one
// without contacting the API, that servosity-msp_execute previews it and sends
// only with confirm: true, and that the read-only switch refuses it.
func TestSecretReadsAreGatedOverMCP(t *testing.T) {
	matched := map[string]bool{}
	for i := range codeOrchEndpoints {
		ep := &codeOrchEndpoints[i]
		if codeOrchNormalizeMethod(ep.Method) != "GET" || !codeOrchSecretReads[ep.Path] {
			continue
		}
		matched[ep.Path] = true
		if !ep.NeedsConfirm {
			t.Errorf("%s (%s) is a secret read without NeedsConfirm", ep.ID, ep.Path)
		}
	}
	for path := range codeOrchSecretReads {
		if !matched[path] {
			t.Errorf("secret read %s matches no GET endpoint; the list drifted from the table", path)
		}
	}

	var hits atomic.Int32
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"key":"` + mcpSentinel + `","version":1}`))
	}))
	defer fixture.Close()
	requestContractConfig(t, fixture.URL)

	call := func(args map[string]any) string {
		var req mcplib.CallToolRequest
		req.Params.Arguments = args
		var res *mcplib.CallToolResult
		var err error
		if args["confirm"] == nil && args["execute"] == nil {
			res, err = handleCodeOrchRead(context.Background(), req)
		} else {
			delete(args, "execute")
			res, err = handleCodeOrchExecute(context.Background(), req)
		}
		if err != nil {
			t.Fatalf("MCP call %v: %v", args, err)
		}
		var b strings.Builder
		for _, c := range res.Content {
			if tc, ok := c.(mcplib.TextContent); ok {
				b.WriteString(tc.Text)
			}
		}
		return b.String()
	}
	params := map[string]any{"id": 21}
	const id = "restic-backups.encryption-key.restic-backups-read"

	if out := call(map[string]any{"endpoint_id": id, "params": params}); strings.Contains(out, mcpSentinel) || hits.Load() != 0 {
		t.Fatalf("servosity-msp_read returned or fetched a secret read: %s (requests %d)", out, hits.Load())
	}
	if out := call(map[string]any{"endpoint_id": id, "params": params, "execute": true}); strings.Contains(out, mcpSentinel) || hits.Load() != 0 {
		t.Fatalf("servosity-msp_execute without confirm sent the secret read: %s (requests %d)", out, hits.Load())
	}
	if out := call(map[string]any{"endpoint_id": id, "params": params, "confirm": true}); !strings.Contains(out, mcpSentinel) || hits.Load() != 1 {
		t.Fatalf("servosity-msp_execute with confirm: true did not send the read: %s (requests %d)", out, hits.Load())
	}
	t.Setenv("SERVOSITY_MSP_MCP_READ_ONLY", "1")
	if out := call(map[string]any{"endpoint_id": id, "params": params, "confirm": true}); strings.Contains(out, mcpSentinel) || hits.Load() != 1 {
		t.Fatalf("the read-only switch let a secret read through: %s (requests %d)", out, hits.Load())
	}
}
