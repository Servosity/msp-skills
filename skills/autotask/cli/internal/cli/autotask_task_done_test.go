// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"autotask-pp-cli/internal/cliutil/testenv"
	"autotask-pp-cli/internal/store"
)

// Class completed-task-read-as-open: TaskModel (spec.json) documents no
// percentComplete; a task is done when its status is Complete (5) or it has a
// completedDateTime. Project 30 has three past-due tasks, two of them done.
func TestCompletedTasksAreNotOpenWork(t *testing.T) {
	testenv.Isolate(t)
	db := filepath.Join(t.TempDir(), "data.db")
	s, err := store.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	past := "2026-01-15T00:00:00Z"
	for id, js := range map[string]string{
		"31": `{"id":31,"projectID":30,"assignedResourceID":7,"status":1,"endDateTime":"` + past + `","title":"open"}`,
		"32": `{"id":32,"projectID":30,"assignedResourceID":7,"status":5,"endDateTime":"` + past + `","title":"done by status"}`,
		"33": `{"id":33,"projectID":30,"assignedResourceID":7,"status":8,"completedDateTime":"2026-01-10T00:00:00Z","endDateTime":"` + past + `","title":"done by date"}`,
	} {
		if err := s.Upsert("tasks", id, []byte(js)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Upsert("projects", "30", []byte(`{"id":30,"projectName":"M365","status":1,"endDateTime":"2027-01-01T00:00:00Z"}`)); err != nil {
		t.Fatal(err)
	}
	for _, rt := range []string{"tasks", "projects", "tickets"} {
		if _, err := s.DB().Exec(`INSERT OR REPLACE INTO sync_state (resource_type, last_attempt_complete, last_synced_at) VALUES (?, 1, '2026-10-29T00:00:00Z')`, rt); err != nil {
			t.Fatal(err)
		}
	}
	recordUnfilteredWalks(t, s)
	_ = s.Close()
	run := func(args ...string) string {
		cmd := RootCmd()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(append(args, "--json", "--db", db))
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out.String())
		}
		return out.String()
	}
	out := run("project-health")
	var ph []struct {
		OverdueTasks int `json:"overdueTasks"`
	}
	if err := json.Unmarshal([]byte(out[strings.Index(out, "["):]), &ph); err != nil || len(ph) != 1 || ph[0].OverdueTasks != 1 {
		t.Errorf("project-health: want project 30 with 1 overdue task, got %v:\n%s", err, out)
	}
	out = run("workload")
	if !strings.Contains(out, `"openTasks": 1`) && !strings.Contains(out, `"openTasks":1`) {
		t.Errorf("workload: want resource 7 with 1 open task:\n%s", out)
	}
}
