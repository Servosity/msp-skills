// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import "fmt"

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
