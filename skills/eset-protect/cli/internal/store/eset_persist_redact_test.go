// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"eset-protect-pp-cli/internal/cliutil"
	"eset-protect-pp-cli/internal/cliutil/credvariants"
)

const storeLinkSentinel = "STORE-INSTALLER-LINK-SENTINEL"

func storeLinkRow(id string, extra string) json.RawMessage {
	return json.RawMessage(`{"uuid":"` + id + `","displayName":"Acme agent","downloadUrl":"https://download.example/` + storeLinkSentinel + `"` + extra + `}`)
}

// storeTokenRow carries the sentinel as a deviceToken, the Credential-tier
// key every scope redacts, for the typed tables (whose schemas never carry an
// installer link; redaction is scoped by schema).
func storeTokenRow(id string, extra string) json.RawMessage {
	return json.RawMessage(`{"uuid":"` + id + `","displayName":"Acme agent","device":{"deviceToken":"` + storeLinkSentinel + `"}` + extra + `}`)
}

// dbFilesHold reports which of the database's files (main, -wal, -shm)
// contain needle anywhere in their bytes.
func dbFilesHold(t *testing.T, dir, needle string) []string {
	t.Helper()
	var hits []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err == nil && bytes.Contains(b, []byte(needle)) {
			hits = append(hits, e.Name())
		}
	}
	return hits
}

// TestStoreNeverPersistsCredentialTierValues guards
// credential-tier-never-persisted at the store funnel: every public write
// path (Upsert, UpsertBatch, UpsertBatchDetailed, each typed Upsert*) stores
// the row with downloadUrl replaced, in the resources table, its FTS index
// and the typed tables, so the link is in no byte of the database files.
func TestStoreNeverPersistsCredentialTierValues(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	must := func(what string, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
	}
	must("Upsert", db.Upsert("installers", "00000000-0000-0000-0000-0000000000b1", storeLinkRow("00000000-0000-0000-0000-0000000000b1", "")))
	_, _, err = db.UpsertBatch("installers", []json.RawMessage{storeLinkRow("00000000-0000-0000-0000-0000000000b2", ""), storeLinkRow("00000000-0000-0000-0000-0000000000b1", `,"usable":true`)})
	must("UpsertBatch", err)
	_, _, _, err = db.UpsertBatchDetailed("installers", []json.RawMessage{storeLinkRow("00000000-0000-0000-0000-0000000000b3", "")})
	must("UpsertBatchDetailed", err)
	_, _, err = db.UpsertBatch("device_groups_devices", []json.RawMessage{storeTokenRow("00000000-0000-0000-0000-0000000000c1", `,"device_groups_id":"g1"`)})
	must("UpsertBatch typed", err)
	must("UpsertDeviceGroupsDevices", db.UpsertDeviceGroupsDevices(storeTokenRow("00000000-0000-0000-0000-0000000000c2", `,"device_groups_id":"g1"`)))
	must("UpsertRuns", db.UpsertRuns(storeTokenRow("00000000-0000-0000-0000-0000000000c3", `,"device_tasks_id":"t1"`)))
	must("UpsertBasicAttributesUpdate", db.UpsertBasicAttributesUpdate(storeTokenRow("00000000-0000-0000-0000-0000000000c4", `,"incidents_id":"i1"`)))
	must("UpsertComments", db.UpsertComments(storeTokenRow("00000000-0000-0000-0000-0000000000c5", `,"incidents_id":"i1"`)))
	must("UpsertDomains", db.UpsertDomains(storeTokenRow("00000000-0000-0000-0000-0000000000c6", `,"web_address_rules_id":"w1"`)))

	// The rows are there, with the marker in place of the link.
	for table, want := range map[string]int{"resources": 9, "device_groups_devices": 2, "runs": 1, "basic_attributes_update": 1, "comments": 1, "domains": 1} {
		var n int
		must("count "+table, db.DB().QueryRow(`SELECT count(*) FROM "`+table+`" WHERE data LIKE '%[redacted]%'`).Scan(&n))
		if n != want {
			t.Errorf("%s: %d rows carry the [redacted] marker, want %d", table, n, want)
		}
	}
	var fts string
	must("fts", db.DB().QueryRow(`SELECT group_concat(content, ' ') FROM resources_fts`).Scan(&fts))
	if strings.Contains(fts, storeLinkSentinel) || !strings.Contains(fts, "Acme agent") {
		t.Errorf("FTS content must index the row without its link: %q", fts)
	}
	must("close", db.Close())
	if hits := dbFilesHold(t, dir, storeLinkSentinel); len(hits) > 0 {
		t.Fatalf("an installer download link reached the database files: %v", hits)
	}
}

