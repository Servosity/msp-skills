// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"backup-radar-pp-cli/internal/cliutil/testenv"
	"backup-radar-pp-cli/internal/radar"
	"backup-radar-pp-cli/internal/store"
)

// seedRadarStore writes v3 and v2 rows (shaped like the official spec
// examples, synthetic names) into a fresh local store the way sync does.
func seedRadarStore(t *testing.T) string {
	t.Helper()
	day := func(n int) string {
		return time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -n).Format("2006-01-02T15:04:05Z")
	}
	v2day := func(n int) string { return strings.TrimSuffix(day(n), "Z") }
	seed := map[string]string{
		"devices": fmt.Sprintf(`[
 {"device_id":"dev-1","br_device_id":"Q2c4N1RhM06xY1k9","client":{"name":"Corp","br_company_name":"Corp BR","client_id":"cli-1"},"device_name":"Server-01","br_device_name":"SERVER-01","br_job_name":"Nightly Backup",
  "last_result":[{"status":"error","date":"%s"}],"history":[{"date":"%s","sla_status":"Out Of Compliance - No Results"},{"date":"%s","sla_status":"Out Of Compliance"},{"date":"%s","sla_status":"In Compliance"}]},
 {"device_id":null,"br_device_id":"Zz9_unmapped","client":{"name":null,"br_company_name":"Fabrik BR","client_id":null},"device_name":null,"br_device_name":"fab-nas.fabrik.local","br_job_name":"NAS Sync",
  "last_result":[],"history":[{"date":"%s","sla_status":"At Risk"}]}]`, day(4), day(1), day(2), day(3), day(1)),
		"health": fmt.Sprintf(`[{"client":{"name":"Corp","client_id":"cli-1"},"history":[
 {"date":"%s","compliance":{"pending_count":0,"at_risk_count":1,"in_compliance_count":8,"out_of_compliance_count":1,"out_of_compliance_no_results_count":0}},
 {"date":"%s","compliance":{"pending_count":0,"at_risk_count":0,"in_compliance_count":10,"out_of_compliance_count":0,"out_of_compliance_no_results_count":0}}]}]`, day(1), day(8)),
		"backups": fmt.Sprintf(`[
 {"ticketingCompany":"CW Company","status":{"id":3,"name":"Failure"},"daysInStatus":2.24,"lastResult":"%s","lastSuccess":"%s","ticketCount":0,"tags":["Veeam"],"note":"Primary DC","backupId":1000,"companyName":"Corp BR","deviceName":"SERVER-01","jobName":"Nightly Backup","methodName":"Veeam",
  "history":[{"status":{"id":3,"name":"Failure"},"date":"%s","countFailure":1,"countWarning":0,"countSuccess":0,"countNoResult":0}]},
 {"ticketingCompany":"[Undefined]","status":{"id":2,"name":"Warning"},"daysInStatus":0.3,"lastResult":"%s","ticketCount":1,"treatWarningAsSuccess":true,"backupId":1003,"companyName":"XYZ Company","deviceName":"BR-DVS01","jobName":"BR Daily Report",
  "history":[{"status":{"id":2,"name":"Warning"},"date":"%s","countWarning":2}]},
 {"ticketingCompany":"CW Company","status":{"id":1,"name":"Success"},"daysInStatus":4,"lastResult":"%s","ticketCount":0,"backupId":1004,"companyName":"Old Co","deviceName":"OLD-SRV","jobName":"Weekly"}]`,
			v2day(4), v2day(5), v2day(1), v2day(0), v2day(1), v2day(74)),
		"backups-inactive": fmt.Sprintf(`[{"emailFrom":"[Undefined]","lastReceived":"%s","backupId":12001,"companyName":"[Undefined]","deviceName":"SERVER09","jobName":"OneTwo - (dead0000beef)","methodName":"Datto API"}]`, v2day(0)),
	}
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for resource, raw := range seed {
		var items []json.RawMessage
		if err := json.Unmarshal([]byte(raw), &items); err != nil {
			t.Fatalf("%s fixture: %v", resource, err)
		}
		if _, _, err := db.UpsertBatch(resource, items); err != nil {
			t.Fatalf("seed %s: %v", resource, err)
		}
		if err := db.SaveSyncState(resource, "", len(items)); err != nil {
			t.Fatal(err)
		}
	}
	return dbPath
}

