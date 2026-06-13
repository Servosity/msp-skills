// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package fleet

import (
	"encoding/json"
	"testing"
	"time"
)

// obj builds a map with json.Number-preserving decode, mirroring how the store
// hands objects to the fleet functions (store.DecodeJSONObject uses UseNumber).
func obj(t *testing.T, jsonStr string) map[string]any {
	t.Helper()
	var m map[string]any
	dec := json.NewDecoder(stringsReader(jsonStr))
	dec.UseNumber()
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("decode %q: %v", jsonStr, err)
	}
	return m
}

func stringsReader(s string) *stringReader { return &stringReader{s: s} }

type stringReader struct {
	s string
	i int
}

func (r *stringReader) Read(p []byte) (int, error) {
	if r.i >= len(r.s) {
		return 0, ioEOF
	}
	n := copy(p, r.s[r.i:])
	r.i += n
	return n, nil
}

var ioEOF = errEOF{}

type errEOF struct{}

func (errEOF) Error() string { return "EOF" }

var now = time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

func TestParseTime(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"2024-01-15T10:30:00Z", true},
		{"2024-01-15T10:30:00", true},
		{"2024-01-15 10:30:00", true},
		{"2024-01-15", true},
		{"/Date(1705314600000)/", true},
		{"", false},
		{"not a date", false},
	}
	for _, c := range cases {
		_, ok := ParseTime(c.in)
		if ok != c.want {
			t.Errorf("ParseTime(%q) ok=%v, want %v", c.in, ok, c.want)
		}
	}
}

func TestIntHandlesJSONNumberAndString(t *testing.T) {
	// json.Number (the store's decode shape)
	if got := Int(obj(t, `{"Priority": 4}`), "Priority"); got != 4 {
		t.Errorf("Int(json number) = %d, want 4", got)
	}
	// numeric string (LookupFieldValue's stringified shape)
	if got := Int(map[string]any{"Priority": "5"}, "Priority"); got != 5 {
		t.Errorf("Int(string) = %d, want 5", got)
	}
	// absent reads as zero
	if got := Int(map[string]any{}, "Priority"); got != 0 {
		t.Errorf("Int(absent) = %d, want 0", got)
	}
}

func TestComputerClientFallbacks(t *testing.T) {
	clients := []map[string]any{obj(t, `{"Id": 7, "Name": "Acme Corp"}`)}
	idx := clientNameIndex(clients)
	// flat ClientName wins
	if got := computerClient(obj(t, `{"ClientName":"Flat Inc"}`), idx); got != "Flat Inc" {
		t.Errorf("flat ClientName = %q", got)
	}
	// nested Client object
	if got := computerClient(obj(t, `{"Client":{"Id":1,"Name":"Nested LLC"}}`), idx); got != "Nested LLC" {
		t.Errorf("nested Client = %q", got)
	}
	// join via ClientId
	if got := computerClient(obj(t, `{"ClientId":7}`), idx); got != "Acme Corp" {
		t.Errorf("join via ClientId = %q", got)
	}
}

func TestFleetHealth(t *testing.T) {
	clients := []map[string]any{
		obj(t, `{"Id":1,"Name":"Acme"}`),
		obj(t, `{"Id":2,"Name":"Globex"}`),
	}
	computers := []map[string]any{
		obj(t, `{"Id":10,"ClientId":1,"Status":"Online","LastContact":"2026-05-30T11:00:00Z"}`),
		obj(t, `{"Id":11,"ClientId":1,"Status":"Offline","LastContact":"2026-01-01T00:00:00Z"}`),
		obj(t, `{"Id":12,"ClientId":2,"Status":"Online","LastContact":"2026-05-29T11:00:00Z"}`),
	}
	alerts := []map[string]any{
		obj(t, `{"Id":100,"ComputerId":11,"Priority":3}`),
	}
	got := FleetHealth(computers, clients, alerts, 7, now)
	if len(got) != 2 {
		t.Fatalf("want 2 client rows, got %d", len(got))
	}
	// Acme has the offline agent, so it sorts first.
	if got[0].Client != "Acme" {
		t.Fatalf("want Acme first (most offline), got %q", got[0].Client)
	}
	if got[0].Computers != 2 || got[0].Online != 1 || got[0].Offline != 1 {
		t.Errorf("Acme health = %+v", got[0])
	}
	if got[0].Stale != 1 {
		t.Errorf("Acme stale = %d, want 1 (the Jan agent)", got[0].Stale)
	}
	if got[0].OpenAlerts != 1 {
		t.Errorf("Acme open alerts = %d, want 1", got[0].OpenAlerts)
	}
}

