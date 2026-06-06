// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0.
//
// Autotask zone helpers, hand-authored alongside the generated config.go.
// Autotask's REST API lives on a tenant-specific numbered zone
// (https://webservicesN.autotask.net/atservicesrest/V1.0/); these helpers
// normalize and persist the discovered zone so every command targets the
// correct host. The `zone` command (internal/cli/zone.go) is the discovery
// flow; AUTOTASK_ZONE_URL is the env override consumed in Load.
package config

import "strings"

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
// `zone` or setting AUTOTASK_ZONE_URL each invocation.
func (c *Config) SaveBaseURL(baseURL string) error {
	c.BaseURL = baseURL
	return c.save()
}
