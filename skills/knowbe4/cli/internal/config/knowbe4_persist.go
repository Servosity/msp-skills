// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

// knowbe4 handfix (config-file-holds-no-credential, Codex r1 4.34.2 class
// credential-persisted-outside-the-store): config.toml is not the credential
// store, so it never holds a credential. Saving the configuration (auth
// set-token, auth login, a minted or refreshed token) wrote base_url and
// token_url as configured, environment values included, so the userinfo
// password or a credential query value they carried landed in config.toml.
// Every value config.toml keeps passes persistedWithoutCredentials (URLs lose
// their userinfo and credential or held query values; a credential-named or
// held header is left out), and refuseCredentialInConfigFile refuses the
// write when the bytes still repeat any value the credential kit holds. The credential
// store (0600) keeps the base URL it was issued for, as before.

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// ErrConfigHoldsCredential marks a config.toml write refused because it would
// hold a credential.
var ErrConfigHoldsCredential = errors.New("refusing to write config.toml: it would hold a credential (keep credentials in the environment or the credential store)")

// persistedWithoutCredentials is what config.toml may keep of p. Header
// values are held for output whatever their name (knowbe4_hold.go), so a header
// is left out by its name or when its value repeats one of the credentials
// this configuration or the environment carries; URLs and the TLS settings
// are checked against every held value.
func persistedWithoutCredentials(p persistedConfig, c *Config) persistedConfig {
	creds := persistGateCredentials(c)
	if clean, changed := urlWithoutCredentials(p.BaseURL, creds); changed {
		if clean == "" {
			clean = builtinBaseURL()
		}
		warnLeftOut("base_url")
		p.BaseURL = clean
	}
	if len(p.Headers) > 0 {
		kept := make(map[string]string, len(p.Headers))
		for k, v := range p.Headers {
			// Codex r1 (domotz 4.34.2) class persistence-gate-narrower-than-chokepoint: a
			// header carrying the key in base32 is the key.
			if headerHoldsCredential(k, v, creds) {
				warnLeftOut("headers." + k)
				continue
			}
			kept[k] = v
		}
		p.Headers = kept
	}
	for _, f := range []*string{&p.CACert, &p.TLSServerName} {
		if *f != "" && kitWithholds([]byte(*f)) {
			*f = ""
		}
	}
	// Every field the write persists is judged together, header values
	// included (persistedEchoesCredential): a credential split across two
	// headers, or across a header and another field, is no header's alone.
	// Fail closed: no header is kept.
	if len(p.Headers) > 0 && persistedEchoesCredential(p, creds) {
		for k := range p.Headers {
			warnLeftOut("headers." + k)
		}
		p.Headers = map[string]string{}
	}
	return p
}

// persistGateCredentials is every credential a config.toml write must not
// repeat: those configCredentialValues names (the key, client ID and secret,
// tokens, auth header, the credential variables) and every
// Reporting credential the configuration carries (ReportingCredentials: the
// credential-named header values and the base URL's userinfo password and
// credential query values, from every layer).
func persistGateCredentials(c *Config) []string {
	return append(configCredentialValues(c), ReportingCredentials(c)...)
}

// headerHoldsCredential reports whether one configured header may not be
// written: its name is a credential's, or its value repeats one of creds in
// any form the output chokepoint finds.
func headerHoldsCredential(name, value string, creds []string) bool {
	return credentialName(name) || echoes(value, creds)
}

// persistedEchoesCredential is the combined check of one config.toml write
// (knowbe4 handfix r2-persistence-gate-skipped-under-agentcookie, Codex r2
// knowbe4 4.34.2): p, every field the write persists with every header value
// in it, marshaled as save() marshals it, must not repeat any of creds in any
// form, whole or split across any fields. A header value alone, or the file
// without its headers, is not the bytes written: KNOWBE4_REGION carrying the
// key's prefix and a header carrying its suffix passed both and were written.
func persistedEchoesCredential(p persistedConfig, creds []string) bool {
	if len(creds) == 0 {
		return false
	}
	b, err := toml.Marshal(p)
	if err != nil {
		return true
	}
	return echoes(string(b), creds)
}

// configCredentialValues is every credential c (and the file layer under it)
// and the environment carry: the client ID and secret, the tokens, the
// auth header (whole and without any scheme word), and the value of every
// credential variable the credential kit declares (credwire SecretEnv).
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
		add(cfg.Knowbe4ApiKey, cfg.ClientID, cfg.ClientSecret, cfg.AccessToken, cfg.RefreshToken, cfg.AuthHeaderVal)
		if _, rest, ok := strings.Cut(cfg.AuthHeaderVal, " "); ok {
			add(rest)
		}
		if cfg.fileConfig == cfg {
			break
		}
	}
	add(os.Getenv("KNOWBE4_API_KEY"), os.Getenv("KNOWBE4_USER_EVENT_API_KEY"), os.Getenv("KNOWBE4_MCP_HTTP_TOKEN"))
	return out
}

// refuseCredentialInConfigFile is the last check before config.toml is
// written: the file, with the header values left out, must not repeat any
// held value in any form (the header values are held for output whatever they
// are, so persistedWithoutCredentials judged them, and the whole file with
// them, against every credential the configuration and environment carry).
// An agentcookie-managed configuration is the external store's declared
// secrets file (agentcookie.toml [secrets.file]): its credential fields are
// written as that store expects, and every other field passes this same
// gate (knowbe4 handfix r1-persistence-gate-skipped-under-agentcookie, Codex
// r1 knowbe4 4.34.2: with the .agentcookie-managed marker, a base_url
// carrying a userinfo password or a credential query value from
// KNOWBE4_BASE_URL was written to config.toml verbatim). Its header values
// are judged as persistedWithoutCredentials judges them, every field the write
// persists is judged together with them (persistedEchoesCredential), and the
// write is refused rather than rewritten. The held-value check below reads the
// file without its header values, which are held for output whatever they
// are; persistedEchoesCredential judged them with every other field.
func refuseCredentialInConfigFile(persist any, data []byte) error {
	p, ok := persist.(persistedConfig)
	if cfg, managed := persist.(Config); managed {
		creds := persistGateCredentials(&cfg)
		for k, v := range cfg.Headers {
			if headerHoldsCredential(k, v, creds) {
				return ErrConfigHoldsCredential
			}
		}
		cfg.clearCredentialFields()
		p, ok = cfg.persisted(), true
		// The combined check covers every field the managed write persists,
		// header values included, as persistedWithoutCredentials does for an
		// unmanaged write (r2-persistence-gate-skipped-under-agentcookie).
		if persistedEchoesCredential(p, creds) {
			return ErrConfigHoldsCredential
		}
	}
	if !ok {
		return nil
	}
	p.Headers = nil
	check, err := toml.Marshal(p)
	if err != nil {
		return err
	}
	if kitWithholds(check) {
		return ErrConfigHoldsCredential
	}
	_ = data // the bytes written differ only by the header values, judged with the whole file by persistedEchoesCredential
	return nil
}

func warnLeftOut(field string) {
	fmt.Fprintf(os.Stderr, "warning: config.toml does not keep the credential in %s; it stays in the environment or the credential store\n", field)
}
