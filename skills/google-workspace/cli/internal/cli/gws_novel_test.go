// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"golang.org/x/oauth2"
	"google-workspace-pp-cli/internal/client"
	"google-workspace-pp-cli/internal/cliutil"
	"google-workspace-pp-cli/internal/config"
)

var testNow = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)

func ts(daysAgo int) string {
	return testNow.Add(-time.Duration(daysAgo) * 24 * time.Hour).Format(time.RFC3339)
}

func TestScopesForProfile(t *testing.T) {
	read, err := scopesForProfile("read")
	if err != nil || len(read) != len(tenantScopeProfiles["read"]) {
		t.Fatalf("read: %v %d", err, len(read))
	}
	full, _ := scopesForProfile("full")
	if len(full) != len(tenantScopeProfiles["read"])+len(tenantScopeProfiles["full"]) {
		t.Fatalf("full should be read+full, got %d", len(full))
	}
	mixed, _ := scopesForProfile("read,admin.directory.user,https://www.googleapis.com/auth/admin.directory.user")
	if len(mixed) != len(read)+1 {
		t.Fatalf("raw scopes must dedupe, got %d", len(mixed))
	}
	if _, err := scopesForProfile("bogus"); err == nil {
		t.Fatal("unknown profile must error")
	}
	for _, s := range full {
		if !strings.HasPrefix(s, scopePrefix) {
			t.Fatalf("scope not expanded: %s", s)
		}
	}
}

func TestTenantOwnsDomainAndForEmail(t *testing.T) {
	reg := &tenantRegistry{Tenants: []gwsTenant{
		{Name: "acme", Domain: "example.com", Domains: []string{"example.org"}},
		{Name: "msp", Domain: "msp.example.com", Reseller: true},
	}}
	if got, ok := reg.forEmail("Jane@EXAMPLE.org"); !ok || got.Name != "acme" {
		t.Fatalf("alias domain should resolve acme, got %v %v", got.Name, ok)
	}
	if _, ok := reg.forEmail("x@other.example.com"); ok {
		t.Fatal("unknown domain must not resolve")
	}
	if c := reg.clients(); len(c) != 1 || c[0].Name != "acme" {
		t.Fatalf("reseller tenant must be excluded from fan-out: %+v", c)
	}
}

func TestMFAGapClassification(t *testing.T) {
	users := []dirUser{
		{PrimaryEmail: "boss@a.example.com", IsAdmin: true, LastLoginTime: ts(1)},
		{PrimaryEmail: "deleg@a.example.com", IsDelegatedAdmin: true, LastLoginTime: "1970-01-01T00:00:00.000Z"},
		{PrimaryEmail: "active@a.example.com", LastLoginTime: ts(3)},
		{PrimaryEmail: "idle@a.example.com", LastLoginTime: ts(90)},
		{PrimaryEmail: "never@a.example.com", LastLoginTime: "1970-01-01T00:00:00.000Z"},
		{PrimaryEmail: "ok@a.example.com", IsEnrolledIn2Sv: true, LastLoginTime: ts(1)},
		{PrimaryEmail: "gone@a.example.com", Suspended: true},
		{PrimaryEmail: "arch@a.example.com", Archived: true},
	}
	rows := mfaGapsForUsers("a", users, testNow, false, false)
	sortMFAGaps(rows)
	got := []string{}
	for _, r := range rows {
		got = append(got, r.Email+":"+r.Severity)
	}
	want := "boss@a.example.com:critical deleg@a.example.com:critical active@a.example.com:high idle@a.example.com:medium never@a.example.com:low"
	if strings.Join(got, " ") != want {
		t.Fatalf("got  %s\nwant %s", strings.Join(got, " "), want)
	}
	if len(mfaGapsForUsers("a", users, testNow, true, false)) != 6 {
		t.Fatal("--include-suspended should add the suspended account")
	}
	if len(mfaGapsForUsers("a", users, testNow, false, true)) != 2 {
		t.Fatal("--admins-only should keep only admins")
	}
}

func TestStaleLicenses(t *testing.T) {
	users := []dirUser{
		{PrimaryEmail: "susp@a.example.com", Suspended: true, LastLoginTime: ts(10), CreationTime: ts(400)},
		{PrimaryEmail: "never@a.example.com", LastLoginTime: "1970-01-01T00:00:00.000Z", CreationTime: ts(100)},
		{PrimaryEmail: "newbie@a.example.com", LastLoginTime: "1970-01-01T00:00:00.000Z", CreationTime: ts(2)},
		{PrimaryEmail: "idle@a.example.com", LastLoginTime: ts(75), CreationTime: ts(400)},
		{PrimaryEmail: "busy@a.example.com", LastLoginTime: ts(1), CreationTime: ts(400)},
	}
	lic := []licenseAssignment{}
	for _, e := range []string{"susp@a.example.com", "NEVER@a.example.com", "newbie@a.example.com", "idle@a.example.com", "busy@a.example.com", "orphan@a.example.com"} {
		lic = append(lic, licenseAssignment{UserID: e, SkuName: "Business Standard"})
	}
	got := map[string]string{}
	for _, r := range staleLicensesFor("a", lic, users, testNow, 60, 14) {
		got[strings.ToLower(r.Email)] = r.Reason
	}
	want := map[string]string{"susp@a.example.com": "suspended", "never@a.example.com": "never_signed_in", "idle@a.example.com": "inactive_60d", "orphan@a.example.com": "no_matching_user"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s: got %q want %q", k, got[k], v)
		}
	}
}

func TestSharingDriftParsers(t *testing.T) {
	raw := `[
	 {"id":{"time":"2026-10-01T10:00:00Z"},"actor":{"email":"admin@a.example.com"},"events":[{"name":"CHANGE_DOCS_SETTING","parameters":[{"name":"SETTING_NAME","value":"SHARING_OUTSIDE_DOMAIN"},{"name":"OLD_VALUE","value":"WARN"},{"name":"NEW_VALUE","value":"ALLOWED"}]}]},
	 {"id":{"time":"2026-10-02T10:00:00Z"},"actor":{"email":"admin@a.example.com"},"events":[{"name":"CHANGE_APPLICATION_SETTING","parameters":[{"name":"APPLICATION_NAME","value":"Calendar"},{"name":"SETTING_NAME","value":"SHARING_OUTSIDE_DOMAIN"}]}]},
	 {"id":{"time":"2026-10-02T11:00:00Z"},"actor":{"email":"admin@a.example.com"},"events":[{"name":"CHANGE_APPLICATION_SETTING","parameters":[{"name":"SETTING_NAME","value":"SESSION_LENGTH"}]}]},
	 {"id":{"time":"2026-10-03T10:00:00Z"},"actor":{"email":"bob@a.example.com"},"events":[{"name":"change_user_access","parameters":[{"name":"visibility_change","value":"external"},{"name":"target_user","value":"x@example.net"},{"name":"doc_title","value":"Q3 plan"}]}]},
	 {"id":{"time":"2026-10-04T10:00:00Z"},"actor":{"email":"bob@a.example.com"},"events":[{"name":"change_user_access","parameters":[{"name":"visibility_change","value":"external"},{"name":"target_user","value":"y@example.net"}]}]},
	 {"id":{"time":"2026-10-04T12:00:00Z"},"actor":{"email":"amy@a.example.com"},"events":[{"name":"change_user_access","parameters":[{"name":"visibility_change","value":"internal"}]}]},
	 {"id":{"time":"2026-10-05T10:00:00Z"},"actor":{"email":"amy@a.example.com"},"events":[{"name":"change_document_visibility","parameters":[{"name":"visibility","value":"public_on_the_web"},{"name":"doc_title","value":"Prices"}]}]},
	 {"id":{"time":"2026-10-05T11:00:00Z"},"actor":{"email":"amy@a.example.com"},"events":[{"name":"change_document_visibility","parameters":[{"name":"visibility","value":"private"}]}]}
	]`
	var acts []auditActivity
	if err := json.Unmarshal([]byte(raw), &acts); err != nil {
		t.Fatal(err)
	}
	sc := settingChanges("a", acts)
	if len(sc) != 2 || sc[0].To != "ALLOWED" || sc[1].Setting != "Calendar: SHARING_OUTSIDE_DOMAIN" {
		t.Fatalf("setting changes: %+v", sc)
	}
	ex := externalShares("a", acts, 5)
	if len(ex) != 1 || ex[0].Actor != "bob@a.example.com" || ex[0].Count != 2 || strings.Join(ex[0].Targets, ",") != "example.net" || ex[0].Time != "2026-10-04T10:00:00Z" {
		t.Fatalf("external shares: %+v", ex)
	}
	// One file shared out twice is one file, with one sample name.
	var again []auditActivity
	twice := `[{"id":{"time":"2026-10-03T10:00:00Z"},"actor":{"email":"bob@a.example.com"},"events":[{"name":"change_user_access","parameters":[{"name":"visibility_change","value":"external"},{"name":"doc_id","value":"d1"},{"name":"doc_title","value":"Q3 plan"}]}]},
	 {"id":{"time":"2026-10-04T10:00:00Z"},"actor":{"email":"bob@a.example.com"},"events":[{"name":"change_user_access","parameters":[{"name":"visibility_change","value":"external"},{"name":"doc_id","value":"d1"},{"name":"doc_title","value":"Q3 plan"}]}]}]`
	if err := json.Unmarshal([]byte(twice), &again); err != nil {
		t.Fatal(err)
	}
	if ex := externalShares("a", again, 5); len(ex) != 1 || ex[0].Count != 1 || len(ex[0].Files) != 1 || ex[0].Time != "2026-10-04T10:00:00Z" {
		t.Fatalf("re-sharing one file counts once: %+v", ex)
	}
	// Files are identified by doc_id: two files with one title are two files.
	again[1].Events[0].Parameters[1].Value = "d2"
	if ex := externalShares("a", again, 5); len(ex) != 1 || ex[0].Count != 2 {
		t.Fatalf("distinct doc_ids sharing a title are two files: %+v", ex)
	}
	pl := publicLinks("a", acts)
	if len(pl) != 1 || pl[0].To != "public_on_the_web" {
		t.Fatalf("public links: %+v", pl)
	}
}

