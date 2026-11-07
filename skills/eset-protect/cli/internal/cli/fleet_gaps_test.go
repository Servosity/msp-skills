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

	"eset-protect-pp-cli/internal/cliutil/testenv"
)

// TestNovelFleetGapsHelpWires smoke-tests that the fleet gaps command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelFleetGapsHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"fleet", "gaps", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("fleet gaps --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "gaps"} {
		if !strings.Contains(help, want) {
			t.Fatalf("fleet gaps --help missing %q in output:\n%s", want, help)
		}
	}
}

// fleetGapsTestDevice builds a device JSON row. comps is "name=version" with
// version either dotted text (sent as version.name) or "#maj.min.patch"
// (sent as numeric fields).
func fleetGapsTestDevice(uuid, name, group, enroll string, licensed bool, comps ...string) string {
	parts := make([]string, 0, len(comps))
	for _, c := range comps {
		kv := strings.SplitN(c, "=", 2)
		ver := `{}`
		if len(kv) == 2 && strings.HasPrefix(kv[1], "#") {
			n := strings.Split(strings.TrimPrefix(kv[1], "#"), ".")
			ver = fmt.Sprintf(`{"major":%s,"minor":%s,"patch":"%s"}`, n[0], n[1], n[2])
		} else if len(kv) == 2 && kv[1] != "" {
			ver = fmt.Sprintf(`{"name":%q}`, kv[1])
		}
		parts = append(parts, fmt.Sprintf(`{"name":%q,"displayName":%q,"version":%s}`, kv[0], strings.ToUpper(kv[0]), ver))
	}
	products := `[]`
	if licensed {
		products = `[{"id":7,"name":"ESET Endpoint Security"}]`
	}
	return fmt.Sprintf(`{"uuid":%q,"displayName":%q,"parentGroupUuid":%q,"enrollmentStatus":%q,"activeProducts":%s,"deployedComponents":[%s]}`,
		uuid, name, group, enroll, products, strings.Join(parts, ","))
}

const (
	fleetGapsTestDone    = "DEVICE_ENROLLMENT_STATUS_FINISHED"
	fleetGapsTestPending = "DEVICE_ENROLLMENT_STATUS_PENDING"
)

// fleetGapsTestFixture: Acme Dental has 4 devices (peer checks run), Beta
// Law has 2 (too small for "missing").
func fleetGapsTestFixture() map[string][]string {
	return map[string][]string{
		"device-groups": {
			`{"uuid":"msp","displayName":"Servosity MSP","linkedEntityType":"DEVICE_GROUP_ENTITY_TYPE_MSP"}`,
			`{"uuid":"acme","displayName":"Acme Dental","parentGroupUuid":"msp","linkedEntityType":"DEVICE_GROUP_ENTITY_TYPE_COMPANY"}`,
			`{"uuid":"beta","displayName":"Beta Law","parentGroupUuid":"msp","linkedEntityType":"DEVICE_GROUP_ENTITY_TYPE_COMPANY"}`,
		},
		"devices": {
			fleetGapsTestDevice("a1", "FRONTDESK-01", "acme", fleetGapsTestDone, true, "agent=11.1.2039.2", "ees=#11.0.1", "edr=2.0.0"),
			fleetGapsTestDevice("a2", "XRAY-02", "acme", fleetGapsTestDone, true, "agent=11.1.2039.2", "ees=#11.0.1"),
			fleetGapsTestDevice("a3", "OFFICE-03", "acme", fleetGapsTestDone, true, "agent=11.0.2032.1", "ees=#11.0.1"),
			fleetGapsTestDevice("a4", "RECEPTION-04", "acme", fleetGapsTestPending, false, "agent=11.1.2039.2"),
			fleetGapsTestDevice("b1", "PARTNER-01", "beta", fleetGapsTestDone, true, "agent=10.0.0", "edr=2.0.0"),
			fleetGapsTestDevice("b2", "PARTNER-02", "beta", fleetGapsTestDone, true, "agent=11.0.0"),
		},
	}
}

func fleetGapsTestKey(r fleetGapsRow) string {
	return r.Device + "/" + r.Gap + "/" + r.Component
}

func fleetGapsTestKeys(rows []fleetGapsRow) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, fleetGapsTestKey(r))
	}
	return out
}

