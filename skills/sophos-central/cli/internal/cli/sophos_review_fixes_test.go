// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"sophos-central-pp-cli/internal/client"
	"sophos-central-pp-cli/internal/cliutil/testenv"
	"sophos-central-pp-cli/internal/config"
	"sophos-central-pp-cli/internal/store"
)

// mockClient is a generated client pointed at an httptest server (no routing
// transport: the host is not a Sophos host).
func mockClient(t *testing.T, h http.HandlerFunc) *client.Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	cfg := &config.Config{BaseURL: srv.URL, AccessToken: "tok", TokenExpiry: time.Now().Add(time.Hour), Path: filepath.Join(t.TempDir(), "config.toml")}
	c := client.New(cfg, 5*time.Second, 0)
	c.NoCache = true
	return c
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// A nextKey that repeats means the listing never ended: it must be an error,
// so fleet refresh never reconciles (deletes rows) from a partial listing.
func TestFetchKeyPagedRepeatedKeyIsAnError(t *testing.T) {
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"items": []any{map[string]string{"id": "x"}}, "pages": map[string]string{"nextKey": "same"}})
	})
	items, err := fetchKeyPaged(context.Background(), c, "/endpoint/v1/endpoints", nil, "T1", 0)
	if err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("repeated key must fail, got items=%d err=%v", len(items), err)
	}
}

func TestTenantCredentialRejectsOtherTenantSelector(t *testing.T) {
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"id": "SELF", "idType": "tenant", "apiHosts": map[string]string{"dataRegion": "https://api-us01.central.sophos.com"}})
	})
	if _, _, err := fleetTenants(context.Background(), c, "OTHER", false); err == nil {
		t.Fatal("a tenant credential must not act on a different selected tenant")
	}
	got, _, err := fleetTenants(context.Background(), c, "SELF", false)
	if err != nil || len(got) != 1 || got[0].ID != "SELF" {
		t.Fatalf("selecting its own tenant should work: %v %v", got, err)
	}
	if _, _, err := fleetTenantsFiltered(context.Background(), c, map[string]bool{"SELF": true, "OTHER": true}, false); err == nil {
		t.Fatal("a multi-tenant selector with an unreachable ID must fail")
	}
}

func TestRevalidateTargetRefusesStaleMatch(t *testing.T) {
	target := containTarget{Target: "WS-FIN-07", TenantID: "T1", EndpointID: "E1"}
	ok := map[string]any{"id": "E1", "hostname": "ws-fin-07", "tenant": map[string]any{"id": "T1"}}
	if err := revalidateTarget(target, ok); err != nil {
		t.Fatalf("matching endpoint refused: %v", err)
	}
	renamed := map[string]any{"id": "E1", "hostname": "WS-HR-02", "tenant": map[string]any{"id": "T1"}}
	if err := revalidateTarget(target, renamed); err == nil {
		t.Fatal("an endpoint renamed since the index was built must not be contained")
	}
	moved := map[string]any{"id": "E1", "hostname": "WS-FIN-07", "tenant": map[string]any{"id": "T2"}}
	if err := revalidateTarget(target, moved); err == nil {
		t.Fatal("an endpoint in a different tenant must not be contained")
	}
	ipTarget := containTarget{Target: "10.0.0.5", TenantID: "T1", EndpointID: "E1"}
	readdressed := map[string]any{"id": "E1", "hostname": "WS-FIN-07", "ipv4Addresses": []any{"10.0.0.9"}}
	if err := revalidateTarget(ipTarget, readdressed); err == nil {
		t.Fatal("an endpoint that no longer holds the IP must not be contained")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func siemStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "siem.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// If the output cannot be written, the cursor must not advance.
func TestSiemWriteFailureKeepsCursor(t *testing.T) {
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"has_more": false, "next_cursor": "c2", "items": []any{map[string]string{"id": "ev1"}}})
	})
	db := siemStore(t)
	_, err := pullSiem(context.Background(), c, db, failingWriter{}, fleetTenant{ID: "T1"}, "events", 1000, 0, time.Now().UTC())
	var outErr errSiemOutput
	if !errors.As(err, &outErr) {
		t.Fatalf("want an output error, got %v", err)
	}
	if raw, _ := db.Get(fleetSiemCursorType, "T1:events"); len(raw) != 0 {
		t.Fatalf("cursor advanced despite the failed write: %s", raw)
	}
}

