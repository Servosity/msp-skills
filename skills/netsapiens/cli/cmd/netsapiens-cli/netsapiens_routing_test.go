// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Class test (credential-routing), the built CLI: an argument can never move a
// credentialed request off the configured server. The Codex round-1 input is
// `sync --resources domains --path-context server=<another host>`.

package main

import (
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"netsapiens-pp-cli/internal/cliutil/testenv"
)

func TestClassArgumentsNeverMoveTheCredentialedServer(t *testing.T) {
	cliBinary(t) // before Isolate: the build needs the real module and build caches
	testenv.Isolate(t)
	var bHits atomic.Int32
	other, _ := vendorTLS(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[]`)
	}))
	configured, trust := vendorTLS(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[]`)
	}))
	env := append(trust,
		"NETSAPIENS_BASE_URL=https://{server}/ns-api/v2",
		"NETSAPIENS_SERVER="+strings.TrimPrefix(configured.URL, "https://"),
		"NETSAPIENS_API_KEY="+apiKey,
		"NETSAPIENS_RETRY_MAX=0",
	)
	otherHost := strings.TrimPrefix(other.URL, "https://")
	for _, args := range [][]string{
		{"sync", "--resources", "domains", "--path-context", "server=" + otherHost, "--json"},
		{"sync", "--resources", "domains", "--path-context", "SERVER=" + otherHost, "--json"},
		{"sync", "--resources", "domains", "--path-context", "host=" + otherHost, "--json"},
	} {
		r := runCLI(t, env, args...)
		if n := bHits.Load(); n != 0 {
			t.Fatalf("%v sent %d credentialed request(s) to another host\nstdout=%s\nstderr=%s", args, n, r.stdout, r.stderr)
		}
	}
}
