// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

// kaseya-bms handfix (config-file-holds-no-credential, Codex r1 4.34.2 class
// credential-persisted-outside-the-store): config.toml is not the credential
// store, so it never holds a credential. Saving the configuration (auth
// set-token, auth login, a minted or refreshed token) wrote base_url as
// configured, environment values included, so the userinfo password or a
// credential query value they carried landed in config.toml. Every value
// config.toml keeps passes persistedWithoutCredentials (URLs lose their
// userinfo and credential-named or held query values; a credential-named or
// credential-shaped header is left out), and refuseCredentialInConfigFile
// refuses the write when the exact bytes about to be written, but the
// [headers] table, still carry a value the credential kit holds, whatever
// shape marshaled them (an agentcookie-managed save marshals the whole
// configuration, credentials included, and is refused). The [headers] table
// is config.toml's own credential source (the kit holds every header value),
// kept as the operator configured it: a config rewrite that keeps it is a
// declared source write, not a sink. The credential store
// (0600) keeps the base URL it was issued for, as before. The write itself
// is a credential source the kit does not route (handfixes.json
// credkit-wiring: Declarations.Files names config.toml); the check is the
// kit's own matcher (cliutil.Held), never a matcher of the connector's.

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"kaseya-bms-pp-cli/internal/cliutil"

	"github.com/pelletier/go-toml/v2"
)

// ErrConfigHoldsCredential marks a config.toml write refused because it would
// hold a credential.
var ErrConfigHoldsCredential = errors.New("refusing to write config.toml: it would hold a credential (keep credentials in the environment or the credential store)")

// persistedWithoutCredentials is what config.toml may keep of p: the base URL
// without its userinfo and credential-named or held query values, the headers
// that are neither credential-named nor credential-shaped, and TLS settings
// that carry no held value.
func persistedWithoutCredentials(p persistedConfig, c *Config) persistedConfig {
	holdConfigValues(c)
	if clean, changed := urlWithoutCredentials(p.BaseURL); changed {
		if clean == "" {
			clean = builtinBaseURL()
		}
		warnLeftOut("base_url")
		p.BaseURL = clean
	}
	if len(p.Headers) > 0 {
		kept := make(map[string]string, len(p.Headers))
		for k, v := range p.Headers {
			if cliutil.SecretFieldName(k) || cliutil.CredentialShaped(v) {
				warnLeftOut("headers." + k)
				continue
			}
			kept[k] = v
		}
		p.Headers = kept
	}
	for _, f := range []*string{&p.CACert, &p.TLSServerName} {
		if cliutil.Held(*f) {
			*f = ""
		}
	}
	return p
}

// urlWithoutCredentials drops a URL's userinfo, each query value whose name
// reads as a credential or whose value is held, and a held fragment. A URL
// that does not parse is dropped whole when it names userinfo or carries a
// held value.
func urlWithoutCredentials(raw string) (clean string, changed bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return raw, false
	}
	u, err := url.Parse(s)
	if err != nil {
		authority := s
		if _, after, ok := strings.Cut(s, "://"); ok {
			authority = after
		}
		if i := strings.IndexAny(authority, "?#"); i >= 0 {
			authority = authority[:i]
		}
		if strings.Contains(authority, "@") || cliutil.Held(s) {
			return "", true
		}
		return raw, false
	}
	if u.User != nil {
		u.User = nil
		changed = true
	}
	if u.RawQuery != "" {
		var kept []string
		for _, pair := range strings.FieldsFunc(u.RawQuery, func(r rune) bool { return r == '&' || r == ';' }) {
			k, v, _ := strings.Cut(pair, "=")
			dk, dv := k, v
			if d, derr := url.QueryUnescape(k); derr == nil {
				dk = d
			}
			if d, derr := url.QueryUnescape(v); derr == nil {
				dv = d
			}
			if cliutil.SecretFieldName(dk) || cliutil.Held(v) || cliutil.Held(dv) {
				changed = true
				continue
			}
			kept = append(kept, pair)
		}
		u.RawQuery = strings.Join(kept, "&")
	}
	if u.Fragment != "" && cliutil.Held(u.Fragment) {
		u.Fragment, u.RawFragment = "", ""
		changed = true
	}
	if !changed {
		return raw, false
	}
	return u.String(), true
}

// refuseCredentialInConfigFile is the last check before config.toml is
// written, on the exact bytes about to be written: they must read back, and
// every value but the [headers] table must carry no value the credential kit
// holds (the configuration's own credentials are held first). config.toml is
// never a credential store, an agentcookie-managed one included: such a save
// is refused rather than written.
func refuseCredentialInConfigFile(data []byte, c *Config) error {
	holdConfigValues(c)
	var doc map[string]any
	if err := toml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("%w (the bytes to be written do not read back: %v)", ErrConfigHoldsCredential, err)
	}
	if h, ok := doc["headers"]; ok {
		if _, isTable := h.(map[string]any); !isTable {
			return ErrConfigHoldsCredential
		}
		delete(doc, "headers")
	}
	rest, err := toml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("%w (%v)", ErrConfigHoldsCredential, err)
	}
	if cliutil.Held(string(rest)) {
		return ErrConfigHoldsCredential
	}
	return nil
}

func warnLeftOut(field string) {
	fmt.Fprintf(os.Stderr, "warning: config.toml does not keep the credential in %s; it stays in the environment or the credential store\n", field)
}
