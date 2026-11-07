// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"eset-protect-pp-cli/internal/cliutil/testenv"
)

// TestNovelDetectionsSpreadHelpWires smoke-tests that the detections spread command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelDetectionsSpreadHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"detections", "spread", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("detections spread --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "spread"} {
		if !strings.Contains(help, want) {
			t.Fatalf("detections spread --help missing %q in output:\n%s", want, help)
		}
	}
}

// TestDetectionsSpreadWiredOnce guards the wire hook: exactly one, real
// (non-scaffold) `spread` under `detections`.
func TestDetectionsSpreadWiredOnce(t *testing.T) {
	testenv.Isolate(t)
	root := RootCmd()
	detections, _, err := root.Find([]string{"detections"})
	if err != nil || detections == root {
		t.Fatalf("detections parent not found: %v", err)
	}
	n := 0
	for _, c := range detections.Commands() {
		if c.Name() == "spread" {
			n++
			if isNovelScaffoldCommand(c) || c.Annotations["pp:data-source"] != "local" || c.Annotations["mcp:read-only"] != "true" {
				t.Fatalf("spread annotations = %v", c.Annotations)
			}
		}
	}
	if n != 1 {
		t.Fatalf("detections has %d spread subcommands, want 1", n)
	}
}

const (
	detectionsSpreadTestHashA = "A94A8FE5CCB19BA61C4C0873D391E987982FBBD3" // upper-case in the detection
	detectionsSpreadTestHashB = "3395856ce81f2b7382dee72602f798b642f14140"
	detectionsSpreadTestHashC = "da39a3ee5e6b4b0d3255bfef95601890afd80709"
)

func detectionsSpreadTestDetection(uuid, sha1, typeName, object, device, severity, occur string) string {
	return fmt.Sprintf(`{"uuid":%q,"displayName":%q,"typeName":%q,"objectName":%q,"objectHashSha1":%q,"occurTime":%q,"severityLevel":%q,"category":"DETECTION_CATEGORY_ANTIVIRUS","context":{"deviceUuid":%q,"userName":"ACME\\frontdesk"}}`,
		uuid, typeName+" detection", typeName, object, sha1, occur, severity, device)
}

