// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored behavioral tests for the axcient fleet novel commands.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"axcient-pp-cli/internal/store"
	"github.com/spf13/cobra"
)

// newFleetFixtureStore seeds a temp store with 2 clients, 4 devices,
// 2 appliances, and 2 autoverify rows covering the pass/fail matrix the
// fleet commands discriminate on.
func newFleetFixtureStore(t *testing.T) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	now := time.Now().UTC()
	ts := func(age time.Duration) string { return now.Add(-age).Format(time.RFC3339) }

	clients := []string{
		`{"id":1,"name":"Acme Corp"}`,
		`{"id":2,"name":"Globex"}`,
	}
	for i, c := range clients {
		if err := db.Upsert("clients", fmt.Sprintf("%d", i+1), json.RawMessage(c)); err != nil {
			t.Fatalf("seed client: %v", err)
		}
	}

	devices := []string{
		// healthy, fresh, appliance-based
		`{"id":100,"name":"acme-dc01","client_id":1,"type":"SERVER","d2c":false,"service_id":"abcd",
		  "current_health_status":{"status":"NORMAL","reason":"","timestamp":"` + ts(time.Hour) + `"},
		  "latest_local_rp":"` + ts(2*time.Hour) + `","latest_cloud_rp":"` + ts(time.Hour) + `",
		  "local_usage":1000,"cloud_usage":2000,"vault_usage":0}`,
		// failing health, fresh RP, D2C
		`{"id":101,"name":"acme-ws07","client_id":1,"type":"WORKSTATION","d2c":true,
		  "current_health_status":{"status":"TROUBLESHOOTING","reason":"JOB_FAILED","timestamp":"` + ts(30*time.Minute) + `"},
		  "latest_cloud_rp":"` + ts(3*time.Hour) + `",
		  "local_usage":0,"cloud_usage":500,"vault_usage":0}`,
		// healthy status but stale RP (50h), appliance-based
		`{"id":102,"name":"globex-sql01","client_id":2,"type":"SERVER","d2c":false,"service_id":"wxyz",
		  "current_health_status":{"status":"NORMAL","reason":"","timestamp":"` + ts(time.Hour) + `"},
		  "latest_local_rp":"` + ts(50*time.Hour) + `",
		  "local_usage":3000,"cloud_usage":4000,"vault_usage":100}`,
		// no restore points at all
		`{"id":103,"name":"globex-nas","client_id":2,"type":"NAS","d2c":false,
		  "local_usage":0,"cloud_usage":0,"vault_usage":0}`,
		// BOTH failing health AND stale RP (100h) — the double-flag case
		`{"id":104,"name":"globex-app01","client_id":2,"type":"SERVER","d2c":false,
		  "current_health_status":{"status":"TROUBLESHOOTING","reason":"AGENT_OFFLINE","timestamp":"` + ts(2*time.Hour) + `"},
		  "latest_local_rp":"` + ts(100*time.Hour) + `",
		  "local_usage":500,"cloud_usage":0,"vault_usage":0}`,
	}
	for _, d := range devices {
		var obj struct {
			ID json.Number `json:"id"`
		}
		if err := json.Unmarshal([]byte(d), &obj); err != nil {
			t.Fatalf("fixture decode: %v", err)
		}
		if err := db.Upsert("device", obj.ID.String(), json.RawMessage(d)); err != nil {
			t.Fatalf("seed device: %v", err)
		}
	}

	appliances := []string{
		`{"id":500,"service_id":"abcd","client_id":1,"alias":"Acme-BDR","ip_address":"10.0.0.5","active":true}`,
		`{"id":501,"service_id":"wxyz","client_id":2,"alias":"Globex-BDR","ip_address":"10.0.1.5","active":true}`,
	}
	for i, a := range appliances {
		if err := db.Upsert("appliance", fmt.Sprintf("%d", 500+i), json.RawMessage(a)); err != nil {
			t.Fatalf("seed appliance: %v", err)
		}
	}

	autoverify := []json.RawMessage{
		// device 100: passing, healthy boot proof
		json.RawMessage(`{"id":"av:100:10:500","device_id":"100","vault_id":10,"appliance_id":500,
		  "autoverify_details":[{"id":"av-1","timestamp":"` + ts(4*time.Hour) + `","end_timestamp":"` + ts(4*time.Hour) + `",
		  "rp":"` + ts(5*time.Hour) + `","status":"success","is_healthy":true,"screenshot_url":"https://example.test/shot/100"}]}`),
		// device 102: completed but unhealthy boot
		json.RawMessage(`{"id":"av:102:11:501","device_id":"102","vault_id":11,"appliance_id":501,
		  "autoverify_details":[{"id":"av-2","timestamp":"` + ts(6*time.Hour) + `","end_timestamp":"` + ts(6*time.Hour) + `",
		  "rp":"` + ts(7*time.Hour) + `","status":"success","is_healthy":false,"screenshot_url":"https://example.test/shot/102"}]}`),
		// device 101: success with is_healthy ABSENT (tri-state nil) — must count as passing
		json.RawMessage(`{"id":"av:101:12:0","device_id":"101","vault_id":12,
		  "autoverify_details":[{"id":"av-3","timestamp":"` + ts(5*time.Hour) + `","end_timestamp":"` + ts(5*time.Hour) + `",
		  "rp":"` + ts(6*time.Hour) + `","status":"success"}]}`),
	}
	if stored, failures, err := db.UpsertBatch("autoverify", autoverify); err != nil || stored != 3 || failures != 0 {
		t.Fatalf("seed autoverify: stored=%d failures=%d err=%v", stored, failures, err)
	}
	return dbPath
}

