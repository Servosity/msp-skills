// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"
	"time"

	"eset-protect-pp-cli/internal/client"
	"eset-protect-pp-cli/internal/cliutil/testenv"
	"eset-protect-pp-cli/internal/config"
)

// hand-fix guard (handfixes.json: install-env-declared-on-every-channel): the
// install channels hand every optional variable to the binary as "" (the
// mcp-install.md env blocks) or as an unresolved ${user_config.*} placeholder
// (an MCPB host that skips a blank optional field). Both must read as unset:
// a TOKEN of "" or of the placeholder text must not become the bearer and
// shadow the email + password grant, and region/base URL fall back to their
// defaults.
func TestOptionalInstallEnvBlankOrPlaceholderIsUnset(t *testing.T) {
	for _, blank := range []string{"", "${user_config.eset_protect_token}"} {
		testenv.Isolate(t)
		withTokenServer(t)
		t.Setenv("ESET_PROTECT_USERNAME", "api@msp.example")
		t.Setenv("ESET_PROTECT_PASSWORD", "s3cret")
		t.Setenv("ESET_PROTECT_TOKEN", blank)
		t.Setenv("ESET_PROTECT_REGION", map[string]string{"": "", "${user_config.eset_protect_token}": "${user_config.eset_protect_region}"}[blank])
		t.Setenv("ESET_PROTECT_BASE_URL", map[string]string{"": "", "${user_config.eset_protect_token}": "${user_config.eset_protect_base_url}"}[blank])
		cfg, err := config.Load(t.TempDir() + "/config.toml")
		if err != nil {
			t.Fatal(err)
		}
		if cfg.EsetProtectToken != "" {
			t.Fatalf("TOKEN %q was taken as a credential: %q", blank, cfg.EsetProtectToken)
		}
		if cfg.TemplateVars["region"] != "eu" || cfg.BaseURL != "https://{region}.device-management.eset.systems" {
			t.Fatalf("blank region/base URL did not fall back: region=%q base=%q", cfg.TemplateVars["region"], cfg.BaseURL)
		}
		c := client.New(cfg, 5*time.Second, 0)
		if err := esetClientHook(c); err != nil {
			t.Fatalf("TOKEN %q: password grant did not run: %v", blank, err)
		}
		if got := cfg.AuthHeader(); got != "Bearer tok-1" {
			t.Fatalf("TOKEN %q: Authorization = %q, want the minted token", blank, got)
		}
	}
}
