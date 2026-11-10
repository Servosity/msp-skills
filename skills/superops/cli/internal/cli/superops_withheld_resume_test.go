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
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"superops-pp-cli/internal/cliutil/testenv"
	"superops-pp-cli/internal/credkit"
)

// heldResp is held as a vendor response value: the one thing the store
// refuses (synccpu, owner GO 2026-11-09).
const heldResp = "SoRespHeld-7Kq2Wz9Lm4Xv8"

// withheldTicket is the ID of fakeTickets' fourth ticket, the one that
// repeats heldResp.
const withheldTicket = "700000000000000004"

// syncCapturingStderr runs the CLI in process with os.Stderr captured, where
// sync reports a withheld row.
func syncCapturingStderr(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w
	got := make(chan string, 1)
	go func() { b, _ := io.ReadAll(r); got <- string(b) }()
	out, runErr := runSuperOpsCLI(t, args...)
	os.Stderr = orig
	_ = w.Close()
	return out, <-got, runErr
}

// A ticket on page 1 of 3 repeats a held response value. Each sync stores
// every other ticket, names the withheld one and exits clean (a withheld row
// is reported, never fatal), and the next sync reads it again, because the
// resource resumes where the run began, not after the page that held it (#612).
func TestSyncAfterAWithheldRowReadsItAgain(t *testing.T) {
	if !heldChild(t) {
		return
	}
	credkit.Hold("response:token", heldResp)
	a, _ := twoTenantFakes(t)
	t.Setenv("SUPEROPS_SUBDOMAIN", "acme-a")
	a.data["getTicketList"] = fakeTickets(120, time.Now())
	a.data["getTicketList"][3]["subject"] = "echo " + heldResp
	db := filepath.Join(t.TempDir(), "held.db")
	for run := 1; run <= 2; run++ {
		out, errOut, err := syncCapturingStderr(t, "sync", "--resources", "tickets", "--db", db)
		if err != nil {
			t.Fatalf("run %d failed on a withheld row: %v\n%s\n%s", run, err, out, errOut)
		}
		if !strings.Contains(errOut, "withheld by the credential check") || !strings.Contains(errOut, withheldTicket) || !strings.Contains(errOut, `"withheld":{"tickets":["`+withheldTicket+`"]}`) {
			t.Fatalf("run %d did not name the withheld ticket on stderr and in its summary:\n%s", run, errOut)
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
// began, reads the withheld ticket again and names it, exiting clean
// (#612, Codex round 2; synccpu).
func TestSyncAfterAWithheldRowAndALaterErrorReadsItAgain(t *testing.T) {
	if !heldChild(t) {
		return
	}
	credkit.Hold("response:token", heldResp)
	a := &fakeSuperOps{t: t, data: map[string][]map[string]any{"getTicketList": fakeTickets(120, time.Now())}}
	a.data["getTicketList"][3]["subject"] = "echo " + heldResp
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
	out, errOut, err := syncCapturingStderr(t, "sync", "--resources", "tickets", "--db", db)
	if err != nil {
		t.Fatalf("run 2 failed on a withheld row: %v\n%s\n%s", err, out, errOut)
	}
	if !strings.Contains(errOut, withheldTicket) {
		t.Fatalf("run 2 did not read the withheld ticket again:\n%s", errOut)
	}
	if got := storeTicketRows(t, db); got != 119 {
		t.Fatalf("run 2: %d tickets stored, want 119\n%s", got, out)
	}
}
