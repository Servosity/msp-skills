// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored. An --agent envelope's meta.source tells an agent whether it
// read fresh ESET data or the synced mirror. The commands annotated
// pp:data-source auto got "live" from the annotation even when their rows
// came from the mirror, so one envelope said meta.source "live" next to
// results.source "local".
package cli

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"eset-protect-pp-cli/internal/cliutil/testenv"
)

func agentMetaSource(t *testing.T, out string) (meta string, results map[string]any) {
	t.Helper()
	var env struct {
		Meta    map[string]any `json:"meta"`
		Results map[string]any `json:"results"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("not an agent envelope: %v\n%s", err, out)
	}
	s, _ := env.Meta["source"].(string)
	return s, env.Results
}

func TestAgentMetaSourceMatchesResults(t *testing.T) {
	t.Run("executables find from the mirror", func(t *testing.T) {
		testenv.Isolate(t)
		path := fleetStaleTestSeedDB(t, map[string][]map[string]any{"executables": executablesFindTestRows()})
		mirrorInterrupt(t, path, []string{"executables"})
		out, err := fleetStaleTestRun(t, "executables", "find", "--hash", executablesFindTestSHA1, "--db", path, "--agent")
		if err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		meta, res := agentMetaSource(t, out)
		if res["source"] != "local" || meta != "local" {
			t.Fatalf("meta.source = %q, results.source = %v; both must be local", meta, res["source"])
		}
	})
	t.Run("executables find live", func(t *testing.T) {
		srv := reportVulnsNewServer(t, func(w http.ResponseWriter, r *http.Request) {
			b, _ := json.Marshal(map[string]any{"executables": executablesFindTestRows(), "nextPageToken": ""})
			_, _ = w.Write(b)
		})
		out, stderr, err := reportVulnsRunCLI(t, srv, "executables", "find", "--hash", executablesFindTestSHA1, "--live", "--agent")
		if err != nil {
			t.Fatalf("%v\n%s", err, stderr)
		}
		meta, res := agentMetaSource(t, out)
		if res["source"] != "live" || meta != "live" {
			t.Fatalf("meta.source = %q, results.source = %v; both must be live", meta, res["source"])
		}
	})
	t.Run("patches backlog from the mirror", func(t *testing.T) {
		testenv.Isolate(t)
		db := exposureSeedDB(t, map[string][]map[string]any{
			"device-groups":  exposureFixtureGroups(),
			"devices":        exposureFixtureDevices(time.Now()),
			"device-patches": patchesBacklogFixturePatches(),
		})
		exposureStub(t, &patchesBacklogNewLive, &exposureFakeLive{lists: map[string]func(map[string]string) (esetPageResult, error){
			"/v1/device-vulnerabilities":   exposureScopedPage(t, patchesBacklogFixtureVulns()...),
			"/v1/patching-process-details": exposurePage(t, false),
		}})
		out, errOut, err := exposureRunCLI(t, "patches", "backlog", "--db", db, "--agent")
		if err != nil {
			t.Fatalf("%v\n%s", err, errOut)
		}
		meta, res := agentMetaSource(t, out)
		if res["patch_source"] != "local" || meta != "local" {
			t.Fatalf("meta.source = %q, results.patch_source = %v; a mirror-built backlog is local", meta, res["patch_source"])
		}
	})
	t.Run("report customer", func(t *testing.T) {
		testenv.Isolate(t)
		now := time.Now()
		db := reportCustomerSeed(t, now)
		exposureStub(t, &reportCustomerNewLive, reportCustomerFixtureLive(t, now))
		out, errOut, err := exposureRunCLI(t, "report", "customer", "Acme Dental", "--db", db, "--agent")
		if err != nil {
			t.Fatalf("%v\n%s", err, errOut)
		}
		if meta, _ := agentMetaSource(t, out); meta != "local" {
			t.Fatalf("meta.source = %q; health, detections and incidents come from the mirror", meta)
		}
	})
}
