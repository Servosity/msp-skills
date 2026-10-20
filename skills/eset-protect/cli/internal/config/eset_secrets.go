// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored (not generator output); pinned by skills/eset-protect/handfixes.json
// under "secret-registry-masks-every-boundary".

package config

import (
	"strings"

	"eset-protect-pp-cli/internal/secrets"
)

// registerSecrets records every credential field of c in the process-wide
// secret registry, so any text that echoes one is masked wherever it leaves
// the process. Load and SaveTokens call it.
func (c *Config) registerSecrets() {
	if c == nil {
		return
	}
	secrets.Register(c.AuthHeaderVal, c.AccessToken, c.RefreshToken, c.ClientSecret, c.EsetProtectPassword, c.EsetProtectToken)
	// A custom request header from the config file is a credential when its
	// name says so (Authorization, Cookie, an API key, token or secret);
	// other header values (Accept, User-Agent) are ordinary text.
	for name, value := range c.Headers {
		if credentialHeaderName(name) {
			secrets.Register(value)
		}
	}
}

func credentialHeaderName(name string) bool {
	n := strings.ToLower(name)
	for _, word := range []string{"authorization", "cookie", "token", "secret", "key", "password", "session"} {
		if strings.Contains(n, word) {
			return true
		}
	}
	return false
}
