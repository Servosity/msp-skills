// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored (codex r1, MCP host-file I/O class): every onboard run reads a
// template file or, with --init, writes one. A process the MCP server spawned
// refuses all of them whichever argument reached onboard, and --dry-run never
// writes the example template. --apply writes only to the selected --company.

package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"hudu-pp-cli/internal/cliutil"
	"hudu-pp-cli/internal/cliutil/testenv"
)

func runOnboardForTest(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := RootCmd()
	cmd.SetArgs(append([]string{"onboard"}, args...))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err := cmd.Execute()
	return out.String(), err
}

func templatesWritten(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(templatesDir())
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestOnboardRefusesHostFileIOInMCPChild(t *testing.T) {
	home := testenv.Isolate(t)
	t.Setenv(cliutil.MCPChildEnvVar, "1")
	tpl := filepath.Join(home, "house.json")
	if err := os.WriteFile(tpl, []byte(`{"folders":[{"name":"Runbooks"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"--init"},
		{"--init", "--template", "msp-standard"},
		{"--init", "--dry-run"},
		{"--company", "42", "--template", tpl},
		{"--company", "42", "--template", tpl, "--dry-run"},
	} {
		out, err := runOnboardForTest(t, args...)
		if err == nil {
			t.Errorf("onboard %v ran in a process the MCP server spawned", args)
		}
		if strings.Contains(out, "Runbooks") || strings.Contains(out, "create-folder") {
			t.Errorf("onboard %v read the template in an MCP child:\n%s", args, out)
		}
	}
	if names := templatesWritten(t); len(names) > 0 {
		t.Errorf("onboard wrote %v under %s in an MCP child", names, templatesDir())
	}
}

// TestOnboardApplyTargetsOnlyTheSelectedCompany (codex r1 P2, target-scope
// mismatch): a template folder that carries its own company_id must not
// retarget the write away from the --company the preview named.
func TestOnboardApplyTargetsOnlyTheSelectedCompany(t *testing.T) {
	home := testenv.Isolate(t)
	t.Setenv(cliutil.MCPChildEnvVar, "")
	var mu sync.Mutex
	var folderBodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/folders" {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			folderBodies = append(folderBodies, body)
			mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"folder":{"id":7}}`))
	}))
	defer srv.Close()
	t.Setenv("HUDU_BASE_URL", srv.URL)
	t.Setenv("HUDU_API_KEY", "fixture-key")

	tpl := filepath.Join(home, "house.json")
	if err := os.WriteFile(tpl, []byte(`{"folders":[{"name":"Runbooks","company_id":43}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := runOnboardForTest(t, "--company", "42", "--template", tpl, "--apply"); err != nil {
		t.Fatalf("onboard --apply: %v\n%s", err, out)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(folderBodies) != 1 {
		t.Fatalf("POST /folders sent %d times, want 1", len(folderBodies))
	}
	if got := folderBodies[0]["company_id"]; got != float64(42) {
		t.Errorf("folder created for company_id %v, want the selected --company 42", got)
	}
}

func TestOnboardInitHonorsDryRun(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv(cliutil.MCPChildEnvVar, "")
	if _, err := runOnboardForTest(t, "--init", "--dry-run"); err != nil {
		t.Fatalf("onboard --init --dry-run: %v", err)
	}
	if names := templatesWritten(t); len(names) > 0 {
		t.Errorf("onboard --init --dry-run wrote %v under %s", names, templatesDir())
	}
	// Without --dry-run, --init still writes the example (the CLI keeps it).
	if _, err := runOnboardForTest(t, "--init"); err != nil {
		t.Fatalf("onboard --init: %v", err)
	}
	if names := templatesWritten(t); len(names) != 1 || names[0] != "msp-standard.json" {
		t.Errorf("onboard --init wrote %v, want [msp-standard.json]", names)
	}
}

func TestOnboardInitKeepsAnEditedTemplate(t *testing.T) {
	testenv.Isolate(t)
	t.Setenv(cliutil.MCPChildEnvVar, "")
	if _, err := runOnboardForTest(t, "--init"); err != nil {
		t.Fatalf("onboard --init: %v", err)
	}
	path := filepath.Join(templatesDir(), "msp-standard.json")
	edited := []byte(`{"name":"operator edit"}`)
	if err := os.WriteFile(path, edited, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runOnboardForTest(t, "--init"); err == nil {
		t.Fatal("onboard --init over an existing template succeeded without --yes")
	}
	if got, _ := os.ReadFile(path); !bytes.Equal(got, edited) {
		t.Fatalf("onboard --init replaced the edited template without --yes: %s", got)
	}
	if _, err := runOnboardForTest(t, "--init", "--yes"); err != nil {
		t.Fatalf("onboard --init --yes: %v", err)
	}
	if got, _ := os.ReadFile(path); bytes.Equal(got, edited) {
		t.Fatal("onboard --init --yes kept the old template")
	}
}
