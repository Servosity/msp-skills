// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"dnsfilter-pp-cli/internal/cliutil/testenv"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// A profile value marks its flag Changed, so a command that gates on
// Changed() (sends a field, or takes a guarded path, only when the flag
// changed) treats a saved --dry-run exactly like a typed one.
func TestApplyProfileMarksFlagsChanged(t *testing.T) {
	var dryRun bool
	cmd := &cobra.Command{Use: "decision"}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "")
	if err := ApplyProfileToFlags(cmd, &Profile{Values: map[string]string{"dry-run": "true"}}); err != nil {
		t.Fatal(err)
	}
	if !dryRun || !cmd.Flags().Changed("dry-run") {
		t.Fatalf("dry-run=%v changed=%v, want both true", dryRun, cmd.Flags().Changed("dry-run"))
	}
}

// A typed --csv wins over a profile's --json (same group), while a
// profile's --compact (another group) still applies.
func TestApplyProfileTypedOutputModeWins(t *testing.T) {
	var asJSON, asCSV, compact bool
	cmd := &cobra.Command{Use: "organizations"}
	cmd.Flags().BoolVar(&asJSON, "json", false, "")
	cmd.Flags().BoolVar(&asCSV, "csv", false, "")
	cmd.Flags().BoolVar(&compact, "compact", false, "")
	if err := cmd.Flags().Set("csv", "true"); err != nil {
		t.Fatal(err)
	}
	if err := ApplyProfileToFlags(cmd, &Profile{Values: map[string]string{"json": "true", "compact": "true"}}); err != nil {
		t.Fatal(err)
	}
	if asJSON || !asCSV || !compact {
		t.Fatalf("json=%v csv=%v compact=%v, want false true true", asJSON, asCSV, compact)
	}
}

// A write authorization is never taken from a profile, even a hand-edited
// profiles.json: allow --confirm, --force and --yes must be typed.
func TestApplyProfileNeverAppliesWriteAuthorization(t *testing.T) {
	var confirm, force, yes bool
	var note string
	cmd := &cobra.Command{Use: "allow"}
	cmd.Flags().BoolVar(&confirm, "confirm", false, "")
	cmd.Flags().BoolVar(&force, "force", false, "")
	cmd.Flags().BoolVar(&yes, "yes", false, "")
	cmd.Flags().StringVar(&note, "note", "", "")
	var errOut bytes.Buffer
	cmd.SetErr(&errOut)
	p := &Profile{Name: "nightly", Values: map[string]string{"confirm": "true", "force": "true", "yes": "true", "note": "TKT-1"}}
	if err := ApplyProfileToFlags(cmd, p); err != nil {
		t.Fatal(err)
	}
	if confirm || force || yes || cmd.Flags().Changed("confirm") {
		t.Fatalf("confirm=%v force=%v yes=%v: a profile must not authorize a write", confirm, force, yes)
	}
	if note != "TKT-1" {
		t.Fatalf("note=%q: ordinary values still apply", note)
	}
	if !strings.Contains(errOut.String(), "write authorizations are never applied from a profile") {
		t.Fatalf("no warning for the ignored write authorization: %q", errOut.String())
	}
}

// profileRunner runs commands against the allow fixture in ONE isolated home,
// so a profile saved by one call is visible to the next.
func profileRunner(t *testing.T, f *fixtureAPI) func(args ...string) (string, string, error) {
	t.Helper()
	testenv.Isolate(t)
	srv := httptest.NewServer(f.handler(t))
	t.Cleanup(srv.Close)
	t.Setenv("DNSFILTER_BASE_URL", srv.URL)
	t.Setenv("DNSFILTER_API_KEY", "fixture-key")
	return func(args ...string) (string, string, error) {
		cmd := RootCmd()
		cmd.SetArgs(args)
		var out, errOut bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&errOut)
		err := cmd.Execute()
		return out.String(), errOut.String(), err
	}
}

