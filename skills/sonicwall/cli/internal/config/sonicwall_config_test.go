// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"path/filepath"
	"testing"
)

func TestStoreScopeIdentityBindsTenantToSavedToken(t *testing.T) {
	dir := t.TempDir()
	c := &Config{Path: filepath.Join(dir, "config.toml"), SonicwallMswApiKey: "msw-key", AccessToken: "tok-A",
		envOverrides: map[string]bool{}, TemplateVars: map[string]string{"region": "uswest"}}
	if got := StoreScopeIdentity(c); got != "" {
		t.Fatalf("no recorded login: identity = %q, want default", got)
	}
	if err := c.SaveNSMLogin(NSMLogin{Tenant: "A", TenantSerial: "SER-A", Region: "uswest"}, "tok-A"); err != nil {
		t.Fatal(err)
	}
	idA := StoreScopeIdentity(c)
	if idA == "" {
		t.Fatal("matching token: expected a tenant-pinned identity")
	}
	// A rotated token recorded by nsm-login for the same tenant keeps the same identity.
	c.AccessToken = "tok-A2"
	if err := c.SaveNSMLogin(NSMLogin{Tenant: "A", TenantSerial: "SER-A", Region: "uswest"}, "tok-A2"); err != nil {
		t.Fatal(err)
	}
	if got := StoreScopeIdentity(c); got != idA {
		t.Fatalf("rotated token, same tenant: identity %q, want %q", got, idA)
	}
	// An auth_header outranks the saved token, so the record does not describe it.
	c.AuthHeaderVal = "Bearer tok-B"
	if got := StoreScopeIdentity(c); got != "" {
		t.Fatalf("auth_header set: identity = %q, want default", got)
	}
	c.AuthHeaderVal = ""
	// A token the record does not describe falls back to credential isolation.
	c.AccessToken = "tok-B"
	if got := StoreScopeIdentity(c); got != "" {
		t.Fatalf("unrecorded token: identity = %q, want default", got)
	}
	// An env-supplied token never borrows the recorded tenant.
	c.AccessToken = "tok-A2"
	c.envOverrides["SonicwallNsmToken"] = true
	c.SonicwallNsmToken = "env-token"
	if got := StoreScopeIdentity(c); got != "" {
		t.Fatalf("env token: identity = %q, want default", got)
	}
	// Another config file in the same directory has its own record.
	other := &Config{Path: filepath.Join(dir, "work.toml")}
	if _, ok := other.LoadNSMLogin(); ok {
		t.Fatal("a second config file must not see the first one's tenant record")
	}
}
