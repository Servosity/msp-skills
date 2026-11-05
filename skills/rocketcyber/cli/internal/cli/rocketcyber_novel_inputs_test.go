// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// rocketcyber handfix novel-missing-input-is-an-error / novel-scan-caps-said:
// the novels never read a failed, missing or undecodable answer as "nothing
// found", never stop at a silent row cap, and say when a list they read was
// only one page of a longer one.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"rocketcyber-pp-cli/internal/cliutil/testenv"
	"rocketcyber-pp-cli/internal/store"
)

type novelFakeAnswer struct {
	status int
	body   string
}

// runNovel runs one command against a fake RocketCyber API answering each
// path (without the /v3 prefix) from answers; unknown paths answer an empty
// list.
func runNovel(t *testing.T, answers map[string]novelFakeAnswer, args ...string) (map[string]any, string, error) {
	t.Helper()
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		a, ok := answers[strings.TrimPrefix(r.URL.Path, "/v3")]
		if !ok {
			a = novelFakeAnswer{200, `{"totalCount":0,"currentPage":1,"totalPages":1,"data":[]}`}
		}
		w.WriteHeader(a.status)
		_, _ = w.Write([]byte(a.body))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("ROCKETCYBER_BASE_URL", srv.URL+"/v3")
	t.Setenv("ROCKETCYBER_API_TOKEN", "novelInputsTok-3c9e71a5")
	cmd := RootCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(append(args, "--json"))
	err := cmd.Execute()
	var view map[string]any
	_ = json.Unmarshal(out.Bytes(), &view)
	return view, out.String() + errOut.String(), err
}

func novelIncident(id int, created time.Time) string {
	return fmt.Sprintf(`{"id":"%d","title":"t%d","status":"open","createdAt":%q}`, id, id, created.UTC().Format(time.RFC3339))
}

func TestTriageFailsWhenEverySourceFails(t *testing.T) {
	bad := novelFakeAnswer{400, `{"message":"bad request"}`}
	_, out, err := runNovel(t, map[string]novelFakeAnswer{"/incidents": bad, "/agents": bad, "/events/summary": bad}, "triage", "--since", "24h")
	if err == nil {
		t.Fatalf("triage exited 0 with every source failing (an empty board reads as an all-clear):\n%s", out)
	}
}

func TestTriageFailedOrUndecodableSourceIsUnknownNotZero(t *testing.T) {
	agents := novelFakeAnswer{200, `{"totalCount":1,"currentPage":1,"totalPages":1,"data":[{"hostname":"pc-1","connectivity":"offline","lastConnected":"2026-10-01T00:00:00Z"}]}`}
	for name, incidents := range map[string]novelFakeAnswer{
		"failed":      {400, `{"message":"bad request"}`},
		"undecodable": {200, `{"message":"maintenance"}`},
	} {
		t.Run(name, func(t *testing.T) {
			view, out, err := runNovel(t, map[string]novelFakeAnswer{"/incidents": incidents, "/agents": agents}, "triage", "--since", "24h")
			if err != nil {
				t.Fatalf("triage failed although two sources answered: %v\n%s", err, out)
			}
			if v, ok := view["open_incidents_total"]; ok && v != nil {
				t.Fatalf("open_incidents_total = %v with the incidents source %s; want null (unknown):\n%s", v, name, out)
			}
			if !strings.Contains(out, `"source":"incidents"`) && !strings.Contains(out, `"source": "incidents"`) {
				t.Fatalf("fetch_failures does not name the incidents source:\n%s", out)
			}
		})
	}
}

func TestTriageSaysWhenTheIncidentPageIsCapped(t *testing.T) {
	now := time.Now()
	var items []string
	for i := 0; i < 50; i++ {
		items = append(items, novelIncident(i, now.Add(-time.Duration(i)*time.Minute)))
	}
	incidents := novelFakeAnswer{200, `{"totalCount":120,"currentPage":1,"totalPages":3,"data":[` + strings.Join(items, ",") + `]}`}
	view, out, err := runNovel(t, map[string]novelFakeAnswer{"/incidents": incidents}, "triage", "--since", "24h")
	if err != nil {
		t.Fatalf("triage: %v\n%s", err, out)
	}
	if view["open_incidents_capped"] != true || view["recent_open_count_capped"] != true {
		t.Fatalf("120 open incidents with 50 shown, all recent: want open_incidents_capped and recent_open_count_capped true:\n%s", out)
	}
}

func seedNovelStore(t *testing.T, resource string, rows []string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "data.db")
	db, err := store.OpenWithContext(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	items := make([]json.RawMessage, len(rows))
	for i, r := range rows {
		items[i] = json.RawMessage(r)
	}
	if stored, _, err := db.UpsertBatch(resource, items); err != nil || stored != len(rows) {
		t.Fatalf("seeding %d %s rows: stored %d, err %v", len(rows), resource, stored, err)
	}
	return path
}

