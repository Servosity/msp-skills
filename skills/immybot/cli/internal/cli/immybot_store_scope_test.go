// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"os"
	"path/filepath"
	"testing"

	"immybot-pp-cli/internal/cliutil"
	"immybot-pp-cli/internal/cliutil/testenv"
	"immybot-pp-cli/internal/config"
)

// Two ImmyBot instances (spec.json servers[0].variables.subdomain) signed in
// with one Entra app registration are two stores; one instance is one store
// across runs; a logged-out CLI is scoped by the instance too.
func TestStoreScopeNamesTheInstance(t *testing.T) {
	cfg := func(sub string) *config.Config {
		return &config.Config{BaseURL: "https://{subdomain}.immy.bot", TemplateVars: map[string]string{"subdomain": sub},
			ClientID: "app-id", ClientSecret: "app-secret", ImmybotTenantId: "entra-tenant"}
	}
	alpha, beta := cfg("alpha").StoreScopeCredential(), cfg("beta").StoreScopeCredential()
	if alpha == "" || alpha == beta {
		t.Fatalf("alpha and beta share a store scope (%q)", alpha)
	}
	if again := cfg("alpha").StoreScopeCredential(); again != alpha {
		t.Fatalf("one instance moved store: %q then %q", alpha, again)
	}
	other := cfg("alpha")
	other.ClientID = "other-app"
	if other.StoreScopeCredential() == alpha {
		t.Fatal("two app registrations on one instance share a store scope")
	}
	out := func(sub string) string {
		return (&config.Config{BaseURL: "https://{subdomain}.immy.bot", TemplateVars: map[string]string{"subdomain": sub}}).StoreScopeCredential()
	}
	if out("alpha") == "" || out("alpha") == out("beta") {
		t.Fatalf("logged-out alpha and beta share a store scope (%q)", out("alpha"))
	}
}

// Every store key names the instance, whatever signs in: a token-only login
// and a logged-out CLI are scoped by the instance too, and a store kept from
// before the instance key (data.db, or one named by the credential alone) is
// never adopted or claimed, since whose instance it holds is unknown
// (round-1 next-batch review).
func TestEveryStoreKeyNamesTheInstance(t *testing.T) {
	home := testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	for _, k := range []string{"IMMYBOT_CLIENT_ID", "IMMYBOT_CLIENT_SECRET", "IMMYBOT_TENANT_ID", "IMMYBOT_BASE_URL", "IMMYBOT_TOKEN_URL"} {
		t.Setenv(k, "")
	}
	dir := t.TempDir()
	path := func(sub, configBody string) string {
		t.Helper()
		cfgPath := filepath.Join(home, sub+"-config.toml")
		if err := os.WriteFile(cfgPath, []byte(configBody), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("IMMYBOT_SUBDOMAIN", sub)
		configureDefaultDBScope(cfgPath)
		return defaultDBPathInDir(dir)
	}
	const tokenOnly = "base_url = \"https://{subdomain}.immy.bot\"\naccess_token = \"token-A\"\n"
	const loggedOut = "base_url = \"https://{subdomain}.immy.bot\"\n"
	unscoped := filepath.Join(dir, "data.db")
	legacy := filepath.Join(dir, "data-"+hashDefaultDBScope("Bearer token-A")+".db")
	for _, f := range []string{unscoped, legacy} {
		if err := os.WriteFile(f, []byte("alpha's mirror"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for name, body := range map[string]string{"token-only": tokenOnly, "logged out": loggedOut} {
		beta, alpha := path("beta", body), path("alpha", body)
		if beta == alpha || beta == unscoped || beta == legacy || alpha == unscoped || alpha == legacy {
			t.Errorf("%s: alpha %s and beta %s are not each their instance's own store", name, filepath.Base(alpha), filepath.Base(beta))
		}
	}
	for _, f := range []string{unscoped, legacy} {
		if got, err := os.ReadFile(f); err != nil || string(got) != "alpha's mirror" {
			t.Errorf("%s was adopted or claimed (%v)", filepath.Base(f), err)
		}
	}
	if _, err := os.Stat(unscoped + ".owner"); !os.IsNotExist(err) {
		t.Errorf("data.db was claimed by an instance (%v)", err)
	}
}
