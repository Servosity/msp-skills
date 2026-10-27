// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): a credentials file refused for its mode still
// gives its values to the holder (credkit-wiring).

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEveryInputIsHeldWhereItIsRead(t *testing.T) {
	t.Parallel()
	cliBin := buildHolderBinary(t, "superops-cli", "../superops-cli")

	// Round-1 input: a credentials file refused for its mode is still read.
	t.Run("a credentials file refused for its mode", func(t *testing.T) {
		const heldValue = "disk-secret-4340x"
		sandbox := t.TempDir()
		cfg := filepath.Join(sandbox, "op", "config.toml")
		creds := filepath.Join(sandbox, "op", "data", "credentials.toml")
		if err := os.MkdirAll(filepath.Dir(creds), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(cfg, []byte("subdomain = \"acme-holder\"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(creds, []byte("api_token = \""+heldValue+"\"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		makeReadableByOthers(t, creds)
		env := holderEnv(sandbox, map[string]string{"SUPEROPS_BASE_URL": "http://127.0.0.1:9"})
		out := runHolderCLI(t, cliBin, env, "", "--config", cfg, "--no-learn", "raw", "mutation", `mutation { f(note:"`+heldValue+`") }`)
		assertRunWithheld(t, sandbox, out, []string{heldValue})
		if !strings.Contains(out, "withheld") {
			t.Errorf("want the echo withheld, not missing:\n%s", out)
		}
	})
}