func runRadarCmd(t *testing.T, stdin string, args ...string) (string, error) {
	t.Helper()
	cmd := RootCmd()
	cmd.SetArgs(args)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetIn(strings.NewReader(stdin))
	err := cmd.Execute()
	return out.String(), err
}

func decodeJSON(t *testing.T, s string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(s), v); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, s)
	}
}

func TestRadarCommandsAgainstSeededStore(t *testing.T) {
	testenv.Isolate(t)
	dbPath := seedRadarStore(t)

	t.Run("check", func(t *testing.T) {
		out, err := runRadarCmd(t, "", "check", "--json", "--db", dbPath)
		if err != nil {
			t.Fatal(err)
		}
		var v struct {
			Rows []struct {
				Device, State, Source, Trend string
				TicketCount                  *int `json:"ticket_count"`
			}
			Counts map[string]int
		}
		decodeJSON(t, out, &v)
		if len(v.Rows) != 1 || v.Rows[0].Device != "Server-01" || v.Rows[0].State != "no_results" || v.Rows[0].Source != "both" || v.Rows[0].Trend != "new" || v.Rows[0].TicketCount == nil {
			t.Errorf("check rows: %+v", v.Rows)
		}
		out, _ = runRadarCmd(t, "", "check", "--include-at-risk", "--json", "--db", dbPath)
		decodeJSON(t, out, &v)
		if v.Counts["total"] != 3 {
			t.Errorf("include-at-risk total: %+v", v.Counts)
		}
	})

	t.Run("unwatched", func(t *testing.T) {
		out, err := runRadarCmd(t, "", "unwatched", "--json", "--db", dbPath)
		if err != nil {
			t.Fatal(err)
		}
		var v struct {
			Counts  map[string]int
			Skipped []any
		}
		decodeJSON(t, out, &v)
		for reason, n := range map[string]int{"inactive": 1, "unmapped": 1, "ticketless": 1, "no-psa": 1, "masked": 1} {
			if v.Counts[reason] != n {
				t.Errorf("%s = %d, want %d (%+v)", reason, v.Counts[reason], n, v.Counts)
			}
		}
		if len(v.Skipped) != 0 {
			t.Errorf("both generations synced, nothing should be skipped: %+v", v.Skipped)
		}
		if _, err := runRadarCmd(t, "", "unwatched", "--reason", "bogus", "--db", dbPath); err == nil {
			t.Error("unknown reason must be a usage error")
		}
	})

	t.Run("coverage diff", func(t *testing.T) {
		vendor := `{"data":[{"hostname":"server-01.corp.local"},{"hostname":"SERVER09"},{"hostname":"ghost-7"}]}`
		out, err := runRadarCmd(t, vendor, "coverage", "diff", "--devices", "-", "--json", "--db", dbPath)
		if err != nil {
			t.Fatal(err)
		}
		var v struct {
			NameFields []string `json:"name_fields"`
			Counts     map[string]int
			VendorOnly []struct{ Name string } `json:"vendor_only"`
		}
		decodeJSON(t, out, &v)
		if len(v.NameFields) != 1 || v.NameFields[0] != "hostname" || v.Counts["matched"] != 2 || v.Counts["matched_inactive_only"] != 1 || len(v.VendorOnly) != 1 || v.VendorOnly[0].Name != "ghost-7" {
			t.Errorf("coverage: %+v", v)
		}
		if _, err := runRadarCmd(t, "", "coverage", "diff", "--json", "--db", dbPath); err == nil {
			t.Error("missing --devices must be a usage error")
		}
	})

	t.Run("stale bodies", func(t *testing.T) {
		out, err := runRadarCmd(t, "", "stale", "--days", "30", "--bodies", "--db", dbPath)
		if err != nil {
			t.Fatal(err)
		}
		// fab-nas (v3, unmapped, scoped by its Backup Radar company) gets a body;
		// OLD-SRV is v2-only, so it gets a retire_note instead.
		lines := strings.Split(strings.TrimSpace(out), "\n")
		if len(lines) != 1 || !strings.Contains(lines[0], "fab-nas") {
			t.Fatalf("want only the fab-nas body, got:\n%s", out)
		}
		var body map[string]any
		decodeJSON(t, lines[0], &body)
		if body["dry_run"] != true || body["reason"] == "" || body["device_name"] == nil {
			t.Errorf("retire body: %v", body)
		}
	})

	t.Run("scorecard", func(t *testing.T) {
		out, err := runRadarCmd(t, "", "scorecard", "Corp", "--days", "30", "--json", "--db", dbPath)
		if err != nil {
			t.Fatal(err)
		}
		var v struct {
			Clients []struct {
				Client       string
				Rate         *float64
				WorstDevices []struct{ Device string } `json:"worst_devices"`
			}
		}
		decodeJSON(t, out, &v)
		if len(v.Clients) != 1 || v.Clients[0].Rate == nil || *v.Clients[0].Rate != 90 || len(v.Clients[0].WorstDevices) != 1 {
			t.Errorf("scorecard: %+v", v.Clients)
		}
		if _, err := runRadarCmd(t, "", "scorecard", "Nobody Inc", "--db", dbPath); err == nil {
			t.Error("unknown client with synced health must be not-found")
		}
	})

	t.Run("timeline", func(t *testing.T) {
		out, err := runRadarCmd(t, "", "timeline", "server-01", "--days", "7", "--json", "--db", dbPath)
		if err != nil {
			t.Fatal(err)
		}
		var v struct {
			Jobs []struct {
				BackupID int64 `json:"backup_id"`
			}
			History []struct{ Date, SLAStatus, V2Status string } `json:"history"`
		}
		decodeJSON(t, out, &v)
		if len(v.Jobs) != 1 || v.Jobs[0].BackupID != 1000 || len(v.History) != 3 {
			t.Errorf("timeline: %+v", v)
		}
	})

	t.Run("push dry-run", func(t *testing.T) {
		csvPath := filepath.Join(t.TempDir(), "results.csv")
		if err := os.WriteFile(csvPath, []byte("client,device,job,time,status\nCorp,SRV1,Nightly,2026-10-13T02:00:00Z,success\nCorp,SRV2,Nightly,2026-10-13T02:00:00Z,failure\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		out, err := runRadarCmd(t, "", "push", "--from", csvPath, "--batch", "1", "--dry-run", "--json")
		if err != nil {
			t.Fatal(err)
		}
		var v struct {
			DryRun      bool `json:"dry_run"`
			Rows        int
			Chunks      []any
			PreviewBody []struct {
				DeviceName string `json:"device_name"`
			} `json:"preview_body"`
		}
		decodeJSON(t, out, &v)
		if !v.DryRun || v.Rows != 2 || len(v.Chunks) != 2 || len(v.PreviewBody) != 1 || v.PreviewBody[0].DeviceName != "SRV1" {
			t.Errorf("push plan: %+v", v)
		}
		bad := filepath.Join(t.TempDir(), "bad.csv")
		_ = os.WriteFile(bad, []byte("client,device,job,time,status\nCorp,SRV1,Nightly,2026-10-13T02:00:00Z,failed\n"), 0o600)
		if _, err := runRadarCmd(t, "", "push", "--from", bad, "--dry-run"); err == nil {
			t.Error("invalid status must fail validation before any request")
		}
	})

	t.Run("empty mirror", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "none.db")
		out, err := runRadarCmd(t, "", "check", "--json", "--db", missing)
		if err != nil {
			t.Fatal(err)
		}
		var v struct{ Rows []any }
		decodeJSON(t, out, &v)
		if v.Rows == nil || len(v.Rows) != 0 {
			t.Errorf("missing mirror must emit rows: [], got %s", out)
		}
	})
}

