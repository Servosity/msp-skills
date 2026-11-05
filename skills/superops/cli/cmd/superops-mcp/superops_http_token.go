// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): the HTTP transport's bearer token must be one
// no caller can know without being given it (superops-mcp-http-token-unguessable).
// Pinned by this connector's handfixes.json.

package main

import (
	"fmt"
	"strings"
)

// minHTTPCallerTokenLen is the shortest SUPEROPS_MCP_HTTP_TOKEN HTTP mode
// accepts; `openssl rand -hex 32`, as mcp-install.md shows, gives 64.
const minHTTPCallerTokenLen = 32

// httpCallerTokenChars are the bearer-token characters (RFC 6750 b64token).
const httpCallerTokenChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-._~+/="

// requireUnguessableHTTPCallerToken refuses a token anyone could know
// without being given it: shorter than minHTTPCallerTokenLen, or holding any
// character outside httpCallerTokenChars. That refuses an unresolved
// manifest placeholder (${user_config.superops_mcp_http_token}), unexpanded
// shell text ($(openssl rand -hex 32), $VAR) and short shared words, so HTTP
// mode never serves behind a public or guessable token. The token is never
// shown.
func requireUnguessableHTTPCallerToken(token string) error {
	if len(token) < minHTTPCallerTokenLen || strings.TrimLeft(token, httpCallerTokenChars) != "" {
		// The refusal names neither the token nor its variable: the kit holds the value, and a placeholder repeats the name.
		return fmt.Errorf("the --transport http token is not a usable bearer token: use at least %d of the characters A-Z a-z 0-9 - . _ ~ + / = (generate one with openssl rand -hex 32); an unexpanded placeholder or shell text is refused", minHTTPCallerTokenLen)
	}
	return nil
}
