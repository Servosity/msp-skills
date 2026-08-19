package store

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func decode(t *testing.T, row string) map[string]any {
	t.Helper()
	obj, err := DecodeJSONObject(json.RawMessage(row))
	if err != nil {
		t.Fatalf("decoding %s: %v", row, err)
	}
	return obj
}

// storedKeys mirrors what UpsertBatch does per item: resolve the id, then
// derive the storage key. Distinct keys are the rows that survive the upsert.
func storedKeys(t *testing.T, resource string, rows []string) (keys map[string]bool, extractFailures int) {
	t.Helper()
	keys = map[string]bool{}
	for _, row := range rows {
		obj := decode(t, row)
		id := ExtractResourceID(resource, obj)
		if id == "" {
			extractFailures++
			continue
		}
		keys[resourceStorageID(resource, id, obj)] = true
	}
	return keys, extractFailures
}

// HaloPSA numbers these resources' rows within a parent, so the bare id
// collides across parents and the last writer wins. Payload shapes are the
// spec's own: Lookup{lookupid,id}, TypeInfo{typeinfo_id,id,field_id} (#264).
func TestParentScopedResourcesKeepEveryRow(t *testing.T) {
	cases := []struct {
		resource string
		rows     []string
	}{
		{"lookup", []string{
			`{"lookupid":153,"id":1,"name":"Manual"}`,
			`{"lookupid":153,"id":2,"name":"Dynamic"}`,
			`{"lookupid":149,"id":1,"name":"firstnamelastname"}`,
			`{"lookupid":149,"id":2,"name":"firstname.lastname"}`,
		}},
		{"asset-type-info", []string{
			`{"typeinfo_id":120,"id":4,"field_id":132,"field_name":"IMEI Number"}`,
			`{"typeinfo_id":120,"id":5,"field_id":131,"field_name":"Telephone Number"}`,
			`{"typeinfo_id":121,"id":4,"field_id":90,"field_name":"Serial"}`,
		}},
	}
	for _, c := range cases {
		keys, failures := storedKeys(t, c.resource, c.rows)
		if failures != 0 {
			t.Errorf("%s: %d row(s) failed id extraction", c.resource, failures)
		}
		if len(keys) != len(c.rows) {
			t.Errorf("%s: %d rows collapsed into %d stored keys", c.resource, len(c.rows), len(keys))
		}
	}
}

// workflowstep needs BOTH halves of its fix. It carries no `id`, so without
// the step_id override the generic fallback reaches `name` and two steps
// sharing a name inside one flow still collide even once fdid qualifies the
// key. This is the case the reporter's suggested fdid-only fix would miss.
func TestWorkflowStepKeyedByStepIDWithinFlow(t *testing.T) {
	rows := []string{
		`{"fdid":1324,"step_id":1,"flow_id":0,"name":"Get Sentiment"}`,
		`{"fdid":1363,"step_id":1,"flow_id":0,"name":"Input"}`,
		`{"fdid":1363,"step_id":2,"flow_id":0,"name":"Input"}`,
	}
	keys, failures := storedKeys(t, "workflowstep", rows)
	if failures != 0 {
		t.Fatalf("%d workflowstep row(s) failed id extraction", failures)
	}
	if len(keys) != 3 {
		t.Fatalf("3 workflow steps collapsed into %d stored keys", len(keys))
	}
	obj := decode(t, rows[1])
	if got := ExtractResourceID("workflowstep", obj); got != "1" {
		t.Errorf("workflowstep id resolved to %q, want the step_id \"1\" (a fall-through to name would give %q)", got, obj["name"])
	}
}

// /Timesheet returns "id": 0 on every record, so there is no per-record id to
// scope. The real key is the agent and the day.
func TestTimesheetKeyedByAgentAndDate(t *testing.T) {
	rows := []string{
		`{"id":0,"agent_id":7,"date":"2026-08-01","actual_hours":7.5}`,
		`{"id":0,"agent_id":7,"date":"2026-08-02","actual_hours":8}`,
		`{"id":0,"agent_id":9,"date":"2026-08-01","actual_hours":6}`,
	}
	keys, failures := storedKeys(t, "timesheet", rows)
	if failures != 0 {
		t.Fatalf("%d timesheet row(s) failed id extraction", failures)
	}
	if len(keys) != 3 {
		t.Fatalf("3 agent-days collapsed into %d stored keys", len(keys))
	}
}

