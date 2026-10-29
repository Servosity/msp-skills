// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package config

// pax8 handfix (config-file-holds-no-credential, Codex r1 4.34.2 class
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

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"pax8-pp-cli/internal/cliutil"
)

// ErrConfigHoldsCredential marks a config.toml write refused because it would
// hold a credential.
var ErrConfigHoldsCredential = errors.New("refusing to write config.toml: it would hold a credential (keep credentials in the environment or the credential store)")

// persistedWithoutCredentials is what config.toml may keep of p. Header
// values are held for output whatever their name (pax8_hold.go), so a header
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
	if clean, changed := cliutil.URLWithoutCredentials(p.TokenURL); changed {
		warnLeftOut("token_url")
		p.TokenURL = clean
	}
	if len(p.Headers) > 0 {
		creds := configCredentialValues(c)
		kept := make(map[string]string, len(p.Headers))
		for k, v := range p.Headers {
			if cliutil.SecretFieldName(k) || (len(creds) > 0 && cliutil.FunnelEchoes(v, creds)) {
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
// credential-named PAX8_* variable.
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
		if ok && strings.HasPrefix(name, "PAX8_") && !strings.HasSuffix(name, "_URL") && cliutil.SecretFieldName(strings.TrimPrefix(name, "PAX8_")) {
			add(v)
		}
	}
	return out
}

// refuseCredentialInConfigFile is the last check before config.toml is
// written: the file, with the header values judged above left out, must not
// repeat any held value in any form. An agentcookie-managed configuration is
// the external store's own file and is written as that store expects.
func refuseCredentialInConfigFile(persist any, data []byte) error {
	p, ok := persist.(persistedConfig)
	if !ok {
		return nil
	}
	p.Headers = nil
	check, err := toml.Marshal(p)
	if err != nil {
		return err
	}
	if cliutil.HeldEchoes(string(check)) {
		return ErrConfigHoldsCredential
	}
	_ = data // the bytes written differ only by the header values checked in persistedWithoutCredentials
	return nil
}

func warnLeftOut(field string) {
	fmt.Fprintf(os.Stderr, "warning: config.toml does not keep the credential in %s; it stays in the environment or the credential store\n", field)
}
