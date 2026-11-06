// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

// An MCP argument is file content, never a path this server opens, whatever the
// environment says.
func TestMCPUploadIsContentNeverAPath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "report.txt"), []byte("host file"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("JAMF_PRO_MCP_UPLOAD_ROOT", dir)
	if path, cleanup, err := resolveMCPMultipartFile("file", "", "report.txt"); err == nil {
		cleanup()
		t.Fatalf("a named host file was staged for upload at %s", path)
	}
	path, cleanup, err := resolveMCPMultipartFile("file", "report.txt", base64.StdEncoding.EncodeToString([]byte("content")))
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "content" {
		t.Fatalf("staged %q", data)
	}
}
