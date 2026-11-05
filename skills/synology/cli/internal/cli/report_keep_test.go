// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/synology/handfixes.json
// under the "agent-compaction-keeps-report-fields" entry.

package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/spf13/cobra"
)

// TestAgentCompactionKeepsReportFields: --agent compaction keeps a row key only
// when it is on the gravity list or on 80% of rows, and the report rows mark
// their reconciliation fields omitempty, so the one NAS that failed among five
// lost its error and read as an unexplained ok:false.
func TestAgentCompactionKeepsReportFields(t *testing.T) {
	rows := []fleetSyncRow{{NAS: "a", OK: true}, {NAS: "b", OK: true}, {NAS: "c", OK: true}, {NAS: "d", OK: true},
		{NAS: "e", OK: false, Error: "DSM login: DSM error 400: invalid account or password"}}
	flags := &rootFlags{asJSON: true, compact: true, agent: true}
	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := printFleetRows(cmd, flags, rows, "nothing"); err != nil {
		t.Fatal(err)
	}
	var env struct {
		Results []map[string]any `json:"results"`
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out.String())
	}
	if len(env.Results) != 5 || env.Results[4]["error"] == nil {
		t.Fatalf("--agent dropped the failing NAS's error: %s", out.String())
	}

	rep := triageReport{NAS: "acme", TaskID: 8, TaskName: "File Server", Timeline: []triageEntry{
		{Source: "abb_log", Time: 1, Level: "info", LogType: 1500},
		{Source: "abb_log", Time: 2, Level: "info", LogType: 1500},
		{Source: "abb_log", Time: 3, Level: "info", LogType: 1500},
		{Source: "abb_log", Time: 4, Level: "info", LogType: 1500},
		{Source: "result_detail", Time: 5, Level: "error", LogType: 6002, ErrorCode: 13, Params: map[string]any{"path": "E:/data", "fs_error": -65}},
	}}
	out.Reset()
	if err := printReportJSON(&out, rep, flags); err != nil {
		t.Fatal(err)
	}
	var tri struct {
		Results struct {
			Timeline []map[string]any `json:"timeline"`
		} `json:"results"`
	}
	if err := json.Unmarshal(out.Bytes(), &tri); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out.String())
	}
	last := tri.Results.Timeline[len(tri.Results.Timeline)-1]
	if last["error_code"] == nil || last["params"] == nil {
		t.Fatalf("--agent dropped the failing step's error_code or params: %s", out.String())
	}
}
