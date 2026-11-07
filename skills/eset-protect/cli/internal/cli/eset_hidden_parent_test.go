// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"eset-protect-pp-cli/internal/cliutil/testenv"
)

// twoHiddenParentsMirror is the adversarial round-5 shape: an API user whose
// permissions cover the Acme COMPANY (under a hidden MSP group) and Lost &
// found (under the hidden All root). The visible top-level groups name two
// different missing parents, so neither is the scope root and Lost & found's
// devices stay owner-unknown, conservatively.
func twoHiddenParentsMirror(t *testing.T) string {
	t.Helper()
	ago := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	return fleetHealthTestSeed(t, map[string][]string{
		"device-groups": {
			`{"uuid":"acme","displayName":"Acme","parentGroupUuid":"hidden-msp","linkedEntityType":"DEVICE_GROUP_ENTITY_TYPE_COMPANY"}`,
			`{"uuid":"acme-hq","displayName":"Acme HQ","parentGroupUuid":"acme","linkedEntityType":"DEVICE_GROUP_ENTITY_TYPE_SITE"}`,
			`{"uuid":"lost","displayName":"Lost & found","parentGroupUuid":"hidden-all"}`,
		},
		"devices": {
			`{"uuid":"d1","displayName":"ACME-01","parentGroupUuid":"acme-hq","lastSyncTime":"` + ago + `"}`,
			`{"uuid":"d8","displayName":"LF-08","parentGroupUuid":"lost","lastSyncTime":"` + ago + `"}`,
		},
		"detections": {
			detectionsSpreadTestDetection("det-8", "cccccccccccccccccccccccccccccccccccccccc", "Trojan", "a.exe", "d8", "SEVERITY_LEVEL_HIGH", ago),
		},
	})
}

