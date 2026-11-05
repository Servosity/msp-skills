// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package credwire

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"liongard-pp-cli/internal/cliutil"
	"liongard-pp-cli/internal/cliutil/testenv"
	"liongard-pp-cli/internal/credkit"
	"liongard-pp-cli/internal/store"
)

func isolate(t *testing.T) string {
	t.Helper()
	home := testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	for _, k := range []string{"LIONGARD_BASE_URL", "LIONGARD_INSTANCE"} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
	return home
}

func heldNow(t *testing.T, v string) bool {
	t.Helper()
	err := credkit.WriteFile(filepath.Join(t.TempDir(), "probe"), []byte(v), 0o600)
	if err != nil && !errors.Is(err, credkit.ErrWithheld) {
		t.Fatal(err)
	}
	return errors.Is(err, credkit.ErrWithheld)
}

// The kit is shown the base URL the client really resolves: the template
// filled with LIONGARD_INSTANCE, the same call the origin pin makes.
func TestShownBaseURLIsTheResolvedInstance(t *testing.T) {
	isolate(t)
	t.Setenv("LIONGARD_INSTANCE", "acme")
	showBaseURL(nil)
	got := os.Getenv("LIONGARD_BASE_URL")
	AfterStart()
	if got != "https://acme.app.liongard.com/api/v1" {
		t.Fatalf("during Start: %q", got)
	}
}

// LIONGARD_BASE_URL is set for credkit.Start only; AfterStart puts it back as
// the process found it, unset or set.
func TestAfterStartRestoresTheBaseURL(t *testing.T) {
	for name, prior := range map[string]string{"unset": "", "set": "https://prior.example/api/v1"} {
		t.Run(name, func(t *testing.T) {
			isolate(t)
			t.Setenv("LIONGARD_INSTANCE", "acme")
			if prior != "" {
				t.Setenv("LIONGARD_BASE_URL", prior)
			}
			showBaseURL(nil)
			want := "https://acme.app.liongard.com/api/v1"
			if prior != "" {
				want = prior
			}
			if got := os.Getenv("LIONGARD_BASE_URL"); got != want {
				t.Fatalf("during Start: %q, want %q", got, want)
			}
			AfterStart()
			got, ok := os.LookupEnv("LIONGARD_BASE_URL")
			if prior == "" && ok {
				t.Fatalf("left set to %q", got)
			}
			if prior != "" && got != prior {
				t.Fatalf("got %q, want %q", got, prior)
			}
		})
	}
}

// With no instance (config.Load's placeholder "example") or a template left
// unfilled, the kit is shown the shipped template, which does not parse as a
// URL: no origin is allowed and every credentialed request is refused.
func TestUnresolvedBaseURLAllowsNoOrigin(t *testing.T) {
	for name, instance := range map[string]string{"no instance": "", "placeholder instance": "example"} {
		t.Run(name, func(t *testing.T) {
			isolate(t)
			if instance != "" {
				t.Setenv("LIONGARD_INSTANCE", instance)
			}
			if got := configuredBase(nil); got != "" {
				t.Fatalf("want nothing to show, got %q", got)
			}
			showBaseURL(nil)
			got := os.Getenv("LIONGARD_BASE_URL")
			AfterStart()
			if got != shippedBaseURL {
				t.Fatalf("during Start: %q, want the shipped template", got)
			}
		})
	}
	isolate(t)
	cfg := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfg, []byte("base_url = \"https://{tenant}.other.example/api\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := configuredBase([]string{"version", "--config", cfg}); got != "" {
		t.Fatalf("an unfilled template showed %q", got)
	}
}

// The store redacts by name before the kit's SQL driver sees a record; its names
// are the declared response fields.
func TestStoreRedactsTheDeclaredResponseFields(t *testing.T) {
	if !slices.Equal(store.CredentialResponseFields, Declarations.ResponseFields) {
		t.Errorf("store.CredentialResponseFields %v differs from Declarations.ResponseFields %v",
			store.CredentialResponseFields, Declarations.ResponseFields)
	}
}
