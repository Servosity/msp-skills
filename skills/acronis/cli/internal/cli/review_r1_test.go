// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): regression tests for the 4.34.0 reprint review,
// Codex round 1. Each test failed on the reviewed code (commit b3371ac) and
// names the finding class it pins.

package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"acronis-pp-cli/internal/store"
)

// --- auth-login: token-endpoint diagnostics and redirects -------------------

type loginRoundTrip func(*http.Request) (*http.Response, error)

func (f loginRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func loginResponse(r *http.Request, status int, body string, header http.Header) *http.Response {
	if header == nil {
		header = http.Header{}
	}
	return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body)), Request: r}
}

// runLoginThroughBackstop runs `auth login` under the same error backstop
// Execute installs, with every HTTP request answered by rt.
func runLoginThroughBackstop(t *testing.T, secret string, rt http.RoundTripper) error {
	t.Helper()
	old := http.DefaultTransport
	http.DefaultTransport = rt
	t.Cleanup(func() { http.DefaultTransport = old })
	t.Setenv("ACRONIS_CLIENT_ID", "")
	t.Setenv("ACRONIS_CLIENT_SECRET", "")
	t.Setenv("ACRONIS_DATACENTER", "")
	t.Setenv("ACRONIS_BASE_URL", "")
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	t.Setenv("ACRONIS_HOME", t.TempDir())
	flags := &rootFlags{configPath: filepath.Join(t.TempDir(), "config.toml")}
	root := &cobra.Command{Use: "root", SilenceUsage: true, SilenceErrors: true}
	root.AddCommand(newAuthLoginCmd(flags))
	funnelCommandErrors(root)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"login", "--client-id", "review-client-id", "--client-secret", secret, "--datacenter", "eu2-cloud"})
	return root.Execute()
}

func TestReviewAuthLoginNeverShowsProviderText(t *testing.T) {
	cases := []struct {
		name, secret, body string
		status             int
		forbidden          []string
	}{
		{"malformed 200 echoes secret into parser error", "R1ParserSecret-7f3a!", `{"access_token":"newtoken","expires_in":"R1ParserSecret-7f3a!"}`, 200, []string{"R1ParserSecret-7f3a!"}},
		{"short secret echoed in 400", "s3cr3t", `{"error":"s3cr3t"}`, 400, []string{"s3cr3t"}},
		{"secret split across JSON fields", "R1SplitSecretValue9!", `{"parts":["R1SplitSec","retValue9!"]}`, 400, []string{"R1SplitSec", "retValue9!"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := runLoginThroughBackstop(t, tc.secret, loginRoundTrip(func(r *http.Request) (*http.Response, error) {
				return loginResponse(r, tc.status, tc.body, nil), nil
			}))
			if err == nil {
				t.Fatal("login must fail")
			}
			for _, f := range tc.forbidden {
				if strings.Contains(err.Error(), f) {
					t.Fatalf("login error shows credential material %q: %v", f, err)
				}
			}
		})
	}
}

func TestReviewAuthLoginKnownOAuthErrorCodeStillShown(t *testing.T) {
	err := runLoginThroughBackstop(t, "R1KnownCodeSecret-1!", loginRoundTrip(func(r *http.Request) (*http.Response, error) {
		return loginResponse(r, 401, `{"error":"invalid_client","error_description":"R1KnownCodeSecret-1! rejected"}`, nil), nil
	}))
	if err == nil || !strings.Contains(err.Error(), "invalid_client") || strings.Contains(err.Error(), "R1KnownCodeSecret") {
		t.Fatalf("want the allowlisted OAuth code and nothing else from the body, got %v", err)
	}
}

func TestReviewAuthLoginNamesAnEchoWithoutShowingIt(t *testing.T) {
	secret := "R1EchoedSecretValue-3!"
	err := runLoginThroughBackstop(t, secret, loginRoundTrip(func(r *http.Request) (*http.Response, error) {
		return loginResponse(r, 400, `{"error":"invalid_request","detail":"bad secret `+secret+`"}`, nil), nil
	}))
	if err == nil || strings.Contains(err.Error(), secret) || !strings.Contains(err.Error(), "repeated the client secret") || !strings.Contains(err.Error(), "invalid_request") {
		t.Fatalf("want code + echo note and no secret, got %v", err)
	}
}

