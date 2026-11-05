// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"superops-pp-cli/internal/cliutil/testenv"
	"superops-pp-cli/internal/config"
	"superops-pp-cli/internal/store"
)

// identityURLPassword is the password a SUPEROPS_BASE_URL carries in its
// userinfo (http://svc:<password>@host), the way a proxy credential is
// written into a URL.
const identityURLPassword = "PwUserinfoSecret4471"

// assertNoFileHoldsSecret reads every file under root whole (the store, its
// -wal and -shm, the response cache, the jobs ledger, the learn journal) and
// fails if any holds a secret, in any letter case.
func assertNoFileHoldsSecret(t *testing.T, root string, secrets ...string) {
	t.Helper()
	files := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files++
		for _, secret := range secrets {
			if bytes.Contains(bytes.ToLower(data), bytes.ToLower([]byte(secret))) {
				t.Fatalf("%s holds %q", path, secret)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files == 0 {
		t.Fatalf("no file under %s to check", root)
	}
}

// assertNoStoreRowHoldsSecret reads every row of every table, the way the MCP
// sql tool can.
func assertNoStoreRowHoldsSecret(t *testing.T, path string, secrets ...string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	names, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'table'`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for names.Next() {
		var name string
		if err := names.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, name)
	}
	_ = names.Close()
	if len(tables) == 0 {
		t.Fatalf("%s has no tables", path)
	}
	for _, table := range tables {
		rows, err := db.Query(`SELECT * FROM "` + table + `"`)
		if err != nil {
			t.Fatalf("dump %s: %v", table, err)
		}
		cols, _ := rows.Columns()
		for rows.Next() {
			values := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range values {
				ptrs[i] = &values[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				t.Fatalf("scan %s: %v", table, err)
			}
			for i, v := range values {
				text := strings.ToLower(fmt.Sprintf("%s", v))
				for _, secret := range secrets {
					if strings.Contains(text, strings.ToLower(secret)) {
						t.Fatalf("table %s column %s holds %q", table, cols[i], secret)
					}
				}
			}
		}
		_ = rows.Close()
	}
}

func checkpointScope(t *testing.T, path, resource string) string {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var scope string
	if err := db.QueryRow(`SELECT scope_key FROM sync_state WHERE resource_type = ?`, resource).Scan(&scope); err != nil {
		t.Fatalf("read %s checkpoint: %v", resource, err)
	}
	return scope
}

// The adversarial round-2 input: a base URL with a password in its userinfo.
// `learnings list` (any read-write open) records the tenant, and `sync`
// writes the checkpoint; neither may keep the password anywhere the MCP sql
// tool or a file read can reach it.
func TestUserinfoBaseURLIsNeverPersisted(t *testing.T) {
	fake := &fakeSuperOps{t: t, data: map[string][]map[string]any{"getTicketList": fakeTickets(12, time.Now())}}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	home := testenv.Isolate(t)
	host := strings.TrimPrefix(srv.URL, "http://")
	t.Setenv("SUPEROPS_BASE_URL", "http://svc:"+identityURLPassword+"@"+host)
	t.Setenv("SUPEROPS_API_TOKEN", reviewToken)
	t.Setenv("SUPEROPS_SUBDOMAIN", "acme-a")
	want := "subdomain=acme-a base_url=http://" + host

	if out, err := runSuperOpsCLI(t, "learnings", "list", "--json"); err != nil {
		t.Fatalf("learnings list: %v\n%s", err, out)
	}
	configureDefaultDBScope("")
	dbPath := defaultDBPath("superops-cli")
	if stamp, ok := readStamp(t, dbPath); !ok || stamp != want {
		t.Fatalf("after learnings list the store records %q (recorded %v), want %q", stamp, ok, want)
	}
	assertNoFileHoldsSecret(t, home, identityURLPassword)
	assertNoStoreRowHoldsSecret(t, dbPath, identityURLPassword)

	if out, err := runSuperOpsCLI(t, "sync", "--resources", "tickets"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if got := storeTicketRows(t, dbPath); got != 12 {
		t.Fatalf("sync stored %d tickets, want 12", got)
	}
	if fakeRequests(fake) == 0 {
		t.Fatal("sync never reached the fake")
	}
	if scope := checkpointScope(t, dbPath, "tickets"); scope != "template:"+config.SuperOpsTenantScopeVar+"="+want {
		t.Fatalf("checkpoint scope %q, want the tenant without the password", scope)
	}
	assertNoFileHoldsSecret(t, home, identityURLPassword)
	assertNoStoreRowHoldsSecret(t, dbPath, identityURLPassword)
}

// assertRefusedBaseURLNeverPersisted runs teach and learnings list (read-write
// opens that record the tenant) with a base URL config.Load refuses: neither
// opens the store, neither shows the secret, and no file, the store included,
// holds it.
func assertRefusedBaseURLNeverPersisted(t *testing.T, base string) {
	t.Helper()
	home := testenv.Isolate(t)
	t.Setenv("SUPEROPS_BASE_URL", base)
	t.Setenv("SUPEROPS_API_TOKEN", reviewToken)
	t.Setenv("SUPEROPS_SUBDOMAIN", "acme-a")
	for _, args := range [][]string{
		{"teach", "--query", "test", "--resource-type", "tickets", "--resource", "T-1", "--no-validate"},
		{"learnings", "list", "--json"},
	} {
		out, err := runSuperOpsCLI(t, args...)
		if strings.Contains(strings.ToLower(fmt.Sprint(out, err)), strings.ToLower(identityURLPassword)) {
			t.Fatalf("%s shows the secret:\n%s\n%v", args[0], out, err)
		}
	}
	_ = filepath.WalkDir(home, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || !d.Type().IsRegular() {
			return walkErr
		}
		if data, _ := os.ReadFile(path); bytes.Contains(bytes.ToLower(data), bytes.ToLower([]byte(identityURLPassword))) {
			t.Fatalf("%s holds the secret", path)
		}
		return nil
	})
}

// The extra-round input: a password with an unescaped '/' and an escaped '@'
// is read as the path, so the base URL is refused and never persisted.
func TestDisplacedUserinfoBaseURLIsNeverPersisted(t *testing.T) {
	assertRefusedBaseURLNeverPersisted(t, "http://svc:1/"+identityURLPassword+"%40127.0.0.1:9")
}

// A query string or fragment on the base URL is refused, so it is never kept.
func TestBaseURLQueryAndFragmentAreNeverPersisted(t *testing.T) {
	for _, tc := range []struct{ name, base string }{
		{"query", "http://127.0.0.1:9/?sig=" + identityURLPassword},
		{"fragment", "http://127.0.0.1:9#" + identityURLPassword},
		{"query-and-fragment", "http://127.0.0.1:9/?sig=" + identityURLPassword + "#" + identityURLPassword},
		{"userinfo-and-query", "http://svc:" + identityURLPassword + "@127.0.0.1:9/?sig=" + identityURLPassword},
	} {
		t.Run(tc.name, func(t *testing.T) { assertRefusedBaseURLNeverPersisted(t, tc.base) })
	}
}

// A --db store the earlier build stamped with the password (record and
// checkpoint) is rewritten by the next read-write command of its tenant, and
// nothing of the password is left in the file or any row.
func TestLegacyStampWithUserinfoIsScrubbedByTheNextSync(t *testing.T) {
	fake := &fakeSuperOps{t: t, data: map[string][]map[string]any{"getTicketList": fakeTickets(12, time.Now())}}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	home := testenv.Isolate(t)
	host := strings.TrimPrefix(srv.URL, "http://")
	raw := "http://svc:" + identityURLPassword + "@" + host
	t.Setenv("SUPEROPS_BASE_URL", raw)
	t.Setenv("SUPEROPS_API_TOKEN", reviewToken)
	t.Setenv("SUPEROPS_SUBDOMAIN", "acme-a")
	legacy := "subdomain=acme-a base_url=" + strings.ToLower(raw)
	path := filepath.Join(home, "legacy", "shared.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}

	// Plant the earlier build's shape: a version-less tenant table and a
	// checkpoint scope, both holding the lowercased URL with its password.
	s, err := store.Open(path)
	if err != nil {
		t.Fatalf("plant open: %v", err)
	}
	row, _ := json.Marshal(map[string]any{"ticketId": "7100000000000001", "displayId": "P-1", "subject": "planted", "updatedTime": "2026-01-01T00:00:00"})
	if _, _, err := s.UpsertBatch("tickets", []json.RawMessage{row}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	// This build never writes a held value to a store, so the earlier
	// build's records are planted with the plain driver, as it wrote them.
	plantEarlierBuildStamp(t, path, stampTableName, legacy, "template:"+config.SuperOpsTenantScopeVar+"="+legacy)
	if stamp, _ := readStamp(t, path); stamp != legacy {
		t.Fatalf("planted stamp %q, want %q", stamp, legacy)
	}

	if out, err := runSuperOpsCLI(t, "sync", "--resources", "tickets", "--db", path); err != nil {
		t.Fatalf("sync into its own tenant's legacy store: %v\n%s", err, out)
	}
	want := "subdomain=acme-a base_url=http://" + host
	if stamp, ok := readStamp(t, path); !ok || stamp != want {
		t.Fatalf("after sync the store records %q, want %q", stamp, want)
	}
	if scope := checkpointScope(t, path, "tickets"); scope != "template:"+config.SuperOpsTenantScopeVar+"="+want {
		t.Fatalf("checkpoint scope %q, want the scrubbed tenant", scope)
	}
	if got := storeTicketRows(t, path); got < 12 {
		t.Fatalf("store holds %d tickets after sync, want the 12 synced", got)
	}
	assertNoFileHoldsSecret(t, home, identityURLPassword)
	assertNoStoreRowHoldsSecret(t, path, identityURLPassword)
}

// plantEarlierBuildStamp writes, with the plain sqlite driver, the shape an
// earlier build left: a version-less tenant table and a completed tickets
// checkpoint, both holding legacy.
func plantEarlierBuildStamp(t *testing.T, path, stampTable, legacy, legacyScope string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, stmt := range []string{
		`DROP TABLE IF EXISTS ` + stampTable,
		`CREATE TABLE ` + stampTable + ` (id INTEGER PRIMARY KEY CHECK (id = 1), tenant TEXT NOT NULL, stamped_at TEXT NOT NULL)`,
		`INSERT INTO ` + stampTable + ` (id, tenant, stamped_at) VALUES (1, '` + legacy + `', '2026-10-26T00:00:00Z')`,
		`DELETE FROM sync_state WHERE resource_type = 'tickets'`,
		`INSERT INTO sync_state (resource_type, last_cursor, last_synced_at, total_count, last_attempt_complete, scope_key) VALUES ('tickets', '', '2026-10-26T00:00:00Z', 1, 1, '` + legacyScope + `')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("plant %q: %v", stmt, err)
		}
	}
}
