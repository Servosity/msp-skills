// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"abnormal-pp-cli/internal/store"
)

// The threats list returns only {threatId}. 4.34.x sync stores each message of
// GET /threats/{threat_id} as its own threats_get row (keyed
// threatId+abxMessageIdStr); 4.24 stored whole detail docs as threats_threats.
// triage must rank one doc per threat with every message of it, once.
func TestTriageRanksTheSyncedThreatDetailOncePerThreat(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	recv := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	msg := func(threat, id, status string) string {
		return `{"threatId":"` + threat + `","abxMessageIdStr":"` + id + `","subject":"Pay now","attackType":"Extortion","receivedTime":"` + recv + `","remediationStatus":"` + status + `"}`
	}
	upsert := func(rt, id, data string) {
		t.Helper()
		if err := db.Upsert(rt, id, json.RawMessage(data)); err != nil {
			t.Fatal(err)
		}
	}
	upsert("threats", "t-1", `{"threatId":"t-1"}`)
	upsert("threats_get", "t-1+m-1", msg("t-1", "m-1", "Not Remediated"))
	upsert("threats_get", "t-1+m-2", msg("t-1", "m-2", "Auto-Remediated"))
	upsert("threats", "t-2", `{"threatId":"t-2"}`)
	upsert("threats_threats", "t-2", `{"threatId":"t-2","recipientCount":4,"messages":[`+msg("t-2", "m-3", "Not Remediated")+`,`+msg("t-2", "m-3", "Not Remediated")+`]}`)
	db.Close()

	cmd := newNovelTriageCmd(&rootFlags{asJSON: true})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--db", dbPath, "--since", "24h"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var view triageView
	if err := json.Unmarshal(out.Bytes(), &view); err != nil {
		t.Fatalf("%v: %s", err, out.String())
	}
	if view.ScannedThreats != 2 || len(view.Items) != 2 {
		t.Fatalf("want two threats, got scanned=%d items=%d: %s", view.ScannedThreats, len(view.Items), out.String())
	}
	got := map[string]triageItem{}
	for _, it := range view.Items {
		got[it.ThreatID] = it
	}
	if it := got["t-1"]; it.AttackType != "Extortion" || it.MessageCount != 2 || it.Remediated {
		t.Errorf("t-1 must carry both synced messages, one unremediated: %+v", it)
	}
	if it := got["t-2"]; it.AttackType != "Extortion" || it.MessageCount != 1 || it.RecipientCount != 4 {
		t.Errorf("t-2 (4.24 detail doc) must count its message once: %+v", it)
	}
}

// Codex r2 class incomplete-threat-detail-read-as-complete: GET
// /threats/{threat_id} accepts pageSize (sync --global-param pageSize=1 stores
// one message) and recipientCount counts recipients, not messages, so no count
// of stored messages proves a campaign complete. triage must never hide a
// threat on stored-message evidence: remediated-looking and out-of-window
// threats stay listed, after every in-window unremediated one.
func TestTriageNeverHidesAThreatOnStoredMessages(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "data.db")
	db, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	recent := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)
	old := time.Now().UTC().Add(-30 * 24 * time.Hour).Format(time.RFC3339)
	msg := func(threat, id, recv, status string) string {
		return `{"threatId":"` + threat + `","abxMessageIdStr":"` + id + `","subject":"Pay now","attackType":"Extortion","receivedTime":"` + recv + `","remediationStatus":"` + status + `"}`
	}
	upsert := func(rt, id, data string) {
		t.Helper()
		if err := db.Upsert(rt, id, json.RawMessage(data)); err != nil {
			t.Fatal(err)
		}
	}
	// t-page1: synced with pageSize=1, its one stored message remediated.
	upsert("threats_get", "t-page1+m-1", msg("t-page1", "m-1", recent, "Auto-Remediated"))
	// t-legacy: a 4.24 detail doc, ten remediated messages, recipientCount 1.
	var ten []string
	for i := 0; i < 10; i++ {
		ten = append(ten, msg("t-legacy", "l-"+string(rune('a'+i)), recent, "Auto-Remediated"))
	}
	upsert("threats_threats", "t-legacy", `{"threatId":"t-legacy","recipientCount":1,"messages":[`+strings.Join(ten, ",")+`]}`)
	// t-old: unremediated, newest stored message outside --since.
	upsert("threats_get", "t-old+m-1", msg("t-old", "m-1", old, "Not Remediated"))
	// t-open: unremediated and recent: the top of the queue.
	upsert("threats_get", "t-open+m-1", msg("t-open", "m-1", recent, "Not Remediated"))
	db.Close()

	cmd := newNovelTriageCmd(&rootFlags{asJSON: true})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--db", dbPath, "--since", "24h"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var view struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(out.Bytes(), &view); err != nil {
		t.Fatalf("%v: %s", err, out.String())
	}
	if len(view.Items) != 4 {
		t.Fatalf("every scanned threat must be listed, got %d items: %s", len(view.Items), out.String())
	}
	if view.Items[0]["threatId"] != "t-open" {
		t.Fatalf("the in-window unremediated threat must rank first: %s", out.String())
	}
	pos := map[string]int{}
	for i, it := range view.Items {
		pos[it["threatId"].(string)] = i
	}
	for _, id := range []string{"t-page1", "t-legacy"} {
		if it := view.Items[pos[id]]; it["remediated"] != true || it["in_window"] != true {
			t.Errorf("%s: want remediated=true in_window=true, got %v", id, it)
		}
	}
	if it := view.Items[pos["t-old"]]; it["in_window"] != false || it["remediated"] != false {
		t.Errorf("t-old: want in_window=false remediated=false, got %v", it)
	}
	if pos["t-old"] != 3 {
		t.Errorf("an out-of-window threat ranks after every in-window one: %s", out.String())
	}
}
