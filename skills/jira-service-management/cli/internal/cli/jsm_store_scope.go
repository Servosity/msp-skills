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
func init() {
	config.StoreScopeIdentity = jsmStoreScopeIdentity
}

func jsmStoreScopeIdentity(c *config.Config) string {
	auth := c.AuthHeader()
	if auth == "" {
		return ""
	}
	return auth + "|site=" + client.ResolvedBaseURL(c.BaseURL, c.TemplateVars)
}
