// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"superops-pp-cli/internal/cliutil"
	"superops-pp-cli/internal/cliutil/testenv"
	"superops-pp-cli/internal/store"
)

// stampTableName is the table a SuperOps store records its tenant in
// (superops-tenant-stamp). The tests name it literally so they compile and
// fail, rather than not compile, against a build without the stamp.
const stampTableName = "superops_store_tenant"

// twoTenantFakes serves tenant A (acme-a, tickets T-1..T-n) and tenant B
// (acme-b, tickets B-1..B-n) from one host, routed by CustomerSubDomain, and
// points the CLI at it with one token for both.
func twoTenantFakes(t *testing.T) (a, b *fakeSuperOps) {
	t.Helper()
	now := time.Now()
	bTickets := fakeTickets(10, now)
	for i := range bTickets {
		bTickets[i]["ticketId"] = fmt.Sprintf("80000000000000%04d", i+1)
		bTickets[i]["displayId"] = fmt.Sprintf("B-%d", i+1)
	}
	a = &fakeSuperOps{t: t, data: map[string][]map[string]any{"getTicketList": fakeTickets(12, now)}}
	b = &fakeSuperOps{t: t, data: map[string][]map[string]any{"getTicketList": bTickets}}
	srv := httptest.NewServer(tenantFakes{"acme-a": a, "acme-b": b})
	t.Cleanup(srv.Close)
	testenv.Isolate(t)
	t.Setenv("SUPEROPS_BASE_URL", srv.URL)
	t.Setenv("SUPEROPS_API_TOKEN", reviewToken)
	return a, b
}

func fakeRequests(f *fakeSuperOps) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, pages := range f.pages {
		n += len(pages)
	}
	return n
}

