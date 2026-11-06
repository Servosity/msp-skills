// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"comet-backup-pp-cli/internal/cli"
	"comet-backup-pp-cli/internal/cliutil"
	"comet-backup-pp-cli/internal/credkit"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// heldNow asks the kit itself: a value it holds cannot be written to a file.
func heldNow(t *testing.T, v string) bool {
	t.Helper()
	err := credkit.WriteFile(filepath.Join(t.TempDir(), "probe"), []byte(v), 0o600)
	if err != nil && !errors.Is(err, credkit.ErrWithheld) {
		t.Fatal(err)
	}
	return errors.Is(err, credkit.ErrWithheld)
}

func TestIdentifiersMatchConfigNotCredentials(t *testing.T) {
	var exempt []string
	for _, id := range Declarations.Identifiers {
		// template_vars.server is the [template_vars] spelling of server; the per-read
		// hold exempts only top-level keys, and holding it there is safe.
		if k, ok := strings.CutPrefix(id.Source, "config:"); ok && !strings.Contains(k, ".") {
			exempt = append(exempt, k)
		}
	}
	want := slices.Clone(cliutil.NotCredentials)
	slices.Sort(exempt)
	slices.Sort(want)
	if !slices.Equal(exempt, want) {
		t.Fatalf("config: Identifiers %q, cliutil.NotCredentials %q: the kit and the per-read hold disagree", exempt, want)
	}
}

// The kit's declared names and the connector's own walker are one rule: a name
// the kit holds and the walker prints would be withheld whole instead of redacted.
func TestDeclaredNamesMatchTheWalker(t *testing.T) {
	for _, name := range Declarations.ResponseFields {
		if !cliutil.SecretFieldName(name) {
			t.Errorf("response field %s is declared but the walker would print it", name)
		}
	}
	for _, name := range []string{"Username", "AccountName", "Description", "FriendlyName"} {
		if slices.Contains(Declarations.ResponseFields, name) {
			t.Errorf("%s is not a credential", name)
		}
	}
}

// Every credential-named flag any command declares is a declared credential flag
// or is held by the argv walk.
func TestDeclaredFlagsCoverTheCommandTree(t *testing.T) {
	var walk func(*cobra.Command)
	var missing []string
	seen := 0
	walk = func(c *cobra.Command) {
		visit := func(f *pflag.Flag) {
			if f.Value.Type() != "string" || !cliutil.SecretFieldName(f.Name) {
				return
			}
			seen++
			if !slices.Contains(Declarations.CredentialFlags, "--"+f.Name) {
				missing = append(missing, c.CommandPath()+" --"+f.Name)
			}
		}
		c.Flags().VisitAll(visit)
		c.PersistentFlags().VisitAll(visit)
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(cli.RootCmd())
	if seen == 0 {
		t.Fatal("no credential flag found, so this test proved nothing")
	}
	if len(missing) > 0 {
		t.Errorf("credential flags not declared: %q", missing)
	}
}

func TestArgvCredentialsHeld(t *testing.T) {
	holdArgvCredentials([]string{"users", "add", "--target-user", "argvUserNotSecret", "--target-password", "-argvPassUnit-7c1e9a", "--require-password-change", "1"})
	holdArgvCredentials([]string{"users", "reset-password", "--target-user", "bob", "--new-password=argvNewPassUnit-3b5d7f", "--old-password", "argvOldPassUnit-9d1a"})
	for _, v := range []string{"-argvPassUnit-7c1e9a", "argvNewPassUnit-3b5d7f", "argvOldPassUnit-9d1a"} {
		if !heldNow(t, v) {
			t.Errorf("%s not held", v)
		}
	}
	for _, v := range []string{"argvUserNotSecret"} {
		if heldNow(t, v) {
			t.Errorf("%s held, but it is not a credential", v)
		}
	}
}

// Brief item 8(c): a run profile's value under a credential flag's name, and
// the password and query values of its base-url, are held inside Start, every
// duplicate included. Its other values are free text and stay printable.
func TestProfileStoreCredentialsHeldAtStart(t *testing.T) {
	home := t.TempDir()
	t.Setenv("COMET_BACKUP_HOME", home)
	dir, err := cliutil.ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	store := `{"profiles":{"p":{"name":"p","description":"profDescUnitPlain-1","values":{` +
		`"target-password":"profDupFirstUnit-5a2c","target-password":"profDupLastUnit-8e1d",` +
		`"base-url":"https://admin:profUrlPwUnit-3f9b@comet.example/?token=profQueryUnit-7c4a","limit":"profLimitPlain-2"}}}}`
	if err := os.WriteFile(filepath.Join(dir, "profiles.json"), []byte(store), 0o600); err != nil {
		t.Fatal(err)
	}
	credentialFiles([]string{"version"})
	for _, v := range []string{"profDupFirstUnit-5a2c", "profDupLastUnit-8e1d", "profUrlPwUnit-3f9b", "profQueryUnit-7c4a"} {
		if !heldNow(t, v) {
			t.Errorf("%s not held", v)
		}
	}
	for _, v := range []string{"profDescUnitPlain-1", "profLimitPlain-2"} {
		if heldNow(t, v) {
			t.Errorf("%s held, but it is free text", v)
		}
	}
}

// COMET_BACKUP_BASE_URL is set for credkit.Start only.
func TestAfterStartRestoresTheBaseURL(t *testing.T) {
	prior := "https://prior.example"
	for name, set := range map[string]bool{"unset": false, "set": true} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("COMET_BACKUP_HOME", home)
			t.Setenv("COMET_BACKUP_SERVER", "comet.example.com")
			t.Setenv("COMET_BACKUP_BASE_URL", "")
			os.Unsetenv("COMET_BACKUP_BASE_URL")
			if set {
				os.Setenv("COMET_BACKUP_BASE_URL", prior)
			}
			showBaseURL("", "")
			if got := os.Getenv("COMET_BACKUP_BASE_URL"); set && got != prior || !set && got != "https://comet.example.com" {
				t.Fatalf("during Start: %q", got)
			}
			AfterStart()
			got, ok := os.LookupEnv("COMET_BACKUP_BASE_URL")
			if !set && ok {
				t.Fatalf("left set to %q", got)
			}
			if set && got != prior {
				t.Fatalf("got %q, want %q", got, prior)
			}
		})
	}
}

// A template with nothing to fill it resolves to nothing, and the kit then allows
// no origin at all: a credentialed request is refused rather than sent.
func TestUnresolvedBaseURLShowsNothing(t *testing.T) {
	t.Setenv("COMET_BACKUP_HOME", t.TempDir())
	t.Setenv("COMET_BACKUP_SERVER", "")
	t.Setenv("COMET_BACKUP_BASE_URL", "")
	os.Unsetenv("COMET_BACKUP_BASE_URL")
	if got := configuredBase("", ""); got != "" {
		t.Fatalf("want nothing to show, got %q", got)
	}
}
