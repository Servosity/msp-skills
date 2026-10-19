// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"fmt"
	"strings"
)

// SaveTemplateVars persists endpoint template values (cloudId, workspaceId)
// to the config file's [template_vars] table so Assets commands work without
// exporting env vars. Only the file's own values plus vars are written: env
// overrides (JIRA_SERVICE_MANAGEMENT_BASE_URL, _SITE, ...) must not leak into
// the file. A config file that still holds inline credentials is refused:
// save() drops them, and the generated migration writes them to the default
// credentials file, which with --config <tenant> would overwrite another
// tenant's token.
func (c *Config) SaveTemplateVars(vars map[string]string) error {
	for _, p := range []string{c.Path, c.legacySourcePath} {
		if ok, _ := FileHasCredentialFields(p); p != "" && ok {
			return fmt.Errorf("%s holds inline credentials; refusing to rewrite it", p)
		}
	}
	merge := func(dst map[string]string) map[string]string {
		out := cloneStringMap(dst)
		if out == nil {
			out = map[string]string{}
		}
		for k, v := range vars {
			if v != "" {
				out[k] = v
			}
		}
		return out
	}
	liveBase, liveVars := c.BaseURL, c.TemplateVars
	if c.fileConfig != nil {
		c.BaseURL = c.fileConfig.BaseURL
		c.TemplateVars = merge(c.fileConfig.TemplateVars)
	} else {
		c.TemplateVars = merge(nil)
	}
	err := c.save()
	c.BaseURL = liveBase
	c.TemplateVars = merge(liveVars)
	return err
}

// Assets ids saved by `cmdb discover --save` are bound to the site they were
// discovered for: an MSP switching JIRA_SERVICE_MANAGEMENT_SITE between client
// sites must never send Assets reads, deletes or syncs to the previous
// client's CMDB. They are stored as cloudId@<site> / workspaceId@<site>.
var jsmAssetsIDKeys = []string{"cloudId", "workspaceId"}

// JSMSiteBoundKey is the [template_vars] key that holds an Assets id for site.
func JSMSiteBoundKey(key, site string) string { return key + "@" + site }

// bindAssetsIDsToSite runs at load, after the site is resolved and before the
// CLOUD_ID / WORKSPACE_ID env overrides: only ids saved for this site apply.
func (c *Config) bindAssetsIDsToSite() {
	site := strings.TrimSpace(c.TemplateVars["site"])
	for _, k := range jsmAssetsIDKeys {
		delete(c.TemplateVars, k)
		if v := c.TemplateVars[JSMSiteBoundKey(k, site)]; site != "" && v != "" {
			c.TemplateVars[k] = v
		}
	}
}

// RebindAssetsIDs re-applies that rule after the site changed at call time
// (sync --path-context site=...): ids passed explicitly win, else the new
// site's saved ids, else none, so Assets resources fail instead of reading
// another client's workspace.
func (c *Config) RebindAssetsIDs(explicit map[string]string) {
	site := strings.TrimSpace(c.TemplateVars["site"])
	for _, k := range jsmAssetsIDKeys {
		if v := explicit[k]; v != "" {
			c.TemplateVars[k] = v
			continue
		}
		delete(c.TemplateVars, k)
		if v := c.TemplateVars[JSMSiteBoundKey(k, site)]; site != "" && v != "" {
			c.TemplateVars[k] = v
		}
	}
}
