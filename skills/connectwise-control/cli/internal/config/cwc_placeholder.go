// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

// Hand-written (NOT generated): handfixes.json no-credential-to-placeholder-host.
//
// The base URL this connector ships with (Load's default) is a placeholder:
// every ConnectWise Control instance has its own host, and this one is a name
// on ConnectWise's public cloud domain that a stranger can own. A credential
// sent there goes to whoever holds that instance name. So no request is sent
// to it, whatever supplied it: the built-in default, a config file (default,
// --config, --home or a relocation variable), CONNECTWISE_CONTROL_BASE_URL, or
// the MCP server's environment. client.doInternal refuses before it builds a
// request and the origin pin (internal/cli/cwc_guard.go) refuses at the
// transport; the config layer never writes the placeholder to disk, and
// doctor reports it as not configured.

package config

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// ShippedPlaceholderHost is the host of the placeholder base URL this
// connector ships with. It is no instance.
const ShippedPlaceholderHost = "your-instance.screenconnect.com"

// ErrPlaceholderBaseURL marks a request refused because no instance is
// configured: the base URL is empty or still the shipped placeholder.
var ErrPlaceholderBaseURL = errors.New("no ConnectWise Control instance is configured, so nothing was sent")

// placeholderRemedy names the variable and the config key that fix it. It
// never repeats the configured value: a base URL can carry userinfo.
const placeholderRemedy = "set CONNECTWISE_CONTROL_BASE_URL (or base_url in config.toml) to your own instance's URL, the address you open the ConnectWise Control web app at, then run connectwise-control-cli doctor"

// IsPlaceholderBaseURL reports whether raw names the shipped placeholder host
// in any spelling a request could still reach it by: any letter case, a
// trailing dot, any port, userinfo, a path or query, either scheme, or no
// scheme. It reads the host both the way net/url does (what the transport
// dials) and by plain splitting (a value net/url refuses), and refuses if
// either names the placeholder.
func IsPlaceholderBaseURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	if u, err := url.Parse(raw); err == nil && u.Host != "" && IsPlaceholderHost(u.Hostname()) {
		return true
	}
	return IsPlaceholderHost(splitHost(raw))
}

// IsPlaceholderHost reports whether host, a URL host with or without a port
// or a trailing dot, is the shipped placeholder's.
func IsPlaceholderHost(host string) bool {
	host = strings.TrimSpace(host)
	if i := strings.LastIndexByte(host, ':'); i >= 0 && !strings.Contains(host[:i], ":") && !strings.HasPrefix(host, "[") {
		host = host[:i]
	}
	host = strings.TrimRight(host, ".")
	return host != "" && foldASCII(host) == ShippedPlaceholderHost
}

// CheckBaseURL returns nil when base names an instance and otherwise an error
// wrapping ErrPlaceholderBaseURL that says how to configure one.
func CheckBaseURL(base string) error {
	switch {
	case strings.TrimSpace(base) == "":
		return fmt.Errorf("%w: the base URL is empty; %s", ErrPlaceholderBaseURL, placeholderRemedy)
	case IsPlaceholderBaseURL(base):
		return placeholderRefusal()
	}
	return nil
}

// CheckRequestURL refuses a request whose URL names the placeholder host.
func CheckRequestURL(u *url.URL) error {
	if u != nil && IsPlaceholderHost(u.Host) {
		return placeholderRefusal()
	}
	return nil
}

func placeholderRefusal() error {
	return fmt.Errorf("%w: the base URL is still the shipped placeholder https://%s, a name on ConnectWise's public cloud domain that is not your instance; %s",
		ErrPlaceholderBaseURL, ShippedPlaceholderHost, placeholderRemedy)
}

// splitHost is the host[:port] of raw by plain splitting: after any scheme,
// before the first path, query or fragment mark, after the last '@'.
func splitHost(raw string) string {
	s := raw
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	} else {
		s = strings.TrimLeft(s, "/\\")
	}
	if i := strings.IndexAny(s, "/\\?#"); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndexByte(s, '@'); i >= 0 {
		s = s[i+1:]
	}
	return s
}

// foldASCII lowercases ASCII letters only. A non-ASCII byte stays as it is,
// so no Unicode spelling compares equal here; the origin pin refuses every
// non-ASCII host on its own.
func foldASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if 'A' <= c && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}