// Hand-fix (handfixes.json novel-review-round16-small): duplicate products are
// read once, and alerts sort newest first by instant.
func TestDuplicateProductsAndAlertOrder(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = io.WriteString(w, `{"items":[{"userId":"u@a.example.com","skuId":"1010020027"}]}`)
	}))
	defer srv.Close()
	c := client.New(&config.Config{BaseURL: srv.URL}, time.Second, 0)
	c.NoCache = true
	ls, err := listProductLicenses(context.Background(), c, "C0", "Google-Apps,Google-Apps")
	if err != nil || len(ls) != 1 || calls != 1 {
		t.Fatalf("a duplicated product must be read once: %d licences, %d calls, %v", len(ls), calls, err)
	}
	rs := []loginAlert{{Severity: "high", User: "old", Last: "2026-10-01T10:00:00Z"}, {Severity: "high", User: "new", Last: "2026-10-01T10:00:00.001Z"}}
	sortLoginAlerts(rs)
	if rs[0].User != "new" {
		t.Fatalf("newer alert first by instant: %+v", rs)
	}
}

func TestDaysSinceAndNeverSentinel(t *testing.T) {
	if daysSince("1970-01-01T00:00:00.000Z", testNow) != -1 || daysSince("", testNow) != -1 {
		t.Fatal("never sentinel must be -1")
	}
	if d := daysSince(ts(10), testNow); d != 10 {
		t.Fatalf("got %d", d)
	}
}

// fakeGoogle records every request and serves canned Directory/DataTransfer
// responses so the offboarding order can be asserted end to end.
type fakeGoogle struct {
	mu              sync.Mutex
	reqs            []string
	pendingTransfer bool
}

func (f *fakeGoogle) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	line := r.Method + " " + r.URL.Path
	if r.Method == http.MethodPatch {
		line += " " + string(body)
	}
	f.reqs = append(f.reqs, line)
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	p := r.URL.Path
	switch {
	case r.Method == http.MethodGet && p == "/admin/directory/v1/users/jane@example.com":
		_, _ = io.WriteString(w, `{"id":"111","primaryEmail":"jane@example.com","orgUnitPath":"/Sales"}`)
	case r.Method == http.MethodGet && p == "/admin/directory/v1/users/jane.alias@example.com":
		_, _ = io.WriteString(w, `{"id":"111","primaryEmail":"jane@example.com"}`)
	case r.Method == http.MethodGet && p == "/admin/directory/v1/users/boss@example.com":
		_, _ = io.WriteString(w, `{"id":"222","primaryEmail":"boss@example.com"}`)
	case r.Method == http.MethodGet && strings.HasSuffix(p, "/tokens"):
		_, _ = io.WriteString(w, `{"kind":"admin#directory#tokenList","etag":"\"e1\"","items":[{"kind":"admin#directory#token","etag":"\"t1\"","clientId":"cid-1","displayText":"Zoom","scopes":["openid"]}]}`)
	case r.Method == http.MethodGet && strings.HasSuffix(p, "/asps"):
		_, _ = io.WriteString(w, `{"kind":"admin#directory#asps","etag":"\"e2\"","items":[{"kind":"admin#directory#asp","etag":"\"a1\"","codeId":1234567,"name":"Thunderbird","creationTime":"1700000000000"}]}`)
	case r.Method == http.MethodGet && p == "/admin/directory/v1/groups":
		_, _ = io.WriteString(w, `{"groups":[{"id":"g1","email":"sales@example.com"}]}`)
	case r.Method == http.MethodGet && p == "/admin/datatransfer/v1/applications":
		_, _ = io.WriteString(w, `{"applications":[{"id":"55656082996","name":"Drive and Docs"},{"id":"435070579839","name":"Calendar"}]}`)
	case r.Method == http.MethodPost && p == "/admin/datatransfer/v1/transfers":
		_, _ = io.WriteString(w, `{"id":"tr-9"}`)
	case r.Method == http.MethodGet && p == "/admin/datatransfer/v1/transfers":
		if f.pendingTransfer {
			_, _ = io.WriteString(w, `{"kind":"admin#datatransfer#dataTransfersList","dataTransfers":[{"id":"tr-old","overallTransferStatusCode":"inProgress"}]}`)
		} else {
			_, _ = io.WriteString(w, `{"dataTransfers":[]}`)
		}
	default:
		_, _ = io.WriteString(w, `{}`)
	}
}

func testClient(t *testing.T, url string) *client.Client {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	return client.New(&config.Config{BaseURL: url}, 5*time.Second, 0)
}

func TestOffboardOrderSuspendLast(t *testing.T) {
	fake := &fakeGoogle{}
	srv := httptest.NewServer(fake)
	defer srv.Close()
	c := testClient(t, srv.URL)
	rec := &offboardReceipt{}
	o := offboardOpts{ou: "/Departed", successor: "boss@example.com", releaseLicense: true}
	if err := runOffboard(context.Background(), c, gwsTenant{Name: "acme"}, "jane@example.com", o, true, rec); err != nil {
		t.Fatalf("runOffboard: %v (%+v)", err, rec.Steps)
	}
	var writes []string
	for _, r := range fake.reqs {
		if !strings.HasPrefix(r, "GET ") {
			writes = append(writes, r)
		}
	}
	want := []string{
		`PATCH /admin/directory/v1/users/111 {"orgUnitPath":"/Departed"}`,
		"PATCH /admin/directory/v1/users/111", // password scramble (body checked below)
		"POST /admin/directory/v1/users/111/signOut",
		"DELETE /admin/directory/v1/users/111/tokens/cid-1",
		"DELETE /admin/directory/v1/users/111/asps/1234567",
		"POST /admin/directory/v1/users/111/verificationCodes/invalidate",
		"DELETE /admin/directory/v1/groups/g1/members/111",
		"POST /admin/datatransfer/v1/transfers",
		`PATCH /admin/directory/v1/users/111 {"suspended":true}`,
	}
	if len(writes) != len(want) {
		t.Fatalf("writes:\n%s", strings.Join(writes, "\n"))
	}
	for i := range want {
		if !strings.HasPrefix(writes[i], want[i]) {
			t.Fatalf("write %d: got %q want prefix %q", i, writes[i], want[i])
		}
	}
	if !strings.Contains(writes[1], `"password"`) {
		t.Fatalf("second write must scramble the password: %s", writes[1])
	}
	if rec.TransferID != "tr-9" {
		t.Fatalf("transfer id: %q", rec.TransferID)
	}
	for _, s := range rec.Steps {
		if s.Step == "release_licenses" && s.Status != "skipped" {
			t.Fatalf("licence release must wait for the transfer: %+v", s)
		}
	}
}

func TestOffboardPreviewMakesNoWrites(t *testing.T) {
	fake := &fakeGoogle{}
	srv := httptest.NewServer(fake)
	defer srv.Close()
	rec := &offboardReceipt{}
	if err := runOffboard(context.Background(), testClient(t, srv.URL), gwsTenant{Name: "acme"}, "jane@example.com",
		offboardOpts{successor: "boss@example.com"}, false, rec); err != nil {
		t.Fatal(err)
	}
	for _, r := range fake.reqs {
		if !strings.HasPrefix(r, "GET ") {
			t.Fatalf("preview sent a write: %s", r)
		}
	}
	for _, s := range rec.Steps {
		if s.Status != "planned" && s.Status != "skipped" {
			t.Fatalf("preview step %s has status %s", s.Step, s.Status)
		}
	}
}

func TestListAllFollowsPageTokens(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch r.URL.Query().Get("pageToken") {
		case "":
			_, _ = io.WriteString(w, `{"users":[{"id":"1"},{"id":"2"}],"nextPageToken":"p2"}`)
		case "p2":
			_, _ = io.WriteString(w, `{"users":[{"id":"3"}]}`)
		default:
			t.Errorf("unexpected token %q", r.URL.Query().Get("pageToken"))
		}
	}))
	defer srv.Close()
	items, err := listAll(context.Background(), testClient(t, srv.URL), "/admin/directory/v1/users", map[string]string{"customer": "my_customer"}, "users", 0)
	if err != nil || len(items) != 3 || calls != 2 {
		t.Fatalf("items=%d calls=%d err=%v", len(items), calls, err)
	}
}

