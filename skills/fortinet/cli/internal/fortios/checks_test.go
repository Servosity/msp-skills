package fortios

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func obj(t *testing.T, s string) Obj {
	t.Helper()
	var o Obj
	if err := json.Unmarshal([]byte(s), &o); err != nil {
		t.Fatalf("bad fixture %s: %v", s, err)
	}
	return o
}

func objs(t *testing.T, s string) []Obj {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("bad fixture %s: %v", s, err)
	}
	return Objs(v)
}

func codes(fs []Finding) map[string]string {
	out := map[string]string{}
	for _, f := range fs {
		out[f.Code] = f.Status
	}
	return out
}

func TestParseVersion(t *testing.T) {
	cases := map[string]Version{"v7.4.3": {7, 4, 3}, "7.2.10": {7, 2, 10}, "v7.0.12,build0523": {7, 0, 12}, "7.6": {7, 6, 0}}
	for in, want := range cases {
		got, ok := ParseVersion(in)
		if !ok || got != want {
			t.Errorf("ParseVersion(%q) = %v,%v want %v", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", "seven", "7", "7.x.1"} {
		if _, ok := ParseVersion(bad); ok {
			t.Errorf("ParseVersion(%q) accepted", bad)
		}
	}
}

func TestParseFloors(t *testing.T) {
	f, err := ParseFloors("7.2.10, 7.4.5")
	if err != nil || f["7.2"] != (Version{7, 2, 10}) || f["7.4"] != (Version{7, 4, 5}) {
		t.Fatalf("ParseFloors = %v, %v", f, err)
	}
	if _, err := ParseFloors("7.4.x"); err == nil {
		t.Fatal("expected error for bad floor")
	}
}

func TestFirmware(t *testing.T) {
	floors, _ := ParseFloors("7.2.10,7.4.5")
	res := obj(t, `{"current":{"version":"v7.4.3","major":7,"minor":4,"patch":3},"available":[{"version":"v7.4.4"},{"version":"v7.4.7"},{"version":"v7.6.2"}]}`)
	r := Firmware("hq", res, "", floors)
	c := codes(r.Findings)
	if c["below-floor"] != Fail || c["patches-behind"] != Warn {
		t.Fatalf("findings = %+v", r.Findings)
	}
	if r.LatestInBranch != "7.4.7" || r.PatchesBehind != 4 || r.NewestOffered != "7.6.2" {
		t.Fatalf("report = %+v", r)
	}

	r = Firmware("hq", obj(t, `{"current":{"version":"v7.0.12"}}`), "", floors)
	if codes(r.Findings)["unsupported-branch"] != Fail {
		t.Fatalf("old branch not flagged: %+v", r.Findings)
	}

	r = Firmware("hq", obj(t, `{"current":{"version":"v7.4.7"},"available":[{"version":"v7.4.7"}]}`), "", floors)
	if Status(r.Findings) != Pass {
		t.Fatalf("current firmware not PASS: %+v", r.Findings)
	}

	// No floor and no available images: cannot grade.
	r = Firmware("hq", Obj{}, "v7.4.3", nil)
	if codes(r.Findings)["no-upgrade-data"] != Unknown {
		t.Fatalf("expected UNKNOWN, got %+v", r.Findings)
	}
}

func TestSSLVPN(t *testing.T) {
	ifaces := objs(t, `[{"name":"wan1","role":"wan"},{"name":"internal","role":"lan"}]`)
	open := obj(t, `{"status":"enable","port":10443,"source-interface":[{"name":"wan1"}],"source-address":[{"name":"all"}]}`)
	if f := SSLVPN("hq", "root", open, true, ifaces, nil, nil, nil, 3); f[0].Status != Fail || f[0].Code != "open-to-internet" {
		t.Fatalf("open = %+v", f)
	}
	restricted := obj(t, `{"status":"enable","source-interface":[{"name":"wan1"}],"source-address":[{"name":"partners"}]}`)
	partners := NewAddressBook(objs(t, `[{"name":"partners","type":"ipmask","subnet":"203.0.113.0 255.255.255.0"}]`), nil)
	if f := SSLVPN("hq", "root", restricted, true, ifaces, nil, partners, nil, 0); f[0].Status != Warn {
		t.Fatalf("restricted = %+v", f)
	}
	if f := SSLVPN("hq", "root", restricted, true, ifaces, nil, nil, nil, 0); f[0].Status != Unknown || f[0].Code != "source-unresolved" {
		t.Fatalf("restricted to an unresolvable name = %+v, want UNKNOWN", f)
	}
	empty := obj(t, `{"status":"enable","source-interface":[{"name":"virtual-wan-link"}],"source-address":[]}`)
	if f := SSLVPN("hq", "root", empty, true, ifaces, nil, nil, nil, 0); f[0].Code != "open-to-internet" {
		t.Fatalf("empty source on sdwan should be open: %+v", f)
	}
	negated := obj(t, `{"status":"enable","source-interface":[{"name":"wan1"}],"source-address":[{"name":"blocklist"}],"source-address-negate":"enable"}`)
	blocklist := NewAddressBook(objs(t, `[{"name":"blocklist","type":"ipmask","subnet":"198.51.100.0 255.255.255.0"}]`), nil)
	if f := SSLVPN("hq", "root", negated, true, ifaces, nil, blocklist, nil, 0); f[0].Code != "open-to-internet" {
		t.Fatalf("negated source (everyone except blocklist) should be open: %+v", f)
	}
	open6 := obj(t, `{"status":"enable","source-interface":[{"name":"wan1"}],"source-address":[{"name":"partners"}],"source-address6":[{"name":"all"}]}`)
	if f := SSLVPN("hq", "root", open6, true, ifaces, nil, nil, nil, 0); f[0].Code != "open-to-internet" {
		t.Fatalf("IPv4 restricted but IPv6 source all should be open: %+v", f)
	}
	internal := obj(t, `{"status":"enable","source-interface":[{"name":"internal"}]}`)
	if f := SSLVPN("hq", "root", internal, true, ifaces, nil, nil, nil, 0); f[0].Status != Pass {
		t.Fatalf("internal-only = %+v", f)
	}
	if f := SSLVPN("hq", "root", obj(t, `{"status":"disable"}`), true, ifaces, nil, nil, nil, 0); f[0].Code != "disabled" {
		t.Fatalf("disabled = %+v", f)
	}
	if f := SSLVPN("hq", "root", nil, false, ifaces, nil, nil, nil, 0); f[0].Code != "not-available" {
		t.Fatalf("absent = %+v", f)
	}
}

func TestPrefixLen(t *testing.T) {
	cases := map[string]int{"10.0.0.0 255.0.0.0": 8, "0.0.0.0 0.0.0.0": 0, "192.0.2.10 255.255.255.255": 32, "10.0.0.0/8": 8, "::/0": 0}
	for in, want := range cases {
		if got, ok := PrefixLen(in); !ok || got != want {
			t.Errorf("PrefixLen(%q) = %d,%v want %d", in, got, ok, want)
		}
	}
	if _, ok := PrefixLen("nonsense"); ok {
		t.Error("PrefixLen accepted nonsense")
	}
}

func TestAdmins(t *testing.T) {
	ifaces := objs(t, `[{"name":"wan1","role":"wan","allowaccess":"ping https http"},{"name":"lan","role":"lan","allowaccess":"https ssh"}]`)
	admins := objs(t, `[
	  {"name":"admin","accprofile":"super_admin","two-factor":"disable","trusthost1":"0.0.0.0 0.0.0.0"},
	  {"name":"noc","accprofile":"prof_admin","two-factor":"fortitoken","trusthost1":"10.0.0.0 255.0.0.0"}]`)
	api := objs(t, `[{"name":"audit","accprofile":"readonly","trusthost":[{"id":1,"ipv4-trusthost":"0.0.0.0 0.0.0.0"}]}]`)
	f := Admins("hq", admins, nil, api, ifaces, 16)
	c := codes(f)
	for code, want := range map[string]string{
		"wan-plaintext-mgmt": Fail, "admin-anywhere-wan-mgmt": Fail, "super-admin-no-2fa": Fail, "api-user-broad-trusthost": Warn,
	} {
		if c[code] != want {
			t.Errorf("code %s = %q want %q (all: %+v)", code, c[code], want, f)
		}
	}
	for _, x := range f {
		if x.Object == "noc" {
			t.Errorf("restricted MFA admin flagged: %+v", x)
		}
	}
	// Clean device: one restricted MFA admin, no WAN management.
	clean := Admins("hq", admins[1:], nil, nil, objs(t, `[{"name":"wan1","role":"wan","allowaccess":"ping"}]`), 16)
	if Status(clean) != Pass {
		t.Fatalf("clean device = %+v", clean)
	}
}

func TestLicenses(t *testing.T) {
	now := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	day := int64(86400)
	res := obj(t, `{
	 "antivirus":{"type":"downloaded_fds_object","status":"licensed","expires":`+itoa(now.Unix()+20*day)+`},
	 "web_filtering":{"type":"live_fortiguard_service","status":"licensed","expires":`+itoa(now.Unix()+400*day)+`},
	 "ips":{"status":"expired","expires":`+itoa(now.Unix()-5*day)+`},
	 "forticare":{"status":"registered","support":{"hardware":{"status":"licensed","expires":`+itoa(now.Unix()+45*day)+`}}},
	 "vdom":{"type":"platform","used":1,"max":10}}`)
	rows, f := Licenses("hq", "FGT60F0000000001", "FGT60F", res, now, 60*24*time.Hour)
	if len(rows) != 4 {
		t.Fatalf("rows = %+v", rows)
	}
	c := map[string]string{}
	for _, x := range f {
		c[x.Object] = x.Status
	}
	if c["antivirus"] != Warn || c["ips"] != Fail || c["forticare.hardware"] != Warn {
		t.Fatalf("findings = %+v", f)
	}
	if _, flagged := c["web_filtering"]; flagged {
		t.Fatalf("400-day licence flagged: %+v", f)
	}
	_, f = Licenses("hq", "", "", Obj{}, now, time.Hour)
	if Status(f) != Unknown {
		t.Fatalf("empty payload = %+v", f)
	}
}

func itoa(n int64) string { b, _ := json.Marshal(n); return string(b) }

func TestCalendarDays(t *testing.T) {
	now := time.Date(2026, 10, 13, 18, 0, 0, 0, time.UTC)
	cases := map[time.Time]int{
		time.Date(2026, 10, 8, 9, 0, 0, 0, time.UTC):   -5, // expired 5 calendar days ago, earlier in the day
		time.Date(2026, 11, 2, 1, 0, 0, 0, time.UTC):   20,
		time.Date(2026, 10, 13, 23, 0, 0, 0, time.UTC): 0,
	}
	for at, want := range cases {
		if got := calendarDays(now, at); got != want {
			t.Errorf("calendarDays(%s) = %d want %d", at, got, want)
		}
	}
	_, f := Licenses("hq", "", "", obj(t, `{"ips":{"status":"expired","expires":`+itoa(time.Date(2026, 10, 8, 9, 0, 0, 0, time.UTC).Unix())+`}}`), now, time.Hour)
	if len(f) != 1 || f[0].Detail != "ips expired 2026-10-08 (5 days ago)" {
		t.Errorf("expired detail = %+v", f)
	}
}

func TestPolicies(t *testing.T) {
	now := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
	pol := objs(t, `[
	 {"policyid":1,"name":"any","action":"accept","status":"enable","srcaddr":[{"name":"all"}],"dstaddr":[{"name":"all"}],"service":[{"name":"ALL"}],"logtraffic":"disable"},
	 {"policyid":2,"name":"web","action":"accept","status":"enable","srcaddr":[{"name":"lan"}],"dstaddr":[{"name":"all"}],"service":[{"name":"HTTPS"}],"logtraffic":"all"},
	 {"policyid":3,"name":"old","action":"deny","status":"disable"},
	 {"policyid":4,"name":"nocounter","action":"accept","status":"enable","logtraffic":"all"}]`)
	stats := objs(t, `[{"policyid":1,"hit_count":50,"last_used":`+itoa(now.Unix()-3600)+`},{"policyid":2,"hit_count":0}]`)
	rows := Policies("hq", "root", pol, stats, now, 90*24*time.Hour)
	got := map[int][]string{}
	for _, r := range rows {
		got[r.PolicyID] = r.Issues
	}
	if len(got[1]) != 2 || got[1][0] != "any-any-accept" || got[1][1] != "logging-disabled" {
		t.Errorf("policy 1 issues = %v", got[1])
	}
	if len(got[2]) != 1 || got[2][0] != "unused" {
		t.Errorf("policy 2 issues = %v", got[2])
	}
	if len(got[3]) != 1 || got[3][0] != "disabled" {
		t.Errorf("policy 3 issues = %v", got[3])
	}
	if _, ok := got[4]; ok {
		t.Errorf("policy without counter flagged unused: %v", got[4])
	}
}

func TestWorst(t *testing.T) {
	if Worst() != Pass || Worst(Pass, Unknown) != Unknown || Worst(Warn, Fail, Pass) != Fail {
		t.Fatal("Worst ordering broken")
	}
}

func TestAdminsIPv4JudgedIndependently(t *testing.T) {
	ifaces := objs(t, `[{"name":"wan1","role":"wan","allowaccess":"https"}]`)
	// IPv6 restricted, IPv4 open: still reachable from any IPv4 address.
	admins := objs(t, `[{"name":"noc","accprofile":"prof_admin","two-factor":"fortitoken","trusthost1":"0.0.0.0 0.0.0.0","ip6-trusthost1":"2001:db8::/64"}]`)
	if c := codes(Admins("hq", admins, nil, nil, ifaces, 16)); c["admin-anywhere-wan-mgmt"] != Fail {
		t.Fatalf("IPv6 restriction masked open IPv4: %v", c)
	}
}

func TestFirmwareNeedsSameBranchEvidence(t *testing.T) {
	floors, _ := ParseFloors("7.2.10")
	// 7.4.3 with only a 7.2 floor and only 7.6 images: nothing shows 7.4 is current.
	r := Firmware("hq", obj(t, `{"current":{"version":"v7.4.3"},"available":[{"version":"v7.6.2"}]}`), "", floors)
	if r.Status != Unknown {
		t.Fatalf("graded without evidence: %+v", r)
	}
}

func TestPoliciesNegatedAllIsNotAnyAny(t *testing.T) {
	base := `{"policyid":%d,"action":"accept","status":"enable","logtraffic":"all","srcaddr":[{"name":"all"}],"dstaddr":[{"name":"all"}],"service":[{"name":"ALL"}]%s}`
	v6 := `{"policyid":%d,"action":"accept","status":"enable","logtraffic":"all","srcaddr":[],"dstaddr":[],"srcaddr6":[{"name":"all"}],"dstaddr6":[{"name":"all"}],"service":[{"name":"ALL"}]%s}`
	pol := objs(t, "["+fmt.Sprintf(base, 1, "")+","+fmt.Sprintf(base, 2, `,"srcaddr-negate":"enable"`)+","+fmt.Sprintf(base, 3, `,"dstaddr-negate":"enable"`)+","+fmt.Sprintf(base, 4, `,"service-negate":"enable"`)+
		","+fmt.Sprintf(v6, 5, "")+","+fmt.Sprintf(v6, 6, `,"srcaddr6-negate":"enable"`)+"]")
	rows := Policies("hq", "root", pol, nil, time.Now(), time.Hour)
	if len(rows) != 2 || rows[0].PolicyID != 1 || rows[1].PolicyID != 5 || rows[0].Issues[0] != "any-any-accept" || rows[1].Issues[0] != "any-any-accept" {
		t.Fatalf("want the un-negated IPv4 and IPv6 policies flagged any-any-accept, got %+v", rows)
	}
}

func TestPoliciesNeedHitCountField(t *testing.T) {
	now := time.Now()
	pol := objs(t, `[{"policyid":5,"name":"x","action":"accept","status":"enable","logtraffic":"all"}]`)
	stats := objs(t, `[{"policyid":5,"bytes":10}]`)
	if rows := Policies("hq", "root", pol, stats, now, time.Hour); len(rows) != 0 {
		t.Fatalf("counter row without hit_count marked unused: %+v", rows)
	}
}

func TestSSLVPNNeedsInterfaceEvidence(t *testing.T) {
	ifaces := objs(t, `[{"name":"wan1","role":"wan"},{"name":"port9","role":"undefined"}]`)
	zones := objs(t, `[{"name":"WAN-ZONE","interface":[{"interface-name":"wan1"}]},{"name":"LAN-ZONE","interface":[{"interface-name":"ghost"}]}]`)
	open := `{"status":"enable","source-interface":[{"name":"%s"}],"source-address":[{"name":"all"}]}`
	cases := map[string]string{"WAN-ZONE": Fail, "port9": Unknown, "LAN-ZONE": Unknown, "missing": Unknown}
	for iface, want := range cases {
		f := SSLVPN("hq", "root", obj(t, fmt.Sprintf(open, iface)), true, ifaces, zones, nil, nil, 0)
		if f[0].Status != want {
			t.Errorf("%s: %+v want %s", iface, f[0], want)
		}
	}
}

func TestAdminsIPv6ExposureJudgedSeparately(t *testing.T) {
	// IPv4 restricted; IPv6 open with HTTPS management on a WAN interface's IPv6.
	ifaces := objs(t, `[{"name":"wan1","role":"wan","allowaccess":"ping","ipv6":{"ip6-allowaccess":"https"}}]`)
	admins := objs(t, `[{"name":"noc","accprofile":"prof_admin","two-factor":"fortitoken","trusthost1":"10.0.0.0 255.0.0.0","ip6-trusthost1":"::/0"}]`)
	if c := codes(Admins("hq", admins, nil, nil, ifaces, 16)); c["admin-anywhere-wan-mgmt-ipv6"] != Fail {
		t.Fatalf("IPv6 exposure missed: %v", c)
	}
}

func TestAPIUserTrusthostHonorsTypeSelector(t *testing.T) {
	ifaces := objs(t, `[{"name":"wan1","role":"wan","allowaccess":"ping"}]`)
	admins := objs(t, `[{"name":"noc","accprofile":"prof_admin","two-factor":"fortitoken","trusthost1":"10.0.0.0 255.0.0.0"}]`)
	// Restricted IPv4 entry whose inactive ipv6-trusthost keeps its ::/0 default.
	quiet := objs(t, `[{"name":"audit","accprofile":"readonly","trusthost":[{"id":1,"type":"ipv4-trusthost","ipv4-trusthost":"192.0.2.10 255.255.255.255","ipv6-trusthost":"::/0"}]}]`)
	if c := codes(Admins("hq", admins, nil, quiet, ifaces, 16)); c["api-user-broad-trusthost"] != "" {
		t.Fatalf("inactive IPv6 default flagged: %v", c)
	}
	broad6 := objs(t, `[{"name":"audit","accprofile":"readonly","trusthost":[{"id":1,"type":"ipv6-trusthost","ipv6-trusthost":"::/0"}]}]`)
	if c := codes(Admins("hq", admins, nil, broad6, ifaces, 16)); c["api-user-broad-trusthost"] != Warn {
		t.Fatalf("active broad IPv6 trusthost missed: %v", c)
	}
}

// A named source address that covers every address is as open as "all".
func TestSSLVPNResolvesNamedSources(t *testing.T) {
	ifaces := objs(t, `[{"name":"wan1","role":"wan"}]`)
	book4 := NewAddressBook(objs(t, `[
		{"name":"Internet-v4","type":"ipmask","subnet":"0.0.0.0 0.0.0.0"},
		{"name":"every-v4","type":"iprange","start-ip":"0.0.0.0","end-ip":"255.255.255.255"},
		{"name":"partners","type":"ipmask","subnet":"203.0.113.0 255.255.255.0"},
		{"name":"geo-us","type":"geography","country":"US"},
		{"name":"odd","type":"wildcard","wildcard":"10.0.0.0 255.0.0.255"},
		{"name":"low-half","type":"ipmask","subnet":"0.0.0.0 128.0.0.0"},
		{"name":"high-half","subnet":"128.0.0.0/1"},
		{"name":"mixed","type":"iprange","start-ip":"0.0.0.0","end-ip":"::ffff:255.255.255.255"}]`),
		objs(t, `[{"name":"grp-open","member":[{"name":"partners"},{"name":"Internet-v4"}]},
		{"name":"grp-closed","member":[{"name":"partners"},{"name":"geo-us"}]},
		{"name":"grp-loop","member":[{"name":"grp-loop"}]},
		{"name":"grp-halves","member":[{"name":"grp-low"},{"name":"high-half"}]},
		{"name":"grp-low","member":[{"name":"low-half"}]},
		{"name":"grp-hollow","member":[{"name":"all"}],"exclude":"enable","exclude-member":[{"name":"low-half"},{"name":"high-half"}]},
		{"name":"grp-excl-unknown","member":[{"name":"Internet-v4"}],"exclude":"enable","exclude-member":[{"name":"ghost"}]}]`))
	book6 := NewAddressBook(objs(t, `[{"name":"Internet-v6","type":"ipprefix","ip6":"::/0"},{"name":"lab6","type":"ipprefix","ip6":"2001:db8::/32"}]`), nil)
	settings := func(extra string) Obj {
		return obj(t, `{"status":"enable","source-interface":[{"name":"wan1"}],`+extra+`}`)
	}
	cases := map[string]string{
		`"source-address":[{"name":"Internet-v4"}]`:                                         Fail,
		`"source-address":[{"name":"every-v4"}]`:                                            Fail,
		`"source-address":[{"name":"grp-open"}]`:                                            Fail,
		`"source-address":[{"name":"grp-closed"}]`:                                          Warn,
		`"source-address":[{"name":"odd"}]`:                                                 Unknown,
		`"source-address":[{"name":"grp-loop"}]`:                                            Unknown,
		`"source-address":[{"name":"partners"}],"source-address6":[{"name":"Internet-v6"}]`: Fail,
		`"source-address":[{"name":"partners"}],"source-address6":[{"name":"lab6"}]`:        Warn,
		`"source-address":[{"name":"grp-open"}],"source-address-negate":"enable"`:           Warn,
		`"source-address":[{"name":"low-half"},{"name":"high-half"}]`:                       Fail,
		`"source-address":[{"name":"grp-halves"}]`:                                          Fail,
		`"source-address":[{"name":"grp-hollow"}]`:                                          Warn,
		`"source-address":[{"name":"grp-hollow"}],"source-address-negate":"enable"`:         Fail,
		`"source-address":[{"name":"grp-excl-unknown"}]`:                                    Unknown,
		`"source-address":[{"name":"ghost"}],"source-address-negate":"enable"`:              Unknown,
		`"source-address":[{"name":"low-half"}]`:                                            Warn,
		`"source-address":[{"name":"all"},{"name":"mixed"}]`:                                Fail,
		`"source-address":[{"name":"mixed"}]`:                                               Unknown,
		`"source-address":[{"name":"Internet-v4"},{"name":"grp-excl-unknown"}]`:             Fail,
	}
	for extra, want := range cases {
		if f := SSLVPN("hq", "root", settings(extra), true, ifaces, nil, book4, book6, 0); f[0].Status != want {
			t.Errorf("%s: %+v, want %s", extra, f[0], want)
		}
	}
}

// A parameter set under config authentication-rule replaces the same global
// parameter for that rule (Fortinet community tip 96303 for source-address,
// 99903 for source-interface); a parameter the rule leaves unset inherits the
// global value. A rule that widens exposure must not hide behind a restricted
// global source list.
func TestSSLVPNAuthenticationRules(t *testing.T) {
	ifaces := objs(t, `[{"name":"wan1","role":"wan"},{"name":"wan2","role":"wan"},{"name":"lan","role":"lan"},{"name":"port9","role":"undefined"}]`)
	zones := objs(t, `[{"name":"WAN-ZONE","interface":[{"interface-name":"wan2"}]}]`)
	book4 := NewAddressBook(objs(t, `[
		{"name":"office","type":"ipmask","subnet":"203.0.113.0 255.255.255.0"},
		{"name":"partners","type":"ipmask","subnet":"198.51.100.0 255.255.255.0"},
		{"name":"Internet-v4","type":"ipmask","subnet":"0.0.0.0 0.0.0.0"}]`), nil)
	book6 := NewAddressBook(objs(t, `[{"name":"lab6","type":"ipprefix","ip6":"2001:db8::/32"}]`), nil)
	const restricted = `"status":"enable","port":10443,"source-interface":[{"name":"wan1"}],"source-address":[{"name":"office"}]`
	const internal = `"status":"enable","source-interface":[{"name":"lan"}],"source-address":[{"name":"office"}]`
	const open = `"status":"enable","source-interface":[{"name":"wan1"}],"source-address":[{"name":"all"}]`
	rules := func(base, list string) Obj { return obj(t, `{`+base+`,"authentication-rule":[`+list+`]}`) }
	cases := []struct {
		name     string
		settings Obj
		status   string
		code     string
	}{
		{"global restricted, no rules", obj(t, `{`+restricted+`}`), Warn, "restricted-source"},
		{"global restricted, empty rule table", rules(restricted, ``), Warn, "restricted-source"},
		{"rule restricted to a subnet", rules(restricted, `{"id":1,"source-interface":[{"name":"wan1"}],"source-address":[{"name":"partners"}],"groups":[{"name":"vpn"}],"portal":"full-access"}`), Warn, "restricted-source"},
		{"rule source all on a WAN interface", rules(restricted, `{"id":1,"source-interface":[{"name":"wan1"}],"source-address":[{"name":"all"}],"groups":[{"name":"vpn"}]}`), Fail, "open-to-internet"},
		{"rule negated address", rules(restricted, `{"id":1,"source-interface":[{"name":"wan1"}],"source-address":[{"name":"partners"}],"source-address-negate":"enable"}`), Fail, "open-to-internet"},
		{"rule names an object covering every address", rules(restricted, `{"id":1,"source-interface":[{"name":"wan1"}],"source-address":[{"name":"Internet-v4"}]}`), Fail, "open-to-internet"},
		{"rule IPv6 source all", rules(restricted, `{"id":1,"source-interface":[{"name":"wan1"}],"source-address":[{"name":"partners"}],"source-address6":[{"name":"all"}]}`), Fail, "open-to-internet"},
		{"rule IPv6 restricted", rules(restricted, `{"id":1,"source-interface":[{"name":"wan1"}],"source-address":[{"name":"partners"}],"source-address6":[{"name":"lab6"}]}`), Warn, "restricted-source"},
		{"rule inherits the global IPv6 source all", rules(restricted+`,"source-address6":[{"name":"all"}]`, `{"id":1,"source-interface":[{"name":"wan1"}],"source-address":[{"name":"partners"}]}`), Fail, "open-to-internet"},
		{"one restricted rule, one open rule", rules(restricted, `{"id":1,"source-interface":[{"name":"wan1"}],"source-address":[{"name":"partners"}]},{"id":2,"source-interface":[{"name":"wan1"}],"source-address":[{"name":"all"}]}`), Fail, "open-to-internet"},
		{"rule listens on a WAN interface the global list omits", rules(internal, `{"id":1,"source-interface":[{"name":"wan2"}],"source-address":[{"name":"all"}]}`), Fail, "open-to-internet"},
		{"rule listens on a WAN zone", rules(internal, `{"id":1,"source-interface":[{"name":"WAN-ZONE"}],"source-address":[{"name":"all"}]}`), Fail, "open-to-internet"},
		{"rule restricted on a WAN interface the global list omits", rules(internal, `{"id":1,"source-interface":[{"name":"wan2"}],"source-address":[{"name":"partners"}]}`), Warn, "restricted-source"},
		{"rule open on an internal interface", rules(internal, `{"id":1,"source-interface":[{"name":"lan"}],"source-address":[{"name":"all"}]}`), Pass, "enabled-internal"},
		{"rule without source-interface inherits restricted global", rules(restricted, `{"id":1,"groups":[{"name":"vpn"}],"portal":"full-access"}`), Warn, "restricted-source"},
		{"rule without source-interface inherits open global", rules(open, `{"id":1,"groups":[{"name":"vpn"}],"portal":"full-access"}`), Fail, "open-to-internet"},
		{"rule without source-interface keeps the global listener", rules(open, `{"id":1,"groups":[{"name":"vpn"}]},{"id":2,"source-interface":[{"name":"wan1"}],"source-address":[{"name":"partners"}]}`), Fail, "open-to-internet"},
		{"every rule names its interface: global interfaces stop listening (99903)", rules(open, `{"id":1,"source-interface":[{"name":"lan"}],"source-address":[{"name":"all"}]}`), Pass, "enabled-internal"},
		{"rule source list overrides a negated global list", rules(restricted+`,"source-address-negate":"enable"`, `{"id":1,"source-interface":[{"name":"wan1"}],"source-address":[{"name":"partners"}]}`), Warn, "restricted-source"},
		{"rule source unresolved", rules(restricted, `{"id":1,"source-interface":[{"name":"wan1"}],"source-address":[{"name":"ghost"}]}`), Unknown, "source-unresolved"},
		{"open rule on an interface of unknown role", rules(restricted, `{"id":1,"source-interface":[{"name":"wan1"}],"source-address":[{"name":"partners"}]},{"id":2,"source-interface":[{"name":"port9"}],"source-address":[{"name":"all"}]}`), Unknown, "interface-role-unknown"},
	}
	for _, c := range cases {
		f := SSLVPN("hq", "root", c.settings, true, ifaces, zones, book4, book6, 1)
		if len(f) != 1 || f[0].Status != c.status || f[0].Code != c.code {
			t.Errorf("%s: %+v, want %s %s", c.name, f, c.status, c.code)
		}
	}
	// The finding names the rule that opened the listener.
	f := SSLVPN("hq", "root", rules(restricted, `{"id":7,"source-interface":[{"name":"wan1"}],"source-address":[{"name":"all"}]}`), true, ifaces, zones, book4, book6, 0)
	if !strings.Contains(f[0].Detail, "authentication rule 7") {
		t.Errorf("detail does not name the rule: %q", f[0].Detail)
	}
}

// Rule-level source names must reach the address-book reader, or a rule
// naming an object that covers every address stays unresolved.
func TestSSLVPNSourceNamesIncludeRules(t *testing.T) {
	s := obj(t, `{"source-address":[{"name":"office"}],"source-address6":[{"name":"lab6"}],"authentication-rule":[{"id":1,"source-address":[{"name":"Internet-v4"}],"source-address6":[{"name":"Internet-v6"}]}]}`)
	v4, v6 := SSLVPNSourceNames(s)
	if !contains(v4, "office") || !contains(v4, "Internet-v4") || !contains(v6, "lab6") || !contains(v6, "Internet-v6") {
		t.Fatalf("v4=%v v6=%v", v4, v6)
	}
}

// A secondary IP on a WAN interface carries its own allowaccess list
// (cmdb system/interface secondaryip[].allowaccess), so management can listen
// there while the primary address allows only ping.
func TestAdminsWANSecondaryIPManagement(t *testing.T) {
	admins := objs(t, `[{"name":"noc","accprofile":"prof_admin","two-factor":"fortitoken","trusthost1":"0.0.0.0 0.0.0.0"}]`)
	secondary := objs(t, `[{"name":"wan1","role":"wan","allowaccess":"ping","secondary-IP":"enable","secondaryip":[{"id":1,"ip":"192.0.2.10 255.255.255.0","allowaccess":"ping https http"}]}]`)
	c := codes(Admins("hq", admins, nil, nil, secondary, 16))
	if c["admin-anywhere-wan-mgmt"] != Fail || c["wan-plaintext-mgmt"] != Fail {
		t.Fatalf("secondary-IP management on WAN missed: %v", c)
	}
	// A secondary-IP table the interface has switched off does not listen.
	off := objs(t, `[{"name":"wan1","role":"wan","allowaccess":"ping","secondary-IP":"disable","secondaryip":[{"id":1,"ip":"192.0.2.10 255.255.255.0","allowaccess":"https"}]}]`)
	if c := codes(Admins("hq", admins, nil, nil, off, 16)); c["admin-anywhere-wan-mgmt"] != "" || c["admin-no-trusthost"] != Warn {
		t.Fatalf("disabled secondary IP flagged: %v", c)
	}
	// Secondary IPs on a LAN interface are not WAN management.
	lan := objs(t, `[{"name":"lan","role":"lan","allowaccess":"ping","secondary-IP":"enable","secondaryip":[{"id":1,"ip":"10.0.0.1 255.255.255.0","allowaccess":"https"}]}]`)
	if c := codes(Admins("hq", admins, nil, nil, lan, 16)); c["admin-anywhere-wan-mgmt"] != "" {
		t.Fatalf("LAN secondary IP flagged: %v", c)
	}
}

// Management on an interface whose role does not establish exposure (no role,
// or FortiOS's default "undefined") cannot grade PASS for an admin open to any
// address: the SSL-VPN check grades the same condition UNKNOWN.
func TestAdminsManagementOnUnknownRoleInterface(t *testing.T) {
	mfa4 := `{"name":"noc","accprofile":"prof_admin","two-factor":"fortitoken","trusthost1":"10.0.0.0 255.0.0.0"%s}`
	cases := []struct {
		name, ifaces, admin, code, status string
	}{
		{"IPv6 https on an undefined-role interface, IPv6 trusthosts open",
			`[{"name":"port9","role":"undefined","allowaccess":"ping","ipv6":{"ip6-allowaccess":"https"}}]`,
			fmt.Sprintf(mfa4, `,"ip6-trusthost1":"::/0"`), "admin-anywhere-mgmt-unknown-role-ipv6", Unknown},
		{"IPv4 https on an interface with no role, IPv4 trusthosts open",
			`[{"name":"port9","allowaccess":"https"}]`,
			`{"name":"noc","accprofile":"prof_admin","two-factor":"fortitoken","trusthost1":"0.0.0.0 0.0.0.0"}`, "admin-anywhere-mgmt-unknown-role", Unknown},
		{"plaintext management on an undefined-role interface",
			`[{"name":"port9","role":"undefined","allowaccess":"http"}]`,
			fmt.Sprintf(mfa4, ``), "plaintext-mgmt-unknown-role", Unknown},
	}
	for _, c := range cases {
		f := Admins("hq", []Obj{obj(t, c.admin)}, nil, nil, objs(t, c.ifaces), 16)
		if codes(f)[c.code] != c.status || Status(f) == Pass {
			t.Errorf("%s: %+v, want %s %s and not PASS", c.name, f, c.code, c.status)
		}
	}
	// Restricted admin (both families) on an undefined-role interface, and an
	// open admin whose management is only on a LAN-role interface, stay PASS.
	restricted := obj(t, fmt.Sprintf(mfa4, `,"ip6-trusthost1":"2001:db8::/64"`))
	if f := Admins("hq", []Obj{restricted}, nil, nil, objs(t, `[{"name":"port9","role":"undefined","allowaccess":"https","ipv6":{"ip6-allowaccess":"https"}}]`), 16); Status(f) != Pass {
		t.Errorf("restricted admin on an undefined-role interface = %+v, want PASS", f)
	}
	if f := Admins("hq", []Obj{obj(t, fmt.Sprintf(mfa4, `,"ip6-trusthost1":"::/0"`))}, nil, nil, objs(t, `[{"name":"lan","role":"lan","allowaccess":"https","ipv6":{"ip6-allowaccess":"https"}}]`), 16); Status(f) != Pass {
		t.Errorf("IPv6-open admin with management only on LAN = %+v, want PASS", f)
	}
}