func TestRadarStoreSemantics(t *testing.T) {
	testenv.Isolate(t)

	t.Run("agent keeps curated fields", func(t *testing.T) {
		dbPath := seedRadarStore(t)
		out, err := runRadarCmd(t, "", "check", "--agent", "--db", dbPath)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, `"ticket_count"`) || !strings.Contains(out, `"days_in_state"`) || !strings.Contains(out, `"v2_status"`) {
			t.Errorf("--agent stripped report fields:\n%s", out)
		}
	})

	t.Run("coverage --agent keeps other_client", func(t *testing.T) {
		dbPath := seedRadarStore(t)
		out, err := runRadarCmd(t, "", "coverage", "diff", "--names", "SERVER-01,BR-DVS01,OLD-SRV,fab-nas", "--client", "o", "--agent", "--db", dbPath) // only fab-nas (Fabrik BR) is out of scope
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, `"other_client": true`) {
			t.Errorf("--agent dropped other_client:\n%s", out)
		}
	})

	t.Run("failed attempt is not synced; retired backups drop out", func(t *testing.T) {
		dbPath := filepath.Join(t.TempDir(), "data.db")
		db, err := store.Open(dbPath)
		if err != nil {
			t.Fatal(err)
		}
		devices := []json.RawMessage{json.RawMessage(`{"device_id":null,"br_device_id":"Zz9","client":{"name":null,"br_company_name":"Fabrik BR","client_id":null},"br_device_name":"fab-nas","br_job_name":"NAS","last_result":[],"history":[]}`)}
		backups := []json.RawMessage{json.RawMessage(`{"status":{"name":"Success"},"lastResult":"2026-01-01T00:00:00","backupId":42,"companyName":"Old Co","deviceName":"GONE","jobName":"Weekly"}`)}
		retired := []json.RawMessage{
			json.RawMessage(`{"backupId":42,"companyName":"Old Co","deviceName":"GONE","jobName":"Weekly","RetiredDate":"2026-02-01T00:00:00"}`),
			json.RawMessage(`{"backupId":43,"companyName":"Fabrik BR","deviceName":"reused","jobName":"NAS","RetiredDate":"2026-02-01T00:00:00"}`),
			json.RawMessage(`{"backupId":44,"companyName":"Old Co","deviceName":"GONE2","jobName":"Weekly","RetiredDate":"2026-02-01T00:00:00"}`),
		}
		// A retired backup leaves the v3 device list (sync prunes it), so only
		// its v2 row is stored; R1 is a new v3 backup reusing retired names.
		devices = append(devices,
			json.RawMessage(`{"device_id":null,"br_device_id":"R1","client":{"name":null,"br_company_name":"Fabrik BR","client_id":null},"br_device_name":"reused","br_job_name":"NAS","last_result":[{"status":"error","date":"2026-03-01T00:00:00Z"}],"history":[]}`),
			// N1 never reported and reuses a retired backup's names; it must stay visible.
			json.RawMessage(`{"device_id":null,"br_device_id":"N1","client":{"name":null,"br_company_name":"Old Co","client_id":null},"br_device_name":"GONE2","br_job_name":"Weekly","last_result":[],"history":[]}`))
		for res, items := range map[string][]json.RawMessage{"devices": devices, "backups": backups, "backups-retired": retired} {
			if _, _, err := db.UpsertBatch(res, items); err != nil {
				t.Fatal(err)
			}
			if err := db.SaveSyncState(res, "", len(items)); err != nil {
				t.Fatal(err)
			}
		}
		// backups-inactive: an attempt started (v2 401) but never completed.
		if err := db.SaveSyncProgress("backups-inactive", "", 0); err != nil {
			t.Fatal(err)
		}
		db.Close()

		out, err := runRadarCmd(t, "", "unwatched", "--json", "--db", dbPath)
		if err != nil {
			t.Fatal(err)
		}
		var v struct {
			Counts  map[string]int
			Skipped []struct{ Reason string }
		}
		decodeJSON(t, out, &v)
		if len(v.Skipped) != 1 || v.Skipped[0].Reason != "inactive" || v.Counts["unmapped"] != 3 {
			t.Errorf("only the never-completed collection is skipped; Zz9, R1 and N1 stay: %+v", v)
		}
		out, err = runRadarCmd(t, "", "stale", "--days", "30", "--json", "--db", dbPath)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out, `"GONE"`) {
			t.Errorf("retired backup's v2 row must not be a retire candidate:\n%s", out)
		}
		if !strings.Contains(out, "GONE2") {
			t.Errorf("a never-reported v3 backup reusing retired names must stay:\n%s", out)
		}
		if !strings.Contains(out, "reused") {
			t.Errorf("a v3 backup reporting after a same-named retirement must stay:\n%s", out)
		}
	})
}

