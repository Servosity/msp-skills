// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"threecx-pp-cli/internal/cliutil"
	"threecx-pp-cli/internal/credkit"
	"threecx-pp-cli/internal/fleet"
)

func heldNow(t *testing.T, v string) bool {
	t.Helper()
	err := credkit.WriteFile(filepath.Join(t.TempDir(), "probe"), []byte(v), 0o600)
	if err != nil && !errors.Is(err, credkit.ErrWithheld) {
		t.Fatal(err)
	}
	return errors.Is(err, credkit.ErrWithheld)
}

// --code is a credential beside --verifier or --token-endpoint and nothing alone:
// the XAPI has many ordinary codes, and holding one would withhold every listing
// that repeats it.
func TestOAuthCodeOnTheCommandLine(t *testing.T) {
	holdOAuthCode([]string{"x", "--code", "loneCodeUnit-77"})
	if heldNow(t, "loneCodeUnit-77") {
		t.Error("a lone --code was held")
	}
	holdOAuthCode([]string{"mcp-servers", "exchange", "--code", "pairedCodeUnit-88", "--token-endpoint", "https://idp.example/t"})
	if !heldNow(t, "pairedCodeUnit-88") {
		t.Error("an exchange's --code was not held")
	}
}

// What the kit cannot name in Declarations is held in credentialFiles, before
// anything prints: the per-PBX secret family (a name is operator data, and a
// parent can print or store a value it never builds a client for) and an OAuth
// exchange's --code. The registry itself is not read as a credential file: the kit
// holds every value in a file it is given, and it holds host names.
func TestCredentialFilesHoldsWhatDeclarationsCannotName(t *testing.T) {
	t.Setenv("THREECX_HOME", t.TempDir())
	t.Setenv(fleet.DirEnv, t.TempDir())
	t.Setenv("THREECX_UNIT_CLIENT_SECRET", "familyUnitSecret-3f9a")
	t.Setenv("CUSTOM_UNIT_CLIENT_SECRET", "Bearer customUnitSecret-8c2e")
	if err := fleet.Update(func(r *fleet.Registry) error {
		r.Upsert(fleet.PBX{Name: "unit", FQDN: "registryunit.example.com", ClientID: "901", EnvPrefix: "CUSTOM_UNIT"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	credentialFiles([]string{"mcp-servers", "exchange", "--code", "wiredCodeUnit-41", "--verifier", "x"})
	for _, v := range []string{"familyUnitSecret-3f9a", "customUnitSecret-8c2e", "wiredCodeUnit-41"} {
		if !heldNow(t, v) {
			t.Errorf("%s not held", v)
		}
	}
	if heldNow(t, "registryunit.example.com") {
		t.Error("a host name from the registry was held")
	}
	AfterStart()
}

// THREECX_BASE_URL is set for credkit.Start only. Left set, it would stop being
// what the operator exported: it bypasses the program's check that a credential
// route was chosen on purpose.
func TestAfterStartRestoresTheBaseURL(t *testing.T) {
	for name, prior := range map[string]*string{"unset": nil, "set": strPtr("https://prior.example/xapi/v1")} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("THREECX_BASE_URL", "")
			os.Unsetenv("THREECX_BASE_URL")
			if prior != nil {
				os.Setenv("THREECX_BASE_URL", *prior)
			}
			showBaseURL(nil, fleet.PBX{FQDN: "pbx.example.com"}, true)
			if got := os.Getenv("THREECX_BASE_URL"); got != "https://pbx.example.com/xapi/v1" {
				t.Fatalf("during Start: %q", got)
			}
			AfterStart()
			got, ok := os.LookupEnv("THREECX_BASE_URL")
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

// A template with nothing to fill it resolves to nothing, and the kit then allows
// no origin at all: a credentialed request is refused rather than sent.
func TestUnresolvedBaseURLShowsNothing(t *testing.T) {
	t.Setenv("THREECX_HOME", t.TempDir())
	t.Setenv("THREECX_FQDN", "")
	t.Setenv("THREECX_BASE_URL", "")
	os.Unsetenv("THREECX_BASE_URL")
	if got := configuredBase(nil); got != "" {
		t.Fatalf("want nothing to show, got %q", got)
	}
}

// Requests and tool arguments hold what a response holds, plus the operator's PINs.
func TestFieldDeclarationsAgree(t *testing.T) {
	want := append(slices.Clone(Declarations.ResponseFields), "PinNumber", "VMPIN")
	if !slices.Equal(Declarations.CredentialFields, want) || !slices.Equal(Declarations.MCPArguments, want) {
		t.Errorf("CredentialFields %v and MCPArguments %v differ from ResponseFields plus the PIN names", Declarations.CredentialFields, Declarations.MCPArguments)
	}
}

func TestProfileFlagsMatchTheDeclaration(t *testing.T) {
	if !slices.Equal(cliutil.ProfileCredentialFlags, Declarations.CredentialFlags) {
		t.Errorf("cliutil.ProfileCredentialFlags differs from Declarations.CredentialFlags")
	}
}
