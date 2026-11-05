// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (netsapiens; handfixes.json doctor-verdict-tracks-reality).

package config

// ServerIsShippedDefault reports that the {server} template var was filled
// with the shipped default (the public sandbox ns-api.com) because neither
// NETSAPIENS_SERVER nor the config's template_vars set it.
func (c *Config) ServerIsShippedDefault() bool {
	return c != nil && c.serverDefaulted
}
