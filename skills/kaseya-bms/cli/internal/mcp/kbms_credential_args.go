// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import "kaseya-bms-pp-cli/internal/cliutil"

// HoldCredentialEndpointArguments holds the params of a code-orchestration
// call (kaseya-bms_execute, kaseya-bms_read, kaseya-bms_get) whose endpoint
// path names a credential, but the allowlisted identifiers (kaseya-bms handfix
// credential-endpoint-request-held; Codex review r2 class holder incomplete:
// the execute preview of security.authenticate returned the OAuth
// authorization code "Code" verbatim, because the preview is built from the
// tool arguments before any request reaches the API client). The MCP server's
// result filter then withholds them in the preview.
func HoldCredentialEndpointArguments(args map[string]any) {
	id, _ := args["endpoint_id"].(string)
	if id == "" {
		return
	}
	ep := findCodeOrchEndpoint(id)
	if ep == nil || !cliutil.CredentialPath(ep.Path) {
		return
	}
	if params, ok := args["params"].(map[string]any); ok {
		cliutil.HoldCredentialEndpointValues(params)
	}
}
