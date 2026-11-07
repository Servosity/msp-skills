// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-authored (not generator output); pinned by skills/eset-protect/handfixes.json
// under "mcp-post-reads-are-reads".

package mcp

// esetPOSTReadNeedsConfirm is the confirm setting of the three ESET Connect
// POST reads (detection-groups search, detections v2 batch get, users batch
// get). The press marks every POST as needing confirm; these change nothing,
// so eset-protect_read serves them, including in MCP read-only mode.
const esetPOSTReadNeedsConfirm = false
