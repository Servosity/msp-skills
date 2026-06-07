// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Behavioral tests for the hand-authored transcendence commands. These seed a
// synthetic JSON:API store and assert each command's actual output — the offline
// correctness gate that stands in for live dogfooding when no API key is present.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"rootly-pp-cli/internal/store"
)

// rfc is a tiny helper to format a time offset from now.
func rfc(d time.Duration) string { return time.Now().Add(d).Format(time.RFC3339) }

func sev(name string) map[string]any {
	return map[string]any{"data": map[string]any{"attributes": map[string]any{"name": name}}}
}
func svcRef(name string) map[string]any {
	return map[string]any{"data": map[string]any{"attributes": map[string]any{"name": name}}}
}

// seedStore populates a temp store with a realistic incident-management fixture
// and returns its path.
func seedStore(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "data.db")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	put := func(rt string, recs ...map[string]any) {
		items := make([]json.RawMessage, 0, len(recs))
		for _, r := range recs {
			b, _ := json.Marshal(r)
			items = append(items, b)
		}
		if _, _, err := db.UpsertBatch(rt, items); err != nil {
			t.Fatalf("upsert %s: %v", rt, err)
		}
	}

	incident := func(id, title, sevName, status, service string, started, resolved time.Duration, acked *time.Duration, resolution string) map[string]any {
		attrs := map[string]any{
			"title":      title,
			"summary":    title + " summary",
			"status":     status,
			"severity":   sev(sevName),
			"services":   []any{svcRef(service)},
			"started_at": rfc(started),
		}
		if resolved != 0 {
			attrs["resolved_at"] = rfc(resolved)
		}
		if acked != nil {
			attrs["acknowledged_at"] = rfc(*acked)
		}
		if resolution != "" {
			attrs["resolution_message"] = resolution
		}
		return map[string]any{"id": id, "type": "incidents", "attributes": attrs}
	}

	ack1 := -170 * time.Minute
	put("incidents",
		incident("INC1", "Checkout API latency spike", "SEV1", "resolved", "checkout-api", -3*time.Hour, -1*time.Hour, &ack1, "rolled back the bad deploy"),
		incident("INC2", "Checkout API returning 500 errors", "SEV1", "resolved", "checkout-api", -5*24*time.Hour, -4*24*time.Hour, nil, "scaled the pods back up"),
		incident("INC3", "Database connection pool exhausted", "SEV2", "started", "db", -30*time.Minute, 0, nil, ""),
		incident("INC4", "Payment gateway timeout", "SEV2", "resolved", "payments", -10*24*time.Hour, -9*24*time.Hour, nil, "restarted the gateway"),
	)

	put("services",
		map[string]any{"id": "SVC1", "type": "services", "attributes": map[string]any{"name": "checkout-api", "escalation_policy_id": "EP1"}},
		map[string]any{"id": "SVC2", "type": "services", "attributes": map[string]any{"name": "db", "escalation_policy_id": "EP2"}},
		map[string]any{"id": "SVC3", "type": "services", "attributes": map[string]any{"name": "payments", "escalation_policy_id": "EP3"}},
	)

	put("oncalls",
		map[string]any{"id": "OC1", "type": "oncalls", "attributes": map[string]any{"user": map[string]any{"name": "Alice"}, "schedule": map[string]any{"name": "Primary"}}, "relationships": map[string]any{"escalation_policy": map[string]any{"data": map[string]any{"id": "EP1"}}}},
		map[string]any{"id": "OC2", "type": "oncalls", "attributes": map[string]any{"user": map[string]any{"name": "Bob"}, "schedule": map[string]any{"name": "DB-OnCall"}}, "relationships": map[string]any{"escalation_policy": map[string]any{"data": map[string]any{"id": "EP2"}}}},
	)

	put("schedules",
		map[string]any{"id": "SCHED1", "type": "schedules", "attributes": map[string]any{"name": "Primary"}},
	)

	// One shift covering [now-1h, now+1h] on SCHED1; leaves a gap afterward.
	put("shifts",
		map[string]any{"id": "SH1", "type": "shifts", "attributes": map[string]any{"schedule_id": "SCHED1", "user_id": "U1", "starts_at": rfc(-1 * time.Hour), "ends_at": rfc(1 * time.Hour)}},
	)

	// Open action item linked to INC3.
	put("action-items",
		map[string]any{"id": "AI1", "type": "action_items", "attributes": map[string]any{"summary": "Add connection pool alerting", "status": "open"}, "relationships": map[string]any{"incident": map[string]any{"data": map[string]any{"id": "INC3"}}}},
	)

	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return dbPath
}

