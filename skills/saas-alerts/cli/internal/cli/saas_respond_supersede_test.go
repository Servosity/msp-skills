// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/saas-alerts/handfixes.json
// under the "respond-supersession-scoped-to-org-and-accounts" entry.

package cli

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func rqSupersedeTrigger(t *testing.T, id, org, email string, ago time.Duration) rqTrigger {
	t.Helper()
	raw := fmt.Sprintf(`{"id":%q,"status":"PENDING","triggeredAt":%d,"organization":{"id":%q},"accounts":[{"email":%q}],"rule":{"id":"R1","ruleTriggerType":"MANUAL_APPROVAL"}}`,
		id, rqtNow.Add(-ago).UnixMilli(), org, email)
	var tr rqTrigger
	if err := json.Unmarshal([]byte(raw), &tr); err != nil {
		t.Fatal(err)
	}
	return tr
}

// TestRespondSupersessionNeverHidesAnotherCustomer: one rule firing for
// customer B must not expire customer A's live approval; a newer firing for the
// same customer and account still supersedes.
func TestRespondSupersessionNeverHidesAnotherCustomer(t *testing.T) {
	items := rqAnnotate([]rqTrigger{
		rqSupersedeTrigger(t, "RT_A", "ORG_A", "alice@a.example", time.Hour),
		rqSupersedeTrigger(t, "RT_B", "ORG_B", "bob@b.example", 30*time.Minute),
		rqSupersedeTrigger(t, "RT_A2", "ORG_A", "alice@a.example", 10*time.Minute),
		rqSupersedeTrigger(t, "RT_C", "ORG_A", "carol@a.example", 5*time.Minute),
	}, rqtNow)
	superseded := map[string]bool{}
	for _, it := range items {
		superseded[it.TriggerID] = it.Superseded
	}
	want := map[string]bool{"RT_A": true, "RT_B": false, "RT_A2": false, "RT_C": false}
	for id, w := range want {
		if superseded[id] != w {
			t.Errorf("%s superseded=%v, want %v (all: %v)", id, superseded[id], w, superseded)
		}
	}
}
