// Copyright 2026 Damien Stevens and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-written: pinned by skills/sophos-central/handfixes.json under
// "secret-responses-never-persisted".
//
// These operations return a live credential in their response body: an
// endpoint's tamper protection password (current and previous), or a
// migration job's token. Through the MCP execute tool an agent could read
// them, and prompt text in alert or endpoint data could ask it to. They stay
// CLI-only, where a human runs them; governance.md tiers them as Credential.
// Derived from a scan of the spec's 2xx response schemas for password,
// previousPasswords and token keys.

package mcp

var cliOnlySecretEndpoints = map[string]bool{
	"endpoints.get-tamper-protection": true,
	"endpoints.set-tamper-protection": true,
	"migrations.create-receiving-job": true,
	"migrations.create-sending-job":   true,
	"migrations.get-job":              true,
	"migrations.get-jobs":             true,
}

func init() {
	kept := codeOrchEndpoints[:0:0]
	for _, ep := range codeOrchEndpoints {
		if !cliOnlySecretEndpoints[ep.ID] {
			kept = append(kept, ep)
		}
	}
	codeOrchEndpoints = kept
}
