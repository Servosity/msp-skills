// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"microsoft-graph-pp-cli/internal/cliutil/testenv"
)

// nextLinkFake answers every collection the way Graph pages it: the first
// page carries an @odata.nextLink, the second ends the collection.
func nextLinkFake(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("$skiptoken") == "page2-7d1c9e" {
			_, _ = w.Write([]byte(`{"value":[{"id":"second-page-item","displayName":"second-page-item"}]}`))
			return
		}
		next := "http://" + r.Host + r.URL.Path + "?$skiptoken=page2-7d1c9e"
		_, _ = fmt.Fprintf(w, `{"@odata.context":"ctx","@odata.nextLink":%q,"value":[{"id":"first-page-item","displayName":"first-page-item"}]}`, next)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// microsoft-graph handfix (list-reads-every-page), class
// incomplete-read-as-complete: every list command and export read the
// collection to its end, never only the first page.
func TestEveryListCommandReadsEveryPage(t *testing.T) {
	testenv.Isolate(t)
	srv := nextLinkFake(t)
	t.Setenv("MICROSOFT_GRAPH_BASE_URL", srv.URL+"/v1.0")
	t.Setenv("MICROSOFT_GRAPH_TOKEN", "listPagesToken-4d1e")

	for _, args := range [][]string{
		{"users", "list"}, {"groups", "list"}, {"devices", "list"}, {"directory-roles", "list"},
		{"managed-devices", "list"}, {"security", "alerts"}, {"security", "incidents"},
		{"licenses", "skus"}, {"users", "messages", "u1"}, {"users", "licenses", "u1"},
		{"groups", "members", "g1"}, {"groups", "owners", "g1"}, {"directory-roles", "members", "r1"},
		{"teams-users", "list"}, {"export", "users", "--format", "json"},
	} {
		out, err := runTranscripts(t, append(args, "--json", "--data-source", "live")...)
		if err != nil || !strings.Contains(out, "first-page-item") || !strings.Contains(out, "second-page-item") {
			t.Errorf("%s: %v, want both pages\n%s", strings.Join(args, " "), err, out)
		}
	}
}
