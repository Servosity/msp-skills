// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored (not generated). The client half of the credential tier
// (internal/cliutil/tl_credential_tier.go). Three places are guarded here,
// each keyed by a path the transport would actually fetch, so every caller is
// covered, generated or novel, CLI or MCP:
//
//   - the response cache: a credential-tier body is never written to
//     <cacheDir>/resources/..., never served from it, and a file an earlier
//     binary wrote at that key is removed on the next attempt;
//   - the transport: inside the MCP server, or a CLI process it spawned, the
//     request is refused before auth is minted or anything is dialed;
//   - redirects: a hop that lands on a credential read is refused unless the
//     request already asked for that same credential in a process allowed to
//     read it. Otherwise a response to an ordinary path (cached under that
//     ordinary path, returned to an MCP client) could carry the key.
//
// Hooked from the generated client.go at readCacheWithHeaders,
// writeCacheWithHeaders, doInternal, redirectGuard and
// terminalTransportRefusal (handfixes.json:
// credential-tier-never-cached-never-served-to-mcp).

package client

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"threatlocker-pp-cli/internal/cliutil"
)

// credentialTierCacheRefused reports whether path is a credential-tier read
// and, when it is, deletes any cache entry already on disk at its key.
func (c *Client) credentialTierCacheRefused(path string, params map[string]string, headers map[string]string) bool {
	if !cliutil.IsCredentialTierPath(path) {
		return false
	}
	if c != nil && c.cacheDir != "" {
		cacheFile := filepath.Join(c.cacheResourceDir(path), c.cacheKeyFor(http.MethodGet, path, params, headers, nil)+".json")
		_ = os.Remove(cacheFile)
		_ = os.Remove(cacheFile + ".platform.json")
	}
	return true
}

// credentialTierTransportError refuses a credential-tier path when this
// process is, or was spawned by, the MCP server.
func credentialTierTransportError(path string) error {
	ep, ok := cliutil.CredentialTierByPath(path)
	if !ok || cliutil.CredentialTierServedHere() {
		return nil
	}
	return errors.New(cliutil.CredentialTierRefusal(ep, "MCP"))
}

// errCredentialTierRedirect marks a refused redirect as terminal, so the
// transport does not retry it (terminalTransportRefusal in client.go).
var errCredentialTierRedirect = errors.New("refusing redirect onto a credential read")

// credentialTierRedirectError refuses a redirect hop whose destination is a
// credential read, unless the original request was that same read and this
// process may serve it (an operator in a terminal whose own request the API
// moved).
func credentialTierRedirectError(req *http.Request, via []*http.Request) error {
	if req == nil || req.URL == nil {
		return nil
	}
	target, ok := cliutil.CredentialTierByPath(req.URL.EscapedPath())
	if !ok {
		return nil
	}
	if !cliutil.CredentialTierServedHere() {
		return fmt.Errorf("%w: %s", errCredentialTierRedirect, cliutil.CredentialTierRefusal(target, "MCP"))
	}
	if len(via) > 0 && via[0] != nil && via[0].URL != nil {
		if origin, ok := cliutil.CredentialTierByPath(via[0].URL.EscapedPath()); ok && origin.ID == target.ID {
			return nil
		}
	}
	return fmt.Errorf("%w %s: it is fetched only when requested directly. Run `%s` in a terminal to read it", errCredentialTierRedirect, target.ID, target.Command)
}
