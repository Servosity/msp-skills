// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: Proxmox API tokens never ride in an endpoint's userinfo, so a
// user:password@ part pasted into a host or URL is dropped at load and
// redacted wherever an endpoint is printed (errors, listings, MCP results).

package config

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// userinfoEnd returns where a user:password@ part of an endpoint starts and
// ends, or -1 when there is none. It runs to the LAST '@' (a password may
// hold '?' or '#'). strict (for stripping, which decides where a request
// goes): the part may hold no '/', so an '@' after a '/' (a path, or a port
// then a path) never moves the host. Loose (for redacting what is printed):
// any '@' after the scheme.
func userinfoEnd(s string, strict bool) (int, int) {
	start := 0
	if i := strings.Index(s, "://"); i > 0 && validScheme(s[:i]) {
		start = i + 3
	} else if strings.HasPrefix(s, "//") {
		start = 2
	}
	at := strings.LastIndex(s[start:], "@")
	if at < 0 || (strict && strings.Contains(s[start:start+at], "/")) {
		return -1, -1
	}
	return start, start + at
}

// AmbiguousUserinfo: an '@' after a '/' could be a password holding '/' or a
// path holding '@'; no request may be sent to such an endpoint.
func AmbiguousUserinfo(s string) bool {
	_, loose := userinfoEnd(s, false)
	_, strict := userinfoEnd(s, true)
	return loose >= 0 && strict < 0
}

func validScheme(s string) bool {
	for i, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || i > 0 && (r >= '0' && r <= '9' || r == '+' || r == '-' || r == '.')) {
			return false
		}
	}
	return s != ""
}

// StripUserinfo drops a user:password@ part from an endpoint.
func StripUserinfo(s string) string {
	start, at := userinfoEnd(s, true)
	if at < 0 {
		return s
	}
	return s[:start] + s[at+1:]
}

// RedactUserinfo replaces a user:password@ part of an endpoint for display.
func RedactUserinfo(s string) string {
	start, at := userinfoEnd(s, false)
	if at < 0 {
		return s
	}
	return s[:start] + "REDACTED@" + s[at+1:]
}

// hostAgrees: a base_url saved in config.toml names the same host and port
// as PROXMOX_HOST (default port 8006), or a PVE request must not be sent.
func hostAgrees(baseURL, hostEnv string) error {
	savedHost, savedPort := "", ""
	if u, err := url.Parse(baseURL); err == nil {
		savedHost, savedPort = u.Hostname(), u.Port()
		if savedPort == "" {
			savedPort = map[string]string{"https": "443", "http": "80"}[strings.ToLower(u.Scheme)]
		}
	}
	want, wantPort := normalizeEndpointTemplateValue(hostEnv), "8006"
	if h, p, err := net.SplitHostPort(want); err == nil {
		want, wantPort = h, p
	}
	want = strings.Trim(want, "[]")
	norm := func(h string) string { return strings.TrimSuffix(strings.ToLower(h), ".") }
	if savedHost != "" && norm(savedHost) == norm(want) && savedPort == wantPort {
		return nil
	}
	return fmt.Errorf("PROXMOX_HOST is %s but base_url in config.toml points at %s; a token would go to the wrong host. Remove base_url from config.toml, or set PROXMOX_HOST to its host:port", net.JoinHostPort(want, wantPort), RedactUserinfo(baseURL))
}