func TestRadarLoaderRules(t *testing.T) {
	testenv.Isolate(t)
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	put := func(res string, sync bool, items ...string) {
		raw := make([]json.RawMessage, 0, len(items))
		for _, it := range items {
			raw = append(raw, json.RawMessage(it))
		}
		if _, _, err := db.UpsertBatch(res, raw); err != nil {
			t.Fatal(err)
		}
		if sync {
			if err := db.SaveSyncState(res, "", len(raw)); err != nil {
				t.Fatal(err)
			}
		}
	}
	// activated since the inactive list was synced: same id now active
	put("backups", true,
		`{"status":{"name":"Success"},"backupId":7,"companyName":"Acme","deviceName":"NAS09","jobName":"J","ticketingCompany":"PSA"}`,
		// a row cached under "backups" by something that is not an ApiBackup (no status)
		`{"backupId":99,"companyName":"Acme","deviceName":"STRAY","jobName":"J","emailFrom":"x"}`)
	put("backups-inactive", true, `{"backupId":7,"companyName":"Acme","deviceName":"NAS09","jobName":"J","lastReceived":"2026-10-13T00:00:00"}`)
	// devices only from a filtered live read: no sync attempt recorded
	put("devices", false, `{"device_id":null,"br_device_id":"Z","client":{"br_company_name":"Acme"},"br_device_name":"DC01","br_job_name":"J","last_result":[],"history":[]}`)
	db.Close()

	out, err := runRadarCmd(t, "", "unwatched", "--json", "--db", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Counts  map[string]int
		Skipped []struct{ Reason string }
	}
	decodeJSON(t, out, &v)
	if v.Counts["inactive"] != 0 {
		t.Errorf("an activated backup must leave the inactive list: %+v", v.Counts)
	}
	if len(v.Skipped) != 1 || v.Skipped[0].Reason != "unmapped" {
		t.Errorf("devices with no sync attempt must count as not synced: %+v", v.Skipped)
	}
	if strings.Contains(out, "STRAY") {
		t.Errorf("a status-less row cached under backups is not an active backup:\n%s", out)
	}
}

