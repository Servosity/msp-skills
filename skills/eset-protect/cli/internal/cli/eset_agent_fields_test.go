// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"eset-protect-pp-cli/internal/cliutil/testenv"
)

var agentFieldBodies = map[string]string{
	"/v1/devices":     `{"devices":[{"uuid":"00000000-0000-0000-0000-0000000000d1","displayName":"FRONTDESK-01","deviceToken":"dt-secret-value","lastSyncTime":"2026-10-18T10:00:00Z","parentGroupUuid":"00000000-0000-0000-0000-0000000000a1","isMuted":false,"enrollmentStatus":"ENROLLED","functionalityStatus":"DEVICE_FUNCTIONALITY_STATUS_OK","functionalityProblemCount":0,"operatingSystem":{"displayName":"Windows 11"}}],"nextPageToken":""}`,
	"/v2/incidents":   `{"incidents":[{"uuid":"00000000-0000-0000-0000-0000000000c1","displayName":"Ransomware on FRONTDESK-01","status":"INCIDENT_STATUS_OPEN","severity":"INCIDENT_SEVERITY_LEVEL_HIGH","createTime":"2026-10-18T09:00:00Z","updateTime":"2026-10-18T09:30:00Z","deviceUuids":["00000000-0000-0000-0000-0000000000d1"],"description":"long text"}],"nextPageToken":""}`,
	"/v1/detections":  `{"detections":[{"uuid":"00000000-0000-0000-0000-0000000000e1","category":"DETECTION_CATEGORY_ANTIMALWARE","displayName":"Win32/Agent.X","occurTime":"2026-10-18T08:00:00Z","deviceUuid":"00000000-0000-0000-0000-0000000000d1","severityLevel":"SEVERITY_LEVEL_HIGH","isResolved":false}],"nextPageToken":""}`,
	"/v1/executables": `{"executables":[{"uuid":"00000000-0000-0000-0000-0000000000f1","displayName":"evil.exe","devicesCount":2,"hashSha1":"da39a3ee5e6b4b0d3255bfef95601890afd80709","isBlocked":true}],"nextPageToken":""}`,
}

func agentFieldServer(t *testing.T) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := agentFieldBodies[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("ESET_PROTECT_BASE_URL", srv.URL)
	t.Setenv("ESET_PROTECT_TOKEN", "agent-fields-token")
	t.Setenv("ESET_PROTECT_REGION", "eu")
	t.Setenv("ESET_PROTECT_USERNAME", "")
	t.Setenv("ESET_PROTECT_PASSWORD", "")
}

// agentRows returns the row objects of a list command's --agent output,
// whether it prints a bare array or an envelope with results.
func agentRows(t *testing.T, out string) []map[string]any {
	t.Helper()
	out = strings.TrimSpace(out)
	var rows []map[string]any
	if json.Unmarshal([]byte(out), &rows) == nil {
		return rows
	}
	var env struct {
		Results json.RawMessage `json:"results"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out)
	}
	if json.Unmarshal(env.Results, &rows) == nil {
		return rows
	}
	var nested map[string]json.RawMessage
	if json.Unmarshal(env.Results, &nested) == nil {
		for _, v := range nested {
			if json.Unmarshal(v, &rows) == nil && len(rows) > 0 {
				return rows
			}
		}
	}
	t.Fatalf("no rows in %s", out)
	return nil
}

// TestAgentOutputKeepsESETIdentity (adversarial round 6, P2): --agent
// compaction kept only fields named like id / *Id / *_at, so devices list
// lost uuid, lastSyncTime and parentGroupUuid, incidents list lost uuid,
// severity and createTime, executables list lost uuid, hashSha1 and
// isBlocked; --quiet printed "[redacted]" (the redacted deviceToken) for
// every device and the category enum for detections. Each list keeps its
// identity, links, timestamps and state under --agent, and --quiet prints the
// row's uuid (handfixes.json: agent-output-keeps-eset-identity).
func TestAgentOutputKeepsESETIdentity(t *testing.T) {
	testenv.Isolate(t)
	respondClearHarness(t)
	agentFieldServer(t)
	for _, tc := range []struct {
		resource string
		uuid     string
		want     []string
	}{
		{"devices", "00000000-0000-0000-0000-0000000000d1", []string{"uuid", "displayName", "lastSyncTime", "parentGroupUuid", "isMuted"}},
		{"incidents", "00000000-0000-0000-0000-0000000000c1", []string{"uuid", "displayName", "status", "severity", "createTime", "updateTime", "deviceUuids"}},
		{"detections", "00000000-0000-0000-0000-0000000000e1", []string{"uuid", "displayName", "occurTime", "deviceUuid", "severityLevel", "isResolved"}},
		{"executables", "00000000-0000-0000-0000-0000000000f1", []string{"uuid", "displayName", "hashSha1", "isBlocked"}},
	} {
		_, out, err := respondRunRoot(t, tc.resource, "list", "--agent", "--data-source", "live")
		if err != nil {
			t.Fatalf("%s list --agent: %v\n%s", tc.resource, err, out)
		}
		rows := agentRows(t, strings.SplitN(out, "\n{\"event\"", 2)[0])
		if len(rows) != 1 {
			t.Fatalf("%s list --agent: %d rows\n%s", tc.resource, len(rows), out)
		}
		for _, key := range tc.want {
			if _, ok := rows[0][key]; !ok {
				t.Fatalf("%s list --agent dropped %q: %v", tc.resource, key, rows[0])
			}
		}
		_, quiet, err := respondRunRoot(t, tc.resource, "list", "--quiet", "--data-source", "live")
		if err != nil {
			t.Fatalf("%s list --quiet: %v", tc.resource, err)
		}
		if got := strings.TrimSpace(quiet); got != tc.uuid {
			t.Fatalf("%s list --quiet printed %q, want the row uuid %s", tc.resource, got, tc.uuid)
		}
	}
}

// TestQuietNeverPrintsAPlaceholder: a row with no uuid falls back to its
// displayName, and a redacted value is never an identity.
func TestQuietNeverPrintsAPlaceholder(t *testing.T) {
	if got := quietRowValue(map[string]any{"deviceToken": "[redacted]", "displayName": "FRONTDESK-01"}); got != "FRONTDESK-01" {
		t.Fatalf("quietRowValue = %q, want the displayName", got)
	}
	if got := quietRowValue(map[string]any{"aToken": "[redacted]", "zone": "eu"}); got != "eu" {
		t.Fatalf("quietRowValue = %q, want a real value", got)
	}
}
