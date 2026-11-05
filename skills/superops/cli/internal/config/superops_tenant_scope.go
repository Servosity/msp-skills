// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): the SuperOps tenant is part of every local
// scope (superops-tenant-scope).

package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"superops-pp-cli/internal/cliutil"
)

// SuperOpsTenantScopeVar names the tenant in the sync checkpoint scope key.
const SuperOpsTenantScopeVar = "superops_tenant"

// SuperOps picks the tenant with the CustomerSubDomain header and the data
// center with the base URL; neither is in the generated store scope (it hashes
// only the credential) or the sync checkpoint scope (it reads only base-URL
// template placeholders, and SuperOps has none). Without them, switching
// SUPEROPS_SUBDOMAIN or SUPEROPS_REGION kept the same local store: tenant B
// resumed tenant A's page cursor and read A's rows. A store and a checkpoint
// now belong to one credential at one tenant on one host. A store from before
// this key proves no tenant, so it is never adopted (storeAdoptsPreTenantStores
// in internal/cli): an upgraded install re-syncs once.
func init() {
	StoreScopeIdentity = superOpsStoreScopeIdentity
}

func superOpsStoreScopeIdentity(c *Config) string {
	credential := c.StoreScopeLegacyCredential()
	if credential == "" {
		return ""
	}
	return credential + "\n" + c.SuperOpsTenant()
}

// SuperOpsTenant is the tenant identity: the normalized subdomain (sent as
// CustomerSubDomain) and the host and path requests go to. It is the one
// source of the identity the store records (superops_store_tenant), the sync
// checkpoint keys (sync_state.scope_key) and the default store path hashes,
// and the MCP sql tool can read the first two, so it never holds a credential
// (superops-tenant-identity-no-credential).
func (c *Config) SuperOpsTenant() string {
	if c == nil {
		return ""
	}
	return superOpsTenantIdentity(c.Subdomain, c.BaseURL)
}

func superOpsTenantIdentity(subdomain, baseURL string) string {
	return "subdomain=" + strings.ToLower(strings.TrimSpace(subdomain)) + " base_url=" + superOpsTenantBaseURL(baseURL)
}

// superOpsTenantInvalid stands in for a base URL that does not parse or names
// no scheme and host, and for a recorded identity that does not parse. Every
// real identity holds "://", so it never equals one, and it holds none of the
// raw text.
const superOpsTenantInvalid = "invalid"

// superOpsTenantBaseURL keeps scheme, host, port and path only. The userinfo
// (http://user:password@host), the query and the fragment can carry a
// credential and never name the tenant, so they are dropped. The result is
// lowercased whole, path included, exactly as the earlier identity was, so an
// ordinary base URL (https://api.superops.ai, https://euapi.superops.ai)
// gives the same identity as before and an installed store keeps its tenant.
// The parse rule is cliutil.URLWithoutUserinfo, the one every display of a
// base URL uses too (cliutil.DisplayURL, superops-base-url-never-printed).
func superOpsTenantBaseURL(raw string) string {
	base, ok := cliutil.URLWithoutUserinfo(raw)
	if !ok {
		return superOpsTenantInvalid
	}
	return strings.ToLower(base)
}

// holdBaseURLUserinfo records the userinfo of the base URL this run uses with
// the kit (superops-base-url-never-printed), which then withholds a text that
// repeats it in any encoding, including on the paths that never build a client.
func (c *Config) holdBaseURLUserinfo() {
	if c != nil {
		cliutil.HoldURLUserinfo(c.BaseURL)
	}
}

// errBaseURLRefused refuses a base URL whose text after the userinfo could
// hold a credential: an '@' outside the userinfo (cliutil.URLUserinfoDisplaced:
// part of a username or password was read as host or path) or a query or
// fragment (cliutil.URLHasQueryOrFragment). The URL is never sent to, and
// never in this text, so no request error or preview can repeat it.
var errBaseURLRefused = errors.New("base URL refused (it is not shown): use scheme://host[:port][/path] with no query or fragment; a username or password may precede the host only with any '/', '?', '#' or '@' inside it percent-encoded")

