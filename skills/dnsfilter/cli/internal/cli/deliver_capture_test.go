// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dnsfilter-pp-cli/internal/cliutil/testenv"
)

// executeArgs runs the real Execute() (the path main.go takes, including the
// stdout/stderr wrappers and --deliver flushing) with args, capturing the
// process streams.
func executeArgs(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	dir := t.TempDir()
	outF, err := os.Create(filepath.Join(dir, "stdout"))
	if err != nil {
		t.Fatal(err)
	}
	errF, err := os.Create(filepath.Join(dir, "stderr"))
	if err != nil {
		t.Fatal(err)
	}
	oldArgs, oldOut, oldErr := os.Args, os.Stdout, os.Stderr
	os.Args = append([]string{"dnsfilter-cli"}, args...)
	os.Stdout, os.Stderr = outF, errF
	defer func() {
		os.Args, os.Stdout, os.Stderr = oldArgs, oldOut, oldErr
		humanFriendly, noColor = false, false
	}()
	runErr := Execute()
	_ = outF.Close()
	_ = errF.Close()
	stdout, _ := os.ReadFile(outF.Name())
	stderr, _ := os.ReadFile(errF.Name())
	return string(stdout), string(stderr), runErr
}

// --deliver captures commands that used to write to the process stdout
// directly (export, agent-context, version) and help, which prints through
// the target command's inherited writer.
func TestDeliverCapturesEveryWriter(t *testing.T) {
	home := testenv.Isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// A list endpoint answers with a list (tail refuses a list response
		// whose collection is not one); the show endpoint with one object.
		if strings.HasSuffix(r.URL.Path, "/all") {
			fmt.Fprint(w, `{"data":[{"id":"1","attributes":{"name":"Acme Dental"}}]}`)
			return
		}
		fmt.Fprint(w, `{"data":{"id":"1","attributes":{"name":"Acme Dental"}}}`)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("DNSFILTER_BASE_URL", srv.URL)
	t.Setenv("DNSFILTER_API_KEY", "fixture-key")
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"version", []string{"version"}, "dnsfilter-cli "},
		{"agent-context", []string{"agent-context"}, `"schema_version"`},
		{"help", []string{"help", "export"}, "Export paginated API data"},
		{"export", []string{"export", "organizations", "1"}, "Acme Dental"},
		{"tail", []string{"tail", "organizations", "--follow=false"}, "Acme Dental"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sink := filepath.Join(home, tc.name+".out")
			stdout, stderr, err := executeArgs(t, append(tc.args, "--deliver", "file:"+sink)...)
			if err != nil {
				t.Fatalf("%v: %v\n%s%s", tc.args, err, stdout, stderr)
			}
			got, err := os.ReadFile(sink)
			if err != nil {
				t.Fatalf("sink not written: %v (stderr %s)", err, stderr)
			}
			if !strings.Contains(string(got), tc.want) {
				t.Fatalf("delivered %d bytes without %q:\n%s", len(got), tc.want, got)
			}
			if string(got) != stdout {
				t.Fatalf("delivered bytes differ from stdout:\nsink=%q\nstdout=%q", got, stdout)
			}
		})
	}
}

// A --deliver restored from a saved profile is honoured: the profile is
// applied before the sink is wired.
func TestDeliverAfterProfile(t *testing.T) {
	home := testenv.Isolate(t)
	sink := filepath.Join(home, "nightly.out")
	if stdout, stderr, err := executeArgs(t, "profile", "save", "nightly", "--deliver", "file:"+sink); err != nil {
		t.Fatalf("profile save: %v\n%s%s", err, stdout, stderr)
	}
	if _, err := os.Stat(sink); err == nil {
		t.Fatal("profile save performed the --deliver it should only record")
	}
	stdout, stderr, err := executeArgs(t, "version", "--profile", "nightly")
	if err != nil {
		t.Fatalf("version --profile: %v\n%s%s", err, stdout, stderr)
	}
	got, err := os.ReadFile(sink)
	if err != nil || !strings.Contains(string(got), "dnsfilter-cli ") {
		t.Fatalf("profile --deliver not honoured: err=%v sink=%q", err, got)
	}
}

// A failed command still delivers what it printed, or says the sink was not
// updated, so a scheduled sink never silently keeps a previous run's report.
func TestDeliverOnFailure(t *testing.T) {
	home := testenv.Isolate(t)
	sink := filepath.Join(home, "fail.out")
	if err := os.WriteFile(sink, []byte("previous run\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// An unknown export resource fails after nothing was printed.
	_, stderr, err := executeArgs(t, "export", "no-such-resource", "--deliver", "file:"+sink)
	if err == nil {
		t.Fatal("export of an unknown resource succeeded")
	}
	if !strings.Contains(stderr, "was not updated") {
		t.Fatalf("no notice that the sink kept the previous run: %q", stderr)
	}
	// doctor --fail-on error prints its report, then fails (no key is set):
	// the report it printed is delivered.
	t.Setenv("DNSFILTER_API_KEY", "")
	t.Setenv("DNSFILTER_BASE_URL", "http://127.0.0.1:1")
	stdout, _, err := executeArgs(t, "doctor", "--json", "--fail-on", "error", "--deliver", "file:"+sink)
	if err == nil {
		t.Fatalf("doctor --fail-on error passed with no key:\n%s", stdout)
	}
	got, _ := os.ReadFile(sink)
	if !strings.Contains(string(got), `"env_vars"`) || string(got) != stdout {
		t.Fatalf("failed run did not deliver what it printed: sink=%q", got)
	}
}

func TestDeliverRefusedOnCompletion(t *testing.T) {
	home := testenv.Isolate(t)
	_, _, err := executeArgs(t, "completion", "bash", "--deliver", "file:"+filepath.Join(home, "c.sh"))
	if err == nil || !strings.Contains(err.Error(), "--deliver does not apply to completion scripts") {
		t.Fatalf("err = %v, want completion to refuse --deliver", err)
	}
}
