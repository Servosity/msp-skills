// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package store

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"appdirect-pp-cli/internal/cliutil"
)

// Class store-write-bypasses-chokepoint: whichever writer reaches the
// read-write store (a learning, a playbook, a learn event, a raw statement,
// INSERT ... RETURNING through QueryRow), a credential the process holds is
// never written to the database or its journal files, and the row itself is.
func TestEveryStoreWriteWithholdsHeldValues(t *testing.T) {
	isolateHome(t)
	const secret = "storeHeldSecret-7c2e9a4f1b3d"
	cliutil.FunnelHold(secret)
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, _, err := s.UpsertLearning(ctx, UpsertLearningInput{Query: "find subscriptions " + secret, ResourceID: "res-" + secret,
		ResourceType: "billing-v1-subscriptions", Notes: "rotated " + secret}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.UpsertPlaybook(UpsertPlaybookInput{QueryFamily: "fam-" + secret, PlaybookJSON: `{"steps":["use ` + secret + `"]}`,
		NotesText: "playnote " + secret}); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertLearnEvent(LearnEventTeach, "hash-"+secret, 1, false, "cli"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE appdirect_probe (id INTEGER PRIMARY KEY, v TEXT, b BLOB)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO appdirect_probe (v, b) VALUES (?, ?)`, "exec "+secret, []byte("blob "+secret)); err != nil {
		t.Fatal(err)
	}
	var id int64
	if err := s.db.QueryRowContext(ctx, `INSERT INTO appdirect_probe (v) VALUES (?) RETURNING id`, "returning "+secret).Scan(&id); err != nil {
		t.Fatal(err)
	}
	stmt, err := s.db.PrepareContext(ctx, `INSERT INTO appdirect_probe (v) VALUES (?)`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stmt.ExecContext(ctx, "prepared "+secret); err != nil {
		t.Fatal(err)
	}
	_ = stmt.Close()
	if _, err := s.db.ExecContext(ctx, `INSERT INTO appdirect_probe (v) VALUES ('`+secret+`')`); err == nil {
		t.Errorf("a statement whose text repeats a held credential was executed")
	}
	// Reads still run, and a lookup for the raw value finds no row: the
	// write kept only the withheld form.
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM appdirect_probe WHERE v = ?`, "exec "+secret).Scan(&n); err != nil || n != 0 {
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
// members only), and in the typed tables (an account user row carrying a
// password, an assisted-sales opportunity carrying an access token); the
// record is kept.
func TestStoreRedactsCredentialNamedValuesNotHeld(t *testing.T) {
	const psk, client = "unheldPreSharedKey-3b7d1e9c", "unheldClientSecret-8a2f6c4e"
	const password, token = "unheldUserPass-5e1c7a3f", "unheldDeviceToken-2d9b4f6a"
	const inArray, inObject = "unheldSecretInArray-6c1e8b3d", "unheldKeyInObject-4f7a2c9e"
	isolateHome(t)
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	// AppDirect-shaped records: an integration whose parameters carry a
	// pre-shared key and a client secret, a setting with a secret list and a
	// key object, an account user and an assisted-sales opportunity (typed
	// tables account and assisted_sales).
	rec := `{"id":"ds-77","name":"Branch-Connector","script":{"authentication":{"parameters":{"psk":"` + psk + `"}}},"keys":[{"clientSecret":"` + client + `"}]}`
	if err := s.Upsert("integration", "ds-77", []byte(rec)); err != nil {
		t.Fatal(err)
	}
	if err := s.Upsert("account-v2-companies", "set-78", []byte(`{"id":"set-78","name":"Second-Connector","secrets":["`+inArray+`"],"key":{"label":"`+inObject+`"}}`)); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertAccount([]byte(`{"id":"5c802b19-3600-83f1-1767-7b9edc7f38ab","firstName":"Ada","email":"ada@example.com","passwordProfile":{"password":"` + password + `"}}`)); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertAssistedSales([]byte(`{"id":"000005c3-b7a6-4c61-89fc-80bf5ccfc366","name":"laptop01","status":"OPEN","integrationAccessToken":"` + token + `"}`)); err != nil {
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

// isolateHome points every config, data, state, cache and credential path at
// a temp dir, so no store test can reach the operator's own files.
func isolateHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	for k, v := range map[string]string{"APPDIRECT_HOME": home, "HOME": home, "USERPROFILE": home,
		"XDG_CONFIG_HOME": filepath.Join(home, ".config"), "XDG_DATA_HOME": filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME": filepath.Join(home, ".local", "state"), "XDG_CACHE_HOME": filepath.Join(home, ".cache"),
		"APPDIRECT_CONFIG": "", "APPDIRECT_CONFIG_DIR": "", "APPDIRECT_DATA_DIR": "", "APPDIRECT_STATE_DIR": "", "APPDIRECT_CACHE_DIR": ""} {
		t.Setenv(k, v)
	}
}
