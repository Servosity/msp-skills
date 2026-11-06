// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

// threecx handfix (mcp-mint-gets-confirm): 3CX exposes functions that mint or
// grant a credential, or allocate an object, as plain GETs (GenerateApiKey,
// GetRequestHelpLink grants vendor remote access, InitTrunk). The generated table
// reads them as reads, so threecx_read would run them with no confirm and under
// the read-only switch. The rule is fleet exec's (deniedXAPIPath), minus the file
// streams. Kept out of code_orch.go so that template file stays regenerable.

import "threecx-pp-cli/internal/cliutil"

func init() {
	for i := range codeOrchEndpoints {
		ep := &codeOrchEndpoints[i]
		if ep.Method == "GET" && cliutil.StateChangingGET(ep.Path) {
			ep.NeedsConfirm, ep.Mutating = true, true
		}
	}
}
