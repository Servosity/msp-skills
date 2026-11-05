// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Next-batch r1 (handfixes.json hubspot-paging-next-after): HubSpot pages
// every CRM list with "paging":{"next":{"after":...}} (CollectionResponse
// ...ForwardPaging, press/tools-manifest.json), so sync and live --all reads
// take the next cursor from paging.next.after and read every page.

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hubspot-pp-cli/internal/cliutil/testenv"
	"hubspot-pp-cli/internal/store"
)

// newPagedDealsServer serves total deals, pageSize per page, with HubSpot's
// paging.next.after cursor (the next record's id) on every page but the last.
func newPagedDealsServer(t *testing.T, total, pageSize int) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/crm/v3/objects/0-3" {
			_, _ = fmt.Fprint(w, `{"results":[]}`)
			return
		}
		start := 1
		if a := r.URL.Query().Get("after"); a != "" {
			_, _ = fmt.Sscan(a, &start)
		}
		var recs []string
		for i := start; i < start+pageSize && i <= total; i++ {
			recs = append(recs, fmt.Sprintf(`{"id":"%d","properties":{"dealname":"D%d","dealstage":"closedwon","pipeline":"default","hs_is_closed":"true"},"createdAt":"2026-01-01T00:00:00Z","updatedAt":"2026-01-01T00:00:00Z","archived":false}`, i, i))
		}
		page := `{"results":[` + strings.Join(recs, ",") + `]`
		if next := start + pageSize; next <= total {
			page += fmt.Sprintf(`,"paging":{"next":{"after":"%d","link":"https://api.hubapi.com/crm/v3/objects/0-3?after=%d"}}`, next, next)
		}
		_, _ = fmt.Fprint(w, page+`}`)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("HUBSPOT_BASE_URL", srv.URL)
	t.Setenv("HUBSPOT_ACCESS_TOKEN", "pat-na1-"+"7f3c9e2a-b14d-4c8e-9a6f-2d5e8b1c7a31")
}

func TestNBSyncReadsEveryHubSpotPage(t *testing.T) {
	testenv.Isolate(t)
	newPagedDealsServer(t, 25, 10)
	path := t.TempDir() + "/paged.db"
	if _, stderr, err := r3dRun(t, "sync", "--resources", "hubspot-deals-crm", "--db", path, "--json"); err != nil {
		t.Fatalf("sync: %v\n%s", err, stderr)
	}
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM hubspot_deals_crm`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 25 {
		t.Fatalf("sync stored %d of 25 deals across 3 pages", n)
	}
}

func TestNBLiveAllReadsEveryHubSpotPage(t *testing.T) {
	testenv.Isolate(t)
	newPagedDealsServer(t, 25, 10)
	out, stderr, err := r3dRun(t, "hubspot-deals-crm", "get-v3-objects-0-3-get-page", "--all", "--json")
	if err != nil {
		t.Fatalf("live --all: %v\n%s", err, stderr)
	}
	if got := strings.Count(out, `"dealname"`); got != 25 {
		t.Fatalf("live --all returned %d of 25 deals across 3 pages:\n%s", got, stderr)
	}
}
