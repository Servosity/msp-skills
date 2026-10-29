// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.

package cliutil

// pax8 handfix (network-sinks-withheld): every request this program
// sends to a host other than the pinned Pax8 API host (the feedback endpoint,
// a --deliver webhook) goes through SendOutbound. The caller withholds the
// body (WithholdBytes) before building the request; SendOutbound refuses the
// request when its URL (userinfo, path, query, fragment) or any header value
// repeats a credential this process holds, in any form HeldEchoes finds, and
// never follows a redirect, so neither the address the operator configured nor
// a Location header can carry a held value to that host. A held value split
// across parts of one request (query values, path segments, headers, body
// values, in any order) is refused too (Codex r1 4.34.2 class
// credential-split-across-outbound-parts): the parts are taken apart and
// decoded, and any held value they reassemble is a refusal.

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrOutboundHoldsSecret marks a non-API request refused before it was sent.
var ErrOutboundHoldsSecret = errors.New("request refused: it would repeat a secret value this program holds")

// SendOutbound checks and sends one non-API request built from method,
// rawURL, header and body (taken apart rather than as an *http.Request, which
// static analysis reads as an incoming request). setting names where the
// address came from (an environment variable or flag); the error names it and
// never the value. A 3xx answer is an error naming its Location: redirects
// are not followed.
func SendOutbound(setting, method, rawURL string, header http.Header, body []byte, timeout time.Duration) (*http.Response, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", setting, ErrOutboundHoldsSecret)
	}
	if outboundHoldsSecret(u, header) || outboundReassemblesHeld(outboundParts(u, header, body)) {
		return nil, fmt.Errorf("%w: its URL or a header repeats a credential; check %s", ErrOutboundHoldsSecret, setting)
	}
	req, err := http.NewRequest(method, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("building the request for %s: %w", setting, err)
	}
	for name, values := range header {
		req.Header[name] = append([]string(nil), values...)
	}
	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("%s answered %s (Location: %q); redirects are not followed", setting, resp.Status, resp.Header.Get("Location"))
	}
	return resp, nil
}

// outboundHoldsSecret reads every part of the request a server or a proxy log
// sees besides the body: the whole URL as sent, its decoded path, query and
// fragment, the userinfo, and each header name and value.
func outboundHoldsSecret(u *url.URL, header http.Header) bool {
	if u == nil {
		return true
	}
	parts := []string{u.String(), u.Host, u.Path, u.RawPath, u.RawQuery, u.Fragment, u.RawFragment, u.Opaque}
	if q, err := url.QueryUnescape(u.RawQuery); err == nil {
		parts = append(parts, q)
	}
	if u.User != nil {
		parts = append(parts, u.User.Username())
		if p, ok := u.User.Password(); ok {
			parts = append(parts, p)
		}
	}
	for name, values := range header {
		parts = append(parts, name)
		parts = append(parts, values...)
	}
	for _, p := range parts {
		if p != "" && HeldEchoes(p) {
			return true
		}
	}
	return false
}

// outboundParts is every value a receiver or a proxy log reads from one
// request, decoded: the userinfo, each host label, each path segment, each
// query name and value, the fragment, each header name and value, and the
// body's JSON string values and words.
func outboundParts(u *url.URL, header http.Header, body []byte) []string {
	var parts []string
	add := func(vs ...string) {
		for _, v := range vs {
			if v == "" {
				continue
			}
			parts = append(parts, v)
			if d, err := url.QueryUnescape(v); err == nil && d != v {
				parts = append(parts, d)
			}
			if d, err := url.PathUnescape(v); err == nil && d != v {
				parts = append(parts, d)
			}
		}
	}
	if u != nil {
		if u.User != nil {
			add(u.User.Username())
			if p, ok := u.User.Password(); ok {
				add(p)
			}
		}
		add(strings.Split(u.Hostname(), ".")...)
		add(strings.Split(u.EscapedPath(), "/")...)
		for _, pair := range strings.FieldsFunc(u.RawQuery, func(r rune) bool { return r == '&' || r == ';' }) {
			k, v, _ := strings.Cut(pair, "=")
			add(k, v)
		}
		add(u.Fragment, u.RawFragment, u.Opaque)
	}
	for name, values := range header {
		add(name)
		add(values...)
		for _, v := range values {
			add(strings.Fields(v)...)
		}
	}
	if len(body) > 0 {
		add(jsonStringValueList(string(body))...)
		add(words(string(body))...)
	}
	return parts
}

// outboundReassemblesHeld reports a held value (any of its held forms, of
// heldMatchFloor or more characters) that the parts spell when put together
// in any order: the parts joined (adjacent order, with the funnel's split
// views), and every held value checked for a cover by the parts: the tail of
// one part, then whole parts, then the head of another (each edge at least
// shortMinLen characters), parts reused or not.
func outboundReassemblesHeld(parts []string) bool {
	if len(parts) == 0 {
		return false
	}
	if HeldEchoes(strings.Join(parts, "\n")) {
		return true
	}
	uniq := map[string]bool{}
	var set []string
	for _, p := range parts {
		if !uniq[p] {
			uniq[p] = true
			set = append(set, p)
		}
	}
	for _, h := range heldValues() {
		if len(h) >= heldMatchFloor && coveredByParts(h, set) {
			return true
		}
	}
	return false
}

func coveredByParts(h string, parts []string) bool {
	reach := make([]bool, len(h)+1)
	for _, p := range parts {
		if strings.Contains(p, h) {
			return true
		}
		if strings.HasPrefix(h, p) {
			reach[len(p)] = true
		}
		for k := shortMinLen; k <= len(p) && k < len(h); k++ {
			if strings.HasSuffix(p, h[:k]) {
				reach[k] = true
			}
		}
	}
	for pos := 1; pos < len(h); pos++ {
		if !reach[pos] {
			continue
		}
		rest := h[pos:]
		for _, p := range parts {
			if strings.HasPrefix(rest, p) {
				reach[pos+len(p)] = true
			} else if len(rest) >= shortMinLen && strings.HasPrefix(p, rest) {
				return true
			}
		}
	}
	return reach[len(h)]
}
