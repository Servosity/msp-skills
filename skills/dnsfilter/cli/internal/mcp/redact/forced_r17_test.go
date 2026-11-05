// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/dnsfilter/handfixes.json
// under the "mcp-read-only-hinted-tools-filter-credentials" entry.

package redact

import (
	"context"
	"strings"
	"testing"

	"dnsfilter-pp-cli/internal/cliutil"
)

// TestAlwaysEntryPointsIgnoreTheSwitch: off the read-only switch Raw and
// Final stay no-ops (Enabled() keeps its meaning), while RawAlways and
// FinalAlways apply the same rules the switch does.
func TestAlwaysEntryPointsIgnoreTheSwitch(t *testing.T) {
	t.Setenv(cliutil.MCPReadOnlyEnvVar, "")
	body := `Error: GET /v1/organizations/1 returned HTTP 422: {"error": "request failed", "secret_key": "SENTINEL-FORCED-KEY"}`
	if Raw(body) != body || Final(body) != body {
		t.Fatal("Raw/Final changed text with the switch off")
	}
	for name, got := range map[string]string{"RawAlways": RawAlways(body), "FinalAlways": FinalAlways(body)} {
		if strings.Contains(got, "SENTINEL-FORCED-KEY") || !strings.Contains(got, "request failed") || !strings.Contains(got, WithheldFieldPlaceholder) {
			t.Fatalf("%s did not redact the field: %q", name, got)
		}
	}
	if got := FinalAlways("Uninstall PIN: SENTINEL-FORCED-PIN"); got != WithheldHintedBlockMessage {
		t.Fatalf("FinalAlways kept a credential pair: %q", got)
	}
	if !IsWithheldBlock(WithheldBlockMessage) || !IsWithheldBlock(WithheldHintedBlockMessage) || IsWithheldBlock("ok") {
		t.Fatal("IsWithheldBlock does not recognize the two block messages")
	}
	if strings.Contains(WithheldHintedBlockMessage, cliutil.MCPReadOnlyEnvVar+" is set") {
		t.Fatal("the default-mode message claims the switch is set")
	}

	t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
	if got := Final("Uninstall PIN: SENTINEL-FORCED-PIN"); got != WithheldBlockMessage {
		t.Fatalf("the switch's message changed: %q", got)
	}
}

// TestForcedContextMark: WithForced marks a context, Forced reads it, and an
// unmarked context is not forced.
func TestForcedContextMark(t *testing.T) {
	ctx := context.Background()
	if Forced(ctx) {
		t.Fatal("background context reads as forced")
	}
	if !Forced(WithForced(ctx)) {
		t.Fatal("WithForced did not mark the context")
	}
	type other struct{}
	if !Forced(context.WithValue(WithForced(ctx), other{}, 1)) {
		t.Fatal("a derived context lost the mark")
	}
}
