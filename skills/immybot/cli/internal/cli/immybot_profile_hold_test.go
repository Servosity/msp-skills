// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"path/filepath"
	"slices"
	"testing"

	"immybot-pp-cli/internal/cliutil"
	"immybot-pp-cli/internal/credwire"
)

// Start holds the credential-named profile values (credwire), and the CLI's own
// check decides what a profile may apply: every name the CLI calls a credential
// must be one Start held, and Start must read the stores the CLI reads.
func TestProfileCredentialNamesCoverTheCLIs(t *testing.T) {
	for name := range everyFlagName() {
		if (credentialLikeRunProfileFlag(name) || credwire.CredentialFlag(name)) && !credwire.ProfileCredentialName(name) {
			t.Errorf("--%s is a credential to the CLI but not held at startup", name)
		}
	}
	for _, name := range []string{"token", "secret", "password", "credential", "api-key", "apikey", "authorization", "cookie", "private-key", "client-secret", "X_Api_Key"} {
		if credentialLikeRunProfileFlag(name) && !credwire.ProfileCredentialName(name) {
			t.Errorf("%q is a credential to the CLI but not held at startup", name)
		}
	}
	paths := credwire.ProfileStorePaths(nil)
	current := func() (string, error) {
		dir, err := cliutil.ConfigDir()
		return filepath.Join(dir, "profiles.json"), err
	}
	for _, get := range []func() (string, error){current, legacyProfileStorePath} {
		p, err := get()
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(paths, p) {
			t.Errorf("the CLI reads %s, which Start does not hold (%v)", p, paths)
		}
	}
}
