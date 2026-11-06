// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored Nerdio-specific helper: the NMM Partner API authenticates
// with an Entra ID client-credentials token minted from the MSP's OWN tenant
// (https://login.microsoftonline.com/<TENANT_ID>/oauth2/v2.0/token) for the
// NMM application's scope (<app-id>/.default). Neither has a usable default.
//
// nerdio handfix token-url-from-operator-only: the token URL is where the
// client ID and secret are posted, so it comes only from what the operator
// configured (NERDIO_TOKEN_URL or token_url in the config file) and Load
// trusted. The press baked a placeholder tenant
// (login.microsoftonline.com/YOUR_TENANT_ID/...) as the fallback, so an
// operator who forgot NERDIO_TOKEN_URL posted their client secret to a URL
// naming no tenant of theirs. A token URL that still carries the placeholder,
// does not parse to a plain http(s) host, or names a user name or password is
// no token URL: the mint, the refresh grant and `auth login` refuse with
// ErrNoNerdioTokenURL and the host guard pins no token origin.
//
// nerdio handfix oauth-scope-required: the press defaulted the scope to
// api://<client_id>/.default. That is the caller's own application, not the
// NMM API, and the api:// prefix is the documented AADSTS500011 trap. The
// scope is the NMM application's bare '<app-id>/.default' and only the
// operator knows it, so NERDIO_OAUTH_SCOPE is required: without it nothing
// is posted and the error names the variable.
//
// Exported because internal/cli's `auth login` shares the exact same
// resolution (cli already depends on client; one copy keeps the two paths
// from diverging).

package client

import (
	"errors"
	"net/url"
	"strings"

	"nerdio-pp-cli/internal/cliutil"
	"nerdio-pp-cli/internal/config"
)

// ErrNoNerdioTokenURL marks a token request refused because the configuration
// names no host the client ID and secret may be posted to.
var ErrNoNerdioTokenURL = errors.New("no token URL to send the client ID and secret to: set NERDIO_TOKEN_URL to your Entra tenant's endpoint, https://login.microsoftonline.com/<TENANT_ID>/oauth2/v2.0/token (no user name or password in it)")

// ErrNoNerdioScope marks a token request refused because NERDIO_OAUTH_SCOPE
// is unset.
var ErrNoNerdioScope = errors.New("NERDIO_OAUTH_SCOPE is not set: set it to the NMM application's scope, '<app-id>/.default' with NO api:// prefix (Settings -> Integrations -> REST API in your NMM portal)")

// ErrHostPinRefused marks a request a transport guard refused before sending
// it (another host, the placeholder base URL, --dry-run). Such a refusal is
// final: the retry loop returns it at once (nerdio handfix
// guard-refusal-is-final) instead of backing off three times.
var ErrHostPinRefused = errors.New("request refused by the host pin")

// placeholderTenant is the tenant segment the shipped token URL template
// carries; a token URL still holding it names no real tenant.
const placeholderTenant = "YOUR_TENANT_ID"

// ResolveNerdioTokenURL is the OAuth2 token endpoint the operator configured,
// or "" when none may be used (unset, still the shipped placeholder, not a
// plain http(s) URL with an ASCII host and no userinfo, or a credential route
// Load refused).
func ResolveNerdioTokenURL(cfg *config.Config) string {
	if cfg == nil || cfg.CredentialRouteBlocked() {
		return ""
	}
	raw := strings.TrimSpace(cfg.TokenURL)
	if raw == "" || strings.Contains(strings.ToUpper(raw), placeholderTenant) {
		return ""
	}
	u, ok := tokenSafeURL(raw)
	if !ok {
		return ""
	}
	return u.String()
}

// ResolveNerdioScope is the scope the token request asks for: the operator's
// NERDIO_OAUTH_SCOPE, or "" when it is unset (the docs and the error carry the
// no-api:// guidance; the value is the operator's to choose).
func ResolveNerdioScope() string {
	return strings.TrimSpace(cliutil.EnvOverride("NERDIO_OAUTH_SCOPE"))
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
		if c := u.Host[i]; c >= 0x80 || c <= ' ' || c == '@' || c == '\\' || c == '%' { // '%': an IPv6 zone (Codex review r1)
			return nil, false
		}
	}
	u.Scheme = strings.ToLower(u.Scheme)
	return u, true
}
