// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package config

// appdirect handfix (config-file-holds-no-credential, Codex r1 4.34.2 class
// credential-persisted-outside-the-store): config.toml is not the credential
// store, so it never holds a credential. Saving the configuration (auth
// set-token, auth login, a minted or refreshed token) wrote base_url and
// token_url as configured, environment values included, so the userinfo
// password or a credential query value they carried landed in config.toml.
// Every value config.toml keeps passes persistedWithoutCredentials (URLs lose
// their userinfo and credential or held query values; a credential-named or
// held header is left out), and refuseCredentialInConfigFile refuses the
// write when the bytes still repeat any held value in any form. The credential
// store (0600) keeps the base URL it was issued for, as before.
//
// Codex r1 (4.34.2 appdirect review) class credential-persisted-outside-the-
// store, closed for every shape a save takes: the last check reads back the
// exact bytes about to be written, whatever type marshaled them (an
// agentcookie-managed save marshals the whole configuration, credentials
// included, and was not checked at all), and judges every header value too,
// against the configuration's own credentials with the output chokepoint's
// full matcher (cliutil.EchoesValues: every encoding, split and spelling it
// withholds; the header filter matched a credential only as written, so a
// header holding its base32 was kept). Any value it cannot read back is a
// refusal.

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"appdirect-pp-cli/internal/cliutil"
)

// ErrConfigHoldsCredential marks a config.toml write refused because it would
// hold a credential.
var ErrConfigHoldsCredential = errors.New("refusing to write config.toml: it would hold a credential (keep credentials in the environment or the credential store)")

// persistedWithoutCredentials is what config.toml may keep of p. Header
// values are held for output whatever their name (appdirect_hold.go), so a header
// is left out by its name or when its value repeats one of the credentials
// this configuration or the environment carries; URLs and the TLS settings
// are checked against every held value.
func persistedWithoutCredentials(p persistedConfig, c *Config) persistedConfig {
	if clean, changed := cliutil.URLWithoutCredentials(p.BaseURL); changed {
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
	if clean, changed := cliutil.URLWithoutCredentials(p.TokenURL); changed {
		warnLeftOut("token_url")
		p.TokenURL = clean
	}
	if len(p.Headers) > 0 {
		creds := configCredentialValues(c)
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
	for _, f := range []*string{&p.CACert, &p.TLSServerName} {
		if *f != "" && cliutil.HeldEchoes(*f) {
			*f = ""
		}
	}
	return p
}

// configCredentialValues is every credential c (and the file layer under it)
// and the environment carry: the client ID and secret, the tokens, the
// Authorization header (whole and without its scheme), and the value of every
// credential-named APPDIRECT_* variable.
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
	for _, kv := range os.Environ() {
		name, v, ok := strings.Cut(kv, "=")
		if ok && strings.HasPrefix(name, "APPDIRECT_") && !strings.HasSuffix(name, "_URL") && cliutil.SecretFieldName(strings.TrimPrefix(name, "APPDIRECT_")) {
			add(v)
		}
	}
	return out
}

// headerHoldsCredential reports a header config.toml may not keep: one named
// like a credential, one whose value is credential-shaped (an auth-scheme
// value or a JWT), and one whose value repeats any of creds in any form the
// output chokepoint withholds.
func headerHoldsCredential(name, value string, creds []string) bool {
	if cliutil.SecretFieldName(name) || cliutil.CredentialShaped(value) {
		return true
	}
	return len(creds) > 0 && cliutil.EchoesValues(value, creds)
}

// refuseCredentialInConfigFile is the last check before config.toml is
// written, on the exact bytes about to be written whatever shape produced them
// (persist is marshaled when data is nil): every header must pass
// headerHoldsCredential, and the rest of the file must repeat no held value
// and none of the configuration's credentials (c, the file layer under it and
// the credential environment variables), in any form. config.toml is never a
// credential store, an agentcookie-managed one included: such a save is
// refused rather than written.
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
	rest, err := toml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("%w (%v)", ErrConfigHoldsCredential, err)
	}
	if cliutil.HeldEchoes(string(rest)) || (len(creds) > 0 && cliutil.EchoesValues(string(rest), creds)) {
		return ErrConfigHoldsCredential
	}
	return nil
}

func warnLeftOut(field string) {
	fmt.Fprintf(os.Stderr, "warning: config.toml does not keep the credential in %s; it stays in the environment or the credential store\n", field)
}
