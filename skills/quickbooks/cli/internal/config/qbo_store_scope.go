// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package config

import (
	"net/url"
	"strings"
)

// StoreScopeCompany names the local store for QuickBooks: the company, which
// is the Intuit host (production or sandbox) plus the realm path of the base
// URL. Intuit access tokens expire after an hour and every refresh returns a
// new one, so the credential never names the store. A base URL that names no
// company (a placeholder, no /v3/company/<realm> path, or unparseable)
// returns "": no scope, so the store is the unclaimed
// data.db every released version used, and the next run that knows the
// company claims it. Recorded hand-fix `qbo-store-scope-company`.
func StoreScopeCompany(c *Config) string {
	if c == nil {
		return ""
	}
	raw := strings.TrimSpace(c.BaseURL)
	if raw == "" || strings.Contains(strings.ToUpper(raw), "YOUR_REALM_ID") {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || !qboNamesARealm(u.EscapedPath()) {
		return ""
	}
	return "qbo_company=" + strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host) + strings.TrimRight(u.EscapedPath(), "/")
}

// qboNamesARealm reports whether path names a company: a /v3/company/<realm>
// segment with a realm in it, as every QBO Accounting API base URL has.
func qboNamesARealm(path string) bool {
	seg := strings.Split(strings.Trim(path, "/"), "/")
	for i := 0; i+2 < len(seg); i++ {
		if strings.EqualFold(seg[i], "v3") && strings.EqualFold(seg[i+1], "company") && seg[i+2] != "" {
			return true
		}
	}
	return false
}
