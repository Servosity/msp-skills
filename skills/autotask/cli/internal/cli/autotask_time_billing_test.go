// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"autotask-pp-cli/internal/cliutil/testenv"
	"autotask-pp-cli/internal/store"
)

// timeBillingStore seeds a finished sync of the documented TimeEntryModel
// shape (spec.json): no companyID and no invoice link, only ticketID / taskID /
// contractID, billingApprovalDateTime (set by Approve and Post) and
// isNonBillable. Company 10 owns ticket 1, project 30 (task 3) and contract 40.
func timeBillingStore(t *testing.T) string {
	t.Helper()
	testenv.Isolate(t)
	db := filepath.Join(t.TempDir(), "data.db")
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	put := func(rt, id, js string) {
		if err := s.Upsert(rt, id, []byte(js)); err != nil {
			t.Fatal(err)
		}
	}
	put("tickets", "1", `{"id":1,"companyID":10,"status":1,"title":"VPN"}`)
	put("tickets", "2", `{"id":2,"companyID":20,"status":1,"title":"Other client"}`)
	put("tasks", "3", `{"id":3,"projectID":30,"title":"Migration"}`)
	put("projects", "30", `{"id":30,"companyID":10,"projectName":"M365"}`)
	put("contracts", "40", `{"id":40,"companyID":10,"contractName":"Block"}`)
	put("time-entries", "101", `{"id":101,"ticketID":1,"hoursWorked":2,"hoursToBill":1.5,"isNonBillable":false}`)
	put("time-entries", "102", `{"id":102,"ticketID":1,"hoursWorked":3,"hoursToBill":3,"isNonBillable":false,"billingApprovalDateTime":"2026-10-20T10:00:00Z"}`)
	put("time-entries", "103", `{"id":103,"ticketID":1,"hoursWorked":1,"hoursToBill":0,"isNonBillable":true}`)
	put("time-entries", "104", `{"id":104,"taskID":3,"hoursWorked":4,"hoursToBill":4,"isNonBillable":false}`)
	put("time-entries", "105", `{"id":105,"contractID":40,"hoursWorked":1,"hoursToBill":1,"isNonBillable":false}`)
	put("time-entries", "106", `{"id":106,"ticketID":2,"hoursWorked":5,"hoursToBill":5,"isNonBillable":false}`)
	for _, rt := range []string{"tickets", "tasks", "projects", "contracts", "time-entries", "contract-blocks"} {
		if _, err := s.DB().Exec(`INSERT OR REPLACE INTO sync_state (resource_type, last_attempt_complete, last_synced_at) VALUES (?, 1, '2026-10-29T00:00:00Z')`, rt); err != nil {
			t.Fatal(err)
		}
	}
	recordUnfilteredWalks(t, s)
	_ = s.Close()
	return db
}

func runJSON(t *testing.T, args ...string) map[string]any {
	t.Helper()
	cmd := RootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("%v: %v\n%s", args, err, out.String())
	}
	var m map[string]any
	b := out.Bytes()
	if i := bytes.IndexByte(b, '{'); i >= 0 {
		b = b[i:]
	}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("%v output: %v\n%s", args, err, out.String())
	}
	return m
}

// Class time-entry-billing-read-from-undocumented-fields: company 10's unbilled
// time is 101 (its ticket), 104 (its project's task) and 105 (its contract),
// 6.5 hours to bill; 102 is approved and posted, 103 is non-billable and 106
// belongs to company 20.
func TestUnbilledReadsTheDocumentedTimeEntryShape(t *testing.T) {
	db := timeBillingStore(t)
	m := runJSON(t, "unbilled", "--company", "10", "--json", "--db", db)
	if n, _ := m["totalEntries"].(float64); n != 3 {
		t.Errorf("unbilled --company 10: %v entries, want 3 (101, 104, 105): %v", m["totalEntries"], m)
	}
	if h, _ := m["totalHours"].(float64); h != 6.5 {
		t.Errorf("unbilled --company 10: %v hours, want 6.5 hours to bill: %v", m["totalHours"], m)
	}
	r := runJSON(t, "reconcile", "--company", "10", "--json", "--db", db)
	u, _ := r["unbilled"].(map[string]any)
	if n, _ := u["totalEntries"].(float64); n != 3 {
		t.Errorf("reconcile --company 10: %v unbilled entries, want 3: %v", u["totalEntries"], r)
	}
	all := runJSON(t, "reconcile", "--json", "--db", db)
	u, _ = all["unbilled"].(map[string]any)
	rows, _ := u["byCompany"].([]any)
	got := map[string]float64{}
	for _, row := range rows {
		rm, _ := row.(map[string]any)
		got[rm["companyID"].(string)], _ = rm["hours"].(float64)
	}
	if got["10"] != 6.5 || got["20"] != 5 || len(got) != 2 {
		t.Errorf("reconcile byCompany = %v, want 10: 6.5 and 20: 5", got)
	}
}
