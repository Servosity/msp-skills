// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"sentinelone-pp-cli/internal/store"
)

// newSeededStore creates a temp store, runs migrations, and upserts the given
// agents and threats into the resources table. Returns the db path.
func newSeededStore(t *testing.T, agents, threats []map[string]any) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "s1.db")
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	upsertAll(t, db, "agents", agents)
	upsertAll(t, db, "threats", threats)
	return dbPath
}

func upsertAll(t *testing.T, db *store.Store, kind string, objs []map[string]any) {
	t.Helper()
	for i, o := range objs {
		raw, err := json.Marshal(o)
		if err != nil {
			t.Fatalf("marshal %s[%d]: %v", kind, i, err)
		}
		id, _ := o["id"].(string)
		if id == "" {
			id = kind + "-" + itoa(i)
		}
		if err := db.Upsert(kind, id, raw); err != nil {
			t.Fatalf("upsert %s %s: %v", kind, id, err)
		}
	}
}

// seedResource upserts arbitrary objects under a resource type (ranger/rogues).
func seedResource(t *testing.T, dbPath, kind string, objs []map[string]any) {
	t.Helper()
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	upsertAll(t, db, kind, objs)
}

// seedSnapshot inserts a history snapshot for an entity at an explicit time.
func seedSnapshot(t *testing.T, dbPath, entity string, at time.Time, objs []map[string]any) {
	t.Helper()
	db, err := store.OpenWithContext(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	stamp := at.UTC().Format(time.RFC3339)
	for i, o := range objs {
		raw, err := json.Marshal(o)
		if err != nil {
			t.Fatalf("marshal snapshot[%d]: %v", i, err)
		}
		id, _ := o["id"].(string)
		if id == "" {
			id = entity + "-" + itoa(i)
		}
		if _, err := db.DB().Exec(
			`INSERT INTO fleet_snapshots (captured_at, entity, entity_id, data) VALUES (?, ?, ?, ?)`,
			stamp, entity, id, string(raw)); err != nil {
			t.Fatalf("insert snapshot: %v", err)
		}
	}
}

// runJSON executes a command with --json and parses its stdout as a JSON object.
func runJSON(t *testing.T, cmd *cobra.Command, args ...string) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	var errBuf bytes.Buffer // sync hints write to stderr; keep them out of parsed stdout
	cmd.SetOut(&buf)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute %s %v: %v\noutput:\n%s", cmd.Name(), args, err, buf.String())
	}
	var out map[string]any
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("output is not a JSON object: %v\noutput:\n%s", err, buf.String())
	}
	return out
}

func jsonFlags() *rootFlags { return &rootFlags{asJSON: true} }

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var b [20]byte
	p := len(b)
	for i > 0 {
		p--
		b[p] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		p--
		b[p] = '-'
	}
	return string(b[p:])
}

// --- fixtures ---

// agentFix builds an agent object with sensible defaults; override via opts.
func agentFix(id, name, site, version string, opts map[string]any) map[string]any {
	a := map[string]any{
		"id":               id,
		"computerName":     name,
		"siteName":         site,
		"agentVersion":     version,
		"networkStatus":    "connected",
		"isUpToDate":       true,
		"infected":         false,
		"isDecommissioned": false,
		"mitigationMode":   "protect",
		"lastActiveDate":   time.Now().UTC().Format(time.RFC3339),
		"scanFinishedAt":   time.Now().UTC().Format(time.RFC3339),
	}
	for k, v := range opts {
		a[k] = v
	}
	return a
}

// threatFix builds a threat object with nested threatInfo/agentRealtimeInfo.
func threatFix(id, name, sha1, endpoint, site string, info map[string]any) map[string]any {
	ti := map[string]any{
		"threatName":       name,
		"sha1":             sha1,
		"analystVerdict":   "undefined",
		"confidenceLevel":  "suspicious",
		"incidentStatus":   "unresolved",
		"mitigationStatus": "active",
		"createdAt":        time.Now().UTC().Format(time.RFC3339),
		"updatedAt":        time.Now().UTC().Format(time.RFC3339),
	}
	for k, v := range info {
		ti[k] = v
	}
	return map[string]any{
		"id":         id,
		"threatInfo": ti,
		"agentRealtimeInfo": map[string]any{
			"agentComputerName": endpoint,
			"siteName":          site,
		},
	}
}
