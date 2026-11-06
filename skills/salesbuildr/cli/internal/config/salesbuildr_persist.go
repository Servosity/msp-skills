// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

// salesbuildr handfix (config-file-holds-no-credential, Codex r1 4.34.2 class
// credential-persisted-outside-the-store), hand-written: config.toml is not
// the credential store, and it is written outside the shared credential kit
// (a credential source, a declared exemption), so the kit cannot withhold a
// credential there. Saving the configuration (auth set-token, a minted or
// refreshed token) wrote base_url as configured, SALESBUILDR_BASE_URL
// included, so a userinfo password or a query value it carried landed in
// config.toml; a [headers] value that carried the new key, plain or encoded,
// was written back beside it. config.toml keeps base_url without its userinfo
// and its query (the Salesbuildr public API base URL takes neither), and
// leaves out every header that is credential-named or whose value, raw or
// decoded, carries a credential this configuration or the environment holds,
// naming each one on stderr. An agentcookie-managed save that would write a
// credential field is refused. The credential store (0600) keeps the base URL
// it was issued for, as before.
// ponytail: the whole query goes, not only credential-named values; keep a
// named parameter when a real base URL needs one.

import (
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
)

// ErrConfigHoldsCredential marks a config.toml write refused because it would
// hold a credential.
var ErrConfigHoldsCredential = errors.New("refusing to write config.toml: it would hold a credential (keep credentials in the environment or the credential store)")

// persistedWithoutCredentials is what config.toml may keep of p.
func persistedWithoutCredentials(p persistedConfig, c *Config) persistedConfig {
	p.BaseURL = baseURLWithoutCredentials(p.BaseURL)
	p.Headers = headersWithoutCredentials(p.Headers, c)
	return p
}

// baseURLWithoutCredentials is raw without its userinfo and query. A URL that
// does not parse and could hide either (an "@", "?" or "#" in it) is replaced
// by the built-in default; one that cannot (an unresolved {placeholder} host)
// is kept as written.
func baseURLWithoutCredentials(raw string) string {
	if raw == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		if !strings.ContainsAny(raw, "@?#") {
			return raw
		}
		warnLeftOut("base_url")
		return builtinBaseURL()
	}
	if u.User == nil && u.RawQuery == "" && !u.ForceQuery && u.Fragment == "" {
		return raw
	}
	u.User, u.RawQuery, u.ForceQuery, u.Fragment, u.RawFragment = nil, "", false, "", ""
	warnLeftOut("base_url")
	return u.String()
}

// headersWithoutCredentials is h without every header that is
// credential-named or whose value carries one of the credentials c (and the
// file layer under it) and the environment hold. The result is a new map, so
// c's own headers are untouched.
func headersWithoutCredentials(h map[string]string, c *Config) map[string]string {
	if len(h) == 0 {
		return h
	}
	creds := configCredentialValues(c)
	kept := make(map[string]string, len(h))
	for k, v := range h {
		if credentialHeaderName(k) || carriesCredential(v, creds) {
			warnLeftOut("headers." + k)
			continue
		}
		kept[k] = v
	}
	return kept
}

// credentialHeaderName reports a header name that names a credential.
func credentialHeaderName(name string) bool {
	n := strings.NewReplacer("-", "", "_", "").Replace(strings.ToLower(name))
	for _, w := range []string{"apikey", "integrationkey", "authorization", "token", "secret", "password", "cookie", "credential"} {
		if strings.Contains(n, w) {
			return true
		}
	}
	return false
}

// carriesCredential reports whether v, or a decoding of it (base64, base32,
// hex or percent encoding, nested up to three layers), contains one of creds.
// ponytail: these standard encodings only; the kit still withholds every
// header value from output, so this guards config.toml alone.
func carriesCredential(v string, creds []string) bool {
	if len(creds) == 0 {
		return false
	}
	forms := []string{v}
	for depth := 0; depth <= 3 && len(forms) > 0; depth++ {
		var next []string
		for _, f := range forms {
			lf := strings.ToLower(f)
			for _, c := range creds {
				if strings.Contains(lf, strings.ToLower(c)) {
					return true
				}
			}
			if depth < 3 {
				next = append(next, decodings(f)...)
			}
		}
		forms = next
	}
	return false
}

// decodings is every standard decoding of s that succeeds.
func decodings(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var out []string
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.URLEncoding, base64.RawStdEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(s); err == nil {
			out = append(out, string(b))
		}
	}
	up := strings.ToUpper(strings.TrimRight(s, "="))
	for _, enc := range []*base32.Encoding{base32.StdEncoding.WithPadding(base32.NoPadding), base32.HexEncoding.WithPadding(base32.NoPadding)} {
		if b, err := enc.DecodeString(up); err == nil {
			out = append(out, string(b))
		}
	}
	if b, err := hex.DecodeString(s); err == nil {
		out = append(out, string(b))
	}
	if u, err := url.QueryUnescape(s); err == nil && u != s {
		out = append(out, u)
	}
	return out
}

// configCredentialValues is every credential c (and the file layer under it)
// and the environment carry: the API key, the client ID and secret, the
// tokens, the Authorization header (whole and without its scheme), and the
// value of every SALESBUILDR_* credential variable the kit declares.
func configCredentialValues(c *Config) []string {
	var out []string
	add := func(vs ...string) {
		for _, v := range vs {
			if v = strings.TrimSpace(v); v != "" {
				out = append(out, v)
			}
		}
	}
	for cfg := c; cfg != nil; cfg = cfg.fileConfig {
		add(cfg.SalesbuildrApiKey, cfg.ClientID, cfg.ClientSecret, cfg.AccessToken, cfg.RefreshToken, cfg.AuthHeaderVal)
		if _, rest, ok := strings.Cut(cfg.AuthHeaderVal, " "); ok {
			add(rest)
		}
		if cfg.fileConfig == cfg {
			break
		}
	}
	add(os.Getenv("SALESBUILDR_API_KEY"), os.Getenv("SALESBUILDR_MCP_HTTP_TOKEN"))
	return out
}

// refuseCredentialInConfigFile is the last check before config.toml is
// written (Codex r1 4.34.2 review class credential-persisted-outside-the-store:
// an agentcookie-managed save marshaled the whole Config, credential fields
// included). The whole Config is refused while any credential field is set:
// the credential would land in config.toml, which is not the 0600 credential
// store. Every shape has already passed persistedWithoutCredentials.
func refuseCredentialInConfigFile(persist any) error {
	if p, ok := persist.(Config); ok && p.hasCredentialFields() {
		return fmt.Errorf("%w; this configuration is marked as agentcookie-managed, so set the credential through agentcookie or the environment", ErrConfigHoldsCredential)
	}
	return nil
}

func warnLeftOut(field string) {
	fmt.Fprintf(os.Stderr, "warning: config.toml does not keep %s: it carried a credential, which stays in the environment or the credential store\n", field)
}
