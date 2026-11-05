// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Each class the last review of the connector's own filter named, driven
// through the built binaries with the credential kit wired in
// (handfixes.json credkit-wiring). The kit closes the class, not the input:
// a held value in any reading the kit knows withholds the whole unit.

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const kitReadArgs = "hubspot-contacts-crm get-v3-objects-contacts-contact-id 1 --json --data-source live"

func TestKitWithholdsEveryReadingOfAHeldValue(t *testing.T) {
	if testing.Short() {
		t.Skip("builds both binaries")
	}
	cliBin, mcpBin := nb2Binaries(t)
	cases := []struct {
		name, token, echo string
		absent            []string // what must not reach either output
	}{
		// base64 of the token wrapped over two lines inside one JSON value.
		{"multi-line-json-value", "ABCDEFGH", "QUJD\nREVGR0g=", []string{"QUJD", "REVGR0g="}},
		// Any 8-character piece of a held value counts, not only the whole.
		{"eight-character-piece", "Tk9mQ2xV7pLw4RzN8sYb", "piece Tk9mQ2xV7pLw here", []string{"Tk9mQ2xV7pLw"}},
		{"hex-of-a-piece", "Tk9mQ2xV7pLw4RzN8sYb", "546b396d51327856374c77", []string{"546b396d5132"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			srv, _ := nb2Portal(t, map[string]string{"note": tc.echo})
			env := nb2Env(home, srv.URL, cliBin, "HUBSPOT_ACCESS_TOKEN="+tc.token)
			out, stderr, _ := nb2Run(t, cliBin, env, "", strings.Fields(kitReadArgs)...)
			mcpOut := nb2MCPRead(t, mcpBin, env)
			for where, got := range map[string]string{"cli": out + stderr, "mcp": mcpOut} {
				for _, a := range tc.absent {
					if strings.Contains(got, a) {
						t.Errorf("%s printed the held value (%q):\n%s", where, a, got)
					}
				}
				if !strings.Contains(got, "credkit: withheld") {
					t.Errorf("%s: want the kit's notice:\n%s", where, got)
				}
			}
		})
	}
}

// A credential spelled like a JSON scalar cannot be told from public text, so
// the program refuses to start: it sends nothing and prints no value.
func TestKitRefusesACredentialThatIsAPublicScalar(t *testing.T) {
	if testing.Short() {
		t.Skip("builds both binaries")
	}
	cliBin, mcpBin := nb2Binaries(t)
	for _, scalar := range []string{"false", "true", "null"} {
		for _, source := range []string{"env", "config"} {
			t.Run(scalar+"-"+source, func(t *testing.T) {
				home := t.TempDir()
				srv, hits := nb2Portal(t, nil)
				env := nb2Env(home, srv.URL, cliBin)
				if source == "env" {
					env = append(env, "HUBSPOT_ACCESS_TOKEN="+scalar)
				} else {
					nb2WriteConfig(t, home, "access_token = \""+scalar+"\"\n")
				}
				for bin, args := range map[string][]string{cliBin: strings.Fields(kitReadArgs), mcpBin: nil} {
					out, stderr, err := nb2Run(t, bin, env, "", args...)
					var exit *exec.ExitError
					if !errors.As(err, &exit) || exit.ExitCode() != 78 {
						t.Errorf("%s: exit %v, want 78:\n%s%s", bin, err, out, stderr)
					}
					if !strings.Contains(stderr, "credkit: refusing to start") {
						t.Errorf("%s: want the refusal on stderr:\n%s", bin, stderr)
					}
				}
				if *hits != 0 {
					t.Errorf("a refused start sent %d request(s)", *hits)
				}
			})
		}
	}
}

// A value split over lines as written, inside one JSON value.
func TestKitWithholdsAValueSplitOverLinesInOneJSONValue(t *testing.T) {
	if testing.Short() {
		t.Skip("builds both binaries")
	}
	cliBin, mcpBin := nb2Binaries(t)
	const held = "Tk9mQ2xV7pLw4RzN8sYb"
	srv, _ := nb2Portal(t, map[string]string{"note": "Tk9mQ2xV\n7pLw4RzN\n8sYb"})
	env := nb2Env(t.TempDir(), srv.URL, cliBin, "HUBSPOT_ACCESS_TOKEN="+held)
	out, stderr, _ := nb2Run(t, cliBin, env, "", strings.Fields(kitReadArgs)...)
	for where, got := range map[string]string{"cli": out + stderr, "mcp": nb2MCPRead(t, mcpBin, env)} {
		for _, part := range []string{"Tk9mQ2xV", "7pLw4RzN"} {
			if strings.Contains(got, part) {
				t.Errorf("%s printed part of the held value (%q):\n%s", where, part, got)
			}
		}
		if !strings.Contains(got, "credkit: withheld") {
			t.Errorf("%s: want the kit's notice:\n%s", where, got)
		}
	}
}

