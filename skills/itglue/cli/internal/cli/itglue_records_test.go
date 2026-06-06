// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Tests for the IT Glue novel-feature helpers and a shared seeded-store harness.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"itglue-pp-cli/internal/store"
)

// recentRFC3339 is a timestamp comfortably inside any reasonable staleness
// window, formatted at test time.
func recentRFC3339() string {
	return time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
}

// itgSeedRecord is one JSON:API record to upsert into a test store.
type itgSeedRecord struct {
	rtype string
	id    string
	json  string
}

// itgSeedRecords returns a small, deterministic MSP tenant:
//   - 3 orgs (Acme has children; Globex has a contact+document; Initech is empty)
//   - 2 contacts in Acme that are duplicates (same name + email), 1 in Globex
//   - 2 passwords in Acme (one stale from 2020, one fresh)
//   - 1 configuration in Acme; 1 document in Globex
func itgSeedRecords() []itgSeedRecord {
	recent := recentRFC3339()
	return []itgSeedRecord{
		{"organizations", "1", fmt.Sprintf(`{"id":"1","type":"organizations","attributes":{"name":"Acme","organization-type-name":"Customer","organization-status-name":"Active","updated-at":%q}}`, recent)},
		{"organizations", "2", fmt.Sprintf(`{"id":"2","type":"organizations","attributes":{"name":"Globex","updated-at":%q}}`, recent)},
		{"organizations", "3", fmt.Sprintf(`{"id":"3","type":"organizations","attributes":{"name":"Initech","updated-at":%q}}`, recent)},

		{"contacts", "10", fmt.Sprintf(`{"id":"10","type":"contacts","attributes":{"name":"Jane Doe","organization-id":1,"organization-name":"Acme","contact-emails":[{"value":"jane@acme.com","primary":true}],"updated-at":%q}}`, recent)},
		{"contacts", "11", fmt.Sprintf(`{"id":"11","type":"contacts","attributes":{"name":"jane  doe","organization-id":1,"organization-name":"Acme","contact-emails":[{"value":"JANE@acme.com","primary":true}],"updated-at":%q}}`, recent)},
		{"contacts", "12", fmt.Sprintf(`{"id":"12","type":"contacts","attributes":{"first-name":"Bob","last-name":"Smith","organization-id":2,"organization-name":"Globex","contact-emails":[{"value":"bob@globex.com","primary":true}],"updated-at":%q}}`, recent)},

		{"passwords", "20", `{"id":"20","type":"passwords","attributes":{"name":"Firewall admin","organization-id":1,"organization-name":"Acme","username":"admin","updated-at":"2020-01-01T00:00:00Z"}}`},
		{"passwords", "21", fmt.Sprintf(`{"id":"21","type":"passwords","attributes":{"name":"VPN","organization-id":1,"organization-name":"Acme","updated-at":%q}}`, recent)},

		{"configurations", "30", fmt.Sprintf(`{"id":"30","type":"configurations","attributes":{"name":"FW-01","organization-id":1,"organization-name":"Acme","serial-number":"SN-ABC123","primary-ip":"10.0.0.1","updated-at":%q}}`, recent)},

		{"documents", "40", `{"id":"40","type":"documents","attributes":{"name":"Runbook","organization-id":2,"organization-name":"Globex","updated-at":"2021-06-01T00:00:00Z"}}`},
	}
}

// seedITGStore points HOME at a temp dir and writes the seed tenant into the
// canonical defaultDBPath so the novel commands (which open that path) read it.
func seedITGStore(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	dbPath := defaultDBPath("itglue-cli")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	for _, r := range itgSeedRecords() {
		if err := db.Upsert(r.rtype, r.id, json.RawMessage(r.json)); err != nil {
			t.Fatalf("upsert %s/%s: %v", r.rtype, r.id, err)
		}
	}
}

// runITGCmd executes the CLI in-process and returns stdout.
func runITGCmd(t *testing.T, args ...string) string {
	t.Helper()
	root := RootCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("execute %v: %v", args, err)
	}
	return buf.String()
}

func TestParseITGRecord(t *testing.T) {
	raw := json.RawMessage(`{"id":"10","type":"contacts","attributes":{"name":"Jane Doe","organization-id":1,"organization-name":"Acme","contact-emails":[{"value":"jane@acme.com","primary":true}],"updated-at":"2024-03-04T05:06:07Z"}}`)
	rec, ok := parseITGRecord(raw)
	if !ok {
		t.Fatal("parse failed")
	}
	if rec.ID != "10" {
		t.Errorf("ID = %q, want 10", rec.ID)
	}
	if rec.Type != "contacts" {
		t.Errorf("Type = %q, want contacts", rec.Type)
	}
	if got := rec.displayName(); got != "Jane Doe" {
		t.Errorf("displayName = %q, want Jane Doe", got)
	}
	if got := rec.orgID(); got != "1" {
		t.Errorf("orgID = %q, want 1", got)
	}
	if got := rec.orgName(); got != "Acme" {
		t.Errorf("orgName = %q, want Acme", got)
	}
	if got := rec.contactEmail(); got != "jane@acme.com" {
		t.Errorf("contactEmail = %q, want jane@acme.com", got)
	}
	ts, ok := rec.updatedAt()
	if !ok || ts.Year() != 2024 {
		t.Errorf("updatedAt = %v ok=%v, want 2024", ts, ok)
	}
}

