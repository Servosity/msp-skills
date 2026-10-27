// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): where a credential may go, and what a body or a
// saved profile gives the holder, driven through the real binary
// (credkit-wiring). The proxy is a loopback listener that answers nothing, so
// nothing here leaves the machine: a request the kit lets through is one
// connection to it, and a withheld error cannot say which it was.

package main

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// recordingProxy counts the connections a run makes to it.
func recordingProxy(t *testing.T) (string, *atomic.Int32) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	var n atomic.Int32
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			n.Add(1)
			_ = c.Close()
		}
	}()
	return "http://" + l.Addr().String(), &n
}

func TestACredentialGoesOnlyToTheSelectedOrigin(t *testing.T) {
	t.Parallel()
	cliBin := buildHolderBinary(t, "superops-cli", "../superops-cli")
	const heldValue = "CobaltAuthSecret4340"
	webhook := func(origin string) []string {
		return []string{"--no-learn", "version", "--deliver", "webhook:" + origin + "/ingest/" + heldValue}
	}
	query := []string{"--no-learn", "raw", "query", "{ __typename }"}

	for _, tc := range []struct {
		name    string
		vars    map[string]string
		config  string // the file --config names, when set
		other   string // a discovered default config, when set
		args    []string
		reached bool
	}{
		{name: "another port of the selected host", vars: map[string]string{"SUPEROPS_BASE_URL": "https://api.superops.ai:8443"}, args: webhook("https://api.superops.ai")},
		{name: "the EU host under a custom base URL", vars: map[string]string{"SUPEROPS_BASE_URL": "https://custom.example"}, args: webhook("https://euapi.superops.ai")},
		{name: "the EU host under the default", args: webhook("https://euapi.superops.ai")},
		{name: "the US host when the region is EU", vars: map[string]string{"SUPEROPS_REGION": "eu"}, args: webhook("https://api.superops.ai")},
		{name: "the host a discovered default names under --config", config: "region = \"us\"\n", other: "base_url = \"https://other.example\"\n", args: webhook("https://other.example")},

		{name: "the default host", args: query, reached: true},
		{name: "a custom base URL", vars: map[string]string{"SUPEROPS_BASE_URL": "https://custom.example"}, args: query, reached: true},
		{name: "the EU host, region from the environment", vars: map[string]string{"SUPEROPS_REGION": "eu"}, args: query, reached: true},
		{name: "the EU host, region from the config file", config: "region = \"eu\"\n", args: query, reached: true},
		{name: "the EU host when the environment names the US host", vars: map[string]string{"SUPEROPS_REGION": "eu", "SUPEROPS_BASE_URL": "https://api.superops.ai"}, args: query, reached: true},
		{name: "the default host under --config past a discovered default", config: "region = \"us\"\n", other: "base_url = \"https://other.example\"\n", args: query, reached: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sandbox := t.TempDir()
			proxy, connections := recordingProxy(t)
			vars := map[string]string{"SUPEROPS_API_TOKEN": heldValue, "SUPEROPS_SUBDOMAIN": "acme-holder", "SUPEROPS_CONFIG_DIR": "{sandbox}/cfg", "HTTPS_PROXY": proxy}
			for k, v := range tc.vars {
				vars[k] = v
			}
			if err := os.MkdirAll(filepath.Join(sandbox, "cfg"), 0o700); err != nil {
				t.Fatal(err)
			}
			args := tc.args
			if tc.other != "" {
				writeHolderFile(t, filepath.Join(sandbox, "cfg", "config.toml"), tc.other)
			}
			if tc.config != "" {
				path := filepath.Join(sandbox, "chosen.toml")
				writeHolderFile(t, path, tc.config)
				args = append([]string{"--config", path}, args...)
			}
			out := runHolderCLI(t, cliBin, holderEnv(sandbox, vars), "", args...)
			if got := connections.Load() > 0; got != tc.reached {
				t.Errorf("reached the proxy=%v, want %v:\n%s", got, tc.reached, out)
			}
			if strings.Contains(out, heldValue) {
				t.Errorf("the token was printed:\n%s", out)
			}
		})
	}
}

func writeHolderFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestABodysCredentialFieldsAreHeldBeforeItPrints(t *testing.T) {
	t.Parallel()
	cliBin := buildHolderBinary(t, "superops-cli", "../superops-cli")
	base := map[string]string{"SUPEROPS_BASE_URL": "http://127.0.0.1:9", "SUPEROPS_SUBDOMAIN": "acme-holder", "SUPEROPS_CONFIG_DIR": "{sandbox}/cfg", "SUPEROPS_STATE_DIR": "{sandbox}/st"}

	for _, tc := range []struct {
		name, stdin string
		args        []string
	}{
		{"a --vars - preview", `{"client_secret":"CobaltField4340"}`, []string{"raw", "query", "{ __typename }", "--vars", "-", "--dry-run"}},
		{"a nested --vars - preview", `{"input":{"password":"CobaltField4340"}}`, []string{"raw", "query", "{ __typename }", "--vars", "-", "--dry-run"}},
		{"a numeric field", `{"password":40123456789}`, []string{"raw", "query", "{ __typename }", "--vars", "-", "--dry-run"}},
		{"a field holding a list", `{"password":["CobaltField4340"]}`, []string{"raw", "query", "{ __typename }", "--vars", "-", "--dry-run"}},
		{"an inline --vars preview", "", []string{"raw", "query", "{ __typename }", "--vars", `{"password":40123456789}`, "--dry-run"}},
		{"a --vars - mutation preview", `{"input":{"token":"CobaltField4340"}}`, []string{"raw", "mutation", "mutation { a }", "--vars", "-"}},
		{"a duplicate member", `{"password":{"value":40123456789,"value":""}}`, []string{"raw", "query", "{ __typename }", "--vars", "-", "--dry-run"}},
		{"a field past many others", `{"pad":[` + strings.Repeat(`{"token":null},`, 255) + `{"token":null}],"password":40123456789}`, []string{"raw", "query", "{ __typename }", "--vars", "-", "--dry-run"}},
		{"a form body", `grant=x&client_secret=CobaltField4340&scope=y`, []string{"raw", "query", "{ __typename }", "--vars", "-", "--dry-run"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := runHolderCLI(t, cliBin, holderEnv(t.TempDir(), base), tc.stdin, append([]string{"--no-learn"}, tc.args...)...)
			if strings.Contains(out, "CobaltField4340") || strings.Contains(out, "40123456789") {
				t.Errorf("the credential field was printed:\n%s", out)
			}
		})
	}

	// Holding a whole note would refuse every note, so a plain one must record.
	t.Run("a plain feedback --stdin note is recorded", func(t *testing.T) {
		t.Parallel()
		out := runHolderCLI(t, cliBin, holderEnv(t.TempDir(), base), "pagination tripped me up", "--no-learn", "feedback", "--stdin")
		if !strings.Contains(out, "recorded") || strings.Contains(out, "not recorded") {
			t.Errorf("not recorded:\n%s", out)
		}
	})
}