// TestStoreMergeNeverRestoresALink covers the merge half of the funnel: a row
// an earlier build stored with a real link is rewritten redacted by the next
// write, and keep-richer merging cannot carry the old link forward.
func TestStoreMergeNeverRestoresALink(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	id := "00000000-0000-0000-0000-0000000000b1"
	if _, err := db.DB().Exec(`INSERT INTO resources (id, resource_type, data, synced_at, updated_at) VALUES (?, 'installers', ?, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`, id, string(storeLinkRow(id, `,"activatedProducts":[{"name":"x"}]`))); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.UpsertBatch("installers", []json.RawMessage{json.RawMessage(`{"uuid":"` + id + `","displayName":"Acme agent"}`)}); err != nil {
		t.Fatal(err)
	}
	got, err := db.Get("installers", id)
	if err != nil {
		t.Fatal(err)
	}
	// sync-rows-replace-wholesale: the incoming row replaces the stored one,
	// so neither the old link nor the old fields come back.
	if strings.Contains(string(got), storeLinkSentinel) || strings.Contains(string(got), "activatedProducts") {
		t.Fatalf("the replaced row must not restore the old link or fields: %s", got)
	}
	// A typed table receives the merged row directly, so the merge itself
	// must hand it over redacted.
	cid := "00000000-0000-0000-0000-0000000000c5"
	legacy := storeTokenRow(cid, `,"incidents_id":"i1"`)
	legacyObj, err := DecodeJSONObject(legacy)
	if err != nil {
		t.Fatal(err)
	}
	storageID := resourceStorageID("comments", ResolveStorageID("comments", legacyObj), legacyObj)
	if _, err := db.DB().Exec(`INSERT INTO resources (id, resource_type, data, synced_at, updated_at) VALUES (?, 'comments', ?, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`, storageID, string(legacy)); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertComments(json.RawMessage(`{"uuid":"` + cid + `","incidents_id":"i1","text":"note"}`)); err != nil {
		t.Fatal(err)
	}
	var typed string
	if err := db.DB().QueryRow(`SELECT data FROM comments WHERE id = ?`, storageID).Scan(&typed); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(typed, storeLinkSentinel) {
		t.Fatalf("the typed comments row carried the old link forward through the merge: %s", typed)
	}
}