func TestReviewAuthLoginDoesNotFollowRedirects(t *testing.T) {
	var mu sync.Mutex
	var sinkAuth []string
	err := runLoginThroughBackstop(t, "R1RedirectSecret-2!", loginRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "eu2-cloud.acronis.com" {
			mu.Lock()
			sinkAuth = append(sinkAuth, r.Header.Get("Authorization"))
			mu.Unlock()
			return loginResponse(r, 200, `{"access_token":"sinktoken","expires_in":3600}`, nil), nil
		}
		return loginResponse(r, 307, ``, http.Header{"Location": []string{"https://sink.eu2-cloud.acronis.com/token"}}), nil
	}))
	mu.Lock()
	defer mu.Unlock()
	if len(sinkAuth) > 0 {
		t.Fatalf("login followed a redirect to another host (Authorization %q)", sinkAuth)
	}
	if err == nil {
		t.Fatal("a redirect from the token endpoint must fail the login")
	}
}

// --- response-validation: in-band failure / partial signals ------------------

type reviewPages struct{ pages []string }

func (p *reviewPages) Get(context.Context, string, map[string]string) (json.RawMessage, error) {
	page := p.pages[0]
	if len(p.pages) > 1 {
		p.pages = p.pages[1:]
	}
	return json.RawMessage(page), nil
}

func TestReviewTaskReadsRefuseInBandFailure(t *testing.T) {
	for _, all := range []bool{true, false} {
		_, err := acronisTaskPages(context.Background(), &reviewPages{pages: []string{`{"items":[],"success":false,"error":"backend unavailable"}`}}, "/api/task_manager/v2/tasks", map[string]string{"limit": "100"}, all, "")
		if err == nil {
			t.Fatalf("all=%v: a page declaring failure was returned as data", all)
		}
	}
}

func TestReviewPageValidatorRefusesPartialSignals(t *testing.T) {
	for _, page := range []string{
		`{"items":[{"id":"k1"}],"warnings":["partial enumeration: failed shard"],"cursors":{}}`,
		`{"items":[{"id":"k1"}],"errors":[{"code":500}]}`,
		`{"items":[{"id":"k1"}],"error":"shard down"}`,
		`{"items":[],"success":false}`,
		`{"items":[{"id":"k1"}],"partial":true}`,
		`{"items":[{"id":"k1"}],"paging":{"cursors":{},"incomplete":true}}`,
		`{"items":[{"id":"k1"}],"meta":{"partial":true}}`,
		`{"items":[{"id":"k1"}],"result":{"success":false}}`,
	} {
		if err := acronisValidatePage(json.RawMessage(page)); err == nil {
			t.Errorf("validator accepted %s", page)
		}
	}
	for _, page := range []string{
		`{"items":[{"id":"k1"}],"paging":{"cursors":{}}}`,
		`{"items":[],"warnings":[],"errors":null,"error":""}`,
		`[{"id":"k1"}]`,
	} {
		if err := acronisValidatePage(json.RawMessage(page)); err != nil {
			t.Errorf("validator rejected clean page %s: %v", page, err)
		}
	}
}

// --- task-partition: local activity reads ------------------------------------

