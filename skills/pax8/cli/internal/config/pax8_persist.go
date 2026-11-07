// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

// pax8 handfix (config-file-holds-no-credential, Codex r1 and r2 4.34.2 class
// credential-persisted-outside-the-store), hand-written: config.toml is written
// outside the shared credential kit (a credential source, a declared exemption,
// credkit-wiring), so the kit cannot withhold it there. Saving the configuration
// (auth set-token, auth login, a minted or refreshed token) wrote base_url and
// token_url as configured, PAX8_BASE_URL and PAX8_TOKEN_URL included, so a
// userinfo password or a query value they carried (whole, or split across two
// query values) landed in config.toml: both are kept without userinfo, query and
// fragment. A [headers] value is a credential of config.toml itself, which the
// kit holds and lets go to the API origin only; a header the kit would refuse to
// send there carries a credential that may not go there (the client secret in
// any encoding the kit reads, ROT13 included), so config.toml leaves it out.
// ponytail: the whole query goes, not only credential-named values; keep a named
// parameter when a real Pax8 URL needs one.

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"pax8-pp-cli/internal/credkit"
)

// persistedWithoutCredentials is what config.toml may keep of p.
func persistedWithoutCredentials(p persistedConfig) persistedConfig {
	p.BaseURL = urlWithoutCredentials("base_url", p.BaseURL, builtinBaseURL())
	p.TokenURL = urlWithoutCredentials("token_url", p.TokenURL, "")
	if len(p.Headers) > 0 {
		kept := make(map[string]string, len(p.Headers))
		for k, v := range p.Headers {
			if !headerMayGoToAPI(p.BaseURL, k, v) {
				warnLeftOut("headers." + k)
				continue
			}
			kept[k] = v
		}
		p.Headers = kept
	}
	return p
}

// urlWithoutCredentials is raw without its userinfo, query and fragment. A URL
// that does not parse and could hide one (an "@", "?" or "#" in it) is replaced
// by fallback; one that cannot (an unresolved {placeholder} host) is kept.
func urlWithoutCredentials(field, raw, fallback string) string {
	if raw == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		if !strings.ContainsAny(raw, "@?#") {
			return raw
		}
		warnLeftOut(field)
		return fallback
	}
	if u.User == nil && u.RawQuery == "" && !u.ForceQuery && u.Fragment == "" {
		return raw
	}
	u.User, u.RawQuery, u.ForceQuery, u.Fragment, u.RawFragment = nil, "", false, "", ""
	warnLeftOut(field)
	return u.String()
}

// headerMayGoToAPI asks the kit's outbound guard whether a request to the API
// base URL carrying this header would be sent. Nothing is sent: the inner
// transport answers without a network.
func headerMayGoToAPI(baseURL, name, value string) bool {
	req, err := http.NewRequest(http.MethodGet, baseURL, nil)
	if err != nil {
		return false
	}
	req.Header.Set(name, value)
	resp, err := credkit.Transport(noSend{}).RoundTrip(req)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	return err == nil
}

type noSend struct{}

func (noSend) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody, Header: http.Header{}, Request: req}, nil
}

func warnLeftOut(field string) {
	fmt.Fprintf(os.Stderr, "warning: config.toml does not keep %s: it carries a credential, which stays in the environment or the file it came from\n", field)
}
