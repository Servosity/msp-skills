// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package rewstblackbox

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// pagerServer is a Rewst fake whose page list holds total pages, paged as
// the SDL documents it (spec.graphql Query.pages(where, limit, offset, order,
// search): [Page!]!): a bare array under data.pages of limit records from
// offset, full pages until the last. The REST-shaped GET /graphql the typed
// commands and sync send is answered the same way. It counts the requests it
// answered and the windows it served.
func pagerServer(t *testing.T, total int) (*server, *int) {
	t.Helper()
	var mu sync.Mutex
	asked := new(int)
	sv := newServer()
	srv := tlsServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		*asked++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query()
		sv.mu.Lock()
		sv.pages["offset="+q.Get("offset")+"&limit="+q.Get("limit")]++
		sv.mu.Unlock()
		limit, err := strconv.Atoi(q.Get("limit"))
		if err != nil || limit <= 0 {
			limit = total
		}
		offset, _ := strconv.Atoi(q.Get("offset"))
		var items []string
		for i := offset + 1; i <= min(total, offset+limit); i++ {
			items = append(items, fmt.Sprintf(`{"id":"pg%04d","name":"Paged page %d","orgId":"org-paged","path":"/p%d"}`, i, i, i))
		}
		fmt.Fprintf(w, `{"data":{"pages":[%s]}}`, strings.Join(items, ","))
	}))
	sv.url, sv.caFile = srv.URL, caFileFor(t, srv)
	return sv, asked
}

// A wrong result on a documented response shape (not a credential class):
// sync walks every page of its default resource (pages) from offset 0 and
// stops after the last; an org with more pages than one window (sync's
// limit is 100) must store every one, or the local search, SQL and stale
// views an MSP acts on silently miss records. The store is read back through
// the MCP sql tool.
func TestSyncWalksEveryPage(t *testing.T) {
	budget(t)
	total := 101 // two windows; the full sweep walks three
	if fullSweep {
		total = 250
	}
	sv, asked := pagerServer(t, total)
	e := newEnv(t, sv, credEnv).lean()
	syncOut := e.run(t, "sync", "--resources", "pages", "--full", "--json")
	if !regexp.MustCompile(`"total_records":\s*` + strconv.Itoa(total) + `\b`).MatchString(syncOut) {
		sv.mu.Lock()
		t.Errorf("sync did not store every page (%d requests, windows %v):\n%.1500s", *asked, sv.pages, syncOut)
		sv.mu.Unlock()
	}
	got := e.mcpCalls(t, []map[string]any{tool("sql", map[string]any{"query": "SELECT count(*) AS n FROM resources WHERE resource_type = 'pages'"})})
	if !regexp.MustCompile(`\\*"n\\*":\s*` + strconv.Itoa(total) + `\b`).MatchString(got) {
		t.Errorf("the local store does not hold %d pages:\n%.1500s", total, got)
	}
	// The live typed read honours the window it is given.
	if out := e.run(t, "pages", "get", "--limit", "2", "--offset", "3", "--json"); !strings.Contains(out, "Paged page 4") ||
		!strings.Contains(out, "Paged page 5") || strings.Contains(out, "Paged page 6") || strings.Contains(out, "Paged page 3\"") {
		t.Errorf("pages get --limit 2 --offset 3 did not answer pages 4 and 5:\n%.1500s", out)
	}
}
