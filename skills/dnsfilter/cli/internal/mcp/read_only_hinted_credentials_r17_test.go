// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "mcp-read-only-hinted-tools-filter-credentials" entry.

package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dnsfilter-pp-cli/internal/cliutil"
	"dnsfilter-pp-cli/internal/mcp/bound"
	"dnsfilter-pp-cli/internal/mcp/redact"
	"dnsfilter-pp-cli/internal/store"
	"github.com/mark3labs/mcp-go/server"
)

// r17FakeCLIEnv turns this test binary into a stand-in companion CLI for the
// tests below (DNSFILTER_CLI_PATH, or the recipe intent's path, points at
// os.Args[0]). It is read in init, so it needs no TestMain mode.
const r17FakeCLIEnv = "DNSFILTER_MCP_TEST_R17_FAKE_CLI"

func init() {
	if mode := os.Getenv(r17FakeCLIEnv); mode != "" {
		os.Exit(runR17FakeCLI(mode))
	}
}

// Low-entropy sentinels, one per path, so a hit names the path that leaked.
const (
	r17APIErrorSecret = "SENTINEL-NETWORK-KEY"
	r17PinTextSecret  = "SENTINEL-PIN-TEXT-0042"
	r17ResultSecret   = "SENTINEL-RESULT-NETWORK-KEY"
	r17HintSecret     = "SENTINEL-HINT-PIN-0077"
	r17TailCutSecret  = "SENTINEL-TAIL-CUT-KEY"
	r17StoreSecret    = "SENTINEL-STORE-NETWORK-KEY"
	r17EchoSecret     = "SENTINEL-ECHO-TOKEN-0099"
)

func runR17FakeCLI(mode string) int {
	switch mode {
	case "api-error-secret":
		// dnsfilter-cli why-blocked's own stderr (captured over stdio
		// 2026-10-20) when GET /v1/organizations/1 answers HTTP 422 with a
		// body that echoes a credential.
		fmt.Fprintln(os.Stderr, `Error: GET /v1/organizations/1 returned HTTP 422: {"error": "request failed", "secret_key": "`+r17APIErrorSecret+`"}`)
		return 5
	case "pin-text-error":
		fmt.Fprintln(os.Stderr, "Error: GET /v1/organizations/1 returned HTTP 422: request failed")
		fmt.Fprintln(os.Stderr, "Uninstall PIN: "+r17PinTextSecret)
		return 5
	case "success-secret":
		fmt.Fprintln(os.Stderr, "hint: Uninstall PIN: "+r17HintSecret)
		fmt.Println(`{"verdict":"blocked","network":{"id":7,"name":"Main Office","secret_key":"` + r17ResultSecret + `"}}`)
		return 0
	case "tail-cut":
		// A failed command whose stdout is one JSON object over 25 lines,
		// the secret's key and value on separate lines: the 20-line tail
		// the error keeps (failedStdoutDetail) starts at the value.
		fmt.Println("{")
		for i := 0; i < 3; i++ {
			fmt.Printf("\"field_%d\": %d,\n", i, i)
		}
		fmt.Println(`"secret_key":`)
		fmt.Printf("%q,\n", r17TailCutSecret)
		for i := 0; i < 17; i++ {
			fmt.Printf("\"filler_%02d\": %d,\n", i, i)
		}
		fmt.Println(`"name": "Main Office"`)
		fmt.Println("}")
		fmt.Fprintln(os.Stderr, "Error: why-blocked failed")
		return 1
	}
	return 0
}

// useR17FakeCLI points the mirror tools and the recipe intent at this test
// binary in the given mode.
func useR17FakeCLI(t *testing.T, mode string) {
	t.Helper()
	t.Setenv("DNSFILTER_CLI_PATH", os.Args[0])
	t.Setenv(r17FakeCLIEnv, mode)
	oldPath, oldErr := recipeCLIPath, recipeCLIPathErr
	t.Cleanup(func() { recipeCLIPath, recipeCLIPathErr = oldPath, oldErr })
	recipeCLIPath, recipeCLIPathErr = os.Args[0], nil
}

func toolReadOnlyHinted(t *testing.T, s *server.MCPServer, name string) bool {
	t.Helper()
	entry := s.GetTool(name)
	if entry == nil {
		t.Fatalf("%s is not registered", name)
	}
	a := entry.Tool.Annotations
	return a.ReadOnlyHint != nil && *a.ReadOnlyHint
}

// r17HintedShellOutTools are read-only-hinted tools that shell out to the
// companion CLI: a Cobra-tree mirror (why_blocked, the finding's tool) and
// the recipe intent (intents.go).
var r17HintedShellOutTools = map[string]map[string]any{
	"why_blocked": {"fqdn": "example.com", "org": "1"},
	"precedent_check_before_allowing_a_domain": {"domain": "example.com"},
}

