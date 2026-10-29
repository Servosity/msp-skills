// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package fleet

import (
	"encoding/json"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestValidateServer(t *testing.T) {
	good := []string{"acme.jamfcloud.com", "jamf.example.com:8443", "10.0.0.5", "10.0.0.5:8443"}
	bad := []string{"", "https://acme.jamfcloud.com", "acme.jamfcloud.com/api", "user@acme.jamfcloud.com", "acme.jamfcloud.com:", "acme.jamfcloud.com:99999",
		"acme..com", "-acme.com", "acme_x.com", "acme.com?x=1", "acme.com#f", "acme .com"}
	for _, s := range good {
		if err := ValidateServer(s); err != nil {
			t.Errorf("ValidateServer(%q) = %v, want ok", s, err)
		}
	}
	for _, s := range bad {
		if err := ValidateServer(s); err == nil {
			t.Errorf("ValidateServer(%q) = nil, want error", s)
		}
	}
}

func TestInstanceValidate(t *testing.T) {
	ok := Instance{Name: "acme", Server: "acme.jamfcloud.com", ClientID: "id-1"}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Acme", "../x", "a..b", ""} {
		p := ok
		p.Name = name
		if err := p.Validate(); err == nil {
			t.Errorf("name %q must be refused", name)
		}
	}
}

// TestParseSecrets: name=secret entries by commas or newlines, split at the
// first "="; a malformed value is refused without repeating secret text.
func TestParseSecrets(t *testing.T) {
	got, err := ParseSecrets(" acme = s=1 ,\nglobex-2.eu=s2,, ")
	if err != nil || len(got) != 2 || got["acme"] != "s=1" || got["globex-2.eu"] != "s2" {
		t.Fatalf("ParseSecrets = %v %v", got, err)
	}
	if got, err := ParseSecrets(""); err != nil || len(got) != 0 {
		t.Fatalf("empty = %v %v", got, err)
	}
	for _, raw := range []string{"zqvalue1zq", "=zqvalue1zq", "acme=", "Acme=zqvalue1zq", "acme=zqvalue1zq,acme=zqvalue2zq", "acme=zqval,ue1zq"} {
		_, err := ParseSecrets(raw)
		if err == nil {
			t.Errorf("ParseSecrets(%q) must be refused", raw)
			continue
		}
		if strings.Contains(err.Error(), "zq") {
			t.Errorf("ParseSecrets(%q) error repeats the value: %v", raw, err)
		}
	}
	t.Setenv(EntriesEnv, "acme=s1,globex=s2")
	if s, err := SecretFor("globex"); err != nil || s != "s2" || !HasSecret("acme") || HasSecret("initech") {
		t.Fatalf("SecretFor = %q %v", s, err)
	}
	t.Setenv(EntriesEnv, "${user_config.jamf_pro_fleet_secrets}")
	if s, err := SecretFor("acme"); err != nil || s != "" {
		t.Fatalf("an unfilled MCPB placeholder reads as unset: %q %v", s, err)
	}
}

func TestRegistryRulesAndSelect(t *testing.T) {
	reg := &Registry{}
	reg.Upsert(Instance{Name: "acme", Server: "acme.jamfcloud.com", ClientID: "1", Tags: []string{"gold"}})
	reg.Upsert(Instance{Name: "globex", Server: "globex.jamfcloud.com", ClientID: "2"})
	if err := reg.Validate(); err != nil {
		t.Fatal(err)
	}
	dup := *reg
	dup.Instances = append([]Instance(nil), reg.Instances...)
	dup.Instances[1].Name = "acme"
	if err := dup.Validate(); err == nil {
		t.Fatal("two instances sharing a name (and so a secret) must be refused")
	}
	got, err := reg.Select(nil, []string{"gold"})
	if err != nil || len(got) != 1 || got[0].Name != "acme" {
		t.Fatalf("select by tag = %v %v", got, err)
	}
	if _, err := reg.Select([]string{"nope"}, nil); err == nil {
		t.Fatal("unknown name must error")
	}
	if _, err := reg.Select(nil, []string{"silver"}); err == nil {
		t.Fatal("unknown tag must error")
	}
	all, _ := reg.Select(nil, nil)
	if len(all) != 2 {
		t.Fatal("empty filter selects all")
	}
	a := reg.Instances[0]
	b := a
	b.ClientID = "other"
	if a.Home("/base") == b.Home("/base") {
		t.Fatal("a new client id must get a new home")
	}
}

