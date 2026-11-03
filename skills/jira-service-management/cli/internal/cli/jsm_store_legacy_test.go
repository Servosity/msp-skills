// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"os"
	"path/filepath"
	"testing"

	"jira-service-management-pp-cli/internal/config"
)

// Handfix jsm-store-scope-by-site: a store named by the credential alone
// (data-<hash(credential)>.db) belongs to no site in particular, so no site may
// adopt it: the first site to run would take another client's rows (an Acme
// mirror read as Beta's sla-report).
func TestNoSiteAdoptsACredentialOnlyStore(t *testing.T) {
	cfg := &config.Config{
		BaseURL:                       "https://{site}.atlassian.net/rest/servicedeskapi",
		TemplateVars:                  map[string]string{"site": "beta"},
		JiraServiceManagementEmail:    "agent@example.com",
		JiraServiceManagementApiToken: "valid-api-token",
	}
	if legacy := cfg.StoreScopeLegacyCredential(); legacy != "" {
		t.Fatalf("a credential-only legacy scope %q is offered for adoption", legacy)
	}

	dir := t.TempDir()
	legacy := filepath.Join(dir, "data-"+hashDefaultDBScope(cfg.AuthHeader())+".db")
	if err := os.WriteFile(legacy, []byte("acme rows"), 0o600); err != nil {
		t.Fatal(err)
	}
	setDefaultDBScopeIdentity(cfg.StoreScopeCredential(), cfg.StoreScopeLegacyCredential(), cfg.BaseURL)
	t.Cleanup(func() { setDefaultDBScopeIdentity("", "", "") })
	got := defaultDBPathInDir(dir)
	if got == legacy {
		t.Fatalf("site beta reads the credential-only store %s", legacy)
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Fatalf("the credential-only store was moved or removed: %v", err)
	}
}

// An unscoped data.db with no .owner marker predates the site scope: no site
// claims it (the first to run would take another client's rows).
func TestNoSiteClaimsAnUnownedUnscopedStore(t *testing.T) {
	cfg := &config.Config{
		BaseURL:                       "https://{site}.atlassian.net/rest/servicedeskapi",
		TemplateVars:                  map[string]string{"site": "beta"},
		JiraServiceManagementEmail:    "agent@example.com",
		JiraServiceManagementApiToken: "valid-api-token",
	}
	dir := t.TempDir()
	unscoped := filepath.Join(dir, "data.db")
	if err := os.WriteFile(unscoped, []byte("acme rows"), 0o600); err != nil {
		t.Fatal(err)
	}
	setDefaultDBScopeIdentity(cfg.StoreScopeCredential(), cfg.StoreScopeLegacyCredential(), cfg.BaseURL)
	t.Cleanup(func() { setDefaultDBScopeIdentity("", "", "") })
	if got := defaultDBPathInDir(dir); got == unscoped {
		t.Fatalf("site beta reads the unowned unscoped store %s", unscoped)
	}
	if _, err := os.Stat(unscoped + ".owner"); err == nil {
		t.Fatal("site beta wrote an owner marker claiming the unscoped store")
	}
}
