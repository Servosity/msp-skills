// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sonicwall-pp-cli/internal/cliutil/testenv"
	"sonicwall-pp-cli/internal/posture"
	"sonicwall-pp-cli/internal/store"
)

// An NSM failure envelope that echoes a secret is redacted inside the 401 text
// the envelope hook builds, and keeps both the 401 and the nsm-login hint; a
// non-JSON error body naming a secret keeps its status and loses its body.
func TestRedactEnvelopeAndErrorBodies(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tenants":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"status":{"success":false,"info":[{"code":"E_INVALIDAUTH","message":"bad","password":"`+redactSentinel+`"}]}}`)
		default:
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `oops "password": `+redactSentinel)
		}
	}))
	defer srv.Close()
	hc := &http.Client{Transport: nsmEnvelopeTransport{base: sonicOSRedactTransport{base: http.DefaultTransport}}}
	for path, want := range map[string]int{"/tenants": http.StatusUnauthorized, "/boom": http.StatusInternalServerError} {
		resp, err := hc.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != want || strings.Contains(string(b), redactSentinel) {
			t.Fatalf("%s: status %d body %q, want %d without the secret", path, resp.StatusCode, b, want)
		}
		if path == "/tenants" && !strings.Contains(string(b), "nsm-login") {
			t.Fatalf("%s: body %q lost the nsm-login hint", path, b)
		}
	}
}

func TestDoctorProbeBodyHasData(t *testing.T) {
	for body, want := range map[string]bool{`{}`: false, ``: false, `<html></html>`: false, `[]`: false,
		`[{"id":1}]`: false, `[{"id":"t1","name":"Acme"}]`: true, `[{"id":"t1"},{"name":"x"}]`: false,
		`{"data":[]}`: true, `{"data":[],"items":[],"results":[]}`: true, `{"error":"unauthorized"}`: false,
		`{"error":[]}`: false, `{"data":[],"error":"x"}`: false, `{"a":1}{"b":2}`: false} {
		if got := doctorProbeBodyHasData([]byte(body)); got != want {
			t.Fatalf("doctorProbeBodyHasData(%q) = %v, want %v", body, got, want)
		}
	}
}

// An empty running firmware must never turn into "no advisories" (a pass).
func TestAdvisoriesForEmptyFirmwareIsUnknown(t *testing.T) {
	for _, fw := range []string{"  ", "N/A", "unknown", "SonicOS"} {
		if _, err := advisoriesForFirmware(context.Background(), nil, fw); err == nil {
			t.Fatalf("advisoriesForFirmware(%q) must fail so the caller reports unknown", fw)
		}
	}
	for fw, want := range map[string]string{"7.0.1-5035": "7.0.1-5035", "SonicOS 6.5.4.4-44n": "6.5.4.4-44n", "7.1.1-7051-R5175": "7.1.1-7051-R5175"} {
		if got := sonicOSVersionPattern.FindString(fw); got != want {
			t.Fatalf("version in %q = %q, want %q", fw, got, want)
		}
	}
}

// A device NSM did not report as online or offline is unknown, never offline,
// and never lets the row grade pass.
func TestPostureOnlineUnknown(t *testing.T) {
	r := finishRow(postureRow{FirmwareStatus: "pass", AdvisoryStatus: "pass", SupportStatus: "pass", LicenceStatus: "pass",
		AdminTOTP: "pass", SSLVPNWAN: "pass", UserMFA: "pass"})
	if r.checks()["online"] != "unknown" || r.Overall == "pass" {
		t.Fatalf("nil online: check %q overall %q, want unknown and not pass", r.checks()["online"], r.Overall)
	}
}

func TestSupportExpiredState(t *testing.T) {
	now := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	state := func(b *bool) string {
		if b == nil {
			return "unknown"
		}
		if *b {
			return "expired"
		}
		return "current"
	}
	cases := []struct {
		flag, expiry, want string
	}{
		{`null`, "2026-10-01", "expired"}, // Codex round 3: a null flag with a past date
		{`null`, "", "unknown"},
		{``, "", "unknown"},
		{`"false"`, "2030-01-01", "current"},
		{`"false"`, "2026-10-01", "expired"}, // the date outranks a stale flag
		{`true`, "", "expired"},
		{`false`, "", "current"},
	}
	for _, c := range cases {
		if got := state(supportExpiredState(json.RawMessage(c.flag), c.expiry, now)); got != c.want {
			t.Errorf("supportExpiredState(%s, %q) = %s, want %s", c.flag, c.expiry, got, c.want)
		}
	}
}