func TestFleetGapsEndToEndCustomerScope(t *testing.T) {
	testenv.Isolate(t)
	path := fleetGapsTestSeedFixture(t)
	out, err := fleetHealthTestRun(t, "fleet", "gaps", "--db", path, "--json")
	if err != nil {
		t.Fatalf("fleet gaps: %v\n%s", err, out)
	}
	var v fleetGapsView
	fleetHealthTestDecode(t, out, &v)
	want := []string{
		"OFFICE-03/outdated/agent",
		"RECEPTION-04/missing/ees",
		"RECEPTION-04/unlicensed/",
		"RECEPTION-04/unenrolled/",
		"PARTNER-01/outdated/agent",
	}
	if got := fleetGapsTestKeys(v.Gaps); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("gaps =\n  %v\nwant\n  %v", got, want)
	}
	// edr is on 1 of 3 Acme peers (below 0.5); Beta is too small for missing.
	for _, g := range v.Gaps {
		if g.Component == "edr" {
			t.Fatalf("edr must not be flagged at threshold 0.5 or in a 2-device group: %+v", g)
		}
	}
	miss := v.Gaps[1]
	if miss.PeerShare == nil || *miss.PeerShare != 1 || miss.NewestVersion != "11.0.1" || miss.Customer != "Acme Dental" || miss.DeviceUUID != "a4" {
		t.Fatalf("missing row = %+v", miss)
	}
	old := v.Gaps[0]
	if old.HaveVersion != "11.0.2032.1" || old.NewestVersion != "11.1.2039.2" || old.PeerShare != nil {
		t.Fatalf("outdated row = %+v", old)
	}
	if v.Gaps[3].Detail != "enrollment PENDING" {
		t.Fatalf("unenrolled detail = %q", v.Gaps[3].Detail)
	}
	wantCounts := map[string]int{"missing": 1, "outdated": 2, "unlicensed": 1, "unenrolled": 1}
	for k, n := range wantCounts {
		if v.CountsByGap[k] != n {
			t.Fatalf("counts_by_gap = %v, want %v", v.CountsByGap, wantCounts)
		}
	}
	if v.DevicesChecked != 6 || v.SkippedSmallGroups != 1 || v.Scope != "customer" || v.PeerThreshold != 0.5 {
		t.Fatalf("view meta = checked %d skipped %d scope %s threshold %v", v.DevicesChecked, v.SkippedSmallGroups, v.Scope, v.PeerThreshold)
	}
}

func fleetGapsTestSeedFixture(t *testing.T) string {
	t.Helper()
	return fleetHealthTestSeed(t, fleetGapsTestFixture())
}

func TestFleetGapsEndToEndFleetAndFilters(t *testing.T) {
	testenv.Isolate(t)
	path := fleetGapsTestSeedFixture(t)

	// --fleet: Beta's devices join one 6-device peer group.
	var v fleetGapsView
	out, err := fleetHealthTestRun(t, "fleet", "gaps", "--db", path, "--json", "--fleet", "--only", "missing,outdated")
	if err != nil {
		t.Fatalf("--fleet: %v", err)
	}
	fleetHealthTestDecode(t, out, &v)
	got := strings.Join(fleetGapsTestKeys(v.Gaps), ",")
	for _, want := range []string{"PARTNER-02/missing/ees", "PARTNER-01/missing/ees", "PARTNER-01/outdated/agent", "PARTNER-02/outdated/agent", "OFFICE-03/outdated/agent"} {
		if !strings.Contains(got, want) {
			t.Fatalf("--fleet gaps missing %s:\n%s", want, got)
		}
	}
	if strings.Contains(got, "unlicensed") || strings.Contains(got, "unenrolled") {
		t.Fatalf("--only missing,outdated leaked other kinds: %s", got)
	}
	if _, ok := v.CountsByGap["unlicensed"]; ok || v.Scope != "fleet" || v.SkippedSmallGroups != 0 {
		t.Fatalf("--fleet view = counts %v scope %s skipped %d", v.CountsByGap, v.Scope, v.SkippedSmallGroups)
	}

	// --customer: only Acme devices are reported, Beta's small group is not counted.
	v = fleetGapsView{}
	out, err = fleetHealthTestRun(t, "fleet", "gaps", "--db", path, "--json", "--customer", "Acme Dental")
	if err != nil {
		t.Fatalf("--customer: %v", err)
	}
	fleetHealthTestDecode(t, out, &v)
	if v.DevicesChecked != 4 || v.SkippedSmallGroups != 0 {
		t.Fatalf("--customer meta: checked %d skipped %d", v.DevicesChecked, v.SkippedSmallGroups)
	}
	for _, g := range v.Gaps {
		if g.Customer != "Acme Dental" {
			t.Fatalf("--customer leaked %+v", g)
		}
	}

	// --peer-threshold 0.3: edr on 1 of 3 peers (0.333) now counts.
	v = fleetGapsView{}
	out, err = fleetHealthTestRun(t, "fleet", "gaps", "--db", path, "--json", "--only", "missing", "--peer-threshold", "0.3")
	if err != nil {
		t.Fatalf("--peer-threshold: %v", err)
	}
	fleetHealthTestDecode(t, out, &v)
	if v.CountsByGap["missing"] != 4 { // ees on RECEPTION-04 + edr on XRAY-02, OFFICE-03, RECEPTION-04
		t.Fatalf("threshold 0.3 missing = %d: %v", v.CountsByGap["missing"], fleetGapsTestKeys(v.Gaps))
	}

	// --limit truncates rows but not counts.
	v = fleetGapsView{}
	out, err = fleetHealthTestRun(t, "fleet", "gaps", "--db", path, "--json", "--limit", "2")
	if err != nil {
		t.Fatalf("--limit: %v", err)
	}
	fleetHealthTestDecode(t, out, &v)
	if len(v.Gaps) != 2 || v.CountsByGap["outdated"] != 2 {
		t.Fatalf("--limit 2: rows %d counts %v", len(v.Gaps), v.CountsByGap)
	}

	for _, bad := range [][]string{{"--only", "stale"}, {"--peer-threshold", "0"}, {"--peer-threshold", "1.5"}, {"--customer", "Nobody Inc"}} {
		args := append([]string{"fleet", "gaps", "--db", path, "--json"}, bad...)
		if _, err := fleetHealthTestRun(t, args...); err == nil {
			t.Fatalf("%v must be rejected", bad)
		}
	}
}

