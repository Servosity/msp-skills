// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package autotaskblackbox

// Credential classes the shared credential kit closes for autotask, each on
// the built binaries (handfixes.json credkit-wiring). Each was seen failing
// with the kit's wiring disabled (nothing declared, the connector's own holds
// a no-op) before it passed; (d), which passes with the wiring disabled, was
// seen failing with the MCP main exiting through os.Exit, so the kit's pipes
// were not drained. AUTOTASK_BLACKBOX_BIN_DIR runs them against such a build.
// (b), a short user name reflected base64-encoded twice, and (f), the user name
// sent to another host, are gone: the API user name is an identifier, not a
// held credential (user_name_identifier_test.go).

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// kitEnv is a clean environment against the fake at base, with the three
// Autotask credentials given (code, user, secret), behind the recording proxy.
func kitEnv(t *testing.T, base, caFile, code, user, secret string) *bbEnv {
	t.Helper()
	home := t.TempDir()
	env := baseEnv(home, &server{url: base, caFile: caFile})
	env = append(env, "AUTOTASK_API_INTEGRATION_CODE="+code, "AUTOTASK_PSA_USER_NAME="+user, "AUTOTASK_PSA_SECRET="+secret)
	return &bbEnv{home: home, env: env, mode: credEnv}
}

// caFileFor writes srv's certificate as a PEM trust bundle.
func caFileFor(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	return ca
}

// (a) A credential split across two exported records: page 1 of the id walk
// is padding rows (about 12.8 KB) ending with a record that carries the first
// half of AUTOTASK_PSA_SECRET, page 2 starts with a record that carries the
// second half. The export file never holds the secret whole or both halves.
func TestExportSplitAcrossRecordsNeverLandsWhole(t *testing.T) {
	t.Parallel()
	const secret = "RecordSplitSecretABCDEFGH"
	first, second := halves(secret)
	var mu sync.Mutex
	pages := 0
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		if !strings.HasSuffix(r.URL.Path, "/Tickets/query") {
			fmt.Fprint(w, `{"items":[],"pageDetails":{"count":0,"requestCount":100,"prevPageUrl":null,"nextPageUrl":null}}`)
			return
		}
		limit := 100
		var q map[string]any
		if json.Unmarshal(body, &q) == nil {
			for k, v := range q {
				if n, ok := v.(float64); ok && strings.EqualFold(k, "maxRecords") && n > 1 {
					limit = int(n)
				}
			}
		}
		var items []string
		switch from := walkFrom(r, body); {
		case from < 0:
			pad := strings.Repeat("p", 12800/limit)
			for i := 1; i < limit; i++ {
				items = append(items, fmt.Sprintf(`{"id":%d,"title":"pad-%s"}`, i, pad))
			}
			items = append(items, fmt.Sprintf(`{"id":%d,"title":%q}`, limit, first))
		case from == float64(limit):
			items = append(items, fmt.Sprintf(`{"id":%d,"title":%q}`, limit+1, second))
		}
		mu.Lock()
		if len(items) > 0 {
			pages++
		}
		mu.Unlock()
		fmt.Fprintf(w, `{"items":[%s],"pageDetails":{"count":%d,"requestCount":%d,"prevPageUrl":null,"nextPageUrl":null}}`,
			strings.Join(items, ","), len(items), limit)
	}))
	e := kitEnv(t, srv.URL+apiBase, caFileFor(t, srv), bbCode, bbUser, secret)
	file := filepath.Join(e.home, "split.jsonl")
	out := e.run(t, "export", "tickets", "--format", "jsonl", "--output", file)
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("export wrote no file: %v\n%.1500s", err, out)
	}
	mu.Lock()
	n := pages
	mu.Unlock()
	if n != 2 || !bytes.Contains(data, []byte("pad-pp")) {
		t.Fatalf("the export did not walk both pages and write the padding (%d pages):\n%.1500s\n%.500s", n, out, data)
	}
	for where, text := range map[string]string{"the export file": string(data), "the output": out} {
		if strings.Contains(text, secret) || (strings.Contains(text, first) && strings.Contains(text, second)) {
			t.Errorf("%s holds the secret split across the two records", where)
		}
	}
}

// (c) A credential in a malformed config never appears in startup output: a
// base_url whose password the URL parser quotes back (the host does not
// parse), and a config.toml that is no TOML document at all. CLI and MCP.
func TestMalformedConfigCredentialNeverPrints(t *testing.T) {
	t.Parallel()
	const pw, broken = "MalformedCfgPassword81x3", "BrokenTomlSecret-55q2w"
	for name, body := range map[string]string{
		"base_url that does not parse": `base_url = "https://ro:` + pw + `@[bad"` + "\n",
		"config that is no TOML":       `psa_secret = "` + broken + "\n" + `api_integration_code = ` + broken + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			cfg := filepath.Join(home, "config.toml")
			if err := os.WriteFile(cfg, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			e := &bbEnv{home: home, env: append(cleanEnv(home), "AUTOTASK_CONFIG="+cfg), mode: credConfigFile}
			out := e.run(t, "tickets", "query-item", "1", "--json")
			out += e.run(t, "doctor")
			mcp := e.mcpCalls(t, []map[string]any{{"name": "autotask_read", "arguments": map[string]any{"endpoint_id": "tickets.query-item", "params": map[string]any{"id": "1"}}}})
			if !strings.Contains(mcp, `"id":1`) {
				t.Fatalf("the MCP call was never answered; the test did not run:\n%.1500s", mcp)
			}
			for _, v := range []string{pw, broken} {
				if strings.Contains(out+mcp, v) {
					t.Errorf("startup output repeats %q:\nCLI: %.1500s\nMCP: %.1500s", v, out, mcp)
				}
			}
		})
	}
}

// (d) autotask-mcp --transport http with no AUTOTASK_MCP_HTTP_TOKEN refuses
// to serve: the refusal is on stderr and the exit status is not zero.
func TestMCPHTTPWithoutTokenRefusesOnStderr(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bbMCP, "--transport", "http", "--addr", "127.0.0.1:0")
	cmd.Env, cmd.Dir = cleanEnv(home), home
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("autotask-mcp served without a token until the deadline:\nstderr: %s", stderr.String())
	}
	if err == nil {
		t.Errorf("autotask-mcp --transport http without a token exited 0")
	}
	if !strings.Contains(stderr.String(), "AUTOTASK_MCP_HTTP_TOKEN") {
		t.Errorf("want the refusal naming AUTOTASK_MCP_HTTP_TOKEN on stderr, got stderr %q, stdout %q", stderr.String(), stdout.String())
	}
}
