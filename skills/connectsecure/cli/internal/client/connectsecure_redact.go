// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored: --dry-run must not echo the secrets a write body carries.

package client

import (
	"bytes"

	"connectsecure-pp-cli/internal/cliutil"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/url"
	"strconv"
	"strings"
)

// secretBodyKeys name body fields whose values are secrets. "params" is the
// whole credential object on credentials and integration-credentials writes;
// "creds" and "hashes" carry the credentials and password hashes an attack
// surface scan found.
var secretBodyKeys = []string{"password", "secret", "token", "passphrase", "private", "privkey", "community", "apikey", "api_key", "params", "creds", "hashes"}

// RedactSecretBody masks secret-named JSON fields for callers outside this
// package (the token mint's error output).
func RedactSecretBody(body []byte) []byte { return connectsecureRedactBody(body) }

// connectsecureRedactBody returns body with every secret-named field's value
// replaced. A body that is not JSON is returned unchanged (dryRun skips it).
func connectsecureRedactBody(body []byte) []byte {
	var v any
	if json.Unmarshal(body, &v) != nil {
		return body
	}
	// No HTML escaping: an error message keeps its & < > readable.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if enc.Encode(redactSecretFields(v)) != nil {
		return body
	}
	return bytes.TrimRight(buf.Bytes(), "\n")
}

func redactSecretFields(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if isSecretBodyKey(k) && !isFlagValue(val) {
				t[k] = "[redacted]"
			} else {
				t[k] = redactSecretFields(val)
			}
		}
	case []any:
		for i := range t {
			t[i] = redactSecretFields(t[i])
		}
	}
	return v
}

func isSecretBodyKey(k string) bool {
	k = strings.ToLower(k)
	for _, s := range secretBodyKeys {
		if strings.Contains(k, s) {
			return true
		}
	}
	return false
}

// isFlagValue keeps booleans and nulls readable (token_expired: true): they
// cannot carry a secret.
func isFlagValue(v any) bool {
	switch v.(type) {
	case bool, nil:
		return true
	}
	return false
}

// connectsecureWithheldBody replaces an error body that echoed a request secret.
const connectsecureWithheldBody = "[response body withheld: it contains a value from a sensitive field of your request, possibly by coincidence; preview the request with --dry-run]"

// connectsecureRequestSecrets returns every secret value a request carried:
// any value whose own key is secret-named; strings of 6+ characters elsewhere
// under a secret-named object (params holds the username, port and type next
// to the password, and a port must not withhold every error); secret-named
// multipart parts (custom-domains create uploads ssl_privkey as a file part);
// and secret-named header overrides, plus the decoded Client-Auth-Token and
// the client secret inside it. Hand-authored (handfixes.json:
// error-body-redacts-secrets).
func connectsecureRequestSecrets(body []byte, headers map[string]string) []string {
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
			if own || (under && len(t) >= 6 && !benignParamKey(key)) {
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
	} else if bytes.HasPrefix(body, []byte("--")) {
		if i := bytes.Index(body, []byte("\r\n")); i > 2 {
			mr := multipart.NewReader(bytes.NewReader(body), string(body[2:i]))
			for {
				part, err := mr.NextPart()
				if err != nil {
					break
				}
				if isSecretBodyKey(part.FormName()) {
					if b, err := io.ReadAll(part); err == nil {
						add(string(b))
					}
				}
			}
		}
	}
	for k, val := range headers {
		if isSecretHeader(k) {
			add(val)
			if parts := authSecrets(val); len(parts) > 1 { // "Bearer <tok>" -> "<tok>"
				add(parts[1])
			}
			// Client-Auth-Token is base64 of "tenant+client_id:client_secret".
			if dec, err := base64.StdEncoding.DecodeString(val); err == nil {
				add(string(dec))
				if i := bytes.LastIndexByte(dec, ':'); i >= 0 {
					add(string(dec[i+1:]))
				}
			}
		}
	}
	return out
}

