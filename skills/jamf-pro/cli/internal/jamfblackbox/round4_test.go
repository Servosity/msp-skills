// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package jamfblackbox

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A registered instance keeps its own run profiles. A credential saved there
// under a credential flag's name is held at startup like the default home's,
// so selecting the instance and printing the profile shows no secret.
func TestInstanceProfileStoreIsHeld(t *testing.T) {
	t.Parallel()
	_, e := honestEnv(t, nil)
	e.run(t, "--instance", "acme", "profile", "save", "seed", "--json", "--compact")
	var store string
	_ = filepath.WalkDir(e.home, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.Name() == "profiles.json" && strings.Contains(p, "instance") {
			store = p
		}
		return nil
	})
	if store == "" {
		t.Fatal("an instance profile save wrote no profiles.json under the instance's home")
	}
	const saved = "InstProfSecret-7q2w9e4r"
	body := `{"profiles":{"p":{"name":"p","values":{"client-secret":"` + saved + `"}}}}`
	if err := os.WriteFile(store, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	elsewhere := t.TempDir()
	// The registry lives in the default config dir; name it, since the cases
	// below move the config dir.
	fleetDir := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(store)))) // <base>/instance/<name>/<suffix>/config
	fleetDir = filepath.Dir(fleetDir)
	fd := "JAMF_PRO_FLEET_DIR=" + fleetDir
	for _, tc := range []struct {
		env  []string
		args []string
	}{
		{nil, []string{"--instance", "acme", "--json=false", "profile", "use", "p"}},
		{nil, []string{"--instance", "acme", "profile", "show", "p", "--json"}},
		// Pinning an instance drops every JAMF_PRO_* path variable, so startup
		// must read the instance's store as the pinned run will.
		{[]string{fd, "JAMF_PRO_CONFIG_DIR=" + elsewhere}, []string{"--instance", "acme", "--json=false", "profile", "use", "p"}},
		{[]string{fd, "JAMF_PRO_CONFIG=" + filepath.Join(elsewhere, "config.toml")}, []string{"--instance", "acme", "--json=false", "profile", "use", "p"}},
		{[]string{fd, "JAMF_PRO_INSTANCE=acme", "JAMF_PRO_CONFIG_DIR=" + elsewhere}, []string{"--json=false", "profile", "use", "p"}},
	} {
		args := tc.args
		out := e.with(tc.env...).run(t, args...)
		if !strings.Contains(out, `"p"`) && !strings.Contains(out, "client-secret") && !strings.Contains(out, "withheld") {
			t.Fatalf("%v %v did not print the profile, so it proves nothing:\n%.800s", tc.env, args, out)
		}
		for _, leak := range []string{saved, saved[:8], saved[len(saved)-8:]} {
			if strings.Contains(out, leak) {
				t.Fatalf("%v %v printed the instance profile's saved secret:\n%.800s", tc.env, args, out)
			}
		}
	}
}

// Deleting learnings is destructive: over MCP it previews, and deletes only
// with confirm set to boolean true.
func TestMCPLearningsForgetNeedsConfirm(t *testing.T) {
	t.Parallel()
	_, e := honestEnv(t, nil)
	e.run(t, "teach", "--query", "review confirm test", "--resource", "17", "--resource-type", "computer_inventory", "--no-validate")
	rows := func() string { return e.run(t, "learnings", "list", "--json") }
	if !strings.Contains(rows(), "review confirm test") {
		t.Fatal("teach stored no learning")
	}
	out := e.mcpCalls(t, []map[string]any{
		{"name": "learnings_forget", "arguments": map[string]any{"query": "review confirm test", "all": true}},
		{"name": "learnings_forget", "arguments": map[string]any{"query": "review confirm test", "all": true, "confirm": "true"}},
		// The preview's own --dry-run cannot be turned off by an argument.
		{"name": "learnings_forget", "arguments": map[string]any{"query": "review confirm test", "all": true, "dry-run": "false"}},
		{"name": "learnings_forget", "arguments": map[string]any{"query": "review confirm test", "all": true, "dry-run": false}},
	})
	if !strings.Contains(rows(), "review confirm test") {
		t.Fatalf("learnings_forget deleted without confirm: true:\n%.1500s", out)
	}
	e.mcpCalls(t, []map[string]any{
		{"name": "learnings_forget", "arguments": map[string]any{"query": "review confirm test", "all": true, "confirm": true}},
	})
	if strings.Contains(rows(), "review confirm test") {
		t.Fatal("learnings_forget with confirm: true deleted nothing")
	}
}

// An export that reads fewer records than the server's totalCount says is not
// complete: it fails instead of exiting 0 with records missing.
func TestExportShortOfTotalCountFails(t *testing.T) {
	t.Parallel()
	_, e := honestEnv(t, map[string]string{
		"/api/v1/buildings": `{"totalCount":2,"results":[{"id":"1","name":"A"}]}`,
	})
	for _, format := range []string{"json", "jsonl"} {
		out, code := e.runCode(t, "export", "buildings", "--format", format)
		if code == 0 {
			t.Fatalf("export --format %s read 1 of a reported 2 records and exited 0:\n%.800s", format, out)
		}
		if !strings.Contains(out, "of a reported 2") {
			t.Fatalf("export --format %s does not say the walk was short:\n%.800s", format, out)
		}
	}
	// An empty first page under a positive total, and a repeated page that
	// makes the count match, are short walks too.
	for name, js := range map[string]map[string]string{
		"empty first page": {"/api/v1/buildings?page=0": `{"totalCount":2,"results":[]}`},
		"repeated page": {
			"/api/v1/buildings?page=0": `{"totalCount":2,"results":[{"id":"1","name":"A"}]}`,
			"/api/v1/buildings?page=1": `{"totalCount":2,"results":[{"id":"1","name":"A"}]}`,
			"/api/v1/buildings?page=2": `{"totalCount":2,"results":[]}`,
		},
	} {
		srv, env := honestEnv(t, nil)
		srv.pages = js
		if out, code := env.runCode(t, "export", "buildings", "--format", "json"); code == 0 || !strings.Contains(out, "incomplete walk") {
			t.Fatalf("%s: export did not fail as a short walk (exit %d):\n%.800s", name, code, out)
		}
	}
	// A complete walk still exits 0.
	_, full := honestEnv(t, map[string]string{
		"/api/v1/buildings": `{"totalCount":1,"results":[{"id":"1","name":"A"}]}`,
	})
	if out, code := full.runCode(t, "export", "buildings", "--format", "json"); code != 0 {
		t.Fatalf("a complete export failed (%d):\n%.800s", code, out)
	}
}

// runCode is run with the exit code.
func (e *bbEnv) runCode(t *testing.T, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(bbCLI, args...)
	cmd.Env = e.env
	cmd.Dir = e.home
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	return out.String() + "\n" + errb.String(), code
}