func TestASavedProfileIsHeldAtStartup(t *testing.T) {
	t.Parallel()
	cliBin := buildHolderBinary(t, "superops-cli", "../superops-cli")
	base := map[string]string{"SUPEROPS_BASE_URL": "http://127.0.0.1:9", "SUPEROPS_SUBDOMAIN": "acme-holder", "SUPEROPS_CONFIG_DIR": "{sandbox}/cfg", "SUPEROPS_STATE_DIR": "{sandbox}/st"}
	plant := func(t *testing.T, store string) (string, []string) {
		sandbox := t.TempDir()
		if err := os.MkdirAll(filepath.Join(sandbox, "cfg"), 0o700); err != nil {
			t.Fatal(err)
		}
		writeHolderFile(t, filepath.Join(sandbox, "cfg", "profiles.json"), store)
		return sandbox, holderEnv(sandbox, base)
	}

	// feedback never opens the store, so only a startup hold can refuse these.
	const heldNumber, otherValue = "40123456789", "OtherVal9876"
	for _, tc := range []struct{ name, store, heldValue string }{
		{"a value under a credential name", `{"profiles":{"p":{"name":"p","values":{"token":"CobaltVal4340"}}}}`, "CobaltVal4340"},
		{"every member of a duplicate key", `{"profiles":{"p":{"name":"p","values":{"password":"CobaltVal4340","password":"` + otherValue + `"}}}}`, "CobaltVal4340"},
		{"a number", `{"profiles":{"p":{"name":"p","values":{"api_key":` + heldNumber + `}}}}`, heldNumber},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, env := plant(t, tc.store)
			out := runHolderCLI(t, cliBin, env, "", "--no-learn", "feedback", "see "+tc.heldValue)
			if !strings.Contains(out, "not recorded") {
				t.Errorf("not held at startup:\n%s", out)
			}
		})
	}

	const store = `{"profiles":{"p":{"name":"p","values":{"not-a-flag":"CobaltStray4340","select":"ticket.subject"}}}}`
	t.Run("a key naming no flag is dropped", func(t *testing.T) {
		t.Parallel()
		_, env := plant(t, store)
		for _, args := range [][]string{{"profile", "show", "p", "--json"}, {"profile", "use", "p"}, {"profile", "list", "--json"}} {
			out := runHolderCLI(t, cliBin, env, "", append([]string{"--no-learn"}, args...)...)
			if strings.Contains(out, "CobaltStray4340") || strings.Contains(out, "not-a-flag") {
				t.Errorf("%v printed the stray value:\n%s", args, out)
			}
		}
	})
	t.Run("a real flag's value stays", func(t *testing.T) {
		t.Parallel()
		_, env := plant(t, store)
		if out := runHolderCLI(t, cliBin, env, "", "--no-learn", "profile", "show", "p", "--json"); !strings.Contains(out, "ticket.subject") {
			t.Errorf("the saved select was lost:\n%s", out)
		}
	})
}

// A config file that does not parse is one of Declarations.Files, which the
// kit reads token by token at startup: no hand-written reader is needed. A file
// whose every string is held may make the kit refuse to start (exit 78), which
// prints no credential either.
func TestAConfigThatDoesNotParseIsHeldAtStartup(t *testing.T) {
	t.Parallel()
	cliBin := buildHolderBinary(t, "superops-cli", "../superops-cli")
	for _, tc := range []struct {
		name, config string
		recorded     bool // the run starts, so the feedback refusal shows the hold
	}{
		{"a broken line", "api_token = \"MalformedTok4340x\"\nretries = = broken\n", true},
		{"an unclosed string", "api_token = \"MalformedTok4340x\n", true},
		{"a broken table", "[headers\nX-Api-Key = \"MalformedTok4340x\"\n", false},
		{"a broken base URL", "base_url = \"https://u:MalformedTok4340x@api.superops.ai\"\n[[x\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sandbox := t.TempDir()
			if err := os.MkdirAll(filepath.Join(sandbox, "cfg"), 0o700); err != nil {
				t.Fatal(err)
			}
			writeHolderFile(t, filepath.Join(sandbox, "cfg", "config.toml"), tc.config)
			env := holderEnv(sandbox, map[string]string{"SUPEROPS_SUBDOMAIN": "acme-holder", "SUPEROPS_CONFIG_DIR": "{sandbox}/cfg"})
			out := runHolderCLI(t, cliBin, env, "", "--no-learn", "feedback", "see MalformedTok4340x")
			if tc.recorded && !strings.Contains(out, "not recorded") {
				t.Errorf("not held at startup:\n%s", out)
			}
			out += runHolderCLI(t, cliBin, env, "", "--no-learn", "doctor", "--json")
			assertRunWithheld(t, sandbox, out, []string{"MalformedTok4340x"})
		})
	}
}
