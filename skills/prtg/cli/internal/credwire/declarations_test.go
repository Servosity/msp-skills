// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"prtg-pp-cli/internal/credkit"
	"prtg-pp-cli/internal/fleet"
)

func heldNow(t *testing.T, v string) bool {
	t.Helper()
	err := credkit.WriteFile(filepath.Join(t.TempDir(), "probe"), []byte(v), 0o600)
	if err != nil && !errors.Is(err, credkit.ErrWithheld) {
		t.Fatal(err)
	}
	return errors.Is(err, credkit.ErrWithheld)
}

// Each key in PRTG_FLEET_KEYS is held before anything prints, and a core name
// beside it is not: output naming a core must stay readable.
func TestCredentialFilesHoldsEachFleetKeyNotTheCoreNames(t *testing.T) {
	t.Setenv("PRTG_HOME", t.TempDir())
	t.Setenv(fleet.DirEnv, t.TempDir())
	t.Setenv(fleet.KeysEnv, "acme-long-core-name=fleetUnitKeyAcme-3f9a,\nother-long-core-name=Bearer fleetUnitKeyOther-8c2e")
	credentialFiles(nil)
	AfterStart()
	for _, v := range []string{"fleetUnitKeyAcme-3f9a", "fleetUnitKeyOther-8c2e"} {
		if !heldNow(t, v) {
			t.Errorf("%s not held", v)
		}
	}
	for _, v := range []string{"acme-long-core-name", "other-long-core-name"} {
		if heldNow(t, v) {
			t.Errorf("core name %s was held", v)
		}
	}
}

// PRTG_BASE_URL is set for credkit.Start only. Left set, it would stop being what
// the operator exported: it bypasses the program's check that a credential route
// was chosen on purpose.
func TestAfterStartRestoresTheBaseURL(t *testing.T) {
	for name, prior := range map[string]*string{"unset": nil, "set": strPtr("https://prior.example/api/v2")} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("PRTG_BASE_URL", "")
			os.Unsetenv("PRTG_BASE_URL")
			if prior != nil {
				os.Setenv("PRTG_BASE_URL", *prior)
			}
			showBaseURL(nil, fleet.Core{Server: "core.example.com:1616"}, true)
			if got := os.Getenv("PRTG_BASE_URL"); got != "https://core.example.com:1616/api/v2" {
				t.Fatalf("during Start: %q", got)
			}
			AfterStart()
			got, ok := os.LookupEnv("PRTG_BASE_URL")
			if prior == nil && ok {
				t.Fatalf("left set to %q", got)
			}
			if prior != nil && got != *prior {
				t.Fatalf("got %q, want %q", got, *prior)
			}
		})
	}
}

func strPtr(s string) *string { return &s }

func TestProfileFlagsMatchTheDeclaration(t *testing.T) {
	if !slices.Equal(profileCredentialFlags, Declarations.CredentialFlags) {
		t.Errorf("profileCredentialFlags differs from Declarations.CredentialFlags")
	}
}
