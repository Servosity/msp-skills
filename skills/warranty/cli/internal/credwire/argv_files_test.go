// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated): Codex publish pass 2 round 2,
// startup-config-discovery. Declarations.Files reads the command line the
// way the program will: a flag's value is never mistaken for a flag, and a
// command line it cannot read exactly names every file it could mean.

package credwire

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolatedEnv points every path lookup at a throwaway home.
func isolatedEnv(t *testing.T) string {
	t.Helper()
	root, err := os.MkdirTemp("", "dct-argv-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, "XDG_") || strings.HasPrefix(name, "WARRANTY_") || strings.HasPrefix(name, "PRINTING_PRESS_") {
			t.Setenv(name, "")
			_ = os.Unsetenv(name)
		}
	}
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
	return root
}

// Only the files the program reads decide the base URL. Each case names the file.
func TestCredentialFilesDecideFromWhatTheProgramReads(t *testing.T) {
	root := isolatedEnv(t)
	real := filepath.Join(root, "real")
	fake := filepath.Join(root, "fake")
	realCfg := filepath.Join(real, "config", "config.toml")
	if err := os.MkdirAll(filepath.Dir(realCfg), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(realCfg, []byte("auth_header = \"x\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := filepath.Join(root, "a.toml")
	for _, tc := range []struct {
		name  string
		args  []string
		env   map[string]string
		first string
	}{
		{name: "the last --home wins", args: []string{"--home", fake, "teach", "--home=" + real, "--notes", "n"}, first: realCfg},
		{name: "the MCP server reads the config environment", args: []string{"--transport", "http"}, env: map[string]string{"WARRANTY_CONFIG": a}, first: a},
		{name: "the MCP server reads WARRANTY_HOME", args: []string{"--transport", "stdio"}, env: map[string]string{"WARRANTY_HOME": real}, first: realCfg},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			if got := credentialFiles(tc.args); len(got) == 0 || got[0] != tc.first {
				t.Errorf("files %q, want first %q", got, tc.first)
			}
		})
	}
}

// A base_url in a file the program does not read is no allowed origin. Input: the
// default config names another host, the run reads another file or home that names
// none, and the token URL points at that host. The Dell secret must not go there. Both
// directions: when the file the program reads names that host, the secret does go.
func TestUnreadConfigGivesNoOrigin(t *testing.T) {
	cli, _ := binaries(t)
	basic := "Basic " + base64.StdEncoding.EncodeToString([]byte(stubDellClient+":"+stubDellSecret))
	gateway := "base_url = \"https://" + hostGateway + "\"\n"
	setup := func(t *testing.T) (sandbox, *vendors, []string) {
		s := newSandbox(t)
		v := newVendors(t, s)
		cfg := filepath.Join(s.home, "config", "config.toml")
		if err := os.MkdirAll(filepath.Dir(cfg), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(cfg, []byte(gateway), 0o600); err != nil {
			t.Fatal(err)
		}
		return s, v, s.env(append(v.env(), "WARRANTY_DELL_CLIENT_ID="+stubDellClient, "WARRANTY_DELL_CLIENT_SECRET="+stubDellSecret,
			"WARRANTY_TOKEN_URL=https://"+hostGateway+"/token")...)
	}
	lookup := []string{"lookup", "1ABC234", "--vendor", "dell", "--no-save", "--json", "--no-learn"}
	for name, extra := range map[string]func(s sandbox) []string{
		"--config": func(s sandbox) []string { return []string{"--config", s.writePrivate(t, "a.toml", "timeout = 30\n")} },
		"--home":   func(s sandbox) []string { return []string{"--home", filepath.Join(s.root, "h2")} },
	} {
		t.Run(name, func(t *testing.T) {
			s, v, env := setup(t)
			run(t, cli, env, append(extra(s), lookup...)...)
			if v.carried(hostGateway, basic) {
				t.Errorf("the Dell secret went to a host only an unread config names")
			}
		})
	}
	t.Run("the config the program reads", func(t *testing.T) {
		s, v, env := setup(t)
		res := run(t, cli, env, append([]string{"--config", s.writePrivate(t, "b.toml", gateway)}, lookup...)...)
		if !v.carried(hostGateway, basic) {
			t.Errorf("a base URL the program reads did not allow its own token URL: %+v %+v", res, v.seen)
		}
	})
}

// The reviewer's input through the real binary: auth_header in
// <real>/config/config.toml, no config environment variable and no fake or
// legacy config, then
//
//	warranty-cli --home <real> teach --notes --home=<fake> --query lowercasecredential123 --resource-type domain --resource example.com
//
// Cobra gives "--home=<fake>" to --notes, so the program reads <real>'s
// config; the credential in it must be held, so the query that carries it
// never reaches <real>/state/learnings.jsonl.
func TestKitStartupConfigDiscoveryHonoursFlagValues(t *testing.T) {
	cli, _ := binaries(t)
	const secret = "lowercasecredential123"
	s := newSandbox(t)
	real := filepath.Join(s.root, "real")
	fake := filepath.Join(s.root, "fake")
	cfg := filepath.Join(real, "config", "config.toml")
	if err := os.MkdirAll(filepath.Dir(cfg), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, []byte("auth_header=\""+secret+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var env []string
	for _, kv := range s.env() {
		if !strings.HasPrefix(kv, "WARRANTY_HOME=") {
			env = append(env, kv)
		}
	}
	args := []string{"--home", real, "teach", "--notes", "--home=" + fake, "--query", secret, "--resource-type", "domain", "--resource", "example.com"}
	res := run(t, cli, env, args...)
	requireAbsent(t, "teach stdout", res.stdout, secret)
	requireAbsent(t, "teach stderr", res.stderr, secret)
	if hits := filesHolding(t, s.root, secret, cfg); len(hits) > 0 {
		t.Errorf("teach persisted the credential to %v (exit %d, stderr=%q)", hits, res.code, res.stderr)
	}
	// The same command with an ordinary query writes it, so the absence
	// above is the credential being held, not a teach that writes nothing.
	// (teach stores the query normalized: "an" is dropped.)
	args[6] = "an ordinary query"
	res = run(t, cli, env, args...)
	if hits := filesHolding(t, filepath.Join(real, "state"), "ordinary query"); len(hits) == 0 {
		t.Errorf("teach wrote nothing under <real>/state even without a credential, so this test proved nothing (exit %d, stderr=%q)", res.code, res.stderr)
	}
}

// A credential store's base_url is the server a credential was saved for, never
// the base URL a run uses. Input: no config.toml, credentials.toml in the data
// directory naming a gateway host, and a token URL on that host. The Dell secret
// must not go there: the store does not make the host an allowed origin.
func TestCredentialStoreBaseURLGivesNoOrigin(t *testing.T) {
	cli, _ := binaries(t)
	basic := "Basic " + base64.StdEncoding.EncodeToString([]byte(stubDellClient+":"+stubDellSecret))
	s := newSandbox(t)
	v := newVendors(t, s)
	creds := filepath.Join(s.home, "data", "credentials.toml")
	if err := os.MkdirAll(filepath.Dir(creds), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(creds, []byte("base_url = \"https://"+hostGateway+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	env := s.env(append(v.env(), "WARRANTY_DELL_CLIENT_ID="+stubDellClient, "WARRANTY_DELL_CLIENT_SECRET="+stubDellSecret,
		"WARRANTY_TOKEN_URL=https://"+hostGateway+"/token")...)
	run(t, cli, env, "lookup", "1ABC234", "--vendor", "dell", "--no-save", "--json", "--no-learn")
	if v.carried(hostGateway, basic) {
		t.Errorf("the Dell secret went to a host only the credential store names")
	}
}
