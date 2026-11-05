// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package prtgblackbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Class config-parse-before-hold (prtg handfix: config-parse-holds-first): a
// credential in a config-like file the program parses at startup (the
// explicit and default config, the legacy config, the credential store, the
// fleet registry and the saved profiles) never reaches the output of the CLI
// or of the MCP server, even when the file does not parse: every value is
// held before the parse can fail, and a parse error names the file and the
// line, never the content.
const (
	bbReviewKey = "prtgReview" + "Key-7f319a"
	bbReviewPin = "84173" + "059261234" // a numeric credential a JSON type error would print
)

// bareEnv is an isolated home with no key in the environment, so nothing but
// the file under test can tell the program the credential.
func bareEnv(t *testing.T) *bbEnv {
	t.Helper()
	home := t.TempDir()
	tmp := filepath.Join(home, "tmp")
	_ = os.MkdirAll(tmp, 0o700)
	return &bbEnv{home: home, env: []string{
		"PATH=" + os.Getenv("PATH"), "SYSTEMROOT=" + os.Getenv("SYSTEMROOT"),
		"HOME=" + home, "USERPROFILE=" + home, "APPDATA=" + filepath.Join(home, "AppData", "Roaming"),
		"LOCALAPPDATA=" + filepath.Join(home, "AppData", "Local"),
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"), "XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"), "XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"TMPDIR=" + tmp, "TMP=" + tmp, "TEMP=" + tmp,
		"PRTG_HOME=" + filepath.Join(home, "prtg"), "PRTG_SERVER=prtg.invalid",
	}}
}

func writePrivate(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Chmod(path, 0o600)
}

// assertNoEchoOf fails when text holds s in any form the server tests use,
// or both of its halves.
func assertNoEchoOf(t *testing.T, where, text, s string) {
	t.Helper()
	for _, f := range echoForms(s) {
		if strings.Contains(text, f) {
			t.Errorf("%s holds the file's credential as %q:\n%.1500s", where, f, text)
			return
		}
	}
	if a, b := halves(s); strings.Contains(text, a) && strings.Contains(text, b) {
		t.Errorf("%s holds both halves of the file's credential:\n%.1500s", where, text)
	}
}

// malformedTOML repeats the credential as a table name the parser rejects
// as a duplicate (go-toml printed "table <name> already exists").
func malformedTOML(key string) string {
	return fmt.Sprintf("api_key = %q\n[%s]\n[%s]\n", key, key, key)
}

func TestMalformedConfigNeverEchoes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		file   func(e *bbEnv) string // the file under test, relative to the home
		body   string
		key    string
		args   [][]string
		mcp    bool   // the MCP server reads the same file (PRTG_CONFIG or the default paths)
		where  string // the error must name this file
		config bool   // pass the file as --config / PRTG_CONFIG
	}{
		{
			name: "explicit config", file: func(e *bbEnv) string { return filepath.Join(e.home, "x.toml") },
			body: malformedTOML(bbReviewKey), key: bbReviewKey, config: true, mcp: true,
			args: [][]string{{"auth", "status", "--json"}, {"doctor"}, {"sensors", "list-experimental", "--json"}},
		},
		{
			name: "default config", file: func(e *bbEnv) string { return filepath.Join(e.home, "prtg", "config", "config.toml") },
			body: malformedTOML(bbReviewKey), key: bbReviewKey, mcp: true,
			args: [][]string{{"auth", "status", "--json"}, {"doctor"}},
		},
		{
			name: "legacy config", file: func(e *bbEnv) string { return filepath.Join(e.home, ".config", "prtg-cli", "config.toml") },
			body: malformedTOML(bbReviewKey), key: bbReviewKey, mcp: true,
			args: [][]string{{"auth", "status", "--json"}, {"doctor"}},
		},
		{
			name: "credential store", file: func(e *bbEnv) string { return filepath.Join(e.home, "prtg", "data", "credentials.toml") },
			body: malformedTOML(bbReviewKey), key: bbReviewKey, mcp: true,
			args: [][]string{{"auth", "status", "--json"}, {"doctor"}, {"sensors", "list-experimental", "--json"}},
		},
		{
			name: "fleet registry (invalid server)", file: func(e *bbEnv) string { return filepath.Join(e.home, "prtg", "config", "fleet.json") },
			body: `{"cores":[{"name":"acme","server":"` + bbReviewKey + `/x"}]}`, key: bbReviewKey, mcp: true,
			args: [][]string{{"core", "list", "--json"}, {"fleet", "status", "--json"}},
		},
		{
			name: "fleet registry (type error)", file: func(e *bbEnv) string { return filepath.Join(e.home, "prtg", "config", "fleet.json") },
			body: `{"cores":[{"name":` + bbReviewPin + `,"server":"prtg.invalid"}]}`, key: bbReviewPin, mcp: true,
			args: [][]string{{"core", "list", "--json"}, {"fleet", "status", "--json"}},
		},
		{
			name: "profiles", file: func(e *bbEnv) string { return filepath.Join(e.home, "prtg", "config", "profiles.json") },
			body: `{"profiles":{"p":{"name":"p","values":{"password":` + bbReviewPin + `}}}}`, key: bbReviewPin,
			args: [][]string{{"profile", "list", "--json"}, {"--profile", "p", "sensors", "list-experimental", "--json"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := bareEnv(t)
			path := tc.file(e)
			writePrivate(t, path, tc.body)
			if tc.config {
				e.env = append(e.env, "PRTG_CONFIG="+path)
			}
			var all string
			for _, args := range tc.args {
				out := e.run(t, args...)
				assertNoEchoOf(t, fmt.Sprintf("output of %v", args), out, tc.key)
				all += out
			}
			if !strings.Contains(all, filepath.Base(path)) {
				t.Errorf("no command named the file that failed to parse (%s):\n%.2000s", filepath.Base(path), all)
			}
			if tc.mcp {
				out := e.mcpCalls(t, []map[string]any{
					{"name": "prtg_read", "arguments": map[string]any{"endpoint_id": "sensors.list-experimental"}},
					{"name": "core_list", "arguments": map[string]any{}},
					{"name": "fleet_exec", "arguments": map[string]any{"path": "/version"}},
				})
				assertNoEchoOf(t, "MCP session", out, tc.key)
				if !strings.Contains(out, `"id":3`) {
					t.Errorf("MCP session did not answer every call:\n%.2000s", out)
				}
			}
		})
	}
}

// The parse error still tells the operator where to look: the file and the
// line, so a malformed file is fixable without the program printing it.
func TestMalformedConfigNamesFileAndLine(t *testing.T) {
	e := bareEnv(t)
	path := filepath.Join(e.home, "x.toml")
	writePrivate(t, path, fmt.Sprintf("api_key = %q\nbroken =\n", bbReviewKey))
	out, code := e.runCode(t, "--config", path, "auth", "status", "--json")
	if code == 0 || !strings.Contains(out, path) || !strings.Contains(out, "line 2") {
		t.Fatalf("want a non-zero exit naming %s and line 2, got exit %d:\n%s", path, code, out)
	}
	assertNoEchoOf(t, "auth status", out, bbReviewKey)
}
