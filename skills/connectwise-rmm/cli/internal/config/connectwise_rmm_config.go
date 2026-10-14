// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: ConnectWise Asio region and token URL resolution.

package config

import (
	"strings"

	"connectwise-rmm-pp-cli/internal/cliutil"
)

// AsioNABaseURL is the North America partner API host and the spec default.
const AsioNABaseURL = "https://openapi.service.itsupport247.net"

// asioRegionByClientIDPrefix maps the documented client ID prefixes to their
// regional hosts (ConnectWise University, "Agent Last Seen API": 0e30 NA,
// 0e31 EU, 0e32 AU).
var asioRegionByClientIDPrefix = map[string]string{
	"0e30": AsioNABaseURL,
	"0e31": "https://openapi.service.euplatform.connectwise.com",
	"0e32": "https://openapi.service.auplatform.connectwise.com",
}

// AsioRegionBaseURL returns the regional host for a client ID, or "" when the
// prefix is not one ConnectWise documents.
func AsioRegionBaseURL(clientID string) string {
	id := strings.ToLower(strings.TrimSpace(clientID))
	if len(id) < 4 {
		return ""
	}
	return asioRegionByClientIDPrefix[id[:4]]
}

// AsioClientCredentials is the one credential resolver for minting, scope
// probing and region inference: auth-login credentials first, then the
// rmm_client_id/rmm_client_secret config fields (or their env vars).
func (c *Config) AsioClientCredentials() (id, secret string) {
	if c == nil {
		return "", ""
	}
	id, secret = c.ClientID, c.ClientSecret
	if id == "" {
		id = c.ConnectwiseRmmClientId
	}
	if secret == "" {
		secret = c.ConnectwiseRmmClientSecret
	}
	return strings.TrimSpace(id), strings.TrimSpace(secret)
}

// regionBase returns the base URL to use for a client ID: the configured base
// URL, or the client's regional host when the configured one is still the
// North America default and CONNECTWISE_RMM_BASE_URL is unset. To pin North
// America for an EU/AU-prefixed ID, set CONNECTWISE_RMM_BASE_URL.
func (c *Config) regionBase(clientID string) string {
	base := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if base == "" {
		base = AsioNABaseURL
	}
	if c.explicitBaseURL(base) {
		return base
	}
	if region := AsioRegionBaseURL(clientID); region != "" {
		return region
	}
	return base
}

// explicitBaseURL reports a base URL the user chose (env or config file), as
// opposed to the NA default or a regional host this file inferred earlier.
func (c *Config) explicitBaseURL(base string) bool {
	if cliutil.EnvOverride("CONNECTWISE_RMM_BASE_URL") != "" {
		return true
	}
	if c.fileConfig != nil {
		file := strings.TrimRight(strings.TrimSpace(c.fileConfig.BaseURL), "/")
		return file != "" && file != AsioNABaseURL
	}
	return base != AsioNABaseURL
}

// applyAsioRegion moves an EU or AU tenant off the North America default.
func (c *Config) applyAsioRegion() {
	id, _ := c.AsioClientCredentials()
	if base := c.regionBase(id); base != strings.TrimRight(strings.TrimSpace(c.BaseURL), "/") {
		c.BaseURL = base
	}
}

// AsioTokenURL returns the token endpoint: an explicit token URL override,
// otherwise <base_url>/v1/token so EU and AU tenants mint on their own host.
func (c *Config) AsioTokenURL() string {
	id, _ := c.AsioClientCredentials()
	return c.AsioTokenURLForClient(id)
}

// AsioTokenURLForClient is AsioTokenURL for a client ID that may not be in the
// config yet (auth login --client-id).
func (c *Config) AsioTokenURLForClient(clientID string) string {
	if c == nil {
		return AsioNABaseURL + "/v1/token"
	}
	if strings.TrimSpace(c.TokenURL) != "" {
		return c.TokenURL
	}
	return c.regionBase(clientID) + "/v1/token"
}

// Hand-wired (handfixes.json: store-scope-pinned-to-client-id): the generated
// store key hashes every credential field that happens to be loaded, and the
// first token mint caches client_id/client_secret beside the env copies, so
// the first sync wrote one database file and every later command read another
// (and dropping the env vars moved to a third). The API client ID names the
// account; pin the local store to it and the API host.
func init() { StoreScopeIdentity = asioStoreScopeIdentity }

func asioStoreScopeIdentity(c *Config) string {
	id, _ := c.AsioClientCredentials()
	if id == "" {
		return ""
	}
	return "asio_client_id=" + strings.ToLower(id) + "\nbase_url=" + normalizeStoreScopeBaseURL(c.BaseURL)
}
