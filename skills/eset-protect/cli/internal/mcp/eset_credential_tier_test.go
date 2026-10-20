// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"eset-protect-pp-cli/internal/cli"
	"eset-protect-pp-cli/internal/cliutil"
	"eset-protect-pp-cli/internal/mcp/cobratree"
	"eset-protect-pp-cli/internal/store"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const credentialTierSentinel = "CREDENTIAL-TIER-SENTINEL"

var credentialTierEndpointIDs = []string{
	"installers.list",
	"installers.get",
	"gpo-sccm-files-generate.installers-generate-gpo-sccm-file",
	"mobile-devices-batch-get-enrollment-links.v1_mobile_devices_batch_get_enrollment_links",
	"quarantined-objects-download.quarantined-objects",
	"quarantined-objects-batch-download.quarantined-objects",
}

func executeEndpoint(t *testing.T, id string, params map[string]any) (*mcplib.CallToolResult, string) {
	t.Helper()
	res, err := handleCodeOrchExecute(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{Arguments: map[string]any{"endpoint_id": id, "params": params}}})
	if err != nil {
		t.Fatalf("%s: transport error: %v", id, err)
	}
	return res, toolText(t, res)
}

// TestReadOnlyRefusesCredentialTierReads guards the hand-fix
// mcp-read-only-refuses-credential-tier: with ESET_PROTECT_MCP_READ_ONLY=1,
// eset-protect_execute refuses the Credential-tier reads (installers.list and
// .get are GETs, so the method gate alone served them) before any request
// leaves; normal mode still serves them, and other GETs stay readable.
func TestReadOnlyRefusesCredentialTierReads(t *testing.T) {
	var mu sync.Mutex
	var hit []string
	esetFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hit = append(hit, r.Method+" "+r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/installers":
			_, _ = io.WriteString(w, `{"installers":[{"uuid":"00000000-0000-0000-0000-0000000000b1","downloadUrl":"https://download.example/`+credentialTierSentinel+`"}],"nextPageToken":""}`)
		case "/v1/devices":
			_, _ = io.WriteString(w, `{"devices":[{"uuid":"00000000-0000-0000-0000-000000000001","displayName":"FRONTDESK-01"}],"nextPageToken":""}`)
		default:
			_, _ = io.WriteString(w, `{"link":"`+credentialTierSentinel+`"}`)
		}
	})
	params := map[string]any{"installerUuid": "00000000-0000-0000-0000-0000000000b1"}

	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	for _, id := range credentialTierEndpointIDs {
		res, text := executeEndpoint(t, id, params)
		if !res.IsError || !strings.Contains(text, "Credential-tier") {
			t.Fatalf("%s was not refused as Credential tier in read-only mode: isError=%v %s", id, res.IsError, text)
		}
		if strings.Contains(text, credentialTierSentinel) {
			t.Fatalf("%s returned Credential-tier data in read-only mode: %s", id, text)
		}
	}
	mu.Lock()
	reached := append([]string(nil), hit...)
	mu.Unlock()
	if len(reached) > 0 {
		t.Fatalf("a refused Credential-tier read still reached ESET: %v", reached)
	}
	if res, text := executeEndpoint(t, "devices.list", nil); res.IsError || !strings.Contains(text, "FRONTDESK-01") {
		t.Fatalf("read-only mode must still serve ordinary GETs: %s", text)
	}

	t.Setenv(cliutil.MCPReadOnlyEnvVar, "")
	if res, text := executeEndpoint(t, "installers.list", nil); res.IsError || !strings.Contains(text, credentialTierSentinel) {
		t.Fatalf("normal mode must still serve installers.list: %s", text)
	}
}

