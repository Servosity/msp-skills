// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/synology/handfixes.json
// under the "mcp-http-token-placeholder" entry.

package dsm

import "testing"

// TestEnvTLSIgnoresUnresolvedPlaceholders: the .mcpb manifest maps
// SYNOLOGY_TLS_FINGERPRINT and SYNOLOGY_SKIP_TLS_VERIFY to optional
// ${user_config...} prompts; a host that leaves the placeholder must not turn
// it into a certificate pin that refuses every NAS.
func TestEnvTLSIgnoresUnresolvedPlaceholders(t *testing.T) {
	t.Setenv("SYNOLOGY_TLS_FINGERPRINT", "${user_config.synology_tls_fingerprint}")
	t.Setenv("SYNOLOGY_SKIP_TLS_VERIFY", "${user_config.synology_skip_tls_verify}")
	if pin, skip := EnvTLS(); pin != "" || skip {
		t.Fatalf("placeholders read as pin=%q skip=%v", pin, skip)
	}
	t.Setenv("SYNOLOGY_TLS_FINGERPRINT", "ab:cd")
	t.Setenv("SYNOLOGY_SKIP_TLS_VERIFY", "1")
	if pin, skip := EnvTLS(); pin != "ab:cd" || !skip {
		t.Fatalf("real values lost: pin=%q skip=%v", pin, skip)
	}
}