// TestAgentsStaleReadsTheWholeStore pins the class for every store-reading
// novel (agents stale and incidents mttr share the fix: List with no limit).
// It runs in a fresh process: the held set is process-wide and only grows, so
// after the package's other tests every seeded row is checked against ~2000
// held values and seeding 10050 rows outlasts the default go test timeout.
func TestAgentsStaleReadsTheWholeStore(t *testing.T) {
	if os.Getenv("ROCKETCYBER_NOVEL_FRESH_PROCESS") == "" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestAgentsStaleReadsTheWholeStore$", "-test.count=1", "-test.v")
		cmd.Env = append(os.Environ(), "ROCKETCYBER_NOVEL_FRESH_PROCESS=1")
		out, err := cmd.CombinedOutput()
		if err != nil || !strings.Contains(string(out), "--- PASS: TestAgentsStaleReadsTheWholeStore") {
			t.Fatalf("fresh-process run: %v\n%s", err, out)
		}
		return
	}
	old := time.Now().Add(-60 * 24 * time.Hour).UTC().Format(time.RFC3339)
	rows := make([]string, 10050)
	for i := range rows {
		rows[i] = fmt.Sprintf(`{"id":"a%d","hostname":"pc-%d","connectivity":"offline","lastConnected":%q}`, i, i, old)
	}
	db := seedNovelStore(t, "agents", rows)
	view, out, err := runNovel(t, nil, "agents", "stale", "--since", "7d", "--db", db, "--limit", "1")
	if err != nil {
		t.Fatalf("agents stale: %v\n%s", err, out)
	}
	if got, _ := view["stale_count"].(float64); got != 10050 {
		t.Fatalf("stale_count = %v for 10050 silent agents in the store; a row cap dropped some", view["stale_count"])
	}
}

func TestAgentsStaleNeverClaimsAnUndatedAgentReported(t *testing.T) {
	db := seedNovelStore(t, "agents", []string{`{"id":"a1","hostname":"pc-1","connectivity":"offline","lastConnected":null}`})
	view, out, err := runNovel(t, nil, "agents", "stale", "--since", "7d", "--db", db)
	if err != nil {
		t.Fatalf("agents stale: %v\n%s", err, out)
	}
	if strings.Contains(out, "reported within") {
		t.Fatalf("an agent with no lastConnected was claimed as reporting within the window:\n%s", out)
	}
	if got, _ := view["without_last_connected"].(float64); got != 1 {
		t.Fatalf("without_last_connected = %v, want 1:\n%s", view["without_last_connected"], out)
	}
}

func TestMTTRCountsEveryResolvedIncidentAsResolved(t *testing.T) {
	now := time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339)
	db := seedNovelStore(t, "incidents", []string{
		// The vendor's documented incident sample answers status "closed".
		`{"id":"1","status":"closed","createdAt":"` + now + `","resolvedAt":null}`,
		`{"id":"2","status":"resolved","createdAt":"` + now + `","resolvedAt":"` + now + `"}`,
	})
	view, out, err := runNovel(t, nil, "incidents", "mttr", "--since", "30d", "--db", db)
	if err != nil {
		t.Fatalf("incidents mttr: %v\n%s", err, out)
	}
	if view["open_count"] != float64(0) || view["resolved_count"] != float64(2) {
		t.Fatalf("a closed incident and one resolved the second it opened: want open_count 0, resolved_count 2:\n%s", out)
	}
}

func TestLiveNovelsRefuseAMissingOrUndecodableAnswer(t *testing.T) {
	cases := []struct {
		name string
		path string
		body string
		args []string
	}{
		{"defender without devicesAtRisk", "/defender", `{"detectionSummary":{"totalEvents":3}}`, []string{"defender", "riskiest"}},
		{"defender not JSON", "/defender", `<html>maintenance</html>`, []string{"defender", "riskiest"}},
		{"office without a series", "/office", `{}`, []string{"office", "trend"}},
		{"office with an empty series", "/office", `{"secureScoreProgress":{"data":[]}}`, []string{"office", "trend"}},
		{"suppression rules undecodable", "/suppression/rules", `{"message":"maintenance"}`, []string{"suppression", "audit"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, out, err := runNovel(t, map[string]novelFakeAnswer{tc.path: {200, tc.body}}, append(tc.args, "--account-id", "2")...)
			if err == nil {
				t.Fatalf("%v exited 0 on %s (read as an all-clear):\n%s", tc.args, tc.body, out)
			}
		})
	}
}

func TestSuppressionAuditSaysWhenTheRuleListIsIncomplete(t *testing.T) {
	rules := novelFakeAnswer{200, `{"totalCount":30,"currentPage":1,"totalPages":2,"data":[{"ruleId":1,"ruleName":"a","status":"active","updatedAt":"2020-01-01T00:00:00Z"},{"ruleId":2,"ruleName":"b","status":"active","updatedAt":"2020-01-01T00:00:00Z"}]}`}
	view, out, err := runNovel(t, map[string]novelFakeAnswer{"/suppression/rules": rules}, "suppression", "audit", "--account-id", "2")
	if err != nil {
		t.Fatalf("suppression audit: %v\n%s", err, out)
	}
	if view["incomplete"] != true {
		t.Fatalf("30 rules with 2 answered: want incomplete true:\n%s", out)
	}
}

// Codex r1 [P2] wrong provenance: the novels declared their data source in a
// comment only, so a live novel's --agent envelope said meta.source "local"
// and ROCKETCYBER_NO_LOCAL_DATA=1 refused the local ones.
func TestNovelsDeclareTheirDataSource(t *testing.T) {
	for path, want := range map[string]string{
		"triage": "live", "defender riskiest": "live", "office trend": "live", "suppression audit": "live",
		"incidents mttr": "local", "agents stale": "local",
	} {
		cmd, _, err := RootCmd().Find(strings.Fields(path))
		if err != nil || cmd.Annotations["pp:data-source"] != want {
			t.Errorf("%s: pp:data-source = %q (err %v), want %q", path, cmd.Annotations["pp:data-source"], err, want)
		}
	}
}