func TestStaleAgents(t *testing.T) {
	clients := []map[string]any{obj(t, `{"Id":1,"Name":"Acme"}`)}
	computers := []map[string]any{
		obj(t, `{"ComputerName":"PC-FRESH","ClientId":1,"LastContact":"2026-05-30T11:00:00Z"}`),
		obj(t, `{"ComputerName":"PC-OLD","ClientId":1,"LastContact":"2026-03-01T00:00:00Z"}`),
		obj(t, `{"ComputerName":"PC-NEVER","ClientId":1}`),
	}
	got := StaleAgents(computers, clients, 30, now)
	// PC-FRESH excluded; PC-OLD and PC-NEVER included.
	if len(got) != 2 {
		t.Fatalf("want 2 stale, got %d: %+v", len(got), got)
	}
	// Most-stale-first: PC-OLD (~90 days) before PC-NEVER (-1 sentinel).
	if got[0].Computer != "PC-OLD" {
		t.Errorf("want PC-OLD first, got %q", got[0].Computer)
	}
	if got[0].Client != "Acme" {
		t.Errorf("PC-OLD client = %q, want Acme", got[0].Client)
	}
	if got[0].DaysStale < 80 {
		t.Errorf("PC-OLD days stale = %d, want ~90", got[0].DaysStale)
	}
}

func TestStaleAgentsEmptyStoreReturnsNil(t *testing.T) {
	got := StaleAgents(nil, nil, 30, now)
	if len(got) != 0 {
		t.Errorf("empty store should yield no stale agents, got %d", len(got))
	}
}

func TestPatchCompliance(t *testing.T) {
	clients := []map[string]any{obj(t, `{"Id":1,"Name":"Acme"}`), obj(t, `{"Id":2,"Name":"Globex"}`)}
	computers := []map[string]any{
		obj(t, `{"Id":10,"ClientId":1}`),
		obj(t, `{"Id":20,"ClientId":2}`),
	}
	patches := []map[string]any{
		obj(t, `{"ComputerId":10,"Status":"Installed"}`),
		obj(t, `{"ComputerId":10,"Status":"Installed"}`),
		obj(t, `{"ComputerId":10,"Status":"Failed"}`),
		obj(t, `{"ComputerId":20,"Status":"Installed"}`),
	}
	got := PatchCompliance(patches, computers, clients)
	if len(got) != 2 {
		t.Fatalf("want 2 clients, got %d", len(got))
	}
	// Worst compliance first: Acme is 2/3 = 66.7%, Globex 1/1 = 100%.
	if got[0].Client != "Acme" {
		t.Fatalf("want Acme first (worst), got %q", got[0].Client)
	}
	if got[0].Installed != 2 || got[0].Failed != 1 || got[0].Total != 3 {
		t.Errorf("Acme patch = %+v", got[0])
	}
	if got[0].CompliancePct != 66.7 {
		t.Errorf("Acme compliance = %v, want 66.7", got[0].CompliancePct)
	}
}

func TestClientRollupsFilterAndZeroComputerClient(t *testing.T) {
	clients := []map[string]any{
		obj(t, `{"Id":1,"Name":"Acme"}`),
		obj(t, `{"Id":2,"Name":"Globex"}`),
	}
	computers := []map[string]any{obj(t, `{"Id":10,"ClientId":1,"Status":"Offline"}`)}
	locations := []map[string]any{obj(t, `{"Id":5,"ClientId":1}`)}
	alerts := []map[string]any{obj(t, `{"Id":1,"ComputerId":10,"Priority":2}`)}

	all := ClientRollups(computers, clients, locations, alerts, "")
	if len(all) != 2 {
		t.Fatalf("want 2 clients (incl. zero-computer Globex), got %d", len(all))
	}

	filtered := ClientRollups(computers, clients, locations, alerts, "acme")
	if len(filtered) != 1 || filtered[0].Client != "Acme" {
		t.Fatalf("filter 'acme' = %+v", filtered)
	}
	a := filtered[0]
	if a.Computers != 1 || a.Locations != 1 || a.Offline != 1 || a.OpenAlerts != 1 {
		t.Errorf("Acme rollup = %+v", a)
	}
}

