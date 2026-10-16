// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"testing"

	"jira-service-management-pp-cli/internal/config"
)

// The store scope must separate two sites reached with one Atlassian token.
func TestStoreScopeSeparatesSites(t *testing.T) {
	cfg := func(site string) *config.Config {
		return &config.Config{
			BaseURL:                       "https://{site}.atlassian.net/rest/servicedeskapi",
			TemplateVars:                  map[string]string{"site": site},
			JiraServiceManagementEmail:    "tech@example.com",
			JiraServiceManagementApiToken: "test-secret",
		}
	}
	a, b := cfg("acme").StoreScopeCredential(), cfg("northwind").StoreScopeCredential()
	if a == "" || a == b {
		t.Fatalf("same token on two sites shares a store scope: %q vs %q", a, b)
	}
	if a != cfg("acme").StoreScopeCredential() {
		t.Fatal("store scope is not stable for one site")
	}
}
