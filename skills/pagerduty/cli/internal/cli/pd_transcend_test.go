package cli

import (
	"testing"
	"time"
)

// Synthetic PagerDuty fixtures stand in for a synced store (this CLI is built
// credential-less, so the transcendence logic is verified behaviorally here
// rather than against a live tenant). The shapes mirror the real API: nested
// "reference" objects, RFC3339 timestamps, array fields as []any.

func ref(id, summary string) map[string]any {
	return map[string]any{"id": id, "summary": summary, "type": "reference"}
}

var fixedNow = time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

func rfc(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func TestBuildPulse(t *testing.T) {
	incidents := []map[string]any{
		{"id": "I1", "status": "triggered", "urgency": "high", "created_at": rfc(fixedNow.Add(-2 * time.Hour)), "service": ref("SVCA", "web")},
		{"id": "I2", "status": "acknowledged", "urgency": "low", "created_at": rfc(fixedNow.Add(-1 * time.Hour)), "service": ref("SVCA", "web")},
		{"id": "I3", "status": "resolved", "urgency": "high", "created_at": rfc(fixedNow.Add(-3 * time.Hour)), "service": ref("SVCB", "db")},
		{"id": "I4", "status": "triggered", "urgency": "high", "created_at": rfc(fixedNow.Add(-30 * time.Minute)), "service": ref("SVCB", "db")},
	}
	res := buildPulse(incidents, fixedNow)
	if res.TotalOpen != 3 || res.Triggered != 2 || res.Acknowledged != 1 {
		t.Fatalf("totals: open=%d trig=%d ack=%d, want 3/2/1", res.TotalOpen, res.Triggered, res.Acknowledged)
	}
	if len(res.Services) != 2 {
		t.Fatalf("services=%d, want 2", len(res.Services))
	}
	// SVCA's oldest unacked (120m) > SVCB's (30m), so SVCA sorts first.
	if res.Services[0].ServiceID != "SVCA" || res.Services[0].OldestUnackedMin != 120 {
		t.Fatalf("first service = %+v, want SVCA oldest=120", res.Services[0])
	}
	if res.Services[0].Open != 2 || res.Services[0].Triggered != 1 || res.Services[0].Acknowledged != 1 {
		t.Fatalf("SVCA counts = %+v", res.Services[0])
	}
	if res.Services[1].ServiceID != "SVCB" || res.Services[1].OldestUnackedMin != 30 {
		t.Fatalf("second service = %+v, want SVCB oldest=30", res.Services[1])
	}
}

func TestBuildPulseEmpty(t *testing.T) {
	res := buildPulse(nil, fixedNow)
	if res.TotalOpen != 0 || res.Services == nil || len(res.Services) != 0 {
		t.Fatalf("empty pulse should have 0 open and non-nil empty services, got %+v", res)
	}
}

func TestBuildOncallWho(t *testing.T) {
	oncalls := []map[string]any{
		{"escalation_policy": ref("EP1", "Primary"), "escalation_level": float64(1), "schedule": ref("S1", "Day"), "user": ref("U1", "Alice")}, // current, indefinite
		{"escalation_policy": ref("EP1", "Primary"), "escalation_level": float64(2), "user": ref("U3", "Carol")},                               // current lvl2
		{"escalation_policy": ref("EP1", "Primary"), "escalation_level": float64(1), "schedule": ref("S1", "Day"), "user": ref("U2", "Bob"), "start": rfc(fixedNow.Add(24 * time.Hour)), "end": rfc(fixedNow.Add(48 * time.Hour))},
		{"escalation_policy": ref("EP2", "Other"), "escalation_level": float64(1), "user": ref("U9", "Zed")},
	}
	var res pdOncallWhoResult
	res.Current = []pdOncallEntry{}
	res.Next = []pdOncallEntry{}
	buildOncallWho(oncalls, "EP1", "", fixedNow, &res)
	if len(res.Current) != 2 {
		t.Fatalf("current=%d, want 2 (Alice lvl1, Carol lvl2)", len(res.Current))
	}
	if res.Current[0].EscalationLevel != 1 || res.Current[0].User != "Alice" {
		t.Fatalf("current[0]=%+v, want Alice lvl1", res.Current[0])
	}
	if len(res.Next) != 1 || res.Next[0].User != "Bob" {
		t.Fatalf("next=%+v, want [Bob]", res.Next)
	}
}

func TestBuildOncallHours(t *testing.T) {
	win := pdWindow{Since: fixedNow.AddDate(0, 0, -7), Until: fixedNow}
	oncalls := []map[string]any{
		{"user": ref("U1", "Alice"), "start": rfc(fixedNow.Add(-72 * time.Hour)), "end": rfc(fixedNow.Add(-48 * time.Hour))}, // 24h
		{"user": ref("U1", "Alice"), "start": rfc(fixedNow.Add(-24 * time.Hour)), "end": rfc(fixedNow)},                      // 24h
		{"user": ref("U2", "Bob"), "start": rfc(fixedNow.Add(-48 * time.Hour)), "end": rfc(fixedNow.Add(-24 * time.Hour))},   // 24h
	}
	res := buildOncallHours(oncalls, win, "")
	if len(res.Users) != 2 {
		t.Fatalf("users=%d, want 2", len(res.Users))
	}
	if res.Users[0].User != "Alice" || res.Users[0].Hours != 48 {
		t.Fatalf("top user=%+v, want Alice 48h", res.Users[0])
	}
	if res.Users[1].User != "Bob" || res.Users[1].Hours != 24 {
		t.Fatalf("second user=%+v, want Bob 24h", res.Users[1])
	}
}

func TestBuildCoverageAudit(t *testing.T) {
	services := []map[string]any{
		{"id": "SVC_NOEP", "name": "no-ep"},
		{"id": "SVC_EMPTY", "name": "empty-policy", "escalation_policy": ref("EP_EMPTY", "ep-empty")},
		{"id": "SVC_TIER", "name": "empty-tier", "escalation_policy": ref("EP_TIER", "ep-tier")},
		{"id": "SVC_SPOF", "name": "spof", "escalation_policy": ref("EP_SPOF", "ep-spof")},
		{"id": "SVC_SCHED", "name": "empty-sched", "escalation_policy": ref("EP_SCHED", "ep-sched")},
		{"id": "SVC_OK", "name": "healthy", "escalation_policy": ref("EP_OK", "ep-ok")},
	}
	eps := []map[string]any{
		{"id": "EP_EMPTY", "name": "ep-empty", "escalation_rules": []any{}},
		{"id": "EP_TIER", "name": "ep-tier", "escalation_rules": []any{
			map[string]any{"targets": []any{}},
		}},
		{"id": "EP_SPOF", "name": "ep-spof", "escalation_rules": []any{
			map[string]any{"targets": []any{map[string]any{"type": "user_reference", "id": "U1", "summary": "Alice"}}},
			map[string]any{"targets": []any{map[string]any{"type": "user_reference", "id": "U1", "summary": "Alice"}}},
		}},
		{"id": "EP_SCHED", "name": "ep-sched", "escalation_rules": []any{
			map[string]any{"targets": []any{map[string]any{"type": "schedule_reference", "id": "SCHED_EMPTY", "summary": "empty"}}},
		}},
		{"id": "EP_OK", "name": "ep-ok", "escalation_rules": []any{
			map[string]any{"targets": []any{map[string]any{"type": "user_reference", "id": "U1", "summary": "Alice"}}},
			map[string]any{"targets": []any{map[string]any{"type": "user_reference", "id": "U2", "summary": "Bob"}}},
		}},
	}
	schedules := []map[string]any{
		{"id": "SCHED_EMPTY", "name": "empty", "users": []any{}, "schedule_layers": []any{}},
	}
	res := buildCoverageAudit(services, eps, schedules, "")
	if res.ServicesChecked != 6 {
		t.Fatalf("checked=%d, want 6", res.ServicesChecked)
	}
	got := map[string]string{} // service_id -> first issue seen
	issues := map[string]bool{}
	for _, g := range res.Gaps {
		if _, ok := got[g.ServiceID]; !ok {
			got[g.ServiceID] = g.Issue
		}
		issues[g.ServiceID+":"+g.Issue] = true
	}
	checks := []struct{ svc, issue string }{
		{"SVC_NOEP", "no_escalation_policy"},
		{"SVC_EMPTY", "empty_policy"},
		{"SVC_TIER", "empty_tier"},
		{"SVC_SPOF", "single_point_of_failure"},
		{"SVC_SCHED", "empty_schedule"},
	}
	for _, c := range checks {
		if !issues[c.svc+":"+c.issue] {
			t.Errorf("expected gap %s for %s; gaps=%+v", c.issue, c.svc, res.Gaps)
		}
	}
	// Healthy service must produce no gap at all.
	for _, g := range res.Gaps {
		if g.ServiceID == "SVC_OK" {
			t.Errorf("healthy service flagged: %+v", g)
		}
	}
}

func TestBuildMttr(t *testing.T) {
	win := pdWindow{Since: fixedNow.AddDate(0, 0, -30), Until: fixedNow}
	trig := fixedNow.Add(-2 * time.Hour)
	incidents := []map[string]any{
		{"id": "I1", "service": ref("SVCA", "web"), "created_at": rfc(trig), "urgency": "high"},
	}
	logs := []map[string]any{
		{"type": "trigger_log_entry", "created_at": rfc(trig), "incident": ref("I1", "")},
		{"type": "acknowledge_log_entry", "created_at": rfc(trig.Add(5 * time.Minute)), "incident": ref("I1", "")},
		{"type": "resolve_log_entry", "created_at": rfc(trig.Add(35 * time.Minute)), "incident": ref("I1", ""), "agent": ref("U1", "Alice")},
	}
	res := buildMttr(incidents, logs, win, "service")
	if len(res.Groups) != 1 {
		t.Fatalf("groups=%d, want 1", len(res.Groups))
	}
	g := res.Groups[0]
	if g.Key != "web" || g.Incidents != 1 || g.Acknowledged != 1 || g.Resolved != 1 {
		t.Fatalf("group=%+v", g)
	}
	if g.MTTASeconds != 300 {
		t.Fatalf("MTTA=%ds, want 300", g.MTTASeconds)
	}
	if g.MTTRSeconds != 2100 {
		t.Fatalf("MTTR=%ds, want 2100", g.MTTRSeconds)
	}
}

func TestBuildResponders(t *testing.T) {
	win := pdWindow{Since: fixedNow.AddDate(0, 0, -30), Until: fixedNow}
	// A Saturday (off-hours) and a weekday afternoon for the page split.
	sat := time.Date(2026, 5, 30, 3, 0, 0, 0, time.UTC)  // Sat 03:00 → off-hours
	wed := time.Date(2026, 5, 27, 14, 0, 0, 0, time.UTC) // Wed 14:00 → on-hours
	logs := []map[string]any{
		{"type": "acknowledge_log_entry", "created_at": rfc(wed), "agent": ref("U1", "Alice")},
		{"type": "resolve_log_entry", "created_at": rfc(wed.Add(time.Hour)), "agent": map[string]any{"id": "U1", "summary": "Alice", "type": "user_reference"}},
		{"type": "notify_log_entry", "created_at": rfc(sat), "user": ref("U2", "Bob")},
		{"type": "notify_log_entry", "created_at": rfc(wed), "user": ref("U2", "Bob")},
	}
	res := buildResponders(logs, win)
	byUser := map[string]pdResponder{}
	for _, r := range res.Responders {
		byUser[r.User] = r
	}
	alice := byUser["Alice"]
	if alice.Acks != 1 || alice.Resolves != 1 {
		t.Fatalf("Alice=%+v, want acks1 resolves1", alice)
	}
	bob := byUser["Bob"]
	if bob.Pages != 2 || bob.OffHoursPages != 1 || bob.OffHoursShare != 0.5 {
		t.Fatalf("Bob=%+v, want pages2 off1 share0.5", bob)
	}
}

func TestBuildNoisy(t *testing.T) {
	win := pdWindow{Since: fixedNow.AddDate(0, 0, -30), Until: fixedNow}
	tA := fixedNow.Add(-5 * time.Hour)
	incidents := []map[string]any{
		{"id": "I1", "service": ref("SVCA", "web"), "created_at": rfc(tA), "urgency": "high"},
		{"id": "I2", "service": ref("SVCA", "web"), "created_at": rfc(tA.Add(time.Hour)), "urgency": "low"},
		{"id": "I3", "service": ref("SVCA", "web"), "created_at": rfc(tA.Add(2 * time.Hour)), "urgency": "low"},
		{"id": "I4", "service": ref("SVCB", "db"), "created_at": rfc(tA), "urgency": "low"},
	}
	logs := []map[string]any{
		// I2 auto-resolved by the service.
		{"type": "resolve_log_entry", "created_at": rfc(tA.Add(90 * time.Minute)), "incident": ref("I2", ""), "agent": map[string]any{"type": "service_reference", "id": "SVCA"}},
		// I3 re-triggered (two triggers).
		{"type": "trigger_log_entry", "created_at": rfc(tA.Add(2 * time.Hour)), "incident": ref("I3", "")},
		{"type": "trigger_log_entry", "created_at": rfc(tA.Add(3 * time.Hour)), "incident": ref("I3", "")},
	}
	res := buildNoisy(incidents, logs, win, 10)
	if len(res.Services) != 2 || res.Services[0].ServiceID != "SVCA" {
		t.Fatalf("services=%+v, want SVCA first", res.Services)
	}
	a := res.Services[0]
	if a.Incidents != 3 || a.HighUrgency != 1 || a.AutoResolved != 1 || a.Reopened != 1 {
		t.Fatalf("SVCA=%+v, want inc3 high1 auto1 reopened1", a)
	}
}

func TestBuildTimeline(t *testing.T) {
	base := fixedNow.Add(-time.Hour)
	logs := []map[string]any{
		{"type": "resolve_log_entry", "created_at": rfc(base.Add(35 * time.Minute)), "incident": ref("PX", ""), "agent": ref("U1", "Alice")},
		{"type": "trigger_log_entry", "created_at": rfc(base), "incident": ref("PX", "")},
		{"type": "acknowledge_log_entry", "created_at": rfc(base.Add(5 * time.Minute)), "incident": ref("PX", ""), "agent": ref("U1", "Alice")},
		{"type": "trigger_log_entry", "created_at": rfc(base), "incident": ref("OTHER", "")}, // excluded
	}
	res := buildTimeline(logs, "PX")
	if len(res.Events) != 3 {
		t.Fatalf("events=%d, want 3", len(res.Events))
	}
	if res.Events[0].Type != "trigger" || res.Events[0].ElapsedSeconds != 0 {
		t.Fatalf("first event=%+v, want trigger @0", res.Events[0])
	}
	if res.Events[1].Type != "acknowledge" || res.Events[1].ElapsedSeconds != 300 {
		t.Fatalf("second event=%+v, want acknowledge @300", res.Events[1])
	}
	if res.Events[2].Type != "resolve" || res.Events[2].ElapsedSeconds != 2100 {
		t.Fatalf("third event=%+v, want resolve @2100", res.Events[2])
	}
}

func TestParseWindowRelative(t *testing.T) {
	w, err := pdParseWindow("7d", "", fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	if !w.Until.Equal(fixedNow) {
		t.Fatalf("until=%v, want now", w.Until)
	}
	if got := fixedNow.Sub(w.Since); got != 7*24*time.Hour {
		t.Fatalf("since delta=%v, want 168h", got)
	}
}

func TestIsOffHours(t *testing.T) {
	cases := []struct {
		t    time.Time
		want bool
	}{
		{time.Date(2026, 5, 27, 14, 0, 0, 0, time.UTC), false}, // Wed 14:00
		{time.Date(2026, 5, 27, 3, 0, 0, 0, time.UTC), true},   // Wed 03:00
		{time.Date(2026, 5, 27, 21, 0, 0, 0, time.UTC), true},  // Wed 21:00
		{time.Date(2026, 5, 30, 14, 0, 0, 0, time.UTC), true},  // Sat 14:00
	}
	for _, c := range cases {
		if got := pdIsOffHours(c.t); got != c.want {
			t.Errorf("pdIsOffHours(%v)=%v, want %v", c.t, got, c.want)
		}
	}
}
