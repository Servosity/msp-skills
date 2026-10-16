// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"path/filepath"
	"testing"
)

// Assets ids saved for one site must never apply to another.
func TestAssetsIDsAreBoundToTheirSite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	t.Setenv("JIRA_SERVICE_MANAGEMENT_CLOUD_ID", "")
	t.Setenv("JIRA_SERVICE_MANAGEMENT_WORKSPACE_ID", "")
	t.Setenv("JIRA_SERVICE_MANAGEMENT_SITE", "acme")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.SaveTemplateVars(map[string]string{JSMSiteBoundKey("cloudId", "acme"): "cloud-a", JSMSiteBoundKey("workspaceId", "acme"): "ws-a"}); err != nil {
		t.Fatal(err)
	}
	load := func(site string) map[string]string {
		t.Setenv("JIRA_SERVICE_MANAGEMENT_SITE", site)
		c, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		return c.TemplateVars
	}
	if v := load("acme"); v["cloudId"] != "cloud-a" || v["workspaceId"] != "ws-a" {
		t.Fatalf("acme lost its saved ids: %v", v)
	}
	if v := load("beta"); v["cloudId"] != "" || v["workspaceId"] != "" {
		t.Fatalf("beta inherited acme's Assets ids: %v", v)
	}

	t.Setenv("JIRA_SERVICE_MANAGEMENT_SITE", "acme")
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	c.TemplateVars["site"] = "beta" // what sync --path-context site=beta does
	c.RebindAssetsIDs(map[string]string{"site": "beta"})
	if c.TemplateVars["cloudId"] != "" {
		t.Fatalf("path-context switch kept acme's cloudId: %v", c.TemplateVars)
	}
	c.RebindAssetsIDs(map[string]string{"site": "beta", "cloudId": "cloud-b"})
	if c.TemplateVars["cloudId"] != "cloud-b" {
		t.Fatalf("explicit cloudId ignored: %v", c.TemplateVars)
	}
}
