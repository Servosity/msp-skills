// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored. Customer-tree resolution and device name matching.
package cli

import (
	"context"
	"encoding/json"
	"eset-protect-pp-cli/internal/client"
	"eset-protect-pp-cli/internal/config"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func esetTestTree() *esetTree {
	return newEsetTree([]esetGroup{
		{UUID: "msp", Name: "Servosity MSP", EntityType: "DEVICE_GROUP_ENTITY_TYPE_MSP"},
		{UUID: "acme", Name: "Acme Dental", ParentUUID: "msp", EntityType: esetCompanyEntityType},
		{UUID: "acme-hq", Name: "HQ", ParentUUID: "acme", EntityType: "DEVICE_GROUP_ENTITY_TYPE_SITE"},
		{UUID: "acme-lab", Name: "Lab", ParentUUID: "acme-hq"},
		{UUID: "acmeplus", Name: "Acme Plumbing", ParentUUID: "msp", EntityType: esetCompanyEntityType},
		{UUID: "lost", Name: "Lost and found", ParentUUID: "msp"},
		{UUID: "loop-a", Name: "A", ParentUUID: "loop-b"},
		{UUID: "loop-b", Name: "B", ParentUUID: "loop-a"},
	})
}

func TestEsetTreeCustomerOf(t *testing.T) {
	tr := esetTestTree()
	cases := map[string]string{
		"acme-lab": "Acme Dental", // two levels below the company
		"acme":     "Acme Dental",
		"acmeplus": "Acme Plumbing",
		"lost":     esetCustomerNone,         // under the MSP root, no company: known
		"loop-a":   esetCustomerUnknownGroup, // a cycle terminates as owner-unknown
		"missing":  esetCustomerUnknownGroup, // not in the tree: owner-unknown
	}
	for group, want := range cases {
		if got := tr.customerOf(group).Name; got != want {
			t.Errorf("customerOf(%s) = %q, want %q", group, got, want)
		}
		if _, known := tr.ownerOf(group); known != (want != esetCustomerUnknownGroup) {
			t.Errorf("ownerOf(%s) known = %v for %q", group, known, want)
		}
	}
	if got := tr.path("acme-lab"); got != "Servosity MSP / Acme Dental / HQ / Lab" {
		t.Errorf("path = %q", got)
	}
	single := newEsetTree([]esetGroup{{UUID: "all", Name: "All"}})
	if got := single.customerOf("all").Name; got != esetCustomerTenant {
		t.Errorf("single-tenant tree should roll up to %q, got %q", esetCustomerTenant, got)
	}
}

func TestEsetTreeResolveCustomer(t *testing.T) {
	tr := esetTestTree()
	if g, err := tr.resolveCustomer("acme dental"); err != nil || g.UUID != "acme" {
		t.Fatalf("exact name: %v %v", g, err)
	}
	if g, err := tr.resolveCustomer("plumb"); err != nil || g.UUID != "acmeplus" {
		t.Fatalf("unique substring: %v %v", g, err)
	}
	if _, err := tr.resolveCustomer("Acme"); err == nil || !strings.Contains(err.Error(), "matches 2 customers") {
		t.Fatalf("ambiguous substring should be refused, got %v", err)
	}
	if _, err := tr.resolveCustomer("lost"); err == nil {
		t.Fatal("a non-company group must not resolve as a customer")
	}
	if g, err := tr.resolveCustomer("acmeplus"); err != nil || g.Name != "Acme Plumbing" {
		t.Fatalf("uuid: %v %v", g, err)
	}
}

func TestResolveEsetDeviceAndParsing(t *testing.T) {
	var d esetDevice
	raw := `{"uuid":"d1","displayName":"FRONTDESK-01","systemHostname":"frontdesk-01.acme.local","functionalityProblemCount":"3",
	"deployedComponents":[{"name":"agent","version":{"major":11,"minor":"2","patch":3}}],"lastSyncTime":"2026-10-01T08:00:00Z"}`
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		t.Fatal(err)
	}
	if d.ProblemCount != 3 || d.Components[0].Version.String() != "11.2.3" {
		t.Fatalf("flexible ints: %+v", d)
	}
	if _, ok := d.lastSeen(); !ok {
		t.Fatal("lastSyncTime should parse")
	}
	devs := []esetDevice{d, {UUID: "d2", DisplayName: "SHARED"}, {UUID: "d3", DisplayName: "shared"}}
	if got, err := resolveEsetDevice(devs, "frontdesk-01"); err != nil || got.UUID != "d1" {
		t.Fatalf("case-insensitive name: %v %v", got, err)
	}
	if got, err := resolveEsetDevice(devs, "frontdesk-01.acme.local"); err != nil || got.UUID != "d1" {
		t.Fatalf("hostname: %v %v", got, err)
	}
	if _, err := resolveEsetDevice(devs, "shared"); err == nil {
		t.Fatal("two devices with the same name must be refused")
	}
	if _, err := resolveEsetDevice(devs, "nope"); err == nil {
		t.Fatal("unknown device must error")
	}
}