func TestParseITGRecordOrgSelf(t *testing.T) {
	rec, _ := parseITGRecord(json.RawMessage(`{"id":"7","type":"organizations","attributes":{"name":"Acme"}}`))
	if rec.orgID() != "7" {
		t.Errorf("org orgID = %q, want 7 (its own id)", rec.orgID())
	}
	if rec.orgName() != "Acme" {
		t.Errorf("org orgName = %q, want Acme", rec.orgName())
	}
}

func TestContactNameFromFirstLast(t *testing.T) {
	rec, _ := parseITGRecord(json.RawMessage(`{"id":"12","type":"contacts","attributes":{"first-name":"Bob","last-name":"Smith"}}`))
	if got := rec.displayName(); got != "Bob Smith" {
		t.Errorf("displayName = %q, want Bob Smith", got)
	}
}

func TestParseITGTime(t *testing.T) {
	cases := []struct {
		in string
		ok bool
	}{
		{"2024-03-04T05:06:07Z", true},
		{"2024-03-04T05:06:07.123Z", true},
		{"2024-03-04T05:06:07-05:00", true},
		{"2024-03-04", true},
		{"", false},
		{"not-a-date", false},
	}
	for _, c := range cases {
		_, ok := parseITGTime(c.in)
		if ok != c.ok {
			t.Errorf("parseITGTime(%q) ok = %v, want %v", c.in, ok, c.ok)
		}
	}
}

func TestParseSinceArg(t *testing.T) {
	now := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	// empty → zero time, no error
	if ts, err := parseSinceArg("", now); err != nil || !ts.IsZero() {
		t.Errorf("empty: ts=%v err=%v", ts, err)
	}
	// 7d window
	if ts, err := parseSinceArg("7d", now); err != nil || !ts.Equal(now.Add(-7*24*time.Hour)) {
		t.Errorf("7d: ts=%v err=%v", ts, err)
	}
	// 24h window
	if ts, err := parseSinceArg("24h", now); err != nil || !ts.Equal(now.Add(-24*time.Hour)) {
		t.Errorf("24h: ts=%v err=%v", ts, err)
	}
	// absolute date
	if ts, err := parseSinceArg("2026-01-02", now); err != nil || ts.Year() != 2026 || ts.Month() != 1 {
		t.Errorf("date: ts=%v err=%v", ts, err)
	}
	// invalid
	if _, err := parseSinceArg("garbage", now); err == nil {
		t.Error("garbage: expected error")
	}
}

func TestNormalizeDupeKey(t *testing.T) {
	if normalizeDupeKey("  Jane   Doe ") != normalizeDupeKey("jane doe") {
		t.Error("normalization should make 'Jane  Doe' == 'jane doe'")
	}
	if normalizeDupeKey("") != "" {
		t.Error("empty stays empty")
	}
}

func TestFTSQuote(t *testing.T) {
	cases := []struct{ in, want string }{
		{"SN-ABC123", `"SN-ABC123"`},
		{"error timeout", `"error" "timeout"`},
		{"", `""`},
		{`a"b`, `"a""b"`},
	}
	for _, c := range cases {
		if got := ftsQuote(c.in); got != c.want {
			t.Errorf("ftsQuote(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSummaryProjection(t *testing.T) {
	rec, _ := parseITGRecord(json.RawMessage(`{"id":"30","type":"configurations","attributes":{"name":"FW-01","organization-id":1,"organization-name":"Acme","serial-number":"SN-ABC123","primary-ip":"10.0.0.1","updated-at":"2026-01-02T00:00:00Z"}}`))
	s := rec.summary()
	if s["resource_type"] != "configurations" || s["id"] != "30" || s["name"] != "FW-01" {
		t.Errorf("summary core fields wrong: %#v", s)
	}
	if s["organization"] != "Acme" || s["organization_id"] != "1" {
		t.Errorf("summary org fields wrong: %#v", s)
	}
	if s["serial_number"] != "SN-ABC123" || s["primary_ip"] != "10.0.0.1" {
		t.Errorf("summary config hints wrong: %#v", s)
	}
}