func TestFleetGapsEndToEndEmpty(t *testing.T) {
	testenv.Isolate(t)
	path := fleetHealthTestSeed(t, map[string][]string{})
	out, err := fleetHealthTestRun(t, "fleet", "gaps", "--db", path, "--json")
	if err != nil {
		t.Fatalf("empty mirror: %v", err)
	}
	if !strings.Contains(string(out), `"gaps": []`) {
		t.Fatalf("gaps must render as []:\n%s", out)
	}
	out, err = fleetHealthTestRun(t, "fleet", "gaps", "--db", filepath.Join(t.TempDir(), "absent.db"), "--json")
	if err != nil {
		t.Fatalf("missing mirror must exit 0: %v", err)
	}
	var v fleetGapsView
	fleetHealthTestDecode(t, out, &v)
	if v.Gaps == nil || len(v.Gaps) != 0 || v.DevicesChecked != 0 {
		t.Fatalf("missing mirror view = %+v", v)
	}
}

func TestFleetGapsThresholdEdgeAndSmallGroups(t *testing.T) {
	tree := newEsetTree([]esetGroup{
		{UUID: "c", Name: "Acme Dental", EntityType: esetCompanyEntityType},
		{UUID: "s", Name: "Solo Co", EntityType: esetCompanyEntityType},
	})
	comp := func(name string) esetComponent { return esetComponent{Name: name, Version: esetVersion{Name: "1.0.0"}} }
	lic := []json.RawMessage{json.RawMessage(`{}`)}
	devices := []esetDevice{
		{UUID: "1", DisplayName: "HAS-X", ParentGroupUUID: "c", Components: []esetComponent{comp("x")}, ActiveProducts: lic},
		{UUID: "2", DisplayName: "NO-X-A", ParentGroupUUID: "c", ActiveProducts: lic},
		{UUID: "3", DisplayName: "NO-X-B", ParentGroupUUID: "c", ActiveProducts: lic},
		{UUID: "4", DisplayName: "SOLO", ParentGroupUUID: "s", ActiveProducts: lic},
	}
	kinds, _ := fleetGapsParseOnly("missing")
	// Each device lacking x has 2 peers, 1 of which has x: share exactly 0.5.
	at := fleetGapsFind(devices, tree, fleetGapsOptions{Kinds: kinds, Threshold: 0.5})
	if at.Counts["missing"] != 2 || *at.Gaps[0].PeerShare != 0.5 {
		t.Fatalf("share == threshold must flag: %+v", at.Gaps)
	}
	if at.SkippedSmallGroups != 1 || at.DevicesChecked != 4 {
		t.Fatalf("solo customer must count as a skipped small group: %+v", at)
	}
	above := fleetGapsFind(devices, tree, fleetGapsOptions{Kinds: kinds, Threshold: 0.51})
	if above.Counts["missing"] != 0 || len(above.Gaps) != 0 {
		t.Fatalf("share below threshold must not flag: %+v", above.Gaps)
	}
	if got := fleetGapsFind(nil, tree, fleetGapsOptions{Kinds: kinds, Threshold: 0.5}); got.Gaps == nil || len(got.Gaps) != 0 || got.DevicesChecked != 0 {
		t.Fatalf("no devices = %+v, want empty non-nil gaps", got)
	}
}

