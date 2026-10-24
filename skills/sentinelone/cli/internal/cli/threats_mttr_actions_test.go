// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

// mitAction builds one entry of SentinelOne's top-level mitigationStatus list.
// A zero endedAt omits mitigationEndedAt; an empty status omits status.
func mitAction(action, status string, endedAt time.Time, extra map[string]any) map[string]any {
	m := map[string]any{"action": action}
	if status != "" {
		m["status"] = status
	}
	if !endedAt.IsZero() {
		m["mitigationEndedAt"] = endedAt.Format(time.RFC3339)
	}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

// mitigatedThreat is a mitigated, resolved threat detected at det carrying the
// given mitigation action records.
func mitigatedThreat(id string, det time.Time, info map[string]any, actions ...map[string]any) map[string]any {
	ti := map[string]any{
		"mitigationStatus": "mitigated", "incidentStatus": "resolved",
		"createdAt": det.Format(time.RFC3339), "updatedAt": det.Add(60 * time.Hour).Format(time.RFC3339),
	}
	for k, v := range info {
		ti[k] = v
	}
	t := threatFix(id, "X-"+id, "h-"+id, "WS", "S", ti)
	if len(actions) > 0 {
		list := make([]any, 0, len(actions))
		for _, a := range actions {
			list = append(list, a)
		}
		t["mitigationStatus"] = list
	}
	return t
}

// TestMTTRIgnoresUndoAndUnknownActions: mitigation completes at the EARLIEST
// end time of a successful action that actually mitigates the threat. An undo
// (un-quarantine), an unknown action, a failed/pending/errored/unended action,
// and a containment-only step never set it, and nothing after completion moves
// it. With no qualifying action the threat is mitigated_unmeasured.
func TestMTTRIgnoresUndoAndUnknownActions(t *testing.T) {
	det := time.Now().UTC().Add(-96 * time.Hour).Truncate(time.Second)
	h := func(n float64) time.Time { return det.Add(time.Duration(n * float64(time.Hour))) }

	cases := []struct {
		name           string
		threat         map[string]any
		wantHours      float64
		wantUnmeasured float64
		wantBreaches   float64
	}{
		{
			name: "un-quarantine after quarantine does not move completion",
			threat: mitigatedThreat("a", det, nil,
				mitAction("quarantine", "success", h(1), nil),
				mitAction("un-quarantine", "success", h(48), nil)),
			wantHours: 1,
		},
		{
			name: "un-quarantine alone is not a mitigation",
			threat: mitigatedThreat("b", det, nil,
				mitAction("un-quarantine", "success", h(1), nil)),
			wantUnmeasured: 1,
		},
		{
			name: "failed kill then successful quarantine",
			threat: mitigatedThreat("c", det, nil,
				mitAction("kill", "failed", h(2), nil),
				mitAction("quarantine", "success", h(5), nil)),
			wantHours: 5,
		},
		{
			name: "unknown action then remediate",
			threat: mitigatedThreat("d", det, nil,
				mitAction("mystery-action", "success", h(1), nil),
				mitAction("remediate", "success", h(3), nil)),
			wantHours: 3,
		},
		{
			name: "a later mitigating action does not move completion",
			threat: mitigatedThreat("e", det, nil,
				mitAction("quarantine", "success", h(1), nil),
				mitAction("remediate", "success", h(48), nil)),
			wantHours: 1,
		},
		{
			name: "pending, errored, pending-reboot and unended actions do not complete",
			threat: mitigatedThreat("f", det, nil,
				mitAction("quarantine", "pending", h(1), nil),
				mitAction("quarantine", "success", h(2), map[string]any{"error": "access denied"}),
				mitAction("remediate", "success", time.Time{}, nil),
				mitAction("remediate", "success", h(3), map[string]any{"actionsCounters": map[string]any{"pendingReboot": float64(1), "success": float64(0), "total": float64(1)}}),
				mitAction("remediate", "success", h(4), map[string]any{"actionsCounters": map[string]any{"failed": float64(1), "total": float64(1)}}),
				mitAction("rollback-remediation", "success", h(6), nil),
				// Late copies: an old reader that trusted these would pick them.
				mitAction("quarantine", "success", h(40), map[string]any{"errors": []any{map[string]any{"code": float64(4000010)}}}),
				mitAction("remediate", "success", h(41), map[string]any{"actionsCounters": map[string]any{"pendingReboot": float64(1), "total": float64(1)}})),
			wantHours: 6,
		},
		{
			name: "an action with no status is not proven successful",
			threat: mitigatedThreat("g", det, nil,
				mitAction("quarantine", "", h(1), nil),
				mitAction("remediate", "success", h(4), nil),
				mitAction("quarantine", "", h(40), nil)),
			wantHours: 4,
		},
		{
			name: "kill and network-quarantine contain but do not complete mitigation",
			threat: mitigatedThreat("i", det, nil,
				mitAction("kill", "success", h(1), nil),
				mitAction("network-quarantine", "success", h(2), nil),
				mitAction("quarantine", "success", h(7), nil)),
			wantHours: 7,
		},
		{
			name: "containment alone is unmeasured",
			threat: mitigatedThreat("j", det, nil,
				mitAction("kill", "success", h(1), nil)),
			wantUnmeasured: 1,
		},
		{
			name: "a completion before detection is not replaced by a later action",
			threat: mitigatedThreat("k", det, nil,
				mitAction("quarantine", "success", det.Add(-10*time.Minute), nil),
				mitAction("remediate", "success", h(48), nil)),
			wantUnmeasured: 1,
		},
		{
			name: "action records with none qualifying never fall back to mitigatedAt",
			threat: mitigatedThreat("l", det, map[string]any{"mitigatedAt": h(30).Format(time.RFC3339)},
				mitAction("un-quarantine", "success", h(1), nil)),
			wantUnmeasured: 1,
		},
		{
			name: "action names are matched case- and separator-insensitively",
			threat: mitigatedThreat("m", det, nil,
				mitAction("UN_QUARANTINE", "SUCCESS", h(1), nil),
				mitAction("Rollback_Remediation", "Success", h(2), nil)),
			wantHours: 2,
		},
		{
			name: "a completion at detection is measured as zero",
			threat: mitigatedThreat("n", det, nil,
				mitAction("quarantine", "success", det, nil)),
			wantHours: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := runJSON(t, newNovelThreatsMttrCmd(jsonFlags()), "--db", newSeededStore(t, nil, []map[string]any{tc.threat}), "--sla", "24")
			site := out["sites"].([]any)[0].(map[string]any)
			if got := out["overall_mttr_hours"].(float64); got != tc.wantHours {
				t.Errorf("overall_mttr_hours = %v, want %v", got, tc.wantHours)
			}
			if got := site["mitigated_unmeasured"].(float64); got != tc.wantUnmeasured {
				t.Errorf("mitigated_unmeasured = %v, want %v", got, tc.wantUnmeasured)
			}
			if got := site["mitigated"].(float64); got != 1 {
				t.Errorf("mitigated = %v, want 1", got)
			}
			if got := site["sla_breaches"].(float64); got != tc.wantBreaches {
				t.Errorf("sla_breaches = %v, want %v (24h SLA)", got, tc.wantBreaches)
			}
		})
	}
}

