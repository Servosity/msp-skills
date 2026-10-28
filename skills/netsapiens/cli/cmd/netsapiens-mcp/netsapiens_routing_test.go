// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Class test (credential-routing), the built MCP server and its companion CLI:
// the Codex round-1 input, sync with path-context server=<another host>, never
// reaches the other host.

package main

import (
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"netsapiens-pp-cli/internal/cliutil/testenv"
)

func TestClassMCPPathContextNeverMovesTheServer(t *testing.T) {
	bin := mcpBinary(t) // before Isolate: the build needs the real module and build caches
	cli := filepath.Join(filepath.Dir(bin), "netsapiens-cli")
	if runtime.GOOS == "windows" {
		cli += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", cli, "../netsapiens-cli").CombinedOutput(); err != nil {
		t.Fatalf("build cli: %v\n%s", err, out)
	}
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
		"NETSAPIENS_API_KEY=RoutingApiKey55443322",
		"NETSAPIENS_RETRY_MAX=0",
		"NETSAPIENS_CLI_PATH="+cli,
	)
	got := callTool(t, bin, env, "sync", map[string]any{"resources": []string{"domains"}, "path-context": []string{"server=" + strings.TrimPrefix(other.URL, "https://")}})
	if n := bHits.Load(); n != 0 {
		t.Fatalf("the MCP sync tool sent %d credentialed request(s) to another host:\n%s", n, got)
	}
}
