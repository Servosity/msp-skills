// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

// jsmDeleteBodyEndpoints are the DELETE operations whose official spec
// declares a JSON request body (the accountIds / organizationId to remove).
// The generated execute path sent every DELETE parameter as a query string,
// so these four could never succeed through MCP; the CLI already sends the body.
var jsmDeleteBodyEndpoints = map[string]bool{
	"organization.user.remove":        true, // removeUsersFromOrganization
	"request.participant.remove":      true, // removeRequestParticipants
	"servicedesk.customer.remove":     true, // removeCustomers
	"servicedesk.organization.remove": true, // removeOrganization
}