func TestRollupPosture(t *testing.T) {
	users := []dirUser{
		{PrimaryEmail: "boss@a.example.com", IsAdmin: true, LastLoginTime: ts(1), CreationTime: ts(400)},
		{PrimaryEmail: "u1@a.example.com", IsEnrolledIn2Sv: true, LastLoginTime: ts(1), CreationTime: ts(400)},
		{PrimaryEmail: "u2@a.example.com", IsEnrolledIn2Sv: true, LastLoginTime: ts(2), CreationTime: ts(400)},
		{PrimaryEmail: "u3@a.example.com", IsEnrolledIn2Sv: true, LastLoginTime: ts(2), CreationTime: ts(400)},
		{PrimaryEmail: "gone@a.example.com", Suspended: true, CreationTime: ts(400)},
	}
	lic := []licenseAssignment{{UserID: "gone@a.example.com"}, {UserID: "u1@a.example.com"}}
	p := rollupPosture(gwsTenant{Name: "a"}, users, lic, 1, testNow, 60, 14, 95)
	if p.Status != "critical" || p.ActiveUsers != 4 || p.AdminsWithout2SV != 1 || p.TwoSVEnrolledPct != 75 ||
		p.StaleLicenses != 1 || p.SuspendedLicensed != 1 || len(p.Reasons) != 4 {
		t.Fatalf("%+v", p)
	}
	users[0].IsEnrolledIn2Sv = true
	if p := rollupPosture(gwsTenant{Name: "a"}, users, nil, 0, testNow, 60, 14, 95); p.Status != "ok" || p.TwoSVEnrolledPct != 100 {
		t.Fatalf("clean tenant should be ok: %+v", p)
	}
	// The threshold compares unrounded coverage: 1,899 of 2,000 is 94.95%.
	many := make([]dirUser, 2000)
	for i := range many {
		many[i] = dirUser{PrimaryEmail: fmt.Sprintf("u%d@a.example.com", i), IsEnrolledIn2Sv: i < 1899, LastLoginTime: ts(1), CreationTime: ts(400)}
	}
	if p := rollupPosture(gwsTenant{Name: "a"}, many, nil, 0, testNow, 60, 14, 95); p.Status != "warn" || p.TwoSVEnrolledPct != 95 {
		t.Fatalf("94.95%% coverage must warn at a 95%% threshold: %+v", p)
	}
	// Exactly on the threshold is not below it (0.57*100 is 56.999... in floats).
	for _, k := range []int{29, 57, 58} {
		hundred := many[:100]
		for i := range hundred {
			hundred[i].IsEnrolledIn2Sv = i < k
		}
		if p := rollupPosture(gwsTenant{Name: "a"}, hundred, nil, 0, testNow, 60, 14, float64(k)); p.Status != "ok" {
			t.Fatalf("%d of 100 at a %d%% threshold must be ok: %+v", k, k, p)
		}
	}
}

func TestListAllRetriesGoogleQuota403(t *testing.T) {
	quotaBackoff = time.Millisecond
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"error":{"code":403,"errors":[{"reason":"userRateLimitExceeded"}]}}`)
			return
		}
		_, _ = io.WriteString(w, `{"users":[{"id":"1"}]}`)
	}))
	defer srv.Close()
	items, err := listAll(context.Background(), testClient(t, srv.URL), "/admin/directory/v1/users", nil, "users", 0)
	if err != nil || len(items) != 1 || calls != 3 {
		t.Fatalf("items=%d calls=%d err=%v", len(items), calls, err)
	}
	calls = 0
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error":{"code":403,"message":"Not Authorized to access this resource/api"}}`)
	}))
	defer srv2.Close()
	if _, err := listAll(context.Background(), testClient(t, srv2.URL), "/x", nil, "users", 0); err == nil || calls != 1 {
		t.Fatalf("a non-quota 403 must fail fast: calls=%d err=%v", calls, err)
	}
}

func TestLeaverIssues(t *testing.T) {
	f := leaverFinding{PaidLicenses: []string{"Business Standard"}, Groups: 2, AdminRoles: 1, OAuthTokens: 3, Transfer: "none"}
	leaverIssues(&f)
	if f.Status != "residual" || len(f.Issues) != 5 {
		t.Fatalf("%+v", f)
	}
	clean := leaverFinding{Transfer: "completed"}
	leaverIssues(&clean)
	if clean.Status != "clean" || len(clean.Issues) != 0 {
		t.Fatalf("fully offboarded account must be clean: %+v", clean)
	}
	unknown := leaverFinding{Transfer: "unknown"}
	leaverIssues(&unknown)
	if unknown.Status != "unverified" || len(unknown.Issues) != 1 {
		t.Fatalf("an unchecked transfer must surface as unverified, not clean: %+v", unknown)
	}
}

func TestScopeRiskAndAggregateApps(t *testing.T) {
	for scope, want := range map[string]int{
		"https://mail.google.com/":               3,
		scopePrefix + "drive":                    3,
		scopePrefix + "admin.directory.user":     3,
		scopePrefix + "drive.readonly":           2,
		scopePrefix + "calendar":                 2,
		scopePrefix + "calendar.events":          2,
		scopePrefix + "calendar.readonly":        2,
		scopePrefix + "calendar.events.readonly": 2,
		scopePrefix + "drive.file":               1,
		"openid":                                 1,
		scopePrefix + "userinfo.email":           1,
	} {
		if got := scopeRiskTier(scope); got != want {
			t.Fatalf("%s: got %d want %d", scope, got, want)
		}
	}
	grants := []appGrant{
		{Tenant: "a", ClientID: "c1", App: "Notes", Scopes: []string{"openid"}, Users: map[string]bool{"x": true, "y": true, "z": true}},
		{Tenant: "a", ClientID: "c2", App: "Mailer", Scopes: []string{"https://mail.google.com/"}, Users: map[string]bool{"x": true}},
		{Tenant: "b", ClientID: "c2", App: "Mailer", Scopes: []string{scopePrefix + "drive.readonly"}, Users: map[string]bool{"q": true}},
	}
	apps := aggregateApps(grants)
	if len(apps) != 2 || apps[0].ClientID != "c2" || apps[0].Risk != "high" || apps[0].Users != 2 ||
		strings.Join(apps[0].Tenants, ",") != "a,b" || apps[0].PerTenant["b"] != 1 || len(apps[0].Scopes) != 2 {
		t.Fatalf("high-risk cross-tenant app must lead: %+v", apps)
	}
	if apps[1].Risk != "low" || apps[1].Users != 3 {
		t.Fatalf("%+v", apps[1])
	}
}

func TestAuthorizeGrantsAndLoginAlerts(t *testing.T) {
	raw := `[
	 {"id":{"time":"2026-10-10T10:00:00Z"},"actor":{"email":"bob@a.example.com"},"events":[{"name":"authorize","parameters":[{"name":"client_id","value":"123.apps"},{"name":"app_name","value":"Zapier"},{"name":"scope","multiValue":["https://mail.google.com/","openid"]}]}]},
	 {"id":{"time":"2026-10-11T10:00:00Z"},"actor":{"email":"amy@a.example.com"},"events":[{"name":"authorize","parameters":[{"name":"client_id","value":"123.apps"},{"name":"app_name","value":"Zapier"}]}]},
	 {"id":{"time":"2026-10-12T08:00:00Z"},"actor":{"email":"system"},"events":[{"name":"suspicious_login","parameters":[{"name":"affected_email_address","value":"bob@a.example.com"}]}]},
	 {"id":{"time":"2026-10-12T09:00:00Z"},"actor":{"email":"system"},"events":[{"name":"suspicious_login","parameters":[{"name":"affected_email_address","value":"bob@a.example.com"}]}]},
	 {"id":{"time":"2026-10-12T10:00:00Z"},"actor":{"email":"amy@a.example.com"},"events":[{"name":"email_forwarding_out_of_domain","parameters":[{"name":"email_forwarding_destination_address","value":"drop@example.net"}]}]}
	]`
	var acts []auditActivity
	if err := json.Unmarshal([]byte(raw), &acts); err != nil {
		t.Fatal(err)
	}
	g := authorizeGrants("a", acts)
	if len(g) != 1 || g[0].App != "Zapier" || len(g[0].Users) != 2 || g[0].Last != "2026-10-11T10:00:00Z" || len(g[0].Scopes) != 2 {
		t.Fatalf("authorize grants: %+v", g)
	}
	sus := groupLoginAlerts("a", "suspicious_login", "high", acts)
	if len(sus) != 1 || sus[0].User != "bob@a.example.com" || sus[0].Count != 2 || sus[0].First != "2026-10-12T08:00:00Z" {
		t.Fatalf("suspicious_login grouping: %+v", sus)
	}
	fwd := groupLoginAlerts("a", "email_forwarding_out_of_domain", "high", acts)
	if len(fwd) != 1 || fwd[0].User != "amy@a.example.com" || fwd[0].Detail != "forwarding to drop@example.net" {
		t.Fatalf("forwarding alert: %+v", fwd)
	}
}

