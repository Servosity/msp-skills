// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sonicwall-pp-cli/internal/client"
	"sonicwall-pp-cli/internal/cliutil/testenv"
	"sonicwall-pp-cli/internal/config"
)

// Spec-shaped NSM fixtures (field names from the vendor NSM API Gateway and
// SonicOS passthrough Swagger). Firewall A is behind, vulnerable, WAN-exposed
// SSL VPN with one SSL-VPN user missing OTP and admin TOTP off. Firewall B is
// current and clean.
const (
	serialA = "0040AAAA0001"
	serialB = "0040AAAA0002"
	serialC = "0040AAAA0003" // every per-device read is unreadable or malformed
)

func newMockNSM(t *testing.T) *httptest.Server {
	t.Helper()
	sonicos := map[string]map[string]string{
		serialA: {
			"/firewall/administration/global":       `{"administration":{"admin":{"one_time_password":null}}}`,
			"/firewall/ssl-vpn/server/accesses":     `{"ssl_vpn":{"server":{"access":[{"zone":"WAN","enable":true},{"zone":"LAN","enable":true}]}}}`,
			"/firewall/ssl-vpn/server/base":         `{"ssl_vpn":{"server":{"auth_type":"password","use_radius":{},"port":4433}}}`,
			"/firewall/user/authentication/methods": `{"user":{"auth":{"auth_method":"local"}}}`,
			"/firewall/user/local/users":            `{"user":{"local":{"user":[{"name":"alice","member_of":[{"name":"SSLVPN Services"}],"one_time_password":null},{"name":"bob","member_of":[{"name":"SSLVPN Services"}],"one_time_password":{"totp":true}}]}}}`,
			"/firewall/user/local/groups":           `{"user":{"local":{"group":[{"name":"SSLVPN Services","member":[],"one_time_password":null}]}}}`,
		},
		serialB: {
			"/firewall/administration/global":   `{"administration":{"admin":{"one_time_password":{"totp":true}}}}`,
			"/firewall/ssl-vpn/server/accesses": `{"ssl_vpn":{"server":{"access":[{"zone":"WAN","enable":false}]}}}`,
		},
	}
	mux := http.NewServeMux()
	write := func(w http.ResponseWriter, body string) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}
	mux.HandleFunc("/devices/tenant", func(w http.ResponseWriter, r *http.Request) {
		write(w, `[{"serialNumber":"`+serialA+`","friendlyName":"HQ","firmwareVersion":"7.0.1-5145","liveStatus":true,"tenantId":"t1"},
		           {"serialNumber":"`+serialB+`","friendlyName":"Branch","firmwareVersion":"7.1.1-7058","liveStatus":true,"tenantId":"t2"},
		           {"serialNumber":"`+serialC+`","friendlyName":"Lab","firmwareVersion":"7.1.1-7058","liveStatus":true,"tenantId":"t2"}]`)
	})
	mux.HandleFunc("/devices/vulnerable", func(w http.ResponseWriter, r *http.Request) {
		write(w, `[{"serialNumber":"`+serialA+`","friendlyName":"HQ","vulnList":[{"advisoryID":"SNWLID-2024-0015","title":"SonicOS Improper Access Control Vulnerability","impact":"Critical","haveWorkaroud":true,"noticeLink":"https://psirt.global.sonicwall.com/vuln-detail/SNWLID-2024-0015"}]}]`)
	})
	mux.HandleFunc("/devices/vulnerableFirmware", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("firmwareVersion") == "7.0.1-5145" {
			write(w, `[{"advisoryID":"SNWLID-2024-0015","title":"SonicOS Improper Access Control Vulnerability"}]`)
			return
		}
		write(w, `[]`)
	})
	mux.HandleFunc("/devices/availableVersions/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, serialA) {
			write(w, `{"availableVersions":[{"version":"7.0.1-5161","releaseDate":"2024-10-01"},{"version":"7.1.1-7058","releaseDate":"2025-01-15"}]}`)
			return
		}
		if strings.HasSuffix(r.URL.Path, serialC) {
			write(w, `{}`) // malformed: no availableVersions collection
			return
		}
		write(w, `{"availableVersions":[]}`)
	})
	mux.HandleFunc("/devices/productLifeCycle/", func(w http.ResponseWriter, r *http.Request) {
		write(w, `{"model":"TZ370","eos":"2031-01-01"}`)
	})
	mux.HandleFunc("/firewall/", func(w http.ResponseWriter, r *http.Request) {
		body, ok := sonicos[r.Header.Get("X-DEVICE-ID")][r.URL.Path]
		if !ok {
			// NSM's in-band failure shape: HTTP 200 + success:false.
			write(w, `{"status":{"info":[{"code":"E_FAIL","message":"no config"}],"success":false}}`)
			return
		}
		write(w, body)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func runCLI(t *testing.T, srv *httptest.Server, args ...string) (string, error) {
	t.Helper()
	t.Setenv("SONICWALL_BASE_URL", srv.URL)
	t.Setenv("SONICWALL_NSM_TOKEN", "test-token")
	cmd := RootCmd()
	cmd.SetArgs(append(args, "--json", "--no-cache"))
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	err := cmd.Execute()
	return out.String(), err
}

func decodeResults(t *testing.T, out string, v any) {
	t.Helper()
	var env struct {
		Results json.RawMessage `json:"results"`
	}
	data := []byte(out)
	if json.Unmarshal(data, &env) == nil && len(env.Results) > 0 {
		data = env.Results
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("decode %T: %v\noutput:\n%s", v, err, out)
	}
}

func TestExposureRanksAndTriages(t *testing.T) {
	testenv.Isolate(t)
	srv := newMockNSM(t)

	out, err := runCLI(t, srv, "exposure")
	if err != nil {
		t.Fatalf("exposure: %v", err)
	}
	var ranked []advisoryRank
	decodeResults(t, out, &ranked)
	if len(ranked) != 1 || ranked[0].AdvisoryID != "SNWLID-2024-0015" || ranked[0].FirewallsHit != 1 || ranked[0].HasWorkaround == nil || !*ranked[0].HasWorkaround {
		t.Fatalf("ranked = %+v", ranked)
	}

	out, err = runCLI(t, srv, "exposure", "snwlid-2024-0015")
	if err != nil {
		t.Fatalf("exposure id: %v", err)
	}
	var hits []exposureRow
	decodeResults(t, out, &hits)
	if len(hits) != 1 || hits[0].Serial != serialA || hits[0].WANReachable != "yes" || hits[0].TenantID != "t1" {
		t.Fatalf("hits = %+v", hits)
	}

	out, err = runCLI(t, srv, "exposure", "CVE-1999-0001")
	if err != nil {
		t.Fatalf("exposure no-match: %v", err)
	}
	hits = nil
	decodeResults(t, out, &hits)
	if len(hits) != 0 {
		t.Fatalf("unmatched query should return no rows, got %+v", hits)
	}
}

func TestFirmwareDrift(t *testing.T) {
	testenv.Isolate(t)
	srv := newMockNSM(t)
	out, err := runCLI(t, srv, "firmware", "drift")
	if err != nil {
		t.Fatalf("firmware drift: %v", err)
	}
	var rows []driftRow
	decodeResults(t, out, &rows)
	if len(rows) != 3 {
		t.Fatalf("rows = %+v", rows)
	}
	a, b, c := rows[0], rows[1], rows[2]
	if a.Serial != serialA || a.Status != "behind" || a.VersionsBehind == nil || *a.VersionsBehind != 2 || a.Latest != "7.1.1-7058" ||
		strings.Join(a.AdvisoriesCleared, ",") != "SNWLID-2024-0015" || a.LatestReleaseDate != "2025-01-15" || a.AdvisoryStatus != "fail" {
		t.Fatalf("A = %+v", a)
	}
	if b.Serial != serialB || b.Status != "current" || len(b.AdvisoriesCurrent) != 0 || b.AdvisoryStatus != "pass" {
		t.Fatalf("B = %+v", b)
	}
	// Malformed availableVersions must be unknown, never "current".
	if c.Serial != serialC || c.Status != "unknown" || c.VersionsBehind != nil || !strings.Contains(c.Error, "unexpected response shape") {
		t.Fatalf("C = %+v", c)
	}

	out, err = runCLI(t, srv, "firmware", "drift", "--behind-only")
	if err != nil {
		t.Fatalf("firmware drift --behind-only: %v", err)
	}
	rows = nil
	decodeResults(t, out, &rows)
	if len(rows) != 2 || rows[0].Serial != serialA || rows[1].Serial != serialC {
		t.Fatalf("--behind-only should keep A (behind) and C (unknown), got %+v", rows)
	}
}

func TestMFAAudit(t *testing.T) {
	testenv.Isolate(t)
	srv := newMockNSM(t)
	out, err := runCLI(t, srv, "mfa-audit")
	if err != nil {
		t.Fatalf("mfa-audit: %v", err)
	}
	var rows []mfaRow
	decodeResults(t, out, &rows)
	got := map[string]string{}
	for _, r := range rows {
		got[r.Serial+"/"+r.Account] = r.Status
	}
	if got[serialA+"/administrators (TOTP policy)"] != "fail" || got[serialA+"/alice"] != "fail" {
		t.Fatalf("expected admin and alice failures on A, got %v", got)
	}
	if _, listed := got[serialA+"/bob"]; listed {
		t.Fatalf("bob has TOTP and should be omitted without --all: %v", got)
	}
	if _, listed := got[serialB+"/administrators (TOTP policy)"]; listed {
		t.Fatalf("B admin TOTP passes and should be omitted: %v", got)
	}
	if got[serialB+"/local users"] != "unknown" {
		t.Fatalf("B local users unreadable should be unknown, got %v", got)
	}
	if got[serialC+"/administrators (TOTP policy)"] != "unknown" {
		t.Fatalf("C admin settings unreadable should be unknown, got %v", got)
	}
}

func TestPostureAndChanges(t *testing.T) {
	testenv.Isolate(t)
	srv := newMockNSM(t)
	db := filepath.Join(t.TempDir(), "posture.db")
	out, err := runCLI(t, srv, "posture", "--db", db)
	if err != nil {
		t.Fatalf("posture: %v", err)
	}
	var rows []postureRow
	decodeResults(t, out, &rows)
	if len(rows) != 3 {
		t.Fatalf("rows = %+v", rows)
	}
	a, b, c := rows[0], rows[1], rows[2]
	if a.Overall != "fail" || a.AdminTOTP != "fail" || a.AdvisoryStatus != "fail" || a.SSLVPNWAN != "warn" ||
		a.UserMFA != "fail" || a.UsersWithoutMFA == nil || *a.UsersWithoutMFA != 1 || a.FirmwareStatus != "warn" {
		t.Fatalf("A = %+v", a)
	}
	// No MySonicWall key in the test: licences must be unknown, never pass.
	if a.LicenceStatus != "unknown" || b.LicenceStatus != "unknown" {
		t.Fatalf("licence status without MSW key: A=%q B=%q", a.LicenceStatus, b.LicenceStatus)
	}
	if b.AdminTOTP != "pass" || b.SSLVPNWAN != "pass" || b.UserMFA != "pass" || b.AdvisoryStatus != "pass" || b.FirmwareStatus != "pass" || b.Overall != "unknown" {
		t.Fatalf("B = %+v", b)
	}
	// C: every per-device read is unreadable; nothing may come back as pass.
	if c.FirmwareStatus != "unknown" || c.AdminTOTP != "unknown" || c.SSLVPNWAN != "unknown" || c.UserMFA != "unknown" || c.Overall != "unknown" || len(c.Errors) == 0 {
		t.Fatalf("C = %+v", c)
	}

	// A single-firewall run is a different scope and must not be diffed against the fleet run.
	if _, err := runCLI(t, srv, "posture", "--serial", serialB, "--db", db); err != nil {
		t.Fatalf("posture --serial: %v", err)
	}
	// Seed an older fleet run where A's admin TOTP passed, then diff.
	older := []postureRow{a, b, c}
	older[0].AdminTOTP = "pass"
	if err := savePostureSnapshot(context.Background(), db, time.Now().Add(-48*time.Hour), "fleet", true, older); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if _, err := runCLI(t, srv, "posture", "--db", db); err != nil {
		t.Fatalf("second fleet posture: %v", err)
	}
	out, err = runCLI(t, srv, "posture", "changes", "--since", "24h", "--db", db)
	if err != nil {
		t.Fatalf("posture changes: %v", err)
	}
	var res postureChangesResult
	decodeResults(t, out, &res)
	if res.Scope != "fleet" {
		t.Fatalf("changes should compare fleet runs, got scope %q", res.Scope)
	}
	found := false
	for _, ch := range res.Changes {
		if ch.Check == "present" {
			t.Fatalf("same-scope runs must not report presence changes: %+v", res.Changes)
		}
		if ch.Serial == serialA && ch.Check == "admin_totp" && ch.Before == "pass" && ch.After == "fail" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected A admin_totp pass->fail, got %+v", res)
	}
}

func TestPostureChangesEmptyStore(t *testing.T) {
	testenv.Isolate(t)
	srv := newMockNSM(t)
	out, err := runCLI(t, srv, "posture", "changes", "--db", filepath.Join(t.TempDir(), "none.db"))
	if err != nil {
		t.Fatalf("posture changes on empty store should exit 0: %v", err)
	}
	var res postureChangesResult
	decodeResults(t, out, &res)
	if len(res.Changes) != 0 || res.Note == "" {
		t.Fatalf("empty store result = %+v", res)
	}
}

func TestNSMLoginCandidates(t *testing.T) {
	var tenants []nsmCloudTenant
	raw := `[{"name":"Acme","productGroupID":123,"cloudServices":[{"serviceName":"CC","tenantSerial":"CCSER"},{"serviceName":"NSM","tenantSerial":"NSMSER"}]},
	         {"name":"NoNSM","productGroupID":9,"cloudServices":[{"serviceName":"X"}]}]`
	if err := json.Unmarshal([]byte(raw), &tenants); err != nil {
		t.Fatal(err)
	}
	cands := nsmLoginCandidates(tenants)
	if len(cands) != 1 || cands[0].TenantSerial != "NSMSER" || cands[0].ProductGroupID != "123" || cands[0].MatchedBy != "service-name" {
		t.Fatalf("candidates = %+v", cands)
	}
	// A tenant with only a non-NSM serial is never picked automatically.
	other := []nsmLoginCandidate{{Name: "Other", ProductGroupID: "5", TenantSerial: "CC1", MatchedBy: "first-serial"}}
	if _, err := pickNSMTenant(other, ""); err == nil {
		t.Fatal("fallback-only candidates must require --tenant")
	}
	if got, err := pickNSMTenant(other, "Other"); err != nil || got.TenantSerial != "CC1" {
		t.Fatalf("explicit --tenant on fallback = %+v, %v", got, err)
	}
	mixed := []nsmLoginCandidate{cands[0], other[0]}
	if got, err := pickNSMTenant(mixed, ""); err != nil || got.Name != "Acme" {
		t.Fatalf("NSM-named candidate should win automatically, got %+v, %v", got, err)
	}
	if got, err := pickNSMTenant(cands, ""); err != nil || got.Name != "Acme" {
		t.Fatalf("pick single = %+v, %v", got, err)
	}
	if _, err := pickNSMTenant(cands, "Other"); err == nil {
		t.Fatal("unknown --tenant should fail")
	}
	two := append(cands, nsmLoginCandidate{Name: "Beta", ProductGroupID: "7", TenantSerial: "S2", MatchedBy: "service-name"})
	if _, err := pickNSMTenant(two, ""); err == nil {
		t.Fatal("two tenants without --tenant should fail")
	}
	if got, err := pickNSMTenant(two, "7"); err != nil || got.Name != "Beta" {
		t.Fatalf("pick by product group = %+v, %v", got, err)
	}
}

func TestNSMGetPath(t *testing.T) {
	cases := map[string]string{"/devices/summary": "/devices/summary", "/firewall/user/authentication/base": "/firewall/user/authentication/base", "devices/summary": "/devices/summary", "/api/manager/tenants": "/tenants",
		"/devices//summary": "/devices/summary", "/aar/api/v1/firewall": "/aar/api/v1/firewall"}
	for in, want := range cases {
		if got, err := nsmGetPath(in); err != nil || got != want {
			t.Errorf("nsmGetPath(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"https://evil.example/x", "//evil.example/x", "", "/api/hgms/get-cloud-tenants", "api/users/get-user-list",
		"/%61pi/hgms/get-cloud-tenants", "/api/manager//api/hgms/get-cloud-tenants", "/devices/../api/hgms/get-cloud-tenants", "/devices?x=1",
		"/auth/tempToken", "/system/licenses/getNSMAuthCode", "/users/u1/totp", "/system/cluster/k8s-dashboard/token",
		"/devices/export/certificates/cert-key-pair/name/n/password/p", "/firewall/export/certificates/cert-key-pair/name/n/password/p",
		"/system/licenses/systemResetLicenses", "/ha/synchronizesettings", "/not/a/vendor/route", "/devices/export",
		"/configaudit/0040AAAA0001/latest", "/configaudit/diff"} {
		if _, err := nsmGetPath(bad); err == nil {
			t.Errorf("nsmGetPath(%q) should fail", bad)
		}
	}
}

func TestDecodeList(t *testing.T) {
	ok := []struct {
		raw  string
		want int
	}{
		{`[{"serialNumber":"A"},{"serialNumber":"B"}]`, 2},
		{`{"devices":[{"serialNumber":"A"}]}`, 1},
		{`[]`, 0},
	}
	for _, c := range ok {
		var out []fleetDevice
		if err := decodeList(json.RawMessage(c.raw), &out); err != nil || len(out) != c.want {
			t.Errorf("decodeList(%s) = %d rows, err %v; want %d", c.raw, len(out), err, c.want)
		}
	}
	// Shapes that are not a list must fail, never become an empty or one-row list.
	for _, bad := range []string{`{"serialNumber":"A"}`, `{}`, `null`, ``, `"oops"`} {
		var out []fleetDevice
		if err := decodeList(json.RawMessage(bad), &out); err == nil {
			t.Errorf("decodeList(%q) should fail, got %d rows", bad, len(out))
		}
	}
}

func TestPostureChangesSeesEmptiedFleet(t *testing.T) {
	testenv.Isolate(t)
	srv := newMockNSM(t)
	db := filepath.Join(t.TempDir(), "posture.db")
	online := true
	row := postureRow{Serial: serialA, Name: "HQ", Online: &online, FirmwareStatus: "pass", AdvisoryStatus: "pass", SupportStatus: "pass",
		LicenceStatus: "pass", AdminTOTP: "pass", SSLVPNWAN: "pass", UserMFA: "pass"}
	if err := savePostureSnapshot(context.Background(), db, time.Now().Add(-48*time.Hour), "fleet", true, []postureRow{row}); err != nil {
		t.Fatal(err)
	}
	if err := savePostureSnapshot(context.Background(), db, time.Now(), "fleet", true, nil); err != nil {
		t.Fatal(err)
	}
	out, err := runCLI(t, srv, "posture", "changes", "--since", "24h", "--db", db)
	if err != nil {
		t.Fatalf("posture changes: %v", err)
	}
	var res postureChangesResult
	decodeResults(t, out, &res)
	if len(res.Changes) != 1 || res.Changes[0].Check != "present" || res.Changes[0].After != "absent" {
		t.Fatalf("an emptied fleet must show the firewall as absent, got %+v", res)
	}
}

func TestFetchVulnerableRejectsNullVulnList(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `[{"serialNumber":"0040AAAA0001","vulnList":null}]`)
	}))
	defer srv.Close()
	_, err := runCLI(t, srv, "exposure")
	if err == nil {
		t.Fatal("a null vulnList must be an error, not 'no advisories'")
	}
}

