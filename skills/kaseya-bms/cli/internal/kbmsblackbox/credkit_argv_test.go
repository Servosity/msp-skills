// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package kbmsblackbox

import (
	"strings"
	"testing"
)

// Argv credentials, through the credential kit (handfixes.json credkit-wiring,
// CredentialFlags in internal/credwire): the login form's --password,
// --mfa-code and --client-secret, and a token typed positionally into auth
// set-token (which reads stdin), never print; the form's --user-name and
// --tenant are identifiers and do (the control preview carries no
// credential, as the kit withholds a unit that carries one whole).
func TestArgvCredentialsNeverPrint(t *testing.T) {
	sv := echoServer(t)
	e := newEnv(t, sv, credStored)
	const (
		user   = "argvUserUnit-1c2d"
		tenant = "argvTenantUnit-3e4f"
	)
	secrets := []string{"-argvPasswordUnit-7c1e9a", "48291375", "argvClientSecretUnit-5b7d"}
	out := e.run(t, "security", "authenticate", "--user-name", user, "--tenant", tenant, "--password", secrets[0],
		"--mfa-code", secrets[1], "--client-secret", secrets[2], "--grant-type", "password", "--dry-run")
	ctl := e.run(t, "security", "authenticate", "--user-name", user, "--tenant", tenant, "--grant-type", "password", "--dry-run")
	if !strings.Contains(ctl, user) || !strings.Contains(ctl, tenant) {
		t.Fatalf("the identifiers do not print in a preview that carries no credential:\n%.1500s", ctl)
	}
	for _, args := range [][]string{{"auth", "set-token", "posTokenUnit-6e2a9c4d"}, {"auth", "set-token", "--", "-posDashTokenUnit-1f8b3e"}} {
		out += "\n" + e.run(t, args...)
	}
	for _, s := range append(secrets, "posTokenUnit-6e2a9c4d", "-posDashTokenUnit-1f8b3e") {
		if strings.Contains(out, s) {
			t.Errorf("the credential %s typed on the command line prints:\n%.2000s", s, out)
		}
	}
	if methods, mints, _, _ := sv.counts(); len(methods) != 0 || mints != 0 {
		t.Fatalf("--dry-run sent %v (%d logins)", methods, mints)
	}
}
