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
// ends, or -1 when there is none ('@' in a path or query is not userinfo).
func userinfoEnd(s string) (int, int) {
	start := 0
	if i := strings.Index(s, "://"); i >= 0 {
		start = i + 3
	}
	end := len(s)
	if i := strings.IndexAny(s[start:], "/?#"); i >= 0 {
		end = start + i
	}
	at := strings.LastIndex(s[start:end], "@")
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

// hostAgrees: a base_url saved in config.toml names the same host as
// PROXMOX_HOST, or the run must not start.
func hostAgrees(baseURL, hostEnv string) error {
	saved := ""
	if u, err := url.Parse(baseURL); err == nil {
		saved = u.Hostname()
	}
	want := normalizeEndpointTemplateValue(hostEnv)
	if h, _, err := net.SplitHostPort(want); err == nil {
		want = h
	}
	want = strings.Trim(want, "[]")
	if saved != "" && strings.EqualFold(saved, want) {
		return nil
	}
	return fmt.Errorf("PROXMOX_HOST is %s but base_url in config.toml points at %s; a token would go to the wrong host. Remove base_url from config.toml, or unset PROXMOX_HOST", want, RedactUserinfo(baseURL))
}