func TestFleetGapsUnknownVersionsAndDuplicates(t *testing.T) {
	tree := newEsetTree(nil) // single tenant: one peer group
	kinds, _ := fleetGapsParseOnly("outdated")
	devices := []esetDevice{
		{UUID: "1", DisplayName: "A", Components: []esetComponent{{Name: "agent", Version: esetVersion{Name: "11.1.0"}}}},
		{UUID: "2", DisplayName: "B", Components: []esetComponent{{Name: "agent"}}}, // version unknown
		// Duplicate entries: the newest one counts, so B-DUP is current.
		{UUID: "3", DisplayName: "B-DUP", Components: []esetComponent{{Name: "agent", Version: esetVersion{Name: "9.0.0"}}, {Name: "agent", Version: esetVersion{Name: "11.1.0"}}}},
		{UUID: "4", DisplayName: "C", Components: []esetComponent{{DisplayName: "Legacy Module", Version: esetVersion{Major: 1}}, {Name: "agent", Version: esetVersion{Major: 11, Minor: 0, Patch: 9}}}},
	}
	res := fleetGapsFind(devices, tree, fleetGapsOptions{Kinds: kinds, Threshold: 0.5})
	if got := strings.Join(fleetGapsTestKeys(res.Gaps), ","); got != "C/outdated/agent" {
		t.Fatalf("outdated = %s (unknown version must not flag; duplicate keeps newest)", got)
	}
	if res.Gaps[0].HaveVersion != "11.0.9" || res.Gaps[0].NewestVersion != "11.1.0" {
		t.Fatalf("versions = %+v", res.Gaps[0])
	}
}

func TestFleetGapsParseOnly(t *testing.T) {
	all, err := fleetGapsParseOnly("")
	if err != nil || len(all) != 4 {
		t.Fatalf("empty --only = %v %v, want all 4 kinds", all, err)
	}
	two, err := fleetGapsParseOnly(" Missing, OUTDATED ,")
	if err != nil || len(two) != 2 || !two["missing"] || !two["outdated"] {
		t.Fatalf("mixed-case csv = %v %v", two, err)
	}
	if _, err := fleetGapsParseOnly("missing,stale"); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("unknown kind must be refused, got %v", err)
	}
}

func TestFleetGapsVersionOrdering(t *testing.T) {
	cases := []struct {
		name string
		v    esetVersion
		want []int64
	}{
		{"name only", esetVersion{Name: "11.1.2039.2"}, []int64{11, 1, 2039, 2}},
		{"numeric only", esetVersion{Major: 11, Minor: 0, Patch: 1}, []int64{11, 0, 1}},
		{"numeric extended by matching name", esetVersion{Name: "11.1.2039.2", Major: 11, Minor: 1, Patch: 2039}, []int64{11, 1, 2039, 2}},
		{"numeric with prose name", esetVersion{Name: "ESET Endpoint 11", Major: 11, Minor: 2}, []int64{11, 2, 0}},
		{"v prefix", esetVersion{Name: "v10.4"}, []int64{10, 4}},
		{"unknown", esetVersion{}, nil},
		{"prose only", esetVersion{Name: "latest"}, nil},
	}
	for _, c := range cases {
		if got := fleetGapsVersionKey(c.v); fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Errorf("%s: key = %v, want %v", c.name, got, c.want)
		}
	}
	if fleetGapsVersionCmp([]int64{11, 1}, []int64{11, 1, 0}) != 0 {
		t.Error("trailing zero segments must compare equal")
	}
	if fleetGapsVersionCmp([]int64{11, 0, 2032, 1}, []int64{11, 1, 2039, 2}) != -1 || fleetGapsVersionCmp([]int64{12}, []int64{11, 9, 9}) != 1 {
		t.Error("segment-wise ordering is wrong")
	}
}

func TestFleetGapsHumanTable(t *testing.T) {
	var buf bytes.Buffer
	empty := fleetGapsView{Gaps: []fleetGapsRow{}, DevicesChecked: 2, SkippedSmallGroups: 1, Scope: "customer"}
	if err := fleetGapsPrintTable(&buf, empty); err != nil || !strings.Contains(buf.String(), "No gaps found across 2 devices") || !strings.Contains(buf.String(), "fewer than 3") {
		t.Fatalf("empty table = %q %v", buf.String(), err)
	}
	buf.Reset()
	v := fleetGapsView{Gaps: []fleetGapsRow{{Customer: "Acme Dental", Device: "OFFICE-03", Gap: "outdated", Component: "agent", HaveVersion: "11.0.1", NewestVersion: "11.1.0"}},
		CountsByGap: map[string]int{"outdated": 1}, DevicesChecked: 4, Scope: "customer"}
	if err := fleetGapsPrintTable(&buf, v); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"OFFICE-03", "outdated=1", "4 devices checked"} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("table missing %q:\n%s", want, buf.String())
		}
	}
}
