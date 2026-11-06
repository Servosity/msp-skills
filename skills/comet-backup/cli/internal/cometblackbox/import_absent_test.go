// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cometblackbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Class write-safety (comet-backup handfix: import-no-storage-vaults): no
// bulk import can request Storage Vaults, from the CLI binary or from the MCP
// server's import tool. request-storage-vault adds a vault on every call, so
// a rerun of a bulk import after a crash or a dropped reply created the same
// vault again (internal/cli TestImportWriteSimulation). The MCP import tool
// shells out to this CLI and cannot pass an --input path (a blocked input
// flag), so it fails before any record is read; the CLI refuses storage as an
// unknown resource. The control proves the same input and binaries do reach
// the server for users.
func TestImportStorageAbsentFromCLIAndMCP(t *testing.T) {
	t.Parallel()
	sv := echoServer(t)
	e := newEnv(t, sv, false)
	input := filepath.Join(e.home, "vaults.jsonl")
	if err := os.WriteFile(input, []byte(`{"TargetUser":"acme","StorageProvider":"prov-1"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hits := func(method string) int { sv.mu.Lock(); defer sv.mu.Unlock(); return sv.hits[method] }

	out := e.run(t, "import", "storage", "--input", input, "--yes", "--json")
	if !strings.Contains(out, "unknown writable resource") {
		t.Errorf("CLI import storage was not refused as an unknown resource:\n%.800s", out)
	}
	mcpOut := e.mcpCalls(t, []map[string]any{
		{"name": "import", "arguments": map[string]any{"resource": "storage", "confirm": true}},
	})
	if !strings.Contains(mcpOut, `"isError":true`) {
		t.Errorf("MCP import storage did not fail:\n%.1500s", mcpOut)
	}
	if n := hits("request-storage-vault"); n != 0 {
		t.Fatalf("import requested %d Storage Vault(s)", n)
	}

	// Control: the CLI import still writes users through the same server.
	before := hits("add-user")
	out = e.run(t, "import", "users", "--input", input, "--yes", "--json")
	if hits("add-user") != before+1 {
		t.Fatalf("control: import users did not reach the server:\n%.800s", out)
	}
}