func TestNovelFeatures(t *testing.T) {
	dbPath := seedStore(t)

	exec := func(build func(*rootFlags) *cobra.Command, args ...string) (map[string]any, error) {
		flags := &rootFlags{asJSON: true}
		cmd := build(flags)
		// Execute the leaf command directly (bypassing root), so silence
		// cobra's own error/usage printing — otherwise it pollutes stdout and
		// breaks the JSON parse on the deploy-guard non-zero-exit path.
		cmd.SilenceErrors = true
		cmd.SilenceUsage = true
		var outBuf, errBuf bytes.Buffer
		cmd.SetOut(&outBuf)
		cmd.SetErr(&errBuf)
		full := append(append([]string{}, args...), "--db", dbPath)
		cmd.SetArgs(full)
		err := cmd.ExecuteContext(context.Background())
		var out map[string]any
		_ = json.Unmarshal(outBuf.Bytes(), &out)
		return out, err
	}

	t.Run("related ranks the sibling checkout incident first", func(t *testing.T) {
		out, err := exec(newNovelRelatedCmd, "INC1")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		rel, _ := out["related"].([]any)
		if len(rel) == 0 {
			t.Fatalf("expected related incidents, got none: %v", out)
		}
		top := rel[0].(map[string]any)
		if top["id"] != "INC2" {
			t.Errorf("expected INC2 as most similar to INC1, got %v", top["id"])
		}
	})

	t.Run("mttr by severity reports two resolved SEV1", func(t *testing.T) {
		out, err := exec(newNovelMttrCmd, "--by", "severity")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		groups, _ := out["groups"].([]any)
		var sev1 map[string]any
		for _, g := range groups {
			m := g.(map[string]any)
			if m["key"] == "SEV1" {
				sev1 = m
			}
		}
		if sev1 == nil {
			t.Fatalf("no SEV1 group: %v", out)
		}
		if got := sev1["resolved"].(float64); got != 2 {
			t.Errorf("expected 2 resolved SEV1, got %v", got)
		}
		if sev1["mttr_minutes"].(float64) <= 0 {
			t.Errorf("expected positive MTTR for SEV1, got %v", sev1["mttr_minutes"])
		}
	})

	t.Run("service-health scopes to checkout-api with on-call", func(t *testing.T) {
		out, err := exec(newNovelServiceHealthCmd, "checkout-api")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		svcs, _ := out["services"].([]any)
		if len(svcs) != 1 {
			t.Fatalf("expected 1 service card, got %d: %v", len(svcs), out)
		}
		c := svcs[0].(map[string]any)
		if c["incidents"].(float64) != 2 {
			t.Errorf("expected 2 incidents on checkout-api, got %v", c["incidents"])
		}
		if oc, _ := c["current_oncall"].(string); !strings.Contains(oc, "Alice") {
			t.Errorf("expected Alice on-call for checkout-api (via EP1), got %q", oc)
		}
	})

	t.Run("oncall-now lists both schedules", func(t *testing.T) {
		out, err := exec(newNovelOncallNowCmd)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if out["count"].(float64) != 2 {
			t.Errorf("expected 2 on-call entries, got %v", out["count"])
		}
	})

	t.Run("coverage-gaps finds a gap after the single shift", func(t *testing.T) {
		out, err := exec(newNovelCoverageGapsCmd, "--days", "7")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if out["schedules_with_gaps"].(float64) < 1 {
			t.Errorf("expected at least one schedule with gaps, got %v", out["schedules_with_gaps"])
		}
		if out["total_gap_minutes"].(float64) <= 0 {
			t.Errorf("expected positive total gap minutes, got %v", out["total_gap_minutes"])
		}
	})

	t.Run("deploy-guard blocks db (open incident) and clears checkout-api", func(t *testing.T) {
		out, err := exec(newNovelDeployGuardCmd, "db")
		if err == nil {
			t.Errorf("expected non-zero exit for db (open INC3), got nil err")
		} else {
			var ce *cliError
			if !errors.As(err, &ce) || ce.code != 8 {
				t.Errorf("expected cliError code 8, got %v", err)
			}
		}
		if out["safe"].(bool) {
			t.Errorf("expected db unsafe, got safe=true")
		}

		out2, err2 := exec(newNovelDeployGuardCmd, "checkout-api")
		if err2 != nil {
			t.Errorf("expected checkout-api safe (exit 0), got err %v", err2)
		}
		if !out2["safe"].(bool) {
			t.Errorf("expected checkout-api safe=true, got false")
		}
	})

	t.Run("fixed-last-time surfaces past resolutions for checkout-api", func(t *testing.T) {
		out, err := exec(newNovelFixedLastTimeCmd, "checkout-api")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		matches, _ := out["matches"].([]any)
		if len(matches) != 2 {
			t.Fatalf("expected 2 resolved checkout-api incidents, got %d: %v", len(matches), out)
		}
		// Most recent first: INC1 resolved 1h ago beats INC2 resolved 4d ago.
		first := matches[0].(map[string]any)
		if !strings.Contains(fmt.Sprint(first["resolution"]), "rolled back") {
			t.Errorf("expected INC1 resolution first, got %v", first["resolution"])
		}
	})

	t.Run("war-room INC3 shows open incident, db on-call, action item", func(t *testing.T) {
		out, err := exec(newNovelWarRoomCmd, "INC3")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !out["open"].(bool) {
			t.Errorf("expected INC3 open")
		}
		ais, _ := out["open_action_items"].([]any)
		if len(ais) != 1 || !strings.Contains(fmt.Sprint(ais[0]), "alerting") {
			t.Errorf("expected the linked open action item, got %v", ais)
		}
		oc, _ := out["current_oncall"].([]any)
		joined := fmt.Sprint(oc...)
		if !strings.Contains(joined, "Bob") {
			t.Errorf("expected Bob on-call for db service (EP2), got %v", oc)
		}
	})

	t.Run("handoff windows incidents over last 24h", func(t *testing.T) {
		out, err := exec(newNovelHandoffCmd)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		opened, _ := out["opened"].([]any)        // INC1 (3h), INC3 (30m)
		closed, _ := out["closed"].([]any)        // INC1 (resolved 1h ago)
		stillOpen, _ := out["still_open"].([]any) // INC3
		if len(opened) < 2 {
			t.Errorf("expected >=2 opened in last 24h, got %d", len(opened))
		}
		if len(closed) < 1 {
			t.Errorf("expected >=1 closed in last 24h, got %d", len(closed))
		}
		foundINC3 := false
		for _, r := range stillOpen {
			if r.(map[string]any)["id"] == "INC3" {
				foundINC3 = true
			}
		}
		if !foundINC3 {
			t.Errorf("expected INC3 still open, got %v", stillOpen)
		}
	})

	t.Run("postmortem-skeleton renders markdown with the resolution", func(t *testing.T) {
		flags := &rootFlags{} // not JSON: capture raw markdown
		cmd := newNovelPostmortemSkeletonCmd(flags)
		var buf bytes.Buffer
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		cmd.SetArgs([]string{"INC1", "--db", dbPath})
		if err := cmd.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("err: %v", err)
		}
		md := buf.String()
		if !strings.Contains(md, "# Post-Mortem") || !strings.Contains(md, "rolled back the bad deploy") {
			t.Errorf("postmortem missing expected content:\n%s", md)
		}
	})
}
