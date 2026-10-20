// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/saas-alerts/handfixes.json
// under the "contain-cross-source-ambiguity" entry.

package cli

import (
	"strings"
	"testing"
	"time"
)

// TestContainCrossSourceAmbiguityRefuses: a unique account in synced triggers
// must not win when synced users name a different Microsoft 365 account with
// the same email.
func TestContainCrossSourceAmbiguityRefuses(t *testing.T) {
	api := newRQTAPI(t)
	rqtEnv(t, api)
	rqtFastPoll(t)
	triggers := []map[string]any{{"id": "RT_SAM", "status": "PENDING", "triggeredAt": rqtMS(-time.Hour),
		"organization": map[string]any{"id": "ORG_1", "name": "Contoso"},
		"accounts":     rqtAccount("ACC_SAM_1", "sam@example.com"),
		"rule":         rqtRule("RULE_A", "Impossible travel", "critical", "Block Sign In")}}
	msUser := map[string]any{"id": "P_MS", "name": "Microsoft 365", "type": "MS"}
	users := []map[string]any{{"id": "USER_SAM_2", "email": "sam@example.com", "name": "Sam", "parent_id": "CUST_2", "customers_id": "CUST_2", "product": msUser}}
	dbPath := rqtSeedStore(t, triggers, users)

	_, _, err := rqtRun(t, "contain", "sam@example.com", "--db", dbPath, "--confirm", "--json")
	if rqtExit(err) != 2 || !strings.Contains(err.Error(), "more than one Microsoft 365 account") {
		t.Fatalf("cross-source ambiguity must stop with exit 2, got exit=%d err=%v", rqtExit(err), err)
	}
	if n := len(api.callsMatching("POST", "/respondApi/api/v1/response-actions/execute")); n != 0 {
		t.Fatalf("an ambiguous target sent %d execute requests", n)
	}

	// The same pair in both sources is not a conflict.
	agree := []map[string]any{{"id": "ACC_SAM_1", "email": "sam@example.com", "name": "Sam", "parent_id": "ORG_1", "customers_id": "ORG_1", "product": msUser}}
	if _, _, err := rqtRun(t, "contain", "sam@example.com", "--db", rqtSeedStore(t, triggers, agree), "--json"); err != nil {
		t.Fatalf("matching sources should preview: %v", err)
	}
}