// TestReadOnlySQLAndSearchRefuseAMirrorHoldingInstallerLinks covers the mirror
// sibling of the same hand-fix. A mirror this build wrote (the store writes
// installer rows with downloadUrl redacted, credential-tier-never-persisted)
// is served by read-only sql and search, so one `installers list` no longer
// locks a read-only agent out; a mirror an earlier build wrote, still holding
// a real link, is refused; and a read-only analytics call naming installers
// is refused before the CLI runs.
func TestReadOnlySQLAndSearchRefuseAMirrorHoldingInstallerLinks(t *testing.T) {
	resetMCPPathEnv(t)
	path, err := mcpDBPath()
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.UpsertBatch("devices", []json.RawMessage{json.RawMessage(`{"uuid":"00000000-0000-0000-0000-000000000001","displayName":"FRONTDESK-01"}`)}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.UpsertBatch("installers", []json.RawMessage{json.RawMessage(`{"uuid":"00000000-0000-0000-0000-0000000000b1","displayName":"INSTALLERNAME","downloadUrl":"https://download.example/` + credentialTierSentinel + `"}`)}); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	sqlQuery := map[string]any{"query": "SELECT data FROM resources"}
	searchQuery := map[string]any{"query": "INSTALLERNAME"}
	calls := map[string]func() (*mcplib.CallToolResult, error){
		"sql": func() (*mcplib.CallToolResult, error) {
			return handleSQL(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{Arguments: sqlQuery}})
		},
		"search": func() (*mcplib.CallToolResult, error) {
			return handleSearch(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{Arguments: searchQuery}})
		},
	}

	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	for name, call := range calls {
		res, err := call()
		if err != nil {
			t.Fatal(err)
		}
		text := mcpTextContent(t, res)
		if res.IsError || !strings.Contains(text, "INSTALLERNAME") || !strings.Contains(text, cliutil.PersistRedactedValue) || strings.Contains(text, credentialTierSentinel) {
			t.Fatalf("read-only %s must serve a mirror this build wrote (installer row present, link redacted): isError=%v %s", name, res.IsError, text)
		}
	}

	// A row an earlier build stored with a real link: refused, never served.
	db, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`UPDATE resources SET data = ? WHERE resource_type = 'installers'`, `{"uuid":"00000000-0000-0000-0000-0000000000b1","displayName":"INSTALLERNAME","downloadUrl":"https://download.example/`+credentialTierSentinel+`"}`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	for name, call := range calls {
		res, err := call()
		if err != nil {
			t.Fatal(err)
		}
		text := mcpTextContent(t, res)
		if !res.IsError || !strings.Contains(text, "Credential tier") || !strings.Contains(text, "earlier build") || strings.Contains(text, credentialTierSentinel) {
			t.Fatalf("read-only %s served a mirror holding a real installer link: isError=%v %s", name, res.IsError, text)
		}
	}
	s := server.NewMCPServer("test", "0")
	cobratree.RegisterAll(s, cli.RootCmd(), func() (string, error) { return "/nonexistent/eset-protect-cli", nil })
	analytics := s.GetTool("analytics")
	if analytics == nil {
		t.Fatal("analytics tool is not offered in read-only mode")
	}
	res, err := analytics.Handler(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{Arguments: map[string]any{"type": "installers", "group-by": "downloadUrl"}}})
	if err != nil {
		t.Fatal(err)
	}
	if text := toolText(t, res); !res.IsError || !strings.Contains(text, "Credential tier") {
		t.Fatalf("read-only analytics --type installers was not refused: %s", text)
	}

	t.Setenv(cliutil.MCPReadOnlyEnvVar, "")
	res, err = handleSQL(context.Background(), mcplib.CallToolRequest{Params: mcplib.CallToolParams{Arguments: sqlQuery}})
	if err != nil || res.IsError || !strings.Contains(mcpTextContent(t, res), "INSTALLERNAME") {
		t.Fatalf("normal-mode sql must still read the mirror: %v %s", err, mcpTextContent(t, res))
	}
}

// TestCodeOrchExecuteNeverPersistsInstallerLinks is the MCP twin of
// TestCredentialTierNeverPersisted: eset-protect_execute returns the installer
// link live in normal mode and leaves no byte of it under the home.
func TestCodeOrchExecuteNeverPersistsInstallerLinks(t *testing.T) {
	esetFakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		inst := `{"uuid":"00000000-0000-0000-0000-0000000000b1","displayName":"Acme agent","downloadUrl":"https://download.example/` + credentialTierSentinel + `"}`
		if r.URL.Path == "/v1/installers" {
			_, _ = io.WriteString(w, `{"installers":[`+inst+`],"nextPageToken":""}`)
			return
		}
		_, _ = io.WriteString(w, `{"installer":`+inst+`}`)
	})
	home := resetMCPPathEnv(t)
	for _, call := range []struct {
		id     string
		params map[string]any
	}{{"installers.list", nil}, {"installers.get", map[string]any{"installerUuid": "00000000-0000-0000-0000-0000000000b1"}}} {
		res, text := executeEndpoint(t, call.id, call.params)
		if res.IsError || !strings.Contains(text, credentialTierSentinel) {
			t.Fatalf("%s must return the link live: %s", call.id, text)
		}
	}
	var hits []string
	_ = filepath.WalkDir(home, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if b, rerr := os.ReadFile(p); rerr == nil && strings.Contains(string(b), credentialTierSentinel) {
				hits = append(hits, p)
			}
		}
		return nil
	})
	if len(hits) > 0 {
		t.Fatalf("eset-protect_execute wrote an installer link to disk: %v", hits)
	}
}
