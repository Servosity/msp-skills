// Copyright 2026 Servosity Inc. and msp-skills contributors. Licensed under Apache-2.0. See LICENSE.

package mcp

import (
	"testing"

	"comet-backup-pp-cli/internal/cliutil"
)

// Every Comet admin call is a POST, so the method says nothing: the spec's
// x-pp-mutation flag marks the calls that change state on the Comet Server or
// a customer device (cancel, abandon, run, restore, retention, uninstall, user
// writes, vault requests). comet-backup_execute must preview each until
// confirm is boolean true, and comet-backup_read must refuse it
// (comet-backup handfix: mcp-mirror-write-confirm). The credential-tier reads
// (responses carrying storage keys, password hashes or signing secrets) are
// gated the same way (comet-backup handfix: credential-tier-read-confirm).
// Every other read runs without confirm.
func TestEveryStateChangingEndpointNeedsConfirm(t *testing.T) {
	writes := map[string]bool{
		"/api/v1/admin/add-user":                            true,
		"/api/v1/admin/delete-user":                         true,
		"/api/v1/admin/disable-user-totp":                   true,
		"/api/v1/admin/dispatcher/apply-retention-rules":    true,
		"/api/v1/admin/dispatcher/deepverify-storage-vault": true,
		"/api/v1/admin/dispatcher/drop-connection":          true,
		"/api/v1/admin/dispatcher/kill-process":             true,
		"/api/v1/admin/dispatcher/refetch-profile":          true,
		"/api/v1/admin/dispatcher/reindex-storage-vault":    true,
		"/api/v1/admin/dispatcher/run-backup":               true,
		"/api/v1/admin/dispatcher/run-backup-custom":        true,
		"/api/v1/admin/dispatcher/run-restore":              true,
		"/api/v1/admin/dispatcher/uninstall-software":       true,
		"/api/v1/admin/dispatcher/unlock":                   true,
		"/api/v1/admin/dispatcher/update-software":          true,
		"/api/v1/admin/job/abandon":                         true,
		"/api/v1/admin/job/cancel":                          true,
		"/api/v1/admin/request-storage-vault":               true,
		"/api/v1/admin/reset-user-password":                 true,
		"/api/v1/admin/revoke-device":                       true,
		"/api/v1/admin/set-user-profile":                    true,
	}
	credentialTier := map[string]bool{
		"users.get": true, "users.list-full": true, "user_groups.get": true, "organizations.list": true,
		"storage.buckets": true, "storage.bucket": true, "constellation.last-report": true,
		"policies.get": true, "policies.list-full": true, "protected_items.get": true,
	}
	seenWrites, seenTier := 0, 0
	for _, ep := range codeOrchEndpoints {
		switch {
		case writes[ep.Path]:
			seenWrites++
			if !ep.NeedsConfirm {
				t.Errorf("%s (%s) changes state but runs without confirm", ep.ID, ep.Path)
			}
		case credentialTier[ep.ID]:
			seenTier++
			if !ep.NeedsConfirm {
				t.Errorf("%s (%s) returns credential material but runs without confirm", ep.ID, ep.Path)
			}
			t.Setenv(cliutil.MCPReadOnlyEnvVar, "1")
			if !codeOrchRefuseForReadOnlySwitch(&ep, ep.Method, false) {
				t.Errorf("%s (%s) returns credential material but runs under %s=1", ep.ID, ep.Path, cliutil.MCPReadOnlyEnvVar)
			}
			t.Setenv(cliutil.MCPReadOnlyEnvVar, "")
		case ep.NeedsConfirm:
			t.Errorf("%s (%s) is a read but needs confirm", ep.ID, ep.Path)
		}
	}
	if seenWrites != len(writes) || seenTier != len(credentialTier) {
		t.Fatalf("found %d of %d writes and %d of %d credential-tier reads; the endpoint table moved", seenWrites, len(writes), seenTier, len(credentialTier))
	}
}