func detectionsSpreadTestFixture() map[string][]string {
	ago := func(d time.Duration) string { return time.Now().Add(-d).UTC().Format(time.RFC3339) }
	return map[string][]string{
		"device-groups": {
			`{"uuid":"msp","displayName":"Servosity MSP","linkedEntityType":"DEVICE_GROUP_ENTITY_TYPE_MSP"}`,
			`{"uuid":"acme","displayName":"Acme Dental","parentGroupUuid":"msp","linkedEntityType":"DEVICE_GROUP_ENTITY_TYPE_COMPANY"}`,
			`{"uuid":"beta","displayName":"Beta Law","parentGroupUuid":"msp","linkedEntityType":"DEVICE_GROUP_ENTITY_TYPE_COMPANY"}`,
		},
		"devices": {
			`{"uuid":"a1","displayName":"FRONTDESK-01","parentGroupUuid":"acme"}`,
			`{"uuid":"a2","displayName":"XRAY-02","parentGroupUuid":"acme"}`,
			`{"uuid":"b1","displayName":"PARTNER-01","parentGroupUuid":"beta"}`,
		},
		"detections": {
			detectionsSpreadTestDetection("det1", detectionsSpreadTestHashA, "Win32/Agent.XYZ", `C:\Users\x\evil.exe`, "a1", "SEVERITY_LEVEL_HIGH", ago(24*time.Hour)),
			detectionsSpreadTestDetection("det2", strings.ToLower(detectionsSpreadTestHashA), "Win32/Agent.XYZ", `C:\Temp\evil.exe`, "b1", "SEVERITY_LEVEL_MEDIUM", ago(48*time.Hour)),
			detectionsSpreadTestDetection("det3", detectionsSpreadTestHashA, "Win32/Agent.XYZ", `C:\Users\x\evil.exe`, "a1", "SEVERITY_LEVEL_LOW", ago(3*time.Hour)),
			detectionsSpreadTestDetection("det4", detectionsSpreadTestHashA, "Win32/Agent.XYZ", `D:\latest\evil.exe`, "f00dbabe-0000-4000-8000-0000000000ff", "SEVERITY_LEVEL_LOW", ago(time.Hour)),
			detectionsSpreadTestDetection("det5", detectionsSpreadTestHashB, "PUA/Toolbar", `C:\tool.exe`, "a2", "SEVERITY_LEVEL_LOW", ago(24*time.Hour)),
			detectionsSpreadTestDetection("det6", "", "HIPS rule", "powershell.exe", "a1", "SEVERITY_LEVEL_MEDIUM", ago(5*time.Hour)),
			detectionsSpreadTestDetection("det7", "", "HIPS rule", "powershell.exe", "a2", "SEVERITY_LEVEL_MEDIUM", ago(6*time.Hour)),
			detectionsSpreadTestDetection("det8", detectionsSpreadTestHashC, "Win32/Old", `C:\old.exe`, "b1", "SEVERITY_LEVEL_HIGH", ago(30*24*time.Hour)),
			detectionsSpreadTestDetection("det9", detectionsSpreadTestHashC, "Win32/Old", `C:\old.exe`, "b1", "SEVERITY_LEVEL_HIGH", "not-a-time"),
		},
		"executables": {
			`{"uuid":"e-aaa","displayName":"evil.exe","hashSha1":"` + strings.ToLower(detectionsSpreadTestHashA) + `","isBlocked":false,"devicesCount":"12","signatures":[{"signatureTrustLevel":"SIGNATURE_TRUST_LEVEL_NO_SIGNATURE"}],"liveGridFindings":{"reputation":0.12,"popularity":0.01}}`,
			`{"uuid":"e-bbb","displayName":"tool.exe","hashSha1":"` + strings.ToUpper(detectionsSpreadTestHashB) + `","isBlocked":true,"devicesCount":1,"signatures":[{"signerName":"Toolbar Vendor Ltd","signatureTrustLevel":"SIGNATURE_TRUST_LEVEL_OS_TRUSTED"}],"liveGridFindings":{"reputation":0.95}}`,
		},
	}
}

func detectionsSpreadTestRun(t *testing.T, path string, extra ...string) detectionsSpreadView {
	t.Helper()
	args := append([]string{"detections", "spread", "--db", path, "--json"}, extra...)
	out, err := fleetHealthTestRun(t, args...)
	if err != nil {
		t.Fatalf("detections spread %v: %v\n%s", extra, err, out)
	}
	var v detectionsSpreadView
	fleetHealthTestDecode(t, out, &v)
	return v
}

