// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED. Pins HAND-FIX(dsm-auth-logout): auth logout's output is
// exactly what it did, and after it the next login needs the password and a
// one-time code.

package cli

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"synology-pp-cli/internal/cliutil"
	"synology-pp-cli/internal/config"
	"synology-pp-cli/internal/dsm"
	"synology-pp-cli/internal/dsm/dsmtest"
)

type logoutJSON struct {
	DryRun       bool                `json:"dry_run"`
	Scope        string              `json:"scope"`
	NAS          string              `json:"nas"`
	SessionFiles []dsm.CachedSession `json:"session_files"`
	Stored       []struct {
		File   string   `json:"file"`
		Kind   string   `json:"kind"`
		Fields []string `json:"fields"`
	} `json:"stored_credentials"`
	Cleared      bool     `json:"cleared"`
	NASContacted bool     `json:"nas_contacted"`
	Result       string   `json:"result"`
	EnvStillSet  []string `json:"env_still_set"`
}

func runLogout(t *testing.T, args ...string) (logoutJSON, string, int) {
	t.Helper()
	out, errText, code := runCLIErr(t, append([]string{"auth", "logout", "--json"}, args...)...)
	var rep logoutJSON
	if code == 0 {
		if err := json.Unmarshal([]byte(out), &rep); err != nil {
			t.Fatalf("auth logout %v: not JSON: %v\n%s", args, err, out)
		}
	}
	return rep, out + errText, code
}

// sessionFilesOnDisk lists every session cache file in the data directory.
func sessionFilesOnDisk(t *testing.T) []string {
	t.Helper()
	dir, err := cliutil.DataDir()
	if err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	var out []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "session") {
			out = append(out, e.Name())
		}
	}
	return out
}

// useNAS points the single-NAS env at nas with the two-factor stand-in's
// account; code adds a one-time code.
func useNAS(t *testing.T, nas *dsmtest.TwoFactorNAS, password, code string) {
	t.Setenv("SYNOLOGY_BASE_URL", nas.URL())
	t.Setenv("SYNOLOGY_ACCOUNT", dsmtest.TwoFactorAccount)
	t.Setenv("SYNOLOGY_PASSWORD", password)
	t.Setenv("SYNOLOGY_OTP_CODE", code)
}

// freshShell clears every single-NAS login variable, as a new terminal has.
func freshShell(t *testing.T) {
	for _, k := range []string{"SYNOLOGY_BASE_URL", "SYNOLOGY_ACCOUNT", "SYNOLOGY_PASSWORD", "SYNOLOGY_OTP_CODE", "SYNOLOGY_DEVICE_ID"} {
		t.Setenv(k, "")
	}
}

// requireCodeAgain proves the next command for nas cannot reuse anything:
// without a password it does not log in at all, with the password alone DSM
// asks for a one-time code (no trusted-device token was sent), and with a
// fresh code it works.
func requireCodeAgain(t *testing.T, nas *dsmtest.TwoFactorNAS) {
	t.Helper()
	before := len(nas.Logins())
	useNAS(t, nas, "", "")
	if out, errText, code := runCLIErr(t, "system", "dsm", "--json", "--no-cache"); code == 0 {
		t.Fatalf("a command with no password succeeded after logout (a cached session was reused):\n%s%s", out, errText)
	}
	if n := len(nas.Logins()); n != before {
		t.Fatalf("a login was attempted without a password (%d)", n-before)
	}
	useNAS(t, nas, nas.Password, "")
	out, errText, code := runCLIErr(t, "system", "dsm", "--json", "--no-cache")
	logins := nas.Logins()
	if code == 0 || !strings.Contains(out+errText, "one-time code") || len(logins) != before+1 {
		t.Fatalf("a password-only login after logout: exit %d (%d logins), want the one-time code demanded:\n%s%s", code, len(logins)-before, out, errText)
	}
	if did := logins[len(logins)-1].Get("device_id"); did != "" {
		t.Fatalf("the login after logout still sent a trusted-device token (%s)", did)
	}
	useNAS(t, nas, nas.Password, dsmtest.TwoFactorOTP)
	if out, errText, code := runCLIErr(t, "system", "dsm", "--json", "--no-cache"); code != 0 {
		t.Fatalf("a login with a fresh code after logout: exit %d\n%s%s", code, out, errText)
	}
}

func loginWithCode(t *testing.T, nas *dsmtest.TwoFactorNAS) {
	t.Helper()
	useNAS(t, nas, nas.Password, dsmtest.TwoFactorOTP)
	if out, errText, code := runCLIErr(t, "auth", "login"); code != 0 {
		t.Fatalf("auth login with a code: exit %d\n%s%s", code, out, errText)
	}
}

