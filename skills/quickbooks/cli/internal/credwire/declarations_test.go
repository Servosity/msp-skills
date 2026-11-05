// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written. Recorded hand-fix `credkit-wiring` in handfixes.json.

package credwire

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"quickbooks-pp-cli/internal/cliutil"
	"quickbooks-pp-cli/internal/cliutil/testenv"
	"quickbooks-pp-cli/internal/credkit"
)

// The kit binds the variable over any file's base_url, so for its read alone
// the variable names what the program will use, and every file stays with the
// kit to hold. The program then gets the variable back as it was.
func TestTheKitIsGivenTheBaseURLTheProgramUses(t *testing.T) {
	const receiver = "https://receiver.example"
	const named = "base_url = \"" + receiver + "\"\n"
	write := func(t *testing.T, path, body string, mode os.FileMode) string {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, mode); err != nil {
			t.Fatal(err)
		}
		return path
	}
	const builtin = "https://quickbooks.api.intuit.com/v3/company/YOUR_REALM_ID"
	const tenant = "https://tenant.example/v3/company/1"
	for _, tc := range []struct {
		name string
		// arrange writes the files and returns the args and the files that must be listed.
		arrange func(t *testing.T, home, cfgDir, legacy, store string) ([]string, []string)
		want    string
	}{
		{"an unused default config", func(t *testing.T, home, cfgDir, _, _ string) ([]string, []string) {
			unused := write(t, filepath.Join(cfgDir, "config.toml"), named, 0o600)
			used := write(t, filepath.Join(home, "tenant.toml"), "base_url = \""+tenant+"\"\n", 0o600)
			return []string{"--config", used}, []string{used, unused}
		}, tenant},
		{"a config in use with no base_url", func(t *testing.T, home, cfgDir, _, _ string) ([]string, []string) {
			unused := write(t, filepath.Join(cfgDir, "config.toml"), named, 0o600)
			used := write(t, filepath.Join(home, "tenant.toml"), "", 0o600)
			return []string{"--config", used}, []string{used, unused}
		}, builtin},
		{"the last of two config flags", func(t *testing.T, home, _, _, _ string) ([]string, []string) {
			first := write(t, filepath.Join(home, "first.toml"), named, 0o600)
			last := write(t, filepath.Join(home, "last.toml"), "base_url = \""+tenant+"\"\n", 0o600)
			return []string{"--config", first, "--config", last}, []string{last}
		}, tenant},
		{"a legacy config behind a primary", func(t *testing.T, _, cfgDir, legacy, _ string) ([]string, []string) {
			primary := write(t, filepath.Join(cfgDir, "config.toml"), "", 0o600)
			return nil, []string{primary, write(t, legacy, named, 0o600)}
		}, builtin},
		{"a legacy config with no primary", func(t *testing.T, _, _, legacy, _ string) ([]string, []string) {
			return nil, []string{write(t, legacy, named, 0o600)}
		}, receiver},
		{"a store saved with its server", func(t *testing.T, _, cfgDir, _, store string) ([]string, []string) {
			primary := write(t, filepath.Join(cfgDir, "config.toml"), "", 0o600)
			return nil, []string{primary, write(t, store, named, 0o600)}
		}, builtin},
		{"a default config under a home flag", func(t *testing.T, home, _, _, _ string) ([]string, []string) {
			t.Setenv("QUICKBOOKS_CONFIG_DIR", "") // an explicit directory would outrank --home
			defaults, err := cliutil.ConfigDir()
			if err != nil {
				t.Fatal(err)
			}
			unused := write(t, filepath.Join(defaults, "config.toml"), named, 0o600)
			other := filepath.Join(home, "other")
			restore, err := cliutil.SetHomeOverride(other)
			if err != nil {
				t.Fatal(err)
			}
			dir, err := cliutil.ConfigDir()
			restore()
			if err != nil {
				t.Fatal(err)
			}
			used := write(t, filepath.Join(dir, "config.toml"), "base_url = \""+tenant+"\"\n", 0o600)
			return []string{"--home", other}, []string{used, unused}
		}, tenant},
		{"a config the program distrusts", func(t *testing.T, home, _, _, _ string) ([]string, []string) {
			if runtime.GOOS == "windows" {
				t.Skip("file modes do not make a config untrusted on Windows")
			}
			loose := write(t, filepath.Join(home, "loose.toml"), named, 0o644)
			return []string{"--config", loose}, []string{loose}
		}, builtin},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := testenv.Isolate(t)
			t.Setenv("QUICKBOOKS_CONFIG", "")
			t.Setenv("QUICKBOOKS_BASE_URL", "")
			t.Setenv("QUICKBOOKS_REALM_ID", "")
			t.Setenv("QUICKBOOKS_CONFIG_DIR", filepath.Join(home, "cfg")) // apart from the legacy path
			cfgDir, err := cliutil.ConfigDir()
			if err != nil {
				t.Fatal(err)
			}
			store, err := cliutil.CredentialsFilePath()
			if err != nil {
				t.Fatal(err)
			}
			prev := cliutil.HoldCredential
			cliutil.HoldCredential = func(credkit.Source, ...string) {}
			t.Cleanup(func() { cliutil.HoldCredential = prev })

			legacy := filepath.Join(home, ".config", "quickbooks-cli", "config.toml")
			args, listed := tc.arrange(t, home, cfgDir, legacy, store)
			got := credentialFiles(args)
			if v := os.Getenv("QUICKBOOKS_BASE_URL"); v != tc.want {
				t.Errorf("the kit reads %q; the program will use %q", v, tc.want)
			}
			RestoreBaseURLEnv()
			if v := os.Getenv("QUICKBOOKS_BASE_URL"); v != "" {
				t.Errorf("the variable stayed %q for the program", v)
			}
			for _, f := range listed {
				if !slices.Contains(got, f) {
					t.Errorf("%s is off the kit's list: %q", f, got)
				}
			}
		})
	}
}