func TestScrub(t *testing.T) {
	env := []string{"PATH=/bin", "JAMF_PRO_FLEET_SECRETS=acme=a,globex=g", "jamf_pro_fleet_secrets=acme=a", "JAMF_PRO_BASE_URL=http://evil", "JAMF_PRO_NO_LEARN=1", "JAMF_PRO_CONFIG_DIR=/x"}
	if got := strings.Join(Scrub(env), " "); got != "PATH=/bin JAMF_PRO_NO_LEARN=1" {
		t.Fatalf("scrub = %s", got)
	}
}

func TestRequestRoundTripKeepsRepeatedKeys(t *testing.T) {
	q := url.Values{}
	q.Add("section", "GENERAL")
	q.Add("section", "HARDWARE")
	q.Add("filter", `general.name=="a,b"`)
	enc := Request{Path: "/api/v4/computers-inventory", Query: q}.Encode()
	dec, err := DecodeRequest(enc)
	if err != nil {
		t.Fatal(err)
	}
	if dec.Path != "/api/v4/computers-inventory" || !reflect.DeepEqual(dec.Query["section"], []string{"GENERAL", "HARDWARE"}) || dec.Query.Get("filter") != `general.name=="a,b"` {
		t.Fatalf("round trip = %+v", dec)
	}
}

func TestItemsShapes(t *testing.T) {
	cases := map[string]int{
		`{"totalCount":2,"results":[{"id":"1"},{"id":"2"}]}`: 2,
		`{"policies":[{"id":1,"name":"a"}]}`:                 1,
		`{"policy":{"general":{"id":1}}}`:                    1,
		`[{"a":1},{"a":2},{"a":3}]`:                          3,
		`{"version":"11.10.1"}`:                              1,
		`"x"`:                                                1,
	}
	for body, n := range cases {
		if got, _ := Items(json.RawMessage(body)); len(got) != n {
			t.Errorf("Items(%s) = %d rows, want %d", body, len(got), n)
		}
	}
	if got, _ := Items(json.RawMessage(`{"policy":{"general":{"id":1}}}`)); got[0]["general"] == nil {
		t.Fatal("a Classic object unwraps its root")
	}
}

func TestRedaction(t *testing.T) {
	in := `{"results":[{"name":"x","clientSecret":"s1","password":"p","bootstrapTokenAllowed":true,"bootstrapTokenEscrowedStatus":"ESCROWED",
	"individualRecoveryKeyValidityStatus":"VALID","personalRecoveryKey":"K-1","lapsUserPasswordList":[{"password":"p2"}],
	"extension_attributes":[{"name":"API_TOKEN","value":"tok"},{"name":"Department","value":"Sales"}],"passwordMinLength":8,
	"extensionAttributes":[{"definitionId":"1","name":"Vault Password","values":["v-secret"]},{"name":"Owner","values":["Ada"]}],
	"general":{"payloads":"<plist><key>Password</key><string>wifi-pw</string></plist>"},"app_configuration":{"preferences":"<dict>pref-pw</dict>"},
	"scriptContents":"curl -u admin:script-pw","productKey":"MSU_UPDATE_23F79",
	"files_processes":{"run_command":"curl -H 'Authorization: run-pw'"},"scripts":[{"id":1,"parameter4":"param-pw","parameter11":"param11-pw"}]}]}`
	out, err := RedactJSON(json.RawMessage(in))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, leak := range []string{"s1", `"p"`, "K-1", "p2", "tok", "v-secret", "wifi-pw", "pref-pw", "script-pw", "run-pw", "param-pw", "param11-pw"} {
		if strings.Contains(s, leak) {
			t.Errorf("redacted output still contains %s: %s", leak, s)
		}
	}
	for _, keep := range []string{`"bootstrapTokenAllowed":true`, "ESCROWED", "VALID", "Sales", `"passwordMinLength":8`, "Ada", "MSU_UPDATE_23F79", RedactedDigestPrefix} {
		if !strings.Contains(s, keep) {
			t.Errorf("redaction removed public field %s: %s", keep, s)
		}
	}
}