// TestRecurrenceUsesMitigationCompletionNotUndo: recurrence reads the same
// completion time, so a detection that returned after a quarantine is a
// recurrence even when the file was un-quarantined later.
func TestRecurrenceUsesMitigationCompletionNotUndo(t *testing.T) {
	ep := func(t map[string]any) map[string]any {
		t["agentRealtimeInfo"] = map[string]any{"agentComputerName": "WS-1", "agentId": "1", "siteName": "S", "siteId": "9"}
		return t
	}
	oct := func(d, h int) time.Time { return time.Date(2026, 10, d, h, 0, 0, 0, time.UTC) }
	first := mitigatedThreat("a", oct(1, 0), map[string]any{"threatName": "X", "sha1": "hh"},
		mitAction("quarantine", "success", oct(1, 1), nil),
		mitAction("un-quarantine", "success", oct(3, 0), nil))
	again := threatFix("b", "X", "hh", "WS-1", "S", map[string]any{"createdAt": oct(2, 0).Format(time.RFC3339)})
	out := runJSON(t, newNovelThreatsRecurrenceCmd(jsonFlags()), "--db", newSeededStore(t, nil, []map[string]any{ep(first), ep(again)}))
	if g := out["threats"].([]any)[0].(map[string]any); !g["recurred_after_mitigation"].(bool) {
		t.Fatalf("recurred_after_mitigation = false for a redetection after quarantine (the later un-quarantine is not mitigation): %v", g)
	}
}