func TestFlexIntStrict(t *testing.T) {
	ok := map[string]int64{`7`: 7, `"42"`: 42, `3.0`: 3, `1e3`: 1000, `null`: 0, `""`: 0, `-5`: -5,
		`9007199254740993.0`: 9007199254740993, `9223372036854775807`: 9223372036854775807}
	for in, want := range ok {
		var f flexInt
		if err := json.Unmarshal([]byte(in), &f); err != nil || int64(f) != want {
			t.Errorf("%s -> %d, %v; want %d", in, f, err, want)
		}
	}
	for _, bad := range []string{`"oops"`, `2.5`, `"1.5"`, `1e400`, `"NaN"`, `true`,
		`1.0000000000000001`, `9223372036854775808`, `"9223372036854775808.0"`} {
		var f flexInt
		if err := json.Unmarshal([]byte(bad), &f); err == nil {
			t.Errorf("%s should be rejected, got %d", bad, f)
		}
	}
	var r esetRejects
	r.add("vulnerability row 3", fmt.Errorf("boom"))
	r.add("vulnerability row 9", fmt.Errorf("again"))
	if r.N != 2 || !strings.Contains(r.First, "row 3") {
		t.Fatalf("rejects = %+v", r)
	}
}

// TestEsetRowDecodersRejectNonObjectRows: json.Unmarshal accepts null into a
// struct, so a null row used to become a zero-valued detection, incident,
// comment, scan or task run with rejected_rows 0. Every row decoder must
// reject null and scalar rows through esetIsObject.
func TestEsetRowDecodersRejectNonObjectRows(t *testing.T) {
	for raw, want := range map[string]bool{`{"a":1}`: true, ` {}`: true, `null`: false, `7`: false, `"x"`: false, `[{}]`: false, ``: false, `  `: false} {
		if esetIsObject(json.RawMessage(raw)) != want {
			t.Errorf("esetIsObject(%q) != %v", raw, want)
		}
	}
	bad := []json.RawMessage{json.RawMessage(`null`), json.RawMessage(`7`), json.RawMessage(`"x"`)}
	check := func(name string, kept int, rej esetRejects, firstPrefix string) {
		t.Helper()
		if kept != 0 || rej.N != len(bad) || !strings.HasPrefix(rej.First, firstPrefix) || !strings.HasSuffix(rej.First, "not a JSON object") {
			t.Errorf("%s: kept %d, rejects %+v", name, kept, rej)
		}
	}
	tree := newEsetTree(nil)

	var rej esetRejects
	check("detections latest", len(detectionsLatestParse(bad, "v1", &rej)), rej, "v1 detection row 0")
	rej = esetRejects{}
	tv := triageBuild(bad, bad, nil, tree, triageOptions{}, &rej)
	if len(tv.Items) != 0 || rej.N != 2*len(bad) || rej.First != "detection row 0: not a JSON object" {
		t.Errorf("triage: items %d, rejects %+v", len(tv.Items), rej)
	}
	rej = esetRejects{}
	check("detections spread executables", len(detectionsSpreadIndexExecutables(bad, &rej)), rej, "executable row 0")
	rej = esetRejects{}
	sr := detectionsSpreadBuild(bad, nil, tree, nil, detectionsSpreadOptions{}, &rej)
	check("detections spread detections", len(sr.Groups)+sr.InWindow, rej, "detection row 0")
	rej = esetRejects{}
	check("executables find", len(executablesFindSearch(bad, executablesFindQuery{Name: "x"}, &rej)), rej, "executable row 0")
	rej = esetRejects{}
	check("report customer detections", len(reportCustomerDecode[reportCustomerDetection](bad, "detection", &rej)), rej, "detection row 0")
	rej = esetRejects{}
	check("report customer incidents", len(reportCustomerDecode[reportCustomerIncident](bad, "incident", &rej)), rej, "incident row 0")
	rej = esetRejects{}
	check("report customer scans", len(reportCustomerDecode[reportCustomerScan](bad, "scan", &rej)), rej, "scan row 0")
	rej = esetRejects{}
	comments, err := incidentsContextParseComments(json.RawMessage(`{"comments":[null,7,"x"]}`), &rej)
	if err != nil {
		t.Fatal(err)
	}
	check("incident comments", len(comments), rej, "comment row 0")
	if runs, rejected := respondDecodeRuns(bad); len(runs) != 0 || rejected != len(bad) {
		t.Errorf("respond task runs: %+v rejected %d", runs, rejected)
	}

	// Single-record responses: a null payload or a null envelope is an error.
	for _, raw := range []string{`null`, `7`} {
		if _, err := incidentsContextParseComments(json.RawMessage(raw), &esetRejects{}); err == nil {
			t.Errorf("comments payload %s must be an error", raw)
		}
	}
	for _, raw := range []string{`null`, `{"incident":null}`, `"x"`} {
		if _, err := incidentsContextParseIncident(json.RawMessage(raw)); err == nil {
			t.Errorf("incident %s must be an error", raw)
		}
	}
	for _, raw := range []string{`null`, `{"detection":null}`, `7`} {
		if d, err := incidentsContextParseDetection(json.RawMessage(raw), "v1", "det-1"); err == nil {
			t.Errorf("detection %s must be an error, got %+v", raw, d)
		}
	}
	fake := &exposureFakeLive{gets: map[string]func() (json.RawMessage, error){
		"/v2/incidents/inc-null":     func() (json.RawMessage, error) { return json.RawMessage(`{"incident":null}`), nil },
		"/v2/incidents/inc-scalar":   func() (json.RawMessage, error) { return json.RawMessage(`7`), nil },
		"/v2/incidents/inc-envelope": func() (json.RawMessage, error) { return json.RawMessage(`null`), nil },
	}}
	incs := []reportCustomerIncident{{UUID: "inc-null"}, {UUID: "inc-scalar"}, {UUID: "inc-envelope"}}
	hydrated, errs := reportCustomerHydrate(context.Background(), fake.api(), incs, []string{"inc-null", "inc-scalar", "inc-envelope"})
	if hydrated != 0 || len(errs) != 3 {
		t.Errorf("hydration of null incidents: hydrated %d, errors %+v", hydrated, errs)
	}
}

func TestEsetFetchAllRejectsBadEnvelopes(t *testing.T) {
	for name, body := range map[string]string{
		"null envelope":  `null`,
		"numeric token":  `{"items":[{"uuid":"a"}],"nextPageToken":7}`,
		"array envelope": `[{"uuid":"a"}]`,
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, body)
			}))
			defer srv.Close()
			t.Setenv("ESET_PROTECT_TOKEN", "test-token")
			cfg, err := config.Load(t.TempDir() + "/c.toml")
			if err != nil {
				t.Fatal(err)
			}
			c := client.New(cfg, 5*time.Second, 0)
			if _, err := esetFetchAll(context.Background(), c, srv.URL+"/v1/items", nil, "items", 3); err == nil {
				t.Fatalf("%s must be an error, not an empty or complete result", name)
			}
		})
	}
}