func TestRSQLQuoteAndFindFilters(t *testing.T) {
	if got := RSQLQuote(`a"b\c*`); got != `"a\"b\\c\*"` {
		t.Fatalf("RSQLQuote = %s", got)
	}
	if _, mob := FindFilters(`ends with "`); !strings.Contains(mob, `displayName=="*ends with \"*"`) {
		t.Fatalf("a trailing quote must stay escaped inside the contains match: %s", mob)
	}
	comp, mob := FindFilters(`C02X*`)
	if !strings.Contains(comp, `hardware.serialNumber=="C02X\*"`) || !strings.Contains(comp, `general.name=="*C02X\**"`) || strings.Count(comp, ",") != 3 {
		t.Fatalf("computer filter = %s", comp)
	}
	if !strings.Contains(mob, `serialNumber=="C02X\*"`) || !strings.Contains(mob, `emailAddress==`) {
		t.Fatalf("mobile filter = %s", mob)
	}
}

func boolp(b bool) *bool { return &b }

func computer(lastContact, os, fv string, fw *bool, mdmExp string) Computer {
	var c Computer
	c.ID, c.General.Name = "1", "mac"
	c.General.LastContact, c.General.MDMProfileExpiration = lastContact, mdmExp
	c.General.RemoteManagement.Managed = boolp(true)
	c.OperatingSystem.Version, c.OperatingSystem.FileVault2Status = os, fv
	c.Security.FirewallEnabled = fw
	return c
}

func TestFailedChecksAndPosture(t *testing.T) {
	now := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	o := PostureOptions{StaleDays: 14, MinOS: "14.6", WithinDays: 30, Now: now}
	healthy := computer("2026-10-14T10:00:00Z", "15.0.1", "BOOT_ENCRYPTED", boolp(true), "2027-06-01T00:00:00Z")
	if got, unk := FailedChecks(healthy, o); len(got) != 0 || len(unk) != 0 {
		t.Fatalf("healthy Mac failed %v unknown %v", got, unk)
	}
	bad := computer("2026-09-01T00:00:00Z", "14.5", "NOT_ENCRYPTED", boolp(false), "2026-10-20T00:00:00Z")
	want := []string{CheckStale, CheckBelowOS, CheckFileVaultOff, CheckFirewallOff, CheckMDMExpiring}
	if got, _ := FailedChecks(bad, o); !reflect.DeepEqual(got, want) {
		t.Fatalf("bad Mac = %v, want %v", got, want)
	}
	unknown := computer("", "", "", nil, "")
	if got, unk := FailedChecks(unknown, o); !reflect.DeepEqual(got, []string{CheckNeverContacted}) || len(unk) != 3 {
		t.Fatalf("missing data: failed %v unknown %v", got, unk)
	}
	fresh := computer("2026-10-14T10:00:00Z", "", "", nil, "")
	if row, _ := Posture("x", InventoryInformation{}, []Computer{fresh}, o); row.Compliant != 0 || row.Incomplete != 1 || *row.PercentCompliant != 0 {
		t.Fatalf("a Mac with no OS/FileVault/firewall data must not count as compliant: %+v", row)
	}
	noState := healthy
	noState.General.RemoteManagement.Managed = nil
	if row, _ := Posture("x", InventoryInformation{}, []Computer{noState}, o); row.Evaluated != 1 || row.ManagementUnknown != 1 {
		t.Fatalf("a computer with no management state must still be evaluated: %+v", row)
	}
	unmanaged := bad
	unmanaged.General.RemoteManagement.Managed = boolp(false)
	row, failing := Posture("acme", InventoryInformation{ManagedComputers: 2, UnmanagedComputers: 1}, []Computer{healthy, bad, unmanaged}, o)
	if row.Evaluated != 2 || row.Failing != 1 || row.Stale != 1 || row.FileVaultOff != 1 || *row.PercentCompliant != 50 || len(failing) != 1 {
		t.Fatalf("posture = %+v failing=%v", row, failing)
	}
	if empty, _ := Posture("x", InventoryInformation{}, nil, o); empty.PercentCompliant != nil {
		t.Fatal("no evaluated Macs must not report a compliance percentage")
	}
}

