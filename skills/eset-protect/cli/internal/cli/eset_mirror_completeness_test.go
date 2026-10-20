// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored. Every local-mirror reader must say when a resource it read
// has no completed sync. The case that matters is a re-sync that died part
// way: SaveSyncProgress keeps the last completed timestamp, so a
// timestamp-only check reads the partial rows as a finished sync and the view
// understates its totals with no warning.
package cli

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"eset-protect-pp-cli/internal/cliutil/testenv"
)

const mirrorIncompleteMarker = "no completed sync for "

// mirrorInterrupt records a completed sync for every resource and then an
// interrupted re-sync of the given ones (the timestamp survives).
func mirrorInterrupt(t *testing.T, path string, all []string, interrupted ...string) {
	t.Helper()
	mirrorHintMarkSync(t, path, true, all...)
	if len(interrupted) > 0 {
		mirrorHintMarkSync(t, path, false, interrupted...)
	}
}

func mirrorSyncedHint(t *testing.T, out string) string {
	t.Helper()
	var v struct {
		SyncedHint string `json:"synced_hint"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	return v.SyncedHint
}

// mirrorReaderCase runs one reader against a mirror whose sync state the test
// sets, returning stdout and stderr.
type mirrorReaderCase struct {
	name        string
	resources   []string // every resource the fixture seeds and syncs
	interrupt   string   // the resource whose re-sync dies
	seed        func(t *testing.T) string
	run         func(t *testing.T, db string, extra ...string) (string, string)
	wantStderr  bool // the reader also prints the "hint:" line MCP forwards
	humanMarker string
}

func mirrorReaderCases() []mirrorReaderCase {
	now := time.Now()
	fleetSeed := func(t *testing.T) string {
		return exposureSeedDB(t, map[string][]map[string]any{
			"device-groups": exposureFixtureGroups(),
			"devices":       exposureFixtureDevices(now),
		})
	}
	cli := func(args ...string) func(t *testing.T, db string, extra ...string) (string, string) {
		return func(t *testing.T, db string, extra ...string) (string, string) {
			t.Helper()
			full := append(append(append([]string{}, args...), "--db", db), extra...)
			out, errOut, err := exposureRunCLI(t, full...)
			if err != nil {
				t.Fatalf("%v: %v\n%s\n%s", full, err, out, errOut)
			}
			return out, errOut
		}
	}
	served := func(handler func(t *testing.T) func(http.ResponseWriter, *http.Request), args ...string) func(t *testing.T, db string, extra ...string) (string, string) {
		return func(t *testing.T, db string, extra ...string) (string, string) {
			t.Helper()
			srv := reportVulnsNewServer(t, handler(t))
			full := append(append(append([]string{}, args...), "--db", db), extra...)
			out, errOut, err := reportVulnsRunCLI(t, srv, full...)
			if err != nil {
				t.Fatalf("%v: %v\n%s\n%s", full, err, out, errOut)
			}
			return out, errOut
		}
	}
	groupsSeed := func(t *testing.T) string { return reportVulnsSeedDB(t, reportVulnsTestGroups(), nil) }
	queueSeed := func(t *testing.T) string {
		return fleetStaleTestSeedDB(t, map[string][]map[string]any{
			"device-groups": fleetStaleTestGroups(),
			"devices":       fleetStaleTestDevices(),
			"detections":    triageTestDetections(),
			"incidents":     triageTestIncidents(),
		})
	}
	queueResources := []string{"device-groups", "devices", "detections", "incidents"}
	return []mirrorReaderCase{
		{
			name: "triage", resources: queueResources, interrupt: "incidents",
			seed: queueSeed, run: cli("triage"), wantStderr: true, humanMarker: mirrorIncompleteMarker + "incidents",
		},
		{
			name: "detections spread", resources: queueResources, interrupt: "detections",
			seed: queueSeed, run: cli("detections", "spread"), wantStderr: true, humanMarker: mirrorIncompleteMarker + "detections",
		},
		{
			name: "fleet gaps", resources: []string{"device-groups", "devices"}, interrupt: "devices",
			seed: fleetSeed, run: cli("fleet", "gaps"), wantStderr: true, humanMarker: mirrorIncompleteMarker + "devices",
		},
		{
			name: "fleet health", resources: []string{"device-groups", "devices"}, interrupt: "devices",
			seed: fleetSeed, run: cli("fleet", "health"), wantStderr: true, humanMarker: mirrorIncompleteMarker + "devices",
		},
		{
			name: "fleet stale", resources: []string{"device-groups", "devices"}, interrupt: "devices",
			seed: fleetSeed, run: cli("fleet", "stale"), wantStderr: true, humanMarker: mirrorIncompleteMarker + "devices",
		},
		{
			name: "exposure", resources: []string{"device-groups", "devices"}, interrupt: "devices",
			seed: fleetSeed, wantStderr: true, humanMarker: mirrorIncompleteMarker + "devices",
			run: func(t *testing.T, db string, extra ...string) (string, string) {
				exposureStub(t, &exposureNewLive, &exposureFakeLive{lists: map[string]func(map[string]string) (esetPageResult, error){
					"/v1/device-vulnerabilities": exposurePage(t, false,
						exposureVulnJSON("dev-front", "acme-hq", "Google Chrome", "CVE-2024-3400", 80, "SEVERITY_LEVEL_HIGH", "2026-09-10T08:00:00Z", "p1")),
				}})
				return cli("exposure", "--cve", "CVE-2024-3400")(t, db, extra...)
			},
		},
		{
			name: "patches backlog", resources: []string{"device-groups", "devices", "device-patches"}, interrupt: "device-patches",
			wantStderr: true, humanMarker: mirrorIncompleteMarker + "device-patches",
			seed: func(t *testing.T) string {
				return exposureSeedDB(t, map[string][]map[string]any{
					"device-groups":  exposureFixtureGroups(),
					"devices":        exposureFixtureDevices(now),
					"device-patches": patchesBacklogFixturePatches(),
				})
			},
			run: func(t *testing.T, db string, extra ...string) (string, string) {
				exposureStub(t, &patchesBacklogNewLive, &exposureFakeLive{lists: map[string]func(map[string]string) (esetPageResult, error){
					"/v1/device-vulnerabilities":   exposureScopedPage(t, patchesBacklogFixtureVulns()...),
					"/v1/patching-process-details": exposurePage(t, false),
				}})
				return cli("patches", "backlog")(t, db, extra...)
			},
		},
		{
			name: "executables find (local)", resources: []string{"executables"}, interrupt: "executables",
			seed: func(t *testing.T) string {
				return fleetStaleTestSeedDB(t, map[string][]map[string]any{"executables": executablesFindTestRows()})
			},
			run: cli("executables", "find", "--hash", executablesFindTestSHA1), wantStderr: true, humanMarker: mirrorIncompleteMarker + "executables",
		},
		{
			name: "devices profile", resources: []string{"device-groups", "devices"}, interrupt: "device-groups",
			seed: func(t *testing.T) string {
				mirrored := `{"uuid":"` + reportVulnsTestDev1 + `","displayName":"FRONTDESK-01","parentGroupUuid":"` + reportVulnsTestHQ + `"}`
				return reportVulnsSeedDB(t, reportVulnsTestGroups(), []string{mirrored})
			},
			run: served(devicesProfileTestHandler, "devices", "profile", "FRONTDESK-01"), wantStderr: true, humanMarker: mirrorIncompleteMarker + "device-groups",
		},
		{
			name: "incidents context", resources: []string{"device-groups"}, interrupt: "device-groups", seed: groupsSeed,
			run: served(func(*testing.T) func(http.ResponseWriter, *http.Request) { return incidentsContextTestHandler },
				"incidents", "context", incidentsContextTestUUID),
			wantStderr: true, humanMarker: mirrorIncompleteMarker + "device-groups",
		},
		{
			name: "report vulnerabilities", resources: []string{"device-groups"}, interrupt: "device-groups", seed: groupsSeed,
			run: served(func(*testing.T) func(http.ResponseWriter, *http.Request) { return reportVulnsTestVulnHandler },
				"report", "vulnerabilities"),
			wantStderr: true, humanMarker: mirrorIncompleteMarker + "device-groups",
		},
		{
			name: "detections latest", resources: []string{"device-groups", "devices"}, interrupt: "devices",
			seed: fleetSeed, humanMarker: mirrorIncompleteMarker + "devices",
			run: served(func(*testing.T) func(http.ResponseWriter, *http.Request) {
				return func(w http.ResponseWriter, r *http.Request) {
					_, _ = w.Write([]byte(`{"detections":[{"uuid":"e1000001-0000-4000-8000-000000000001","displayName":"Win32/Filecoder.WannaCryptor.D",
					  "severityLevel":"SEVERITY_LEVEL_HIGH","occurTime":"` + fleetStaleTestAgo(time.Hour) + `","context":{"deviceUuid":"dev-front"}}],"nextPageToken":""}`))
				}
			}, "detections", "latest", "--since", "7d"),
		},
	}
}

// TestMirrorReadersFlagAnInterruptedSync guards mirror-views-synced-hint for
// every mirror reader: with every resource synced the view carries no
// completeness hint; after a re-sync of one resource dies (its old timestamp
// survives) the JSON synced_hint, the stderr "hint:" line and the human output
// all name that resource.
func TestMirrorReadersFlagAnInterruptedSync(t *testing.T) {
	for _, tc := range mirrorReaderCases() {
		t.Run(tc.name, func(t *testing.T) {
			testenv.Isolate(t)
			db := tc.seed(t)
			mirrorInterrupt(t, db, tc.resources)
			out, errOut := tc.run(t, db, "--json")
			if h := mirrorSyncedHint(t, out); strings.Contains(h, mirrorIncompleteMarker) {
				t.Fatalf("a completed sync must not carry a completeness hint, got %q", h)
			}
			if strings.Contains(errOut, mirrorIncompleteMarker) {
				t.Fatalf("a completed sync must not print a completeness hint, stderr %q", errOut)
			}

			mirrorInterrupt(t, db, tc.resources, tc.interrupt)
			out, errOut = tc.run(t, db, "--json")
			want := mirrorIncompleteMarker + tc.interrupt
			if h := mirrorSyncedHint(t, out); !strings.Contains(h, want) {
				t.Fatalf("an interrupted %s sync must set synced_hint naming it, got %q\n%s", tc.interrupt, h, out)
			}
			if tc.wantStderr && !strings.Contains(errOut, "hint: "+want) {
				t.Fatalf("stderr must carry the \"hint:\" line MCP forwards, got %q", errOut)
			}
			human, _ := tc.run(t, db, "--human-friendly")
			if !strings.Contains(human, tc.humanMarker) {
				t.Fatalf("human output must show the completeness hint %q, got:\n%s", tc.humanMarker, human)
			}
		})
	}
}

// TestReportCustomerFlagsIncompleteSections: report customer carries the
// completeness verdict per section, so a QBR built from a half-synced
// detections table says which section is partial and leaves the rest clean.
func TestReportCustomerFlagsIncompleteSections(t *testing.T) {
	testenv.Isolate(t)
	now := time.Now()
	db := reportCustomerSeed(t, now)
	exposureStub(t, &reportCustomerNewLive, reportCustomerFixtureLive(t, now))
	all := []string{"device-groups", "devices", "detections", "incidents", "device-patches"}

	mirrorInterrupt(t, db, all)
	view, _ := reportCustomerRun(t, "Acme Dental", "--db", db, "--json")
	if view.SyncedHint != "" || view.Health.Incomplete || view.Detections.Incomplete || view.Incidents.Incomplete || view.Patches.Incomplete {
		t.Fatalf("a completed sync must not flag the report: hint=%q health=%+v", view.SyncedHint, view.Health.reportCustomerSectionSync)
	}

	mirrorInterrupt(t, db, all, "detections")
	view, errOut := reportCustomerRun(t, "Acme Dental", "--db", db, "--json")
	if !strings.Contains(view.SyncedHint, mirrorIncompleteMarker+"detections") {
		t.Fatalf("report synced_hint = %q", view.SyncedHint)
	}
	if !view.Detections.Incomplete || !strings.Contains(view.Detections.SyncedHint, "detections") {
		t.Fatalf("detections section must be incomplete: %+v", view.Detections.reportCustomerSectionSync)
	}
	if view.Health.Incomplete || view.Incidents.Incomplete || view.Vulnerabilities.Incomplete || view.Patches.Incomplete || view.ScanCoverage.Incomplete {
		t.Fatalf("only the detections section is partial: health=%v incidents=%v vulns=%v patches=%v scans=%v",
			view.Health.Incomplete, view.Incidents.Incomplete, view.Vulnerabilities.Incomplete, view.Patches.Incomplete, view.ScanCoverage.Incomplete)
	}
	if !strings.Contains(errOut, "hint: "+mirrorIncompleteMarker+"detections") {
		t.Fatalf("stderr hint = %q", errOut)
	}
	human, _, err := exposureRunCLI(t, "report", "customer", "Acme Dental", "--db", db, "--human-friendly")
	if err != nil {
		t.Fatal(err)
	}
	det := human[strings.Index(human, "Detections:"):]
	det = det[:strings.Index(det, "Incidents:")]
	if !strings.Contains(det, "note: incomplete: "+mirrorIncompleteMarker+"detections") {
		t.Fatalf("the Detections section must say it is incomplete:\n%s", human)
	}

	// An interrupted devices sync touches every section that places rows in
	// the customer's subtree.
	mirrorInterrupt(t, db, all, "devices")
	view, _ = reportCustomerRun(t, "Acme Dental", "--db", db, "--json")
	if !view.Health.Incomplete || !view.Detections.Incomplete || !view.ScanCoverage.Incomplete || !view.Vulnerabilities.Incomplete {
		t.Fatalf("devices feeds subtree membership; sections = %+v %+v %+v %+v", view.Health.reportCustomerSectionSync,
			view.Detections.reportCustomerSectionSync, view.ScanCoverage.reportCustomerSectionSync, view.Vulnerabilities.reportCustomerSectionSync)
	}
	raw, _, err := exposureRunCLI(t, "report", "customer", "Acme Dental", "--db", db, "--json")
	if err != nil || strings.Count(raw, `"incomplete": true`) != len(reportCustomerSections) {
		t.Fatalf("every section carries incomplete (%v):\n%s", err, raw)
	}
	mirrorInterrupt(t, db, all)
	raw, _, err = exposureRunCLI(t, "report", "customer", "Acme Dental", "--db", db, "--json")
	if err != nil || strings.Count(raw, `"incomplete": false`) != len(reportCustomerSections) {
		t.Fatalf("incomplete is present (false) on every section of a complete report (%v):\n%s", err, raw)
	}
}

// TestGenericLocalReadsHintAfterAnInterruptedSync: the generated local-read
// path (a list command with --data-source local) used the timestamp-only
// sync check, so rows left by a dead re-sync came back with no hint.
func TestGenericLocalReadsHintAfterAnInterruptedSync(t *testing.T) {
	testenv.Isolate(t)
	db := defaultDBPath(esetCLIName)
	seeded := exposureSeedDB(t, map[string][]map[string]any{"devices": exposureFixtureDevices(time.Now())})
	if err := copyFileForTest(seeded, db); err != nil {
		t.Fatal(err)
	}
	mirrorInterrupt(t, db, []string{"devices"})
	_, errOut, err := exposureRunCLI(t, "devices", "list", "--data-source", "local", "--json")
	if err != nil {
		t.Fatalf("devices list: %v\n%s", err, errOut)
	}
	if strings.Contains(errOut, mirrorIncompleteMarker) {
		t.Fatalf("a completed sync must not print a completeness hint: %q", errOut)
	}
	mirrorInterrupt(t, db, []string{"devices"}, "devices")
	_, errOut, err = exposureRunCLI(t, "devices", "list", "--data-source", "local", "--json")
	if err != nil {
		t.Fatalf("devices list: %v\n%s", err, errOut)
	}
	if !strings.Contains(errOut, "hint: "+mirrorIncompleteMarker+"devices") {
		t.Fatalf("an interrupted sync must print the completeness hint, stderr %q", errOut)
	}
}

func copyFileForTest(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o600)
}