// TestHiddenParentCauseIsHonest (adversarial round 5, P2): with visible
// top-level groups under two different hidden parents, a device in Lost &
// found was owner-unknown forever, and every view blamed a deleted group or
// one added after the last sync, filed it under "(group not in synced
// tree)" beside a group path the mirror holds, and promised that a resync
// attributes it. The ownership rule stays conservative (owner-unknown), but
// the row is "(parent group not visible)", the cause says the parent is not
// visible to this API user, and the remedy says a sync does not fix it.
func TestHiddenParentCauseIsHonest(t *testing.T) {
	testenv.Isolate(t)
	tree := newEsetTree([]esetGroup{
		{UUID: "acme", Name: "Acme", ParentUUID: "hidden-msp", EntityType: esetCompanyEntityType},
		{UUID: "acme-hq", Name: "Acme HQ", ParentUUID: "acme"},
		{UUID: "lost", Name: "Lost & found", ParentUUID: "hidden-all"},
	})
	if got := tree.parentNotVisible(); !reflect.DeepEqual(got, []string{"Lost & found"}) {
		t.Fatalf("parentNotVisible = %v, want only Lost & found (Acme still owns its rows)", got)
	}
	if g, k := tree.ownerOf("lost"); k || g.Name != esetCustomerParentHidden {
		t.Fatalf("ownerOf(lost) = %q %v, want %q owner-unknown", g.Name, k, esetCustomerParentHidden)
	}
	if g, k := tree.ownerOf("acme-hq"); !k || g.Name != "Acme" {
		t.Fatalf("ownerOf(acme-hq) = %q %v", g.Name, k)
	}
	if g, k := tree.ownerOf("created-after-sync"); k || g.Name != esetCustomerUnknownGroup {
		t.Fatalf("a group the mirror lacks must stay %q: %q %v", esetCustomerUnknownGroup, g.Name, k)
	}

	two := esetUnattributedLine(2, "synced device(s)", []string{"a", "b"}, "", "Run sync.", []string{"Lost & found", "Spare room"})
	if !strings.Contains(two, `the parent group of "Lost & found", "Spare room" is not visible`) || !strings.Contains(two, "rows under them") || !strings.Contains(two, "access to their parent group") {
		t.Fatalf("two hidden parents: %s", two)
	}
	if plain := esetUnattributedLine(1, "synced device(s)", []string{"a"}, "", "Run sync.", nil); strings.Contains(plain, "not visible") || !strings.HasSuffix(plain, "Run sync.") {
		t.Fatalf("no hidden parent: %s", plain)
	}

	path := twoHiddenParentsMirror(t)
	savedHF := humanFriendly
	defer func() { humanFriendly = savedHF }()
	cause := `the parent group of "Lost & found" is not visible to this API user (outside its permission scope)`
	remedy := `A sync does not attribute rows under "Lost & found": give the API user access to its parent group in ESET PROTECT`
	for _, args := range [][]string{
		{"fleet", "health"},
		{"fleet", "health", "--customer", "Acme"},
		{"fleet", "gaps", "--customer", "Acme"},
		{"detections", "spread"},
		{"triage", "--since", "24h"},
		{"triage", "--since", "24h", "--customer", "Acme"},
	} {
		out, err := fleetHealthTestRun(t, append(args, "--db", path, "--human-friendly")...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		text := string(out)
		if !strings.Contains(text, cause) || !strings.Contains(text, remedy) {
			t.Fatalf("%v: the unattributed text does not say the parent is not visible and that a sync does not fix it:\n%s", args, text)
		}
		if strings.Contains(text, esetCustomerUnknownGroup) {
			t.Fatalf("%v: files a device of a synced group under %q:\n%s", args, esetCustomerUnknownGroup, text)
		}
		// Adversarial round 6: agents read --agent JSON, which must carry
		// the cause too.
		agent, err := fleetHealthTestRun(t, append(args, "--db", path, "--agent")...)
		if err != nil {
			t.Fatalf("%v --agent: %v\n%s", args, err, agent)
		}
		assertParentNotVisibleJSON(t, args, string(agent))
	}
	out, err := fleetHealthTestRun(t, "fleet", "health", "--db", path, "--json")
	if err != nil || !strings.Contains(string(out), `"customer": "`+esetCustomerParentHidden+`"`) || !strings.Contains(string(out), `"unattributed_devices": 1`) {
		t.Fatalf("fleet health JSON: %v\n%s", err, out)
	}

	// Control: a device in a group created after the sync, under the usual
	// single hidden scope root, keeps the resync advice and no permission
	// clause.
	control := fleetHealthTestSeed(t, map[string][]string{
		"device-groups": {`{"uuid":"acme","displayName":"Acme","parentGroupUuid":"hidden-msp","linkedEntityType":"DEVICE_GROUP_ENTITY_TYPE_COMPANY"}`},
		"devices":       {`{"uuid":"d2","displayName":"NEW-02","parentGroupUuid":"made-after-sync"}`},
	})
	out, err = fleetHealthTestRun(t, "fleet", "health", "--db", control, "--human-friendly")
	if err != nil || strings.Contains(string(out), "not visible to this API user") || !strings.Contains(string(out), "sync --resources device-groups' to attribute them") {
		t.Fatalf("control: %v\n%s", err, out)
	}
}

// TestHiddenParentCauseInLiveViews runs the views that read ESET live and
// name customers through the mirror (exposure, patches backlog, report
// vulnerabilities, report customer, detections latest, incidents context)
// plus fleet stale, against the two-hidden-parents mirror: every one says the
// parent is not visible and that a sync does not fix it.
func TestHiddenParentCauseInLiveViews(t *testing.T) {
	const (
		dev8   = "d8000000-0000-4000-8000-000000000008"
		devA   = "d1000000-0000-4000-8000-000000000001"
		incID  = "1c000000-0000-4000-8000-0000000000c1"
		detID  = "de000000-0000-4000-8000-0000000000d8"
		cve    = "CVE-2024-1111"
		patchA = "pa000000-0000-4000-8000-0000000000a1"
	)
	now := time.Now()
	old := now.Add(-40 * 24 * time.Hour).UTC().Format(time.RFC3339)
	recent := now.Add(-2 * time.Hour).UTC().Format(time.RFC3339)
	vuln := exposureVulnJSON(dev8, "lost", "Zoom", cve, 70, "SEVERITY_LEVEL_HIGH", recent, patchA)
	db := exposureSeedDB(t, map[string][]map[string]any{
		"device-groups": {
			{"uuid": "acme", "displayName": "Acme", "parentGroupUuid": "hidden-msp", "linkedEntityType": esetCompanyEntityType},
			{"uuid": "acme-hq", "displayName": "Acme HQ", "parentGroupUuid": "acme"},
			{"uuid": "lost", "displayName": "Lost & found", "parentGroupUuid": "hidden-all"},
		},
		"devices": {
			{"uuid": devA, "displayName": "ACME-01", "parentGroupUuid": "acme-hq", "lastSyncTime": recent},
			{"uuid": dev8, "displayName": "LF-08", "parentGroupUuid": "lost", "lastSyncTime": old},
		},
		"detections": {{"uuid": detID, "displayName": "Win32/Agent", "typeName": "Trojan", "occurTime": recent,
			"severityLevel": "SEVERITY_LEVEL_HIGH", "context": map[string]any{"deviceUuid": dev8}}},
		"incidents": {{"uuid": incID, "displayName": "Ransomware behavior", "deviceUuids": []string{dev8}, "status": "INCIDENT_STATUS_OPEN",
			"severity": "INCIDENT_SEVERITY_LEVEL_HIGH", "createTime": recent, "updateTime": recent}},
	})
	vulnBody, _ := json.Marshal(map[string]any{"vulnerabilities": []any{vuln}, "nextPageToken": ""})
	srv := reportVulnsNewServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/device-vulnerabilities":
			_, _ = w.Write(vulnBody)
		case "/v1/device-patches":
			_, _ = w.Write([]byte(`{"devices":[],"nextPageToken":""}`))
		case "/v1/patching-process-details":
			_, _ = w.Write([]byte(`{"patchingDetails":[],"nextPageToken":""}`))
		case "/v1/scans/recent":
			_, _ = w.Write([]byte(`{"scanDetails":[],"nextPageToken":""}`))
		case "/v1/detections":
			_, _ = w.Write([]byte(`{"detections":[{"uuid":"` + detID + `","displayName":"Win32/Agent","severityLevel":"SEVERITY_LEVEL_HIGH","occurTime":"` + recent + `","context":{"deviceUuid":"` + dev8 + `"}}],"nextPageToken":""}`))
		case "/v2/incidents/" + incID:
			_, _ = w.Write([]byte(`{"incident":{"uuid":"` + incID + `","displayName":"Ransomware behavior","status":"INCIDENT_STATUS_OPEN","severity":"INCIDENT_SEVERITY_LEVEL_HIGH","createTime":"` + recent + `","updateTime":"` + recent + `","detectionUuids":[],"deviceUuids":["` + dev8 + `"]}}`))
		case "/v2/incidents/" + incID + "/comments":
			_, _ = w.Write([]byte(`{"comments":[]}`))
		case "/v1/devices/" + dev8:
			_, _ = w.Write([]byte(`{"device":{"uuid":"` + dev8 + `","displayName":"LF-08","parentGroupUuid":"lost","lastSyncTime":"` + old + `"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"message":"not found"}`))
		}
	})
	savedHF := humanFriendly
	defer func() { humanFriendly = savedHF }()
	cause := `the parent group of "Lost & found" is not visible to this API user (outside its permission scope)`
	remedy := `A sync does not attribute rows under "Lost & found": give the API user access to its parent group in ESET PROTECT`
	for _, args := range [][]string{
		{"exposure", "--cve", cve},
		{"patches", "backlog"},
		{"report", "vulnerabilities"},
		{"report", "customer", "Acme"},
		{"detections", "latest", "--since", "3650d"},
		{"incidents", "context", incID},
		{"fleet", "stale", "--customer", "Acme", "--days", "14"},
	} {
		out, stderr, err := reportVulnsRunCLI(t, srv, append(args, "--db", db, "--human-friendly")...)
		if err != nil {
			t.Fatalf("%v: %v\n%s\n%s", args, err, out, stderr)
		}
		text := out + stderr
		if testing.Verbose() {
			t.Logf("%v:\n%s", args, text)
		}
		if !strings.Contains(text, cause) || !strings.Contains(text, remedy) {
			t.Fatalf("%v: the unattributed text does not say the parent is not visible and that a sync does not fix it:\nstdout %s\nstderr %s", args, out, stderr)
		}
		if strings.Contains(text, esetCustomerUnknownGroup) {
			t.Fatalf("%v: files a device of a synced group under %q:\n%s", args, esetCustomerUnknownGroup, text)
		}
		agent, _, err := reportVulnsRunCLI(t, srv, append(args, "--db", db, "--agent")...)
		if err != nil {
			t.Fatalf("%v --agent: %v\n%s", args, err, agent)
		}
		assertParentNotVisibleJSON(t, args, agent)
		// report customer's stderr warning carries the cause on its own.
		if args[0] == "report" && args[1] == "customer" {
			warn := ""
			for _, l := range strings.Split(stderr, "\n") {
				if strings.HasPrefix(l, "warning: ") && strings.Contains(l, "could not be tied to any customer") {
					warn = l
				}
			}
			if !strings.Contains(warn, cause) {
				t.Fatalf("report customer stderr warning does not name the hidden parent: %q", warn)
			}
		}
	}
}

