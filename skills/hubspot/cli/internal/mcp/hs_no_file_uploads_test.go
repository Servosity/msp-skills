// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pins handfixes.json mcp-no-file-uploads. No
// MCP tool uploads a file: hubspot_execute refuses a multipart endpoint before
// any preview or request, no endpoint advertises a file parameter, and an MCP
// multipart value is only ever base64 content, never a host path.

package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"
	"hubspot-pp-cli/internal/cliutil/testenv"
)

func TestExecuteRefusesFileUploadEndpoints(t *testing.T) {
	testenv.Isolate(t)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("HUBSPOT_BASE_URL", srv.URL)
	t.Setenv("HUBSPOT_ACCESS_TOKEN", "pat-na1-"+"7f3c9e2a-b14d-4c8e-9a6f-2d5e8b1c7a30")
	secret := filepath.Join(t.TempDir(), "secret.csv")
	if err := os.WriteFile(secret, []byte("email\nsecret@example.com\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	multipart := 0
	for i := range codeOrchEndpoints {
		ep := &codeOrchEndpoints[i]
		if len(ep.FileParams) > 0 {
			t.Errorf("%s advertises file parameters %v over MCP", ep.ID, ep.FileParams)
		}
		if !ep.Multipart {
			continue
		}
		multipart++
		for _, confirm := range []any{nil, true} {
			args := map[string]any{"endpoint_id": ep.ID, "params": map[string]any{"files": secret, "importRequest": "{}"}}
			if confirm != nil {
				args["confirm"] = confirm
			}
			req := mcplib.CallToolRequest{}
			req.Params.Name = "hubspot_execute"
			req.Params.Arguments = args
			res, err := handleCodeOrchExecute(context.Background(), req)
			if err != nil {
				t.Fatalf("%s: %v", ep.ID, err)
			}
			if res == nil || !res.IsError {
				t.Errorf("%s confirm=%v: a file upload must be refused over MCP, got %+v", ep.ID, confirm, res)
			}
		}
	}
	if multipart == 0 {
		t.Fatal("no multipart endpoint found; the HubSpot CRM import endpoint should be one")
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("the refused upload still sent %d request(s)", n)
	}
}

func TestMultipartValueIsNeverAHostPath(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "secret.csv")
	if err := os.WriteFile(f, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HUBSPOT_MCP_UPLOAD_ROOT", dir)
	for _, v := range []string{f, "secret.csv"} {
		if _, cleanup, err := resolveMCPMultipartFile("files", "", v); err == nil {
			cleanup()
			t.Errorf("value %q was accepted; an MCP multipart value must be base64 content, never a path", v)
		}
	}
}

// hubspot_search never describes an upload hubspot_execute would refuse.
func TestSearchDoesNotAdvertiseUploads(t *testing.T) {
	for i := range codeOrchEndpoints {
		ep := &codeOrchEndpoints[i]
		if !ep.Multipart {
			continue
		}
		meta, _ := codeOrchEndpointMetadata(ep)["multipart"].(map[string]any)
		if _, ok := meta["file_value"]; ok || meta["mcp"] == nil {
			t.Errorf("%s: search metadata advertises an MCP upload: %v", ep.ID, meta)
		}
	}
}
