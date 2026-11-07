// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Codex review round 2 of the pagerduty 4.34.2 reprint: the three wrong-result
// classes it left open. Each test is built from the reviewer's failing input,
// written for the class, and was seen failing on the round-1 code
// (.review/seen-r2-<class>-fails.log in the publish worktree).

import (
	"strings"
	"testing"
	"time"
)

// Class log-entry-detail-wrong-field: spec.json LogEntry.changeset is an array
// of strings ("String record of custom field updates"); only a string was read,
// so the documented change details were replaced by the generic summary.
func TestR2TimelineReadsChangesetAsDocumented(t *testing.T) {
	at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	inc := map[string]any{"id": "PINC001"}
	for _, c := range []struct {
		name      string
		changeset any
		want      []string
	}{
		{"an array, the reviewer's input", []any{"Updated serial_number to 123"}, []string{"Updated serial_number to 123"}},
		{"every item of an array", []any{"Updated serial_number to 123", `Updated affected_room to ["Queen West"]`}, []string{"Updated serial_number to 123", "Queen West"}},
		{"a string", "Updated serial_number to 123", []string{"Updated serial_number to 123"}},
		{"an empty array falls back to the summary", []any{}, []string{"Custom fields updated"}},
	} {
		res := buildTimeline([]map[string]any{{
			"type": "custom_field_value_change_log_entry", "created_at": at, "incident": inc,
			"summary": "Custom fields updated", "changeset": c.changeset,
		}}, "PINC001")
		if len(res.Events) != 1 {
			t.Fatalf("%s: events = %+v", c.name, res.Events)
		}
		for _, w := range c.want {
			if !strings.Contains(res.Events[0].Detail, w) {
				t.Errorf("%s: detail %q lacks %q", c.name, res.Events[0].Detail, w)
			}
		}
	}
}
