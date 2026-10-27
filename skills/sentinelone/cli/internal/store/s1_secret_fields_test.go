// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): the local store never keeps a credential,
// whichever write path a row arrives through.

package store

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func secretStoreFileLeaks(t *testing.T, dir string, sentinels ...string) []string {
	t.Helper()
	var leaks []string
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}
		b, rerr := os.ReadFile(filepath.Clean(path))
		if rerr != nil {
			return nil
		}
		low := bytes.ToLower(b)
		for _, s := range sentinels {
			if bytes.Contains(low, []byte(strings.ToLower(s))) {
				leaks = append(leaks, s+" in "+filepath.Base(path))
			}
		}
		return nil
	})
	return leaks
}

func secretRecord(id, ordinary, s, s2, s3 string) json.RawMessage {
	rec := map[string]any{
		"id":              id,
		"accountId":       "acc1",
		"accounts_id":     "acc1",
		"users_id":        "user1",
		"sites_id":        "site1",
		"groups_id":       "group1",
		"computerName":    ordinary,
		"passphrase":      s,
		"passwordExpired": true,
		"nested":          map[string]any{"apiToken": s2, "keep": ordinary + "-nested"},
		"settings":        []any{map[string]any{"name": "apiKey", "value": s3}, map[string]any{"name": "region", "value": ordinary + "-region"}},
	}
	b, _ := json.Marshal(rec)
	return b
}

// TestStoreDropsSecretNamedFields upserts an ordinary record carrying a
// passphrase, a nested apiToken and a name/value apiKey through every store
// write path (generic Upsert, UpsertBatch, UpsertBatchDetailed, typed
// upserts directly and through the batch dispatcher, and the keep-richer
// merge over a row that already holds a secret). No secret may reach the
// file; every ordinary field must.
func TestStoreDropsSecretNamedFields(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}

	type path struct {
		name  string
		write func(rec json.RawMessage) error
	}
	paths := []path{
		{"Upsert", func(rec json.RawMessage) error { return s.Upsert("agents", "a-upsert", rec) }},
		{"UpsertBatch", func(rec json.RawMessage) error {
			_, _, err := s.UpsertBatch("agents", []json.RawMessage{rec})
			return err
		}},
		{"UpsertBatchDetailed", func(rec json.RawMessage) error {
			_, _, _, err := s.UpsertBatchDetailed("sites", []json.RawMessage{rec})
			return err
		}},
		{"UpsertUninstallPassword", s.UpsertUninstallPassword},
		{"UpsertApiTokenDetails", s.UpsertApiTokenDetails},
		{"UpsertSitesRegenerateKey", s.UpsertSitesRegenerateKey},
		{"UpsertGroupsRegenerateKey", s.UpsertGroupsRegenerateKey},
		{"UpsertBatch typed uninstall_password", func(rec json.RawMessage) error {
			_, _, err := s.UpsertBatch("uninstall_password", []json.RawMessage{rec})
			return err
		}},
	}
	var secrets, ordinaries []string
	for i, p := range paths {
		ordinary := "ORDINARY" + string(rune('A'+i)) + "value"
		sec := []string{"passphrasesentinel" + string(rune('a'+i)) + "q9", "apitokensentinel" + string(rune('a'+i)) + "w8", "apikeysentinel" + string(rune('a'+i)) + "e7"}
		if err := p.write(secretRecord("id-"+string(rune('a'+i)), ordinary, sec[0], sec[1], sec[2])); err != nil {
			t.Fatalf("%s: %v", p.name, err)
		}
		secrets = append(secrets, sec...)
		ordinaries = append(ordinaries, ordinary, ordinary+"-nested", ordinary+"-region")
	}

	// A never-stored resource keeps no content, even a credential under a
	// name the field rule does not know (an SNMP community string).
	if err := s.Upsert("ranger", "c1", json.RawMessage(`{"id":"c1","credType":"snmp","community":"rangercommunitysentinel"}`)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.UpsertBatch("agents-passphrases", []json.RawMessage{json.RawMessage(`{"id":"11","computerName":"WS1","passphrase":"batchpassphrasesentinel"}`)}); err != nil {
		t.Fatal(err)
	}
	secrets = append(secrets, "rangercommunitysentinel", "batchpassphrasesentinel")

	// Keep-richer merge: a row an older build wrote with a secret must not
	// carry it into the merged row a new write produces.
	if _, err := s.DB().Exec(`INSERT INTO resources (id, resource_type, data) VALUES ('m1', 'agents', ?)`,
		`{"id":"m1","computerName":"MERGEROW","passphrase":"mergepassphrasesentinel"}`); err != nil {
		t.Fatal(err)
	}
	if err := s.Upsert("agents", "m1", json.RawMessage(`{"id":"m1","computerName":"MERGEROW"}`)); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get("agents", "m1")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "mergepassphrasesentinel") {
		t.Errorf("merge kept the old secret: %s", got)
	}
	if !strings.Contains(string(got), "MERGEROW") {
		t.Errorf("merge lost the ordinary field: %s", got)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	if leaks := secretStoreFileLeaks(t, dir, secrets...); len(leaks) > 0 {
		t.Fatalf("secret fields persisted:\n%s", strings.Join(leaks, "\n"))
	}
	for _, o := range ordinaries {
		if len(secretStoreFileLeaks(t, dir, o)) == 0 {
			t.Errorf("ordinary field %q was not stored", o)
		}
	}
}

