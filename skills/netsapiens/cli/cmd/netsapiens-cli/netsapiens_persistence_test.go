// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Class test (credential-persistence), the built CLI: no file the program
// writes (HTTP cache, SQLite store, export output, logs) ever carries a held
// credential, raw or encoded. The Codex round-1 inputs are a binary answer
// whose body is the API key (cached as a base64 envelope) and a
// teach-playbook --notes-file holding it (stored raw in the playbooks table).

package main

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"netsapiens-pp-cli/internal/cliutil/testenv"
)

func TestClassNoFileEverHoldsACredential(t *testing.T) {
	cliBinary(t) // before Isolate: the build needs the real module and build caches
	srv, trust := vendorTLS(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = io.WriteString(w, apiKey)
	}))
	env := append(trust, "NETSAPIENS_BASE_URL="+srv.URL, "NETSAPIENS_API_KEY="+apiKey, "NETSAPIENS_RETRY_MAX=0")
	notes := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(notes, []byte("rotate the key "+apiKey+" monthly\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, args := range map[string][]string{
		"http cache":        {"domains", "list", "--data-source", "live", "--json"},
		"export output":     {"export", "domains", "-o", "{home}/export.jsonl"},
		"store (playbooks)": {"teach-playbook", "--query", "find offline devices", "--notes-file", notes},
	} {
		t.Run(name, func(t *testing.T) {
			home := testenv.Isolate(t)
			for i, a := range args {
				args[i] = strings.ReplaceAll(a, "{home}", home)
			}
			r := runCLI(t, env, args...)
			for p, data := range filesUnder(t, home) {
				if f := shown(data, apiKey); f != "" {
					t.Fatalf("%v left the credential (as %q) in %s\nstdout=%s\nstderr=%s", args, f, p, r.stdout, r.stderr)
				}
			}
		})
	}
}
