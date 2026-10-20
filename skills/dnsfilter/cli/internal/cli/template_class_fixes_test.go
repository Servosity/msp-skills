// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Regression tests for press 4.33.5 template defects fixed as classes in this
// connector (see skills/dnsfilter/handfixes.json).

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dnsfilter-pp-cli/internal/cliutil/testenv"
	"dnsfilter-pp-cli/internal/config"
	"github.com/spf13/cobra"
)

// hasTerminalControl reports a byte or rune a terminal acts on: C0 other
// than tab/newline, DEL, or C1.
func hasTerminalControl(s string) bool {
	for _, r := range s {
		if (r < 0x20 && r != '\t' && r != '\n') || (r >= 0x7f && r <= 0x9f) {
			return true
		}
	}
	return false
}

const evilName = "Acme\x1b]52;c;aGk=\x07\x1b[2J\u009b2J Dental"

// --csv, --plain and --quiet drop terminal controls from tenant values.
func TestTabularPrintersScrubTerminalControls(t *testing.T) {
	data, _ := json.Marshal([]map[string]any{{"id": "1", "name": evilName}})
	for name, print := range map[string]func(*bytes.Buffer) error{
		"csv":   func(b *bytes.Buffer) error { return printCSV(b, data) },
		"plain": func(b *bytes.Buffer) error { return printPlain(b, data) },
		"quiet": func(b *bytes.Buffer) error { return printQuiet(b, data) },
		"quiet-scalars": func(b *bytes.Buffer) error {
			return printQuiet(b, json.RawMessage(`["Acme\u001b]52;c;aGk=\u0007"]`))
		},
		"quiet-raw": func(b *bytes.Buffer) error { return printQuiet(b, json.RawMessage("\"x\x1b[2J\"")) },
	} {
		var out bytes.Buffer
		if err := print(&out); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if hasTerminalControl(out.String()) {
			t.Fatalf("%s output carries a terminal control: %q", name, out.String())
		}
	}
	if got := quietScalar(evilName); hasTerminalControl(got) {
		t.Fatalf("quietScalar kept a control: %q", got)
	}
}

// The novel commands' human output drops terminal controls from tenant
// strings (here allow's renderer; a DNS query name can carry any byte).
func TestNovelHumanOutputScrubsTerminalControls(t *testing.T) {
	humanFriendly = true
	t.Cleanup(func() { humanFriendly = false })
	cmd := &cobra.Command{Use: "allow"}
	var out bytes.Buffer
	cmd.SetOut(&out)
	view := allowView{
		FQDN:        "ex\x1b[2Jample.com",
		Org:         orgRef{ID: "1", Name: evilName},
		Attribution: "query_log",
		Targets:     []allowTarget{{PolicyID: "10", PolicyName: evilName, UsedBy: []string{"HQ\x07"}, Action: "add", Reason: "blocked\u009d52"}},
		Notes:       []string{"note\x1b]0;title\x07"},
	}
	if err := renderAllow(cmd, &rootFlags{}, view); err != nil {
		t.Fatal(err)
	}
	if hasTerminalControl(out.String()) {
		t.Fatalf("allow human output carries a terminal control: %q", out.String())
	}
	if !strings.Contains(out.String(), "Acme]52;c;aGk=[2J2J Dental") {
		t.Fatalf("tenant text lost beyond the controls: %q", out.String())
	}
}

// agents stale's human table (hostnames are agent-reported) and the
// ambiguous-org error drop terminal controls.
func TestStaleHumanOutputScrubsAgentHostnames(t *testing.T) {
	run := runFleetFixture(t, &fixtureAPI{agents: []string{`{"id":"a1","attributes":{"hostname":"PC\u001b]52;c;aGk=\u0007-1","agent_type":"windows","agent_version":"3.1.0\u001b[2J","status":"active","last_sync":"2020-01-01T00:00:00Z"},"relationships":{"network":{"data":{"id":"50"}},"policy":{"data":{"id":"10"}}}}`}})
	db := t.TempDir() + "/mirror.db"
	run("sync", "--resources", "user-agents", "--db", db, "--json")
	human := run("agents", "stale", "--db", db, "--human-friendly")
	if !strings.Contains(human, "PC]52;c;aGk=-1") {
		t.Fatalf("stale did not list the silent client:\n%q", human)
	}
	if hasTerminalControl(human) {
		t.Fatalf("stale human output carries a terminal control: %q", human)
	}
}

// The generator's hidden "create" alias stays only on commands that create;
// on a revoke/cancel/decision/update/dequeue POST it let a human approving a
// "create" approve something else, and bypassed a name-keyed deny rule.
func TestCreateAliasOnlyOnCreates(t *testing.T) {
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, a := range c.Aliases {
			if a == "create" && !strings.Contains(c.Name(), "create") && !strings.Contains(c.Name(), "generate") {
				t.Errorf("%q carries the alias create but does not create (%s %s)", c.CommandPath(), c.Annotations["pp:method"], c.Annotations["pp:path"])
			}
		}
		for _, child := range c.Commands() {
			walk(child)
		}
	}
	walk(RootCmd())
}

