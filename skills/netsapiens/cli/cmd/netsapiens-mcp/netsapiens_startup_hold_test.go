// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Class test (credential-output, startup ordering), the built MCP server: a
// config or credentials file that fails to parse still has every value in it
// held before any startup diagnostic can print.

package main

import (
	"bytes"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"netsapiens-pp-cli/internal/cliutil/testenv"
)

func TestClassMCPMalformedConfigNeverPrintsACredential(t *testing.T) {
	bin := mcpBinary(t) // before Isolate: the build needs the real module and build caches
	const secret = "StartupSecret55443322"
	// The duplicate key is the credential, so the parser's diagnostic repeats
	// it: under [headers] for a config (a map), top-level for the credentials
	// file (its parser skips unknown tables).
	body := func(pad int, table bool) string {
		dup := secret + " = \"a\"\n" + secret + " = \"b\"\n"
		if table {
			dup = "[headers]\n" + dup
		}
		return strings.Repeat("# padding line before the credential\n", pad) + "api_key = \"" + secret + "\"\n" + dup
	}
	cases := []struct {
		name  string
		file  func(home string) string
		pad   int
		table bool
	}{
		{"legacy config (Codex round 1)", func(home string) string { return filepath.Join(home, ".config", "netsapiens-cli", "config.toml") }, 0, true},
		{"legacy config, credential past the kit stream window", func(home string) string { return filepath.Join(home, ".config", "netsapiens-cli", "config.toml") }, 1000, true},
		{"credentials file", func(home string) string { return filepath.Join(home, "data", "credentials.toml") }, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := testenv.Isolate(t)
			p := tc.file(home)
			if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(body(tc.pad, tc.table)), 0o600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(bin, "--help")
			cmd.Env = append(os.Environ(), "NETSAPIENS_API_KEY=", "NETSAPIENS_CONFIG_DIR="+filepath.Join(home, "cfg"), "NETSAPIENS_DATA_DIR="+filepath.Join(home, "data"))
			var out bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &out
			_ = cmd.Run()
			for _, f := range []string{secret, base64.StdEncoding.EncodeToString([]byte(secret))} {
				if strings.Contains(out.String(), f) {
					t.Fatalf("%q reached the terminal:\n%s", f, out.String())
				}
			}
		})
	}
}
