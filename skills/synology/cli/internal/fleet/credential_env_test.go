// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package fleet

import "testing"

// acme and acme-b are both default prefixes: one must never read as the other's.
func TestCredentialEnvIsExact(t *testing.T) {
	n := NAS{Name: "acme", EnvPrefix: "SYNOLOGY_ACME"}
	for name, want := range map[string]bool{
		"SYNOLOGY_ACME_PASSWORD":   true,
		"SYNOLOGY_ACME_OTP_CODE":   true,
		"SYNOLOGY_ACME_DEVICE_ID":  true,
		"SYNOLOGY_ACME_B_PASSWORD": false,
	} {
		if got := n.IsCredentialEnv(name); got != want {
			t.Errorf("%s: %v", name, got)
		}
	}
}
