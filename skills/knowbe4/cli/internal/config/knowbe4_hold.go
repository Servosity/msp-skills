// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"bytes"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"regexp"
	"strings"

	"knowbe4-pp-cli/internal/credkit"
)

// ReportingCredentials is every Reporting API credential c carries, whatever
// its source (the environment, the config file or the 0600 credential store,
// and the file layer an environment value overrode): the API key, the tokens,
// the client secret, the auth header and every credential-named configured
// header (whole and without a scheme word), and the base URL's userinfo
// password and credential-named query values.
// The User Event API client refuses a request that repeats any of them
// (knowbe4 handfix r1-credential-to-other-api-host): the Reporting key never
// reaches the User Event host.
func ReportingCredentials(c *Config) []string {
	var out []string
	add := func(vs ...string) {
		for _, v := range vs {
			if v = strings.TrimSpace(v); v != "" {
				out = append(out, v)
			}
		}
	}
	seen := map[*Config]bool{}
	for cfg := c; cfg != nil && !seen[cfg]; cfg = cfg.fileConfig {
		seen[cfg] = true
		add(cfg.Knowbe4ApiKey, cfg.AccessToken, cfg.RefreshToken, cfg.ClientSecret, cfg.AuthHeaderVal)
		if _, rest, ok := strings.Cut(strings.TrimSpace(cfg.AuthHeaderVal), " "); ok {
			add(rest)
		}
		for k, v := range cfg.Headers {
			if credentialName(k) {
				add(v)
				if _, rest, ok := strings.Cut(strings.TrimSpace(v), " "); ok {
					add(rest)
				}
			}
		}
		add(urlCredentials(cfg.BaseURL)...)
	}
	return out
}

// SentReportingCredentials is the credential the Reporting API client
// actually sends as its own: the Authorization header AuthHeader builds from
// the effective configuration (the auth header, else the API key, else the
// access token, in AuthHeader's order), whole and without its scheme word, and
// the value it is built from. A value an environment variable overrode (the
// file layer's api_key under KNOWBE4_API_KEY) is not sent, so it is not here.
// Only this exempts a value from the Reporting host's foreign set (knowbe4
// handfix r2-credential-to-other-api-host, Codex r2 knowbe4 4.34.2: the
// exemption read ReportingCredentials, every layer, so a config file api_key
// equal to the User Event key, overridden by KNOWBE4_API_KEY, let the User
// Event key into a Reporting query). Unlike AuthHeader it records no auth
// source.
func SentReportingCredentials(c *Config) []string {
	if c == nil {
		return nil
	}
	var header, raw string
	switch {
	case strings.TrimSpace(c.AuthHeaderVal) != "":
		header = c.AuthHeaderVal
	case c.Knowbe4ApiKey != "":
		raw = c.Knowbe4ApiKey
		header = applyAuthFormat("Bearer {token}", map[string]string{"token": raw})
	case c.AccessToken != "":
		raw = c.AccessToken
		header = applyAuthFormat("Bearer {token}", map[string]string{"token": raw})
	}
	var out []string
	for _, v := range []string{header, raw} {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	if _, rest, ok := strings.Cut(strings.TrimSpace(header), " "); ok && strings.TrimSpace(rest) != "" {
		out = append(out, strings.TrimSpace(rest))
	}
	return out
}

// credentialName reports whether a header or query name says it carries a
// credential.
func credentialName(name string) bool {
	n := strings.NewReplacer("-", "", "_", "", " ", "").Replace(strings.ToLower(name))
	for _, m := range []string{"token", "secret", "passw", "pwd", "key", "auth", "credential", "sig", "cookie", "bearer", "session"} {
		if strings.Contains(n, m) {
			return true
		}
	}
	return false
}

var (
	urlUserinfo = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9+.-]*://)([^/?#@]*)@`)
	encodedRun  = regexp.MustCompile(`[A-Za-z0-9+/_=-]{12,}`)
)

// urlCredentials is the userinfo password and every credential-named query
// value of a base URL, read from its text so a template base URL
// (https://{region}...), which does not parse, gives them up too.
func urlCredentials(raw string) []string {
	var out []string
	if m := urlUserinfo.FindStringSubmatch(strings.TrimSpace(raw)); m != nil {
		if _, pw, ok := strings.Cut(m[2], ":"); ok && pw != "" {
			out = append(out, pw)
		}
	}
	_, query, _ := strings.Cut(raw, "?")
	query, _, _ = strings.Cut(query, "#")
	for _, pair := range strings.Split(query, "&") {
		if name, v, _ := strings.Cut(pair, "="); v != "" && credentialName(name) {
			out = append(out, v)
		}
	}
	return out
}

// urlWithoutCredentials is raw without its userinfo and without every query
// value that is credential-named or one of creds.
func urlWithoutCredentials(raw string, creds []string) (string, bool) {
	clean := urlUserinfo.ReplaceAllString(raw, "$1")
	if base, query, ok := strings.Cut(clean, "?"); ok {
		query, frag, hasFrag := strings.Cut(query, "#")
		var kept []string
		for _, pair := range strings.Split(query, "&") {
			name, v, _ := strings.Cut(pair, "=")
			if pair == "" || credentialName(name) || (v != "" && echoes(v, creds)) {
				continue
			}
			kept = append(kept, pair)
		}
		clean = base
		if len(kept) > 0 {
			clean += "?" + strings.Join(kept, "&")
		}
		if hasFrag {
			clean += "#" + frag
		}
	}
	return clean, clean != raw
}

// echoes reports whether text repeats any of values (the effective
// credentials of this configuration, a short known list): whole, any piece of
// 8 characters, or inside a base64, base32 or hex run of text that decodes to
// one. It is the config-write gate's check against those values only; the
// credential kit judges every output against everything it holds.
func echoes(text string, values []string) bool {
	forms := []string{text}
	for _, run := range encodedRun.FindAllString(text, -1) {
		forms = append(forms, decodings(run)...)
	}
	for _, f := range forms {
		for _, v := range values {
			if repeats(f, v) {
				return true
			}
		}
	}
	return false
}

func repeats(text, v string) bool {
	if len(v) < 8 {
		return v != "" && strings.Contains(text, v)
	}
	for i := 0; i+8 <= len(v); i++ {
		if strings.Contains(text, v[i:i+8]) {
			return true
		}
	}
	return false
}

func decodings(run string) []string {
	var out []string
	for _, dec := range []func(string) ([]byte, error){
		base64.StdEncoding.DecodeString, base64.RawStdEncoding.DecodeString,
		base64.URLEncoding.DecodeString, base64.RawURLEncoding.DecodeString,
		base32.StdEncoding.DecodeString, base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString,
		hex.DecodeString,
	} {
		if b, err := dec(run); err == nil {
			out = append(out, string(b))
		}
	}
	return out
}

// kitWithholds reports whether the credential kit would withhold b from a
// file: it repeats, in any form the kit reads, a value the kit holds.
func kitWithholds(b []byte) bool {
	var buf bytes.Buffer
	_, err := credkit.NewWriter(&buf, credkit.SinkFile).Write(b)
	return err != nil || !bytes.Equal(buf.Bytes(), b)
}
