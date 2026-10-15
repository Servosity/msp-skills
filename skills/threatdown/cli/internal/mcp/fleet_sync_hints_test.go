// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"strings"
	"testing"
)

// Review rounds 13-14 + Codex rounds 10-11 (2026-10-14): endpoints,
// detections and jobs come only from fleet sync, so every store recovery
// hint must name it; generic sync alone leaves an agent with no fleet data.
func TestStoreHintsNameFleetSync(t *testing.T) {
	for name, msg := range map[string]string{
		"missing": mcpMissingStoreMessage("/tmp/data.db"),
		"empty":   mcpEmptyStoreNextStep(),
		"partial": mcpPartialStoreNextStep(),
	} {
		if !strings.Contains(msg, "fleet sync") {
			t.Errorf("%s-store hint must name fleet sync: %q", name, msg)
		}
	}
}
