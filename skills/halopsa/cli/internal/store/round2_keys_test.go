package store

import (
	"path/filepath"
	"testing"
)

// team-tree numbers its nodes within a type, so two nodes both carry id 16.
// Every row also carries a guid, which the generic fallback list only reaches
// after `id` and so never used (#264).
func TestTeamTreeKeyedByGUID(t *testing.T) {
	rows := []string{
		`{"id":16,"guid":"a3b34d24-e2c5-4a8f-a5ae-857003acfd01","type":1,"type_name":"Opportunities","sequence":110}`,
		`{"id":16,"guid":"b7c45e35-f3d6-5b90-b6bf-968114bd0e12","type":2,"type_name":"Projects","sequence":120}`,
	}
	keys, failures := storedKeys(t, "team-tree", rows)
	if failures != 0 {
		t.Fatalf("%d team-tree row(s) failed id extraction", failures)
	}
	if len(keys) != 2 {
		t.Fatalf("two tree nodes sharing id 16 collapsed into %d stored key(s)", len(keys))
	}
	if got := ExtractResourceID("team-tree", decode(t, rows[0])); got != "a3b34d24-e2c5-4a8f-a5ae-857003acfd01" {
		t.Errorf("team-tree id resolved to %q, want the guid", got)
	}
}

// A row without a guid must still resolve through the generic fallback rather
// than failing extraction outright, so a shape surprise degrades to the old
// behaviour instead of dropping the row.
func TestTeamTreeWithoutGUIDFallsBackToID(t *testing.T) {
	if got := ExtractResourceID("team-tree", decode(t, `{"id":16,"type":1}`)); got != "16" {
		t.Errorf("team-tree row without a guid resolved to %q, want the bare id %q", got, "16")
	}
}

// /Timesheet/forecasting has the same shape as /Timesheet: "id": 0 on every
// record, keyed in reality by the agent and the day (#264).
func TestTimesheetForecastingKeyedByAgentAndDate(t *testing.T) {
	rows := []string{
		`{"id":0,"agent_id":1,"date":"2026-10-25T19:13:41.4892555+00:00","target_hours":0,"workdayid":1}`,
		`{"id":0,"agent_id":1,"date":"2026-10-26T19:13:41.4892555+00:00","target_hours":8,"workdayid":1}`,
		`{"id":0,"agent_id":2,"date":"2026-10-25T19:13:41.4892555+00:00","target_hours":8,"workdayid":1}`,
	}
	keys, failures := storedKeys(t, "timesheet-forecasting", rows)
	if failures != 0 {
		t.Fatalf("%d forecasting row(s) failed id extraction", failures)
	}
	if len(keys) != 3 {
		t.Fatalf("3 agent-days collapsed into %d stored key(s)", len(keys))
	}
}

// integration-runbook-variable-group carries no id-shaped field at all; its
// rows are {label, value} pairs where value is the stable key. A string key
// needs no special handling (#264).
func TestRunbookVariableGroupKeyedByValue(t *testing.T) {
	rows := []string{
		`{"label":"Ticket Variables","value":"faults"}`,
		`{"label":"Client Variables","value":"client"}`,
		`{"label":"Runbook: Example Automation","value":"runbookc9f1a2b3-45de-6f78-9a01-bcdef2345678"}`,
	}
	keys, failures := storedKeys(t, "integration-runbook-variable-group", rows)
	if failures != 0 {
		t.Fatalf("%d runbook-variable-group row(s) still fail id extraction", failures)
	}
	if len(keys) != 3 {
		t.Fatalf("3 variable groups collapsed into %d stored key(s)", len(keys))
	}
}

// feed is deliberately NOT rekeyed. Its id is globally unique (the endpoint's
// own newer_than_id/older_than_id contract depends on that), so two rows
// carrying the same id are the SAME row re-fetched, and collapsing them is
// correct rather than lossy. Pinning this stops a future reader from "fixing"
// feed by adding a parent key and turning honest dedup into duplicate rows.
func TestFeedIsNotParentScoped(t *testing.T) {
	if cols := resourceParentKeyColumns["feed"]; len(cols) != 0 {
		t.Fatalf("feed gained parent key columns %v; its id is global, see #264", cols)
	}
	same := `{"id":5600,"datetime":"2026-04-16T16:13:52.903","entitytype":1,"content_id1":47}`
	keys, _ := storedKeys(t, "feed", []string{same, same,
		`{"id":5601,"datetime":"2026-04-16T16:14:52.903","entitytype":1,"content_id1":48}`})
	if len(keys) != 2 {
		t.Fatalf("feed stored %d keys for 2 distinct ids; re-fetched rows must collapse", len(keys))
	}
}

