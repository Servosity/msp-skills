// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written tests for the risk-overlap novel feature.

package cli

import "testing"

func TestPeopleJoinKey(t *testing.T) {
	cases := []struct {
		name string
		id   peopleIdentity
		want string
	}{
		{name: "guid wins", id: peopleIdentity{GUID: "g-1", Emails: []string{"a@example.test"}}, want: "guid:g-1"},
		{name: "email fallback lowercased", id: peopleIdentity{Emails: []string{"Jane.Doe@Example.Test"}}, want: "email:jane.doe@example.test"},
		{name: "no identity", id: peopleIdentity{}, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := peopleJoinKey(tc.id); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func overlapFixtures() ([]vapAPIUser, []clickerAPIUser) {
	vaps := []vapAPIUser{
		{Identity: peopleIdentity{GUID: "g-low", Emails: []string{"low@example.test"}, Name: "Low Attack"}},
		{Identity: peopleIdentity{GUID: "g-high", Emails: []string{"high@example.test"}, Name: "High Attack", Department: "Finance"}},
		{Identity: peopleIdentity{GUID: "g-noclick", Emails: []string{"noclick@example.test"}}},
	}
	vaps[0].ThreatStatistics.AttackIndex = 100
	vaps[1].ThreatStatistics.AttackIndex = 900
	vaps[1].ThreatStatistics.Families = []struct {
		Name  string `json:"name"`
		Score int    `json:"score"`
	}{{Name: "phish", Score: 800}}
	vaps[2].ThreatStatistics.AttackIndex = 500

	clickers := []clickerAPIUser{
		{Identity: peopleIdentity{GUID: "g-high", Emails: []string{"high@example.test"}}},
		{Identity: peopleIdentity{GUID: "g-low", Emails: []string{"low@example.test"}}},
		{Identity: peopleIdentity{GUID: "g-onlyclick", Emails: []string{"onlyclick@example.test"}}},
	}
	clickers[0].ClickStatistics.ClickCount = 4
	clickers[1].ClickStatistics.ClickCount = 9
	clickers[2].ClickStatistics.ClickCount = 20
	return vaps, clickers
}

func TestJoinRiskOverlapIntersectsAndSorts(t *testing.T) {
	vaps, clickers := overlapFixtures()
	rows := joinRiskOverlap(vaps, clickers)

	if len(rows) != 2 {
		t.Fatalf("got %d overlap rows, want 2 (vap-only and clicker-only people must be excluded)", len(rows))
	}
	if rows[0].AttackIndex != 900 || rows[0].Name != "High Attack" {
		t.Fatalf("rows must sort by attack index desc; got first row %+v", rows[0])
	}
	if rows[0].ClickCount != 4 {
		t.Fatalf("click count must come from the clicker record; got %d, want 4", rows[0].ClickCount)
	}
	if len(rows[0].ThreatFamilies) != 1 || rows[0].ThreatFamilies[0] != "phish" {
		t.Fatalf("threat families not projected: %+v", rows[0].ThreatFamilies)
	}
	if rows[1].AttackIndex != 100 || rows[1].ClickCount != 9 {
		t.Fatalf("second row wrong: %+v", rows[1])
	}
	for _, row := range rows {
		if row.Emails[0] == "noclick@example.test" || row.Emails[0] == "onlyclick@example.test" {
			t.Fatalf("non-overlapping person leaked into the join: %+v", row)
		}
	}
}

func TestJoinRiskOverlapEmailFallbackJoin(t *testing.T) {
	vaps := []vapAPIUser{{Identity: peopleIdentity{Emails: []string{"Same@Example.Test"}}}}
	vaps[0].ThreatStatistics.AttackIndex = 42
	clickers := []clickerAPIUser{{Identity: peopleIdentity{Emails: []string{"same@example.test"}}}}
	clickers[0].ClickStatistics.ClickCount = 3

	rows := joinRiskOverlap(vaps, clickers)
	if len(rows) != 1 {
		t.Fatalf("case-insensitive email join failed: got %d rows", len(rows))
	}
}

func TestJoinRiskOverlapNoOverlap(t *testing.T) {
	vaps := []vapAPIUser{{Identity: peopleIdentity{GUID: "g-a"}}}
	clickers := []clickerAPIUser{{Identity: peopleIdentity{GUID: "g-b"}}}
	rows := joinRiskOverlap(vaps, clickers)
	if len(rows) != 0 {
		t.Fatalf("disjoint inputs must produce zero rows, got %d", len(rows))
	}
}