// TestReadOnlyHintedToolsWithholdCredentialsInDefaultMode is Codex round
// 17's P1: with DNSFILTER_MCP_READ_ONLY unset, why_blocked (advertised
// readOnlyHint=true, so hosts auto-approve it) returned an API error body
// echoing secret_key verbatim. Every read-only-hinted tool's errors and
// results now pass the read-only switch's rules in every mode, while the
// rest of the text (status, error message, site name) survives.
func TestReadOnlyHintedToolsWithholdCredentialsInDefaultMode(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	setIsolatedHome(t, srv.URL)
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "")

	cases := []struct {
		name, mode, secret string
		keep               []string
	}{
		{"api-error-body", "api-error-secret", r17APIErrorSecret, []string{"HTTP 422", "request failed", redact.WithheldFieldPlaceholder}},
		{"pin-text-error", "pin-text-error", r17PinTextSecret, []string{redact.WithheldHintedBlockMessage}},
		{"success-result-field", "success-secret", r17ResultSecret, []string{"Main Office", "blocked", redact.WithheldFieldPlaceholder}},
		{"success-stderr-hint", "success-secret", r17HintSecret, nil},
		// The tail cut separates the value from its key, so only redacting the
		// whole captured stdout before the cut (redact.Forced on the call's
		// context) catches it; the funnel alone sees a bare string.
		{"failure-stdout-tail-cut", "tail-cut", r17TailCutSecret, []string{"why-blocked failed", "Main Office"}},
	}
	for _, tc := range cases {
		for tool, args := range r17HintedShellOutTools {
			t.Run(tc.name+"/"+tool, func(t *testing.T) {
				useR17FakeCLI(t, tc.mode)
				s := newRegisteredServer(t)
				if !toolReadOnlyHinted(t, s, tool) {
					t.Fatalf("%s is not advertised readOnlyHint=true in default mode; the test proves nothing", tool)
				}
				text := strings.Join(resultTexts(t, callToolWire(t, s, tool, args)), "\n")
				if strings.Contains(text, tc.secret) {
					t.Fatalf("%s (%s, default mode) returned a credential: %.600s", tool, tc.mode, text)
				}
				for _, want := range tc.keep {
					if !strings.Contains(text, want) {
						t.Fatalf("%s (%s, default mode) lost %q: %.600s", tool, tc.mode, want, text)
					}
				}
			})
		}
	}
}

// TestReadOnlyHintedOversizedMirrorOutputMatchesTheSwitch: an oversized
// output of a read-only-hinted mirror tool (the TestMain fake CLI's JSON,
// NDJSON and CSV bodies, and a failure carrying one on stdout or stderr) is
// redacted before it is budgeted in default mode too, so the preview keeps
// the non-secret data exactly as under the switch; CSV, which cannot be
// redacted by key, is withheld in both modes.
func TestReadOnlyHintedOversizedMirrorOutputMatchesTheSwitch(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	setIsolatedHome(t, srv.URL)
	t.Setenv("DNSFILTER_CLI_PATH", os.Args[0])
	for _, mode := range []string{"json", "ndjson", "csv", "fail-stdout", "fail-stderr"} {
		for _, ro := range []string{"", "1"} {
			t.Run(mode+"/read-only="+ro, func(t *testing.T) {
				t.Setenv(fakeCLIModeEnv, mode)
				t.Setenv(cliutil.MCPReadOnlyEnvVar, ro)
				frame := callToolWire(t, newRegisteredServer(t), "why_blocked", r17HintedShellOutTools["why_blocked"])
				if strings.Contains(frame, "NETSECRET") {
					t.Fatalf("oversized output leaked a secret_key: %.600s", frame)
				}
				if !strings.Contains(frame, "_pp_truncated") {
					t.Fatalf("expected a bounded preview: %.300s", frame)
				}
				if mode != "csv" && !strings.Contains(frame, "Site 0000") {
					t.Fatalf("the preview lost the non-secret data (redaction ran after bounding?): %.600s", frame)
				}
			})
		}
	}
}

// TestReadOnlyHintedWithheldMessageNamesTheMode: a whole withheld block in
// default mode does not claim the read-only switch is set; under the switch
// the switch's own message stays.
func TestReadOnlyHintedWithheldMessageNamesTheMode(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	setIsolatedHome(t, srv.URL)
	useR17FakeCLI(t, "pin-text-error")
	for mode, want := range map[string]string{"": redact.WithheldHintedBlockMessage, "1": redact.WithheldBlockMessage} {
		t.Setenv(cliutil.MCPReadOnlyEnvVar, mode)
		text := strings.Join(resultTexts(t, callToolWire(t, newRegisteredServer(t), "why_blocked", r17HintedShellOutTools["why_blocked"])), "\n")
		if text != want {
			t.Fatalf("read-only=%q: got %q, want %q", mode, text, want)
		}
	}
	if got := redact.FinalAlways(redact.WithheldHintedBlockMessage); got != redact.WithheldHintedBlockMessage {
		t.Fatalf("the hinted withheld message does not pass its own filter: %q", got)
	}
}

