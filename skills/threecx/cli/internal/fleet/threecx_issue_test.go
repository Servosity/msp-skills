// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package fleet

import (
	"encoding/json"
	"testing"
	"time"
)

// ConfigurationIssue is nullable (vendor-docs/3cx-swagger.yaml, Pbx.Trunk): a null
// or a missing member says nothing about the trunk, so the row is unknown, and
// history never reads it as an issue cleared.
func TestUnreportedConfigurationIssueIsNotHealthy(t *testing.T) {
	now := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
	rows := func(body string) []TrunkRow {
		var trunks []Trunk
		if err := json.Unmarshal([]byte(body), &trunks); err != nil {
			t.Fatal(err)
		}
		return TrunkRows(PBX{Name: "acme"}, trunks, now)
	}
	for _, body := range []string{
		`[{"Number":"1","IsOnline":true,"ConfigurationIssue":null}]`,
		`[{"Number":"1","IsOnline":true}]`,
	} {
		if r := rows(body)[0]; r.State != "unknown" || !r.NeedsAttention() {
			t.Errorf("%s: want unknown and needs attention, got %+v", body, r)
		}
	}
	if r := rows(`[{"Number":"1","IsOnline":true,"ConfigurationIssue":""}]`)[0]; r.State != "online" || r.NeedsAttention() {
		t.Errorf("a reported empty issue is healthy, got %+v", r)
	}
	if r := rows(`[{"Number":"1","IsOnline":false,"ConfigurationIssue":null}]`)[0]; r.State != "offline" {
		t.Errorf("a trunk reported offline stays offline, got %+v", r)
	}
	issue := rows(`[{"Number":"1","IsOnline":true,"ConfigurationIssue":"bad CID"}]`)
	silent := rows(`[{"Number":"1","IsOnline":true,"ConfigurationIssue":null}]`)
	for _, c := range DiffTrunks(snap("trunks", true, []string{"acme"}, nil, issue), snap("trunks", true, []string{"acme"}, nil, silent)) {
		if c.Change == "trunk_issue_cleared" {
			t.Errorf("an unreported issue read as cleared: %+v", c)
		}
	}
}

// fleet health says it rates version and extensions registered, so a phone system
// with none registered or that does not report them is not a pass.
func TestHealthRatesExtensionsAndVersion(t *testing.T) {
	o := HealthOptions{Now: now, WarnDays: 30, BackupStale: 48 * time.Hour}
	level := func(s SystemStatus, name string) Level {
		for _, c := range EvaluateHealth(PBX{Name: "acme"}, s, o).Checks {
			if c.Name == name {
				return c.Level
			}
		}
		t.Fatalf("no %s check", name)
		return ""
	}
	for _, c := range []struct {
		reg, total int
		want       Level
	}{{0, 10, Fail}, {4, 10, Warn}, {5, 10, Pass}, {10, 10, Pass}, {0, 0, Pass}} {
		s := healthyStatus()
		s.ExtensionsRegistered, s.ExtensionsTotal = ptr(c.reg), ptr(c.total)
		if got := level(s, "extensions"); got != c.want {
			t.Errorf("%d of %d: got %s, want %s", c.reg, c.total, got, c.want)
		}
	}
	s := healthyStatus()
	s.ExtensionsRegistered = nil
	if got := level(s, "extensions"); got == Pass {
		t.Error("unreported extensions read as pass")
	}
	s = healthyStatus()
	s.Version = ""
	if got := level(s, "version"); got == Pass {
		t.Error("an unreported version read as pass")
	}
	if r := EvaluateHealth(PBX{Name: "acme"}, healthyStatus(), o); r.Status != Pass {
		t.Errorf("the healthy fixture stopped passing: %+v", r)
	}
}