func allowWriteFixture() *fixtureAPI {
	return &fixtureAPI{lookupBody: fixtureLookupOK, logPages: map[string]string{"1": `[{"fqdn":"example.com","result":"blocked","policy_id":"11"}]`}, allowlisted: map[string]bool{"11": true}}
}

// --dry-run wins over --confirm, typed or restored from a saved profile:
// allow writes nothing.
func TestAllowDryRunBeatsConfirm(t *testing.T) {
	f := allowWriteFixture()
	run := profileRunner(t, f)
	if out, errOut, err := run("allow", "example.com", "--org", "1", "--note", "TKT-9", "--confirm", "--dry-run", "--json"); err != nil {
		t.Fatalf("typed --dry-run --confirm: %v\n%s%s", err, out, errOut)
	}
	if out, errOut, err := run("profile", "save", "preview", "--dry-run"); err != nil {
		t.Fatalf("profile save: %v\n%s%s", err, out, errOut)
	}
	if out, errOut, err := run("allow", "example.com", "--org", "1", "--note", "TKT-9", "--confirm", "--profile", "preview", "--json"); err != nil {
		t.Fatalf("profile --dry-run with typed --confirm: %v\n%s%s", err, out, errOut)
	}
	if len(f.writes) != 0 {
		t.Fatalf("allow wrote under --dry-run: %v", f.writes)
	}
}

// A profile carrying confirm (only possible by editing profiles.json, since
// profile save refuses it) leaves allow a preview.
func TestAllowProfileCannotConfirmAWrite(t *testing.T) {
	f := allowWriteFixture()
	run := profileRunner(t, f)
	path, err := profileStorePath()
	if err != nil {
		t.Fatal(err)
	}
	store := map[string]any{"profiles": map[string]any{"nightly": map[string]any{"name": "nightly", "values": map[string]string{"confirm": "true", "note": "TKT-9"}}}}
	data, _ := json.Marshal(store)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	out, errOut, err := run("allow", "example.com", "--org", "1", "--profile", "nightly", "--json")
	if err != nil {
		t.Fatalf("allow --profile nightly: %v\n%s%s", err, out, errOut)
	}
	if len(f.writes) != 0 {
		t.Fatalf("a profile turned the preview into a write: %v", f.writes)
	}
	var view allowView
	if err := json.Unmarshal([]byte(out), &view); err != nil || view.Applied {
		t.Fatalf("applied=%v err=%v; want an unapplied preview:\n%s", view.Applied, err, out)
	}
	// The same flags typed on the command line still write.
	if out, errOut, err := run("allow", "example.com", "--org", "1", "--note", "TKT-9", "--confirm", "--json"); err != nil || len(f.writes) != 1 {
		t.Fatalf("typed --confirm: err=%v writes=%v\n%s%s", err, f.writes, out, errOut)
	}
}

func TestProfileSaveRefusesWriteAuthorization(t *testing.T) {
	run := profileRunner(t, &fixtureAPI{})
	_, _, err := run("profile", "save", "risky", "--yes", "--json")
	if err == nil || !strings.Contains(err.Error(), "may not store write authorizations or write-outcome flags: --yes;") {
		t.Fatalf("err = %v, want a refusal to store --yes", err)
	}
	if p, _ := GetProfile("risky"); p != nil {
		t.Fatalf("refused profile was saved: %+v", p)
	}
	// Flags that only change how a write's outcome is judged are refused too.
	_, _, err = run("profile", "save", "lenient", "--idempotent", "--ignore-missing", "--allow-partial-failure", "--json")
	if err == nil || !strings.Contains(err.Error(), "--allow-partial-failure, --idempotent, --ignore-missing;") {
		t.Fatalf("err = %v, want a refusal naming the write-outcome flags", err)
	}
	// Output and transport flags still save.
	if _, _, err := run("profile", "save", "report", "--json", "--timeout", "30s", "--dry-run"); err != nil {
		t.Fatalf("an output/transport profile was refused: %v", err)
	}
}

