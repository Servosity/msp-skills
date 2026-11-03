// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: pinned by handfixes.json "mcp-http-token-ignores-mcpb-placeholder".

package cliutil

import "testing"

// Codex 2026-11-02 r1: an unresolved MCPB placeholder padded with whitespace is
// still a public string, never a value.
func TestEffectiveEnvIgnoresPaddedPlaceholder(t *testing.T) {
	for _, v := range []string{"${user_config.x}", " ${user_config.x} ", "\t${user_config.x}\n"} {
		if got := EffectiveEnv(v); got != "" {
			t.Errorf("EffectiveEnv(%q) = %q, want unset", v, got)
		}
	}
	if got := EffectiveEnv(" real-value "); got != " real-value " {
		t.Errorf("a real value changed: %q", got)
	}
}
