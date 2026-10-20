// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/backup-radar/handfixes.json
// under the "mcp-read-only-accepts-true-yes" entry.

package cliutil

import "testing"

func TestMCPReadOnlyEnvAcceptsTruthyValues(t *testing.T) {
	for _, v := range []string{"1", "true", "TRUE", "yes", " Yes ", "on"} {
		t.Setenv(MCPReadOnlyEnvVar, v)
		if !IsMCPReadOnlyEnv() {
			t.Fatalf("%q must enable read-only mode", v)
		}
	}
	for _, v := range []string{"", "0", "false", "no", "${user_config.backup_radar_mcp_read_only}"} {
		t.Setenv(MCPReadOnlyEnvVar, v)
		if IsMCPReadOnlyEnv() {
			t.Fatalf("%q must not enable read-only mode", v)
		}
	}
}