// online-status names its identifier techID, which no spelling of "id"
// reaches, so every record failed extraction and the resource stored nothing.
func TestOnlineStatusExtractsTechID(t *testing.T) {
	rows := []string{`{"techID":12,"isOnline":true}`, `{"techID":31,"isOnline":false}`}
	keys, failures := storedKeys(t, "online-status", rows)
	if failures != 0 {
		t.Fatalf("%d online-status row(s) still fail id extraction", failures)
	}
	if len(keys) != 2 {
		t.Fatalf("2 agents collapsed into %d stored keys", len(keys))
	}
}

// A single-column parent key must stay byte-identical to the pre-#264 form,
// and a row missing its parent column must stay addressable by its bare id.
// Otherwise the composite change silently orphans rows written by 0.2.7.
func TestSingleParentKeyFormIsUnchanged(t *testing.T) {
	obj := decode(t, `{"id":7,"ticket_id":42}`)
	want := "7" + string([]byte{0}) + "42"
	if got := resourceStorageID("actions", "7", obj); got != want {
		t.Errorf("actions key = %q, want the unchanged %q", got, want)
	}
	if got := resourceStorageID("actions", "7", decode(t, `{"id":7}`)); got != "7" {
		t.Errorf("actions row without ticket_id = %q, want the bare %q", got, "7")
	}
	if got := resourceStorageID("tickets", "7", obj); got != "7" {
		t.Errorf("unqualified resource key = %q, want the bare %q", got, "7")
	}
}

// A timesheet row missing one half of its key must not pad the gap, or it
// would land under a different key than the same row synced complete.
func TestCompositeKeySkipsAbsentColumns(t *testing.T) {
	got := resourceStorageID("timesheet", "0", decode(t, `{"id":0,"agent_id":7}`))
	want := "0" + string([]byte{0}) + "7"
	if got != want {
		t.Errorf("partial timesheet key = %q, want %q", got, want)
	}
	if strings.Count(got, string([]byte{0})) != 1 {
		t.Errorf("absent column was padded into the key: %q", got)
	}
}

// The purge has to actually run on a database stamped by the previous
// release, clear every table it names, and leave the store openable. The FTS
// index is included because it is maintained by hand rather than by delete
// triggers, so a corrected key would otherwise strand its legacy entry.
func TestPurgeClearsLegacyRowsAndFTSOnUpgrade(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "data.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	// Rows as v0.2.7 would have written them: bare, colliding ids.
	for _, r := range []struct{ resource, id string }{
		{"lookup", "1"}, {"asset-type-info", "4"}, {"workflowstep", "Input"},
		{"timesheet", "0"}, {"tickets", "900"},
	} {
		if _, err := s.DB().Exec(
			`INSERT INTO resources (id, resource_type, data) VALUES (?, ?, ?)`,
			r.id, r.resource, `{"id":"`+r.id+`"}`); err != nil {
			t.Fatalf("seeding %s: %v", r.resource, err)
		}
		if _, err := s.DB().Exec(
			`INSERT INTO resources_fts (id, resource_type, content) VALUES (?, ?, ?)`,
			r.id, r.resource, "legacy"); err != nil {
			t.Fatalf("seeding fts for %s: %v", r.resource, err)
		}
	}
	if _, err := s.DB().Exec(`PRAGMA user_version = 5`); err != nil {
		t.Fatalf("stamping the previous schema version: %v", err)
	}
	s.Close()

	s2, err := Open(dbPath)
	if err != nil {
		t.Fatalf("reopen (the upgrade path): %v", err)
	}
	defer s2.Close()

	for _, table := range []string{"resources", "resources_fts"} {
		for _, resource := range []string{"lookup", "asset-type-info", "workflowstep", "timesheet"} {
			var n int
			if err := s2.DB().QueryRow(
				`SELECT count(*) FROM `+table+` WHERE resource_type = ?`, resource).Scan(&n); err != nil {
				t.Fatalf("counting %s.%s: %v", table, resource, err)
			}
			if n != 0 {
				t.Errorf("%s: %d legacy %s row(s) survived the upgrade", table, n, resource)
			}
		}
		// A resource whose key did not change must be left alone.
		var kept int
		if err := s2.DB().QueryRow(
			`SELECT count(*) FROM ` + table + ` WHERE resource_type = 'tickets'`).Scan(&kept); err != nil {
			t.Fatalf("counting %s.tickets: %v", table, err)
		}
		if kept != 1 {
			t.Errorf("%s: purge removed an unrelated resource's rows (tickets count = %d, want 1)", table, kept)
		}
	}

	if v, err := s2.SchemaVersion(); err != nil || v != StoreSchemaVersion {
		t.Errorf("schema version after upgrade = %d (err %v), want %d", v, err, StoreSchemaVersion)
	}
}
