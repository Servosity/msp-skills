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
// ends, or -1 when there is none. It runs to the LAST '@': a password may hold
// '/', '?' or '#', and Proxmox endpoints carry no '@' in a path.
func userinfoEnd(s string) (int, int) {
	start := 0
	if i := strings.Index(s, "://"); i >= 0 {
		start = i + 3
	} else if strings.HasPrefix(s, "//") {
		start = 2
	}
	at := strings.LastIndex(s[start:], "@")
	if at < 0 {
		return -1, -1
	}
	return start, start + at
}

// StripUserinfo drops a user:password@ part from an endpoint.
func StripUserinfo(s string) string {
	start, at := userinfoEnd(s)
	if at < 0 {
		return s
	}
	return s[:start] + s[at+1:]
}

// RedactUserinfo replaces a user:password@ part of an endpoint for display.
func RedactUserinfo(s string) string {
	start, at := userinfoEnd(s)
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