func TestRadarCommandEdges(t *testing.T) {
	testenv.Isolate(t)
	dbPath := seedRadarStore(t)

	out, err := runRadarCmd(t, "", "timeline", "server-01", "--agent", "--db", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"note"`, `"tags"`, `"ticket_count"`, `"v2_status"`} {
		if !strings.Contains(out, key) {
			t.Errorf("timeline --agent dropped %s:\n%s", key, out)
		}
	}

	t.Setenv("BACKUP_RADAR_V2_API_KEY", "")
	if _, err := runRadarCmd(t, "", "timeline", "server-01", "--live", "--db", dbPath); ExitCode(err) != 4 {
		t.Errorf("--live without the v2 key must exit 4 before any call, got %v (exit %d)", err, ExitCode(err))
	}
	if _, err := runRadarCmd(t, "", "scorecard", " ", "--db", dbPath); ExitCode(err) != 2 {
		t.Errorf("blank client must be a usage error, got exit %d", ExitCode(err))
	}

	// stale hint only when a body exists
	empty := filepath.Join(t.TempDir(), "data.db")
	db, err := store.Open(empty)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _ = db.UpsertBatch("backups", []json.RawMessage{json.RawMessage(`{"status":{"name":"Success"},"backupId":3,"companyName":"[Undefined]","deviceName":"X","jobName":"J","lastResult":"2025-01-01T00:00:00"}`)})
	_ = db.SaveSyncState("backups", "", 1)
	db.Close()
	out, err = runRadarCmd(t, "", "stale", "--json", "--db", empty)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, `"next"`) {
		t.Errorf("no retire bodies, so no pipe hint:\n%s", out)
	}
}

func TestRetiredNamesSharedByTwoV3Rows(t *testing.T) {
	testenv.Isolate(t)
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _ = db.UpsertBatch("devices", []json.RawMessage{
		json.RawMessage(`{"device_id":null,"br_device_id":"A1","client":{"br_company_name":"Acme"},"br_device_name":"DC01","br_job_name":"J","last_result":[],"history":[]}`),
		json.RawMessage(`{"device_id":null,"br_device_id":"A2","client":{"br_company_name":"Acme"},"br_device_name":"DC01","br_job_name":"J","last_result":[],"history":[]}`)})
	_, _, _ = db.UpsertBatch("backups-retired", []json.RawMessage{json.RawMessage(`{"backupId":9,"companyName":"Acme","deviceName":"DC01","jobName":"J","RetiredDate":"2026-10-01T00:00:00"}`)})
	_ = db.SaveSyncState("devices", "", 2)
	_ = db.SaveSyncState("backups-retired", "", 1)
	db.Close()
	out, err := runRadarCmd(t, "", "unwatched", "--reason", "unmapped", "--json", "--db", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	var v struct{ Counts map[string]int }
	decodeJSON(t, out, &v)
	if v.Counts["unmapped"] != 2 {
		t.Errorf("names shared by two v3 rows cannot be pinned to the retired one; keep both: %+v", v.Counts)
	}
}

func TestShellQuote(t *testing.T) {
	if got, want := shellQuote(`Zebra $(rm -rf ~) 'x'`), `'Zebra $(rm -rf ~) '\''x'\'''`; got != want {
		t.Errorf("shellQuote = %s, want %s", got, want)
	}
}

