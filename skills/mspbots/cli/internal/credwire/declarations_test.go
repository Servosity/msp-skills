// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"mspbots-pp-cli/internal/cliutil"
	"mspbots-pp-cli/internal/cliutil/testenv"
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
// An explicit --config or MSPBOTS_CONFIG wins over the discovered
// config.toml, whose host is then never an allowed origin; a config other
// users can change never chooses one; MSPBOTS_BASE_URL wins over both. The
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
		{name: "MSPBOTS_CONFIG", config: explicit, want: "https://explicit.example"},
		{name: "MSPBOTS_BASE_URL", args: []string{"--config", explicit}, envBase: "https://env.example", want: "https://env.example"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("MSPBOTS_CONFIG", c.config)
			t.Setenv("MSPBOTS_BASE_URL", c.envBase)
			if c.envBase == "" {
				os.Unsetenv("MSPBOTS_BASE_URL")
			}
			credentialFiles(c.args)
			if got := os.Getenv("MSPBOTS_BASE_URL"); got != c.want {
				t.Errorf("shown base %q, want %q", got, c.want)
			}
			AfterStart()
			got, set := os.LookupEnv("MSPBOTS_BASE_URL")
			if c.envBase == "" && set {
				t.Errorf("MSPBOTS_BASE_URL left set to %q", got)
			}
			if c.envBase != "" && got != c.envBase {
				t.Errorf("MSPBOTS_BASE_URL restored to %q, want %q", got, c.envBase)
			}
		})
	}
	if runtime.GOOS != "windows" {
		t.Run("untrusted config", func(t *testing.T) {
			os.Unsetenv("MSPBOTS_BASE_URL")
			writeConfig(t, discovered, "https://discovered.example", 0o644)
			credentialFiles(nil)
			if got := os.Getenv("MSPBOTS_BASE_URL"); got != mspbotsAPI {
				t.Errorf("an untrusted config chose the shown base %q, want %q", got, mspbotsAPI)
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
