// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored (handfixes.json: error-body-redacts-secrets,
// transport-error-withholds-echoed-secret, dry-run-redacts-secret-body),
// ported from skills/connectsecure/cli/internal/client/connectsecure_redact.go.

package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"

	"watchguard-pp-cli/internal/cliutil"
)

// secretBodyKeys name request and response body fields whose values are
// secrets in WatchGuard Cloud: BOVPN pre-shared keys (psk, psk_encrypted),
// certificate private keys and passwords (pvt_key, password, passphrase),
// AuthPoint passwords, one-time passwords and token activation and
// verification codes, product activation keys, and generic credential names.
// Matched as substrings of the lower-cased key, except secretBodyExactKeys.
var secretBodyKeys = []string{"password", "passphrase", "secret", "token", "psk", "pvt_key", "private_key", "privatekey", "privkey", "apikey", "api_key", "activationkey", "activation_key", "activationcode", "verificationcode"}

// secretBodyExactKeys are too short to match as substrings ("otp" is inside
// "footprint").
var secretBodyExactKeys = map[string]bool{"otp": true}

// withheldBody replaces an error body that echoed a request secret.
const withheldBody = "[response body withheld: it contains a value from a sensitive field of your request, possibly by coincidence; preview the request with --dry-run]"

func isSecretBodyKey(k string) bool {
	k = strings.ToLower(k)
	if secretBodyExactKeys[k] {
		return true
	}
	for _, s := range secretBodyKeys {
		if strings.Contains(k, s) {
			return true
		}
	}
	return false
}

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
			if isSecretBodyKey(k) && !isFlagValue(val) {
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
func requestSecrets(body []byte, headers map[string]string) []string {
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
				collect(val, k, isSecretBodyKey(k), under || own)
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
	if authHeader == "" {
		return nil
	}
	out := []string{authHeader}
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
func transportError(err error, reqURL string, reqBody []byte, headers map[string]string, authHeader string) error {
	full := err.Error()
	text := strings.ReplaceAll(full, reqURL, "")
	host, hostname := "", ""
	if u, perr := url.Parse(reqURL); perr == nil && u.Host != "" {
		host, hostname = u.Host, u.Hostname()
		text = strings.ReplaceAll(strings.ReplaceAll(text, host, ""), hostname, "")
	}
	secrets := append(requestSecrets(reqBody, headers), authSecrets(authHeader)...)
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
func errorBody(respBody, reqBody []byte, headers map[string]string, authHeader string) string {
	secrets := append(requestSecrets(reqBody, headers), authSecrets(authHeader)...)
	shown := truncateBody(redactBody(respBody))
	if cliutil.EchoesSecret(string(respBody), secrets) || cliutil.EchoesSecret(shown, secrets) {
		return withheldBody
	}
	return shown
}
