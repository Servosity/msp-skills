// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"superops-pp-cli/internal/cliutil"
	"superops-pp-cli/internal/cliutil/testenv"
)

func TestGraphQLOperationKindsRefusesEveryNonQueryDocument(t *testing.T) {
	refused := []string{
		`mutation { updateTicket(input: {ticketId: "1"}) { ticketId } }`,
		`  # leading comment
mutation M { a }`,
		graphQLBOM + "mutation { a }",
		`query A { a } mutation B { b }`,
		`{ a } mutation { b }`,
		`subscription { a }`,
		`fragment F on Ticket { a } mutation { b }`,
		`query { a } junk`,
		`query { a `,
		`query { a } }`,
		`"mutation" { a }`,
		`Query { a }`,
		`schema { query: Q }`,
		`query { a(s: "unterminated) }`,
		`query { a(s: """unterminated) }`,
		``,
		`   `,
	}
	for _, doc := range refused {
		if err := requireQueryOnlyDocument(doc); err == nil {
			t.Errorf("raw query accepted %q", doc)
		}
	}
}

func TestGraphQLOperationKindsAcceptsQueryDocuments(t *testing.T) {
	accepted := []string{
		`{ getTicketList(input: {pageSize: 5}) { tickets { ticketId } } }`,
		`query { a }`,
		`query Q($in: ListInfoInput = {pageSize: 5, page: 1}) @cached(ttl: 1) { getTicketList(input: $in) { tickets { ticketId } } }`,
		`query { a(s: "mutation { x }", t: "{") }`,
		`query { a(s: """block "with" mutation { } \""" escaped""") }`,
		`# mutation { x }
query { a }`,
		`fragment F on Ticket { ticketId } query { getTicket(input: {ticketId: "1"}) { ...F } }`,
		`query A { a } query B { b }`,
	}
	for _, doc := range accepted {
		if err := requireQueryOnlyDocument(doc); err != nil {
			t.Errorf("raw query refused %q: %v", doc, err)
		}
	}
}

func rawTestServer(t *testing.T) *atomic.Int32 {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"ok":true}}`))
	}))
	t.Cleanup(srv.Close)
	testenv.Isolate(t)
	t.Setenv("SUPEROPS_BASE_URL", srv.URL)
	t.Setenv("SUPEROPS_API_TOKEN", "sop-test-token-4471b9")
	t.Setenv("SUPEROPS_SUBDOMAIN", "acme-msp")
	return &hits
}

func TestRawMutationSendsNothingWithoutYes(t *testing.T) {
	hits := rawTestServer(t)
	out, err := runSuperOpsCLI(t, "raw", "mutation", `mutation { resolveAlerts(input: {alertIds: ["1"]}) }`)
	if err == nil || hits.Load() != 0 {
		t.Fatalf("raw mutation without --yes: err=%v requests=%d\n%s", err, hits.Load(), out)
	}
	if !strings.Contains(out, "resolveAlerts") || !strings.Contains(out, "--yes") {
		t.Fatalf("raw mutation without --yes should print the request and name --yes:\n%s", out)
	}
	if out, err := runSuperOpsCLI(t, "raw", "mutation", `mutation { resolveAlerts(input: {alertIds: ["1"]}) }`, "--yes"); err != nil || hits.Load() != 1 {
		t.Fatalf("raw mutation --yes: err=%v requests=%d\n%s", err, hits.Load(), out)
	}
}

func TestRawQueryNeverSendsAWrite(t *testing.T) {
	hits := rawTestServer(t)
	for _, doc := range []string{`mutation { updateTicket(input: {ticketId: "1"}) { ticketId } }`, `{ a } mutation { b }`} {
		if out, err := runSuperOpsCLI(t, "raw", "query", doc, "--yes"); err == nil {
			t.Fatalf("raw query sent %q\n%s", doc, out)
		}
	}
	if hits.Load() != 0 {
		t.Fatalf("raw query reached the API %d time(s) with a write document", hits.Load())
	}
	if out, err := runSuperOpsCLI(t, "raw", "query", `{ getStatusList { statusId name } }`); err != nil || hits.Load() != 1 {
		t.Fatalf("raw query of a read: err=%v requests=%d\n%s", err, hits.Load(), out)
	}
}

func TestRawRefusesAtPathInAnMCPChild(t *testing.T) {
	hits := rawTestServer(t)
	dir := t.TempDir()
	doc := filepath.Join(dir, "op.graphql")
	vars := filepath.Join(dir, "vars.json")
	if err := os.WriteFile(doc, []byte(`{ getStatusList { statusId } }`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(vars, []byte(`{"a":1}`), 0o600); err != nil {
		t.Fatal(err)
	}
	// The operator at the command line may read local files.
	if out, err := runSuperOpsCLI(t, "raw", "query", "@"+doc, "--vars", "@"+vars); err != nil || hits.Load() != 1 {
		t.Fatalf("CLI @path read: err=%v requests=%d\n%s", err, hits.Load(), out)
	}
	t.Setenv(cliutil.MCPChildEnvVar, "1")
	for _, args := range [][]string{
		{"raw", "query", "@" + doc},
		{"raw", "query", `{ a }`, "--vars", "@" + vars},
		{"raw", "mutation", "@" + doc, "--yes"},
	} {
		out, err := runSuperOpsCLI(t, args...)
		if err == nil || !strings.Contains(out, "names a local file on the machine running the MCP server") {
			t.Fatalf("%v in an MCP child: err=%v\n%s", args, err, out)
		}
	}
	if hits.Load() != 1 {
		t.Fatalf("an MCP child reached the API with an @path value (%d requests)", hits.Load())
	}
}
