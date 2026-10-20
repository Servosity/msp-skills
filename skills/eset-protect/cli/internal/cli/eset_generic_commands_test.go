// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"eset-protect-pp-cli/internal/cliutil/testenv"
	"eset-protect-pp-cli/internal/store"

	"github.com/spf13/cobra"
)

// TestGenericProjectManagementCommandsAreGone (adversarial round 5, P2): the
// generator's project-management commands gave confident wrong answers on
// ESET data. 'load' (workload per assignee) never read assigneeUuid and filed
// every synced row as unassigned; 'orphans' flagged every detection for a
// missing assignee/project/priority/labels ESET objects do not have; 'stale'
// knew none of ESET's timestamps (lastSyncTime, updateTime) and always found
// nothing. None is in the command tree; 'fleet stale' is.
func TestGenericProjectManagementCommandsAreGone(t *testing.T) {
	root := RootCmd()
	for _, c := range root.Commands() {
		switch c.Name() {
		case "load", "orphans", "stale":
			t.Errorf("the generic %q command is still registered: %s", c.Name(), c.Short)
		}
	}
	if c, _, err := root.Find([]string{"fleet", "stale"}); err != nil || c.Name() != "stale" || c.Parent().Name() != "fleet" {
		t.Fatalf("fleet stale is missing: %v", err)
	}
}

// exampleLines returns the command lines of a cobra Example block.
func exampleLines(example string) [][]string {
	var out [][]string
	quoted := regexp.MustCompile(`"[^"]*"|\S+`)
	for _, line := range strings.Split(example, "\n") {
		line = strings.TrimSpace(line)
		if i := strings.Index(line, "  #"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		if !strings.HasPrefix(line, esetCLIName+" ") {
			continue
		}
		var args []string
		for _, f := range quoted.FindAllString(strings.TrimPrefix(line, esetCLIName+" "), -1) {
			args = append(args, strings.Trim(f, `"`))
		}
		out = append(out, args)
	}
	return out
}

// TestHelpExamplesAreESETExamples: the analytics and which examples came from
// a chat API ('--type messages --group-by author_id', 'send message', 'stale
// tickets'). Every example now runs against an ESET mirror and answers
// something an MSP would ask.
func TestHelpExamplesAreESETExamples(t *testing.T) {
	testenv.Isolate(t)
	path := filepath.Join(t.TempDir(), "mirror.db")
	db, err := store.OpenWithContext(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.UpsertBatch("devices", []json.RawMessage{
		json.RawMessage(`{"uuid":"d1","displayName":"DC01","functionalityStatus":"DEVICE_FUNCTIONALITY_STATUS_OK"}`),
		json.RawMessage(`{"uuid":"d2","displayName":"KIOSK-03","functionalityStatus":"DEVICE_FUNCTIONALITY_STATUS_ATTENTION_REQUIRED"}`),
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.UpsertBatch("detections", []json.RawMessage{
		json.RawMessage(`{"uuid":"x1","severityLevel":"SEVERITY_LEVEL_HIGH"}`),
	}); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	root := RootCmd()
	run := func(args ...string) (string, error) {
		r := RootCmd()
		var out, errOut bytes.Buffer
		r.SetOut(&out)
		r.SetErr(&errOut)
		r.SetArgs(args)
		err := r.Execute()
		return out.String() + errOut.String(), err
	}
	for name, ex := range map[string]string{"analytics": mustFind(t, root, "analytics").Example, "which": mustFind(t, root, "which").Example} {
		lower := strings.ToLower(ex + mustFind(t, root, name).Long)
		for _, leak := range []string{"messages", "author_id", "channel_id", "send message", "tickets", "bottleneck"} {
			if strings.Contains(lower, leak) {
				t.Errorf("%s help still names %q from another API", name, leak)
			}
		}
	}
	want := map[string]string{
		"devices":                     `"count"`,
		"devices functionalityStatus": "DEVICE_FUNCTIONALITY_STATUS_ATTENTION_REQUIRED",
		"detections severityLevel":    "SEVERITY_LEVEL_HIGH",
		"stale devices":               "fleet stale",
		"file hash spread":            "detections spread",
		"isolate a machine":           "respond",
	}
	ran := 0
	for _, args := range append(exampleLines(mustFind(t, root, "analytics").Example), exampleLines(mustFind(t, root, "which").Example)...) {
		key := ""
		switch args[0] {
		case "analytics":
			for i, a := range args {
				if a == "--type" {
					key = args[i+1]
				}
				if a == "--group-by" {
					key += " " + args[i+1]
				}
			}
			args = append(args, "--db", path, "--json")
		case "which":
			if len(args) == 1 {
				continue
			}
			key = args[len(args)-1]
			args = append(args, "--json")
		}
		out, err := run(args...)
		if err != nil {
			t.Fatalf("example %v failed: %v\n%s", args, err, out)
		}
		if w, ok := want[key]; !ok || !strings.Contains(out, w) {
			t.Fatalf("example %v: output does not answer it (want %q):\n%s", args, w, out)
		}
		ran++
	}
	if ran != 6 {
		t.Fatalf("ran %d examples, want 6", ran)
	}
}

func mustFind(t *testing.T, root *cobra.Command, name string) *cobra.Command {
	t.Helper()
	c, _, err := root.Find([]string{name})
	if err != nil || c == root {
		t.Fatalf("no %s command: %v", name, err)
	}
	return c
}
