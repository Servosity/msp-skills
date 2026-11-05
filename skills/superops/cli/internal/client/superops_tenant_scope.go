// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): exposes the SuperOps tenant to the generated
// sync checkpoint scope (superops-tenant-scope).

package client

import "superops-pp-cli/internal/config"

// CurrentTemplateVars is what the generated syncCheckpointScope reads to tell
// one tenant's checkpoint from another's. SuperOps has no base-URL template
// placeholders; its tenant is the CustomerSubDomain header and the host.
func (c *Client) CurrentTemplateVars() map[string]string {
	if c == nil || c.Config == nil {
		return nil
	}
	return map[string]string{config.SuperOpsTenantScopeVar: c.Config.SuperOpsTenant()}
}

// superOpsTenantHeader reports the header that picks the SuperOps tenant
// (CustomerSubDomain, folded). The generated cache key folds every
// tenant-selecting header so two tenants never share a cached GET body; the
// generic rules (an "id" or "filter" suffix, a few fixed names) do not match
// this one, so one token at two subdomains shared every cached response.
func superOpsTenantHeader(folded string) bool {
	return folded == "customersubdomain"
}
