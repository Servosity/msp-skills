// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/servosity/handfixes.json under
// the "secret-family-never-persisted" entry. See docs/reprint-survival.md.

package mcp

// codeOrchSecretReads are GET endpoints whose response is a credential: backup
// encryption keys, stored credential passwords, MFA codes, DRaaS passwords,
// agent and install tokens, upload and download tokens, the Postmark API token
// and backup login/config secrets. governance.md puts these reads in the
// human-in-the-loop tier, so over MCP they go through servosity-msp_execute:
// a preview first, the value only with confirm: true, and refused outright
// when SERVOSITY_MSP_MCP_READ_ONLY=1. servosity-msp_read rejects them.
var codeOrchSecretReads = map[string]bool{
	"/credentials/":                                          true,
	"/credentials/{id}/":                                     true,
	"/credentials/{id}/versions/":                            true,
	"/backups/mfa_codes/":                                    true,
	"/current-user/mfa-backup-codes/":                        true,
	"/current-user/start-mfa/":                               true,
	"/backups/{id}/encryption-key/":                          true,
	"/dr-backups/{id}/encryption-key/":                       true,
	"/restic-backups/{id}/encryption-key/":                   true,
	"/backups/{id}/encryption-key-versions/":                 true,
	"/dr-backups/{id}/encryption-key-versions/":              true,
	"/restic-backups/{id}/encryption-key-versions/":          true,
	"/companies/{id}/draas/extra-instance-password/":         true,
	"/companies/{id}/draas/windows-helper-password/":         true,
	"/companies/{id}/draas/client-vpn-config/":               true,
	"/restic-backups/{id}/restic-env/":                       true,
	"/dr-backups/{id}/agent-token/":                          true,
	"/restic-backups/{id}/agent-token/":                      true,
	"/resellers/{id}/agent-install-token/":                   true,
	"/companies/{id}/log-upload/":                            true,
	"/companies/{id}/other-info/":                            true,
	"/companies/{id}/servosity-one/download/windows/latest/": true,
	"/backups/{id}/config.ini/":                              true,
	"/backups/{id}/login/":                                   true,
}

func init() {
	for i := range codeOrchEndpoints {
		ep := &codeOrchEndpoints[i]
		if codeOrchNormalizeMethod(ep.Method) == "GET" && codeOrchSecretReads[ep.Path] {
			ep.NeedsConfirm = true
		}
	}
}
