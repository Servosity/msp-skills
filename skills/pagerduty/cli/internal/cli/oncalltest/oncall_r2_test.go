// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package oncalltest

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"pagerduty-pp-cli/internal/cliutil/testenv"
)

// Class partial-pagination-used-as-deletion-evidence: reaching the last page
// proves the walk was complete only when it started at the first page and read
// every page. Each case ends on a natural last page but never read the
// entries of the first, so none of them may delete what it did not see.
func TestR2PartialWalkIsNotDeletionEvidence(t *testing.T) {
	permanent := func(user string) string { return pdOncallJSON("PEP0001", "1", "", user, "", "") }

	t.Run("a user offset", func(t *testing.T) { // the reviewer's input
		testenv.Isolate(t)
		set := pdPagedOncalls(t)
		set(func(q map[string]string) (string, bool, int) {
			return `[` + permanent("PUSR001") + `,` + permanent("PUSR002") + `]`, false, 0
		})
		mustRunPagerDuty(t, "sync", "--resources", "oncalls", "--no-learn")
		set(func(q map[string]string) (string, bool, int) { return `[` + permanent("PUSR002") + `]`, false, 0 })
		mustRunPagerDuty(t, "sync", "--resources", "oncalls", "--param", "offset=1", "--no-learn")
		if got := strings.Join(pdWhoUserIDs(t), ","); got != "PUSR001,PUSR002" {
			t.Fatalf("sync --param offset=1 deleted an entry it never read: on call = %s", got)
		}
	})

	t.Run("a user limit", func(t *testing.T) {
		testenv.Isolate(t)
		set := pdPagedOncalls(t)
		set(func(q map[string]string) (string, bool, int) {
			return `[` + permanent("PUSR001") + `,` + permanent("PUSR002") + `]`, false, 0
		})
		mustRunPagerDuty(t, "sync", "--resources", "oncalls", "--no-learn")
		// limit=1 narrows the page, while the walk still advances by its own
		// page size: PUSR002 at offset 1 is stepped over.
		set(func(q map[string]string) (string, bool, int) {
			if q["offset"] == "" || q["offset"] == "0" {
				return `[` + permanent("PUSR001") + `]`, true, 0
			}
			return `[]`, false, 0
		})
		mustRunPagerDuty(t, "sync", "--resources", "oncalls", "--param", "limit=1", "--no-learn")
		if got := strings.Join(pdWhoUserIDs(t), ","); got != "PUSR001,PUSR002" {
			t.Fatalf("sync --param limit=1 deleted an entry it stepped over: on call = %s", got)
		}
	})

	t.Run("a resumed walk", func(t *testing.T) {
		testenv.Isolate(t)
		set := pdPagedOncalls(t)
		var first []string
		for i := 0; i < 100; i++ {
			first = append(first, permanent(fmt.Sprintf("PUSR%03d", i)))
		}
		page := `[` + strings.Join(first, ",") + `]`
		set(func(q map[string]string) (string, bool, int) {
			if q["offset"] == "" || q["offset"] == "0" {
				return page, true, 0
			}
			return "", false, http.StatusInternalServerError
		})
		// The walk dies on page 2 and leaves its resume point behind.
		if _, _, err := runPagerDuty(t, "sync", "--resources", "oncalls", "--no-learn"); err == nil {
			t.Fatal("the first walk was expected to fail on page 2")
		}
		set(func(q map[string]string) (string, bool, int) {
			if q["offset"] == "" || q["offset"] == "0" {
				t.Errorf("the second sync restarted instead of resuming: %v", q)
			}
			return `[` + permanent("PUSR900") + `]`, false, 0
		})
		mustRunPagerDuty(t, "sync", "--resources", "oncalls", "--no-learn")
		got := pdWhoUserIDs(t)
		if len(got) != 101 {
			t.Fatalf("a resumed walk deleted entries it never read: %d on call, want 101", len(got))
		}
	})

	t.Run("a whole walk still removes", func(t *testing.T) { // the other direction
		testenv.Isolate(t)
		set := pdPagedOncalls(t)
		set(func(q map[string]string) (string, bool, int) {
			return `[` + permanent("PUSR001") + `,` + permanent("PUSR002") + `]`, false, 0
		})
		mustRunPagerDuty(t, "sync", "--resources", "oncalls", "--no-learn")
		set(func(q map[string]string) (string, bool, int) { return `[` + permanent("PUSR002") + `]`, false, 0 })
		mustRunPagerDuty(t, "sync", "--resources", "oncalls", "--no-learn")
		if got := strings.Join(pdWhoUserIDs(t), ","); got != "PUSR002" {
			t.Fatalf("a whole walk left a responder PagerDuty no longer returns: on call = %s", got)
		}
	})
}

