// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

import "testing"

// nerdio handfix credential-names-match-nerdio-data, MFA status: NMM's only
// MFA surface is GET users mfaStatus, a registration status read. Its status
// names are data (held, "Enabled" withheld whole outputs); a code, OTP, PIN,
// seed or any other credential word after "mfa" stays a credential.
func TestMFAStatusNamesAreData(t *testing.T) {
	for _, n := range []string{"mfaStatus", "mfaEnabled", "mfa_methods", "MfaState", "mfaRegistered", "mfa"} {
		if SecretFieldName(n) {
			t.Errorf("%q reads as a credential; it names an MFA status", n)
		}
	}
	for _, n := range []string{"mfaCode", "mfa_otp", "mfaPin", "mfaSeed", "mfaToken", "mfaSecret", "mfaSessionToken", "mfaPasscode"} {
		if !SecretFieldName(n) {
			t.Errorf("%q does not read as a credential", n)
		}
	}
}
