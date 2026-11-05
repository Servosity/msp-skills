// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"superops-pp-cli/internal/config"
)

const (
	identityPassword = "PwUserinfoSecret4471"
	identityRawBase  = "http://svc:" + identityPassword + "@127.0.0.1:18791"
)

// legacyTenantRecord is the tenant record exactly as the build before
// superops-tenant-identity-no-credential wrote it: the whole base URL,
// lowercased, userinfo included.
func legacyTenantRecord(subdomain, baseURL string) string {
	return "subdomain=" + strings.ToLower(strings.TrimSpace(subdomain)) +
		" base_url=" + strings.ToLower(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
}

// currentTenant is the identity the running build computes for one config.
func currentTenant(subdomain, baseURL string) string {
	return (&config.Config{Subdomain: subdomain, BaseURL: baseURL}).SuperOpsTenant()
}

// plantLegacyStampedStore writes a store the way the earlier build left it:
// a synced ticket, a checkpoint whose scope key holds the record, and a
// version-less tenant table holding the record.
func plantLegacyStampedStore(t *testing.T, path, record string) {
	t.Helper()
	previous := currentTenantSource()
	SetTenantSource(nil)
	defer SetTenantSource(previous)
	s, err := Open(path)
	if err != nil {
		t.Fatalf("plant open: %v", err)
	}
	row, _ := json.Marshal(map[string]any{"ticketId": "7000000000000001", "displayId": "T-1", "subject": "planted"})
	if _, _, err := s.UpsertBatch("tickets", []json.RawMessage{row}); err != nil {
		t.Fatalf("plant ticket: %v", err)
	}
	scope := "template:" + config.SuperOpsTenantScopeVar + "=" + record
	if _, _, _, err := s.BindSyncScope("tickets", scope); err != nil {
		t.Fatalf("plant checkpoint scope: %v", err)
	}
	if err := s.SaveSyncStateInScope("tickets", scope, "cursor-7", 1); err != nil {
		t.Fatalf("plant checkpoint: %v", err)
	}
	for _, stmt := range []string{
		`DROP TABLE IF EXISTS ` + TenantStampTable,
		`CREATE TABLE ` + TenantStampTable + ` (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		tenant TEXT NOT NULL,
		stamped_at TEXT NOT NULL
	)`,
	} {
		if _, err := s.DB().Exec(stmt); err != nil {
			t.Fatalf("plant stamp table: %v", err)
		}
	}
	if _, err := s.DB().Exec(`INSERT INTO `+TenantStampTable+` (id, tenant, stamped_at) VALUES (1, ?, '2026-10-26T00:00:00Z')`, record); err != nil {
		t.Fatalf("plant stamp: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

// rawStore opens the file around every gate, read-only.
func rawStore(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// stampRecord returns the recorded tenant and its version (1 when the table
// has no version column, the shape of the earlier build).
func stampRecord(t *testing.T, path string) (string, int) {
	t.Helper()
	db := rawStore(t, path)
	var tenant string
	if err := db.QueryRow(`SELECT tenant FROM ` + TenantStampTable + ` WHERE id = 1`).Scan(&tenant); err != nil {
		t.Fatalf("read stamp: %v", err)
	}
	var hasVersion int
	if err := db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('` + TenantStampTable + `') WHERE name = 'version'`).Scan(&hasVersion); err != nil {
		t.Fatalf("read stamp columns: %v", err)
	}
	if hasVersion == 0 {
		return tenant, 1
	}
	var version int
	if err := db.QueryRow(`SELECT version FROM ` + TenantStampTable + ` WHERE id = 1`).Scan(&version); err != nil {
		t.Fatalf("read stamp version: %v", err)
	}
	return tenant, version
}

func checkpointOf(t *testing.T, path, resource string) (scope, cursor string) {
	t.Helper()
	if err := rawStore(t, path).QueryRow(`SELECT scope_key, last_cursor FROM sync_state WHERE resource_type = ?`, resource).Scan(&scope, &cursor); err != nil {
		t.Fatalf("read checkpoint: %v", err)
	}
	return scope, cursor
}

// assertNoSecretInStoreFiles reads the database file and every sidecar whole
// and every row of every table, case-insensitively.
func assertNoSecretInStoreFiles(t *testing.T, path string, secrets ...string) {
	t.Helper()
	for _, file := range []string{path, path + "-wal", path + "-shm", path + "-journal"} {
		data, err := os.ReadFile(file)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for _, secret := range secrets {
			if bytes.Contains(bytes.ToLower(data), bytes.ToLower([]byte(secret))) {
				t.Fatalf("%s holds %q in its bytes", filepath.Base(file), secret)
			}
		}
	}
	db := rawStore(t, path)
	tables, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'table'`)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for tables.Next() {
		var name string
		if err := tables.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	_ = tables.Close()
	for _, name := range names {
		rows, err := db.Query(`SELECT * FROM "` + name + `"`)
		if err != nil {
			t.Fatalf("dump %s: %v", name, err)
		}
		cols, _ := rows.Columns()
		for rows.Next() {
			values := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range values {
				ptrs[i] = &values[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				t.Fatalf("scan %s: %v", name, err)
			}
			for i, v := range values {
				text := strings.ToLower(fmt.Sprintf("%s", v))
				for _, secret := range secrets {
					if strings.Contains(text, strings.ToLower(secret)) {
						t.Fatalf("table %s column %s holds %q", name, cols[i], secret)
					}
				}
			}
		}
		_ = rows.Close()
	}
}

// A fresh store opened read-write as a tenant whose base URL carries a
// password records the tenant without it, in no row and in no file byte.
func TestTenantStampOfAUserinfoBaseURLHoldsNoPassword(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fresh.db")
	tenant := currentTenant("acme-a", identityRawBase)
	runAsTenant(t, tenant, true)
	s, err := Open(path)
	if err != nil {
		t.Fatalf("read-write open: %v", err)
	}
	// While the store is open its writes sit in the -wal file.
	assertNoSecretInStoreFiles(t, path, identityPassword)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	assertNoSecretInStoreFiles(t, path, identityPassword)
	got, version := stampRecord(t, path)
	if got != "subdomain=acme-a base_url=http://127.0.0.1:18791" || version != 2 {
		t.Fatalf("stamp %q version %d, want subdomain=acme-a base_url=http://127.0.0.1:18791 version 2", got, version)
	}
}

// A record the earlier build wrote with a URL password is rewritten, in the
// open that finds it, to the identity without it: the record, the checkpoint
// scope key, every row and every file byte. The checkpoint resumes. A long
// password is the case where the shorter rewritten row does not cover the
// old one, so its bytes would survive in the page's free space. Each planted
// store has its own user and password: reading a planted record holds them,
// and the holder then refuses to plant them again.
func TestTenantStampV1WithCredentialIsScrubbedOnReadWriteOpen(t *testing.T) {
	for _, tc := range []struct{ name, user, needle, password string }{
		{"short password", "svcshort", "PwV1ShortSecret4471", "PwV1ShortSecret4471"},
		{"long password", "svclong", "PwV1LongSecret4471", "PwV1LongSecret4471" + strings.Repeat("Q7", 120)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := "http://" + tc.user + ":" + tc.password + "@127.0.0.1:18791"
			path := filepath.Join(t.TempDir(), "v1.db")
			plantLegacyStampedStore(t, path, legacyTenantRecord("acme-a", raw))
			if got, version := stampRecord(t, path); version != 1 || !strings.Contains(got, strings.ToLower(tc.password)) {
				t.Fatalf("the planted record %q (version %d) is not the earlier build's shape", got, version)
			}

			tenant := currentTenant("acme-a", raw)
			runAsTenant(t, tenant, true)
			s, err := Open(path)
			if err != nil {
				t.Fatalf("read-write open of its own tenant's v1 store: %v", err)
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}

			got, version := stampRecord(t, path)
			if got != tenant || version != 2 || strings.Contains(got, "@") {
				t.Fatalf("after a read-write open the stamp is %q version %d, want %q version 2", got, version, tenant)
			}
			scope, cursor := checkpointOf(t, path, "tickets")
			if scope != "template:"+config.SuperOpsTenantScopeVar+"="+tenant || cursor != "cursor-7" {
				t.Fatalf("checkpoint scope %q cursor %q, want the scrubbed scope and the kept cursor", scope, cursor)
			}
			assertNoSecretInStoreFiles(t, path, tc.needle, tc.user+":")

			// The scrubbed store keeps working for its tenant, read-only and read-write.
			for _, open := range []func(string) (*Store, error){OpenReadOnly, Open} {
				s, err := open(path)
				if err != nil {
					t.Fatalf("reopen after scrub: %v", err)
				}
				_ = s.Close()
			}
			// The same tenant with the URL password rotated is the same tenant.
			runAsTenant(t, currentTenant("acme-a", "http://svc:RotatedPassword@127.0.0.1:18791/"), true)
			s, err = OpenReadOnly(path)
			if err != nil {
				t.Fatalf("read-only open after the URL password rotated: %v", err)
			}
			_ = s.Close()
		})
	}
}

// A read-only open cannot rewrite the record, so it refuses the store rather
// than leave the password readable, names the remedy, and prints nothing of
// the record. The file is left as it was.
func TestTenantStampV1WithCredentialIsRefusedOnReadOnlyOpen(t *testing.T) {
	const user, heldValue = "svcreadonly", "PwV1ReadOnlySecret4471"
	rawBase := "http://" + user + ":" + heldValue + "@127.0.0.1:18791"
	path := filepath.Join(t.TempDir(), "v1.db")
	record := legacyTenantRecord("acme-a", rawBase)
	plantLegacyStampedStore(t, path, record)

	runAsTenant(t, currentTenant("acme-a", rawBase), true)
	s, err := OpenReadOnly(path)
	if err == nil {
		_ = s.Close()
		t.Fatal("a read-only open of a v1 store whose record holds a URL password opened")
	}
	if !errors.Is(err, ErrStoreTenant) {
		t.Fatalf("err %v, want a store tenant refusal", err)
	}
	msg := err.Error()
	for _, leaked := range []string{heldValue, strings.ToLower(heldValue), user + ":", "127.0.0.1", "base_url="} {
		if strings.Contains(msg, leaked) {
			t.Fatalf("the refusal echoes the record (%q):\n%s", leaked, msg)
		}
	}
	if !strings.Contains(msg, "delete") || !strings.Contains(msg, "superops-cli") {
		t.Fatalf("the refusal does not name the remedy (a command that writes the store, or delete the file):\n%s", msg)
	}
	if got, version := stampRecord(t, path); got != record || version != 1 {
		t.Fatalf("a read-only open changed the record to %q version %d", got, version)
	}
}

// A v1 record of another tenant is refused on every open and never
// rewritten; the refusal prints nothing of it.
func TestTenantStampV1OfAnotherTenantIsRefusedAndNotScrubbed(t *testing.T) {
	const heldValue = "PwV1OtherSecret4471"
	rawBase := "http://svcother:" + heldValue + "@127.0.0.1:18791"
	path := filepath.Join(t.TempDir(), "v1-b.db")
	record := legacyTenantRecord("acme-b", rawBase)
	plantLegacyStampedStore(t, path, record)

	runAsTenant(t, currentTenant("acme-a", rawBase), true)
	for _, open := range []func(string) (*Store, error){OpenReadOnly, Open} {
		s, err := open(path)
		wantRefusal(t, "open of another tenant's v1 store", s, err, false)
		if strings.Contains(strings.ToLower(err.Error()), strings.ToLower(heldValue)) || strings.Contains(err.Error(), "acme-b") {
			t.Fatalf("the refusal echoes the record:\n%s", err)
		}
	}
	if got, version := stampRecord(t, path); got != record || version != 1 {
		t.Fatalf("another tenant's v1 record was rewritten to %q version %d", got, version)
	}
}

// An ordinary v1 record (no credential in the URL) keeps working on both
// opens; the first read-write open bumps it to version 2, record unchanged.
func TestTenantStampV1WithoutCredentialKeepsWorking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v1-plain.db")
	record := legacyTenantRecord("acme-a", "https://api.superops.ai")
	plantLegacyStampedStore(t, path, record)

	runAsTenant(t, currentTenant("acme-a", "https://api.superops.ai"), true)
	s, err := OpenReadOnly(path)
	if err != nil {
		t.Fatalf("read-only open of an ordinary v1 store: %v", err)
	}
	_ = s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatalf("read-write open of an ordinary v1 store: %v", err)
	}
	_ = s.Close()
	if got, version := stampRecord(t, path); got != record || version != 2 {
		t.Fatalf("ordinary v1 store stamp %q version %d, want %q version 2", got, version, record)
	}
	if scope, cursor := checkpointOf(t, path, "tickets"); scope != "template:"+config.SuperOpsTenantScopeVar+"="+record || cursor != "cursor-7" {
		t.Fatalf("ordinary checkpoint changed: scope %q cursor %q", scope, cursor)
	}
}
