// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"strings"
	"sync"
	"time"

	"cisco-umbrella-pp-cli/internal/config"
)

// OrgHeader is the token-request header Umbrella reads to scope an access
// token to one child (customer) org of a Multi-org or MSP console.
const OrgHeader = "X-Umbrella-OrgId"

// childOrgs maps a client's *config.Config to the child org its tokens are
// scoped to. Keyed by config so concurrent per-org clients stay independent.
var childOrgs sync.Map

// ScopeToChildOrg makes every token this client mints carry X-Umbrella-OrgId.
// It drops any token loaded from disk (that one belongs to the parent console)
// and disables the response cache so one org's reads are never served to
// another.
func ScopeToChildOrg(c *Client, org string) {
	if c == nil || c.Config == nil || org == "" {
		return
	}
	childOrgs.Store(c.Config, org)
	// A configured static Authorization header would override the minted child
	// bearer on every request; drop it for this process (the file is untouched).
	if len(c.Config.Headers) > 0 {
		headers := make(map[string]string, len(c.Config.Headers))
		for k, v := range c.Config.Headers {
			if !strings.EqualFold(strings.TrimSpace(k), "Authorization") {
				headers[k] = v
			}
		}
		c.Config.Headers = headers
	}
	c.Config.AuthHeaderVal = ""
	c.Config.AccessToken = ""
	c.Config.TokenExpiry = time.Time{}
	c.NoCache = true // ponytail: no per-org cache; add an org-keyed cache dir if child-org repeat reads get hot
}

// ChildOrgOf returns the child org a client is scoped to, or "".
func ChildOrgOf(c *Client) string {
	if c == nil {
		return ""
	}
	return childOrgOf(c.Config)
}

func childOrgOf(cfg *config.Config) string {
	if cfg == nil {
		return ""
	}
	if v, ok := childOrgs.Load(cfg); ok {
		return v.(string)
	}
	return ""
}
