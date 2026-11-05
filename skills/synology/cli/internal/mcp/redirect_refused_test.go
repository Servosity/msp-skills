// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// HAND-AUTHORED.

package mcp

import (
	"strings"
	"testing"

	"synology-pp-cli/internal/cliutil"
	"synology-pp-cli/internal/cliutil/testenv"
	"synology-pp-cli/internal/dsm/dsmtest"

	"github.com/mark3labs/mcp-go/server"
)

// TestMCPReadToolNeverFollowsARedirect: with SYNOLOGY_MCP_READ_ONLY=1 the
// read-only abb_tasks tool, answered by the NAS with a same-origin
// 302 Location: /webapi/entry.cgi?api=SYNO.ActiveBackup.Task&method=backup,
// used to follow it with the session and start a backup. No DSM request
// follows a redirect: the result is an error naming the status only.
func TestMCPReadToolNeverFollowsARedirect(t *testing.T) {
	nas := dsmtest.NewRedirectingNAS(t, nil)
	testenv.Isolate(t, cliutil.ConfigDir, cliutil.DataDir, cliutil.StateDir, cliutil.CacheDir)
	t.Setenv("SYNOLOGY_NO_LEARN", "1")
	t.Setenv("SYNOLOGY_ACCOUNT", "msp-admin")
	t.Setenv("SYNOLOGY_PASSWORD", "s3cretpw")
	t.Setenv("SYNOLOGY_MCP_READ_ONLY", "1")
	t.Setenv("SYNOLOGY_BASE_URL", nas.URL)
	s := server.NewMCPServer("synology", "test")
	RegisterTools(s)
	text, isErr := callTool(t, s, "abb_tasks", map[string]any{})
	if n := nas.Count("method=backup"); n != 0 {
		t.Fatalf("read-only abb_tasks followed a redirect into a backup start: %v", nas.Requests())
	}
	if !isErr || !strings.Contains(text, "status 302") {
		t.Fatalf("abb_tasks: isErr=%v text=%s, want an error naming status 302", isErr, text)
	}
	if strings.Contains(text, "method=backup") || strings.Contains(text, "SID1") {
		t.Fatalf("the result quoted the Location: %s", text)
	}
}
