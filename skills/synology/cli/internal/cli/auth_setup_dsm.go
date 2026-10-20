// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/synology/handfixes.json
// under the "auth-setup-windows-twin" entry.

package cli

import (
	"fmt"
	"io"

	"synology-pp-cli/internal/config"
)

// printDSMSetup prints how to configure a DSM login for both shells. The
// generated auth setup printed "No setup URL is configured for this CLI" and
// the unauthenticated auth status ended on a bare "Set your token:", so an
// operator was never told which variables to set, and most MSPs run Windows
// PowerShell, where `export` does not exist.
func printDSMSetup(w io.Writer) {
	fmt.Fprint(w, `Set the NAS address and a DSM account, then log in once:

  macOS/Linux:
    export SYNOLOGY_BASE_URL=https://<nas>:5001
    export SYNOLOGY_ACCOUNT=<account>
    export SYNOLOGY_PASSWORD=<password>
    synology-cli auth login

  PowerShell:
    $env:SYNOLOGY_BASE_URL = "https://<nas>:5001"
    $env:SYNOLOGY_ACCOUNT = "<account>"
    $env:SYNOLOGY_PASSWORD = "<password>"
    synology-cli auth login

Two-factor account: set SYNOLOGY_OTP_CODE to a fresh code for that first login
(export SYNOLOGY_OTP_CODE=123456, or $env:SYNOLOGY_OTP_CODE = "123456" in
PowerShell); DSM returns a trusted-device token the CLI keeps.
Self-signed certificate: set SYNOLOGY_TLS_FINGERPRINT to the certificate's
SHA-256 fingerprint. Several NAS units: synology-cli fleet add --help.
`)
}

// dsmCredentialState reports whether cfg carries a DSM login (account and
// password, from the environment or the config file) or a raw session id
// (auth_header), and names whatever is missing. HAND-FIX(auth-setup-windows-twin):
// the generated auth status looked only at auth_header, so an operator with
// SYNOLOGY_ACCOUNT and SYNOLOGY_PASSWORD set, even after a successful
// auth login, was told "Not authenticated" and exited 4.
func dsmCredentialState(cfg *config.Config) (configured bool, missing []string) {
	if cfg == nil {
		return false, []string{"SYNOLOGY_ACCOUNT", "SYNOLOGY_PASSWORD"}
	}
	if cfg.AuthHeader() != "" {
		return true, nil
	}
	if cfg.SynologyAccount == "" {
		missing = append(missing, "SYNOLOGY_ACCOUNT")
	}
	if cfg.SynologyPassword == "" {
		missing = append(missing, "SYNOLOGY_PASSWORD")
	}
	return len(missing) == 0, missing
}