func TestPatchRows(t *testing.T) {
	titles := []PatchTitle{{ID: "1", DisplayName: "Chrome"}, {ID: "2", DisplayName: "Zoom"}}
	sums := []PatchSummary{{LatestVersion: "130", UpToDate: 3, OutOfDate: 1}, {LatestVersion: "6.2", UpToDate: 0, OutOfDate: 0}}
	pols := []PatchPolicy{{SoftwareTitleConfigurationID: "1", Failed: 2}, {SoftwareTitleConfigurationID: "1", Failed: 1}}
	ups := []UpdateStatus{{Status: "INSTALL_FAILED"}, {Status: "IDLE"}, {Status: "ERROR"}}
	rows := PatchRows("acme", titles, sums, pols, ups)
	if len(rows) != 3 || *rows[0].PercentLatest != 75 || rows[0].PolicyFailures != 3 || rows[1].PercentLatest != nil || rows[2].FailedUpdates != 2 || rows[2].OutOfDate != 0 {
		t.Fatalf("patch rows = %+v", rows)
	}
	if !rows[0].Behind() || rows[1].Behind() || !rows[2].Behind() {
		t.Fatal("Behind mismatch")
	}
}

func TestHealthRows(t *testing.T) {
	now := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	if r := ExpiryRow("a", "ade_token", "abm", "2026-10-20T00:00:00Z", 30, now); r.Status != StatusWarn || *r.DaysLeft != 5 {
		t.Fatalf("soon = %+v", r)
	}
	if r := ExpiryRow("a", "ade_token", "abm", "2026-01-01T00:00:00Z", 30, now); r.Status != StatusFail {
		t.Fatalf("expired = %+v", r)
	}
	if r := ExpiryRow("a", "vpp_token", "vpp", "", 30, now); r.Status != StatusWarn {
		t.Fatalf("missing = %+v", r)
	}
	if r := SyncRow("a", "abm", EnrollmentSync{SyncState: "CONNECTION_ERROR"}); r.Status != StatusFail {
		t.Fatalf("sync = %+v", r)
	}
	rows := []HealthRow{{Item: "version", Name: "11.10.1"}, {Item: "version", Name: "11.9.0"}, {Item: "version", Name: "x", Error: "down"}}
	MarkVersionDrift(rows)
	if rows[0].Status == StatusWarn || rows[1].Status != StatusWarn {
		t.Fatalf("drift = %+v", rows)
	}
}

