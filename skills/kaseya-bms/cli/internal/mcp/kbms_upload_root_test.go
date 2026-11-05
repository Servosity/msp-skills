// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

// kaseya-bms handfix mcp-arguments-never-name-a-host-file (Codex review r1
// class host file read): a multipart file argument is file content (base64),
// never a host path, whatever KASEYA_BMS_MCP_UPLOAD_ROOT says.

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestMCPFileArgumentNeverNamesAHostFile(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(dir, "passwd")
	if err := os.WriteFile(secret, []byte("root:x:0:0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(mcpUploadRootEnvVar, dir)
	// A path argument is refused, or (when the path text happens to be valid
	// base64, as a 64-character Linux temp path is) decoded as content; either
	// way the upload never carries the host file's bytes.
	for _, arg := range []string{"passwd", secret, "/tmp/TestMCPFileArgumentNeverNamesAHostFile3988820707/001/passwd"} {
		p, cleanup, err := resolveMCPMultipartFile("File", "", arg)
		if err != nil {
			continue
		}
		b, _ := os.ReadFile(p)
		cleanup()
		if bytes.Contains(b, []byte("root:x:0:0")) {
			t.Fatalf("MCP argument %q named a host file under the upload root and was read (%s)", arg, p)
		}
	}
	// Content still uploads.
	p, cleanup, err := resolveMCPMultipartFile("File", "note.txt", "aGVsbG8=")
	if err != nil {
		t.Fatalf("base64 content was refused: %v", err)
	}
	defer cleanup()
	if b, _ := os.ReadFile(p); string(b) != "hello" {
		t.Fatalf("decoded upload = %q", b)
	}
}