// Every other place a public scalar can be a credential refuses to start too.
func TestKitRefusesAPublicScalarFromEverySource(t *testing.T) {
	if testing.Short() {
		t.Skip("builds both binaries")
	}
	cliBin, mcpBin := nb2Binaries(t)
	cases := []struct {
		name  string
		setup func(t *testing.T, home string, env []string) []string
	}{
		{"mcp-bearer-env", func(t *testing.T, home string, env []string) []string {
			return append(env, "HUBSPOT_MCP_HTTP_TOKEN=null")
		}},
		{"config-header", func(t *testing.T, home string, env []string) []string {
			nb2WriteConfig(t, home, "[headers]\nX-Extra = \"true\"\n")
			return env
		}},
		{"config-bare-bool", func(t *testing.T, home string, env []string) []string {
			nb2WriteConfig(t, home, "access_token = false\n")
			return env
		}},
		{"credentials-store", func(t *testing.T, home string, env []string) []string {
			dir := filepath.Join(home, "data")
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "credentials.toml"), []byte("access_token = \"null\"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			return env
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			srv, hits := nb2Portal(t, nil)
			env := tc.setup(t, home, nb2Env(home, srv.URL, cliBin))
			for bin, args := range map[string][]string{cliBin: strings.Fields(kitReadArgs), mcpBin: nil} {
				out, stderr, err := nb2Run(t, bin, env, "", args...)
				var exit *exec.ExitError
				if !errors.As(err, &exit) || exit.ExitCode() != 78 {
					t.Errorf("%s: exit %v, want 78:\n%s%s", filepath.Base(bin), err, out, stderr)
				}
			}
			if *hits != 0 {
				t.Errorf("a refused start sent %d request(s)", *hits)
			}
		})
	}
}

// Every way to name a config file holds its credentials before anything
// prints, past flags the early scan cannot parse: unknown commands echo the
// value, so a credential the scan missed reaches stderr.
func TestKitHoldsEveryConfigBeforeAnythingPrints(t *testing.T) {
	if testing.Short() {
		t.Skip("builds both binaries")
	}
	cliBin, _ := nb2Binaries(t)
	without := func(env []string, names ...string) []string {
		var kept []string
		for _, kv := range env {
			name, _, _ := strings.Cut(kv, "=")
			if !slices.Contains(names, name) {
				kept = append(kept, kv)
			}
		}
		return kept
	}
	const held = "Tk9mQ2xV7pLw4RzN8sYb"
	// The control writes another value: the same run must print the argument,
	// or a case that never echoes would pass with nothing held.
	body := ""
	write := func(t *testing.T, path string) string {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	cases := []struct {
		name  string
		setup func(t *testing.T, home string, env []string) ([]string, []string)
	}{
		{"env-config", func(t *testing.T, home string, env []string) ([]string, []string) {
			return append(env, "HUBSPOT_CONFIG="+write(t, filepath.Join(home, "x", "c.toml"))), []string{held}
		}},
		{"unknown-flag-name-then-config-equals", func(t *testing.T, home string, env []string) ([]string, []string) {
			return env, []string{"--" + held, "--config=" + write(t, filepath.Join(home, "x", "c.toml"))}
		}},
		{"bad-shorthand-then-config", func(t *testing.T, home string, env []string) ([]string, []string) {
			return env, []string{"-Z" + held, "--config", write(t, filepath.Join(home, "x", "c.toml"))}
		}},
		{"bad-duration-then-config", func(t *testing.T, home string, env []string) ([]string, []string) {
			return env, []string{"--timeout", "nope" + held, "--config", write(t, filepath.Join(home, "x", "c.toml"))}
		}},
		{"config-swallowed-as-a-value", func(t *testing.T, home string, env []string) ([]string, []string) {
			return env, []string{"hubspot-contacts-crm", "--select", "--config", write(t, filepath.Join(home, "x", "c.toml")), "--" + held}
		}},
		{"config-after-subcommand", func(t *testing.T, home string, env []string) ([]string, []string) {
			return env, []string{"hubspot-contacts-crm", "--" + held, "--config", write(t, filepath.Join(home, "x", "c.toml"))}
		}},
		{"config-twice", func(t *testing.T, home string, env []string) ([]string, []string) {
			p := write(t, filepath.Join(home, "x", "c.toml"))
			return env, []string{"--config", p, "--config", filepath.Join(home, "x", "absent.toml"), held}
		}},
		{"config-after-double-dash", func(t *testing.T, home string, env []string) ([]string, []string) {
			return env, []string{"--" + held, "--", "--config", write(t, filepath.Join(home, "x", "c.toml"))}
		}},
		{"home-flag-after-bad-flag", func(t *testing.T, home string, env []string) ([]string, []string) {
			root := filepath.Join(home, "root")
			write(t, filepath.Join(root, "config", "config.toml"))
			return without(env, "HUBSPOT_HOME", "HUBSPOT_CONFIG_DIR"), []string{"hubspot-contacts-crm", "--" + held, "--home", root}
		}},
		{"env-home", func(t *testing.T, home string, env []string) ([]string, []string) {
			root := filepath.Join(home, "root")
			write(t, filepath.Join(root, "config", "config.toml"))
			return append(without(env, "HUBSPOT_HOME", "HUBSPOT_CONFIG_DIR"), "HUBSPOT_HOME="+root), []string{held}
		}},
		{"xdg-config-home", func(t *testing.T, home string, env []string) ([]string, []string) {
			xdg := filepath.Join(home, "xdg")
			write(t, filepath.Join(xdg, "hubspot-cli", "config.toml"))
			return append(without(env, "HUBSPOT_HOME", "HUBSPOT_CONFIG_DIR"), "XDG_CONFIG_HOME="+xdg), []string{held}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			run := func(value string) string {
				body = "access_token = \"" + value + "\"\n"
				home := t.TempDir()
				env, args := tc.setup(t, home, nb2Env(home, "http://127.0.0.1:1", cliBin))
				out, stderr, _ := nb2Run(t, cliBin, env, "", args...)
				return out + stderr
			}
			if got := run("Other7Qz4kWp2Lx9Hb"); !strings.Contains(got, "Tk9mQ2xV") {
				t.Errorf("control: the run never prints the argument, so this case proves nothing:\n%s", got)
			}
			if got := run(held); strings.Contains(got, "Tk9mQ2xV") {
				t.Errorf("printed the held value:\n%s", got)
			}
		})
	}
}