func TestDetectionsSpreadEndToEnd(t *testing.T) {
	testenv.Isolate(t)
	path := fleetHealthTestSeed(t, detectionsSpreadTestFixture())
	v := detectionsSpreadTestRun(t, path)

	keys := make([]string, 0, len(v.Groups))
	for _, g := range v.Groups {
		keys = append(keys, g.Key)
	}
	want := []string{strings.ToLower(detectionsSpreadTestHashA), "name:HIPS rule|powershell.exe", detectionsSpreadTestHashB}
	if strings.Join(keys, ",") != strings.Join(want, ",") {
		t.Fatalf("group order = %v, want %v", keys, want)
	}
	// det9's occurTime "not-a-time" cannot be placed in the window: it is
	// counted in rejected_rows rather than silently dropped.
	if v.DetectionsScanned != 9 || v.DetectionsInWindow != 7 || v.GroupsMatched != 3 || v.Since == "" || v.RejectedRows != 1 {
		t.Fatalf("view meta = scanned %d in_window %d matched %d since %q rejected %d", v.DetectionsScanned, v.DetectionsInWindow, v.GroupsMatched, v.Since, v.RejectedRows)
	}

	a := v.Groups[0]
	if a.SHA1 != strings.ToLower(detectionsSpreadTestHashA) || a.Occurrences != 4 || a.Devices != 3 || a.CustomersKnown != 2 ||
		a.DevicesUnknownOwner != 1 || a.CustomersPossible != 3 || !a.ThresholdsProven || a.UnattributedDetections != 1 {
		t.Fatalf("hash A group = %+v", a)
	}
	if strings.Join(a.Customers, "|") != "Acme Dental|Beta Law" {
		t.Fatalf("hash A customers = %v (an unsynced device is an unknown owner, never a customer)", a.Customers)
	}
	if a.SeverityMax != "HIGH" || a.Object != `D:\latest\evil.exe` || a.Name != "Win32/Agent.XYZ detection" {
		t.Fatalf("hash A severity/object/name = %q %q %q", a.SeverityMax, a.Object, a.Name)
	}
	first, ferr := time.Parse(time.RFC3339, a.FirstSeen)
	last, lerr := time.Parse(time.RFC3339, a.LastSeen)
	if ferr != nil || lerr != nil || time.Since(first) < 47*time.Hour || time.Since(last) > 2*time.Hour {
		t.Fatalf("first/last seen = %s / %s", a.FirstSeen, a.LastSeen)
	}
	if a.Executable == nil || a.Executable.UUID != "e-aaa" || a.Executable.IsBlocked || a.Executable.DevicesCount != 12 ||
		a.Executable.Reputation == nil || *a.Executable.Reputation != 0.12 || a.Executable.ReputationVerdict != "malicious" ||
		a.Executable.Signer != "" || a.Executable.SignatureTrust != "NO_SIGNATURE" {
		t.Fatalf("hash A executable = %+v", a.Executable)
	}
	if a.Next != "eset-protect-cli executables block e-aaa --dry-run" {
		t.Fatalf("hash A next = %q", a.Next)
	}

	n := v.Groups[1]
	if n.SHA1 != "" || n.Executable != nil || n.Next != "" || n.Devices != 2 || n.CustomersKnown != 1 || n.Customers[0] != "Acme Dental" {
		t.Fatalf("name group = %+v", n)
	}

	b := v.Groups[2]
	if b.Executable == nil || !b.Executable.IsBlocked || b.Executable.Signer != "Toolbar Vendor Ltd" || b.Executable.ReputationVerdict != "safe" || b.Next != "" {
		t.Fatalf("hash B (blocked, upper-case executable hash) = %+v / %+v", b, b.Executable)
	}

	// The raw JSON keeps executable null for unknown hashes.
	out, _ := fleetHealthTestRun(t, "detections", "spread", "--db", path, "--json")
	if !strings.Contains(string(out), `"executable": null`) {
		t.Fatalf("unknown executable must serialize as null:\n%s", out)
	}
}

func TestDetectionsSpreadEndToEndFilters(t *testing.T) {
	testenv.Isolate(t)
	path := fleetHealthTestSeed(t, detectionsSpreadTestFixture())

	// --sha1 is case-insensitive in both directions.
	for _, h := range []string{strings.ToLower(detectionsSpreadTestHashA), detectionsSpreadTestHashA} {
		v := detectionsSpreadTestRun(t, path, "--sha1", h)
		if len(v.Groups) != 1 || v.Groups[0].Occurrences != 4 || v.DetectionsInWindow != 4 {
			t.Fatalf("--sha1 %s = %+v (in_window %d)", h, v.Groups, v.DetectionsInWindow)
		}
	}
	if v := detectionsSpreadTestRun(t, path, "--min-customers", "2"); len(v.Groups) != 1 || v.Groups[0].CustomersKnown != 2 || v.Groups[0].CustomersPossible != 3 {
		t.Fatalf("--min-customers 2 = %+v", v.Groups)
	}
	if v := detectionsSpreadTestRun(t, path, "--min-devices", "2"); len(v.Groups) != 2 || v.GroupsMatched != 2 {
		t.Fatalf("--min-devices 2 = %+v", v.Groups)
	}
	// A wider window pulls in the 30-day-old hash C; the malformed time stays
	// out and is counted as rejected.
	v := detectionsSpreadTestRun(t, path, "--since", "60d")
	if v.DetectionsInWindow != 8 || len(v.Groups) != 4 || v.RejectedRows != 1 {
		t.Fatalf("--since 60d: in_window %d groups %d rejected %d", v.DetectionsInWindow, len(v.Groups), v.RejectedRows)
	}
	if v := detectionsSpreadTestRun(t, path, "--limit", "1"); len(v.Groups) != 1 || v.GroupsMatched != 3 {
		t.Fatalf("--limit 1: groups %d matched %d", len(v.Groups), v.GroupsMatched)
	}
	for _, bad := range [][]string{{"--sha1", "abc"}, {"--sha1", strings.Repeat("z", 40)}, {"--since", "soon"}, {"--min-devices", "-1"}} {
		args := append([]string{"detections", "spread", "--db", path, "--json"}, bad...)
		if _, err := fleetHealthTestRun(t, args...); err == nil {
			t.Fatalf("%v must be rejected", bad)
		}
	}
}