// --dry-run changes nothing locally: export -o, feedback, auth set-token,
// auth logout and profile delete return the preview instead of mutating.
func TestDryRunGuardsLocalMutations(t *testing.T) {
	home := testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":{"id":"1","attributes":{"name":"Acme Dental"}}}`)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DNSFILTER_BASE_URL", srv.URL)
	t.Setenv("DNSFILTER_API_KEY", "")
	run := func(stdin string, args ...string) (string, error) {
		cmd := RootCmd()
		cmd.SetArgs(args)
		cmd.SetIn(strings.NewReader(stdin))
		var out, errOut bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&errOut)
		err := cmd.Execute()
		return out.String() + errOut.String(), err
	}
	keep := filepath.Join(home, "keep.jsonl")
	if err := os.WriteFile(keep, []byte("previous export\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := run("", "export", "organizations", "1", "-o", keep, "--dry-run"); err != nil {
		t.Fatalf("export --dry-run: %v\n%s", err, out)
	}
	if got, _ := os.ReadFile(keep); string(got) != "previous export\n" {
		t.Fatalf("export --dry-run rewrote -o: %q", got)
	}
	if out, err := run("saved-key\n", "auth", "set-token"); err != nil {
		t.Fatalf("set-token: %v\n%s", err, out)
	}
	if out, err := run("other-key\n", "auth", "set-token", "--dry-run"); err != nil {
		t.Fatalf("set-token --dry-run: %v\n%s", err, out)
	}
	if out, err := run("", "auth", "logout", "--dry-run"); err != nil {
		t.Fatalf("logout --dry-run: %v\n%s", err, out)
	}
	if cfg, err := config.Load(""); err != nil || cfg.DnsfilterApiKey != "saved-key" {
		t.Fatalf("a --dry-run set-token or logout changed the stored key: err=%v", err)
	}
	if out, err := run("", "profile", "save", "keepme", "--json"); err != nil {
		t.Fatalf("profile save: %v\n%s", err, out)
	}
	if out, err := run("", "profile", "delete", "keepme", "--yes", "--dry-run"); err != nil {
		t.Fatalf("profile delete --dry-run: %v\n%s", err, out)
	}
	if p, _ := GetProfile("keepme"); p == nil {
		t.Fatal("profile delete --dry-run deleted the profile")
	}
	if out, err := run("", "feedback", "dry run must not record this", "--dry-run"); err != nil {
		t.Fatalf("feedback --dry-run: %v\n%s", err, out)
	}
	if out, _ := run("", "feedback", "list", "--json"); strings.Contains(out, "dry run must not record this") {
		t.Fatalf("feedback --dry-run recorded the entry:\n%s", out)
	}
}

// No tenant-gated client profile source is registered for DNSFilter, so
// --client-profile changes nothing; its help must not promise tenant gating
// an agent could believe it had scoped itself with.
func TestClientProfileFlagDocumentedInert(t *testing.T) {
	if registeredPlatformSource != nil {
		t.Skip("a platform source is registered; --client-profile is live, update its help")
	}
	f := RootCmd().PersistentFlags().Lookup("client-profile")
	if f == nil {
		t.Fatal("--client-profile flag missing")
	}
	if !strings.Contains(f.Usage, "Unused by this connector") || !strings.Contains(f.Usage, "does not limit which organizations") {
		t.Fatalf("--client-profile help promises gating it does not provide: %q", f.Usage)
	}
}

// The root help names the product with the vendor's casing ("DNSFilter CLI"),
// not the generator's title-cased slug ("Dnsfilter CLI").
func TestRootHelpBrandCasing(t *testing.T) {
	root := RootCmd()
	if !strings.HasPrefix(root.Long, "DNSFilter CLI") || strings.Contains(root.Long+root.Short, "Dnsfilter") {
		t.Fatalf("root help casing: Short=%q Long starts %q", root.Short, root.Long[:40])
	}
}

// --csv and --plain cells, and the novel commands' human output, write bidi
// controls as \u20XX so a tenant name cannot reorder an allow preview.
func TestScrubbersEscapeBidiControls(t *testing.T) {
	name := "Acme \u202eexe.gnp\u202c Dental"
	want := `Acme \u202eexe.gnp\u202c Dental`
	if got := scrubCell(name); got != want {
		t.Fatalf("scrubCell = %q, want %q", got, want)
	}
	if got := humanText(name); got != want {
		t.Fatalf("humanText = %q, want %q", got, want)
	}
	data, _ := json.Marshal([]map[string]any{{"id": "1", "name": name}})
	var out bytes.Buffer
	if err := printCSV(&out, data); err != nil {
		t.Fatal(err)
	}
	if strings.ContainsRune(out.String(), '\u202e') || !strings.Contains(out.String(), want) {
		t.Fatalf("--csv kept a raw bidi control: %q", out.String())
	}
}

// psa-integrations is a GET that creates a PSA integration, so it must not
// carry mcp:read-only (MCP hosts would see readOnlyHint and may auto-approve).
func TestPSAIntegrationsNotReadOnly(t *testing.T) {
	cmd, _, err := RootCmd().Find([]string{"psa-integrations"})
	if err != nil || cmd.Name() != "psa-integrations" {
		t.Fatalf("psa-integrations not found: %v", err)
	}
	if cmd.Annotations["mcp:read-only"] == "true" {
		t.Fatal("psa-integrations creates a PSA integration but is annotated mcp:read-only")
	}
	if cmd.Annotations["pp:path"] != "/v1/psa_integrations/redirect_link" {
		t.Fatalf("unexpected path %q", cmd.Annotations["pp:path"])
	}
}
