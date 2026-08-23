// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored. Kept in its own file so `generate --force` preserves it.
//
// ImmyBot's Entra application publishes the tenant's own instance URL as its
// App ID URI, so the client-credentials scope is
// https://<subdomain>.immy.bot/.default -- instance-specific. A spec-baked
// static scope cannot express that, and the generator's default for any
// Microsoft Entra token URL is api://{client_id}/.default, which names the
// caller's own app registration rather than ImmyBot as the resource and is
// therefore always wrong here.
//
// The generated resolveClientCredentialsScope() reads IMMYBOT_OAUTH_SCOPE
// before falling back to that default, so populating the env var before any
// token mint is the durable seam. An operator who sets IMMYBOT_OAUTH_SCOPE
// explicitly still wins.

package cli

import (
	"os"
	"strings"
)

func init() { applyDerivedOAuthScope() }

// applyDerivedOAuthScope sets IMMYBOT_OAUTH_SCOPE from the configured instance
// when the operator has not set it. It is a no-op when the scope is already
// set or when neither the base URL nor the subdomain is known.
func applyDerivedOAuthScope() {
	if strings.TrimSpace(os.Getenv("IMMYBOT_OAUTH_SCOPE")) != "" {
		return
	}
	base := strings.TrimSpace(os.Getenv("IMMYBOT_BASE_URL"))
	if base == "" {
		sub := strings.TrimSpace(os.Getenv("IMMYBOT_SUBDOMAIN"))
		if sub == "" || sub == "your-instance" {
			return
		}
		base = "https://" + sub + ".immy.bot"
	}
	base = strings.TrimRight(base, "/")
	if base == "" {
		return
	}
	// os.Setenv only fails on a malformed variable name; the name here is a
	// compile-time literal, so the error is unreachable. Discarded explicitly
	// rather than left bare so the intent is visible to readers and linters.
	_ = os.Setenv("IMMYBOT_OAUTH_SCOPE", base+"/.default")
}
