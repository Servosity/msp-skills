// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"

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
	pl := publicLinks("a", acts)
	if len(pl) != 1 || pl[0].To != "public_on_the_web" {
		t.Fatalf("public links: %+v", pl)
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
		"https://mail.google.com/":           3,
		scopePrefix + "drive":                3,
		scopePrefix + "admin.directory.user": 3,
		scopePrefix + "drive.readonly":       2,
		scopePrefix + "calendar":             2,
		scopePrefix + "drive.file":           1,
		"openid":                             1,
		scopePrefix + "userinfo.email":       1,
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
	mixed := []map[string]any{pol(map[string]any{"externalSharingMode": "ALLOWED"}), pol(map[string]any{"external_sharing_mode": "DISALLOWED"})}
	if got := summarizePolicyValues(drive, mixed, testNow); got != "mixed:ALLOWED,DISALLOWED" {
		t.Fatal(got)
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

	t.Setenv("GOOGLE_WORKSPACE_TOKEN", "own-org-token")
	resetTenantSelection()
	configureDefaultDBScope("")
	if err := localStoreTenantErr(); err != nil {
		t.Fatalf("a bearer token keys the store to its own organization: %v", err)
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
		key := `{"client_email":"sa@example.iam.gserviceaccount.com","private_key":"-----BEGIN PRIVATE KEY-----\nAA==\n-----END PRIVATE KEY-----\n","private_key_id":"` + id + `"}`
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
