// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"bytes"
	"encoding/json"
	"strings"

	"acronis-pp-cli/internal/cliutil"
)

// holdRequestCredentials records credentials that arrive in a request rather
// than in the config (a client secret passed to idp request-token, a token
// passed to idp revoke-token), so every error backstop withholds a server
// answer that repeats them, in the CLI and the MCP server alike (hand-fix
// handfixes.json request-credentials-held).
//
// On the identity provider's endpoints (the credential tier) every string and
// number the request carries is held, whatever its name. Elsewhere a query or
// body value is held when its name reads as a credential. A JSON body is read
// with its numbers kept as written, so a numeric credential is held in the
// digits the server can repeat.
func holdRequestCredentials(path string, params map[string]string, body any) {
	all := strings.Contains(strings.ToLower(path), "/idp/")
	for k, v := range params {
		holdRequestValue(all, k, v)
	}
	switch b := body.(type) {
	case nil:
	case formRequestBody:
		for k, vs := range b.Fields {
			for _, v := range vs {
				holdRequestValue(all, k, v)
			}
		}
	default:
		raw, ok := body.(json.RawMessage)
		if !ok {
			if bs, isBytes := body.([]byte); isBytes {
				raw = bs
			} else if m, err := json.Marshal(body); err == nil {
				raw = m
			}
		}
		var v any
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if json.Valid(raw) && dec.Decode(&v) == nil {
			holdRequestTree(all, "", v)
		} else if all {
			cliutil.FunnelHold(string(raw))
		}
	}
}

func holdRequestTree(all bool, key string, v any) {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			holdRequestTree(all || requestKeyIsCredential(k), k, child)
		}
	case []any:
		for _, child := range t {
			holdRequestTree(all, key, child)
		}
	case string:
		holdRequestValue(all, key, t)
	case json.Number:
		holdRequestValue(all, key, t.String())
	}
}

func holdRequestValue(all bool, key, v string) {
	if all || requestKeyIsCredential(key) {
		cliutil.FunnelHold(v)
	}
}

// requestKeyIsCredential reports whether a request field name reads as a
// credential: every name the shared predicate cliutil.IsSecretFieldName
// matches (the one the local store and response cache apply, hand-fix
// handfixes.json credential-fields-never-persisted), plus the request-only
// signals kept from before it: secret, password, passphrase, token,
// assertion, credential, an API, private or signing key anywhere in the name,
// or an OAuth authorization code.
func requestKeyIsCredential(key string) bool {
	if cliutil.IsSecretFieldName(key) {
		return true
	}
	k := strings.NewReplacer("_", "", "-", "", ".", "").Replace(strings.ToLower(key))
	if k == "code" || k == "authorizationcode" {
		return true
	}
	for _, s := range []string{"secret", "password", "passwd", "passphrase", "token", "assertion", "credential", "apikey", "privatekey", "signingkey"} {
		if strings.Contains(k, s) {
			return true
		}
	}
	return false
}