// assertParentNotVisibleJSON (adversarial round 6, P2): the hidden-parent
// cause reached only the human lines, so under --agent an agent saw
// unattributed_* counts, followed the resync advice and got the same count
// back forever. Every view's JSON names the groups whose parent this API user
// cannot see in parent_not_visible (handfixes.json:
// unattributed-cause-names-hidden-parent).
func assertParentNotVisibleJSON(t *testing.T, args []string, out string) {
	t.Helper()
	start := strings.Index(out, "{")
	if start < 0 {
		t.Fatalf("%v --agent printed no JSON:\n%s", args, out)
	}
	dec := json.NewDecoder(strings.NewReader(out[start:]))
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("%v --agent: %v\n%s", args, err, out)
	}
	var found []any
	var walk func(any)
	walk = func(x any) {
		switch t := x.(type) {
		case map[string]any:
			for k, y := range t {
				if k == "parent_not_visible" {
					if list, ok := y.([]any); ok {
						found = append(found, list...)
					}
				}
				walk(y)
			}
		case []any:
			for _, y := range t {
				walk(y)
			}
		}
	}
	walk(v)
	for _, g := range found {
		if m, ok := g.(map[string]any); ok && m["uuid"] == "lost" && m["name"] == "Lost & found" {
			return
		}
	}
	t.Fatalf("%v --agent: no parent_not_visible entry for Lost & found (got %v):\n%s", args, found, out)
}