// TestOldStoreSecretsArePurgedOnOpen: a store an older build wrote (agent
// passphrase rows, registration tokens on sites, a typed uninstall password,
// a fleet snapshot of an agent row that carried a passphrase, their FTS
// entries and sync state) is scrubbed on its next read-write open, down to
// the file bytes.
func TestOldStoreSecretsArePurgedOnOpen(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	raw := s.DB()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := raw.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	oldPass := `{"id":"11","computerName":"OLDWS","passphrase":"OLDPASS ALPHAOLDPASS"}`
	oldSite := `{"id":"s1","name":"OLDSITE","registrationToken":"oldsiteregtokensentinel"}`
	oldAgent := `{"id":"a1","computerName":"OLDAGENT","passphrase":"oldmergedpassphrasesentinel"}`
	exec(`INSERT INTO resources (id, resource_type, data) VALUES ('11', 'agents-passphrases', ?)`, oldPass)
	exec(`INSERT INTO resources (id, resource_type, data) VALUES ('s1', 'sites', ?)`, oldSite)
	exec(`INSERT INTO resources (id, resource_type, data) VALUES ('a1', 'agents', ?)`, oldAgent)
	exec(`INSERT INTO resources_fts (rowid, id, resource_type, content) VALUES (?, '11', 'agents-passphrases', 'OLDWS OLDPASS ALPHAOLDPASS')`, ftsRowID("agents-passphrases", "11"))
	exec(`INSERT INTO resources_fts (rowid, id, resource_type, content) VALUES (?, 's1', 'sites', 'OLDSITE oldsiteregtokensentinel')`, ftsRowID("sites", "s1"))
	exec(`INSERT INTO resources_fts (rowid, id, resource_type, content) VALUES (?, 'a1', 'agents', 'OLDAGENT oldmergedpassphrasesentinel')`, ftsRowID("agents", "a1"))
	exec(`INSERT INTO uninstall_password (id, accounts_id, data) VALUES ('u1', 'acc1', ?)`, `{"id":"u1","password":"olduninstallpwsentinel"}`)
	exec(`INSERT INTO fleet_snapshots (captured_at, entity, entity_id, data) VALUES ('2026-01-01T00:00:00Z', 'agents', 'a1', ?)`, oldAgent)
	exec(`INSERT INTO sync_state (resource_type, last_synced_at, total_count) VALUES ('agents-passphrases', '2026-01-01T00:00:00Z', 1)`)
	// A store an older (v13) build wrote, already scrubbed under that build's
	// own table list: the schema version, not the scrub stamp, is the gate.
	exec(`CREATE TABLE IF NOT EXISTS secret_scrub_state (id INTEGER PRIMARY KEY CHECK (id = 1), rule_version INTEGER NOT NULL)`)
	exec(`INSERT INTO secret_scrub_state (id, rule_version) VALUES (1, 1) ON CONFLICT(id) DO UPDATE SET rule_version = 1`)
	exec(`PRAGMA user_version = 13`)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s, err = Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM resources WHERE resource_type = 'agents-passphrases'`).Scan(&n); err != nil || n != 0 {
		t.Errorf("agents-passphrases rows left: %d (%v)", n, err)
	}
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM sync_state WHERE resource_type = 'agents-passphrases'`).Scan(&n); err != nil || n != 0 {
		t.Errorf("agents-passphrases sync_state left: %d (%v)", n, err)
	}
	site, err := s.Get("sites", "s1")
	if err != nil || !strings.Contains(string(site), "OLDSITE") {
		t.Errorf("ordinary site row lost: %s (%v)", site, err)
	}
	if results, err := s.Search("OLDSITE", 10); err != nil || len(results) == 0 {
		t.Errorf("scrubbed site is no longer searchable: %v %v", results, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if leaks := secretStoreFileLeaks(t, dir, "OLDPASS", "ALPHAOLDPASS", "oldsiteregtokensentinel", "oldmergedpassphrasesentinel", "olduninstallpwsentinel"); len(leaks) > 0 {
		t.Fatalf("old secrets survived the scrub:\n%s", strings.Join(leaks, "\n"))
	}
}
