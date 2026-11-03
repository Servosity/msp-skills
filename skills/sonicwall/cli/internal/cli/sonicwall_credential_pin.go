// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"

	"sonicwall-pp-cli/internal/client"
)

// Hand-wired (handfixes.json: credential-host-pin). The CLI talks to two
// vendor hosts with two credentials: the MySonicWall API key (X-API-KEY) belongs
// to api.mysonicwall.com and the NSM bearer token (Authorization) to the
// configured NSM host. The generated client picks a credential by request PATH,
// so a same-host redirect or a SONICWALL_BASE_URL override could hand one
// host's credential to the other. This transport is the last hop before the
// wire, so it sees every request, redirects included, and refuses any request
// that carries a credential to a host that does not own it. A loopback NSM base
// (a local test server) owns both.

const mswAPIOrigin = "https://api.mysonicwall.com:443"

var errCredentialWrongHost = errors.New("refused: a SonicWall credential was about to be sent to a host that does not own it")

type credentialPinTransport struct {
	base http.RoundTripper
	c    *client.Client
}

func (t credentialPinTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := credentialHostAllowed(req, configuredNSMOrigin(t.c)); err != nil {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, err
	}
	return t.base.RoundTrip(req)
}

// credentialHostAllowed applies the pin. nsmOrigin is "" when the configured
// base cannot be resolved; then no request may carry the NSM token.
func credentialHostAllowed(req *http.Request, nsmOrigin string) error {
	to := requestOrigin(req.URL)
	nsmLoopback := nsmOrigin != "" && originIsLoopback(nsmOrigin)
	if req.Header.Get("X-API-KEY") != "" && to != mswAPIOrigin && !(nsmLoopback && to == nsmOrigin) {
		return errCredentialWrongHost
	}
	if req.Header.Get("Authorization") != "" && (nsmOrigin == "" || to != nsmOrigin || to == mswAPIOrigin) {
		return errCredentialWrongHost
	}
	return nil
}

func configuredNSMOrigin(c *client.Client) string {
	if c == nil {
		return ""
	}
	base := c.RequestBaseURL()
	if c.Config != nil {
		for k, v := range c.Config.TemplateVars {
			base = strings.ReplaceAll(base, "{"+k+"}", v)
		}
	}
	if strings.ContainsAny(base, "{}") {
		return ""
	}
	u, err := url.Parse(strings.TrimSpace(base))
	if err != nil || u.Host == "" {
		return ""
	}
	return requestOrigin(u)
}

func requestOrigin(u *url.URL) string {
	if u == nil {
		return ""
	}
	scheme := strings.ToLower(u.Scheme)
	port := u.Port()
	if port == "" {
		switch scheme {
		case "https":
			port = "443"
		case "http":
			port = "80"
		}
	}
	return scheme + "://" + net.JoinHostPort(strings.TrimSuffix(strings.ToLower(u.Hostname()), "."), port)
}

func originIsLoopback(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	h := u.Hostname()
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}
