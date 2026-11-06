// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"maxio-pp-cli/internal/cli"
	"maxio-pp-cli/internal/cliutil"
	"maxio-pp-cli/internal/cliutil/testenv"
)

func writeConfig(t *testing.T, path, base string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("base_url = \""+base+"\"\n"), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

// Kit item 8(b): the base URL shown to the kit is the one config.Load uses.
// An explicit --config or MAXIO_CONFIG wins over the discovered
// config.toml, whose host is then never an allowed origin; a config other
// users can change never chooses one; MAXIO_BASE_URL wins over both. The
// variable is back as the process found it after AfterStart.
func TestShownBaseIsTheConfigTheProgramUses(t *testing.T) {
	home := testenv.Isolate(t, cliutil.ConfigDir)
	dir, err := cliutil.ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	discovered := filepath.Join(dir, "config.toml")
	writeConfig(t, discovered, "https://discovered.example", 0o600)
	explicit := filepath.Join(home, "explicit.toml")
	writeConfig(t, explicit, "https://explicit.example", 0o600)

	cases := []struct {
		name, config, envBase, want string
		args                        []string
	}{
		{name: "discovered", want: "https://discovered.example"},
		{name: "--config", args: []string{"--config", explicit, "dataset", "1"}, want: "https://explicit.example"},
		{name: "--config=", args: []string{"--config=" + explicit}, want: "https://explicit.example"},
		{name: "MAXIO_CONFIG", config: explicit, want: "https://explicit.example"},
		{name: "MAXIO_BASE_URL", args: []string{"--config", explicit}, envBase: "https://env.example", want: "https://env.example"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("MAXIO_CONFIG", c.config)
			t.Setenv("MAXIO_BASE_URL", c.envBase)
			if c.envBase == "" {
				os.Unsetenv("MAXIO_BASE_URL")
			}
			credentialFiles(c.args)
			if got := os.Getenv("MAXIO_BASE_URL"); got != c.want {
				t.Errorf("shown base %q, want %q", got, c.want)
			}
			AfterStart()
			got, set := os.LookupEnv("MAXIO_BASE_URL")
			if c.envBase == "" && set {
				t.Errorf("MAXIO_BASE_URL left set to %q", got)
			}
			if c.envBase != "" && got != c.envBase {
				t.Errorf("MAXIO_BASE_URL restored to %q, want %q", got, c.envBase)
			}
		})
	}
	if runtime.GOOS != "windows" {
		t.Run("untrusted config", func(t *testing.T) {
			os.Unsetenv("MAXIO_BASE_URL")
			writeConfig(t, discovered, "https://discovered.example", 0o644)
			credentialFiles(nil)
			if got := os.Getenv("MAXIO_BASE_URL"); got != noSiteOrigin {
				t.Errorf("an untrusted config chose the shown base %q, want %q", got, noSiteOrigin)
			}
			AfterStart()
		})
	}
}

func TestProfileFlagsMatchTheDeclaration(t *testing.T) {
	if !slices.Equal(profileCredentialFlags, Declarations.CredentialFlags) {
		t.Errorf("profileCredentialFlags differs from Declarations.CredentialFlags")
	}
}

// TestCredentialFlagsMatchTheNameRule keeps credentialFlags whole: every flag of
// every command the name rule reads as a credential, vault references aside (see
// credentialFlags), is declared, and no declared flag has gone.
func TestCredentialFlagsMatchTheNameRule(t *testing.T) {
	found := map[string]bool{}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		c.Flags().VisitAll(func(f *pflag.Flag) {
			if cliutil.SecretFlagName(f.Name) && !strings.Contains(f.Name, "vault") {
				found["--"+f.Name] = true
			}
		})
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(cli.RootCmd())
	for f := range found {
		if !slices.Contains(credentialFlags, f) {
			t.Errorf("%s reads as a credential and is not in credentialFlags", f)
		}
	}
	for _, f := range credentialFlags {
		if !found[f] {
			t.Errorf("%s is declared and no command has it", f)
		}
	}
}

func TestCredentialFieldsMatchTheNameRule(t *testing.T) {
	for _, f := range credentialFields {
		if !cliutil.SecretFieldName(f) {
			t.Errorf("%s is declared and the name rule does not read it as a credential", f)
		}
	}
	for _, public := range []string{"key", "public_key", "uniqueness_token", "requires_security_token", "masked_card_number", "last_four"} {
		if slices.Contains(credentialFields, public) || cliutil.SecretFieldName(public) {
			t.Errorf("%s is billing data, not a credential", public)
		}
	}
}

// The key goes to the site's API host and the Events-Based Billing host alone,
// and the MCP bearer goes nowhere.
func TestOriginsAreTheSiteAndTheEventsHost(t *testing.T) {
	got := map[string][]string{}
	for _, o := range Declarations.Origins {
		got[o.Source] = o.Allowed
	}
	if !slices.Equal(got["*"], []string{"base_url", "https://events.chargify.com"}) {
		t.Errorf("origins of every credential: %v", got["*"])
	}
	if a, ok := got["env:MAXIO_MCP_HTTP_TOKEN"]; !ok || len(a) != 0 {
		t.Errorf("origins of the MCP bearer: %v (present %v), want none", a, ok)
	}
}
