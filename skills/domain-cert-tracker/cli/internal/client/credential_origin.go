// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"net/http"
	"net/url"
	"strings"
)

// Hand-fix (handfixes.json credentials-base-origin-only): the configured
// auth_header and custom headers belong to base_url's origin. Requests to the
// per-operation servers (crt.sh, IANA) and redirect hops off the origin carry
// neither.

// credentialOrigin reports whether target has base's scheme, host and port.
func credentialOrigin(target, base string) bool {
	t, err1 := url.Parse(strings.TrimSpace(target))
	b, err2 := url.Parse(strings.TrimSpace(base))
	if err1 != nil || err2 != nil || t.Host == "" || b.Host == "" {
		return false
	}
	return strings.EqualFold(t.Scheme, b.Scheme) && strings.EqualFold(originHostPort(t), originHostPort(b))
}

func originHostPort(u *url.URL) string {
	if p := u.Port(); p != "" {
		return u.Hostname() + ":" + p
	}
	if strings.EqualFold(u.Scheme, "http") {
		return u.Hostname() + ":80"
	}
	return u.Hostname() + ":443"
}

// dropOffOriginCredentials removes the auth header and every configured
// custom header from a redirect hop that leaves base's origin.
func (c *Client) dropOffOriginCredentials(req *http.Request) {
	if req == nil || req.URL == nil || credentialOrigin(req.URL.String(), c.BaseURL) {
		return
	}
	req.Header.Del("Authorization")
	if c.Config != nil {
		for k := range c.Config.Headers {
			req.Header.Del(k)
		}
	}
}