// TestAuthLogoutInAFreshShell is the review's lo2 repro: in a new terminal
// (SYNOLOGY_BASE_URL unset) logout printed {"cleared": true} for the example
// placeholder, wrote a config.json naming it, and kept the live sid,
// SynoToken and trusted-device token, so the next command reused the sid with
// no password. It now refuses (exit 2, naming SYNOLOGY_BASE_URL and --all)
// and changes nothing; --all --dry-run lists the file and keeps it; --all
// removes it and says what it held; and the next command needs the password
// and a one-time code.
func TestAuthLogoutInAFreshShell(t *testing.T) {
	sweepIsolate(t)
	dsm.ForgetVerifiedAddresses()
	nas := dsmtest.NewTwoFactorNAS(t, "")
	loginWithCode(t, nas)
	if files := sessionFilesOnDisk(t); len(files) != 1 {
		t.Fatalf("auth login cached %v", files)
	}
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	configPath := cfg.Path

	freshShell(t)
	for _, args := range [][]string{{"auth", "logout", "--json"}, {"auth", "logout"}, {"auth", "logout", "--dry-run"}} {
		out, errText, code := runCLIErr(t, args...)
		if code != 2 || !strings.Contains(errText, "SYNOLOGY_BASE_URL") || !strings.Contains(errText, "--all") ||
			!strings.Contains(errText, "nothing was cleared") || strings.Contains(out, "cleared\": true") {
			t.Fatalf("%v in a fresh shell: exit %d, want 2 and a refusal naming SYNOLOGY_BASE_URL and --all:\n%s%s", args, code, out, errText)
		}
	}
	if files := sessionFilesOnDisk(t); len(files) != 1 {
		t.Fatalf("a refused logout changed the session cache: %v", files)
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		raw, _ := os.ReadFile(configPath)
		t.Fatalf("a refused logout wrote %s: %s", configPath, raw)
	}

	rep, text, code := runLogout(t, "--all", "--dry-run")
	if code != 0 || !rep.DryRun || rep.Cleared || rep.Scope != "all" || len(rep.SessionFiles) != 1 ||
		!rep.SessionFiles[0].SessionID || strings.Join(rep.SessionFiles[0].DeviceTokenAccounts, ",") != dsmtest.TwoFactorAccount ||
		!strings.Contains(rep.Result, "nothing was changed") {
		t.Fatalf("auth logout --all --dry-run: exit %d\n%s", code, text)
	}
	if files := sessionFilesOnDisk(t); len(files) != 1 {
		t.Fatalf("--dry-run removed the session cache: %v", files)
	}

	rep, text, code = runLogout(t, "--all")
	if code != 0 || rep.DryRun || !rep.Cleared || rep.NASContacted || len(rep.SessionFiles) != 1 || !rep.SessionFiles[0].SessionID ||
		strings.Join(rep.SessionFiles[0].DeviceTokenAccounts, ",") != dsmtest.TwoFactorAccount {
		t.Fatalf("auth logout --all: exit %d\n%s", code, text)
	}
	if files := sessionFilesOnDisk(t); len(files) != 0 {
		t.Fatalf("auth logout --all left %v", files)
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("auth logout --all created %s with nothing stored in it", configPath)
	}
	rep, text, code = runLogout(t, "--all")
	if code != 0 || rep.Cleared || len(rep.SessionFiles) != 0 || len(rep.Stored) != 0 || !strings.Contains(rep.Result, "nothing cached") {
		t.Fatalf("a second auth logout --all must say nothing was cached: exit %d\n%s", code, text)
	}
	out, errText, code := runCLIErr(t, "auth", "logout", "--all")
	if code != 0 || !strings.Contains(out, "nothing was removed") || strings.Contains(out, "Credentials cleared") {
		t.Fatalf("human auth logout --all with nothing cached: exit %d\n%s%s", code, out, errText)
	}
	requireCodeAgain(t, nas)
}

// TestAuthLogoutWithoutAnAccount is the review's lo repro: with
// SYNOLOGY_BASE_URL set and SYNOLOGY_ACCOUNT unset, logout removed the sid
// but kept the account's trusted-device token, so the next password login
// skipped the second factor. The whole session file goes now.
func TestAuthLogoutWithoutAnAccount(t *testing.T) {
	sweepIsolate(t)
	dsm.ForgetVerifiedAddresses()
	nas := dsmtest.NewTwoFactorNAS(t, "")
	loginWithCode(t, nas)
	t.Setenv("SYNOLOGY_ACCOUNT", "")
	t.Setenv("SYNOLOGY_PASSWORD", "")
	t.Setenv("SYNOLOGY_OTP_CODE", "")
	out, errText, code := runCLIErr(t, "auth", "logout")
	if code != 0 || !strings.Contains(out, "forgot 1 session file(s)") || !strings.Contains(out, "trusted-device token for admin") ||
		!strings.Contains(out, "the NAS was not contacted") {
		t.Fatalf("auth logout without SYNOLOGY_ACCOUNT: exit %d\n%s%s", code, out, errText)
	}
	if files := sessionFilesOnDisk(t); len(files) != 0 {
		t.Fatalf("auth logout left %v", files)
	}
	requireCodeAgain(t, nas)
}

