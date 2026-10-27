// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pins the CLI write-confirmation gate
// (handfixes.json "cli-writes-require-yes"). Every command that changes the
// SentinelOne tenant must refuse to run, before it sends anything, unless the
// caller passed --yes (send it) or --dry-run (preview it). The tests drive the
// real Cobra tree against a fake SentinelOne API and read the API's own
// request log, so a gate that only changes an error message cannot pass them.

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
	"sentinelone-pp-cli/internal/cliutil/testenv"
)

// gateReadPOSTPaths is this test's own list of the POST endpoints that only
// read. It is deliberately independent of the code's allowlist: if the code
// ever exempts another write path, the tree walk below sees that command send
// without --yes and fails.
var gateReadPOSTPaths = map[string]bool{
	"/agents/actions/can-start-remote-shell": true,
	"/exclusions/validate":                   true,
	"/restrictions/validate":                 true,
	"/users/api-token-details":               true,
}

// gateRecorder is a fake SentinelOne API that records every request it
// answers.
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
		_, _ = w.Write([]byte(`{"data":{}}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("SENTINELONE_BASE_URL", srv.URL)
	t.Setenv("SENTINELONE_API_TOKEN", "test-token-gate-0001")
	t.Setenv("SENTINELONE_NO_LEARN", "1")
	return rec
}

// gateRunCLI runs a fresh root command and returns stdout, stderr and the
// error.
func gateRunCLI(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := RootCmd()
	cmd.SetArgs(args)
	var out, errb bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errb)
	err := cmd.Execute()
	return out.String(), errb.String(), err
}

// gateWithStdin points os.Stdin at a file holding body for the duration of fn.
func gateWithStdin(t *testing.T, body string, fn func()) {
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
// tenant, with an argument list that is enough to send the request.
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
	gateWithStdin(t, `{}`, func() {
		stdout, stderr, err = gateRunCLI(t, append(append([]string{}, wc.argv...), extra...)...)
	})
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
	seen := map[string]bool{}
	methods := map[string]int{}
	for _, wc := range cmds {
		seen[wc.label] = true
		methods[wc.method]++
	}
	for _, must := range []string{"agents abort-scan", "agents disconnect-from-network", "agents uninstall", "agents decommission", "remote-scripts run"} {
		if !seen[must] {
			t.Fatalf("tree walk did not find %q among %d write commands; the walk is broken", must, len(cmds))
		}
	}
	if len(cmds) < 200 || methods["POST"] == 0 || methods["PUT"] == 0 || methods["DELETE"] == 0 {
		t.Fatalf("tree walk found %d write commands (%v); the walk is broken", len(cmds), methods)
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
			if usage, _ := body["usage"].(string); !strings.Contains(usage, "--yes") {
				t.Errorf("%s --agent: envelope usage must carry --yes, got %v", wc.label, env)
			}
			if preview, _ := body["preview"].(string); !strings.Contains(preview, "--dry-run") {
				t.Errorf("%s --agent: envelope preview must carry --dry-run, got %v", wc.label, env)
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

// TestReadPOSTsAreNotGated: the POST endpoints that only read run without
// --yes, and so does a plain GET.
func TestReadPOSTsAreNotGated(t *testing.T) {
	rec := startGateServer(t)
	cases := []struct {
		argv []string
		want string
	}{
		{[]string{"agents", "can-run-remote-shell", "--stdin"}, "POST /agents/actions/can-start-remote-shell"},
		{[]string{"exclusions", "validate-item", "--stdin"}, "POST /exclusions/validate"},
		{[]string{"restrictions", "validate-blacklist-item", "--stdin"}, "POST /restrictions/validate"},
		{[]string{"users", "token-details", "--stdin"}, "POST /users/api-token-details"},
		{[]string{"agents", "get", "--json"}, "GET /agents"},
	}
	for _, tc := range cases {
		rec.reset()
		var stderr string
		var err error
		gateWithStdin(t, `{}`, func() {
			_, stderr, err = gateRunCLI(t, append(append([]string{}, tc.argv...), "--json")...)
		})
		if err != nil {
			t.Errorf("%v without --yes: %v\nstderr: %s", tc.argv, err, stderr)
		}
		sent := rec.requests()
		if len(sent) != 1 || sent[0] != tc.want {
			t.Errorf("%v sent %v, want one %s", tc.argv, sent, tc.want)
		}
	}
}

// TestSyncIsNotGated: sync reads the tenant into the local store and changes
// nothing in SentinelOne, so it runs without --yes and sends only reads.
func TestSyncIsNotGated(t *testing.T) {
	rec := startGateServer(t)
	db := filepath.Join(t.TempDir(), "data.db")
	_, stderr, err := gateRunCLI(t, "sync", "--resources", "agents", "--db", db, "--max-pages", "1", "--json")
	if err != nil && strings.Contains(err.Error(), "refusing") {
		t.Fatalf("sync was refused by the write gate: %v\nstderr: %s", err, stderr)
	}
	sent := rec.requests()
	if len(sent) == 0 {
		t.Fatalf("sync sent nothing (err=%v, stderr=%s); it must not be gated", err, stderr)
	}
	for _, r := range sent {
		if !strings.HasPrefix(r, "GET ") {
			t.Errorf("sync sent a non-read request %q", r)
		}
	}
}

// TestImportRequiresConfirmation: import POSTs every record of its input file
// to the API, so it sits behind the same gate.
func TestImportRequiresConfirmation(t *testing.T) {
	rec := startGateServer(t)
	input := filepath.Join(t.TempDir(), "exclusions.jsonl")
	if err := os.WriteFile(input, []byte("{\"data\":{\"value\":\"a\"}}\n{\"data\":{\"value\":\"b\"}}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := gateRunCLI(t, "import", "exclusions", "--input", input)
	assertConfirmationRefusal(t, "import", err)
	if sent := rec.requests(); len(sent) != 0 {
		t.Fatalf("import sent %v without --yes", sent)
	}

	rec.reset()
	stdout, _, err := gateRunCLI(t, "import", "exclusions", "--input", input, "--json")
	assertConfirmationRefusal(t, "import --json", err)
	if sent := rec.requests(); len(sent) != 0 {
		t.Fatalf("import --json sent %v without --yes", sent)
	}
	var env map[string]any
	if jerr := json.Unmarshal([]byte(stdout), &env); jerr != nil || env["confirmation_required"] != true {
		t.Fatalf("import --json refusal envelope = %q (%v); want confirmation_required=true", stdout, jerr)
	}

	rec.reset()
	if _, stderr, err := gateRunCLI(t, "import", "exclusions", "--input", input, "--dry-run"); err != nil {
		t.Fatalf("import --dry-run: %v\nstderr: %s", err, stderr)
	}
	if sent := rec.requests(); len(sent) != 0 {
		t.Fatalf("import --dry-run sent %v", sent)
	}

	rec.reset()
	if _, stderr, err := gateRunCLI(t, "import", "exclusions", "--input", input, "--yes"); err != nil {
		t.Fatalf("import --yes: %v\nstderr: %s", err, stderr)
	}
	sent := rec.requests()
	if len(sent) != 2 || sent[0] != "POST /exclusions" || sent[1] != "POST /exclusions" {
		t.Fatalf("import --yes sent %v, want two POST /exclusions", sent)
	}
}

// TestRunProfileYesDoesNotConfirmWrites: a saved run profile can carry
// --dry-run, but a --yes saved once must not pre-approve every later write.
func TestRunProfileYesDoesNotConfirmWrites(t *testing.T) {
	rec := startGateServer(t)
	if _, stderr, err := gateRunCLI(t, "profile", "save", "always-yes", "--yes"); err != nil {
		t.Fatalf("profile save: %v\nstderr: %s", err, stderr)
	}
	if _, stderr, err := gateRunCLI(t, "profile", "save", "preview", "--dry-run"); err != nil {
		t.Fatalf("profile save: %v\nstderr: %s", err, stderr)
	}
	isolate := []string{"agents", "disconnect-from-network", "--stdin"}
	var err error
	gateWithStdin(t, `{}`, func() {
		_, _, err = gateRunCLI(t, append(append([]string{}, isolate...), "--profile", "always-yes")...)
	})
	assertConfirmationRefusal(t, "disconnect-from-network --profile always-yes", err)
	if sent := rec.requests(); len(sent) != 0 {
		t.Fatalf("a saved --yes isolated endpoints: sent %v", sent)
	}
	rec.reset()
	var stderr string
	gateWithStdin(t, `{}`, func() {
		_, stderr, err = gateRunCLI(t, append(append([]string{}, isolate...), "--profile", "preview")...)
	})
	if err != nil {
		t.Fatalf("disconnect-from-network --profile preview: %v\nstderr: %s", err, stderr)
	}
	if sent := rec.requests(); len(sent) != 0 {
		t.Fatalf("a saved --dry-run sent %v", sent)
	}
	rec.reset()
	gateWithStdin(t, `{}`, func() {
		_, stderr, err = gateRunCLI(t, append(append([]string{}, isolate...), "--profile", "always-yes", "--yes")...)
	})
	if err != nil {
		t.Fatalf("disconnect-from-network --profile always-yes --yes: %v\nstderr: %s", err, stderr)
	}
	if sent := rec.requests(); len(sent) != 1 || !strings.HasPrefix(sent[0], "POST ") {
		t.Fatalf("a typed --yes sent %v, want one POST", sent)
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
			ok := false
			for _, f := range strings.Fields(line) {
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
	if checked < 200 {
		t.Fatalf("only %d example lines checked; the walk is broken", checked)
	}
}