// writeProfileFixture serves the allow fixture plus a global policy 12 that
// blocked example.com, so only --include-global makes allow write to it.
func writeProfileFixture(t *testing.T, f *fixtureAPI) http.Handler {
	base := f.handler(t)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/policies/12" {
			fmt.Fprint(w, `{"data":{"id":"12","attributes":{"name":"Global","can_edit":true,"is_global_policy":true,"whitelist_domains":[]}}}`)
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/v1/policies/12/add_whitelist_domain" {
			f.mu.Lock()
			f.writes = append(f.writes, r.URL.Path)
			f.mu.Unlock()
			fmt.Fprint(w, `{"data":{"id":"12","attributes":{}}}`)
			return
		}
		base.ServeHTTP(w, r)
	})
}

// On a command that writes to DNSFilter a profile supplies only output and
// transport flags. A hand-edited profiles.json carrying include-global, force,
// note, org, host, since, max-scan-pages and a generated write's body flags
// applies none of them: allow stays off the global policy and the generated
// write is never sent.
func TestTenantWriteProfileAllowlist(t *testing.T) {
	f := &fixtureAPI{lookupBody: fixtureLookupOK, logPages: map[string]string{"1": `[{"fqdn":"example.com","result":"blocked","policy_id":"12"}]`}}
	testenv.Isolate(t)
	srv := httptest.NewServer(writeProfileFixture(t, f))
	t.Cleanup(srv.Close)
	t.Setenv("DNSFILTER_BASE_URL", srv.URL)
	t.Setenv("DNSFILTER_API_KEY", "fixture-key")
	path, err := profileStorePath()
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]string{
		"include-global": "true", "force": "true", "note": "FROM-PROFILE", "org": "2", "host": "a1",
		"since": "1h", "max-scan-pages": "1", "data-source": "local",
		"domain": "evil.example", // body flags of policies add-whitelist-domain
		"json":   "true",         // output: still applied
	}
	data, _ := json.Marshal(map[string]any{"profiles": map[string]any{"nightly": map[string]any{"name": "nightly", "values": values}}})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) (string, string, error) {
		cmd := RootCmd()
		cmd.SetArgs(args)
		var out, errOut bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&errOut)
		err := cmd.Execute()
		return out.String(), errOut.String(), err
	}

	out, errOut, err := run("allow", "example.com", "--org", "1", "--note", "TKT-1", "--confirm", "--profile", "nightly")
	if err != nil && exitCode(err) != 6 {
		t.Fatalf("allow: %v\n%s%s", err, out, errOut)
	}
	if len(f.writes) != 0 {
		t.Fatalf("a profile value reached a write: %v", f.writes)
	}
	var view allowView
	if err := json.Unmarshal([]byte(out), &view); err != nil {
		t.Fatalf("profile --json (an output flag) was not applied: %v\n%s", err, out)
	}
	if view.Org.ID != "1" {
		t.Fatalf("typed --org lost to the profile: %+v", view.Org)
	}
	for _, name := range []string{"include-global", "host", "since", "max-scan-pages", "data-source"} {
		if !strings.Contains(errOut, "sets --"+name+";") {
			t.Fatalf("no warning naming the ignored --%s:\n%s", name, errOut)
		}
	}
	// Untyped, the profile's org and note are not used either: allow refuses.
	_, errOut, err = run("allow", "example.com", "--profile", "nightly")
	if err == nil || !strings.Contains(err.Error(), "--org is required") || len(f.writes) != 0 {
		t.Fatalf("allow took --org from the profile: err=%v writes=%v", err, f.writes)
	}
	for _, name := range []string{"org", "note"} {
		if !strings.Contains(errOut, "sets --"+name+";") {
			t.Fatalf("no warning naming the ignored --%s:\n%s", name, errOut)
		}
	}

	// A generated write gets none of its body flags from the profile.
	_, errOut, err = run("policies", "add-whitelist-domain", "11", "--profile", "nightly")
	if err == nil || len(f.writes) != 0 {
		t.Fatalf("generated write ran from profile values: err=%v writes=%v\n%s", err, f.writes, errOut)
	}
	if !strings.Contains(errOut, "sets --domain;") {
		t.Fatalf("no warning naming the ignored --domain:\n%s", errOut)
	}

	// The same flag typed on the command line still writes (positive control;
	// the fixture's read-back never shows the entry, hence exit 6).
	if _, errOut, err := run("allow", "example.com", "--org", "1", "--note", "TKT-1", "--confirm", "--include-global", "--json"); (err != nil && exitCode(err) != 6) || len(f.writes) != 1 || f.writes[0] != "/v1/policies/12/add_whitelist_domain" {
		t.Fatalf("typed --include-global did not write to the global policy: err=%v writes=%v\n%s", err, f.writes, errOut)
	}
}