func TestSiemCursorLoopsAndGapsAreErrors(t *testing.T) {
	for name, body := range map[string]map[string]any{
		"repeated cursor":    {"has_more": true, "next_cursor": "loop", "items": []any{}},
		"has_more no cursor": {"has_more": true, "next_cursor": "", "items": []any{}},
	} {
		t.Run(name, func(t *testing.T) {
			c := mockClient(t, func(w http.ResponseWriter, r *http.Request) { writeJSON(w, body) })
			var out strings.Builder
			_, err := pullSiem(context.Background(), c, siemStore(t), &out, fleetTenant{ID: "T1"}, "events", 1000, 0, time.Now().UTC())
			if err == nil || !strings.Contains(err.Error(), "incomplete") {
				t.Fatalf("want an incomplete-collection error, got %v", err)
			}
		})
	}
	// A page cap with more data waiting reports "capped", not success-as-drained.
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"has_more": true, "next_cursor": "c-" + r.URL.Query().Get("cursor"), "items": []any{}})
	})
	var out strings.Builder
	if _, err := pullSiem(context.Background(), c, siemStore(t), &out, fleetTenant{ID: "T1"}, "events", 1000, 1, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"status":"capped"`) {
		t.Fatalf("page cap must be reported as capped:\n%s", out.String())
	}
}

func TestIdentityCacheKeyedPerCredential(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("SOPHOS_CENTRAL_CLIENT_ID", "")
	mk := func(cfg *config.Config) string { return sophosContextPath(client.New(cfg, time.Second, 0)) }
	a := mk(&config.Config{AccessToken: "token-a"})
	b := mk(&config.Config{AccessToken: "token-b"})
	if a == "" || b == "" || a == b {
		t.Fatalf("token-only credentials must get distinct caches: %q %q", a, b)
	}
	if p := mk(&config.Config{}); p != "" {
		t.Fatalf("no credential at all must disable the disk cache, got %q", p)
	}
	if mk(&config.Config{ClientID: "x", ClientSecret: "s"}) == mk(&config.Config{ClientID: "y", ClientSecret: "s"}) {
		t.Fatal("different client IDs must not share a cache")
	}
}

func TestLedgerMergeKeepsOlderFailures(t *testing.T) {
	prev := fleetLedgerRow{ID: "T1", Resources: map[string]fleetOutcome{"endpoints": {Status: "error"}}}
	cur := fleetLedgerRow{ID: "T1", Resources: map[string]fleetOutcome{"health": {Status: "ok"}}}
	merged := mergeLedgerRow(prev, cur)
	if merged.Status != "partial" || merged.Resources["endpoints"].Status != "error" {
		t.Fatalf("a health-only refresh must not hide the endpoint failure: %+v", merged)
	}
	if ledgerStatus(map[string]fleetOutcome{"a": {Status: "forbidden"}, "b": {Status: "forbidden"}}) != "forbidden" {
		t.Fatal("all-forbidden should be forbidden")
	}
}

func TestRevenueRanksWithinOneCurrency(t *testing.T) {
	lines := []billingLine{
		{AccountID: "A", BillableQuantity: 1, UnitPrice: "100", UnitPriceCurrency: "USD"},
		{AccountID: "B", BillableQuantity: 1, UnitPrice: "50", UnitPriceCurrency: "USD"},
		{AccountID: "C", BillableQuantity: 1, UnitPrice: "90000", UnitPriceCurrency: "JPY"},
	}
	v := buildBilling("2026-09", lines, nil, nil)
	if v.RankedBy != "JPY" {
		// JPY has the largest raw total; ranking currency must be explicit.
		t.Fatalf("ranked_by_currency = %q", v.RankedBy)
	}
	lines[2].UnitPrice = "10"
	v = buildBilling("2026-09", lines, nil, nil)
	if v.RankedBy != "USD" || v.Items[0].AccountID != "A" || v.Items[1].AccountID != "B" || v.Items[2].AccountID != "C" {
		t.Fatalf("USD customers should rank by USD, others after: %+v", v.Items)
	}
}

func TestTamperReadability(t *testing.T) {
	r, u := tamperReadability([]tamperTenant{{Status: "forbidden"}, {Status: "error"}})
	if r != 0 || u != 1 {
		t.Fatalf("readable=%d unexpected=%d", r, u)
	}
	r, _ = tamperReadability([]tamperTenant{{Status: "global_off"}})
	if r != 1 {
		t.Fatal("global_off tenants were read")
	}
}

func TestBulkIsolationErrorsAreFailures(t *testing.T) {
	if msg := bulkIsolationErrors(json.RawMessage(`{"items":[{"id":"E1"}],"errors":[]}`), []string{"E1"}); msg != "" {
		t.Fatalf("clean receipt reported %q", msg)
	}
	msg := bulkIsolationErrors(json.RawMessage(`{"items":[],"errors":[{"id":"E2","errorCode":"notFound","errorMessage":"Endpoint not found"}]}`), []string{"E2"})
	if !strings.Contains(msg, "E2") || !strings.Contains(msg, "notFound") {
		t.Fatalf("rejected endpoint not reported: %q", msg)
	}
	if msg := bulkIsolationErrors(json.RawMessage(`{"items":[{"id":"E1"}]}`), []string{"E1", "E3"}); !strings.Contains(msg, "E3") {
		t.Fatalf("an endpoint missing from the receipt must be reported: %q", msg)
	}
	if msg := bulkIsolationErrors(json.RawMessage(`{"errors":[{"id":"E1"},42]}`), []string{"E1"}); msg == "" {
		t.Fatal("an undecodable receipt must not count as success")
	}
}

func TestCredentialKeyFollowsEffectivePrecedence(t *testing.T) {
	t.Setenv("SOPHOS_CENTRAL_CLIENT_ID", "")
	t.Setenv("SOPHOS_CENTRAL_CLIENT_SECRET", "")
	// A full pair mints and the client clears the static header on mint;
	// pinClientPairAuth makes that hold from the first request.
	if k := sophosCredentialKey(&config.Config{AuthHeaderVal: "Bearer x", ClientID: "id", ClientSecret: "s"}); k != "client|id" {
		t.Fatalf("a full client pair must win over a static header, got %q", k)
	}
	if k := sophosCredentialKey(&config.Config{AuthHeaderVal: "Bearer x", AccessToken: "t"}); k != "header|Bearer x" {
		t.Fatalf("without a pair the static header wins over a saved token, got %q", k)
	}
	// Provider-specific central_* fields never mint: the saved token is sent.
	if k := sophosCredentialKey(&config.Config{SophosCentralClientId: "cid", SophosCentralClientSecret: "s", AccessToken: "t"}); k != "token|t" {
		t.Fatalf("central_* fields are not a client pair, got %q", k)
	}
	if k := sophosCredentialKey(&config.Config{ClientID: "id", ClientSecret: "s", AccessToken: "t"}); k != "client|id" {
		t.Fatalf("client id+secret mint the token, got %q", k)
	}
	// A client ID without a secret cannot mint: the stored token is what is sent.
	if k := sophosCredentialKey(&config.Config{ClientID: "id", AccessToken: "t"}); k != "token|t" {
		t.Fatalf("id without secret must key on the token, got %q", k)
	}
	if k := sophosCredentialKey(&config.Config{}); k != "" {
		t.Fatalf("no credential must give no key, got %q", k)
	}
}

func TestSiemWatermarkUsesEventTime(t *testing.T) {
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	// Saved 1h ago, but the stream was capped while still 30h behind.
	behind := &siemCursor{Cursor: "c", SavedAt: now.Add(-time.Hour), EventAt: now.Add(-30 * time.Hour)}
	if p, stale := siemPageParams(behind, 1000, now); !stale || p["cursor"] != "" {
		t.Fatalf("a cursor 30h behind in event time is out of the window: %v stale=%v", p, stale)
	}
	caughtUp := &siemCursor{Cursor: "c", SavedAt: now.Add(-time.Hour), EventAt: now.Add(-time.Hour)}
	if p, stale := siemPageParams(caughtUp, 1000, now); stale || p["cursor"] != "c" {
		t.Fatalf("a caught-up cursor must be reused: %v stale=%v", p, stale)
	}
	items := []json.RawMessage{json.RawMessage(`{"created_at":"2026-10-12T01:00:00Z"}`), json.RawMessage(`{"created_at":"2026-10-12T02:00:00Z"}`)}
	if got := lastSiemEventTime(items, now); !got.Equal(time.Date(2026, 10, 12, 2, 0, 0, 0, time.UTC)) {
		t.Fatalf("last event time = %v", got)
	}
}

// fleetFake serves two term tenants whose endpoints differ, so a per-tenant
// reconcile can be observed. drop removes T1's second endpoint.
func fleetFake(t *testing.T, drop *bool, healthFails bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenant := r.Header.Get("X-Tenant-ID")
		switch {
		case r.URL.Path == "/token":
			writeJSON(w, map[string]any{"access_token": "tok", "token_type": "bearer", "expires_in": 3600})
		case r.URL.Path == "/whoami/v1":
			writeJSON(w, map[string]any{"id": "P1", "idType": "partner"})
		case r.URL.Path == "/partner/v1/tenants":
			writeJSON(w, map[string]any{"items": []map[string]any{
				{"id": "T1", "name": "One", "billingType": "term", "apiHost": "https://api-us01.central.sophos.com"},
				{"id": "T2", "name": "Two", "billingType": "term", "apiHost": "https://api-us01.central.sophos.com"},
			}, "pages": map[string]int{"current": 1, "total": 1}})
		case r.URL.Path == "/endpoint/v1/endpoints":
			ep := func(n string) map[string]any {
				return map[string]any{"id": n + "-0000-4000-a000-000000000000", "hostname": "h" + n, "tenant": map[string]any{"id": tenant}}
			}
			items := []any{ep(map[string]string{"T1": "11111111", "T2": "22222222"}[tenant])}
			if tenant == "T1" && !*drop {
				items = append(items, ep("13131313"))
			}
			writeJSON(w, map[string]any{"items": items, "pages": map[string]any{}})
		case r.URL.Path == "/licenses/v1/licenses":
			writeJSON(w, map[string]any{"licenses": []any{}})
		case r.URL.Path == "/account-health-check/v1/health-check" && healthFails:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "badRequest"})
		default:
			writeJSON(w, map[string]any{"items": []any{}, "pages": map[string]any{}})
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func fleetEnv(t *testing.T, srv *httptest.Server) {
	testenv.Isolate(t)
	t.Setenv("SOPHOS_CENTRAL_BASE_URL", srv.URL)
	t.Setenv("SOPHOS_CENTRAL_TOKEN_URL", srv.URL+"/token")
	t.Setenv("SOPHOS_CENTRAL_CLIENT_ID", "cid")
	t.Setenv("SOPHOS_CENTRAL_CLIENT_SECRET", "secret")
	t.Setenv(sophosTenantEnv, "")
}

func endpointTenants(t *testing.T, dbPath string) map[string]int {
	t.Helper()
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	out := map[string]int{}
	_ = db.ListScan("endpoints", func(_ string, raw json.RawMessage) bool {
		var e indexedEndpoint
		_ = json.Unmarshal(raw, &e)
		out[e.Tenant.ID]++
		return true
	})
	return out
}

// Generic `sync --full` for one tenant must prune only that tenant's rows,
// never other tenants' fleet data.
func TestFullSyncPrunesOnlySelectedTenant(t *testing.T) {
	drop := false
	srv := fleetFake(t, &drop, false)
	fleetEnv(t, srv)
	dbPath := filepath.Join(t.TempDir(), "fleet.db")
	if out, err := runRoot(t, "fleet", "refresh", "--resources", "tenants,endpoints", "--db", dbPath, "--json"); err != nil {
		t.Fatalf("fleet refresh: %v\n%s", err, out)
	}
	if got := endpointTenants(t, dbPath); got["T1"] != 2 || got["T2"] != 1 {
		t.Fatalf("after refresh: %v", got)
	}
	drop = true
	if out, err := runRoot(t, "sync", "--full", "--resources", "endpoints", "--tenant", "T1", "--db", dbPath, "--json"); err != nil {
		t.Fatalf("sync --full: %v\n%s", err, out)
	}
	got := endpointTenants(t, dbPath)
	if got["T2"] != 1 {
		t.Fatalf("sync --full for T1 deleted another tenant's rows: %v", got)
	}
	if got["T1"] != 1 {
		t.Fatalf("T1's removed endpoint should have been pruned: %v", got)
	}
}

// If this run refreshed nothing (every tenant's only requested resource
// failed), refresh must fail even when older ledger outcomes were good.
func TestRefreshFailsWhenCurrentRunRefreshedNothing(t *testing.T) {
	drop := false
	srv := fleetFake(t, &drop, true)
	fleetEnv(t, srv)
	dbPath := filepath.Join(t.TempDir(), "fleet.db")
	if out, err := runRoot(t, "fleet", "refresh", "--resources", "endpoints", "--db", dbPath, "--json"); err != nil {
		t.Fatalf("endpoint refresh: %v\n%s", err, out)
	}
	if _, err := runRoot(t, "fleet", "refresh", "--resources", "health", "--db", dbPath, "--json"); err == nil {
		t.Fatal("a health-only refresh that failed for every tenant must not exit 0")
	}
}

// whoamiByToken answers whoami according to the bearer token sent.
type whoamiByToken struct{ seen []*http.Request }

func (f *whoamiByToken) RoundTrip(req *http.Request) (*http.Response, error) {
	f.seen = append(f.seen, req)
	body := `{}`
	if req.URL.Path == "/whoami/v1" {
		switch req.Header.Get("Authorization") {
		case "Bearer tok-partner":
			body = `{"id":"P-A","idType":"partner"}`
		case "Bearer tok-tenant":
			body = `{"id":"T-B","idType":"tenant","apiHosts":{"dataRegion":"https://api-eu01.central.sophos.com"}}`
		}
	}
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
}

// A cached identity is only reused for the credential that fetched it.
func TestIdentityFollowsTheCredentialActuallySent(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv(sophosTenantEnv, "")
	sophosTenantFlag = ""
	cfg := &config.Config{BaseURL: "https://api.central.sophos.com", AuthHeaderVal: "Bearer tok-partner", Path: filepath.Join(t.TempDir(), "c.toml")}
	c := client.New(cfg, 5*time.Second, 0)
	c.NoCache = true
	fake := &whoamiByToken{}
	c.HTTPClient.Transport = fake
	if err := installSophosRouting(c); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), "/partner/v1/tenants", nil); err != nil {
		t.Fatal(err)
	}
	// Switch credentials on the same router: the partner identity must not
	// be reused for the tenant credential.
	cfg.AuthHeaderVal = "Bearer tok-tenant"
	if _, err := c.Get(context.Background(), "/endpoint/v1/endpoints", nil); err != nil {
		t.Fatalf("tenant credential should route to its own tenant: %v", err)
	}
	last := fake.seen[len(fake.seen)-1]
	if last.Header.Get(headerTenantID) != "T-B" || last.URL.Host != "api-eu01.central.sophos.com" {
		t.Fatalf("request used the wrong identity: tenant=%q host=%s", last.Header.Get(headerTenantID), last.URL.Host)
	}
}

func TestEmptyEnvelopeIsNotAnEmptyListing(t *testing.T) {
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) { writeJSON(w, map[string]any{}) })
	if _, err := fetchKeyPaged(context.Background(), c, "/endpoint/v1/endpoints", nil, "T1", 0); err == nil {
		t.Fatal("a body without items must not be treated as a complete empty listing")
	}
}

func TestStoreTenantRowsSkipsReconcileOnIDlessRows(t *testing.T) {
	db := siemStore(t)
	ep := func(id string) json.RawMessage {
		b, _ := json.Marshal(map[string]any{"id": id, "hostname": "h", "tenant": map[string]any{"id": "T1"}})
		return b
	}
	if _, err := storeTenantRows(db, "endpoints", "T1", []json.RawMessage{ep("11111111-1111-4111-a111-111111111111"), ep("22222222-2222-4222-a222-222222222222")}, true); err != nil {
		t.Fatal(err)
	}
	broken := []json.RawMessage{ep("11111111-1111-4111-a111-111111111111"), json.RawMessage(`{"hostname":"no-id","tenant":{"id":"T1"}}`)}
	if _, err := storeTenantRows(db, "endpoints", "T1", broken, true); err == nil {
		t.Fatal("an id-less row must stop the reconcile")
	}
	n := 0
	_ = db.ListScan("endpoints", func(string, json.RawMessage) bool { n++; return true })
	if n != 2 {
		t.Fatalf("reconcile ran on an incomplete seen set: %d rows left, want 2", n)
	}
}

func TestSiemCollectRejectsDeliverSink(t *testing.T) {
	testenv.Isolate(t)
	if _, err := runRoot(t, "fleet", "siem-collect", "--deliver", "file:/tmp/x.ndjson"); err == nil {
		t.Fatal("--deliver must be rejected: cursors advance before a sink is written")
	}
}

func TestFilteredFullSyncDoesNotPrune(t *testing.T) {
	drop := false
	srv := fleetFake(t, &drop, false)
	fleetEnv(t, srv)
	dbPath := filepath.Join(t.TempDir(), "fleet.db")
	if out, err := runRoot(t, "fleet", "refresh", "--resources", "tenants,endpoints", "--db", dbPath, "--json"); err != nil {
		t.Fatalf("fleet refresh: %v\n%s", err, out)
	}
	drop = true // the filtered call returns fewer rows; that is not a deletion
	if out, err := runRoot(t, "sync", "--full", "--resources", "endpoints", "--tenant", "T1", "--param", "type=server", "--db", dbPath, "--json"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if got := endpointTenants(t, dbPath); got["T1"] != 2 {
		t.Fatalf("a filtered full sync pruned rows: %v", got)
	}
}

func TestSiemMissingItemsKeepsCursor(t *testing.T) {
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"has_more": false, "next_cursor": "c2"})
	})
	db := siemStore(t)
	var out strings.Builder
	if _, err := pullSiem(context.Background(), c, db, &out, fleetTenant{ID: "T1"}, "events", 1000, 0, time.Now().UTC()); err == nil {
		t.Fatal("a page without items must be an error")
	}
	if raw, _ := db.Get(fleetSiemCursorType, "T1:events"); len(raw) != 0 {
		t.Fatalf("cursor advanced past a page with no items: %s", raw)
	}
}

// A per-call Authorization override must drive the identity used for that
// request, including the nested whoami lookup.
func TestPerCallAuthorizationDrivesIdentity(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv(sophosTenantEnv, "")
	sophosTenantFlag = ""
	cfg := &config.Config{BaseURL: "https://api.central.sophos.com", AuthHeaderVal: "Bearer tok-partner", Path: filepath.Join(t.TempDir(), "c.toml")}
	c := client.New(cfg, 5*time.Second, 0)
	c.NoCache = true
	fake := &whoamiByToken{}
	c.HTTPClient.Transport = fake
	if err := installSophosRouting(c); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetWithHeaders(context.Background(), "/endpoint/v1/endpoints", nil, map[string]string{"Authorization": "Bearer tok-tenant"}); err != nil {
		t.Fatalf("override credential is a tenant credential and should route: %v", err)
	}
	last := fake.seen[len(fake.seen)-1]
	if last.Header.Get("Authorization") != "Bearer tok-tenant" || last.Header.Get(headerTenantID) != "T-B" {
		t.Fatalf("auth=%q tenant=%q", last.Header.Get("Authorization"), last.Header.Get(headerTenantID))
	}
}

func TestConfiguredAuthorizationHeaderDisablesCache(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	cfg := &config.Config{BaseURL: "https://api.central.sophos.com", Headers: map[string]string{"authorization": "Bearer x"}}
	c := client.New(cfg, time.Second, 0)
	if err := installSophosRouting(c); err != nil {
		t.Fatal(err)
	}
	if !c.NoCache {
		t.Fatal("a configured Authorization header must disable the response cache")
	}
}

func TestTenantScopedResourcesReconcilePerTenant(t *testing.T) {
	for rt, mode := range flatReconcileModes {
		path, err := syncResourcePath(rt)
		if err != nil {
			continue
		}
		global := strings.HasPrefix(path, "/partner/") || strings.HasPrefix(path, "/organization/")
		if global && mode != "flat_global" {
			t.Errorf("%s (%s) should keep whole-table reconcile, got %s", rt, path, mode)
		}
		if !global && mode == "flat_global" {
			t.Errorf("%s (%s) is tenant-scoped but reconciles the whole table", rt, path)
		}
	}
}

// Generic sync must stop on a multi-key cursor cycle and on an undecodable
// item must not prune.
func TestGenericSyncCycleAndUndecodableItem(t *testing.T) {
	cycle := map[string]string{"": "A", "A": "B", "B": "A"}
	var mode string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			writeJSON(w, map[string]any{"access_token": "tok", "expires_in": 3600})
		case r.URL.Path == "/endpoint/v1/endpoints" && mode == "cycle":
			key := r.URL.Query().Get("pageFromKey")
			writeJSON(w, map[string]any{"items": []any{map[string]any{"id": "c" + key + "-0000-4000-a000-000000000000", "tenant": map[string]any{"id": "T1"}}}, "pages": map[string]any{"nextKey": cycle[key]}})
		case r.URL.Path == "/endpoint/v1/endpoints" && mode == "samepage":
			writeJSON(w, map[string]any{"items": []any{map[string]any{"id": "55555555-0000-4000-a000-000000000000", "tenant": map[string]any{"id": "T1"}}}, "pages": map[string]any{"nextKey": "A"}})
		case r.URL.Path == "/endpoint/v1/endpoints" && mode == "seed":
			writeJSON(w, map[string]any{"items": []any{
				map[string]any{"id": "11111111-0000-4000-a000-000000000000", "tenant": map[string]any{"id": "T1"}},
				map[string]any{"id": "22222222-0000-4000-a000-000000000000", "tenant": map[string]any{"id": "T1"}},
			}, "pages": map[string]any{}})
		case r.URL.Path == "/endpoint/v1/endpoints":
			writeJSON(w, map[string]any{"items": []any{map[string]any{"id": "11111111-0000-4000-a000-000000000000", "tenant": map[string]any{"id": "T1"}}, 42}, "pages": map[string]any{}})
		default:
			writeJSON(w, map[string]any{"items": []any{}})
		}
	}))
	t.Cleanup(srv.Close)
	fleetEnv(t, srv)
	dbPath := filepath.Join(t.TempDir(), "s.db")

	mode = "cycle"
	done := make(chan error, 1)
	go func() {
		_, err := runRoot(t, "sync", "--full", "--resources", "endpoints", "--tenant", "T1", "--db", dbPath, "--json")
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a sync that hit a cursor cycle must not report success")
		}
	case <-time.After(30 * time.Second):
		t.Fatal("generic sync did not stop on an A->B->A cursor cycle")
	}

	// The same page returned again under the same key is incomplete too.
	mode = "samepage"
	if _, err := runRoot(t, "sync", "--full", "--resources", "endpoints", "--tenant", "T1", "--db", filepath.Join(t.TempDir(), "p.db"), "--json"); err == nil {
		t.Fatal("a sync that saw the same page twice must not report success")
	}
	// The hand-fixed --all paginator must fail on a cursor cycle too.
	mode = "cycle"
	t.Setenv(sophosTenantEnv, "T1")
	if out, err := runRoot(t, "endpoints", "list", "--all", "--json"); err == nil {
		t.Fatalf("endpoints list --all must fail on a cursor cycle, got success:\n%.300s", out)
	}
	t.Setenv(sophosTenantEnv, "")

	dbPath = filepath.Join(t.TempDir(), "u.db")
	mode = "seed"
	if out, err := runRoot(t, "sync", "--full", "--resources", "endpoints", "--tenant", "T1", "--db", dbPath, "--json"); err != nil {
		t.Fatalf("seed sync: %v\n%s", err, out)
	}
	mode = "broken"
	_, _ = runRoot(t, "sync", "--full", "--resources", "endpoints", "--tenant", "T1", "--db", dbPath, "--json")
	if got := endpointTenants(t, dbPath); got["T1"] != 2 {
		t.Fatalf("a walk with an undecodable item pruned rows: %v", got)
	}
}

func TestTenantFlagReplacesCaseVariantHeader(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	sophosTenantFlag = "T1"
	t.Cleanup(func() { sophosTenantFlag = "" })
	cfg := &config.Config{BaseURL: "http://127.0.0.1:1", Headers: map[string]string{"x-tenant-id": "T2"}}
	c := client.New(cfg, time.Second, 0)
	if err := installSophosRouting(c); err != nil {
		t.Fatal(err)
	}
	n := 0
	for k, v := range cfg.Headers {
		if strings.EqualFold(k, headerTenantID) {
			n++
			if v != "T1" {
				t.Fatalf("selected tenant lost to a configured header: %s=%s", k, v)
			}
		}
	}
	if n != 1 {
		t.Fatalf("want exactly one tenant header, got %d: %v", n, cfg.Headers)
	}
}

// partialFake: two term tenants; T2 fails (HTTP 400) on the path in failPath,
// everything else succeeds. Endpoints for "endless" mode page forever with
// distinct keys.
func partialFake(t *testing.T, failPath string, endless bool, cycleEndpoints bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenant := r.Header.Get("X-Tenant-ID")
		switch {
		case r.URL.Path == "/token":
			writeJSON(w, map[string]any{"access_token": "tok", "expires_in": 3600})
		case r.URL.Path == "/whoami/v1":
			writeJSON(w, map[string]any{"id": "P1", "idType": "partner"})
		case r.URL.Path == "/partner/v1/tenants":
			writeJSON(w, map[string]any{"items": []map[string]any{
				{"id": "T1", "name": "One", "billingType": "term"}, {"id": "T2", "name": "Two", "billingType": "term"},
			}, "pages": map[string]int{"current": 1, "total": 1}})
		case r.URL.Path == failPath && (tenant == "T2" || failPath == "/common/v1/alerts"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "badRequest"})
		case r.URL.Path == "/endpoint/v1/endpoints" && endless:
			key := r.URL.Query().Get("pageFromKey")
			writeJSON(w, map[string]any{"items": []any{map[string]any{"id": "e" + key + "-0000-4000-a000-000000000000", "tenant": map[string]any{"id": tenant}}}, "pages": map[string]any{"nextKey": key + "x"}})
		case r.URL.Path == "/endpoint/v1/endpoints" && cycleEndpoints:
			next := map[string]string{"": "A", "A": "B", "B": "A"}[r.URL.Query().Get("pageFromKey")]
			writeJSON(w, map[string]any{"items": []any{map[string]any{"id": "c" + r.URL.Query().Get("pageFromKey") + "-0000-4000-a000-000000000000", "tenant": map[string]any{"id": tenant}}}, "pages": map[string]any{"nextKey": next}})
		case r.URL.Path == "/endpoint/v1/endpoints":
			writeJSON(w, map[string]any{"items": []any{map[string]any{"id": tenant + "000000-0000-4000-a000-000000000000", "hostname": "h" + tenant, "tenant": map[string]any{"id": tenant}, "tamperProtectionSupported": true, "tamperProtectionEnabled": false}}, "pages": map[string]any{}})
		case r.URL.Path == "/endpoint/v1/settings/tamper-protection":
			writeJSON(w, map[string]any{"enabled": true})
		case strings.HasPrefix(r.URL.Path, "/siem/v1/"):
			writeJSON(w, map[string]any{"has_more": false, "next_cursor": "c1", "items": []any{}})
		default:
			writeJSON(w, map[string]any{"items": []any{}, "pages": map[string]any{}})
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestFleetCommandsFailOnUnexpectedPartialErrors(t *testing.T) {
	srv := partialFake(t, "/common/v1/alerts", false, false)
	fleetEnv(t, srv)
	db := filepath.Join(t.TempDir(), "f.db")
	if _, err := runRoot(t, "fleet", "refresh", "--resources", "endpoints,alerts", "--db", db, "--json"); err == nil {
		t.Fatal("fleet refresh with a 400 on alerts must exit non-zero")
	}

	srv2 := partialFake(t, "/siem/v1/alerts", false, false)
	fleetEnv(t, srv2)
	if _, err := runRoot(t, "fleet", "siem-collect", "--stream", "both", "--db", filepath.Join(t.TempDir(), "s.db")); err == nil {
		t.Fatal("fleet siem-collect with one failing stream must exit non-zero")
	}

	srv3 := partialFake(t, "/endpoint/v1/settings/tamper-protection", false, false)
	fleetEnv(t, srv3)
	if _, err := runRoot(t, "fleet", "tamper", "--json"); err == nil {
		t.Fatal("fleet tamper with an unreadable tenant must exit non-zero")
	}
}

func TestMultiResourceSyncFailsWhenOneResourceCycles(t *testing.T) {
	srv := partialFake(t, "", false, true)
	fleetEnv(t, srv)
	if _, err := runRoot(t, "sync", "--full", "--resources", "endpoints,alerts", "--tenant", "T1", "--db", filepath.Join(t.TempDir(), "m.db"), "--json"); err == nil {
		t.Fatal("a cycling resource must fail the whole sync even when another resource succeeds")
	}
}

func TestAllStopsWithErrorAtPageCap(t *testing.T) {
	srv := partialFake(t, "", true, false)
	fleetEnv(t, srv)
	t.Setenv(sophosTenantEnv, "T1")
	_, err := runRoot(t, "endpoints", "list", "--all", "--json")
	if err == nil || !strings.Contains(err.Error(), "more available") {
		t.Fatalf("--all must fail at the page cap with more pages waiting, got %v", err)
	}
}

func TestLocalReadsScopedToSelectedTenant(t *testing.T) {
	drop := false
	srv := fleetFake(t, &drop, false)
	fleetEnv(t, srv)
	// Default store, as generated local reads use it.
	if out, err := runRoot(t, "fleet", "refresh", "--resources", "tenants,endpoints", "--json"); err != nil {
		t.Fatalf("fleet refresh: %v\n%s", err, out)
	}
	out, err := runRoot(t, "endpoints", "list", "--data-source", "local", "--tenant", "T2", "--json")
	if err != nil {
		t.Fatalf("local list: %v\n%s", err, out)
	}
	if !strings.Contains(out, "h22222222") || strings.Contains(out, "h11111111") || strings.Contains(out, "h13131313") {
		t.Fatalf("local read for T2 returned other tenants' rows:\n%s", out)
	}
	if _, err := runRoot(t, "endpoints", "list", "--data-source", "local", "--json"); err == nil {
		t.Fatal("a local read of mixed-tenant rows without --tenant must fail closed")
	}
}

func TestTenantListWithoutItemsFails(t *testing.T) {
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/whoami/v1" {
			writeJSON(w, map[string]any{"id": "P1", "idType": "partner"})
			return
		}
		writeJSON(w, map[string]any{})
	})
	if _, _, err := fleetTenants(context.Background(), c, "", false); err == nil {
		t.Fatal("a tenant list without items must fail, not return an empty fleet")
	}
}

func TestTamperPlanningRejectsUnreadableData(t *testing.T) {
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/endpoint/v1/settings/tamper-protection" {
			writeJSON(w, map[string]any{})
			return
		}
		writeJSON(w, map[string]any{"items": []any{}})
	})
	if tt := planTamperTenant(context.Background(), c, fleetTenant{ID: "T1"}, 0); tt.Status != "error" {
		t.Fatalf("missing enabled field must be an error, got %+v", tt)
	}
	c = mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/endpoint/v1/settings/tamper-protection" {
			writeJSON(w, map[string]any{"enabled": true})
			return
		}
		writeJSON(w, map[string]any{"items": []any{map[string]any{"id": "E1"}, map[string]any{"hostname": "no-id"}}, "pages": map[string]any{}})
	})
	if tt := planTamperTenant(context.Background(), c, fleetTenant{ID: "T1"}, 0); tt.Status != "error" || len(tt.Endpoints) != 0 {
		t.Fatalf("an id-less endpoint row must stop the tenant, got %+v", tt)
	}
}

func TestEmptyTenantSelectorIsRejected(t *testing.T) {
	drop := false
	srv := fleetFake(t, &drop, false)
	fleetEnv(t, srv)
	for _, sel := range []string{",", " ", ""} {
		out, err := runRoot(t, "fleet", "tamper", "--tenant", sel, "--yes", "--json")
		if err == nil || !strings.Contains(err.Error()+out, "names no tenant") {
			t.Fatalf("--tenant %q must be rejected, not widened to every tenant: err=%v\n%s", sel, err, out)
		}
	}
	t.Setenv(sophosTenantEnv, " ")
	out, err := runRoot(t, "fleet", "tamper", "--yes", "--json")
	if err == nil || !strings.Contains(err.Error()+out, "names no tenant") {
		t.Fatalf("a blank %s must be rejected, not widened to every tenant: err=%v\n%s", sophosTenantEnv, err, out)
	}
	// The flag value must not carry into the next invocation.
	t.Setenv(sophosTenantEnv, "")
	if _, err := runRoot(t, "fleet", "health", "--tenant", "T2", "--json"); err != nil {
		t.Fatalf("health --tenant T2: %v", err)
	}
	if sophosTenantFlag != "T2" {
		t.Fatalf("flag not applied: %q", sophosTenantFlag)
	}
	if _, err := runRoot(t, "fleet", "health", "--json"); err != nil {
		t.Fatalf("health: %v", err)
	}
	if sophosTenantFlag != "" {
		t.Fatalf("--tenant from the previous invocation leaked: %q", sophosTenantFlag)
	}
}

func TestTenantListEmptyPageBeforeTotalFails(t *testing.T) {
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/whoami/v1" {
			writeJSON(w, map[string]any{"id": "P1", "idType": "partner"})
			return
		}
		if r.URL.Query().Get("page") == "1" {
			writeJSON(w, map[string]any{"items": []any{map[string]any{"id": "T1"}}, "pages": map[string]any{"current": 1, "total": 3}})
			return
		}
		writeJSON(w, map[string]any{"items": []any{}, "pages": map[string]any{}})
	})
	if _, _, err := fleetTenants(context.Background(), c, "", false); err == nil {
		t.Fatal("an empty page before the reported last page must fail as an incomplete tenant list")
	}
}

func TestLocalPagingRunsAfterTenantScope(t *testing.T) {
	drop := false
	srv := fleetFake(t, &drop, false)
	fleetEnv(t, srv)
	if out, err := runRoot(t, "fleet", "refresh", "--resources", "tenants,endpoints", "--json"); err != nil {
		t.Fatalf("fleet refresh: %v\n%s", err, out)
	}
	for _, tenant := range []string{"T1", "T2"} {
		out, err := runRoot(t, "endpoints", "list", "--data-source", "local", "--tenant", tenant, "--page-size", "1", "--json")
		if err != nil {
			t.Fatalf("local list %s: %v\n%s", tenant, err, out)
		}
		if strings.Count(out, `"hostname"`) != 1 {
			t.Fatalf("--page-size 1 for %s must return exactly that tenant's first row:\n%s", tenant, out)
		}
	}
}

func TestContainFailsClosedOnUnreadableEndpointRows(t *testing.T) {
	bad := map[string]any{"id": "E2", "hostname": "WS-FIN-07", "ipv4Addresses": "10.0.0.2", "tenant": map[string]any{"id": "T1"}}
	good := map[string]any{"id": "E1", "hostname": "WS-FIN-07", "ipv4Addresses": []string{"10.0.0.1"}, "tenant": map[string]any{"id": "T1"}}
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"items": []any{good, bad}, "pages": map[string]any{}})
	})
	if _, err := liveFindEndpoint(context.Background(), c, "T1", "WS-FIN-07"); err == nil {
		t.Fatal("a live search with an unreadable row must fail, not report a unique match")
	}

	dbPath := filepath.Join(t.TempDir(), "fleet.db")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	for id, row := range map[string]any{"E1": good, "E2": bad} {
		raw, _ := json.Marshal(row)
		if err := db.Upsert("endpoints", id, raw); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	if _, err := resolveContainTargets(context.Background(), c, dbPath, []string{"WS-FIN-07"}, nil); err == nil || !strings.Contains(err.Error(), "unreadable") {
		t.Fatalf("an unreadable index row must fail closed, got %v", err)
	}
}

func TestSiemUnreadableCursorIsAnError(t *testing.T) {
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"items": []any{}, "has_more": false})
	})
	for _, row := range []string{`{"cursor": 7}`, `{"cursor": null, "saved_at": "2026-10-13T00:00:00Z"}`, `{"cursor": "", "saved_at": "2026-10-13T00:00:00Z"}`, `{"saved_at": "2026-10-13T00:00:00Z"}`, `{"cursor": "c1"}`} {
		db := siemStore(t) // fresh: Upsert merges into an existing row
		if err := db.Upsert(fleetSiemCursorType, "T1:events", json.RawMessage(row)); err != nil {
			t.Fatal(err)
		}
		if _, err := pullSiem(context.Background(), c, db, io.Discard, fleetTenant{ID: "T1"}, "events", 100, 5, time.Now()); err == nil {
			t.Fatalf("saved cursor %s must fail, not silently restart the stream", row)
		}
	}
}

func TestLocalReportsListUnreadableRowsAsBlind(t *testing.T) {
	db := siemStore(t)
	rows := map[string]map[string]string{
		fleetHealthType: {"T1": `{"tenant_id": "T1", "data": "not-an-object"}`, "T3": `{"tenant_id": "T3", "data": null}`},
		fleetLicensesType: {"T2": `{"tenant_id": "T2", "data": {"licenses": [{"quantity": "unreadable"}]}}`,
			"T4": `{"tenant_id": "T4", "data": null}`, "T5": `{"tenant_id": "T5", "data": {"licenses": null}}`,
			"T6": `{"tenant_id": "T6", "data": {"licenses": []}}`, "T7": `{"tenant_id": "T7", "data": {"licenses": [null]}}`},
	}
	for typ, byID := range rows {
		for id, raw := range byID {
			if err := db.Upsert(typ, id, json.RawMessage(raw)); err != nil {
				t.Fatal(err)
			}
		}
	}
	gaps, err := buildGaps(db, nil, gapOptions{threshold: 80, now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	blind := map[string]bool{}
	for _, b := range gaps.BlindTenants {
		if b.Status == "unreadable" {
			blind[b.TenantID] = true
		}
	}
	if !blind["T1"] || !blind["T2"] || !blind["T3"] || !blind["T4"] || !blind["T5"] || blind["T6"] || !blind["T7"] {
		t.Fatalf("gaps must list unreadable or null rows (T1-T5) as blind and accept an empty licence list (T6), got %+v", gaps.BlindTenants)
	}
	health, err := buildHealth(db, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(health.BlindTenants) != 2 || health.BlindTenants[0].TenantID != "T1" || health.BlindTenants[1].TenantID != "T3" || health.BlindTenants[1].Status != "unreadable" {
		t.Fatalf("health must list the unreadable T1 and null T3 rows as blind, got %+v", health.BlindTenants)
	}
}

func TestUnreadableLedgerRowStaysBlind(t *testing.T) {
	db := siemStore(t)
	if err := db.Upsert(fleetLedgerType, "T9", json.RawMessage(`{"id": 9}`)); err != nil {
		t.Fatal(err)
	}
	view, err := buildTriage(db, map[string]bool{"high": true}, 0, false, time.Now(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.BlindTenants) != 1 || view.BlindTenants[0].TenantID != "T9" || view.BlindTenants[0].Status != "unreadable" {
		t.Fatalf("an unreadable ledger row must surface as a blind tenant, got %+v", view.BlindTenants)
	}
}

func TestRefreshRejectsNullTenantDocs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			writeJSON(w, map[string]any{"access_token": "tok", "token_type": "bearer", "expires_in": 3600})
		case "/whoami/v1":
			writeJSON(w, map[string]any{"id": "P1", "idType": "partner"})
		case "/partner/v1/tenants":
			writeJSON(w, map[string]any{"items": []map[string]any{{"id": "T1", "name": "One", "billingType": "term", "apiHost": "https://api-us01.central.sophos.com"},
				{"id": "T2", "name": "Two", "billingType": "term", "apiHost": "https://api-us01.central.sophos.com"}},
				"pages": map[string]int{"current": 1, "total": 1}})
		case "/account-health-check/v1/health-check":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("null"))
		case "/licenses/v1/licenses":
			if r.Header.Get("X-Tenant-ID") == "T2" {
				writeJSON(w, map[string]any{"licenses": []any{nil}})
				return
			}
			writeJSON(w, map[string]any{"organization": map[string]any{"id": "T1"}})
		default:
			writeJSON(w, map[string]any{"items": []any{}, "pages": map[string]any{}})
		}
	}))
	t.Cleanup(srv.Close)
	fleetEnv(t, srv)
	dbPath := filepath.Join(t.TempDir(), "fleet.db")
	out, err := runRoot(t, "fleet", "refresh", "--resources", "tenants,health,licenses", "--db", dbPath, "--json")
	if err == nil {
		t.Fatalf("a null health body and a licences body without licenses must fail the refresh:\n%s", out)
	}
	db, oerr := store.OpenWithContext(context.Background(), dbPath)
	if oerr != nil {
		t.Fatal(oerr)
	}
	defer db.Close()
	for _, tid := range []string{"T1", "T2"} {
		for _, typ := range []string{fleetHealthType, fleetLicensesType} {
			if raw, gerr := db.Get(typ, tid); gerr == nil && len(raw) > 0 {
				t.Fatalf("%s stored an invalid document for %s: %s", typ, tid, raw)
			}
		}
	}
}

func TestKeyPagedRejectsNullItems(t *testing.T) {
	c := mockClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"items": []any{map[string]any{"id": "A"}, nil}, "pages": map[string]any{}})
	})
	if _, err := fetchKeyPaged(context.Background(), c, "/partner/v1/billing/usage/2026/09", nil, "", 0); err == nil {
		t.Fatal("a null list item must fail, not decode into an empty row")
	}
}

func TestSyncCheckpointIsPerTenant(t *testing.T) {
	for _, viaHeader := range []bool{false, true} {
		t.Run(fmt.Sprintf("header=%v", viaHeader), func(t *testing.T) { checkpointPerTenant(t, viaHeader) })
	}
}

func checkpointPerTenant(t *testing.T, viaHeader bool) {
	var mu sync.Mutex
	var seen []string // "<tenant> <pageFromKey>" per endpoints request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenant := r.Header.Get("X-Tenant-ID")
		switch r.URL.Path {
		case "/token":
			writeJSON(w, map[string]any{"access_token": "tok", "token_type": "bearer", "expires_in": 3600})
		case "/whoami/v1":
			writeJSON(w, map[string]any{"id": "P1", "idType": "partner"})
		case "/partner/v1/tenants":
			writeJSON(w, map[string]any{"items": []map[string]any{
				{"id": "T1", "name": "One", "apiHost": "https://api-us01.central.sophos.com"},
				{"id": "T2", "name": "Two", "apiHost": "https://api-us01.central.sophos.com"}}, "pages": map[string]int{"current": 1, "total": 1}})
		case "/endpoint/v1/endpoints":
			key := r.URL.Query().Get("pageFromKey")
			mu.Lock()
			seen = append(seen, tenant+" "+key)
			n := len(seen)
			mu.Unlock()
			id := fmt.Sprintf("%08d-0000-4000-a000-000000000000", n)
			writeJSON(w, map[string]any{"items": []any{map[string]any{"id": id, "hostname": "h" + id[:8], "tenant": map[string]any{"id": tenant}}},
				"pages": map[string]any{"nextKey": "key-" + tenant + "-" + id[:8]}})
		default:
			writeJSON(w, map[string]any{"items": []any{}, "pages": map[string]any{}})
		}
	}))
	t.Cleanup(srv.Close)
	fleetEnv(t, srv)
	// Prime the tenant hosts the router needs.
	if out, err := runRoot(t, "fleet", "refresh", "--resources", "tenants", "--json"); err != nil {
		t.Fatalf("fleet refresh: %v\n%s", err, out)
	}
	syncFor := func(tenant string) {
		if !viaHeader {
			_, _ = runRoot(t, "sync", "--resources", "endpoints", "--tenant", tenant, "--max-pages", "1", "--json")
			return
		}
		cfg := filepath.Join(t.TempDir(), "config.toml")
		if err := os.WriteFile(cfg, []byte("[headers]\n\"X-Tenant-ID\" = \""+tenant+"\"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("SOPHOS_CENTRAL_CONFIG", cfg)
		_, _ = runRoot(t, "sync", "--resources", "endpoints", "--max-pages", "1", "--json")
	}
	syncFor("T1")
	mu.Lock()
	seen = nil
	mu.Unlock()
	syncFor("T2")
	mu.Lock()
	defer mu.Unlock()
	if len(seen) == 0 {
		t.Fatal("T2 sync made no endpoints request")
	}
	if strings.Contains(seen[0], "key-T1") || !strings.HasPrefix(seen[0], "T2 ") {
		t.Fatalf("T2 sync resumed from T1's checkpoint: first request %q", seen[0])
	}
}

func TestDependentSyncParentsScopedToTenant(t *testing.T) {
	db := siemStore(t)
	rows := []json.RawMessage{
		json.RawMessage(`{"id": "U1", "name": "one", "tenant": {"id": "T1"}}`),
		json.RawMessage(`{"id": "U2", "name": "two", "tenant": {"id": "T2"}}`),
		json.RawMessage(`{"id": "U3", "name": "unattributed"}`),
	}
	if _, _, err := db.UpsertBatch("users", rows); err != nil {
		t.Fatal(err)
	}
	var dep dependentResourceDef
	for _, d := range dependentResourceDefs() {
		if d.ParentTable == "users" {
			dep = d
		}
	}
	parents := []map[string]string{{"id": "U1"}, {"id": "U2"}, {"id": "U3"}}
	sophosTenantFlag = "T2"
	t.Cleanup(func() { sophosTenantFlag = "" })
	got, err := sophosScopeParentRows(db, dep, parents)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, r := range got {
		ids = append(ids, r["id"])
	}
	if strings.Join(ids, ",") != "U2,U3" {
		t.Fatalf("parents for --tenant T2 = %v, want [U2 U3] (T1's user dropped, unattributed kept)", ids)
	}
}

func TestTenantScopeFollowsConfiguredHeaderAndCredential(t *testing.T) {
	t.Cleanup(resetSophosClientScope)
	recordSophosClientScope(&config.Config{Headers: map[string]string{"x-tenant-id": "T2"}})
	if got := sophosSyncStateKey("endpoints"); got != "endpoints@T2" {
		t.Fatalf("configured header tenant must key the checkpoint, got %q", got)
	}
	if got := sophosSyncStateKey("tenants"); got != "tenants" {
		t.Fatalf("partner resources keep the plain key, got %q", got)
	}
	db := siemStore(t)
	if _, _, err := db.UpsertBatch("users", []json.RawMessage{json.RawMessage(`{"id": "U1", "tenant": {"id": "T1"}}`), json.RawMessage(`{"id": "U2", "tenant": {"id": "T2"}}`)}); err != nil {
		t.Fatal(err)
	}
	var dep dependentResourceDef
	for _, d := range dependentResourceDefs() {
		if d.ParentTable == "users" {
			dep = d
		}
	}
	if got, err := sophosScopeParentRows(db, dep, []map[string]string{{"id": "U1"}, {"id": "U2"}}); err != nil || len(got) != 1 || got[0]["id"] != "U2" {
		t.Fatalf("configured header T2 must drop T1's user, got %v", got)
	}
	// No selector: two tenant credentials sharing one --db get separate checkpoints.
	recordSophosClientScope(&config.Config{ClientID: "tenant-cred-a", ClientSecret: "s"})
	a := sophosSyncStateKey("endpoints")
	recordSophosClientScope(&config.Config{ClientID: "tenant-cred-b", ClientSecret: "s"})
	b := sophosSyncStateKey("endpoints")
	if a == b || a == "endpoints" || strings.Contains(a+b, "tenant-cred") {
		t.Fatalf("credential-scoped keys must differ and not expose the client id: %q %q", a, b)
	}
}

func TestUnknownTenantOverMixedParentsFails(t *testing.T) {
	t.Cleanup(resetSophosClientScope)
	resetSophosClientScope()
	var dep dependentResourceDef
	for _, d := range dependentResourceDefs() {
		if d.ParentTable == "users" {
			dep = d
		}
	}
	one := siemStore(t)
	if _, _, err := one.UpsertBatch("users", []json.RawMessage{json.RawMessage(`{"id": "U1", "tenant": {"id": "T1"}}`)}); err != nil {
		t.Fatal(err)
	}
	if got, err := sophosScopeParentRows(one, dep, []map[string]string{{"id": "U1"}}); err != nil || len(got) != 1 {
		t.Fatalf("one tenant's parents with no selector must be kept, got %v %v", got, err)
	}
	mixed := siemStore(t)
	if _, _, err := mixed.UpsertBatch("users", []json.RawMessage{json.RawMessage(`{"id": "U1", "tenant": {"id": "T1"}}`), json.RawMessage(`{"id": "U2", "tenant": {"id": "T2"}}`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := sophosScopeParentRows(mixed, dep, []map[string]string{{"id": "U1"}, {"id": "U2"}}); err == nil {
		t.Fatal("an unknown tenant over parents from two tenants must fail, not fan out under one header")
	}
}

func TestConflictingHeaderSpellingsRejected(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Cleanup(resetSophosClientScope)
	for _, h := range []map[string]string{
		{"X-Tenant-ID": "T1", "x-tenant-id": "T2"},
		{"Authorization": "Bearer a", "authorization": "Bearer b"},
	} {
		c := client.New(&config.Config{BaseURL: "http://127.0.0.1:1", Headers: h}, time.Second, 0)
		if err := installSophosRouting(c); err == nil {
			t.Fatalf("conflicting header spellings %v must be refused", h)
		}
	}
	cfg := &config.Config{BaseURL: "http://127.0.0.1:1", Headers: map[string]string{"X-Tenant-ID": "T1", "x-tenant-id ": " T1"}}
	if err := installSophosRouting(client.New(cfg, time.Second, 0)); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Headers) != 1 || cfg.Headers[headerTenantID] != "T1" {
		t.Fatalf("same-value variants must fold into one canonical header, got %v", cfg.Headers)
	}
}

func TestConfiguredAuthorizationScopesCheckpoints(t *testing.T) {
	t.Cleanup(resetSophosClientScope)
	recordSophosClientScope(&config.Config{Headers: map[string]string{"Authorization": "Bearer tenant-a"}})
	a := sophosSyncStateKey("endpoints")
	recordSophosClientScope(&config.Config{Headers: map[string]string{"Authorization": "Bearer tenant-b"}})
	b := sophosSyncStateKey("endpoints")
	if a == b || a == "endpoints" || strings.Contains(a+b, "tenant-") {
		t.Fatalf("two configured Authorization tokens must get distinct, opaque checkpoint keys: %q %q", a, b)
	}
}

func TestCachedIdentityBecomesActive(t *testing.T) {
	r := &sophosRouter{identities: map[string]identityEntry{}, tenantHosts: map[string]string{}}
	auth := "Bearer cached"
	r.identities[authFingerprint(auth)] = identityEntry{Identity: sophosIdentity{ID: "T7", IDType: "tenant"}, FetchedAt: time.Now()}
	if _, err := r.whoami(context.Background(), auth); err != nil {
		t.Fatal(err)
	}
	if r.lastIdentity == nil || r.lastIdentity.ID != "T7" {
		t.Fatalf("a disk-cached identity must become the active identity, got %+v", r.lastIdentity)
	}
}

func TestStoreScopeFollowsEffectiveCredential(t *testing.T) {
	headerA := &config.Config{BaseURL: "https://api.central.sophos.com", Headers: map[string]string{"Authorization": "Bearer a"}}
	headerB := &config.Config{BaseURL: "https://api.central.sophos.com", Headers: map[string]string{"authorization": "Bearer b"}}
	if headerA.StoreScopeCredential() == headerB.StoreScopeCredential() {
		t.Fatal("two header-only credentials must not share a store")
	}
	withID := func(auth string) *config.Config {
		return &config.Config{BaseURL: "https://api.central.sophos.com", ClientID: "cid", ClientSecret: "s", Headers: map[string]string{"Authorization": auth}}
	}
	if withID("Bearer a").StoreScopeCredential() == withID("Bearer b").StoreScopeCredential() {
		t.Fatal("different Authorization overrides on one client ID must not share a store")
	}
	plain := &config.Config{BaseURL: "https://api.central.sophos.com", ClientID: "cid", ClientSecret: "s"}
	tokened := &config.Config{BaseURL: "https://api.central.sophos.com", ClientID: "cid", ClientSecret: "s", AccessToken: "tok"}
	if plain.StoreScopeCredential() != tokened.StoreScopeCredential() {
		t.Fatal("a client-credential store must not move once a token is saved")
	}
	// A client ID without a secret cannot mint: requests carry the saved token,
	// so two accounts' tokens must not share the client-ID store.
	t.Setenv("SOPHOS_CENTRAL_CLIENT_SECRET", "")
	idOnly := func(tok string) *config.Config {
		return &config.Config{BaseURL: "https://api.central.sophos.com", ClientID: "cid", AccessToken: tok}
	}
	if idOnly("token-a").StoreScopeCredential() == idOnly("token-b").StoreScopeCredential() {
		t.Fatal("a client ID without a secret must scope by the effective token")
	}
}

func TestMixedTenantDependentRefusalFailsTheRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			writeJSON(w, map[string]any{"access_token": "tok", "token_type": "bearer", "expires_in": 3600})
		case r.URL.Path == "/whoami/v1":
			writeJSON(w, map[string]any{"id": "P1", "idType": "partner"})
		case r.URL.Path == "/common/v1/directory/users":
			writeJSON(w, map[string]any{"items": []any{
				map[string]any{"id": "U1", "name": "one", "tenant": map[string]any{"id": "T1"}},
				map[string]any{"id": "U2", "name": "two", "tenant": map[string]any{"id": "T2"}}}, "pages": map[string]any{}})
		default:
			writeJSON(w, map[string]any{"items": []any{map[string]any{"id": "A1", "name": "admin"}}, "pages": map[string]any{}})
		}
	}))
	t.Cleanup(srv.Close)
	fleetEnv(t, srv)
	dbPath := filepath.Join(t.TempDir(), "s.db")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.UpsertBatch("users", []json.RawMessage{json.RawMessage(`{"id": "U1", "name": "one", "tenant": {"id": "T1"}}`), json.RawMessage(`{"id": "U2", "name": "two", "tenant": {"id": "T2"}}`)}); err != nil {
		t.Fatal(err)
	}
	db.Close()
	out, errOut, err := runRootArgs(t, "sync", "--resources", "partner_admins,users", "--db", dbPath, "--json", "--rate-limit", "0")
	if err == nil {
		t.Fatalf("a refused mixed-tenant dependent sync must fail the run even when another resource succeeded:\n%s\n%s", out, errOut)
	}
}

func TestClientPairPinsTheRequestCredential(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Cleanup(resetSophosClientScope)
	t.Setenv("SOPHOS_CENTRAL_CLIENT_ID", "env-id")
	t.Setenv("SOPHOS_CENTRAL_CLIENT_SECRET", "env-secret")
	// A token saved by auth set-token (no client ID) plus a stale static header.
	cfg := &config.Config{BaseURL: "http://127.0.0.1:1", AuthHeaderVal: "Bearer stale", AccessToken: "foreign", TokenExpiry: time.Now().Add(time.Hour)}
	if err := installSophosRouting(client.New(cfg, time.Second, 0)); err != nil {
		t.Fatal(err)
	}
	if cfg.AuthHeaderVal != "" || cfg.AccessToken != "" {
		t.Fatalf("with a client pair the request must mint, not send %q / %q", cfg.AuthHeaderVal, cfg.AccessToken)
	}
	// A token the saved pair minted is kept.
	minted := &config.Config{BaseURL: "http://127.0.0.1:1", ClientID: "env-id", ClientSecret: "env-secret", AccessToken: "mine", TokenExpiry: time.Now().Add(time.Hour)}
	if err := installSophosRouting(client.New(minted, time.Second, 0)); err != nil {
		t.Fatal(err)
	}
	if minted.AccessToken != "mine" {
		t.Fatal("a token minted from the saved pair must be kept")
	}
	// central_* fields alone: tokens A and B are different accounts.
	t.Setenv("SOPHOS_CENTRAL_CLIENT_ID", "")
	t.Setenv("SOPHOS_CENTRAL_CLIENT_SECRET", "")
	central := func(tok string) *config.Config {
		return &config.Config{BaseURL: "https://api.central.sophos.com", SophosCentralClientId: "cid", SophosCentralClientSecret: "s", AccessToken: tok}
	}
	if central("a").StoreScopeCredential() == central("b").StoreScopeCredential() {
		t.Fatal("central_* fields must not pin two accounts' tokens to one store")
	}
}
