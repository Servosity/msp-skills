// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package platform

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"cisco-umbrella-pp-cli/internal/credkit"
)

// The fingerprint key goes through the credential kit (handfixes.json
// credkit-every-file-write), which withholds a random key that happens to hold a
// one-character credential. A key it withholds is an error and leaves nothing behind,
// so the next call draws a new one instead of failing on an empty file; a key it
// passes is the file's content. In a child process: a hold lasts until the process
// exits, and would withhold the rest of this package's files.
func TestFingerprintKeyGoesThroughTheKit(t *testing.T) {
	if os.Getenv("UMBRELLA_FINGERPRINT_CHILD") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestFingerprintKeyGoesThroughTheKit$", "-test.count=1", "-test.v")
		cmd.Env = append(os.Environ(), "UMBRELLA_FINGERPRINT_CHILD=1", "XDG_STATE_HOME="+t.TempDir())
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		return
	}
	credkit.Hold("stdin", "\x00")
	root, err := stateRoot()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "cache-key")
	var withheld, passed int
	for i := 0; i < 300; i++ {
		_ = os.Remove(path)
		key, err := loadOrCreateFingerprintKey()
		if errors.Is(err, credkit.ErrWithheld) {
			withheld++
			if _, statErr := os.Lstat(path); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("a withheld key left a file behind: %v", statErr)
			}
			continue
		}
		if err != nil {
			t.Fatalf("draw %d: %v", i, err)
		}
		passed++
		if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, key) || len(key) != 32 {
			t.Fatalf("draw %d: the file holds %x (%v), want the 32-byte key %x", i, got, err, key)
		}
	}
	if withheld == 0 || passed == 0 {
		t.Fatalf("%d withheld, %d passed: this test needs both to prove anything", withheld, passed)
	}
}
