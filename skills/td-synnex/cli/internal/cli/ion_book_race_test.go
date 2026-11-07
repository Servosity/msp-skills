// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written (handfixes.json: ion-book-one-account): two syncs of two ION
// accounts into one explicit --db never mix their rows in the book.

package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"td-synnex-pp-cli/internal/cliutil/testenv"
)

// ionAccountConfig writes a config file naming account, so two syncs in one
// process can each walk their own account (TD_SYNNEX_ACCOUNT_ID is process-wide).
func ionAccountConfig(t *testing.T, account string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[template_vars]\naccountId = \""+account+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// Codex round 2 (publish 2026-11-06), P1 (sync.go:575): account 10505's
// sync claimed subscriptions in a shared --db, then waited on its response;
// account 20000's sync claimed the list and completed; 10505's late page then
// committed its rows (only the checkpoint after it noticed the scope change),
// and margin for 20000 answered with both accounts' subscriptions. The row
// write now checks the list's owner and the walk's scope in its own
// transaction, and the book reads only rows its account's walks wrote.
func TestConcurrentSyncsOfTwoAccountsNeverMixTheBook(t *testing.T) {
	testenv.Isolate(t)
	inner := ionTwoAccountServer(t)
	defer inner.Close()
	held, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/accounts/10505/subscriptions") {
			once.Do(func() { close(held) })
			<-release
		}
		req, _ := http.NewRequest(r.Method, inner.URL+r.URL.RequestURI(), r.Body)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	}))
	defer srv.Close()
	ionBookEnv(t, srv)
	t.Setenv("TD_SYNNEX_ACCOUNT_ID", "")
	cfg10505, cfg20000 := ionAccountConfig(t, "10505"), ionAccountConfig(t, "20000")
	db := filepath.Join(t.TempDir(), "shared.db")

	first := make(chan error, 1)
	go func() {
		_, err := runIONCLI(t, "sync", "--config", cfg10505, "--resources", "subscriptions", "--db", db, "--json")
		first <- err
	}()
	<-held
	if out, err := runIONCLI(t, "sync", "--config", cfg20000, "--resources", "customers,subscriptions,orders", "--db", db, "--json"); err != nil {
		close(release)
		t.Fatalf("sync of account 20000: %v\n%s", err, out)
	}
	close(release)
	<-first // 10505's walk lost its claim; whether it errors is not the point, its rows are

	out, err := runIONCLI(t, "margin", "--config", cfg20000, "--by", "customer", "--db", db, "--json")
	if err != nil {
		t.Fatalf("margin for 20000 after both syncs: %v\n%s", err, out)
	}
	for _, c := range []string{"108536", "108537"} {
		if strings.Contains(out, c) {
			t.Fatalf("margin for account 20000 answered with account 10505's customer %s:\n%s", c, out)
		}
	}
	for _, c := range []string{"208536", "208537"} {
		if !strings.Contains(out, c) {
			t.Fatalf("margin for account 20000 lost its own customer %s:\n%s", c, out)
		}
	}
}

// The book reads only rows a write for its account committed: a row no such
// write tagged (another account's, written past the claim by an earlier
// build) refuses the book with the exact sync, and that sync recovers it.
func TestBookRefusesARowNoWriteForItsAccountTagged(t *testing.T) {
	testenv.Isolate(t)
	srv := ionTwoAccountServer(t)
	defer srv.Close()
	ionBookEnv(t, srv)
	db := filepath.Join(t.TempDir(), "book.db")
	mustSync(t, "--resources", "customers,subscriptions,orders", "--db", db, "--json")
	sub, _ := json.Marshal(ionOneSeat("208536"))
	execStore(t, db, `INSERT INTO resources (id, resource_type, data, synced_at, updated_at) VALUES ('208536-0', 'subscriptions', ?, '2026-11-06T00:00:00Z', '2026-11-06T00:00:00Z')`, string(sub))
	wantBookRefusal(t, db, "108536")
	execStore(t, db, `INSERT INTO ion_book_row (resource_type, id, account) VALUES ('subscriptions', '208536-0', '20000')`)
	wantBookRefusal(t, db, "108536")

	mustSync(t, "--resources", "customers,subscriptions,orders", "--db", db, "--json")
	wantMarginCustomers(t, db, "108536", "108537")
}
