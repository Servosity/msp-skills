// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"acronis-pp-cli/internal/cliutil"
	"acronis-pp-cli/internal/cliutil/testenv"
	"acronis-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

func TestAcronisPartitionByEndpoint(t *testing.T) {
	for _, c := range []struct{ label, path, want string }{
		{"task-manager", "/api/task_manager/v2/tasks", "task-manager"},
		{"task-manager", "/api/task_manager/v2/tasks/t1", "task-manager"},
		{"task-manager", "/api/task_manager/v2/activities", "task-manager-v2-activities"},
		{"task-manager", "/api/task_manager/v2/activities/a1", "task-manager-v2-activities"},
		{"agent-manager", "/api/agent_manager/v2/agents/ag1", "agent-manager"},
		{"agent-manager", "/api/agent_manager/v2/hardware_nodes", ""},
		{"agent-manager", "/api/agent_manager/v2/hardware_nodes/n1", ""},
		{"agent-manager", "/api/agent_manager/v2/agents/update:force", ""},
		{"agent-manager", "/api/agent_manager/v2/agent_update_settings", ""},
		{"offering-items", "/api/2/tenants/T/offering_items", "offering_items"},
		{"usages", "/api/2/tenants/T/usages", "usages"},
		{"tenants", "/api/2/tenants/T", "tenants"},
		{"clients", "/api/2/clients", "clients"},
		{"reports", "/api/2/reports", "reports"},
		{"remote_search", "/api/2/search", "remote_search"},
	} {
		if got := acronisPartition(c.label, c.path); got != c.want {
			t.Errorf("acronisPartition(%q, %q) = %q, want %q", c.label, c.path, got, c.want)
		}
	}
}

// TestAcronisReadsAndWritesStayInTheirEndpointPartition drives the detail
// reads Codex named (an activity, Acronis Task Manager API Activity schema; a
// hardware node) and a mutation answer through the real commands and the
// write-through, then reads the tables the rollups read: nothing lands in the
// tasks or agents partition, and a hardware-node read is not answered from
// the agents mirror.
func TestAcronisReadsAndWritesStayInTheirEndpointPartition(t *testing.T) {
	home := testenv.Isolate(t, cliutil.DataDir)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/task_manager/v2/activities/a1":
			fmt.Fprint(w, `{"id":"a1","task_id":"t1","type":"backup","state":"completed","result_code":"error","completedAt":"2026-10-27T14:00:00Z"}`)
		case "/api/agent_manager/v2/hardware_nodes/n1":
			fmt.Fprint(w, `{"id":"n1","hostname":"node-host","tenant":{"uuid":"child"}}`)
		default:
			t.Error("unexpected request", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	t.Setenv("ACRONIS_BASE_URL", srv.URL)
	t.Setenv("ACRONIS_BEARER_AUTH", "fixture")

	var out bytes.Buffer
	run := func(newCmd func(*rootFlags) *cobra.Command, dataSource string, args ...string) error {
		flags := &rootFlags{configPath: filepath.Join(home, "config.toml"), dataSource: dataSource, asJSON: true}
		cmd := newCmd(flags)
		out.Reset()
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(args)
		return cmd.Execute()
	}
	if err := run(newTaskManagerCmd, "auto", "get-activity", "a1"); err != nil {
		t.Fatal(err)
	}
	if err := run(newAgentManagerCmd, "auto", "get-hardware-node", "n1"); err != nil {
		t.Fatal(err)
	}
	writeMutationResponseToStore(context.Background(), "agent-manager", "/api/agent_manager/v2/agents/update:force", json.RawMessage(`{"id":"force-1","hostname":"forced"}`), "")
	writeMutationResponseToStore(context.Background(), "agent-manager", "/api/agent_manager/v2/agent_update_settings", json.RawMessage(`{"id":"settings-1","hostname":"settings"}`), "")

	db, err := store.OpenWithContext(context.Background(), defaultDBPath("acronis-cli"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	count := func(q string, args ...any) int {
		var n int
		if err := db.DB().QueryRow(q, args...).Scan(&n); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		return n
	}
	if n := count(`SELECT COUNT(*) FROM task_manager WHERE id = 'a1'`); n != 0 {
		t.Errorf("the activity landed in the tasks partition (%d rows); failures and health would count it as a task", n)
	}
	if n := count(`SELECT COUNT(*) FROM agent_manager WHERE id IN ('n1', 'force-1', 'settings-1')`); n != 0 {
		t.Errorf("%d non-agent rows landed in the agents partition", n)
	}
	if n := count(`SELECT COUNT(*) FROM resources WHERE resource_type = 'task-manager-v2-activities' AND id = 'a1'`); n != 1 {
		t.Errorf("the activity is not in the activities partition (%d rows)", n)
	}
	if err := db.UpsertAgentManager([]byte(`{"id":"real-agent","hostname":"agent-host","tenant":{"uuid":"child"}}`)); err != nil {
		t.Fatal(err)
	}
	err = run(newAgentManagerCmd, "local", "get-hardware-node", "real-agent")
	if err == nil || bytes.Contains(out.Bytes(), []byte("agent-host")) {
		t.Errorf("a local hardware-node read was answered from the agents partition (err=%v): %s", err, out.String())
	}
}
