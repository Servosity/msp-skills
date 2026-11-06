// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"quickbooks-pp-cli/internal/cliutil/testenv"
	"quickbooks-pp-cli/internal/store"
)

// fakeQBOQuery answers GET /query the way QuickBooks Online does: the page is
// chosen by STARTPOSITION/MAXRESULTS inside the query text (1-based, MAXRESULTS
// capped at 1000), and rows come back under QueryResponse.<Entity>. A request
// without a select statement gets QBO's 200 Fault envelope.
func fakeQBOQuery(t *testing.T, total int, requests *int64) *httptest.Server {
	t.Helper()
	sel := regexp.MustCompile(`(?i)^select \* from (\w+)(?: startposition (\d+))?(?: maxresults (\d+))?$`)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(requests, 1)
		w.Header().Set("Content-Type", "application/json")
		m := sel.FindStringSubmatch(strings.TrimSpace(r.URL.Query().Get("query")))
		if r.URL.Path != "/query" || m == nil {
			fmt.Fprint(w, `{"Fault":{"Error":[{"Message":"message=ParseError","code":"4000"}],"type":"ValidationFault"}}`)
			return
		}
		start, max := 1, 100
		if m[2] != "" {
			start, _ = strconv.Atoi(m[2])
		}
		if m[3] != "" {
			max, _ = strconv.Atoi(m[3])
		}
		if max > 1000 {
			max = 1000
		}
		var rows []string
		for i := start; i < start+max && i <= total; i++ {
			rows = append(rows, fmt.Sprintf(`{"Id":"%d","DisplayName":"Customer %d","SyncToken":"0"}`, i, i))
		}
		if len(rows) == 0 {
			// QBO's answer for an empty result (zero rows, or a STARTPOSITION
			// past the last row): an empty QueryResponse object, no entity key.
			fmt.Fprint(w, `{"QueryResponse":{},"time":"2026-01-01T00:00:00Z"}`)
			return
		}
		fmt.Fprintf(w, `{"QueryResponse":{"%s":[%s],"startPosition":%d,"maxResults":%d},"time":"2026-01-01T00:00:00Z"}`, m[1], strings.Join(rows, ","), start, len(rows))
	}))
}

// TestQBOSyncPagesThroughTheWholeBook pins qbo-query-injection, qbo-query-paging
// and qbo-queryresponse-envelope together: 2500 customers need three 1000-row
// pages, and every row must land in the mirror.
func TestQBOSyncPagesThroughTheWholeBook(t *testing.T) {
	testenv.Isolate(t)
	const total = 2500
	var requests int64
	srv := fakeQBOQuery(t, total, &requests)
	defer srv.Close()
	t.Setenv("QUICKBOOKS_BASE_URL", srv.URL)
	t.Setenv("QUICKBOOKS_ACCESS_TOKEN", "fixture-token")
	dbPath := filepath.Join(t.TempDir(), "data.db")

	root := newRootCmd(&rootFlags{})
	var out, errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)
	root.SetArgs([]string{"sync", "--resources", "customers", "--full", "--db", dbPath, "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync failed: %v\nstdout: %s\nstderr: %s", err, out.String(), errBuf.String())
	}
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	n, err := db.Count("customers")
	if err != nil {
		t.Fatal(err)
	}
	if n != total {
		t.Fatalf("mirror holds %d customers, want %d (requests=%d)\n%s", n, total, atomic.LoadInt64(&requests), out.String())
	}
	if got := atomic.LoadInt64(&requests); got != 3 {
		t.Fatalf("sync made %d /query requests, want 3 (1000+1000+500)", got)
	}
	// The local novels read the rows sync wrote (same store file, same table).
	var top []map[string]any
	if err := json.Unmarshal([]byte(runNovel(t, dbPath, "customers", "top", "--by", "balance", "--limit", "5000")), &top); err != nil {
		t.Fatal(err)
	}
	if len(top) != total {
		t.Fatalf("customers top over the synced mirror returned %d rows, want %d", len(top), total)
	}
}

// TestQBOSyncEndsCleanlyOnAnEmptyQueryResponse covers QBO's empty-result shape,
// `{"QueryResponse":{}}`: an entity with no rows at all, and the page after a
// book whose size is an exact multiple of the 1000-row page.
func TestQBOSyncEndsCleanlyOnAnEmptyQueryResponse(t *testing.T) {
	for _, total := range []int{0, 2000} {
		t.Run(strconv.Itoa(total), func(t *testing.T) {
			testenv.Isolate(t)
			var requests int64
			srv := fakeQBOQuery(t, total, &requests)
			defer srv.Close()
			t.Setenv("QUICKBOOKS_BASE_URL", srv.URL)
			t.Setenv("QUICKBOOKS_ACCESS_TOKEN", "fixture-token")
			dbPath := filepath.Join(t.TempDir(), "data.db")
			root := newRootCmd(&rootFlags{})
			var out, errBuf bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&errBuf)
			root.SetArgs([]string{"sync", "--resources", "customers", "--full", "--db", dbPath, "--json"})
			if err := root.Execute(); err != nil {
				t.Fatalf("sync of %d rows failed: %v\nstdout: %s\nstderr: %s", total, err, out.String(), errBuf.String())
			}
			db, err := store.OpenWithContext(context.Background(), dbPath)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if n, _ := db.Count("customers"); n != total {
				t.Fatalf("mirror holds %d customers, want %d", n, total)
			}
		})
	}
}

// TestQBOSyncFailsOnAFault keeps the other direction: QBO's 200 Fault envelope
// is a failure, never an empty page.
func TestQBOSyncFailsOnAFault(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"Fault":{"Error":[{"Message":"message=AuthorizationFailure","code":"120"}],"type":"AuthorizationFault"},"QueryResponse":{}}`)
	}))
	defer srv.Close()
	t.Setenv("QUICKBOOKS_BASE_URL", srv.URL)
	t.Setenv("QUICKBOOKS_ACCESS_TOKEN", "fixture-token")
	root := newRootCmd(&rootFlags{})
	var out, errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)
	root.SetArgs([]string{"sync", "--resources", "customers", "--full", "--db", filepath.Join(t.TempDir(), "data.db"), "--json"})
	if err := root.Execute(); err == nil {
		t.Fatalf("a Fault response synced cleanly:\n%s", errBuf.String())
	}
}
