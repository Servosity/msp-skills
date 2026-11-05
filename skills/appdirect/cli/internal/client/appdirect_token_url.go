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
//
// appdirect handfix per-marketplace-token-url (Codex r1 4.34.2 class
// credential-sent-to-another-host): the token URL is where the client ID and
// secret are posted, so it comes only from what the operator configured and
// Load trusted. A base URL it cannot derive an origin from (userinfo, no host,
// a scheme other than http or https, a non-ASCII host, a URL that does not
// parse) no longer falls back to AppDirect's own marketplace, a token URL is
// held to the same rule, and a route Load refused (a configuration file other
// users can change names the host) names no token URL at all. No token URL
// means nothing is posted: the mint, the refresh grant and `auth login` refuse
// with ErrNoAppDirectTokenURL, and the host guard pins no token origin.

package client

import (
	"errors"
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

// ErrNoAppDirectTokenURL marks a token request refused because the
// configuration names no host the client ID and secret may be posted to.
var ErrNoAppDirectTokenURL = errors.New("no token URL to send the client ID and secret to: set APPDIRECT_BASE_URL to your marketplace (https://<marketplace>/api, no user name or password in it) or APPDIRECT_TOKEN_URL to its https://<marketplace>/oauth2/token")

// ResolveAppDirectTokenURL picks the OAuth2 token endpoint in priority order:
// explicit TokenURL config, then scheme://host of the configured BaseURL plus
// /oauth2/token, then the spec-baked default when no base URL is configured
// at all. It returns "" (no token request may be sent) when the configured
// value cannot name a host safely or Load refused the credential route.
func ResolveAppDirectTokenURL(cfg *config.Config) string {
	if cfg == nil {
		return defaultAppDirectTokenURL
	}
	if cfg.CredentialRouteBlocked() {
		return ""
	}
	if raw := strings.TrimSpace(cfg.TokenURL); raw != "" {
		u, ok := tokenSafeURL(raw)
		if !ok {
			return ""
		}
		return u.String()
	}
	raw := strings.TrimSpace(cfg.BaseURL)
	if raw == "" {
		return defaultAppDirectTokenURL
	}
	if !strings.Contains(raw, "://") {
		// Scheme-less base URL (e.g. "mp.example.com/api"): assume https
		// rather than silently minting against AppDirect's own tenant.
		raw = "https://" + raw
	}
	u, ok := tokenSafeURL(raw)
	if !ok {
		return ""
	}
	return u.Scheme + "://" + u.Host + "/oauth2/token"
}

// tokenSafeURL parses raw and accepts it only as an http or https URL with a
// non-empty ASCII host, no userinfo, and nothing that would make the host it
// names differ from the host it is sent to.
func tokenSafeURL(raw string) (*url.URL, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Opaque != "" || u.Hostname() == "" {
		return nil, false
	}
	switch strings.ToLower(u.Scheme) {
	case "https", "http":
	default:
		return nil, false
	}
	for i := 0; i < len(u.Host); i++ {
		if c := u.Host[i]; c >= 0x80 || c <= ' ' || c == '@' || c == '\\' {
			return nil, false
		}
	}
	u.Scheme = strings.ToLower(u.Scheme)
	return u, true
}
