// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

// crowdstrike handfix stored-credential-bound-to-its-origin (Codex r1 of the
// 4.34.2 review, classes credential-sent-to-another-host and
// token-sent-for-another-tenant): every stored credential is presented only
// to the Falcon cloud it was recorded for, and only the token binding sets
// Authorization.
//
//   - The credential store records the base URL each credential was saved
//     for (Credentials.BaseURL). A store recorded for another cloud than the
//     one this run sends to is not applied at all (token, auth header,
//     refresh token, client pair): it is refused and reported, as the
//     generated explicit-config path already refused the global store. Load
//     applied it whenever it named any server, so `auth set-token` against
//     US-1 followed by CROWDSTRIKE_BASE_URL=EU-1 sent the US-1 token to EU-1,
//     and a config.toml naming EU-1 over a store recorded for US-1 sent the
//     US-1 token there too (the token binding compared the configuration with
//     itself). A store with no recorded base URL belongs to the base URL the
//     configuration names (what the generated save would have recorded).
//   - A trusted config file's own credentials belong to the base URL that file
//     names; an environment base URL naming another cloud refuses them.
//   - No configured header carries a credential: [headers] is not a
//     credential store, and a configured Authorization was set after the
//     client's bearer, so it overrode the token the binding chose (the parent
//     pair's minted token, or a child CID's member_cid token in fleet sync,
//     which then stored the parent tenant's hosts under the child CID). Each
//     credential header is held, dropped and reported; every other header is
//     kept.
//
// "Recorded for" compares normalized origins (cliutil.CredentialOriginsMatch:
// scheme, host, port and path, case and default ports folded). A base URL
// without a scheme is read as https, as the token URL resolver reads it.
// Anything that does not name an origin matches nothing: fail closed.

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"crowdstrike-pp-cli/internal/cliutil"
)

// CredentialHeaderName reports a header name that carries a credential
// (Authorization, Proxy-Authorization, Cookie, X-Api-Key, any credential
// name in any spelling): the one rule the stores and the saved config use for names.
func CredentialHeaderName(name string) bool {
	return cliutil.SecretFieldName(name)
}

// WithoutCredentialHeaders returns a copy of h without any credential header.
func WithoutCredentialHeaders(h map[string]string) map[string]string {
	if h == nil {
		return nil
	}
	out := make(map[string]string, len(h))
	for k, v := range h {
		if !CredentialHeaderName(k) {
			out[k] = v
		}
	}
	return out
}

func credentialOrigin(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw != "" && !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	return raw
}

// shownOrigin is the scheme and host of raw for a refusal message: never its
// userinfo, path or query (any of which can carry a credential).
func shownOrigin(raw string) string {
	u, err := url.Parse(credentialOrigin(raw))
	if err != nil || u.Host == "" {
		return "a URL that names no server"
	}
	return u.Scheme + "://" + u.Host
}

// recordedFor reports whether a credential recorded for issued may be sent to
// target.
func recordedFor(issued, target string) bool {
	return cliutil.CredentialOriginsMatch(credentialOrigin(issued), credentialOrigin(target))
}

// bindFileCredentials runs as the credential store is consulted, before any
// environment credential applies: the config file's credential headers are
// dropped, and its credentials are dropped when this run sends to another
// cloud than the file names. Both are refusals the operator sees.
func (c *Config) bindFileCredentials() {
	if c == nil {
		return
	}
	var dropped []string
	for k, v := range c.Headers {
		if CredentialHeaderName(k) {
			cliutil.HoldValues("header:"+strings.ToLower(k), v)
			dropped = append(dropped, k)
		}
	}
	if len(dropped) > 0 {
		sort.Strings(dropped)
		c.Headers = WithoutCredentialHeaders(c.Headers)
		c.addCredentialRefusal(cliutil.CredentialRefusal{
			Source:             "config [headers]",
			Path:               c.Path,
			Err:                fmt.Errorf("header %s carries a credential and was not sent (only the CrowdStrike token binding sets Authorization: use auth login, auth set-token or FALCON_CLIENT_ID/FALCON_CLIENT_SECRET)", strings.Join(dropped, ", ")),
			CredentialsPresent: true,
		})
	}
	if !c.hasCredentialFields() {
		return
	}
	target := c.credentialTargetURL()
	if recordedFor(c.BaseURL, target) {
		return
	}
	c.addCredentialRefusal(cliutil.CredentialRefusal{
		Source:             "config file",
		Path:               c.Path,
		Err:                fmt.Errorf("its credentials are for %s and were not sent to %s (log in again for that server)", shownOrigin(c.BaseURL), shownOrigin(target)),
		CredentialsPresent: true,
	})
	c.clearCredentialFields()
}

// storedCredentialsBound reports whether creds (a credential store) were
// recorded for the server this run sends to; when not, it records the
// refusal and the store is not applied.
func (c *Config) storedCredentialsBound(creds *cliutil.Credentials) bool {
	if c == nil || creds == nil {
		return false
	}
	issued := strings.TrimSpace(creds.BaseURL)
	if issued == "" {
		issued = c.BaseURL
	}
	target := c.credentialTargetURL()
	if recordedFor(issued, target) {
		return true
	}
	c.addCredentialRefusal(cliutil.CredentialRefusal{
		Source:             "credentials file",
		Path:               "the credential store",
		Err:                fmt.Errorf("the stored credentials are for %s and were not sent to %s (run auth login or auth set-token for that server)", shownOrigin(issued), shownOrigin(target)),
		CredentialsPresent: true,
	})
	return false
}