func TestDetectionsSpreadEndToEndEmpty(t *testing.T) {
	testenv.Isolate(t)
	path := fleetHealthTestSeed(t, map[string][]string{})
	out, err := fleetHealthTestRun(t, "detections", "spread", "--db", path, "--json")
	if err != nil {
		t.Fatalf("empty mirror: %v", err)
	}
	if !strings.Contains(string(out), `"groups": []`) || !strings.Contains(string(out), `"rejected_rows": 0`) {
		t.Fatalf("groups must render as [] and rejected_rows as 0:\n%s", out)
	}
	out, err = fleetHealthTestRun(t, "detections", "spread", "--db", filepath.Join(t.TempDir(), "absent.db"), "--json")
	if err != nil {
		t.Fatalf("missing mirror must exit 0: %v", err)
	}
	var v detectionsSpreadView
	fleetHealthTestDecode(t, out, &v)
	if v.Groups == nil || len(v.Groups) != 0 || v.DetectionsScanned != 0 {
		t.Fatalf("missing mirror view = %+v", v)
	}
}

func TestDetectionsSpreadPureHelpers(t *testing.T) {
	if got := detectionsSpreadKey(detectionsSpreadDetection{ObjectHashSha1: " ABCDEF "}); got != "abcdef" {
		t.Errorf("hash key = %q", got)
	}
	if got := detectionsSpreadKey(detectionsSpreadDetection{TypeName: "HIPS rule", ObjectName: "cmd.exe"}); got != "name:HIPS rule|cmd.exe" {
		t.Errorf("fallback key = %q", got)
	}
	if h, err := detectionsSpreadNormalizeSHA1(" " + detectionsSpreadTestHashA + " "); err != nil || h != strings.ToLower(detectionsSpreadTestHashA) {
		t.Errorf("normalize = %q %v", h, err)
	}
	if h, err := detectionsSpreadNormalizeSHA1(""); err != nil || h != "" {
		t.Errorf("empty --sha1 must mean no filter: %q %v", h, err)
	}
	if _, err := detectionsSpreadNormalizeSHA1(strings.Repeat("a", 64)); err == nil {
		t.Error("a SHA-256 digest must be refused")
	}

	f := func(x float64) *float64 { return &x }
	verdicts := []struct {
		r    *float64
		want string
	}{{nil, ""}, {f(0.29), "malicious"}, {f(0.3), "suspicious"}, {f(0.8), "suspicious"}, {f(0.81), "safe"}}
	for _, c := range verdicts {
		if got := detectionsSpreadReputationVerdict(c.r); got != c.want {
			t.Errorf("verdict(%v) = %q, want %q", c.r, got, c.want)
		}
	}

	var rej esetRejects
	execs := detectionsSpreadIndexExecutables([]json.RawMessage{
		json.RawMessage(`{"uuid":"e1","hashSha1":"ABC","liveGridFindings":{"reputation":"0.5"}}`),
		json.RawMessage(`{"uuid":"e2","hashSha1":"def"}`),
		json.RawMessage(`{"uuid":"","hashSha1":"ghi"}`),
		json.RawMessage(`not json`),
		json.RawMessage(`{"uuid":"e5","hashSha1":"jkl","devicesCount":"many"}`),
		json.RawMessage(`{"uuid":"e6"}`),
	}, &rej)
	if len(execs) != 2 || execs["abc"] == nil || *execs["abc"].Reputation != 0.5 || execs["def"].Reputation != nil || execs["def"].ReputationVerdict != "" {
		t.Fatalf("executable index = %+v", execs)
	}
	// no uuid, not JSON, and a non-integer devicesCount are rejected; a row
	// without a hash simply cannot be joined.
	if rej.N != 3 || execs["jkl"] != nil {
		t.Fatalf("executable rejects = %+v", rej)
	}

	res := detectionsSpreadBuild(nil, nil, newEsetTree(nil), nil, detectionsSpreadOptions{MinCustomers: 1, MinDevices: 1}, nil)
	if res.Groups == nil || len(res.Groups) != 0 || res.Scanned != 0 || res.Rejected != 0 {
		t.Fatalf("empty build = %+v, want empty non-nil groups", res)
	}

	// No window: rows without a parseable time still count; an unknown
	// severity value is kept when nothing ranks higher.
	raw := []json.RawMessage{json.RawMessage(`{"objectHashSha1":"aa","typeName":"X","severityLevel":"SEVERITY_LEVEL_EXTREME","context":{}}`)}
	res = detectionsSpreadBuild(raw, nil, newEsetTree(nil), nil, detectionsSpreadOptions{}, nil)
	if len(res.Groups) != 1 || res.Groups[0].SeverityMax != "EXTREME" || res.Groups[0].Customers[0] != esetCustomerTenant || res.Groups[0].Devices != 0 || res.Rejected != 0 {
		t.Fatalf("no-window build = %+v", res.Groups)
	}
	// The default --min-devices 1 drops a group with no device at all.
	res = detectionsSpreadBuild(raw, nil, newEsetTree(nil), nil, detectionsSpreadOptions{MinDevices: 1}, nil)
	if len(res.Groups) != 0 {
		t.Fatalf("deviceless group must be filtered by min-devices 1: %+v", res.Groups)
	}

	// Wrongly typed detection rows are rejected, not dropped silently.
	bad := []json.RawMessage{
		json.RawMessage(`{"uuid":"d1","objectHashSha1":"aa","occurTime":1760430000,"context":{"deviceUuid":"x"}}`),
		json.RawMessage(`{"uuid":"d2","objectHashSha1":"aa","occurTime":"2026-10-14T08:00:00Z","context":"x"}`),
		json.RawMessage(`[]`),
	}
	rej = esetRejects{}
	res = detectionsSpreadBuild(bad, nil, newEsetTree(nil), nil, detectionsSpreadOptions{}, &rej)
	if res.Rejected != 3 || rej.N != 3 || res.Scanned != 3 || res.InWindow != 0 || len(res.Groups) != 0 {
		t.Fatalf("wrongly typed rows = %+v / %+v", res, rej)
	}
}

