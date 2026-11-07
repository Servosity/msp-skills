// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Codex review round 1 of the pagerduty 4.34.2 reprint, the wrong-result
// classes of the store-backed novels. Each test was seen failing on the
// round-1 code (seen/r1-<class>-seen-failing.log in the run dir) and is
// written for the class, not the instance Codex named. The shapes are the
// documented ones: GET /oncalls entries (List all of the on-calls: user,
// schedule, escalation_policy, escalation_level, start, end; null start and
// end for a permanent on-call), escalation policies with their teams, and
// log entries with note, channel, event_details, assignees and user.

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func pdRef(typ, id, summary string) string {
	return fmt.Sprintf(`{"id":%q,"type":%q,"summary":%q}`, id, typ, summary)
}

func pdOncallJSON(epID, level, schedID, userID, start, end string) string {
	sched := "null"
	if schedID != "" {
		sched = pdRef("schedule_reference", schedID, "Schedule "+schedID)
	}
	q := func(s string) string {
		if s == "" {
			return "null"
		}
		return `"` + s + `"`
	}
	return `{"escalation_policy":` + pdRef("escalation_policy_reference", epID, "Policy "+epID) + `,"escalation_level":` + level +
		`,"schedule":` + sched + `,"user":` + pdRef("user_reference", userID, "User "+userID) + `,"start":` + q(start) + `,"end":` + q(end) + `}`
}

func pdSetBody(f *fakePagerDuty, p, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bodies[p] = body
}

// Class duty-double-counted: hours summed each on-call entry, and GET
// /oncalls returns one entry per policy and level a schedule serves, so one
// 24-hour shift under two policies counted 48 hours. A responder's hours are
// the union of their intervals.
func TestR1OncallHoursCountEachHourOnce(t *testing.T) {
	day := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	rfc3339 := func(d time.Duration) string { return day.Add(d).Format(time.RFC3339) }
	entry := func(ep, level, user string, from, to time.Duration) map[string]any {
		return map[string]any{"escalation_policy": map[string]any{"id": ep}, "escalation_level": level,
			"schedule": map[string]any{"id": "PSCH001"}, "user": map[string]any{"id": user, "summary": "User " + user},
			"start": rfc3339(from), "end": rfc3339(to)}
	}
	win := pdWindow{Since: day, Until: day.Add(72 * time.Hour)}
	for _, c := range []struct {
		name   string
		in     []map[string]any
		hours  float64
		shifts int
	}{
		{"same shift, two policies", []map[string]any{entry("PEP0001", "1", "PUSR001", 0, 24*time.Hour), entry("PEP0002", "1", "PUSR001", 0, 24*time.Hour)}, 24, 1},
		{"same shift, two levels", []map[string]any{entry("PEP0001", "1", "PUSR001", 0, 24*time.Hour), entry("PEP0001", "2", "PUSR001", 0, 24*time.Hour)}, 24, 1},
		{"overlapping shifts", []map[string]any{entry("PEP0001", "1", "PUSR001", 0, 24*time.Hour), entry("PEP0002", "1", "PUSR001", 12*time.Hour, 36*time.Hour)}, 36, 1},
		{"separate shifts", []map[string]any{entry("PEP0001", "1", "PUSR001", 0, 12*time.Hour), entry("PEP0001", "1", "PUSR001", 24*time.Hour, 36*time.Hour)}, 24, 2},
	} {
		res := buildOncallHours(c.in, win, "PUSR001")
		if len(res.Users) != 1 || res.Users[0].Hours != c.hours || res.Users[0].Shifts != c.shifts || res.TotalHours != c.hours {
			t.Errorf("%s: got %+v total %v, want %vh in %d shifts", c.name, res.Users, res.TotalHours, c.hours, c.shifts)
		}
	}
}

// Class log-entry-detail-wrong-field: the timeline detail read only
// channel.summary or channel.type, so an annotation's note (LogEntry.note),
// an assignment's assignees and a notification's user were replaced by the
// channel type. The detail carries every documented field that says what
// happened.
func TestR1TimelineDetailReadsTheDocumentedFields(t *testing.T) {
	at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	inc := map[string]any{"id": "PINC001"}
	user := func(id, name string) map[string]any {
		return map[string]any{"id": id, "type": "user_reference", "summary": name}
	}
	logs := []map[string]any{
		{"type": "annotate_log_entry", "created_at": at.Format(time.RFC3339), "incident": inc,
			"note": "Do not restart the database.", "channel": map[string]any{"type": "website"}},
		{"type": "assign_log_entry", "created_at": at.Add(time.Minute).Format(time.RFC3339), "incident": inc,
			"channel": map[string]any{"type": "website"}, "assignees": []any{user("PUSR001", "Ana Ruiz"), user("PUSR002", "Ben Okafor")}},
		{"type": "notify_log_entry", "created_at": at.Add(2 * time.Minute).Format(time.RFC3339), "incident": inc,
			"channel": map[string]any{"type": "sms"}, "user": user("PUSR003", "Cleo Park")},
		{"type": "trigger_log_entry", "created_at": at.Add(3 * time.Minute).Format(time.RFC3339), "incident": inc,
			"channel": map[string]any{"type": "api"}, "event_details": map[string]any{"description": "Disk full on db-01"}},
		{"type": "escalate_log_entry", "created_at": at.Add(4 * time.Minute).Format(time.RFC3339), "incident": inc,
			"summary": "Escalated to level 2."},
	}
	res := buildTimeline(logs, "PINC001")
	want := [][]string{
		{"Do not restart the database."},
		{"Ana Ruiz", "Ben Okafor"},
		{"Cleo Park"},
		{"Disk full on db-01"},
		{"Escalated to level 2."},
	}
	if len(res.Events) != len(want) {
		t.Fatalf("events = %+v", res.Events)
	}
	for i, ws := range want {
		for _, w := range ws {
			if !strings.Contains(res.Events[i].Detail, w) {
				t.Errorf("%s detail %q lacks %q", res.Events[i].Type, res.Events[i].Detail, w)
			}
		}
	}
}