func TestMSWStatusError(t *testing.T) {
	fails := []string{
		`{"operationStatus":"Failure","msgCode":7,"content":{"totalProducts":"0"}}`,
		`{"operationStatus":"Failure","msgCode":"E1","msgDesc":"denied"}`,
		`{"operationStatus":"Error"}`,
	}
	for _, raw := range fails {
		if mswStatusError(json.RawMessage(raw)) == nil {
			t.Errorf("mswStatusError(%s) = nil, want an error", raw)
		}
	}
	for _, raw := range []string{`{"operationStatus":"Success","content":{}}`, `{"content":{}}`, `[1]`} {
		if err := mswStatusError(json.RawMessage(raw)); err != nil {
			t.Errorf("mswStatusError(%s) = %v, want nil", raw, err)
		}
	}
}

func TestRedactCredentials(t *testing.T) {
	var v any
	raw := `{"counts":1,"reports":[{"deviceID":"0040AAAA0001","deviceUserName":"admin","devicePassword":"credential"}],"tempToken":"tt","nested":{"pre_shared_key":"k","name":"x"},"appflow":{"sfr_mailing":{"pop_pass":"pp","smtp_pass":"sp","bypass_rules":"keep"}},"ipsec":{"authentication_key":"ak","encryption_key":"ek","in_authentication_key":"ik"}}`
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(redactCredentials(v))
	for _, secret := range []string{`"credential"`, `"tt"`, `"k"`, `"pp"`, `"sp"`, `"ak"`, `"ek"`, `"ik"`} {
		if strings.Contains(string(out), secret) {
			t.Fatalf("secret %s survived redaction: %s", secret, out)
		}
	}
	var nestedSecret any
	_ = json.Unmarshal([]byte(`{"cli":{"ftp":{"password":{"default":"ftp-credential"}}},"wep_key":{"key1":{"value":"wep-credential"}},"psk_list":["s1","s2"]}`), &nestedSecret)
	nestedOut, _ := json.Marshal(redactCredentials(nestedSecret))
	for _, secret := range []string{"ftp-credential", "wep-credential", `"s1"`, `"s2"`} {
		if strings.Contains(string(nestedOut), secret) {
			t.Fatalf("secret %s under a credential container survived: %s", secret, nestedOut)
		}
	}
	var ssl any
	_ = json.Unmarshal([]byte(`{"ssl_vpn":{"server":{"auth_type":"password","port":4433}},"user":{"local":{"user":[{"name":"alice","type":"local","id":7}]}}}`), &ssl)
	sslOut, _ := json.Marshal(redactCredentials(ssl))
	for _, kept := range []string{`"auth_type":"password"`, `4433`, `"alice"`, `"type":"local"`, `"id":7`} {
		if !strings.Contains(string(sslOut), kept) {
			t.Fatalf("ordinary SonicOS fields must survive redaction, lost %s: %s", kept, sslOut)
		}
	}
	for _, kept := range []string{`"admin"`, `"0040AAAA0001"`, `"x"`, `"keep"`} {
		if !strings.Contains(string(out), kept) {
			t.Fatalf("non-secret %s was removed: %s", kept, out)
		}
	}
}