// TestDetectionsSpreadCustomersByUUID: two different customers both named
// "Acme" are two customers. Deduplicating by display name used to report
// customer_count 1 and hide the hash under --min-customers 2.
func TestDetectionsSpreadCustomersByUUID(t *testing.T) {
	const acme1, acme2 = "aaaaaaaa-1111-4000-8000-000000000001", "bbbbbbbb-2222-4000-8000-000000000002"
	tree := newEsetTree([]esetGroup{
		{UUID: "msp", Name: "Servosity MSP", EntityType: "DEVICE_GROUP_ENTITY_TYPE_MSP"},
		{UUID: acme1, Name: "Acme", ParentUUID: "msp", EntityType: esetCompanyEntityType},
		{UUID: acme2, Name: "Acme", ParentUUID: "msp", EntityType: esetCompanyEntityType},
		{UUID: "beta", Name: "Beta Law", ParentUUID: "msp", EntityType: esetCompanyEntityType},
	})
	devices := []esetDevice{
		{UUID: "d1", ParentGroupUUID: acme1}, {UUID: "d2", ParentGroupUUID: acme2},
		{UUID: "d3", ParentGroupUUID: acme1}, {UUID: "d4", ParentGroupUUID: "beta"},
	}
	when := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	raw := []json.RawMessage{
		json.RawMessage(detectionsSpreadTestDetection("x1", detectionsSpreadTestHashB, "Trojan", "a.exe", "d1", "SEVERITY_LEVEL_HIGH", when)),
		json.RawMessage(detectionsSpreadTestDetection("x2", detectionsSpreadTestHashB, "Trojan", "a.exe", "d2", "SEVERITY_LEVEL_HIGH", when)),
		json.RawMessage(detectionsSpreadTestDetection("x3", detectionsSpreadTestHashB, "Trojan", "a.exe", "d3", "SEVERITY_LEVEL_HIGH", when)),
		json.RawMessage(detectionsSpreadTestDetection("x4", detectionsSpreadTestHashC, "PUA", "b.exe", "d4", "SEVERITY_LEVEL_LOW", when)),
	}
	res := detectionsSpreadBuild(raw, devices, tree, nil, detectionsSpreadOptions{Since: time.Now().Add(-24 * time.Hour), MinCustomers: 2}, nil)
	if len(res.Groups) != 1 || res.Groups[0].SHA1 != detectionsSpreadTestHashB {
		t.Fatalf("--min-customers 2 must keep the hash seen at two different Acmes: %+v", res.Groups)
	}
	g := res.Groups[0]
	if g.CustomersKnown != 2 || g.Devices != 3 || strings.Join(g.Customers, "|") != "Acme (aaaaaaaa)|Acme (bbbbbbbb)" {
		t.Fatalf("customers = %d %v", g.CustomersKnown, g.Customers)
	}
	// Names that differ only in case are also told apart (--customer matching
	// is case-insensitive, so they are ambiguous to an operator).
	ci := detectionsSpreadCustomerLabels(newEsetTree([]esetGroup{
		{UUID: acme1, Name: "Acme", EntityType: esetCompanyEntityType},
		{UUID: acme2, Name: "ACME", EntityType: esetCompanyEntityType},
	}))
	if got := ci(esetGroup{UUID: acme2, Name: "ACME"}); got != "ACME (bbbbbbbb)" {
		t.Fatalf("case-insensitive duplicate label = %q", got)
	}
	// A unique name keeps its plain label; buckets keep theirs.
	label := detectionsSpreadCustomerLabels(tree)
	if got := label(esetGroup{UUID: "beta", Name: "Beta Law"}); got != "Beta Law" {
		t.Fatalf("unique name label = %q", got)
	}
	if got := label(esetGroup{Name: esetCustomerNone}); got != esetCustomerNone {
		t.Fatalf("bucket label = %q", got)
	}
	// A COMPANY literally named like a bucket is told apart from the bucket.
	odd := newEsetTree([]esetGroup{{UUID: "cccccccc-3333-4000-8000-000000000003", Name: "(unknown device)", EntityType: esetCompanyEntityType}})
	if got := detectionsSpreadCustomerLabels(odd)(esetGroup{UUID: "cccccccc-3333-4000-8000-000000000003", Name: "(unknown device)"}); got != "(unknown device) (cccccccc)" {
		t.Fatalf("bucket-named company label = %q", got)
	}
	if got := detectionsSpreadCustomerLabels(nil)(esetGroup{UUID: "u", Name: "Solo"}); got != "Solo" {
		t.Fatalf("nil tree label = %q", got)
	}
}

