// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package keeperblackbox

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readOnlyCalls is one call of every tool the server annotates read-only, with a
// placeholder for each required argument, phased so each one finishes first.
func readOnlyCalls(t *testing.T, e *bbEnv) [][]map[string]any {
	t.Helper()
	out := e.mcpRaw(t, []map[string]any{{"jsonrpc": "2.0", "id": 77, "method": "tools/list", "params": map[string]any{}}})
	var phases [][]map[string]any
	for _, line := range strings.Split(out, "\n") {
		var msg struct {
			ID     int `json:"id"`
			Result struct {
				Tools []struct {
					Name        string `json:"name"`
					Annotations struct {
						ReadOnlyHint *bool `json:"readOnlyHint"`
					} `json:"annotations"`
					InputSchema struct {
						Required []string `json:"required"`
					} `json:"inputSchema"`
				} `json:"tools"`
			} `json:"result"`
		}
		if json.Unmarshal([]byte(line), &msg) != nil || msg.ID != 77 {
			continue
		}
		for _, tool := range msg.Result.Tools {
			if tool.Annotations.ReadOnlyHint == nil || !*tool.Annotations.ReadOnlyHint {
				continue
			}
			args := map[string]any{}
			for _, r := range tool.InputSchema.Required {
				args[r] = "4421"
			}
			phases = append(phases, []map[string]any{{"name": tool.Name, "arguments": args}})
		}
	}
	if len(phases) < 20 {
		t.Fatalf("only %d read-only tools listed:\n%.1500s", len(phases), out)
	}
	return phases
}

// homeTree hashes every file under the home except SQLite's scratch files, which a
// reader of a WAL database may create.
func homeTree(t *testing.T, home string) map[string]string {
	t.Helper()
	tree := map[string]string{}
	err := filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasSuffix(path, "-wal") || strings.HasSuffix(path, "-shm") {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		tree[path] = fmt.Sprintf("%x", sha256.Sum256(b))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

func treeChanges(before, after map[string]string) []string {
	var changed []string
	for p, sum := range after {
		if before[p] != sum {
			changed = append(changed, p)
		}
	}
	for p := range before {
		if _, ok := after[p]; !ok {
			changed = append(changed, "removed "+p)
		}
	}
	return changed
}

// Failing input (Codex r1): learnings_stats answers tools/call by pruning old learn
// events, and any store open creates or migrates a database on a fresh home. No tool
// annotated read-only may change anything under the home.
func TestMCPReadOnlyToolsWriteNothing(t *testing.T) {
	sv := echoServer(t)
	sv.honest.Store(true)
	seeded := newEnv(t, sv, true)
	seeded.run(t, "posture", "report", "--json")
	seedLearnEvent(t, seeded.home)
	phases := readOnlyCalls(t, seeded)
	wantEvents := learnEvents(t, seeded.home)
	if wantEvents == 0 {
		t.Fatal("seeding left no learn event")
	}
	before := homeTree(t, seeded.home)
	out := seeded.mcpPhases(t, phases...)
	if changed := treeChanges(before, homeTree(t, seeded.home)); len(changed) != 0 {
		t.Errorf("read-only tools changed the home: %q", changed)
	}
	if n := learnEvents(t, seeded.home); n != wantEvents {
		t.Errorf("learn events went from %d to %d", wantEvents, n)
	}
	for _, refusal := range []string{"readonly database", "read-only database", "attempt to write"} {
		if strings.Contains(out, refusal) {
			t.Errorf("a read-only tool tried to write (%q):\n%.3000s", refusal, out)
		}
	}

	fresh := newEnv(t, sv, true)
	before = homeTree(t, fresh.home)
	out = fresh.mcpPhases(t, phases...)
	if changed := treeChanges(before, homeTree(t, fresh.home)); len(changed) != 0 {
		t.Errorf("read-only tools wrote into a fresh home: %v", changed)
	}
	for _, failure := range []string{"readonly database", "an empty store", "opening database"} {
		if strings.Contains(out, failure) {
			t.Errorf("a read-only tool failed on a fresh home (%q):\n%.3000s", failure, out)
		}
	}

	// The one writer: its preview writes nothing, and its confirmed run is saved.
	posture := func(args map[string]any) []map[string]any {
		return []map[string]any{{"name": "posture_report", "arguments": args}}
	}
	fresh.mcpCalls(t, posture(map[string]any{}))
	if changed := treeChanges(before, homeTree(t, fresh.home)); len(changed) != 0 {
		t.Errorf("the posture_report preview wrote: %v", changed)
	}
	fresh.mcpCalls(t, posture(map[string]any{"confirm": true}))
	if out := fresh.run(t, "posture", "runs", "--json"); !strings.Contains(out, `"run_id"`) && !strings.Contains(out, `"id"`) {
		t.Errorf("a confirmed posture_report saved no run:\n%.1500s", out)
	}
}

// seedLearnEvent adds a learn event far older than the retention window.
func seedLearnEvent(t *testing.T, home string) {
	t.Helper()
	seeded := 0
	_ = filepath.WalkDir(home, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".db") {
			return nil
		}
		db, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		if _, err := db.Exec(`INSERT INTO learn_events (ts, event, surface) VALUES ('2001-01-01T00:00:00Z', 'teach', 'cli')`); err == nil {
			seeded++
		}
		// The whole store in the main file, so the hash sees any later write.
		_, _ = db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)
		return nil
	})
	if seeded == 0 {
		t.Fatal("no store to seed under the home")
	}
}