// runFleetCmd executes a novel command constructor against the fixture store
// and returns its stdout.
func runFleetCmd(t *testing.T, ctor func(*rootFlags) *cobra.Command, args ...string) string {
	t.Helper()
	flags := &rootFlags{asJSON: true}
	cmd := ctor(flags)
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute %v: %v (stderr: %s)", args, err, errBuf.String())
	}
	return out.String()
}

func TestParseAxTime(t *testing.T) {
	cases := []struct {
		in string
		ok bool
	}{
		{"2023-01-20T04:56:07Z", true},
		{"2021-07-01T10:08:00", true},
		{"2024-01-3T11:33:00", true},
		{"2024-1-3T11:33:00Z", true},
		{"", false},
		{"not-a-time", false},
	}
	for _, c := range cases {
		if _, ok := parseAxTime(c.in); ok != c.ok {
			t.Errorf("parseAxTime(%q) ok = %v, want %v", c.in, ok, c.ok)
		}
	}
}

func TestNewestRestorePoint(t *testing.T) {
	cases := []struct {
		name   string
		dev    fleetDevice
		target string
		ok     bool
	}{
		{"cloud newest", fleetDevice{LatestLocal: "2023-01-01T00:00:00Z", LatestCloud: "2023-06-01T00:00:00Z"}, "cloud", true},
		{"local only", fleetDevice{LatestLocal: "2023-01-01T00:00:00Z"}, "local", true},
		{"none", fleetDevice{}, "", false},
	}
	for _, c := range cases {
		_, target, ok := c.dev.newestRestorePoint()
		if ok != c.ok || target != c.target {
			t.Errorf("%s: newestRestorePoint() = (%q, %v), want (%q, %v)", c.name, target, ok, c.target, c.ok)
		}
	}
}

func TestHealthCommand(t *testing.T) {
	dbPath := newFleetFixtureStore(t)
	out := runFleetCmd(t, newNovelHealthCmd, "--hours", "24", "--db", dbPath)
	var view healthView
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("health output not JSON: %v\n%s", err, out)
	}
	if view.TotalDevices != 5 {
		t.Errorf("total_devices = %d, want 5", view.TotalDevices)
	}
	if view.FailingDevices != 2 {
		t.Errorf("failing_devices = %d, want 2 (acme-ws07, globex-app01)", view.FailingDevices)
	}
	if view.StaleDevices != 3 {
		t.Errorf("stale_devices = %d, want 3 (globex-sql01 50h, globex-nas never, globex-app01 100h)", view.StaleDevices)
	}
	// double-flag device appears once with both markers, not twice
	seen104 := 0
	for _, c := range view.Clients {
		for _, d := range c.Devices {
			if d.DeviceID == "104" {
				seen104++
				if !d.Failing || !d.Stale {
					t.Errorf("device 104 should be both failing and stale: %+v", d)
				}
			}
		}
	}
	if seen104 != 1 {
		t.Errorf("device 104 appeared %d times, want exactly 1", seen104)
	}
	if len(view.Clients) != 2 {
		t.Fatalf("clients = %d, want 2", len(view.Clients))
	}
	if view.Clients[0].ClientName != "Acme Corp" || view.Clients[1].ClientName != "Globex" {
		t.Errorf("client names = %q, %q — clients join broken", view.Clients[0].ClientName, view.Clients[1].ClientName)
	}
	// absence-of-correctness: healthy fresh device must NOT appear
	for _, c := range view.Clients {
		for _, d := range c.Devices {
			if d.DeviceID == "100" {
				t.Errorf("healthy device 100 reported in health sweep")
			}
		}
	}
}

