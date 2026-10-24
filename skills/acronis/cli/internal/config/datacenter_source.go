// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"strings"

	"acronis-pp-cli/internal/cliutil"
)

// DatacenterChosen reports whether the operator chose the Acronis datacenter,
// through ACRONIS_DATACENTER or template_vars.datacenter in the config file.
// Load fills an unset datacenter with eu2-cloud; that fill-in is not a choice,
// so doctor must not report a host the operator never picked as their API
// (hand-fix handfixes.json doctor-verdict-tracks-reality).
func (c *Config) DatacenterChosen() bool {
	if c == nil {
		return false
	}
	if strings.TrimSpace(cliutil.EnvOverride("ACRONIS_DATACENTER")) != "" {
		return true
	}
	return c.fileConfig != nil && strings.TrimSpace(c.fileConfig.TemplateVars["datacenter"]) != ""
}
