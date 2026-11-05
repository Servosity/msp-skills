// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// What the credential kit knows when the program starts, through the built
// binaries (handfixes.json credkit-wiring): the one base URL the program will
// use, whatever other files lie around, and the TLS key before a flag can fail.

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hubspot-pp-cli/internal/cliutil"
)

// A credential goes only to the origin the program itself talks to. Target is
// the webhook's origin and nothing listens there, so the kit's verdict shows in
// the delivery error: its notice when it refused, a dial error when it let the
// credential through.
func TestKitTakesTheBaseURLOnlyFromTheFileTheProgramUses(t *testing.T) {
	if testing.Short() {
		t.Skip("builds both binaries")
	}
	cliBin, _ := nb2Binaries(t)
	const target = "https://127.0.0.1:1"
	const named = "base_url = \"" + target + "\"\n"
	const other = "base_url = \"https://api.hubapi.com\"\n"
	write := func(t *testing.T, path, body string) string {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	type setup = func(t *testing.T, home string) (env, args []string)
	selected := func(home, body string, t *testing.T) []string {
		return []string{"--config", write(t, filepath.Join(home, "x", "selected.toml"), body)}
	}
	refused := map[string]setup{
		"env-config-then-flag-with-another-base": func(t *testing.T, home string) ([]string, []string) {
			return []string{"HUBSPOT_CONFIG=" + write(t, filepath.Join(home, "x", "old.toml"), named)}, selected(home, other, t)
		},
		"env-config-then-flag-without-a-base": func(t *testing.T, home string) ([]string, []string) {
			return []string{"HUBSPOT_CONFIG=" + write(t, filepath.Join(home, "x", "old.toml"), named)}, selected(home, "", t)
		},
		"default-config-then-flag-without-a-base": func(t *testing.T, home string) ([]string, []string) {
			nb2WriteConfig(t, home, named)
			return nil, selected(home, "", t)
		},
		"global-store-names-a-base": func(t *testing.T, home string) ([]string, []string) {
			write(t, filepath.Join(home, "data", "credentials.toml"), named)
			return nil, nil
		},
		"config-store-names-a-base": func(t *testing.T, home string) ([]string, []string) {
			args := selected(home, "", t)
			store, err := cliutil.CredentialsFilePathForConfig(args[1])
			if err != nil {
				t.Fatal(err)
			}
			write(t, store, named)
			return nil, args
		},
		"legacy-file-beside-the-config-in-use": func(t *testing.T, home string) ([]string, []string) {
			nb2WriteConfig(t, home, named) // HOME's legacy path, with the config directory moved away
			dir := filepath.Join(home, "new")
			write(t, filepath.Join(dir, "config.toml"), "")
			return []string{"HUBSPOT_CONFIG_DIR=" + dir}, nil
		},
		"legacy-file-that-does-not-parse": func(t *testing.T, home string) ([]string, []string) {
			nb2WriteConfig(t, home, named+"access_token = \n")
			return []string{"HUBSPOT_CONFIG_DIR=" + filepath.Join(home, "new")}, nil
		},
		"env-home-then-home-flag": func(t *testing.T, home string) ([]string, []string) {
			stale, root := filepath.Join(home, "stale"), filepath.Join(home, "root")
			write(t, filepath.Join(stale, "config", "config.toml"), named)
			write(t, filepath.Join(root, "config", "config.toml"), "")
			return []string{"HUBSPOT_HOME=" + stale, "HUBSPOT_CONFIG_DIR="}, []string{"--home", root}
		},
	}
	allowed := map[string]setup{
		"flag-config": func(t *testing.T, home string) ([]string, []string) {
			return []string{"HUBSPOT_CONFIG=" + write(t, filepath.Join(home, "x", "old.toml"), other)}, selected(home, named, t)
		},
		"last-flag-config": func(t *testing.T, home string) ([]string, []string) {
			first := write(t, filepath.Join(home, "x", "first.toml"), other)
			return nil, append([]string{"--config", first}, selected(home, named, t)...)
		},
		"env-config": func(t *testing.T, home string) ([]string, []string) {
			nb2WriteConfig(t, home, other)
			return []string{"HUBSPOT_CONFIG=" + write(t, filepath.Join(home, "x", "c.toml"), named)}, nil
		},
		"default-config": func(t *testing.T, home string) ([]string, []string) {
			nb2WriteConfig(t, home, named)
			return nil, nil
		},
		"legacy-file-when-no-config-exists": func(t *testing.T, home string) ([]string, []string) {
			nb2WriteConfig(t, home, named)
			return []string{"HUBSPOT_CONFIG_DIR=" + filepath.Join(home, "new")}, nil
		},
		"home-flag": func(t *testing.T, home string) ([]string, []string) {
			root := filepath.Join(home, "root")
			write(t, filepath.Join(root, "config", "config.toml"), named)
			nb2WriteConfig(t, home, other)
			return []string{"HUBSPOT_CONFIG_DIR="}, []string{"--home", root}
		},
		"base-url-env-over-every-file": func(t *testing.T, home string) ([]string, []string) {
			nb2WriteConfig(t, home, other)
			return []string{"HUBSPOT_BASE_URL=" + target}, selected(home, other, t)
		},
	}
	deliver := func(t *testing.T, tc setup) string {
		t.Helper()
		const held = "Tk9mQ2xV7pLw4RzN8sYb"
		home := t.TempDir()
		env, args := tc(t, home)
		env = append(nb2Env(home, "", cliBin, "HUBSPOT_ACCESS_TOKEN="+held, "HUBSPOT_USER_AGENT="+held), env...)
		args = append(args, "--deliver", "webhook:"+target+"/", "version")
		out, stderr, _ := nb2Run(t, cliBin, env, "", args...)
		if !strings.Contains(stderr, "posting to webhook") {
			t.Fatalf("the delivery never ran, so this case proves nothing:\n%s%s", out, stderr)
		}
		return stderr
	}
	for name, tc := range refused {
		t.Run("refused/"+name, func(t *testing.T) {
			if got := deliver(t, tc); !strings.Contains(got, "credkit: withheld") {
				t.Errorf("the kit let a credential go to an origin the program does not use:\n%s", got)
			}
		})
	}
	for name, tc := range allowed {
		t.Run("allowed/"+name, func(t *testing.T) {
			if got := deliver(t, tc); strings.Contains(got, "credkit: withheld") {
				t.Errorf("the kit refused the origin the program uses:\n%s", got)
			}
		})
	}
}

// The MCP server's own flags are not the CLI's, so reading them with the CLI's
// parser fails, and the server still takes its base URL from its config file.
func TestMCPTakesTheBaseURLFromItsConfigPastItsOwnFlags(t *testing.T) {
	if testing.Short() {
		t.Skip("builds both binaries")
	}
	cliBin, mcpBin := nb2Binaries(t)
	srv, hits := nb2Portal(t, nil)
	home := t.TempDir()
	nb2WriteConfig(t, home, "base_url = \""+srv.URL+"\"\n")
	const held = "Tk9mQ2xV7pLw4RzN8sYb"
	env := nb2Env(home, srv.URL, cliBin, "HUBSPOT_ACCESS_TOKEN="+held, "HUBSPOT_BASE_URL=")
	got := nb2MCPRead(t, mcpBin, env, "--transport", "stdio")
	if !strings.Contains(got, "Lovelace") || *hits == 0 {
		t.Errorf("the server did not reach its configured base URL (%d hits):\n%s", *hits, got)
	}
}

// A setting in a file the program does not read is not a credential, so it
// cannot refuse a start; a credential there still does.
func TestKitRefusesOnlyOverACredentialInAFileItDoesNotRead(t *testing.T) {
	if testing.Short() {
		t.Skip("builds both binaries")
	}
	cliBin, _ := nb2Binaries(t)
	run := func(body string) error {
		home := t.TempDir()
		nb2WriteConfig(t, home, body) // HOME's legacy path, with the config directory moved away
		dir := filepath.Join(home, "new")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "config.toml"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
		env := nb2Env(home, "http://127.0.0.1:1", cliBin, "HUBSPOT_CONFIG_DIR="+dir)
		_, _, err := nb2Run(t, cliBin, env, "", "version")
		return err
	}
	if err := run("debug = true\nname = \"null\"\n"); err != nil {
		t.Errorf("a setting refused the start: %v", err)
	}
	if err := run("access_token = \"null\"\n"); err == nil {
		t.Error("a credential that is public text did not refuse the start")
	}
}

// A run profile is a file the CLI prints back. A value under a credential's name
// or under base-url is a credential source and is held at startup, every
// duplicate included; everything else in it, free text included, still prints.
func TestKitHoldsWhatARunProfileCarries(t *testing.T) {
	if testing.Short() {
		t.Skip("builds both binaries")
	}
	cliBin, _ := nb2Binaries(t)
	const held = "Zq7nM4vR8sT2xK5w"
	store := func(description string, values map[string]string) string {
		b, err := json.Marshal(map[string]any{"profiles": map[string]any{"p": map[string]any{"name": "p", "description": description, "values": values}}})
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	write := func(t *testing.T, path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	type place = func(t *testing.T, home, body string) (env, args []string)
	inConfigDir := func(t *testing.T, home, body string) ([]string, []string) {
		write(t, filepath.Join(nb2ConfigDir(home), "profiles.json"), body)
		return nil, nil
	}
	// Profile q's last member is another value, so only a reader that sees every
	// member holds what profile p prints.
	duplicate := func(name string) string {
		last := strings.Repeat("Wk3nP8dL", 2)
		return `{"profiles":{"p":{"name":"p","description":"` + held + `","values":{"limit":"7"}},` +
			`"q":{"name":"q","values":{` + name + `:"` + held + `","api-token":"` + last + `"}}}}`
	}
	carriers := map[string]string{
		"field-named-for-it":     store("", map[string]string{"api-token": held}),
		"base-url-password":      store("", map[string]string{"base-url": "https://u:" + held + "@example.com/"}),
		"base-url-query":         store("", map[string]string{"base_url": "https://example.com/?access_token=" + held}),
		"base-url-broken":        store("", map[string]string{"base-url": "https://[broken/?access_token=" + held}),
		"first-of-a-duplicate":   duplicate(`"api-token"`),
		"escaped-name-duplicate": duplicate(`"api\u002dtoken"`),
	}
	// Every credential field Declarations declares, and how a person might spell one.
	for _, field := range []string{
		"auth_header", "auth-header", "authHeader", "access_token", "refresh_token", "client_id", "client-id",
		"clientId", "client_secret", "clientSecret", "api_key", "password", "cookie", "bearer",
	} {
		carriers["field-"+field] = store("", map[string]string{field: "Bearer " + held})
	}
	places := map[string]place{
		"config-dir": inConfigDir,
		"legacy": func(t *testing.T, home, body string) ([]string, []string) {
			write(t, filepath.Join(home, ".hubspot-cli", "profiles.json"), body)
			return nil, nil
		},
		"home-flag": func(t *testing.T, home, body string) ([]string, []string) {
			root := filepath.Join(home, "root")
			write(t, filepath.Join(root, "config", "profiles.json"), body)
			return []string{"HUBSPOT_HOME=", "HUBSPOT_CONFIG_DIR="}, []string{"--home", root}
		},
	}
	show := func(t *testing.T, where place, body string) (string, error) {
		t.Helper()
		home := t.TempDir()
		extra, args := where(t, home, body)
		env := append(nb2Env(home, "http://127.0.0.1:1", cliBin), extra...)
		out, stderr, err := nb2Run(t, cliBin, env, "", append(args, "profile", "show", "p")...)
		return out + stderr, err
	}
	for name, body := range carriers {
		t.Run(name, func(t *testing.T) {
			got, _ := show(t, inConfigDir, body)
			if strings.Contains(got, held[:8]) || !strings.Contains(got, "credkit: withheld") {
				t.Errorf("want the kit's notice in place of the credential:\n%s", got)
			}
		})
	}
	for name, where := range places {
		t.Run("place/"+name, func(t *testing.T) {
			got, _ := show(t, where, carriers["field-named-for-it"])
			if strings.Contains(got, held[:8]) || !strings.Contains(got, "credkit: withheld") {
				t.Errorf("want the kit's notice in place of the credential:\n%s", got)
			}
		})
	}
	// Free text is not a source (contract section 1), so a word in a URL there
	// cannot refuse a start.
	t.Run("free-text-url-still-prints", func(t *testing.T) {
		const text = "https://example.com/?cookieConsent=true"
		got, err := show(t, inConfigDir, store(text, map[string]string{"limit": "7"}))
		if err != nil || !strings.Contains(got, text) {
			t.Errorf("free text was held (%v):\n%s", err, got)
		}
	})
	t.Run("plain-profile-still-prints", func(t *testing.T) {
		body := store("weekly review", map[string]string{"limit": "7", "agent": "true", "filter": "https://example.com/?page=1"})
		got, err := show(t, inConfigDir, body)
		if err != nil {
			t.Fatalf("a plain profile failed the run: %v\n%s", err, got)
		}
		for _, want := range []string{"weekly review", `"limit": "7"`, "page=1"} {
			if !strings.Contains(got, want) {
				t.Errorf("lost %q:\n%s", want, got)
			}
		}
	})
}

// A credentials store that does not parse is read line by line, and a value there
// may be spelled with escapes: what is held is the value the escapes spell, or
// another output that shows it prints the credential.
func TestKitHoldsTheValueAnEscapeSpellsInAFileThatDoesNotParse(t *testing.T) {
	if testing.Short() {
		t.Skip("builds both binaries")
	}
	cliBin, _ := nb2Binaries(t)
	const held = "Zq7nM4vR"
	spell := func(format string) string {
		var b strings.Builder
		for _, r := range held {
			fmt.Fprintf(&b, format, r)
		}
		return b.String()
	}
	u := spell(`\u%04X`)
	for name, written := range map[string]string{
		"u":                  `"` + u + `"`,
		"U":                  `"` + spell(`\U%08X`) + `"`,
		"x":                  `"` + spell(`\x%02X`) + `"`,
		"toml-escape":        `"` + u + `\e"`,
		"unknown-escape":     `"` + u + `\q"`,
		"truncated-escape":   `"` + u + `\u00"`,
		"multi-line":         "\"\"\"\n" + u + "\n\"\"\"",
		"line-ending-escape": "\"\"\"\\\nZq7n\\\n  M4vR\"\"\"",
	} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			root := filepath.Join(home, "root")
			for path, body := range map[string]string{
				"config/config.toml":    "access_token = \"N8pL6sV2\"\n",
				"data/credentials.toml": "access_token = " + written + "\naccess_token = \"Lp8Tz5wK\"\n",
				"config/profiles.json":  `{"profiles":{"p":{"name":"p","description":"` + held + `","values":{}}}}`,
			} {
				path = filepath.Join(root, filepath.FromSlash(path))
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			env := append(nb2Env(home, "", cliBin), "HUBSPOT_HOME=", "HUBSPOT_CONFIG_DIR=", "HUBSPOT_BASE_URL=")
			out, stderr, _ := nb2Run(t, cliBin, env, "", "--home", root, "profile", "show", "p", "--json")
			if got := out + stderr; strings.Contains(got, held) || !strings.Contains(got, "credkit: withheld") {
				t.Errorf("want the kit's notice in place of the credential:\n%s", got)
			}
		})
	}
}

// A flag a run profile may carry is never named like a credential, or holding
// fields by name would withhold every output that shows a saved filter.
func TestNoRunProfileFlagIsNamedForACredential(t *testing.T) {
	for name := range runProfileAllowedFlags {
		if cliutil.CredentialFieldName(name) {
			t.Errorf("--%s is allowed in a run profile and named for a credential", name)
		}
	}
	for _, field := range []string{"name", "description", "values", "profiles"} {
		if cliutil.CredentialFieldName(field) {
			t.Errorf("the profile store's own field %q reads as a credential", field)
		}
	}
}

// A base URL that does not parse still gives up the credential in its query.
func TestKitHoldsTheQueryOfABaseURLThatDoesNotParse(t *testing.T) {
	if testing.Short() {
		t.Skip("builds both binaries")
	}
	cliBin, _ := nb2Binaries(t)
	const held = "Zq7nM4vR8sT2xK5w"
	run := func(value string) string {
		home := t.TempDir()
		dir := filepath.Join(home, "data")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		body := "base_url = \"https://[broken/?access_token=" + value + "\"\n"
		if err := os.WriteFile(filepath.Join(dir, "credentials.toml"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		out, stderr, _ := nb2Run(t, cliBin, nb2Env(home, "http://127.0.0.1:1", cliBin), "", "--help="+held)
		return out + stderr
	}
	if got := run("Other4Lm9Pq2Wx7Ka3"); !strings.Contains(got, held[:8]) {
		t.Fatalf("control: the run never prints the argument, so this case proves nothing:\n%s", got)
	}
	if got := run(held); strings.Contains(got, held[:8]) {
		t.Errorf("printed the credential:\n%s", got)
	}
}

// No HubSpot request body carries a credential, so --stdin is not a credential
// flag and a write's preview still shows its body. A body that does carry a
// credential the program holds is withheld all the same.
func TestStdinBodyPreviewStaysReadableAndKeepsHeldValuesOut(t *testing.T) {
	if testing.Short() {
		t.Skip("builds both binaries")
	}
	cliBin, _ := nb2Binaries(t)
	const held = "Tk9mQ2xV7pLw4RzN8sYb"
	preview := func(body string) string {
		home := t.TempDir()
		env := nb2Env(home, "http://127.0.0.1:1", cliBin, "HUBSPOT_ACCESS_TOKEN="+held)
		out, stderr, _ := nb2Run(t, cliBin, env, body, "hubspot-contacts-crm", "post-v3-objects-contacts", "--stdin", "--dry-run")
		return out + stderr
	}
	if got := preview(`{"properties":{"email":"ada@example.com"}}`); !strings.Contains(got, "ada@example.com") || strings.Contains(got, "credkit: withheld") {
		t.Errorf("the preview lost an ordinary body:\n%s", got)
	}
	got := preview(`{"properties":{"note":"` + held + `"}}`)
	if strings.Contains(got, held[:8]) || !strings.Contains(got, "credkit: withheld") {
		t.Errorf("want the kit's notice in place of the held value:\n%s", got)
	}
}

// A flag that fails before --tls-key echoes its own name, so a piece of the key
// reaches stderr unless the key was held before parsing began.
func TestMCPHoldsTheTLSKeyBeforeAFlagCanFail(t *testing.T) {
	if testing.Short() {
		t.Skip("builds both binaries")
	}
	cliBin, mcpBin := nb2Binaries(t)
	writeKey := func(t *testing.T) (path, piece string) {
		t.Helper()
		for {
			_, priv, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			der, err := x509.MarshalPKCS8PrivateKey(priv)
			if err != nil {
				t.Fatal(err)
			}
			body := base64.StdEncoding.EncodeToString(der)
			piece = body[len(body)-8:]
			if strings.ContainsAny(piece, "+/=") {
				continue
			}
			path = filepath.Join(t.TempDir(), "key.pem")
			data := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			return path, piece
		}
	}
	spellings := map[string]func(path string) []string{
		"two-dashes":        func(p string) []string { return []string{"--tls-key", p} },
		"two-dashes-equals": func(p string) []string { return []string{"--tls-key=" + p} },
		"one-dash":          func(p string) []string { return []string{"-tls-key", p} },
		"one-dash-equals":   func(p string) []string { return []string{"-tls-key=" + p} },
	}
	for name, spell := range spellings {
		t.Run(name, func(t *testing.T) {
			run := func(path, piece string) string {
				home := t.TempDir()
				args := append([]string{"--" + piece}, spell(path)...)
				out, stderr, _ := nb2Run(t, mcpBin, nb2Env(home, "http://127.0.0.1:1", cliBin), "", args...)
				return out + stderr
			}
			path, piece := writeKey(t)
			other, _ := writeKey(t)
			if got := run(other, piece); !strings.Contains(got, piece) {
				t.Fatalf("control: the run never prints the flag's name, so this case proves nothing:\n%s", got)
			}
			if got := run(path, piece); strings.Contains(got, piece) {
				t.Errorf("printed a piece of the TLS key:\n%s", got)
			}
		})
	}
}