// refuseUnshowableBaseURL is the config-load check for the base URL this run
// uses (superops-base-url-never-printed): only what may be shown, or the
// userinfo the output backstops remove, is ever used.
func (c *Config) refuseUnshowableBaseURL() error {
	if c != nil && (cliutil.URLUserinfoDisplaced(c.BaseURL) || cliutil.URLHasQueryOrFragment(c.BaseURL)) {
		return errBaseURLRefused
	}
	return nil
}

// errCredentialTooShort refuses a credential shorter than
// cliutil.MinDetectableSecretLen. Such a value is still held and withheld in
// every encoding by the kit (credkit-wiring), but its
// encodings match so much ordinary text that most output would be withheld,
// and no SuperOps credential is that short. The value is never in this text.
var errCredentialTooShort = fmt.Errorf("credential refused (it is not shown): the API token, auth header token, or base URL password is shorter than %d characters; nothing was sent", cliutil.MinDetectableSecretLen)

// refuseUndetectableCredentials is the config-load check that every
// credential this run would send is long enough for the output checks to
// catch in any encoding (superops-credential-floor): the API token, the
// access, refresh and client-secret fields, an auth header (and the token
// after its scheme), and the base URL's userinfo secret.
func (c *Config) refuseUndetectableCredentials() error {
	if c == nil {
		return nil
	}
	if cliutil.URLUserinfoTooShort(c.BaseURL) {
		return errCredentialTooShort
	}
	for _, v := range []string{c.SuperopsApiToken, c.AccessToken, c.RefreshToken, c.ClientSecret, c.AuthHeaderVal} {
		v = strings.TrimSpace(v)
		if _, token, ok := strings.Cut(v, " "); ok {
			v = strings.TrimSpace(token)
		}
		if v != "" && len(v) < cliutil.MinDetectableSecretLen {
			return errCredentialTooShort
		}
	}
	return nil
}

// SuperOpsTenantFromRecord is the identity a tenant record written by an
// earlier build stands for: the record re-read through the same rule as
// SuperOpsTenant, so a record that kept a URL password, query or fragment
// becomes the identity without it. A record that is not in the
// "subdomain=<s> base_url=<url>" form gives an identity that matches no
// tenant. The store uses it to scrub such a record, or to refuse it, without
// ever showing it.
func SuperOpsTenantFromRecord(record string) string {
	rest, ok := strings.CutPrefix(record, "subdomain=")
	if !ok {
		return superOpsTenantInvalid
	}
	subdomain, baseURL, ok := strings.Cut(rest, " base_url=")
	if !ok {
		return superOpsTenantInvalid
	}
	return superOpsTenantIdentity(subdomain, baseURL)
}

// errTenantHeaderConflict refuses a config whose tenant routing has two
// sources that disagree. The values are not shown.
var errTenantHeaderConflict = errors.New("tenant refused: the subdomain setting and a [headers] CustomerSubDomain value disagree, or two spellings of that header do; keep one (SUPEROPS_SUBDOMAIN replaces both)")

// unifyTenantHeader makes Subdomain the one source of the CustomerSubDomain
// header every request sends, so the tenant identity (SuperOpsTenant) always
// names the tenant the requests reach (superops-tenant-header-one-source).
// Every [headers] spelling of the header (HTTP header names ignore case, and
// net/http sends only one) is removed and folded into Subdomain; Load then
// sets the header from Subdomain (applyTenantRouting). A value that differs
// from Subdomain or from another spelling is refused. SUPEROPS_SUBDOMAIN
// replaces them all, as an env value replaces every config value.
func (c *Config) unifyTenantHeader() error {
	if c == nil {
		return nil
	}
	var values []string
	for name, v := range c.Headers {
		if strings.EqualFold(strings.TrimSpace(name), "CustomerSubDomain") {
			delete(c.Headers, name)
			if v = strings.TrimSpace(v); v != "" {
				values = append(values, v)
			}
		}
	}
	if strings.TrimSpace(os.Getenv("SUPEROPS_SUBDOMAIN")) != "" {
		return nil
	}
	sub := strings.TrimSpace(c.Subdomain)
	for _, v := range values {
		if sub == "" {
			sub = v
		} else if v != sub {
			return errTenantHeaderConflict
		}
	}
	c.Subdomain = sub
	return nil
}