func TestSummarizePolicyValues(t *testing.T) {
	pol := func(v map[string]any) map[string]any { return map[string]any{"setting": map[string]any{"value": v}} }
	drive := postureColumns[1]
	if got := summarizePolicyValues(drive, nil, testNow); got != "default" {
		t.Fatalf("no policy -> default, got %s", got)
	}
	if got := summarizePolicyValues(drive, []map[string]any{pol(map[string]any{"externalSharingMode": "ALLOWED"})}, testNow); got != "ALLOWED" {
		t.Fatal(got)
	}
	scoped := func(ou string, order float64, v map[string]any) map[string]any {
		p := pol(v)
		p["policyQuery"] = map[string]any{"orgUnit": ou, "sortOrder": order}
		return p
	}
	mixed := []map[string]any{scoped("orgUnits/root", 1, map[string]any{"externalSharingMode": "ALLOWED"}), scoped("orgUnits/sales", 2, map[string]any{"external_sharing_mode": "DISALLOWED"})}
	if got := summarizePolicyValues(drive, mixed, testNow); got != "mixed:ALLOWED,DISALLOWED" {
		t.Fatal(got)
	}
	// Google's reduction: for the same users the greatest sortOrder wins, in
	// either list order (a SYSTEM default overridden by an ADMIN policy).
	overridden := []map[string]any{scoped("orgUnits/root", 1, map[string]any{"externalSharingMode": "ALLOWED"}), scoped("orgUnits/root", 7, map[string]any{"externalSharingMode": "DISALLOWED"})}
	for _, list := range [][]map[string]any{overridden, {overridden[1], overridden[0]}} {
		if got := summarizePolicyValues(drive, list, testNow); got != "DISALLOWED" {
			t.Fatalf("higher sortOrder must win for one scope, got %s", got)
		}
	}
	// Max reducer: a winning policy that omits the field takes Google's
	// documented default, not a lower policy's value.
	if got := summarizePolicyValues(drive, []map[string]any{scoped("orgUnits/root", 1, map[string]any{"externalSharingMode": "DISALLOWED"}), scoped("orgUnits/root", 9, map[string]any{})}, testNow); got != "ALLOWED" {
		t.Fatalf("an omitted field must read as the default, got %s", got)
	}
	gmail := postureColumns[2]
	if got := summarizePolicyValues(gmail, []map[string]any{scoped("orgUnits/root", 1, map[string]any{"enableAutoForwarding": false}), scoped("orgUnits/root", 9, map[string]any{})}, testNow); got != "true" {
		t.Fatalf("gmail auto-forwarding defaults to true, got %s", got)
	}
	// Licence conditions live in the query: same org unit, different SKUs
	// are different users.
	licensed := func(query string, order float64, v map[string]any) map[string]any {
		p := scoped("orgUnits/root", order, v)
		p["policyQuery"].(map[string]any)["query"] = query
		return p
	}
	bySKU := []map[string]any{licensed("sku == 'A'", 1, map[string]any{"externalSharingMode": "ALLOWED"}), licensed("sku == 'B'", 9, map[string]any{"externalSharingMode": "DISALLOWED"})}
	if got := summarizePolicyValues(drive, bySKU, testNow); got != "mixed:ALLOWED,DISALLOWED" {
		t.Fatalf("licence-scoped policies must not collapse, got %s", got)
	}
	twosv := postureColumns[0]
	if got := summarizePolicyValues(twosv, []map[string]any{pol(map[string]any{"enforcedFrom": "2024-01-01T00:00:00Z"})}, testNow); got != "enforced" {
		t.Fatal(got)
	}
	if got := summarizePolicyValues(twosv, []map[string]any{pol(map[string]any{})}, testNow); got != "not_enforced" {
		t.Fatal(got)
	}
}

func TestListAllScanCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"users":[{"id":"1"}],"nextPageToken":"more"}`)
	}))
	defer srv.Close()
	ctx := withScanCap(context.Background(), 3)
	items, err := listAll(ctx, testClient(t, srv.URL), "/admin/directory/v1/users", nil, "users", 0)
	if !errors.Is(err, errScanCapped) || len(items) != 3 {
		t.Fatalf("an endless list must stop at the cap and say so: items=%d err=%v", len(items), err)
	}
}

func TestEmitFanoutEnvelopeAndPartialExit(t *testing.T) {
	cmd := &cobra.Command{}
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	flags := &rootFlags{asJSON: true}
	res := fanoutResult[mfaGap]{Meta: fanoutMeta{TenantsQueried: 2, FetchFailures: []fetchFailure{{Tenant: "b", Error: "unauthorized_client"}}},
		Results: []mfaGap{{Tenant: "a", Email: "x@a.example.com"}}}
	err := emitFanout(cmd, flags, res, nil)
	var ce *cliError
	if !errors.As(err, &ce) || ce.code != 6 || !strings.Contains(err.Error(), "1 of 2") {
		t.Fatalf("a partial fan-out must exit 6 (partial failure): %v", err)
	}
	var env map[string]any
	if jerr := json.Unmarshal(out.Bytes(), &env); jerr != nil {
		t.Fatalf("stdout must be the JSON envelope: %v\n%s", jerr, out.String())
	}
	meta, _ := env["meta"].(map[string]any)
	if _, ok := meta["fetch_failures"]; !ok || meta["tenants_queried"].(float64) != 2 {
		t.Fatalf("envelope missing fields: %v", env)
	}
	if !strings.Contains(errb.String(), "1 of 2 tenant(s) failed; results cover the remaining 1") {
		t.Fatalf("stderr must name the denominator: %s", errb.String())
	}
	flags.allowPartialFailure = true
	out.Reset()
	if err := emitFanout(cmd, flags, res, nil); err != nil {
		t.Fatalf("--allow-partial-failure must exit 0: %v", err)
	}
}

func TestEmitFanoutAllFailedExits5(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	res := fanoutResult[mfaGap]{Meta: fanoutMeta{TenantsQueried: 2, FetchFailures: []fetchFailure{{Tenant: "a", Error: "x"}, {Tenant: "b", Error: "y"}}}, Results: []mfaGap{}}
	var ce *cliError
	if err := emitFanout(cmd, &rootFlags{asJSON: true}, res, nil); !errors.As(err, &ce) || ce.code != 5 {
		t.Fatalf("every tenant failing is not partial; want exit 5: %v", err)
	}
}

func TestBindTenantTokenBeatsEnvBearer(t *testing.T) {
	cfg := &config.Config{GoogleWorkspaceToken: "env-token-of-tenant-a", AccessToken: "stale"}
	bindTenantToken(cfg, gwsTenant{Name: "b"}, "minted-b")
	if got := cfg.AuthHeader(); got != "Bearer minted-b" {
		t.Fatalf("tenant token must win over GOOGLE_WORKSPACE_TOKEN, got %q", got)
	}
	// And on the wire.
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()
	cfg.BaseURL = srv.URL
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	c := client.New(cfg, 5*time.Second, 0)
	if _, err := c.GetNoCache(context.Background(), "/x", nil); err != nil || seen != "Bearer minted-b" {
		t.Fatalf("outgoing header %q err=%v", seen, err)
	}
}

func TestOffboardReleaseRefusedWhilePriorTransferPending(t *testing.T) {
	fake := &fakeGoogle{pendingTransfer: true}
	srv := httptest.NewServer(fake)
	defer srv.Close()
	rec := &offboardReceipt{}
	err := runOffboard(context.Background(), testClient(t, srv.URL), gwsTenant{Name: "acme"}, "jane@example.com",
		offboardOpts{releaseLicense: true, keepGroups: true, keepPassword: true, noSuspend: true}, true, rec)
	if err == nil {
		t.Fatal("a pending transfer must fail the release step")
	}
	for _, s := range rec.Steps {
		if s.Step == "release_licenses" && (s.Status != "failed" || !strings.Contains(s.Detail, "tr-old")) {
			t.Fatalf("release step: %+v", s)
		}
	}
	for _, r := range fake.reqs {
		if strings.Contains(r, "licensing") {
			t.Fatalf("no licensing call may happen while a transfer is pending: %s", r)
		}
	}
}

func TestLatestDriveTransferStatus(t *testing.T) {
	mk := func(when, status string, apps ...string) dataTransfer {
		tr := dataTransfer{RequestTime: when, Status: status}
		for _, a := range apps {
			tr.Apps = append(tr.Apps, struct {
				ApplicationID json.RawMessage `json:"applicationId"`
			}{json.RawMessage(`"` + a + `"`)})
		}
		return tr
	}
	drive := "55656082996"
	if got := latestDriveTransferStatus([]dataTransfer{mk("2026-01-01T00:00:00Z", "completed", drive), mk("2026-09-01T00:00:00Z", "inProgress", drive)}, drive); got != "in_progress" {
		t.Fatalf("an old completed transfer must not hide a newer pending one: %s", got)
	}
	if got := latestDriveTransferStatus([]dataTransfer{mk("2026-09-01T00:00:00Z", "completed", "435070579839")}, drive); got != "none" {
		t.Fatalf("a Calendar-only transfer is not a Drive transfer: %s", got)
	}
	if got := latestDriveTransferStatus([]dataTransfer{mk("2026-09-01T00:00:00Z", "completed", drive, "435070579839")}, drive); got != "completed" {
		t.Fatal(got)
	}
	// Sub-second ordering: a string compare would pick the older completed one.
	if got := latestDriveTransferStatus([]dataTransfer{mk("2026-10-13T12:00:00.001Z", "inProgress", drive), mk("2026-10-13T12:00:00Z", "completed", drive)}, drive); got != "in_progress" {
		t.Fatalf("newer by milliseconds must win: %s", got)
	}
}

func TestEnforcementState(t *testing.T) {
	if got := enforcementState("2030-01-01T00:00:00Z", testNow); got != "scheduled:2030-01-01" {
		t.Fatalf("future enforcedFrom is scheduled, got %s", got)
	}
	if got := enforcementState("2024-01-01T00:00:00Z", testNow); got != "enforced" {
		t.Fatal(got)
	}
	if got := enforcementState("soon", testNow); got != "invalid:soon" {
		t.Fatal(got)
	}
}

func TestListAllMaxReportsTruncation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"items":[{"id":"1"},{"id":"2"},{"id":"3"}],"nextPageToken":"more"}`)
	}))
	defer srv.Close()
	items, err := listAll(context.Background(), testClient(t, srv.URL), "/a", nil, "items", 2)
	if !errors.Is(err, errScanCapped) || len(items) != 2 {
		t.Fatalf("a capped window must say so: items=%d err=%v", len(items), err)
	}
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"items":[{"id":"1"},{"id":"2"}]}`)
	}))
	defer srv2.Close()
	if items, err := listAll(context.Background(), testClient(t, srv2.URL), "/a", nil, "items", 2); err != nil || len(items) != 2 {
		t.Fatalf("exactly max with no more pages is complete: items=%d err=%v", len(items), err)
	}
}

func TestBindTenantTokenStripsConfiguredAuthorizationHeader(t *testing.T) {
	cfg := &config.Config{Headers: map[string]string{"authorization": "Bearer tenant-a", "X-Other": "keep"}}
	bindTenantToken(cfg, gwsTenant{Name: "b"}, "minted-b")
	var seen, other string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen, other = r.Header.Get("Authorization"), r.Header.Get("X-Other")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()
	cfg.BaseURL = srv.URL
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	if _, err := client.New(cfg, 5*time.Second, 0).GetNoCache(context.Background(), "/x", nil); err != nil || seen != "Bearer minted-b" || other != "keep" {
		t.Fatalf("auth=%q other=%q err=%v", seen, other, err)
	}
}

func TestConfirmedRequiresExplicitYes(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	flags := &rootFlags{}
	root.PersistentFlags().BoolVar(&flags.yes, "yes", false, "")
	child := &cobra.Command{Use: "child", RunE: func(*cobra.Command, []string) error { return nil }}
	root.AddCommand(child)
	root.SetArgs([]string{"child"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	flags.yes = true // what a run profile does: Value.Set without Changed
	if confirmed(child, flags) {
		t.Fatal("a profile-supplied yes must not confirm a destructive action")
	}
	root.SetArgs([]string{"child", "--yes"})
	if err := root.Execute(); err != nil || !confirmed(child, flags) {
		t.Fatalf("explicit --yes must confirm: %v", err)
	}
}

func TestUpdateTenantRegistryCreatesConfigDir(t *testing.T) {
	home := t.TempDir()
	restore, err := cliutil.SetHomeOverride(home + "/fresh")
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	err = updateTenantRegistry(func(reg *tenantRegistry) error {
		reg.Tenants = append(reg.Tenants, gwsTenant{Name: "acme", Domain: "example.com"})
		return nil
	})
	if err != nil {
		t.Fatalf("first registration on a fresh home must work: %v", err)
	}
	reg, err := loadTenantRegistry()
	if err != nil || len(reg.Tenants) != 1 {
		t.Fatalf("registry: %+v %v", reg, err)
	}
}

func TestOffboardRefusesSuccessorAliasOfLeaver(t *testing.T) {
	fake := &fakeGoogle{}
	srv := httptest.NewServer(fake)
	defer srv.Close()
	rec := &offboardReceipt{}
	err := runOffboard(context.Background(), testClient(t, srv.URL), gwsTenant{Name: "acme"}, "jane@example.com",
		offboardOpts{successor: "jane.alias@example.com"}, true, rec)
	if err == nil || !strings.Contains(err.Error(), "same account") {
		t.Fatalf("an alias of the leaver must be refused: %v", err)
	}
	for _, r := range fake.reqs {
		if !strings.HasPrefix(r, "GET ") {
			t.Fatalf("no write may happen before the successor is validated: %s", r)
		}
	}
}

func TestGetenvTenantIgnoresMCPBPlaceholder(t *testing.T) {
	t.Setenv("GOOGLE_WORKSPACE_TENANT", "${user_config.google_workspace_tenant}")
	if got := getenvTenant(); got != "" {
		t.Fatalf("an unresolved MCPB placeholder must read as no tenant, got %q", got)
	}
	t.Setenv("GOOGLE_WORKSPACE_TENANT", " acme ")
	if got := getenvTenant(); got != "acme" {
		t.Fatalf("got %q", got)
	}
}

func TestOffboardFollowUpHintsAreRunnable(t *testing.T) {
	for _, tc := range []struct {
		tenant  gwsTenant
		wantArg string
	}{
		{gwsTenant{Name: "default"}, ""}, // single-org bearer mode: no registry entry to name
		{gwsTenant{Name: "acme", KeyFile: "keys/acme.json"}, " --tenant acme"},
	} {
		fake := &fakeGoogle{}
		srv := httptest.NewServer(fake)
		rec := &offboardReceipt{}
		o := offboardOpts{successor: "boss@example.com", releaseLicense: true, keepGroups: true}
		if err := runOffboard(context.Background(), testClient(t, srv.URL), tc.tenant, "jane@example.com", o, true, rec); err != nil {
			t.Fatalf("runOffboard: %v", err)
		}
		srv.Close()
		next := strings.Join(rec.Next, "\n")
		if strings.Contains(next, "offboard ") || !strings.Contains(next, "license-assignments delete") {
			t.Fatalf("follow-up must be a licence delete, not an offboard re-run:\n%s", next)
		}
		if strings.Contains(next, "--tenant default") || (tc.wantArg != "" && !strings.Contains(next, tc.wantArg)) {
			t.Fatalf("tenant flag for %q wrong:\n%s", tc.tenant.Name, next)
		}
	}
}

func TestTenantStoreNeverAdoptsBearerStore(t *testing.T) {
	restore, err := cliutil.SetHomeOverride(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	if err := updateTenantRegistry(func(reg *tenantRegistry) error {
		reg.Tenants = append(reg.Tenants, gwsTenant{Name: "acme", Domain: "example.com", KeyFile: "k.json"}, gwsTenant{Name: "beta", Domain: "example.org", KeyFile: "k2.json"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOOGLE_WORKSPACE_TOKEN", "another-orgs-token")
	defer func() { tenantFlag = ""; resetTenantSelection(); setDefaultDBScopeIdentity("", "", "") }()

	tenantFlag = "acme"
	resetTenantSelection()
	configureDefaultDBScope("")
	hash, legacy, _ := currentDefaultDBScope()
	if legacy != "" {
		t.Fatalf("a tenant-pinned store must not adopt the bearer-named store (legacy hash %q)", legacy)
	}
	if hash != hashDefaultDBScope(tenantStoreIdentity(&gwsTenant{Name: "acme", Domain: "example.com"})) {
		t.Fatalf("store must be keyed to the tenant: %q", hash)
	}

	// No tenant selected (two registered): the generic token-keyed behavior is unchanged.
	tenantFlag = ""
	resetTenantSelection()
	configureDefaultDBScope("")
	if hash, _, _ := currentDefaultDBScope(); hash == hashDefaultDBScope(tenantStoreIdentity(&gwsTenant{Name: "acme", Domain: "example.com"})) {
		t.Fatal("without a selected tenant the store must not be the tenant's")
	}
}

func TestTenantStoreNeverClaimsUnscopedDBAndLocalReadNeedsTenant(t *testing.T) {
	restore, err := cliutil.SetHomeOverride(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	if err := updateTenantRegistry(func(reg *tenantRegistry) error {
		reg.Tenants = append(reg.Tenants, gwsTenant{Name: "acme", Domain: "example.com", KeyFile: "k.json"}, gwsTenant{Name: "beta", Domain: "example.org", KeyFile: "k2.json"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOOGLE_WORKSPACE_TOKEN", "")
	t.Setenv("ADMIN_ACCESS_TOKEN", "")
	defer func() { tenantFlag = ""; resetTenantSelection(); setDefaultDBScopeIdentity("", "", "") }()
	dir, err := cliutil.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	unscoped := filepath.Join(dir, "data.db")
	if err := os.WriteFile(unscoped, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	tenantFlag = "acme"
	resetTenantSelection()
	configureDefaultDBScope("")
	if got := defaultDBPathInDir(dir); got != filepath.Join(dir, "data-"+hashDefaultDBScope(tenantStoreIdentity(&gwsTenant{Name: "acme", Domain: "example.com"}))+".db") {
		t.Fatalf("a tenant must use its own store, never claim data.db: %s", got)
	}
	if _, err := os.Stat(unscoped + ".owner"); !os.IsNotExist(err) {
		t.Fatalf("a tenant must not mark data.db as its own: %v", err)
	}
	if err := localStoreTenantErr(); err != nil {
		t.Fatalf("a selected tenant reads its own mirror: %v", err)
	}

	tenantFlag = ""
	resetTenantSelection()
	configureDefaultDBScope("")
	if err := localStoreTenantErr(); err == nil || !strings.Contains(err.Error(), "none is selected") {
		t.Fatalf("two tenants and none selected must refuse a local read: %v", err)
	}
	if _, err := MCPStorePath(); err == nil {
		t.Fatal("MCP sql/search must refuse too")
	}

	// An ambient token is not a tenant choice: with tenants registered and
	// none selected, local reads and API clients both refuse.
	t.Setenv("GOOGLE_WORKSPACE_TOKEN", "old-org-token")
	resetTenantSelection()
	configureDefaultDBScope("")
	if err := localStoreTenantErr(); err == nil || !strings.Contains(err.Error(), "none is selected") {
		t.Fatalf("a bearer token must not stand in for a tenant once tenants are registered: %v", err)
	}
	c := client.New(&config.Config{BaseURL: "https://example.invalid", GoogleWorkspaceToken: "old-org-token"}, time.Second, 0)
	if err := ApplyClientHooks(c); err == nil || !strings.Contains(err.Error(), "none is selected") {
		t.Fatalf("an API client must refuse rather than act on the ambient token's organization: %v", err)
	}
}

// TestTenantSelectionFollowsRegistryChanges: a CLI command keeps one tenant
// for its whole run, while each MCP call (MCPStorePath, newMCPClient) re-reads
// the registry. No resetTenantSelection between phases, as in production.
func TestTenantSelectionFollowsRegistryChanges(t *testing.T) {
	restore, err := cliutil.SetHomeOverride(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	t.Setenv("GOOGLE_WORKSPACE_TOKEN", "")
	t.Setenv("ADMIN_ACCESS_TOKEN", "")
	tenantFlag = ""
	resetTenantSelection()
	defer func() { resetTenantSelection(); setDefaultDBScopeIdentity("", "", "") }()
	add := func(tn gwsTenant) {
		t.Helper()
		if err := updateTenantRegistry(func(reg *tenantRegistry) error {
			reg.Tenants = append(reg.Tenants, tn)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}

	add(gwsTenant{Name: "acme", Domain: "example.com", KeyFile: "k.json"})
	if got, err := selectedTenant(); err != nil || got == nil || got.Name != "acme" {
		t.Fatalf("the only tenant auto-selects: %v %v", got, err)
	}
	add(gwsTenant{Name: "beta", Domain: "example.org", KeyFile: "k2.json"})
	if got, err := selectedTenant(); err != nil || got == nil || got.Name != "acme" {
		t.Fatalf("within one command the tenant must not change under it: %v %v", got, err)
	}
	if _, err := MCPStorePath(); err == nil || !strings.Contains(err.Error(), "none is selected") {
		t.Fatalf("an MCP call after a second tenant is added must re-read the registry and refuse: %v", err)
	}
	if got, err := selectedTenant(); err != nil || got != nil {
		t.Fatalf("after the refresh no tenant is auto-selected: %v %v", got, err)
	}

	// Same name, different client: a different store.
	acme := tenantStoreIdentity(&gwsTenant{Name: "acme", Domain: "example.com"})
	if acme == tenantStoreIdentity(&gwsTenant{Name: "acme", Domain: "other.example"}) {
		t.Fatal("re-registering a name for another domain must not reuse its store")
	}

	// A malformed config keeps the tenant's own store scope.
	tenantFlag = "acme"
	defer func() { tenantFlag = "" }()
	resetTenantSelection()
	bad := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(bad, []byte("not = [valid"), 0o600); err != nil {
		t.Fatal(err)
	}
	configureDefaultDBScope(bad)
	if hash, _, _ := currentDefaultDBScope(); hash != hashDefaultDBScope(acme) {
		t.Fatalf("a bad config must not drop the tenant's store scope (hash %q)", hash)
	}
	if err := localStoreTenantErr(); err != nil {
		t.Fatalf("the tenant's own store stays readable: %v", err)
	}
}

func TestTokenSourceFollowsKeyRotation(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "k.json")
	write := func(id string) {
		t.Helper()
		key := `{"client_email":"sa@example.iam.gserviceaccount.com","private_key":"fake-key","private_key_id":"` + id + `"}`
		if err := os.WriteFile(keyPath, []byte(key), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	tn := gwsTenant{Name: "rotate-test", Domain: "example.com", Admin: "admin@example.com", KeyFile: keyPath}
	scopes := []string{"https://www.googleapis.com/auth/admin.directory.user.readonly"}
	write("old")
	first, err := tenantTokenSource(tn, scopes)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := tenantTokenSource(tn, scopes)
	if first != again {
		t.Fatal("an unchanged key must reuse its token source")
	}
	write("new")
	rotated, err := tenantTokenSource(tn, scopes)
	if err != nil {
		t.Fatal(err)
	}
	if rotated == first {
		t.Fatal("a rotated key must not reuse the old key's token source")
	}
}

func TestAuthSetupHintNamesTheTenant(t *testing.T) {
	restore, err := cliutil.SetHomeOverride(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	tenantFlag = ""
	resetTenantSelection()
	defer func() { tenantFlag = ""; resetTenantSelection() }()
	if h := authSetupHint(); !strings.Contains(h, "GOOGLE_WORKSPACE_TOKEN") {
		t.Fatalf("no tenants: the bearer token is the fix: %q", h)
	}
	if err := updateTenantRegistry(func(reg *tenantRegistry) error {
		reg.Tenants = append(reg.Tenants, gwsTenant{Name: "acme", Domain: "example.com", KeyFile: "k.json"}, gwsTenant{Name: "beta", Domain: "example.org", KeyFile: "k2.json"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	resetTenantSelection()
	if h := authSetupHint(); !strings.Contains(h, "--tenant") || strings.Contains(h, "export GOOGLE_WORKSPACE_TOKEN") {
		t.Fatalf("tenants registered, none selected: name --tenant, not a bearer token: %q", h)
	}
	tenantFlag = "acme"
	resetTenantSelection()
	if h := authSetupHint(); !strings.Contains(h, "tenants test acme") {
		t.Fatalf("a selected tenant: point at its delegation: %q", h)
	}
}

type seqTokenSource struct{ n int }

func (s *seqTokenSource) Token() (*oauth2.Token, error) {
	s.n++
	return &oauth2.Token{AccessToken: fmt.Sprintf("minted-%d", s.n)}, nil
}

// TestTenantTokenRefreshesPerRequest: a long run must send the token
// source's current token, not the one minted when the client was built.
func TestTenantTokenRefreshesPerRequest(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	cfg := &config.Config{BaseURL: srv.URL}
	bindTenantToken(cfg, gwsTenant{Name: "acme"}, "minted-at-build")
	cfg.AuthHeaderSource = refreshingAuthHeader(&seqTokenSource{})
	c := client.New(cfg, 5*time.Second, 0)
	c.NoCache = true
	for i := 0; i < 2; i++ {
		if _, err := c.Get(context.Background(), "/admin/directory/v1/users", nil); err != nil {
			t.Fatal(err)
		}
	}
	if len(seen) != 2 || seen[0] == "Bearer minted-at-build" || seen[0] == seen[1] {
		t.Fatalf("each request must carry the token source's current token: %v", seen)
	}
}

func TestAuthErrorHintForMCPExecute(t *testing.T) {
	restore, err := cliutil.SetHomeOverride(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	tenantFlag = ""
	resetTenantSelection()
	defer resetTenantSelection()
	if err := updateTenantRegistry(func(reg *tenantRegistry) error {
		reg.Tenants = append(reg.Tenants, gwsTenant{Name: "acme", Domain: "example.com", KeyFile: "k.json"}, gwsTenant{Name: "beta", Domain: "example.org", KeyFile: "k2.json"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	resetTenantSelection()
	if h := AuthErrorHint(errors.New("GET /admin/directory/v1/users returned HTTP 401: {}")); !strings.Contains(h, "--tenant") {
		t.Fatalf("a 401 with tenants registered and none selected must name --tenant: %q", h)
	}
	if h := AuthErrorHint(errors.New("returned HTTP 404")); h != "" {
		t.Fatalf("only auth errors get the hint: %q", h)
	}
	if h := AuthErrorHint(errors.New(`returned HTTP 403: {"error":{"errors":[{"reason":"userRateLimitExceeded"}]}}`)); h != "" {
		t.Fatalf("a quota 403 is not a permission problem: %q", h)
	}
	if h := AuthErrorHint(errors.New("returned HTTP 403: PERMISSION_DENIED")); !strings.Contains(h, "permission denied") || !strings.Contains(h, "--tenant") {
		t.Fatalf("a tenant-mode 403 names the permission problem and the tenant fix: %q", h)
	}
	if err := updateTenantRegistry(func(reg *tenantRegistry) error { reg.Tenants = nil; return nil }); err != nil {
		t.Fatal(err)
	}
	resetTenantSelection()
	if h := AuthErrorHint(errors.New("returned HTTP 403: PERMISSION_DENIED")); !strings.Contains(h, "permission denied") || strings.Contains(h, "GOOGLE_WORKSPACE_TOKEN") {
		t.Fatalf("a bearer-mode 403 must not tell the user to set the token they already sent: %q", h)
	}
}

// Hand-fix (handfixes.json licence-products-include-education): paid
// Education seats live under product 101031. A customer without Education
// must not fail when 101031 follows another product; a quota 403, 101031
// listed alone, or any other product's failure must fail loudly.
func TestLicenceProductsIncludeEducation(t *testing.T) {
	eduStatus, eduBody := 0, ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/product/Google-Apps/users"):
			_, _ = io.WriteString(w, `{"items":[{"userId":"a@example.com","productId":"Google-Apps","skuId":"1010020028","skuName":"Business Standard"}]}`)
		case strings.HasSuffix(r.URL.Path, "/product/101031/users"):
			if eduStatus != 0 {
				w.WriteHeader(eduStatus)
				_, _ = io.WriteString(w, eduBody)
				return
			}
			_, _ = io.WriteString(w, `{"items":[{"userId":"b@example.com","productId":"101031","skuId":"1010310008","skuName":"Education Plus"}]}`)
		case strings.HasSuffix(r.URL.Path, "/product/Google-Vault/users"):
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `{"error":{"code":403,"message":"Not Authorized"}}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	c := testClient(t, srv.URL)
	ctx := context.Background()
	if ls, err := listProductLicenses(ctx, c, "C1", defaultLicenceProducts); err != nil || len(ls) != 2 {
		t.Fatalf("default products must include paid Education seats: %v %v", ls, err)
	}
	eduStatus, eduBody = http.StatusBadRequest, `{"error":{"code":400,"message":"Invalid productId"}}`
	if ls, err := listProductLicenses(ctx, c, "C2", defaultLicenceProducts); err != nil || len(ls) != 1 {
		t.Fatalf("a customer without Education must not fail the default list: %v %v", ls, err)
	}
	if _, err := listProductLicenses(ctx, c, "C3", "101031"); err == nil {
		t.Fatal("101031 listed alone must fail loudly")
	}
	if _, err := listProductLicenses(ctx, c, "C4", "Google-Apps,Google-Vault"); err == nil {
		t.Fatal("a product other than 101031 must fail loudly")
	}
	eduStatus, eduBody = http.StatusForbidden, `{"error":{"code":403,"errors":[{"reason":"quotaExceeded"}]}}`
	if _, err := listProductLicenses(ctx, c, "C5", defaultLicenceProducts); err == nil {
		t.Fatal("a quota 403 on 101031 must not be read as no Education seats")
	}
}

