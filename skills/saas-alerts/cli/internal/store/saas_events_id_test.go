// Hand-written for saas-alerts (not generated): events storage identity through
// the same batch upsert sync uses.

package store

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaasEventStorageIDThroughBatchUpsert(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ev := func(s string) json.RawMessage { return json.RawMessage(s) }
	items := []json.RawMessage{
		// Same raw eventId at two customers, two products, two users: four events.
		ev(`{"eventId":"logins_2026-10-13T07:36:46Z","customer":{"id":"C1"},"product":{"name":"Microsoft 365"},"user":{"name":"a@example.com"},"time":"2026-10-13T07:36:46Z"}`),
		ev(`{"eventId":"logins_2026-10-13T07:36:46Z","customer":{"id":"C2"},"product":{"name":"Microsoft 365"},"user":{"name":"a@example.com"},"time":"2026-10-13T07:36:46Z"}`),
		ev(`{"eventId":"logins_2026-10-13T07:36:46Z","customer":{"id":"C1"},"product":{"name":"Dropbox"},"user":{"name":"a@example.com"},"time":"2026-10-13T07:36:46Z"}`),
		ev(`{"eventId":"logins_2026-10-13T07:36:46Z","customer":{"id":"C1"},"product":{"name":"Microsoft 365"},"user":{"name":"b@example.com"},"time":"2026-10-13T07:36:46Z"}`),
		// Exact re-delivery of the first event: collapses.
		ev(`{"eventId":"logins_2026-10-13T07:36:46Z","customer":{"id":"C1"},"product":{"name":"Microsoft 365"},"user":{"name":"a@example.com"},"time":"2026-10-13T07:36:46Z"}`),
		// _id wins: same _id collapses even with a different eventId.
		ev(`{"_id":"doc-1","eventId":"x","customer":{"id":"C3"},"time":"2026-10-13T09:00:00Z"}`),
		ev(`{"_id":"doc-1","eventId":"y","customer":{"id":"C3"},"time":"2026-10-13T09:00:00Z"}`),
		// No ids at all: kept, distinct by content.
		ev(`{"eventId":"","jointType":"outside.own.location","time":"2026-10-13T08:00:00Z"}`),
		ev(`{"eventId":"","jointType":"outside.own.location","time":"2026-10-13T08:00:01Z"}`),
	}
	stored, failures, err := db.UpsertBatch("events", items)
	if err != nil || failures != 0 {
		t.Fatalf("upsert: stored=%d failures=%d err=%v", stored, failures, err)
	}
	var n int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM resources WHERE resource_type = 'events'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 7 {
		t.Fatalf("stored events = %d, want 7 (4 scoped + 1 _id + 2 content-hash)", n)
	}
}

func TestSaasEventStorageIDNeverBareEventID(t *testing.T) {
	if id := saasEventStorageID(map[string]any{"eventId": "logins_2026-10-13T07:36:46Z", "time": "2026-10-13T07:36:46Z"}); id == "logins_2026-10-13T07:36:46Z" || id == "" {
		t.Fatalf("unscoped eventId used as identity: %q", id)
	}
}