// TestStoreGenericInsertRedactsOnItsOwn pins the second half of the funnel:
// the resources/FTS insert redacts even when a caller hands it a row that did
// not come through the merge step.
func TestStoreGenericInsertRedactsOnItsOwn(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.DB().Begin()
	if err != nil {
		t.Fatal(err)
	}
	id := "00000000-0000-0000-0000-0000000000b9"
	if err := db.upsertGenericResourceTx(tx, "installers", id, storeLinkRow(id, "")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var data, content string
	if err := db.DB().QueryRow(`SELECT r.data, f.content FROM resources r JOIN resources_fts f ON f.id = r.id AND f.resource_type = r.resource_type WHERE r.id = ?`, id).Scan(&data, &content); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(data, storeLinkSentinel) || strings.Contains(content, storeLinkSentinel) {
		t.Fatalf("the generic insert stored the link: data=%s fts=%s", data, content)
	}
}

const storePropertySentinel = "STORESECRET-2M6P"

// TestStoreFunnelPropertyNeverPersistsTheSentinel is the store half of the
// round-3 class test for credential-tier-never-persisted: for each
// credential-bearing resource type, a sample of the credvariants corpus for
// that type's keys (valid, malformed, truncated, concatenated and non-JSON
// variants) goes through Upsert and UpsertBatch, and the deviceToken corpus
// through a typed Upsert. A row that is not one JSON value is refused with
// ErrNotPersistable and leaves no row; no byte of the database files holds a
// recoverable sentinel afterwards.
func TestStoreFunnelPropertyNeverPersistsTheSentinel(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	stored := 0
	for _, resourceType := range []string{"installers", "gpo-sccm-files-generate", "mobile-devices-batch-get-enrollment-links", "devices", "comments"} {
		corpus := credvariants.Corpus(cliutil.PersistCredentialKeysFor(resourceType), storePropertySentinel, cliutil.PersistRedactedValue, 41, false)
		corpus = append(corpus, `{}]{"deviceToken":"`+storePropertySentinel+`"}`)
		for i, v := range corpus {
			if i%4 != 0 && i != len(corpus)-1 {
				continue
			}
			id := fmt.Sprintf("%s-row-%d", resourceType, i)
			if resourceType == "comments" {
				_ = db.UpsertComments(json.RawMessage(v))
				continue
			}
			err := db.Upsert(resourceType, id, json.RawMessage(v))
			valid := json.Valid([]byte(v))
			if valid != (err == nil) {
				t.Fatalf("Upsert(%s, %q) err = %v; want an error exactly when the row is not one JSON value", resourceType, v, err)
			}
			if err != nil && !errors.Is(err, cliutil.ErrNotPersistable) {
				t.Fatalf("Upsert(%s, %q) failed for another reason: %v", resourceType, v, err)
			}
			if err == nil {
				stored++
			} else if _, gerr := db.Get(resourceType, id); gerr == nil {
				t.Fatalf("a refused row was stored: %q", v)
			}
			_, _, _ = db.UpsertBatch(resourceType, []json.RawMessage{json.RawMessage(v)})
		}
	}
	if stored < 400 {
		t.Fatalf("too few rows stored to mean anything: %d", stored)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err == nil && credvariants.Recoverable(b, storePropertySentinel) {
			t.Fatalf("the database file %s holds a recoverable sentinel", e.Name())
		}
	}
}

// TestStoreKeepsForensicURIsOutsideCredentialSchemas (round 3, adversarial
// P2): a v2 detection's triggeringEvent.data.uri (a free-form object) is
// stored and indexed as it is, while an installer row's downloadUrl in the
// same database is redacted.
func TestStoreKeepsForensicURIsOutsideCredentialSchemas(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	det := json.RawMessage(`{"uuid":"00000000-0000-0000-0000-0000000000d2","displayName":"Web threat","triggeringEvent":{"type":"web","data":{"uri":"https://evil.example/LEGIT-EVENT-URI-R3","downloadUrl":"https://evil.example/payload.exe"}}}`)
	if _, _, err := db.UpsertBatch("detections_v2", []json.RawMessage{det}); err != nil {
		t.Fatal(err)
	}
	if err := db.Upsert("installers", "00000000-0000-0000-0000-0000000000b1", storeLinkRow("00000000-0000-0000-0000-0000000000b1", "")); err != nil {
		t.Fatal(err)
	}
	got, err := db.Get("detections_v2", "00000000-0000-0000-0000-0000000000d2")
	if err != nil || !strings.Contains(string(got), "https://evil.example/LEGIT-EVENT-URI-R3") || !strings.Contains(string(got), "payload.exe") {
		t.Fatalf("the detection's forensic URIs were altered: %s %v", got, err)
	}
	// Local search finds the detection and hands back the row with its URI.
	// (The generated FTS index never tokenizes http(s) URLs, with or without
	// redaction, so the row is found by its other text.)
	hits, err := db.Search("Web threat", 10)
	if err != nil || len(hits) != 1 || !strings.Contains(string(hits[0]), "https://evil.example/LEGIT-EVENT-URI-R3") {
		t.Fatalf("search must return the detection with its URI: %s %v", hits, err)
	}
	var n int
	if err := db.DB().QueryRow(`SELECT count(*) FROM resources WHERE resource_type = 'detections_v2' AND data LIKE '%evil.example/LEGIT-EVENT-URI-R3%'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("sql over the mirror must find the URI: %d %v", n, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if hits := dbFilesHold(t, dir, storeLinkSentinel); len(hits) > 0 {
		t.Fatalf("the installer link reached the database files: %v", hits)
	}
}
