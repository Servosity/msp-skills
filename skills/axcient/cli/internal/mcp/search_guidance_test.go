package mcp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"axcient-pp-cli/internal/store"
	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// TestHandleSearch_Issue148_Guidance verifies the MCP search tool gives
// actionable guidance instead of a cryptic SQLite error (no local DB) or a bare
// `null` (no matches), while still returning the record on a real match.
func TestHandleSearch_Issue148_Guidance(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	mkReq := func(q string) mcplib.CallToolRequest {
		var req mcplib.CallToolRequest
		req.Params.Arguments = map[string]any{"query": q}
		return req
	}

	// (1) No DB yet -> clear "run sync" error, never the cryptic SQLite text.
	r1, err := handleSearch(context.Background(), mkReq("anything"))
	if err != nil {
		t.Fatal(err)
	}
	if !r1.IsError {
		t.Fatalf("missing-DB search should be an error result")
	}
	txt1 := mcpTextContent(t, r1)
	if !strings.Contains(txt1, "sync") {
		t.Fatalf("missing-DB error should mention sync, got %q", txt1)
	}
	if strings.Contains(strings.ToLower(txt1), "out of memory") {
		t.Fatalf("missing-DB error leaked the cryptic SQLite message: %q", txt1)
	}

	// Populate a synced DB at the resolved dbPath().
	dbDir := filepath.Join(home, ".local", "share", "axcient-cli")
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(filepath.Join(dbDir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.UpsertBatch("device", []json.RawMessage{json.RawMessage(`{"id_":1,"name":"AcmeServer01"}`)}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	// (2) No match -> a hint, not a bare "null".
	r2, err := handleSearch(context.Background(), mkReq("NoSuchNameXyz"))
	if err != nil {
		t.Fatal(err)
	}
	txt2 := mcpTextContent(t, r2)
	if strings.TrimSpace(txt2) == "null" || !strings.Contains(txt2, "No matches") {
		t.Fatalf("no-match search should return a 'No matches' hint, got %q", txt2)
	}

	// (3) Real match still returns the record as JSON.
	r3, err := handleSearch(context.Background(), mkReq("AcmeServer01"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(mcpTextContent(t, r3), "AcmeServer01") {
		t.Fatalf("matching search should return the record")
	}
}
