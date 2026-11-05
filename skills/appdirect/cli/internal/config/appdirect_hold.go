// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package config

import "appdirect-pp-cli/internal/cliutil"

// holdConfigCredentials registers every credential the configuration carries
// (environment, config file or credential store) with the one holder the
// output chokepoint reads (appdirect handfix: output-chokepoint): the credential
// fields, the Authorization header, header values, the userinfo password and
// credential-named query values of the base URL (Codex r1 class
// output-before-chokepoint: a refusal message repeats a config's base_url),
// and the same of the file layer an environment value overrode.
func holdConfigCredentials(c *Config) {
	if c == nil {
		return
	}
	cliutil.FunnelHold(c.ClientID, c.ClientSecret, c.AccessToken, c.RefreshToken)
	cliutil.HoldKeys(c.AuthHeaderVal, c.AuthHeader()) // the header whole and without its Bearer scheme
	for _, v := range c.Headers {
		cliutil.FunnelHold(v)
	}
	cliutil.HoldURLCredentials(c.BaseURL)
	if c.fileConfig != nil && c.fileConfig != c {
		holdConfigCredentials(c.fileConfig)
	}
}