// TestReadOnlyHintedInProcessToolsWithholdCredentialsInDefaultMode covers the
// read-only-hinted tools that run in this process: a successful sql/search
// result over a store holding a raw secret (written before
// secrets-never-persisted), and error texts built by tools.go (sql) and
// code_orch.go (dnsfilter_get) that echo a credential pair.
func TestReadOnlyHintedInProcessToolsWithholdCredentialsInDefaultMode(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	setIsolatedHome(t, srv.URL)
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "")
	path, err := mcpDBPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	row, _ := json.Marshal(map[string]any{"id": 7, "name": "Main Office Springfield", "secret_key": r17StoreSecret})
	if _, err := db.DB().Exec(`INSERT INTO resources (id, resource_type, data, synced_at) VALUES ('7', 'networks', ?, CURRENT_TIMESTAMP)`, string(row)); err != nil {
		t.Fatalf("seed resources: %v", err)
	}
	if _, err := db.DB().Exec(`INSERT INTO resources_fts (id, resource_type, content) VALUES ('7', 'networks', 'Main Office Springfield')`); err != nil {
		t.Fatalf("seed fts: %v", err)
	}
	_ = db.Close()

	s := newRegisteredServer(t)
	calls := []struct {
		name, tool, secret, keep string
		args                     map[string]any
	}{
		{"sql-result", "sql", r17StoreSecret, "Main Office", map[string]any{"query": "SELECT data FROM resources WHERE resource_type = 'networks'"}},
		{"search-result", "search", r17StoreSecret, "Main Office", map[string]any{"query": "Springfield"}},
		{"sql-error-text", "sql", r17EchoSecret, "", map[string]any{"query": `SELECT * FROM "token=` + r17EchoSecret + `"`}},
		{"dnsfilter_get-error-text", "dnsfilter_get", r17EchoSecret, "", map[string]any{"endpoint_id": "token=" + r17EchoSecret}},
	}
	for _, c := range calls {
		t.Run(c.name, func(t *testing.T) {
			if !toolReadOnlyHinted(t, s, c.tool) {
				t.Fatalf("%s is not advertised readOnlyHint=true; the test proves nothing", c.tool)
			}
			text := strings.Join(resultTexts(t, callToolWire(t, s, c.tool, c.args)), "\n")
			if strings.Contains(text, c.secret) {
				t.Fatalf("%s (default mode) returned a credential: %.600s", c.tool, text)
			}
			if c.keep != "" && !strings.Contains(text, c.keep) {
				t.Fatalf("%s (default mode) lost the non-secret data: %.600s", c.tool, text)
			}
		})
	}
}

// TestReadOnlyHintedRegistryToolsUnchanged: the filter costs discovery
// nothing; dnsfilter_get returns every registry entry, the credential
// endpoints included, byte-for-byte in default mode.
func TestReadOnlyHintedRegistryToolsUnchanged(t *testing.T) {
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "")
	s := newRegisteredServer(t)
	checked := 0
	for i := range codeOrchEndpoints {
		ep := &codeOrchEndpoints[i]
		if ep.Method != "GET" {
			continue
		}
		want, err := bound.JSON(codeOrchEndpointMetadata(ep))
		if err != nil {
			t.Fatal(err)
		}
		texts := resultTexts(t, callToolWire(t, s, "dnsfilter_get", map[string]any{"endpoint_id": ep.ID}))
		if len(texts) != 1 || texts[0] != escapeTerminalControls(want) {
			t.Fatalf("dnsfilter_get %s changed under the filter: %q", ep.ID, texts)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no GET endpoints checked")
	}
}

// TestDestructiveHintedCredentialToolsUnchangedInDefaultMode is the control:
// tools that are not advertised read-only (dnsfilter_execute, tail) return
// credentials in default mode by design, so the filter must not touch them.
func TestDestructiveHintedCredentialToolsUnchangedInDefaultMode(t *testing.T) {
	m := newDNSFilterMock(t)
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "")
	useR17FakeCLI(t, "success-secret")
	s := newRegisteredServer(t)
	for _, tool := range []string{"dnsfilter_execute", "tail"} {
		if toolReadOnlyHinted(t, s, tool) {
			t.Fatalf("%s is advertised read-only in default mode; the control proves nothing", tool)
		}
	}
	frame := callToolWire(t, s, "dnsfilter_execute", map[string]any{"endpoint_id": "networks.show", "params": map[string]any{"id": 7}})
	if !strings.Contains(frame, credentialSentinel) || len(m.requests("GET /v1/networks/7")) != 1 {
		t.Fatalf("default-mode dnsfilter_execute should return the network secret_key by design: %.400s", frame)
	}
	frame = callToolWire(t, s, "dnsfilter_execute", map[string]any{"endpoint_id": "api-keys.show", "params": map[string]any{"id": 1}})
	if !strings.Contains(frame, "SENTINEL-API-TOKEN-91c2") {
		t.Fatalf("default-mode dnsfilter_execute should return the API key token by design: %.400s", frame)
	}
	frame = callToolWire(t, s, "tail", map[string]any{"resource": "networks", "follow": false})
	if !strings.Contains(frame, r17ResultSecret) {
		t.Fatalf("default-mode tail should return the mirrored secret_key by design: %.400s", frame)
	}
}
