// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"net/url"
	"strings"
	"testing"
)

// TestTemplateValueCannotMoveTheRequestHost: a {subdomain} value from any
// source (env, config, sync --path-context from an MCP call) is one DNS label,
// so the request, and the bearer token on it, stays on <subdomain>.immy.bot.
func TestTemplateValueCannotMoveTheRequestHost(t *testing.T) {
	const base = "https://{subdomain}.immy.bot"
	for _, bad := range []string{
		"evil.example/x?", "evil.example#", "evil.example", "user@evil.example", "evil.example:443",
		"127.0.0.1:9/x?", "acme/../x", "acme\\x", "acme%2F", "acme x", "-acme", "acme-", strings.Repeat("a", 64),
	} {
		got, err := buildURL(base, "/api/v1/tenants", map[string]string{"subdomain": bad})
		if err == nil {
			t.Errorf("subdomain %q accepted: %s", bad, got)
		}
	}
	for _, good := range []string{"acme", "Acme-01", "a", strings.Repeat("a", 63)} {
		got, err := buildURL(base, "/api/v1/tenants", map[string]string{"subdomain": good})
		if err != nil {
			t.Fatalf("subdomain %q refused: %v", good, err)
		}
		if u, _ := url.Parse(got); u.Hostname() != strings.ToLower(good)+".immy.bot" && u.Hostname() != good+".immy.bot" {
			t.Errorf("subdomain %q gave host %q", good, u.Hostname())
		}
	}
	if got := ResolvedBaseURL(base, map[string]string{"subdomain": "evil.example/x?"}); got != base {
		t.Errorf("ResolvedBaseURL resolved a host-moving value: %s", got)
	}
}

// TestPathTemplateValueStaysOneSegment: a value placed in the request path
// is escaped, so it cannot add segments or a query.
func TestPathTemplateValueStaysOneSegment(t *testing.T) {
	got, err := buildURL("https://acme.immy.bot", "/api/v1/{tenantId}/x", map[string]string{"tenantId": "1/../../admin?x=1"})
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if u.RawQuery != "" || strings.Count(u.EscapedPath(), "/") != 4 {
		t.Errorf("path value escaped its segment: %s", got)
	}
}
