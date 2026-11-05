// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

// rocketcyber handfix (config-file-holds-no-credential, Codex r1 4.34.2 class
// credential-persisted-outside-the-store), hand-written: saving the
// configuration (auth set-token, a saved token) wrote base_url as configured,
// ROCKETCYBER_BASE_URL included, so a userinfo password or a query value it
// carried landed in config.toml. config.toml is written outside the shared
// credential kit (a credential source, a declared exemption), so the kit
// cannot withhold it there: config.toml keeps base_url without its userinfo
// and its query. The RocketCyber Customer API base URL takes neither.
// ponytail: the whole query goes, not only credential-named values; keep a
// named parameter when a real base URL needs one.

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

// persistedWithoutCredentials is what config.toml may keep of p.
func persistedWithoutCredentials(p persistedConfig) persistedConfig {
	p.BaseURL = baseURLWithoutCredentials(p.BaseURL)
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
		warnLeftOut()
		return builtinBaseURL()
	}
	if u.User == nil && u.RawQuery == "" && !u.ForceQuery && u.Fragment == "" {
		return raw
	}
	u.User, u.RawQuery, u.ForceQuery, u.Fragment, u.RawFragment = nil, "", false, "", ""
	warnLeftOut()
	return u.String()
}

func warnLeftOut() {
	fmt.Fprintln(os.Stderr, "warning: config.toml keeps base_url without its userinfo and query; a credential there stays in the environment or the file it came from")
}