func TestHealthCommandClientFilter(t *testing.T) {
	dbPath := newFleetFixtureStore(t)
	out := runFleetCmd(t, newNovelHealthCmd, "--hours", "24", "--client", "2", "--db", dbPath)
	var view healthView
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if view.TotalDevices != 3 || len(view.Clients) != 1 || view.Clients[0].ClientID != "2" {
		t.Errorf("client filter broken: total=%d clients=%d", view.TotalDevices, len(view.Clients))
	}
}

func TestRpoCommand(t *testing.T) {
	dbPath := newFleetFixtureStore(t)
	out := runFleetCmd(t, newNovelRpoCmd, "--hours", "24", "--db", dbPath)
	var view rpoView
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("rpo output not JSON: %v\n%s", err, out)
	}
	if len(view.Breaches) != 3 {
		t.Fatalf("breaches = %d, want 3 (102, 103, 104)", len(view.Breaches))
	}
	ids := map[string]rpoBreachView{}
	for _, b := range view.Breaches {
		ids[b.DeviceID] = b
	}
	if b, ok := ids["102"]; !ok || b.NeverRun || b.HoursSinceRP < 49 {
		t.Errorf("device 102 breach wrong: %+v", b)
	}
	if b, ok := ids["103"]; !ok || !b.NeverRun {
		t.Errorf("device 103 should be never_run: %+v", b)
	}
}

func TestRpoCommandTargetCloud(t *testing.T) {
	dbPath := newFleetFixtureStore(t)
	out := runFleetCmd(t, newNovelRpoCmd, "--hours", "24", "--target", "cloud", "--db", dbPath)
	var view rpoView
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// 102/104 have no cloud RP (local only) and 103 has nothing -> all three breach on cloud tier
	if len(view.Breaches) != 3 {
		t.Errorf("cloud-target breaches = %d, want 3", len(view.Breaches))
	}
	for _, b := range view.Breaches {
		if b.DeviceID == "100" || b.DeviceID == "101" {
			t.Errorf("device %s has fresh cloud RP but was flagged", b.DeviceID)
		}
	}
}

func TestRpoCommandRejectsBadTarget(t *testing.T) {
	dbPath := newFleetFixtureStore(t)
	flags := &rootFlags{asJSON: true}
	cmd := newNovelRpoCmd(flags)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--target", "offsite", "--db", dbPath})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected usage error for --target offsite")
	}
}

func TestComplianceCommand(t *testing.T) {
	dbPath := newFleetFixtureStore(t)
	out := runFleetCmd(t, newNovelComplianceCmd, "--hours", "24", "--db", dbPath)
	var rows []complianceRow
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("compliance output not a JSON array: %v\n%s", err, out)
	}
	if len(rows) != 5 {
		t.Fatalf("rows = %d, want 5", len(rows))
	}
	byID := map[string]complianceRow{}
	for _, r := range rows {
		byID[r.DeviceID] = r
	}
	if r := byID["100"]; !r.Compliant || !r.RPOPass || !r.AutoverifyPass || r.ScreenshotURL == "" {
		t.Errorf("device 100 should be fully compliant with screenshot: %+v", r)
	}
	if r := byID["101"]; !r.Compliant || r.AutoverifyStatus != "success" || r.AutoverifyHealthy != nil || !r.AutoverifyPass {
		t.Errorf("device 101 (success + nil is_healthy tri-state) should pass autoverify and be compliant: %+v", r)
	}
	if r := byID["104"]; r.Compliant || r.RPOPass || r.AutoverifyStatus != "never_run" {
		t.Errorf("device 104 should fail RPO and have never_run autoverify: %+v", r)
	}
	if r := byID["102"]; r.Compliant || r.RPOPass || r.AutoverifyPass {
		t.Errorf("device 102 should fail both RPO and AutoVerify (unhealthy boot): %+v", r)
	}
}

