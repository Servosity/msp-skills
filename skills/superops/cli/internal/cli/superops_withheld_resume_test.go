// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): Hand-fix (handfixes.json sync-skips-withheld-row).

package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"superops-pp-cli/internal/cliutil/testenv"
)

// A ticket on page 1 of 3 repeats the token. The first sync stores every
// other ticket and fails; so does the second, because it resumes where the
// first began instead of after the page that held the withheld ticket (#612).
func TestSyncAfterAWithheldRowReadsItAgain(t *testing.T) {
	if !heldChild(t) {
		return
	}
	a, _ := twoTenantFakes(t)
	t.Setenv("SUPEROPS_SUBDOMAIN", "acme-a")
	a.data["getTicketList"] = fakeTickets(120, time.Now())
	a.data["getTicketList"][3]["subject"] = "echo " + reviewToken
	db := filepath.Join(t.TempDir(), "held.db")
	for run := 1; run <= 2; run++ {
		out, err := runSuperOpsCLI(t, "sync", "--resources", "tickets", "--db", db)
		if err == nil {
			t.Fatalf("run %d exited clean while a ticket stays withheld\n%s", run, out)
		}
		if got := storeTicketRows(t, db); got != 119 {
			t.Fatalf("run %d: %d tickets stored, want 119\n%s", run, got, out)
		}
	}
}

// failPageOnce answers the first request for one page with HTTP 400.
type failPageOnce struct {
	next http.Handler
	page int
	once sync.Once
}

func (f *failPageOnce) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(body))
	var req struct {
		Variables map[string]any `json:"variables"`
	}
	_ = json.Unmarshal(body, &req)
	if p, _ := req.Variables["page"].(float64); int(p) == f.page {
		failed := false
		f.once.Do(func() { failed = true })
		if failed {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
	}
	f.next.ServeHTTP(w, r)
}

// A ticket on page 1 is withheld and page 2 then fails: the run returns
// early on that error, and the next run still starts where the first one
// began, reads the withheld ticket again and fails naming it (#612, Codex
// round 2).
func TestSyncAfterAWithheldRowAndALaterErrorReadsItAgain(t *testing.T) {
	if !heldChild(t) {
		return
	}
	a := &fakeSuperOps{t: t, data: map[string][]map[string]any{"getTicketList": fakeTickets(120, time.Now())}}
	a.data["getTicketList"][3]["subject"] = "echo " + reviewToken
	srv := httptest.NewServer(&failPageOnce{next: a, page: 2})
	t.Cleanup(srv.Close)
	testenv.Isolate(t)
	t.Setenv("SUPEROPS_BASE_URL", srv.URL)
	t.Setenv("SUPEROPS_API_TOKEN", reviewToken)
	t.Setenv("SUPEROPS_SUBDOMAIN", "acme-msp")
	db := filepath.Join(t.TempDir(), "held.db")
	if out, err := runSuperOpsCLI(t, "sync", "--resources", "tickets", "--db", db); err == nil {
		t.Fatalf("run 1 exited clean with page 2 failing\n%s", out)
	}
	out, err := runSuperOpsCLI(t, "sync", "--resources", "tickets", "--db", db)
	if err == nil {
		t.Fatalf("run 2 exited clean while a ticket stays withheld\n%s", out)
	}
	if got := storeTicketRows(t, db); got != 119 {
		t.Fatalf("run 2: %d tickets stored, want 119\n%s", got, out)
	}
}