func TestAppVersionsBothShapes(t *testing.T) {
	xmlish := `{"computer_applications":{"versions":{"version":[{"number":"129.0.1","computers":{"computer":[{"id":1,"name":"mac-1"}]}},{"number":"131.0","computers":{"computer":{"id":2,"name":"mac-2"}}}]}}}`
	flat := `{"computer_applications":{"versions":[{"number":"129.0.1","computers":[{"id":1,"name":"mac-1"}]},{"number":"131.0","computers":[{"id":2,"name":"mac-2"}]}]}}`
	for _, body := range []string{xmlish, flat} {
		v, err := AppVersions(json.RawMessage(body))
		if err != nil {
			t.Fatal(err)
		}
		below := BelowFloor(v, "130")
		if len(below) != 1 || below[0].Number != "129.0.1" || below[0].Computers[0] != "mac-1" {
			t.Fatalf("below = %+v (body %s)", below, body)
		}
	}
	if _, err := AppVersions(json.RawMessage(`{"x":1}`)); err == nil {
		t.Fatal("a body without computer_applications must error")
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{{"129.0.6668.100", "129.0.6668.9", 1}, {"14.6", "14.6.0", 0}, {"11.10.1-t1736", "11.9", 1}, {"13", "14.0", -1}}
	for _, c := range cases {
		if got := CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareVersions(%s,%s) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestParseTime(t *testing.T) {
	for _, s := range []string{"2026-10-15T01:02:03Z", "2026-10-15T01:02:03.456Z", "2026-10-15 01:02:03", "1760490123000"} {
		if _, ok := ParseTime(s); !ok {
			t.Errorf("ParseTime(%q) failed", s)
		}
	}
	if _, ok := ParseTime(""); ok {
		t.Error("empty time must be unset")
	}
}

func TestPolicyDefects(t *testing.T) {
	now := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	k := NewKnown([]Script{{ID: "5", Name: "setup.sh"}}, []Package{{ID: "9", PackageName: "App.pkg"}})
	broken := `{"policy":{"general":{"id":7,"name":"Old","enabled":true,"trigger_checkin":false,"trigger_other":"","category":{"id":-1,"name":"No category assigned"},
	"date_time_limitations":{"expiration_date_epoch":1700000000000}},"scope":{"all_computers":false,"computers":[],"computer_groups":[]},
	"self_service":{"use_for_self_service":false},"scripts":{"size":1,"script":[{"id":6,"name":"gone.sh"}]},
	"package_configuration":{"packages":[{"package":{"id":9,"name":"App.pkg"}}]}}}`
	row, err := PolicyDefects("acme", json.RawMessage(broken), k, now)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{DefectNoTrigger, DefectNoScope, DefectExpired, DefectUncategorized, DefectMissingScript}
	if !reflect.DeepEqual(row.Defects, want) || row.ID != "7" {
		t.Fatalf("defects = %v (id %s), want %v", row.Defects, row.ID, want)
	}
	healthy := `{"policy":{"general":{"id":8,"name":"Good","enabled":true,"trigger_checkin":true,"category":{"id":2,"name":"Apps"}},
	"scope":{"all_computers":true},"scripts":[{"id":5,"name":"renamed.sh"}],"package_configuration":{"packages":[{"id":9,"name":"App.pkg"}]}}}`
	row, err = PolicyDefects("acme", json.RawMessage(healthy), k, now)
	if err != nil || len(row.Defects) != 0 {
		t.Fatalf("healthy policy (script matched by id after a rename) = %v %v", row.Defects, err)
	}
	recreated := `{"policy":{"general":{"id":9,"name":"R","enabled":true,"trigger_checkin":true,"category":{"id":2,"name":"Apps"}},
	"scope":{"all_computers":true},"scripts":[{"id":99,"name":"setup.sh"}]}}`
	if row, _ := PolicyDefects("acme", json.RawMessage(recreated), k, now); !reflect.DeepEqual(row.Defects, []string{DefectMissingScript}) {
		t.Fatalf("a deleted script recreated under the same name must still be missing: %v", row.Defects)
	}
	noID := `{"policy":{"general":{"id":10,"name":"N","enabled":true,"trigger_checkin":true,"category":{"id":2,"name":"Apps"}},
	"scope":{"all_computers":true},"scripts":[{"name":"setup.sh"}]}}`
	if row, _ := PolicyDefects("acme", json.RawMessage(noID), k, now); len(row.Defects) != 0 {
		t.Fatalf("a reference without an id falls back to the name: %v", row.Defects)
	}
	if _, err := PolicyDefects("acme", json.RawMessage(`{"x":1}`), k, now); err == nil {
		t.Fatal("a body without policy must error")
	}
}

func TestDiff(t *testing.T) {
	golden := Snapshot{KindScript: {"a.sh": Fingerprint("echo 1\necho 2"), "b.sh": Fingerprint("echo 2")}, KindPackage: {"X.pkg": ""}}
	other := Snapshot{KindScript: {"a.sh": Fingerprint("echo 1\r\necho 2"), "b.sh": Fingerprint("echo old"), "c.sh": ""}, KindPackage: {}}
	rows := Diff("acme", golden, other, true)
	var got []string
	for _, r := range rows {
		got = append(got, r.Kind+":"+r.Name+":"+r.Status)
	}
	want := []string{"package:X.pkg:missing", "script:b.sh:differs", "script:c.sh:extra"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("diff = %v, want %v", got, want)
	}
	if rows := Diff("acme", golden, golden, true); len(rows) != 0 {
		t.Fatal("identical snapshots must not drift")
	}
	if Fingerprint("echo 1") != Fingerprint("echo 1") || Fingerprint("") == "" || Fingerprint("") == Fingerprint("x") {
		t.Fatal("fingerprint")
	}
	emptyGolden := Snapshot{KindScript: {"a.sh": Fingerprint("")}}
	if rows := Diff("acme", emptyGolden, Snapshot{KindScript: {"a.sh": Fingerprint("rm -rf /")}}, false); len(rows) != 1 || rows[0].Status != DriftDiffers {
		t.Fatalf("an empty golden script must still differ from a non-empty copy: %v", rows)
	}
}

func TestObjectsUnder(t *testing.T) {
	cases := map[string]int{
		`[{"id":1},{"id":2}]`:                       2,
		`[{"computer":{"id":1}}]`:                   1,
		`{"size":2,"computer":[{"id":1},{"id":2}]}`: 2,
		`{"computer":{"id":1}}`:                     1,
		`null`:                                      0,
	}
	for body, n := range cases {
		var v any
		_ = json.Unmarshal([]byte(body), &v)
		if got := ObjectsUnder(v, "computer"); len(got) != n {
			t.Errorf("ObjectsUnder(%s) = %d, want %d", body, len(got), n)
		}
	}
}

func TestStrictDecoders(t *testing.T) {
	for _, bad := range []string{`null`, `{}`, `{"results":null}`, `{"httpStatus":401,"errors":[]}`, `[]`} {
		if _, err := Results[Computer](json.RawMessage(bad)); err == nil {
			t.Errorf("Results(%s) must be an error, not an empty collection", bad)
		}
	}
	if got, err := Results[Computer](json.RawMessage(`{"totalCount":0,"results":[]}`)); err != nil || len(got) != 0 {
		t.Fatalf("an empty page is a valid empty collection: %v %v", got, err)
	}
	var info InventoryInformation
	if err := DecodeObject(json.RawMessage(`{"managedComputers":1}`), &info, InventoryInformationKeys...); err == nil {
		t.Fatal("a partial inventory-information body must be an error")
	}
	if err := DecodeObject(json.RawMessage(`null`), &info); err == nil {
		t.Fatal("null must be an error")
	}
}

func TestStrictDecodersRound2(t *testing.T) {
	var sum PatchSummary
	if err := DecodeObject(json.RawMessage(`{"upToDate":null,"outOfDate":null}`), &sum, "upToDate", "outOfDate"); err == nil {
		t.Fatal("null required values must be rejected")
	}
	if _, err := Results[Computer](json.RawMessage(`{"totalCount":1,"results":[null]}`), "id"); err == nil {
		t.Fatal("a null result entry must be rejected")
	}
	if _, err := Results[Computer](json.RawMessage(`{"totalCount":1,"results":[{}]}`), "id"); err == nil {
		t.Fatal("a result entry without its required id must be rejected")
	}
	for _, bad := range []string{`null`, `{}`, `{"policies":null}`, `{"policies":"x"}`} {
		if _, err := ClassicList(json.RawMessage(bad), "policies", "policy"); err == nil {
			t.Errorf("ClassicList(%s) must be an error", bad)
		}
	}
	for body, n := range map[string]int{`{"policies":[]}`: 0, `{"policies":{"size":0}}`: 0, `{"policies":[{"id":1,"name":"a"}]}`: 1} {
		if got, err := ClassicList(json.RawMessage(body), "policies", "policy"); err != nil || len(got) != n {
			t.Errorf("ClassicList(%s) = %d, %v; want %d", body, len(got), err, n)
		}
	}
	if _, err := AppVersions(json.RawMessage(`{"computer_applications":{}}`)); err == nil {
		t.Fatal("an application body without a versions list must be an error")
	}
}

func TestURLRedaction(t *testing.T) {
	in := `{"webhook":{"name":"Slack","url":"https://hooks.slack.com/services/T000/B000/slack-secret","icon_url":"https://jamf.example.com/"},
	"distribution_point":{"server":"https://svc:dp-pass@dp.example.com/share","password":"https://u:pw-user@host.example.com/path-secret"},
	"self_service_icon":{"uri":"https://acme.jamfcloud.com/icon?id=1&token=icon-tok"}}`
	out, err := RedactJSON(json.RawMessage(in))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, leak := range []string{"slack-secret", "dp-pass", "icon-tok", "T000/B000", "pw-user", "path-secret"} {
		if strings.Contains(s, leak) {
			t.Errorf("URL credential %q not redacted: %s", leak, s)
		}
	}
	// A credential-bearing URL is withheld whole; a bare host URL stays.
	for _, keep := range []string{`"url":"[redacted]"`, `"server":"[redacted]"`, `"icon_url":"https://jamf.example.com/"`} {
		if !strings.Contains(s, keep) {
			t.Errorf("want %s in %s", keep, s)
		}
	}
}

func TestAppVersionsNestedArrays(t *testing.T) {
	nested := `{"computer_applications":{"versions":{"version":[[{"number":"99.0","computers":{"computer":[{"id":1,"name":"old-mac"}]}}]]}}}`
	v, err := AppVersions(json.RawMessage(nested))
	if err != nil || len(v) != 1 || v[0].Number != "99.0" || len(v[0].Computers) != 1 {
		t.Fatalf("nested version arrays must be flattened, got %+v %v", v, err)
	}
}

// TestListContradictions: a declared Classic size must match its entries, a
// null versions list is an error, and a page reporting a total with no
// results is not an empty list.
func TestListContradictions(t *testing.T) {
	if _, err := ClassicList(json.RawMessage(`{"policies":{"size":4}}`), "policies", "policy"); err == nil {
		t.Fatal("size 4 with no entries must be an error")
	}
	if items, err := ClassicList(json.RawMessage(`{"policies":{"size":"1","policy":{"id":1}}}`), "policies", "policy"); err != nil || len(items) != 1 {
		t.Fatalf("a matching size must pass: %v %v", items, err)
	}
	if items, err := ClassicList(json.RawMessage(`{"policies":{"size":0}}`), "policies", "policy"); err != nil || len(items) != 0 {
		t.Fatalf("size 0 is a real empty list: %v %v", items, err)
	}
	if _, err := AppVersions(json.RawMessage(`{"computer_applications":{"versions":null}}`)); err == nil {
		t.Fatal("versions: null must be an error")
	}
	if _, err := Results[Named](json.RawMessage(`{"totalCount":2,"results":[]}`), "name"); err == nil {
		t.Fatal("totalCount 2 with empty results must be an error")
	}
	if got, err := Results[Named](json.RawMessage(`{"totalCount":0,"results":[]}`), "name"); err != nil || len(got) != 0 {
		t.Fatalf("a real empty page must pass: %v %v", got, err)
	}
}

// TestRedactLabelledRecords: a name/value record labelled by displayName
// (mobile group membership extension attributes) is masked like one
// labelled by name.
func TestRedactLabelledRecords(t *testing.T) {
	var v any
	_ = json.Unmarshal([]byte(`[{"displayName":"API Password","value":"s3cret"},{"label":"Owner","value":"Ada"}]`), &v)
	out, _ := json.Marshal(Redact(v))
	if strings.Contains(string(out), "s3cret") || !strings.Contains(string(out), "Ada") {
		t.Fatalf("redacted = %s", out)
	}
}

// TestClassicEntryWrappers: a list entry that wraps its object beside the
// XML-derived size is unwrapped, and entries missing a required key fail.
func TestClassicEntryWrappers(t *testing.T) {
	items, err := ClassicList(json.RawMessage(`{"policies":[{"size":1,"policy":{"id":7,"name":"x"}}]}`), "policies", "policy", "id")
	if err != nil || len(items) != 1 || IDString(items[0]["id"]) != "7" {
		t.Fatalf("items = %v, err = %v", items, err)
	}
	if _, err := ClassicList(json.RawMessage(`{"policies":[{"name":"no id"}]}`), "policies", "policy", "id"); err == nil {
		t.Fatal("an entry without an id must fail")
	}
}

// TestRedactLabelIgnoresFieldExemptions: a record labelled with an exempt
// field name (productKey) is still judged by its label text.
func TestRedactLabelIgnoresFieldExemptions(t *testing.T) {
	var v any
	_ = json.Unmarshal([]byte(`{"name":"productKey","values":["private-license-key"]}`), &v)
	if b, _ := json.Marshal(Redact(v)); strings.Contains(string(b), "private-license-key") {
		t.Fatalf("a secret-labelled record must be masked even when its label is an exempt field name: %s", b)
	}
}

// TestClassicMembersAndUnknownVersions: a null or scalar list entry fails,
// and an install with no version is reported, not treated as current.
func TestClassicMembersAndUnknownVersions(t *testing.T) {
	for _, body := range []string{`{"policies":[null]}`, `{"policies":[{"id":1},"x"]}`, `{"policies":{"policy":"x"}}`} {
		if _, err := ClassicList(json.RawMessage(body), "policies", "policy", "id"); err == nil {
			t.Fatalf("%s must fail", body)
		}
	}
	got := BelowFloor([]AppVersion{{Number: "", Computers: []string{"m1"}}, {Number: "200.0", Computers: []string{"m2"}}}, "100.0")
	if len(got) != 1 || got[0].Number != "" {
		t.Fatalf("an unversioned install must be reported: %v", got)
	}
}

// TestRedactURLsInListsAndNestedClassic: a URL inside a list loses its
// credentials like a URL in a field, and malformed nested Classic lists fail.
func TestRedactURLsInListsAndNestedClassic(t *testing.T) {
	var v any
	_ = json.Unmarshal([]byte(`{"name":"Repository address","values":["https://svc:pa55word@host/path"],
		"webhook_urls":["https://hooks.example/services/T/B/s3cr3t"],"list":["ftp://u:p4ss@h/x"]}`), &v)
	out, _ := json.Marshal(Redact(v))
	for _, leak := range []string{"pa55word", "s3cr3t", "p4ss"} {
		if strings.Contains(string(out), leak) {
			t.Fatalf("%s survived: %s", leak, out)
		}
	}
	for _, body := range []string{`{"policies":[{"size":1,"policy":[null]}]}`, `{"policies":[{"size":2,"policy":[{"id":1}]}]}`} {
		if _, err := ClassicList(json.RawMessage(body), "policies", "policy"); err == nil {
			t.Fatalf("%s must fail", body)
		}
	}
	if _, err := AppVersions(json.RawMessage(`{"computer_applications":{"versions":[{"number":"1","computers":{"size":1,"computer":[null]}}]}}`)); err == nil {
		t.Fatal("a malformed computers list must fail")
	}
}

// TestClassicContainerNeedsEntriesOrSizeZero: an object container without
// its entries is not an empty list unless it says size 0.
func TestClassicContainerNeedsEntriesOrSizeZero(t *testing.T) {
	if _, err := AppVersions(json.RawMessage(`{"computer_applications":{"versions":{}}}`)); err == nil {
		t.Fatal("versions {} must fail")
	}
	if _, err := ClassicList(json.RawMessage(`{"policies":{}}`), "policies", "policy"); err == nil {
		t.Fatal("policies {} must fail")
	}
	if items, err := ClassicList(json.RawMessage(`{"policies":{"size":0}}`), "policies", "policy"); err != nil || len(items) != 0 {
		t.Fatalf("size 0 is a real empty list: %v %v", items, err)
	}
}

// TestItemsNullMember: a null list member is an error, not a nil row.
func TestItemsNullMember(t *testing.T) {
	for _, body := range []string{`{"totalCount":1,"results":[null]}`, `[null]`, `{"policies":[{"id":1},null]}`} {
		if _, err := Items(json.RawMessage(body)); err == nil {
			t.Errorf("%s must be an error", body)
		}
	}
}

// TestPolicyWithoutScopeIsAnError: a policy whose scope cannot be read is
// not "no defects".
func TestPolicyWithoutScopeIsAnError(t *testing.T) {
	for _, body := range []string{`{"policy":{"general":{"id":1,"name":"p","enabled":true,"trigger_checkin":true}}}`,
		`{"policy":{"general":{"id":1,"name":"p","enabled":true,"trigger_checkin":true},"scope":null}}`} {
		if _, err := PolicyDefects("acme", json.RawMessage(body), NewKnown(nil, nil), time.Now()); err == nil {
			t.Fatalf("%s must be an error", body)
		}
	}
}

// Only the two statuses Jamf Pro documents as encrypted pass, the two partial or
// absent ones fail, and anything else is unknown rather than compliant.
func TestFileVaultStatusIsAnAllowList(t *testing.T) {
	now, _ := ParseTime("2026-10-15T00:00:00Z")
	o := PostureOptions{StaleDays: 14, WithinDays: 30, Now: now}
	for status, want := range map[string]string{
		"ALL_ENCRYPTED": "pass", "BOOT_ENCRYPTED": "pass",
		"NOT_ENCRYPTED": "fail", "SOME_ENCRYPTED": "fail",
		"NOT_APPLICABLE": "unknown", "": "unknown", "SOMETHING_NEW": "unknown",
	} {
		failed, unknown := FailedChecks(computer("2026-10-14T10:00:00Z", "15.0.1", status, boolp(true), "2027-06-01T00:00:00Z"), o)
		got := "pass"
		if slices.Contains(failed, CheckFileVaultOff) {
			got = "fail"
		} else if slices.Contains(unknown, CheckFileVaultOff) {
			got = "unknown"
		}
		if got != want {
			t.Errorf("%q = %s, want %s", status, got, want)
		}
	}
}
