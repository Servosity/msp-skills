// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pins the CLI write-confirmation gate
// (handfixes.json "cli-writes-require-yes"). Every command that changes the
// HubSpot portal must refuse to run, before it sends anything, unless the
// caller passed --yes (send it) or --dry-run (preview it). The tests drive the
// real Cobra tree against a fake HubSpot API and read the API's own request
// log, so a gate that only changes an error message cannot pass them.

package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"
	"hubspot-pp-cli/internal/cliutil/testenv"
)

// gateReadPOSTPaths is this test's own list of the POST endpoints that only
// read. It is deliberately independent of the code's allowlist: if the code
// ever exempts another write path, the tree walk below sees that command send
// without --yes and fails.
var gateReadPOSTPaths = func() map[string]bool {
	m := map[string]bool{
		"/crm/v3/properties/{objectType}/batch/read":                      true,
		"/crm/v4/associations/{fromObjectType}/{toObjectType}/batch/read": true,
		"/crm/v3/lists/search":                                            true,
		"/crm/v3/lists/records/memberships/batch/read":                    true,
	}
	for _, o := range []string{"{objectType}", "calls", "companies", "contacts", "0-3", "emails", "leads", "line_items", "meetings", "notes", "products", "quotes", "tasks", "tickets"} {
		m["/crm/v3/objects/"+o+"/batch/read"] = true
		m["/crm/v3/objects/"+o+"/search"] = true
	}
	return m
}()

// runCLI runs the real Cobra tree and returns stdout, stderr and the error.
func runCLI(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := RootCmd()
	cmd.SetArgs(args)
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	err := cmd.Execute()
	return out.String(), errb.String(), err
}

// TestImportTargetsAreCreateEndpoints: every resource import accepts maps to
// that object's single-record create endpoint, never to a batch archive,
// update or other operation. The map is checked against the command tree's own
// pp:path annotations, so a reprint that picks another POST fails here.
func TestImportTargetsAreCreateEndpoints(t *testing.T) {
	creates := map[string]bool{}
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		for _, child := range cmd.Commands() {
			walk(child)
		}
		if strings.ToUpper(cmd.Annotations["pp:method"]) == "POST" && strings.HasPrefix(cmd.Annotations["pp:path"], "/crm/v3/objects/") &&
			strings.Count(cmd.Annotations["pp:path"], "/") == 4 && !strings.Contains(cmd.Annotations["pp:path"], "{") {
			creates[cmd.Annotations["pp:path"]] = true
		}
	}
	walk(RootCmd())
	if len(creates) < 10 {
		t.Fatalf("found %d create endpoints; the walk is broken", len(creates))
	}
	for resource, path := range resourceWritePaths {
		if !creates[path] {
			t.Errorf("import %s posts to %s, which is not a single-record create endpoint", resource, path)
		}
	}
	for _, r := range []string{"hubspot-lists-crm", "hubspot-imports-crm"} {
		if _, err := resourceWritePath(r); err == nil {
			t.Errorf("import %s must be refused: it has no JSON create-one-record endpoint", r)
		}
	}
}

// gateRecorder is a fake HubSpot API that records every request it answers.
type gateRecorder struct {
	mu   sync.Mutex
	reqs []string
}

func (g *gateRecorder) requests() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.reqs...)
}

func (g *gateRecorder) reset() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.reqs = nil
}

