// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"zoom-phone-pp-cli/internal/cliutil"
	"zoom-phone-pp-cli/internal/config"
)

func acctGuardNewClientErr(t *testing.T, configPath string) error {
	t.Helper()
	f := &rootFlags{configPath: configPath, timeout: time.Second}
	_, err := f.newClient()
	return err
}

func acctAssertProfileIntact(t *testing.T, path, secret string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), secret) || !strings.Contains(string(data), "phone_account_id") {
		t.Fatalf("profile %s lost its credentials:\n%s", path, data)
	}
}

// Item 2: selecting an account profile by --config / ZOOM_PHONE_CONFIG alone
// is refused before any request, with the fix in the message.
func TestAcctGuardRefusesProfileWithoutAccountHome(t *testing.T) {
	acctTestHome(t)
	acme := acctMustAdd(t, "acme", "A1", "C1", "acme-secret")
	globex := acctMustAdd(t, "globex", "A2", "C2", "globex-secret")

	// --config alone.
	err := acctGuardNewClientErr(t, acme)
	if err == nil || ExitCode(err) != 10 {
		t.Fatalf("--config alone: err=%v code=%d, want exit 10", err, ExitCode(err))
	}
	for _, want := range []string{`eval "$(zoom-phone-cli accounts path acme --env)"`, "fleet --accounts acme --", "ZOOM_PHONE_HOME"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("guard message missing %q: %v", want, err)
		}
	}

	// ZOOM_PHONE_CONFIG alone, through a real API command: refused, nothing sent,
	// profile intact.
	t.Setenv("ZOOM_PHONE_CONFIG", acme)
	_, errOut, code := acctRun(t, "", "users", "list-phone", "--json")
	if code != 10 || !strings.Contains(errOut, "accounts path acme --env") {
		t.Fatalf("users list-phone with ZOOM_PHONE_CONFIG alone: exit=%d stderr=%s", code, errOut)
	}
	acctAssertProfileIntact(t, acme, "acme-secret")

	// auth logout would wipe the profile's credentials: refused too.
	_, errOut, code = acctRun(t, "", "auth", "logout", "--config", acme)
	if code != 10 || !strings.Contains(errOut, "accounts path acme --env") {
		t.Fatalf("auth logout --config <profile>: exit=%d stderr=%s", code, errOut)
	}
	acctAssertProfileIntact(t, acme, "acme-secret")
	t.Setenv("ZOOM_PHONE_CONFIG", "")

	// Another account's home with this account's config.
	t.Setenv("ZOOM_PHONE_HOME", filepath.Dir(globex))
	if err := acctGuardNewClientErr(t, acme); err == nil || ExitCode(err) != 10 {
		t.Fatalf("acme config with globex home: err=%v", err)
	}

	// An account home with no profile config (default config path).
	if err := acctGuardNewClientErr(t, ""); err == nil || !strings.Contains(err.Error(), "belongs to the \"globex\" account") {
		t.Fatalf("globex home with default config: err=%v", err)
	}

	// The supported pairing passes: the account's config plus its own home.
	t.Setenv("ZOOM_PHONE_HOME", filepath.Dir(acme))
	if err := acctGuardNewClientErr(t, acme); err != nil {
		t.Fatalf("acme config with acme home refused: %v", err)
	}
}

