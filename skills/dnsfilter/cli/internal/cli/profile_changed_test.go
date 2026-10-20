// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"dnsfilter-pp-cli/internal/cliutil/testenv"
	"github.com/spf13/cobra"
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
	if err == nil || !strings.Contains(err.Error(), "may not store write authorizations: --yes;") {
		t.Fatalf("err = %v, want a refusal to store --yes", err)
	}
	if p, _ := GetProfile("risky"); p != nil {
		t.Fatalf("refused profile was saved: %+v", p)
	}
}
