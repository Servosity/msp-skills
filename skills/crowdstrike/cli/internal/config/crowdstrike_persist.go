// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

// crowdstrike handfix (config-file-holds-no-credential, Codex r1 4.34.2 class
// credential-persisted-outside-the-store): config.toml is not the credential
// store, so it never holds a credential. Saving the configuration (auth
// set-token, auth login, a minted or refreshed token) wrote base_url and
// token_url as configured, environment values included, so the userinfo
// password or a credential query value they carried landed in config.toml.
// Every value config.toml keeps passes persistedWithoutCredentials (URLs lose
// their userinfo and credential-named query values; a credential-named or
// credential-shaped header, or one equal to a configured credential, is left
// out), and refuseCredentialInConfigFile reads back the exact bytes about to
// be written, whatever type marshaled them (handfix
// config-check-reads-back-every-shape: an agentcookie-managed save marshals the
// whole configuration), and refuses the write when they still carry a
// credential key or one of the configuration's credentials as written. The
// credential store (0600) keeps the base URL it was issued for, as before.
// Which values are withheld from output is the shared credential kit's
// (handfixes.json credkit-wiring); this file judges only what config.toml keeps.

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"crowdstrike-pp-cli/internal/cliutil"
)

// ErrConfigHoldsCredential marks a config.toml write refused because it would
// hold a credential.
var ErrConfigHoldsCredential = errors.New("refusing to write config.toml: it would hold a credential (keep credentials in the environment or the credential store)")

// persistedWithoutCredentials is what config.toml may keep of p: the URLs
// without the credentials they carry (a query value or fragment that repeats
// one of the configuration's credentials included), and every header but one
// named or shaped like a credential or equal to one of the credentials this
// configuration or the environment carries.
func persistedWithoutCredentials(p persistedConfig, c *Config) persistedConfig {
	creds := configCredentialValues(c)
	if clean, changed := cliutil.URLWithoutCredentials(p.BaseURL, creds...); changed {
		if clean == "" {
			clean = builtinBaseURL()
		}
		warnLeftOut("base_url")
		p.BaseURL = clean
	}
	// Every URL config.toml keeps, the token URL too: one that carried a
	// credential made the last check refuse the whole save, so no token could
	// be saved at all. An emptied token URL is left out (the mint derives it
	// from the base URL; the environment value still applies at run time).
	if clean, changed := cliutil.URLWithoutCredentials(p.TokenURL, creds...); changed {
		warnLeftOut("token_url")
		p.TokenURL = clean
	}
	if len(p.Headers) > 0 {
		kept := make(map[string]string, len(p.Headers))
		for k, v := range p.Headers {
			if headerHoldsCredential(k, v, creds) {
				warnLeftOut("headers." + k)
				continue
			}
			kept[k] = v
		}
		p.Headers = kept
	}
	return p
}

// configCredentialValues is every credential c (and the file layer under it)
// and the environment carry: the client ID and secret, the tokens, the
// Authorization header (whole and without its scheme), and the value of every
// credential variable (the client pair and the MCP HTTP token).
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
		add(cfg.ClientID, cfg.ClientSecret, cfg.AccessToken, cfg.RefreshToken, cfg.AuthHeaderVal)
		if _, rest, ok := strings.Cut(cfg.AuthHeaderVal, " "); ok {
			add(rest)
		}
		if cfg.fileConfig == cfg {
			break
		}
	}
	// The credential variables credwire declares (SecretEnv), by constant name.
	add(os.Getenv("FALCON_CLIENT_ID"), os.Getenv("FALCON_CLIENT_SECRET"), os.Getenv("CROWDSTRIKE_MCP_HTTP_TOKEN"))
	return out
}

// headerHoldsCredential reports a header config.toml may not keep: one named
// like a credential, one whose value is credential-shaped (an auth-scheme
// value or a JWT), and one whose value contains any of creds as written.
func headerHoldsCredential(name, value string, creds []string) bool {
	if cliutil.SecretFieldName(name) || cliutil.CredentialShaped(value) {
		return true
	}
	return containsAny(value, creds)
}

// containsAny reports whether s contains any of values as written.
func containsAny(s string, values []string) bool {
	for _, v := range values {
		if v != "" && strings.Contains(s, v) {
			return true
		}
	}
	return false
}

// refuseCredentialInConfigFile is the last check before config.toml is
// written, on the exact bytes about to be written whatever shape produced them
// (persist is marshaled when data is nil): every header must pass
// headerHoldsCredential, no key may be a credential key, and the rest of the
// file must contain none of the configuration's credentials (c, the file layer
// under it and the credential environment variables) as written. config.toml
// is never a credential store, an agentcookie-managed one included: such a
// save is refused rather than written.
func refuseCredentialInConfigFile(persist any, data []byte, c *Config) error {
	if data == nil {
		b, err := toml.Marshal(persist)
		if err != nil {
			return err
		}
		data = b
	}
	var doc map[string]any
	if err := toml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("%w (the bytes to be written do not read back: %v)", ErrConfigHoldsCredential, err)
	}
	creds := configCredentialValues(c)
	if h, ok := doc["headers"]; ok {
		headers, isTable := h.(map[string]any)
		if !isTable {
			return ErrConfigHoldsCredential
		}
		for k, v := range headers {
			s, isString := v.(string)
			if !isString || headerHoldsCredential(k, s, creds) {
				return ErrConfigHoldsCredential
			}
		}
		delete(doc, "headers")
	}
	for k, v := range doc {
		if s, isString := v.(string); isString && s != "" && credentialKey(k) {
			return ErrConfigHoldsCredential
		}
	}
	rest, err := toml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("%w (%v)", ErrConfigHoldsCredential, err)
	}
	if containsAny(string(rest), creds) {
		return ErrConfigHoldsCredential
	}
	return nil
}

// credentialKey reports a config.toml key whose value is a credential: the
// credential store's own keys and any credential-named key.
func credentialKey(k string) bool {
	switch k {
	case "token_expiry", "token_url":
		return false
	}
	return cliutil.SecretFieldName(k)
}

func warnLeftOut(field string) {
	fmt.Fprintf(os.Stderr, "warning: config.toml does not keep the credential in %s; it stays in the environment or the credential store\n", field)
}