func TestReviewLocalActivitiesReadTheActivitiesPartition(t *testing.T) {
	t.Setenv("ACRONIS_HOME", t.TempDir())
	path := defaultDBPath("acronis-cli")
	db, err := store.OpenWithContext(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.UpsertBatch("task-manager", []json.RawMessage{json.RawMessage(`{"id":"task1","state":"completed"}`)}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.UpsertBatch("task-manager-v2-activities", []json.RawMessage{json.RawMessage(`{"id":"act1","state":"started"}`)}); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	// The generated list-activities call site passes "task-manager"; the
	// endpoint path must decide the partition.
	data, _, err := resolveAcronisTaskRead(context.Background(), nil, &rootFlags{dataSource: "local"}, "task-manager", "/api/task_manager/v2/activities", map[string]string{"limit": "100"}, false, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "act1") || strings.Contains(string(data), "task1") {
		t.Fatalf("local activities read returned the wrong partition: %s", data)
	}
}

// --- task-outcome / backup-scope ---------------------------------------------

func seedTask(t *testing.T, db *store.Store, id, tenant, typ, state, rc, completedAt string) {
	t.Helper()
	mustExec(t, db, `INSERT INTO task_manager(id, data, tenant_id, type, state, result_code, completed_at) VALUES(?,?,?,?,?,?,?)`,
		id, `{}`, tenant, nullable(typ), nullable(state), nullable(rc), nullable(completedAt))
}

func TestReviewTerminalFailuresAreNotSuccesses(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	for _, rc := range []string{"timedout", "abandoned", "cancelled", "error"} {
		db := newNovelTestStore(t)
		seedTenant(t, db, "t1", "Acme", "customer", "")
		seedTask(t, db, "k1", "t1", "backup", "completed", rc, "2026-10-24T11:00:00Z")
		health, err := computeHealth(db, 1, now)
		if err != nil || len(health) == 0 || health[0].OK != 0 || health[0].Failed != 1 {
			t.Fatalf("%s: health = %+v, %v; want ok=0 failed=1", rc, health, err)
		}
		fails, err := computeFailures(db, 24*time.Hour, "", now)
		if err != nil || len(fails) != 1 {
			t.Fatalf("%s: failures = %+v, %v; want the task listed", rc, fails, err)
		}
		fresh, err := computeFreshness(db, 24*time.Hour, false, "", now)
		if err != nil || len(fresh) != 1 || !fresh[0].Breached {
			t.Fatalf("%s: freshness = %+v, %v; want breached", rc, fresh, err)
		}
	}
}

func TestReviewNonBackupTasksDoNotEstablishBackupSuccess(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	for _, typ := range []string{"restore", ""} {
		db := newNovelTestStore(t)
		seedTenant(t, db, "t1", "Acme", "customer", "")
		mustExec(t, db, `INSERT INTO agent_manager(id, data, tenant_id, status) VALUES('a1','{}','t1','online')`)
		mustExec(t, db, `INSERT INTO offering_items(id, tenants_id, data) VALUES('o1','t1','{"name":"vms","status":1}')`)
		seedTask(t, db, "k1", "t1", typ, "completed", "ok", "2026-10-24T11:00:00Z")
		fresh, err := computeFreshness(db, 24*time.Hour, false, "", now)
		if err != nil || len(fresh) != 1 || !fresh[0].Breached || fresh[0].LastSuccess != "" {
			t.Fatalf("type %q: freshness = %+v, %v; want breached with no last success", typ, fresh, err)
		}
		cov, err := computeCoverage(db, 7, false, now)
		if err != nil || len(cov) != 1 || cov[0].RecentSuccess || cov[0].Protected {
			t.Fatalf("type %q: coverage = %+v, %v; want unprotected", typ, cov, err)
		}
		card, err := computeCustomerCard(db, "t1", now)
		if err != nil || card.TasksOK7d != 0 || card.LastSuccess != "" {
			t.Fatalf("type %q: customer = %+v, %v; want no backup success", typ, card, err)
		}
	}
}

func TestReviewRestoreFailuresAreNotBackupFailures(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	db := newNovelTestStore(t)
	seedTenant(t, db, "t1", "Acme", "customer", "")
	seedTask(t, db, "k1", "t1", "restore", "completed", "error", "2026-10-24T11:00:00Z")
	seedTask(t, db, "k2", "t1", "", "completed", "error", "2026-10-24T11:00:00Z")
	seedTask(t, db, "k3", "t1", "backup_incremental", "completed", "error", "2026-10-24T11:00:00Z")
	fails, err := computeFailures(db, 24*time.Hour, "", now)
	if err != nil || len(fails) != 2 {
		t.Fatalf("failures = %+v, %v; want the untyped and the unrecognized-type failure, not the restore", fails, err)
	}
	for _, f := range fails {
		if f.TaskID == "k1" {
			t.Fatalf("a restore failure was listed as a backup failure: %+v", fails)
		}
	}
}

func TestReviewOnlyCompletedExplicitBackupsSucceed(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct{ typ, state string }{{"backup_incremental", "completed"}, {"backup", ""}, {"backup", "started"}} {
		db := newNovelTestStore(t)
		seedTenant(t, db, "t1", "Acme", "customer", "")
		seedTask(t, db, "k1", "t1", tc.typ, tc.state, "ok", "2026-10-24T11:00:00Z")
		fresh, err := computeFreshness(db, 24*time.Hour, false, "", now)
		if err != nil || len(fresh) != 1 || !fresh[0].Breached {
			t.Fatalf("%+v: freshness = %+v, %v; want breached", tc, fresh, err)
		}
	}
}

func TestReviewPlaceholderTimesAreUnknown(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	db := newNovelTestStore(t)
	seedTenant(t, db, "t1", "Acme", "customer", "")
	seedTask(t, db, "k1", "t1", "backup", "completed", "error", "0001-01-01T00:00:00Z")
	fails, err := computeFailures(db, 24*time.Hour, "", now)
	if err != nil || len(fails) != 1 || !fails[0].TimeUnknown {
		t.Fatalf("a zero completion time pushed a failure out of the window: %+v, %v", fails, err)
	}
}

// --- agent-status --------------------------------------------------------------

func TestReviewNegativeAgentStatesAreNotOnline(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	for _, status := range []string{"inactive", "disconnected", "unavailable", "", "weird"} {
		db := newNovelTestStore(t)
		seedTenant(t, db, "t1", "Acme", "customer", "")
		mustExec(t, db, `INSERT INTO agent_manager(id, data, tenant_id, status, last_seen) VALUES('a1','{}','t1',?,?)`, nullable(status), "2026-10-24T11:59:00Z")
		mustExec(t, db, `INSERT INTO offering_items(id, tenants_id, data) VALUES('o1','t1','{"name":"vms","status":1}')`)
		seedTask(t, db, "k1", "t1", "backup", "completed", "ok", "2026-10-24T11:00:00Z")
		cov, err := computeCoverage(db, 7, true, now)
		if err != nil || len(cov) != 1 || cov[0].OnlineAgents != 0 {
			t.Fatalf("status %q: coverage = %+v, %v; want unprotected with 0 online", status, cov, err)
		}
		stale, err := computeStaleAgents(db, 24*time.Hour, now)
		if err != nil || len(stale) != 1 {
			t.Fatalf("status %q: stale = %+v, %v; want the agent listed", status, stale, err)
		}
	}
}

// --- agent-timestamp / unparseable timestamps ----------------------------------

func TestReviewUnparseableTimestampsNeverHideFindings(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	db := newNovelTestStore(t)
	seedTenant(t, db, "t1", "Acme", "customer", "")
	mustExec(t, db, `INSERT INTO agent_manager(id, data, tenant_id, status, last_seen) VALUES('a1','{}','t1','online','not-a-time')`)
	stale, err := computeStaleAgents(db, 24*time.Hour, now)
	if err == nil && len(stale) != 1 {
		t.Fatalf("an unparseable last_seen produced a clean zero-stale result: %+v", stale)
	}
	seedTask(t, db, "k1", "t1", "backup", "completed", "error", "not-a-time")
	fails, err := computeFailures(db, 24*time.Hour, "", now)
	if err == nil && len(fails) != 1 {
		t.Fatalf("an unparseable completion time hid a failure: %+v", fails)
	}
	rep, err := computeAlertsRepeat(db, 7, now)
	if err == nil && len(rep) != 1 {
		t.Fatalf("an unparseable completion time hid a repeat failure: %+v", rep)
	}
}

func TestReviewUndatedFailuresStayVisible(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	db := newNovelTestStore(t)
	seedTenant(t, db, "t1", "Acme", "customer", "")
	seedTask(t, db, "k1", "t1", "backup", "completed", "error", "")
	fails, err := computeFailures(db, 24*time.Hour, "", now)
	if err != nil || len(fails) != 1 {
		t.Fatalf("an undated failure was dropped: %+v, %v", fails, err)
	}
	rep, err := computeAlertsRepeat(db, 7, now)
	if err != nil || len(rep) != 1 {
		t.Fatalf("an undated failure was dropped from repeat: %+v, %v", rep, err)
	}
	card, err := computeCustomerCard(db, "t1", now)
	if err != nil || card.TasksFailed7d != 1 {
		t.Fatalf("an undated failure was dropped from the customer card: %+v, %v", card, err)
	}
}

// --- usage-identity / inactive-license ----------------------------------------

func seedUsage(t *testing.T, db *store.Store, id, tenant, data string) {
	t.Helper()
	mustExec(t, db, `INSERT INTO usages(id, tenants_id, data) VALUES(?,?,?)`, id, tenant, data)
}

func seedOffering(t *testing.T, db *store.Store, id, tenant, data string) {
	t.Helper()
	mustExec(t, db, `INSERT INTO offering_items(id, tenants_id, data) VALUES(?,?,?)`, id, tenant, data)
}

func TestReviewReconcileKeepsEditionIdentity(t *testing.T) {
	db := newNovelTestStore(t)
	seedTenant(t, db, "t1", "Acme", "customer", "")
	seedUsage(t, db, "u1", "t1", `{"name":"vms","application_id":"app1","edition":"advanced","value":7}`)
	seedOffering(t, db, "o1", "t1", `{"name":"vms","application_id":"app1","edition":"standard","status":1}`)
	rows, err := computeReconcile(db, "t1")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.HasUsage && r.Finding == "ok" {
			t.Fatalf("advanced-edition usage reconciled against a standard-edition license: %+v", rows)
		}
	}
}

func TestReviewSnapshotKeepsEveryUsageIdentity(t *testing.T) {
	db := newNovelTestStore(t)
	seedTenant(t, db, "t1", "Acme", "customer", "")
	seedUsage(t, db, "u1", "t1", `{"name":"vms","application_id":"app1","edition":"standard","value":3}`)
	seedUsage(t, db, "u2", "t1", `{"name":"vms","application_id":"app1","edition":"advanced","value":5}`)
	for _, d := range []string{"2026-10-23", "2026-10-24"} {
		if _, err := captureUsageSnapshot(db, d); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := computeDrift(db, "2026-10-23", "2026-10-24", "")
	if err != nil {
		t.Fatal(err)
	}
	total := 0.0
	for _, r := range rows {
		if r.ToValue != nil {
			total += *r.ToValue
		}
	}
	if len(rows) != 2 || total != 8 {
		t.Fatalf("drift collapsed distinct usage identities: %+v", rows)
	}
}

func TestReviewInactiveOfferingsAreNotLicenses(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	db := newNovelTestStore(t)
	seedTenant(t, db, "t1", "Acme", "customer", "")
	seedOffering(t, db, "o1", "t1", `{"name":"vms","status":0}`)
	seedUsage(t, db, "u1", "t1", `{"name":"vms","value":7}`)
	inv, err := computeOfferingInventory(db)
	if err != nil || len(inv) != 0 {
		t.Fatalf("inventory counted a disabled offering: %+v, %v", inv, err)
	}
	rec, err := computeReconcile(db, "t1")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rec {
		if r.Finding == "ok" {
			t.Fatalf("usage on a disabled offering reconciled ok: %+v", rec)
		}
	}
	audit, err := computeTenantAudit(db, "customer", true)
	if err != nil || len(audit) != 1 || audit[0].OfferingItems != 0 {
		t.Fatalf("audit counted a disabled offering: %+v, %v", audit, err)
	}
	cov, err := computeCoverage(db, 7, false, now)
	if err != nil || len(cov) != 0 {
		t.Fatalf("coverage treated a disabled offering as paying: %+v, %v", cov, err)
	}
}

// --- version-order --------------------------------------------------------------

func TestReviewVersionsCompareNumerically(t *testing.T) {
	db := newNovelTestStore(t)
	seedTenant(t, db, "t1", "Acme", "customer", "")
	mustExec(t, db, `INSERT INTO agent_manager(id, data, tenant_id, hostname, version) VALUES('a1','{}','t1','h1','21.9.1')`)
	mustExec(t, db, `INSERT INTO agent_manager(id, data, tenant_id, hostname, version) VALUES('a2','{}','t1','h2','21.10.3')`)
	rep, err := computeCompliance(db, "21.10.0")
	if err != nil || len(rep.NonCompliant) != 1 || rep.NonCompliant[0].Hostname != "h1" {
		t.Fatalf("compliance = %+v, %v; want only h1 (21.9.1) behind 21.10.0", rep, err)
	}
}

// --- tenant-coverage --------------------------------------------------------------

func TestReviewHealthReportsTenantsWithoutTasks(t *testing.T) {
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	db := newNovelTestStore(t)
	seedTenant(t, db, "t1", "Acme", "customer", "")
	seedTenant(t, db, "t2", "Globex", "customer", "")
	seedTask(t, db, "k1", "t1", "backup", "completed", "ok", "2026-10-24T11:00:00Z")
	rows, err := computeHealth(db, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.TenantID == "t2" {
			if !r.Stale || r.Total != 0 {
				t.Fatalf("t2 = %+v; want total=0 stale=true", r)
			}
			return
		}
	}
	t.Fatalf("a tenant with no tasks vanished from health: %+v", rows)
}

// --- snapshot-replacement / drift-null / drift-date ------------------------------

func TestReviewSnapshotRecaptureReplacesTheDate(t *testing.T) {
	db := newNovelTestStore(t)
	seedTenant(t, db, "t1", "Acme", "customer", "")
	seedUsage(t, db, "u1", "t1", `{"name":"vms","value":10}`)
	for _, d := range []string{"2026-10-23", "2026-10-24"} {
		if _, err := captureUsageSnapshot(db, d); err != nil {
			t.Fatal(err)
		}
	}
	mustExec(t, db, `DELETE FROM usages`)
	if _, err := captureUsageSnapshot(db, "2026-10-24"); err != nil {
		t.Fatal(err)
	}
	rows, err := computeDrift(db, "2026-10-23", "2026-10-24", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ToValue != nil {
		t.Fatalf("recapture kept a removed metric: %+v", rows)
	}
}

func TestReviewDriftKeepsUnknownValuesUnknown(t *testing.T) {
	db := newNovelTestStore(t)
	seedTenant(t, db, "t1", "Acme", "customer", "")
	seedUsage(t, db, "u1", "t1", `{"name":"vms"}`)
	for _, d := range []string{"2026-10-23", "2026-10-24"} {
		if _, err := captureUsageSnapshot(db, d); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := computeDrift(db, "2026-10-23", "2026-10-24", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].FromValue != nil || rows[0].ToValue != nil || rows[0].Delta != nil {
		t.Fatalf("an unmeasured usage became a measured zero: %+v", rows)
	}
}

func TestReviewDriftRequiresBothSnapshotDates(t *testing.T) {
	db := newNovelTestStore(t)
	seedTenant(t, db, "t1", "Acme", "customer", "")
	seedUsage(t, db, "u1", "t1", `{"name":"vms","value":1}`)
	for _, d := range []string{"2026-10-23", "2026-10-24"} {
		if _, err := captureUsageSnapshot(db, d); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := computeDrift(db, "2026-10-01", "2026-10-02", ""); err == nil {
		t.Fatal("drift between dates with no capture succeeded")
	}
	mustExec(t, db, `DELETE FROM usages`)
	if _, err := captureUsageSnapshot(db, "2026-10-25"); err != nil {
		t.Fatal(err)
	}
	if _, err := computeDrift(db, "2026-10-24", "2026-10-25", ""); err != nil {
		t.Fatalf("an intentionally empty capture is a real snapshot: %v", err)
	}
}
