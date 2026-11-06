// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

// autotask handfix (config-file-holds-no-credential): saving the configuration
// (auth set-token, zone) writes base_url as this run has it, environment value
// included, so a userinfo password or a credential query value that
// AUTOTASK_BASE_URL carried landed in config.toml. config.toml is written
// outside the credential kit (it is a credential source), so the kit cannot
// stop it. Autotask authenticates with three headers and never needs either
// part of the URL, so config.toml keeps base_url without its userinfo and
// query. Structural only: no value is matched.

import (
	"fmt"
	"net/url"
	"os"
)

func persistedWithoutCredentials(p persistedConfig) persistedConfig {
	u, err := url.Parse(p.BaseURL)
	switch {
	case err != nil:
		// A base URL that does not parse cannot be used, and its parts cannot
		// be told apart: keep the built-in one instead.
		p.BaseURL = builtinBaseURL()
	case u.User != nil || u.RawQuery != "" || u.ForceQuery:
		u.User, u.RawQuery, u.ForceQuery = nil, "", false
		p.BaseURL = u.String()
	default:
		return p
	}
	fmt.Fprintln(os.Stderr, "warning: config.toml does not keep the userinfo or query of base_url; they stay in AUTOTASK_BASE_URL")
	return p
}