// Hand-fix (handfixes.json profiles-never-store-url-secrets).
func TestProfileSaveRefusesURLSecrets(t *testing.T) {
	restore, err := cliutil.SetHomeOverride(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	for _, spec := range []string{"webhook:https://hooks.example/cb?token=SECRET", "webhook:https://hooks.slack.example/services/T0/B0/SECRET"} {
		root := RootCmd()
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs([]string{"profile", "save", "p", "--deliver", spec})
		if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "deliver") || strings.Contains(err.Error()+out.String(), "SECRET") {
			t.Fatalf("%s must not be saved: %v %s", spec, err, out.String())
		}
	}
}

// Hand-fix (handfixes.json tenants-test-checks-domain).
func TestTenantDomainsRequiresConfiguredDomain(t *testing.T) {
	raw := []byte(`{"domains":[{"domainName":"Acme.com","domainAliases":[{"domainAliasName":"acme.net"}]},{"domainName":"acme.org"}]}`)
	got, err := tenantDomains(gwsTenant{Name: "acme", Domain: "acme.com"}, raw)
	if err != nil || strings.Join(got, ",") != "acme.net,acme.org" {
		t.Fatalf("primary domain: %v %v", got, err)
	}
	if _, err := tenantDomains(gwsTenant{Name: "acme", Domain: "acme.net"}, raw); err != nil {
		t.Fatalf("a domain alias belongs to the customer: %v", err)
	}
	if _, err := tenantDomains(gwsTenant{Name: "acme", Domain: "other.com"}, raw); !errors.Is(err, errWrongCustomer) {
		t.Fatalf("another customer's credentials must fail verification: %v", err)
	}
}