// TestDetectionsSpreadEndToEndDuplicateNames runs the real command over a
// mirror with two distinct "Acme" customers and a malformed executable row.
func TestDetectionsSpreadEndToEndDuplicateNames(t *testing.T) {
	testenv.Isolate(t)
	ago := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	path := fleetHealthTestSeed(t, map[string][]string{
		"device-groups": {
			`{"uuid":"msp","displayName":"Servosity MSP","linkedEntityType":"DEVICE_GROUP_ENTITY_TYPE_MSP"}`,
			`{"uuid":"aaaaaaaa-1111-4000-8000-000000000001","displayName":"Acme","parentGroupUuid":"msp","linkedEntityType":"DEVICE_GROUP_ENTITY_TYPE_COMPANY"}`,
			`{"uuid":"bbbbbbbb-2222-4000-8000-000000000002","displayName":"Acme","parentGroupUuid":"msp","linkedEntityType":"DEVICE_GROUP_ENTITY_TYPE_COMPANY"}`,
		},
		"devices": {
			`{"uuid":"a1","displayName":"ACME1-PC","parentGroupUuid":"aaaaaaaa-1111-4000-8000-000000000001"}`,
			`{"uuid":"b1","displayName":"ACME2-PC","parentGroupUuid":"bbbbbbbb-2222-4000-8000-000000000002"}`,
		},
		"detections": {
			detectionsSpreadTestDetection("det1", detectionsSpreadTestHashB, "Trojan", "a.exe", "a1", "SEVERITY_LEVEL_HIGH", ago),
			detectionsSpreadTestDetection("det2", detectionsSpreadTestHashB, "Trojan", "a.exe", "b1", "SEVERITY_LEVEL_HIGH", ago),
		},
		"executables": {
			`{"uuid":"e-bad","hashSha1":"` + detectionsSpreadTestHashB + `","devicesCount":"lots"}`,
		},
	})
	cmd := RootCmd()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"detections", "spread", "--db", path, "--min-customers", "2", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("run: %v\n%s", err, errOut.String())
	}
	var v detectionsSpreadView
	fleetHealthTestDecode(t, out.Bytes(), &v)
	if len(v.Groups) != 1 || v.Groups[0].CustomersKnown != 2 || strings.Join(v.Groups[0].Customers, "|") != "Acme (aaaaaaaa)|Acme (bbbbbbbb)" {
		t.Fatalf("groups = %+v", v.Groups)
	}
	if v.RejectedRows != 1 || v.Groups[0].Executable != nil {
		t.Fatalf("the malformed executable must be rejected, not joined: rejected %d exec %+v", v.RejectedRows, v.Groups[0].Executable)
	}
	if !strings.Contains(errOut.String(), "warning: 1 row(s) could not be decoded") || !strings.Contains(errOut.String(), "executable row 0") {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestDetectionsSpreadHumanTable(t *testing.T) {
	var buf bytes.Buffer
	if err := detectionsSpreadPrintTable(&buf, detectionsSpreadView{Groups: []detectionsSpreadGroup{}, DetectionsScanned: 5}); err != nil || !strings.Contains(buf.String(), "No detections matched (scanned 5") {
		t.Fatalf("empty table = %q %v", buf.String(), err)
	}
	buf.Reset()
	v := detectionsSpreadView{GroupsMatched: 2, DetectionsInWindow: 9, Groups: []detectionsSpreadGroup{
		{Name: "Win32/Agent.XYZ", SHA1: detectionsSpreadTestHashB, CustomersKnown: 2, Devices: 3, Occurrences: 4, Executable: &detectionsSpreadExecutable{IsBlocked: true, ReputationVerdict: "malicious"}},
		{Name: "HIPS rule", CustomersKnown: 1, Devices: 1, Occurrences: 1},
	}}
	if err := detectionsSpreadPrintTable(&buf, v); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Win32/Agent.XYZ", "malicious", "true", "unknown", "2 of 2 groups shown"} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("table missing %q:\n%s", want, buf.String())
		}
	}
}