// The schema-7 purge must clear every rekeyed resource from both the resources
// mirror and the hand-maintained FTS index, and leave everything else alone.
func TestRound2PurgeClearsRekeyedRows(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "data.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	seed := []struct{ resource, id string }{
		{"team-tree", "16"}, {"timesheet-forecasting", "0"},
		{"integration-runbook-variable-group", "x"}, {"tickets", "900"},
	}
	for _, r := range seed {
		if _, err := s.DB().Exec(`INSERT INTO resources (id, resource_type, data) VALUES (?, ?, ?)`,
			r.id, r.resource, `{"id":"`+r.id+`"}`); err != nil {
			t.Fatalf("seeding %s: %v", r.resource, err)
		}
		if _, err := s.DB().Exec(`INSERT INTO resources_fts (id, resource_type, content) VALUES (?, ?, ?)`,
			r.id, r.resource, "legacy"); err != nil {
			t.Fatalf("seeding fts for %s: %v", r.resource, err)
		}
	}
	if _, err := s.DB().Exec(`PRAGMA user_version = 6`); err != nil {
		t.Fatalf("stamping the previous schema version: %v", err)
	}
	s.Close()

	s2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("reopen (the upgrade path): %v", err)
	}
	defer s2.Close()

	for _, table := range []string{"resources", "resources_fts"} {
		for _, resource := range []string{"team-tree", "timesheet-forecasting", "integration-runbook-variable-group"} {
			var n int
			if err := s2.DB().QueryRow(`SELECT count(*) FROM `+table+` WHERE resource_type = ?`, resource).Scan(&n); err != nil {
				t.Fatalf("counting %s.%s: %v", table, resource, err)
			}
			if n != 0 {
				t.Errorf("%s: %d legacy %s row(s) survived the upgrade", table, n, resource)
			}
		}
		var kept int
		if err := s2.DB().QueryRow(`SELECT count(*) FROM ` + table + ` WHERE resource_type = 'tickets'`).Scan(&kept); err != nil {
			t.Fatalf("counting %s.tickets: %v", table, err)
		}
		if kept != 1 {
			t.Errorf("%s: purge removed an unrelated resource's rows (tickets = %d, want 1)", table, kept)
		}
	}
	if v, err := s2.SchemaVersion(); err != nil || v != StoreSchemaVersion {
		t.Errorf("schema version after upgrade = %d (err %v), want %d", v, err, StoreSchemaVersion)
	}
}

// The timesheet resources are keyed by (agent_id, date) because their records
// carry no usable id. Halo declares `date` as a DATETIME and returns a
// meaningless time of day on it; if that component is recomputed per request,
// the raw string would change every sync while the row does not, inserting a
// fresh copy each time instead of updating. The key must depend only on the
// day (#264).
func TestTimesheetKeyIsStableAcrossTimestampDrift(t *testing.T) {
	for _, resource := range []string{"timesheet", "timesheet-forecasting"} {
		morning := resourceStorageID(resource, "0",
			decode(t, `{"id":0,"agent_id":1,"date":"2026-10-25T09:13:41.4892555+00:00"}`))
		evening := resourceStorageID(resource, "0",
			decode(t, `{"id":0,"agent_id":1,"date":"2026-10-25T19:47:02.1230000+00:00"}`))
		if morning != evening {
			t.Errorf("%s: the same agent-day produced two keys (%q vs %q); every sync would insert a copy",
				resource, morning, evening)
		}
		next := resourceStorageID(resource, "0",
			decode(t, `{"id":0,"agent_id":1,"date":"2026-10-26T09:13:41.4892555+00:00"}`))
		if next == morning {
			t.Errorf("%s: two different days collapsed onto one key %q", resource, morning)
		}
		// A date-only value must key identically to the same day sent as a
		// timestamp, so a tenant serializing either way lands on one row.
		dateOnly := resourceStorageID(resource, "0", decode(t, `{"id":0,"agent_id":1,"date":"2026-10-25"}`))
		if dateOnly != morning {
			t.Errorf("%s: date-only %q and timestamp %q disagree", resource, dateOnly, morning)
		}
	}
}

// Only the `date` column is normalised. Every other parent column passes
// through byte-for-byte, so a future column carrying an opaque id that merely
// starts with a date-like prefix cannot be truncated into a collision.
func TestParentKeyValueNormalisesOnlyTheDateColumn(t *testing.T) {
	for _, v := range []string{"42", "0", "runbookc9f1a2b3-45de-6f78-9a01-bcdef2345678", "", "2026", "a3b34d24-e2c5"} {
		if got := parentKeyValue("date", v); got != v {
			t.Errorf("parentKeyValue(date, %q) = %q, want it unchanged", v, got)
		}
	}
	if got := parentKeyValue("date", "2026-10-25T19:13:41Z"); got != "2026-10-25" {
		t.Errorf("date truncation = %q, want %q", got, "2026-10-25")
	}
	if got := parentKeyValue("date", "2026-10-25 19:13:41"); got != "2026-10-25" {
		t.Errorf("space-separated date = %q, want %q", got, "2026-10-25")
	}
	// The same timestamp-shaped value under any other column is untouched.
	for _, col := range []string{"ticket_id", "agent_id", "lookupid", "fdid", "typeinfo_id", "invoice_id"} {
		if got := parentKeyValue(col, "2026-10-25T19:13:41Z"); got != "2026-10-25T19:13:41Z" {
			t.Errorf("parentKeyValue(%s, timestamp) = %q, want it unchanged", col, got)
		}
	}
}
