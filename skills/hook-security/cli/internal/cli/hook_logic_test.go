// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"
)

func f64(v float64) *float64 { return &v }
func str(v string) *string   { return &v }
func num(v int) *int         { return &v }

func TestClientRiskFleetIsTotalsNotAverageOfRates(t *testing.T) {
	rows := []clientRiskRow{
		{hookClientSummary: hookClientSummary{OrganizationName: "Small", Simulations: &hookSimulations{Sent: 1, Clicked: 1}}, ClickRate: f64(1)},
		{hookClientSummary: hookClientSummary{OrganizationName: "Large", Simulations: &hookSimulations{Sent: 99, Clicked: 0}}, ClickRate: f64(0)},
		{hookClientSummary: hookClientSummary{OrganizationName: "Unmeasured"}},
		{hookClientSummary: hookClientSummary{OrganizationName: "Huge", TooLarge: true}},
	}
	fleet := clientRiskTotals(rows)
	if fleet.ClickRate == nil || *fleet.ClickRate != 0.01 {
		t.Fatalf("fleet rate = %v, want 1 clicked over 100 sent", fleet.ClickRate)
	}
	if fleet.ClientsCounted != 2 || fleet.ClientsTooLarge != 1 || fleet.ClientsNoSimulations != 1 {
		t.Fatalf("fleet counts = %+v", fleet)
	}
	rankClientRisk(rows)
	if rows[0].OrganizationName != "Small" || rows[1].OrganizationName != "Large" || rows[2].ClickRate != nil {
		t.Fatalf("rank order wrong: %+v", rows)
	}
}

func clickBy(id, email, at string) hookClickResult {
	return hookClickResult{CampaignName: "Q3", OrganizationID: "org_1", Person: &hookPerson{ID: id, Name: id, Email: email}, ClickedAt: str(at)}
}

func enrollmentFor(id, email, status string) hookEnrollment {
	return hookEnrollment{ID: "enr_" + id + status, OrganizationID: "org_1", Course: hookCourse{Name: "Phishing 101"}, Person: &hookPerson{ID: id, Email: email}, Status: status}
}

func TestJoinClickedUntrained(t *testing.T) {
	at := "2026-09-01T00:00:00Z"
	results := []hookClickResult{
		clickBy("per_open", "open@example.com", at),
		clickBy("per_open", "open@example.com", "2026-09-02T00:00:00Z"),
		clickBy("per_done", "done@example.com", at),
		clickBy("per_none", "none@example.com", at),
		clickBy("per_cancel", "cancel@example.com", at),
		clickBy("per_legacy", "Legacy@Example.com", at),
		{OrganizationID: "org_1", ClickedAt: str(at)},
	}
	enrollments := []hookEnrollment{
		enrollmentFor("per_open", "open@example.com", "in_progress"),
		enrollmentFor("per_open", "open@example.com", "completed"),
		enrollmentFor("per_done", "done@example.com", "completed"),
		enrollmentFor("per_cancel", "cancel@example.com", "cancelled"),
		enrollmentFor("", "legacy@example.com", "assigned"),
	}
	rows, finished, removed := joinClickedUntrained(results, enrollments, time.Time{}, false)
	if finished != 1 || removed != 1 || len(rows) != 4 {
		t.Fatalf("finished=%d removed=%d rows=%+v", finished, removed, rows)
	}
	got := map[string]clickedUntrainedRow{}
	for _, row := range rows {
		got[row.PersonID] = row
	}
	if got["per_open"].TrainingState != "open" || got["per_open"].Clicks != 2 || len(got["per_open"].Courses) != 1 {
		t.Fatalf("open row wrong: %+v", got["per_open"])
	}
	if got["per_none"].TrainingState != "none_assigned" || got["per_cancel"].TrainingState != "none_assigned" {
		t.Fatalf("none rows wrong: %+v %+v", got["per_none"], got["per_cancel"])
	}
	if got["per_legacy"].TrainingState != "open" {
		t.Fatalf("legacy learner must match on email regardless of case: %+v", got["per_legacy"])
	}
}

func TestJoinClickedUntrainedTruncatedIsNeverFinished(t *testing.T) {
	results := []hookClickResult{clickBy("per_done", "done@example.com", "2026-09-01T00:00:00Z")}
	enrollments := []hookEnrollment{enrollmentFor("per_done", "done@example.com", "completed")}
	rows, finished, _ := joinClickedUntrained(results, enrollments, time.Time{}, true)
	if finished != 0 || len(rows) != 1 || rows[0].TrainingState != "unknown" {
		t.Fatalf("finished=%d rows=%+v, want one unknown row", finished, rows)
	}
}

func TestJoinClickedUntrainedSince(t *testing.T) {
	results := []hookClickResult{clickBy("per_old", "old@example.com", "2026-01-01T00:00:00Z"), clickBy("per_new", "new@example.com", "2026-09-01T00:00:00Z")}
	since := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	rows, _, _ := joinClickedUntrained(results, nil, since, false)
	if len(rows) != 1 || rows[0].PersonID != "per_new" {
		t.Fatalf("since filter wrong: %+v", rows)
	}
}

func TestGroupOverdue(t *testing.T) {
	enrollments := []hookEnrollment{
		{OrganizationID: "org_1", OrganizationName: "One", Status: "assigned"},
		{OrganizationID: "org_2", OrganizationName: "Two", Person: &hookPerson{Email: "a@example.com"}},
		{OrganizationID: "org_2", OrganizationName: "Two", Person: &hookPerson{Email: "b@example.com"}},
	}
	clients := groupOverdue(enrollments, false)
	if len(clients) != 2 || clients[0].OrganizationID != "org_2" || clients[0].OverdueCount != 2 || clients[1].OverdueCount != 1 || len(clients[1].People) != 1 {
		t.Fatalf("grouping wrong: %+v", clients)
	}
	for _, client := range groupOverdue(enrollments, true) {
		if len(client.People) != 0 {
			t.Fatalf("counts-only must not carry people: %+v", client)
		}
	}
}

func TestQbrReportRateWithheldIsNull(t *testing.T) {
	if rate := qbrReportRate(&hookSimulations{Sent: 10}); rate != nil {
		t.Fatalf("withheld reported must stay null, got %v", *rate)
	}
	if rate := qbrReportRate(&hookSimulations{Sent: 10, Reported: num(2)}); rate == nil || *rate != 0.2 {
		t.Fatalf("report rate = %v, want 0.2", rate)
	}
	if rate := qbrReportRate(&hookSimulations{Sent: 0, Reported: num(0)}); rate != nil {
		t.Fatalf("zero sent must be null, got %v", *rate)
	}
}

func TestQbrWindow(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	start, end, err := qbrWindow(now, 30, "", "")
	if err != nil || start != "2026-09-02" || end != "2026-10-01" {
		t.Fatalf("window = %s %s %v", start, end, err)
	}
	start, end, err = qbrWindow(now, 1, "", "2026-10-01")
	if err != nil || start != "2026-10-01" || end != "2026-10-01" {
		t.Fatalf("one-day window = %s %s %v", start, end, err)
	}
	if _, _, err := qbrWindow(now, 90, "2026-10-02", "2026-10-01"); err == nil {
		t.Fatal("start after end must fail")
	}
	if _, _, err := qbrWindow(now, 90, "10/02/2026", ""); err == nil {
		t.Fatal("bad date must fail")
	}
}
