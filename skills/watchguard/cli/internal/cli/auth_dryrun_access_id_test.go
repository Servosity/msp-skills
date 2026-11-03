// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"strings"
	"testing"
)

// TestAuthLoginDryRunNeverPrintsTheAccessID (handfixes.json
// auth-login-dry-run-no-access-id): the Access ID is a declared credential
// (WATCHGUARD_CLIENT_ID, --client-id). The dry-run line named it, so the kit
// withheld the whole preview in the built binary and the unguarded line
// printed it. The preview says what would happen without it, from the flag
// and from the environment.
func TestAuthLoginDryRunNeverPrintsTheAccessID(t *testing.T) {
	accessID := "acc" + "essId-Q7m2Xw9r"
	secret := "sec" + "ret-Z4n8Kp1v"
	for _, tc := range []struct {
		name string
		args []string
		env  bool
	}{
		{"flag", []string{"auth", "login", "--dry-run", "--client-id", accessID, "--client-secret", secret}, false},
		{"env", []string{"auth", "login", "--dry-run"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dryRunEnv(t)
			if tc.env {
				t.Setenv("WATCHGUARD_CLIENT_ID", accessID)
				t.Setenv("WATCHGUARD_CLIENT_SECRET", secret)
			}
			out, err := runAuthCmd(t, "", tc.args...)
			if err != nil {
				t.Fatalf("auth login --dry-run: %v\n%s", err, out)
			}
			if !strings.Contains(out, "grant_type=client_credentials") {
				t.Fatalf("dry run printed no preview:\n%s", out)
			}
			for _, v := range []string{accessID, secret} {
				if strings.Contains(out, v) || strings.Contains(out, v[:8]) {
					t.Errorf("dry run printed credential %q:\n%s", v[:3], out)
				}
			}
		})
	}
}