// TestAuthLogoutForgetsOnlyTheNamedNAS: logout for one NAS (SYNOLOGY_BASE_URL
// or --nas) removes that NAS's session file and nothing else, and says
// "nothing cached" when there is nothing; a stored login in the CLI's config
// file is cleared field by field with the rest of the file kept.
func TestAuthLogoutForgetsOnlyTheNamedNAS(t *testing.T) {
	sweepIsolate(t)
	dsm.ForgetVerifiedAddresses()
	single := dsmtest.NewTwoFactorNAS(t, "")
	fleetNAS := dsmtest.NewTwoFactorNAS(t, "")
	loginWithCode(t, single)
	freshShell(t)
	t.Setenv("SYNOLOGY_BETA_ACCOUNT", dsmtest.TwoFactorAccount)
	t.Setenv("SYNOLOGY_BETA_PASSWORD", fleetNAS.Password)
	t.Setenv("SYNOLOGY_BETA_OTP_CODE", dsmtest.TwoFactorOTP)
	if out, errText, code := runCLIErr(t, "fleet", "add", "beta", "--url", fleetNAS.URL(), "--check"); code != 0 {
		t.Fatalf("fleet add beta --check: exit %d\n%s%s", code, out, errText)
	}
	t.Setenv("SYNOLOGY_BETA_OTP_CODE", "")
	if files := sessionFilesOnDisk(t); len(files) != 2 {
		t.Fatalf("want one session file per NAS, got %v", files)
	}

	rep, text, code := runLogout(t, "--nas", "beta")
	if code != 0 || !rep.Cleared || rep.Scope != "nas" || rep.NAS != fleetNAS.URL() || len(rep.SessionFiles) != 1 ||
		rep.SessionFiles[0].SessionID || strings.Join(rep.SessionFiles[0].DeviceTokenAccounts, ",") != dsmtest.TwoFactorAccount ||
		strings.Join(rep.EnvStillSet, ",") != "SYNOLOGY_BETA_ACCOUNT,SYNOLOGY_BETA_PASSWORD" {
		t.Fatalf("auth logout --nas beta: exit %d\n%s", code, text)
	}
	if dir, _ := cliutil.DataDir(); dsm.CachedDeviceID(dir, single.URL(), dsmtest.TwoFactorAccount) == "" {
		t.Fatal("logging out of beta removed the single NAS's trusted-device token")
	}
	if rep, text, code = runLogout(t, "--nas", "beta"); code != 0 || rep.Cleared || len(rep.SessionFiles) != 0 || !strings.Contains(rep.Result, "nothing cached for "+fleetNAS.URL()) {
		t.Fatalf("a second logout of beta: exit %d\n%s", code, text)
	}
	before := len(fleetNAS.Logins())
	out, errText, code := runCLIErr(t, "fleet", "sync", "--nas", "beta", "--json")
	logins := fleetNAS.Logins()
	if code == 0 || !strings.Contains(out+errText, "one-time code") || len(logins) == before || logins[len(logins)-1].Get("device_id") != "" {
		t.Fatalf("fleet sync of beta after logout: exit %d, want a one-time code demanded and no device token sent\n%s%s", code, out, errText)
	}

	// A login stored in the config file: only its credential fields go.
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	stored := `{"base_url":"` + single.URL() + `","account":"admin","password":"stored-pw-1","headers":{"X-Keep":"1"}}`
	if err := cliutil.AtomicWritePrivateFile(cfg.Path, []byte(stored), 0o600, 0o700); err != nil {
		t.Fatal(err)
	}
	rep, text, code = runLogout(t) // the NAS comes from the config's base_url
	if code != 0 || !rep.Cleared || rep.NAS != single.URL() || len(rep.SessionFiles) != 1 || len(rep.Stored) != 1 ||
		rep.Stored[0].Kind != "config" || rep.Stored[0].File != cfg.Path || strings.Join(rep.Stored[0].Fields, ",") != "account,password" {
		t.Fatalf("auth logout with a stored login: exit %d\n%s", code, text)
	}
	raw, _ := os.ReadFile(cfg.Path)
	var kept map[string]any
	if err := json.Unmarshal(raw, &kept); err != nil || kept["base_url"] != single.URL() || kept["headers"] == nil || kept["account"] != nil || kept["password"] != nil {
		t.Fatalf("the config file after logout: %s", raw)
	}
	if files := sessionFilesOnDisk(t); len(files) != 0 {
		t.Fatalf("left %v", files)
	}
}