func TestBlindSpotsRejectsShortRegistry(t *testing.T) {
	if n, ok := mswReportedCount(json.RawMessage(`"1"`), json.RawMessage(`"9"`)); !ok || n != 1 {
		t.Fatalf("mswReportedCount = %d,%v want 1 (filterResultCount first)", n, ok)
	}
	if _, ok := mswReportedCount(json.RawMessage(`null`), json.RawMessage(``)); ok {
		t.Fatal("no numeric count should be ok=false")
	}
}

// hostRedirect sends every request to target, so absolute MySonicWall URLs can be tested offline.
type hostRedirect struct{ target *url.URL }

func (h hostRedirect) RoundTrip(req *http.Request) (*http.Response, error) {
	r := req.Clone(req.Context())
	r.URL.Scheme, r.URL.Host, r.Host = h.target.Scheme, h.target.Host, h.target.Host
	return http.DefaultTransport.RoundTrip(r)
}

func TestFetchMSWProductsRejectsShortRegistry(t *testing.T) {
	testenv.Isolate(t)
	body := `{"operationStatus":"Success","content":{"totalProducts":"1","filterResultCount":"1","prodList":[]}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()
	target, _ := url.Parse(srv.URL)
	t.Setenv("SONICWALL_MSW_API_KEY", "test-key")
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	c := client.New(cfg, 10*time.Second, 0)
	c.HTTPClient.Transport = hostRedirect{target: target}
	c.NoCache = true
	if _, err := fetchMSWProducts(context.Background(), c); err == nil || !strings.Contains(err.Error(), "reported 1 products") {
		t.Fatalf("a registry reporting 1 product with an empty list must fail, got %v", err)
	}
	body = `{"operationStatus":"Success","content":{"totalProducts":"0","filterResultCount":"0","prodList":[]}}`
	if got, err := fetchMSWProducts(context.Background(), c); err != nil || len(got) != 0 {
		t.Fatalf("an explicitly empty registry should read as empty, got %v, %v", got, err)
	}
}

func TestIdempotentDoesNotMaskFailedRead(t *testing.T) {
	testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(w, `{"message":"inventory is locked"}`)
	}))
	defer srv.Close()
	if _, err := runCLI(t, srv, "posture", "--idempotent", "--db", filepath.Join(t.TempDir(), "p.db")); err == nil {
		t.Fatal("a 409 on the inventory read must fail even with --idempotent")
	}
}
