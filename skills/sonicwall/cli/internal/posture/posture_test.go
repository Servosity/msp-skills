// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package posture

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestWorst(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{Pass, Pass}, Pass},
		{[]string{Pass, Warn}, Warn},
		{[]string{Warn, Unknown}, Unknown},
		{[]string{Unknown, Fail, Pass}, Fail},
		{[]string{"", Pass}, Pass},
		{nil, Pass},
	}
	for _, c := range cases {
		if got := Worst(c.in...); got != c.want {
			t.Errorf("Worst(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCompareFirmware(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"7.0.1-5145", "7.0.1-5161", -1},
		{"SonicOS 7.1.1-7058-R6416", "7.0.1-5165", 1},
		{"7.0.1-5145", "7.0.1-5145", 0},
		{"6.5.4.15-116n", "6.5.4.4-44n", 1},
		{"7.0.1", "7.0.1-5145", -1},
	}
	for _, c := range cases {
		if got := CompareFirmware(c.a, c.b); got != c.want {
			t.Errorf("CompareFirmware(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestFirmwareGap(t *testing.T) {
	cases := []struct {
		current    string
		available  []string
		wantBehind int
		wantNewest string
		wantOK     bool
	}{
		{"7.0.1-5145", []string{"7.0.1-5161", "7.1.1-7058", "7.0.1-5145"}, 2, "7.1.1-7058", true},
		{"7.1.1-7058", nil, 0, "7.1.1-7058", true},
		{"7.1.1-7058", []string{"7.0.1-5161"}, 0, "7.1.1-7058", true},
		{"", []string{"7.1.1-7058"}, 0, "", false},
	}
	for _, c := range cases {
		behind, newest, ok := FirmwareGap(c.current, c.available)
		if behind != c.wantBehind || newest != c.wantNewest || ok != c.wantOK {
			t.Errorf("FirmwareGap(%q) = (%d,%q,%v), want (%d,%q,%v)", c.current, behind, newest, ok, c.wantBehind, c.wantNewest, c.wantOK)
		}
	}
}

func TestAvailableVersions(t *testing.T) {
	cases := []struct {
		raw    string
		want   []string
		wantOK bool
	}{
		{`{"availableVersions":[{"version":"7.1.1-7058","releaseDate":"2024-09-01"},{"version":"7.0.1-5161"}]}`, []string{"7.1.1-7058", "7.0.1-5161"}, true},
		{`{"availableVersions":[{"version":"7.1.1-7058"},{"version":""}]}`, nil, false},
		{`{"availableVersions":[{}]}`, nil, false},
		{`{"availableVersions":[null]}`, nil, false},
		{`{"availableVersions":[]}`, []string{}, true},
		{`{"availableVersions":null}`, nil, false},
		{`{}`, nil, false},
		{`not json`, nil, false},
	}
	for _, c := range cases {
		got, ok := AvailableVersions(json.RawMessage(c.raw))
		if !reflect.DeepEqual(got, c.want) || ok != c.wantOK {
			t.Errorf("AvailableVersions(%s) = %#v,%v want %#v,%v", c.raw, got, ok, c.want, c.wantOK)
		}
	}
}

func TestOTPMethod(t *testing.T) {
	cases := map[string]string{
		`{"totp":true}`:  "totp",
		`{"otp":true}`:   "otp-email",
		`{}`:             "none",
		`null`:           "none",
		``:               "unknown",
		`{"totp":false}`: "none",
	}
	for raw, want := range cases {
		if got := OTPMethod(json.RawMessage(raw)); got != want {
			t.Errorf("OTPMethod(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestAdminTOTP(t *testing.T) {
	cases := []struct {
		raw         string
		wantEnabled bool
		wantOK      bool
	}{
		{`{"administration":{"admin":{"one_time_password":{"totp":true}}}}`, true, true},
		{`{"administration":{"admin":{"one_time_password":null}}}`, false, true},
		{`{"administration":{"http_port":80}}`, false, false},
		{`{"administration":{"admin":{"http_port":80}}}`, false, false},
		{`{"administration":{"admin":{"one_time_password":{}}}}`, false, true},
		{`{"status":{"success":false}}`, false, false},
	}
	for _, c := range cases {
		enabled, ok := AdminTOTP(json.RawMessage(c.raw))
		if enabled != c.wantEnabled || ok != c.wantOK {
			t.Errorf("AdminTOTP(%s) = (%v,%v), want (%v,%v)", c.raw, enabled, ok, c.wantEnabled, c.wantOK)
		}
	}
}

func TestLocalAccounts(t *testing.T) {
	users := `{"user":{"local":{"user":[
		{"name":"vpn-alice","member_of":[{"name":"SSLVPN Services"}],"one_time_password":{"totp":true}},
		{"name":"vpn-bob","member_of":[{"name":"SSLVPN Services"},{"name":"Remote Staff"}],"one_time_password":null},
		{"name":"vpn-carol","member_of":[{"name":"SSLVPN Services"}],"force_password_change":true,"one_time_password":{}},
		{"name":"nested-dave","member_of":[{"name":"Contractors"}],"one_time_password":{}},
		{"name":"printer","member_of":[{"name":"Everyone"}],"one_time_password":null}
	]}}}`
	groups := `{"user":{"local":{"group":[
		{"name":"Remote Staff","one_time_password":{"otp":true},"member":[]},
		{"name":"SSLVPN Services","member":[{"name":"Contractors"}],"one_time_password":null},
		{"name":"Contractors","member":[],"one_time_password":{}},
		{"name":"Everyone","member":[],"one_time_password":null}
	]}}}`
	got, ok := LocalAccounts(json.RawMessage(users), json.RawMessage(groups))
	if !ok {
		t.Fatal("LocalAccounts ok=false")
	}
	isMember, notMember := true, false
	want := []Account{
		{Name: "nested-dave", SSLVPNMember: &isMember, Method: "none"},
		{Name: "printer", SSLVPNMember: &notMember, Method: "none"},
		{Name: "vpn-alice", SSLVPNMember: &isMember, Method: "totp", MethodSource: "user"},
		{Name: "vpn-bob", SSLVPNMember: &isMember, Method: "otp-email", MethodSource: "group:Remote Staff"},
		{Name: "vpn-carol", SSLVPNMember: &isMember, Method: "none", ForceNewLogin: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("LocalAccounts =\n%#v\nwant\n%#v", got, want)
	}

	// Unreadable groups: users with their own OTP still pass; everyone else is unknown, never "none".
	got, ok = LocalAccounts(json.RawMessage(users), json.RawMessage(`{"status":{"success":false}}`))
	if !ok {
		t.Fatal("LocalAccounts with unreadable groups should still read users")
	}
	for _, a := range got {
		if a.Name == "vpn-alice" && a.Method != "totp" {
			t.Errorf("alice = %+v", a)
		}
		if a.Name != "vpn-alice" && a.Method != "unknown" {
			t.Errorf("%s without own OTP and unreadable groups = %q, want unknown", a.Name, a.Method)
		}
	}

	if _, ok := LocalAccounts(json.RawMessage(`{"user":{"local":{"user":[null]}}}`), json.RawMessage(`{"user":{"local":{"group":[]}}}`)); ok {
		t.Error("a null user row must make the list unreadable")
	}
	if _, ok := LocalAccounts(json.RawMessage(`{"user":{"local":{"user":[{}]}}}`), json.RawMessage(`{"user":{"local":{"group":[]}}}`)); ok {
		t.Error("a nameless user row must make the list unreadable")
	}
	if _, ok := LocalAccounts(json.RawMessage(`{"user":{"local":{"user":[{"name":"alice","member_of":[null]}]}}}`), json.RawMessage(`{"user":{"local":{"group":[]}}}`)); ok {
		t.Error("a null member_of reference must make the user list unreadable")
	}
	if got, ok := LocalAccounts(json.RawMessage(`{"user":{"local":{"user":[{"name":"alice"}]}}}`), json.RawMessage(`{"user":{"local":{"group":[{"name":"SSLVPN Services","member":[null]}]}}}`)); !ok || got[0].Method != "unknown" {
		t.Errorf("a null group member reference must make groups unreadable, got %+v ok=%v", got, ok)
	}
	if got, ok := LocalAccounts(json.RawMessage(users), json.RawMessage(`{"user":{"local":{"group":[null]}}}`)); !ok || got[0].Method != "unknown" {
		t.Errorf("a null group row must make groups unreadable (users without own OTP unknown), got %+v ok=%v", got, ok)
	}
	// Membership the firewall did not report is unknown, never "not a member":
	// a user with no member_of and a SSLVPN Services group with no member key.
	if got, ok := LocalAccounts(json.RawMessage(`{"user":{"local":{"user":[{"name":"alice","one_time_password":null}]}}}`),
		json.RawMessage(`{"user":{"local":{"group":[{"name":"SSLVPN Services"}]}}}`)); !ok || got[0].SSLVPNMember != nil {
		t.Errorf("unreported membership = %+v ok=%v, want SSLVPNMember nil", got, ok)
	}
	// Codex round 4: no member_of on the user, and a group with a TOTP policy
	// that did not report its members: coverage is unknown, not "none".
	if got, ok := LocalAccounts(json.RawMessage(`{"user":{"local":{"user":[{"name":"alice","one_time_password":{}}]}}}`),
		json.RawMessage(`{"user":{"local":{"group":[{"name":"SSLVPN Services","one_time_password":{},"member":[{"name":"alice"}]},{"name":"MFA staff","one_time_password":{"totp":true}}]}}}`)); !ok || got[0].Method != "unknown" {
		t.Errorf("unreported group coverage = %+v ok=%v, want Method unknown", got, ok)
	}
	// A user whose one_time_password key is absent is unknown, not "none".
	if got, ok := LocalAccounts(json.RawMessage(`{"user":{"local":{"user":[{"name":"alice","member_of":[]}]}}}`),
		json.RawMessage(`{"user":{"local":{"group":[{"name":"SSLVPN Services","member":[]}]}}}`)); !ok || got[0].Method != "unknown" {
		t.Errorf("absent one_time_password = %+v ok=%v, want Method unknown", got, ok)
	}
	if _, ok := LocalAccounts(json.RawMessage(`{"user":{"local":{}}}`), nil); ok {
		t.Error("missing user collection should be ok=false")
	}
	if got, ok := LocalAccounts(json.RawMessage(`{"user":{"local":{"user":[]}}}`), json.RawMessage(`{"user":{"local":{"group":[]}}}`)); !ok || len(got) != 0 {
		t.Errorf("explicit empty user list = %v,%v; want [] ok", got, ok)
	}
}

func TestSSLVPNZones(t *testing.T) {
	cases := []struct {
		raw       string
		wantZones []string
		wantWAN   bool
		wantOK    bool
	}{
		{`{"ssl_vpn":{"server":{"access":[{"zone":"WAN","enable":true},{"zone":"LAN","enable":false}]}}}`, []string{"WAN"}, true, true},
		{`{"ssl_vpn":{"server":{"access":[{"zone":"LAN","enable":true}]}}}`, []string{"LAN"}, false, true},
		{`{"ssl_vpn":{"server":{"access":[]}}}`, []string{}, false, true},
		{`{"ssl_vpn":{"server":{}}}`, nil, false, false},
		{`{"ssl_vpn":{"server":{"access":[null]}}}`, nil, false, false},
		{`{"ssl_vpn":{"server":{"access":[{"zone":"WAN"}]}}}`, nil, false, false},
		{`{"ssl_vpn":{"server":{"access":[{"zone":"WAN","enable":null}]}}}`, nil, false, false},
		{`{"ssl_vpn":{"server":{"access":null}}}`, nil, false, false},
		{`{}`, nil, false, false},
	}
	for _, c := range cases {
		zones, wan, ok := SSLVPNZones(json.RawMessage(c.raw))
		if !reflect.DeepEqual(zones, c.wantZones) || wan != c.wantWAN || ok != c.wantOK {
			t.Errorf("SSLVPNZones(%s) = (%v,%v,%v), want (%v,%v,%v)", c.raw, zones, wan, ok, c.wantZones, c.wantWAN, c.wantOK)
		}
	}
}

func TestSSLVPNAuth(t *testing.T) {
	cases := []struct {
		raw      string
		wantType string
		wantSAML bool
		wantOK   bool
	}{
		{`{"ssl_vpn":{"server":{"auth_type":"saml"}}}`, "saml", true, true},
		{`{"ssl_vpn":{"server":{"auth_type":"password"}}}`, "password", false, true},
		{`{"ssl_vpn":{"server":{"auth_type":"password","use_radius":{"mschapv2":true}}}}`, "password", false, true},
		{`{"ssl_vpn":{"server":{"auth_type":"certificate"}}}`, "certificate", false, true},
		{`{"ssl_vpn":{"server":{"port":4433}}}`, "", false, false},
		{`{"ssl_vpn":{"server":{"use_radius":{"mschapv2":true}}}}`, "", false, false},
		{`{"other":1}`, "", false, false},
		{`{"ssl_vpn":{"server":{"auth_type":"sso"}}}`, "", false, false},
	}
	for _, c := range cases {
		typ, saml, ok := SSLVPNAuth(json.RawMessage(c.raw))
		if typ != c.wantType || saml != c.wantSAML || ok != c.wantOK {
			t.Errorf("SSLVPNAuth(%s) = (%q,%v,%v), want (%q,%v,%v)", c.raw, typ, saml, ok, c.wantType, c.wantSAML, c.wantOK)
		}
	}
}

func TestUserAuthMethods(t *testing.T) {
	cases := []struct {
		raw          string
		wantExternal string
		wantLocal    bool
		wantOK       bool
	}{
		{`{"user":{"auth":{"auth_method":"local"}}}`, "", true, true},
		{`{"user":{"auth":{"auth_method":"ldap-local"}}}`, "ldap", true, true},
		{`{"user":{"auth":{"auth_method":"radius"}}}`, "radius", false, true},
		{`{"user":{"auth":{"auth_method":"local","partition":[{"name":"p1","auth_method":"tacacs"}]}}}`, "tacacs", true, true},
		{`{"user":{"auth":{"auth_method":"local","partition":[{"name":"p1"}]}}}`, "", false, false},
		{`{"user":{"auth":{}}}`, "", false, false},
		{`{"other":1}`, "", false, false},
	}
	for _, c := range cases {
		methods, ok := UserAuthMethods(json.RawMessage(c.raw))
		if ok != c.wantOK {
			t.Errorf("UserAuthMethods(%s) ok = %v, want %v", c.raw, ok, c.wantOK)
			continue
		}
		if !ok {
			continue
		}
		ext, local := SignInSources(methods)
		if strings.Join(ext, "+") != c.wantExternal || local != c.wantLocal {
			t.Errorf("SignInSources(%v) = (%v,%v), want (%q,%v)", methods, ext, local, c.wantExternal, c.wantLocal)
		}
	}
}

func intp(n int) *int { return &n }

func TestLicences(t *testing.T) {
	now := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	raw := `{"serviceTypeList":[{"serviceTypeName":"Security","serviceList":[
		{"serviceName":"Gateway Anti-Virus","serviceFamily":"GAV","expirydate":"2026-11-01","nDaysToExpiry":19,"expired":"false"},
		{"serviceName":"Capture ATP","expirydate":"2026-01-01","nDaysToExpiry":-285,"expired":"false"},
		{"serviceName":"Content Filtering","expirydate":"2026-10-23"},
		{"serviceName":"Support 24x7","expirydate":"TBD"},
		{"serviceName":null,"serviceFamily":"CFS","expired":"true"}
	]}]}`
	got, ok := Licences(json.RawMessage(raw), now)
	want := []Licence{
		{Service: "Gateway Anti-Virus", Family: "GAV", Expiry: "2026-11-01", DaysToExpiry: intp(19)},
		{Service: "Capture ATP", Expiry: "2026-01-01", DaysToExpiry: intp(-285), Expired: true},
		{Service: "Content Filtering", Expiry: "2026-10-23", DaysToExpiry: intp(10)},
		{Service: "Support 24x7", Expiry: "TBD", Undated: true},
		{Service: "(unnamed service 5, family CFS)", Family: "CFS", Expired: true},
	}
	if !ok || !reflect.DeepEqual(got, want) {
		t.Errorf("Licences =\n%#v (ok=%v)\nwant\n%#v", got, ok, want)
	}
	if !AnyUndated(got) {
		t.Error("AnyUndated should see Support 24x7")
	}
	wrapped := `{"content":` + raw + `}`
	if got2, ok2 := Licences(json.RawMessage(wrapped), now); !ok2 || len(got2) != 5 {
		t.Errorf("Licences(content-wrapped) = %d rows ok=%v, want 5 rows", len(got2), ok2)
	}
	onlyUnnamedExpired := `{"serviceTypeList":[{"serviceList":[{"serviceName":null,"expired":"true"},{"serviceName":"GAV","nDaysToExpiry":365}]}]}`
	ls2, _ := Licences(json.RawMessage(onlyUnnamedExpired), now)
	if soon, _ := SoonestExpiry(ls2); !soon.Expired || soon.Service != "(unnamed service 1)" {
		t.Errorf("an unnamed expired service must beat an active one, got %+v", soon)
	}
	for _, bad := range []string{`{"operationStatus":"Failure"}`, `{"serviceTypeList":null}`, `{"serviceTypeList":[{"serviceTypeName":"Security"}]}`, `{"serviceTypeList":[{"serviceList":null}]}`, `{"serviceTypeList":[{"serviceList":[null]}]}`} {
		if _, ok := Licences(json.RawMessage(bad), now); ok {
			t.Errorf("Licences(%s) should be ok=false", bad)
		}
	}
}

func TestSoonestExpiry(t *testing.T) {
	ls := []Licence{{Service: "A", DaysToExpiry: intp(40)}, {Service: "B", DaysToExpiry: intp(5)}, {Service: "C"}}
	got, ok := SoonestExpiry(ls)
	if !ok || got.Service != "B" {
		t.Errorf("SoonestExpiry = %+v ok=%v, want B", got, ok)
	}
	if _, ok := SoonestExpiry([]Licence{{Service: "C"}}); ok {
		t.Error("SoonestExpiry with no dated licences should be ok=false")
	}
	// An explicitly expired service wins even when its day count is contradictory.
	mixed := []Licence{{Service: "Active", DaysToExpiry: intp(50)}, {Service: "Lapsed", DaysToExpiry: intp(100), Expired: true}}
	if got, _ := SoonestExpiry(mixed); got.Service != "Lapsed" {
		t.Errorf("SoonestExpiry(mixed) = %q, want Lapsed", got.Service)
	}
}

func TestParseLifecycleDate(t *testing.T) {
	cases := map[string]bool{"2027-03-31": true, "03/31/2027": true, "Mar 31, 2027": true, "TBD": false, "": false}
	for in, want := range cases {
		if _, ok := ParseLifecycleDate(in); ok != want {
			t.Errorf("ParseLifecycleDate(%q) ok=%v, want %v", in, ok, want)
		}
	}
}

func TestSupportStatus(t *testing.T) {
	now := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	year := 365 * 24 * time.Hour
	cases := map[string]string{"2025-12-31": Fail, "2027-03-31": Warn, "2030-01-01": Pass, "TBD": Unknown}
	for in, want := range cases {
		if got := SupportStatus(in, now, year); got != want {
			t.Errorf("SupportStatus(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMatchesAdvisory(t *testing.T) {
	cases := []struct {
		q, id, title, link string
		want               bool
	}{
		{"snwlid-2024-0015", "SNWLID-2024-0015", "SonicOS Improper Access Control", "", true},
		{"CVE-2024-40766", "SNWLID-2024-0015", "SonicOS Improper Access Control (CVE-2024-40766)", "", true},
		{"CVE-2024-40766", "SNWLID-2024-0015", "SonicOS Improper Access Control", "https://psirt.global.sonicwall.com/vuln-detail/SNWLID-2024-0015", false},
		{"", "SNWLID-2024-0015", "x", "", false},
	}
	for _, c := range cases {
		if got := MatchesAdvisory(c.q, c.id, c.title, c.link); got != c.want {
			t.Errorf("MatchesAdvisory(%q, %q) = %v, want %v", c.q, c.id, got, c.want)
		}
	}
}

func TestDiffSnapshots(t *testing.T) {
	before := map[string]map[string]string{
		"0040AAAA0001": {"admin_totp": Pass, "advisories": Pass},
		"0040AAAA0002": {"admin_totp": Pass},
	}
	after := map[string]map[string]string{
		"0040AAAA0001": {"admin_totp": Fail, "advisories": Pass},
		"0040AAAA0003": {"admin_totp": Pass},
	}
	got := DiffSnapshots(before, after, map[string]string{"0040AAAA0001": "HQ"})
	want := []Change{
		{Serial: "0040AAAA0001", Name: "HQ", Check: "admin_totp", Before: Pass, After: Fail},
		{Serial: "0040AAAA0002", Check: "present", Before: "present", After: "absent"},
		{Serial: "0040AAAA0003", Check: "present", Before: "absent", After: "present"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("DiffSnapshots =\n%#v\nwant\n%#v", got, want)
	}
	if got := DiffSnapshots(before, before, nil); len(got) != 0 {
		t.Errorf("DiffSnapshots(identical) = %v, want empty", got)
	}
}

func TestLicencesFractionalDayExpiredIsExpired(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	raw := json.RawMessage(`{"serviceTypeList":[{"serviceList":[{"serviceName":"GAV","expirydate":"2026-10-15T00:00:00"}]}]}`)
	ls, ok := Licences(raw, now)
	if !ok || len(ls) != 1 {
		t.Fatalf("Licences: ok=%v len=%d", ok, len(ls))
	}
	if !ls[0].Expired || ls[0].DaysToExpiry == nil || *ls[0].DaysToExpiry != -1 {
		t.Fatalf("expiry 12h ago: expired=%v days=%v, want expired at day -1", ls[0].Expired, ls[0].DaysToExpiry)
	}
}

func TestLicencesDateBeatsStaleDayCount(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	// nDaysToExpiry says 0 (today) but the timestamp passed 12 hours ago.
	raw := json.RawMessage(`{"serviceTypeList":[{"serviceList":[{"serviceName":"GAV","expirydate":"2026-10-15T00:00:00","nDaysToExpiry":0}]}]}`)
	ls, ok := Licences(raw, now)
	if !ok || len(ls) != 1 || !ls[0].Expired {
		t.Fatalf("stale day count: ok=%v ls=%+v, want expired", ok, ls)
	}
	// A date with no time of day is still valid on that day.
	raw = json.RawMessage(`{"serviceTypeList":[{"serviceList":[{"serviceName":"GAV","expirydate":"2026-10-15"}]}]}`)
	ls, ok = Licences(raw, now)
	if !ok || len(ls) != 1 || ls[0].Expired || ls[0].DaysToExpiry == nil || *ls[0].DaysToExpiry != 0 {
		t.Fatalf("date-only expiry today: ok=%v ls=%+v, want not expired at day 0", ok, ls)
	}
}

func TestLicencesSecondaryServices(t *testing.T) {
	now := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	raw := json.RawMessage(`{"serviceTypeList":[{"serviceList":[{"serviceName":"Security Suite","expirydate":"2030-01-01","secondaryServiceList":[{"serviceName":"Capture ATP","expirydate":"2026-10-01","expired":"true"}]}]}]}`)
	ls, ok := Licences(raw, now)
	if !ok || len(ls) != 2 || ls[1].Service != "Capture ATP" || !ls[1].Expired {
		t.Fatalf("secondary services: ok=%v ls=%+v, want the bundled Capture ATP row, expired", ok, ls)
	}
	if _, ok := Licences(json.RawMessage(`{"serviceTypeList":[{"serviceList":[{"serviceName":"Suite","secondaryServiceList":[null]}]}]}`), now); ok {
		t.Fatal("a null secondary service row must make the list unreadable")
	}
}

// A name in the SSLVPN Services chain or in member_of that the group list did
// not return is never read as "not a member" or "no OTP".
func TestLocalAccountsUnresolvedGroup(t *testing.T) {
	groups := `{"user":{"local":{"group":[{"name":"SSLVPN Services","one_time_password":{},"member":[{"name":"Remote Staff"}]}]}}}`
	carol := `{"user":{"local":{"user":[{"name":"carol","one_time_password":{},"member_of":[{"name":"Remote Staff"}]},{"name":"dave","one_time_password":{},"member_of":[]}]}}}`
	got, ok := LocalAccounts(json.RawMessage(carol), json.RawMessage(groups))
	if !ok || len(got) != 2 {
		t.Fatalf("LocalAccounts = %+v, %v", got, ok)
	}
	if c := got[0]; c.SSLVPNMember == nil || !*c.SSLVPNMember || c.Method != "unknown" {
		t.Fatalf("carol = %+v, want member and unknown method", c)
	}
	if d := got[1]; d.SSLVPNMember != nil {
		t.Fatalf("dave membership = %v, want null (Remote Staff's members were not reported)", *d.SSLVPNMember)
	}
}

// Codex round 7: a user with no member_of, where some group member list names
// a group the list did not return, has unknown coverage, not "none".
func TestLocalAccountsUnresolvedGroupNoMemberOf(t *testing.T) {
	users := `{"user":{"local":{"user":[{"name":"alice","one_time_password":{}}]}}}`
	groups := `{"user":{"local":{"group":[{"name":"SSLVPN Services","one_time_password":{},"member":[{"name":"alice"},{"name":"Remote Staff"}]}]}}}`
	got, ok := LocalAccounts(json.RawMessage(users), json.RawMessage(groups))
	if !ok || len(got) != 1 || got[0].Method != "unknown" || got[0].MethodSource != "" {
		t.Fatalf("LocalAccounts = %+v, %v; want alice method unknown with no source", got, ok)
	}
}

// Adversarial round 17 + Codex round 19: two different names redacted to the
// same placeholder joined as one group, crediting one group's TOTP to another's
// member. A redacted or repeated name carries no identity, so the list that
// holds it is unreadable and coverage is unknown, never pass.
func TestLocalAccountsRedactedOrRepeatedNamesAreNotJoined(t *testing.T) {
	users := `{"user":{"local":{"user":[{"name":"alice","one_time_password":{},"member_of":[{"name":"SSLVPN Services"},{"name":"[redacted]"}]}]}}}`
	repeated := `{"user":{"local":{"user":[{"name":"alice","one_time_password":{},"member_of":[{"name":"SSLVPN Services"},{"name":"Remote Staff"}]}]}}}`
	for name, groups := range map[string]string{
		"redacted names": `{"user":{"local":{"group":[{"name":"SSLVPN Services","one_time_password":{},"member":[]},` +
			`{"name":"[redacted]","one_time_password":{},"member":[]},{"name":"[redacted]","one_time_password":{"totp":true},"member":[]}]}}}`,
		"repeated names": `{"user":{"local":{"group":[{"name":"SSLVPN Services","one_time_password":{},"member":[]},` +
			`{"name":"Remote Staff","one_time_password":{},"member":[]},{"name":"remote staff","one_time_password":{"totp":true},"member":[]}]}}}`,
	} {
		u := users
		if name == "repeated names" {
			u = repeated
		}
		// Unreadable (ok false) or unknown coverage; never a credited group TOTP.
		if got, ok := LocalAccounts(json.RawMessage(u), json.RawMessage(groups)); ok && (len(got) != 1 || got[0].Method != "unknown") {
			t.Errorf("%s: LocalAccounts = %+v; want the list unreadable or alice's coverage unknown", name, got)
		}
	}
	redactedRef := `{"user":{"local":{"user":[{"name":"alice","one_time_password":{},"member_of":[{"name":"[redacted]"}]}]}}}`
	groups := `{"user":{"local":{"group":[{"name":"SSLVPN Services","one_time_password":{},"member":[{"name":"alice"}]}]}}}`
	if got, ok := LocalAccounts(json.RawMessage(redactedRef), json.RawMessage(groups)); ok {
		t.Errorf("a redacted member_of reference must make the user list unreadable, got %+v", got)
	}
	twoAlices := `{"user":{"local":{"user":[{"name":"alice","one_time_password":{}},{"name":"Alice","one_time_password":{"totp":true}}]}}}`
	if got, ok := LocalAccounts(json.RawMessage(twoAlices), json.RawMessage(groups)); ok {
		t.Errorf("a repeated user name must make the user list unreadable, got %+v", got)
	}
	for _, z := range []string{"[redacted]", "[redacted 2]", "[REDACTED] 3"} {
		if _, _, ok := SSLVPNZones(json.RawMessage(`{"ssl_vpn":{"server":{"access":[{"zone":"` + z + `","enable":true}]}}}`)); ok {
			t.Errorf("zone %q is shaped like a redaction placeholder and must not grade as a non-WAN zone", z)
		}
	}
	numbered := `{"user":{"local":{"group":[{"name":"SSLVPN Services","one_time_password":{},"member":[]},{"name":"[redacted 2]","one_time_password":{"totp":true},"member":[]}]}}}`
	ref2 := `{"user":{"local":{"user":[{"name":"alice","one_time_password":{},"member_of":[{"name":"SSLVPN Services"},{"name":"[redacted 2]"}]}]}}}`
	if got, ok := LocalAccounts(json.RawMessage(ref2), json.RawMessage(numbered)); ok && (len(got) != 1 || got[0].Method != "unknown") {
		t.Errorf("a numbered placeholder must not join as a group, got %+v", got)
	}
}
