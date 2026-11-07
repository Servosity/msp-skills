// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"io"
	"net/http"
	"riverside-fm-pp-cli/internal/cliutil"
	"strings"

	"riverside-fm-pp-cli/internal/client"
)

// Riverside refreshes a browser session by sliding it: every response, a 401 or
// 403 included, sets a fresh riverside_auth_access cookie validated against the
// long-lived refresh cookie. The cookie jar keeps that rotation for the next
// request, but the request that came back 401/403 on a cold access cookie has
// already failed. Retrying it once with the rotated cookie succeeds (observed
// live when this connector was first printed), so the first command after the
// access cookie lapses does not fail.
func init() {
	registerClientHook(func(c *client.Client) error {
		if c.HTTPClient != nil {
			c.HTTPClient.Transport = &sessionRetryTransport{base: c.HTTPClient.Transport}
		}
		return nil
	})
}

var rotatedSessionCookies = map[string]bool{"riverside_auth_access": true, "riverside_auth_refresh": true}

type sessionRetryTransport struct{ base http.RoundTripper }

func (t *sessionRetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = cliutil.OutboundGuard(nil)
	}
	resp, err := base.RoundTrip(req)
	if err != nil || (resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden) {
		return resp, err
	}
	// Only a request that already carried a session cookie and can be replayed.
	if req.Header.Get("Cookie") == "" || (req.Body != nil && req.GetBody == nil) {
		return resp, nil
	}
	cookie, rotated := mergeRotatedSessionCookies(req.Header.Get("Cookie"), resp.Cookies())
	if !rotated {
		return resp, nil
	}
	retry := req.Clone(req.Context())
	if req.GetBody != nil {
		body, berr := req.GetBody()
		if berr != nil {
			return resp, nil
		}
		retry.Body = body
	}
	retry.Header.Set("Cookie", cookie)
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	_ = resp.Body.Close()
	return base.RoundTrip(retry)
}

// mergeRotatedSessionCookies folds rotated riverside_auth_* values into a Cookie
// header. Blank, "deleted" and non-JWT values are ignored so a logout cookie
// cannot replace a working session. Reports whether anything changed.
func mergeRotatedSessionCookies(header string, set []*http.Cookie) (string, bool) {
	parts := strings.Split(header, "; ")
	changed := false
	for _, ck := range set {
		if !rotatedSessionCookies[ck.Name] || ck.Value == "" || ck.Value == "deleted" || strings.Count(ck.Value, ".") != 2 {
			continue
		}
		found := false
		for i, p := range parts {
			if strings.HasPrefix(p, ck.Name+"=") {
				found = true
				if p != ck.Name+"="+ck.Value {
					parts[i] = ck.Name + "=" + ck.Value
					changed = true
				}
			}
		}
		if !found {
			parts = append(parts, ck.Name+"="+ck.Value)
			changed = true
		}
	}
	return strings.Join(parts, "; "), changed
}