func TestLiveSummary(t *testing.T) {
	raw := json.RawMessage(`[{"Success":true},{"Warning":true,"Success":true},{"Failure":true},{"Success":false}]`)
	if got, want := liveSummary(raw), "1 ok / 1 warn / 1 fail"; got != want {
		t.Errorf("liveSummary = %q, want %q", got, want)
	}
	if got := liveSummary(nil); got != "" {
		t.Errorf("no live fetch must render empty, got %q", got)
	}
}

// Regression guards for the polish output review (Phase 4.85 findings).
func TestRadarOutputReviewFixes(t *testing.T) {
	testenv.Isolate(t)
	dbPath := seedRadarStore(t)

	t.Run("timeline names a job-only miss", func(t *testing.T) {
		out, err := runRadarCmd(t, "", "timeline", "server-01", "--job", "No Such Job", "--json", "--db", dbPath)
		if err != nil {
			t.Fatal(err)
		}
		var v struct {
			Job  string
			Note string
		}
		decodeJSON(t, out, &v)
		if v.Job != "No Such Job" || !strings.Contains(v.Note, `no job named "No Such Job"`) || !strings.Contains(v.Note, "Nightly Backup") {
			t.Errorf("job miss must echo the job and list the device's jobs: %+v", v)
		}
	})

	t.Run("unwatched reports zero for a checked reason", func(t *testing.T) {
		// Only Fabrik's unmapped device matches, so both requested reasons find nothing.
		out, err := runRadarCmd(t, "", "unwatched", "--reason", "inactive,ticketless", "--client", "Fabrik", "--json", "--db", dbPath)
		if err != nil {
			t.Fatal(err)
		}
		var v struct{ Counts map[string]int }
		decodeJSON(t, out, &v)
		for _, r := range []string{"inactive", "ticketless"} {
			if n, ok := v.Counts[r]; !ok || n != 0 {
				t.Errorf("checked reason %q must report 0: %+v", r, v.Counts)
			}
		}
	})

	t.Run("timeline empty store has one note", func(t *testing.T) {
		out, err := runRadarCmd(t, "", "timeline", "DC01", "--json", "--db", filepath.Join(t.TempDir(), "none.db"))
		if err != nil {
			t.Fatal(err)
		}
		var v struct {
			Note    string
			Sources struct{ Note string }
		}
		decodeJSON(t, out, &v)
		if v.Note != "" || !strings.Contains(v.Sources.Note, "nothing synced yet") {
			t.Errorf("empty store: note belongs in sources only: %+v", v)
		}
	})

	t.Run("sources note names only the collections a report reads", func(t *testing.T) {
		snap := radar.Snapshot{Synced: map[string]bool{"health": true, "devices": true}}
		if n := sourcesOf(snap, "health", "devices").Note; n != "" {
			t.Errorf("scorecard on a v3-only store must not ask for more: %q", n)
		}
		snap = radar.Snapshot{Synced: map[string]bool{"devices": true}}
		n := sourcesOf(snap, "health", "devices").Note
		if !strings.Contains(n, "client health") || strings.Contains(n, "BACKUP_RADAR_V2_API_KEY") {
			t.Errorf("note must name only missing health: %q", n)
		}
		if n := sourcesOf(radar.Snapshot{}, "devices").Note; !strings.Contains(n, "nothing synced yet") {
			t.Errorf("empty store: %q", n)
		}
	})
}
