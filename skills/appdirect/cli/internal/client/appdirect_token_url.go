// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored AppDirect-specific helper: every AppDirect tenant runs at its
// own marketplace host, and the OAuth token endpoint lives on that same host
// at /oauth2/token. When the operator overrides APPDIRECT_BASE_URL but does
// not set APPDIRECT_TOKEN_URL, derive the token endpoint from the base URL so
// client-credentials minting hits their marketplace instead of the spec-baked
// default (marketplace.appdirect.com, AppDirect's own instance).
//
// Exported because internal/cli's `auth login` shares the exact same
// derivation (cli already depends on client; keeping one copy prevents the
// two paths from silently diverging).

package client

import (
	"net/url"
	"strings"

	"appdirect-pp-cli/internal/config"
)

// defaultAppDirectTokenURL is the spec-baked fallback used when no base URL
// or token URL override is configured.
//
// #nosec G101 -- this is AppDirect's public OAuth2 token endpoint URL, not a
// credential. The literal carries no secret; client_id/client_secret are read
// from config at request time and posted to this endpoint.
const defaultAppDirectTokenURL = "https://marketplace.appdirect.com/oauth2/token"

// ResolveAppDirectTokenURL picks the OAuth2 token endpoint in priority order:
// explicit TokenURL config, then scheme://host of the configured BaseURL plus
// /oauth2/token, then the spec-baked default.
//
// The derived host is the POST target for client_id+client_secret, so the
// parse is defensive: URLs carrying a userinfo component ("a@b" — the classic
// host-confusion footgun) or yielding an empty hostname fall back to the
// default rather than routing credentials to a surprise host.
func ResolveAppDirectTokenURL(cfg *config.Config) string {
	if cfg != nil && cfg.TokenURL != "" {
		return cfg.TokenURL
	}
	if cfg != nil && cfg.BaseURL != "" {
		raw := strings.TrimSpace(cfg.BaseURL)
		if u, err := url.Parse(raw); err == nil && u.Scheme != "" && u.Host != "" {
			if u.User == nil && u.Hostname() != "" {
				return u.Scheme + "://" + u.Host + "/oauth2/token"
			}
			// Scheme present but authority is suspect (userinfo or empty
			// hostname): never retry with a prepend — fall back to default.
			return defaultAppDirectTokenURL
		}
		// Scheme-less base URL (e.g. "mp.example.com/api"): assume https
		// rather than silently minting against AppDirect's own tenant.
		if u, err := url.Parse("https://" + raw); err == nil && u.Host != "" && u.User == nil && u.Hostname() != "" {
			return "https://" + u.Host + "/oauth2/token"
		}
	}
	return defaultAppDirectTokenURL
}
