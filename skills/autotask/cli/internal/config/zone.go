// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Autotask zone helpers, hand-authored alongside the generated config.go.
// Autotask's REST API lives on a tenant-specific numbered zone
// (https://webservicesN.autotask.net/atservicesrest/V1.0/); these helpers
// normalize and persist the discovered zone so every command targets the
// correct host. The `zone` command (internal/cli/zone.go) is the discovery
// flow; AUTOTASK_BASE_URL is the env override consumed in Load.
package config

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// zoneHost is an Autotask zone API host (webservices1.autotask.net,
// webservices14.autotask.net, ...). The un-numbered webservices.autotask.net is
// the discovery host, not a zone.
var zoneHost = regexp.MustCompile(`^webservices[0-9]+\.autotask\.net$`)

// ValidateZoneURL accepts only what zoneInformation documents: an https URL on
// a webservicesN.autotask.net host, default port, no userinfo, query or
// fragment, and the /atservicesrest path (with or without /V1.0). autotask
// handfix zone-discovery-pinned: a cached base URL is where every later
// credential is sent, so nothing else is ever saved from discovery.
func ValidateZoneURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("zoneInformation returned an unparseable url; set AUTOTASK_BASE_URL instead")
	}
	path := strings.ToLower(strings.TrimRight(u.Path, "/"))
	if u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		(u.Port() != "" && u.Port() != "443") || !zoneHost.MatchString(strings.ToLower(u.Hostname())) ||
		(path != "/atservicesrest" && path != "/atservicesrest/v1.0") {
		return fmt.Errorf("zoneInformation returned %q, which is not an Autotask zone URL (https://webservicesN.autotask.net/atservicesrest/); nothing was cached; set AUTOTASK_BASE_URL instead", u.Redacted())
	}
	return nil
}

// NormalizeZoneBaseURL turns any of the forms Autotask's zoneInformation
// endpoint or a user might supply into the REST API base the client prepends to
// every path. zoneInformation returns
// "https://webservicesN.autotask.net/atservicesrest/" (no version segment); the
// client's paths are resource-relative ("/Companies/query"), so the cached base
// must end in "/atservicesrest/V1.0". Idempotent: a value already ending in
// /V1.0 is returned unchanged (minus a trailing slash).
func NormalizeZoneBaseURL(raw string) string {
	u := strings.TrimRight(strings.TrimSpace(raw), "/")
	if u == "" {
		return u
	}
	if strings.HasSuffix(u, "/atservicesrest/V1.0") || strings.HasSuffix(u, "/atservicesrest/v1.0") {
		return u
	}
	if strings.HasSuffix(u, "/atservicesrest") {
		return u + "/V1.0"
	}
	return u + "/atservicesrest/V1.0"
}

// SaveBaseURL persists a resolved API base URL (typically the tenant's
// discovered Autotask zone, normalized via NormalizeZoneBaseURL) to the config
// file so later commands use the correct webservicesN host without re-running
// `zone` or setting AUTOTASK_BASE_URL each invocation.
func (c *Config) SaveBaseURL(baseURL string) error {
	c.BaseURL = baseURL
	return c.saveCredentialsThenConfig() // Codex r1 P2: save() alone dropped config-file credentials
}