func TestComplianceFailingOnly(t *testing.T) {
	dbPath := newFleetFixtureStore(t)
	out := runFleetCmd(t, newNovelComplianceCmd, "--failing-only", "--db", dbPath)
	var rows []complianceRow
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(rows) != 3 {
		t.Errorf("failing-only rows = %d, want 3 (100 and 101 are compliant)", len(rows))
	}
	for _, r := range rows {
		if r.Compliant {
			t.Errorf("compliant row %s leaked through --failing-only", r.DeviceID)
		}
	}
}

func TestBillingCommand(t *testing.T) {
	dbPath := newFleetFixtureStore(t)
	out := runFleetCmd(t, newNovelBillingCmd, "--db", dbPath)
	var rows []billingRow
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("billing output not a JSON array: %v\n%s", err, out)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2 clients", len(rows))
	}
	byClient := map[string]billingRow{}
	for _, r := range rows {
		byClient[r.ClientID] = r
	}
	acme := byClient["1"]
	if acme.DevicesTotal != 2 || acme.Servers != 1 || acme.Workstations != 1 || acme.D2CDevices != 1 || acme.ApplianceDevices != 1 {
		t.Errorf("acme counts wrong: %+v", acme)
	}
	if acme.CloudUsageBytes != 2500 || acme.LocalUsageBytes != 1000 {
		t.Errorf("acme usage sums wrong: cloud=%d local=%d", acme.CloudUsageBytes, acme.LocalUsageBytes)
	}
	globex := byClient["2"]
	if globex.DevicesTotal != 3 || globex.Servers != 2 || globex.NAS != 1 {
		t.Errorf("globex counts wrong: %+v", globex)
	}
	if globex.LocalUsageBytes != 3500 {
		t.Errorf("globex local usage = %d, want 3500", globex.LocalUsageBytes)
	}
}

func TestApplianceMapCommand(t *testing.T) {
	dbPath := newFleetFixtureStore(t)
	out := runFleetCmd(t, newNovelApplianceMapCmd, "--db", dbPath)
	var view applianceMapView
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("appliance-map output not JSON: %v\n%s", err, out)
	}
	if view.TotalAppliances != 2 {
		t.Fatalf("appliances = %d, want 2", view.TotalAppliances)
	}
	if view.AssignedDevices != 2 || view.UnmappedDevices != 3 {
		t.Errorf("assigned=%d unmapped=%d, want 2/3 (d2c + two no-sid devices unmapped)", view.AssignedDevices, view.UnmappedDevices)
	}
	for _, a := range view.Appliances {
		switch a.ServiceID {
		case "abcd":
			if a.DeviceCount != 1 || a.Devices[0].DeviceID != "100" || a.ClientName != "Acme Corp" {
				t.Errorf("Acme-BDR mapping wrong: %+v", a)
			}
		case "wxyz":
			if a.DeviceCount != 1 || a.Devices[0].DeviceID != "102" {
				t.Errorf("Globex-BDR mapping wrong: %+v", a)
			}
		}
	}
}

func TestClientRollupCommand(t *testing.T) {
	dbPath := newFleetFixtureStore(t)
	out := runFleetCmd(t, newNovelClientRollupCmd, "--hours", "24", "--db", dbPath)
	var rows []clientRollupRow
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("client-rollup output not a JSON array: %v\n%s", err, out)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	byClient := map[string]clientRollupRow{}
	for _, r := range rows {
		byClient[r.ClientID] = r
	}
	acme := byClient["1"]
	if acme.DevicesTotal != 2 || acme.Failing != 1 || acme.RPOBreaches != 0 || acme.AutoverifyFailing != 0 || acme.Healthy {
		t.Errorf("acme rollup wrong: %+v", acme)
	}
	globex := byClient["2"]
	if globex.DevicesTotal != 3 || globex.Failing != 1 || globex.RPOBreaches != 3 || globex.AutoverifyFailing != 1 || globex.Healthy {
		t.Errorf("globex rollup wrong: %+v", globex)
	}
}