func TestSaasEventStorageIDIgnoresDisplayNames(t *testing.T) {
	base := func(extra string) map[string]any {
		var m map[string]any
		if err := json.Unmarshal([]byte(`{"eventId":"logins_1","time":"2026-10-13T07:36:46.000+00:00","customer":{"id":"C1"},"product":{"name":"Microsoft 365"},"user":{"name":"Charles Buck"}`+extra+`}`), &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	a := saasEventStorageID(base(`,"UserId":"U1"`))
	b := saasEventStorageID(base(`,"UserId":"U2"`))
	c := saasEventStorageID(base(`,"userEmail":"c1@example.com"`))
	d := saasEventStorageID(base(`,"userEmail":"c2@example.com"`))
	if a == b || c == d {
		t.Fatalf("same display name, different account collapsed: %q %q %q %q", a, b, c, d)
	}
	e := saasEventStorageID(base(``))
	f := saasEventStorageID(base(`,"ip":"10.0.0.2"`))
	if e == f || !strings.HasPrefix(e, "evth:") {
		t.Fatalf("display-name-only events must hash content: %q %q", e, f)
	}
}

func TestSaasEventStorageIDRejectsErrorBodies(t *testing.T) {
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	db, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, body := range []string{
		`{"message":"backend failure"}`,
		`{"id":"REQUEST_ERROR_123","message":"backend failure"}`,
		`{"_id":"doc-9","message":"backend failure"}`,
		`{"_id":"doc-9","eventId":"x","time":"not-a-time"}`,
		`{"eventId":"logins_1","time":""}`,
		`{"_id":"doc-9","time":"2026-10-14T12:00:00Z","success":false,"error":"backend failure"}`,
		`{"_id":"doc-9","time":"2026-10-14T12:00:00Z","ok":false}`,
		`{"_id":"doc-9","time":"2026-10-14T12:00:00Z","errors":["x"]}`,
		`{"_id":"doc-9","time":"2026-10-14T12:00:00Z","message":"backend failure"}`,
		`{"_id":"doc-9","time":"2026-10-14T12:00:00Z","code":500}`,
		`{"eventId":"logins_1","time":"2026-10-14T12:00:00Z","code":"500","customer":{"id":"C1"},"product":{"id":"P1"},"user":{"id":"U1"}}`,
	} {
		var m map[string]any
		if err := json.Unmarshal([]byte(body), &m); err != nil {
			t.Fatal(err)
		}
		if id := saasEventStorageID(m); id != "" {
			t.Errorf("%s got an event identity %q", body, id)
		}
		if err := db.UpsertEvents(json.RawMessage(body)); err == nil {
			t.Errorf("%s was stored as an event", body)
		}
	}
	var n int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM resources WHERE resource_type = 'events'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("stored rows = %d (err %v), want 0", n, err)
	}
}

func TestSaasEventStorageIDVerifyHarnessOnly(t *testing.T) {
	fixture := json.RawMessage(`{"id": 1, "name": "mock-item-1", "created_at": "2026-03-27T00:00:00Z"}`)
	for _, tc := range []struct {
		env   string
		store bool
	}{{"", false}, {"1", true}} {
		t.Setenv("PRINTING_PRESS_VERIFY", tc.env)
		db, err := Open(filepath.Join(t.TempDir(), "data.db"))
		if err != nil {
			t.Fatal(err)
		}
		err = db.UpsertEvents(fixture)
		db.Close()
		if stored := err == nil; stored != tc.store {
			t.Fatalf("PRINTING_PRESS_VERIFY=%q: stored=%v err=%v, want stored=%v", tc.env, stored, err, tc.store)
		}
	}
}

func TestSaasEventStorageIDKeepsDocumentedStatus(t *testing.T) {
	var m map[string]any
	body := `{"_id":"doc-1","time":"2026-10-14T12:00:00Z","status":"open","code":200,"success":true,"message":""}`
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatal(err)
	}
	if id := saasEventStorageID(m); id != "doc-1" {
		t.Fatalf("a real event with a status field got identity %q, want doc-1", id)
	}
}

func TestSaasEventBatchRejectsWrappedAndScalarRows(t *testing.T) {
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	db, err := Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	seed := json.RawMessage(`{"_id":"doc-1","time":"2026-10-13T07:36:46Z","type":"logins"}`)
	if stored, fails, _, err := db.UpsertBatchDetailed("events", []json.RawMessage{seed}); err != nil || stored != 1 || fails != 0 {
		t.Fatalf("seed: stored=%d fails=%d err=%v", stored, fails, err)
	}
	var before string
	if err := db.DB().QueryRow(`SELECT data FROM resources WHERE resource_type = 'events' AND id = 'doc-1'`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	for name, items := range map[string][]json.RawMessage{
		"failure row wrapping a stored event": {json.RawMessage(`{"time":"2026-10-14T12:00:00Z","success":false,"error":"backend failure","event":{"_id":"doc-1","time":"2026-10-13T07:36:46Z"}}`)},
		"scalar member beside a valid event":  {json.RawMessage(`{"_id":"doc-2","time":"2026-10-14T12:00:00Z"}`), json.RawMessage(`"backend failure"`)},
	} {
		_, fails, _, err := db.UpsertBatchDetailed("events", items)
		if err != nil || fails != 1 {
			t.Errorf("%s: extract failures=%d err=%v, want 1", name, fails, err)
		}
	}
	var after string
	if err := db.DB().QueryRow(`SELECT data FROM resources WHERE resource_type = 'events' AND id = 'doc-1'`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("stored event was overwritten:\nbefore %s\nafter  %s", before, after)
	}
}