// Codex round 5: a vulnList entry without haveWorkaroud is "workaround
// unknown" (null), never "no workaround".
func TestAdvisoryWorkaroundUnreported(t *testing.T) {
	var v []struct {
		VulnList []advisory `json:"vulnList"`
	}
	if err := json.Unmarshal([]byte(`[{"serialNumber":"00000000000A","vulnList":[{"advisoryID":"SNWLID-2024-0015"}]}]`), &v); err != nil {
		t.Fatal(err)
	}
	if v[0].VulnList[0].HaveWorkaround != nil {
		t.Fatalf("absent haveWorkaroud = %v, want nil", *v[0].VulnList[0].HaveWorkaround)
	}
}

// Codex round 5: an SSL-VPN member whose one_time_password was not reported
// leaves the without-MFA count undecided, never a definite 0.
func TestSSLVPNMFACountUndecided(t *testing.T) {
	member := true
	missing, undecided := countSSLVPNMFA([]posture.Account{{Name: "alice", SSLVPNMember: &member, Method: "unknown"}})
	if missing != 0 || undecided != 1 {
		t.Fatalf("missing=%d undecided=%d, want 0 and 1", missing, undecided)
	}
}

// Adversarial round 7: when SAML or a directory signs users in, or the sign-in
// source was not reported, mfa-audit says so in its own row; local accounts
// passing never makes the firewall look fully read.
func TestMFAAuditSignInRow(t *testing.T) {
	base := map[string]string{
		"/firewall/administration/global": `{"administration":{"admin":{"one_time_password":{"totp":true}}}}`,
		"/firewall/ssl-vpn/server/base":   `{"ssl_vpn":{"server":{"auth_type":"password"}}}`,
		"/firewall/user/local/users":      `{"user":{"local":{"user":[{"name":"alice","one_time_password":{"totp":true},"member_of":[{"name":"SSLVPN Services"}]}]}}}`,
		"/firewall/user/local/groups":     `{"user":{"local":{"group":[{"name":"SSLVPN Services","one_time_password":{},"member":[{"name":"alice"}]}]}}}`,
	}
	cases := []struct {
		name, methods, server, wantKind, wantStatus string
	}{
		{"local", `{"user":{"auth":{"auth_method":"local"}}}`, "", "", ""},
		{"ldap only", `{"user":{"auth":{"auth_method":"ldap"}}}`, "", "directory", "warn"},
		{"radius mix", `{"user":{"auth":{"auth_method":"radius-local"}}}`, "", "directory", "warn"},
		{"unreported", `{"user":{"auth":{}}}`, "", "sign-in", "unknown"},
		{"undocumented", `{"user":{"auth":{"auth_method":"sso"}}}`, "", "sign-in", "unknown"},
		{"saml", `{"user":{"auth":{"auth_method":"local"}}}`, `{"ssl_vpn":{"server":{"auth_type":"saml"}}}`, "directory", "warn"},
		{"auth_type unreported", `{"user":{"auth":{"auth_method":"local"}}}`, `{"ssl_vpn":{"server":{}}}`, "sign-in", "unknown"},
		{"auth_type undocumented", `{"user":{"auth":{"auth_method":"local"}}}`, `{"ssl_vpn":{"server":{"auth_type":"sso"}}}`, "sign-in", "unknown"},
	}
	for _, c := range cases {
		routes := map[string]string{"/firewall/user/authentication/methods": c.methods}
		for k, v := range base {
			routes[k] = v
		}
		if c.server != "" {
			routes["/firewall/ssl-vpn/server/base"] = c.server
		}
		read := func(path string) ([]byte, error) { return []byte(routes[path]), nil }
		var kind, status string
		for _, r := range auditDeviceMFA(read, fleetDevice{SerialNumber: "00000000000A"}) {
			if r.Kind == "directory" || r.Kind == "sign-in" {
				kind, status = r.Kind, r.Status
			} else if r.Status != "pass" {
				t.Errorf("%s: %s row status %q, want pass", c.name, r.Account, r.Status)
			}
		}
		if kind != c.wantKind || status != c.wantStatus {
			t.Errorf("%s: sign-in row (%q,%q), want (%q,%q)", c.name, kind, status, c.wantKind, c.wantStatus)
		}
	}
}

// Adversarial rounds 8-9: a fleet report that matched no firewall must not
// answer with the empty list that reads as an all-clear; it exits 3 with a
// JSON error envelope.
func TestFleetReportsRefuseNoMatchingFirewall(t *testing.T) {
	testenv.Isolate(t)
	srv := newMockNSM(t)
	t.Setenv("SONICWALL_MSW_API_KEY", "test-key") // licenses renewals needs it before it lists firewalls
	for _, args := range [][]string{
		{"mfa-audit", "--serial", "00000000000Z"},
		{"mfa-audit", "--tenant", "nope"},
		{"posture", "--serial", "00000000000Z"},
		{"firmware", "drift", "--behind-only", "--tenant", "nope"},
		{"licenses", "renewals", "--tenant", "nope"},
	} {
		out, err := runCLI(t, srv, args...)
		if ExitCode(err) != 3 || !strings.Contains(out, `"code":3`) {
			t.Errorf("%v: exit %d, stdout %q; want exit 3 with a JSON error envelope", args, ExitCode(err), out)
		}
	}
}

