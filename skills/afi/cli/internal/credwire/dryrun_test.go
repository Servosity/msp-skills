// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated; handfixes.json dry-run-sends-nothing): the
// global --dry-run never sends a request, on any write command, every way it
// takes its input. Moved here from the retired black-box package, which held
// the print's own credential layer tests too.

package credwire

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"afi-pp-cli/internal/cli"

	"github.com/spf13/cobra"
)

// dryRunKey is a held placeholder key: letters and dashes, low-entropy so
// secret scanners pass it, and no 8-character piece of it is program text.
const dryRunKey = "dryrunkey-qqqq-wwww-qqqq"

// Fixture ids the fake answers for.
const (
	fxTenant   = "tnt-1042"
	fxResource = "res-1042"
	fxPolicy   = "pol-1042"
	fxJob      = "job-1042"
	fxTask     = "tsk-1042"
	fxArchive  = "arc-1042"
)

const orgJSON = `{"name":"HQ-Org","external_id":"ext-import","admin_email":"admin@corp.example"}`

// afiFake answers the write endpoints and what offboard reads: one protection
// for fxResource, a finished task, and a fresh archive.
func afiFake(w http.ResponseWriter, r *http.Request, _ seen) {
	w.Header().Set("Content-Type", "application/json")
	base := "/api/v1/tenants/" + fxTenant
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/orgs":
		_, _ = w.Write([]byte(`{"org":{"id":"org-new-1042","name":"HQ-Org"}}`))
	case r.URL.Path == base+"/protections":
		_, _ = fmt.Fprintf(w, `{"items":[{"id":"prot-1042","resource_id":%q,"policy_id":%q,"job_id":%q}],"next_page_token":""}`, fxResource, fxPolicy, fxJob)
	case r.Method == http.MethodPut && r.URL.Path == base+"/jobs/"+fxJob+"/trigger":
		_, _ = fmt.Fprintf(w, `{"task_id":%q}`, fxTask)
	case r.URL.Path == base+"/tasks/"+fxTask:
		_, _ = fmt.Fprintf(w, `{"id":%q,"resource_id":%q,"status":"done"}`, fxTask, fxResource)
	case r.URL.Path == base+"/archives":
		_, _ = fmt.Fprintf(w, `{"items":[{"id":%q,"tenant_id":%q,"resource_id":%q,"delete_pending":false,"created_at":"2099-01-01T00:00:00Z"}],"next_page_token":""}`, fxArchive, fxTenant, fxResource)
	default:
		_, _ = w.Write([]byte(`{}`))
	}
}

// writeCommands is every command that can send a write to Afi: the write
// endpoints and the hand-written senders (handWrittenWriters).
// TestEveryWriteCommandIsCovered keeps it complete.
var writeCommands = []string{
	"orgs create",
	"tenants archives delete",
	"tenants jobs tasks-trigger",
	"tenants resources protections-protect",
	"tenants resources protections-unprotect",
	"offboard",
	"import",
}

// handWrittenWriters maps each internal/cli file that sends a write outside a
// generated endpoint command to the command it implements.
var handWrittenWriters = map[string]string{
	"offboard.go": "offboard",
	"import.go":   "import",
}

