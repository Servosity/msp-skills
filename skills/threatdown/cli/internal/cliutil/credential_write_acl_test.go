// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestCredentialWriteRefusesInheritedBroadAccess (handfixes.json
// credential-file-private-before-write, Codex r1): a directory whose ACL hands
// read access to every user on each new file (THREATDOWN_DATA_DIR pointed at
// a shared folder) used to receive the credential store anyway: the bytes were
// written and the file published, and only then did the permission check fail,
// leaving the readable file behind. Every credential-source write now checks
// the new file before any secret byte lands, and leaves nothing behind.
func TestCredentialWriteRefusesInheritedBroadAccess(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "shared")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	switch runtime.GOOS {
	case "darwin":
		if out, err := exec.Command("/bin/chmod", "+a", "everyone allow read,file_inherit", dir).CombinedOutput(); err != nil {
			t.Skipf("chmod +a failed (environment): %v %s", err, out)
		}
	case "windows":
		if out, err := exec.Command("icacls", dir, "/grant", "*S-1-5-32-545:(OI)(CI)R").CombinedOutput(); err != nil {
			t.Skipf("icacls grant failed (environment): %v %s", err, out)
		}
	default:
		t.Skip("POSIX mode bits are set on the new file itself; no inheritable ACL to test here")
	}
	const secret = "tdstore" + "Hq4Vb7Nm2"
	path := filepath.Join(dir, "credentials.toml")
	err := AtomicWriteCredentialFile(path, []byte("access_token = \""+secret+"\"\n"), 0o600, 0o700)
	if err == nil {
		t.Fatal("AtomicWriteCredentialFile wrote a credential into a directory that grants every user read access")
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		data, _ := os.ReadFile(filepath.Join(dir, e.Name()))
		if strings.Contains(string(data), secret) {
			t.Fatalf("%s carries the credential after a refused write", e.Name())
		}
	}
}