// commandWritesTenant classifies the writers: every non-GET endpoint, the GET
// that creates a PSA integration, allow and import; reads stay unrestricted.
func TestCommandWritesTenantClassification(t *testing.T) {
	root := RootCmd()
	for path, want := range map[string]bool{
		"allow": true, "import": true, "psa-integrations": true,
		"policies add-whitelist-domain": true, "organizations destroy": true, "policies update": true,
		"organizations index": false, "why-blocked": false, "agents stale": false, "profile save": false, "export": false,
	} {
		cmd, _, err := root.Find(strings.Fields(path))
		if err != nil || cmd.CommandPath() != "dnsfilter-cli "+path {
			t.Fatalf("%s not found: %v", path, err)
		}
		if got := commandWritesTenant(cmd); got != want {
			t.Errorf("commandWritesTenant(%s) = %v, want %v", path, got, want)
		}
	}
	// Read-only commands keep applying any profile value.
	var org string
	read := &cobra.Command{Use: "index", Annotations: map[string]string{"pp:method": "GET"}}
	read.Flags().StringVar(&org, "organization-id", "", "")
	if err := ApplyProfileToFlags(read, &Profile{Values: map[string]string{"organization-id": "5"}}); err != nil || org != "5" {
		t.Fatalf("a read command lost its profile value: org=%q err=%v", org, err)
	}
}

// On a writer, every allowlisted output and transport flag still applies from
// a profile, while body flags do not.
func TestTenantWriteProfileAppliesSafeFlags(t *testing.T) {
	cmd, _, err := RootCmd().Find([]string{"policies", "add-whitelist-domain"})
	if err != nil {
		t.Fatal(err)
	}
	var errOut bytes.Buffer
	cmd.SetErr(&errOut)
	safe := map[string]string{"json": "true", "compact": "true", "no-color": "true", "timeout": "5s", "rate-limit": "1", "no-cache": "true", "dry-run": "true", "max-age": "1m", "no-learn": "true", "no-input": "true"}
	values := map[string]string{"domain": "evil.example", "note": "x"}
	for k, v := range safe {
		values[k] = v
	}
	if err := ApplyProfileToFlags(cmd, &Profile{Name: "p", Values: values}); err != nil {
		t.Fatal(err)
	}
	get := func(name string) *pflag.Flag {
		if f := cmd.Flags().Lookup(name); f != nil {
			return f
		}
		return cmd.InheritedFlags().Lookup(name)
	}
	for name := range safe {
		if f := get(name); f == nil || !f.Changed {
			t.Errorf("allowlisted --%s was not applied to a writer", name)
		}
	}
	for _, name := range []string{"domain", "note"} {
		if f := get(name); f == nil || f.Changed {
			t.Errorf("body flag --%s was applied from a profile", name)
		}
	}
}
