// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored (not generated): every GET (and HEAD) declared state-changing in
// cliutil.MutatingGETPaths is a write to the MCP executor, so immybot_read
// refuses it, the read-only switch refuses it and immybot_execute previews it
// until confirm is true. See handfixes.json: mutating-gets-are-writes.

package mcp

import "immybot-pp-cli/internal/cliutil"

func init() {
	for i := range codeOrchEndpoints {
		ep := &codeOrchEndpoints[i]
		if m := codeOrchNormalizeMethod(ep.Method); (m == "GET" || m == "HEAD") && cliutil.IsMutatingGETPath(ep.Path) {
			ep.Mutating, ep.NeedsConfirm = true, true
		}
	}
}
