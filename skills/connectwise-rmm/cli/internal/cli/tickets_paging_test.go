// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/connectwise-rmm/handfixes.json
// under the "tickets-paging-together" entry.

package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestTicketsListNeverSendsALonePagingKey: Asio ignores pageSize without
// pageNum (and vice versa), so --page-size alone pairs with pageNum=1 and
// --page-num alone is refused before any request.
func TestTicketsListNeverSendsALonePagingKey(t *testing.T) {
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/token") {
			_, _ = io.WriteString(w, `{"access_token":"stub","expires_in":3600}`)
			return
		}
		queries = append(queries, r.URL.RawQuery)
		_, _ = io.WriteString(w, `{"tickets":[],"totalCount":0}`)
	}))
	defer srv.Close()
	t.Setenv("CONNECTWISE_RMM_HOME", t.TempDir())
	t.Setenv("CONNECTWISE_RMM_BASE_URL", srv.URL)
	t.Setenv("CONNECTWISE_RMM_TOKEN_URL", srv.URL+"/token")
	t.Setenv("CONNECTWISE_RMM_CLIENT_ID", "stub-id-1234")
	t.Setenv("CONNECTWISE_RMM_CLIENT_SECRET", "stub-secret-1234")

	cmd := RootCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"tickets", "list", "--page-size", "5", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("tickets list --page-size 5: %v", err)
	}
	if len(queries) != 1 || !strings.Contains(queries[0], "pageNum=1") || !strings.Contains(queries[0], "pageSize=5") {
		t.Fatalf("queries = %q, want one request carrying pageNum=1 and pageSize=5", queries)
	}

	queries = nil
	cmd = RootCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"tickets", "list", "--page-num", "2", "--json"})
	if err := cmd.Execute(); err == nil || len(queries) != 0 {
		t.Fatalf("--page-num alone: err=%v queries=%q, want a refusal and no request", err, queries)
	}
}