// isSecretHeader: a secret-named header, or one that authenticates
// (Authorization, Cookie, X-Api-Key) even though the name says no "token".
func isSecretHeader(k string) bool {
	l := strings.ToLower(k)
	return isSecretBodyKey(k) || strings.Contains(l, "auth") || strings.Contains(l, "cookie") || strings.Contains(l, "key")
}

// connectsecureSentHeaders is every header the request sent that can carry a
// credential: the configured Headers (config file), then per-request
// overrides, which win as they do on the wire.
func (c *Client) connectsecureSentHeaders(overrides map[string]string) map[string]string {
	if c == nil || c.Config == nil || len(c.Config.Headers) == 0 {
		return overrides
	}
	out := make(map[string]string, len(c.Config.Headers)+len(overrides))
	for k, v := range c.Config.Headers {
		out[k] = v
	}
	for k, v := range overrides {
		out[k] = v
	}
	return out
}

// connectsecureTransportError withholds a transport error that echoes a
// secret the request carried: a refused redirect's error names its Location,
// which a pod can fill with the submitted password. The request's own URL
// and host are built by the CLI (a DNS error names the host), so they are
// dropped before matching; a server-supplied Location keeps its query.
func connectsecureTransportError(err error, reqURL string, reqBody []byte, headers map[string]string, authHeader string) error {
	text := strings.ReplaceAll(err.Error(), reqURL, "")
	if u, perr := url.Parse(reqURL); perr == nil && u.Host != "" {
		text = strings.ReplaceAll(strings.ReplaceAll(text, u.Host, ""), u.Hostname(), "")
	}
	secrets := append(connectsecureRequestSecrets(reqBody, headers), authSecrets(authHeader)...)
	if cliutil.ConnectsecureEchoesSecret(text, secrets) {
		return errors.New("[transport error withheld: it echoed a secret the request carried, possibly by coincidence]")
	}
	return err
}

// connectsecureErrorBody is the text an API error may show: withheld when the
// response carries a request secret in any form (raw or as displayed after
// re-encoding, checked before truncation), else secret-named fields masked.
func connectsecureErrorBody(respBody, reqBody []byte, headers map[string]string, authHeader string) string {
	secrets := append(connectsecureRequestSecrets(reqBody, headers), authSecrets(authHeader)...)
	shown := truncateBody(connectsecureRedactBody(respBody))
	if cliutil.ConnectsecureEchoesSecret(string(respBody), secrets) || cliutil.ConnectsecureEchoesSecret(shown, secrets) {
		return connectsecureWithheldBody
	}
	return shown
}

// connectsecureStatusFalse reports whether a 2xx write answered with the
// vendor's failure envelope {"status": false, ...}; the token mint and doctor
// already read it that way. Hand-authored (handfixes.json:
// write-status-false-is-an-error).
func connectsecureStatusFalse(body []byte) bool {
	var env struct {
		Status *bool `json:"status"`
	}
	return json.Unmarshal(body, &env) == nil && env.Status != nil && !*env.Status
}

// benignParamKeys are params fields that name the account or target rather
// than hold a secret; an error naming them ("login failed for user
// administrator") stays readable.
var benignParamKeys = map[string]bool{"username": true, "user_name": true, "login": true, "domain": true, "host": true, "hostname": true, "address": true, "ip": true, "name": true, "type": true, "credential_type": true, "port": true, "version": true, "protocol": true, "auth_type": true}

func benignParamKey(k string) bool { return benignParamKeys[strings.ToLower(k)] }

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

// SafeServerText gives a server message the CLI prints outside an APIError
// (a 200 {"status": false} envelope read by a novel command or doctor) the
// same treatment: withheld if it echoes the CLI's token, else masked.
// Hand-authored (handfixes.json: error-body-redacts-secrets).
func (c *Client) SafeServerText(text string) string {
	auth := ""
	if c != nil && c.Config != nil {
		auth = c.Config.AuthHeader()
	}
	if cliutil.ConnectsecureEchoesSecret(text, append(authSecrets(auth), connectsecureRequestSecrets(nil, c.connectsecureSentHeaders(nil))...)) {
		return connectsecureWithheldBody
	}
	return c.maskCredentialText(text, auth)
}
