// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"fmt"
	"strings"

	"eset-protect-pp-cli/internal/cliutil"
	"eset-protect-pp-cli/internal/mcp/cobratree"
	"eset-protect-pp-cli/internal/store"
	mcplib "github.com/mark3labs/mcp-go/mcp"
)

// esetReadOnlyMirrorRefusal keeps the sql and search tools from serving a
// Credential-tier value while ESET_PROTECT_MCP_READ_ONLY=1. Since
// credential-tier-never-persisted the store writes every installer row with
// downloadUrl replaced by "[redacted]", so a mirror this build wrote never
// trips it, however often `installers list` or `sync --resources installers`
// ran. It refuses only a mirror an earlier build wrote, whose installers rows
// still hold a real link (handfixes.json: mcp-read-only-refuses-credential-tier).
// An unreadable check refuses too, so a broken mirror never fails open.
func esetReadOnlyMirrorRefusal(db *store.Store, path string) *mcplib.CallToolResult {
	if db == nil || !cliutil.IsMCPReadOnlyEnv() {
		return nil
	}
	names := cobratree.CredentialTierResourceNames()
	if len(names) == 0 {
		return nil
	}
	args := make([]any, len(names))
	for i, n := range names {
		args[i] = n
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(names)), ",")
	rows, err := db.DB().Query(`SELECT resource_type, data FROM resources WHERE resource_type IN (`+placeholders+`)`, args...)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "no such table") {
			return nil
		}
		return esetMirrorCheckFailed(path, err)
	}
	defer rows.Close()
	found := ""
	for rows.Next() {
		var resourceType, data string
		if err := rows.Scan(&resourceType, &data); err != nil {
			return esetMirrorCheckFailed(path, err)
		}
		if cliutil.HoldsPersistCredential(resourceType, []byte(data)) {
			found = resourceType
			break
		}
	}
	if err := rows.Err(); err != nil {
		return esetMirrorCheckFailed(path, err)
	}
	if found == "" {
		return nil
	}
	return mcplib.NewToolResultError(fmt.Sprintf("refusing sql and search while %s=1: the local mirror %s holds %s rows with a real download link, which is Credential tier (installer download links install and enroll a device into the tenant). An earlier build of %s wrote them; this build stores the link as %q. Delete that file and run '%s sync' to rebuild the mirror", cliutil.MCPReadOnlyEnvVar, path, found, "eset-protect-cli", cliutil.PersistRedactedValue, "eset-protect-cli"))
}

func esetMirrorCheckFailed(path string, err error) *mcplib.CallToolResult {
	return mcplib.NewToolResultError(fmt.Sprintf("refusing while %s=1: could not check the local mirror %s for Credential-tier rows: %v", cliutil.MCPReadOnlyEnvVar, path, err))
}
