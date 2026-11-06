// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import "testing"

// Download output shares restore's funnel, so a right-to-left override in a
// file name cannot flip what the operator reads.
func TestTenantTextDropsBidiControls(t *testing.T) {
	if got := tenantText("‮ppa.exe.zip\x1b[2J"); got != "ppa.exe.zip[2J" {
		t.Fatalf("tenantText = %q", got)
	}
}
