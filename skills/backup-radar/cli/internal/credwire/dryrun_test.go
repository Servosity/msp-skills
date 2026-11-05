// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json retire-stdin-dry-run,
// dry-run-sends-nothing): the global --dry-run never sends a request, on any
// command. `backups retire --dry-run` used to set the vendor's body field
// dry_run, so a "dry run" reached the API; the vendor's own server-side preview
// is --server-dry-run now, a real (non-destructive) request.

package credwire

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"backup-radar-pp-cli/internal/cli"

	"github.com/spf13/cobra"
)

// dryRunKey is a held placeholder key: letters and dashes, low-entropy so
// secret scanners pass it, and no 8-character piece of it is program text.
const dryRunKey = "dryrunkey-qqqq-wwww-qqqq"

const ingestBody = `[{"client_name":"Acme","device_name":"srv-01","job_name":"Nightly","backups":[{"status":"success","time":"2026-07-07T13:06:00Z"}]}]`

// TestDryRunSendsNothing: every write command under the global --dry-run puts
// zero requests on the wire, to the fake or to any other host.
func TestDryRunSendsNothing(t *testing.T) {
	cases := []struct {
		name  string
		stdin string
		args  func(t *testing.T, e *env) []string
	}{
		{"backups retire", "", func(*testing.T, *env) []string {
			return []string{"backups", "retire", "--client-id", "cid-1042", "--reason", "decommissioned", "--dry-run"}
		}},
		{"backups retire --json", "", func(*testing.T, *env) []string {
			return []string{"backups", "retire", "--client-id", "cid-1042", "--reason", "decommissioned", "--dry-run", "--json"}
		}},
		{"backups retire --server-dry-run", "", func(*testing.T, *env) []string {
			return []string{"backups", "retire", "--client-id", "cid-1042", "--reason", "decommissioned", "--server-dry-run", "--dry-run"}
		}},
		{"backups retire --stdin", `{"client_id":"cid-1042","reason":"decommissioned"}`, func(*testing.T, *env) []string {
			return []string{"backups", "retire", "--stdin", "--dry-run"}
		}},
		{"backups ingest --body-json", "", func(*testing.T, *env) []string {
			return []string{"backups", "ingest", "--body-json", ingestBody, "--dry-run"}
		}},
		{"backups ingest --stdin", ingestBody, func(*testing.T, *env) []string {
			return []string{"backups", "ingest", "--stdin", "--dry-run"}
		}},
		{"import backups", "", func(t *testing.T, e *env) []string {
			in := writeFile(t, filepath.Join(e.home, "in", "rows.jsonl"), `{"client_name":"Acme","device_name":"srv-01","job_name":"Nightly","backups":[{"status":"success","time":"2026-07-07T13:06:00Z"}]}`+"\n")
			return []string{"import", "backups", "--input", in, "--dry-run"}
		}},
		{"push", "", func(t *testing.T, e *env) []string {
			in := writeFile(t, filepath.Join(e.home, "in", "rows.csv"), "client_name,device_name,job_name,time,status\nAcme,srv-01,Nightly,2026-07-07T13:06:00Z,success\n")
			return []string{"push", "--from", in, "--dry-run"}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t, nil, "BACKUP_RADAR_API_KEY="+dryRunKey)
			r := e.run(t, tc.stdin, tc.args(t, e)...)
			if got := e.api.all(); len(got) != 0 {
				t.Fatalf("--dry-run sent %d request(s) to the API, first %s %s body %s\nprinted:\n%s", len(got), got[0].Method, got[0].Path, got[0].Body, r.out)
			}
			if got := e.proxy.all(); len(got) != 0 {
				t.Fatalf("--dry-run sent %d request(s) to another host, first %s %s\nprinted:\n%s", len(got), got[0].Method, got[0].Host, r.out)
			}
		})
	}
}

// TestServerDryRunAsksTheVendor: the vendor's own preview stays reachable. It
// is a real request carrying body dry_run: true, and nothing else turns it on.
func TestServerDryRunAsksTheVendor(t *testing.T) {
	answer := func(w http.ResponseWriter, _ *http.Request, _ seen) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[],"total_count":0}`))
	}
	for name, tc := range map[string]struct {
		args []string
		want any
	}{
		"server preview": {[]string{"backups", "retire", "--client-id", "cid-1042", "--reason", "decommissioned", "--server-dry-run", "--json"}, true},
		"live retire":    {[]string{"backups", "retire", "--client-id", "cid-1042", "--reason", "decommissioned", "--json"}, nil},
	} {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t, answer, "BACKUP_RADAR_API_KEY="+dryRunKey)
			r := e.run(t, "", tc.args...)
			got := e.api.all()
			if len(got) != 1 || got[0].Method != http.MethodPost || !strings.HasSuffix(got[0].Path, "/backups/retire") {
				t.Fatalf("want one POST /backups/retire, got %+v\nprinted:\n%s", got, r.out)
			}
			var body map[string]any
			if err := json.Unmarshal([]byte(got[0].Body), &body); err != nil {
				t.Fatalf("body %q: %v", got[0].Body, err)
			}
			if body["dry_run"] != tc.want {
				t.Fatalf("body dry_run = %v, want %v (body %s)", body["dry_run"], tc.want, got[0].Body)
			}
		})
	}
}

// TestEveryWriteCommandIsCovered fails when the command tree gains a command
// that sends a write the dry-run cases above do not drive.
func TestEveryWriteCommandIsCovered(t *testing.T) {
	covered := map[string]bool{"backups ingest": true, "backups retire": true}
	var writes []string
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if m := c.Annotations["pp:method"]; m != "" && m != http.MethodGet {
			writes = append(writes, strings.TrimPrefix(c.CommandPath(), c.Root().Name()+" "))
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(cli.RootCmd())
	sort.Strings(writes)
	for _, w := range writes {
		if !covered[w] {
			t.Errorf("write command %q has no --dry-run case in TestDryRunSendsNothing", w)
		}
	}
	if len(writes) != len(covered) {
		t.Errorf("write commands %v, covered %v", writes, covered)
	}
}

// TestRetireHasNoLocalDryRunFlag: the retire command declares no flag that
// shadows the global --dry-run.
func TestRetireHasNoLocalDryRunFlag(t *testing.T) {
	root := cli.RootCmd()
	cmd, _, err := root.Find([]string{"backups", "retire"})
	if err != nil {
		t.Fatal(err)
	}
	if f := cmd.LocalNonPersistentFlags().Lookup("dry-run"); f != nil {
		t.Fatalf("backups retire declares its own --dry-run (%q), which shadows the global one", f.Usage)
	}
	if cmd.Flags().Lookup("server-dry-run") == nil {
		t.Fatal("backups retire lost --server-dry-run, the vendor's own preview")
	}
}