// TestDryRunSendsNothing: every write command under the global --dry-run puts
// zero requests on the wire, to the fake or to any other host, and prints its
// preview (so the run got past flag parsing). The control shows the same
// writes without --dry-run reach the fake.
func TestDryRunSendsNothing(t *testing.T) {
	cases := []struct {
		cmd, stdin string
		args       func(t *testing.T, e *env) []string
	}{
		{"orgs create", "", func(*testing.T, *env) []string {
			return []string{"orgs", "create", "--name", "HQ-Org", "--admin-email", "admin@corp.example"}
		}},
		{"orgs create", orgJSON, func(*testing.T, *env) []string { return []string{"orgs", "create", "--stdin"} }},
		{"tenants archives delete", "", func(*testing.T, *env) []string {
			return []string{"tenants", "archives", "delete", fxTenant, fxArchive, "--reason", "retired"}
		}},
		{"tenants jobs tasks-trigger", "", func(*testing.T, *env) []string {
			return []string{"tenants", "jobs", "tasks-trigger", fxTenant, fxJob}
		}},
		{"tenants jobs tasks-trigger", `{"note":"final"}`, func(*testing.T, *env) []string {
			return []string{"tenants", "jobs", "tasks-trigger", fxTenant, fxJob, "--stdin"}
		}},
		{"tenants resources protections-protect", "", func(*testing.T, *env) []string {
			return []string{"tenants", "resources", "protections-protect", fxTenant, fxResource, "--policy-id", fxPolicy}
		}},
		{"tenants resources protections-protect", `{"note":"protect"}`, func(*testing.T, *env) []string {
			return []string{"tenants", "resources", "protections-protect", fxTenant, fxResource, "--policy-id", fxPolicy, "--stdin"}
		}},
		{"tenants resources protections-unprotect", "", func(*testing.T, *env) []string {
			return []string{"tenants", "resources", "protections-unprotect", fxTenant, fxResource, "--policy-id", fxPolicy}
		}},
		{"offboard", "", func(*testing.T, *env) []string {
			return []string{"offboard", fxResource, "--tenant", fxTenant, "--policy", fxPolicy, "--reason", "departure"}
		}},
		{"offboard", "", func(*testing.T, *env) []string { return []string{"offboard", fxResource} }},
		{"import", "", func(t *testing.T, e *env) []string {
			in := writeFile(t, filepath.Join(e.home, "in", "orgs.jsonl"), orgJSON+"\n")
			return []string{"import", "orgs", "--input", in}
		}},
		{"import", orgJSON + "\n", func(*testing.T, *env) []string { return []string{"import", "orgs", "--input", "-"} }},
	}
	ran := map[string]bool{}
	for _, tc := range cases {
		e := newEnv(t, afiFake, "AFI_API_KEY="+dryRunKey)
		args := append(tc.args(t, e), "--dry-run")
		r := e.run(t, tc.stdin, args...)
		// A preview that repeats a --stdin body is withheld: the kit holds the
		// body (StdinFlags), so the notice stands in for the preview.
		if !strings.Contains(r.out, "dry run") && !strings.Contains(r.out, "would run") && !(tc.stdin != "" && strings.Contains(r.out, "withheld")) {
			t.Errorf("%v printed no preview:\n%s", args, r.out)
		}
		if got := e.api.all(); len(got) != 0 {
			t.Errorf("%v sent %d request(s) to the API, first %s %s\nprinted:\n%s", args, len(got), got[0].Method, got[0].Path, r.out)
		}
		if got := e.proxy.all(); len(got) != 0 {
			t.Errorf("%v sent %d request(s) to another host, first %s %s", args, len(got), got[0].Method, got[0].Host)
		}
		ran[tc.cmd] = true
	}
	for _, c := range writeCommands {
		if !ran[c] {
			t.Errorf("the dry-run cases never ran %s", c)
		}
	}

	// Control: the same writes without --dry-run reach the fake.
	e := newEnv(t, afiFake, "AFI_API_KEY="+dryRunKey)
	e.run(t, orgJSON, "orgs", "create", "--stdin")
	e.run(t, "", "tenants", "archives", "delete", fxTenant, fxArchive, "--reason", "retired")
	e.run(t, "", "offboard", fxResource, "--tenant", fxTenant, "--poll-interval", "10ms")
	base := "/api/v1/tenants/" + fxTenant
	want := []string{
		"POST /api/v1/orgs",
		"DELETE " + base + "/archives/" + fxArchive,
		"GET " + base + "/protections",
		"PUT " + base + "/jobs/" + fxJob + "/trigger",
		"GET " + base + "/tasks/" + fxTask,
		"GET " + base + "/archives",
		"DELETE " + base + "/resources/" + fxResource + "/protect",
	}
	var got []string
	for _, s := range e.api.all() {
		got = append(got, s.Method+" "+s.Path)
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("control: want %v, got %v", want, got)
	}
}

// TestEveryWriteCommandIsCovered fails when the command tree gains a write
// endpoint, or internal/cli gains a hand-written sender, the dry-run cases
// above do not drive.
func TestEveryWriteCommandIsCovered(t *testing.T) {
	listed := map[string]bool{}
	for _, c := range writeCommands {
		listed[c] = true
	}
	covered := map[string]bool{}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if m := c.Annotations["pp:method"]; m != "" && m != http.MethodGet {
			path := strings.TrimPrefix(c.CommandPath(), c.Root().Name()+" ")
			covered[path] = true
			if !listed[path] {
				t.Errorf("write command %q has no --dry-run case in TestDryRunSendsNothing", path)
			}
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(cli.RootCmd())

	clientWrite := regexp.MustCompile(`\.(Post|Put|Patch|Delete)(WithParams|WithHeaders|WithParamsAndHeaders|QueryWithParams|QueryWithParamsAndHeaders|WithBody|WithParamsAndBody|WithBodyAndHeaders|WithParamsAndBodyAndHeaders)?\(|\.GetMutating(WithHeaders)?\(`)
	endpointFile := regexp.MustCompile(`"pp:method": "(POST|PUT|PATCH|DELETE)"`)
	src := filepath.Join("..", "cli")
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	senders := map[string]bool{}
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".go") || strings.HasSuffix(ent.Name(), "_test.go") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(src, ent.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if !clientWrite.Match(b) || endpointFile.Match(b) {
			continue
		}
		senders[ent.Name()] = true
		c, ok := handWrittenWriters[ent.Name()]
		if !ok {
			t.Errorf("internal/cli/%s sends a write outside an endpoint command and is not in handWrittenWriters", ent.Name())
			continue
		}
		covered[c] = true
	}
	for f := range handWrittenWriters {
		if !senders[f] {
			t.Errorf("handWrittenWriters lists internal/cli/%s, which no longer sends a write", f)
		}
	}
	var extra []string
	for _, c := range writeCommands {
		if !covered[c] {
			extra = append(extra, c)
		}
	}
	sort.Strings(extra)
	if len(extra) > 0 {
		t.Errorf("writeCommands lists %v, which are neither write endpoints nor hand-written senders", extra)
	}
	if len(covered) < 7 {
		t.Fatalf("found %d write commands; the walk is not reading the tree", len(covered))
	}
}