// A variable the operator set is theirs: the kit reads it itself, and it comes
// through untouched. An MCP host's unfilled template counts as not set.
func TestABaseURLVariableIsLeftAlone(t *testing.T) {
	testenv.Isolate(t)
	prev := cliutil.HoldCredential
	cliutil.HoldCredential = func(credkit.Source, ...string) {}
	t.Cleanup(func() { cliutil.HoldCredential = prev })
	t.Setenv("QUICKBOOKS_CONFIG", "")
	t.Setenv("QUICKBOOKS_REALM_ID", "")

	t.Setenv("QUICKBOOKS_BASE_URL", "https://operator.example/v3/company/9")
	credentialFiles(nil)
	if v := os.Getenv("QUICKBOOKS_BASE_URL"); v != "https://operator.example/v3/company/9" {
		t.Errorf("the variable became %q", v)
	}
	RestoreBaseURLEnv()

	const template = "${user_config.quickbooks_base_url}"
	t.Setenv("QUICKBOOKS_BASE_URL", template)
	credentialFiles(nil)
	if v := os.Getenv("QUICKBOOKS_BASE_URL"); v == template {
		t.Errorf("the kit would bind an unfilled template")
	}
	RestoreBaseURLEnv()
	if v := os.Getenv("QUICKBOOKS_BASE_URL"); v != template {
		t.Errorf("the template did not come back: %q", v)
	}
}

// A credential goes to the base URL the program uses and, for the OAuth client
// and tokens, Intuit's token endpoint. No other API host is open because it is
// also Intuit's: the program sends to one company at one host.
func TestOriginsCoverTheBaseURLOnly(t *testing.T) {
	for _, o := range Declarations.Origins {
		for _, a := range o.Allowed {
			if a != "base_url" && a != intuitOAuth {
				t.Errorf("%s may go to %s", o.Source, a)
			}
		}
	}
}

// A write's params are free-form and its preview repeats them, so every field
// name this file calls a credential is an MCP argument too.
func TestEveryDeclaredCredentialFieldIsAnMCPArgument(t *testing.T) {
	d := Declarations
	for _, names := range [][]string{d.ConfigFields, d.StoredRecords, d.CredentialFields, d.ResponseFields} {
		for _, name := range names {
			if !slices.Contains(d.MCPArguments, name) {
				t.Errorf("%s is declared a credential field but is not in MCPArguments", name)
			}
		}
	}
}