func TestAcctGuardLeavesOrdinaryConfigsAlone(t *testing.T) {
	home := acctTestHome(t)
	acctMustAdd(t, "acme", "A1", "C1", "s1")

	// Default config, no account selected.
	if err := acctGuardNewClientErr(t, ""); err != nil {
		t.Fatalf("default config refused: %v", err)
	}
	// An explicit non-profile config file.
	other := filepath.Join(home, "work", "config.toml")
	if err := os.MkdirAll(filepath.Dir(other), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("client_id = 'X'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := acctGuardNewClientErr(t, other); err != nil {
		t.Fatalf("plain --config refused: %v", err)
	}
	// A general-purpose ZOOM_PHONE_HOME.
	t.Setenv("ZOOM_PHONE_HOME", filepath.Join(home, "zp-home"))
	if err := acctGuardNewClientErr(t, ""); err != nil {
		t.Fatalf("general ZOOM_PHONE_HOME refused: %v", err)
	}
	if _, err := config.Load(""); err != nil {
		t.Fatal(err)
	}
}

// Round 2 item 2: a profile is recognised by file identity (case variants,
// hard links) and, for files that do not exist yet, by a case-insensitive
// path shape.
func TestAcctGuardIdentifiesProfileVariants(t *testing.T) {
	home := acctTestHome(t)
	acme := acctMustAdd(t, "acme", "A1", "C1", "acme-secret")
	root := filepath.Dir(filepath.Dir(acme))

	hardLink := filepath.Join(home, "elsewhere", "client.toml")
	if err := os.MkdirAll(filepath.Dir(hardLink), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(acme, hardLink); err != nil {
		t.Fatal(err)
	}
	upperFile := filepath.Join(root, "acme", "CONFIG.TOML")
	upperDir := filepath.Join(filepath.Dir(root), "Accounts", "acme", "config.toml")
	missingUpper := filepath.Join(filepath.Dir(root), "ACCOUNTS", "Newco", "Config.Toml")

	cases := []struct{ name, path, account string }{
		{"hard link", hardLink, "acme"},
		{"upper-case file name", upperFile, "acme"},
		{"upper-case accounts dir", upperDir, "acme"},
		{"missing file, case variant", missingUpper, "newco"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name, _, ok := acctIdentifyProfile(tc.path)
			if !ok || name != tc.account {
				t.Fatalf("acctIdentifyProfile(%s) = %q, %v; want %q", tc.path, name, ok, tc.account)
			}
			err := acctGuardNewClientErr(t, tc.path)
			if err == nil || ExitCode(err) != 10 || !strings.Contains(err.Error(), "accounts path "+tc.account+" --env") {
				t.Fatalf("--config %s without the account home: err=%v", tc.path, err)
			}
		})
	}
	acctAssertProfileIntact(t, acme, "acme-secret")

	// With the account's own home selected, a link to the profile from outside
	// its directory is still refused (the run would write outside the account,
	// and the profile now has a second hard link). Once the link is gone, a
	// case-variant spelling of the profile itself is accepted.
	t.Setenv("ZOOM_PHONE_HOME", filepath.Dir(acme))
	if err := acctGuardNewClientErr(t, hardLink); err == nil || ExitCode(err) != 10 {
		t.Fatalf("hard link from outside with acme's home: err=%v, want exit 10", err)
	}
	if err := os.Remove(hardLink); err != nil {
		t.Fatal(err)
	}
	if err := acctGuardNewClientErr(t, upperFile); err != nil {
		t.Fatalf("--config %s with acme's home refused: %v", upperFile, err)
	}
}

// Round 2 item 3: a data directory inside an account directory (via --home,
// ZOOM_PHONE_HOME or ZOOM_PHONE_DATA_DIR) is only usable with that account's
// profile.
func TestAcctGuardRefusesForeignConfigInAccountData(t *testing.T) {
	home := acctTestHome(t)
	acme := acctMustAdd(t, "acme", "A1", "C1", "acme-secret")
	// The root command applies --home process-wide; never leak it to other tests.
	t.Cleanup(func() { _, _ = cliutil.SetHomeOverride("") })
	acmeHome := filepath.Dir(acme)
	other := filepath.Join(home, "work", "other.toml")
	if err := os.MkdirAll(filepath.Dir(other), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("client_id = 'OTHER'\nclient_secret = 'other-secret'\nphone_account_id = 'OTHER_ACCT'\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// --home <account dir> (the flag) with a foreign config, through a real command.
	_, errOut, code := acctRun(t, "", "users", "list-phone", "--json", "--home", acmeHome, "--config", other)
	if code != 10 || !strings.Contains(errOut, `belongs to the "acme" account`) {
		t.Fatalf("--home <acme> --config other: exit=%d stderr=%s", code, errOut)
	}
	// --home <account dir> with the default config.
	_, errOut, code = acctRun(t, "", "users", "list-phone", "--json", "--home", acmeHome)
	if code != 10 || !strings.Contains(errOut, `belongs to the "acme" account`) {
		t.Fatalf("--home <acme> with default config: exit=%d stderr=%s", code, errOut)
	}
	if _, err := cliutil.SetHomeOverride(""); err != nil {
		t.Fatal(err)
	}
	// ZOOM_PHONE_DATA_DIR=<account>/data with a foreign config.
	t.Setenv("ZOOM_PHONE_DATA_DIR", filepath.Join(acmeHome, "data"))
	if err := acctGuardNewClientErr(t, other); err == nil || ExitCode(err) != 10 {
		t.Fatalf("ZOOM_PHONE_DATA_DIR=<acme>/data --config other: err=%v", err)
	}
	// ZOOM_PHONE_DATA_DIR=<account>/data with the account's own profile is fine.
	if err := acctGuardNewClientErr(t, acme); err != nil {
		t.Fatalf("ZOOM_PHONE_DATA_DIR=<acme>/data with acme's profile refused: %v", err)
	}
	// A data dir nested deeper in the account (not <account>/data) is refused
	// even with the profile: the reload would look in <account>/data.
	t.Setenv("ZOOM_PHONE_DATA_DIR", filepath.Join(acmeHome, "data", "nested"))
	if err := acctGuardNewClientErr(t, acme); err == nil {
		t.Fatalf("nested data dir with acme's profile accepted")
	}
	t.Setenv("ZOOM_PHONE_DATA_DIR", "")
	acctAssertProfileIntact(t, acme, "acme-secret")
	if _, err := os.Stat(filepath.Join(acmeHome, "data", "credentials.toml")); !os.IsNotExist(err) {
		t.Fatalf("a foreign run wrote into acme's token cache: %v", err)
	}

	// --home <account dir> with that account's profile is the supported pairing.
	restore, err := cliutil.SetHomeOverride(acmeHome)
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	if err := acctGuardNewClientErr(t, acme); err != nil {
		t.Fatalf("--home <acme> --config <acme profile> refused: %v", err)
	}
}