// Class resolved-ids-confused-with-display-names: a scope flag names a thing
// by id or by name. It resolves to ids once, an id wins over a name that merely
// looks like one, and from there on ids compare to ids only.
func TestR2ScopesCompareIDsToIDs(t *testing.T) {
	now := time.Now().UTC()
	start, end := now.Add(-time.Hour).Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339)
	oncall := func(ep, epLabel, user, userLabel string) string {
		return fmt.Sprintf(`{"escalation_policy":%s,"escalation_level":1,"schedule":null,"user":%s,"start":%q,"end":%q}`,
			pdRef("escalation_policy_reference", ep, epLabel), pdRef("user_reference", user, userLabel), start, end)
	}
	policy := func(id, name, team, teamName string) string {
		return fmt.Sprintf(`{"id":%q,"type":"escalation_policy","name":%q,"summary":%q,"teams":[%s]}`, id, name, name, pdRef("team_reference", team, teamName))
	}
	service := func(id, name, ep string) string {
		return fmt.Sprintf(`{"id":%q,"type":"service","name":%q,"summary":%q,"status":"active","escalation_policy":%s}`, id, name, name, pdRef("escalation_policy_reference", ep, ep))
	}

	t.Run("a policy label that reads as another's id", func(t *testing.T) { // the reviewer's input
		testenv.Isolate(t)
		f := newFakePagerDuty(t)
		pdSetBody(f, "/escalation_policies", `[`+policy("PEP0001", "Payments EP", "PTEAM01", "Payments")+`,`+policy("PEP0002", "PEP0001", "PTEAM02", "Platform")+`]`)
		pdSetBody(f, "/oncalls", `[`+oncall("PEP0001", "Payments EP", "PUSR001", "Ana Ruiz")+`,`+oncall("PEP0002", "PEP0001", "PUSR002", "Ben Okafor")+`]`)
		mustRunPagerDuty(t, "sync", "--resources", pdOncallTables, "--no-learn")
		for _, c := range []struct {
			args []string
			want string
		}{
			{[]string{"--team", "PTEAM01"}, "PUSR001"},
			{[]string{"--team", "Platform"}, "PUSR002"},
			{[]string{"--escalation-policy", "PEP0001"}, "PUSR001"},
			{[]string{"--escalation-policy", "PEP0002"}, "PUSR002"},
			{[]string{"--escalation-policy", "Payments EP"}, "PUSR001"},
			{[]string{"--escalation-policy", "PTEAM09"}, ""},
		} {
			if got := strings.Join(pdWhoUserIDs(t, c.args...), ","); got != c.want {
				t.Errorf("oncall who %v = %q, want %q", c.args, got, c.want)
			}
		}
	})

	t.Run("every scope flag, a name that reads as another's id", func(t *testing.T) {
		testenv.Isolate(t)
		f := newFakePagerDuty(t)
		// The look-alike comes first, so a first-match reader finds it.
		pdSetBody(f, "/escalation_policies", `[`+policy("PEP0004", "Ops EP 4", "PTEAM04", "PTEAM03")+`,`+policy("PEP0003", "Ops EP 3", "PTEAM03", "Ops")+`]`)
		pdSetBody(f, "/services", `[`+service("PSVC004", "PSVC003", "PEP0004")+`,`+service("PSVC003", "Checkout", "PEP0003")+`]`)
		pdSetBody(f, "/oncalls", `[`+oncall("PEP0004", "Ops EP 4", "PUSR004", "PUSR003")+`,`+oncall("PEP0003", "Ops EP 3", "PUSR003", "Ann Lee")+`]`)
		mustRunPagerDuty(t, "sync", "--resources", pdOncallTables, "--no-learn")
		for _, c := range []struct {
			args []string
			want string
		}{
			{[]string{"--team", "PTEAM03"}, "PUSR003"},
			{[]string{"--team", "Ops"}, "PUSR003"},
			{[]string{"--user", "PUSR003"}, "PUSR003"},
			{[]string{"--user", "Ann Lee"}, "PUSR003"},
			{[]string{"--service", "PSVC003"}, "PUSR003"},
			{[]string{"--service", "Checkout"}, "PUSR003"},
			{[]string{"--user", "PUSR009"}, ""},
		} {
			if got := strings.Join(pdWhoUserIDs(t, c.args...), ","); got != c.want {
				t.Errorf("oncall who %v = %q, want %q", c.args, got, c.want)
			}
		}
	})

	t.Run("a name two things share answers for both", func(t *testing.T) {
		testenv.Isolate(t)
		f := newFakePagerDuty(t)
		pdSetBody(f, "/escalation_policies", `[`+policy("PEP0005", "On call", "PTEAM05", "Ops")+`,`+policy("PEP0006", "On call", "PTEAM06", "Ops")+`]`)
		pdSetBody(f, "/services", `[`+service("PSVC005", "Checkout", "PEP0005")+`,`+service("PSVC006", "Checkout", "PEP0006")+`]`)
		pdSetBody(f, "/oncalls", `[`+oncall("PEP0005", "On call", "PUSR005", "Sam Lee")+`,`+oncall("PEP0006", "On call", "PUSR006", "Sam Lee")+`]`)
		mustRunPagerDuty(t, "sync", "--resources", pdOncallTables, "--no-learn")
		for _, args := range [][]string{{"--escalation-policy", "On call"}, {"--team", "Ops"}, {"--service", "Checkout"}, {"--user", "Sam Lee"}} {
			if got := strings.Join(pdWhoUserIDs(t, args...), ","); got != "PUSR005,PUSR006" {
				t.Errorf("oncall who %v = %q, want both", args, got)
			}
		}
	})
}