func startGateServer(t *testing.T) *gateRecorder {
	t.Helper()
	testenv.Isolate(t)
	rec := &gateRecorder{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.mu.Lock()
		rec.reqs = append(rec.reqs, r.Method+" "+r.URL.Path)
		rec.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("HUBSPOT_BASE_URL", srv.URL)
	t.Setenv("HUBSPOT_ACCESS_TOKEN", "pat-na1-"+"7f3c9e2a-b14d-4c8e-9a6f-2d5e8b1c7a31")
	t.Setenv("HUBSPOT_NO_LEARN", "1")
	return rec
}

// withStdin points os.Stdin at a file holding body for the duration of fn.
func withStdin(t *testing.T, body string, fn func()) {
	t.Helper()
	f := filepath.Join(t.TempDir(), "stdin.json")
	if err := os.WriteFile(f, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	in, err := os.Open(f)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	prev := os.Stdin
	os.Stdin = in
	defer func() { os.Stdin = prev }()
	fn()
}

// gateWriteCommand is one generated endpoint command whose method changes the
// portal, with an argument list that is enough to send the request.
type gateWriteCommand struct {
	label    string
	argv     []string
	method   string
	apiPath  string
	useStdin bool
}

var gatePlaceholder = regexp.MustCompile(`<([^>]+)>`)

// collectGateWriteCommands walks the whole command tree. Every command with a
// pp:endpoint annotation is a write unless its method is a read method or it
// is a POST on a path this test knows is a read; a missing or unknown method
// counts as a write.
func collectGateWriteCommands(t *testing.T) []gateWriteCommand {
	t.Helper()
	var out []gateWriteCommand
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		for _, child := range cmd.Commands() {
			walk(child)
		}
		if strings.TrimSpace(cmd.Annotations["pp:endpoint"]) == "" {
			return
		}
		method := strings.ToUpper(strings.TrimSpace(cmd.Annotations["pp:method"]))
		apiPath := strings.TrimSpace(cmd.Annotations["pp:path"])
		switch method {
		case "GET", "HEAD", "OPTIONS":
			return
		}
		if method == "POST" && gateReadPOSTPaths[apiPath] {
			return
		}
		out = append(out, gateWriteCommandFor(cmd, method, apiPath))
	}
	walk(RootCmd())
	sort.Slice(out, func(i, j int) bool { return out[i].label < out[j].label })
	return out
}

func gateWriteCommandFor(cmd *cobra.Command, method, apiPath string) gateWriteCommand {
	path := strings.Fields(cmd.CommandPath())[1:]
	argv := append([]string{}, path...)
	happy := map[string]string{}
	var happyFlags []string
	for _, part := range strings.Split(cmd.Annotations["pp:happy-args"], ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.HasPrefix(part, "--") {
			happyFlags = append(happyFlags, part)
			continue
		}
		if k, v, ok := strings.Cut(part, "="); ok {
			happy[k] = v
		}
	}
	for _, m := range gatePlaceholder.FindAllStringSubmatch(cmd.Use, -1) {
		if v, ok := happy[m[1]]; ok {
			argv = append(argv, v)
		} else {
			argv = append(argv, "1")
		}
	}
	useStdin := cmd.Flags().Lookup("stdin") != nil
	if useStdin {
		argv = append(argv, "--stdin")
	} else {
		argv = append(argv, happyFlags...)
	}
	return gateWriteCommand{label: strings.Join(path, " "), argv: argv, method: method, apiPath: apiPath, useStdin: useStdin}
}

func runGateCommand(t *testing.T, wc gateWriteCommand, extra ...string) (string, string, error) {
	t.Helper()
	var stdout, stderr string
	var err error
	for _, body := range []string{`{}`, `[]`} {
		withStdin(t, body, func() {
			stdout, stderr, err = runCLI(t, append(append([]string{}, wc.argv...), extra...)...)
		})
		// A few endpoints take a JSON array body; the refusal path never
		// reads stdin, so only a confirmed run can reach this parse error.
		if err == nil || !strings.Contains(err.Error(), "cannot unmarshal object into Go value of type []") {
			break
		}
	}
	return stdout, stderr, err
}

func assertConfirmationRefusal(t *testing.T, label string, err error) {
	t.Helper()
	if err == nil {
		t.Errorf("%s: ran without --yes or --dry-run; want a refusal", label)
		return
	}
	if code := ExitCode(err); code != 2 {
		t.Errorf("%s: refusal exit code = %d, want 2 (usage error): %v", label, code, err)
	}
	if msg := err.Error(); !strings.Contains(msg, "--yes") || !strings.Contains(msg, "--dry-run") {
		t.Errorf("%s: refusal must name both --yes and --dry-run, got %q", label, msg)
	}
}

// TestEveryWriteCommandRefusesWithoutConfirmation is the tree walk: every
// generated endpoint command with a write method, present or future, must
// refuse without --yes or --dry-run and put nothing on the wire, in human and
// in agent mode. The same argument list is then shown to be enough to send
// (with --yes), so a refusal here is the gate and not a missing argument.
func TestEveryWriteCommandRefusesWithoutConfirmation(t *testing.T) {
	rec := startGateServer(t)
	cmds := collectGateWriteCommands(t)
	found := false
	for _, wc := range cmds {
		if wc.label == "hubspot-contacts-crm delete-v3-objects-contacts-contact-id" {
			found = true
		}
	}
	if len(cmds) < 20 || !found {
		t.Fatalf("tree walk found %d write commands (contact delete present: %v); the walk is broken", len(cmds), found)
	}
	for _, wc := range cmds {
		rec.reset()
		_, _, err := runGateCommand(t, wc)
		assertConfirmationRefusal(t, wc.label, err)
		if sent := rec.requests(); len(sent) != 0 {
			t.Errorf("%s: sent %v without --yes; the gate must refuse before any request", wc.label, sent)
		}

		rec.reset()
		stdout, _, err := runGateCommand(t, wc, "--agent")
		assertConfirmationRefusal(t, wc.label+" --agent", err)
		if sent := rec.requests(); len(sent) != 0 {
			t.Errorf("%s --agent: sent %v without --yes", wc.label, sent)
		}
		var env map[string]any
		if jerr := json.Unmarshal([]byte(stdout), &env); jerr != nil {
			t.Errorf("%s --agent: refusal is not a JSON envelope on stdout: %v\n%s", wc.label, jerr, stdout)
		} else {
			body := env
			if results, ok := env["results"].(map[string]any); ok {
				body = results
			}
			msg, _ := body["error"].(string)
			if !strings.Contains(msg, "--yes") || !strings.Contains(msg, "--dry-run") {
				t.Errorf("%s --agent: envelope error must name --yes and --dry-run, got %v", wc.label, env)
			}
			if body["confirmation_required"] != true {
				t.Errorf("%s --agent: envelope must carry confirmation_required=true, got %v", wc.label, env)
			}
		}

		rec.reset()
		_, stderr, err := runGateCommand(t, wc, "--yes")
		if err != nil {
			t.Errorf("%s --yes: %v\nstderr: %s", wc.label, err, stderr)
		}
		sent := rec.requests()
		if len(sent) != 1 || !strings.HasPrefix(sent[0], wc.method+" ") {
			t.Errorf("%s --yes: sent %v, want exactly one %s request", wc.label, sent, wc.method)
		}
	}
}

// TestWriteCommandsDryRunPreviewsWithoutSending: --dry-run alone passes the
// gate and still sends nothing.
func TestWriteCommandsDryRunPreviewsWithoutSending(t *testing.T) {
	rec := startGateServer(t)
	for _, wc := range collectGateWriteCommands(t) {
		rec.reset()
		_, stderr, err := runGateCommand(t, wc, "--dry-run")
		if err != nil {
			t.Errorf("%s --dry-run: %v\nstderr: %s", wc.label, err, stderr)
		}
		if sent := rec.requests(); len(sent) != 0 {
			t.Errorf("%s --dry-run: sent %v; a preview must send nothing", wc.label, sent)
		}
	}
}

// TestPOSTReadsAreNotGated: a search and a batch read only read, so they run
// without --yes.
func TestPOSTReadsAreNotGated(t *testing.T) {
	rec := startGateServer(t)
	for _, argv := range [][]string{
		{"hubspot-contacts-crm", "post-v3-objects-contacts-search", "--stdin", "--json"},
		{"hubspot-deals-crm", "post-v3-objects-0-3-batch-read-read", "--stdin", "--json"},
	} {
		rec.reset()
		var stderr string
		var err error
		withStdin(t, `{}`, func() { _, stderr, err = runCLI(t, argv...) })
		if err != nil {
			t.Fatalf("%v without --yes: %v\nstderr: %s", argv, err, stderr)
		}
		if sent := rec.requests(); len(sent) != 1 || !strings.HasPrefix(sent[0], "POST ") {
			t.Fatalf("%v sent %v, want one POST", argv, sent)
		}
	}
}

// TestImportRequiresConfirmation: import POSTs every record of its input file
// to the API, so it sits behind the same gate.
func TestImportRequiresConfirmation(t *testing.T) {
	rec := startGateServer(t)
	input := filepath.Join(t.TempDir(), "deals.jsonl")
	if err := os.WriteFile(input, []byte("{\"properties\":{\"dealname\":\"A\"}}\n{\"properties\":{\"dealname\":\"B\"}}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := runCLI(t, "import", "hubspot-deals-crm", "--input", input)
	assertConfirmationRefusal(t, "import", err)
	if sent := rec.requests(); len(sent) != 0 {
		t.Fatalf("import sent %v without --yes", sent)
	}

	rec.reset()
	if _, stderr, err := runCLI(t, "import", "hubspot-deals-crm", "--input", input, "--dry-run"); err != nil {
		t.Fatalf("import --dry-run: %v\nstderr: %s", err, stderr)
	}
	if sent := rec.requests(); len(sent) != 0 {
		t.Fatalf("import --dry-run sent %v", sent)
	}

	rec.reset()
	if _, stderr, err := runCLI(t, "import", "hubspot-deals-crm", "--input", input, "--yes"); err != nil {
		t.Fatalf("import --yes: %v\nstderr: %s", err, stderr)
	}
	if sent := rec.requests(); len(sent) != 2 || sent[0] != "POST /crm/v3/objects/0-3" || sent[1] != "POST /crm/v3/objects/0-3" {
		t.Fatalf("import --yes sent %v, want two POST /crm/v3/objects/0-3 (the deal create endpoint)", sent)
	}
}

// TestRunProfileYesDoesNotConfirmWrites: a saved run profile can carry
// --dry-run, but a --yes saved once must not pre-approve every later write.
func TestRunProfileYesDoesNotConfirmWrites(t *testing.T) {
	rec := startGateServer(t)
	if _, stderr, err := runCLI(t, "profile", "save", "always-yes", "--yes", "--compact"); err != nil {
		t.Fatalf("profile save: %v\nstderr: %s", err, stderr)
	}
	// A run profile carries only output, format and filter flags: a profile
	// that holds nothing else is refused at save time.
	if _, _, err := runCLI(t, "profile", "save", "preview", "--dry-run"); err == nil {
		t.Fatal("profile save stored --dry-run; a run profile carries only output, format and filter flags")
	}
	approve := []string{"hubspot-contacts-crm", "delete-v3-objects-contacts-contact-id", "101"}
	_, _, err := runCLI(t, append(append([]string{}, approve...), "--profile", "always-yes")...)
	assertConfirmationRefusal(t, "contact delete --profile always-yes", err)
	if sent := rec.requests(); len(sent) != 0 {
		t.Fatalf("a saved --yes deleted a contact: sent %v", sent)
	}
}

// TestWriteCommandExamplesCarryConfirmation: no help example of a gated
// command may show an invocation the gate would refuse.
func TestWriteCommandExamplesCarryConfirmation(t *testing.T) {
	root := RootCmd()
	gated := map[string]bool{"import": true}
	for _, wc := range collectGateWriteCommands(t) {
		gated[wc.label] = true
	}
	checked := 0
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		for _, child := range cmd.Commands() {
			walk(child)
		}
		label := strings.Join(strings.Fields(cmd.CommandPath())[1:], " ")
		if !gated[label] {
			return
		}
		for _, line := range strings.Split(cmd.Example, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") || !strings.Contains(line, root.Name()+" ") {
				continue
			}
			checked++
			fields := strings.Fields(line)
			ok := false
			for _, f := range fields {
				if f == "--yes" || f == "--dry-run" {
					ok = true
				}
			}
			if !ok {
				t.Errorf("%s: help example %q would be refused; it needs --yes or --dry-run", label, trimmed)
			}
		}
	}
	walk(root)
	if checked == 0 {
		t.Fatal("no example lines checked; the walk is broken")
	}
}