// Hand-fix (handfixes.json deliver-errors-mask-url-secrets).
func TestDeliverErrorsMaskURLSecrets(t *testing.T) {
	for _, spec := range []string{"x?token=SECRETA&t=12:00", "webhook:ftp://h/?token=SECRETB", "webhook:https://h/?sig=SECRETF:x"} {
		if _, err := ParseDeliverSink(spec); err != nil && strings.Contains(err.Error(), "SECRET") {
			t.Errorf("%s: %v", spec, err)
		}
	}
	for _, u := range []string{"https://h.example/%zz?token=SECRETC", "https://user:SECRETD@127.0.0.1:9/cb?token=SECRETE"} {
		if err := deliverWebhook(u, nil, false); err == nil || strings.Contains(err.Error(), "SECRET") {
			t.Errorf("%s: %v", u, err)
		}
	}
}

// Hand-fix (handfixes.json deliver-webhook-https-only).
func TestDeliverWebhookHTTPSOnly(t *testing.T) {
	for spec, ok := range map[string]bool{
		"webhook:http://hooks.example/cb":    false,
		"webhook:http://user:pw@10.0.0.5/cb": false,
		"webhook:https://hooks.example/cb":   true,
		"webhook:http://127.0.0.1:9/cb":      true,
		"webhook:http://localhost/cb":        true,
	} {
		if _, err := ParseDeliverSink(spec); (err == nil) != ok {
			t.Errorf("%s: %v", spec, err)
		}
	}
}

// Hand-fix (handfixes.json tenants-test-checks-domain): a quarantined tenant
// is never used to build a client.
func TestQuarantinedTenantRefused(t *testing.T) {
	c := client.New(&config.Config{BaseURL: "https://example.invalid"}, time.Second, 0)
	err := applyTenantClient(context.Background(), c, gwsTenant{Name: "acme", KeyFile: "k.json", OwnershipMismatch: "wrong customer"})
	if err == nil || !strings.Contains(err.Error(), "quarantined") {
		t.Fatalf("a quarantined tenant must be refused: %v", err)
	}
}

// Hand-fix (handfixes.json free-license-skus-not-waste): an Archived User
// seat retains a leaver's data on purpose.
func TestArchivedUserSeatIsNotWaste(t *testing.T) {
	if !freeLicenseSKU(licenseAssignment{ProductID: "101034", SkuID: "1010340001"}) {
		t.Fatal("Archived User seats are not waste")
	}
	if freeLicenseSKU(licenseAssignment{ProductID: "Google-Apps", SkuID: "1010020020"}) {
		t.Fatal("a paid Workspace seat is not free")
	}
}

// Hand-fix (handfixes.json deliver-errors-mask-url-secrets): a webhook is
// named by host only (Slack keeps its secret in the path), and a redirect is
// refused (deliver-webhook-https-only).
func TestDeliverWebhookLabelsAndRedirects(t *testing.T) {
	if got := webhookLabel("https://user:pw@hooks.slack.example/services/T0/B0/PATH-SECRET?token=Q-SECRET"); got != "https://hooks.slack.example/..." {
		t.Fatalf("label %q", got)
	}
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://203.0.113.9/cb?token=R-SECRET", http.StatusTemporaryRedirect)
	}))
	defer redirect.Close()
	err := deliverWebhook(redirect.URL+"/services/T0/B0/PATH-SECRET", []byte(`{}`), false)
	if err == nil || !strings.Contains(err.Error(), "redirect") || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("a redirect must be refused without echoing the URL: %v", err)
	}
	for _, ep := range []string{"http://203.0.113.9/fb?token=F-SECRET", "HTTP://user:F-SECRET@203.0.113.9/fb"} {
		if err := postFeedback(ep, FeedbackEntry{}); err == nil || strings.Contains(err.Error(), "SECRET") {
			t.Fatalf("plain-http feedback to a remote host must be refused (%s): %v", ep, err)
		}
	}
	// A 302 after the POST (Apps Script web apps) drops the body and is
	// followed on https or loopback; the delivery succeeds.
	var gets int
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			gets++
			return
		}
		http.Redirect(w, r, "/echo", http.StatusFound)
	}))
	defer final.Close()
	if err := deliverWebhook(final.URL+"/macros/s/ID/exec", []byte(`{}`), false); err != nil || gets != 1 {
		t.Fatalf("a 302 to a GET must be followed: %v (gets=%d)", err, gets)
	}
}

