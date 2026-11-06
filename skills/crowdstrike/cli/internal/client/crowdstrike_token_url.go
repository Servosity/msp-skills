// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored CrowdStrike-specific helper: each Falcon cloud (US-1
// api.crowdstrike.com, US-2 api.us-2.crowdstrike.com, EU-1
// api.eu-1.crowdstrike.com, US-GOV-1 api.laggar.gcw.crowdstrike.com) serves
// its own OAuth2 token endpoint at <cloud>/oauth2/token (FalconPy derives it
// the same way). When the operator overrides CROWDSTRIKE_BASE_URL but does not
// set CROWDSTRIKE_TOKEN_URL, derive the token endpoint from the base URL so
// client-credentials minting posts the client ID and secret to the cloud the
// operator configured, never to US-1 by default.
//
// Exported because internal/cli's `auth login` and the fleet member_cid mint
// share the exact same derivation (cli already depends on client; keeping one
// copy prevents the paths from silently diverging).
//
// crowdstrike handfix per-cloud-token-url (class credential-sent-to-another-
// host): the token URL is where the client ID and secret are posted, so it
// comes only from what the operator configured and Load trusted. A base URL it
// cannot derive an origin from (userinfo, no host, a scheme other than http or
// https, a non-ASCII host, a URL that does not parse) never falls back to
// US-1, a token URL is held to the same rule, and a route Load refused (a
// configuration file other users can change names the host) names no token
// URL at all. No token URL means nothing is posted: the mint, the refresh
// grant, `auth login` and `fleet sync` refuse with ErrNoCrowdStrikeTokenURL,
// and the host guard pins no token origin.

package client

import (
	"errors"
	"net/url"
	"strings"

	"crowdstrike-pp-cli/internal/config"
)

// defaultUS1MintEndpoint is the spec-baked fallback (US-1 /oauth2/token) used
// when no base URL or token URL override is configured. It is an endpoint URL,
// not a credential; client_id/client_secret are posted to it at request time.
const defaultUS1MintEndpoint = "https://api.crowdstrike.com/oauth2/token"

// ErrNoCrowdStrikeTokenURL marks a token request refused because the
// configuration names no host the client ID and secret may be posted to.
var ErrNoCrowdStrikeTokenURL = errors.New("no token URL to send the client ID and secret to: set CROWDSTRIKE_BASE_URL to your Falcon cloud (for example https://api.eu-1.crowdstrike.com, no user name or password in it) or CROWDSTRIKE_TOKEN_URL to its https://<cloud>/oauth2/token")

// ResolveCrowdStrikeTokenURL picks the OAuth2 token endpoint in priority order:
// explicit TokenURL config, then scheme://host of the configured BaseURL plus
// /oauth2/token, then the spec-baked default when no base URL is configured
// at all. It returns "" (no token request may be sent) when the configured
// value cannot name a host safely or Load refused the credential route.
func ResolveCrowdStrikeTokenURL(cfg *config.Config) string {
	if cfg == nil {
		return defaultUS1MintEndpoint
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
		return defaultUS1MintEndpoint
	}
	if !strings.Contains(raw, "://") {
		// Scheme-less base URL (e.g. "api.eu-1.crowdstrike.com"): assume https
		// rather than silently minting against US-1.
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
