// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

// codeOrchTakesPerPage is hand-wired (handfixes.json:
// meraki-mcp-execute-link-next): only an endpoint that declares perPage can be
// re-requested with a smaller page, so only those get the shrink-perPage note
// when a result is cut to fit the MCP budget.
func codeOrchTakesPerPage(ep *codeOrchEndpoint) bool {
	for _, b := range ep.QueryParams {
		if b.WireName == "perPage" || b.PublicName == "perPage" {
			return true
		}
	}
	return false
}