func TestAlertTriageFilterDedupSort(t *testing.T) {
	clients := []map[string]any{obj(t, `{"Id":1,"Name":"Acme"}`)}
	computers := []map[string]any{obj(t, `{"Id":10,"ClientId":1,"LocationName":"HQ"}`)}
	alerts := []map[string]any{
		obj(t, `{"Id":1,"ComputerId":10,"Priority":1,"Message":"low"}`),
		obj(t, `{"Id":2,"ComputerId":10,"Priority":5,"Message":"disk full"}`),
		obj(t, `{"Id":3,"ComputerId":10,"Priority":3,"Message":"disk full"}`), // dup message, lower prio
	}
	got := AlertTriage(alerts, computers, clients, 3)
	// priority 1 filtered out; the two "disk full" collapse to the priority-5 one.
	if len(got) != 1 {
		t.Fatalf("want 1 triaged alert after filter+dedup, got %d: %+v", len(got), got)
	}
	if got[0].Priority != 5 || got[0].Message != "disk full" {
		t.Errorf("kept alert = %+v, want the priority-5 disk full", got[0])
	}
	if got[0].Client != "Acme" || got[0].Location != "HQ" {
		t.Errorf("context join failed: %+v", got[0])
	}
}

func TestOSInventoryEOL(t *testing.T) {
	computers := []map[string]any{
		obj(t, `{"OperatingSystemName":"Windows 11 Pro"}`),
		obj(t, `{"OperatingSystemName":"Windows 11 Pro"}`),
		obj(t, `{"OperatingSystemName":"Windows Server 2012 R2"}`),
		obj(t, `{"OperatingSystemName":"Windows 7 Professional"}`),
	}
	all := OSInventory(computers, false)
	if len(all) != 3 {
		t.Fatalf("want 3 OS groups, got %d", len(all))
	}
	if all[0].OS != "Windows 11 Pro" || all[0].Count != 2 || all[0].EOL {
		t.Errorf("top group = %+v, want Win11 x2 non-EOL", all[0])
	}
	eol := OSInventory(computers, true)
	if len(eol) != 2 {
		t.Fatalf("want 2 EOL groups (Server 2012, Win7), got %d: %+v", len(eol), eol)
	}
	for _, g := range eol {
		if !g.EOL {
			t.Errorf("eol-only returned non-EOL group %+v", g)
		}
	}
}

func TestSinceWindow(t *testing.T) {
	clients := []map[string]any{obj(t, `{"Id":1,"Name":"Acme"}`)}
	computers := []map[string]any{
		obj(t, `{"Id":10,"ClientId":1,"LastContact":"2026-05-30T11:00:00Z"}`), // within 24h
		obj(t, `{"Id":11,"ClientId":1,"LastContact":"2026-05-01T00:00:00Z"}`), // older
	}
	alerts := []map[string]any{
		obj(t, `{"Id":1,"ComputerId":10,"Priority":4,"Message":"new","CreatedDate":"2026-05-30T10:00:00Z"}`), // within
		obj(t, `{"Id":2,"ComputerId":10,"Priority":2,"Message":"old","CreatedDate":"2026-05-20T10:00:00Z"}`), // outside
	}
	patches := []map[string]any{
		obj(t, `{"ComputerId":10,"Status":"Installed","InstallDate":"2026-05-30T09:00:00Z"}`), // within
		obj(t, `{"ComputerId":10,"Status":"Installed","InstallDate":"2026-05-10T09:00:00Z"}`), // outside
	}
	rep := Since(alerts, computers, patches, clients, 24, now)
	if rep.AgentsCheckedIn != 1 {
		t.Errorf("agents checked in = %d, want 1", rep.AgentsCheckedIn)
	}
	if len(rep.NewAlerts) != 1 || rep.NewAlerts[0].AlertID != "1" {
		t.Errorf("new alerts = %+v, want only alert 1", rep.NewAlerts)
	}
	if rep.NewAlerts[0].Client != "Acme" {
		t.Errorf("new alert client = %q, want Acme", rep.NewAlerts[0].Client)
	}
	if rep.PatchesInstalled != 1 {
		t.Errorf("patches installed = %d, want 1", rep.PatchesInstalled)
	}
}
