// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package oncalltest

import (
	"strings"
	"testing"
	"time"

	"pagerduty-pp-cli/internal/cliutil/testenv"
)

// Class stale-oncall-rows-read-as-current: sync only upserted on-call
// entries, so a responder PagerDuty no longer returns (a permanent on-call
// replaced, a shift reassigned) stayed current after a fresh complete sync.
// GET /oncalls answers for a time window: a complete walk removes the stored
// entries inside its window it did not return, and keeps every entry outside
// it (history and future shifts) and every entry a narrowed walk may have
// left out.
// pdOncallTables is what the on-call answers read. Walking only these keeps
// each sync in these tests under a second; TestNovelsReadWhatSyncWrites walks
// every resource.
const pdOncallTables = "oncalls,escalation-policies,services,teams,users,schedules"

// Class requested-scope-ignored: `oncall who --team` set only the label, so
// every team's responders answered for the one asked; and one scope flag
// replaced another (--service overrode --escalation-policy). Every scope
// flag narrows the result, together.
func TestR1OncallWhoAppliesEveryScope(t *testing.T) {
	testenv.Isolate(t)
	f := newFakePagerDuty(t)
	now := time.Now().UTC()
	ts := func(d time.Duration) string { return now.Add(d).Format(time.RFC3339) }
	pdSetBody(f, "/escalation_policies", `[`+
		`{"id":"PEP0001","type":"escalation_policy","name":"Policy PEP0001","summary":"Policy PEP0001","teams":[`+pdRef("team_reference", "PTEAM01", "Payments")+`]},`+
		`{"id":"PEP0002","type":"escalation_policy","name":"Policy PEP0002","summary":"Policy PEP0002","teams":[`+pdRef("team_reference", "PTEAM02", "Platform")+`]}]`)
	pdSetBody(f, "/oncalls", `[`+
		pdOncallJSON("PEP0001", "1", "PSCH001", "PUSR001", ts(-time.Hour), ts(time.Hour))+`,`+
		pdOncallJSON("PEP0002", "1", "PSCH002", "PUSR004", ts(-time.Hour), ts(time.Hour))+`]`)
	mustRunPagerDuty(t, "sync", "--resources", pdOncallTables, "--no-learn")
	for _, c := range []struct {
		args       []string
		want, deny []string
	}{
		{[]string{"--team", "PTEAM01"}, []string{"PUSR001"}, []string{"PUSR004"}},
		{[]string{"--team", "Payments"}, []string{"PUSR001"}, []string{"PUSR004"}},
		{[]string{"--team", "Platform"}, []string{"PUSR004"}, []string{"PUSR001"}},
		{[]string{"--team", "PTEAM09"}, nil, []string{"PUSR001", "PUSR004"}},
		{[]string{"--team", "PTEAM01", "--escalation-policy", "PEP0002"}, nil, []string{"PUSR001", "PUSR004"}},
		{[]string{"--service", "PSVC001", "--escalation-policy", "PEP0002"}, nil, []string{"PUSR001", "PUSR004"}},
		{[]string{"--escalation-policy", "PEP0002", "--user", "PUSR001"}, nil, []string{"PUSR001", "PUSR004"}},
	} {
		out := mustRunPagerDuty(t, append(append([]string{"oncall", "who"}, c.args...), "--json", "--no-learn")...)
		for _, w := range c.want {
			if !strings.Contains(out, w) {
				t.Errorf("oncall who %v: missing %s in\n%s", c.args, w, out)
			}
		}
		for _, d := range c.deny {
			if strings.Contains(out, d) {
				t.Errorf("oncall who %v: %s is outside the requested scope but answered:\n%s", c.args, d, out)
			}
		}
	}
}

