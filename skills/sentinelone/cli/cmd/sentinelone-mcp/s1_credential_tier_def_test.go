// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): the shared credential-tier definition
// (cliutil.CredentialTiers and its matchers) as the MCP choke point uses it.

package main

import (
	"fmt"
	"testing"

	"sentinelone-pp-cli/internal/cliutil"
	mcptools "sentinelone-pp-cli/internal/mcp"
)

// TestCredentialTierDefinitionAndSpellings pins the one shared definition:
// every secret read path and every never-stored resource is a member, the
// documented spellings match, and ordinary names (including the endpoint ids
// an agent passes to sentinelone_execute) do not.
func TestCredentialTierDefinitionAndSpellings(t *testing.T) {
	members := map[string]bool{}
	for _, m := range cliutil.CredentialTiers() {
		members[m.Path] = true
	}
	for _, p := range cliutil.SecretReadPaths() {
		if !members[p] {
			t.Errorf("secret read path %s is not in the credential tier", p)
		}
		if _, ok := cliutil.CredentialTierPath(p); !ok {
			t.Errorf("CredentialTierPath(%s) = false", p)
		}
		if len(mcptools.CredentialTierEndpointIDs(p)) == 0 {
			t.Errorf("no sentinelone_execute endpoint reads %s; a refusal could not name the confirmed path", p)
		}
	}
	for name := range cliutil.NeverStoredResources() {
		if _, ok := cliutil.CredentialTierResource(name); !ok {
			t.Errorf("never-stored resource %s is not in the credential tier", name)
		}
	}
	for _, name := range []string{
		"ranger", "RANGER", "/ranger", "/web/api/v2.1/ranger", "Ranger_Cred_Groups", "rangerCredGroups", "ranger-cred-groups-details",
		"agents-passphrases", "agents_passphrases", "AgentsPassphrases", "/agents/passphrases",
		"uninstall-password", "uninstall_password", "UninstallPassword", "login-by-token", "loginByToken", "users_login_by_token",
	} {
		if _, ok := cliutil.CredentialTierResource(name); !ok {
			t.Errorf("CredentialTierResource(%q) = false", name)
		}
	}
	if _, ok := cliutil.CredentialTierTable("uninstall_password"); !ok {
		t.Error(`CredentialTierTable("uninstall_password") = false`)
	}
	for _, p := range []string{
		"/ranger/cred-groups/details", "https://x.sentinelone.net/web/api/v2.1/ranger/cred-groups/details?limit=5",
		"/accounts/123/uninstall-password/view", "123/uninstall-password/view", "/accounts/123%2Funinstall-password%2Fview",
		"/accounts/123%252Funinstall-password%252Fview", "/x/../users/login/by-token", `ranger\cred-groups\details`,
	} {
		if _, ok := cliutil.CredentialTierPath(p); !ok {
			t.Errorf("CredentialTierPath(%q) = false", p)
		}
	}
	for _, ordinary := range []string{
		"agents", "accounts", "ranger-table-view", "ranger-gateways", "rangerStatus", "ranger_exposure", "enable-ranger",
		"ranger.get-cred-group-details", "agents.get-passphrase", "users.login-by-token", "accounts.uninstall-password.get-metadata",
		"/accounts/123/uninstall-password/metadata", "/ranger/table-view", "sites.regenerate-key",
	} {
		if m, ok := cliutil.CredentialTierInText(ordinary); ok {
			t.Errorf("CredentialTierInText(%q) matched %v; it is not a credential-tier name", ordinary, m)
		}
	}
	if fmt.Sprint(mcptools.CredentialTierEndpointIDs("/ranger/cred-groups/details")) != "[ranger.get-cred-group-details]" {
		t.Errorf("ranger endpoint ids = %v", mcptools.CredentialTierEndpointIDs("/ranger/cred-groups/details"))
	}
}
