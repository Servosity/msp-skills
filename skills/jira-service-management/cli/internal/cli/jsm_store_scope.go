// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"jira-service-management-pp-cli/internal/client"
	"jira-service-management-pp-cli/internal/config"
)

// One Atlassian API token reaches every site its account belongs to, which is
// normal for an MSP working several client sites. The generated store scope
// hashes only the credential, so switching JIRA_SERVICE_MANAGEMENT_SITE reused
// one local database: sites mixed, and a --full sync of one pruned the other's
// rows. Scope by credential AND resolved site URL. The MCP server resolves its
// store path through this package too, so both binaries agree.
//
// A store named by the credential alone belongs to no site in particular, so
// no site adopts one: the first site to run would take another client's rows.
func init() {
	config.StoreScopeIdentity = jsmStoreScopeIdentity
	config.StoreScopeLegacyIdentity = func(*config.Config) string { return "" }
}

// jsmNoUnscopedClaim: an unscoped data.db with no .owner marker predates the
// site scope, so no site claims it (claimLegacyUnscopedDB). A marker a site
// already wrote is still honoured.
var jsmNoUnscopedClaim = true

func jsmStoreScopeIdentity(c *config.Config) string {
	auth := c.AuthHeader()
	if auth == "" {
		return ""
	}
	return auth + "|site=" + client.ResolvedBaseURL(c.BaseURL, c.TemplateVars)
}