// Class simultaneous-responders-dropped: the next responders were kept one
// per (policy, level), so two schedules (or two directly targeted users) on
// one level handing on at once showed one person. Every target of a level is
// a responder of its own.
func TestR1OncallWhoKeepsEverySimultaneousNextResponder(t *testing.T) {
	testenv.Isolate(t)
	f := newFakePagerDuty(t)
	now := time.Now().UTC()
	ts := func(d time.Duration) string { return now.Add(d).Format(time.RFC3339) }
	pdSetBody(f, "/oncalls", `[`+
		pdOncallJSON("PEP0001", "1", "PSCH001", "PUSR001", ts(10*time.Hour), ts(34*time.Hour))+`,`+
		pdOncallJSON("PEP0001", "1", "PSCH002", "PUSR002", ts(10*time.Hour), ts(34*time.Hour))+`,`+
		pdOncallJSON("PEP0001", "2", "", "PUSR003", ts(12*time.Hour), ts(36*time.Hour))+`,`+
		pdOncallJSON("PEP0001", "2", "", "PUSR005", ts(12*time.Hour), ts(36*time.Hour))+`,`+
		pdOncallJSON("PEP0001", "1", "PSCH001", "PUSR006", ts(34*time.Hour), ts(58*time.Hour))+`]`)
	mustRunPagerDuty(t, "sync", "--resources", pdOncallTables, "--no-learn")
	out := mustRunPagerDuty(t, "oncall", "who", "--json", "--no-learn")
	for _, w := range []string{"PUSR001", "PUSR002", "PUSR003", "PUSR005"} {
		if !strings.Contains(out, w) {
			t.Errorf("next responder %s dropped:\n%s", w, out)
		}
	}
	if strings.Contains(out, "PUSR006") {
		t.Errorf("a later shift of the same schedule answered as next:\n%s", out)
	}
}

func TestR1ResyncRemovesOncallsTheWindowNoLongerReturns(t *testing.T) {
	testenv.Isolate(t)
	f := newFakePagerDuty(t)
	now := time.Now().UTC()
	ts := func(d time.Duration) string { return now.Add(d).Format(time.RFC3339) }
	past := pdOncallJSON("PEP0001", "1", "PSCH001", "PUSR007", ts(-72*time.Hour), ts(-48*time.Hour))
	future := pdOncallJSON("PEP0001", "1", "PSCH001", "PUSR005", ts(48*time.Hour), ts(72*time.Hour))
	pdSetBody(f, "/oncalls", `[`+pdOncallJSON("PEP0001", "1", "", "PUSR001", "", "")+`,`+
		pdOncallJSON("PEP0001", "2", "PSCH001", "PUSR003", ts(-time.Hour), ts(time.Hour))+`,`+past+`,`+future+`]`)
	mustRunPagerDuty(t, "sync", "--resources", pdOncallTables, "--no-learn")

	// The direct target is replaced and the level-2 shift reassigned; a sync
	// narrowed to one user's on-calls may not remove anything.
	pdSetBody(f, "/oncalls", `[`+pdOncallJSON("PEP0001", "1", "", "PUSR002", "", "")+`,`+
		pdOncallJSON("PEP0001", "2", "PSCH001", "PUSR008", ts(-time.Hour), ts(time.Hour))+`]`)
	mustRunPagerDuty(t, "sync", "--resources", pdOncallTables, "--no-learn", "--param", "user_ids[]=PUSR002")
	if out := mustRunPagerDuty(t, "oncall", "who", "--json", "--no-learn"); !strings.Contains(out, "PUSR001") || !strings.Contains(out, "PUSR003") {
		t.Fatalf("a sync narrowed to one user removed other responders:\n%s", out)
	}

	for _, args := range [][]string{{"sync", "--resources", pdOncallTables, "--no-learn"}, {"sync", "--resources", pdOncallTables, "--no-learn", "--full"}} {
		mustRunPagerDuty(t, args...)
		out := mustRunPagerDuty(t, "oncall", "who", "--json", "--no-learn")
		for _, gone := range []string{"PUSR001", "PUSR003"} {
			if strings.Contains(out, gone) {
				t.Errorf("%v: %s, no longer returned for now, still answers as on call:\n%s", args, gone, out)
			}
		}
		for _, w := range []string{"PUSR002", "PUSR008", "PUSR005"} {
			if !strings.Contains(out, w) {
				t.Errorf("%v: %s missing:\n%s", args, w, out)
			}
		}
		if hours := mustRunPagerDuty(t, "oncall", "hours", "--since", "7d", "--json", "--no-learn"); !strings.Contains(hours, "PUSR007") {
			t.Errorf("%v: a past shift outside the walked window was removed:\n%s", args, hours)
		}
	}
}
