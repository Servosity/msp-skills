// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored (handfixes.json: error-body-redacts-secrets,
// transport-error-withholds-echoed-secret, dry-run-redacts-secret-body),
// ported from skills/connectsecure/cli/internal/client/connectsecure_redact.go.

package client

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"

	"watchguard-pp-cli/internal/cliutil"
)

// withheldBody replaces an error body that echoed a request secret.
const withheldBody = "[response body withheld: it contains a value from a sensitive field of your request, possibly by coincidence; preview the request with --dry-run]"

// isSecretBodyKey is cliutil.IsSecretFieldKey: one list for the store, the
// response cache, tail, row identity, --dry-run and error-body withholding.
func isSecretBodyKey(k string) bool { return cliutil.IsSecretFieldKey(k) }

// isFlagValue keeps booleans and nulls readable (pskEnabled: true): they
// cannot carry a secret.
func isFlagValue(v any) bool {
	switch v.(type) {
	case bool, nil:
		return true
	}
	return false
}

// redactBody returns body with every secret-named field's value replaced. A
// body that is not JSON is returned unchanged.
func redactBody(body []byte) []byte {
	var v any
	if json.Unmarshal(body, &v) != nil {
		return body
	}
	// No HTML escaping: an error message keeps its & < > readable.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if enc.Encode(redactSecretValues(v)) != nil {
		return body
	}
	return bytes.TrimRight(buf.Bytes(), "\n")
}

func redactSecretValues(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if (isSecretBodyKey(k) || cliutil.IsLicenseKeyField(t, k)) && !isFlagValue(val) {
				t[k] = "[redacted]"
			} else {
				t[k] = redactSecretValues(val)
			}
		}
	case []any:
		for i := range t {
			t[i] = redactSecretValues(t[i])
		}
	}
	return v
}

// benignParamKeys name the account or target next to a secret rather than a
// secret; an error naming them stays readable.
var benignParamKeys = map[string]bool{"username": true, "user_name": true, "login": true, "domain": true, "host": true, "hostname": true, "address": true, "ip": true, "name": true, "type": true, "port": true, "version": true, "protocol": true}

// requestSecrets returns every secret value a request carried: any value
// whose own key is secret-named; strings of 6+ characters elsewhere under a
// secret-named object; and secret-named or authenticating headers (the
// WatchGuard-API-Key, an audience-scoped Authorization override).
func requestSecrets(body []byte, headers, params map[string]string) []string {
	var out []string
	add := func(s string) {
		if s != "" {
			out = append(out, s)
		}
	}
	var collect func(v any, key string, own, under bool)
	collect = func(v any, key string, own, under bool) {
		switch t := v.(type) {
		case map[string]any:
			for k, val := range t {
				collect(val, k, isSecretBodyKey(k) || cliutil.IsLicenseKeyField(t, k), under || own)
			}
		case []any:
			for _, val := range t {
				collect(val, key, own, under)
			}
		case string:
			if own || (under && len(t) >= 6 && !benignParamKeys[strings.ToLower(key)]) {
				add(t)
			}
		case float64:
			if own {
				add(strconv.FormatFloat(t, 'f', -1, 64))
			}
		}
	}
	var v any
	if json.Unmarshal(body, &v) == nil {
		collect(v, "", false, false)
	}
	out = append(out, querySecrets(params)...)
	for k, val := range headers {
		if isSecretHeader(k) {
			add(val)
			if parts := authSecrets(val); len(parts) > 1 { // "Bearer <tok>" -> "<tok>"
				add(parts[1])
			}
		}
	}
	return out
}

// querySecrets is every secret a request's query string carried: a
// secret-named parameter, or a software license key (allocations deallocate
// sends serialOrLicense as a query parameter). Hand-fix (handfixes.json:
// license-keys-fingerprinted).
func querySecrets(params map[string]string) []string {
	q := make(map[string]any, len(params))
	for k, v := range params {
		q[k] = v
	}
	var out []string
	for k, v := range params {
		if v != "" && (isSecretBodyKey(k) || cliutil.IsLicenseKeyField(q, k)) {
			out = append(out, v)
		}
	}
	return out
}

// urlQuery is the query a request URL carries: the CLI's params and any
// query the MCP server built into the path.
func urlQuery(u *url.URL) map[string]string {
	out := map[string]string{}
	for k, v := range u.Query() {
		if len(v) > 0 {
			out[k] = v[0]
		}
	}
	return out
}

// withholdQuerySecrets drops the query from a URL or path shown in an error
// or preview when it carries a query secret (an MCP deallocate builds
// serialOrLicense into the path). Hand-fix (handfixes.json:
// license-keys-fingerprinted).
func withholdQuerySecrets(raw string) string {
	base, rq, ok := strings.Cut(raw, "?")
	if !ok {
		return raw
	}
	if u, err := url.Parse("http://x/?" + rq); err != nil || len(querySecrets(urlQuery(u))) > 0 {
		return base + "?[query withheld]"
	}
	return raw
}

// isSecretHeader: a secret-named header, or one that authenticates
// (Authorization, Cookie, WatchGuard-API-Key) though its name says no "token".
func isSecretHeader(k string) bool {
	l := strings.ToLower(k)
	return isSecretBodyKey(k) || strings.Contains(l, "auth") || strings.Contains(l, "cookie") || strings.Contains(l, "key")
}

