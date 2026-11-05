//go:build darwin

// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json credential-file-private-before-bytes):
// a directory whose inherited ACL lets another user read new files gets no
// credential bytes: the write is refused before anything is written, and a file
// already there is left as it was.

package cliutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCredentialFileRefusedUnderAnInheritedReadACL(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "credentials.toml")
	if err := os.WriteFile(path, []byte("access_token = 'OlderToken'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("chmod", "+a", "everyone allow read,execute,file_inherit,directory_inherit", dir) // #nosec G204 -- fixed ACL spec on a temp dir this test created.
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("chmod +a: %v\n%s", err, out)
	}
	const secret = "NewTenantToken-71c3e9b05d2a4f68"
	err := AtomicWriteCredentialFile(path, []byte("access_token = '"+secret+"'\n"), 0o600, 0o700)
	if err == nil {
		t.Fatal("a credential file under an inherited read ACL was written")
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		b, _ := os.ReadFile(filepath.Join(dir, e.Name())) // #nosec G304 -- files in a temp dir this test created.
		if strings.Contains(string(b), secret) {
			t.Fatalf("%s holds the credential after a refused write", e.Name())
		}
	}
	if b, _ := os.ReadFile(path); string(b) != "access_token = 'OlderToken'\n" { // #nosec G304 -- temp file.
		t.Fatalf("existing file changed: %q", b)
	}
}
