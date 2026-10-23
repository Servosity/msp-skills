// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.
//
// Hand-written (NOT generated): pinned by skills/servosity/handfixes.json under
// the "secret-family-never-persisted" entry. See docs/reprint-survival.md.

package cli

import "fmt"

// mfaSecretSyncResources are sync resources that return MFA secrets. The store
// refuses their rows, and two of their GETs can regenerate backup codes or
// start an enrollment on the server, so sync refuses them before any request.
var mfaSecretSyncResources = map[string]bool{
	"backups-mfa-codes":             true,
	"current-user-mfa-backup-codes": true,
	"current-user-start-mfa":        true,
}

func refuseMFASecretSync(resources []string) error {
	for _, r := range resources {
		if mfaSecretSyncResources[r] {
			return fmt.Errorf("%s is not synced: it returns MFA secrets the local store never keeps; read it live with the matching command", r)
		}
	}
	return nil
}
