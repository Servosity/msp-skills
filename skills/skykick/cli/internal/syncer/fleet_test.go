// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored tests for the fleet-sync orchestration.
package syncer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"skykick-pp-cli/internal/client"
	"skykick-pp-cli/internal/config"
	"skykick-pp-cli/internal/store"
)

func testClient(t *testing.T, baseURL string) *client.Client {
	t.Helper()
	cfg := &config.Config{
		BaseURL:     baseURL,
		AccessToken: "test-token",
		TokenExpiry: time.Now().Add(1 * time.Hour),
	}
	return client.New(cfg, 10*time.Second, 100)
}

func testStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("opening store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// fixture server: 2 subscriptions; sub-2's mailboxes facet 500s.
func fixtureServer(t *testing.T, failures *int64) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/Backup", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"Id":"sub-1","CompanyName":"Contoso"},{"Id":"sub-2","CompanyName":"Fabrikam"}]`))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case strings.HasSuffix(p, "/subscriptionsettings"):
			w.Write([]byte(`{"CustomerInformation":{"CompanyName":"Contoso"},"ExchangeBackupEnabled":true,"SharePointBackupEnabled":false}`))
		case strings.HasSuffix(p, "/retentionperiod"):
			w.Write([]byte(`{"ExchangeRentionPeriodInDays":365}`))
		case strings.HasSuffix(p, "/autodiscover"):
			w.Write([]byte(`{"exchangeAutoDiscoverEnabled":true}`))
		case strings.HasSuffix(p, "/lastsnapshotstats"):
			w.Write([]byte(`[{"SmtpAddress":"a@x.com","LastSnapshotDate":"2026-06-01T08:00:00Z"}]`))
		case strings.HasSuffix(p, "/mailboxes"):
			if strings.Contains(p, "sub-2") {
				atomic.AddInt64(failures, 1)
				http.Error(w, `{"Message":"boom"}`, http.StatusInternalServerError)
				return
			}
			w.Write([]byte(`{"IndividualMailboxes":[{"MailboxId":"m1","SmtpAddress":"a@x.com","IsEnabled":true}]}`))
		case strings.HasSuffix(p, "/sites"):
			w.Write([]byte(`[{"Url":"https://x/sites/hq","BackupEnabled":"Enabled"}]`))
		case strings.HasPrefix(p, "/Alerts/"):
			w.Write([]byte(`[{"Id":"al-1","Severity":"Critical","Description":"Backup failed"}]`))
		default:
			http.NotFound(w, r)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestRunFleetSync_HappyPathWithPartialFailure(t *testing.T) {
	var upstreamFailures int64
	srv := fixtureServer(t, &upstreamFailures)
	c := testClient(t, srv.URL)
	db := testStore(t)

	res, err := RunFleetSync(context.Background(), c, db, Options{Workers: 2})
	if err != nil {
		t.Fatalf("RunFleetSync: %v", err)
	}
	if res.Subscriptions != 2 {
		t.Errorf("subscriptions synced=%d want 2 (failed facet must not drop the subscription)", res.Subscriptions)
	}
	if res.SubscriptionsSeen != 2 {
		t.Errorf("seen=%d want 2", res.SubscriptionsSeen)
	}
	// sub-2's mailboxes facet failed -> exactly that fetch recorded.
	if len(res.FetchFailures) == 0 {
		t.Fatalf("expected fetch_failures for sub-2 mailboxes, got none")
	}
	found := false
	for _, f := range res.FetchFailures {
		if f.SubscriptionID == "sub-2" && f.Facet == "mailboxes" {
			found = true
		}
		if f.SubscriptionID == "sub-1" {
			t.Errorf("sub-1 should have no failures: %+v", f)
		}
	}
	if !found {
		t.Errorf("missing sub-2/mailboxes failure: %+v", res.FetchFailures)
	}
	// Mailbox count covers ONLY successful fetches (no phantom rows).
	if res.Mailboxes != 1 {
		t.Errorf("mailboxes=%d want 1", res.Mailboxes)
	}
	if res.Alerts != 2 || res.Sites != 2 || res.SnapshotRows != 2 {
		t.Errorf("counts alerts=%d sites=%d snaps=%d want 2/2/2", res.Alerts, res.Sites, res.SnapshotRows)
	}

	// Round-trip: load the state back and verify extraction survived storage.
	state, err := db.LoadFleetState(context.Background(), res.RunID)
	if err != nil {
		t.Fatalf("LoadFleetState: %v", err)
	}
	if len(state.Subscriptions) != 2 || len(state.Settings) != 2 || len(state.Alerts) != 2 {
		t.Errorf("loaded subs=%d settings=%d alerts=%d", len(state.Subscriptions), len(state.Settings), len(state.Alerts))
	}
	for _, s := range state.Settings {
		if s.ExchangeEnabled == nil || !*s.ExchangeEnabled {
			t.Errorf("exchange_enabled lost in round-trip: %+v", s)
		}
		if s.SharePointEnabled == nil || *s.SharePointEnabled {
			t.Errorf("sharepoint_enabled lost in round-trip: %+v", s)
		}
	}
	for _, r := range state.Retention {
		if r.ExchangeDays == nil || *r.ExchangeDays != 365 {
			t.Errorf("retention lost in round-trip: %+v", r)
		}
	}
	for _, st := range state.Stats {
		if st.LastSnapshot == nil {
			t.Errorf("snapshot time lost in round-trip: %+v", st)
		}
	}
}

func TestRunFleetSync_LimitAndRunHistory(t *testing.T) {
	var n int64
	srv := fixtureServer(t, &n)
	c := testClient(t, srv.URL)
	db := testStore(t)

	res1, err := RunFleetSync(context.Background(), c, db, Options{Limit: 1, Workers: 1})
	if err != nil {
		t.Fatalf("run1: %v", err)
	}
	if res1.Subscriptions != 1 || res1.SubscriptionsSeen != 2 {
		t.Errorf("limit not applied: synced=%d seen=%d", res1.Subscriptions, res1.SubscriptionsSeen)
	}
	res2, err := RunFleetSync(context.Background(), c, db, Options{Limit: 1, Workers: 1})
	if err != nil {
		t.Fatalf("run2: %v", err)
	}
	runs, err := db.LatestFleetRuns(context.Background(), 10)
	if err != nil {
		t.Fatalf("LatestFleetRuns: %v", err)
	}
	if len(runs) != 2 || runs[0].ID != res2.RunID || runs[1].ID != res1.RunID {
		t.Errorf("run history wrong: %+v", runs)
	}
	// A --limit'd run must self-identify as partial so drift can suppress
	// added/removed-subscription false positives across the boundary.
	for _, r := range runs {
		if !r.Partial() {
			t.Errorf("run %d synced 1/2 subscriptions but Partial()=false", r.ID)
		}
	}
}

func TestRunFleetSync_FullRunNotPartial(t *testing.T) {
	var n int64
	srv := fixtureServer(t, &n)
	c := testClient(t, srv.URL)
	db := testStore(t)
	if _, err := RunFleetSync(context.Background(), c, db, Options{Workers: 1}); err != nil {
		t.Fatalf("sync: %v", err)
	}
	runs, err := db.LatestFleetRuns(context.Background(), 1)
	if err != nil || len(runs) != 1 {
		t.Fatalf("runs: %v %v", runs, err)
	}
	if runs[0].Partial() {
		t.Errorf("unlimited run should not be partial: %+v", runs[0])
	}
}

func TestRunFleetSync_SkipFacet(t *testing.T) {
	var n int64
	srv := fixtureServer(t, &n)
	c := testClient(t, srv.URL)
	db := testStore(t)

	res, err := RunFleetSync(context.Background(), c, db, Options{Workers: 1, Skip: map[string]bool{"alerts": true}})
	if err != nil {
		t.Fatalf("RunFleetSync: %v", err)
	}
	if res.Alerts != 0 {
		t.Errorf("alerts=%d want 0 when skipped", res.Alerts)
	}
	for _, f := range res.FetchFailures {
		if f.Facet == "alerts" {
			t.Errorf("skipped facet must not record failures: %+v", f)
		}
	}
}

func TestDogfoodCurtailed(t *testing.T) {
	cases := []struct {
		name      string
		dogfood   bool
		requested int
		want      int
	}{
		{"normal env passthrough", false, 0, 0},
		{"normal env keeps explicit", false, 10, 10},
		{"dogfood caps unlimited", true, 0, 2},
		{"dogfood caps large", true, 50, 2},
		{"dogfood keeps small", true, 1, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.dogfood {
				t.Setenv("PRINTING_PRESS_DOGFOOD", "1")
			} else {
				t.Setenv("PRINTING_PRESS_DOGFOOD", "")
			}
			if got := DogfoodCurtailed(tc.requested); got != tc.want {
				t.Errorf("DogfoodCurtailed(%d)=%d want %d", tc.requested, got, tc.want)
			}
		})
	}
}
