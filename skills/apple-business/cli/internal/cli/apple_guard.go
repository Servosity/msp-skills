// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (apple-business; handfixes.json apple-business-host-pin): host
// pinning and no response cache.
//
// The client sends to the configured API origin and the configured token
// endpoint only (scheme, host and port); every other request, including a
// redirect hop, is refused before it is dialled. The HTTP response cache is
// off, so no vendor answer is written to disk outside the store.

package cli

import (
	"cmp"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"apple-business-pp-cli/internal/client"
	"apple-business-pp-cli/internal/config"
)

const appleDefaultExchangeURL = "https://account.apple.com/auth/oauth2/token"

func init() {
	registerClientHook(func(c *client.Client) error {
		c.NoCache = true
		if c.HTTPClient == nil {
			return nil
		}
		pinHTTPClient(c.HTTPClient, c.BaseURL, tokenURLOf(c.Config))
		return nil
	})
}

func tokenURLOf(cfg *config.Config) string {
	if cfg == nil {
		return appleDefaultExchangeURL
	}
	return cmp.Or(strings.TrimSpace(cfg.TokenURL), appleDefaultExchangeURL)
}

// pinHTTPClient wraps hc's transport so it reaches only the given origins.
func pinHTTPClient(hc *http.Client, origins ...string) {
	allowed := map[string]bool{}
	for _, o := range origins {
		if key, ok := originKey(o); ok {
			allowed[key] = true
		}
	}
	if _, already := hc.Transport.(*pinnedTransport); already {
		return
	}
	hc.Transport = &pinnedTransport{base: cmp.Or[http.RoundTripper](hc.Transport, http.DefaultTransport), allowed: allowed}
}

// originKey is scheme://host:port with the scheme's default port filled in.
func originKey(raw string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return "", false
	}
	port := u.Port()
	if port == "" {
		port = map[string]string{"https": "443", "http": "80"}[u.Scheme]
	}
	return strings.ToLower(u.Scheme + "://" + u.Hostname() + ":" + port), true
}

type pinnedTransport struct {
	base    http.RoundTripper
	allowed map[string]bool
}

func (t *pinnedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if key, ok := originKey(r.URL.String()); !ok || !t.allowed[key] {
		// The URL is not printed: it may be what a hostile answer redirected to.
		return nil, fmt.Errorf("refusing a request to %s://%s: the client sends only to the configured Apple Business API and token endpoint", r.URL.Scheme, r.URL.Host)
	}
	return t.base.RoundTrip(r)
}
