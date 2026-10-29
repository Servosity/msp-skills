// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pax8-pp-cli/internal/cliutil"
)

// Class store-write-bypasses-chokepoint: whichever writer reaches the
// read-write store (a learning, a playbook, a learn event, a raw statement,
// INSERT ... RETURNING through QueryRow), a credential the process holds is
// never written to the database or its journal files, and the row itself is.
func TestEveryStoreWriteWithholdsHeldValues(t *testing.T) {
	const secret = "storeHeldSecret-7c2e9a4f1b3d"
	cliutil.FunnelHold(secret)
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, _, err := s.UpsertLearning(ctx, UpsertLearningInput{Query: "find networks " + secret, ResourceID: "res-" + secret,
		ResourceType: "networks", Notes: "rotated " + secret}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.UpsertPlaybook(UpsertPlaybookInput{QueryFamily: "fam-" + secret, PlaybookJSON: `{"steps":["use ` + secret + `"]}`,
		NotesText: "playnote " + secret}); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertLearnEvent(LearnEventTeach, "hash-"+secret, 1, false, "cli"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE pax8_probe (id INTEGER PRIMARY KEY, v TEXT, b BLOB)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO pax8_probe (v, b) VALUES (?, ?)`, "exec "+secret, []byte("blob "+secret)); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := s.db.QueryRowContext(ctx, `INSERT INTO pax8_probe (v) VALUES (?) RETURNING id`, "returning "+secret).Scan(&id); err != nil {
		t.Fatal(err)
	}
	stmt, err := s.db.PrepareContext(ctx, `INSERT INTO pax8_probe (v) VALUES (?)`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stmt.ExecContext(ctx, "prepared "+secret); err != nil {
		t.Fatal(err)
	}
	_ = stmt.Close()
	if _, err := s.db.ExecContext(ctx, `INSERT INTO pax8_probe (v) VALUES ('`+secret+`')`); err == nil {
		t.Errorf("a statement whose text repeats a held credential was executed")
	}
	// Reads still run, and a lookup for the raw value finds no row: the
	// write kept only the withheld form.
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM pax8_probe WHERE v = ?`, "exec "+secret).Scan(&n); err != nil || n != 0 {
		t.Errorf("a lookup for the raw value found %d rows (err %v): the write kept it", n, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	var all []byte
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		all = append(all, b...)
		if bytes.Contains(b, []byte(secret)) || bytes.Contains(b, []byte(secret[:len(secret)/2])) && bytes.Contains(b, []byte(secret[len(secret)/2:])) {
			t.Errorf("%s holds the held credential", e.Name())
		}
	}
	for _, want := range []string{"rotated", "playnote", "exec", "blob", "returning", "prepared"} {
		if !strings.Contains(string(all), want) {
			t.Errorf("the row written as %q is missing; the write was dropped, not withheld", want)
		}
	}
}

// Class credential-persisted-in-files, by name: a credential-named value the
// process never held (a record that did not arrive as an API response) is
// redacted by the resource-row strip, at any depth, whole containers under a
// credential name included (the chokepoint's member redaction covers string
// members only), and in the typed tables (users and devices rows
// carrying a password or token member); the record is kept.
func TestStoreRedactsCredentialNamedValuesNotHeld(t *testing.T) {
	const psk, client = "unheldPreSharedKey-3b7d1e9c", "unheldClientSecret-8a2f6c4e"
	const password, token = "unheldUserPass-5e1c7a3f", "unheldDeviceToken-2d9b4f6a"
	const inArray, inObject = "unheldSecretInArray-6c1e8b3d", "unheldKeyInObject-4f7a2c9e"
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	rec := `{"id":77,"name":"Branch-Connector","client":{"authentication":{"parameters":{"psk":"` + psk + `"}}},"keys":[{"clientSecret":"` + client + `"}]}`
	if err := s.Upsert("outbound-connectors", "77", []byte(rec)); err != nil {
		t.Fatal(err)
	}
	if err := s.Upsert("outbound-connectors", "78", []byte(`{"id":78,"name":"Second-Connector","secrets":["`+inArray+`"],"key":{"label":"`+inObject+`"}}`)); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertProducts([]byte(`{"id":"5c802b19-3600-83f1-1767-7b9edc7f38ab","name":"Ada","passwordProfile":{"password":"` + password + `"}}`)); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertCompanies([]byte(`{"id":"000005c3-b7a6-4c61-89fc-80bf5ccfc366","name":"laptop01","DeviceAccessToken":"` + token + `"}`)); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	var all []byte
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if b, err := os.ReadFile(filepath.Join(dir, e.Name())); err == nil {
			all = append(all, b...)
		}
	}
	for _, v := range []string{psk, client, password, token, inArray, inObject} {
		if bytes.Contains(all, []byte(v)) {
			t.Errorf("the store kept the credential-named value %q", v)
		}
	}
	for _, v := range []string{"Branch-Connector", "Second-Connector", "Ada", "laptop01"} {
		if !bytes.Contains(all, []byte(v)) {
			t.Errorf("the record holding %q was not stored", v)
		}
	}
}