// Hand-fix (handfixes.json tenants-test-checks-domain): quarantine deletes
// the tenant's mirror, so lifting it later starts from an empty store.
func TestDiscardTenantMirror(t *testing.T) {
	restore, err := cliutil.SetHomeOverride(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	tn := gwsTenant{Name: "acme", Domain: "example.com"}
	dir, err := cliutil.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(dir, "data-"+hashDefaultDBScope(tenantStoreIdentity(&tn))+".db")
	for _, f := range []string{base, base + "-wal"} {
		if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := discardTenantMirror(tn); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{base, base + "-wal"} {
		if _, err := os.Stat(f); !os.IsNotExist(err) {
			t.Fatalf("%s must be moved off the live path: %v", f, err)
		}
	}
	// Moved aside, not erased: the learn tables live in the same file.
	aside, _ := filepath.Glob(base + ".quarantined-*")
	if len(aside) != 2 {
		t.Fatalf("mirror and its -wal must be kept aside, got %v", aside)
	}
	if err := discardTenantMirror(tn); err != nil {
		t.Fatalf("a retry with nothing left to move must succeed: %v", err)
	}
	// A retry after a partial move pairs a leftover sidecar with the newest
	// moved-aside mirror that has none, and never overwrites one.
	for _, f := range aside {
		_ = os.Remove(f)
	}
	if err := os.WriteFile(base+".quarantined-1", []byte("db"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(base+"-wal", []byte("wal"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := discardTenantMirror(tn); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(base + ".quarantined-1-wal"); err != nil || string(b) != "wal" {
		t.Fatalf("the leftover -wal must join its mirror: %q %v", b, err)
	}
	if err := os.WriteFile(base+"-wal", []byte("wal2"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := discardTenantMirror(tn); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(base + ".quarantined-1-wal"); string(b) != "wal" {
		t.Fatalf("an earlier moved-aside sidecar must never be overwritten: %q", b)
	}
	// A different leftover sidecar still joins the mirror whose slot is free.
	if err := os.WriteFile(base+"-shm", []byte("shm"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := discardTenantMirror(tn); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(base + ".quarantined-1-shm"); err != nil || string(b) != "shm" {
		t.Fatalf("the leftover -shm must join its mirror: %q %v", b, err)
	}
	// A live mirror (re-quarantined after a lift) moves with its own
	// sidecars; none of them joins an older moved-aside mirror.
	if err := os.Remove(base + ".quarantined-1-journal"); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	for _, f := range []string{base, base + "-journal"} {
		if err := os.WriteFile(f, []byte("live"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := discardTenantMirror(tn); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(base + ".quarantined-1-journal"); !os.IsNotExist(err) {
		t.Fatalf("a live mirror's sidecar must not join an older moved-aside mirror: %v", err)
	}
}

// Hand-fix (handfixes.json tenants-test-checks-domain): replacing or removing
// a quarantined tenant moves its mirror aside first, and a replacement stays
// quarantined until tenants test passes.
func TestQuarantineReplaceAndRemoveMoveMirror(t *testing.T) {
	home := t.TempDir()
	restore, err := cliutil.SetHomeOverride(home)
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	defer func() { tenantFlag = ""; resetTenantSelection(); setDefaultDBScopeIdentity("", "", "") }()
	keyPath := filepath.Join(t.TempDir(), "sa.json")
	if err := os.WriteFile(keyPath, []byte(`{"type":"service_account","client_email":"sa@p.iam.gserviceaccount.com","client_id":"111"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	tn := gwsTenant{Name: "acme", Domain: "acme.com", Admin: "admin@acme.com", KeyFile: keyPath, OwnershipMismatch: "wrong customer"}
	if err := updateTenantRegistry(func(reg *tenantRegistry) error { reg.Tenants = append(reg.Tenants, tn); return nil }); err != nil {
		t.Fatal(err)
	}
	dir, err := cliutil.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	mirror := filepath.Join(dir, "data-"+hashDefaultDBScope(tenantStoreIdentity(&tn))+".db")
	seed := func() {
		t.Helper()
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(mirror, []byte("ORG_B_ROWS"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	run := func(args ...string) error {
		root := RootCmd()
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs(append(args, "--home", home))
		return root.Execute()
	}
	seed()
	if err := run("tenants", "add", "acme", "--domain", "acme.com", "--admin", "admin@acme.com", "--key", keyPath, "--yes"); err != nil {
		t.Fatal(err)
	}
	reg, err := loadTenantRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := reg.find("acme"); !ok || got.OwnershipMismatch == "" || got.KeyFile == keyPath {
		t.Fatalf("a replacement must stay quarantined until tenants test passes: %+v", got)
	}
	if _, err := os.Stat(mirror); !os.IsNotExist(err) {
		t.Fatalf("replacing a quarantined tenant must move its mirror aside: %v", err)
	}
	seed()
	if err := run("tenants", "remove", "acme", "--yes"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(mirror); !os.IsNotExist(err) {
		t.Fatalf("removing a quarantined tenant must move its mirror aside: %v", err)
	}
}

// Hand-fix (handfixes.json tenants-test-checks-domain): a passing test of a
// quarantined tenant moves its mirror aside before the quarantine lifts, and
// stays failed (quarantined) when the move fails.
func TestQuarantineLiftMovesMirrorFirst(t *testing.T) {
	restore, err := cliutil.SetHomeOverride(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	tn := gwsTenant{Name: "acme", Domain: "acme.com", OwnershipMismatch: "wrong customer"}
	dir, err := cliutil.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	mirror := filepath.Join(dir, "data-"+hashDefaultDBScope(tenantStoreIdentity(&tn))+".db")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mirror, []byte("ORG_B_ROWS"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	failed := clearQuarantinedMirror(tenantTestResult{OK: true, tested: tn})
	_ = os.Chmod(dir, 0o700)
	if failed.OK || !strings.Contains(failed.Error, "still quarantined") {
		t.Fatalf("a failed move must keep the quarantine: %+v", failed)
	}
	if got := clearQuarantinedMirror(tenantTestResult{OK: true, tested: tn}); !got.OK {
		t.Fatalf("a successful move lifts: %+v", got)
	}
	if _, err := os.Stat(mirror); !os.IsNotExist(err) {
		t.Fatalf("the mirror must be off its live path before the lift: %v", err)
	}
}

// Hand-fix (handfixes.json tenants-test-checks-domain): tenants test writes
// the registry and can move a mirror, so it is never an MCP read-only tool.
func TestTenantsTestIsNotMCPReadOnly(t *testing.T) {
	if v, ok := newTenantsTestCmd(&rootFlags{}).Annotations["mcp:read-only"]; ok {
		t.Fatalf("tenants test must carry no mcp:read-only annotation, got %q", v)
	}
}

// Hand-fix (handfixes.json novel-review-round16-small): an --events list
// that names nothing is refused, and a valid one is not.
func TestLoginAlertsEventsSelector(t *testing.T) {
	home := t.TempDir()
	restore, err := cliutil.SetHomeOverride(home)
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	run := func(events string) error {
		root := RootCmd()
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs([]string{"login-alerts", "--events", events, "--home", home})
		return root.Execute()
	}
	if err := run(",,"); err == nil || !strings.Contains(err.Error(), "names no event") {
		t.Fatalf("--events ',,' must be refused: %v", err)
	}
	for _, valid := range []string{"suspicious_login", "suspicious_login,2sv_disable"} {
		if err := run(valid); err != nil && strings.Contains(err.Error(), "names no event") {
			t.Fatalf("--events %s is valid: %v", valid, err)
		}
	}
	if err := run("bogus"); err == nil || !strings.Contains(err.Error(), "unknown --events bogus") {
		t.Fatalf("an unknown event must be named: %v", err)
	}
}

// Hand-fix (handfixes.json novel-review-round16-small): offboard refuses an
// empty --products list before any step runs.
func TestOffboardEmptyProductsRefusedUpFront(t *testing.T) {
	home := t.TempDir()
	restore, err := cliutil.SetHomeOverride(home)
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	root := RootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"offboard", "jane@acme.com", "--products", ",,", "--yes", "--home", home})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "names no licence product") {
		t.Fatalf("offboard --products ',,' must be refused up front: %v", err)
	}
}

// Hand-fix (handfixes.json novel-review-round16-small): posture enforces the
// same threshold bounds as stale-licenses.
func TestPostureThresholdsValidated(t *testing.T) {
	home := t.TempDir()
	restore, err := cliutil.SetHomeOverride(home)
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	for _, bad := range [][]string{{"--inactive-days", "-1"}, {"--grace-days", "-1"}, {"--min-2sv-pct", "NaN"}, {"--min-2sv-pct", "101"}} {
		root := RootCmd()
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs(append([]string{"posture", "--home", home}, bad...))
		if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "--inactive-days must be >= 1") {
			t.Fatalf("posture %v must be refused: %v", bad, err)
		}
	}
}

// Hand-fix (handfixes.json verification-codes-never-cached): a user's live
// backup codes are never written to the on-disk response cache.
func TestVerificationCodesNeverCached(t *testing.T) {
	home := t.TempDir()
	restore, err := cliutil.SetHomeOverride(home)
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"kind":"admin#directory#verificationCodesList","items":[{"verificationCode":"BACKUP_CODE_SENTINEL"}]}`)
	}))
	defer srv.Close()
	t.Setenv("GOOGLE_WORKSPACE_BASE_URL", srv.URL)
	t.Setenv("GOOGLE_WORKSPACE_TOKEN", "test-token")
	root := RootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"verification-codes", "list", "jane@acme.com", "--json", "--home", home})
	if err := root.Execute(); err != nil || !strings.Contains(out.String(), "BACKUP_CODE_SENTINEL") {
		t.Fatalf("the stub must be read: %v %s", err, out.String())
	}
	_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if b, _ := os.ReadFile(p); bytes.Contains(b, []byte("BACKUP_CODE_SENTINEL")) {
				t.Errorf("backup codes written to disk: %s", p)
			}
		}
		return nil
	})
}

func TestEmptyTenantsSelectorRefused(t *testing.T) {
	restore, err := cliutil.SetHomeOverride(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	ff := fanoutFlags{tenants: ",,"}
	if _, err := ff.targets(""); err == nil || !strings.Contains(err.Error(), "names no tenant") {
		t.Fatalf("--tenants ',,' must be refused: %v", err)
	}
	if err := updateTenantRegistry(func(reg *tenantRegistry) error {
		reg.Tenants = append(reg.Tenants, gwsTenant{Name: "acme", Domain: "acme.com", KeyFile: "k.json"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	dup := fanoutFlags{tenants: "acme,acme"}
	if got, err := dup.targets(""); err != nil || len(got) != 1 {
		t.Fatalf("--tenants acme,acme must query acme once: %v %v", got, err)
	}
	if _, err := listProductLicenses(context.Background(), nil, "C0", ",,"); err == nil || !strings.Contains(err.Error(), "names no licence product") {
		t.Fatalf("--products ',,' must be refused: %v", err)
	}
}

// Hand-fix (handfixes.json tenants-test-checks-domain): a quarantined
// tenant's local mirror is not served either.
func TestQuarantinedTenantLocalReadRefused(t *testing.T) {
	restore, err := cliutil.SetHomeOverride(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	if err := updateTenantRegistry(func(reg *tenantRegistry) error {
		reg.Tenants = append(reg.Tenants, gwsTenant{Name: "acme", Domain: "example.com", KeyFile: "k.json", OwnershipMismatch: "wrong customer"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	defer func() { tenantFlag = ""; resetTenantSelection(); setDefaultDBScopeIdentity("", "", "") }()
	tenantFlag = "acme"
	resetTenantSelection()
	configureDefaultDBScope("")
	if err := localStoreTenantErr(); err == nil || !strings.Contains(err.Error(), "quarantined") {
		t.Fatalf("a quarantined tenant's mirror must not be read: %v", err)
	}
	// Every learn/playbook opener resolves its store through learnDBPath.
	if _, err := learnDBPath(""); err == nil || !strings.Contains(err.Error(), "quarantined") {
		t.Fatalf("the default learn store of a quarantined tenant must be refused: %v", err)
	}
}