// sentHeaders is every header the request sent that can carry a credential:
// the WatchGuard-API-Key, the configured Headers (config file), then
// per-request overrides, which win as they do on the wire.
func (c *Client) sentHeaders(overrides map[string]string) map[string]string {
	out := map[string]string{}
	if c != nil && c.Config != nil {
		if c.Config.WatchguardApiKey != "" {
			out["WatchGuard-API-Key"] = c.Config.WatchguardApiKey
		}
		for k, v := range c.Config.Headers {
			out[k] = v
		}
	}
	for k, v := range overrides {
		out[k] = v
	}
	return out
}

// authSecrets returns the configured Authorization value and its credential
// part ("Bearer <jwt>" -> "<jwt>") so an echo of the CLI's own token is
// withheld like a request secret.
func authSecrets(authHeader string) []string {
	// Hand-fix (handfixes.json: scoped-bearer-in-secret-set): a customer's
	// audience token is substituted at the transport, so the configured
	// credential's guards never saw it.
	out := scopedSecrets()
	if authHeader == "" {
		return out
	}
	out = append(out, authHeader)
	if i := strings.LastIndexByte(authHeader, ' '); i >= 0 && i+1 < len(authHeader) {
		out = append(out, authHeader[i+1:])
	}
	return out
}

// transportError withholds a transport error that echoes a secret the request
// carried: a refused redirect's error names its Location, which a server can
// fill with the submitted pre-shared key. The request's own URL and host are
// built by the CLI (a DNS error names the host), so they are dropped before
// matching; a server-supplied Location keeps its query.
func transportError(err error, reqURL string, reqBody []byte, headers, params map[string]string, authHeader string) error {
	shown := transportErrorFor(err, reqURL, append(requestSecrets(reqBody, headers, params), authSecrets(authHeader)...))
	// The CLI's own URL is cut before matching, but it still carries the
	// query, so a query secret is withheld from the shown URL. Hand-fix
	// (handfixes.json: license-keys-fingerprinted).
	if shown == err && len(querySecrets(params)) > 0 {
		if base, _, ok := strings.Cut(reqURL, "?"); ok {
			return errors.New(strings.ReplaceAll(err.Error(), reqURL, base+"?[query withheld]"))
		}
	}
	return shown
}

func transportErrorFor(err error, reqURL string, secrets []string) error {
	if err == nil {
		return nil
	}
	full := err.Error()
	text := strings.ReplaceAll(full, reqURL, "")
	host, hostname := "", ""
	if u, perr := url.Parse(reqURL); perr == nil && u.Host != "" {
		host, hostname = u.Host, u.Hostname()
		text = strings.ReplaceAll(strings.ReplaceAll(text, host, ""), hostname, "")
	}
	// Stripping the host would also cut it out of a secret that contains it,
	// so such a secret is matched against the unstripped text.
	var hostSecrets []string
	for _, sec := range secrets {
		if hostname != "" && (strings.Contains(sec, host) || strings.Contains(sec, hostname)) {
			hostSecrets = append(hostSecrets, sec)
		}
	}
	if cliutil.EchoesSecret(text, secrets) || (len(hostSecrets) > 0 && cliutil.EchoesSecret(full, hostSecrets)) {
		return errors.New("[transport error withheld: it echoed a secret the request carried, possibly by coincidence]")
	}
	return err
}

// errorBody is the text an API error may show: withheld when the response
// carries a request secret in any form (raw or as displayed after
// re-encoding, checked before truncation), else secret-named fields masked.
func errorBody(respBody, reqBody []byte, headers, params map[string]string, authHeader string) string {
	secrets := append(requestSecrets(reqBody, headers, params), authSecrets(authHeader)...)
	shown := truncateBody(redactBody(respBody))
	if cliutil.EchoesSecret(string(respBody), secrets) || cliutil.EchoesSecret(shown, secrets) {
		return withheldBody
	}
	return shown
}

// tokenRequestSecrets is everything a token request carries (hand-fix,
// handfixes.json token-error-withholds-credentials): the Access ID and API
// password, the "id:password" pair HTTP Basic encodes, its base64 and the
// whole Basic header value, the WatchGuard-API-Key and any extra values (a
// refresh token). A proxy or a wrong WATCHGUARD_TOKEN_URL that echoes the
// request must not put them in an error.
func tokenRequestSecrets(clientID, clientSecret, apiKey string, extra ...string) []string {
	var out []string
	add := func(s string) {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	add(clientID)
	add(clientSecret)
	add(apiKey)
	for _, s := range extra {
		add(s)
	}
	if clientID != "" || clientSecret != "" {
		pair := clientID + ":" + clientSecret
		b64 := base64.StdEncoding.EncodeToString([]byte(pair))
		add(pair)
		add(b64)
		add("Basic " + b64)
	}
	return out
}

// SafeTokenEndpointBody is the text a token-endpoint error may show: withheld
// when the response echoes anything the request carried, in any encoding the
// echo check decodes; otherwise sanitized with secret-named fields masked.
func SafeTokenEndpointBody(body []byte, clientID, clientSecret, apiKey string, extra ...string) string {
	secrets := tokenRequestSecrets(clientID, clientSecret, apiKey, extra...)
	shown := cliutil.SanitizeErrorBody(truncateBody(redactBody(body)))
	if cliutil.EchoesSecret(string(body), secrets) || cliutil.EchoesSecret(shown, secrets) {
		return withheldBody
	}
	return shown
}

// SafeTokenEndpointError withholds a token-request transport error that
// echoes a credential (a refused redirect names its Location).
func SafeTokenEndpointError(err error, tokenURL, clientID, clientSecret, apiKey string, extra ...string) error {
	return transportErrorFor(err, tokenURL, tokenRequestSecrets(clientID, clientSecret, apiKey, extra...))
}