// Adversarial round 9: report tables keep every column, in order, every run.
func TestPrintOrderedTableKeepsColumns(t *testing.T) {
	var buf strings.Builder
	cols := []string{"serial", "firewall", "account", "sslvpn", "mfa", "status", "note"}
	row := map[string]any{"serial": "S", "firewall": "F", "account": "alice", "sslvpn": "yes", "mfa": "none", "status": "fail", "note": "n"}
	if err := printOrderedTable(&buf, cols, []map[string]any{row}); err != nil {
		t.Fatal(err)
	}
	head := strings.Fields(strings.SplitN(buf.String(), "\n", 2)[0])
	if strings.Join(head, " ") != "SERIAL FIREWALL ACCOUNT SSLVPN MFA STATUS NOTE" {
		t.Fatalf("header = %v", head)
	}
}

// Adversarial round 10: on an empty NSM fleet, exposure refuses instead of an
// empty ranking, and posture still saves the empty run so posture changes
// lists every firewall as removed.
func TestEmptyFleetExposureAndPostureHistory(t *testing.T) {
	testenv.Isolate(t)
	full := newMockNSM(t)
	db := filepath.Join(t.TempDir(), "posture.db")
	if _, err := runCLI(t, full, "posture", "--db", db); err != nil {
		t.Fatalf("posture on the full fleet: %v", err)
	}
	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[]`)
	}))
	t.Cleanup(empty.Close)
	if out, err := runCLI(t, empty, "exposure"); ExitCode(err) != 3 {
		t.Errorf("exposure on an empty fleet: exit %d, stdout %q; want 3", ExitCode(err), out)
	}
	if _, err := runCLI(t, empty, "posture", "--db", db); ExitCode(err) != 3 {
		t.Fatalf("posture on an empty fleet: exit %d; want 3", ExitCode(err))
	}
	out, err := runCLI(t, empty, "posture", "changes", "--db", db)
	if err != nil {
		t.Fatalf("posture changes: %v", err)
	}
	if !strings.Contains(out, `"absent"`) {
		t.Fatalf("posture changes after the fleet emptied should list removals, got %s", out)
	}
}

// Codex round 11 + 17: a MySonicWall product with no product name or line, or
// a model this build does not recognise, is unclassified, never assumed not a
// firewall; "other" needs a positive match on a known non-firewall family.
func TestProductClass(t *testing.T) {
	for _, c := range []struct {
		p    mswProduct
		want string
	}{
		{mswProduct{ProductNameRaw: "TZ 370"}, "firewall"},
		{mswProduct{ProductNameRaw: "TZ370W"}, "firewall"},
		{mswProduct{ProductNameRaw: "NSa 2700"}, "firewall"},
		{mswProduct{ProductNameRaw: "NSA E7500"}, "firewall"}, // Codex round 17: the E-Class carries no digit after NSA
		{mswProduct{ProductNameRaw: "NSsp 15700"}, "firewall"},
		{mswProduct{ProductNameRaw: "NSv 270"}, "firewall"},
		{mswProduct{ProductNameRaw: "PRO 4060"}, "firewall"},
		{mswProduct{ProdLineDesc: "Gen 7 Firewalls"}, "firewall"},
		{mswProduct{ProductName: "SonicWall Switch SWS12-8"}, "other"},
		{mswProduct{ProductName: "SMA 500v"}, "other"},
		{mswProduct{ProductName: "SonicWave 641"}, "other"},
		{mswProduct{ProductName: "Capture Security Appliance CSa 1000"}, "other"},
		{mswProduct{ProductName: "Email Security ESA 5000"}, "other"},
		{mswProduct{SerNo: "00000000000A"}, "unknown"},
		{mswProduct{ProductNameRaw: "Frobnicator 9000"}, "unknown"}, // unrecognised model: listed, not dropped as "other"
		{mswProduct{ProductName: "Prometheus"}, "unknown"},          // "pro" needs a word boundary or a digit after it
	} {
		if got := productClass(c.p); got != c.want {
			t.Errorf("productClass(%+v) = %q, want %q", c.p, got, c.want)
		}
	}
}

// Adversarial round 11: an empty run is saved only when it empties a scope
// whose last run had firewalls; a filter that never matched leaves no history.
func TestEmptyPostureRunSavedOnlyWhenScopeEmptied(t *testing.T) {
	ctx := context.Background()
	db := filepath.Join(t.TempDir(), "posture.db")
	now := time.Now().UTC()
	count := func(scope string) int {
		s, err := store.OpenWithContext(ctx, db)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		var n int
		if err := s.DB().QueryRowContext(ctx, `SELECT count(*) FROM posture_runs WHERE scope = ?`, scope).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	steps := []struct {
		scope string
		rows  []postureRow
		want  int
	}{
		{"fleet", []postureRow{{Serial: "00000000000A"}}, 1},
		{"tenant:nope", nil, 0}, // never matched: not saved
		{"tenant:nope", nil, 0},
		{"fleet", nil, 2}, // empties a scope that had a firewall: saved
		{"fleet", nil, 2}, // already empty: not saved again
	}
	for i, st := range steps {
		if err := savePostureSnapshot(ctx, db, now.Add(time.Duration(i)*time.Second), st.scope, true, st.rows); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		if got := count(st.scope); got != st.want {
			t.Fatalf("step %d (%s): %d runs, want %d", i, st.scope, got, st.want)
		}
	}
}

// Adversarial round 12 / Codex round 13: --tenant is part of a --serial run's
// scope, so a mismatched tenant never empties the plain --serial history.
func TestPostureScopeKeepsBothFilters(t *testing.T) {
	if a, b := postureScope("", "0040aaaa0001"), postureScope("t2", "0040aaaa0001"); a == b {
		t.Fatalf("serial and tenant+serial runs share scope %q", a)
	}
	// Adversarial round 14: the fleet filter ignores case, so the scope does too.
	if a, b := postureScope("Ten-ABC", ""), postureScope("ten-abc", ""); a != b {
		t.Fatalf("tenant scope depends on case: %q vs %q", a, b)
	}
}

// Adversarial round 12: an empty MySonicWall service list is not "nothing
// expiring"; licenses renewals reports it instead of an all-clear.
func TestRenewalsEmptyServiceListNotAllClear(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/devices/tenant":
			_, _ = io.WriteString(w, `[{"serialNumber":"00000000000A","friendlyName":"HQ","tenantId":"t1","firmwareVersion":"7.0.1-5145"}]`)
		case r.URL.Path == "/api/product/serviceInfo":
			_, _ = io.WriteString(w, `{"serviceTypeList":[]}`)
		default:
			_, _ = io.WriteString(w, `{}`)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("SONICWALL_MSW_API_KEY", "test-key")
	if out, err := runCLI(t, srv, "licenses", "renewals"); err == nil {
		t.Fatalf("licenses renewals succeeded on an empty service list: %s", out)
	}
}

// Adversarial round 13: posture changes can name the scope, so a one-off
// --serial run never hides the fleet answer.
func TestPostureChangesScopeFlags(t *testing.T) {
	testenv.Isolate(t)
	srv := newMockNSM(t)
	ctx := context.Background()
	db := filepath.Join(t.TempDir(), "p.db")
	now := time.Now().UTC()
	runs := []struct {
		at    time.Duration
		scope string
		mfa   string
	}{
		{-72 * time.Hour, "fleet", "pass"},
		{-2 * time.Hour, "fleet", "fail"},
		{-1 * time.Hour, "serial:00000000000A", "fail"},
		{-30 * time.Minute, "serial:00000000000A", "fail"},
	}
	for _, r := range runs {
		row := postureRow{Serial: "00000000000A", Name: "HQ", AdminTOTP: r.mfa}
		if err := savePostureSnapshot(ctx, db, now.Add(r.at), r.scope, true, []postureRow{row}); err != nil {
			t.Fatal(err)
		}
	}
	var res postureChangesResult
	out, err := runCLI(t, srv, "posture", "changes", "--since", "24h", "--db", db)
	if err != nil {
		t.Fatal(err)
	}
	decodeResults(t, out, &res)
	if res.Scope != "serial:00000000000A" || len(res.Changes) != 0 {
		t.Fatalf("default should use the newest run's scope: %+v", res)
	}
	out, err = runCLI(t, srv, "posture", "changes", "--fleet", "--since", "24h", "--db", db)
	if err != nil {
		t.Fatal(err)
	}
	res = postureChangesResult{}
	decodeResults(t, out, &res)
	if res.Scope != "fleet" || len(res.Changes) != 1 || res.Changes[0].Check != "admin_totp" || res.Changes[0].After != "fail" {
		t.Fatalf("--fleet should diff the fleet runs: %+v", res)
	}
	out, err = runCLI(t, srv, "posture", "changes", "--serial", "00000000000b", "--db", db)
	if err != nil {
		t.Fatal(err)
	}
	res = postureChangesResult{}
	decodeResults(t, out, &res)
	if res.Scope != "serial:00000000000B" || res.Note == "" || len(res.Changes) != 0 {
		t.Fatalf("an unseen scope should say so: %+v", res)
	}
	if _, err := runCLI(t, srv, "posture", "changes", "--fleet", "--tenant", "t1", "--db", db); err == nil {
		t.Fatal("--fleet with --tenant should be a usage error")
	}
}