func storeTicketRows(t *testing.T, path string) int {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM resources WHERE resource_type = 'tickets'`).Scan(&n); err != nil {
		t.Fatalf("count tickets in %s: %v", path, err)
	}
	return n
}

// readStamp reads the recorded tenant straight from the file, around every
// gate, so a test can see what was written.
func readStamp(t *testing.T, path string) (string, bool) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var tenant string
	err = db.QueryRow(`SELECT tenant FROM ` + stampTableName + ` WHERE id = 1`).Scan(&tenant)
	if err != nil {
		return "", false
	}
	return tenant, true
}

// plantUnprovenStore writes rows the way a store from before the tenant stamp
// holds them: synced data, and nothing that says which tenant it came from.
func plantUnprovenStore(t *testing.T, path string, displayIDs ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(path)
	if err != nil {
		t.Fatalf("plant store: %v", err)
	}
	var rows []json.RawMessage
	for i, id := range displayIDs {
		row, _ := json.Marshal(map[string]any{
			"ticketId": fmt.Sprintf("9100000000000%04d", i+1), "displayId": id, "subject": "planted " + id,
			"status": "Open", "updatedTime": "2026-01-01T00:00:00", "client": map[string]any{"name": "Acme"},
		})
		rows = append(rows, row)
	}
	if _, _, err := db.UpsertBatch("tickets", rows); err != nil {
		t.Fatalf("plant rows: %v", err)
	}
	if err := db.SaveSyncState("tickets", "", len(rows)); err != nil {
		t.Fatalf("plant checkpoint: %v", err)
	}
	if _, err := db.DB().Exec(`DROP TABLE IF EXISTS ` + stampTableName); err != nil {
		t.Fatalf("drop stamp: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, ok := readStamp(t, path); ok {
		t.Fatal("the planted store still records a tenant")
	}
}

func assertRefusal(t *testing.T, what, out string, err error, leaked ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s was not refused:\n%s", what, out)
	}
	for _, l := range leaked {
		if strings.Contains(out, l) {
			t.Fatalf("%s showed another tenant's row %s:\n%s", what, l, out)
		}
	}
	if !strings.Contains(out, "tenant") || !strings.Contains(out, "--db") || !strings.Contains(out, "sync") {
		t.Fatalf("%s refusal does not name the remedy (sync into a fresh --db or the default store):\n%s", what, out)
	}
	for _, f := range reviewTokenForms() {
		if strings.Contains(out, f) {
			t.Fatalf("%s refusal printed the credential:\n%s", what, out)
		}
	}
}

// Member 1 (Codex P1, sync.go): an explicit --db shared by two tenants. The
// checkpoint reset for B, but A's rows stayed, so B's local read showed them.
func TestSharedDBIsNeverReadOrWrittenAsAnotherTenant(t *testing.T) {
	a, b := twoTenantFakes(t)
	shared := filepath.Join(t.TempDir(), "x", "shared.db")

	t.Setenv("SUPEROPS_SUBDOMAIN", "acme-a")
	if out, err := runSuperOpsCLI(t, "sync", "--resources", "tickets", "--db", shared); err != nil {
		t.Fatalf("tenant A sync: %v\n%s", err, out)
	}
	if got := storeTicketRows(t, shared); got != 12 {
		t.Fatalf("tenant A synced %d tickets, want 12", got)
	}

	t.Setenv("SUPEROPS_SUBDOMAIN", "acme-b")
	out, err := runSuperOpsCLI(t, "sync", "--resources", "tickets", "--db", shared)
	assertRefusal(t, "tenant B sync into tenant A's --db", out, err)
	if n := fakeRequests(b); n != 0 {
		t.Fatalf("tenant B's sync reached the API %d times before it was refused", n)
	}
	if got := storeTicketRows(t, shared); got != 12 {
		t.Fatalf("tenant A's store holds %d tickets after B's refused sync, want A's 12 only", got)
	}

	out, err = runSuperOpsCLI(t, "stale-tickets", "--days", "1", "--db", shared, "--json")
	assertRefusal(t, "tenant B stale-tickets on tenant A's --db", out, err, `"T-1"`, `"T-12"`)
	out, err = runSuperOpsCLI(t, "search", "ticket", "--db", shared, "--data-source", "local", "--json")
	assertRefusal(t, "tenant B search on tenant A's --db", out, err, `"T-1"`, "ticket 1")

	// Tenant A still owns the file and still reads it.
	t.Setenv("SUPEROPS_SUBDOMAIN", "acme-a")
	out, err = runSuperOpsCLI(t, "stale-tickets", "--days", "1", "--db", shared, "--json")
	if err != nil || !strings.Contains(out, `"T-1"`) || strings.Contains(out, `"B-1"`) {
		t.Fatalf("tenant A no longer reads its own --db (err %v):\n%s", err, out)
	}
	if n := fakeRequests(a); n == 0 {
		t.Fatal("tenant A's sync never reached its fake")
	}
}

// A sync never writes into a store recorded for another tenant, with or
// without --full, and the refused store is left exactly as it was.
func TestSyncRefusesAStoreStampedForAnotherTenant(t *testing.T) {
	_, b := twoTenantFakes(t)
	shared := filepath.Join(t.TempDir(), "stamped.db")
	t.Setenv("SUPEROPS_SUBDOMAIN", "acme-a")
	if out, err := runSuperOpsCLI(t, "sync", "--resources", "tickets", "--db", shared); err != nil {
		t.Fatalf("tenant A sync: %v\n%s", err, out)
	}
	before, _ := readStamp(t, shared)

	t.Setenv("SUPEROPS_SUBDOMAIN", "acme-b")
	for _, extra := range [][]string{nil, {"--full"}, {"--latest-only"}} {
		args := append([]string{"sync", "--resources", "tickets", "--db", shared}, extra...)
		out, err := runSuperOpsCLI(t, args...)
		assertRefusal(t, fmt.Sprintf("tenant B %v", args), out, err)
	}
	out, err := runSuperOpsCLI(t, "workflow", "archive", "--db", shared)
	assertRefusal(t, "tenant B workflow archive into tenant A's --db", out, err)
	if n := fakeRequests(b); n != 0 {
		t.Fatalf("tenant B reached the API %d times", n)
	}
	if got := storeTicketRows(t, shared); got != 12 {
		t.Fatalf("store holds %d tickets, want tenant A's 12", got)
	}
	stamp, ok := readStamp(t, shared)
	if !ok || !strings.Contains(stamp, "subdomain=acme-a") || stamp != before {
		t.Fatalf("tenant A's store records tenant %q (recorded %v, was %q), want subdomain=acme-a unchanged", stamp, ok, before)
	}
	for _, f := range reviewTokenForms() {
		if strings.Contains(stamp, f) {
			t.Fatalf("the tenant stamp holds the credential: %q", stamp)
		}
	}
}

// Member 2 (adversarial P2, helpers.go): with no credential the default store
// is the unscoped data.db. A data.db filled before the stamp (or by another
// tenant) is read by no one who cannot prove to be its tenant.
func TestUnprovenStoreIsNotReadWithoutACredential(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv("SUPEROPS_API_TOKEN", "")
	t.Setenv("SUPEROPS_SUBDOMAIN", "acme-a")
	configureDefaultDBScope("")
	dir, err := cliutil.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	planted := filepath.Join(dir, "data.db")
	plantUnprovenStore(t, planted, "A-1", "A-2")

	for _, subdomain := range []string{"acme-b", ""} {
		t.Setenv("SUPEROPS_SUBDOMAIN", subdomain)
		configureDefaultDBScope("")
		if got := defaultDBPath("superops-cli"); got != planted {
			t.Fatalf("with no credential the default store is %s, want the planted %s", got, planted)
		}
		out, err := runSuperOpsCLI(t, "stale-tickets", "--days", "1", "--json")
		assertRefusal(t, fmt.Sprintf("no credential, subdomain %q, stale-tickets", subdomain), out, err, "A-1", "A-2")
		out, err = runSuperOpsCLI(t, "search", "planted", "--data-source", "local", "--json")
		assertRefusal(t, fmt.Sprintf("no credential, subdomain %q, search", subdomain), out, err, "A-1", "A-2")
	}

	// With tenant B's credential the default store moves to B's own file, and
	// pointing --db at the planted store is refused the same way.
	t.Setenv("SUPEROPS_API_TOKEN", reviewToken)
	t.Setenv("SUPEROPS_SUBDOMAIN", "acme-b")
	out, err := runSuperOpsCLI(t, "stale-tickets", "--days", "1", "--db", planted, "--json")
	assertRefusal(t, "tenant B credential, --db planted, stale-tickets", out, err, "A-1", "A-2")
	if got := storeTicketRows(t, planted); got != 2 {
		t.Fatalf("the planted store holds %d rows, want its 2 untouched", got)
	}
	if _, ok := readStamp(t, planted); ok {
		t.Fatal("a refused open stamped the planted store")
	}
}

// Positive path: a fresh --db syncs, records its tenant, and reads for that
// tenant; a second sync of the same tenant resumes into it.
func TestFreshStoreSyncsAndReadsForItsOwnTenant(t *testing.T) {
	a, _ := twoTenantFakes(t)
	fresh := filepath.Join(t.TempDir(), "fresh.db")
	t.Setenv("SUPEROPS_SUBDOMAIN", "acme-a")
	for i := 0; i < 2; i++ {
		if out, err := runSuperOpsCLI(t, "sync", "--resources", "tickets", "--db", fresh); err != nil {
			t.Fatalf("sync %d: %v\n%s", i+1, err, out)
		}
	}
	if got := storeTicketRows(t, fresh); got != 12 {
		t.Fatalf("fresh store holds %d tickets, want 12", got)
	}
	if stamp, ok := readStamp(t, fresh); !ok || !strings.Contains(stamp, "subdomain=acme-a") {
		t.Fatalf("fresh store records tenant %q (recorded %v)", stamp, ok)
	}
	out, err := runSuperOpsCLI(t, "stale-tickets", "--days", "1", "--db", fresh, "--json")
	if err != nil || !strings.Contains(out, `"T-1"`) {
		t.Fatalf("tenant A cannot read its own fresh store (err %v):\n%s", err, out)
	}
	if fakeRequests(a) == 0 {
		t.Fatal("sync never reached tenant A")
	}
}

// Positive path: the default store keeps working for each tenant in turn, and
// switching back to a tenant finds its own store again.
func TestDefaultStoreKeepsWorkingForEachTenant(t *testing.T) {
	twoTenantFakes(t)
	for _, tenant := range []struct{ subdomain, own, other string }{
		{"acme-a", `"T-1"`, `"B-1"`}, {"acme-b", `"B-1"`, `"T-1"`},
	} {
		t.Setenv("SUPEROPS_SUBDOMAIN", tenant.subdomain)
		if out, err := runSuperOpsCLI(t, "sync", "--resources", "tickets"); err != nil {
			t.Fatalf("%s sync: %v\n%s", tenant.subdomain, err, out)
		}
	}
	for _, tenant := range []struct{ subdomain, own, other string }{
		{"acme-a", `"T-1"`, `"B-1"`}, {"acme-b", `"B-1"`, `"T-1"`},
	} {
		t.Setenv("SUPEROPS_SUBDOMAIN", tenant.subdomain)
		out, err := runSuperOpsCLI(t, "stale-tickets", "--days", "1", "--json")
		if err != nil || !strings.Contains(out, tenant.own) || strings.Contains(out, tenant.other) {
			t.Fatalf("%s default store read (err %v):\n%s", tenant.subdomain, err, out)
		}
		configureDefaultDBScope("")
		if stamp, ok := readStamp(t, defaultDBPath("superops-cli")); !ok || !strings.Contains(stamp, "subdomain="+tenant.subdomain) {
			t.Fatalf("%s default store records tenant %q (recorded %v)", tenant.subdomain, stamp, ok)
		}
	}
}

// The learn commands share the store file. On a fresh install, with or
// without a credential, they keep working, and a later sync of the same
// tenant into the same store is not refused.
func TestLearnCommandsWorkOnAFreshInstall(t *testing.T) {
	twoTenantFakes(t)
	learnFlow := [][]string{
		{"learnings", "list", "--json"},
		{"learnings", "stats", "--json"},
		{"playbook", "list", "--json"},
		{"teach", "--query", "stale tickets for acme", "--resource-type", "tickets", "--resource", "T-1", "--no-validate"},
		{"recall", "stale tickets for acme", "--json"},
	}
	t.Setenv("SUPEROPS_API_TOKEN", "")
	t.Setenv("SUPEROPS_SUBDOMAIN", "")
	for _, args := range learnFlow {
		if out, err := runSuperOpsCLI(t, args...); err != nil {
			t.Fatalf("no credential: %v failed on a fresh install: %v\n%s", args, err, out)
		}
	}

	t.Setenv("SUPEROPS_API_TOKEN", reviewToken)
	t.Setenv("SUPEROPS_SUBDOMAIN", "acme-a")
	flow := append([][]string{}, learnFlow...)
	flow = append(flow, []string{"sync", "--resources", "tickets"}, []string{"stale-tickets", "--days", "1", "--json"})
	flow = append(flow, learnFlow...)
	for _, args := range flow {
		if out, err := runSuperOpsCLI(t, args...); err != nil {
			t.Fatalf("tenant A: %v failed on a fresh install: %v\n%s", args, err, out)
		}
	}
}
