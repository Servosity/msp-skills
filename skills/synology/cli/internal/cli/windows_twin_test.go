// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/synology/handfixes.json under
// the "auth-setup-windows-twin" and "help-examples-windows-twin" entries.

package cli

import (
	"strings"
	"testing"
)

// TestAuthSetupPrintsBothShells: auth setup printed "No setup URL is
// configured for this CLI" and named no variable; most MSPs run PowerShell,
// where `export` does not exist.
func TestAuthSetupPrintsBothShells(t *testing.T) {
	sweepIsolate(t)
	out, _, code := sweepExecute(t, "auth", "setup")
	if code != 0 {
		t.Fatalf("auth setup exited %d", code)
	}
	for _, want := range []string{"export SYNOLOGY_BASE_URL=", "export SYNOLOGY_ACCOUNT=", "export SYNOLOGY_PASSWORD=",
		`$env:SYNOLOGY_BASE_URL = "`, `$env:SYNOLOGY_ACCOUNT = "`, `$env:SYNOLOGY_PASSWORD = "`, `$env:SYNOLOGY_OTP_CODE = "`} {
		if !strings.Contains(out, want) {
			t.Fatalf("auth setup does not print %q:\n%s", want, out)
		}
	}
}

// TestAuthStatusReadsDSMLogin: auth status looked only at auth_header, so an
// operator with SYNOLOGY_ACCOUNT and SYNOLOGY_PASSWORD set was told "Not
// authenticated", followed by a bare "Set your token:", and exited 4.
func TestAuthStatusReadsDSMLogin(t *testing.T) {
	sweepIsolate(t)
	out, _, code := sweepExecute(t, "auth", "status")
	if code != 4 || !strings.Contains(out, `$env:SYNOLOGY_ACCOUNT = "`) || !strings.Contains(out, "export SYNOLOGY_ACCOUNT=") {
		t.Fatalf("auth status without a login: exit %d, output %q", code, out)
	}
	t.Setenv("SYNOLOGY_ACCOUNT", "msp-admin")
	t.Setenv("SYNOLOGY_PASSWORD", "s3cretpw")
	out, _, code = sweepExecute(t, "auth", "status", "--json")
	if code != 0 || !strings.Contains(out, `"authenticated": true`) {
		t.Fatalf("auth status with a DSM login: exit %d, output %q", code, out)
	}
}

// TestHelpExamplesCarryPowerShellTwin: the POSIX `VAR=value cmd` and
// `< file` forms do not run in PowerShell.
func TestHelpExamplesCarryPowerShellTwin(t *testing.T) {
	root := RootCmd()
	for _, tc := range []struct {
		path []string
		want string
	}{
		{[]string{"auth", "login"}, `$env:SYNOLOGY_OTP_CODE = "123456"; synology-cli auth login`},
		{[]string{"feedback"}, "Get-Content notes.txt | synology-cli feedback --stdin"},
	} {
		cmd, _, err := root.Find(tc.path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(cmd.Example, tc.want) {
			t.Fatalf("%v example lacks the PowerShell twin %q:\n%s", tc.path, tc.want, cmd.Example)
		}
	}
}

// TestFleetSyncCredentialHintCarriesPowerShellTwin: a fleet NAS whose login
// variables are unset reported "export <prefix>_ACCOUNT ..." only.
func TestFleetSyncCredentialHintCarriesPowerShellTwin(t *testing.T) {
	sweepIsolate(t)
	if _, _, code := sweepExecute(t, "fleet", "add", "acme", "--url", "https://192.0.2.10:5001", "--env-prefix", "ACME_NAS"); code != 0 {
		t.Fatalf("fleet add exited %d", code)
	}
	out, _, _ := sweepExecute(t, "fleet", "sync", "--json")
	if !strings.Contains(out, `$env:ACME_NAS_ACCOUNT`) || !strings.Contains(out, "export ACME_NAS_ACCOUNT=") {
		t.Fatalf("fleet sync credential hint is not twinned: %q", out)
	}
}