// Empty-store behavior: commands must return honest empties, not errors.
func TestFleetCommandsEmptyStore(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "empty.db")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	db.Close()

	out := runFleetCmd(t, newNovelHealthCmd, "--hours", "24", "--db", dbPath)
	var hv healthView
	if err := json.Unmarshal([]byte(out), &hv); err != nil || hv.TotalDevices != 0 || hv.Note == "" {
		t.Errorf("empty health should carry a note: %v %s", err, out)
	}

	out = runFleetCmd(t, newNovelBillingCmd, "--db", dbPath)
	if out == "null\n" || out == "null" {
		t.Errorf("billing emitted null instead of [] on empty store")
	}
	var br []billingRow
	if err := json.Unmarshal([]byte(out), &br); err != nil || len(br) != 0 {
		t.Errorf("empty billing should be []: %v %s", err, out)
	}
}

// Synthetic autoverify ids: rows without a native id field must not collapse.
func TestAutoverifySyntheticIDsDistinct(t *testing.T) {
	dbPath := newFleetFixtureStore(t)
	db, err := store.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	var n int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM "autoverify"`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 3 {
		t.Errorf("autoverify rows = %d, want 3 distinct rows", n)
	}
	av, err := loadLatestAutoverify(db)
	if err != nil {
		t.Fatalf("loadLatestAutoverify: %v", err)
	}
	if len(av) != 3 {
		t.Errorf("loadLatestAutoverify devices = %d, want 3", len(av))
	}
	// tri-state: success with absent is_healthy must count as passing
	if got := av["101"]; got.IsHealthy != nil || !avPassed(got, true) {
		t.Errorf("device 101 nil-is_healthy should pass autoverify: %+v", got)
	}
	if got := av["100"]; got.Status != "success" || got.IsHealthy == nil || !*got.IsHealthy {
		t.Errorf("device 100 autoverify summary wrong: %+v", got)
	}
	if got := av["102"]; got.IsHealthy == nil || *got.IsHealthy {
		t.Errorf("device 102 should be unhealthy: %+v", got)
	}
}

// Live API shape: the vendor backend leaks Python-style id_ keys and omits
// client_id on org-level device rows. The fleet commands must still attribute
// devices to clients via the client_device dependent mapping.
func TestFleetLiveShapeIDUnderscoreAndClientFallback(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "live.db")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	now := time.Now().UTC()
	ts := func(age time.Duration) string { return now.Add(-age).Format(time.RFC3339) }

	// client row in live shape (id_)
	if err := db.Upsert("clients", "7", json.RawMessage(`{"id_":7,"name":"Initech"}`)); err != nil {
		t.Fatalf("seed client: %v", err)
	}
	// device row in live shape: id_ only, NO client_id, failing status
	dev := `{"id_":700,"name":"initech-fs01","type":"SERVER","d2c":false,
	  "current_health_status":{"status":"TROUBLESHOOTING","reason":"JOB_FAILED","timestamp":"` + ts(time.Hour) + `"},
	  "latest_local_rp":"` + ts(2*time.Hour) + `"}`
	if err := db.Upsert("device", "700", json.RawMessage(dev)); err != nil {
		t.Fatalf("seed device: %v", err)
	}
	// client_device dependent row mapping device 700 -> client 7 (as the
	// hand-wired sync injection would store it: client_id injected)
	if stored, failures, err := db.UpsertBatch("client_device", []json.RawMessage{
		json.RawMessage(`{"id_":700,"name":"initech-fs01","client_id":"7"}`),
	}); err != nil || stored != 1 || failures != 0 {
		t.Fatalf("seed client_device: stored=%d failures=%d err=%v", stored, failures, err)
	}
	db.Close()

	out := runFleetCmd(t, newNovelHealthCmd, "--hours", "24", "--db", dbPath)
	var view healthView
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("decode: %v\n%s", err, out)
	}
	if view.TotalDevices != 1 || view.FailingDevices != 1 {
		t.Fatalf("live-shape device not loaded: %+v", view)
	}
	if len(view.Clients) != 1 {
		t.Fatalf("clients = %d, want 1", len(view.Clients))
	}
	c := view.Clients[0]
	if c.ClientID != "7" {
		t.Errorf("client fallback mapping failed: client_id = %q, want \"7\"", c.ClientID)
	}
	if c.ClientName != "Initech" {
		t.Errorf("id_ client-name join failed: %q, want Initech", c.ClientName)
	}
	if c.Devices[0].DeviceID != "700" {
		t.Errorf("device id_ coalesce failed: %q, want 700", c.Devices[0].DeviceID)
	}
}
