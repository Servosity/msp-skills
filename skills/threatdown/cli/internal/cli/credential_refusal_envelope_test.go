// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"threatdown-pp-cli/internal/cliutil"
)

// TestRefusedCredentialWriteReportsNotSaved (handfixes.json
// credential-file-private-before-write, Codex r2): a credential write refused
// before any byte landed must not produce a "saved": true receipt in the
// --json error envelope.
func TestRefusedCredentialWriteReportsNotSaved(t *testing.T) {
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
		t.Skip("no inheritable ACL to test here")
	}
	path := filepath.Join(dir, "credentials.toml")
	werr := cliutil.AtomicWriteCredentialFile(path, []byte("access_token = \"tdenv"+"Rk5Tz8Qw3\"\n"), 0o600, 0o700)
	if werr == nil {
		t.Fatal("write was not refused")
	}
	var buf bytes.Buffer
	if !writeCredentialSaveErrorEnvelope(&buf, &rootFlags{asJSON: true}, werr) {
		t.Fatalf("no envelope for %v", werr)
	}
	var env map[string]any
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("envelope is not JSON: %v: %s", err, buf.String())
	}
	if env["saved"] != false {
		t.Fatalf("envelope claims saved=%v for a refused write: %s", env["saved"], buf.String())
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("credentials file exists after a refused write: %v", err)
	}
}
