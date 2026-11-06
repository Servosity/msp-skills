// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

// Moved verbatim from the generated helpers.go (tools/apply_edits.py): the allowlisted paginatedGet calls it,
// and the press dogfood dead-helper scan does not follow calls out of allowlisted helpers.

import "encoding/json"

func isDryRunResponseForClient(c any, data json.RawMessage) bool {
	dryRunClient, ok := c.(interface{ IsDryRun() bool })
	return ok && isDryRunResponse(dryRunClient.IsDryRun(), data)
}
